package domain

import (
	"errors"
	"testing"
	"time"
)

func TestWagerTransactionFailureCodeString(t *testing.T) {
	tests := []struct {
		code WagerTransactionFailureCode
		want string
	}{
		{code: WagerTransactionFailureCodeInsufficientFunds, want: "INSUFFICIENT_FUNDS"},
		{code: WagerTransactionFailureCodeReversalInsufficientFunds, want: "REVERSAL_INSUFFICIENT_FUNDS"},
		{code: WagerTransactionFailureCodeReferenceNotFound, want: "REFERENCE_NOT_FOUND"},
		{code: WagerTransactionFailureCodeInvalidReference, want: "INVALID_REFERENCE"},
		{code: WagerTransactionFailureCodeDuplicateReversal, want: "DUPLICATE_REVERSAL"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.code.String(); got != tt.want {
				t.Errorf("String() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestWagerTransactionReject(t *testing.T) {
	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	pendingReference := mustNewPendingReferenceWagerTransaction(t)

	tests := []struct {
		name        string
		original    WagerTransaction
		failureCode WagerTransactionFailureCode
	}{
		{
			name:        "pending bet with insufficient funds",
			original:    pending,
			failureCode: WagerTransactionFailureCodeInsufficientFunds,
		},
		{
			name:        "pending reference expired",
			original:    pendingReference,
			failureCode: WagerTransactionFailureCodeReferenceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rejectedAt := tt.original.UpdatedAt().Add(time.Minute)

			got, err := tt.original.Reject(RejectWagerTransactionInput{
				FailureCode: tt.failureCode,
				OccurredAt:  rejectedAt,
			})
			if err != nil {
				t.Fatalf("Reject() returned an unexpected error: %v", err)
			}

			if got.Status() != WagerTransactionStatusRejected {
				t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusRejected)
			}
			gotFailureCode, present := got.FailureCode()
			if !present || gotFailureCode != tt.failureCode {
				t.Errorf("FailureCode() = (%q, %t); want (%q, true)", gotFailureCode, present, tt.failureCode)
			}
			if !got.UpdatedAt().Equal(rejectedAt) {
				t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), rejectedAt)
			}
			if !got.CreatedAt().Equal(tt.original.CreatedAt()) {
				t.Errorf("CreatedAt() = %s; want %s", got.CreatedAt(), tt.original.CreatedAt())
			}
			if _, present := got.ResultBalance(); present {
				t.Error("ResultBalance() is present; want absent for rejected transaction")
			}
			if _, present := got.ResolvedReferenceTransactionID(); present {
				t.Error("ResolvedReferenceTransactionID() is present; want absent for rejected transaction")
			}

			if tt.original.Status() == WagerTransactionStatusPendingReference {
				if got.ReferenceAttempts() != tt.original.ReferenceAttempts() {
					t.Errorf("ReferenceAttempts() = %d; want preserved %d", got.ReferenceAttempts(), tt.original.ReferenceAttempts())
				}
				if _, present := got.NextReferenceAttemptAt(); present {
					t.Error("NextReferenceAttemptAt() is present; want cleared after rejection")
				}
				gotExpiresAt, gotExpiration := got.ReferenceExpiresAt()
				wantExpiresAt, wantExpiration := tt.original.ReferenceExpiresAt()
				if gotExpiration != wantExpiration || !gotExpiresAt.Equal(wantExpiresAt) {
					t.Errorf("ReferenceExpiresAt() = (%s, %t); want (%s, %t)", gotExpiresAt, gotExpiration, wantExpiresAt, wantExpiration)
				}
			} else {
				assertPendingReferenceMetadataAbsent(t, got)
			}

			if tt.original.Status() == WagerTransactionStatusRejected {
				t.Fatal("test setup must not start rejected")
			}
			if _, present := tt.original.FailureCode(); present {
				t.Error("Reject() mutated original failure code")
			}
		})
	}
}

func TestWagerTransactionRejectRejectsInvalidInput(t *testing.T) {
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

	validInput := RejectWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodeInsufficientFunds,
		OccurredAt:  pending.UpdatedAt().Add(time.Minute),
	}

	tests := []struct {
		name      string
		state     WagerTransaction
		input     RejectWagerTransactionInput
		wantError error
	}{
		{name: "uninitialized transaction", state: WagerTransaction{}, input: validInput, wantError: ErrInvalidWagerTransaction},
		{name: "processed transaction", state: processed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "already rejected transaction", state: rejected, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "failed transaction", state: failed, input: validInput, wantError: ErrInvalidWagerTransactionStatusTransition},
		{name: "empty failure code", state: pending, input: RejectWagerTransactionInput{OccurredAt: validInput.OccurredAt}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "unknown failure code", state: pending, input: RejectWagerTransactionInput{FailureCode: WagerTransactionFailureCode("UNKNOWN"), OccurredAt: validInput.OccurredAt}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "lowercase failure code", state: pendingReference, input: RejectWagerTransactionInput{FailureCode: WagerTransactionFailureCode("reference_not_found"), OccurredAt: pendingReference.UpdatedAt().Add(time.Minute)}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "zero occurrence time", state: pending, input: RejectWagerTransactionInput{FailureCode: WagerTransactionFailureCodeInsufficientFunds}, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "backdated occurrence time", state: pending, input: RejectWagerTransactionInput{FailureCode: WagerTransactionFailureCodeInsufficientFunds, OccurredAt: pending.UpdatedAt().Add(-time.Nanosecond)}, wantError: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.state

			got, err := tt.state.Reject(tt.input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Reject() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "reject wager transaction", tt.wantError.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("Reject() transaction = %+v; want zero value on error", got)
			}
			if tt.state != original {
				t.Errorf("Reject() mutated original transaction: got %+v; want %+v", tt.state, original)
			}
		})
	}
}

func mustNewPendingReferenceWagerTransaction(t *testing.T) WagerTransaction {
	t.Helper()

	transaction := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindRefund
		input.ReferenceExternalTransactionID = ExternalTransactionID("missing-external-transaction-1")
	})
	occurredAt := transaction.UpdatedAt().Add(time.Minute)

	pending, err := transaction.MarkPendingReference(PendingReferenceWagerTransactionInput{
		OccurredAt:    occurredAt,
		NextAttemptAt: occurredAt.Add(5 * time.Second),
		ExpiresAt:     occurredAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("MarkPendingReference() returned an unexpected error: %v", err)
	}

	return pending
}
