package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestRecordWinCreditsWallet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("win-1", "ledger-1", "event-1", "event-2"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "40.00")
	command.ExternalTransactionID = "external-win-1"
	command.IdempotencyKey = "provider-a:external-win-1"

	credited, win, replay, err := service.RecordWin(context.Background(), command)
	if err != nil {
		t.Fatalf("RecordWin() returned an unexpected error: %v", err)
	}
	if replay || win.Kind() != domain.WagerTransactionKindWin || win.Status() != domain.WagerTransactionStatusProcessed {
		t.Fatalf("win = %s %s replay %t", win.Kind(), win.Status(), replay)
	}
	result, present := win.ResultBalance()
	if !present || result.Amount() != "140.00" || credited.Version() != wallet.Version()+1 {
		t.Fatalf("result = (%s, %t) version %d", result.Amount(), present, credited.Version())
	}
	if store.ledger.Direction() != domain.LedgerDirectionCredit || store.saveCalls != 1 {
		t.Fatalf("ledger %s saves %d", store.ledger.Direction(), store.saveCalls)
	}

	_, stored, replay, err := service.RecordWin(context.Background(), command)
	if err != nil || !replay || stored.ID() != win.ID() || store.saveCalls != 1 {
		t.Fatalf("replay = %t err %v saves %d", replay, err, store.saveCalls)
	}
}

func TestRecordWinCreditsReferencedBet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"win-1", "ledger-2", "event-3", "event-4",
	), func() time.Time { return placedAt })
	if _, _, _, err := service.PlaceBet(context.Background(), placeBetCommand(t, wallet, "25.00")); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	storedBet := store.bet
	command := placeBetCommand(t, wallet, "40.00")
	command.ExternalTransactionID = "external-win-1"
	command.IdempotencyKey = "provider-a:external-win-1"
	command.ReferenceExternalTransactionID = storedBet.ExternalTransactionID()

	_, win, _, err := service.RecordWin(context.Background(), command)
	if err != nil {
		t.Fatalf("RecordWin() returned an unexpected error: %v", err)
	}
	resolved, present := win.ResolvedReferenceTransactionID()
	if !present || resolved != storedBet.ID() {
		t.Fatalf("resolved reference = (%s, %t); want %s", resolved, present, storedBet.ID())
	}
	result, present := win.ResultBalance()
	if !present || result.Amount() != "115.00" {
		t.Fatalf("result = (%s, %t); want 115.00", result.Amount(), present)
	}
}

func TestRecordWinRejectsZeroAmountAndAnotherCurrency(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("win-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})

	_, _, _, err := service.RecordWin(context.Background(), placeBetCommand(t, wallet, "0.00"))
	if !errors.Is(err, domain.ErrInvalidWagerTransactionAmount) || store.saveCalls != 0 {
		t.Fatalf("zero win error = %v saves %d", err, store.saveCalls)
	}
	command := placeBetCommand(t, wallet, "40.00")
	command.Money = mustApplicationMoney(t, "40.00", "USD")
	_, _, _, err = service.RecordWin(context.Background(), command)
	if !errors.Is(err, domain.ErrCurrencyMismatch) || store.saveCalls != 0 {
		t.Fatalf("currency error = %v saves %d", err, store.saveCalls)
	}
}

func TestRecordWinRejectsUnprocessedBetReference(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "10.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("bet-1", "event-1", "win-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	betCommand := placeBetCommand(t, wallet, "25.00")
	if _, _, _, err := service.PlaceBet(context.Background(), betCommand); !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("PlaceBet() error = %v; want insufficient funds", err)
	}
	command := placeBetCommand(t, wallet, "40.00")
	command.ExternalTransactionID = "external-win-1"
	command.IdempotencyKey = "provider-a:external-win-1"
	command.ReferenceExternalTransactionID = betCommand.ExternalTransactionID

	_, _, _, err := service.RecordWin(context.Background(), command)
	if !errors.Is(err, ErrInvalidReference) || store.saveCalls != 0 {
		t.Fatalf("error = %v saves %d; want invalid reference and no win save", err, store.saveCalls)
	}
}

func TestRecordWinHoldsAMissingReference(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	now := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("win-1", "event-1"), func() time.Time {
		return now
	})
	command := placeBetCommand(t, wallet, "40.00")
	command.ReferenceExternalTransactionID = "missing-bet"

	returned, win, replay, err := service.RecordWin(context.Background(), command)
	next, hasNext := win.NextReferenceAttemptAt()
	expires, hasExpires := win.ReferenceExpiresAt()
	if err != nil || replay || win.Status() != domain.WagerTransactionStatusPendingReference || win.ReferenceAttempts() != 1 || !hasNext || !next.Equal(now.Add(time.Second)) || !hasExpires || !expires.Equal(now.Add(time.Minute)) {
		t.Fatalf("status = %s attempts = %d next = %s expires = %s error = %v", win.Status(), win.ReferenceAttempts(), next, expires, err)
	}
	if returned.Balance().Amount() != "100.00" || returned.Version() != wallet.Version() {
		t.Fatalf("wallet = %s version %d", returned.Balance().Amount(), returned.Version())
	}
}

func TestRecordWinRejectsReferenceFromAnotherRound(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2", "win-1",
	), func() time.Time { return placedAt })
	betCommand := placeBetCommand(t, wallet, "25.00")
	if _, _, _, err := service.PlaceBet(context.Background(), betCommand); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	command := placeBetCommand(t, wallet, "40.00")
	command.ExternalTransactionID = "external-win-1"
	command.IdempotencyKey = "provider-a:external-win-1"
	command.ReferenceExternalTransactionID = betCommand.ExternalTransactionID
	command.RoundID = "round-2"

	_, _, _, err := service.RecordWin(context.Background(), command)
	if !errors.Is(err, ErrInvalidReference) || store.saveCalls != 1 {
		t.Fatalf("error = %v saves %d; want invalid reference and only the bet save", err, store.saveCalls)
	}
}
