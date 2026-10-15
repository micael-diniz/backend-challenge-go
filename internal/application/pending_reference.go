package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

const (
	referenceRetryBase = time.Second
	referenceTTL       = time.Minute
)

func (service *PlaceBetService) holdMissingReference(
	ctx context.Context,
	command PlaceBetCommand,
	kind domain.WagerTransactionKind,
	wallet domain.Wallet,
) (domain.Wallet, domain.WagerTransaction, error) {
	occurredAt := service.now()
	hash, err := betPayloadHash(command, kind)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("hold reference: payload hash: %w", err)
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
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("hold reference: create transaction: %w", err)
	}
	held, err := pending.MarkPendingReference(domain.PendingReferenceWagerTransactionInput{
		OccurredAt:    occurredAt,
		NextAttemptAt: occurredAt.Add(referenceRetryBase),
		ExpiresAt:     occurredAt.Add(referenceTTL),
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("hold reference: mark pending: %w", err)
	}
	event, err := domain.NewWagerTransactionPendingReference(referenceEventMetadata(service, command.CorrelationID, held), held)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("hold reference: create event: %w", err)
	}
	if err := service.store.SavePendingReference(ctx, held, event); err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, fmt.Errorf("hold reference: %w: %w", ErrPlaceBetPersistence, err)
	}
	return wallet, held, nil
}

func (service *PlaceBetService) AdvanceDueReferences(ctx context.Context) error {
	due, err := service.store.ListDuePendingReferences(ctx, service.now())
	if err != nil {
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	for _, pending := range due {
		if err := service.advancePendingReference(ctx, pending); err != nil {
			return err
		}
	}
	return nil
}

func (service *PlaceBetService) advancePendingReference(
	ctx context.Context,
	pending domain.WagerTransaction,
) error {
	now := service.now()
	expiresAt, _ := pending.ReferenceExpiresAt()
	if !now.Before(expiresAt) {
		return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeReferenceNotFound)
	}
	reference, found, err := service.store.FindBetByExternalID(ctx, pending.ProviderID(), pendingReferenceID(pending))
	if err != nil {
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	if !found || referenceWaiting(reference) {
		return service.retryPendingReference(ctx, pending, now)
	}
	if !pendingReferenceReady(pending, reference) {
		return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeInvalidReference)
	}
	if pending.Kind() != domain.WagerTransactionKindWin {
		duplicate, duplicateFound, err := service.store.FindProcessedReversal(ctx, pending.ProviderID(), pendingReferenceID(pending))
		if err != nil {
			return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
		}
		if duplicateFound && duplicate.ID() != pending.ID() {
			return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeDuplicateReversal)
		}
	}
	return service.resolvePendingReference(ctx, pending, reference, now)
}

func (service *PlaceBetService) retryPendingReference(
	ctx context.Context,
	pending domain.WagerTransaction,
	now time.Time,
) error {
	expiresAt, _ := pending.ReferenceExpiresAt()
	delay := referenceRetryBase * time.Duration(int64(1)<<pending.ReferenceAttempts())
	nextAttemptAt := now.Add(delay)
	if pending.ReferenceAttempts() >= 30 || !nextAttemptAt.Before(expiresAt) {
		return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeReferenceNotFound)
	}
	retried, err := pending.RetryPendingReference(domain.RetryPendingReferenceWagerTransactionInput{
		OccurredAt:    now,
		NextAttemptAt: nextAttemptAt,
	})
	if err != nil {
		return fmt.Errorf("advance references: retry: %w", err)
	}
	event, err := domain.NewWagerTransactionPendingReference(referenceEventMetadata(service, domain.CorrelationID(pending.IdempotencyKey()), retried), retried)
	if err != nil {
		return fmt.Errorf("advance references: retry event: %w", err)
	}
	if err := service.store.SavePendingReferenceRetry(ctx, retried, event); err != nil {
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	return nil
}

