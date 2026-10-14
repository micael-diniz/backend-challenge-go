package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestOpenWalletWithPositiveBalancePersistsCompleteOpeningAtomically(t *testing.T) {
	t.Parallel()

	store := &walletOpeningStoreStub{}
	now := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	service := mustNewOpenWalletService(t, store, sequentialIDGenerator(
		"wallet-1",
		"opening-transaction-1",
		"ledger-1",
		"processed-event-1",
		"balance-event-1",
	), func() time.Time { return now })
	ctx := context.WithValue(context.Background(), testContextKey{}, "request-1")
	command := OpenWalletCommand{
		PlayerID:       domain.PlayerID("player-1"),
		InitialBalance: mustApplicationMoney(t, "1000.00", "BRL"),
		CorrelationID:  domain.CorrelationID("correlation-1"),
		CausationID:    domain.CausationID("request-1"),
	}

	got, err := service.OpenWallet(ctx, command)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	if store.createWalletCalls != 0 {
		t.Errorf("CreateWallet() calls = %d; want 0 for positive opening", store.createWalletCalls)
	}
	if store.createWithOpeningCalls != 1 {
		t.Fatalf("CreateWalletWithOpening() calls = %d; want 1", store.createWithOpeningCalls)
	}
	if store.ctx != ctx {
		t.Error("OpenWallet() did not forward the request context")
	}
	if got.ID() != domain.WalletID("wallet-1") || store.wallet != got {
		t.Errorf("OpenWallet() wallet = %+v; want atomically persisted wallet %+v", got, store.wallet)
	}
	if got.Version() != 1 {
		t.Errorf("OpenWallet() wallet version = %d; want 1", got.Version())
	}
	if store.opening.ID() != domain.TransactionID("opening-transaction-1") || store.opening.Kind() != domain.WagerTransactionKindOpening || store.opening.Status() != domain.WagerTransactionStatusProcessed {
		t.Errorf("persisted opening = %+v; want processed OPENING with generated ID", store.opening)
	}
	if store.ledger.ID() != domain.LedgerEntryID("ledger-1") || store.ledger.Direction() != domain.LedgerDirectionCredit {
		t.Errorf("persisted ledger = %+v; want generated opening CREDIT", store.ledger)
	}
	if store.ledger.TransactionID() != store.opening.ID() {
		t.Errorf("ledger transaction ID = %q; want opening ID %q", store.ledger.TransactionID(), store.opening.ID())
	}
	if store.processedEvent.EventID() != domain.DomainEventID("processed-event-1") || store.processedEvent.AggregateID() != domain.AggregateID(store.opening.ID()) {
		t.Errorf("processed event IDs = (%q, %q); want generated event and opening aggregate", store.processedEvent.EventID(), store.processedEvent.AggregateID())
	}
	if store.balanceChangedEvent.EventID() != domain.DomainEventID("balance-event-1") || store.balanceChangedEvent.AggregateID() != domain.AggregateID(got.ID()) {
		t.Errorf("balance event IDs = (%q, %q); want generated event and wallet aggregate", store.balanceChangedEvent.EventID(), store.balanceChangedEvent.AggregateID())
	}
	if !store.processedEvent.OccurredAt().Equal(now) || !store.balanceChangedEvent.OccurredAt().Equal(now) {
		t.Error("opening events do not preserve the opening instant")
	}
}

func TestOpenWalletWithZeroBalancePersistsOnlyWallet(t *testing.T) {
	t.Parallel()

	store := &walletOpeningStoreStub{}
	generatedIDs := sequentialIDGenerator("wallet-1")
	service := mustNewOpenWalletService(
		t,
		store,
		generatedIDs,
		func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
	)

	got, err := service.OpenWallet(context.Background(), OpenWalletCommand{
		PlayerID:       domain.PlayerID("player-1"),
		InitialBalance: mustApplicationMoney(t, "0.00", "BRL"),
	})
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	if store.createWalletCalls != 1 {
		t.Errorf("CreateWallet() calls = %d; want 1", store.createWalletCalls)
	}
	if store.createWithOpeningCalls != 0 {
		t.Errorf("CreateWalletWithOpening() calls = %d; want 0 for zero opening", store.createWithOpeningCalls)
	}
	if store.wallet != got {
		t.Errorf("persisted wallet = %+v; want returned wallet %+v", store.wallet, got)
	}
}

