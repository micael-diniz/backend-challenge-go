//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
)

func TestPostgresWalletOpeningStorePersistsZeroBalanceWallet(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "0.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	var (
		id           string
		player       string
		currency     string
		balanceMinor int64
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
	)
	if err := connection.QueryRow(ctx, `SELECT id::text, player_id::text, currency, balance_minor, version, created_at, updated_at FROM wallets`).Scan(
		&id, &player, &currency, &balanceMinor, &version, &createdAt, &updatedAt,
	); err != nil {
		t.Fatalf("read persisted wallet: %v", err)
	}
	if id != string(wallet.ID()) || player != string(playerID) || currency != "BRL" || balanceMinor != 0 || version != 1 {
		t.Errorf(
			"row = (%s, %s, %s, %d, %d); want (%s, %s, BRL, 0, 1)",
			id, player, currency, balanceMinor, version, wallet.ID(), playerID,
		)
	}
	if !createdAt.Equal(openedAt) || !updatedAt.Equal(openedAt) {
		t.Errorf("timestamps = (%s, %s); want both %s", createdAt, updatedAt, openedAt)
	}

	_, err = service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "0.00"))
	if !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second OpenWallet() error = %v; want ErrWalletAlreadyExists", err)
	}
	if count := countWallets(t, ctx, connection); count != 1 {
		t.Errorf("wallets count = %d; want 1", count)
	}
}

func TestPostgresWalletOpeningStoreRejectsCanceledContextWithoutInsert(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")

	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), time.Now)
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := service.OpenWallet(canceled, postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "0.00"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenWallet() error = %v; want context.Canceled", err)
	}
	if count := countWallets(t, ctx, connection); count != 0 {
		t.Errorf("wallets count = %d; want 0", count)
	}
}

func TestPostgresWalletOpeningStorePersistsPositiveOpening(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePositiveOpeningSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	if wallet.Balance().MinorUnits() != 10000 || wallet.Version() != 1 {
		t.Errorf("wallet = %d version %d; want 10000 version 1", wallet.Balance().MinorUnits(), wallet.Version())
	}

	var balanceMinor int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor); err != nil {
		t.Fatalf("read wallet balance: %v", err)
	}
	if balanceMinor != 10000 {
		t.Errorf("stored balance_minor = %d; want 10000", balanceMinor)
	}

	var kind, status string
	var amountMinor int64
	if err := connection.QueryRow(ctx, "SELECT kind, status, amount_minor FROM wager_transactions").Scan(&kind, &status, &amountMinor); err != nil {
		t.Fatalf("read opening: %v", err)
	}
	if kind != "OPENING" || status != "PROCESSED" || amountMinor != 10000 {
		t.Errorf("opening = (%s, %s, %d); want OPENING PROCESSED 10000", kind, status, amountMinor)
	}

	var direction string
	var beforeMinor, afterMinor int64
	if err := connection.QueryRow(ctx, "SELECT direction, balance_before_minor, balance_after_minor FROM wallet_ledger_entries").Scan(&direction, &beforeMinor, &afterMinor); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if direction != "CREDIT" || beforeMinor != 0 || afterMinor != 10000 {
		t.Errorf("ledger = (%s, %d, %d); want CREDIT 0 10000", direction, beforeMinor, afterMinor)
	}

	var outboxCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND attempts = 0").Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if outboxCount != 2 {
		t.Errorf("unpublished outbox events = %d; want 2", outboxCount)
	}

	_, err = service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second OpenWallet() error = %v; want ErrWalletAlreadyExists", err)
	}
	if count := countWallets(t, ctx, connection); count != 1 {
		t.Errorf("wallets count = %d; want 1", count)
	}
}

func TestPostgresWalletOpeningStorePersistsProcessedBet(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	openService := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt })
	betService := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := openService.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	updated, bet, _, err := betService.PlaceBet(ctx, application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
		CausationID:           "request-bet-1",
	})
	if err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	if updated.Balance().Amount() != "75.00" || updated.Version() != 2 {
		t.Errorf("wallet = %s version %d; want 75.00 version 2", updated.Balance().Amount(), updated.Version())
	}
	result, present := bet.ResultBalance()
	if bet.Status() != domain.WagerTransactionStatusProcessed || !present || result.Amount() != "75.00" {
		t.Errorf("bet result = (%s, %s, %t); want processed 75.00", bet.Status(), result.Amount(), present)
	}

	var balanceMinor, version int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if balanceMinor != 7500 || version != 2 {
		t.Errorf("stored wallet = (%d, %d); want (7500, 2)", balanceMinor, version)
	}
	var kind, status string
	var resultMinor int64
	if err := connection.QueryRow(ctx, "SELECT kind, status, result_balance_minor FROM wager_transactions WHERE kind = 'BET'").Scan(&kind, &status, &resultMinor); err != nil {
		t.Fatalf("read bet: %v", err)
	}
	if kind != "BET" || status != "PROCESSED" || resultMinor != 7500 {
		t.Errorf("stored bet = (%s, %s, %d); want BET PROCESSED 7500", kind, status, resultMinor)
	}
	var direction string
	var beforeMinor, afterMinor int64
	if err := connection.QueryRow(ctx, "SELECT direction, balance_before_minor, balance_after_minor FROM wallet_ledger_entries WHERE direction = 'DEBIT'").Scan(&direction, &beforeMinor, &afterMinor); err != nil {
		t.Fatalf("read debit: %v", err)
	}
	if beforeMinor != 10000 || afterMinor != 7500 {
		t.Errorf("debit = (%d, %d); want (10000, 7500)", beforeMinor, afterMinor)
	}
	var outboxCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events").Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outboxCount != 4 {
		t.Errorf("outbox events = %d; want 4", outboxCount)
	}
}

