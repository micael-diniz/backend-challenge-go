package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewWalletLedgerEntry(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		id            LedgerEntryID
		walletID      WalletID
		transactionID TransactionID
		direction     LedgerDirection
		amount        Money
		balanceBefore Money
		balanceAfter  Money
	}{
		{
			name:          "credit",
			id:            LedgerEntryID("ledger-1"),
			walletID:      WalletID("wallet-1"),
			transactionID: TransactionID("transaction-1"),
			direction:     LedgerDirectionCredit,
			amount:        mustNewMoney(t, "25.50", "BRL"),
			balanceBefore: mustNewMoney(t, "100.00", "BRL"),
			balanceAfter:  mustNewMoney(t, "125.50", "BRL"),
		},
		{
			name:          "debit",
			id:            LedgerEntryID("ledger-2"),
			walletID:      WalletID("wallet-2"),
			transactionID: TransactionID("transaction-2"),
			direction:     LedgerDirectionDebit,
			amount:        mustNewMoney(t, "25.50", "USD"),
			balanceBefore: mustNewMoney(t, "100.00", "USD"),
			balanceAfter:  mustNewMoney(t, "74.50", "USD"),
		},
		{
			name:          "opening credit from zero",
			id:            LedgerEntryID("ledger-3"),
			walletID:      WalletID("wallet-3"),
			transactionID: TransactionID("transaction-3"),
			direction:     LedgerDirectionCredit,
			amount:        mustNewMoney(t, "1000.00", "BRL"),
			balanceBefore: mustNewMoney(t, "0.00", "BRL"),
			balanceAfter:  mustNewMoney(t, "1000.00", "BRL"),
		},
		{
			name:          "debit reaches exactly zero",
			id:            LedgerEntryID("ledger-4"),
			walletID:      WalletID("wallet-4"),
			transactionID: TransactionID("transaction-4"),
			direction:     LedgerDirectionDebit,
			amount:        mustNewMoney(t, "0.01", "BRL"),
			balanceBefore: mustNewMoney(t, "0.01", "BRL"),
			balanceAfter:  mustNewMoney(t, "0.00", "BRL"),
		},
		{
			name:          "credit reaches maximum balance",
			id:            LedgerEntryID("ledger-5"),
			walletID:      WalletID("wallet-5"),
			transactionID: TransactionID("transaction-5"),
			direction:     LedgerDirectionCredit,
			amount:        mustNewMoney(t, "0.01", "BRL"),
			balanceBefore: mustNewMoney(t, "92233720368547758.06", "BRL"),
			balanceAfter:  mustNewMoney(t, "92233720368547758.07", "BRL"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWalletLedgerEntry(WalletLedgerEntryInput{
				ID:            tt.id,
				WalletID:      tt.walletID,
				TransactionID: tt.transactionID,
				Direction:     tt.direction,
				Amount:        tt.amount,
				BalanceBefore: tt.balanceBefore,
				BalanceAfter:  tt.balanceAfter,
				CreatedAt:     createdAt,
			})
			if err != nil {
				t.Fatalf("NewWalletLedgerEntry() returned an unexpected error: %v", err)
			}

			assertWalletLedgerEntryState(
				t,
				got,
				tt.id,
				tt.walletID,
				tt.transactionID,
				tt.direction,
				tt.amount,
				tt.balanceBefore,
				tt.balanceAfter,
				createdAt,
			)
			if got.Direction().String() != string(tt.direction) {
				t.Errorf("Direction().String() = %q; want %q", got.Direction().String(), tt.direction)
			}
		})
	}
}

func TestNewOpeningWalletLedgerEntry(t *testing.T) {
	t.Parallel()

	amount := mustNewMoney(t, "1000.00", "BRL")
	createdAt := time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC)
	got, err := NewOpeningWalletLedgerEntry(OpeningWalletLedgerEntryInput{
		ID:            LedgerEntryID("ledger-1"),
		WalletID:      WalletID("wallet-1"),
		TransactionID: TransactionID("opening-transaction-1"),
		Amount:        amount,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("NewOpeningWalletLedgerEntry() returned an unexpected error: %v", err)
	}

	assertWalletLedgerEntryState(
		t,
		got,
		LedgerEntryID("ledger-1"),
		WalletID("wallet-1"),
		TransactionID("opening-transaction-1"),
		LedgerDirectionCredit,
		amount,
		mustNewMoney(t, "0.00", "BRL"),
		amount,
		createdAt,
	)
}

