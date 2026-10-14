package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewWagerTransactionProcessed(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	processed := mustProcessWagerTransaction(t, pending)
	metadata := validDomainEventMetadataInput()

	event, err := NewWagerTransactionProcessed(metadata, processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}

	assertDomainEventMetadata(
		t,
		event,
		metadata,
		WagerTransactionProcessedEventType,
		AggregateID(processed.ID()),
		processed.UpdatedAt(),
	)
	data := event.Data()
	if data.TransactionID() != processed.ID() {
		t.Errorf("Data().TransactionID() = %q; want %q", data.TransactionID(), processed.ID())
	}
	if data.WalletID() != processed.WalletID() {
		t.Errorf("Data().WalletID() = %q; want %q", data.WalletID(), processed.WalletID())
	}
	if data.ExternalTransactionID() != processed.ExternalTransactionID() {
		t.Errorf("Data().ExternalTransactionID() = %q; want %q", data.ExternalTransactionID(), processed.ExternalTransactionID())
	}
	if data.PlayerID() != processed.PlayerID() {
		t.Errorf("Data().PlayerID() = %q; want %q", data.PlayerID(), processed.PlayerID())
	}
	if data.RoundID() != processed.RoundID() {
		t.Errorf("Data().RoundID() = %q; want %q", data.RoundID(), processed.RoundID())
	}
	if data.GameID() != processed.GameID() {
		t.Errorf("Data().GameID() = %q; want %q", data.GameID(), processed.GameID())
	}
	if data.Kind() != processed.Kind() {
		t.Errorf("Data().Kind() = %q; want %q", data.Kind(), processed.Kind())
	}
	assertMoneyState(t, "Data().Money()", data.Money(), processed.Money())
	resultBalance, present := data.ResultBalance()
	if !present {
		t.Fatal("Data().ResultBalance() is absent; want present")
	}
	wantResultBalance, _ := processed.ResultBalance()
	assertMoneyState(t, "Data().ResultBalance()", resultBalance, wantResultBalance)
	if _, present := data.ResolvedReferenceTransactionID(); present {
		t.Error("Data().ResolvedReferenceTransactionID() is present; want absent")
	}
	if _, present := data.ReferenceExternalTransactionID(); present {
		t.Error("Data().ReferenceExternalTransactionID() is present; want absent")
	}
}

func TestNewWagerTransactionProcessedPreservesResolvedReference(t *testing.T) {
	t.Parallel()

	processed := mustProcessWagerTransaction(t, mustNewPendingReferenceWagerTransaction(t))
	event, err := NewWagerTransactionProcessed(validDomainEventMetadataInput(), processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}

	got, present := event.Data().ResolvedReferenceTransactionID()
	want, _ := processed.ResolvedReferenceTransactionID()
	if !present || got != want {
		t.Errorf("Data().ResolvedReferenceTransactionID() = (%q, %t); want (%q, true)", got, present, want)
	}
	externalReferenceID, present := event.Data().ReferenceExternalTransactionID()
	wantExternalReferenceID, _ := processed.ReferenceExternalTransactionID()
	if !present || externalReferenceID != wantExternalReferenceID {
		t.Errorf("Data().ReferenceExternalTransactionID() = (%q, %t); want (%q, true)", externalReferenceID, present, wantExternalReferenceID)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() returned an unexpected error: %v", err)
	}
	for _, fragment := range []string{
		`"referenceExternalTransactionId":"missing-external-transaction-1"`,
		`"resolvedReferenceTransactionId":"referenced-transaction-1"`,
	} {
		if !strings.Contains(string(encoded), fragment) {
			t.Errorf("json.Marshal() = %s; want fragment %s", encoded, fragment)
		}
	}
}

func TestNewWagerTransactionProcessedSupportsInternalOpening(t *testing.T) {
	t.Parallel()

	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}

	event, err := NewWagerTransactionProcessed(validDomainEventMetadataInput(), opening)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}

	data := event.Data()
	if data.Kind() != WagerTransactionKindOpening {
		t.Errorf("Data().Kind() = %q; want %q", data.Kind(), WagerTransactionKindOpening)
	}
	if data.ProviderID() != "" {
		t.Errorf("Data().ProviderID() = %q; want absent internal metadata", data.ProviderID())
	}
	if _, present := data.ResultBalance(); present {
		t.Error("Data().ResultBalance() is present; want absent for opening")
	}
}

