package domain

import (
	"errors"
	"fmt"
	"math"
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
type WagerTransactionFailureCode string

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

const (
	WagerTransactionFailureCodeInsufficientFunds              WagerTransactionFailureCode = "INSUFFICIENT_FUNDS"
	WagerTransactionFailureCodeReversalInsufficientFunds      WagerTransactionFailureCode = "REVERSAL_INSUFFICIENT_FUNDS"
	WagerTransactionFailureCodeReferenceNotFound              WagerTransactionFailureCode = "REFERENCE_NOT_FOUND"
	WagerTransactionFailureCodeInvalidReference               WagerTransactionFailureCode = "INVALID_REFERENCE"
	WagerTransactionFailureCodeDuplicateReversal              WagerTransactionFailureCode = "DUPLICATE_REVERSAL"
	WagerTransactionFailureCodePermanentInfrastructureFailure WagerTransactionFailureCode = "PERMANENT_INFRASTRUCTURE_FAILURE"
)

var (
	ErrInvalidTransactionID                    = errors.New("invalid transaction ID")
	ErrInvalidProviderID                       = errors.New("invalid provider ID")
	ErrInvalidExternalTransactionID            = errors.New("invalid external transaction ID")
	ErrInvalidIdempotencyKey                   = errors.New("invalid idempotency key")
	ErrInvalidPayloadHash                      = errors.New("invalid payload hash")
	ErrInvalidRoundID                          = errors.New("invalid round ID")
	ErrInvalidGameID                           = errors.New("invalid game ID")
	ErrInvalidWagerTransactionKind             = errors.New("invalid wager transaction kind")
	ErrInvalidWagerTransactionAmount           = errors.New("invalid wager transaction amount")
	ErrInvalidWagerTransactionReference        = errors.New("invalid wager transaction reference")
	ErrInvalidWagerTransactionTimestamp        = errors.New("invalid wager transaction timestamp")
	ErrInvalidWagerTransaction                 = errors.New("invalid wager transaction")
	ErrInvalidWagerTransactionOrigin           = errors.New("invalid wager transaction origin")
	ErrInvalidWagerTransactionStatus           = errors.New("invalid wager transaction status")
	ErrInvalidWagerTransactionStatusTransition = errors.New(
		"invalid wager transaction status transition",
	)
	ErrInvalidWagerTransactionResultBalance = errors.New(
		"invalid wager transaction result balance",
	)
	ErrInvalidWagerTransactionReferenceSchedule = errors.New(
		"invalid wager transaction reference schedule",
	)
	ErrWagerTransactionReferenceRetryNotDue = errors.New(
		"wager transaction reference retry is not due",
	)
	ErrWagerTransactionReferenceExpired = errors.New(
		"wager transaction reference expired",
	)
	ErrInvalidWagerTransactionFailureCode = errors.New(
		"invalid wager transaction failure code",
	)
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
	resolvedReferenceTransactionID TransactionID
	status                         WagerTransactionStatus
	failureCode                    WagerTransactionFailureCode
	resultBalance                  Money
	referenceAttempts              int32
	nextReferenceAttemptAt         time.Time
	referenceExpiresAt             time.Time
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

type OpeningWagerTransactionInput struct {
	ID        TransactionID
	WalletID  WalletID
	PlayerID  PlayerID
	Money     Money
	CreatedAt time.Time
}

type RehydrateWagerTransactionInput struct {
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
	ResolvedReferenceTransactionID TransactionID
	Status                         WagerTransactionStatus
	FailureCode                    WagerTransactionFailureCode
	ResultBalance                  Money
	ReferenceAttempts              int32
	NextReferenceAttemptAt         time.Time
	ReferenceExpiresAt             time.Time
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type ProcessWagerTransactionInput struct {
	ResultBalance                  Money
	ResolvedReferenceTransactionID TransactionID
	OccurredAt                     time.Time
}

type PendingReferenceWagerTransactionInput struct {
	OccurredAt    time.Time
	NextAttemptAt time.Time
	ExpiresAt     time.Time
}

type RetryPendingReferenceWagerTransactionInput struct {
	OccurredAt    time.Time
	NextAttemptAt time.Time
}

type RejectWagerTransactionInput struct {
	FailureCode WagerTransactionFailureCode
	OccurredAt  time.Time
}

type FailWagerTransactionInput struct {
	FailureCode WagerTransactionFailureCode
	OccurredAt  time.Time
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

func (kind WagerTransactionKind) isSupported() bool {
	switch kind {
	case WagerTransactionKindOpening,
		WagerTransactionKindBet,
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
		validAmount = money.isZero()
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

func validateOpeningWagerTransactionRequiredFields(
	input OpeningWagerTransactionInput,
) error {
	if strings.TrimSpace(string(input.ID)) == "" {
		return ErrInvalidTransactionID
	}

	if strings.TrimSpace(string(input.WalletID)) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(string(input.PlayerID)) == "" {
		return ErrInvalidPlayerID
	}

	return nil
}

func buildOpeningWagerTransaction(
	input OpeningWagerTransactionInput,
) (WagerTransaction, error) {
	if err := validateOpeningWagerTransactionRequiredFields(input); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateWagerTransactionMoney(WagerTransactionKindOpening, input.Money); err != nil {
		return WagerTransaction{}, err
	}

	if err := validateWagerTransactionTimestamp(input.CreatedAt); err != nil {
		return WagerTransaction{}, err
	}

	return WagerTransaction{
		id:        input.ID,
		walletID:  input.WalletID,
		playerID:  input.PlayerID,
		kind:      WagerTransactionKindOpening,
		money:     input.Money,
		status:    WagerTransactionStatusProcessed,
		createdAt: input.CreatedAt,
		updatedAt: input.CreatedAt,
	}, nil
}

func NewOpeningWagerTransaction(
	input OpeningWagerTransactionInput,
) (WagerTransaction, error) {
	transaction, err := buildOpeningWagerTransaction(input)
	if err != nil {
		return WagerTransaction{}, fmt.Errorf(
			"create opening wager transaction: %w",
			err,
		)
	}

	return transaction, nil
}

func RehydrateWagerTransaction(
	input RehydrateWagerTransactionInput,
) (WagerTransaction, error) {
	transaction := WagerTransaction{
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
		resolvedReferenceTransactionID: input.ResolvedReferenceTransactionID,
		status:                         input.Status,
		failureCode:                    input.FailureCode,
		resultBalance:                  input.ResultBalance,
		referenceAttempts:              input.ReferenceAttempts,
		nextReferenceAttemptAt:         input.NextReferenceAttemptAt,
		referenceExpiresAt:             input.ReferenceExpiresAt,
		createdAt:                      input.CreatedAt,
		updatedAt:                      input.UpdatedAt,
	}

	if err := transaction.validateRehydratedState(); err != nil {
		return WagerTransaction{}, fmt.Errorf(
			"rehydrate wager transaction: %w: %w",
			ErrInvalidWagerTransaction,
			err,
		)
	}

	return transaction, nil
}

func (transaction WagerTransaction) validateRehydratedState() error {
	if transaction.kind == WagerTransactionKindOpening {
		if err := transaction.validateRehydratedOpening(); err != nil {
			return err
		}
		if err := transaction.validateRehydratedOpeningState(); err != nil {
			return err
		}
	} else {
		if err := transaction.validateRehydratedExternalOrigin(); err != nil {
			return err
		}
		if err := transaction.validateRehydratedExternalState(); err != nil {
			return err
		}
	}

	return transaction.validateForTransition()
}

func (transaction WagerTransaction) validateRehydratedOpening() error {
	input := OpeningWagerTransactionInput{
		ID:        transaction.id,
		WalletID:  transaction.walletID,
		PlayerID:  transaction.playerID,
		Money:     transaction.money,
		CreatedAt: transaction.createdAt,
	}
	if err := validateOpeningWagerTransactionRequiredFields(input); err != nil {
		return err
	}
	if err := validateWagerTransactionMoney(WagerTransactionKindOpening, input.Money); err != nil {
		return err
	}
	if err := validateWagerTransactionTimestamp(input.CreatedAt); err != nil {
		return err
	}

	if transaction.providerID != "" {
		return fmt.Errorf("%w: opening has provider ID", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.externalTransactionID != "" {
		return fmt.Errorf("%w: opening has external transaction ID", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.idempotencyKey != "" {
		return fmt.Errorf("%w: opening has idempotency key", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.payloadHash != "" {
		return fmt.Errorf("%w: opening has payload hash", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.roundID != "" {
		return fmt.Errorf("%w: opening has round ID", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.gameID != "" {
		return fmt.Errorf("%w: opening has game ID", ErrInvalidWagerTransactionOrigin)
	}
	if transaction.referenceExternalTransactionID != "" {
		return fmt.Errorf("%w: opening has external reference", ErrInvalidWagerTransactionOrigin)
	}

	return nil
}

func (transaction WagerTransaction) validateRehydratedExternalOrigin() error {
	input := ExternalWagerTransactionInput{
		ID:                             transaction.id,
		ProviderID:                     transaction.providerID,
		ExternalTransactionID:          transaction.externalTransactionID,
		IdempotencyKey:                 transaction.idempotencyKey,
		PayloadHash:                    transaction.payloadHash,
		WalletID:                       transaction.walletID,
		PlayerID:                       transaction.playerID,
		RoundID:                        transaction.roundID,
		GameID:                         transaction.gameID,
		Kind:                           transaction.kind,
		Money:                          transaction.money,
		ReferenceExternalTransactionID: transaction.referenceExternalTransactionID,
		CreatedAt:                      transaction.createdAt,
	}
	if err := validateExternalWagerTransactionRequiredFields(input); err != nil {
		return err
	}
	if err := validateExternalWagerTransactionKind(input.Kind); err != nil {
		return err
	}
	if err := validateWagerTransactionMoney(input.Kind, input.Money); err != nil {
		return err
	}
	if err := validateWagerTransactionReference(
		input.Kind,
		input.ReferenceExternalTransactionID,
	); err != nil {
		return err
	}
	if err := validateWagerTransactionTimestamp(input.CreatedAt); err != nil {
		return err
	}

	return nil
}

func (transaction WagerTransaction) validateRehydratedOpeningState() error {
	if transaction.status != WagerTransactionStatusProcessed {
		return fmt.Errorf(
			"%w: opening status=%s must be %s",
			ErrInvalidWagerTransactionStatus,
			transaction.status,
			WagerTransactionStatusProcessed,
		)
	}
	if !transaction.updatedAt.Equal(transaction.createdAt) {
		return fmt.Errorf(
			"%w: opening updatedAt=%s must equal createdAt=%s",
			ErrInvalidWagerTransactionTimestamp,
			transaction.updatedAt,
			transaction.createdAt,
		)
	}
	if transaction.resolvedReferenceTransactionID != "" {
		return fmt.Errorf(
			"%w: opening cannot have a resolved reference",
			ErrInvalidWagerTransactionReference,
		)
	}
	if transaction.resultBalance.isInitialized() {
		return fmt.Errorf(
			"%w: opening cannot have a result balance",
			ErrInvalidWagerTransactionResultBalance,
		)
	}
	if transaction.failureCode != "" {
		return fmt.Errorf(
			"%w: opening cannot have failure code=%q",
			ErrInvalidWagerTransactionFailureCode,
			transaction.failureCode,
		)
	}
	if transaction.hasAnyReferenceSchedule() {
		return fmt.Errorf(
			"%w: opening cannot have reference retry metadata",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	return nil
}

func (transaction WagerTransaction) validateRehydratedExternalState() error {
	switch transaction.status {
	case WagerTransactionStatusPending:
		return transaction.validateRehydratedPendingState()
	case WagerTransactionStatusPendingReference:
		return transaction.validateRehydratedPendingReferenceState()
	case WagerTransactionStatusProcessed:
		return transaction.validateRehydratedProcessedState()
	case WagerTransactionStatusRejected:
		return transaction.validateRehydratedRejectedState()
	case WagerTransactionStatusFailed:
		return transaction.validateRehydratedFailedState()
	default:
		return fmt.Errorf(
			"%w: status=%q",
			ErrInvalidWagerTransactionStatus,
			transaction.status,
		)
	}
}

func (transaction WagerTransaction) validateRehydratedPendingState() error {
	if !transaction.updatedAt.Equal(transaction.createdAt) {
		return fmt.Errorf(
			"%w: pending updatedAt=%s must equal createdAt=%s",
			ErrInvalidWagerTransactionTimestamp,
			transaction.updatedAt,
			transaction.createdAt,
		)
	}
	if err := transaction.validateAbsentOutcome(); err != nil {
		return err
	}
	if transaction.hasAnyReferenceSchedule() {
		return fmt.Errorf(
			"%w: pending transaction cannot have reference retry metadata",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	return nil
}

func (transaction WagerTransaction) validateRehydratedPendingReferenceState() error {
	if err := transaction.validateAbsentOutcome(); err != nil {
		return err
	}
	if err := transaction.validatePendingReferenceEligibility(); err != nil {
		return err
	}

	return transaction.validatePendingReferenceState()
}

func (transaction WagerTransaction) validateRehydratedProcessedState() error {
	if !transaction.resultBalance.isInitialized() {
		return fmt.Errorf(
			"%w: processed transaction requires a result balance",
			ErrInvalidWagerTransactionResultBalance,
		)
	}
	if err := transaction.validateProcessResultBalance(transaction.resultBalance); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWagerTransactionResultBalance, err)
	}
	if transaction.failureCode != "" {
		return fmt.Errorf(
			"%w: processed transaction has code=%q",
			ErrInvalidWagerTransactionFailureCode,
			transaction.failureCode,
		)
	}
	if !transaction.nextReferenceAttemptAt.IsZero() {
		return fmt.Errorf(
			"%w: processed transaction cannot have a next attempt",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}
	if err := transaction.validateProcessReference(transaction.resolvedReferenceTransactionID); err != nil {
		return err
	}
	if err := transaction.validateRehydratedReferenceHistory(); err != nil {
		return err
	}
	if transaction.referenceAttempts > 0 {
		return validateWagerTransactionReferenceNotExpired(
			transaction.updatedAt,
			transaction.referenceExpiresAt,
		)
	}

	return nil
}

func (transaction WagerTransaction) validateRehydratedRejectedState() error {
	if !transaction.failureCode.isSupportedRejection() {
		return fmt.Errorf(
			"%w: rejected transaction has code=%q",
			ErrInvalidWagerTransactionFailureCode,
			transaction.failureCode,
		)
	}

	return transaction.validateRehydratedUnsuccessfulTerminalState()
}

func (transaction WagerTransaction) validateRehydratedFailedState() error {
	if !transaction.failureCode.isSupportedPermanentFailure() {
		return fmt.Errorf(
			"%w: failed transaction has code=%q",
			ErrInvalidWagerTransactionFailureCode,
			transaction.failureCode,
		)
	}

	return transaction.validateRehydratedUnsuccessfulTerminalState()
}

func (transaction WagerTransaction) validateRehydratedUnsuccessfulTerminalState() error {
	if transaction.resultBalance.isInitialized() {
		return fmt.Errorf(
			"%w: unsuccessful transaction cannot have a result balance",
			ErrInvalidWagerTransactionResultBalance,
		)
	}
	if transaction.resolvedReferenceTransactionID != "" {
		return fmt.Errorf(
			"%w: unsuccessful transaction cannot have a resolved reference",
			ErrInvalidWagerTransactionReference,
		)
	}
	if !transaction.nextReferenceAttemptAt.IsZero() {
		return fmt.Errorf(
			"%w: terminal transaction cannot have a next attempt",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	return transaction.validateRehydratedReferenceHistory()
}

func (transaction WagerTransaction) validateAbsentOutcome() error {
	if transaction.resolvedReferenceTransactionID != "" {
		return fmt.Errorf(
			"%w: non-processed transaction cannot have a resolved reference",
			ErrInvalidWagerTransactionReference,
		)
	}
	if transaction.resultBalance.isInitialized() {
		return fmt.Errorf(
			"%w: non-processed transaction cannot have a result balance",
			ErrInvalidWagerTransactionResultBalance,
		)
	}
	if transaction.failureCode != "" {
		return fmt.Errorf(
			"%w: non-terminal transaction cannot have code=%q",
			ErrInvalidWagerTransactionFailureCode,
			transaction.failureCode,
		)
	}

	return nil
}

func (transaction WagerTransaction) hasAnyReferenceSchedule() bool {
	return transaction.referenceAttempts != 0 ||
		!transaction.nextReferenceAttemptAt.IsZero() ||
		!transaction.referenceExpiresAt.IsZero()
}

func (transaction WagerTransaction) validateRehydratedReferenceHistory() error {
	hasAttempts := transaction.referenceAttempts != 0
	hasExpiration := !transaction.referenceExpiresAt.IsZero()
	if !hasAttempts && !hasExpiration {
		return nil
	}
	if transaction.referenceAttempts <= 0 {
		return fmt.Errorf(
			"%w: referenceAttempts=%d must be positive when history is present",
			ErrInvalidWagerTransactionReferenceSchedule,
			transaction.referenceAttempts,
		)
	}
	if !hasExpiration {
		return fmt.Errorf(
			"%w: reference history requires expiresAt",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}
	if !transaction.referenceExpiresAt.After(transaction.createdAt) {
		return fmt.Errorf(
			"%w: expiresAt=%s must be after createdAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			transaction.referenceExpiresAt,
			transaction.createdAt,
		)
	}
	if err := transaction.validateReferenceHistoryEligibility(); err != nil {
		return err
	}

	return nil
}

func (transaction WagerTransaction) validateReferenceHistoryEligibility() error {
	switch transaction.kind {
	case WagerTransactionKindWin,
		WagerTransactionKindRefund,
		WagerTransactionKindRollback:
	default:
		return fmt.Errorf(
			"%w: kind=%s cannot have reference history",
			ErrInvalidWagerTransactionReference,
			transaction.kind,
		)
	}

	if transaction.referenceExternalTransactionID == "" {
		return fmt.Errorf(
			"%w: reference history requires an external reference",
			ErrInvalidWagerTransactionReference,
		)
	}

	return nil
}

func (status WagerTransactionStatus) isSupported() bool {
	switch status {
	case WagerTransactionStatusPending,
		WagerTransactionStatusPendingReference,
		WagerTransactionStatusProcessed,
		WagerTransactionStatusRejected,
		WagerTransactionStatusFailed:
		return true
	default:
		return false
	}
}

func (status WagerTransactionStatus) canTransitionTo(
	target WagerTransactionStatus,
) bool {
	switch status {
	case WagerTransactionStatusPending:
		switch target {
		case WagerTransactionStatusPendingReference,
			WagerTransactionStatusProcessed,
			WagerTransactionStatusRejected,
			WagerTransactionStatusFailed:
			return true
		}
	case WagerTransactionStatusPendingReference:
		switch target {
		case WagerTransactionStatusProcessed,
			WagerTransactionStatusRejected,
			WagerTransactionStatusFailed:
			return true
		}
	}

	return false
}

func (transaction WagerTransaction) validateForTransition() error {
	if strings.TrimSpace(string(transaction.id)) == "" {
		return fmt.Errorf(
			"%w: %w: transactionID=%q",
			ErrInvalidWagerTransaction,
			ErrInvalidTransactionID,
			transaction.id,
		)
	}

	if strings.TrimSpace(string(transaction.walletID)) == "" {
		return fmt.Errorf(
			"%w: %w: walletID=%q",
			ErrInvalidWagerTransaction,
			ErrInvalidWalletID,
			transaction.walletID,
		)
	}

	if strings.TrimSpace(string(transaction.playerID)) == "" {
		return fmt.Errorf(
			"%w: %w: playerID=%q",
			ErrInvalidWagerTransaction,
			ErrInvalidPlayerID,
			transaction.playerID,
		)
	}

	if !transaction.money.isInitialized() {
		return fmt.Errorf(
			"%w: %w: money is uninitialized",
			ErrInvalidWagerTransaction,
			ErrInvalidMoney,
		)
	}

	if !transaction.kind.isSupported() {
		return fmt.Errorf(
			"%w: %w: kind=%q",
			ErrInvalidWagerTransaction,
			ErrInvalidWagerTransactionKind,
			transaction.kind,
		)
	}

	if !transaction.status.isSupported() {
		return fmt.Errorf(
			"%w: %w: status=%q",
			ErrInvalidWagerTransaction,
			ErrInvalidWagerTransactionStatus,
			transaction.status,
		)
	}

	if transaction.createdAt.IsZero() {
		return fmt.Errorf(
			"%w: %w: createdAt cannot be zero",
			ErrInvalidWagerTransaction,
			ErrInvalidWagerTransactionTimestamp,
		)
	}

	if transaction.updatedAt.IsZero() {
		return fmt.Errorf(
			"%w: %w: updatedAt cannot be zero",
			ErrInvalidWagerTransaction,
			ErrInvalidWagerTransactionTimestamp,
		)
	}

	if transaction.updatedAt.Before(transaction.createdAt) {
		return fmt.Errorf(
			"%w: %w: updatedAt=%s precedes createdAt=%s",
			ErrInvalidWagerTransaction,
			ErrInvalidWagerTransactionTimestamp,
			transaction.updatedAt,
			transaction.createdAt,
		)
	}

	return nil
}

func (transaction WagerTransaction) validateProcessResultBalance(resultBalance Money) error {
	if err := transaction.money.validateBinaryOperation("process wager transaction", resultBalance); err != nil {
		return err
	}

	if resultBalance.isNegative() {
		return fmt.Errorf(
			"%w: balance=%s",
			ErrInvalidWagerTransactionResultBalance,
			resultBalance.formatForError(),
		)
	}

	return nil
}

func (transaction WagerTransaction) validateProcessReference(
	resolvedReferenceID TransactionID,
) error {
	resolvedText := string(resolvedReferenceID)
	hasResolvedReference := resolvedText != ""
	hasExternalReference := transaction.referenceExternalTransactionID != ""

	if hasResolvedReference && strings.TrimSpace(resolvedText) == "" {
		return fmt.Errorf(
			"%w: resolvedReferenceTransactionID=%q",
			ErrInvalidWagerTransactionReference,
			resolvedReferenceID,
		)
	}

	if hasResolvedReference && resolvedReferenceID == transaction.id {
		return fmt.Errorf(
			"%w: transaction cannot reference itself: transactionID=%q",
			ErrInvalidWagerTransactionReference,
			transaction.id,
		)
	}

	switch transaction.kind {
	case WagerTransactionKindRefund, WagerTransactionKindRollback:
		if !hasResolvedReference {
			return fmt.Errorf(
				"%w: kind=%s requires a resolved reference",
				ErrInvalidWagerTransactionReference,
				transaction.kind,
			)
		}
	case WagerTransactionKindWin:
		if hasExternalReference != hasResolvedReference {
			return fmt.Errorf(
				"%w: kind=%s externalReference=%t, resolvedReference=%t",
				ErrInvalidWagerTransactionReference,
				transaction.kind,
				hasExternalReference,
				hasResolvedReference,
			)
		}
	case WagerTransactionKindBet, WagerTransactionKindLoss:
		if hasResolvedReference {
			return fmt.Errorf(
				"%w: kind=%s does not support a resolved reference",
				ErrInvalidWagerTransactionReference,
				transaction.kind,
			)
		}
	}

	return nil
}

func (transaction WagerTransaction) validateTransitionTimestamp(occurredAt time.Time) error {
	if err := validateWagerTransactionTimestamp(occurredAt); err != nil {
		return err
	}

	if occurredAt.Before(transaction.updatedAt) {
		return fmt.Errorf(
			"%w: occurredAt=%s precedes updatedAt=%s",
			ErrInvalidWagerTransactionTimestamp,
			occurredAt,
			transaction.updatedAt,
		)
	}

	return nil
}

func validateWagerTransactionReferenceNotExpired(
	occurredAt time.Time,
	expiresAt time.Time,
) error {
	if !occurredAt.Before(expiresAt) {
		return fmt.Errorf(
			"%w: %w: occurredAt=%s reached expiresAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			ErrWagerTransactionReferenceExpired,
			occurredAt,
			expiresAt,
		)
	}

	return nil
}

func (transaction WagerTransaction) Process(
	input ProcessWagerTransactionInput,
) (WagerTransaction, error) {
	if err := transaction.validateForTransition(); err != nil {
		return WagerTransaction{}, fmt.Errorf(
			"process wager transaction: %w", err,
		)
	}

	if !transaction.status.canTransitionTo(WagerTransactionStatusProcessed) {
		return WagerTransaction{}, fmt.Errorf(
			"process wager transaction: %w: from=%s, to=%s",
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusProcessed,
		)
	}

	if transaction.status == WagerTransactionStatusPendingReference {
		if err := transaction.validatePendingReferenceState(); err != nil {
			return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
		}

		if err := transaction.validatePendingReferenceEligibility(); err != nil {
			return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
		}
	}

	if err := transaction.validateProcessResultBalance(input.ResultBalance); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	if transaction.status == WagerTransactionStatusPendingReference {
		if err := validateWagerTransactionReferenceNotExpired(
			input.OccurredAt,
			transaction.referenceExpiresAt,
		); err != nil {
			return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
		}
	}

	if err := transaction.validateProcessReference(input.ResolvedReferenceTransactionID); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	processed := transaction
	processed.resolvedReferenceTransactionID = input.ResolvedReferenceTransactionID
	processed.status = WagerTransactionStatusProcessed
	processed.resultBalance = input.ResultBalance
	processed.nextReferenceAttemptAt = time.Time{}
	processed.updatedAt = input.OccurredAt

	return processed, nil
}

func (transaction WagerTransaction) validatePendingReferenceEligibility() error {
	switch transaction.kind {
	case WagerTransactionKindWin,
		WagerTransactionKindRefund,
		WagerTransactionKindRollback:
	default:
		return fmt.Errorf(
			"%w: kind=%s does not support a pending reference",
			ErrInvalidWagerTransactionReference,
			transaction.kind,
		)
	}

	if transaction.referenceExternalTransactionID == "" {
		return fmt.Errorf(
			"%w: kind=%s requires an external reference",
			ErrInvalidWagerTransactionReference,
			transaction.kind,
		)
	}

	if transaction.resolvedReferenceTransactionID != "" {
		return fmt.Errorf(
			"%w: reference is already resolved: transactionID=%q",
			ErrInvalidWagerTransactionReference,
			transaction.resolvedReferenceTransactionID,
		)
	}

	return nil
}

func validatePendingReferenceSchedule(
	input PendingReferenceWagerTransactionInput,
) error {
	if input.NextAttemptAt.IsZero() {
		return fmt.Errorf(
			"%w: nextAttemptAt cannot be zero",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	if !input.NextAttemptAt.After(input.OccurredAt) {
		return fmt.Errorf(
			"%w: nextAttemptAt=%s must be after occurredAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			input.NextAttemptAt,
			input.OccurredAt,
		)
	}

	if input.ExpiresAt.IsZero() {
		return fmt.Errorf(
			"%w: expiresAt cannot be zero",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	if !input.ExpiresAt.After(input.NextAttemptAt) {
		return fmt.Errorf(
			"%w: expiresAt=%s must be after nextAttemptAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			input.ExpiresAt,
			input.NextAttemptAt,
		)
	}

	return nil
}

func (transaction WagerTransaction) MarkPendingReference(
	input PendingReferenceWagerTransactionInput,
) (WagerTransaction, error) {
	const operation = "mark wager transaction pending reference"

	if err := transaction.validateForTransition(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if !transaction.status.canTransitionTo(WagerTransactionStatusPendingReference) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: from=%s, to=%s",
			operation,
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusPendingReference,
		)
	}

	if err := transaction.validatePendingReferenceEligibility(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if err := validatePendingReferenceSchedule(input); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	pending := transaction
	pending.status = WagerTransactionStatusPendingReference
	pending.referenceAttempts = 1
	pending.nextReferenceAttemptAt = input.NextAttemptAt
	pending.referenceExpiresAt = input.ExpiresAt
	pending.updatedAt = input.OccurredAt

	return pending, nil
}

func (transaction WagerTransaction) validatePendingReferenceState() error {
	if transaction.referenceAttempts <= 0 {
		return fmt.Errorf(
			"%w: referenceAttempts=%d must be positive",
			ErrInvalidWagerTransactionReferenceSchedule,
			transaction.referenceAttempts,
		)
	}

	if transaction.nextReferenceAttemptAt.IsZero() {
		return fmt.Errorf(
			"%w: nextAttemptAt cannot be zero",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	if !transaction.nextReferenceAttemptAt.After(transaction.updatedAt) {
		return fmt.Errorf(
			"%w: nextAttemptAt=%s must be after updatedAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			transaction.nextReferenceAttemptAt,
			transaction.updatedAt,
		)
	}

	if transaction.referenceExpiresAt.IsZero() {
		return fmt.Errorf(
			"%w: expiresAt cannot be zero",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	if !transaction.referenceExpiresAt.After(transaction.nextReferenceAttemptAt) {
		return fmt.Errorf(
			"%w: expiresAt=%s must be after nextAttemptAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			transaction.referenceExpiresAt,
			transaction.nextReferenceAttemptAt,
		)
	}

	return nil
}

func validateNextPendingReferenceAttempt(
	input RetryPendingReferenceWagerTransactionInput,
	expiresAt time.Time,
) error {
	if input.NextAttemptAt.IsZero() {
		return fmt.Errorf(
			"%w: nextAttemptAt cannot be zero",
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	if !input.NextAttemptAt.After(input.OccurredAt) {
		return fmt.Errorf(
			"%w: nextAttemptAt=%s must be after occurredAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			input.NextAttemptAt,
			input.OccurredAt,
		)
	}

	if !input.NextAttemptAt.Before(expiresAt) {
		return fmt.Errorf(
			"%w: nextAttemptAt=%s must be before expiresAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			input.NextAttemptAt,
			expiresAt,
		)
	}

	return nil
}

func (transaction WagerTransaction) RetryPendingReference(
	input RetryPendingReferenceWagerTransactionInput,
) (WagerTransaction, error) {
	const operation = "retry pending wager transaction reference"

	if err := transaction.validateForTransition(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if transaction.status != WagerTransactionStatusPendingReference {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: from=%s, to=%s",
			operation,
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusPendingReference,
		)
	}

	if err := transaction.validatePendingReferenceState(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if input.OccurredAt.Before(transaction.nextReferenceAttemptAt) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: %w: occurredAt=%s precedes nextAttemptAt=%s",
			operation,
			ErrInvalidWagerTransactionReferenceSchedule,
			ErrWagerTransactionReferenceRetryNotDue,
			input.OccurredAt,
			transaction.nextReferenceAttemptAt,
		)
	}

	if err := validateWagerTransactionReferenceNotExpired(
		input.OccurredAt,
		transaction.referenceExpiresAt,
	); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if err := validateNextPendingReferenceAttempt(input, transaction.referenceExpiresAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if transaction.referenceAttempts == math.MaxInt32 {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: reference attempts overflow",
			operation,
			ErrInvalidWagerTransactionReferenceSchedule,
		)
	}

	retried := transaction
	retried.referenceAttempts++
	retried.nextReferenceAttemptAt = input.NextAttemptAt
	retried.updatedAt = input.OccurredAt

	return retried, nil
}

func (failureCode WagerTransactionFailureCode) isSupportedRejection() bool {
	switch failureCode {
	case WagerTransactionFailureCodeInsufficientFunds,
		WagerTransactionFailureCodeReversalInsufficientFunds,
		WagerTransactionFailureCodeReferenceNotFound,
		WagerTransactionFailureCodeInvalidReference,
		WagerTransactionFailureCodeDuplicateReversal:
		return true
	default:
		return false
	}
}

func (transaction WagerTransaction) Reject(
	input RejectWagerTransactionInput,
) (WagerTransaction, error) {
	const operation = "reject wager transaction"

	if err := transaction.validateForTransition(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if !transaction.status.canTransitionTo(WagerTransactionStatusRejected) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: from=%s, to=%s",
			operation,
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusRejected,
		)
	}

	if !input.FailureCode.isSupportedRejection() {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: code=%q",
			operation,
			ErrInvalidWagerTransactionFailureCode,
			input.FailureCode,
		)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	rejected := transaction
	rejected.status = WagerTransactionStatusRejected
	rejected.failureCode = input.FailureCode
	rejected.nextReferenceAttemptAt = time.Time{}
	rejected.updatedAt = input.OccurredAt

	return rejected, nil
}

func (failureCode WagerTransactionFailureCode) isSupportedPermanentFailure() bool {
	return failureCode == WagerTransactionFailureCodePermanentInfrastructureFailure
}

func (transaction WagerTransaction) Fail(
	input FailWagerTransactionInput,
) (WagerTransaction, error) {
	const operation = "fail wager transaction"

	if err := transaction.validateForTransition(); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if !transaction.status.canTransitionTo(WagerTransactionStatusFailed) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: from=%s, to=%s",
			operation,
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusFailed,
		)
	}

	if !input.FailureCode.isSupportedPermanentFailure() {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: code=%q",
			operation,
			ErrInvalidWagerTransactionFailureCode,
			input.FailureCode,
		)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	failed := transaction
	failed.status = WagerTransactionStatusFailed
	failed.failureCode = input.FailureCode
	failed.nextReferenceAttemptAt = time.Time{}
	failed.updatedAt = input.OccurredAt

	return failed, nil
}

func (transaction WagerTransaction) ReferenceExternalTransactionID() (
	ExternalTransactionID,
	bool,
) {
	referenceID := transaction.referenceExternalTransactionID
	return referenceID, referenceID != ""
}

func (transaction WagerTransaction) ResolvedReferenceTransactionID() (
	TransactionID,
	bool,
) {
	referenceID := transaction.resolvedReferenceTransactionID
	return referenceID, referenceID != ""
}

func (transaction WagerTransaction) FailureCode() (
	WagerTransactionFailureCode,
	bool,
) {
	failureCode := transaction.failureCode
	return failureCode, failureCode != ""
}

func (transaction WagerTransaction) ResultBalance() (Money, bool) {
	resultBalance := transaction.resultBalance
	return resultBalance, resultBalance.isInitialized()
}

func (transaction WagerTransaction) ReferenceAttempts() int32 {
	return transaction.referenceAttempts
}

func (transaction WagerTransaction) NextReferenceAttemptAt() (time.Time, bool) {
	nextAttemptAt := transaction.nextReferenceAttemptAt
	return nextAttemptAt, !nextAttemptAt.IsZero()
}

func (transaction WagerTransaction) ReferenceExpiresAt() (time.Time, bool) {
	expiresAt := transaction.referenceExpiresAt
	return expiresAt, !expiresAt.IsZero()
}

func (wagerTransactionKind WagerTransactionKind) String() string {
	return string(wagerTransactionKind)
}

func (wagerTransactionStatus WagerTransactionStatus) String() string {
	return string(wagerTransactionStatus)
}

func (failureCode WagerTransactionFailureCode) String() string {
	return string(failureCode)
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
