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

func TestPlaceBetReplaysWhenTheRowAppearsBetweenTheTwoLookups(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"bet-2", "ledger-2", "event-3", "event-4",
	), func() time.Time { return openedAt.Add(time.Second) })
	command := placeBetCommand(t, wallet, "25.00")
	if _, _, _, err := service.PlaceBet(context.Background(), command); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	store.skipNextKeyLookup = true

	_, bet, replay, err := service.PlaceBet(context.Background(), command)
	if err != nil || !replay {
		t.Fatalf("PlaceBet() = replay %t error %v; want replay", replay, err)
	}
	result, present := bet.ResultBalance()
	if !present || result.Amount() != "75.00" {
		t.Fatalf("replay balance = (%s, %t); want 75.00", result.Amount(), present)
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
	leftHash, err := betPayloadHash(left, domain.WagerTransactionKindBet)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	rightHash, err := betPayloadHash(right, domain.WagerTransactionKindBet)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	if leftHash != rightHash {
		t.Errorf("hash changed when only the idempotency key and transport metadata changed")
	}
	changed := left
	changed.Money = mustApplicationMoney(t, "10.00", "BRL")
	changedHash, err := betPayloadHash(changed, domain.WagerTransactionKindBet)
	if err != nil {
		t.Fatalf("betPayloadHash() returned an unexpected error: %v", err)
	}
	if changedHash == leftHash {
		t.Error("hash stayed the same after the amount changed")
	}
}

func TestPlaceBetRetriesVersionConflictAndRejectsInsufficientFunds(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	spent, err := wallet.Debit(mustApplicationMoney(t, "80.00", "BRL"), placedAt)
	if err != nil {
		t.Fatalf("Debit() returned an unexpected error: %v", err)
	}
	store := &betStoreStub{wallet: wallet, conflicts: 1, walletAfterConflict: spent}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2", "bet-2", "event-3",
	), func() time.Time {
		return placedAt
	})

	_, _, _, err = service.PlaceBet(context.Background(), placeBetCommand(t, wallet, "80.00"))
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("PlaceBet() error = %v; want insufficient funds", err)
	}
	if store.saveCalls != 1 {
		t.Errorf("SaveProcessedBet() calls = %d; want 1 rejected attempt", store.saveCalls)
	}
	if store.rejectedCalls != 1 {
		t.Errorf("SaveRejectedBet() calls = %d; want 1", store.rejectedCalls)
	}
}

func TestPlaceBetWithInsufficientFundsPersistsNothing(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	wallet := mustPlaceBetWallet(t, "10.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator("bet-1", "event-1"), func() time.Time {
		return openedAt.Add(time.Second)
	})

	command := placeBetCommand(t, wallet, "25.00")
	_, _, _, err := service.PlaceBet(context.Background(), command)
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("PlaceBet() error = %v; want insufficient funds", err)
	}
	if store.saveCalls != 0 || store.rejectedCalls != 1 {
		t.Fatalf("processed saves = %d rejected saves = %d; want 0 and 1", store.saveCalls, store.rejectedCalls)
	}
	code, present := store.bet.FailureCode()
	if !present || code != domain.WagerTransactionFailureCodeInsufficientFunds {
		t.Fatalf("failure code = (%q, %t); want INSUFFICIENT_FUNDS", code, present)
	}
	if store.wallet.Balance().Amount() != "10.00" || store.wallet.Version() != wallet.Version() {
		t.Fatalf("wallet changed to %s version %d", store.wallet.Balance().Amount(), store.wallet.Version())
	}

	_, stored, replay, err := service.PlaceBet(context.Background(), command)
	if err != nil {
		t.Fatalf("replay PlaceBet() returned an unexpected error: %v", err)
	}
	if !replay || stored.ID() != store.bet.ID() || store.rejectedCalls != 1 {
		t.Fatalf("replay = %t id %s rejected saves %d", replay, stored.ID(), store.rejectedCalls)
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
	rejectedCalls       int
	lossCalls           int
	conflicts           int
	walletAfterConflict domain.Wallet
	earlier             []domain.WagerTransaction
	skipNextKeyLookup   bool
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
	if store.skipNextKeyLookup {
		store.skipNextKeyLookup = false
		return domain.WagerTransaction{}, false, nil
	}
	return store.matchTransaction(func(bet domain.WagerTransaction) bool {
		return bet.ID() != "" && bet.ProviderID() == providerID && bet.IdempotencyKey() == key
	})
}

func (store *betStoreStub) FindBetByExternalID(
	ctx context.Context,
	providerID domain.ProviderID,
	externalID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	store.ctx = ctx
	return store.matchTransaction(func(bet domain.WagerTransaction) bool {
		return bet.ID() != "" && bet.ProviderID() == providerID && bet.ExternalTransactionID() == externalID
	})
}

