package domain

import (
	"errors"
	"testing"
)

func TestMoneyCompare(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "less than", left: "10.00", right: "25.00", want: -1},
		{name: "equal", left: "25.00", right: "25.00", want: 0},
		{name: "greater than", left: "25.00", right: "10.00", want: 1},
		{name: "negative less than positive", left: "-1.00", right: "1.00", want: -1},
		{name: "more negative is less", left: "-25.00", right: "-10.00", want: -1},
		{name: "minimum is less than maximum", left: "-92233720368547758.08", right: "92233720368547758.07", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustNewMoney(t, tt.left, "BRL")
			right := mustNewMoney(t, tt.right, "BRL")

			got, err := left.Compare(right)
			if err != nil {
				t.Fatalf("Compare() returned an unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Compare(%s, %s) = %d; want %d", left.formatForError(), right.formatForError(), got, tt.want)
			}
			if left.Amount() != tt.left {
				t.Errorf("Compare() changed the receiver to %q; want it to remain %q", left.Amount(), tt.left)
			}
			if right.Amount() != tt.right {
				t.Errorf("Compare() changed the argument to %q; want it to remain %q", right.Amount(), tt.right)
			}
		})
	}
}

func TestMoneyEqualsRequiresSameAmountAndCurrency(t *testing.T) {
	tests := []struct {
		name  string
		left  Money
		right Money
		want  bool
	}{
		{name: "same amount and currency", left: mustNewMoney(t, "25.00", "BRL"), right: mustNewMoney(t, "25.00", "BRL"), want: true},
		{name: "different amount", left: mustNewMoney(t, "25.00", "BRL"), right: mustNewMoney(t, "25.01", "BRL"), want: false},
		{name: "different currency", left: mustNewMoney(t, "25.00", "BRL"), right: mustNewMoney(t, "25.00", "USD"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.left.equals(tt.right); got != tt.want {
				t.Errorf("Money.equals() = %t; want %t", got, tt.want)
			}
		})
	}
}

func TestMoneyCompareRejectsCurrencyMismatch(t *testing.T) {
	brl := mustNewMoney(t, "10.00", "BRL")
	usd := mustNewMoney(t, "5.00", "USD")

	_, err := brl.Compare(usd)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Compare() error = %v; want ErrCurrencyMismatch", err)
	}
	assertErrorContains(t, err, "compare money", "left=10.00 BRL", "right=5.00 USD")
}

func TestMoneyCompareRejectsUninitializedMoney(t *testing.T) {
	valid := mustNewMoney(t, "10.00", "BRL")

	tests := []struct {
		name         string
		left         Money
		right        Money
		wantFragment string
	}{
		{name: "uninitialized receiver", left: Money{}, right: valid, wantFragment: "left operand"},
		{name: "uninitialized argument", left: valid, right: Money{}, wantFragment: "right operand"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.left.Compare(tt.right)
			if !errors.Is(err, ErrInvalidMoney) {
				t.Fatalf("Compare() error = %v; want ErrInvalidMoney", err)
			}
			assertErrorContains(t, err, "compare money", tt.wantFragment)
		})
	}
}
