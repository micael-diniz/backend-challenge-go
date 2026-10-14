package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestWalletOpeningStorePersistsPositiveOpeningArtifacts(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	service := mustMemoryOpenWalletService(t, store)

	wallet, err := service.OpenWallet(context.Background(), memoryOpenWalletCommand(t, "1000.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 1 || store.wallets[wallet.ID()] != wallet {
		t.Errorf("wallets = %+v; want exactly returned wallet", store.wallets)
	}
	if len(store.transactions) != 1 {
		t.Errorf("transactions count = %d; want 1 OPENING", len(store.transactions))
	}
	if len(store.ledgerEntries) != 1 {
		t.Errorf("ledger entries count = %d; want 1", len(store.ledgerEntries))
	}
	if len(store.outboxPayloads) != 2 {
		t.Errorf("outbox payloads count = %d; want 2", len(store.outboxPayloads))
	}
	for _, payload := range store.outboxPayloads {
		if len(payload) == 0 {
			t.Error("stored outbox payload is empty")
		}
	}
}

func TestWalletOpeningStorePersistsZeroOpeningWithoutFinancialArtifacts(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	service := mustMemoryOpenWalletService(t, store)

	if _, err := service.OpenWallet(context.Background(), memoryOpenWalletCommand(t, "0.00")); err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 1 {
		t.Errorf("wallets count = %d; want 1", len(store.wallets))
	}
	if len(store.transactions) != 0 || len(store.ledgerEntries) != 0 || len(store.outboxPayloads) != 0 {
		t.Errorf(
			"financial artifact counts = (%d, %d, %d); want all zero",
			len(store.transactions),
			len(store.ledgerEntries),
			len(store.outboxPayloads),
		)
	}
}

func TestWalletOpeningStoreRejectsDuplicatePlayerCurrencyWithoutPartialWrites(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	service := mustMemoryOpenWalletService(t, store)
	command := memoryOpenWalletCommand(t, "100.00")

	if _, err := service.OpenWallet(context.Background(), command); err != nil {
		t.Fatalf("first OpenWallet() returned an unexpected error: %v", err)
	}
	if _, err := service.OpenWallet(context.Background(), command); !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second OpenWallet() error = %v; want ErrWalletAlreadyExists", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 1 || len(store.transactions) != 1 || len(store.ledgerEntries) != 1 || len(store.outboxPayloads) != 2 {
		t.Errorf(
			"artifact counts = (%d, %d, %d, %d); want first opening only",
			len(store.wallets),
			len(store.transactions),
			len(store.ledgerEntries),
			len(store.outboxPayloads),
		)
	}
}

func TestWalletOpeningStoreSerializesConcurrentDuplicateOpenings(t *testing.T) {
	t.Parallel()

	const attempts = 20
	store := NewWalletOpeningStore()
	service := mustMemoryOpenWalletService(t, store)
	command := memoryOpenWalletCommand(t, "100.00")
	start := make(chan struct{})
	errorsByAttempt := make(chan error, attempts)
	var waitGroup sync.WaitGroup

	for range attempts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := service.OpenWallet(context.Background(), command)
			errorsByAttempt <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsByAttempt)

	successes := 0
	conflicts := 0
	for err := range errorsByAttempt {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, application.ErrWalletAlreadyExists):
			conflicts++
		default:
			t.Errorf("OpenWallet() error = %v; want nil or ErrWalletAlreadyExists", err)
		}
	}
	if successes != 1 || conflicts != attempts-1 {
		t.Errorf("results = (%d successes, %d conflicts); want (1, %d)", successes, conflicts, attempts-1)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 1 || len(store.transactions) != 1 || len(store.ledgerEntries) != 1 || len(store.outboxPayloads) != 2 {
		t.Errorf(
			"artifact counts = (%d, %d, %d, %d); want one complete opening",
			len(store.wallets),
			len(store.transactions),
			len(store.ledgerEntries),
			len(store.outboxPayloads),
		)
	}
}

