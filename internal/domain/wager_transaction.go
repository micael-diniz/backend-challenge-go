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
	ErrInvalidWagerTransactionStatusTransition = errors.New(
		"invalid wager transaction status transition",
	)
	ErrInvalidWagerTransactionResultBalance = errors.New(
		"invalid wager transaction result balance",
	)
	ErrInvalidWagerTransactionReferenceSchedule = errors.New(
		"invalid wager transaction reference schedule",
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

func (transaction WagerTransaction) isInitialized() bool {
	validKind := transaction.kind == WagerTransactionKindOpening || transaction.kind.isExternal()

	return strings.TrimSpace(string(transaction.id)) != "" &&
		strings.TrimSpace(string(transaction.walletID)) != "" &&
		strings.TrimSpace(string(transaction.playerID)) != "" &&
		transaction.money.isInitialized() &&
		validKind &&
		transaction.status.isSupported() &&
		!transaction.createdAt.IsZero() &&
		!transaction.updatedAt.IsZero()
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

func (transaction WagerTransaction) Process(
	input ProcessWagerTransactionInput,
) (WagerTransaction, error) {
	if !transaction.isInitialized() {
		return WagerTransaction{}, fmt.Errorf(
			"process wager transaction: %w",
			ErrInvalidWagerTransaction,
		)
	}

	if transaction.status != WagerTransactionStatusPending {
		return WagerTransaction{}, fmt.Errorf(
			"process wager transaction: %w: from=%s, to=%s",
			ErrInvalidWagerTransactionStatusTransition,
			transaction.status,
			WagerTransactionStatusProcessed,
		)
	}

	if err := transaction.validateProcessResultBalance(input.ResultBalance); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	if err := transaction.validateProcessReference(input.ResolvedReferenceTransactionID); err != nil {
		return WagerTransaction{}, fmt.Errorf("process wager transaction: %w", err)
	}

	processed := transaction
	processed.resolvedReferenceTransactionID = input.ResolvedReferenceTransactionID
	processed.status = WagerTransactionStatusProcessed
	processed.resultBalance = input.ResultBalance
	processed.updatedAt = input.OccurredAt

	return processed, nil
}

func (transaction WagerTransaction) validatePendingReferenceEligibility() error {
	if transaction.kind != WagerTransactionKindWin &&
		transaction.kind != WagerTransactionKindRefund &&
		transaction.kind != WagerTransactionKindRollback {
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
	if input.NextAttemptAt.IsZero() || !input.NextAttemptAt.After(input.OccurredAt) {
		return fmt.Errorf(
			"%w: nextAttemptAt=%s must be after occurredAt=%s",
			ErrInvalidWagerTransactionReferenceSchedule,
			input.NextAttemptAt,
			input.OccurredAt,
		)
	}

	if input.ExpiresAt.IsZero() || !input.ExpiresAt.After(input.NextAttemptAt) {
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

	if !transaction.isInitialized() {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, ErrInvalidWagerTransaction)
	}

	if transaction.status != WagerTransactionStatusPending {
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

func (transaction WagerTransaction) hasValidPendingReferenceSchedule() bool {
	return transaction.referenceAttempts > 0 &&
		!transaction.nextReferenceAttemptAt.IsZero() &&
		transaction.nextReferenceAttemptAt.After(transaction.updatedAt) &&
		!transaction.referenceExpiresAt.IsZero() &&
		transaction.referenceExpiresAt.After(transaction.nextReferenceAttemptAt)
}

func (transaction WagerTransaction) RetryPendingReference(
	input RetryPendingReferenceWagerTransactionInput,
) (WagerTransaction, error) {
	const operation = "retry pending wager transaction reference"

	if !transaction.isInitialized() {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, ErrInvalidWagerTransaction)
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

	if !transaction.hasValidPendingReferenceSchedule() {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, ErrInvalidWagerTransaction)
	}

	if err := transaction.validateTransitionTimestamp(input.OccurredAt); err != nil {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, err)
	}

	if input.OccurredAt.Before(transaction.nextReferenceAttemptAt) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: occurredAt=%s precedes nextAttemptAt=%s",
			operation,
			ErrInvalidWagerTransactionReferenceSchedule,
			input.OccurredAt,
			transaction.nextReferenceAttemptAt,
		)
	}

	if !input.OccurredAt.Before(transaction.referenceExpiresAt) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: occurredAt=%s reached expiresAt=%s",
			operation,
			ErrInvalidWagerTransactionReferenceSchedule,
			input.OccurredAt,
			transaction.referenceExpiresAt,
		)
	}

	if input.NextAttemptAt.IsZero() ||
		!input.NextAttemptAt.After(input.OccurredAt) ||
		!input.NextAttemptAt.Before(transaction.referenceExpiresAt) {
		return WagerTransaction{}, fmt.Errorf(
			"%s: %w: nextAttemptAt=%s must be after occurredAt=%s and before expiresAt=%s",
			operation,
			ErrInvalidWagerTransactionReferenceSchedule,
			input.NextAttemptAt,
			input.OccurredAt,
			transaction.referenceExpiresAt,
		)
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

	if !transaction.isInitialized() {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, ErrInvalidWagerTransaction)
	}

	if transaction.status != WagerTransactionStatusPending &&
		transaction.status != WagerTransactionStatusPendingReference {
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

	if !transaction.isInitialized() {
		return WagerTransaction{}, fmt.Errorf("%s: %w", operation, ErrInvalidWagerTransaction)
	}

	if transaction.status != WagerTransactionStatusPending &&
		transaction.status != WagerTransactionStatusPendingReference {
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
