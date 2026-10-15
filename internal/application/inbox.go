package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

const wagerTransactionRequestedType = "WagerTransactionRequested"

var (
	ErrInvalidInboxService = errors.New("invalid inbox service")
	ErrInvalidWagerMessage = errors.New("invalid wager message")
	ErrInboxConflict       = errors.New("inbox payload conflict")
)

type WagerMessage struct {
	ID      string
	Type    string
	Command PlaceBetCommand
	Kind    domain.WagerTransactionKind
}

type InboxRecord struct {
	MessageID     string
	PayloadHash   string
	TransactionID domain.TransactionID
}

type InboxStore interface {
	FindInbox(ctx context.Context, messageID string) (InboxRecord, bool, error)
	SaveInbox(ctx context.Context, record InboxRecord) error
}

type WagerApplier interface {
	Apply(ctx context.Context, command PlaceBetCommand, kind domain.WagerTransactionKind) (domain.WagerTransaction, bool, error)
}

type InboxService struct {
	wagers WagerApplier
	inbox  InboxStore
}

func NewInboxService(wagers WagerApplier, inbox InboxStore) (*InboxService, error) {
	if wagers == nil || inbox == nil {
		return nil, fmt.Errorf("%w: wagers and inbox are required", ErrInvalidInboxService)
	}
	return &InboxService{wagers: wagers, inbox: inbox}, nil
}

func (service *PlaceBetService) Apply(
	ctx context.Context,
	command PlaceBetCommand,
	kind domain.WagerTransactionKind,
) (domain.WagerTransaction, bool, error) {
	switch kind {
	case domain.WagerTransactionKindBet:
		_, transaction, replay, err := service.PlaceBet(ctx, command)
		return transaction, replay, err
	case domain.WagerTransactionKindLoss:
		_, transaction, replay, err := service.RecordLoss(ctx, command)
		return transaction, replay, err
	case domain.WagerTransactionKindWin:
		_, transaction, replay, err := service.RecordWin(ctx, command)
		return transaction, replay, err
	case domain.WagerTransactionKindRefund:
		_, transaction, replay, err := service.RecordRefund(ctx, command)
		return transaction, replay, err
	case domain.WagerTransactionKindRollback:
		_, transaction, replay, err := service.RecordRollback(ctx, command)
		return transaction, replay, err
	default:
		return domain.WagerTransaction{}, false, fmt.Errorf("%w: kind %s", ErrInvalidWagerMessage, kind)
	}
}

func (service *InboxService) Receive(ctx context.Context, message WagerMessage) (domain.WagerTransaction, error) {
	if message.ID == "" || message.Type != wagerTransactionRequestedType {
		return domain.WagerTransaction{}, fmt.Errorf("%w", ErrInvalidWagerMessage)
	}
	hash, err := betPayloadHash(message.Command, message.Kind)
	if err != nil {
		return domain.WagerTransaction{}, fmt.Errorf("receive wager message: %w", err)
	}
	existing, found, err := service.inbox.FindInbox(ctx, message.ID)
	if err != nil {
		return domain.WagerTransaction{}, fmt.Errorf("receive wager message: %w: %w", ErrPlaceBetPersistence, err)
	}
	if found {
		if existing.PayloadHash != string(hash) {
			return domain.WagerTransaction{}, fmt.Errorf("%w", ErrInboxConflict)
		}
		RecordWagerDuplicate()
		return domain.WagerTransaction{}, nil
	}

	transaction, _, applyErr := service.wagers.Apply(ctx, message.Command, message.Kind)
	if retryableWagerError(applyErr) {
		return domain.WagerTransaction{}, applyErr
	}
	record := InboxRecord{MessageID: message.ID, PayloadHash: string(hash), TransactionID: transaction.ID()}
	if err := service.inbox.SaveInbox(ctx, record); err != nil {
		return domain.WagerTransaction{}, fmt.Errorf("receive wager message: %w: %w", ErrPlaceBetPersistence, err)
	}
	if applyErr != nil {
		return domain.WagerTransaction{}, applyErr
	}
	return transaction, nil
}

func retryableWagerError(err error) bool {
	return errors.Is(err, ErrPlaceBetPersistence) || errors.Is(err, ErrWalletNotFound) || errors.Is(err, ErrWalletVersionConflict)
}

type QueuedMessage struct {
	ID      string
	Body    []byte
	Receipt string
}

type MessageQueue interface {
	Receive(ctx context.Context) (QueuedMessage, bool, error)
	Delete(ctx context.Context, receipt string) error
}

type WagerHandler interface {
	Receive(ctx context.Context, message WagerMessage) (domain.WagerTransaction, error)
}

func Consume(ctx context.Context, queue MessageQueue, handler WagerHandler) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		err := ConsumeOne(ctx, queue, handler)
		if err == nil || leaveOnQueue(err) {
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
}

func leaveOnQueue(err error) bool {
	return errors.Is(err, ErrInvalidWagerMessage) || errors.Is(err, ErrInboxConflict) || retryableWagerError(err)
}

func ConsumeOne(ctx context.Context, queue MessageQueue, handler WagerHandler) error {
	message, ok, err := queue.Receive(ctx)
	if err != nil || !ok {
		return err
	}
	decoded, err := DecodeWagerMessage(message.Body)
	if err != nil {
		RecordWagerDLQ()
		return err
	}
	if decoded.ID == "" {
		decoded.ID = message.ID
	}
	started := time.Now()
	transaction, handleErr := handler.Receive(ctx, decoded)
	RecordWagerProcessing(time.Since(started))
	RecordWagerResult(transaction.Status())
	LogWager(decoded.Command, decoded.ID, transaction)
	if errors.Is(handleErr, ErrInvalidWagerMessage) || errors.Is(handleErr, ErrInboxConflict) {
		RecordWagerDLQ()
	}
	if handleErr != nil && !terminalWagerError(handleErr) {
		return handleErr
	}
	if err := queue.Delete(ctx, message.Receipt); err != nil {
		return fmt.Errorf("delete wager message: %w", err)
	}
	return nil
}

func terminalWagerError(err error) bool {
	return err != nil && !retryableWagerError(err) && !errors.Is(err, ErrInvalidWagerMessage) && !errors.Is(err, ErrInboxConflict)
}

type wagerMessageEnvelope struct {
	MessageID string           `json:"messageId"`
	Type      string           `json:"type"`
	Data      WagerMessageData `json:"data"`
}

func DecodeWagerMessage(body []byte) (WagerMessage, error) {
	var payload wagerMessageEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return WagerMessage{}, fmt.Errorf("%w: %w", ErrInvalidWagerMessage, err)
	}
	if payload.Data.Money.Currency() == "" {
		return WagerMessage{}, ErrInvalidWagerMessage
	}
	return WagerMessage{
		ID:      payload.MessageID,
		Type:    payload.Type,
		Kind:    domain.WagerTransactionKind(payload.Data.Kind),
		Command: payload.Data.Command(domain.CorrelationID(payload.MessageID)),
	}, nil
}