func TestWalletOpeningStoreRejectsCanceledContextWithoutWrites(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	wallet, opening, ledger, processedEvent, balanceEvent := validMemoryOpeningArtifacts(t, "event-1", "event-2")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.CreateWallet(ctx, wallet); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateWallet() error = %v; want context.Canceled", err)
	}
	if err := store.CreateWalletWithOpening(ctx, wallet, opening, ledger, processedEvent, balanceEvent); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateWalletWithOpening() error = %v; want context.Canceled", err)
	}
	assertMemoryStoreEmpty(t, store)
}

func TestWalletOpeningStoreRejectsDuplicateWalletID(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	wallet, _, _, _, _ := validMemoryOpeningArtifacts(t, "event-1", "event-2")
	if err := store.CreateWallet(context.Background(), wallet); err != nil {
		t.Fatalf("first CreateWallet() returned an unexpected error: %v", err)
	}

	err := store.CreateWallet(context.Background(), wallet)
	if !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second CreateWallet() error = %v; want ErrWalletAlreadyExists", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 1 {
		t.Errorf("wallets count = %d; want 1", len(store.wallets))
	}
}

func TestWalletOpeningStoreRejectsArtifactConflictsBeforeWriting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		seed func(*WalletOpeningStore, domain.WagerTransaction, domain.WalletLedgerEntry, domain.WagerTransactionProcessed, domain.WalletBalanceChanged)
	}{
		{name: "transaction ID", seed: func(store *WalletOpeningStore, opening domain.WagerTransaction, _ domain.WalletLedgerEntry, _ domain.WagerTransactionProcessed, _ domain.WalletBalanceChanged) {
			store.transactions[opening.ID()] = opening
		}},
		{name: "ledger ID", seed: func(store *WalletOpeningStore, _ domain.WagerTransaction, ledger domain.WalletLedgerEntry, _ domain.WagerTransactionProcessed, _ domain.WalletBalanceChanged) {
			store.ledgerEntries[ledger.ID()] = ledger
		}},
		{name: "wallet transaction ledger key", seed: func(store *WalletOpeningStore, _ domain.WagerTransaction, ledger domain.WalletLedgerEntry, _ domain.WagerTransactionProcessed, _ domain.WalletBalanceChanged) {
			store.ledgerByWalletTransaction[walletTransactionKey{walletID: ledger.WalletID(), transactionID: ledger.TransactionID()}] = "other-ledger"
		}},
		{name: "processed outbox ID", seed: func(store *WalletOpeningStore, _ domain.WagerTransaction, _ domain.WalletLedgerEntry, processed domain.WagerTransactionProcessed, _ domain.WalletBalanceChanged) {
			store.outboxPayloads[processed.EventID()] = []byte(`{"existing":true}`)
		}},
		{name: "balance outbox ID", seed: func(store *WalletOpeningStore, _ domain.WagerTransaction, _ domain.WalletLedgerEntry, _ domain.WagerTransactionProcessed, balance domain.WalletBalanceChanged) {
			store.outboxPayloads[balance.EventID()] = []byte(`{"existing":true}`)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewWalletOpeningStore()
			wallet, opening, ledger, processedEvent, balanceEvent := validMemoryOpeningArtifacts(t, "event-1", "event-2")
			tt.seed(store, opening, ledger, processedEvent, balanceEvent)

			if err := store.CreateWalletWithOpening(context.Background(), wallet, opening, ledger, processedEvent, balanceEvent); err == nil {
				t.Fatal("CreateWalletWithOpening() error = nil; want conflict")
			}
			store.mu.RLock()
			if len(store.wallets) != 0 {
				t.Errorf("wallets count = %d; want 0 after rejected atomic write", len(store.wallets))
			}
			store.mu.RUnlock()
		})
	}
}

func TestWalletOpeningStoreRejectsSharedOpeningEventID(t *testing.T) {
	t.Parallel()

	store := NewWalletOpeningStore()
	wallet, opening, ledger, processedEvent, balanceEvent := validMemoryOpeningArtifacts(t, "shared-event", "shared-event")

	if err := store.CreateWalletWithOpening(context.Background(), wallet, opening, ledger, processedEvent, balanceEvent); err == nil {
		t.Fatal("CreateWalletWithOpening() error = nil; want shared event ID rejection")
	}
	assertMemoryStoreEmpty(t, store)
}

