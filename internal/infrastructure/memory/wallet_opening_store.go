package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

type walletOwnerCurrencyKey struct {
	playerID domain.PlayerID
	currency domain.Currency
}

type walletTransactionKey struct {
	walletID      domain.WalletID
	transactionID domain.TransactionID
}

type WalletOpeningStore struct {
	mu                        sync.RWMutex
	wallets                   map[domain.WalletID]domain.Wallet
	walletsByOwnerCurrency    map[walletOwnerCurrencyKey]domain.WalletID
	transactions              map[domain.TransactionID]domain.WagerTransaction
	ledgerEntries             map[domain.LedgerEntryID]domain.WalletLedgerEntry
	ledgerByWalletTransaction map[walletTransactionKey]domain.LedgerEntryID
	outboxPayloads            map[domain.DomainEventID][]byte
}

func NewWalletOpeningStore() *WalletOpeningStore {
	return &WalletOpeningStore{
		wallets:                   make(map[domain.WalletID]domain.Wallet),
		walletsByOwnerCurrency:    make(map[walletOwnerCurrencyKey]domain.WalletID),
		transactions:              make(map[domain.TransactionID]domain.WagerTransaction),
		ledgerEntries:             make(map[domain.LedgerEntryID]domain.WalletLedgerEntry),
		ledgerByWalletTransaction: make(map[walletTransactionKey]domain.LedgerEntryID),
		outboxPayloads:            make(map[domain.DomainEventID][]byte),
	}
}

func (store *WalletOpeningStore) CreateWallet(
	ctx context.Context,
	wallet domain.Wallet,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateWalletUniqueness(wallet); err != nil {
		return err
	}

	store.insertWallet(wallet)
	return nil
}

func (store *WalletOpeningStore) CreateWalletWithOpening(
	ctx context.Context,
	wallet domain.Wallet,
	opening domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	processedPayload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize processed opening event: %w", err)
	}
	balancePayload, err := json.Marshal(balanceChangedEvent)
	if err != nil {
		return fmt.Errorf("serialize opening balance event: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateWalletUniqueness(wallet); err != nil {
		return err
	}
	if _, exists := store.transactions[opening.ID()]; exists {
		return fmt.Errorf("opening transaction ID %q already exists", opening.ID())
	}
	if _, exists := store.ledgerEntries[ledger.ID()]; exists {
		return fmt.Errorf("ledger entry ID %q already exists", ledger.ID())
	}
	ledgerKey := walletTransactionKey{
		walletID:      ledger.WalletID(),
		transactionID: ledger.TransactionID(),
	}
	if _, exists := store.ledgerByWalletTransaction[ledgerKey]; exists {
		return fmt.Errorf(
			"ledger entry for wallet %q and transaction %q already exists",
			ledger.WalletID(),
			ledger.TransactionID(),
		)
	}
	if _, exists := store.outboxPayloads[processedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", processedEvent.EventID())
	}
	if _, exists := store.outboxPayloads[balanceChangedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", balanceChangedEvent.EventID())
	}
	if processedEvent.EventID() == balanceChangedEvent.EventID() {
		return fmt.Errorf("opening events share event ID %q", processedEvent.EventID())
	}

	store.insertWallet(wallet)
	store.transactions[opening.ID()] = opening
	store.ledgerEntries[ledger.ID()] = ledger
	store.ledgerByWalletTransaction[ledgerKey] = ledger.ID()
	store.outboxPayloads[processedEvent.EventID()] = processedPayload
	store.outboxPayloads[balanceChangedEvent.EventID()] = balancePayload
	return nil
}

