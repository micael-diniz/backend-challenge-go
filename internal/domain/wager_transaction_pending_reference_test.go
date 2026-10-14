package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestWagerTransactionMarkPendingReference(t *testing.T) {
	tests := []struct {
		name string
		kind WagerTransactionKind
	}{
		{name: "win with optional reference", kind: WagerTransactionKindWin},
		{name: "refund", kind: WagerTransactionKindRefund},
		{name: "rollback", kind: WagerTransactionKindRollback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
				input.Kind = tt.kind
				input.ReferenceExternalTransactionID = ExternalTransactionID("missing-external-transaction-1")
			})
			occurredAt := original.UpdatedAt().Add(time.Minute)
			nextAttemptAt := occurredAt.Add(5 * time.Second)
			expiresAt := occurredAt.Add(24 * time.Hour)

			got, err := original.MarkPendingReference(PendingReferenceWagerTransactionInput{
				OccurredAt:    occurredAt,
				NextAttemptAt: nextAttemptAt,
				ExpiresAt:     expiresAt,
			})
			if err != nil {
				t.Fatalf("MarkPendingReference() returned an unexpected error: %v", err)
			}

			if got.Status() != WagerTransactionStatusPendingReference {
				t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusPendingReference)
			}
			if !got.UpdatedAt().Equal(occurredAt) {
				t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), occurredAt)
			}
			if got.ReferenceAttempts() != 1 {
				t.Errorf("ReferenceAttempts() = %d; want 1", got.ReferenceAttempts())
			}

			gotNextAttemptAt, present := got.NextReferenceAttemptAt()
			if !present || !gotNextAttemptAt.Equal(nextAttemptAt) {
				t.Errorf("NextReferenceAttemptAt() = (%s, %t); want (%s, true)", gotNextAttemptAt, present, nextAttemptAt)
			}
			gotExpiresAt, present := got.ReferenceExpiresAt()
			if !present || !gotExpiresAt.Equal(expiresAt) {
				t.Errorf("ReferenceExpiresAt() = (%s, %t); want (%s, true)", gotExpiresAt, present, expiresAt)
			}

			if _, present := got.ResolvedReferenceTransactionID(); present {
				t.Error("ResolvedReferenceTransactionID() is present; want absent while reference is missing")
			}
			if _, present := got.ResultBalance(); present {
				t.Error("ResultBalance() is present; want absent while reference is missing")
			}
			if _, present := got.FailureCode(); present {
				t.Error("FailureCode() is present; want absent while reference is pending")
			}

			if original.Status() != WagerTransactionStatusPending {
				t.Errorf("original Status() = %q; want %q", original.Status(), WagerTransactionStatusPending)
			}
			assertPendingReferenceMetadataAbsent(t, original)
		})
	}
}

