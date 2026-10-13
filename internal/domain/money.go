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

func (m Money) Add(other Money) (Money, error) {
	if !m.isInitialized() || !other.isInitialized() {
		return Money{}, ErrInvalidMoney
	}

	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if other.amountMinor > 0 &&
		m.amountMinor > math.MaxInt64-other.amountMinor {
		return Money{}, ErrAmountOutOfRange
	}

	if other.amountMinor < 0 &&
		m.amountMinor < math.MinInt64-other.amountMinor {
		return Money{}, ErrAmountOutOfRange
	}

	return Money{
		amountMinor: m.amountMinor + other.amountMinor,
		currency:    m.currency,
	}, nil
}
