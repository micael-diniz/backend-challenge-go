package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

const postgresUniqueViolation = "23505"

type database interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type WalletOpeningStore struct {
	connection database
}

type rowExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func NewWalletOpeningStore(connection database) *WalletOpeningStore {
	return &WalletOpeningStore{connection: connection}
}

func (store *WalletOpeningStore) CreateWallet(
	ctx context.Context,
	wallet domain.Wallet,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return insertWallet(ctx, store.connection, wallet)
}

func (store *WalletOpeningStore) CreateWalletWithOpening(
	ctx context.Context,
	wallet domain.Wallet,
	opening domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	processedPayload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize processed opening event: %w", err)
	}
	balancePayload, err := json.Marshal(balanceChangedEvent)
	if err != nil {
		return fmt.Errorf("serialize opening balance event: %w", err)
	}

	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin wallet opening: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := insertWallet(ctx, tx, wallet); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, wallet_id, player_id, kind, amount_minor, currency, status, created_at, updated_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9
		)`,
		string(opening.ID()),
		string(opening.WalletID()),
		string(opening.PlayerID()),
		string(opening.Kind()),
		opening.Money().MinorUnits(),
		opening.Money().Currency().String(),
		string(opening.Status()),
		opening.CreatedAt(),
		opening.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("insert opening transaction: %w", err)
	}
	if err := insertLedger(ctx, tx, ledger); err != nil {
		return err
	}
	if err := insertOutboxEvent(ctx, tx, processedEvent.EventID(), processedEvent.AggregateID(), processedEvent.EventType(), processedPayload, processedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := insertOutboxEvent(ctx, tx, balanceChangedEvent.EventID(), balanceChangedEvent.AggregateID(), balanceChangedEvent.EventType(), balancePayload, balanceChangedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit wallet opening: %w", err)
	}
	return nil
}

func (store *WalletOpeningStore) LoadWallet(
	ctx context.Context,
	walletID domain.WalletID,
) (domain.Wallet, error) {
	if err := ctx.Err(); err != nil {
		return domain.Wallet{}, err
	}

	var (
		playerID     string
		currency     string
		balanceMinor int64
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
	)
	err := store.connection.QueryRow(ctx, `
		SELECT player_id::text, currency, balance_minor, version, created_at, updated_at
		FROM wallets
		WHERE id = $1::uuid`,
		string(walletID),
	).Scan(&playerID, &currency, &balanceMinor, &version, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wallet{}, fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, walletID)
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("load wallet: %w", err)
	}

	balance, err := moneyFromMinor(balanceMinor, currency)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("load wallet balance: %w", err)
	}
	wallet, err := domain.RehydrateWallet(
		walletID,
		domain.PlayerID(playerID),
		balance,
		version,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("rehydrate wallet: %w", err)
	}
	return wallet, nil
}

func (store *WalletOpeningStore) FindBetByIdempotencyKey(
	ctx context.Context,
	providerID domain.ProviderID,
	key domain.IdempotencyKey,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, providerID, "idempotency_key", string(key))
}

func (store *WalletOpeningStore) FindBetByExternalID(
	ctx context.Context,
	providerID domain.ProviderID,
	externalID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, providerID, "external_transaction_id", string(externalID))
}

func (store *WalletOpeningStore) findBet(
	ctx context.Context,
	providerID domain.ProviderID,
	column string,
	value string,
) (domain.WagerTransaction, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.WagerTransaction{}, false, err
	}

	var query string
	switch column {
	case "idempotency_key":
		query = betLookupSQL + " AND idempotency_key = $2"
	case "external_transaction_id":
		query = betLookupSQL + " AND external_transaction_id = $2"
	default:
		return domain.WagerTransaction{}, false, fmt.Errorf("find bet: unsupported column %q", column)
	}

	var (
		id, externalID, idempotencyKey, payloadHash string
		walletID, playerID, roundID, gameID         string
		kind, currency, status                      string
		amountMinor                                 int64
		failureCode                                 *string
		resultMinor                                 *int64
		resultCurrency                              *string
		createdAt, updatedAt                        time.Time
	)
	err := store.connection.QueryRow(ctx, query, string(providerID), value).Scan(
		&id, &externalID, &idempotencyKey, &payloadHash,
		&walletID, &playerID, &roundID, &gameID,
		&kind, &amountMinor, &currency, &status, &failureCode,
		&resultMinor, &resultCurrency, &createdAt, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WagerTransaction{}, false, nil
	}
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("find bet: %w", err)
	}

	money, err := moneyFromMinor(amountMinor, currency)
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("find bet amount: %w", err)
	}
	var resultBalance domain.Money
	if resultMinor != nil {
		currencyValue := ""
		if resultCurrency != nil {
			currencyValue = *resultCurrency
		}
		resultBalance, err = moneyFromMinor(*resultMinor, currencyValue)
		if err != nil {
			return domain.WagerTransaction{}, false, fmt.Errorf("find bet result: %w", err)
		}
	}
	var rejectionCode domain.WagerTransactionFailureCode
	if failureCode != nil {
		rejectionCode = domain.WagerTransactionFailureCode(*failureCode)
	}
	bet, err := domain.RehydrateWagerTransaction(domain.RehydrateWagerTransactionInput{
		ID:                    domain.TransactionID(id),
		ProviderID:            providerID,
		ExternalTransactionID: domain.ExternalTransactionID(externalID),
		IdempotencyKey:        domain.IdempotencyKey(idempotencyKey),
		PayloadHash:           domain.PayloadHash(payloadHash),
		WalletID:              domain.WalletID(walletID),
		PlayerID:              domain.PlayerID(playerID),
		RoundID:               domain.RoundID(roundID),
		GameID:                domain.GameID(gameID),
		Kind:                  domain.WagerTransactionKind(kind),
		Money:                 money,
		Status:                domain.WagerTransactionStatus(status),
		FailureCode:           rejectionCode,
		ResultBalance:         resultBalance,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	})
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("rehydrate bet: %w", err)
	}
	return bet, true, nil
}

const betLookupSQL = `
	SELECT id::text, external_transaction_id, idempotency_key, payload_hash,
		wallet_id::text, player_id::text, round_id, game_id, kind,
		amount_minor, currency, status, failure_code, result_balance_minor, result_balance_currency,
		created_at, updated_at
	FROM wager_transactions
	WHERE provider_id = $1`

func (store *WalletOpeningStore) SaveProcessedBet(
	ctx context.Context,
	wallet domain.Wallet,
	bet domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resultBalance, present := bet.ResultBalance()
	if !present {
		return fmt.Errorf("save processed bet: result balance is absent")
	}
	processedPayload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize processed bet event: %w", err)
	}
	balancePayload, err := json.Marshal(balanceChangedEvent)
	if err != nil {
		return fmt.Errorf("serialize bet balance event: %w", err)
	}

	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin processed bet: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx, `
		UPDATE wallets
		SET balance_minor = $2, version = $3, updated_at = $4
		WHERE id = $1::uuid AND version = $5`,
		string(wallet.ID()),
		wallet.Balance().MinorUnits(),
		wallet.Version(),
		wallet.UpdatedAt(),
		wallet.Version()-1,
	)
	if err != nil {
		return fmt.Errorf("update wallet: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: wallet ID=%q version=%d", application.ErrWalletVersionConflict, wallet.ID(), wallet.Version())
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key, payload_hash,
			wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
			status, result_balance_minor, result_balance_currency, reference_attempts,
			created_at, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5,
			$6::uuid, $7::uuid, $8, $9, $10, $11, $12,
			$13, $14, $15, 0,
			$16, $17
		)`,
		string(bet.ID()),
		string(bet.ProviderID()),
		string(bet.ExternalTransactionID()),
		string(bet.IdempotencyKey()),
		string(bet.PayloadHash()),
		string(bet.WalletID()),
		string(bet.PlayerID()),
		string(bet.RoundID()),
		string(bet.GameID()),
		string(bet.Kind()),
		bet.Money().MinorUnits(),
		bet.Money().Currency().String(),
		string(bet.Status()),
		resultBalance.MinorUnits(),
		resultBalance.Currency().String(),
		bet.CreatedAt(),
		bet.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("insert processed bet: %w", err)
	}
	if err := insertLedger(ctx, tx, ledger); err != nil {
		return err
	}
	if err := insertOutboxEvent(ctx, tx, processedEvent.EventID(), processedEvent.AggregateID(), processedEvent.EventType(), processedPayload, processedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := insertOutboxEvent(ctx, tx, balanceChangedEvent.EventID(), balanceChangedEvent.AggregateID(), balanceChangedEvent.EventType(), balancePayload, balanceChangedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit processed bet: %w", err)
	}
	return nil
}