func (store *betStoreStub) FindProcessedReversal(
	_ context.Context,
	providerID domain.ProviderID,
	referenceID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	return store.matchTransaction(func(bet domain.WagerTransaction) bool {
		reference, present := bet.ReferenceExternalTransactionID()
		return present &&
			reference == referenceID &&
			bet.ProviderID() == providerID &&
			bet.Status() == domain.WagerTransactionStatusProcessed &&
			(bet.Kind() == domain.WagerTransactionKindRefund || bet.Kind() == domain.WagerTransactionKindRollback)
	})
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
	if store.conflicts > 0 {
		store.conflicts--
		store.wallet = store.walletAfterConflict
		return ErrWalletVersionConflict
	}
	store.savedWallet = wallet
	store.wallet = wallet
	store.keep(bet)
	store.ledger = ledger
	store.processedEvent = processedEvent
	store.balanceChangedEvent = balanceChangedEvent
	return nil
}

func (store *betStoreStub) SaveRejectedBet(
	_ context.Context,
	bet domain.WagerTransaction,
	_ domain.WagerTransactionRejected,
) error {
	store.rejectedCalls++
	store.keep(bet)
	return nil
}

func (store *betStoreStub) SaveProcessedWin(
	ctx context.Context,
	wallet domain.Wallet,
	win domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	return store.SaveProcessedBet(ctx, wallet, win, ledger, processedEvent, balanceChangedEvent)
}

func (store *betStoreStub) SaveProcessedLoss(
	_ context.Context,
	loss domain.WagerTransaction,
	_ domain.WagerTransactionProcessed,
) error {
	store.lossCalls++
	store.keep(loss)
	return nil
}

func (store *betStoreStub) SavePendingReference(
	_ context.Context,
	transaction domain.WagerTransaction,
	_ domain.WagerTransactionPendingReference,
) error {
	store.keep(transaction)
	return nil
}

func (store *betStoreStub) ListDuePendingReferences(
	_ context.Context,
	dueAt time.Time,
) ([]domain.WagerTransaction, error) {
	var due []domain.WagerTransaction
	collect := func(transaction domain.WagerTransaction) {
		next, present := transaction.NextReferenceAttemptAt()
		if transaction.Status() == domain.WagerTransactionStatusPendingReference && present && !next.After(dueAt) {
			due = append(due, transaction)
		}
	}
	if store.bet.ID() != "" {
		collect(store.bet)
	}
	for _, transaction := range store.earlier {
		collect(transaction)
	}
	return due, nil
}

func (store *betStoreStub) SavePendingReferenceRetry(
	_ context.Context,
	transaction domain.WagerTransaction,
	_ domain.WagerTransactionPendingReference,
) error {
	store.replace(transaction)
	return nil
}

func (store *betStoreStub) RejectPendingReference(
	_ context.Context,
	transaction domain.WagerTransaction,
	_ domain.WagerTransactionRejected,
) error {
	store.rejectedCalls++
	store.replace(transaction)
	return nil
}

func (store *betStoreStub) ResolvePendingReference(
	_ context.Context,
	wallet domain.Wallet,
	transaction domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	store.saveCalls++
	store.wallet = wallet
	store.ledger = ledger
	store.processedEvent = processedEvent
	store.balanceChangedEvent = balanceChangedEvent
	store.replace(transaction)
	return nil
}

func (store *betStoreStub) replace(transaction domain.WagerTransaction) {
	if store.bet.ID() == transaction.ID() {
		store.bet = transaction
		return
	}
	for index := range store.earlier {
		if store.earlier[index].ID() == transaction.ID() {
			store.earlier[index] = transaction
			return
		}
	}
	store.keep(transaction)
}

func (store *betStoreStub) SaveProcessedReversal(
	ctx context.Context,
	wallet domain.Wallet,
	reversal domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	return store.SaveProcessedBet(ctx, wallet, reversal, ledger, processedEvent, balanceChangedEvent)
}

func (store *betStoreStub) SaveRejectedReversal(
	ctx context.Context,
	reversal domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	return store.SaveRejectedBet(ctx, reversal, rejectedEvent)
}

func (store *betStoreStub) keep(next domain.WagerTransaction) {
	if store.bet.ID() != "" && store.bet.ID() != next.ID() {
		store.earlier = append(store.earlier, store.bet)
	}
	store.bet = next
}

func (store *betStoreStub) matchTransaction(
	match func(domain.WagerTransaction) bool,
) (domain.WagerTransaction, bool, error) {
	if store.bet.ID() != "" && match(store.bet) {
		return store.bet, true, nil
	}
	for index := len(store.earlier) - 1; index >= 0; index-- {
		if match(store.earlier[index]) {
			return store.earlier[index], true, nil
		}
	}
	return domain.WagerTransaction{}, false, nil
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
