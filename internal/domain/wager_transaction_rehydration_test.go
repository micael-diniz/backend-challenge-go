package domain

import (
	"errors"
	"testing"
	"time"
)

func TestRehydrateWagerTransactionPreservesValidPersistedStates(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	processed := mustProcessWagerTransaction(t, pending)
	processedReference := mustProcessWagerTransaction(t, pendingReference)
	rejected := mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds)
	rejectedReference := mustRejectWagerTransaction(t, pendingReference, WagerTransactionFailureCodeReferenceNotFound)
	failed := mustFailWagerTransaction(t, pending)
	failedReference := mustFailWagerTransaction(t, pendingReference)
	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}

	tests := []struct {
		name string
		want WagerTransaction
	}{
		{name: "internal opening", want: opening},
		{name: "external pending", want: pending},
		{name: "external pending reference", want: pendingReference},
		{name: "external processed directly", want: processed},
		{name: "external processed after pending reference", want: processedReference},
		{name: "external rejected directly", want: rejected},
		{name: "external rejected after pending reference", want: rejectedReference},
		{name: "external failed directly", want: failed},
		{name: "external failed after pending reference", want: failedReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := rehydrateInputFromWagerTransaction(tt.want)

			got, err := RehydrateWagerTransaction(input)
			if err != nil {
				t.Fatalf("RehydrateWagerTransaction() returned an unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("RehydrateWagerTransaction() = %+v; want exact persisted state %+v", got, tt.want)
			}
			if input != rehydrateInputFromWagerTransaction(tt.want) {
				t.Error("RehydrateWagerTransaction() mutated its input")
			}
		})
	}
}

func TestRehydrateWagerTransactionRejectsInvalidOriginShape(t *testing.T) {
	t.Parallel()

	opening, err := NewOpeningWagerTransaction(validOpeningWagerTransactionInput(t))
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}
	validOpening := rehydrateInputFromWagerTransaction(opening)
	validExternal := rehydrateInputFromWagerTransaction(
		mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {}),
	)

	tests := []struct {
		name      string
		input     RehydrateWagerTransactionInput
		mutate    func(*RehydrateWagerTransactionInput)
		wantError error
	}{
		{name: "opening with provider ID", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ProviderID = "provider-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with external transaction ID", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ExternalTransactionID = "external-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with idempotency key", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.IdempotencyKey = "key-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with payload hash", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.PayloadHash = "hash-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with round ID", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.RoundID = "round-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with game ID", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.GameID = "game-1" }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with external reference", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceExternalTransactionID = "external-reference-1"
		}, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "opening with resolved reference", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ResolvedReferenceTransactionID = "transaction-2" }, wantError: ErrInvalidWagerTransactionReference},
		{name: "opening with result balance", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = mustNewMoney(t, "100.00", "BRL") }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "opening with failure code", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) {
			input.FailureCode = WagerTransactionFailureCodeInsufficientFunds
		}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "opening with reference attempts", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceAttempts = 1 }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "opening with next reference attempt", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) {
			input.NextReferenceAttemptAt = input.UpdatedAt.Add(time.Second)
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "opening with reference expiration", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceExpiresAt = input.UpdatedAt.Add(time.Hour) }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "opening with changed updated time", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.UpdatedAt = input.CreatedAt.Add(time.Second) }, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "opening with pending status", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.Status = WagerTransactionStatusPending }, wantError: ErrInvalidWagerTransactionStatus},
		{name: "opening with blank transaction ID", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.ID = " " }, wantError: ErrInvalidTransactionID},
		{name: "opening with uninitialized money", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.Money = Money{} }, wantError: ErrInvalidMoney},
		{name: "opening with zero creation time", input: validOpening, mutate: func(input *RehydrateWagerTransactionInput) { input.CreatedAt = time.Time{} }, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "external transaction missing provider ID", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) { input.ProviderID = "" }, wantError: ErrInvalidProviderID},
		{name: "external transaction with opening kind", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) { input.Kind = WagerTransactionKindOpening }, wantError: ErrInvalidWagerTransactionOrigin},
		{name: "external transaction with unsupported kind", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) { input.Kind = "UNKNOWN" }, wantError: ErrInvalidWagerTransactionKind},
		{name: "external transaction with invalid amount", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) { input.Money = mustNewMoney(t, "0.00", "BRL") }, wantError: ErrInvalidWagerTransactionAmount},
		{name: "external bet with reference", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceExternalTransactionID = "external-reference-1"
		}, wantError: ErrInvalidWagerTransactionReference},
		{name: "external transaction with zero creation time", input: validExternal, mutate: func(input *RehydrateWagerTransactionInput) { input.CreatedAt = time.Time{} }, wantError: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			tt.mutate(&input)

			got, err := RehydrateWagerTransaction(input)
			assertInvalidWagerTransactionRehydration(t, got, err, tt.wantError)
		})
	}
}

