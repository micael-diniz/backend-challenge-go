package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestRecordLossKeepsWalletAndStoresResult(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("loss-1", "event-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "0.00")

	current, loss, replay, err := service.RecordLoss(context.Background(), command)
	if err != nil {
		t.Fatalf("RecordLoss() returned an unexpected error: %v", err)
	}
	if replay || loss.Status() != domain.WagerTransactionStatusProcessed || loss.Kind() != domain.WagerTransactionKindLoss {
		t.Fatalf("loss = %s %s replay %t", loss.Kind(), loss.Status(), replay)
	}
	result, present := loss.ResultBalance()
	if !present || result.Amount() != "100.00" {
		t.Fatalf("result = (%s, %t); want 100.00", result.Amount(), present)
	}
	if current.Balance().Amount() != "100.00" || current.Version() != wallet.Version() {
		t.Fatalf("wallet = %s version %d", current.Balance().Amount(), current.Version())
	}
	if store.lossCalls != 1 || store.saveCalls != 0 || store.wallet.Version() != wallet.Version() {
		t.Fatalf("loss saves %d processed saves %d wallet version %d", store.lossCalls, store.saveCalls, store.wallet.Version())
	}

	_, stored, replay, err := service.RecordLoss(context.Background(), command)
	if err != nil {
		t.Fatalf("replay RecordLoss() returned an unexpected error: %v", err)
	}
	if !replay || stored.ID() != loss.ID() || store.lossCalls != 1 {
		t.Fatalf("replay = %t id %s loss saves %d", replay, stored.ID(), store.lossCalls)
	}
}

func TestRecordLossRejectsPlayerMismatch(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("loss-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "0.00")
	command.PlayerID = "other-player"

	_, _, _, err := service.RecordLoss(context.Background(), command)
	if !errors.Is(err, ErrBetPlayerMismatch) || store.lossCalls != 0 {
		t.Fatalf("error = %v loss saves %d; want player mismatch and no save", err, store.lossCalls)
	}
}

func TestRecordLossRejectsAnotherCurrency(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("loss-1", "event-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "0.00")
	command.Money = mustApplicationMoney(t, "0.00", "USD")

	_, _, _, err := service.RecordLoss(context.Background(), command)
	if !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("RecordLoss() error = %v; want currency mismatch", err)
	}
	if store.lossCalls != 0 {
		t.Fatalf("SaveProcessedLoss() calls = %d; want 0", store.lossCalls)
	}
}

func TestRecordLossRejectsPositiveAmount(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("loss-1", "event-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})

	_, _, _, err := service.RecordLoss(context.Background(), placeBetCommand(t, wallet, "1.00"))
	if !errors.Is(err, domain.ErrInvalidWagerTransactionAmount) {
		t.Fatalf("RecordLoss() error = %v; want invalid amount", err)
	}
	if store.lossCalls != 0 {
		t.Fatalf("SaveProcessedLoss() calls = %d; want 0", store.lossCalls)
	}
}
