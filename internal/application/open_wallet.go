package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

var (
	ErrInvalidOpenWalletService = errors.New("invalid open wallet service")
	ErrOpenWalletPersistence    = errors.New("open wallet persistence failed")
	ErrWalletAlreadyExists      = errors.New("wallet already exists")
)

type OpenWalletCommand struct {
	PlayerID       domain.PlayerID
	InitialBalance domain.Money
	CorrelationID  domain.CorrelationID
	CausationID    domain.CausationID
}

type WalletOpeningStore interface {
	CreateWallet(
		ctx context.Context,
		wallet domain.Wallet,
	) error
	CreateWalletWithOpening(
		ctx context.Context,
		wallet domain.Wallet,
		opening domain.WagerTransaction,
		ledger domain.WalletLedgerEntry,
		processedEvent domain.WagerTransactionProcessed,
		balanceChangedEvent domain.WalletBalanceChanged,
	) error
}

type OpenWalletService struct {
	store WalletOpeningStore
	newID func() string
	now   func() time.Time
}

func NewOpenWalletService(
	store WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) (*OpenWalletService, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is required", ErrInvalidOpenWalletService)
	}
	if newID == nil {
		return nil, fmt.Errorf("%w: ID generator is required", ErrInvalidOpenWalletService)
	}
	if now == nil {
		return nil, fmt.Errorf("%w: clock is required", ErrInvalidOpenWalletService)
	}

	return &OpenWalletService{
		store: store,
		newID: newID,
		now:   now,
	}, nil
}

func (service *OpenWalletService) OpenWallet(
	ctx context.Context,
	command OpenWalletCommand,
) (domain.Wallet, error) {
	openedAt := service.now()
	wallet, err := domain.NewWallet(
		domain.WalletID(service.newID()),
		command.PlayerID,
		command.InitialBalance,
		openedAt,
	)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("open wallet: %w", err)
	}

	if wallet.Balance().IsZero() {
		if err := service.store.CreateWallet(ctx, wallet); err != nil {
			return domain.Wallet{}, fmt.Errorf(
				"open wallet: %w: %w",
				ErrOpenWalletPersistence,
				err,
			)
		}
		return wallet, nil
	}

	opening, err := domain.NewOpeningWagerTransaction(
		domain.OpeningWagerTransactionInput{
			ID:        domain.TransactionID(service.newID()),
			WalletID:  wallet.ID(),
			PlayerID:  wallet.PlayerID(),
			Money:     wallet.Balance(),
			CreatedAt: openedAt,
		},
	)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("open wallet: create opening transaction: %w", err)
	}

	ledger, err := domain.NewOpeningWalletLedgerEntry(domain.OpeningWalletLedgerEntryInput{
		ID:            domain.LedgerEntryID(service.newID()),
		WalletID:      wallet.ID(),
		TransactionID: opening.ID(),
		Amount:        wallet.Balance(),
		CreatedAt:     openedAt,
	})
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("open wallet: create opening ledger: %w", err)
	}

	processedEvent, err := domain.NewWagerTransactionProcessed(
		domain.DomainEventMetadataInput{
			EventID:       domain.DomainEventID(service.newID()),
			CorrelationID: command.CorrelationID,
			CausationID:   command.CausationID,
		},
		opening,
	)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("open wallet: create processed event: %w", err)
	}

	balanceChangedEvent, err := domain.NewWalletBalanceChanged(
		domain.DomainEventMetadataInput{
			EventID:       domain.DomainEventID(service.newID()),
			CorrelationID: command.CorrelationID,
			CausationID:   command.CausationID,
		},
		ledger,
		wallet,
	)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("open wallet: create balance event: %w", err)
	}

	if err := service.store.CreateWalletWithOpening(
		ctx,
		wallet,
		opening,
		ledger,
		processedEvent,
		balanceChangedEvent,
	); err != nil {
		return domain.Wallet{}, fmt.Errorf(
			"open wallet: %w: %w",
			ErrOpenWalletPersistence,
			err,
		)
	}

	return wallet, nil
}