func TestNewWagerTransactionProcessedSupportsLoss(t *testing.T) {
	t.Parallel()

	loss := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindLoss
		input.Money = mustNewMoney(t, "0.00", "BRL")
	})
	processed := mustProcessWagerTransaction(t, loss)

	event, err := NewWagerTransactionProcessed(validDomainEventMetadataInput(), processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	if event.Data().Kind() != WagerTransactionKindLoss {
		t.Errorf("Data().Kind() = %q; want %q", event.Data().Kind(), WagerTransactionKindLoss)
	}
	if !event.Data().Money().isZero() {
		t.Errorf("Data().Money() = %s; want zero LOSS amount", event.Data().Money().Amount())
	}
}

func TestNewWagerTransactionRejected(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	rejected := mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds)
	metadata := validDomainEventMetadataInput()

	event, err := NewWagerTransactionRejected(metadata, rejected)
	if err != nil {
		t.Fatalf("NewWagerTransactionRejected() returned an unexpected error: %v", err)
	}

	assertDomainEventMetadata(
		t,
		event,
		metadata,
		WagerTransactionRejectedEventType,
		AggregateID(rejected.ID()),
		rejected.UpdatedAt(),
	)
	data := event.Data()
	if data.TransactionID() != rejected.ID() {
		t.Errorf("Data().TransactionID() = %q; want %q", data.TransactionID(), rejected.ID())
	}
	failureCode, _ := rejected.FailureCode()
	if data.FailureCode() != failureCode {
		t.Errorf("Data().FailureCode() = %q; want %q", data.FailureCode(), failureCode)
	}
}

func TestNewWagerTransactionPendingReference(t *testing.T) {
	t.Parallel()

	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	metadata := validDomainEventMetadataInput()

	event, err := NewWagerTransactionPendingReference(metadata, pendingReference)
	if err != nil {
		t.Fatalf("NewWagerTransactionPendingReference() returned an unexpected error: %v", err)
	}

	assertDomainEventMetadata(
		t,
		event,
		metadata,
		WagerTransactionPendingReferenceEventType,
		AggregateID(pendingReference.ID()),
		pendingReference.UpdatedAt(),
	)
	data := event.Data()
	referenceID, _ := pendingReference.ReferenceExternalTransactionID()
	gotReferenceID, present := data.ReferenceExternalTransactionID()
	if !present || gotReferenceID != referenceID {
		t.Errorf("Data().ReferenceExternalTransactionID() = (%q, %t); want (%q, true)", gotReferenceID, present, referenceID)
	}
	if data.ReferenceAttempts() != pendingReference.ReferenceAttempts() {
		t.Errorf("Data().ReferenceAttempts() = %d; want %d", data.ReferenceAttempts(), pendingReference.ReferenceAttempts())
	}
	wantNextAttemptAt, _ := pendingReference.NextReferenceAttemptAt()
	if !data.NextReferenceAttemptAt().Equal(wantNextAttemptAt) {
		t.Errorf("Data().NextReferenceAttemptAt() = %s; want %s", data.NextReferenceAttemptAt(), wantNextAttemptAt)
	}
	wantExpiresAt, _ := pendingReference.ReferenceExpiresAt()
	if !data.ReferenceExpiresAt().Equal(wantExpiresAt) {
		t.Errorf("Data().ReferenceExpiresAt() = %s; want %s", data.ReferenceExpiresAt(), wantExpiresAt)
	}
	if data.NextReferenceAttemptAt().Location() != time.UTC || data.ReferenceExpiresAt().Location() != time.UTC {
		t.Error("pending reference event schedule is not normalized to UTC")
	}
}

func TestDomainEventCausationIDIsOptional(t *testing.T) {
	t.Parallel()

	processed := mustProcessWagerTransaction(
		t,
		mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {}),
	)
	metadata := validDomainEventMetadataInput()
	metadata.CausationID = ""

	event, err := NewWagerTransactionProcessed(metadata, processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	if causationID, present := event.CausationID(); present || causationID != "" {
		t.Errorf("CausationID() = (%q, %t); want empty and absent", causationID, present)
	}
}

