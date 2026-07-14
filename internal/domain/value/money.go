/*
   Panvara
   internal/domain/value/money.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package value contains global-safe domain primitives.
package value

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrAmountOverflow   = errors.New("money amount overflow")
	ErrInvalidMoney     = errors.New("money has no valid currency")
)

// Currency is an uppercase ISO 4217-style three-letter code. The alpha-1 Core
// validates shape; an updatable currency catalog will validate membership.
type Currency struct {
	code string
}

// ParseCurrency normalizes and validates a currency code.
func ParseCurrency(value string) (Currency, error) {
	code := strings.ToUpper(strings.TrimSpace(value))
	if len(code) != 3 {
		return Currency{}, fmt.Errorf("currency must have exactly three letters: %q", value)
	}
	for _, char := range code {
		if char < 'A' || char > 'Z' {
			return Currency{}, fmt.Errorf("currency must contain ASCII letters only: %q", value)
		}
	}
	return Currency{code: code}, nil
}

// String returns the normalized currency code.
func (currency Currency) String() string {
	return currency.code
}

// Valid reports whether the value was created by ParseCurrency.
func (currency Currency) Valid() bool {
	_, err := ParseCurrency(currency.code)
	return err == nil
}

// Money stores an amount in the currency's minor unit. Floating point values
// are intentionally excluded from the domain.
type Money struct {
	minor    int64
	currency Currency
}

// NewMoney validates a currency and creates a monetary value.
func NewMoney(minor int64, currency string) (Money, error) {
	code, err := ParseCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: minor, currency: code}, nil
}

// Minor returns the amount in the currency's minor unit.
func (money Money) Minor() int64 {
	return money.minor
}

// Currency returns the validated currency.
func (money Money) Currency() Currency {
	return money.currency
}

// Add adds values of the same currency with overflow protection.
func (money Money) Add(other Money) (Money, error) {
	if !money.currency.Valid() || !other.currency.Valid() {
		return Money{}, ErrInvalidMoney
	}
	if money.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (other.minor > 0 && money.minor > math.MaxInt64-other.minor) ||
		(other.minor < 0 && money.minor < math.MinInt64-other.minor) {
		return Money{}, ErrAmountOverflow
	}
	money.minor += other.minor
	return money, nil
}

// Sub subtracts values of the same currency with overflow protection.
func (money Money) Sub(other Money) (Money, error) {
	if !money.currency.Valid() || !other.currency.Valid() {
		return Money{}, ErrInvalidMoney
	}
	if money.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (other.minor > 0 && money.minor < math.MinInt64+other.minor) ||
		(other.minor < 0 && money.minor > math.MaxInt64+other.minor) {
		return Money{}, ErrAmountOverflow
	}
	money.minor -= other.minor
	return money, nil
}
