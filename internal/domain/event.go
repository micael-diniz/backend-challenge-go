package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type DomainEventID string
type DomainEventType string
type AggregateID string
type CorrelationID string
type CausationID string

const (
	WagerTransactionProcessedEventType        DomainEventType = "WagerTransactionProcessed"
	WagerTransactionRejectedEventType         DomainEventType = "WagerTransactionRejected"
	WalletBalanceChangedEventType             DomainEventType = "WalletBalanceChanged"
	WagerTransactionPendingReferenceEventType DomainEventType = "WagerTransactionPendingReference"
	domainEventVersion                                        = 1
)

var (
	ErrInvalidDomainEvent       = errors.New("invalid domain event")
	ErrInvalidDomainEventID     = errors.New("invalid domain event ID")
	ErrInvalidCorrelationID     = errors.New("invalid correlation ID")
	ErrInvalidCausationID       = errors.New("invalid causation ID")
	ErrInvalidDomainEventSource = errors.New("invalid domain event source")
)

type DomainEventMetadataInput struct {
	EventID       DomainEventID
	CorrelationID CorrelationID
	CausationID   CausationID
}

type domainEventMetadata struct {
	eventID       DomainEventID
	eventType     DomainEventType
	aggregateID   AggregateID
	correlationID CorrelationID
	causationID   CausationID
	occurredAt    time.Time
	version       int
}

func newDomainEventMetadata(
	input DomainEventMetadataInput,
	eventType DomainEventType,
	aggregateID AggregateID,
	occurredAt time.Time,
) (domainEventMetadata, error) {
	if strings.TrimSpace(string(input.EventID)) == "" {
		return domainEventMetadata{}, ErrInvalidDomainEventID
	}
	if strings.TrimSpace(string(input.CorrelationID)) == "" {
		return domainEventMetadata{}, ErrInvalidCorrelationID
	}
	causationText := string(input.CausationID)
	if causationText != "" && strings.TrimSpace(causationText) == "" {
		return domainEventMetadata{}, ErrInvalidCausationID
	}

	return domainEventMetadata{
		eventID:       input.EventID,
		eventType:     eventType,
		aggregateID:   aggregateID,
		correlationID: input.CorrelationID,
		causationID:   input.CausationID,
		occurredAt:    occurredAt.UTC(),
		version:       domainEventVersion,
	}, nil
}

func (metadata domainEventMetadata) EventID() DomainEventID {
	return metadata.eventID
}

func (metadata domainEventMetadata) EventType() DomainEventType {
	return metadata.eventType
}

func (metadata domainEventMetadata) AggregateID() AggregateID {
	return metadata.aggregateID
}

func (metadata domainEventMetadata) CorrelationID() CorrelationID {
	return metadata.correlationID
}

func (metadata domainEventMetadata) CausationID() (CausationID, bool) {
	return metadata.causationID, metadata.causationID != ""
}

func (metadata domainEventMetadata) OccurredAt() time.Time {
	return metadata.occurredAt
}

func (metadata domainEventMetadata) Version() int {
	return metadata.version
}

type wagerTransactionEventSnapshot struct {
	transactionID                  TransactionID
	providerID                     ProviderID
	externalTransactionID          ExternalTransactionID
	walletID                       WalletID
	playerID                       PlayerID
	roundID                        RoundID
	gameID                         GameID
	kind                           WagerTransactionKind
	money                          Money
	referenceExternalTransactionID ExternalTransactionID
}

func newWagerTransactionEventSnapshot(
	transaction WagerTransaction,
) wagerTransactionEventSnapshot {
	return wagerTransactionEventSnapshot{
		transactionID:                  transaction.ID(),
		providerID:                     transaction.ProviderID(),
		externalTransactionID:          transaction.ExternalTransactionID(),
		walletID:                       transaction.WalletID(),
		playerID:                       transaction.PlayerID(),
		roundID:                        transaction.RoundID(),
		gameID:                         transaction.GameID(),
		kind:                           transaction.Kind(),
		money:                          transaction.Money(),
		referenceExternalTransactionID: transaction.referenceExternalTransactionID,
	}
}

func (snapshot wagerTransactionEventSnapshot) TransactionID() TransactionID {
	return snapshot.transactionID
}

