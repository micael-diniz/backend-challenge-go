package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestPlaceBetDebitsWalletAndPersistsProcessedBet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1",
		"ledger-bet-1",
		"processed-bet-event-1",
		"balance-bet-event-1",
	), func() time.Time { return placedAt })
	ctx := context.WithValue(context.Background(), testContextKey{}, "request-bet-1")

	updated, bet, replay, err := service.PlaceBet(ctx, placeBetCommand(t, wallet, "25.00"))
	if err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	if replay {
		t.Error("first PlaceBet() was marked as a replay")
	}
	if store.saveCalls != 1 {
		t.Fatalf("SaveProcessedBet() calls = %d; want 1", store.saveCalls)
	}
	if store.ctx != ctx {
		t.Error("PlaceBet() did not forward the request context")
	}
	if updated.Balance().Amount() != "75.00" || updated.Version() != 2 {
		t.Errorf("wallet = balance %s version %d; want 75.00 version 2", updated.Balance().Amount(), updated.Version())
	}
	if store.savedWallet != updated {
		t.Errorf("persisted wallet = %+v; want returned wallet %+v", store.savedWallet, updated)
	}
	if bet.Kind() != domain.WagerTransactionKindBet || bet.Status() != domain.WagerTransactionStatusProcessed {
		t.Errorf("bet = kind %s status %s; want BET PROCESSED", bet.Kind(), bet.Status())
	}
	result, resultPresent := bet.ResultBalance()
	if !resultPresent || result.Amount() != "75.00" || result.Currency() != domain.CurrencyBRL {
		t.Errorf("result balance = (%s, %s, %t); want 75.00 BRL present", result.Amount(), result.Currency(), resultPresent)
	}
	if store.ledger.Direction() != domain.LedgerDirectionDebit || store.ledger.BalanceBefore().Amount() != "100.00" || store.ledger.BalanceAfter().Amount() != "75.00" {
		t.Errorf("ledger = %+v; want DEBIT from 100.00 to 75.00", store.ledger)
	}
	if store.processedEvent.EventID() != domain.DomainEventID("processed-bet-event-1") {
		t.Errorf("processed event ID = %q; want processed-bet-event-1", store.processedEvent.EventID())
	}
	if store.balanceChangedEvent.EventID() != domain.DomainEventID("balance-bet-event-1") {
		t.Errorf("balance event ID = %q; want balance-bet-event-1", store.balanceChangedEvent.EventID())
	}
}

func TestPlaceBetRejectsPlayerDifferentFromWallet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("bet-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})
	command := placeBetCommand(t, wallet, "25.00")
	command.PlayerID = domain.PlayerID("other-player")

	_, _, _, err := service.PlaceBet(context.Background(), command)
	if !errors.Is(err, ErrBetPlayerMismatch) {
		t.Fatalf("PlaceBet() error = %v; want player mismatch", err)
	}
	if store.saveCalls != 0 {
		t.Errorf("SaveProcessedBet() calls = %d; want 0", store.saveCalls)
	}
}

func TestPlaceBetReplayReturnsOriginalResultWithoutAnotherDebit(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-bet-1", "processed-bet-event-1", "balance-bet-event-1",
		"bet-2", "ledger-bet-2", "processed-bet-event-2", "balance-bet-event-2",
	), func() time.Time { return placedAt })
	command := placeBetCommand(t, wallet, "25.00")

	if _, _, _, err := service.PlaceBet(context.Background(), command); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	moved, err := domain.RehydrateWallet(wallet.ID(), wallet.PlayerID(), mustApplicationMoney(t, "50.00", "BRL"), 3, openedAt, placedAt)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}
	store.wallet = moved

	updated, bet, replay, err := service.PlaceBet(context.Background(), command)
	if err != nil {
		t.Fatalf("replay PlaceBet() returned an unexpected error: %v", err)
	}
	if !replay {
		t.Fatal("replay PlaceBet() was not marked as a replay")
	}
	if store.saveCalls != 1 {
		t.Errorf("SaveProcessedBet() calls = %d; want 1", store.saveCalls)
	}
	result, present := bet.ResultBalance()
	if !present || result.Amount() != "75.00" {
		t.Errorf("replay result = (%s, %t); want 75.00", result.Amount(), present)
	}
	if updated.Balance().Amount() != "50.00" || updated.Version() != 3 {
		t.Errorf("wallet = %s version %d; want current 50.00 version 3", updated.Balance().Amount(), updated.Version())
	}
}

func TestPlaceBetRejectsSameKeyWithDifferentContent(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"bet-2", "ledger-2", "event-3", "event-4",
	), func() time.Time { return openedAt.Add(time.Second) })
	if _, _, _, err := service.PlaceBet(context.Background(), placeBetCommand(t, wallet, "25.00")); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	command := placeBetCommand(t, wallet, "10.00")

	_, _, replay, err := service.PlaceBet(context.Background(), command)
	if !errors.Is(err, ErrIdempotencyConflict) || replay {
		t.Fatalf("PlaceBet() = (%t, %v); want idempotency conflict", replay, err)
	}
	if store.saveCalls != 1 {
		t.Errorf("SaveProcessedBet() calls = %d; want 1", store.saveCalls)
	}
}

