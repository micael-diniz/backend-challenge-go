package domain

import (
	"errors"
	"testing"
	"time"
)

func TestWagerTransactionProcess(t *testing.T) {
	processedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                string
		kind                WagerTransactionKind
		money               Money
		externalReferenceID ExternalTransactionID
		resolvedReferenceID TransactionID
		resultBalance       Money
	}{
		{
			name:          "bet",
			kind:          WagerTransactionKindBet,
			money:         mustNewMoney(t, "25.00", "BRL"),
			resultBalance: mustNewMoney(t, "75.00", "BRL"),
		},
		{
			name:          "win without reference",
			kind:          WagerTransactionKindWin,
			money:         mustNewMoney(t, "40.00", "BRL"),
			resultBalance: mustNewMoney(t, "115.00", "BRL"),
		},
		{
			name:                "win with resolved reference",
			kind:                WagerTransactionKindWin,
			money:               mustNewMoney(t, "40.00", "BRL"),
			externalReferenceID: ExternalTransactionID("external-bet-1"),
			resolvedReferenceID: TransactionID("bet-transaction-1"),
			resultBalance:       mustNewMoney(t, "115.00", "BRL"),
		},
		{
			name:          "loss with unchanged balance",
			kind:          WagerTransactionKindLoss,
			money:         mustNewMoney(t, "0.00", "BRL"),
			resultBalance: mustNewMoney(t, "75.00", "BRL"),
		},
		{
			name:                "refund with resolved reference",
			kind:                WagerTransactionKindRefund,
			money:               mustNewMoney(t, "25.00", "BRL"),
			externalReferenceID: ExternalTransactionID("external-bet-1"),
			resolvedReferenceID: TransactionID("bet-transaction-1"),
			resultBalance:       mustNewMoney(t, "100.00", "BRL"),
		},
		{
			name:                "rollback with resolved reference",
			kind:                WagerTransactionKindRollback,
			money:               mustNewMoney(t, "25.00", "BRL"),
			externalReferenceID: ExternalTransactionID("external-win-1"),
			resolvedReferenceID: TransactionID("win-transaction-1"),
			resultBalance:       mustNewMoney(t, "75.00", "BRL"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			input.Kind = tt.kind
			input.Money = tt.money
			input.ReferenceExternalTransactionID = tt.externalReferenceID

			original, err := NewExternalWagerTransaction(input)
			if err != nil {
				t.Fatalf("NewExternalWagerTransaction() returned an unexpected error: %v", err)
			}

			got, err := original.Process(ProcessWagerTransactionInput{
				ResultBalance:                  tt.resultBalance,
				ResolvedReferenceTransactionID: tt.resolvedReferenceID,
				OccurredAt:                     processedAt,
			})
			if err != nil {
				t.Fatalf("Process() returned an unexpected error: %v", err)
			}

			if got.Status() != WagerTransactionStatusProcessed {
				t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusProcessed)
			}
			if !got.UpdatedAt().Equal(processedAt) {
				t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), processedAt)
			}
			if !got.CreatedAt().Equal(original.CreatedAt()) {
				t.Errorf("CreatedAt() = %s; want %s", got.CreatedAt(), original.CreatedAt())
			}

			gotBalance, present := got.ResultBalance()
			if !present {
				t.Fatal("ResultBalance() is absent; want present")
			}
			assertMoneyState(t, "ResultBalance()", gotBalance, tt.resultBalance)

			gotReferenceID, present := got.ResolvedReferenceTransactionID()
			if present != (tt.resolvedReferenceID != "") {
				t.Errorf("ResolvedReferenceTransactionID() present = %t; want %t", present, tt.resolvedReferenceID != "")
			}
			if gotReferenceID != tt.resolvedReferenceID {
				t.Errorf("ResolvedReferenceTransactionID() = %q; want %q", gotReferenceID, tt.resolvedReferenceID)
			}

			if _, present := got.FailureCode(); present {
				t.Error("FailureCode() is present; want absent for processed transaction")
			}

			if original.Status() != WagerTransactionStatusPending {
				t.Errorf("original Status() = %q; want %q", original.Status(), WagerTransactionStatusPending)
			}
			if !original.UpdatedAt().Equal(original.CreatedAt()) {
				t.Errorf("original UpdatedAt() = %s; want unchanged %s", original.UpdatedAt(), original.CreatedAt())
			}
			assertWagerTransactionOutcomeAbsent(t, original)
		})
	}
}

