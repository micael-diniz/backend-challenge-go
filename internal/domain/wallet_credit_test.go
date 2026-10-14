package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestWalletCredit(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	creditedAt := createdAt.Add(30 * time.Minute)

	tests := []struct {
		name        string
		initial     string
		credit      string
		currency    string
		occurredAt  time.Time
		wantBalance string
	}{
		{name: "positive BRL credit", initial: "100.00", credit: "25.50", currency: "BRL", occurredAt: creditedAt, wantBalance: "125.50"},
		{name: "credit from zero", initial: "0.00", credit: "0.01", currency: "BRL", occurredAt: creditedAt, wantBalance: "0.01"},
		{name: "USD credit", initial: "10.00", credit: "2.50", currency: "USD", occurredAt: creditedAt, wantBalance: "12.50"},
		{name: "equal timestamp", initial: "10.00", credit: "1.00", currency: "BRL", occurredAt: createdAt, wantBalance: "11.00"},
		{name: "maximum balance boundary", initial: "92233720368547758.06", credit: "0.01", currency: "BRL", occurredAt: creditedAt, wantBalance: "92233720368547758.07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet := mustNewWallet(t, "wallet-1", "player-1", tt.initial, tt.currency, createdAt)
			amount := mustNewMoney(t, tt.credit, tt.currency)
			before := wallet

			got, err := wallet.Credit(amount, tt.occurredAt)
			if err != nil {
				t.Fatalf("Credit() returned an unexpected error: %v", err)
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

func TestWalletCreditIncrementsRehydratedVersion(t *testing.T) {
	createdAt := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	creditedAt := updatedAt.Add(time.Minute)
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
	got, err := wallet.Credit(mustNewMoney(t, "25.00", "BRL"), creditedAt)
	if err != nil {
		t.Fatalf("Credit() returned an unexpected error: %v", err)
	}

	assertWalletState(
		t,
		got,
		WalletID("wallet-1"),
		PlayerID("player-1"),
		"125.00",
		CurrencyBRL,
		9,
		createdAt,
		creditedAt,
	)
}

func TestWalletCreditAllowsTimestampEqualToLastUpdate(t *testing.T) {
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

	got, err := wallet.Credit(mustNewMoney(t, "1.00", "BRL"), updatedAt)
	if err != nil {
		t.Fatalf("Credit() returned an unexpected error: %v", err)
	}

	assertWalletState(
		t,
		got,
		WalletID("wallet-1"),
		PlayerID("player-1"),
		"101.00",
		CurrencyBRL,
		9,
		createdAt,
		updatedAt,
	)
}

func TestWalletCreditRejectsInvalidOperation(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	creditedAt := createdAt.Add(time.Minute)
	validWallet := mustNewWallet(t, "wallet-1", "player-1", "100.00", "BRL", createdAt)
	maxBalanceWallet := mustNewWallet(t, "wallet-2", "player-2", "92233720368547758.07", "BRL", createdAt)
	maxVersionWallet, err := RehydrateWallet(
		WalletID("wallet-3"),
		PlayerID("player-3"),
		mustNewMoney(t, "100.00", "BRL"),
		math.MaxInt64,
		createdAt,
		createdAt,
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}
	historicalWallet, err := RehydrateWallet(
		WalletID("wallet-4"),
		PlayerID("player-4"),
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
		{name: "uninitialized wallet", wallet: Wallet{}, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: creditedAt, wantError: ErrInvalidWallet},
		{name: "inconsistent wallet currency", wallet: inconsistentCurrencyWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: creditedAt, wantError: ErrInvalidWallet},
		{name: "uninitialized amount", wallet: validWallet, amount: Money{}, occurredAt: creditedAt, wantError: ErrInvalidMoney, unwantedError: ErrInvalidWallet},
		{name: "zero amount", wallet: validWallet, amount: mustNewMoney(t, "0.00", "BRL"), occurredAt: creditedAt, wantError: ErrNonPositiveWalletMovement, unwantedError: ErrInvalidWallet, messageFragments: []string{"amount=0.00 BRL"}},
		{name: "negative amount", wallet: validWallet, amount: mustNewMoney(t, "-0.01", "BRL"), occurredAt: creditedAt, wantError: ErrNonPositiveWalletMovement, unwantedError: ErrInvalidWallet, messageFragments: []string{"amount=-0.01 BRL"}},
		{name: "currency mismatch", wallet: validWallet, amount: mustNewMoney(t, "1.00", "USD"), occurredAt: creditedAt, wantError: ErrCurrencyMismatch, unwantedError: ErrInvalidWallet, messageFragments: []string{"left=100.00 BRL", "right=1.00 USD"}},
		{name: "zero timestamp", wallet: validWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: time.Time{}, wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"creditedAt cannot be zero"}},
		{name: "timestamp before creation", wallet: validWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: createdAt.Add(-time.Nanosecond), wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"precedes updatedAt"}},
		{name: "timestamp after creation but before last update", wallet: historicalWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: createdAt.Add(time.Hour), wantError: ErrInvalidWalletTimestamp, unwantedError: ErrInvalidWallet, messageFragments: []string{"precedes updatedAt"}},
		{name: "balance overflow", wallet: maxBalanceWallet, amount: mustNewMoney(t, "0.01", "BRL"), occurredAt: creditedAt, wantError: ErrAmountOutOfRange, unwantedError: ErrInvalidWallet, messageFragments: []string{"left=92233720368547758.07 BRL", "right=0.01 BRL"}},
		{name: "version overflow", wallet: maxVersionWallet, amount: mustNewMoney(t, "1.00", "BRL"), occurredAt: creditedAt, wantError: ErrInvalidWalletVersion, unwantedError: ErrInvalidWallet, messageFragments: []string{"version=9223372036854775807"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.wallet

			_, err := tt.wallet.Credit(tt.amount, tt.occurredAt)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Credit() error = %v; want %v", err, tt.wantError)
			}
			if tt.unwantedError != nil && errors.Is(err, tt.unwantedError) {
				t.Errorf("Credit() error = %v; must not also classify as %v", err, tt.unwantedError)
			}
			assertErrorContains(t, err, "credit wallet")
			assertErrorContains(t, err, tt.messageFragments...)
			assertWalletUnchanged(t, tt.wallet, before)
		})
	}
}

func mustNewWallet(
	t *testing.T,
	id string,
	playerID string,
	amount string,
	currency string,
	createdAt time.Time,
) Wallet {
	t.Helper()

	wallet, err := NewWallet(
		WalletID(id),
		PlayerID(playerID),
		mustNewMoney(t, amount, currency),
		createdAt,
	)
	if err != nil {
		t.Fatalf("NewWallet() returned an unexpected error: %v", err)
	}

	return wallet
}

func assertWalletUnchanged(t *testing.T, got Wallet, want Wallet) {
	t.Helper()

	assertWalletState(
		t,
		got,
		want.ID(),
		want.PlayerID(),
		want.Balance().Amount(),
		want.Currency(),
		want.Version(),
		want.CreatedAt(),
		want.UpdatedAt(),
	)
}
