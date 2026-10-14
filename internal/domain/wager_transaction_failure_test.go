package domain

import (
	"errors"
	"testing"
	"time"
)

func TestWagerTransactionPermanentFailureCodeString(t *testing.T) {
	if got := WagerTransactionFailureCodePermanentInfrastructureFailure.String(); got != "PERMANENT_INFRASTRUCTURE_FAILURE" {
		t.Errorf("String() = %q; want %q", got, "PERMANENT_INFRASTRUCTURE_FAILURE")
	}
}

func TestWagerTransactionFail(t *testing.T) {
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	pendingReference := mustNewPendingReferenceWagerTransaction(t)

	tests := []struct {
		name     string
		original WagerTransaction
	}{
		{name: "pending transaction", original: pending},
		{name: "pending reference transaction", original: pendingReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.original
			failedAt := tt.original.UpdatedAt().Add(time.Minute)

			got, err := tt.original.Fail(FailWagerTransactionInput{
				FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure,
				OccurredAt:  failedAt,
			})
			if err != nil {
				t.Fatalf("Fail() returned an unexpected error: %v", err)
			}

			if got.Status() != WagerTransactionStatusFailed {
				t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusFailed)
			}
			gotFailureCode, present := got.FailureCode()
			if !present || gotFailureCode != WagerTransactionFailureCodePermanentInfrastructureFailure {
				t.Errorf(
					"FailureCode() = (%q, %t); want (%q, true)",
					gotFailureCode,
					present,
					WagerTransactionFailureCodePermanentInfrastructureFailure,
				)
			}
			if !got.UpdatedAt().Equal(failedAt) {
				t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), failedAt)
			}
			if !got.CreatedAt().Equal(tt.original.CreatedAt()) {
				t.Errorf("CreatedAt() = %s; want %s", got.CreatedAt(), tt.original.CreatedAt())
			}
			if _, present := got.ResultBalance(); present {
				t.Error("ResultBalance() is present; want absent for failed transaction")
			}
			if _, present := got.ResolvedReferenceTransactionID(); present {
				t.Error("ResolvedReferenceTransactionID() is present; want absent for failed transaction")
			}

			if tt.original.Status() == WagerTransactionStatusPendingReference {
				if got.ReferenceAttempts() != tt.original.ReferenceAttempts() {
					t.Errorf("ReferenceAttempts() = %d; want preserved %d", got.ReferenceAttempts(), tt.original.ReferenceAttempts())
				}
				if _, present := got.NextReferenceAttemptAt(); present {
					t.Error("NextReferenceAttemptAt() is present; want cleared after permanent failure")
				}
				gotExpiresAt, gotExpiration := got.ReferenceExpiresAt()
				wantExpiresAt, wantExpiration := tt.original.ReferenceExpiresAt()
				if gotExpiration != wantExpiration || !gotExpiresAt.Equal(wantExpiresAt) {
					t.Errorf("ReferenceExpiresAt() = (%s, %t); want (%s, %t)", gotExpiresAt, gotExpiration, wantExpiresAt, wantExpiration)
				}
			} else {
				assertPendingReferenceMetadataAbsent(t, got)
			}

			if tt.original != original {
				t.Errorf("Fail() mutated original transaction: got %+v; want %+v", tt.original, original)
			}
		})
	}
}

func TestWagerTransactionFailRejectsInvalidInput(t *testing.T) {
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	processed, err := pending.Process(ProcessWagerTransactionInput{
		ResultBalance: mustNewMoney(t, "75.00", "BRL"),
		OccurredAt:    pending.UpdatedAt().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}
	rejected, err := pending.Reject(RejectWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodeInsufficientFunds,
		OccurredAt:  pending.UpdatedAt().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Reject() returned an unexpected error: %v", err)
	}
	failed := pending
	failed.status = WagerTransactionStatusFailed

	validInput := FailWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure,
		OccurredAt:  pending.UpdatedAt().Add(time.Minute),
	}

	tests := []struct {
		name      string
		state     WagerTransaction
		input     FailWagerTransactionInput
		wantError error
	}{
		{name: "uninitialized transaction", state: WagerTransaction{}, input: validInput, wantError: ErrInvalidWagerTransaction},
		{name: "processed transaction", state: processed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "rejected transaction", state: rejected, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "already failed transaction", state: failed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "empty failure code", state: pending, input: FailWagerTransactionInput{OccurredAt: validInput.OccurredAt}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "business failure code", state: pending, input: FailWagerTransactionInput{FailureCode: WagerTransactionFailureCodeInsufficientFunds, OccurredAt: validInput.OccurredAt}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "unknown failure code", state: pendingReference, input: FailWagerTransactionInput{FailureCode: WagerTransactionFailureCode("UNKNOWN"), OccurredAt: pendingReference.UpdatedAt().Add(time.Minute)}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "zero occurrence time", state: pending, input: FailWagerTransactionInput{FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "backdated occurrence time", state: pending, input: FailWagerTransactionInput{FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure, OccurredAt: pending.UpdatedAt().Add(-time.Nanosecond)}, wantError: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.state

			got, err := tt.state.Fail(tt.input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Fail() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "fail wager transaction", tt.wantError.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("Fail() transaction = %+v; want zero value on error", got)
			}
			if tt.state != original {
				t.Errorf("Fail() mutated original transaction: got %+v; want %+v", tt.state, original)
			}
		})
	}
}

func TestRejectDoesNotAcceptPermanentInfrastructureFailure(t *testing.T) {
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})

	_, err := pending.Reject(RejectWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure,
		OccurredAt:  pending.UpdatedAt().Add(time.Minute),
	})
	if !errors.Is(err, ErrInvalidWagerTransactionFailureCode) {
		t.Fatalf("Reject() error = %v; want ErrInvalidWagerTransactionFailureCode", err)
	}
}
