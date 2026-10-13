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
		return "", fmt.Errorf("%w: %q", ErrUnsupportedCurrency, currency)
	}

	return normalized, nil
}

func NewMoney(amount string, currency string) (Money, error) {
	normalizedCurrency, err := parseCurrency(currency)
	if err != nil {
		return Money{}, err
	}

	amountMinor, err := parseAmountMinor(amount)
	if err != nil {
		return Money{}, err
	}

	return Money{
		amountMinor: amountMinor,
		currency:    normalizedCurrency,
	}, nil
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

func (left Money) Add(right Money) (Money, error) {
	if !left.isInitialized() || !right.isInitialized() {
		return Money{}, ErrInvalidMoney
	}

	if left.currency != right.currency {
		return Money{}, ErrCurrencyMismatch
	}

	exceedsMaximum := right.amountMinor > 0 &&
		left.amountMinor > math.MaxInt64-right.amountMinor

	fallsBelowMinimum := right.amountMinor < 0 &&
		left.amountMinor < math.MinInt64-right.amountMinor

	if exceedsMaximum || fallsBelowMinimum {
		return Money{}, ErrAmountOutOfRange
	}

	return Money{
		amountMinor: left.amountMinor + right.amountMinor,
		currency:    left.currency,
	}, nil
}

func (left Money) Subtract(right Money) (Money, error) {
	if !left.isInitialized() || !right.isInitialized() {
		return Money{}, ErrInvalidMoney
	}

	if left.currency != right.currency {
		return Money{}, ErrCurrencyMismatch
	}

	exceedsMaximum := right.amountMinor < 0 &&
		left.amountMinor > math.MaxInt64+right.amountMinor

	fallsBelowMinimum := right.amountMinor > 0 &&
		left.amountMinor < math.MinInt64+right.amountMinor

	if exceedsMaximum || fallsBelowMinimum {
		return Money{}, ErrAmountOutOfRange
	}

	return Money{
		amountMinor: left.amountMinor - right.amountMinor,
		currency:    left.currency,
	}, nil
}
