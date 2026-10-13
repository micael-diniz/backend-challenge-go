package domain

import (
	"errors"
	"testing"
)

func TestNewMoneyValidInput(t *testing.T) {
	tests := []struct {
		name         string
		amount       string
		currency     string
		wantAmount   string
		wantCurrency Currency
	}{
		{name: "whole amount", amount: "25.00", currency: "BRL", wantAmount: "25.00", wantCurrency: CurrencyBRL},
		{name: "fractional amount", amount: "25.42", currency: "BRL", wantAmount: "25.42", wantCurrency: CurrencyBRL},
		{name: "one minor unit", amount: "0.01", currency: "BRL", wantAmount: "0.01", wantCurrency: CurrencyBRL},
		{name: "zero", amount: "0.00", currency: "USD", wantAmount: "0.00", wantCurrency: CurrencyUSD},
		{name: "negative amount", amount: "-20.00", currency: "BRL", wantAmount: "-20.00", wantCurrency: CurrencyBRL},
		{name: "negative minor unit", amount: "-0.01", currency: "USD", wantAmount: "-0.01", wantCurrency: CurrencyUSD},
		{name: "lowercase BRL", amount: "25.00", currency: "brl", wantAmount: "25.00", wantCurrency: CurrencyBRL},
		{name: "mixed case BRL", amount: "25.00", currency: "BRl", wantAmount: "25.00", wantCurrency: CurrencyBRL},
		{name: "lowercase USD", amount: "25.00", currency: "usd", wantAmount: "25.00", wantCurrency: CurrencyUSD},
		{name: "mixed case USD", amount: "25.00", currency: "uSd", wantAmount: "25.00", wantCurrency: CurrencyUSD},
		{name: "maximum int64 minor units", amount: "92233720368547758.07", currency: "BRL", wantAmount: "92233720368547758.07", wantCurrency: CurrencyBRL},
		{name: "minimum int64 minor units", amount: "-92233720368547758.08", currency: "BRL", wantAmount: "-92233720368547758.08", wantCurrency: CurrencyBRL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			money, err := NewMoney(tt.amount, tt.currency)
			if err != nil {
				t.Fatalf("NewMoney(%q, %q) returned an unexpected error: %v", tt.amount, tt.currency, err)
			}
			if got := money.Amount(); got != tt.wantAmount {
				t.Errorf("Amount() = %q; want %q", got, tt.wantAmount)
			}
			if got := money.Currency(); got != tt.wantCurrency {
				t.Errorf("Currency() = %q; want %q", got, tt.wantCurrency)
			}
		})
	}
}

func TestNewMoneyInvalidAmount(t *testing.T) {
	tests := []struct {
		name           string
		amount         string
		wantOutOfRange bool
	}{
		{name: "empty", amount: ""},
		{name: "NaN", amount: "NaN"},
		{name: "infinity", amount: "Infinity"},
		{name: "negative infinity", amount: "-Infinity"},
		{name: "scientific notation", amount: "1e2"},
		{name: "scientific notation with decimal", amount: "1.00E2"},
		{name: "missing decimal places", amount: "25"},
		{name: "one decimal place", amount: "25.0"},
		{name: "excess decimal places", amount: "25.001"},
		{name: "excess trailing zero", amount: "25.000"},
		{name: "missing integer part", amount: ".25"},
		{name: "missing fractional part", amount: "25."},
		{name: "explicit positive sign", amount: "+25.00"},
		{name: "leading whitespace", amount: " 25.00"},
		{name: "trailing whitespace", amount: "25.00 "},
		{name: "embedded whitespace", amount: "2 5.00"},
		{name: "trailing newline", amount: "25.00\n"},
		{name: "leading zeros", amount: "025.00"},
		{name: "multiple integer zeros", amount: "00.00"},
		{name: "negative leading zeros", amount: "-025.00"},
		{name: "decimal comma", amount: "25,00"},
		{name: "thousands separator", amount: "1,000.00"},
		{name: "multiple decimal points", amount: "25.0.0"},
		{name: "letters", amount: "abc"},
		{name: "non ASCII digits", amount: "２５.００"},
		{name: "positive overflow", amount: "92233720368547758.08", wantOutOfRange: true},
		{name: "negative overflow", amount: "-92233720368547758.09", wantOutOfRange: true},
		{name: "very large amount", amount: "999999999999999999999999999999.99", wantOutOfRange: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			money, err := NewMoney(tt.amount, "BRL")
			if err == nil {
				t.Fatalf("NewMoney(%q, BRL) succeeded with %+v; want an error", tt.amount, money)
			}
			if !errors.Is(err, ErrInvalidAmount) {
				t.Errorf("NewMoney(%q, BRL) error = %v; want ErrInvalidAmount", tt.amount, err)
			}
			if got := errors.Is(err, ErrAmountOutOfRange); got != tt.wantOutOfRange {
				t.Errorf(
					"NewMoney(%q, BRL) errors.Is(ErrAmountOutOfRange) = %t; want %t",
					tt.amount,
					got,
					tt.wantOutOfRange,
				)
			}
		})
	}
}

func TestNewMoneyInvalidCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
	}{
		{name: "empty", currency: ""},
		{name: "unknown code", currency: "XYZ"},
		{name: "unsupported currency", currency: "EUR"},
		{name: "too short", currency: "BR"},
		{name: "too long", currency: "BRLL"},
		{name: "leading whitespace", currency: " BRL"},
		{name: "trailing whitespace", currency: "USD "},
		{name: "trailing newline", currency: "BRL\n"},
		{name: "numeric code", currency: "986"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			money, err := NewMoney("25.00", tt.currency)
			if err == nil {
				t.Fatalf("NewMoney(25.00, %q) succeeded with %+v; want an error", tt.currency, money)
			}
			if !errors.Is(err, ErrUnsupportedCurrency) {
				t.Errorf("NewMoney(25.00, %q) error = %v; want ErrUnsupportedCurrency", tt.currency, err)
			}
		})
	}
}
