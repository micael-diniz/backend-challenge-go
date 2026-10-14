package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TransactionID string
type ProviderID string
type ExternalTransactionID string
type IdempotencyKey string
type PayloadHash string
type RoundID string
type GameID string

type WagerTransactionKind string
type WagerTransactionStatus string

const (
	WagerTransactionKindOpening  WagerTransactionKind = "OPENING"
	WagerTransactionKindBet      WagerTransactionKind = "BET"
	WagerTransactionKindWin      WagerTransactionKind = "WIN"
	WagerTransactionKindLoss     WagerTransactionKind = "LOSS"
	WagerTransactionKindRefund   WagerTransactionKind = "REFUND"
	WagerTransactionKindRollback WagerTransactionKind = "ROLLBACK"
)

const (
	WagerTransactionStatusPending          WagerTransactionStatus = "PENDING"
	WagerTransactionStatusPendingReference WagerTransactionStatus = "PENDING_REFERENCE"
	WagerTransactionStatusProcessed        WagerTransactionStatus = "PROCESSED"
	WagerTransactionStatusRejected         WagerTransactionStatus = "REJECTED"
	WagerTransactionStatusFailed           WagerTransactionStatus = "FAILED"
)

var (
	ErrInvalidTransactionID             = errors.New("invalid transaction ID")
	ErrInvalidProviderID                = errors.New("invalid provider ID")
	ErrInvalidExternalTransactionID     = errors.New("invalid external transaction ID")
	ErrInvalidIdempotencyKey            = errors.New("invalid idempotency key")
	ErrInvalidPayloadHash               = errors.New("invalid payload hash")
	ErrInvalidRoundID                   = errors.New("invalid round ID")
	ErrInvalidGameID                    = errors.New("invalid game ID")
	ErrInvalidWagerTransactionKind      = errors.New("invalid wager transaction kind")
	ErrInvalidWagerTransactionAmount    = errors.New("invalid wager transaction amount")
	ErrInvalidWagerTransactionReference = errors.New("invalid wager transaction reference")
	ErrInvalidWagerTransactionTimestamp = errors.New("invalid wager transaction timestamp")
)

type WagerTransaction struct {
	id                             TransactionID
	providerID                     ProviderID
	externalTransactionID          ExternalTransactionID
	idempotencyKey                 IdempotencyKey
	payloadHash                    PayloadHash
	walletID                       WalletID
	playerID                       PlayerID
	roundID                        RoundID
	gameID                         GameID
	kind                           WagerTransactionKind
	money                          Money
	referenceExternalTransactionID ExternalTransactionID
	status                         WagerTransactionStatus
	createdAt                      time.Time
	updatedAt                      time.Time
}

type ExternalWagerTransactionInput struct {
	ID                             TransactionID
	ProviderID                     ProviderID
	ExternalTransactionID          ExternalTransactionID
	IdempotencyKey                 IdempotencyKey
	PayloadHash                    PayloadHash
	WalletID                       WalletID
	PlayerID                       PlayerID
	RoundID                        RoundID
	GameID                         GameID
	Kind                           WagerTransactionKind
	Money                          Money
	ReferenceExternalTransactionID ExternalTransactionID
	CreatedAt                      time.Time
}

func validateExternalWagerTransactionRequiredFields(
	input ExternalWagerTransactionInput,
) error {
	if strings.TrimSpace(string(input.ID)) == "" {
		return ErrInvalidTransactionID
	}

	if strings.TrimSpace(string(input.ProviderID)) == "" {
		return ErrInvalidProviderID
	}

	if strings.TrimSpace(string(input.ExternalTransactionID)) == "" {
		return ErrInvalidExternalTransactionID
	}

	if strings.TrimSpace(string(input.IdempotencyKey)) == "" {
		return ErrInvalidIdempotencyKey
	}

	if strings.TrimSpace(string(input.PayloadHash)) == "" {
		return ErrInvalidPayloadHash
	}

	if strings.TrimSpace(string(input.WalletID)) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(string(input.PlayerID)) == "" {
		return ErrInvalidPlayerID
	}

	if strings.TrimSpace(string(input.RoundID)) == "" {
		return ErrInvalidRoundID
	}

	if strings.TrimSpace(string(input.GameID)) == "" {
		return ErrInvalidGameID
	}

	return nil
}

func (kind WagerTransactionKind) isExternal() bool {
	switch kind {
	case WagerTransactionKindBet,
		WagerTransactionKindWin,
		WagerTransactionKindLoss,
		WagerTransactionKindRefund,
		WagerTransactionKindRollback:
		return true
	default:
		return false
	}
}

func validateExternalWagerTransactionKind(kind WagerTransactionKind) error {
	if !kind.isExternal() {
		return fmt.Errorf(
			"%w: kind=%q",
			ErrInvalidWagerTransactionKind,
			kind,
		)
	}

	return nil
}

func validateWagerTransactionMoney(
	kind WagerTransactionKind,
	money Money,
) error {
	if !money.isInitialized() {
		return ErrInvalidMoney
	}

	validAmount := money.isPositive()
	if kind == WagerTransactionKindLoss {
		validAmount = !money.isPositive() && !money.isNegative()
	}

	if !validAmount {
		return fmt.Errorf(
			"%w: kind=%s, amount=%s",
			ErrInvalidWagerTransactionAmount,
			kind,
			money.formatForError(),
		)
	}

	return nil
}

