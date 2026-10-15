package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func (service *PlaceBetService) RecordWin(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	if wallet, win, replay, err := service.existingBet(ctx, command, domain.WagerTransactionKindWin); err != nil || replay {
		return wallet, win, replay, err
	}

	var lastConflict error
	for attempt := 0; attempt < placeBetAttempts; attempt++ {
		wallet, win, err := service.recordNewWin(ctx, command)
		if errors.Is(err, ErrWalletVersionConflict) {
			lastConflict = err
			continue
		}
		return wallet, win, false, err
	}
	return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
		"record win: %w: %w",
		ErrPlaceBetPersistence,
		lastConflict,
	)
}

func (service *PlaceBetService) recordNewWin(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, error) {
	wallet, err := service.store.LoadWallet(ctx, command.WalletID)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"record win: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	if command.PlayerID != wallet.PlayerID() {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"record win: %w: player ID=%q, wallet player ID=%q",
			ErrBetPlayerMismatch,
			command.PlayerID,
			wallet.PlayerID(),
		)
	}
	if command.Money.Currency() != wallet.Currency() {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"record win: %w",
			domain.ErrCurrencyMismatch,
		)
	}
	resolvedReferenceID, err := service.resolvedWinReference(ctx, command, wallet)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, err
	}

	occurredAt := service.now()
	hash, err := betPayloadHash(command, domain.WagerTransactionKindWin)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: payload hash: %w", err)
	}
	pending, err := domain.NewExternalWagerTransaction(domain.ExternalWagerTransactionInput{
		ID:                             domain.TransactionID(service.newID()),
		ProviderID:                     command.ProviderID,
		ExternalTransactionID:          command.ExternalTransactionID,
		IdempotencyKey:                 command.IdempotencyKey,
		PayloadHash:                    hash,
		WalletID:                       wallet.ID(),
		PlayerID:                       wallet.PlayerID(),
		RoundID:                        command.RoundID,
		GameID:                         command.GameID,
		Kind:                           domain.WagerTransactionKindWin,
		Money:                          command.Money,
		ReferenceExternalTransactionID: command.ReferenceExternalTransactionID,
		CreatedAt:                      occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: create win: %w", err)
	}

	balanceBefore := wallet.Balance()
	credited, err := wallet.Credit(command.Money, occurredAt)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: credit wallet: %w", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{
		ResultBalance:                  credited.Balance(),
		ResolvedReferenceTransactionID: resolvedReferenceID,
		OccurredAt:                     occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: process win: %w", err)
	}
	ledger, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            domain.LedgerEntryID(service.newID()),
		WalletID:      credited.ID(),
		TransactionID: processed.ID(),
		Direction:     domain.LedgerDirectionCredit,
		Amount:        command.Money,
		BalanceBefore: balanceBefore,
		BalanceAfter:  credited.Balance(),
		CreatedAt:     occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: create ledger: %w", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, processed)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: create processed event: %w", err)
	}
	balanceChangedEvent, err := domain.NewWalletBalanceChanged(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, ledger, credited)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record win: create balance event: %w", err)
	}
	if err := service.store.SaveProcessedWin(ctx, credited, processed, ledger, processedEvent, balanceChangedEvent); err != nil {
		if errors.Is(err, ErrWalletVersionConflict) {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf(
			"record win: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	return credited, processed, nil
}

func (service *PlaceBetService) resolvedWinReference(
	ctx context.Context,
	command PlaceBetCommand,
	wallet domain.Wallet,
) (domain.TransactionID, error) {
	if command.ReferenceExternalTransactionID == "" {
		return "", nil
	}
	reference, found, err := service.store.FindBetByExternalID(
		ctx,
		command.ProviderID,
		command.ReferenceExternalTransactionID,
	)
	if err != nil {
		return "", fmt.Errorf("record win: %w: %w", ErrPlaceBetPersistence, err)
	}
	if !found {
		return "", fmt.Errorf("record win: %w", ErrReferenceNotFound)
	}
	if reference.Kind() != domain.WagerTransactionKindBet || reference.Status() != domain.WagerTransactionStatusProcessed {
		return "", fmt.Errorf("record win: %w", ErrInvalidReference)
	}
	if reference.PlayerID() != wallet.PlayerID() ||
		reference.WalletID() != wallet.ID() ||
		reference.RoundID() != command.RoundID ||
		reference.Money().Currency() != wallet.Currency() {
		return "", fmt.Errorf("record win: %w", ErrInvalidReference)
	}
	return reference.ID(), nil
}
