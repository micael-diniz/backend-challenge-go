package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type LedgerEntryID string
type LedgerDirection string

const (
	LedgerDirectionDebit  LedgerDirection = "DEBIT"
	LedgerDirectionCredit LedgerDirection = "CREDIT"
)

var (
	ErrInvalidLedgerEntryID    = errors.New("invalid ledger entry ID")
	ErrInvalidLedgerDirection  = errors.New("invalid ledger direction")
	ErrNonPositiveLedgerAmount = errors.New("ledger amount must be positive")
	ErrNegativeLedgerBalance   = errors.New("ledger balance cannot be negative")
	ErrInvalidLedgerTimestamp  = errors.New("invalid ledger timestamp")
	ErrLedgerBalanceMismatch   = errors.New("ledger balance mismatch")
)

type WalletLedgerEntry struct {
	id            LedgerEntryID
	walletID      WalletID
	transactionID TransactionID
	direction     LedgerDirection
	money         Money
	balanceBefore Money
	balanceAfter  Money
	createdAt     time.Time
}

type WalletLedgerEntryInput struct {
	ID            LedgerEntryID
	WalletID      WalletID
	TransactionID TransactionID
	Direction     LedgerDirection
	Amount        Money
	BalanceBefore Money
	BalanceAfter  Money
	CreatedAt     time.Time
}

type OpeningWalletLedgerEntryInput struct {
	ID            LedgerEntryID
	WalletID      WalletID
	TransactionID TransactionID
	Amount        Money
	CreatedAt     time.Time
}

func validateWalletLedgerEntryIdentities(
	id LedgerEntryID,
	walletID WalletID,
	transactionID TransactionID,
) error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrInvalidLedgerEntryID
	}

	if strings.TrimSpace(string(walletID)) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(string(transactionID)) == "" {
		return ErrInvalidTransactionID
	}

	return nil
}

func (direction LedgerDirection) isSupported() bool {
	switch direction {
	case LedgerDirectionDebit, LedgerDirectionCredit:
		return true
	default:
		return false
	}
}

func validateWalletLedgerEntryTimestamp(createdAt time.Time) error {
	if createdAt.IsZero() {
		return fmt.Errorf(
			"%w: createdAt cannot be zero",
			ErrInvalidLedgerTimestamp,
		)
	}

	return nil
}

func buildWalletLedgerEntry(
	input WalletLedgerEntryInput,
) (WalletLedgerEntry, error) {
	if err := validateWalletLedgerEntryIdentities(input.ID, input.WalletID, input.TransactionID); err != nil {
		return WalletLedgerEntry{}, err
	}

	if !input.Direction.isSupported() {
		return WalletLedgerEntry{}, ErrInvalidLedgerDirection
	}

	if !input.Amount.isInitialized() {
		return WalletLedgerEntry{}, ErrInvalidMoney
	}

	if !input.Amount.isPositive() {
		return WalletLedgerEntry{}, fmt.Errorf(
			"%w: amount=%s",
			ErrNonPositiveLedgerAmount,
			input.Amount.formatForError(),
		)
	}

	if err := input.BalanceBefore.validateBinaryOperation("ledger entry", input.BalanceAfter); err != nil {
		return WalletLedgerEntry{}, err
	}

	if input.BalanceBefore.isNegative() {
		return WalletLedgerEntry{}, fmt.Errorf(
			"create wallet ledger entry: %w: balanceBefore=%s, balanceAfter=%s",
			ErrNegativeLedgerBalance,
			input.BalanceBefore.formatForError(),
			input.BalanceAfter.formatForError(),
		)
	}

	if input.BalanceAfter.isNegative() {
		return WalletLedgerEntry{}, fmt.Errorf(
			"ledger entry balances: %w: balanceBefore=%s, balanceAfter=%s",
			ErrNegativeLedgerBalance,
			input.BalanceBefore.formatForError(),
			input.BalanceAfter.formatForError(),
		)
	}

	if err := validateWalletLedgerEntryTimestamp(input.CreatedAt); err != nil {
		return WalletLedgerEntry{}, err
	}

	var expectedBalance Money
	var err error

	switch input.Direction {
	case LedgerDirectionCredit:
		expectedBalance, err = input.BalanceBefore.Add(input.Amount)
	case LedgerDirectionDebit:
		expectedBalance, err = input.BalanceBefore.Subtract(input.Amount)
	}

	if err != nil {
		return WalletLedgerEntry{}, fmt.Errorf(
			"calculate ledger balance: %w",
			err,
		)
	}

	if !expectedBalance.equals(input.BalanceAfter) {
		return WalletLedgerEntry{}, fmt.Errorf(
			"%w: direction=%s, amount=%s, balanceBefore=%s, balanceAfter=%s",
			ErrLedgerBalanceMismatch,
			input.Direction,
			input.Amount.formatForError(),
			input.BalanceBefore.formatForError(),
			input.BalanceAfter.formatForError(),
		)
	}

	return WalletLedgerEntry{
		id:            input.ID,
		walletID:      input.WalletID,
		transactionID: input.TransactionID,
		direction:     input.Direction,
		money:         input.Amount,
		balanceBefore: input.BalanceBefore,
		balanceAfter:  input.BalanceAfter,
		createdAt:     input.CreatedAt,
	}, nil
}

func NewWalletLedgerEntry(
	input WalletLedgerEntryInput,
) (WalletLedgerEntry, error) {
	ledgerEntry, err := buildWalletLedgerEntry(input)
	if err != nil {
		return WalletLedgerEntry{}, fmt.Errorf("create wallet ledger entry: %w", err)
	}
	return ledgerEntry, nil
}

func NewOpeningWalletLedgerEntry(
	input OpeningWalletLedgerEntryInput,
) (WalletLedgerEntry, error) {
	zero := Money{currency: input.Amount.currency}
	ledgerEntry, err := buildWalletLedgerEntry(WalletLedgerEntryInput{
		ID:            input.ID,
		WalletID:      input.WalletID,
		TransactionID: input.TransactionID,
		Direction:     LedgerDirectionCredit,
		Amount:        input.Amount,
		BalanceBefore: zero,
		BalanceAfter:  input.Amount,
		CreatedAt:     input.CreatedAt,
	})
	if err != nil {
		return WalletLedgerEntry{}, fmt.Errorf(
			"create opening wallet ledger entry: %w",
			err,
		)
	}

	return ledgerEntry, nil
}

func (ledgerEntry WalletLedgerEntry) ID() LedgerEntryID {
	return ledgerEntry.id
}

func (ledgerEntry WalletLedgerEntry) WalletID() WalletID {
	return ledgerEntry.walletID
}

func (ledgerEntry WalletLedgerEntry) TransactionID() TransactionID {
	return ledgerEntry.transactionID
}

func (ledgerEntry WalletLedgerEntry) Direction() LedgerDirection {
	return ledgerEntry.direction
}

func (direction LedgerDirection) String() string {
	return string(direction)
}

func (ledgerEntry WalletLedgerEntry) Money() Money {
	return ledgerEntry.money
}

func (ledgerEntry WalletLedgerEntry) BalanceBefore() Money {
	return ledgerEntry.balanceBefore
}

func (ledgerEntry WalletLedgerEntry) BalanceAfter() Money {
	return ledgerEntry.balanceAfter
}

func (ledgerEntry WalletLedgerEntry) CreatedAt() time.Time {
	return ledgerEntry.createdAt
}
