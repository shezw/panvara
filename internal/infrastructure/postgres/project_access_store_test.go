/*
   Panvara
   internal/infrastructure/postgres/project_access_store_test.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const projectAccessTestID = "019f5c36-b322-7c52-9325-ec59f95c8fae"

func TestNewProjectAccessStoreRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewProjectAccessStore(nil); err == nil {
		t.Fatal("NewProjectAccessStore(nil) error = nil")
	}
	if _, err := newProjectAccessStore(&pgxpool.Pool{}, nil); err == nil {
		t.Fatal("newProjectAccessStore(pool, nil) error = nil")
	}
}

func TestValidateBootstrapInputNormalizesEnvironmentKey(t *testing.T) {
	t.Parallel()

	definition := mustProjectAccessContext(t, "demo", "en-US", "UTC", "USD")
	environmentKey, principalID, err := validateBootstrapInput(
		definition,
		"  PREVIEW-1  ",
		"bootstrap-admin",
	)
	if err != nil {
		t.Fatalf("validateBootstrapInput() error = %v", err)
	}
	if environmentKey != "preview-1" || principalID != "bootstrap-admin" {
		t.Fatalf("validateBootstrapInput() = %q, %q", environmentKey, principalID)
	}

	for _, test := range []struct {
		name          string
		environment   string
		principal     string
		useZeroConfig bool
	}{
		{name: "empty environment", environment: "", principal: "bootstrap-admin"},
		{name: "invalid environment", environment: "bad environment", principal: "bootstrap-admin"},
		{name: "empty principal", environment: "default"},
		{name: "invalid principal", environment: "default", principal: "bad principal"},
		{name: "zero project context", environment: "default", principal: "bootstrap-admin", useZeroConfig: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := definition
			if test.useZeroConfig {
				input = project.Context{}
			}
			if _, _, err := validateBootstrapInput(input, test.environment, test.principal); err == nil {
				t.Fatal("validateBootstrapInput() error = nil")
			}
		})
	}
}

func TestPersistedProjectMatchesEveryConfiguredSetting(t *testing.T) {
	t.Parallel()

	definition := mustProjectAccessContext(t, "demo", "en-US", "Asia/Shanghai", "CNY")
	persisted := persistedProject{
		key: "demo", locale: "en-US", timeZone: "Asia/Shanghai", currency: "CNY",
	}
	if err := persisted.matches(definition); err != nil {
		t.Fatalf("matches(equal) error = %v", err)
	}

	for _, changed := range []persistedProject{
		{key: "other", locale: "en-US", timeZone: "Asia/Shanghai", currency: "CNY"},
		{key: "demo", locale: "zh-CN", timeZone: "Asia/Shanghai", currency: "CNY"},
		{key: "demo", locale: "en-US", timeZone: "UTC", currency: "CNY"},
		{key: "demo", locale: "en-US", timeZone: "Asia/Shanghai", currency: "USD"},
	} {
		if err := changed.matches(definition); err == nil {
			t.Fatalf("matches(%+v) error = nil", changed)
		}
	}
}

func TestProjectAccessLockIDIsStableAndProjectScoped(t *testing.T) {
	t.Parallel()

	first, err := project.ParseID(projectAccessTestID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := project.ParseID("019f5c36-b322-7c52-9325-ec59f95c8faf")
	if err != nil {
		t.Fatal(err)
	}
	if projectAccessLockID(first) != projectAccessLockID(first) {
		t.Fatal("projectAccessLockID() is not stable")
	}
	if projectAccessLockID(first) == projectAccessLockID(second) {
		t.Fatal("projectAccessLockID() returned the same lock for two projects")
	}
}

func TestCredentialActiveRejectsInvalidEvidenceBeforeQuery(t *testing.T) {
	t.Parallel()

	store := &ProjectAccessStore{}
	if active, err := store.CredentialActive(
		context.Background(), project.Scope{}, "bootstrap-admin", domainaccess.ID{},
	); err == nil || active {
		t.Fatalf("CredentialActive(invalid evidence) = %t, %v", active, err)
	}
}

func mustProjectAccessContext(
	t *testing.T,
	key string,
	locale string,
	timeZone string,
	currency string,
) project.Context {
	t.Helper()
	definition, err := project.NewContext(projectAccessTestID, key, locale, timeZone, currency)
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}
	return definition
}