func (snapshot wagerTransactionEventSnapshot) ProviderID() ProviderID {
	return snapshot.providerID
}

func (snapshot wagerTransactionEventSnapshot) ExternalTransactionID() ExternalTransactionID {
	return snapshot.externalTransactionID
}

func (snapshot wagerTransactionEventSnapshot) WalletID() WalletID {
	return snapshot.walletID
}

func (snapshot wagerTransactionEventSnapshot) PlayerID() PlayerID {
	return snapshot.playerID
}

func (snapshot wagerTransactionEventSnapshot) RoundID() RoundID {
	return snapshot.roundID
}

func (snapshot wagerTransactionEventSnapshot) GameID() GameID {
	return snapshot.gameID
}

func (snapshot wagerTransactionEventSnapshot) Kind() WagerTransactionKind {
	return snapshot.kind
}

func (snapshot wagerTransactionEventSnapshot) Money() Money {
	return snapshot.money
}

func (snapshot wagerTransactionEventSnapshot) ReferenceExternalTransactionID() (
	ExternalTransactionID,
	bool,
) {
	return snapshot.referenceExternalTransactionID,
		snapshot.referenceExternalTransactionID != ""
}

type WagerTransactionProcessed struct {
	domainEventMetadata
	data WagerTransactionProcessedData
}

type WagerTransactionProcessedData struct {
	wagerTransactionEventSnapshot
	resolvedReferenceTransactionID TransactionID
	resultBalance                  Money
}

func NewWagerTransactionProcessed(
	input DomainEventMetadataInput,
	transaction WagerTransaction,
) (WagerTransactionProcessed, error) {
	if err := validateWagerTransactionEventSource(
		transaction,
		WagerTransactionStatusProcessed,
	); err != nil {
		return WagerTransactionProcessed{}, wrapDomainEventError(
			"wager transaction processed",
			err,
		)
	}

	metadata, err := newDomainEventMetadata(
		input,
		WagerTransactionProcessedEventType,
		AggregateID(transaction.ID()),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return WagerTransactionProcessed{}, wrapDomainEventError(
			"wager transaction processed",
			err,
		)
	}

	resolvedReferenceID, _ := transaction.ResolvedReferenceTransactionID()
	resultBalance, _ := transaction.ResultBalance()
	return WagerTransactionProcessed{
		domainEventMetadata: metadata,
		data: WagerTransactionProcessedData{
			wagerTransactionEventSnapshot:  newWagerTransactionEventSnapshot(transaction),
			resolvedReferenceTransactionID: resolvedReferenceID,
			resultBalance:                  resultBalance,
		},
	}, nil
}

func (event WagerTransactionProcessed) Data() WagerTransactionProcessedData {
	return event.data
}

func (data WagerTransactionProcessedData) ResolvedReferenceTransactionID() (
	TransactionID,
	bool,
) {
	return data.resolvedReferenceTransactionID, data.resolvedReferenceTransactionID != ""
}

func (data WagerTransactionProcessedData) ResultBalance() (Money, bool) {
	return data.resultBalance, data.resultBalance.isInitialized()
}

func (event WagerTransactionProcessed) MarshalJSON() ([]byte, error) {
	data := wagerTransactionProcessedJSON{
		wagerTransactionEventJSON:      wagerTransactionJSON(event.data.wagerTransactionEventSnapshot),
		ResolvedReferenceTransactionID: event.data.resolvedReferenceTransactionID,
	}
	if event.data.resultBalance.isInitialized() {
		resultBalance := event.data.resultBalance
		data.ResultBalance = &resultBalance
	}

	return marshalDomainEvent(event.domainEventMetadata, data)
}

type WagerTransactionRejected struct {
	domainEventMetadata
	data WagerTransactionRejectedData
}

type WagerTransactionRejectedData struct {
	wagerTransactionEventSnapshot
	failureCode WagerTransactionFailureCode
}

