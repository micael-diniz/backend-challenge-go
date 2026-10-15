package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func (service *PlaceBetService) RecordRefund(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	return service.recordReversal(ctx, command, domain.WagerTransactionKindRefund)
}

func (service *PlaceBetService) RecordRollback(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	return service.recordReversal(ctx, command, domain.WagerTransactionKindRollback)
}

func (service *PlaceBetService) recordReversal(
	ctx context.Context,
	command PlaceBetCommand,
	kind domain.WagerTransactionKind,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	if wallet, reversal, replay, err := service.existingBet(ctx, command, kind); err != nil || replay {
		return wallet, reversal, replay, err
	}
	wallet, reversal, err := service.retryVersionConflict("record reversal", func() (domain.Wallet, domain.WagerTransaction, error) {
		return service.recordNewReversal(ctx, command, kind)
	})
	return wallet, reversal, false, err
}

func (service *PlaceBetService) recordNewReversal(
	ctx context.Context,
	command PlaceBetCommand,
	kind domain.WagerTransactionKind,
) (domain.Wallet, domain.WagerTransaction, error) {
	if command.ReferenceExternalTransactionID == "" {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrInvalidReference)
	}
	wallet, err := service.loadWalletForCommand(ctx, command, "record reversal")
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, err
	}
	if command.Money.Currency() != wallet.Currency() {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", domain.ErrCurrencyMismatch)
	}
	reference, err := service.reversalReference(ctx, command, wallet, kind)
	if errors.Is(err, ErrReferenceNotFound) {
		return service.holdMissingReference(ctx, command, kind, wallet)
	}
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, err
	}
	if command.Money.MinorUnits() != reference.Money().MinorUnits() {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrInvalidReference)
	}
	occurredAt := service.now()
	hash, err := betPayloadHash(command, kind)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: payload hash: %w", err)
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
		Kind:                           kind,
		Money:                          command.Money,
		ReferenceExternalTransactionID: command.ReferenceExternalTransactionID,
		CreatedAt:                      occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: create reversal: %w", err)
	}
	duplicate, found, err := service.store.FindProcessedReversal(ctx, command.ProviderID, command.ReferenceExternalTransactionID)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w: %w", ErrPlaceBetPersistence, err)
	}
	if found && duplicate.ID() != pending.ID() {
		if err := service.saveRejectedReversal(ctx, command, pending, occurredAt, domain.WagerTransactionFailureCodeDuplicateReversal); err != nil {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrDuplicateReversal)
	}

	credit, err := reversalCredits(kind, reference.Kind())
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, err
	}
	balanceBefore := wallet.Balance()
	var moved domain.Wallet
	if credit {
		moved, err = wallet.Credit(command.Money, occurredAt)
	} else {
		moved, err = wallet.Debit(command.Money, occurredAt)
	}
	if errors.Is(err, domain.ErrInsufficientFunds) {
		if err := service.saveRejectedReversal(ctx, command, pending, occurredAt, domain.WagerTransactionFailureCodeReversalInsufficientFunds); err != nil {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrReversalInsufficientFunds)
	}
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: move wallet: %w", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{
		ResultBalance:                  moved.Balance(),
		ResolvedReferenceTransactionID: reference.ID(),
		OccurredAt:                     occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: process reversal: %w", err)
	}
	direction := domain.LedgerDirectionDebit
	if credit {
		direction = domain.LedgerDirectionCredit
	}
	ledger, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            domain.LedgerEntryID(service.newID()),
		WalletID:      moved.ID(),
		TransactionID: processed.ID(),
		Direction:     direction,
		Amount:        command.Money,
		BalanceBefore: balanceBefore,
		BalanceAfter:  moved.Balance(),
		CreatedAt:     occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: create ledger: %w", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, processed)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: create processed event: %w", err)
	}
	balanceChangedEvent, err := domain.NewWalletBalanceChanged(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, ledger, moved)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: create balance event: %w", err)
	}
	if err := service.store.SaveProcessedReversal(ctx, moved, processed, ledger, processedEvent, balanceChangedEvent); err != nil {
		if errors.Is(err, ErrWalletVersionConflict) || errors.Is(err, ErrDuplicateReversal) {
			return domain.Wallet{}, domain.WagerTransaction{}, err
		}
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("record reversal: %w: %w", ErrPlaceBetPersistence, err)
	}
	return moved, processed, nil
}

func (service *PlaceBetService) reversalReference(
	ctx context.Context,
	command PlaceBetCommand,
	wallet domain.Wallet,
	kind domain.WagerTransactionKind,
) (domain.WagerTransaction, error) {
	reference, found, err := service.store.FindBetByExternalID(ctx, command.ProviderID, command.ReferenceExternalTransactionID)
	if err != nil {
		return domain.WagerTransaction{}, fmt.Errorf("record reversal: %w: %w", ErrPlaceBetPersistence, err)
	}
	if !found || referenceWaiting(reference) {
		return domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrReferenceNotFound)
	}
	if reference.Status() != domain.WagerTransactionStatusProcessed || !reversalKindAllows(kind, reference.Kind()) {
		return domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrInvalidReference)
	}
	if reference.PlayerID() != wallet.PlayerID() ||
		reference.WalletID() != wallet.ID() ||
		reference.RoundID() != command.RoundID ||
		reference.Money().Currency() != wallet.Currency() {
		return domain.WagerTransaction{}, fmt.Errorf("record reversal: %w", ErrInvalidReference)
	}
	return reference, nil
}

func reversalKindAllows(kind domain.WagerTransactionKind, referenceKind domain.WagerTransactionKind) bool {
	switch kind {
	case domain.WagerTransactionKindRefund:
		return referenceKind == domain.WagerTransactionKindBet
	case domain.WagerTransactionKindRollback:
		return referenceKind == domain.WagerTransactionKindBet ||
			referenceKind == domain.WagerTransactionKindWin ||
			referenceKind == domain.WagerTransactionKindRefund
	default:
		return false
	}
}

func reversalCredits(kind domain.WagerTransactionKind, referenceKind domain.WagerTransactionKind) (bool, error) {
	switch kind {
	case domain.WagerTransactionKindRefund:
		return true, nil
	case domain.WagerTransactionKindRollback:
		return referenceKind == domain.WagerTransactionKindBet, nil
	default:
		return false, fmt.Errorf("record reversal: %w", ErrInvalidReference)
	}
}

func (service *PlaceBetService) saveRejectedReversal(
	ctx context.Context,
	command PlaceBetCommand,
	pending domain.WagerTransaction,
	occurredAt time.Time,
	code domain.WagerTransactionFailureCode,
) error {
	rejected, err := pending.Reject(domain.RejectWagerTransactionInput{
		FailureCode: code,
		OccurredAt:  occurredAt,
	})
	if err != nil {
		return fmt.Errorf("record reversal: reject reversal: %w", err)
	}
	rejectedEvent, err := domain.NewWagerTransactionRejected(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, rejected)
	if err != nil {
		return fmt.Errorf("record reversal: create rejected event: %w", err)
	}
	if err := service.store.SaveRejectedReversal(ctx, rejected, rejectedEvent); err != nil {
		return fmt.Errorf("record reversal: %w: %w", ErrPlaceBetPersistence, err)
	}
	return nil
}