func TestNewWalletBalanceChanged(t *testing.T) {
	t.Parallel()

	entry := mustNewWalletBalanceChangedLedgerEntry(t)
	wallet := mustWalletAfterBalanceChange(t)
	metadata := validDomainEventMetadataInput()

	event, err := NewWalletBalanceChanged(metadata, entry, wallet)
	if err != nil {
		t.Fatalf("NewWalletBalanceChanged() returned an unexpected error: %v", err)
	}

	assertDomainEventMetadata(
		t,
		event,
		metadata,
		WalletBalanceChangedEventType,
		AggregateID(entry.WalletID()),
		entry.CreatedAt(),
	)
	data := event.Data()
	if data.WalletID() != entry.WalletID() {
		t.Errorf("Data().WalletID() = %q; want %q", data.WalletID(), entry.WalletID())
	}
	if data.TransactionID() != entry.TransactionID() {
		t.Errorf("Data().TransactionID() = %q; want %q", data.TransactionID(), entry.TransactionID())
	}
	if data.Direction() != entry.Direction() {
		t.Errorf("Data().Direction() = %q; want %q", data.Direction(), entry.Direction())
	}
	assertMoneyState(t, "Data().Money()", data.Money(), entry.Money())
	assertMoneyState(t, "Data().BalanceBefore()", data.BalanceBefore(), entry.BalanceBefore())
	assertMoneyState(t, "Data().BalanceAfter()", data.BalanceAfter(), entry.BalanceAfter())
	if data.WalletVersion() != wallet.Version() {
		t.Errorf("Data().WalletVersion() = %d; want %d", data.WalletVersion(), wallet.Version())
	}
}

func TestWalletBalanceChangedMarshalsRequiredContract(t *testing.T) {
	t.Parallel()

	event, err := NewWalletBalanceChanged(
		validDomainEventMetadataInput(),
		mustNewWalletBalanceChangedLedgerEntry(t),
		mustWalletAfterBalanceChange(t),
	)
	if err != nil {
		t.Fatalf("NewWalletBalanceChanged() returned an unexpected error: %v", err)
	}

	got, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() returned an unexpected error: %v", err)
	}
	want := `{"eventId":"event-1","eventType":"WalletBalanceChanged","aggregateId":"wallet-1","correlationId":"correlation-1","causationId":"message-1","occurredAt":"2026-10-14T18:00:00Z","version":1,"data":{"walletId":"wallet-1","transactionId":"transaction-1","direction":"DEBIT","money":{"amount":"25.00","currency":"BRL"},"balanceBefore":{"amount":"100.00","currency":"BRL"},"balanceAfter":{"amount":"75.00","currency":"BRL"},"walletVersion":2}}`
	if string(got) != want {
		t.Errorf("json.Marshal() = %s; want %s", got, want)
	}
}