func NewWagerTransactionRejected(
	input DomainEventMetadataInput,
	transaction WagerTransaction,
) (WagerTransactionRejected, error) {
	if err := validateWagerTransactionEventSource(
		transaction,
		WagerTransactionStatusRejected,
	); err != nil {
		return WagerTransactionRejected{}, wrapDomainEventError(
			"wager transaction rejected",
			err,
		)
	}

	metadata, err := newDomainEventMetadata(
		input,
		WagerTransactionRejectedEventType,
		AggregateID(transaction.ID()),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return WagerTransactionRejected{}, wrapDomainEventError(
			"wager transaction rejected",
			err,
		)
	}

	failureCode, _ := transaction.FailureCode()
	return WagerTransactionRejected{
		domainEventMetadata: metadata,
		data: WagerTransactionRejectedData{
			wagerTransactionEventSnapshot: newWagerTransactionEventSnapshot(transaction),
			failureCode:                   failureCode,
		},
	}, nil
}

func (event WagerTransactionRejected) Data() WagerTransactionRejectedData {
	return event.data
}

func (data WagerTransactionRejectedData) FailureCode() WagerTransactionFailureCode {
	return data.failureCode
}

func (event WagerTransactionRejected) MarshalJSON() ([]byte, error) {
	return marshalDomainEvent(
		event.domainEventMetadata,
		wagerTransactionRejectedJSON{
			wagerTransactionEventJSON: wagerTransactionJSON(event.data.wagerTransactionEventSnapshot),
			FailureCode:               event.data.failureCode,
		},
	)
}

type WagerTransactionPendingReference struct {
	domainEventMetadata
	data WagerTransactionPendingReferenceData
}

type WagerTransactionPendingReferenceData struct {
	wagerTransactionEventSnapshot
	referenceAttempts      int32
	nextReferenceAttemptAt time.Time
	referenceExpiresAt     time.Time
}

func NewWagerTransactionPendingReference(
	input DomainEventMetadataInput,
	transaction WagerTransaction,
) (WagerTransactionPendingReference, error) {
	if err := validateWagerTransactionEventSource(
		transaction,
		WagerTransactionStatusPendingReference,
	); err != nil {
		return WagerTransactionPendingReference{}, wrapDomainEventError(
			"wager transaction pending reference",
			err,
		)
	}

	metadata, err := newDomainEventMetadata(
		input,
		WagerTransactionPendingReferenceEventType,
		AggregateID(transaction.ID()),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return WagerTransactionPendingReference{}, wrapDomainEventError(
			"wager transaction pending reference",
			err,
		)
	}

	nextAttemptAt, _ := transaction.NextReferenceAttemptAt()
	expiresAt, _ := transaction.ReferenceExpiresAt()
	return WagerTransactionPendingReference{
		domainEventMetadata: metadata,
		data: WagerTransactionPendingReferenceData{
			wagerTransactionEventSnapshot: newWagerTransactionEventSnapshot(transaction),
			referenceAttempts:             transaction.ReferenceAttempts(),
			nextReferenceAttemptAt:        nextAttemptAt.UTC(),
			referenceExpiresAt:            expiresAt.UTC(),
		},
	}, nil
}

func (event WagerTransactionPendingReference) Data() WagerTransactionPendingReferenceData {
	return event.data
}

func (data WagerTransactionPendingReferenceData) ReferenceAttempts() int32 {
	return data.referenceAttempts
}

func (data WagerTransactionPendingReferenceData) NextReferenceAttemptAt() time.Time {
	return data.nextReferenceAttemptAt
}

func (data WagerTransactionPendingReferenceData) ReferenceExpiresAt() time.Time {
	return data.referenceExpiresAt
}

func (event WagerTransactionPendingReference) MarshalJSON() ([]byte, error) {
	return marshalDomainEvent(
		event.domainEventMetadata,
		wagerTransactionPendingReferenceJSON{
			wagerTransactionEventJSON: wagerTransactionJSON(event.data.wagerTransactionEventSnapshot),
			ReferenceAttempts:         event.data.referenceAttempts,
			NextReferenceAttemptAt:    event.data.nextReferenceAttemptAt,
			ReferenceExpiresAt:        event.data.referenceExpiresAt,
		},
	)
}

type WalletBalanceChanged struct {
	domainEventMetadata
	data WalletBalanceChangedData
}

