package application

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

const (
	defaultLedgerLimit = 50
	maxLedgerLimit     = 100
)

var (
	ErrInvalidReadService  = errors.New("invalid read service")
	ErrInvalidLedgerLimit  = errors.New("invalid ledger limit")
	ErrInvalidLedgerCursor = errors.New("invalid ledger cursor")
)

type LedgerPage struct {
	Entries    []domain.WalletLedgerEntry
	NextCursor string
}

type ReadStore interface {
	LoadWallet(ctx context.Context, walletID domain.WalletID) (domain.Wallet, error)
	ListLedgerEntries(
		ctx context.Context,
		walletID domain.WalletID,
		after time.Time,
		afterID domain.LedgerEntryID,
		limit int,
	) ([]domain.WalletLedgerEntry, error)
	FindTransactionByID(ctx context.Context, transactionID domain.TransactionID) (domain.WagerTransaction, bool, error)
	FindBetByExternalID(
		ctx context.Context,
		providerID domain.ProviderID,
		externalID domain.ExternalTransactionID,
	) (domain.WagerTransaction, bool, error)
	WalletLedger(ctx context.Context, walletID domain.WalletID) (domain.Wallet, []domain.WalletLedgerEntry, error)
}

type ReadService struct {
	store ReadStore
}

func NewReadService(store ReadStore) (*ReadService, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is required", ErrInvalidReadService)
	}
	return &ReadService{store: store}, nil
}

func (service *ReadService) Wallet(ctx context.Context, walletID domain.WalletID) (domain.Wallet, error) {
	wallet, err := service.store.LoadWallet(ctx, walletID)
	if err != nil {
		return domain.Wallet{}, err
	}
	return wallet, nil
}

func (service *ReadService) Ledger(
	ctx context.Context,
	walletID domain.WalletID,
	cursor string,
	limit int,
) (LedgerPage, error) {
	if limit == 0 {
		limit = defaultLedgerLimit
	}
	if limit < 1 || limit > maxLedgerLimit {
		return LedgerPage{}, fmt.Errorf("read ledger: %w", ErrInvalidLedgerLimit)
	}
	after, afterID, err := decodeLedgerCursor(cursor)
	if err != nil {
		return LedgerPage{}, fmt.Errorf("read ledger: %w", err)
	}
	if _, err := service.store.LoadWallet(ctx, walletID); err != nil {
		return LedgerPage{}, err
	}
	entries, err := service.store.ListLedgerEntries(ctx, walletID, after, afterID, limit+1)
	if err != nil {
		return LedgerPage{}, fmt.Errorf("read ledger: %w", err)
	}
	page := LedgerPage{Entries: entries}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		last := page.Entries[len(page.Entries)-1]
		page.NextCursor = encodeLedgerCursor(last.CreatedAt(), last.ID())
	}
	return page, nil
}

func (service *ReadService) Transaction(ctx context.Context, transactionID domain.TransactionID) (domain.WagerTransaction, bool, error) {
	transaction, found, err := service.store.FindTransactionByID(ctx, transactionID)
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("read transaction: %w", err)
	}
	return transaction, found, nil
}

func (service *ReadService) ProviderTransaction(
	ctx context.Context,
	providerID domain.ProviderID,
	externalID domain.ExternalTransactionID,
) (domain.WagerTransaction, bool, error) {
	transaction, found, err := service.store.FindBetByExternalID(ctx, providerID, externalID)
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("read transaction: %w", err)
	}
	return transaction, found, nil
}

func encodeLedgerCursor(createdAt time.Time, id domain.LedgerEntryID) string {
	payload := createdAt.UTC().Format(time.RFC3339Nano) + "\n" + string(id)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeLedgerCursor(cursor string) (time.Time, domain.LedgerEntryID, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w", ErrInvalidLedgerCursor)
	}
	createdAtText, id, ok := strings.Cut(string(raw), "\n")
	if !ok || id == "" {
		return time.Time{}, "", fmt.Errorf("%w", ErrInvalidLedgerCursor)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtText)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w", ErrInvalidLedgerCursor)
	}
	return createdAt, domain.LedgerEntryID(id), nil
}