func validateWagerTransactionReference(
	kind WagerTransactionKind,
	referenceID ExternalTransactionID,
) error {
	referenceText := string(referenceID)
	hasReference := referenceText != ""

	if hasReference && strings.TrimSpace(referenceText) == "" {
		return fmt.Errorf(
			"%w: kind=%s, referenceExternalTransactionID=%q",
			ErrInvalidWagerTransactionReference,
			kind,
			referenceID,
		)
	}

	switch kind {
	case WagerTransactionKindRefund, WagerTransactionKindRollback:
		if !hasReference {
			return fmt.Errorf(
				"%w: kind=%s requires a reference",
				ErrInvalidWagerTransactionReference,
				kind,
			)
		}
	case WagerTransactionKindBet, WagerTransactionKindLoss:
		if hasReference {
			return fmt.Errorf(
				"%w: kind=%s does not support a reference",
				ErrInvalidWagerTransactionReference,
				kind,
			)
		}
	}

	return nil
}

func validateWagerTransactionTimestamp(createdAt time.Time) error {
	if createdAt.IsZero() {
		return fmt.Errorf(
			"%w: createdAt cannot be zero",
			ErrInvalidWagerTransactionTimestamp,
		)
	}

	return nil
}

func buildExternalWagerTransaction(
	input ExternalWagerTransactionInput,
) (WagerTransaction, error) {
	if err := validateExternalWagerTransactionRequiredFields(input); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateExternalWagerTransactionKind(input.Kind); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateWagerTransactionMoney(input.Kind, input.Money); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateWagerTransactionReference(
		input.Kind,
		input.ReferenceExternalTransactionID,
	); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateWagerTransactionTimestamp(input.CreatedAt); err != nil {
		return WagerTransaction{}, err
	}

	return WagerTransaction{
		id:                             input.ID,
		providerID:                     input.ProviderID,
		externalTransactionID:          input.ExternalTransactionID,
		idempotencyKey:                 input.IdempotencyKey,
		payloadHash:                    input.PayloadHash,
		walletID:                       input.WalletID,
		playerID:                       input.PlayerID,
		roundID:                        input.RoundID,
		gameID:                         input.GameID,
		kind:                           input.Kind,
		money:                          input.Money,
		referenceExternalTransactionID: input.ReferenceExternalTransactionID,
		status:                         WagerTransactionStatusPending,
		createdAt:                      input.CreatedAt,
		updatedAt:                      input.CreatedAt,
	}, nil
}

func NewExternalWagerTransaction(
	input ExternalWagerTransactionInput,
) (WagerTransaction, error) {
	transaction, err := buildExternalWagerTransaction(input)
	if err != nil {
		return WagerTransaction{}, fmt.Errorf(
			"create external wager transaction: %w",
			err,
		)
	}

	return transaction, nil
}

func (transaction WagerTransaction) ReferenceExternalTransactionID() (
	ExternalTransactionID,
	bool,
) {
	referenceID := transaction.referenceExternalTransactionID
	return referenceID, referenceID != ""
}

func (wagerTransactionKind WagerTransactionKind) String() string {
	return string(wagerTransactionKind)
}

func (wagerTransactionStatus WagerTransactionStatus) String() string {
	return string(wagerTransactionStatus)
}

func (wagerTransaction WagerTransaction) ID() TransactionID {
	return wagerTransaction.id
}

func (wagerTransaction WagerTransaction) ProviderID() ProviderID {
	return wagerTransaction.providerID
}

func (wagerTransaction WagerTransaction) ExternalTransactionID() ExternalTransactionID {
	return wagerTransaction.externalTransactionID
}

func (wagerTransaction WagerTransaction) IdempotencyKey() IdempotencyKey {
	return wagerTransaction.idempotencyKey
}

func (wagerTransaction WagerTransaction) PayloadHash() PayloadHash {
	return wagerTransaction.payloadHash
}

func (wagerTransaction WagerTransaction) WalletID() WalletID {
	return wagerTransaction.walletID
}

func (wagerTransaction WagerTransaction) PlayerID() PlayerID {
	return wagerTransaction.playerID
}

func (wagerTransaction WagerTransaction) RoundID() RoundID {
	return wagerTransaction.roundID
}

func (wagerTransaction WagerTransaction) GameID() GameID {
	return wagerTransaction.gameID
}

func (wagerTransaction WagerTransaction) Kind() WagerTransactionKind {
	return wagerTransaction.kind
}

func (wagerTransaction WagerTransaction) Money() Money {
	return wagerTransaction.money
}

func (wagerTransaction WagerTransaction) Status() WagerTransactionStatus {
	return wagerTransaction.status
}

func (wagerTransaction WagerTransaction) CreatedAt() time.Time {
	return wagerTransaction.createdAt
}

func (wagerTransaction WagerTransaction) UpdatedAt() time.Time {
	return wagerTransaction.updatedAt
}