func (store *WalletOpeningStore) SaveRejectedBet(
	ctx context.Context,
	bet domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(rejectedEvent)
	if err != nil {
		return fmt.Errorf("serialize rejected bet event: %w", err)
	}
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rejected bet: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key, payload_hash,
			wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
			status, failure_code, reference_attempts, created_at, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5,
			$6::uuid, $7::uuid, $8, $9, $10, $11, $12,
			$13, $14, 0, $15, $16
		)`,
		string(bet.ID()),
		string(bet.ProviderID()),
		string(bet.ExternalTransactionID()),
		string(bet.IdempotencyKey()),
		string(bet.PayloadHash()),
		string(bet.WalletID()),
		string(bet.PlayerID()),
		string(bet.RoundID()),
		string(bet.GameID()),
		string(bet.Kind()),
		bet.Money().MinorUnits(),
		bet.Money().Currency().String(),
		string(bet.Status()),
		betFailureCode(bet),
		bet.CreatedAt(),
		bet.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("insert rejected bet: %w", err)
	}
	if err := insertOutboxEvent(ctx, tx, rejectedEvent.EventID(), rejectedEvent.AggregateID(), rejectedEvent.EventType(), payload, rejectedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rejected bet: %w", err)
	}
	return nil
}

func betFailureCode(bet domain.WagerTransaction) string {
	code, _ := bet.FailureCode()
	return code.String()
}

func insertWallet(ctx context.Context, executor rowExecutor, wallet domain.Wallet) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO wallets (
			id, player_id, currency, balance_minor, version, created_at, updated_at
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6, $7
		)`,
		string(wallet.ID()),
		string(wallet.PlayerID()),
		wallet.Currency().String(),
		wallet.Balance().MinorUnits(),
		wallet.Version(),
		wallet.CreatedAt(),
		wallet.UpdatedAt(),
	)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == postgresUniqueViolation {
			return fmt.Errorf(
				"%w: constraint=%s",
				application.ErrWalletAlreadyExists,
				databaseError.ConstraintName,
			)
		}
		return fmt.Errorf("insert wallet: %w", err)
	}
	return nil
}

func insertLedger(ctx context.Context, executor rowExecutor, ledger domain.WalletLedgerEntry) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO wallet_ledger_entries (
			id, wallet_id, transaction_id, direction, amount_minor,
			balance_before_minor, balance_after_minor, currency, created_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9
		)`,
		string(ledger.ID()),
		string(ledger.WalletID()),
		string(ledger.TransactionID()),
		string(ledger.Direction()),
		ledger.Money().MinorUnits(),
		ledger.BalanceBefore().MinorUnits(),
		ledger.BalanceAfter().MinorUnits(),
		ledger.Money().Currency().String(),
		ledger.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("insert ledger: %w", err)
	}
	return nil
}

func moneyFromMinor(minor int64, currency string) (domain.Money, error) {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return domain.NewMoney(fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100), currency)
}

func insertOutboxEvent(
	ctx context.Context,
	executor rowExecutor,
	eventID domain.DomainEventID,
	aggregateID domain.AggregateID,
	eventType domain.DomainEventType,
	payload []byte,
	occurredAt time.Time,
) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO outbox_events (
			id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, 0, $5
		)`,
		string(eventID),
		string(aggregateID),
		string(eventType),
		payload,
		occurredAt,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}