func TestRehydrateWagerTransactionRejectsInvalidPersistedStateShape(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	pendingReference := mustNewPendingReferenceWagerTransaction(t)
	processed := mustProcessWagerTransaction(t, pending)
	processedReference := mustProcessWagerTransaction(t, pendingReference)
	rejected := mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds)
	failed := mustFailWagerTransaction(t, pending)

	tests := []struct {
		name      string
		input     RehydrateWagerTransactionInput
		mutate    func(*RehydrateWagerTransactionInput)
		wantError error
	}{
		{name: "unsupported status", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) { input.Status = "UNKNOWN" }, wantError: ErrInvalidWagerTransactionStatus},
		{name: "pending updated after creation", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) { input.UpdatedAt = input.CreatedAt.Add(time.Second) }, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "processed with zero updated time", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) { input.UpdatedAt = time.Time{} }, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "processed updated before creation", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) { input.UpdatedAt = input.CreatedAt.Add(-time.Nanosecond) }, wantError: ErrInvalidWagerTransactionTimestamp},
		{name: "pending with resolved reference", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) { input.ResolvedReferenceTransactionID = "transaction-2" }, wantError: ErrInvalidWagerTransactionReference},
		{name: "pending with result balance", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = mustNewMoney(t, "75.00", "BRL") }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "pending with failure code", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) {
			input.FailureCode = WagerTransactionFailureCodeInsufficientFunds
		}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "pending with reference history", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceAttempts = 1
			input.ReferenceExpiresAt = input.UpdatedAt.Add(time.Hour)
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "pending reference with zero attempts", input: rehydrateInputFromWagerTransaction(pendingReference), mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceAttempts = 0 }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "pending reference without next attempt", input: rehydrateInputFromWagerTransaction(pendingReference), mutate: func(input *RehydrateWagerTransactionInput) { input.NextReferenceAttemptAt = time.Time{} }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "pending reference without expiration", input: rehydrateInputFromWagerTransaction(pendingReference), mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceExpiresAt = time.Time{} }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "pending reference with result balance", input: rehydrateInputFromWagerTransaction(pendingReference), mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = mustNewMoney(t, "100.00", "BRL") }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "pending reference for ineligible kind", input: rehydrateInputFromWagerTransaction(pending), mutate: func(input *RehydrateWagerTransactionInput) {
			input.Status = WagerTransactionStatusPendingReference
			input.ReferenceAttempts = 1
			input.NextReferenceAttemptAt = input.UpdatedAt.Add(time.Second)
			input.ReferenceExpiresAt = input.UpdatedAt.Add(time.Hour)
		}, wantError: ErrInvalidWagerTransactionReference},
		{name: "processed without result balance", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = Money{} }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "processed with negative result balance", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = mustNewMoney(t, "-0.01", "BRL") }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "processed with failure code", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) {
			input.FailureCode = WagerTransactionFailureCodeInsufficientFunds
		}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "processed bet with resolved reference", input: rehydrateInputFromWagerTransaction(processed), mutate: func(input *RehydrateWagerTransactionInput) { input.ResolvedReferenceTransactionID = "transaction-2" }, wantError: ErrInvalidWagerTransactionReference},
		{name: "processed with next reference attempt", input: rehydrateInputFromWagerTransaction(processedReference), mutate: func(input *RehydrateWagerTransactionInput) {
			input.NextReferenceAttemptAt = input.UpdatedAt.Add(time.Second)
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "processed with incomplete reference history", input: rehydrateInputFromWagerTransaction(processedReference), mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceExpiresAt = time.Time{} }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "processed reference at expiration", input: rehydrateInputFromWagerTransaction(processedReference), mutate: func(input *RehydrateWagerTransactionInput) { input.UpdatedAt = input.ReferenceExpiresAt }, wantError: ErrWagerTransactionReferenceExpired},
		{name: "rejected without failure code", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) { input.FailureCode = "" }, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "rejected with infrastructure failure code", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) {
			input.FailureCode = WagerTransactionFailureCodePermanentInfrastructureFailure
		}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "rejected with result balance", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) { input.ResultBalance = mustNewMoney(t, "75.00", "BRL") }, wantError: ErrInvalidWagerTransactionResultBalance},
		{name: "failed without failure code", input: rehydrateInputFromWagerTransaction(failed), mutate: func(input *RehydrateWagerTransactionInput) { input.FailureCode = "" }, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "failed with business failure code", input: rehydrateInputFromWagerTransaction(failed), mutate: func(input *RehydrateWagerTransactionInput) {
			input.FailureCode = WagerTransactionFailureCodeInsufficientFunds
		}, wantError: ErrInvalidWagerTransactionFailureCode},
		{name: "terminal state with resolved reference", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) { input.ResolvedReferenceTransactionID = "transaction-2" }, wantError: ErrInvalidWagerTransactionReference},
		{name: "terminal state with next attempt", input: rehydrateInputFromWagerTransaction(failed), mutate: func(input *RehydrateWagerTransactionInput) {
			input.NextReferenceAttemptAt = input.UpdatedAt.Add(time.Second)
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "reference history without attempts", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) { input.ReferenceExpiresAt = input.CreatedAt.Add(time.Hour) }, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "reference history with negative attempts", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceAttempts = -1
			input.ReferenceExpiresAt = input.CreatedAt.Add(time.Hour)
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "reference history expiration not after creation", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceAttempts = 1
			input.ReferenceExpiresAt = input.CreatedAt
		}, wantError: ErrInvalidWagerTransactionReferenceSchedule},
		{name: "ineligible kind with reference history", input: rehydrateInputFromWagerTransaction(rejected), mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceAttempts = 1
			input.ReferenceExpiresAt = input.CreatedAt.Add(time.Hour)
		}, wantError: ErrInvalidWagerTransactionReference},
		{name: "reference history without external reference", input: rehydrateInputFromWagerTransaction(mustRejectedWinWithoutReference(t)), mutate: func(input *RehydrateWagerTransactionInput) {
			input.ReferenceAttempts = 1
			input.ReferenceExpiresAt = input.CreatedAt.Add(time.Hour)
		}, wantError: ErrInvalidWagerTransactionReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			tt.mutate(&input)

			got, err := RehydrateWagerTransaction(input)
			assertInvalidWagerTransactionRehydration(t, got, err, tt.wantError)
		})
	}
}

