/*
   Panvara
   internal/domain/actor/context_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package actor

import (
	"reflect"
	"testing"
)

const testProjectID = "019f5c36-b322-7c52-9325-ec59f95c8fae"

func TestAuthenticatedContextNormalizesRoles(t *testing.T) {
	t.Parallel()

	context, err := New(testProjectID, "actor:019f", []string{"project.editor", "project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	if context.Anonymous() ||
		!context.Valid() ||
		context.ActorID() != "actor:019f" ||
		context.ProjectID().String() != testProjectID {
		t.Fatalf("context = %+v", context)
	}
	if !context.HasRole("project.owner") || context.HasRole("project.viewer") {
		t.Fatalf("unexpected role lookup: %v", context.Roles())
	}
	if want := []string{"project.editor", "project.owner"}; !reflect.DeepEqual(context.Roles(), want) {
		t.Fatalf("roles = %v, want %v", context.Roles(), want)
	}
}

func TestAnonymousContextHasNoIdentityOrRoles(t *testing.T) {
	t.Parallel()

	context, err := NewAnonymous(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !context.Anonymous() || !context.Valid() || context.ActorID() != "" || len(context.Roles()) != 0 {
		t.Fatalf("anonymous context = %+v", context)
	}
}

func TestContextRejectsInvalidOrDuplicateRoles(t *testing.T) {
	t.Parallel()

	for _, roles := range [][]string{{"Admin"}, {"owner", "owner"}, {"bad role"}} {
		if _, err := New(testProjectID, "actor-1", roles); err == nil {
			t.Fatalf("New() accepted roles %v", roles)
		}
	}
}

func TestRolesReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	context, err := New(testProjectID, "actor-1", []string{"owner"})
	if err != nil {
		t.Fatal(err)
	}
	roles := context.Roles()
	roles[0] = "mutated"
	if context.Roles()[0] != "owner" {
		t.Fatal("Roles() leaked mutable context state")
	}
}

func TestZeroContextFailsClosedAsAnonymousAndInvalid(t *testing.T) {
	t.Parallel()

	var context Context
	if !context.Anonymous() {
		t.Fatal("zero Context must be anonymous")
	}
	if context.Valid() {
		t.Fatal("zero Context must not be valid")
	}
	if context.HasRole("owner") {
		t.Fatal("zero Context unexpectedly has a role")
	}
}
