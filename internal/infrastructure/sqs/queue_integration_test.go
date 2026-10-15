//go:build integration

package sqs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/sqs"
)

func TestLocalStackDeletesAWagerAfterTheHandlerCommits(t *testing.T) {
	endpoint := os.Getenv("SQS_ENDPOINT")
	if endpoint == "" {
		t.Skip("SQS_ENDPOINT is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	wagers, events, err := sqs.NewAPI(endpoint).Open(ctx)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	body := []byte(`{"messageId":"msg-123","type":"WagerTransactionRequested","occurredAt":"2026-09-08T12:00:00.000Z","data":{"providerId":"provider-a","externalTransactionId":"transaction-123","idempotencyKey":"provider-a:transaction-123","playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","walletId":"0192f291-27dd-7d3f-8071-5f8685deef37","roundId":"round-987","gameId":"fortune-chimp","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}}`)
	if err := wagers.Send(ctx, string(body), "0192f291-27dd-7d3f-8071-5f8685deef37", "msg-123"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	handler := &recordingHandler{done: make(chan struct{})}
	consumeCtx, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() {
		finished <- application.Consume(consumeCtx, wagers, handler)
	}()
	select {
	case <-handler.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for the wager message")
	}
	stop()
	if err := <-finished; err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if handler.id != "msg-123" {
		t.Fatalf("handled id = %s", handler.id)
	}

	if err := events.Publish(ctx, application.OutboxMessage{
		ID:          "00000000-0000-0000-0000-000000000401",
		AggregateID: "00000000-0000-0000-0000-000000000201",
		Payload:     []byte(`{"eventId":"00000000-0000-0000-0000-000000000401"}`),
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	message, ok, err := events.Receive(ctx)
	if err != nil || !ok || string(message.Body) != `{"eventId":"00000000-0000-0000-0000-000000000401"}` {
		t.Fatalf("event = %s ok %t error %v", message.Body, ok, err)
	}
}

type recordingHandler struct {
	id   string
	done chan struct{}
}

func (handler *recordingHandler) Receive(_ context.Context, message application.WagerMessage) (domain.WagerTransaction, error) {
	handler.id = message.ID
	close(handler.done)
	return domain.WagerTransaction{}, nil
}
