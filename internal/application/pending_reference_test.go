package application

import (
	"context"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestAdvanceDueReferenceCreditsTheWinAfterTheBetArrives(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	start := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	clock := start
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"win-1", "pending-event-1",
		"retry-event-1",
		"bet-1", "bet-ledger-1", "bet-processed-1", "bet-balance-1",
		"win-ledger-1", "win-processed-1", "win-balance-1",
	), func() time.Time { return clock })
	command := placeBetCommand(t, wallet, "40.00")
	command.ExternalTransactionID = "win-123"
	command.IdempotencyKey = "provider-a:win-123"
	command.ReferenceExternalTransactionID = "bet-123"

	_, win, replay, err := service.RecordWin(context.Background(), command)
	if err != nil || replay || win.Status() != domain.WagerTransactionStatusPendingReference {
		t.Fatalf("RecordWin() status = %s replay = %v error = %v", win.Status(), replay, err)
	}
	if err := service.AdvanceDueReferences(context.Background()); err != nil || store.bet.ReferenceAttempts() != 1 {
		t.Fatalf("early advance attempts = %d error = %v", store.bet.ReferenceAttempts(), err)
	}

	clock = start.Add(time.Second)
	if err := service.AdvanceDueReferences(context.Background()); err != nil || store.bet.ReferenceAttempts() != 2 {
		t.Fatalf("retry attempts = %d error = %v", store.bet.ReferenceAttempts(), err)
	}
	bet := placeBetCommand(t, wallet, "25.00")
	bet.ExternalTransactionID = "bet-123"
	bet.IdempotencyKey = "provider-a:bet-123"
	if _, _, _, err := service.PlaceBet(context.Background(), bet); err != nil {
		t.Fatalf("PlaceBet() error = %v", err)
	}

	clock = clock.Add(2 * time.Second)
	if err := service.AdvanceDueReferences(context.Background()); err != nil {
		t.Fatalf("AdvanceDueReferences() error = %v", err)
	}
	resolved := storedTransaction(store, "win-1")
	if resolved.Status() != domain.WagerTransactionStatusProcessed || store.wallet.Balance().Amount() != "115.00" || store.wallet.Version() != 3 {
		t.Fatalf("status = %s wallet = %s version %d", resolved.Status(), store.wallet.Balance().Amount(), store.wallet.Version())
	}
}

func TestRecordRefundHoldsAMissingBet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("refund-1", "event-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "25.00")
	command.ExternalTransactionID = "refund-123"
	command.IdempotencyKey = "provider-a:refund-123"
	command.ReferenceExternalTransactionID = "missing-bet"

	_, refund, replay, err := service.RecordRefund(context.Background(), command)
	if err != nil || replay || refund.Kind() != domain.WagerTransactionKindRefund || refund.Status() != domain.WagerTransactionStatusPendingReference {
		t.Fatalf("refund = %s %s replay %t error %v", refund.Kind(), refund.Status(), replay, err)
	}
	if store.wallet.Version() != wallet.Version() {
		t.Fatalf("version = %d; want the wallet unchanged", store.wallet.Version())
	}
}

func TestAdvanceDueReferenceRejectsWhenTheWaitExpires(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	start := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	clock := start
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("win-1", "pending-event-1", "rejected-event-1"), func() time.Time {
		return clock
	})
	command := placeBetCommand(t, wallet, "40.00")
	command.ReferenceExternalTransactionID = "missing-bet"
	if _, _, _, err := service.RecordWin(context.Background(), command); err != nil {
		t.Fatalf("RecordWin() error = %v", err)
	}

	clock = start.Add(time.Minute)
	if err := service.AdvanceDueReferences(context.Background()); err != nil {
		t.Fatalf("AdvanceDueReferences() error = %v", err)
	}
	rejected := storedTransaction(store, "win-1")
	code, _ := rejected.FailureCode()
	if rejected.Status() != domain.WagerTransactionStatusRejected || code != domain.WagerTransactionFailureCodeReferenceNotFound || store.wallet.Version() != wallet.Version() {
		t.Fatalf("status = %s code = %s version = %d", rejected.Status(), code, store.wallet.Version())
	}
}

func storedTransaction(store *betStoreStub, id domain.TransactionID) domain.WagerTransaction {
	if store.bet.ID() == id {
		return store.bet
	}
	for index := len(store.earlier) - 1; index >= 0; index-- {
		if store.earlier[index].ID() == id {
			return store.earlier[index]
		}
	}
	return domain.WagerTransaction{}
}