func TestWagerTransactionMarkPendingReferenceRejectsInvalidInput(t *testing.T) {
	refund := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindRefund
		input.ReferenceExternalTransactionID = ExternalTransactionID("missing-external-transaction-1")
	})
	rollback := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindRollback
		input.ReferenceExternalTransactionID = ExternalTransactionID("missing-external-transaction-1")
	})
	bet := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	winWithoutReference := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindWin
	})
	loss := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindLoss
		input.Money = mustNewMoney(t, "0.00", "BRL")
	})
	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}

	missingExternalReference := refund
	missingExternalReference.referenceExternalTransactionID = ""
	alreadyResolved := refund
	alreadyResolved.resolvedReferenceTransactionID = TransactionID("bet-transaction-1")
	pendingReference := refund
	pendingReference.status = WagerTransactionStatusPendingReference
	rejected := refund
	rejected.status = WagerTransactionStatusRejected
	failed := refund
	failed.status = WagerTransactionStatusFailed

	occurredAt := refund.UpdatedAt().Add(time.Minute)
	nextAttemptAt := occurredAt.Add(5 * time.Second)
	expiresAt := occurredAt.Add(24 * time.Hour)
	validInput := PendingReferenceWagerTransactionInput{
		OccurredAt:    occurredAt,
		NextAttemptAt: nextAttemptAt,
		ExpiresAt:     expiresAt,
	}

	tests := []struct {
		name      string
		state     WagerTransaction
		input     PendingReferenceWagerTransactionInput
		wantError error
	}{
		{name: "uninitialized transaction", state: WagerTransaction{}, input: validInput, wantError: ErrInvalidWagerTransaction},
		{name: "processed transaction", state: opening, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "already pending reference", state: pendingReference, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "rejected transaction", state: rejected, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "failed transaction", state: failed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "bet does not wait for reference", state: bet, input: validInput, wantError: ErrInvalidWagerTransactionReference},
		{name: "win without external reference does not wait", state: winWithoutReference, input: validInput, wantError: ErrInvalidWagerTransactionReference},
		{name: "loss does not wait for reference", state: loss, input: validInput, wantError: ErrInvalidWagerTransactionReference},
		{name: "required external reference is missing", state: missingExternalReference, input: validInput, wantError: ErrInvalidWagerTransactionReference},
		{name: "reference is already resolved", state: alreadyResolved, input: validInput, wantError: ErrInvalidWagerTransactionReference},
		{name: "zero occurrence time", state: refund, input: PendingReferenceWagerTransactionInput{NextAttemptAt: nextAttemptAt, ExpiresAt: expiresAt}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "backdated occurrence time", state: refund, input: PendingReferenceWagerTransactionInput{OccurredAt: refund.UpdatedAt().Add(-time.Nanosecond), NextAttemptAt: nextAttemptAt, ExpiresAt: expiresAt}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "zero next attempt", state: refund, input: PendingReferenceWagerTransactionInput{OccurredAt: occurredAt, ExpiresAt: expiresAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "next attempt equals occurrence", state: refund, input: PendingReferenceWagerTransactionInput{OccurredAt: occurredAt, NextAttemptAt: occurredAt, ExpiresAt: expiresAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "zero expiration", state: refund, input: PendingReferenceWagerTransactionInput{OccurredAt: occurredAt, NextAttemptAt: nextAttemptAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "expiration equals next attempt", state: refund, input: PendingReferenceWagerTransactionInput{OccurredAt: occurredAt, NextAttemptAt: nextAttemptAt, ExpiresAt: nextAttemptAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "rollback invalid schedule", state: rollback, input: PendingReferenceWagerTransactionInput{OccurredAt: occurredAt, NextAttemptAt: nextAttemptAt, ExpiresAt: occurredAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.state

			got, err := tt.state.MarkPendingReference(tt.input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("MarkPendingReference() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "mark wager transaction pending reference", tt.wantError.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("MarkPendingReference() transaction = %+v; want zero value on error", got)
			}
			if tt.state != original {
				t.Errorf("MarkPendingReference() mutated original transaction: got %+v; want %+v", tt.state, original)
			}
		})
	}
}

func TestNewWagerTransactionsHaveNoPendingReferenceMetadata(t *testing.T) {
	external := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}

	assertPendingReferenceMetadataAbsent(t, external)
	assertPendingReferenceMetadataAbsent(t, opening)
}

func TestWagerTransactionRetryPendingReference(t *testing.T) {
	original := mustNewPendingReferenceWagerTransaction(t)
	currentNextAttemptAt, present := original.NextReferenceAttemptAt()
	if !present {
		t.Fatal("test setup has no next reference attempt")
	}
	expiresAt, present := original.ReferenceExpiresAt()
	if !present {
		t.Fatal("test setup has no reference expiration")
	}
	nextAttemptAt := currentNextAttemptAt.Add(10 * time.Second)

	got, err := original.RetryPendingReference(RetryPendingReferenceWagerTransactionInput{
		OccurredAt:    currentNextAttemptAt,
		NextAttemptAt: nextAttemptAt,
	})
	if err != nil {
		t.Fatalf("RetryPendingReference() returned an unexpected error: %v", err)
	}

	if got.Status() != WagerTransactionStatusPendingReference {
		t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusPendingReference)
	}
	if got.ReferenceAttempts() != original.ReferenceAttempts()+1 {
		t.Errorf("ReferenceAttempts() = %d; want %d", got.ReferenceAttempts(), original.ReferenceAttempts()+1)
	}
	gotNextAttemptAt, present := got.NextReferenceAttemptAt()
	if !present || !gotNextAttemptAt.Equal(nextAttemptAt) {
		t.Errorf("NextReferenceAttemptAt() = (%s, %t); want (%s, true)", gotNextAttemptAt, present, nextAttemptAt)
	}
	gotExpiresAt, present := got.ReferenceExpiresAt()
	if !present || !gotExpiresAt.Equal(expiresAt) {
		t.Errorf("ReferenceExpiresAt() = (%s, %t); want (%s, true)", gotExpiresAt, present, expiresAt)
	}
	if !got.UpdatedAt().Equal(currentNextAttemptAt) {
		t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), currentNextAttemptAt)
	}
	assertWagerTransactionOutcomeAbsent(t, got)

	if original.ReferenceAttempts() != 1 {
		t.Errorf("RetryPendingReference() mutated original attempts: got %d; want 1", original.ReferenceAttempts())
	}
	originalNextAttemptAt, present := original.NextReferenceAttemptAt()
	if !present || !originalNextAttemptAt.Equal(currentNextAttemptAt) {
		t.Errorf("RetryPendingReference() mutated original schedule: got (%s, %t); want (%s, true)", originalNextAttemptAt, present, currentNextAttemptAt)
	}
}

