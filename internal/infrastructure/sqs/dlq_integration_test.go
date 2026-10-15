//go:build integration

package sqs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestInvalidMessageMovesToTheDLQAfterFiveReceives(t *testing.T) {
	endpoint := os.Getenv("SQS_ENDPOINT")
	if endpoint == "" {
		t.Skip("SQS_ENDPOINT is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	api := NewAPI(endpoint)
	wagers, _, err := api.Open(ctx)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	deadLetter, err := api.deadLetter(ctx)
	if err != nil {
		t.Fatalf("deadLetter() error = %v", err)
	}

	body := fmt.Sprintf("not-json-%d", time.Now().UnixNano())
	if err := wagers.Send(ctx, body, body, body); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	receives := 0
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if receives >= maxReceiveCount {
			message, ok, err := deadLetter.Receive(ctx)
			if err != nil {
				t.Fatalf("DLQ Receive() error = %v", err)
			}
			if ok && string(message.Body) == body {
				if err := deadLetter.Delete(ctx, message.Receipt); err != nil {
					t.Fatalf("DLQ Delete() error = %v", err)
				}
				return
			}
			if ok {
				if err := deadLetter.release(ctx, message.Receipt); err != nil {
					t.Fatalf("release DLQ message: %v", err)
				}
			}
		}

		message, ok, err := wagers.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		if !ok {
			continue
		}
		if string(message.Body) != body {
			if err := wagers.release(ctx, message.Receipt); err != nil {
				t.Fatalf("release other message: %v", err)
			}
			continue
		}
		receives++
		if err := wagers.release(ctx, message.Receipt); err != nil && receives <= maxReceiveCount {
			t.Fatalf("release invalid message: %v", err)
		}
	}
	t.Fatalf("invalid message did not reach the DLQ after %d receives", receives)
}
