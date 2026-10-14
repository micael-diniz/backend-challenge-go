package domain

import (
	"errors"
	"testing"
	"time"
)

func TestWagerTransactionTransitionValidationPreservesSpecificCause(t *testing.T) {
	valid := mustNewExternalWagerTransaction(t, func(input *ExternalWagerTransactionInput) {})

	tests := []struct {
		name      string
		mutate    func(*WagerTransaction)
		wantCause error
	}{
		{name: "empty transaction ID", mutate: func(transaction *WagerTransaction) { transaction.id = "" }, wantCause: ErrInvalidTransactionID},
		{name: "blank transaction ID", mutate: func(transaction *WagerTransaction) { transaction.id = " \t" }, wantCause: ErrInvalidTransactionID},
		{name: "empty wallet ID", mutate: func(transaction *WagerTransaction) { transaction.walletID = "" }, wantCause: ErrInvalidWalletID},
		{name: "blank player ID", mutate: func(transaction *WagerTransaction) { transaction.playerID = "\n" }, wantCause: ErrInvalidPlayerID},
		{name: "uninitialized money", mutate: func(transaction *WagerTransaction) { transaction.money = Money{} }, wantCause: ErrInvalidMoney},
		{name: "unsupported kind", mutate: func(transaction *WagerTransaction) { transaction.kind = "UNKNOWN" }, wantCause: ErrInvalidWagerTransactionKind},
		{name: "unsupported status", mutate: func(transaction *WagerTransaction) { transaction.status = "UNKNOWN" }, wantCause: ErrInvalidWagerTransactionStatus},
		{name: "zero creation time", mutate: func(transaction *WagerTransaction) { transaction.createdAt = time.Time{} }, wantCause: ErrInvalidWagerTransactionTimestamp},
		{name: "zero update time", mutate: func(transaction *WagerTransaction) { transaction.updatedAt = time.Time{} }, wantCause: ErrInvalidWagerTransactionTimestamp},
		{name: "update before creation", mutate: func(transaction *WagerTransaction) {
			transaction.updatedAt = transaction.createdAt.Add(-time.Nanosecond)
		}, wantCause: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := valid
			tt.mutate(&invalid)

			got, err := invalid.Process(ProcessWagerTransactionInput{
				ResultBalance: mustNewMoney(t, "75.00", "BRL"),
				OccurredAt:    valid.UpdatedAt().Add(time.Minute),
			})
			if !errors.Is(err, ErrInvalidWagerTransaction) {
				t.Fatalf("Process() error = %v; want ErrInvalidWagerTransaction", err)
			}
			if !errors.Is(err, tt.wantCause) {
				t.Errorf("Process() error = %v; want specific cause %v", err, tt.wantCause)
			}
			assertErrorContains(t, err, "process wager transaction", tt.wantCause.Error())
			if got != (WagerTransaction{}) {
				t.Errorf("Process() transaction = %+v; want zero value on error", got)
			}
		})
	}
}

func TestWagerTransactionOperationsPreserveValidationCause(t *testing.T) {
	invalid := WagerTransaction{}
	processBalance := mustNewMoney(t, "0.00", "BRL")
	now := time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC)

	operations := []struct {
		name string
		run  func() error
	}{
		{name: "process", run: func() error {
			_, err := invalid.Process(ProcessWagerTransactionInput{ResultBalance: processBalance, OccurredAt: now})
			return err
		}},
		{name: "mark pending reference", run: func() error {
			_, err := invalid.MarkPendingReference(PendingReferenceWagerTransactionInput{OccurredAt: now, NextAttemptAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour)})
			return err
		}},
		{name: "retry pending reference", run: func() error {
			_, err := invalid.RetryPendingReference(RetryPendingReferenceWagerTransactionInput{OccurredAt: now, NextAttemptAt: now.Add(time.Second)})
			return err
		}},
		{name: "reject", run: func() error {
			_, err := invalid.Reject(RejectWagerTransactionInput{FailureCode: WagerTransactionFailureCodeInsufficientFunds, OccurredAt: now})
			return err
		}},
		{name: "fail", run: func() error {
			_, err := invalid.Fail(FailWagerTransactionInput{FailureCode: WagerTransactionFailureCodePermanentInfrastructureFailure, OccurredAt: now})
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			if !errors.Is(err, ErrInvalidWagerTransaction) {
				t.Fatalf("operation error = %v; want ErrInvalidWagerTransaction", err)
			}
			if !errors.Is(err, ErrInvalidTransactionID) {
				t.Errorf("operation error = %v; want ErrInvalidTransactionID cause", err)
			}
		})
	}
}

func TestWalletOperationsPreserveInvalidStateCause(t *testing.T) {
	createdAt := time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC)
	wallet := mustNewWallet(t, "wallet-1", "player-1", "100.00", "BRL", createdAt)
	wallet.id = ""
	amount := mustNewMoney(t, "1.00", "BRL")

	operations := []struct {
		name string
		run  func() error
	}{
		{name: "credit", run: func() error {
			_, err := wallet.Credit(amount, createdAt.Add(time.Minute))
			return err
		}},
		{name: "debit", run: func() error {
			_, err := wallet.Debit(amount, createdAt.Add(time.Minute))
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			if !errors.Is(err, ErrInvalidWallet) {
				t.Fatalf("operation error = %v; want ErrInvalidWallet", err)
			}
			if !errors.Is(err, ErrInvalidWalletID) {
				t.Errorf("operation error = %v; want ErrInvalidWalletID cause", err)
			}
		})
	}
}