func TestPlaceBetRejectsSameExternalIDWithAnotherKey(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"bet-2", "ledger-2", "event-3", "event-4",
	), func() time.Time { return openedAt.Add(time.Second) })
	if _, _, _, err := service.PlaceBet(context.Background(), placeBetCommand(t, wallet, "25.00")); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	command := placeBetCommand(t, wallet, "25.00")
	command.IdempotencyKey = "provider-a:other-key"

	_, _, replay, err := service.PlaceBet(context.Background(), command)
	if !errors.Is(err, ErrIdempotencyConflict) || replay {
		t.Fatalf("PlaceBet() = (%t, %v); want idempotency conflict", replay, err)
	}
	if store.saveCalls != 1 {
		t.Errorf("SaveProcessedBet() calls = %d; want 1", store.saveCalls)
	}
}

func TestBetPayloadHashIgnoresKeyAndChangesWithAmount(t *testing.T) {
	t.Parallel()

	wallet := mustPlaceBetWallet(t, "100.00", time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC))
	left := placeBetCommand(t, wallet, "25.00")
	right := left
	right.IdempotencyKey = "provider-a:other-key"
	right.CorrelationID = "other-correlation"
	right.CausationID = "other-causation"
	leftHash, err := betPayloadHash(left)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	rightHash, err := betPayloadHash(right)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	if leftHash != rightHash {
		t.Errorf("hash changed when only the idempotency key and transport metadata changed")
	}
	changed := left
	changed.Money = mustApplicationMoney(t, "10.00", "BRL")
	changedHash, err := betPayloadHash(changed)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	if changedHash == leftHash {
		t.Error("hash stayed the same after the amount changed")
	}
}

func TestPlaceBetWithInsufficientFundsPersistsNothing(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "10.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("bet-1", "ledger-1", "event-1", "event-2"), func() time.Time {
		return openedAt.Add(time.Second)
	})

	_, _, _, err := service.PlaceBet(context.Background(), placeBetCommand(t, wallet, "25.00"))
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("PlaceBet() error = %v; want insufficient funds", err)
	}
	if store.saveCalls != 0 {
		t.Errorf("SaveProcessedBet() calls = %d; want 0", store.saveCalls)
	}
}

type betStoreStub struct {
	wallet              domain.Wallet
	ctx                 context.Context
	savedWallet         domain.Wallet
	bet                 domain.WagerTransaction
	ledger              domain.WalletLedgerEntry
	processedEvent      domain.WagerTransactionProcessed
	balanceChangedEvent domain.WalletBalanceChanged
	saveCalls           int
}

func (store *betStoreStub) LoadWallet(
	ctx context.Context,
	walletID domain.WalletID,
) (domain.Wallet, error) {
	store.ctx = ctx
	if store.wallet.ID() != walletID {
		return domain.Wallet{}, ErrWalletNotFound
	}
	return store.wallet, nil
}

func (store *betStoreStub) FindBetByIdempotencyKey(
	ctx context.Context,
	providerID domain.ProviderID,
	key domain.IdempotencyKey,
) (domain.WagerTransaction, bool, error) {
	store.ctx = ctx
	if store.saveCalls == 0 || store.bet.ProviderID() != providerID || store.bet.IdempotencyKey() != key {
		return domain.WagerTransaction{}, false, nil
	}
	return store.bet, true, nil
}

func (store *betStoreStub) FindBetByExternalID(
	ctx context.Context,
	providerID domain.ProviderID,
	externalID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	store.ctx = ctx
	if store.saveCalls == 0 || store.bet.ProviderID() != providerID || store.bet.ExternalTransactionID() != externalID {
		return domain.WagerTransaction{}, false, nil
	}
	return store.bet, true, nil
}

func (store *betStoreStub) SaveProcessedBet(
	ctx context.Context,
	wallet domain.Wallet,
	bet domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	store.saveCalls++
	store.ctx = ctx
	store.savedWallet = wallet
	store.bet = bet
	store.ledger = ledger
	store.processedEvent = processedEvent
	store.balanceChangedEvent = balanceChangedEvent
	return nil
}

func mustNewPlaceBetService(
	t *testing.T,
	store BetStore,
	newID func() string,
	now func() time.Time,
) *PlaceBetService {
	t.Helper()

	service, err := NewPlaceBetService(store, newID, now)
	if err != nil {
		t.Fatalf("NewPlaceBetService() returned an unexpected error: %v", err)
	}
	return service
}

func mustPlaceBetWallet(t *testing.T, amount string, openedAt time.Time) domain.Wallet {
	t.Helper()

	wallet, err := domain.NewWallet(
		domain.WalletID("wallet-1"),
		domain.PlayerID("player-1"),
		mustApplicationMoney(t, amount, "BRL"),
		openedAt,
	)
	if err != nil {
		t.Fatalf("NewWallet() returned an unexpected error: %v", err)
	}
	return wallet
}

func placeBetCommand(t *testing.T, wallet domain.Wallet, amount string) PlaceBetCommand {
	t.Helper()

	return PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            domain.ProviderID("provider-a"),
		ExternalTransactionID: domain.ExternalTransactionID("external-bet-1"),
		IdempotencyKey:        domain.IdempotencyKey("provider-a:external-bet-1"),
		RoundID:               domain.RoundID("round-1"),
		GameID:                domain.GameID("game-1"),
		Money:                 mustApplicationMoney(t, amount, "BRL"),
		CorrelationID:         domain.CorrelationID("correlation-1"),
		CausationID:           domain.CausationID("request-bet-1"),
	}
}