type WalletBalanceChangedData struct {
	walletID      WalletID
	transactionID TransactionID
	direction     LedgerDirection
	money         Money
	balanceBefore Money
	balanceAfter  Money
	walletVersion int64
}

func NewWalletBalanceChanged(
	input DomainEventMetadataInput,
	entry WalletLedgerEntry,
	wallet Wallet,
) (WalletBalanceChanged, error) {
	rebuilt, err := buildWalletLedgerEntry(WalletLedgerEntryInput{
		ID:            entry.ID(),
		WalletID:      entry.WalletID(),
		TransactionID: entry.TransactionID(),
		Direction:     entry.Direction(),
		Amount:        entry.Money(),
		BalanceBefore: entry.BalanceBefore(),
		BalanceAfter:  entry.BalanceAfter(),
		CreatedAt:     entry.CreatedAt(),
	})
	if err != nil {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			fmt.Errorf("%w: %w", ErrInvalidDomainEventSource, err),
		)
	}
	if err := wallet.validateForOperation(); err != nil {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			fmt.Errorf("%w: %w", ErrInvalidDomainEventSource, err),
		)
	}
	if wallet.ID() != rebuilt.WalletID() {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			fmt.Errorf(
				"%w: wallet ID=%q differs from ledger wallet ID=%q",
				ErrInvalidDomainEventSource,
				wallet.ID(),
				rebuilt.WalletID(),
			),
		)
	}
	if !wallet.Balance().equals(rebuilt.BalanceAfter()) {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			fmt.Errorf(
				"%w: wallet balance=%s differs from ledger balanceAfter=%s",
				ErrInvalidDomainEventSource,
				wallet.Balance().formatForError(),
				rebuilt.BalanceAfter().formatForError(),
			),
		)
	}
	if !wallet.UpdatedAt().Equal(rebuilt.CreatedAt()) {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			fmt.Errorf(
				"%w: wallet updatedAt=%s differs from ledger createdAt=%s",
				ErrInvalidDomainEventSource,
				wallet.UpdatedAt(),
				rebuilt.CreatedAt(),
			),
		)
	}

	metadata, err := newDomainEventMetadata(
		input,
		WalletBalanceChangedEventType,
		AggregateID(rebuilt.WalletID()),
		rebuilt.CreatedAt(),
	)
	if err != nil {
		return WalletBalanceChanged{}, wrapDomainEventError(
			"wallet balance changed",
			err,
		)
	}

	return WalletBalanceChanged{
		domainEventMetadata: metadata,
		data: WalletBalanceChangedData{
			walletID:      rebuilt.WalletID(),
			transactionID: rebuilt.TransactionID(),
			direction:     rebuilt.Direction(),
			money:         rebuilt.Money(),
			balanceBefore: rebuilt.BalanceBefore(),
			balanceAfter:  rebuilt.BalanceAfter(),
			walletVersion: wallet.Version(),
		},
	}, nil
}

func (event WalletBalanceChanged) Data() WalletBalanceChangedData {
	return event.data
}

func (data WalletBalanceChangedData) WalletID() WalletID {
	return data.walletID
}

func (data WalletBalanceChangedData) TransactionID() TransactionID {
	return data.transactionID
}

func (data WalletBalanceChangedData) Direction() LedgerDirection {
	return data.direction
}

func (data WalletBalanceChangedData) Money() Money {
	return data.money
}

func (data WalletBalanceChangedData) BalanceBefore() Money {
	return data.balanceBefore
}

func (data WalletBalanceChangedData) BalanceAfter() Money {
	return data.balanceAfter
}

func (data WalletBalanceChangedData) WalletVersion() int64 {
	return data.walletVersion
}

func (event WalletBalanceChanged) MarshalJSON() ([]byte, error) {
	return marshalDomainEvent(
		event.domainEventMetadata,
		walletBalanceChangedJSON{
			WalletID:      event.data.walletID,
			TransactionID: event.data.transactionID,
			Direction:     event.data.direction,
			Money:         event.data.money,
			BalanceBefore: event.data.balanceBefore,
			BalanceAfter:  event.data.balanceAfter,
			WalletVersion: event.data.walletVersion,
		},
	)
}

