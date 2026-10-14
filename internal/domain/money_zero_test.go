package domain

import (
	"errors"
	"testing"
)

func TestNewZeroMoney(t *testing.T) {
	tests := []struct {
		name         string
		currency     string
		wantCurrency Currency
	}{
		{name: "BRL", currency: "BRL", wantCurrency: CurrencyBRL},
		{name: "USD", currency: "USD", wantCurrency: CurrencyUSD},
		{name: "lowercase currency", currency: "brl", wantCurrency: CurrencyBRL},
		{name: "mixed case currency", currency: "uSd", wantCurrency: CurrencyUSD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewZeroMoney(tt.currency)
			if err != nil {
				t.Fatalf("NewZeroMoney(%q) returned an unexpected error: %v", tt.currency, err)
			}
			if got.Amount() != "0.00" {
				t.Errorf("NewZeroMoney(%q) amount = %q; want %q", tt.currency, got.Amount(), "0.00")
			}
			if got.Currency() != tt.wantCurrency {
				t.Errorf("NewZeroMoney(%q) currency = %q; want %q", tt.currency, got.Currency(), tt.wantCurrency)
			}
		})
	}
}

func TestNewZeroMoneyRejectsUnsupportedCurrency(t *testing.T) {
	_, err := NewZeroMoney("EUR")
	if !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("NewZeroMoney(EUR) error = %v; want ErrUnsupportedCurrency", err)
	}
	assertErrorContains(t, err, "create money", `amount="0.00"`, `currency="EUR"`)
}

func TestMoneyIsZero(t *testing.T) {
	tests := []struct {
		name  string
		money Money
		want  bool
	}{
		{name: "zero", money: mustNewMoney(t, "0.00", "BRL"), want: true},
		{name: "negative zero canonicalizes to zero", money: mustNewMoney(t, "-0.00", "USD"), want: true},
		{name: "positive", money: mustNewMoney(t, "0.01", "BRL"), want: false},
		{name: "negative", money: mustNewMoney(t, "-0.01", "BRL"), want: false},
		{name: "uninitialized", money: Money{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.money.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %t; want %t", got, tt.want)
			}
		})
	}
}