func TestWagerTransactionRetryPendingReferenceRejectsInvalidInput(t *testing.T) {
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	dueAt, _ := pendingReference.NextReferenceAttemptAt()
	expiresAt, _ := pendingReference.ReferenceExpiresAt()
	validInput := RetryPendingReferenceWagerTransactionInput{
		OccurredAt:    dueAt,
		NextAttemptAt: dueAt.Add(time.Second),
	}

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	processed, err := pending.Process(ProcessWagerTransactionInput{
		ResultBalance: mustNewMoney(t, "75.00", "BRL"),
		OccurredAt:    pending.UpdatedAt().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}
	rejected, err := pendingReference.Reject(RejectWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodeReferenceNotFound,
		OccurredAt:  dueAt,
	})
	if err != nil {
		t.Fatalf("Reject() returned an unexpected error: %v", err)
	}
	failed, err := pendingReference.Fail(FailWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure,
		OccurredAt:  dueAt,
	})
	if err != nil {
		t.Fatalf("Fail() returned an unexpected error: %v", err)
	}

	missingAttempts := pendingReference
	missingAttempts.referenceAttempts = 0
	missingNextAttempt := pendingReference
	missingNextAttempt.nextReferenceAttemptAt = time.Time{}
	missingExpiration := pendingReference
	missingExpiration.referenceExpiresAt = time.Time{}
	invalidCurrentNextAttempt := pendingReference
	invalidCurrentNextAttempt.nextReferenceAttemptAt = invalidCurrentNextAttempt.UpdatedAt()
	invalidCurrentExpiration := pendingReference
	invalidCurrentExpiration.referenceExpiresAt = invalidCurrentExpiration.nextReferenceAttemptAt
	exhaustedAttempts := pendingReference
	exhaustedAttempts.referenceAttempts = math.MaxInt32

	tests := []struct {
		name      string
		state     WagerTransaction
		input     RetryPendingReferenceWagerTransactionInput
		wantError error
	}{
		{name: "uninitialized transaction", state: WagerTransaction{}, input: validInput, wantError: ErrInvalidWagerTransaction},
		{name: "pending transaction", state: pending, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "processed transaction", state: processed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "rejected transaction", state: rejected, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "failed transaction", state: failed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "missing attempt history", state: missingAttempts, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "missing current schedule", state: missingNextAttempt, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "missing expiration", state: missingExpiration, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "current next attempt is not after update", state: invalidCurrentNextAttempt, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "current expiration is not after next attempt", state: invalidCurrentExpiration, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "attempt counter overflow", state: exhaustedAttempts, input: validInput, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "zero occurrence time", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{NextAttemptAt: dueAt.Add(time.Second)}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "backdated occurrence time", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: pendingReference.UpdatedAt().Add(-time.Nanosecond), NextAttemptAt: dueAt.Add(time.Second)}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "retry before scheduled time", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: dueAt.Add(-time.Nanosecond), NextAttemptAt: dueAt.Add(time.Second)}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "retry at expiration", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: expiresAt, NextAttemptAt: expiresAt.Add(time.Second)}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "zero next attempt", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: dueAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "next attempt equals occurrence", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: dueAt, NextAttemptAt: dueAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "next attempt equals expiration", state: pendingReference, input: RetryPendingReferenceWagerTransactionInput{OccurredAt: dueAt, NextAttemptAt: expiresAt}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.state

			got, err := tt.state.RetryPendingReference(tt.input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("RetryPendingReference() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "retry pending wager transaction reference", tt.wantError.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("RetryPendingReference() transaction = %+v; want zero value on error", got)
			}
			if tt.state != original {
				t.Errorf("RetryPendingReference() mutated original transaction: got %+v; want %+v", tt.state, original)
			}
		})
	}
}

func assertPendingReferenceMetadataAbsent(t *testing.T, transaction WagerTransaction) {
	t.Helper()

	if transaction.ReferenceAttempts() != 0 {
		t.Errorf("ReferenceAttempts() = %d; want 0", transaction.ReferenceAttempts())
	}
	if nextAttemptAt, present := transaction.NextReferenceAttemptAt(); present || !nextAttemptAt.IsZero() {
		t.Errorf("NextReferenceAttemptAt() = (%s, %t); want zero and absent", nextAttemptAt, present)
	}
	if expiresAt, present := transaction.ReferenceExpiresAt(); present || !expiresAt.IsZero() {
		t.Errorf("ReferenceExpiresAt() = (%s, %t); want zero and absent", expiresAt, present)
	}
}
