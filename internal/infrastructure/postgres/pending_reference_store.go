package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func (store *WalletOpeningStore) SavePendingReference(
	ctx context.Context,
	transaction domain.WagerTransaction,
	pendingEvent domain.WagerTransactionPendingReference,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(pendingEvent)
	if err != nil {
		return fmt.Errorf("serialize pending reference event: %w", err)
	}
	nextAttemptAt, _ := transaction.NextReferenceAttemptAt()
	expiresAt, _ := transaction.ReferenceExpiresAt()
	referenceExternal, _ := transaction.ReferenceExternalTransactionID()
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pending reference: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key, payload_hash,
			wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
			reference_external_transaction_id, status, reference_attempts,
			next_reference_attempt_at, reference_expires_at, created_at, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5,
			$6::uuid, $7::uuid, $8, $9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19
		)`,
		string(transaction.ID()),
		string(transaction.ProviderID()),
		string(transaction.ExternalTransactionID()),
		string(transaction.IdempotencyKey()),
		string(transaction.PayloadHash()),
		string(transaction.WalletID()),
		string(transaction.PlayerID()),
		string(transaction.RoundID()),
		string(transaction.GameID()),
		string(transaction.Kind()),
		transaction.Money().MinorUnits(),
		transaction.Money().Currency().String(),
		string(referenceExternal),
		string(transaction.Status()),
		transaction.ReferenceAttempts(),
		nextAttemptAt,
		expiresAt,
		transaction.CreatedAt(),
		transaction.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("insert pending reference: %w", err)
	}
	if err := insertOutboxEvent(ctx, tx, pendingEvent.EventID(), pendingEvent.AggregateID(), pendingEvent.EventType(), payload, pendingEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pending reference: %w", err)
	}
	return nil
}

func (store *WalletOpeningStore) ListDuePendingReferences(
	ctx context.Context,
	dueAt time.Time,
) ([]domain.WagerTransaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := store.connection.Query(ctx, wagerColumnsSQL+`
		WHERE status = 'PENDING_REFERENCE' AND next_reference_attempt_at <= $1
		ORDER BY next_reference_attempt_at, id`, dueAt)
	if err != nil {
		return nil, fmt.Errorf("list pending references: %w", err)
	}
	defer rows.Close()
	var due []domain.WagerTransaction
	for rows.Next() {
		transaction, err := scanWagerTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("list pending references: %w", err)
		}
		due = append(due, transaction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending references: %w", err)
	}
	return due, nil
}

func (store *WalletOpeningStore) SavePendingReferenceRetry(
	ctx context.Context,
	transaction domain.WagerTransaction,
	pendingEvent domain.WagerTransactionPendingReference,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(pendingEvent)
	if err != nil {
		return fmt.Errorf("serialize pending reference retry: %w", err)
	}
	nextAttemptAt, _ := transaction.NextReferenceAttemptAt()
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pending reference retry: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	tag, err := tx.Exec(ctx, `
		UPDATE wager_transactions
		SET reference_attempts = $2, next_reference_attempt_at = $3, updated_at = $4
		WHERE id = $1::uuid AND status = 'PENDING_REFERENCE'`,
		string(transaction.ID()),
		transaction.ReferenceAttempts(),
		nextAttemptAt,
		transaction.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("update pending reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update pending reference: %w: transaction ID=%q", errTransactionNotWaiting, transaction.ID())
	}
	if err := insertOutboxEvent(ctx, tx, pendingEvent.EventID(), pendingEvent.AggregateID(), pendingEvent.EventType(), payload, pendingEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pending reference retry: %w", err)
	}
	return nil
}

func (store *WalletOpeningStore) RejectPendingReference(
	ctx context.Context,
	transaction domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(rejectedEvent)
	if err != nil {
		return fmt.Errorf("serialize rejected reference event: %w", err)
	}
	code, _ := transaction.FailureCode()
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rejected reference: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	tag, err := tx.Exec(ctx, `
		UPDATE wager_transactions
		SET status = 'REJECTED', failure_code = $2, next_reference_attempt_at = NULL, updated_at = $3
		WHERE id = $1::uuid AND status = 'PENDING_REFERENCE'`,
		string(transaction.ID()),
		string(code),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("reject pending reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("reject pending reference: %w: transaction ID=%q", errTransactionNotWaiting, transaction.ID())
	}
	if err := insertOutboxEvent(ctx, tx, rejectedEvent.EventID(), rejectedEvent.AggregateID(), rejectedEvent.EventType(), payload, rejectedEvent.OccurredAt()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rejected reference: %w", err)
	}
	return nil
}

func (store *WalletOpeningStore) ResolvePendingReference(
	ctx context.Context,
	wallet domain.Wallet,
	transaction domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resultBalance, present := transaction.ResultBalance()
	if !present {
		return fmt.Errorf("resolve pending reference: %w", errResultBalanceAbsent)
	}
	processedPayload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize resolved reference event: %w", err)
	}
	balancePayload, err := json.Marshal(balanceChangedEvent)
	if err != nil {
		return fmt.Errorf("serialize resolved balance event: %w", err)
	}
	resolvedReference, _ := transaction.ResolvedReferenceTransactionID()
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin resolved reference: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := updateWallet(ctx, tx, wallet); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE wager_transactions
		SET status = 'PROCESSED',
			result_balance_minor = $2,
			result_balance_currency = $3,
			resolved_reference_transaction_id = $4::uuid,
			next_reference_attempt_at = NULL,
			updated_at = $5
		WHERE id = $1::uuid AND status = 'PENDING_REFERENCE'`,
		string(transaction.ID()),
		resultBalance.MinorUnits(),
		resultBalance.Currency().String(),
		string(resolvedReference),
		transaction.UpdatedAt(),
	)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == postgresUniqueViolation && databaseError.ConstraintName == "wager_transactions_processed_reversal_reference_unique" {
			return fmt.Errorf("%w", application.ErrDuplicateReversal)
		}
		return fmt.Errorf("resolve pending reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resolve pending reference: %w: transaction ID=%q", errTransactionNotWaiting, transaction.ID())
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
		return fmt.Errorf("commit resolved reference: %w", err)
	}
	return nil
}
