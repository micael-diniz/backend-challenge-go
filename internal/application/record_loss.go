package application

import (
	"context"
	"fmt"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func (service *PlaceBetService) RecordLoss(
	ctx context.Context,
	command PlaceBetCommand,
) (domain.Wallet, domain.WagerTransaction, bool, error) {
	if wallet, loss, replay, err := service.existingBet(ctx, command, domain.WagerTransactionKindLoss); err != nil || replay {
		return wallet, loss, replay, err
	}

	wallet, err := service.store.LoadWallet(ctx, command.WalletID)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"record loss: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	if command.PlayerID != wallet.PlayerID() {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"record loss: %w: player ID=%q, wallet player ID=%q",
			ErrBetPlayerMismatch,
			command.PlayerID,
			wallet.PlayerID(),
		)
	}
	if command.Money.Currency() != wallet.Currency() {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"record loss: %w",
			domain.ErrCurrencyMismatch,
		)
	}

	occurredAt := service.now()
	hash, err := betPayloadHash(command, domain.WagerTransactionKindLoss)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("record loss: payload hash: %w", err)
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
		Kind:                  domain.WagerTransactionKindLoss,
		Money:                 command.Money,
		CreatedAt:             occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("record loss: create loss: %w", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{
		ResultBalance: wallet.Balance(),
		OccurredAt:    occurredAt,
	})
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("record loss: process loss: %w", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       domain.DomainEventID(service.newID()),
		CorrelationID: command.CorrelationID,
		CausationID:   command.CausationID,
	}, processed)
	if err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf("record loss: create processed event: %w", err)
	}
	if err := service.store.SaveProcessedLoss(ctx, processed, processedEvent); err != nil {
		return domain.Wallet{}, domain.WagerTransaction{}, false, fmt.Errorf(
			"record loss: %w: %w",
			ErrPlaceBetPersistence,
			err,
		)
	}
	return wallet, processed, false, nil
}