func TestWagerTransactionEventsMarshalTypedData(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	rejected := mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds)
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	processed := mustProcessWagerTransaction(t, pending)

	type eventCase struct {
		value      any
		eventType  DomainEventType
		aggregate  AggregateID
		occurredAt time.Time
	}
	events := []eventCase{}
	processedEvent, err := NewWagerTransactionProcessed(validDomainEventMetadataInput(), processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	events = append(events, eventCase{
		value:      processedEvent,
		eventType:  processedEvent.EventType(),
		aggregate:  processedEvent.AggregateID(),
		occurredAt: processedEvent.OccurredAt(),
	})
	rejectedEvent, err := NewWagerTransactionRejected(validDomainEventMetadataInput(), rejected)
	if err != nil {
		t.Fatalf("NewWagerTransactionRejected() returned an unexpected error: %v", err)
	}
	events = append(events, eventCase{
		value:      rejectedEvent,
		eventType:  rejectedEvent.EventType(),
		aggregate:  rejectedEvent.AggregateID(),
		occurredAt: rejectedEvent.OccurredAt(),
	})
	pendingEvent, err := NewWagerTransactionPendingReference(validDomainEventMetadataInput(), pendingReference)
	if err != nil {
		t.Fatalf("NewWagerTransactionPendingReference() returned an unexpected error: %v", err)
	}
	events = append(events, eventCase{
		value:      pendingEvent,
		eventType:  pendingEvent.EventType(),
		aggregate:  pendingEvent.AggregateID(),
		occurredAt: pendingEvent.OccurredAt(),
	})

	for _, event := range events {
		encoded, err := json.Marshal(event.value)
		if err != nil {
			t.Fatalf("json.Marshal(%s) returned an unexpected error: %v", event.eventType, err)
		}
		if !json.Valid(encoded) {
			t.Errorf("json.Marshal(%s) = %s; want valid JSON", event.eventType, encoded)
		}
		for _, fragment := range []string{
			`"eventId":"event-1"`,
			`"eventType":"` + event.eventType.String() + `"`,
			`"aggregateId":"` + string(event.aggregate) + `"`,
			`"correlationId":"correlation-1"`,
			`"occurredAt":"` + event.occurredAt.Format(time.RFC3339Nano) + `"`,
			`"version":1`,
			`"data":{`,
			`"transactionId":"transaction-1"`,
			`"money":{"amount":"25.00","currency":"BRL"}`,
		} {
			if !strings.Contains(string(encoded), fragment) {
				t.Errorf("json.Marshal(%s) = %s; want fragment %s", event.eventType, encoded, fragment)
			}
		}
	}
}

func TestInternalEventJSONOmitsInapplicableOptionalFields(t *testing.T) {
	t.Parallel()

	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}
	metadata := validDomainEventMetadataInput()
	metadata.CausationID = ""
	event, err := NewWagerTransactionProcessed(metadata, opening)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() returned an unexpected error: %v", err)
	}
	for _, unwanted := range []string{
		`"causationId"`,
		`"providerId"`,
		`"externalTransactionId"`,
		`"roundId"`,
		`"gameId"`,
		`"referenceExternalTransactionId"`,
		`"resolvedReferenceTransactionId"`,
		`"resultBalance"`,
	} {
		if strings.Contains(string(encoded), unwanted) {
			t.Errorf("json.Marshal() = %s; want %s omitted", encoded, unwanted)
		}
	}
}

func TestDomainEventConstructorsRejectInvalidMetadata(t *testing.T) {
	t.Parallel()

	processed := mustProcessWagerTransaction(
		t,
		mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {}),
	)
	valid := validDomainEventMetadataInput()
	tests := []struct {
		name      string
		mutate    func(*DomainEventMetadataInput)
		wantError error
	}{
		{name: "empty event ID", mutate: func(input *DomainEventMetadataInput) { input.EventID = "" }, wantError: ErrInvalidDomainEventID},
		{name: "blank event ID", mutate: func(input *DomainEventMetadataInput) { input.EventID = " " }, wantError: ErrInvalidDomainEventID},
		{name: "empty correlation ID", mutate: func(input *DomainEventMetadataInput) { input.CorrelationID = "" }, wantError: ErrInvalidCorrelationID},
		{name: "blank correlation ID", mutate: func(input *DomainEventMetadataInput) { input.CorrelationID = "\t" }, wantError: ErrInvalidCorrelationID},
		{name: "blank present causation ID", mutate: func(input *DomainEventMetadataInput) { input.CausationID = " " }, wantError: ErrInvalidCausationID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)

			got, err := NewWagerTransactionProcessed(input, processed)
			if !errors.Is(err, ErrInvalidDomainEvent) {
				t.Fatalf("NewWagerTransactionProcessed() error = %v; want ErrInvalidDomainEvent", err)
			}
			if !errors.Is(err, tt.wantError) {
				t.Errorf("NewWagerTransactionProcessed() error = %v; want %v", err, tt.wantError)
			}
			if got != (WagerTransactionProcessed{}) {
				t.Errorf("NewWagerTransactionProcessed() event = %+v; want zero value", got)
			}
		})
	}
}