func TestNewOpeningWalletLedgerEntryPreservesValidationErrors(t *testing.T) {
	t.Parallel()

	_, err := NewOpeningWalletLedgerEntry(OpeningWalletLedgerEntryInput{
		ID:            LedgerEntryID("ledger-1"),
		WalletID:      WalletID("wallet-1"),
		TransactionID: TransactionID("opening-transaction-1"),
		Amount:        Money{},
		CreatedAt:     time.Date(2026, time.October, 14, 18, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("NewOpeningWalletLedgerEntry() error = %v; want ErrInvalidMoney", err)
	}
}

func TestNewWalletLedgerEntryRejectsInvalidInput(t *testing.T) {
	validID := LedgerEntryID("ledger-1")
	validWalletID := WalletID("wallet-1")
	validTransactionID := TransactionID("transaction-1")
	validDirection := LedgerDirectionCredit
	validAmount := mustNewMoney(t, "25.00", "BRL")
	validBalanceBefore := mustNewMoney(t, "100.00", "BRL")
	validBalanceAfter := mustNewMoney(t, "125.00", "BRL")
	validTime := time.Date(2026, time.October, 13, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		id               LedgerEntryID
		walletID         WalletID
		transactionID    TransactionID
		direction        LedgerDirection
		amount           Money
		balanceBefore    Money
		balanceAfter     Money
		createdAt        time.Time
		wantError        error
		messageFragments []string
	}{
		{name: "empty ledger entry ID", id: LedgerEntryID(""), walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidLedgerEntryID},
		{name: "blank ledger entry ID", id: LedgerEntryID(" \t"), walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidLedgerEntryID},
		{name: "empty wallet ID", id: validID, walletID: WalletID(""), transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidWalletID},
		{name: "blank wallet ID", id: validID, walletID: WalletID("\n"), transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidWalletID},
		{name: "empty transaction ID", id: validID, walletID: validWalletID, transactionID: TransactionID(""), direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidTransactionID},
		{name: "blank transaction ID", id: validID, walletID: validWalletID, transactionID: TransactionID("  "), direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidTransactionID},
		{name: "empty direction", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirection(""), amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidLedgerDirection},
		{name: "unknown direction", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirection("TRANSFER"), amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidLedgerDirection},
		{name: "lowercase direction", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirection("credit"), amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidLedgerDirection},
		{name: "uninitialized amount", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: Money{}, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidMoney},
		{name: "zero amount", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: mustNewMoney(t, "0.00", "BRL"), balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrNonPositiveLedgerAmount, messageFragments: []string{"amount=0.00 BRL"}},
		{name: "negative amount", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: mustNewMoney(t, "-0.01", "BRL"), balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrNonPositiveLedgerAmount, messageFragments: []string{"amount=-0.01 BRL"}},
		{name: "uninitialized balance before", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: Money{}, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrInvalidMoney},
		{name: "negative balance before", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: mustNewMoney(t, "-0.01", "BRL"), balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrNegativeLedgerBalance, messageFragments: []string{"balanceBefore=-0.01 BRL", "balanceAfter=125.00 BRL"}},
		{name: "uninitialized balance after", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: Money{}, createdAt: validTime, wantError: ErrInvalidMoney},
		{name: "negative balance after", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirectionDebit, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: mustNewMoney(t, "-0.01", "BRL"), createdAt: validTime, wantError: ErrNegativeLedgerBalance, messageFragments: []string{"balanceBefore=100.00 BRL", "balanceAfter=-0.01 BRL"}},
		{name: "amount currency mismatch", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: mustNewMoney(t, "25.00", "USD"), balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: validTime, wantError: ErrCurrencyMismatch},
		{name: "balance after currency mismatch", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: mustNewMoney(t, "125.00", "USD"), createdAt: validTime, wantError: ErrCurrencyMismatch},
		{name: "credit equation mismatch", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirectionCredit, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: mustNewMoney(t, "124.99", "BRL"), createdAt: validTime, wantError: ErrLedgerBalanceMismatch, messageFragments: []string{"direction=CREDIT", "amount=25.00 BRL", "balanceBefore=100.00 BRL", "balanceAfter=124.99 BRL"}},
		{name: "debit equation mismatch", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirectionDebit, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: mustNewMoney(t, "75.01", "BRL"), createdAt: validTime, wantError: ErrLedgerBalanceMismatch, messageFragments: []string{"direction=DEBIT", "amount=25.00 BRL", "balanceBefore=100.00 BRL", "balanceAfter=75.01 BRL"}},
		{name: "debit exceeds balance", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirectionDebit, amount: mustNewMoney(t, "100.01", "BRL"), balanceBefore: validBalanceBefore, balanceAfter: mustNewMoney(t, "0.00", "BRL"), createdAt: validTime, wantError: ErrLedgerBalanceMismatch},
		{name: "credit balance overflow", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: LedgerDirectionCredit, amount: mustNewMoney(t, "0.01", "BRL"), balanceBefore: mustNewMoney(t, "92233720368547758.07", "BRL"), balanceAfter: mustNewMoney(t, "92233720368547758.07", "BRL"), createdAt: validTime, wantError: ErrAmountOutOfRange},
		{name: "zero creation time", id: validID, walletID: validWalletID, transactionID: validTransactionID, direction: validDirection, amount: validAmount, balanceBefore: validBalanceBefore, balanceAfter: validBalanceAfter, createdAt: time.Time{}, wantError: ErrInvalidLedgerTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWalletLedgerEntry(WalletLedgerEntryInput{
				ID:            tt.id,
				WalletID:      tt.walletID,
				TransactionID: tt.transactionID,
				Direction:     tt.direction,
				Amount:        tt.amount,
				BalanceBefore: tt.balanceBefore,
				BalanceAfter:  tt.balanceAfter,
				CreatedAt:     tt.createdAt,
			})
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("NewWalletLedgerEntry() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "create wallet ledger entry")
			assertErrorContains(t, err, tt.wantError.Error())
			assertErrorContains(t, err, tt.messageFragments...)
		})
	}
}

func assertWalletLedgerEntryState(
	t *testing.T,
	entry WalletLedgerEntry,
	wantID LedgerEntryID,
	wantWalletID WalletID,
	wantTransactionID TransactionID,
	wantDirection LedgerDirection,
	wantAmount Money,
	wantBalanceBefore Money,
	wantBalanceAfter Money,
	wantCreatedAt time.Time,
) {
	t.Helper()

	if entry.ID() != wantID {
		t.Errorf("WalletLedgerEntry.ID() = %q; want %q", entry.ID(), wantID)
	}
	if entry.WalletID() != wantWalletID {
		t.Errorf("WalletLedgerEntry.WalletID() = %q; want %q", entry.WalletID(), wantWalletID)
	}
	if entry.TransactionID() != wantTransactionID {
		t.Errorf("WalletLedgerEntry.TransactionID() = %q; want %q", entry.TransactionID(), wantTransactionID)
	}
	if entry.Direction() != wantDirection {
		t.Errorf("WalletLedgerEntry.Direction() = %q; want %q", entry.Direction(), wantDirection)
	}
	assertMoneyState(t, "WalletLedgerEntry.Money()", entry.Money(), wantAmount)
	assertMoneyState(t, "WalletLedgerEntry.BalanceBefore()", entry.BalanceBefore(), wantBalanceBefore)
	assertMoneyState(t, "WalletLedgerEntry.BalanceAfter()", entry.BalanceAfter(), wantBalanceAfter)
	if !entry.CreatedAt().Equal(wantCreatedAt) {
		t.Errorf("WalletLedgerEntry.CreatedAt() = %s; want %s", entry.CreatedAt(), wantCreatedAt)
	}
}

func assertMoneyState(t *testing.T, field string, got Money, want Money) {
	t.Helper()

	if got.Amount() != want.Amount() {
		t.Errorf("%s.Amount() = %q; want %q", field, got.Amount(), want.Amount())
	}
	if got.Currency() != want.Currency() {
		t.Errorf("%s.Currency() = %q; want %q", field, got.Currency(), want.Currency())
	}
}
