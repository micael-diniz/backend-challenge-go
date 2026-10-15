package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

var (
	ErrInvalidPlaceBetService = errors.New("invalid place bet service")
	ErrPlaceBetPersistence    = errors.New("place bet persistence failed")
	ErrWalletNotFound         = errors.New("wallet not found")
	ErrBetPlayerMismatch      = errors.New("bet player does not match wallet")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrWalletVersionConflict  = errors.New("wallet version conflict")
	ErrReferenceNotFound      = errors.New("reference not found")
	ErrInvalidReference       = errors.New("invalid reference")
)

const placeBetAttempts = 3

type PlaceBetCommand struct {
	WalletID                       domain.WalletID
	PlayerID                       domain.PlayerID
	ProviderID                     domain.ProviderID
	ExternalTransactionID          domain.ExternalTransactionID
	IdempotencyKey                 domain.IdempotencyKey
	RoundID                        domain.RoundID
	GameID                         domain.GameID
	Money                          domain.Money
	ReferenceExternalTransactionID domain.ExternalTransactionID
	CorrelationID                  domain.CorrelationID
	CausationID                    domain.CausationID
}

type BetStore interface {
	LoadWallet(
		ctx context.Context,
		walletID domain.WalletID,
	) (domain.Wallet, error)
	SaveProcessedBet(
		ctx context.Context,
		wallet domain.Wallet,
		bet domain.WagerTransaction,
		ledger domain.WalletLedgerEntry,
		processedEvent domain.WagerTransactionProcessed,
		balanceChangedEvent domain.WalletBalanceChanged,
	) error
	FindBetByIdempotencyKey(
		ctx context.Context,
		providerID domain.ProviderID,
		key domain.IdempotencyKey,
	) (domain.WagerTransaction, bool, error)
	FindBetByExternalID(
		ctx context.Context,
		providerID domain.ProviderID,
		externalID domain.ExternalTransactionID,
	) (domain.WagerTransaction, bool, error)
	SaveRejectedBet(
		ctx context.Context,
		bet domain.WagerTransaction,
		rejectedEvent domain.WagerTransactionRejected,
	) error
	SaveProcessedLoss(
		ctx context.Context,
		loss domain.WagerTransaction,
		processedEvent domain.WagerTransactionProcessed,
	) error
	SaveProcessedWin(
		ctx context.Context,
		wallet domain.Wallet,
		win domain.WagerTransaction,
		ledger domain.WalletLedgerEntry,
		processedEvent domain.WagerTransactionProcessed,
		balanceChangedEvent domain.WalletBalanceChanged,
	) error
}

type PlaceBetService struct {
	store BetStore
	newID func() string
	now   func() time.Time
}

func NewPlaceBetService(
	store BetStore,
	newID func() string,
	now func() time.Time,
) (*PlaceBetService, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is required", ErrInvalidPlaceBetService)
	}
	if newID == nil {
		return nil, fmt.Errorf("%w: ID generator is required", ErrInvalidPlaceBetService)
	}
	if now == nil {
		return nil, fmt.Errorf("%w: clock is required", ErrInvalidPlaceBetService)
	}

	return &PlaceBetService{
		store: store,
		newID: newID,
		now:   now,
	}, nil
}

func (service *PlaceBetService) PlaceBet(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	if wallet, bet, replay, err := service.existingBet(ctx, command, domain.WagerTransactionKindBet); err != nil || replay {
		return wallet, bet, replay, err
	}

	var lastConflict error
	for attempt := 0; attempt < placeBetAttempts; attempt++ {
		wallet, bet, err := service.placeNewBet(ctx, command)
		if errors.Is(err, ErrWalletVersionConflict) {
			lastConflict = err
			continue
		}
		return wallet, bet, false, err
	}
	return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
		"place bet: %w: %w",
		ErrPlaceBetPersistence,
		lastConflict,
	)
}

