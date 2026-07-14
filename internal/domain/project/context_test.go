/*
   Panvara
   internal/domain/project/context_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package project

import "testing"

const testProjectID = "019f5c36-b322-7c52-9325-ec59f95c8fae"

func TestNewContextAcceptsGlobalDefaults(t *testing.T) {
	t.Parallel()

	context, err := NewContext(testProjectID, "demo-cn", "zh-Hans-CN", "Asia/Shanghai", "cny")
	if err != nil {
		t.Fatal(err)
	}
	if context.ID().String() != testProjectID ||
		context.Key().String() != "demo-cn" ||
		context.Currency().String() != "CNY" {
		t.Fatalf("NewContext() = %+v", context)
	}
}

func TestNewContextAcceptsBCP47Extensions(t *testing.T) {
	t.Parallel()

	if _, err := NewContext(
		testProjectID,
		"calendar-demo",
		"en-US-u-ca-gregory",
		"America/New_York",
		"USD",
	); err != nil {
		t.Fatalf("NewContext() rejected a structurally valid BCP 47 extension: %v", err)
	}
}

func TestNewContextRejectsInvalidGlobalDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       string
		key      string
		locale   string
		timeZone string
		currency string
	}{
		{name: "project id", id: "not-a-uuid", key: "demo", locale: "en-US", timeZone: "UTC", currency: "USD"},
		{name: "project key", id: testProjectID, key: "../escape", locale: "en-US", timeZone: "UTC", currency: "USD"},
		{name: "locale", id: testProjectID, key: "demo", locale: "en--US", timeZone: "UTC", currency: "USD"},
		{name: "time zone", id: testProjectID, key: "demo", locale: "en-US", timeZone: "Moon/Base", currency: "USD"},
		{name: "host local time zone", id: testProjectID, key: "demo", locale: "en-US", timeZone: "Local", currency: "USD"},
		{name: "currency", id: testProjectID, key: "demo", locale: "en-US", timeZone: "UTC", currency: "US"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewContext(test.id, test.key, test.locale, test.timeZone, test.currency); err == nil {
				t.Fatal("NewContext() unexpectedly succeeded")
			}
		})
	}
}
