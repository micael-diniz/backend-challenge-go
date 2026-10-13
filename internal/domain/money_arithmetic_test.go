package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestMoneyAdd(t *testing.T) {
	tests := []struct {
		name       string
		left       string
		right      string
		wantAmount string
	}{
		{name: "positive amounts", left: "25.40", right: "10.35", wantAmount: "35.75"},
		{name: "zero", left: "25.40", right: "0.00", wantAmount: "25.40"},
		{name: "negative amount", left: "25.40", right: "-10.35", wantAmount: "15.05"},
		{name: "negative result", left: "10.35", right: "-25.40", wantAmount: "-15.05"},
		{name: "maximum boundary", left: "92233720368547758.06", right: "0.01", wantAmount: "92233720368547758.07"},
		{name: "minimum boundary", left: "-92233720368547758.07", right: "-0.01", wantAmount: "-92233720368547758.08"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustNewMoney(t, tt.left, "BRL")
			right := mustNewMoney(t, tt.right, "BRL")

			got, err := left.Add(right)
			if err != nil {
				t.Fatalf("Add() returned an unexpected error: %v", err)
			}
			if got.Amount() != tt.wantAmount {
				t.Errorf("Add() amount = %q; want %q", got.Amount(), tt.wantAmount)
			}
			if got.Currency() != CurrencyBRL {
				t.Errorf("Add() currency = %q; want BRL", got.Currency())
			}
			if left.Amount() != tt.left {
				t.Errorf("Add() changed the receiver to %q; want it to remain %q", left.Amount(), tt.left)
			}
			if right.Amount() != tt.right {
				t.Errorf("Add() changed the argument to %q; want it to remain %q", right.Amount(), tt.right)
			}
		})
	}
}

func TestMoneyAddRejectsCurrencyMismatch(t *testing.T) {
	brl := mustNewMoney(t, "10.00", "BRL")
	usd := mustNewMoney(t, "5.00", "USD")

	_, err := brl.Add(usd)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add() error = %v; want ErrCurrencyMismatch", err)
	}
	assertErrorContains(t, err, "add money", "left=10.00 BRL", "right=5.00 USD")
}

func TestMoneyAddRejectsUninitializedMoney(t *testing.T) {
	valid := mustNewMoney(t, "10.00", "BRL")

	tests := []struct {
		name  string
		left  Money
		right Money
	}{
		{name: "uninitialized receiver", left: Money{}, right: valid},
		{name: "uninitialized argument", left: valid, right: Money{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.left.Add(tt.right)
			if !errors.Is(err, ErrInvalidMoney) {
				t.Fatalf("Add() error = %v; want ErrInvalidMoney", err)
			}
		})
	}
}

func TestMoneyAddRejectsOverflow(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{name: "positive overflow", left: "92233720368547758.07", right: "0.01"},
		{name: "negative overflow", left: "-92233720368547758.08", right: "-0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustNewMoney(t, tt.left, "BRL")
			right := mustNewMoney(t, tt.right, "BRL")

			_, err := left.Add(right)
			if !errors.Is(err, ErrAmountOutOfRange) {
				t.Fatalf("Add() error = %v; want ErrAmountOutOfRange", err)
			}
			assertErrorContains(t, err, "add money", "left="+tt.left+" BRL", "right="+tt.right+" BRL")
		})
	}
}

func TestMoneySubtract(t *testing.T) {
	tests := []struct {
		name       string
		left       string
		right      string
		wantAmount string
	}{
		{name: "positive amounts", left: "25.40", right: "10.35", wantAmount: "15.05"},
		{name: "zero", left: "25.40", right: "0.00", wantAmount: "25.40"},
		{name: "negative amount", left: "25.40", right: "-10.35", wantAmount: "35.75"},
		{name: "negative result", left: "10.35", right: "25.40", wantAmount: "-15.05"},
		{name: "maximum boundary", left: "92233720368547758.06", right: "-0.01", wantAmount: "92233720368547758.07"},
		{name: "minimum boundary", left: "-92233720368547758.07", right: "0.01", wantAmount: "-92233720368547758.08"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustNewMoney(t, tt.left, "BRL")
			right := mustNewMoney(t, tt.right, "BRL")

			got, err := left.Subtract(right)
			if err != nil {
				t.Fatalf("Subtract() returned an unexpected error: %v", err)
			}
			if got.Amount() != tt.wantAmount {
				t.Errorf("Subtract() amount = %q; want %q", got.Amount(), tt.wantAmount)
			}
			if got.Currency() != CurrencyBRL {
				t.Errorf("Subtract() currency = %q; want BRL", got.Currency())
			}
			if left.Amount() != tt.left {
				t.Errorf("Subtract() changed the receiver to %q; want it to remain %q", left.Amount(), tt.left)
			}
			if right.Amount() != tt.right {
				t.Errorf("Subtract() changed the argument to %q; want it to remain %q", right.Amount(), tt.right)
			}
		})
	}
}

