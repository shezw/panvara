/*
   Panvara
   internal/domain/project/context.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package project defines the project boundary shared by every request.
package project

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/domain/value"
)

var (
	localePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)
)

// Context carries project-local globalisation choices. It is explicit rather
// than stored in process globals so future multi-project routing remains safe.
type Context struct {
	id       ID
	key      Key
	locale   string
	timeZone string
	currency value.Currency
}

// NewContext validates project identity and globalisation defaults.
func NewContext(id, key, locale, timeZone, currency string) (Context, error) {
	locale = strings.TrimSpace(locale)
	timeZone = strings.TrimSpace(timeZone)

	projectID, err := ParseID(id)
	if err != nil {
		return Context{}, err
	}
	projectKey, err := ParseKey(key)
	if err != nil {
		return Context{}, err
	}
	if len(locale) > 128 {
		return Context{}, fmt.Errorf("locale is too long")
	}
	if !localePattern.MatchString(locale) {
		return Context{}, fmt.Errorf("invalid BCP 47 locale shape %q", locale)
	}
	if timeZone == "" {
		return Context{}, fmt.Errorf("time zone is required")
	}
	if timeZone == "Local" {
		return Context{}, fmt.Errorf("time zone Local is host-dependent; use an IANA name or UTC")
	}
	if len(timeZone) > 128 {
		return Context{}, fmt.Errorf("time zone is too long")
	}
	if _, err := time.LoadLocation(timeZone); err != nil {
		return Context{}, fmt.Errorf("invalid IANA time zone %q: %w", timeZone, err)
	}
	code, err := value.ParseCurrency(currency)
	if err != nil {
		return Context{}, err
	}

	return Context{
		id:       projectID,
		key:      projectKey,
		locale:   locale,
		timeZone: timeZone,
		currency: code,
	}, nil
}

// ID returns the stable project identity.
func (context Context) ID() ID {
	return context.id
}

// Key returns the readable project routing key.
func (context Context) Key() Key {
	return context.key
}

// Locale returns the structurally validated BCP 47 tag.
func (context Context) Locale() string {
	return context.locale
}

// TimeZone returns the validated IANA time zone name.
func (context Context) TimeZone() string {
	return context.timeZone
}

// Currency returns the validated default currency.
func (context Context) Currency() value.Currency {
	return context.currency
}