func TestOpenWalletRejectsInvalidDomainInputWithoutPersistence(t *testing.T) {
	t.Parallel()

	store := &walletOpeningStoreStub{}
	service := mustNewOpenWalletService(
		t,
		store,
		sequentialIDGenerator("wallet-1"),
		func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
	)

	got, err := service.OpenWallet(context.Background(), OpenWalletCommand{
		PlayerID:       domain.PlayerID(" "),
		InitialBalance: mustApplicationMoney(t, "100.00", "BRL"),
	})
	if !errors.Is(err, domain.ErrInvalidPlayerID) {
		t.Fatalf("OpenWallet() error = %v; want ErrInvalidPlayerID", err)
	}
	if got != (domain.Wallet{}) {
		t.Errorf("OpenWallet() wallet = %+v; want zero value on error", got)
	}
	if store.createWalletCalls != 0 || store.createWithOpeningCalls != 0 {
		t.Errorf("store calls = (%d, %d); want no persistence", store.createWalletCalls, store.createWithOpeningCalls)
	}
}

func TestOpenWalletPreservesPersistenceError(t *testing.T) {
	t.Parallel()

	storeError := errors.New("store unavailable")
	store := &walletOpeningStoreStub{err: storeError}
	service := mustNewOpenWalletService(
		t,
		store,
		sequentialIDGenerator("wallet-1"),
		func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
	)

	got, err := service.OpenWallet(context.Background(), OpenWalletCommand{
		PlayerID:       domain.PlayerID("player-1"),
		InitialBalance: mustApplicationMoney(t, "0.00", "BRL"),
	})
	if !errors.Is(err, ErrOpenWalletPersistence) {
		t.Fatalf("OpenWallet() error = %v; want ErrOpenWalletPersistence", err)
	}
	if !errors.Is(err, storeError) {
		t.Errorf("OpenWallet() error = %v; want underlying store error", err)
	}
	if got != (domain.Wallet{}) {
		t.Errorf("OpenWallet() wallet = %+v; want zero value on error", got)
	}
}

func TestOpenWalletPreservesPositiveOpeningPersistenceError(t *testing.T) {
	t.Parallel()

	storeError := errors.New("atomic write failed")
	store := &walletOpeningStoreStub{err: storeError}
	service := mustNewOpenWalletService(
		t,
		store,
		sequentialIDGenerator(
			"wallet-1",
			"opening-transaction-1",
			"ledger-1",
			"processed-event-1",
			"balance-event-1",
		),
		func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
	)

	got, err := service.OpenWallet(context.Background(), OpenWalletCommand{
		PlayerID:       domain.PlayerID("player-1"),
		InitialBalance: mustApplicationMoney(t, "100.00", "BRL"),
		CorrelationID:  domain.CorrelationID("correlation-1"),
	})
	if !errors.Is(err, ErrOpenWalletPersistence) || !errors.Is(err, storeError) {
		t.Fatalf("OpenWallet() error = %v; want persistence and underlying store errors", err)
	}
	if got != (domain.Wallet{}) {
		t.Errorf("OpenWallet() wallet = %+v; want zero value on error", got)
	}
	if store.createWithOpeningCalls != 1 || store.createWalletCalls != 0 {
		t.Errorf("store calls = (%d, %d); want one atomic positive-opening call", store.createWalletCalls, store.createWithOpeningCalls)
	}
}