func TestMoneySubtractRejectsCurrencyMismatch(t *testing.T) {
	brl := mustNewMoney(t, "10.00", "BRL")
	usd := mustNewMoney(t, "5.00", "USD")

	_, err := brl.Subtract(usd)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Subtract() error = %v; want ErrCurrencyMismatch", err)
	}
	assertErrorContains(t, err, "subtract money", "left=10.00 BRL", "right=5.00 USD")
}

func TestMoneySubtractRejectsUninitializedMoney(t *testing.T) {
	valid := mustNewMoney(t, "10.00", "BRL")

	tests := []struct {
		name  string
		left  Money
		right Money
	}{
		{name: "uninitialized receiver", left: Money{}, right: valid},
		{name: "uninitialized argument", left: valid, right: Money{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.left.Subtract(tt.right)
			if !errors.Is(err, ErrInvalidMoney) {
				t.Fatalf("Subtract() error = %v; want ErrInvalidMoney", err)
			}
		})
	}
}

func TestMoneySubtractRejectsOverflow(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{name: "positive overflow", left: "92233720368547758.07", right: "-0.01"},
		{name: "negative overflow", left: "-92233720368547758.08", right: "0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustNewMoney(t, tt.left, "BRL")
			right := mustNewMoney(t, tt.right, "BRL")

			_, err := left.Subtract(right)
			if !errors.Is(err, ErrAmountOutOfRange) {
				t.Fatalf("Subtract() error = %v; want ErrAmountOutOfRange", err)
			}
			assertErrorContains(t, err, "subtract money", "left="+tt.left+" BRL", "right="+tt.right+" BRL")
		})
	}
}

func TestMoneyNegate(t *testing.T) {
	tests := []struct {
		name       string
		amount     string
		wantAmount string
	}{
		{name: "positive amount", amount: "25.40", wantAmount: "-25.40"},
		{name: "negative amount", amount: "-25.40", wantAmount: "25.40"},
		{name: "zero", amount: "0.00", wantAmount: "0.00"},
		{name: "maximum boundary", amount: "92233720368547758.07", wantAmount: "-92233720368547758.07"},
		{name: "negative maximum boundary", amount: "-92233720368547758.07", wantAmount: "92233720368547758.07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := mustNewMoney(t, tt.amount, "BRL")

			got, err := original.Negate()
			if err != nil {
				t.Fatalf("Negate() returned an unexpected error: %v", err)
			}
			if got.Amount() != tt.wantAmount {
				t.Errorf("Negate() amount = %q; want %q", got.Amount(), tt.wantAmount)
			}
			if got.Currency() != CurrencyBRL {
				t.Errorf("Negate() currency = %q; want BRL", got.Currency())
			}
			if original.Amount() != tt.amount {
				t.Errorf("Negate() changed the receiver to %q; want it to remain %q", original.Amount(), tt.amount)
			}
		})
	}
}

func TestMoneyNegateRejectsUninitializedMoney(t *testing.T) {
	_, err := (Money{}).Negate()
	if !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("Negate() error = %v; want ErrInvalidMoney", err)
	}
	assertErrorContains(t, err, "negate money", "operand")
}

func TestMoneyNegateRejectsOverflow(t *testing.T) {
	minimum := mustNewMoney(t, "-92233720368547758.08", "BRL")

	_, err := minimum.Negate()
	if !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("Negate() error = %v; want ErrAmountOutOfRange", err)
	}
	assertErrorContains(t, err, "negate money", "value=-92233720368547758.08 BRL")
}

func assertErrorContains(t *testing.T, err error, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not contain %q", err, fragment)
		}
	}
}

func mustNewMoney(t *testing.T, amount, currency string) Money {
	t.Helper()

	money, err := NewMoney(amount, currency)
	if err != nil {
		t.Fatalf("NewMoney(%q, %q) returned an unexpected error: %v", amount, currency, err)
	}

	return money
}