func TestPostgresWalletOpeningStoreRejectsStaleBetVersion(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	debited, err := wallet.Debit(amount, placedAt)
	if err != nil {
		t.Fatalf("Debit() returned an unexpected error: %v", err)
	}
	pending, err := domain.NewExternalWagerTransaction(domain.ExternalWagerTransactionInput{
		ID:                    "00000000-0000-0000-0000-000000000301",
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-stale",
		IdempotencyKey:        "provider-a:external-bet-stale",
		PayloadHash:           "hash-bet-stale",
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  domain.WagerTransactionKindBet,
		Money:                 amount,
		CreatedAt:             placedAt,
	})
	if err != nil {
		t.Fatalf("NewExternalWagerTransaction() returned an unexpected error: %v", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{ResultBalance: debited.Balance(), OccurredAt: placedAt})
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}
	ledger, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            "00000000-0000-0000-0000-000000000302",
		WalletID:      wallet.ID(),
		TransactionID: processed.ID(),
		Direction:     domain.LedgerDirectionDebit,
		Amount:        amount,
		BalanceBefore: wallet.Balance(),
		BalanceAfter:  debited.Balance(),
		CreatedAt:     placedAt,
	})
	if err != nil {
		t.Fatalf("NewWalletLedgerEntry() returned an unexpected error: %v", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       "00000000-0000-0000-0000-000000000303",
		CorrelationID: "correlation-1",
	}, processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	balanceEvent, err := domain.NewWalletBalanceChanged(domain.DomainEventMetadataInput{
		EventID:       "00000000-0000-0000-0000-000000000304",
		CorrelationID: "correlation-1",
	}, ledger, debited)
	if err != nil {
		t.Fatalf("NewWalletBalanceChanged() returned an unexpected error: %v", err)
	}

	err = store.SaveProcessedBet(ctx, wallet, processed, ledger, processedEvent, balanceEvent)
	if !errors.Is(err, application.ErrWalletVersionConflict) {
		t.Fatalf("SaveProcessedBet() error = %v; want version conflict", err)
	}
	var betCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET'").Scan(&betCount); err != nil {
		t.Fatalf("count bets: %v", err)
	}
	if betCount != 0 {
		t.Errorf("bet count = %d; want 0", betCount)
	}
	var version int64
	if err := connection.QueryRow(ctx, "SELECT version FROM wallets").Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 1 {
		t.Errorf("wallet version = %d; want 1", version)
	}
}

func TestPostgresWalletOpeningStoreReplaysBetAndRejectsChangedContent(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	betService := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	command := application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
	}
	if _, _, _, err := betService.PlaceBet(ctx, command); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	_, bet, replay, err := betService.PlaceBet(ctx, command)
	if err != nil {
		t.Fatalf("replay PlaceBet() returned an unexpected error: %v", err)
	}
	if !replay {
		t.Fatal("second PlaceBet() was not a replay")
	}
	result, present := bet.ResultBalance()
	if !present || result.Amount() != "75.00" {
		t.Errorf("replay result = (%s, %t); want 75.00", result.Amount(), present)
	}

	changed := command
	changedAmount, err := domain.NewMoney("10.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	changed.Money = changedAmount
	if _, _, _, err := betService.PlaceBet(ctx, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed content error = %v; want idempotency conflict", err)
	}
	otherKey := command
	otherKey.IdempotencyKey = "provider-a:other-key"
	if _, _, _, err := betService.PlaceBet(ctx, otherKey); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("other key error = %v; want idempotency conflict", err)
	}

	var balanceMinor, version, betCount int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET'").Scan(&betCount); err != nil {
		t.Fatalf("count bets: %v", err)
	}
	if balanceMinor != 7500 || version != 2 || betCount != 1 {
		t.Errorf("stored state = (%d, %d, %d); want (7500, 2, 1)", balanceMinor, version, betCount)
	}
}

func mustPostgresPlaceBetService(
	t *testing.T,
	store *postgres.WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) *application.PlaceBetService {
	t.Helper()
	service, err := application.NewPlaceBetService(store, newID, now)
	if err != nil {
		t.Fatalf("NewPlaceBetService() returned an unexpected error: %v", err)
	}
	return service
}

func prepareBetPersistenceSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	for _, name := range []string{
		"000001_wallets.up.sql",
		"000002_wager_transactions.up.sql",
		"000003_pending_bet.up.sql",
		"000004_external_identity.up.sql",
		"000005_processed_bet.up.sql",
		"000006_wallet_ledger.up.sql",
		"000007_outbox.up.sql",
		"000008_rejected_bet.up.sql",
	} {
		applyMigrationFile(t, ctx, connection, name)
	}
}

func preparePositiveOpeningSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
	applyMigrationFile(t, ctx, connection, "000002_wager_transactions.up.sql")
	applyMigrationFile(t, ctx, connection, "000006_wallet_ledger.up.sql")
	applyMigrationFile(t, ctx, connection, "000007_outbox.up.sql")
}

func mustPostgresOpenWalletService(
	t *testing.T,
	store *postgres.WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) *application.OpenWalletService {
	t.Helper()
	service, err := application.NewOpenWalletService(store, newID, now)
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	return service
}

func postgresOpenWalletCommand(t *testing.T, playerID domain.PlayerID, amount string) application.OpenWalletCommand {
	t.Helper()
	balance, err := domain.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	return application.OpenWalletCommand{
		PlayerID:       playerID,
		InitialBalance: balance,
		CorrelationID:  "correlation-1",
	}
}

func uuidSequence() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", next)
	}
}

func countWallets(t *testing.T, ctx context.Context, connection *pgx.Conn) int {
	t.Helper()
	var count int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallets").Scan(&count); err != nil {
		t.Fatalf("count wallets: %v", err)
	}
	return count
}