func TestDomainEventConstructorsRejectInvalidSources(t *testing.T) {
	t.Parallel()

	metadata := validDomainEventMetadataInput()
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})

	if got, err := NewWagerTransactionProcessed(metadata, pending); !errors.Is(err, ErrInvalidDomainEventSource) || got != (WagerTransactionProcessed{}) {
		t.Errorf("NewWagerTransactionProcessed() = (%+v, %v); want zero event and ErrInvalidDomainEventSource", got, err)
	}
	if got, err := NewWagerTransactionProcessed(metadata, WagerTransaction{}); !errors.Is(err, ErrInvalidTransactionID) || got != (WagerTransactionProcessed{}) {
		t.Errorf("NewWagerTransactionProcessed() = (%+v, %v); want zero event and ErrInvalidTransactionID", got, err)
	}
	if got, err := NewWagerTransactionRejected(metadata, pending); !errors.Is(err, ErrInvalidDomainEventSource) || got != (WagerTransactionRejected{}) {
		t.Errorf("NewWagerTransactionRejected() = (%+v, %v); want zero event and ErrInvalidDomainEventSource", got, err)
	}
	if got, err := NewWagerTransactionPendingReference(metadata, pending); !errors.Is(err, ErrInvalidDomainEventSource) || got != (WagerTransactionPendingReference{}) {
		t.Errorf("NewWagerTransactionPendingReference() = (%+v, %v); want zero event and ErrInvalidDomainEventSource", got, err)
	}
	wallet := mustWalletAfterBalanceChange(t)
	if got, err := NewWalletBalanceChanged(metadata, WalletLedgerEntry{}, wallet); !errors.Is(err, ErrInvalidDomainEventSource) || got != (WalletBalanceChanged{}) {
		t.Errorf("NewWalletBalanceChanged() = (%+v, %v); want zero event and ErrInvalidDomainEventSource", got, err)
	}
	entry := mustNewWalletBalanceChangedLedgerEntry(t)
	if got, err := NewWalletBalanceChanged(metadata, entry, Wallet{}); !errors.Is(err, ErrInvalidWalletID) || got != (WalletBalanceChanged{}) {
		t.Errorf("NewWalletBalanceChanged() = (%+v, %v); want zero event and ErrInvalidWalletID", got, err)
	}
}

func TestNewWalletBalanceChangedRejectsMismatchedSnapshots(t *testing.T) {
	t.Parallel()

	metadata := validDomainEventMetadataInput()
	entry := mustNewWalletBalanceChangedLedgerEntry(t)
	validWallet := mustWalletAfterBalanceChange(t)

	tests := []struct {
		name   string
		mutate func(*Wallet)
	}{
		{name: "wallet ID", mutate: func(wallet *Wallet) { wallet.id = "other-wallet" }},
		{name: "balance after", mutate: func(wallet *Wallet) { wallet.balance = mustNewMoney(t, "74.99", "BRL") }},
		{name: "occurrence time", mutate: func(wallet *Wallet) { wallet.updatedAt = wallet.updatedAt.Add(time.Second) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet := validWallet
			tt.mutate(&wallet)

			got, err := NewWalletBalanceChanged(metadata, entry, wallet)
			if !errors.Is(err, ErrInvalidDomainEventSource) {
				t.Fatalf("NewWalletBalanceChanged() error = %v; want ErrInvalidDomainEventSource", err)
			}
			if got != (WalletBalanceChanged{}) {
				t.Errorf("NewWalletBalanceChanged() event = %+v; want zero value", got)
			}
		})
	}
}

func TestEachDomainEventConstructorValidatesMetadata(t *testing.T) {
	t.Parallel()

	invalidMetadata := validDomainEventMetadataInput()
	invalidMetadata.EventID = ""
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	rejected := mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds)
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	entry := mustNewWalletBalanceChangedLedgerEntry(t)
	wallet := mustWalletAfterBalanceChange(t)

	if _, err := NewWagerTransactionRejected(invalidMetadata, rejected); !errors.Is(err, ErrInvalidDomainEventID) {
		t.Errorf("NewWagerTransactionRejected() error = %v; want ErrInvalidDomainEventID", err)
	}
	if _, err := NewWagerTransactionPendingReference(invalidMetadata, pendingReference); !errors.Is(err, ErrInvalidDomainEventID) {
		t.Errorf("NewWagerTransactionPendingReference() error = %v; want ErrInvalidDomainEventID", err)
	}
	if _, err := NewWalletBalanceChanged(invalidMetadata, entry, wallet); !errors.Is(err, ErrInvalidDomainEventID) {
		t.Errorf("NewWalletBalanceChanged() error = %v; want ErrInvalidDomainEventID", err)
	}
}

