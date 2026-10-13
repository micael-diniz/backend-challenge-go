package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestWalletDebit(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	debitedAt := createdAt.Add(30 * time.Minute)

	tests := []struct {
		name        string
		initial     string
		debit       string
		currency    string
		occurredAt  time.Time
		wantBalance string
	}{
		{name: "positive BRL debit", initial: "100.00", debit: "25.50", currency: "BRL", occurredAt: debitedAt, wantBalance: "74.50"},
		{name: "exact balance reaches zero", initial: "25.50", debit: "25.50", currency: "BRL", occurredAt: debitedAt, wantBalance: "0.00"},
		{name: "minimum debit", initial: "0.01", debit: "0.01", currency: "BRL", occurredAt: debitedAt, wantBalance: "0.00"},
		{name: "USD debit", initial: "10.00", debit: "2.50", currency: "USD", occurredAt: debitedAt, wantBalance: "7.50"},
		{name: "equal timestamp", initial: "10.00", debit: "1.00", currency: "BRL", occurredAt: createdAt, wantBalance: "9.00"},
		{name: "maximum balance boundary", initial: "92233720368547758.07", debit: "0.01", currency: "BRL", occurredAt: debitedAt, wantBalance: "92233720368547758.06"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet := mustNewWallet(t, "wallet-1", "player-1", tt.initial, tt.currency, createdAt)
			amount := mustNewMoney(t, tt.debit, tt.currency)
			before := wallet

			got, err := wallet.Debit(amount, tt.occurredAt)
			if err != nil {
				t.Fatalf("Debit() returned an unexpected error: %v", err)
			}

			assertWalletState(
				t,
				got,
				WalletID("wallet-1"),
				PlayerID("player-1"),
				tt.wantBalance,
				Currency(tt.currency),
				2,
				createdAt,
				tt.occurredAt,
			)
			assertWalletUnchanged(t, wallet, before)
		})
	}
}

func TestWalletDebitIncrementsRehydratedVersionAndAllowsEqualTimestamp(t *testing.T) {
	createdAt := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(24 * time.Hour)
	wallet, err := RehydrateWallet(
		WalletID("wallet-1"),
		PlayerID("player-1"),
		mustNewMoney(t, "100.00", "BRL"),
		8,
		createdAt,
		updatedAt,
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}

	got, err := wallet.Debit(mustNewMoney(t, "25.00", "BRL"), updatedAt)
	if err != nil {
		t.Fatalf("Debit() returned an unexpected error: %v", err)
	}

	assertWalletState(
		t,
		got,
		WalletID("wallet-1"),
		PlayerID("player-1"),
		"75.00",
		CurrencyBRL,
		9,
		createdAt,
		updatedAt,
	)
}

func TestWalletDebitRejectsInvalidOperation(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	debitedAt := createdAt.Add(time.Minute)
	validWallet := mustNewWallet(t, "wallet-1", "player-1", "100.00", "BRL", createdAt)
	maxVersionWallet, err := RehydrateWallet(
		WalletID("wallet-2"),
		PlayerID("player-2"),
		mustNewMoney(t, "100.00", "BRL"),
		math.MaxInt64,
		createdAt,
		createdAt,
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}
	historicalWallet, err := RehydrateWallet(
		WalletID("wallet-3"),
		PlayerID("player-3"),
		mustNewMoney(t, "100.00", "BRL"),
		8,
		createdAt,
		createdAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}
	inconsistentCurrencyWallet := validWallet
	inconsistentCurrencyWallet.currency = CurrencyUSD

	tests := []struct {
		name             string
		wallet           Wallet
		amount           Money
		occurredAt       time.Time
		wantError        error
		unwantedError    error
		messageFragments []string
	}{
		{name: "uninitialized wallet", wallet: Wallet{}, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: debitedAt, wantError: ErrInvalidWallet},
		{name: "inconsistent wallet currency", wallet: inconsistentCurrencyWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: debitedAt, wantError: ErrInvalidWallet},
		{name: "uninitialized amount", wallet: validWallet, amount: Money{}, occurredAt: debitedAt, wantError: ErrInvalidMoney, unwantedError: ErrInvalidWallet},
		{name: "zero amount", wallet: validWallet, amount: mustNewMoney(t, "0.00", "BRL"), occurredAt: debitedAt, wantError: ErrNonPositiveWalletMovement, unwantedError: ErrInvalidWallet, messageFragments: []string{"amount=0.00 BRL"}},
		{name: "negative amount", wallet: validWallet, amount: mustNewMoney(t, "-0.01", "BRL"), occurredAt: debitedAt, wantError: ErrNonPositiveWalletMovement, unwantedError: ErrInvalidWallet, messageFragments: []string{"amount=-0.01 BRL"}},
		{name: "currency mismatch", wallet: validWallet, amount: mustNewMoney(t, "1.00", "USD"), occurredAt: debitedAt, wantError: ErrCurrencyMismatch, unwantedError: ErrInvalidWallet, messageFragments: []string{"left=100.00 BRL", "right=1.00 USD"}},
		{name: "insufficient funds from zero balance", wallet: mustNewWallet(t, "wallet-4", "player-4", "0.00", "BRL", createdAt), amount: mustNewMoney(t, "0.01", "BRL"), occurredAt: debitedAt, wantError: ErrInsufficientFunds, unwantedError: ErrInvalidWallet, messageFragments: []string{"balance=0.00 BRL", "amount=0.01 BRL"}},
		{name: "insufficient funds", wallet: validWallet, amount: mustNewMoney(t, "100.01", "BRL"), occurredAt: debitedAt, wantError: ErrInsufficientFunds, unwantedError: ErrInvalidWallet, messageFragments: []string{"balance=100.00 BRL", "amount=100.01 BRL"}},
		{name: "zero timestamp", wallet: validWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: time.Time{}, wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"debitedAt cannot be zero"}},
		{name: "timestamp before creation", wallet: validWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: createdAt.Add(-time.Nanosecond), wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"precedes updatedAt"}},
		{name: "timestamp after creation but before last update", wallet: historicalWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: createdAt.Add(time.Hour), wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"precedes updatedAt"}},
		{name: "version overflow", wallet: maxVersionWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: debitedAt, wantError: ErrInvalidWalletVersion, unwantedError: ErrInvalidWallet, messageFragments: []string{"version=9223372036854775807"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.wallet

			_, err := tt.wallet.Debit(tt.amount, tt.occurredAt)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Debit() error = %v; want %v", err, tt.wantError)
			}
			if tt.unwantedError != nil && errors.Is(err, tt.unwantedError) {
				t.Errorf("Debit() error = %v; must not also classify as %v", err, tt.unwantedError)
			}
			assertErrorContains(t, err, "debit wallet")
			assertErrorContains(t, err, tt.messageFragments...)
			assertWalletUnchanged(t, tt.wallet, before)
		})
	}
}
