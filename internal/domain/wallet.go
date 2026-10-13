package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrInvalidWalletID           = errors.New("invalid wallet ID")
	ErrInvalidPlayerID           = errors.New("invalid player ID")
	ErrNegativeWalletBalance     = errors.New("wallet balance cannot be negative")
	ErrInvalidWalletVersion      = errors.New("invalid wallet version")
	ErrInvalidWalletTimestamp    = errors.New("invalid wallet timestamp")
	ErrInvalidWallet             = errors.New("invalid wallet")
	ErrNonPositiveWalletMovement = errors.New("wallet movement amount must be positive")
)

type WalletID string
type PlayerID string

type Wallet struct {
	id        WalletID
	playerID  PlayerID
	currency  Currency
	balance   Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

func validateWalletIdentity(id WalletID, playerID PlayerID) error {
	if id == "" {
		return ErrInvalidWalletID
	}

	if playerID == "" {
		return ErrInvalidPlayerID
	}

	return nil
}

func validateWalletBalance(balance Money) error {
	if !balance.isInitialized() {
		return ErrInvalidMoney
	}

	if balance.isNegative() {
		return fmt.Errorf(
			"%w: balance=%s",
			ErrNegativeWalletBalance,
			balance.formatForError(),
		)
	}

	return nil
}

func validateWalletVersion(version int64) error {
	if version < 1 {
		return fmt.Errorf(
			"%w: version=%d",
			ErrInvalidWalletVersion,
			version,
		)
	}

	return nil
}

func validateWalletTimestamps(
	createdAt time.Time,
	updatedAt time.Time,
) error {
	if createdAt.IsZero() {
		return fmt.Errorf(
			"%w: createdAt cannot be zero",
			ErrInvalidWalletTimestamp,
		)
	}

	if updatedAt.IsZero() {
		return fmt.Errorf(
			"%w: updatedAt cannot be zero",
			ErrInvalidWalletTimestamp,
		)
	}

	if updatedAt.Before(createdAt) {
		return fmt.Errorf(
			"%w: updatedAt cannot precede createdAt",
			ErrInvalidWalletTimestamp,
		)
	}

	return nil
}

func buildWallet(
	id WalletID,
	playerID PlayerID,
	balance Money,
	version int64,
	createdAt time.Time,
	updatedAt time.Time,
) (Wallet, error) {
	if err := validateWalletIdentity(id, playerID); err != nil {
		return Wallet{}, err
	}

	if err := validateWalletBalance(balance); err != nil {
		return Wallet{}, err
	}

	if err := validateWalletVersion(version); err != nil {
		return Wallet{}, err
	}

	if err := validateWalletTimestamps(createdAt, updatedAt); err != nil {
		return Wallet{}, err
	}

	return Wallet{
		id:        id,
		playerID:  playerID,
		currency:  balance.Currency(),
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func NewWallet(
	id WalletID,
	playerID PlayerID,
	initialBalance Money,
	createdAt time.Time,
) (Wallet, error) {
	version := int64(1)
	updatedAt := createdAt

	wallet, err := buildWallet(id, playerID, initialBalance, version, createdAt, updatedAt)
	if err != nil {
		return Wallet{}, fmt.Errorf("create wallet: %w", err)
	}

	return wallet, nil
}

func RehydrateWallet(
	id WalletID,
	playerID PlayerID,
	balance Money,
	version int64,
	createdAt time.Time,
	updatedAt time.Time,
) (Wallet, error) {
	wallet, err := buildWallet(id, playerID, balance, version, createdAt, updatedAt)
	if err != nil {
		return Wallet{}, fmt.Errorf("rehydrate wallet: %w", err)
	}

	return wallet, nil
}

func (wallet Wallet) validateForOperation() error {
	rebuilt, err := buildWallet(
		wallet.id,
		wallet.playerID,
		wallet.balance,
		wallet.version,
		wallet.createdAt,
		wallet.updatedAt,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWallet, err)
	}

	if rebuilt.currency != wallet.currency {
		return fmt.Errorf(
			"%w: wallet currency=%s, balance currency=%s",
			ErrInvalidWallet,
			wallet.currency,
			wallet.balance.Currency(),
		)
	}

	return nil
}

func (wallet Wallet) Credit(amount Money, creditedAt time.Time) (Wallet, error) {
	if err := wallet.validateForOperation(); err != nil {
		return Wallet{}, fmt.Errorf("credit wallet: %w", err)
	}

	if !amount.isInitialized() {
		return Wallet{}, fmt.Errorf("credit wallet: %w", ErrInvalidMoney)
	}

	if !amount.isPositive() {
		return Wallet{}, fmt.Errorf(
			"credit wallet: %w: amount=%s",
			ErrNonPositiveWalletMovement,
			amount.formatForError(),
		)
	}

	if creditedAt.IsZero() {
		return Wallet{}, fmt.Errorf(
			"credit wallet: %w: creditedAt cannot be zero",
			ErrInvalidWalletTimestamp,
		)
	}

	if creditedAt.Before(wallet.updatedAt) {
		return Wallet{}, fmt.Errorf(
			"credit wallet: %w: creditedAt=%s precedes updatedAt=%s",
			ErrInvalidWalletTimestamp,
			creditedAt.Format(time.RFC3339Nano),
			wallet.updatedAt.Format(time.RFC3339Nano),
		)
	}

	if wallet.version == math.MaxInt64 {
		return Wallet{}, fmt.Errorf(
			"credit wallet: %w: version=%d cannot be incremented",
			ErrInvalidWalletVersion,
			wallet.version,
		)
	}

	newBalance, err := wallet.balance.Add(amount)
	if err != nil {
		return Wallet{}, fmt.Errorf("credit wallet: %w", err)
	}

	updated := wallet
	updated.balance = newBalance
	updated.version++
	updated.updatedAt = creditedAt

	return updated, nil
}

func (wallet Wallet) ID() WalletID {
	return wallet.id
}

func (wallet Wallet) PlayerID() PlayerID {
	return wallet.playerID
}

func (wallet Wallet) Currency() Currency {
	return wallet.currency
}

func (wallet Wallet) Balance() Money {
	return wallet.balance
}

func (wallet Wallet) Version() int64 {
	return wallet.version
}

func (wallet Wallet) CreatedAt() time.Time {
	return wallet.createdAt
}

func (wallet Wallet) UpdatedAt() time.Time {
	return wallet.updatedAt
}