type testDomainEvent interface {
	EventID() DomainEventID
	EventType() DomainEventType
	AggregateID() AggregateID
	CorrelationID() CorrelationID
	CausationID() (CausationID, bool)
	OccurredAt() time.Time
	Version() int
}

func assertDomainEventMetadata(
	t *testing.T,
	event testDomainEvent,
	wantInput DomainEventMetadataInput,
	wantType DomainEventType,
	wantAggregateID AggregateID,
	wantOccurredAt time.Time,
) {
	t.Helper()

	if event.EventID() != wantInput.EventID {
		t.Errorf("EventID() = %q; want %q", event.EventID(), wantInput.EventID)
	}
	if event.EventType() != wantType {
		t.Errorf("EventType() = %q; want %q", event.EventType(), wantType)
	}
	if event.AggregateID() != wantAggregateID {
		t.Errorf("AggregateID() = %q; want %q", event.AggregateID(), wantAggregateID)
	}
	if event.CorrelationID() != wantInput.CorrelationID {
		t.Errorf("CorrelationID() = %q; want %q", event.CorrelationID(), wantInput.CorrelationID)
	}
	causationID, present := event.CausationID()
	if !present || causationID != wantInput.CausationID {
		t.Errorf("CausationID() = (%q, %t); want (%q, true)", causationID, present, wantInput.CausationID)
	}
	if !event.OccurredAt().Equal(wantOccurredAt) {
		t.Errorf("OccurredAt() = %s; want instant %s", event.OccurredAt(), wantOccurredAt)
	}
	if event.OccurredAt().Location() != time.UTC {
		t.Errorf("OccurredAt().Location() = %s; want UTC", event.OccurredAt().Location())
	}
	if event.Version() != 1 {
		t.Errorf("Version() = %d; want constructor-owned version 1", event.Version())
	}
}

func validDomainEventMetadataInput() DomainEventMetadataInput {
	return DomainEventMetadataInput{
		EventID:       DomainEventID("event-1"),
		CorrelationID: CorrelationID("correlation-1"),
		CausationID:   CausationID("message-1"),
	}
}

func mustNewWalletBalanceChangedLedgerEntry(t *testing.T) WalletLedgerEntry {
	t.Helper()

	entry, err := NewWalletLedgerEntry(WalletLedgerEntryInput{
		ID:            LedgerEntryID("ledger-1"),
		WalletID:      WalletID("wallet-1"),
		TransactionID: TransactionID("transaction-1"),
		Direction:     LedgerDirectionDebit,
		Amount:        mustNewMoney(t, "25.00", "BRL"),
		BalanceBefore: mustNewMoney(t, "100.00", "BRL"),
		BalanceAfter:  mustNewMoney(t, "75.00", "BRL"),
		CreatedAt: time.Date(
			2026,
			time.October,
			14,
			15,
			0,
			0,
			0,
			time.FixedZone("BRT", -3*60*60),
		),
	})
	if err != nil {
		t.Fatalf("NewWalletLedgerEntry() returned an unexpected error: %v", err)
	}
	return entry
}

func mustWalletAfterBalanceChange(t *testing.T) Wallet {
	t.Helper()

	changedAt := time.Date(
		2026,
		time.October,
		14,
		15,
		0,
		0,
		0,
		time.FixedZone("BRT", -3*60*60),
	)
	wallet, err := NewWallet(
		WalletID("wallet-1"),
		PlayerID("player-1"),
		mustNewMoney(t, "100.00", "BRL"),
		changedAt.Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("NewWallet() returned an unexpected error: %v", err)
	}
	wallet, err = wallet.Debit(mustNewMoney(t, "25.00", "BRL"), changedAt)
	if err != nil {
		t.Fatalf("Wallet.Debit() returned an unexpected error: %v", err)
	}
	return wallet
}
