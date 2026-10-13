package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewWallet(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name            string
		id              WalletID
		playerID        PlayerID
		initialBalance  Money
		wantAmount      string
		wantCurrency    Currency
		wantVersion     int64
		wantCreatedAt   time.Time
		wantLastUpdated time.Time
	}{
		{
			name:            "positive BRL balance",
			id:              WalletID("wallet-1"),
			playerID:        PlayerID("player-1"),
			initialBalance:  mustNewMoney(t, "1000.00", "BRL"),
			wantAmount:      "1000.00",
			wantCurrency:    CurrencyBRL,
			wantVersion:     1,
			wantCreatedAt:   createdAt,
			wantLastUpdated: createdAt,
		},
		{
			name:            "zero USD balance",
			id:              WalletID("wallet-2"),
			playerID:        PlayerID("player-2"),
			initialBalance:  mustNewMoney(t, "0.00", "USD"),
			wantAmount:      "0.00",
			wantCurrency:    CurrencyUSD,
			wantVersion:     1,
			wantCreatedAt:   createdAt,
			wantLastUpdated: createdAt,
		},
		{
			name:            "maximum BRL balance",
			id:              WalletID("wallet-3"),
			playerID:        PlayerID("player-3"),
			initialBalance:  mustNewMoney(t, "92233720368547758.07", "BRL"),
			wantAmount:      "92233720368547758.07",
			wantCurrency:    CurrencyBRL,
			wantVersion:     1,
			wantCreatedAt:   createdAt,
			wantLastUpdated: createdAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWallet(tt.id, tt.playerID, tt.initialBalance, createdAt)
			if err != nil {
				t.Fatalf("NewWallet() returned an unexpected error: %v", err)
			}

			assertWalletState(
				t,
				got,
				tt.id,
				tt.playerID,
				tt.wantAmount,
				tt.wantCurrency,
				tt.wantVersion,
				tt.wantCreatedAt,
				tt.wantLastUpdated,
			)
		})
	}
}

func TestNewWalletRejectsInvalidInput(t *testing.T) {
	validID := WalletID("wallet-1")
	validPlayerID := PlayerID("player-1")
	validBalance := mustNewMoney(t, "100.00", "BRL")
	validTime := time.Date(2026, time.October, 13, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name           string
		id             WalletID
		playerID       PlayerID
		initialBalance Money
		createdAt      time.Time
		wantError      error
	}{
		{
			name:           "empty wallet ID",
			id:             WalletID(""),
			playerID:       validPlayerID,
			initialBalance: validBalance,
			createdAt:      validTime,
			wantError:      ErrInvalidWalletID,
		},
		{
			name:           "empty player ID",
			id:             validID,
			playerID:       PlayerID(""),
			initialBalance: validBalance,
			createdAt:      validTime,
			wantError:      ErrInvalidPlayerID,
		},
		{
			name:           "uninitialized balance",
			id:             validID,
			playerID:       validPlayerID,
			initialBalance: Money{},
			createdAt:      validTime,
			wantError:      ErrInvalidMoney,
		},
		{
			name:           "negative balance",
			id:             validID,
			playerID:       validPlayerID,
			initialBalance: mustNewMoney(t, "-0.01", "BRL"),
			createdAt:      validTime,
			wantError:      ErrNegativeWalletBalance,
		},
		{
			name:           "zero creation time",
			id:             validID,
			playerID:       validPlayerID,
			initialBalance: validBalance,
			createdAt:      time.Time{},
			wantError:      ErrInvalidWalletTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWallet(tt.id, tt.playerID, tt.initialBalance, tt.createdAt)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("NewWallet() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "create wallet")
		})
	}
}

func TestRehydrateWallet(t *testing.T) {
	createdAt := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.October, 13, 17, 45, 0, 0, time.UTC)
	balance := mustNewMoney(t, "750.25", "BRL")

	got, err := RehydrateWallet(
		WalletID("wallet-1"),
		PlayerID("player-1"),
		balance,
		8,
		createdAt,
		updatedAt,
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() returned an unexpected error: %v", err)
	}

	assertWalletState(
		t,
		got,
		WalletID("wallet-1"),
		PlayerID("player-1"),
		"750.25",
		CurrencyBRL,
		8,
		createdAt,
		updatedAt,
	)
}