func TestWagerTransactionProcessPendingReference(t *testing.T) {
	original := mustNewPendingReferenceWagerTransaction(t)
	originalNextAttemptAt, present := original.NextReferenceAttemptAt()
	if !present {
		t.Fatal("test setup has no next reference attempt")
	}
	originalExpiresAt, present := original.ReferenceExpiresAt()
	if !present {
		t.Fatal("test setup has no reference expiration")
	}
	processedAt := original.UpdatedAt().Add(time.Second)
	resultBalance := mustNewMoney(t, "100.00", "BRL")
	resolvedReferenceID := TransactionID("bet-transaction-1")

	got, err := original.Process(ProcessWagerTransactionInput{
		ResultBalance:                  resultBalance,
		ResolvedReferenceTransactionID: resolvedReferenceID,
		OccurredAt:                     processedAt,
	})
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}

	if got.Status() != WagerTransactionStatusProcessed {
		t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusProcessed)
	}
	if got.ReferenceAttempts() != original.ReferenceAttempts() {
		t.Errorf("ReferenceAttempts() = %d; want preserved %d", got.ReferenceAttempts(), original.ReferenceAttempts())
	}
	if _, present := got.NextReferenceAttemptAt(); present {
		t.Error("NextReferenceAttemptAt() is present; want cleared after resolution")
	}
	gotExpiresAt, present := got.ReferenceExpiresAt()
	if !present || !gotExpiresAt.Equal(originalExpiresAt) {
		t.Errorf("ReferenceExpiresAt() = (%s, %t); want (%s, true)", gotExpiresAt, present, originalExpiresAt)
	}
	gotReferenceID, present := got.ResolvedReferenceTransactionID()
	if !present || gotReferenceID != resolvedReferenceID {
		t.Errorf("ResolvedReferenceTransactionID() = (%q, %t); want (%q, true)", gotReferenceID, present, resolvedReferenceID)
	}
	gotBalance, present := got.ResultBalance()
	if !present {
		t.Fatal("ResultBalance() is absent; want present")
	}
	assertMoneyState(t, "ResultBalance()", gotBalance, resultBalance)
	if !got.UpdatedAt().Equal(processedAt) {
		t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), processedAt)
	}

	if original.Status() != WagerTransactionStatusPendingReference {
		t.Errorf("Process() mutated original status: got %q; want %q", original.Status(), WagerTransactionStatusPendingReference)
	}
	gotOriginalNextAttemptAt, present := original.NextReferenceAttemptAt()
	if !present || !gotOriginalNextAttemptAt.Equal(originalNextAttemptAt) {
		t.Errorf("Process() mutated original schedule: got (%s, %t); want (%s, true)", gotOriginalNextAttemptAt, present, originalNextAttemptAt)
	}
	assertWagerTransactionOutcomeAbsent(t, original)
}

