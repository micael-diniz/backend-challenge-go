package application

import (
	"context"
	"testing"
	"time"
)

func TestPublishDueSendsEachUnpublishedEventOnce(t *testing.T) {
	t.Parallel()

	store := &outboxStub{messages: []OutboxMessage{{
		ID:          "00000000-0000-0000-0000-000000000401",
		AggregateID: "wallet-1",
		EventType:   "WagerTransactionProcessed",
		Payload:     []byte(`{"eventId":"00000000-0000-0000-0000-000000000401"}`),
	}}}
	destination := &eventSink{}
	publisher := mustNewOutboxPublisher(t, store, destination)

	if err := publisher.PublishDue(context.Background()); err != nil {
		t.Fatalf("PublishDue() error = %v", err)
	}
	if len(destination.ids) != 1 || destination.ids[0] != store.sent[0] {
		t.Fatalf("published %v; stored %v", destination.ids, store.sent)
	}
}

func TestOutboxRetryDelayDoublesUntilOneMinute(t *testing.T) {
	t.Parallel()

	if OutboxRetryDelay(0) != time.Second || OutboxRetryDelay(1) != 2*time.Second || OutboxRetryDelay(2) != 4*time.Second {
		t.Fatalf("delays = %s, %s, %s", OutboxRetryDelay(0), OutboxRetryDelay(1), OutboxRetryDelay(2))
	}
	if OutboxRetryDelay(6) != time.Minute || OutboxRetryDelay(30) != time.Minute {
		t.Fatalf("capped delays = %s, %s", OutboxRetryDelay(6), OutboxRetryDelay(30))
	}
}

func TestConsumeFinishesTheMessageItAlreadyReceived(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	queue := &blockingQueue{
		message: QueuedMessage{ID: "msg-1", Receipt: "receipt-1", Body: wagerMessageBody("msg-1")},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		done <- Consume(ctx, queue, inboxHandler{})
	}()
	<-queue.started
	cancel()
	close(queue.release)
	if err := <-done; err != nil || !queue.deleted || queue.receives != 1 {
		t.Fatalf("deleted %t receives %d error %v", queue.deleted, queue.receives, err)
	}
}

type outboxStub struct {
	messages []OutboxMessage
	sent     []string
}

func (stub *outboxStub) DeliverDue(_ context.Context, _ time.Time, publish func(OutboxMessage) error) (bool, error) {
	if len(stub.messages) == 0 {
		return false, nil
	}
	message := stub.messages[0]
	stub.messages = stub.messages[1:]
	if err := publish(message); err != nil {
		return false, err
	}
	stub.sent = append(stub.sent, message.ID)
	return true, nil
}

type eventSink struct {
	ids []string
}

func (sink *eventSink) Publish(_ context.Context, message OutboxMessage) error {
	sink.ids = append(sink.ids, message.ID)
	return nil
}

func mustNewOutboxPublisher(t *testing.T, store OutboxStore, destination EventPublisher) *OutboxPublisher {
	t.Helper()
	publisher, err := NewOutboxPublisher(store, destination, time.Now)
	if err != nil {
		t.Fatalf("NewOutboxPublisher() error = %v", err)
	}
	return publisher
}

type blockingQueue struct {
	message  QueuedMessage
	started  chan struct{}
	release  chan struct{}
	deleted  bool
	receives int
}

func (queue *blockingQueue) Receive(context.Context) (QueuedMessage, bool, error) {
	queue.receives++
	if queue.receives == 1 {
		close(queue.started)
		<-queue.release
		return queue.message, true, nil
	}
	return QueuedMessage{}, false, context.Canceled
}

func (queue *blockingQueue) Delete(context.Context, string) error {
	queue.deleted = true
	return nil
}