func TestRehydrateWagerTransactionPreservesNestedResultBalanceError(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	input := rehydrateInputFromWagerTransaction(mustProcessWagerTransaction(t, pending))
	input.ResultBalance = mustNewMoney(t, "75.00", "USD")

	got, err := RehydrateWagerTransaction(input)
	for _, wantError := range []error{
		ErrInvalidWagerTransaction,
		ErrInvalidWagerTransactionResultBalance,
		ErrCurrencyMismatch,
	} {
		if !errors.Is(err, wantError) {
			t.Errorf("RehydrateWagerTransaction() error = %v; want %v", err, wantError)
		}
	}
	if got != (WagerTransaction{}) {
		t.Errorf("RehydrateWagerTransaction() transaction = %+v; want zero value on error", got)
	}
}

func TestRehydratedTerminalWagerTransactionCannotTransition(t *testing.T) {
	t.Parallel()

	pending := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})
	terminalStates := []WagerTransaction{
		mustProcessWagerTransaction(t, pending),
		mustRejectWagerTransaction(t, pending, WagerTransactionFailureCodeInsufficientFunds),
		mustFailWagerTransaction(t, pending),
	}

	for _, terminal := range terminalStates {
		terminal := terminal
		t.Run(terminal.Status().String(), func(t *testing.T) {
			rehydrated, err := RehydrateWagerTransaction(rehydrateInputFromWagerTransaction(terminal))
			if err != nil {
				t.Fatalf("RehydrateWagerTransaction() returned an unexpected error: %v", err)
			}

			got, err := rehydrated.Process(ProcessWagerTransactionInput{
				ResultBalance: mustNewMoney(t, "75.00", "BRL"),
				OccurredAt:    rehydrated.UpdatedAt().Add(time.Second),
			})
			if !errors.Is(err, ErrInvalidWagerTransactionStatusTransition) {
				t.Fatalf("Process() error = %v; want ErrInvalidWagerTransactionStatusTransition", err)
			}
			if got != (WagerTransaction{}) {
				t.Errorf("Process() transaction = %+v; want zero value on error", got)
			}
		})
	}
}

