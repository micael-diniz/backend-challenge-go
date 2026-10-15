//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestOutboxMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)

	applyMigrationFile(t, ctx, connection, "000007_outbox.up.sql")
	if !tableExists(t, ctx, connection, "outbox_events") {
		t.Error("outbox_events table does not exist after up migration")
	}

	applyMigrationFile(t, ctx, connection, "000007_outbox.down.sql")
	if tableExists(t, ctx, connection, "outbox_events") {
		t.Error("outbox_events table still exists after down migration")
	}
}

func TestOutboxAcceptsUnpublishedAndPublishedEvents(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareOutboxSchema(t, ctx, connection)

	if _, err := connection.Exec(ctx, `INSERT INTO outbox_events (
		id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
	) VALUES (
		'00000000-0000-0000-0000-000000000401',
		'00000000-0000-0000-0000-000000000201',
		'WagerTransactionProcessed',
		'{"eventType":"WagerTransactionProcessed"}',
		'2026-10-14T12:00:00Z',
		0,
		'2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("insert unpublished outbox event: %v", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO outbox_events (
		id, aggregate_id, event_type, payload, occurred_at, attempts, published_at
	) VALUES (
		'00000000-0000-0000-0000-000000000402',
		'00000000-0000-0000-0000-000000000001',
		'WalletBalanceChanged',
		'{"eventType":"WalletBalanceChanged"}',
		'2026-10-14T12:00:00Z',
		1,
		'2026-10-14T12:00:01Z'
	)`); err != nil {
		t.Fatalf("insert published outbox event: %v", err)
	}
}

func TestOutboxRejectsInvalidEvents(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		sql        string
		constraint string
	}{
		{
			name: "negative attempts",
			sql: `INSERT INTO outbox_events (
				id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
			) VALUES (
				'00000000-0000-0000-0000-000000000401', 'aggregate-1', 'WagerTransactionProcessed',
				'{}', '2026-10-14T12:00:00Z', -1, '2026-10-14T12:00:00Z'
			)`,
			constraint: "outbox_events_attempts_nonnegative",
		},
		{
			name: "published event still scheduled",
			sql: `INSERT INTO outbox_events (
				id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at, published_at
			) VALUES (
				'00000000-0000-0000-0000-000000000401', 'aggregate-1', 'WagerTransactionProcessed',
				'{}', '2026-10-14T12:00:00Z', 0, '2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
			)`,
			constraint: "outbox_events_publication",
		},
		{
			name: "unknown event type",
			sql: `INSERT INTO outbox_events (
				id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
			) VALUES (
				'00000000-0000-0000-0000-000000000401', 'aggregate-1', 'Unknown',
				'{}', '2026-10-14T12:00:00Z', 0, '2026-10-14T12:00:00Z'
			)`,
			constraint: "outbox_events_type_supported",
		},
		{
			name: "blank aggregate",
			sql: `INSERT INTO outbox_events (
				id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
			) VALUES (
				'00000000-0000-0000-0000-000000000401', '   ', 'WagerTransactionRejected',
				'{}', '2026-10-14T12:00:00Z', 0, '2026-10-14T12:00:00Z'
			)`,
			constraint: "outbox_events_aggregate_present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareOutboxSchema(t, ctx, connection)
			_, err := connection.Exec(ctx, tt.sql)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

func prepareOutboxSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000007_outbox.up.sql")
}