func TestRehydrateWalletRejectsInvalidState(t *testing.T) {
	validID := WalletID("wallet-1")
	validPlayerID := PlayerID("player-1")
	validBalance := mustNewMoney(t, "100.00", "BRL")
	createdAt := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.October, 13, 17, 45, 0, 0, time.UTC)

	tests := []struct {
		name      string
		id        WalletID
		playerID  PlayerID
		balance   Money
		version   int64
		createdAt time.Time
		updatedAt time.Time
		wantError error
	}{
		{
			name: "empty wallet ID", id: WalletID(""), playerID: validPlayerID,
			balance: validBalance, version: 1, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrInvalidWalletID,
		},
		{
			name: "empty player ID", id: validID, playerID: PlayerID(""),
			balance: validBalance, version: 1, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrInvalidPlayerID,
		},
		{
			name: "uninitialized balance", id: validID, playerID: validPlayerID,
			balance: Money{}, version: 1, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrInvalidMoney,
		},
		{
			name: "negative balance", id: validID, playerID: validPlayerID,
			balance: mustNewMoney(t, "-0.01", "BRL"), version: 1, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrNegativeWalletBalance,
		},
		{
			name: "zero version", id: validID, playerID: validPlayerID,
			balance: validBalance, version: 0, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrInvalidWalletVersion,
		},
		{
			name: "negative version", id: validID, playerID: validPlayerID,
			balance: validBalance, version: -1, createdAt: createdAt, updatedAt: updatedAt,
			wantError: ErrInvalidWalletVersion,
		},
		{
			name: "zero creation time", id: validID, playerID: validPlayerID,
			balance: validBalance, version: 1, createdAt: time.Time{}, updatedAt: updatedAt,
			wantError: ErrInvalidWalletTimestamp,
		},
		{
			name: "zero update time", id: validID, playerID: validPlayerID,
			balance: validBalance, version: 1, createdAt: createdAt, updatedAt: time.Time{},
			wantError: ErrInvalidWalletTimestamp,
		},
		{
			name: "update before creation", id: validID, playerID: validPlayerID,
			balance: validBalance, version: 1, createdAt: updatedAt, updatedAt: createdAt,
			wantError: ErrInvalidWalletTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RehydrateWallet(
				tt.id,
				tt.playerID,
				tt.balance,
				tt.version,
				tt.createdAt,
				tt.updatedAt,
			)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("RehydrateWallet() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "rehydrate wallet")
		})
	}
}

func assertWalletState(
	t *testing.T,
	wallet Wallet,
	wantID WalletID,
	wantPlayerID PlayerID,
	wantAmount string,
	wantCurrency Currency,
	wantVersion int64,
	wantCreatedAt time.Time,
	wantUpdatedAt time.Time,
) {
	t.Helper()

	if wallet.ID() != wantID {
		t.Errorf("Wallet.ID() = %q; want %q", wallet.ID(), wantID)
	}
	if wallet.PlayerID() != wantPlayerID {
		t.Errorf("Wallet.PlayerID() = %q; want %q", wallet.PlayerID(), wantPlayerID)
	}
	if wallet.Balance().Amount() != wantAmount {
		t.Errorf("Wallet.Balance().Amount() = %q; want %q", wallet.Balance().Amount(), wantAmount)
	}
	if wallet.Currency() != wantCurrency {
		t.Errorf("Wallet.Currency() = %q; want %q", wallet.Currency(), wantCurrency)
	}
	if wallet.Version() != wantVersion {
		t.Errorf("Wallet.Version() = %d; want %d", wallet.Version(), wantVersion)
	}
	if !wallet.CreatedAt().Equal(wantCreatedAt) {
		t.Errorf("Wallet.CreatedAt() = %s; want %s", wallet.CreatedAt(), wantCreatedAt)
	}
	if !wallet.UpdatedAt().Equal(wantUpdatedAt) {
		t.Errorf("Wallet.UpdatedAt() = %s; want %s", wallet.UpdatedAt(), wantUpdatedAt)
	}
}