func TestOpenWalletRejectsInvalidGeneratedArtifactsWithoutPersistence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ids           []string
		correlationID domain.CorrelationID
		wantError     error
	}{
		{
			name:          "invalid opening transaction ID",
			ids:           []string{"wallet-1", ""},
			correlationID: "correlation-1",
			wantError:     domain.ErrInvalidTransactionID,
		},
		{
			name:          "invalid ledger ID",
			ids:           []string{"wallet-1", "opening-1", ""},
			correlationID: "correlation-1",
			wantError:     domain.ErrInvalidLedgerEntryID,
		},
		{
			name:          "invalid processed event ID",
			ids:           []string{"wallet-1", "opening-1", "ledger-1", ""},
			correlationID: "correlation-1",
			wantError:     domain.ErrInvalidDomainEventID,
		},
		{
			name:          "missing correlation ID",
			ids:           []string{"wallet-1", "opening-1", "ledger-1", "processed-event-1"},
			correlationID: "",
			wantError:     domain.ErrInvalidCorrelationID,
		},
		{
			name:          "invalid balance event ID",
			ids:           []string{"wallet-1", "opening-1", "ledger-1", "processed-event-1", ""},
			correlationID: "correlation-1",
			wantError:     domain.ErrInvalidDomainEventID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &walletOpeningStoreStub{}
			service := mustNewOpenWalletService(
				t,
				store,
				sequentialIDGenerator(tt.ids...),
				func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
			)

			got, err := service.OpenWallet(context.Background(), OpenWalletCommand{
				PlayerID:       domain.PlayerID("player-1"),
				InitialBalance: mustApplicationMoney(t, "100.00", "BRL"),
				CorrelationID:  tt.correlationID,
			})
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("OpenWallet() error = %v; want %v", err, tt.wantError)
			}
			if got != (domain.Wallet{}) {
				t.Errorf("OpenWallet() wallet = %+v; want zero value on error", got)
			}
			if store.createWalletCalls != 0 || store.createWithOpeningCalls != 0 {
				t.Errorf("store calls = (%d, %d); want no persistence", store.createWalletCalls, store.createWithOpeningCalls)
			}
		})
	}
}

func TestNewOpenWalletServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	store := &walletOpeningStoreStub{}
	newID := sequentialIDGenerator("wallet-1")
	now := func() time.Time { return time.Now() }

	tests := []struct {
		name  string
		store WalletOpeningStore
		newID func() string
		now   func() time.Time
	}{
		{name: "missing store", newID: newID, now: now},
		{name: "missing ID generator", store: store, now: now},
		{name: "missing clock", store: store, newID: newID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOpenWalletService(tt.store, tt.newID, tt.now)
			if !errors.Is(err, ErrInvalidOpenWalletService) {
				t.Fatalf("NewOpenWalletService() error = %v; want ErrInvalidOpenWalletService", err)
			}
			if got != nil {
				t.Errorf("NewOpenWalletService() = %+v; want nil on error", got)
			}
		})
	}
}

type testContextKey struct{}

type walletOpeningStoreStub struct {
	err                    error
	ctx                    context.Context
	wallet                 domain.Wallet
	opening                domain.WagerTransaction
	ledger                 domain.WalletLedgerEntry
	processedEvent         domain.WagerTransactionProcessed
	balanceChangedEvent    domain.WalletBalanceChanged
	createWalletCalls      int
	createWithOpeningCalls int
}

func (store *walletOpeningStoreStub) CreateWallet(
	ctx context.Context,
	wallet domain.Wallet,
) error {
	store.createWalletCalls++
	store.ctx = ctx
	store.wallet = wallet
	return store.err
}

func (store *walletOpeningStoreStub) CreateWalletWithOpening(
	ctx context.Context,
	wallet domain.Wallet,
	opening domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	store.createWithOpeningCalls++
	store.ctx = ctx
	store.wallet = wallet
	store.opening = opening
	store.ledger = ledger
	store.processedEvent = processedEvent
	store.balanceChangedEvent = balanceChangedEvent
	return store.err
}

func mustNewOpenWalletService(
	t *testing.T,
	store WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) *OpenWalletService {
	t.Helper()

	service, err := NewOpenWalletService(store, newID, now)
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	return service
}

func sequentialIDGenerator(ids ...string) func() string {
	next := 0
	return func() string {
		if next >= len(ids) {
			panic("test ID generator exhausted")
		}
		id := ids[next]
		next++
		return id
	}
}

func mustApplicationMoney(t *testing.T, amount, currency string) domain.Money {
	t.Helper()

	money, err := domain.NewMoney(amount, currency)
	if err != nil {
		t.Fatalf("NewMoney(%q, %q) returned an unexpected error: %v", amount, currency, err)
	}
	return money
}
