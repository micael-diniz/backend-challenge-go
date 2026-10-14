package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func FuzzMoneyConstructionAndJSONRoundTrip(f *testing.F) {
	seeds := []struct {
		amount   string
		currency string
	}{
		{amount: "0.00", currency: "BRL"},
		{amount: "-0.00", currency: "usd"},
		{amount: "25.40", currency: "BRl"},
		{amount: "-10.05", currency: "USD"},
		{amount: "92233720368547758.07", currency: "BRL"},
		{amount: "-92233720368547758.08", currency: "USD"},
		{amount: "invalid", currency: "EUR"},
	}
	for _, seed := range seeds {
		f.Add(seed.amount, seed.currency)
	}

	f.Fuzz(func(t *testing.T, amount string, currency string) {
		original, err := NewMoney(amount, currency)
		if err != nil {
			return
		}

		encoded, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal(%s) returned an unexpected error: %v", original.formatForError(), err)
		}

		var decoded Money
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("json.Unmarshal(%s) returned an unexpected error: %v", encoded, err)
		}

		comparison, err := decoded.Compare(original)
		if err != nil {
			t.Fatalf("Compare() returned an unexpected error: %v", err)
		}
		if comparison != 0 {
			t.Fatalf("JSON round trip produced %s; want %s", decoded.formatForError(), original.formatForError())
		}
	})
}

func FuzzMoneyAddSubtractRoundTrip(f *testing.F) {
	seeds := []struct {
		left     string
		right    string
		currency string
	}{
		{left: "25.40", right: "10.35", currency: "BRL"},
		{left: "-25.40", right: "10.35", currency: "USD"},
		{left: "92233720368547758.07", right: "0.01", currency: "BRL"},
		{left: "-92233720368547758.08", right: "-0.01", currency: "USD"},
	}
	for _, seed := range seeds {
		f.Add(seed.left, seed.right, seed.currency)
	}

	f.Fuzz(func(t *testing.T, leftAmount string, rightAmount string, currency string) {
		left, leftErr := NewMoney(leftAmount, currency)
		right, rightErr := NewMoney(rightAmount, currency)
		if leftErr != nil || rightErr != nil {
			return
		}

		sum, err := left.Add(right)
		if err != nil {
			if !errors.Is(err, ErrAmountOutOfRange) {
				t.Fatalf("Add() error = %v; want ErrAmountOutOfRange", err)
			}
			return
		}

		restored, err := sum.Subtract(right)
		if err != nil {
			t.Fatalf("Subtract() returned an unexpected error after successful Add(): %v", err)
		}

		comparison, err := restored.Compare(left)
		if err != nil {
			t.Fatalf("Compare() returned an unexpected error: %v", err)
		}
		if comparison != 0 {
			t.Fatalf("(%s + %s) - %s = %s; want %s", left.Amount(), right.Amount(), right.Amount(), restored.Amount(), left.Amount())
		}
	})
}

func FuzzWalletCreditPreservesInvariants(f *testing.F) {
	seeds := []struct {
		initial  string
		credit   string
		currency string
	}{
		{initial: "0.00", credit: "0.01", currency: "BRL"},
		{initial: "100.00", credit: "25.50", currency: "USD"},
		{initial: "92233720368547758.06", credit: "0.01", currency: "BRL"},
		{initial: "92233720368547758.07", credit: "0.01", currency: "USD"},
	}
	for _, seed := range seeds {
		f.Add(seed.initial, seed.credit, seed.currency)
	}

	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	creditedAt := createdAt.Add(time.Minute)

	f.Fuzz(func(t *testing.T, initialAmount string, creditAmount string, currency string) {
		initial, initialErr := NewMoney(initialAmount, currency)
		credit, creditErr := NewMoney(creditAmount, currency)
		if initialErr != nil || creditErr != nil {
			return
		}

		zero, err := NewZeroMoney(currency)
		if err != nil {
			return
		}
		initialComparison, err := initial.Compare(zero)
		if err != nil || initialComparison < 0 {
			return
		}
		creditComparison, err := credit.Compare(zero)
		if err != nil || creditComparison <= 0 {
			return
		}

		wallet, err := NewWallet(WalletID("wallet-1"), PlayerID("player-1"), initial, createdAt)
		if err != nil {
			t.Fatalf("NewWallet() returned an unexpected error: %v", err)
		}
		before := wallet

		expectedBalance, expectedErr := initial.Add(credit)
		got, creditErr := wallet.Credit(credit, creditedAt)
		if expectedErr != nil {
			if !errors.Is(expectedErr, ErrAmountOutOfRange) || !errors.Is(creditErr, ErrAmountOutOfRange) {
				t.Fatalf("Credit() error = %v; expected balance error = %v", creditErr, expectedErr)
			}
			assertWalletUnchanged(t, wallet, before)
			return
		}
		if creditErr != nil {
			t.Fatalf("Credit() returned an unexpected error: %v", creditErr)
		}

		assertWalletState(
			t,
			got,
			WalletID("wallet-1"),
			PlayerID("player-1"),
			expectedBalance.Amount(),
			initial.Currency(),
			2,
			createdAt,
			creditedAt,
		)
		assertWalletUnchanged(t, wallet, before)
	})
}