func TestWalletOpeningStoreRejectsUnserializableEventsWithoutWrites(t *testing.T) {
	t.Parallel()

	wallet, opening, ledger, processedEvent, _ := validMemoryOpeningArtifacts(t, "event-1", "event-2")
	tests := []struct {
		name      string
		processed domain.WagerTransactionProcessed
		balance   domain.WalletBalanceChanged
	}{
		{name: "processed event", processed: domain.WagerTransactionProcessed{}, balance: domain.WalletBalanceChanged{}},
		{name: "balance event", processed: processedEvent, balance: domain.WalletBalanceChanged{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewWalletOpeningStore()
			if err := store.CreateWalletWithOpening(context.Background(), wallet, opening, ledger, tt.processed, tt.balance); err == nil {
				t.Fatal("CreateWalletWithOpening() error = nil; want serialization error")
			}
			assertMemoryStoreEmpty(t, store)
		})
	}
}

func mustMemoryOpenWalletService(
	t *testing.T,
	store *WalletOpeningStore,
) *application.OpenWalletService {
	t.Helper()

	var sequence atomic.Int64
	newID := func() string {
		return fmt.Sprintf("generated-%d", sequence.Add(1))
	}
	service, err := application.NewOpenWalletService(
		store,
		newID,
		func() time.Time { return time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	return service
}

func memoryOpenWalletCommand(t *testing.T, amount string) application.OpenWalletCommand {
	t.Helper()

	balance, err := domain.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	return application.OpenWalletCommand{
		PlayerID:       domain.PlayerID("player-1"),
		InitialBalance: balance,
		CorrelationID:  domain.CorrelationID("correlation-1"),
	}
}

func validMemoryOpeningArtifacts(
	t *testing.T,
	processedEventID string,
	balanceEventID string,
) (
	domain.Wallet,
	domain.WagerTransaction,
	domain.WalletLedgerEntry,
	domain.WagerTransactionProcessed,
	domain.WalletBalanceChanged,
) {
	t.Helper()

	createdAt := time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC)
	balance, err := domain.NewMoney("100.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	wallet, err := domain.NewWallet("wallet-1", "player-1", balance, createdAt)
	if err != nil {
		t.Fatalf("NewWallet() returned an unexpected error: %v", err)
	}
	opening, err := domain.NewOpeningWagerTransaction(domain.OpeningWagerTransactionInput{
		ID:        "opening-1",
		WalletID:  wallet.ID(),
		PlayerID:  wallet.PlayerID(),
		Money:     balance,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}
	ledger, err := domain.NewOpeningWalletLedgerEntry(domain.OpeningWalletLedgerEntryInput{
		ID:            "ledger-1",
		WalletID:      wallet.ID(),
		TransactionID: opening.ID(),
		Amount:        balance,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("NewOpeningWalletLedgerEntry() returned an unexpected error: %v", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(processedEventID),
		CorrelationID: "correlation-1",
	}, opening)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	balanceEvent, err := domain.NewWalletBalanceChanged(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(balanceEventID),
		CorrelationID: "correlation-1",
	}, ledger, wallet)
	if err != nil {
		t.Fatalf("NewWalletBalanceChanged() returned an unexpected error: %v", err)
	}
	return wallet, opening, ledger, processedEvent, balanceEvent
}

func assertMemoryStoreEmpty(t *testing.T, store *WalletOpeningStore) {
	t.Helper()

	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.wallets) != 0 || len(store.transactions) != 0 || len(store.ledgerEntries) != 0 || len(store.outboxPayloads) != 0 {
		t.Errorf(
			"artifact counts = (%d, %d, %d, %d); want empty store",
			len(store.wallets),
			len(store.transactions),
			len(store.ledgerEntries),
			len(store.outboxPayloads),
		)
	}
}
