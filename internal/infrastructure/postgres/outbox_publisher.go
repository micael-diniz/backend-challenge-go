package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
)

func (store *WalletOpeningStore) DeliverDue(
	ctx context.Context,
	now time.Time,
	publish func(application.OutboxMessage) error,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	tx, err := store.connection.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("deliver outbox: %w", err)
	}
	defer tx.Rollback(ctx)

	var message application.OutboxMessage
	err = tx.QueryRow(ctx, `
		SELECT id::text, aggregate_id, event_type, payload, attempts, occurred_at
		FROM outbox_events
		WHERE published_at IS NULL AND next_attempt_at <= $1
		ORDER BY next_attempt_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, now).Scan(
		&message.ID,
		&message.AggregateID,
		&message.EventType,
		&message.Payload,
		&message.Attempts,
		&message.OccurredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("deliver outbox: %w", err)
	}

	if err := publish(message); err != nil {
		application.RecordOutboxRetry()
		if _, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET attempts = attempts + 1, next_attempt_at = $2
			WHERE id = $1::uuid`, message.ID, now.Add(application.OutboxRetryDelay(message.Attempts))); err != nil {
			return false, fmt.Errorf("deliver outbox: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("deliver outbox: %w", err)
		}
		return true, nil
	}
	application.RecordOutboxLag(now.Sub(message.OccurredAt))

	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET published_at = $2, next_attempt_at = NULL
		WHERE id = $1::uuid`, message.ID, now); err != nil {
		return false, fmt.Errorf("deliver outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("deliver outbox: %w", err)
	}
	return true, nil
}