type domainEventEnvelopeJSON[T any] struct {
	EventID       DomainEventID   `json:"eventId"`
	EventType     DomainEventType `json:"eventType"`
	AggregateID   AggregateID     `json:"aggregateId"`
	CorrelationID CorrelationID   `json:"correlationId"`
	CausationID   CausationID     `json:"causationId,omitempty"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Version       int             `json:"version"`
	Data          T               `json:"data"`
}

type wagerTransactionEventJSON struct {
	TransactionID                  TransactionID         `json:"transactionId"`
	ProviderID                     ProviderID            `json:"providerId,omitempty"`
	ExternalTransactionID          ExternalTransactionID `json:"externalTransactionId,omitempty"`
	WalletID                       WalletID              `json:"walletId"`
	PlayerID                       PlayerID              `json:"playerId"`
	RoundID                        RoundID               `json:"roundId,omitempty"`
	GameID                         GameID                `json:"gameId,omitempty"`
	Kind                           WagerTransactionKind  `json:"kind"`
	Money                          Money                 `json:"money"`
	ReferenceExternalTransactionID ExternalTransactionID `json:"referenceExternalTransactionId,omitempty"`
}

type wagerTransactionProcessedJSON struct {
	wagerTransactionEventJSON
	ResolvedReferenceTransactionID TransactionID `json:"resolvedReferenceTransactionId,omitempty"`
	ResultBalance                  *Money        `json:"resultBalance,omitempty"`
}

type wagerTransactionRejectedJSON struct {
	wagerTransactionEventJSON
	FailureCode WagerTransactionFailureCode `json:"failureCode"`
}

type wagerTransactionPendingReferenceJSON struct {
	wagerTransactionEventJSON
	ReferenceAttempts      int32     `json:"referenceAttempts"`
	NextReferenceAttemptAt time.Time `json:"nextReferenceAttemptAt"`
	ReferenceExpiresAt     time.Time `json:"referenceExpiresAt"`
}

type walletBalanceChangedJSON struct {
	WalletID      WalletID        `json:"walletId"`
	TransactionID TransactionID   `json:"transactionId"`
	Direction     LedgerDirection `json:"direction"`
	Money         Money           `json:"money"`
	BalanceBefore Money           `json:"balanceBefore"`
	BalanceAfter  Money           `json:"balanceAfter"`
	WalletVersion int64           `json:"walletVersion"`
}

func wagerTransactionJSON(
	snapshot wagerTransactionEventSnapshot,
) wagerTransactionEventJSON {
	return wagerTransactionEventJSON{
		TransactionID:                  snapshot.transactionID,
		ProviderID:                     snapshot.providerID,
		ExternalTransactionID:          snapshot.externalTransactionID,
		WalletID:                       snapshot.walletID,
		PlayerID:                       snapshot.playerID,
		RoundID:                        snapshot.roundID,
		GameID:                         snapshot.gameID,
		Kind:                           snapshot.kind,
		Money:                          snapshot.money,
		ReferenceExternalTransactionID: snapshot.referenceExternalTransactionID,
	}
}

func marshalDomainEvent[T any](
	metadata domainEventMetadata,
	data T,
) ([]byte, error) {
	return json.Marshal(domainEventEnvelopeJSON[T]{
		EventID:       metadata.eventID,
		EventType:     metadata.eventType,
		AggregateID:   metadata.aggregateID,
		CorrelationID: metadata.correlationID,
		CausationID:   metadata.causationID,
		OccurredAt:    metadata.occurredAt,
		Version:       metadata.version,
		Data:          data,
	})
}

func validateWagerTransactionEventSource(
	transaction WagerTransaction,
	wantStatus WagerTransactionStatus,
) error {
	if err := transaction.validateRehydratedState(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDomainEventSource, err)
	}
	if transaction.Status() != wantStatus {
		return fmt.Errorf(
			"%w: %w: status=%s, want=%s",
			ErrInvalidDomainEventSource,
			ErrInvalidWagerTransactionStatus,
			transaction.Status(),
			wantStatus,
		)
	}

	return nil
}

func wrapDomainEventError(eventName string, err error) error {
	return fmt.Errorf("create %s: %w: %w", eventName, ErrInvalidDomainEvent, err)
}

func (eventType DomainEventType) String() string {
	return string(eventType)
}
