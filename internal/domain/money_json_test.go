package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMoneyMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		wantJSON string
	}{
		{name: "BRL", amount: "25.40", currency: "BRL", wantJSON: `{"amount":"25.40","currency":"BRL"}`},
		{name: "USD zero", amount: "0.00", currency: "USD", wantJSON: `{"amount":"0.00","currency":"USD"}`},
		{name: "negative", amount: "-10.05", currency: "BRL", wantJSON: `{"amount":"-10.05","currency":"BRL"}`},
		{name: "minimum boundary", amount: "-92233720368547758.08", currency: "BRL", wantJSON: `{"amount":"-92233720368547758.08","currency":"BRL"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			money := mustNewMoney(t, tt.amount, tt.currency)

			got, err := json.Marshal(money)
			if err != nil {
				t.Fatalf("json.Marshal(%s) returned an unexpected error: %v", money.formatForError(), err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("json.Marshal(%s) = %s; want %s", money.formatForError(), got, tt.wantJSON)
			}
		})
	}
}

func TestMoneyMarshalJSONRejectsUninitializedMoney(t *testing.T) {
	_, err := json.Marshal(Money{})
	if !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("json.Marshal(Money{}) error = %v; want ErrInvalidMoney", err)
	}
}

func TestMoneyUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantAmount   string
		wantCurrency Currency
	}{
		{name: "BRL", input: `{"amount":"25.40","currency":"BRL"}`, wantAmount: "25.40", wantCurrency: CurrencyBRL},
		{name: "normalizes currency", input: `{"amount":"25.40","currency":"uSd"}`, wantAmount: "25.40", wantCurrency: CurrencyUSD},
		{name: "zero", input: `{"amount":"0.00","currency":"BRL"}`, wantAmount: "0.00", wantCurrency: CurrencyBRL},
		{name: "negative", input: `{"amount":"-10.05","currency":"USD"}`, wantAmount: "-10.05", wantCurrency: CurrencyUSD},
		{name: "maximum boundary", input: `{"amount":"92233720368547758.07","currency":"BRL"}`, wantAmount: "92233720368547758.07", wantCurrency: CurrencyBRL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Money

			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("json.Unmarshal(%s) returned an unexpected error: %v", tt.input, err)
			}
			if got.Amount() != tt.wantAmount {
				t.Errorf("unmarshaled amount = %q; want %q", got.Amount(), tt.wantAmount)
			}
			if got.Currency() != tt.wantCurrency {
				t.Errorf("unmarshaled currency = %q; want %q", got.Currency(), tt.wantCurrency)
			}
		})
	}
}

func TestMoneyUnmarshalJSONRejectsInvalidInputWithoutChangingReceiver(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError error
	}{
		{name: "null", input: `null`, wantError: ErrInvalidMoney},
		{name: "numeric amount", input: `{"amount":25.40,"currency":"BRL"}`, wantError: ErrInvalidMoney},
		{name: "numeric currency", input: `{"amount":"25.40","currency":986}`, wantError: ErrInvalidMoney},
		{name: "missing amount", input: `{"currency":"BRL"}`, wantError: ErrInvalidAmount},
		{name: "missing currency", input: `{"amount":"25.40"}`, wantError: ErrUnsupportedCurrency},
		{name: "invalid amount", input: `{"amount":"25.4","currency":"BRL"}`, wantError: ErrInvalidAmount},
		{name: "amount overflow", input: `{"amount":"92233720368547758.08","currency":"BRL"}`, wantError: ErrAmountOutOfRange},
		{name: "unsupported currency", input: `{"amount":"25.40","currency":"EUR"}`, wantError: ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustNewMoney(t, "10.00", "USD")

			err := json.Unmarshal([]byte(tt.input), &got)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("json.Unmarshal(%s) error = %v; want %v", tt.input, err, tt.wantError)
			}
			if got.Amount() != "10.00" || got.Currency() != CurrencyUSD {
				t.Errorf("failed unmarshal changed receiver to %s; want 10.00 USD", got.formatForError())
			}
		})
	}
}

func TestMoneyJSONRoundTrip(t *testing.T) {
	want := mustNewMoney(t, "1234567890.12", "BRL")

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() returned an unexpected error: %v", err)
	}

	var got Money
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("json.Unmarshal() returned an unexpected error: %v", err)
	}

	comparison, err := got.Compare(want)
	if err != nil {
		t.Fatalf("Compare() returned an unexpected error: %v", err)
	}
	if comparison != 0 {
		t.Errorf("JSON round trip produced %s; want %s", got.formatForError(), want.formatForError())
	}
}