func FuzzWalletDebitPreservesInvariants(f *testing.F) {
	seeds := []struct {
		initial  string
		debit    string
		currency string
	}{
		{initial: "0.00", debit: "0.01", currency: "BRL"},
		{initial: "25.50", debit: "25.50", currency: "USD"},
		{initial: "100.00", debit: "25.50", currency: "BRL"},
		{initial: "100.00", debit: "100.01", currency: "USD"},
		{initial: "92233720368547758.07", debit: "0.01", currency: "BRL"},
	}
	for _, seed := range seeds {
		f.Add(seed.initial, seed.debit, seed.currency)
	}

	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	debitedAt := createdAt.Add(time.Minute)

	f.Fuzz(func(t *testing.T, initialAmount string, debitAmount string, currency string) {
		initial, initialErr := NewMoney(initialAmount, currency)
		debit, debitErr := NewMoney(debitAmount, currency)
		if initialErr != nil || debitErr != nil {
			return
		}

		zero, err := NewZeroMoney(currency)
		if err != nil {
			return
		}
		initialComparison, err := initial.Compare(zero)
		if err != nil || initialComparison < 0 {
			return
		}
		debitComparison, err := debit.Compare(zero)
		if err != nil || debitComparison <= 0 {
			return
		}

		wallet, err := NewWallet(WalletID("wallet-1"), PlayerID("player-1"), initial, createdAt)
		if err != nil {
			t.Fatalf("NewWallet() returned an unexpected error: %v", err)
		}
		before := wallet

		expectedBalance, err := initial.Subtract(debit)
		if err != nil {
			t.Fatalf("Subtract() returned an unexpected error: %v", err)
		}
		expectedComparison, err := expectedBalance.Compare(zero)
		if err != nil {
			t.Fatalf("Compare() returned an unexpected error: %v", err)
		}

		got, debitErr := wallet.Debit(debit, debitedAt)
		if expectedComparison < 0 {
			if !errors.Is(debitErr, ErrInsufficientFunds) {
				t.Fatalf("Debit() error = %v; want ErrInsufficientFunds", debitErr)
			}
			assertWalletUnchanged(t, wallet, before)
			return
		}
		if debitErr != nil {
			t.Fatalf("Debit() returned an unexpected error: %v", debitErr)
		}

		assertWalletState(
			t,
			got,
			WalletID("wallet-1"),
			PlayerID("player-1"),
			expectedBalance.Amount(),
			initial.Currency(),
			2,
			createdAt,
			debitedAt,
		)
		assertWalletUnchanged(t, wallet, before)
	})
}

func FuzzWalletLedgerEntryPreservesBalanceEquation(f *testing.F) {
	seeds := []struct {
		balanceBefore string
		amount        string
		currency      string
		credit        bool
	}{
		{balanceBefore: "100.00", amount: "25.50", currency: "BRL", credit: true},
		{balanceBefore: "100.00", amount: "25.50", currency: "USD", credit: false},
		{balanceBefore: "0.01", amount: "0.01", currency: "BRL", credit: false},
		{balanceBefore: "92233720368547758.06", amount: "0.01", currency: "BRL", credit: true},
	}
	for _, seed := range seeds {
		f.Add(seed.balanceBefore, seed.amount, seed.currency, seed.credit)
	}

	createdAt := time.Date(2026, time.October, 13, 15, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, beforeAmount string, amountText string, currency string, credit bool) {
		balanceBefore, beforeErr := NewMoney(beforeAmount, currency)
		amount, amountErr := NewMoney(amountText, currency)
		if beforeErr != nil || amountErr != nil || balanceBefore.isNegative() || !amount.isPositive() {
			return
		}

		direction := LedgerDirectionDebit
		expectedBalance, err := balanceBefore.Subtract(amount)
		if credit {
			direction = LedgerDirectionCredit
			expectedBalance, err = balanceBefore.Add(amount)
		}
		if err != nil || expectedBalance.isNegative() {
			return
		}

		entry, err := NewWalletLedgerEntry(
			LedgerEntryID("ledger-1"),
			WalletID("wallet-1"),
			TransactionID("transaction-1"),
			direction,
			amount,
			balanceBefore,
			expectedBalance,
			createdAt,
		)
		if err != nil {
			t.Fatalf("NewWalletLedgerEntry() returned an unexpected error: %v", err)
		}

		assertWalletLedgerEntryState(
			t,
			entry,
			LedgerEntryID("ledger-1"),
			WalletID("wallet-1"),
			TransactionID("transaction-1"),
			direction,
			amount,
			balanceBefore,
			expectedBalance,
			createdAt,
		)

		minorUnit := mustNewMoney(t, "0.01", currency)
		wrongBalance, wrongBalanceErr := expectedBalance.Add(minorUnit)
		if wrongBalanceErr != nil {
			wrongBalance, wrongBalanceErr = expectedBalance.Subtract(minorUnit)
		}
		if wrongBalanceErr != nil || wrongBalance.isNegative() {
			return
		}

		_, err = NewWalletLedgerEntry(
			LedgerEntryID("ledger-2"),
			WalletID("wallet-1"),
			TransactionID("transaction-2"),
			direction,
			amount,
			balanceBefore,
			wrongBalance,
			createdAt,
		)
		if !errors.Is(err, ErrLedgerBalanceMismatch) {
			t.Fatalf("NewWalletLedgerEntry() error = %v; want ErrLedgerBalanceMismatch", err)
		}
	})
}
