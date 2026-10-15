package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidOutboxPublisher = errors.New("invalid outbox publisher")

type OutboxMessage struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     []byte
	Attempts    int
	OccurredAt  time.Time
}

type OutboxStore interface {
	DeliverDue(ctx context.Context, now time.Time, publish func(OutboxMessage) error) (bool, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, message OutboxMessage) error
}

type OutboxPublisher struct {
	store       OutboxStore
	destination EventPublisher
	now         func() time.Time
}

func NewOutboxPublisher(store OutboxStore, destination EventPublisher, now func() time.Time) (*OutboxPublisher, error) {
	if store == nil || destination == nil || now == nil {
		return nil, fmt.Errorf("%w: store, destination, and clock are required", ErrInvalidOutboxPublisher)
	}
	return &OutboxPublisher{store: store, destination: destination, now: now}, nil
}

func (publisher *OutboxPublisher) PublishDue(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		delivered, err := publisher.store.DeliverDue(ctx, publisher.now(), func(message OutboxMessage) error {
			return publisher.destination.Publish(ctx, message)
		})
		if err != nil || !delivered {
			return err
		}
	}
}

func OutboxRetryDelay(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 6 {
		attempts = 6
	}
	delay := time.Second << attempts
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