func rehydrateInputFromWagerTransaction(transaction WagerTransaction) RehydrateWagerTransactionInput {
	return RehydrateWagerTransactionInput{
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
		ResolvedReferenceTransactionID: transaction.resolvedReferenceTransactionID,
		Status:                         transaction.status,
		FailureCode:                    transaction.failureCode,
		ResultBalance:                  transaction.resultBalance,
		ReferenceAttempts:              transaction.referenceAttempts,
		NextReferenceAttemptAt:         transaction.nextReferenceAttemptAt,
		ReferenceExpiresAt:             transaction.referenceExpiresAt,
		CreatedAt:                      transaction.createdAt,
		UpdatedAt:                      transaction.updatedAt,
	}
}

func mustProcessWagerTransaction(t *testing.T, transaction WagerTransaction) WagerTransaction {
	t.Helper()

	input := ProcessWagerTransactionInput{
		ResultBalance: mustNewMoney(t, "75.00", "BRL"),
		OccurredAt:    transaction.UpdatedAt().Add(time.Second),
	}
	if _, present := transaction.ReferenceExternalTransactionID(); present {
		input.ResolvedReferenceTransactionID = TransactionID("referenced-transaction-1")
	}

	processed, err := transaction.Process(input)
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}
	return processed
}

func mustRejectWagerTransaction(
	t *testing.T,
	transaction WagerTransaction,
	failureCode WagerTransactionFailureCode,
) WagerTransaction {
	t.Helper()

	rejected, err := transaction.Reject(RejectWagerTransactionInput{
		FailureCode: failureCode,
		OccurredAt:  transaction.UpdatedAt().Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Reject() returned an unexpected error: %v", err)
	}
	return rejected
}

func mustFailWagerTransaction(t *testing.T, transaction WagerTransaction) WagerTransaction {
	t.Helper()

	failed, err := transaction.Fail(FailWagerTransactionInput{
		FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure,
		OccurredAt:  transaction.UpdatedAt().Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Fail() returned an unexpected error: %v", err)
	}
	return failed
}

func mustRejectedWinWithoutReference(t *testing.T) WagerTransaction {
	t.Helper()

	transaction := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {
		input.Kind = WagerTransactionKindWin
	})
	return mustRejectWagerTransaction(
		t,
		transaction,
		WagerTransactionFailureCodeInvalidReference,
	)
}

func assertInvalidWagerTransactionRehydration(
	t *testing.T,
	got WagerTransaction,
	err error,
	wantError error,
) {
	t.Helper()

	if !errors.Is(err, ErrInvalidWagerTransaction) {
		t.Fatalf("RehydrateWagerTransaction() error = %v; want ErrInvalidWagerTransaction", err)
	}
	if !errors.Is(err, wantError) {
		t.Errorf("RehydrateWagerTransaction() error = %v; want %v", err, wantError)
	}
	assertErrorContains(t, err, "rehydrate wager transaction", wantError.Error())
	if got != (WagerTransaction{}) {
		t.Errorf("RehydrateWagerTransaction() transaction = %+v; want zero value on error", got)
	}
}