func (service *PlaceBetService) rejectPendingReference(
	ctx context.Context,
	pending domain.WagerTransaction,
	now time.Time,
	code domain.WagerTransactionFailureCode,
) error {
	rejected, err := pending.Reject(domain.RejectWagerTransactionInput{
		FailureCode: code,
		OccurredAt:  now,
	})
	if err != nil {
		return fmt.Errorf("advance references: reject: %w", err)
	}
	event, err := domain.NewWagerTransactionRejected(referenceEventMetadata(service, domain.CorrelationID(pending.IdempotencyKey()), rejected), rejected)
	if err != nil {
		return fmt.Errorf("advance references: rejected event: %w", err)
	}
	if err := service.store.RejectPendingReference(ctx, rejected, event); err != nil {
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	return nil
}

func (service *PlaceBetService) resolvePendingReference(
	ctx context.Context,
	pending domain.WagerTransaction,
	reference domain.WagerTransaction,
	now time.Time,
) error {
	wallet, err := service.store.LoadWallet(ctx, pending.WalletID())
	if err != nil {
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	balanceBefore := wallet.Balance()
	credit := pending.Kind() == domain.WagerTransactionKindWin
	if !credit {
		var creditErr error
		credit, creditErr = reversalCredits(pending.Kind(), reference.Kind())
		if creditErr != nil {
			return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeInvalidReference)
		}
	}
	var moved domain.Wallet
	if credit {
		moved, err = wallet.Credit(pending.Money(), now)
	} else {
		moved, err = wallet.Debit(pending.Money(), now)
	}
	if errors.Is(err, domain.ErrInsufficientFunds) {
		return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeReversalInsufficientFunds)
	}
	if err != nil {
		return fmt.Errorf("advance references: move wallet: %w", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{
		ResultBalance:                  moved.Balance(),
		ResolvedReferenceTransactionID: reference.ID(),
		OccurredAt:                     now,
	})
	if err != nil {
		return fmt.Errorf("advance references: process: %w", err)
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
		Amount:        pending.Money(),
		BalanceBefore: balanceBefore,
		BalanceAfter:  moved.Balance(),
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("advance references: create ledger: %w", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(referenceEventMetadata(service, domain.CorrelationID(pending.IdempotencyKey()), processed), processed)
	if err != nil {
		return fmt.Errorf("advance references: processed event: %w", err)
	}
	balanceEvent, err := domain.NewWalletBalanceChanged(referenceEventMetadata(service, domain.CorrelationID(pending.IdempotencyKey()), processed), ledger, moved)
	if err != nil {
		return fmt.Errorf("advance references: balance event: %w", err)
	}
	if err := service.store.ResolvePendingReference(ctx, moved, processed, ledger, processedEvent, balanceEvent); err != nil {
		if errors.Is(err, ErrDuplicateReversal) {
			return service.rejectPendingReference(ctx, pending, now, domain.WagerTransactionFailureCodeDuplicateReversal)
		}
		if errors.Is(err, ErrWalletVersionConflict) {
			return err
		}
		return fmt.Errorf("advance references: %w: %w", ErrPlaceBetPersistence, err)
	}
	return nil
}

func referenceEventMetadata(
	service *PlaceBetService,
	correlationID domain.CorrelationID,
	transaction domain.WagerTransaction,
) domain.DomainEventMetadataInput {
	if correlationID == "" {
		correlationID = domain.CorrelationID(transaction.IdempotencyKey())
	}
	return domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: correlationID,
		CausationID:   domain.CausationID(transaction.ID()),
	}
}

func pendingReferenceID(transaction domain.WagerTransaction) domain.ExternalTransactionID {
	referenceID, _ := transaction.ReferenceExternalTransactionID()
	return referenceID
}

func referenceWaiting(reference domain.WagerTransaction) bool {
	return reference.Status() == domain.WagerTransactionStatusPending ||
		reference.Status() == domain.WagerTransactionStatusPendingReference
}

func pendingReferenceReady(pending domain.WagerTransaction, reference domain.WagerTransaction) bool {
	if reference.Status() != domain.WagerTransactionStatusProcessed {
		return false
	}
	if reference.PlayerID() != pending.PlayerID() ||
		reference.WalletID() != pending.WalletID() ||
		reference.RoundID() != pending.RoundID() ||
		reference.Money().Currency() != pending.Money().Currency() {
		return false
	}
	switch pending.Kind() {
	case domain.WagerTransactionKindWin:
		return reference.Kind() == domain.WagerTransactionKindBet
	case domain.WagerTransactionKindRefund:
		return reference.Kind() == domain.WagerTransactionKindBet &&
			pending.Money().MinorUnits() == reference.Money().MinorUnits()
	case domain.WagerTransactionKindRollback:
		return reversalKindAllows(pending.Kind(), reference.Kind()) &&
			pending.Money().MinorUnits() == reference.Money().MinorUnits()
	default:
		return false
	}
}
