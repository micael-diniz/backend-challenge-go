package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestReceiveWagerMessageDebitsOnceWhenTheSameMessageArrivesAgain(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	bets := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
	), func() time.Time { return openedAt.Add(time.Second) })
	applier := &countingApplier{inner: bets}
	inbox := &inboxStub{}
	service := mustNewInboxService(t, applier, inbox)
	message := wagerMessage(t, wallet, "25.00")

	first, err := service.Receive(context.Background(), message)
	if err != nil || first.ID() == "" || store.wallet.Balance().Amount() != "75.00" || applier.calls != 1 {
		t.Fatalf("first = %s balance %s calls %d error %v", first.ID(), store.wallet.Balance().Amount(), applier.calls, err)
	}
	if _, err := service.Receive(context.Background(), message); err != nil || applier.calls != 1 || store.wallet.Balance().Amount() != "75.00" || inbox.saves != 1 || inbox.record.TransactionID != first.ID() {
		t.Fatalf("redelivery balance %s calls %d saves %d stored %s error %v", store.wallet.Balance().Amount(), applier.calls, inbox.saves, inbox.record.TransactionID, err)
	}

	changed := message
	changed.Command.Money = mustApplicationMoney(t, "10.00", "BRL")
	if _, err := service.Receive(context.Background(), changed); !errors.Is(err, ErrInboxConflict) || applier.calls != 1 || store.wallet.Balance().Amount() != "75.00" {
		t.Fatalf("conflict error %v calls %d balance %s", err, applier.calls, store.wallet.Balance().Amount())
	}
}

func TestConsumeDeletesTheMessageOnlyAfterDurableHandling(t *testing.T) {
	t.Parallel()

	queue := &queueStub{message: QueuedMessage{ID: "msg-1", Receipt: "receipt-1", Body: wagerMessageBody("msg-1")}}
	if err := ConsumeOne(context.Background(), queue, inboxHandler{err: nil}); err != nil || !queue.deleted {
		t.Fatalf("success deleted %t error %v", queue.deleted, err)
	}

	queue = &queueStub{message: QueuedMessage{ID: "msg-2", Receipt: "receipt-2", Body: wagerMessageBody("msg-2")}}
	if err := ConsumeOne(context.Background(), queue, inboxHandler{err: ErrPlaceBetPersistence}); !errors.Is(err, ErrPlaceBetPersistence) || queue.deleted {
		t.Fatalf("retry deleted %t error %v", queue.deleted, err)
	}

	queue = &queueStub{message: QueuedMessage{ID: "msg-3", Receipt: "receipt-3", Body: wagerMessageBody("msg-3")}}
	if err := ConsumeOne(context.Background(), queue, inboxHandler{err: domain.ErrInsufficientFunds}); err != nil || !queue.deleted {
		t.Fatalf("rejection deleted %t error %v", queue.deleted, err)
	}
}

type countingApplier struct {
	inner *PlaceBetService
	calls int
}

func (applier *countingApplier) Apply(ctx context.Context, command PlaceBetCommand, kind domain.WagerTransactionKind) (domain.WagerTransaction, bool, error) {
	applier.calls++
	return applier.inner.Apply(ctx, command, kind)
}

type inboxStub struct {
	record InboxRecord
	found  bool
	saves  int
}

func (stub *inboxStub) FindInbox(context.Context, string) (InboxRecord, bool, error) {
	return stub.record, stub.found, nil
}

func (stub *inboxStub) SaveInbox(_ context.Context, record InboxRecord) error {
	stub.saves++
	stub.record = record
	stub.found = true
	return nil
}

type queueStub struct {
	message QueuedMessage
	deleted bool
}

func (queue *queueStub) Receive(context.Context) (QueuedMessage, bool, error) {
	if queue.message.ID == "" {
		return QueuedMessage{}, false, nil
	}
	return queue.message, true, nil
}

func (queue *queueStub) Delete(context.Context, string) error {
	queue.deleted = true
	return nil
}

type inboxHandler struct {
	err error
}

func (handler inboxHandler) Receive(context.Context, WagerMessage) (domain.WagerTransaction, error) {
	if handler.err != nil {
		return domain.WagerTransaction{}, handler.err
	}
	return domain.WagerTransaction{}, nil
}

func mustNewInboxService(t *testing.T, applier WagerApplier, inbox InboxStore) *InboxService {
	t.Helper()
	service, err := NewInboxService(applier, inbox)
	if err != nil {
		t.Fatalf("NewInboxService() error = %v", err)
	}
	return service
}

func wagerMessageBody(messageID string) []byte {
	return []byte(`{"messageId":"` + messageID + `","type":"WagerTransactionRequested","data":{"providerId":"provider-a","externalTransactionId":"transaction-123","idempotencyKey":"provider-a:transaction-123","playerId":"player-1","walletId":"wallet-1","roundId":"round-1","gameId":"game-1","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}}`)
}

func wagerMessage(t *testing.T, wallet domain.Wallet, amount string) WagerMessage {
	t.Helper()
	return WagerMessage{
		ID:      "msg-123",
		Type:    wagerTransactionRequestedType,
		Command: placeBetCommand(t, wallet, amount),
		Kind:    domain.WagerTransactionKindBet,
	}
}
