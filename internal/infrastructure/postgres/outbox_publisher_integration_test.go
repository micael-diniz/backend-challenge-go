//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
)

func TestPostgresOutboxPublishesTheSameEventAfterALostConfirmation(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareOutboxSchema(t, ctx, connection)
	insertUnpublishedOutbox(t, ctx, connection)

	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := postgres.NewWalletOpeningStore(connection)
	var sent []string
	delivered, err := store.DeliverDue(ctx, now, func(message application.OutboxMessage) error {
		sent = append(sent, message.ID)
		return errors.New("broker accepted the event and the confirmation was lost")
	})
	if err != nil || !delivered || len(sent) != 1 {
		t.Fatalf("first delivery = %t %v sent %v", delivered, err, sent)
	}

	delivered, err = store.DeliverDue(ctx, now, func(application.OutboxMessage) error {
		t.Fatal("event was retried before the backoff")
		return nil
	})
	if err != nil || delivered {
		t.Fatalf("early retry = %t %v", delivered, err)
	}

	delivered, err = store.DeliverDue(ctx, now.Add(time.Second), func(message application.OutboxMessage) error {
		sent = append(sent, message.ID)
		return nil
	})
	if err != nil || !delivered || len(sent) != 2 || sent[0] != sent[1] {
		t.Fatalf("republish = %t %v sent %v", delivered, err, sent)
	}
	assertOutboxPublished(t, ctx, connection, sent[0])
}

func TestPostgresOutboxLockLetsAnotherPublisherResumeAfterRollback(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareOutboxSchema(t, ctx, connection)
	insertUnpublishedOutbox(t, ctx, connection)
	second := openSecondDatabase(t, ctx)

	holding := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		tx, err := connection.Begin(ctx)
		if err != nil {
			finished <- err
			return
		}
		var id string
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM outbox_events
			WHERE published_at IS NULL
			FOR UPDATE`).Scan(&id)
		if err != nil {
			finished <- err
			return
		}
		close(holding)
		<-release
		finished <- tx.Rollback(ctx)
	}()
	<-holding

	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	claimCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	delivered, err := postgres.NewWalletOpeningStore(second).DeliverDue(claimCtx, now, func(application.OutboxMessage) error {
		t.Errorf("locked event was delivered twice")
		return nil
	})
	if err != nil || delivered {
		t.Errorf("second publisher = %t %v", delivered, err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("abandoned claim: %v", err)
	}

	var sent []string
	delivered, err = postgres.NewWalletOpeningStore(second).DeliverDue(ctx, now, func(message application.OutboxMessage) error {
		sent = append(sent, message.ID)
		return nil
	})
	if err != nil || !delivered || len(sent) != 1 || sent[0] != "00000000-0000-0000-0000-000000000401" {
		t.Fatalf("resume = %t %v sent %v", delivered, err, sent)
	}
	assertOutboxPublished(t, ctx, second, sent[0])
}

func insertUnpublishedOutbox(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	if _, err := connection.Exec(ctx, `INSERT INTO outbox_events (
		id, aggregate_id, event_type, payload, occurred_at, attempts, next_attempt_at
	) VALUES (
		'00000000-0000-0000-0000-000000000401',
		'00000000-0000-0000-0000-000000000201',
		'WagerTransactionProcessed',
		'{"eventId":"00000000-0000-0000-0000-000000000401"}',
		'2026-10-14T12:00:00Z',
		0,
		'2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("insert unpublished outbox event: %v", err)
	}
}

func assertOutboxPublished(t *testing.T, ctx context.Context, connection *pgx.Conn, id string) {
	t.Helper()
	var publishedAt *time.Time
	var nextAttempt *time.Time
	if err := connection.QueryRow(ctx, `
		SELECT published_at, next_attempt_at
		FROM outbox_events
		WHERE id = $1::uuid`, id).Scan(&publishedAt, &nextAttempt); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if publishedAt == nil || nextAttempt != nil {
		t.Fatalf("published_at %v next_attempt_at %v", publishedAt, nextAttempt)
	}
}

func openSecondDatabase(t *testing.T, ctx context.Context) *pgx.Conn {
	t.Helper()
	connection, err := pgx.Connect(ctx, os.Getenv(testDatabaseURLEnvironment))
	if err != nil {
		t.Fatalf("second PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(ctx); err != nil {
			t.Errorf("close second PostgreSQL connection: %v", err)
		}
	})
	return connection
}