func (service *PlaceBetService) placeNewBet(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, error) {
	wallet, err := service.store.LoadWallet(ctx, command.WalletID)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"place bet: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	if command.PlayerID != wallet.PlayerID() {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"place bet: %w: player ID=%q, wallet player ID=%q",
			ErrBetPlayerMismatch,
			command.PlayerID,
			wallet.PlayerID(),
		)
	}

	placedAt := service.now()
	hash, err := betPayloadHash(command, domain.WagerTransactionKindBet)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: payload hash: %w", err)
	}
	pending, err := domain.NewExternalWagerTransaction(domain.ExternalWagerTransactionInput{
		ID:                    domain.TransactionID(service.newID()),
		ProviderID:            command.ProviderID,
		ExternalTransactionID: command.ExternalTransactionID,
		IdempotencyKey:        command.IdempotencyKey,
		PayloadHash:           hash,
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		RoundID:               command.RoundID,
		GameID:                command.GameID,
		Kind:                  domain.WagerTransactionKindBet,
		Money:                 command.Money,
		CreatedAt:             placedAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: create bet: %w", err)
	}

	balanceBefore := wallet.Balance()
	debited, err := wallet.Debit(command.Money, placedAt)
	if errors.Is(err, domain.ErrInsufficientFunds) {
		if err := service.saveInsufficientFunds(ctx, command, pending, placedAt); err != nil {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: debit wallet: %w", err)
	}
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: debit wallet: %w", err)
	}

	processed, err := pending.Process(domain.ProcessWagerTransactionInput{
		ResultBalance: debited.Balance(),
		OccurredAt:    placedAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: process bet: %w", err)
	}

	ledger, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            domain.LedgerEntryID(service.newID()),
		WalletID:      debited.ID(),
		TransactionID: processed.ID(),
		Direction:     domain.LedgerDirectionDebit,
		Amount:        command.Money,
		BalanceBefore: balanceBefore,
		BalanceAfter:  debited.Balance(),
		CreatedAt:     placedAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: create ledger: %w", err)
	}

	processedEvent, err := domain.NewWagerTransactionProcessed(
		domain.DomainEventMetadataInput{
			EventID:       domain.DomainEventID(service.newID()),
			CorrelationID: command.CorrelationID,
			CausationID:   command.CausationID,
		},
		processed,
	)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: create processed event: %w", err)
	}

	balanceChangedEvent, err := domain.NewWalletBalanceChanged(
		domain.DomainEventMetadataInput{
			EventID:       domain.DomainEventID(service.newID()),
			CorrelationID: command.CorrelationID,
			CausationID:   command.CausationID,
		},
		ledger,
		debited,
	)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("place bet: create balance event: %w", err)
	}

	if err := service.store.SaveProcessedBet(
		ctx,
		debited,
		processed,
		ledger,
		processedEvent,
		balanceChangedEvent,
	); err != nil {
		if errors.Is(err, ErrWalletVersionConflict) {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"place bet: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}

	return debited, processed, nil
}

func (service *PlaceBetService) saveInsufficientFunds(
	ctx context.Context,
	command PlaceBetCommand,
	pending domain.WagerTransaction,
	occurredAt time.Time,
) error {
	rejected, err := pending.Reject(domain.RejectWagerTransactionInput{
		FailureCode: domain.WagerTransactionFailureCodeInsufficientFunds,
		OccurredAt:  occurredAt,
	})
	if err != nil {
		return fmt.Errorf("place bet: reject bet: %w", err)
	}
	rejectedEvent, err := domain.NewWagerTransactionRejected(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, rejected)
	if err != nil {
		return fmt.Errorf("place bet: create rejected event: %w", err)
	}
	if err := service.store.SaveRejectedBet(ctx, rejected, rejectedEvent); err != nil {
		return fmt.Errorf("place bet: %w: %w", ErrPlaceBetPersistence, err)
	}
	return nil
}

func (service *PlaceBetService) existingBet(
	ctx context.Context,
	command PlaceBetCommand,
	kind domain.WagerTransactionKind,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	hash, err := betPayloadHash(command, kind)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("place bet: payload hash: %w", err)
	}
	stored, found, err := service.store.FindBetByIdempotencyKey(ctx, command.ProviderID, command.IdempotencyKey)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"place bet: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	if found {
		if stored.PayloadHash() == hash {
			wallet, err := service.store.LoadWallet(ctx, stored.WalletID())
			if err != nil {
				return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
					"place bet: %w: %w",
					ErrPlaceBetPersistence,
					err,
				)
			}
			return wallet, stored, true, nil
		}
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("place bet: %w", ErrIdempotencyConflict)
	}

	stored, found, err = service.store.FindBetByExternalID(ctx, command.ProviderID, command.ExternalTransactionID)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"place bet: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	if found {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("place bet: %w", ErrIdempotencyConflict)
	}
	return domain.Wallet{}, domain.WagerTransaction{}, false, nil
}

func betPayloadHash(command PlaceBetCommand, kind domain.WagerTransactionKind) (domain.PayloadHash, error) {
	payload := map[string]any{
		"externalTransactionId": string(command.ExternalTransactionID),
		"gameId":                string(command.GameID),
		"kind":                  string(kind),
		"money": map[string]string{
			"amount":   command.Money.Amount(),
			"currency": command.Money.Currency().String(),
		},
		"playerId":   string(command.PlayerID),
		"providerId": string(command.ProviderID),
		"roundId":    string(command.RoundID),
		"walletId":   string(command.WalletID),
	}
	if kind == domain.WagerTransactionKindWin {
		payload["referenceExternalTransactionId"] = string(command.ReferenceExternalTransactionID)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return domain.PayloadHash(hex.EncodeToString(sum[:])), nil
}
