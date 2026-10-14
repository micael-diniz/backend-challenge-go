package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type LedgerEntryID string
type TransactionID string
type LedgerDirection string

const (
	LedgerDirectionDebit  LedgerDirection = "DEBIT"
	LedgerDirectionCredit LedgerDirection = "CREDIT"
)

var (
	ErrInvalidLedgerEntryID    = errors.New("invalid ledger entry ID")
	ErrInvalidTransactionID    = errors.New("invalid transaction ID")
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
	id LedgerEntryID,
	walletID WalletID,
	transactionID TransactionID,
	direction LedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (WalletLedgerEntry, error) {
	if err := validateWalletLedgerEntryIdentities(id, walletID, transactionID); err != nil {
		return WalletLedgerEntry{}, err
	}

	if !direction.isSupported() {
		return WalletLedgerEntry{}, ErrInvalidLedgerDirection
	}

	if !amount.isInitialized() {
		return WalletLedgerEntry{}, ErrInvalidMoney
	}

	if !amount.isPositive() {
		return WalletLedgerEntry{}, fmt.Errorf(
			"%w: amount=%s",
			ErrNonPositiveLedgerAmount,
			amount.formatForError(),
		)
	}

	if err := balanceBefore.validateBinaryOperation("ledger entry", balanceAfter); err != nil {
		return WalletLedgerEntry{}, err
	}

	if balanceBefore.isNegative() || balanceAfter.isNegative() {
		return WalletLedgerEntry{}, fmt.Errorf(
			"ledger entry balances: %w: balanceBefore=%s, balanceAfter=%s",
			ErrNegativeLedgerBalance,
			balanceBefore.formatForError(),
			balanceAfter.formatForError(),
		)
	}

	if err := validateWalletLedgerEntryTimestamp(createdAt); err != nil {
		return WalletLedgerEntry{}, err
	}

	var expectedBalance Money
	var err error

	switch direction {
	case LedgerDirectionCredit:
		expectedBalance, err = balanceBefore.Add(amount)
	case LedgerDirectionDebit:
		expectedBalance, err = balanceBefore.Subtract(amount)
	}

	if err != nil {
		return WalletLedgerEntry{}, fmt.Errorf(
			"calculate ledger balance: %w",
			err,
		)
	}

	if !expectedBalance.equals(balanceAfter) {
		return WalletLedgerEntry{}, fmt.Errorf(
			"%w: direction=%s, amount=%s, balanceBefore=%s, balanceAfter=%s",
			ErrLedgerBalanceMismatch,
			direction,
			amount.formatForError(),
			balanceBefore.formatForError(),
			balanceAfter.formatForError(),
		)
	}

	return WalletLedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		money:         amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt,
	}, nil
}

func NewWalletLedgerEntry(
	id LedgerEntryID,
	walletID WalletID,
	transactionID TransactionID,
	direction LedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (WalletLedgerEntry, error) {
	ledgerEntry, err := buildWalletLedgerEntry(
		id,
		walletID,
		transactionID,
		direction,
		amount,
		balanceBefore,
		balanceAfter,
		createdAt,
	)
	if err != nil {
		return WalletLedgerEntry{}, fmt.Errorf("create wallet ledger entry: %w", err)
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
