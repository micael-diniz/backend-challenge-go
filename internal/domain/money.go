package domain

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Currency string

type Money struct {
	amountMinor int64
	currency    Currency
}

const (
	CurrencyBRL Currency = "BRL"
	CurrencyUSD Currency = "USD"
)

var (
	ErrInvalidAmount       = errors.New("invalid money amount")
	ErrAmountOutOfRange    = errors.New("value is out of range")
	ErrUnsupportedCurrency = errors.New("unsupported currency")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrInvalidMoney        = errors.New("invalid money")
)

var moneyAmountPattern = regexp.MustCompile(
	`^-?(0|[1-9][0-9]*)\.[0-9]{2}$`,
)

func parseAmountMinor(amount string) (int64, error) {
	if !moneyAmountPattern.MatchString(amount) {
		return 0, ErrInvalidAmount
	}

	minorUnitsText := strings.Replace(amount, ".", "", 1)

	amountMinor, err := strconv.ParseInt(minorUnitsText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidAmount, ErrAmountOutOfRange)
	}

	return amountMinor, nil
}

func parseCurrency(currency string) (Currency, error) {
	normalized := Currency(strings.ToUpper(currency))

	if !normalized.isSupported() {
		return "", ErrUnsupportedCurrency
	}

	return normalized, nil
}

func NewMoney(amount string, currency string) (Money, error) {
	normalizedCurrency, err := parseCurrency(currency)
	if err != nil {
		return Money{}, wrapMoneyCreationError(amount, currency, err)
	}

	amountMinor, err := parseAmountMinor(amount)
	if err != nil {
		return Money{}, wrapMoneyCreationError(amount, currency, err)
	}

	return Money{
		amountMinor: amountMinor,
		currency:    normalizedCurrency,
	}, nil
}

func wrapMoneyCreationError(amount, currency string, err error) error {
	return fmt.Errorf(
		"create money: amount=%q, currency=%q: %w",
		amount,
		currency,
		err,
	)
}

func (money Money) Amount() string {
	whole := money.amountMinor / 100
	fraction := money.amountMinor % 100

	sign := ""
	if money.amountMinor < 0 {
		sign = "-"
		whole = -whole
		fraction = -fraction
	}

	return fmt.Sprintf("%s%d.%02d", sign, whole, fraction)
}

func (money Money) Currency() Currency {
	return money.currency
}

func (currency Currency) String() string {
	return string(currency)
}

func (currency Currency) isSupported() bool {
	switch currency {
	case CurrencyBRL, CurrencyUSD:
		return true
	default:
		return false
	}
}

func (money Money) isInitialized() bool {
	return money.currency.isSupported()
}

func (money Money) formatForError() string {
	return fmt.Sprintf("%s %s", money.Amount(), money.currency)
}

func (left Money) validateBinaryOperation(operation string, right Money) error {
	if !left.isInitialized() {
		return fmt.Errorf("%s money: %w: left operand", operation, ErrInvalidMoney)
	}

	if !right.isInitialized() {
		return fmt.Errorf("%s money: %w: right operand", operation, ErrInvalidMoney)
	}

	if left.currency != right.currency {
		return fmt.Errorf(
			"%s money: %w: left=%s, right=%s",
			operation,
			ErrCurrencyMismatch,
			left.formatForError(),
			right.formatForError(),
		)
	}

	return nil
}

func (left Money) Add(right Money) (Money, error) {
	if err := left.validateBinaryOperation("add", right); err != nil {
		return Money{}, err
	}

	exceedsMaximum := right.amountMinor > 0 &&
		left.amountMinor > math.MaxInt64-right.amountMinor

	fallsBelowMinimum := right.amountMinor < 0 &&
		left.amountMinor < math.MinInt64-right.amountMinor

	if exceedsMaximum || fallsBelowMinimum {
		return Money{}, fmt.Errorf(
			"add money: %w: left=%s, right=%s",
			ErrAmountOutOfRange,
			left.formatForError(),
			right.formatForError(),
		)
	}

	return Money{
		amountMinor: left.amountMinor + right.amountMinor,
		currency:    left.currency,
	}, nil
}

func (left Money) Subtract(right Money) (Money, error) {
	if err := left.validateBinaryOperation("subtract", right); err != nil {
		return Money{}, err
	}

	exceedsMaximum := right.amountMinor < 0 &&
		left.amountMinor > math.MaxInt64+right.amountMinor

	fallsBelowMinimum := right.amountMinor > 0 &&
		left.amountMinor < math.MinInt64+right.amountMinor

	if exceedsMaximum || fallsBelowMinimum {
		return Money{}, fmt.Errorf(
			"subtract money: %w: left=%s, right=%s",
			ErrAmountOutOfRange,
			left.formatForError(),
			right.formatForError(),
		)
	}

	return Money{
		amountMinor: left.amountMinor - right.amountMinor,
		currency:    left.currency,
	}, nil
}

func (money Money) Negate() (Money, error) {
	if !money.isInitialized() {
		return Money{}, fmt.Errorf("negate money: %w: operand", ErrInvalidMoney)
	}

	if money.amountMinor == math.MinInt64 {
		return Money{}, fmt.Errorf(
			"negate money: %w: value=%s",
			ErrAmountOutOfRange,
			money.formatForError(),
		)
	}

	return Money{
		amountMinor: -money.amountMinor,
		currency:    money.currency,
	}, nil
}