func TestWagerTransactionProcessRejectsInvalidInput(t *testing.T) {
	validTransaction := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	refundTransaction := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindRefund
		input.ReferenceExternalTransactionID = ExternalTransactionID("external-bet-1")
	})
	winWithoutReference := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindWin
	})
	winWithReference := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindWin
		input.ReferenceExternalTransactionID = ExternalTransactionID("external-bet-1")
	})
	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}
	invalidPendingReferenceTransaction := refundTransaction
	invalidPendingReferenceTransaction.status = WagerTransactionStatusPendingReference
	invalidPendingReferenceEligibility := mustNewPendingReferenceWagerTransaction(t)
	invalidPendingReferenceEligibility.referenceExternalTransactionID = ""
	rejectedTransaction := validTransaction
	rejectedTransaction.status = WagerTransactionStatusRejected
	failedTransaction := validTransaction
	failedTransaction.status = WagerTransactionStatusFailed
	invalidStatusTransaction := validTransaction
	invalidStatusTransaction.status = WagerTransactionStatus("UNKNOWN")

	validOccurredAt := validTransaction.UpdatedAt().Add(time.Minute)
	validBalance := mustNewMoney(t, "75.00", "BRL")
	validReferenceID := TransactionID("bet-transaction-1")

	tests := []struct {
		name      string
		state     WagerTransaction
		input     ProcessWagerTransactionInput
		wantError error
	}{
		{
			name:      "uninitialized transaction",
			state:     WagerTransaction{},
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransaction,
		},
		{
			name:      "transaction with unsupported status",
			state:     invalidStatusTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransaction,
		},
		{
			name:      "already processed transaction",
			state:     opening,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionStatusTransition,
		},
		{
			name:      "pending reference transaction without schedule",
			state:     invalidPendingReferenceTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: validReferenceID, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReferenceSchedule,
		},
		{
			name:      "pending reference transaction is no longer eligible",
			state:     invalidPendingReferenceEligibility,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: validReferenceID, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "rejected transaction",
			state:     rejectedTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionStatusTransition,
		},
		{
			name:      "failed transaction",
			state:     failedTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionStatusTransition,
		},
		{
			name:      "uninitialized result balance",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: Money{}, OccurredAt: validOccurredAt},
			wantError: ErrInvalidMoney,
		},
		{
			name:      "negative result balance",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: mustNewMoney(t, "-0.01", "BRL"), OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionResultBalance,
		},
		{
			name:      "result balance currency mismatch",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: mustNewMoney(t, "75.00", "USD"), OccurredAt: validOccurredAt},
			wantError: ErrCurrencyMismatch,
		},
		{
			name:      "zero occurrence time",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance},
			wantError: ErrInvalidWagerTransactionTimestamp,
		},
		{
			name:      "backdated occurrence time",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validTransaction.UpdatedAt().Add(-time.Nanosecond)},
			wantError: ErrInvalidWagerTransactionTimestamp,
		},
		{
			name:      "missing resolved refund reference",
			state:     refundTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "blank resolved refund reference",
			state:     refundTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: TransactionID(" \t"), OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "self resolved refund reference",
			state:     refundTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: refundTransaction.ID(), OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "unexpected resolved bet reference",
			state:     validTransaction,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: validReferenceID, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "unexpected resolved win reference",
			state:     winWithoutReference,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, ResolvedReferenceTransactionID: validReferenceID, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
		{
			name:      "missing resolved win reference",
			state:     winWithReference,
			input:     ProcessWagerTransactionInput{ResultBalance: validBalance, OccurredAt: validOccurredAt},
			wantError: ErrInvalidWagerTransactionReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.state

			got, err := tt.state.Process(tt.input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Process() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "process wager transaction", tt.wantError.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("Process() transaction = %+v; want zero value on error", got)
			}
			if tt.state != original {
				t.Errorf("Process() mutated original transaction: got %+v; want %+v", tt.state, original)
			}
		})
	}
}

func mustNewExternalWagerTransaction(
	t *testing.T,
	change func(*ExternalWagerTransactionInput),
) WagerTransaction {
	t.Helper()

	input := validExternalWagerTransactionInput(t)
	change(&input)

	transaction, err := NewExternalWagerTransaction(input)
	if err != nil {
		t.Fatalf("NewExternalWagerTransaction() returned an unexpected error: %v", err)
	}

	return transaction
}
