/*
   Panvara
   internal/domain/value/money_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package value

import (
	"errors"
	"math"
	"testing"
)

func TestParseCurrencyNormalizesASCIICode(t *testing.T) {
	t.Parallel()

	currency, err := ParseCurrency(" usd ")
	if err != nil {
		t.Fatal(err)
	}
	if currency.String() != "USD" {
		t.Fatalf("ParseCurrency() = %q, want USD", currency)
	}
}

func TestParseCurrencyRejectsInvalidCodes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"US", "US12", "美金", "EU€"} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseCurrency(value); err == nil {
				t.Fatalf("ParseCurrency(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestMoneyArithmetic(t *testing.T) {
	t.Parallel()

	left, _ := NewMoney(125, "USD")
	right, _ := NewMoney(75, "USD")
	sum, err := left.Add(right)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Minor() != 200 || sum.Currency().String() != "USD" {
		t.Fatalf("Add() = %+v, want 200 USD", sum)
	}
	difference, err := sum.Sub(right)
	if err != nil {
		t.Fatal(err)
	}
	if difference != left {
		t.Fatalf("Sub() = %+v, want %+v", difference, left)
	}
}

func TestMoneyRejectsCrossCurrencyArithmetic(t *testing.T) {
	t.Parallel()

	usd, _ := NewMoney(1, "USD")
	eur, _ := NewMoney(1, "EUR")
	if _, err := usd.Add(eur); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add() error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMoneyDetectsOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  Money
		right Money
		sub   bool
	}{
		{name: "add positive", left: mustMoney(t, math.MaxInt64, "USD"), right: mustMoney(t, 1, "USD")},
		{name: "add negative", left: mustMoney(t, math.MinInt64, "USD"), right: mustMoney(t, -1, "USD")},
		{name: "subtract positive", left: mustMoney(t, math.MinInt64, "USD"), right: mustMoney(t, 1, "USD"), sub: true},
		{name: "subtract negative", left: mustMoney(t, math.MaxInt64, "USD"), right: mustMoney(t, -1, "USD"), sub: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var err error
			if test.sub {
				_, err = test.left.Sub(test.right)
			} else {
				_, err = test.left.Add(test.right)
			}
			if !errors.Is(err, ErrAmountOverflow) {
				t.Fatalf("error = %v, want ErrAmountOverflow", err)
			}
		})
	}
}

func TestMoneyZeroValueIsInvalid(t *testing.T) {
	t.Parallel()

	valid, _ := NewMoney(1, "USD")
	if _, err := (Money{}).Add(valid); !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("Add() error = %v, want ErrInvalidMoney", err)
	}
}

func mustMoney(t *testing.T, minor int64, currency string) Money {
	t.Helper()
	money, err := NewMoney(minor, currency)
	if err != nil {
		t.Fatal(err)
	}
	return money
}