func (store *WalletOpeningStore) LoadWallet(
	ctx context.Context,
	walletID domain.WalletID,
) (domain.Wallet, error) {
	if err := ctx.Err(); err != nil {
		return domain.Wallet{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	wallet, exists := store.wallets[walletID]
	if !exists {
		return domain.Wallet{}, fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, walletID)
	}
	return wallet, nil
}

func (store *WalletOpeningStore) ListLedgerEntries(
	ctx context.Context,
	walletID domain.WalletID,
	after time.Time,
	afterID domain.LedgerEntryID,
	limit int,
) ([]domain.WalletLedgerEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	entries := make([]domain.WalletLedgerEntry, 0)
	for _, entry := range store.ledgerEntries {
		if entry.WalletID() != walletID || !ledgerEntryAfter(entry, after, afterID) {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		if !entries[left].CreatedAt().Equal(entries[right].CreatedAt()) {
			return entries[left].CreatedAt().Before(entries[right].CreatedAt())
		}
		return entries[left].ID() < entries[right].ID()
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (store *WalletOpeningStore) FindTransactionByID(
	ctx context.Context,
	transactionID domain.TransactionID,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, func(transaction domain.WagerTransaction) bool {
		return transaction.ID() == transactionID
	})
}

func ledgerEntryAfter(entry domain.WalletLedgerEntry, after time.Time, afterID domain.LedgerEntryID) bool {
	if after.IsZero() && afterID == "" {
		return true
	}
	if entry.CreatedAt().After(after) {
		return true
	}
	return entry.CreatedAt().Equal(after) && entry.ID() > afterID
}

func (store *WalletOpeningStore) FindBetByIdempotencyKey(
	ctx context.Context,
	providerID domain.ProviderID,
	key domain.IdempotencyKey,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, func(bet domain.WagerTransaction) bool {
		return bet.ProviderID() == providerID && bet.IdempotencyKey() == key
	})
}

func (store *WalletOpeningStore) FindProcessedReversal(
	ctx context.Context,
	providerID domain.ProviderID,
	referenceID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, func(bet domain.WagerTransaction) bool {
		reference, present := bet.ReferenceExternalTransactionID()
		return present &&
			reference == referenceID &&
			bet.ProviderID() == providerID &&
			bet.Status() == domain.WagerTransactionStatusProcessed &&
			(bet.Kind() == domain.WagerTransactionKindRefund || bet.Kind() == domain.WagerTransactionKindRollback)
	})
}

func (store *WalletOpeningStore) FindBetByExternalID(
	ctx context.Context,
	providerID domain.ProviderID,
	externalID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	return store.findBet(ctx, func(bet domain.WagerTransaction) bool {
		return bet.ProviderID() == providerID && bet.ExternalTransactionID() == externalID
	})
}

func (store *WalletOpeningStore) findBet(
	ctx context.Context,
	matches func(domain.WagerTransaction) bool,
) (domain.WagerTransaction, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.WagerTransaction{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, bet := range store.transactions {
		if matches(bet) {
			return bet, true, nil
		}
	}
	return domain.WagerTransaction{}, false, nil
}

func (store *WalletOpeningStore) SaveProcessedBet(
	ctx context.Context,
	wallet domain.Wallet,
	bet domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	processedPayload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize processed bet event: %w", err)
	}
	balancePayload, err := json.Marshal(balanceChangedEvent)
	if err != nil {
		return fmt.Errorf("serialize bet balance event: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.wallets[wallet.ID()]; !exists {
		return fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, wallet.ID())
	}
	if _, exists := store.transactions[bet.ID()]; exists {
		return fmt.Errorf("bet transaction ID %q already exists", bet.ID())
	}
	if _, exists := store.ledgerEntries[ledger.ID()]; exists {
		return fmt.Errorf("ledger entry ID %q already exists", ledger.ID())
	}
	ledgerKey := walletTransactionKey{
		walletID:      ledger.WalletID(),
		transactionID: ledger.TransactionID(),
	}
	if _, exists := store.ledgerByWalletTransaction[ledgerKey]; exists {
		return fmt.Errorf(
			"ledger entry for wallet %q and transaction %q already exists",
			ledger.WalletID(),
			ledger.TransactionID(),
		)
	}
	if _, exists := store.outboxPayloads[processedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", processedEvent.EventID())
	}
	if _, exists := store.outboxPayloads[balanceChangedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", balanceChangedEvent.EventID())
	}
	if processedEvent.EventID() == balanceChangedEvent.EventID() {
		return fmt.Errorf("bet events share event ID %q", processedEvent.EventID())
	}

	store.wallets[wallet.ID()] = wallet
	store.transactions[bet.ID()] = bet
	store.ledgerEntries[ledger.ID()] = ledger
	store.ledgerByWalletTransaction[ledgerKey] = ledger.ID()
	store.outboxPayloads[processedEvent.EventID()] = processedPayload
	store.outboxPayloads[balanceChangedEvent.EventID()] = balancePayload
	return nil
}

func (store *WalletOpeningStore) SaveRejectedBet(
	ctx context.Context,
	bet domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(rejectedEvent)
	if err != nil {
		return fmt.Errorf("serialize rejected bet event: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.wallets[bet.WalletID()]; !exists {
		return fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, bet.WalletID())
	}
	if _, exists := store.transactions[bet.ID()]; exists {
		return fmt.Errorf("bet transaction ID %q already exists", bet.ID())
	}
	if _, exists := store.outboxPayloads[rejectedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", rejectedEvent.EventID())
	}
	store.transactions[bet.ID()] = bet
	store.outboxPayloads[rejectedEvent.EventID()] = payload
	return nil
}

func (store *WalletOpeningStore) SaveProcessedLoss(
	ctx context.Context,
	loss domain.WagerTransaction,
	processedEvent domain.WagerTransactionProcessed,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(processedEvent)
	if err != nil {
		return fmt.Errorf("serialize processed loss event: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.wallets[loss.WalletID()]; !exists {
		return fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, loss.WalletID())
	}
	if _, exists := store.transactions[loss.ID()]; exists {
		return fmt.Errorf("loss transaction ID %q already exists", loss.ID())
	}
	if _, exists := store.outboxPayloads[processedEvent.EventID()]; exists {
		return fmt.Errorf("outbox event ID %q already exists", processedEvent.EventID())
	}
	store.transactions[loss.ID()] = loss
	store.outboxPayloads[processedEvent.EventID()] = payload
	return nil
}

func (store *WalletOpeningStore) SaveProcessedReversal(
	ctx context.Context,
	wallet domain.Wallet,
	reversal domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	return store.SaveProcessedBet(ctx, wallet, reversal, ledger, processedEvent, balanceChangedEvent)
}

func (store *WalletOpeningStore) SaveRejectedReversal(
	ctx context.Context,
	reversal domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	return store.SaveRejectedBet(ctx, reversal, rejectedEvent)
}

func (store *WalletOpeningStore) SaveProcessedWin(
	ctx context.Context,
	wallet domain.Wallet,
	win domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	return store.SaveProcessedBet(ctx, wallet, win, ledger, processedEvent, balanceChangedEvent)
}

func (store *WalletOpeningStore) SavePendingReference(
	ctx context.Context,
	transaction domain.WagerTransaction,
	pendingEvent domain.WagerTransactionPendingReference,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(pendingEvent)
	if err != nil {
		return fmt.Errorf("serialize pending reference event: %w", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.wallets[transaction.WalletID()]; !exists {
		return fmt.Errorf("%w: wallet ID=%q", application.ErrWalletNotFound, transaction.WalletID())
	}
	if _, exists := store.transactions[transaction.ID()]; exists {
		return fmt.Errorf("transaction ID %q already exists", transaction.ID())
	}
	store.transactions[transaction.ID()] = transaction
	store.outboxPayloads[pendingEvent.EventID()] = payload
	return nil
}

func (store *WalletOpeningStore) ListDuePendingReferences(
	ctx context.Context,
	dueAt time.Time,
) ([]domain.WagerTransaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	var due []domain.WagerTransaction
	for _, transaction := range store.transactions {
		next, present := transaction.NextReferenceAttemptAt()
		if transaction.Status() == domain.WagerTransactionStatusPendingReference && present && !next.After(dueAt) {
			due = append(due, transaction)
		}
	}
	return due, nil
}

func (store *WalletOpeningStore) SavePendingReferenceRetry(
	ctx context.Context,
	transaction domain.WagerTransaction,
	pendingEvent domain.WagerTransactionPendingReference,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(pendingEvent)
	if err != nil {
		return fmt.Errorf("serialize pending reference retry: %w", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.transactions[transaction.ID()] = transaction
	store.outboxPayloads[pendingEvent.EventID()] = payload
	return nil
}

func (store *WalletOpeningStore) RejectPendingReference(
	ctx context.Context,
	transaction domain.WagerTransaction,
	rejectedEvent domain.WagerTransactionRejected,
) error {
	store.mu.Lock()
	delete(store.transactions, transaction.ID())
	store.mu.Unlock()
	return store.SaveRejectedBet(ctx, transaction, rejectedEvent)
}

func (store *WalletOpeningStore) ResolvePendingReference(
	ctx context.Context,
	wallet domain.Wallet,
	transaction domain.WagerTransaction,
	ledger domain.WalletLedgerEntry,
	processedEvent domain.WagerTransactionProcessed,
	balanceChangedEvent domain.WalletBalanceChanged,
) error {
	if _, exists := store.transactions[transaction.ID()]; exists {
		delete(store.transactions, transaction.ID())
	}
	return store.SaveProcessedBet(ctx, wallet, transaction, ledger, processedEvent, balanceChangedEvent)
}

func (store *WalletOpeningStore) validateWalletUniqueness(wallet domain.Wallet) error {
	if _, exists := store.wallets[wallet.ID()]; exists {
		return fmt.Errorf("%w: wallet ID=%q", application.ErrWalletAlreadyExists, wallet.ID())
	}
	key := walletOwnerCurrencyKey{
		playerID: wallet.PlayerID(),
		currency: wallet.Currency(),
	}
	if existingID, exists := store.walletsByOwnerCurrency[key]; exists {
		return fmt.Errorf(
			"%w: player ID=%q, currency=%s, wallet ID=%q",
			application.ErrWalletAlreadyExists,
			wallet.PlayerID(),
			wallet.Currency(),
			existingID,
		)
	}

	return nil
}

func (store *WalletOpeningStore) insertWallet(wallet domain.Wallet) {
	store.wallets[wallet.ID()] = wallet
	store.walletsByOwnerCurrency[walletOwnerCurrencyKey{
		playerID: wallet.PlayerID(),
		currency: wallet.Currency(),
	}] = wallet.ID()
}
