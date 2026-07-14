/*
   Panvara
   internal/application/record/service_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package record

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

func TestServiceCreateNormalizesAndPersistsValidatorOutput(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	recordID := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	targetID := testRecordID(t, "01981234-5678-7abc-8def-0123456789ad")
	at := time.Date(2026, time.July, 14, 8, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	validator := validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
		if input.Scope != scope {
			t.Fatalf("Validate() scope = %#v, want %#v", input.Scope, scope)
		}
		if input.Surface != SurfaceAdmin || input.Mutation != MutationCreate {
			t.Fatalf("Validate() policy = %q/%q", input.Surface, input.Mutation)
		}
		return ValidatedData{
			Data: []byte(" { \"name\" : \"Ada\" } \n"),
			Uniques: []UniqueValue{
				{Field: "phone", CanonicalValue: "phone:+12025550123"},
				{Field: "email", CanonicalValue: "email:ada@example.com"},
			},
			References: []Reference{{Field: "owner", TargetResource: "user", TargetID: targetID}},
		}, nil
	})
	service, err := NewService(store, validator, fixedClock{at: at}, fixedIDGenerator{id: recordID})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := service.Create(
		context.Background(), scope, SurfaceAdmin, json.RawMessage(`{"ignored":"source"}`),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != recordID || created.Version != 1 {
		t.Fatalf("Create() = %#v", created)
	}
	if got := string(store.created.Data); got != `{"name":"Ada"}` {
		t.Fatalf("CreateCommand.Data = %s", got)
	}
	if got := store.created.Uniques[0].Field; got != "email" {
		t.Fatalf("CreateCommand.Uniques first field = %q, want sorted email", got)
	}
	if store.created.At != at || store.created.Scope != scope || store.created.ID != recordID {
		t.Fatalf("CreateCommand = %#v", store.created)
	}
}

func TestServiceUpdatePreservesOptimisticConflict(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	id := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	store := &fakeStore{updateErr: ErrVersionConflict}
	validator := validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
		return ValidatedData{Data: input.Data}, nil
	})
	service, err := NewService(
		store,
		validator,
		fixedClock{at: time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)},
		fixedIDGenerator{id: id},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(
		context.Background(), scope, id, 3, SurfacePublic, json.RawMessage(`{"name":"Grace"}`),
	)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("Update() error = %v, want ErrVersionConflict", err)
	}
	if store.updated.ExpectedVersion != 3 {
		t.Fatalf("UpdateCommand.ExpectedVersion = %d, want 3", store.updated.ExpectedVersion)
	}
}

func TestServiceListAppliesBoundedDefault(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	store := &fakeStore{}
	service, err := NewService(
		store,
		validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
			return ValidatedData{Data: input.Data}, nil
		}),
		fixedClock{},
		fixedIDGenerator{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := service.List(context.Background(), scope, SurfaceAdmin, ListOptions{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.listed.Limit != DefaultListLimit {
		t.Fatalf("ListOptions.Limit = %d, want %d", store.listed.Limit, DefaultListLimit)
	}
	if _, err := service.List(
		context.Background(), scope, SurfaceAdmin, ListOptions{Limit: MaxListLimit + 1},
	); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("List() error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceListSortsFiltersAndBindsCursor(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	id := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	at := time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)
	store := &fakeStore{listResult: ListResult{
		Next: &ListCursor{CreatedAt: at, ID: id},
	}}
	service, err := NewService(
		store,
		validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
			return ValidatedData{Data: input.Data}, nil
		}),
		fixedClock{},
		fixedIDGenerator{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.List(context.Background(), scope, SurfacePublic, ListOptions{
		Filters: []ListFilter{
			{Field: "status", Value: "open"},
			{Field: "email", Value: "ada@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.listed.Filters[0].Field != "email" || store.listed.Filters[1].Field != "status" {
		t.Fatalf("ListOptions.Filters = %#v, want deterministic order", store.listed.Filters)
	}
	if result.Next == nil || result.Next.QueryHash == "" {
		t.Fatalf("ListResult.Next = %#v, want query-bound cursor", result.Next)
	}

	wrongCursor := *result.Next
	wrongCursor.QueryHash = "sha256:" + strings.Repeat("0", 64)
	if _, err := service.List(context.Background(), scope, SurfacePublic, ListOptions{
		Cursor: &wrongCursor,
		Filters: []ListFilter{
			{Field: "status", Value: "open"},
			{Field: "email", Value: "ada@example.com"},
		},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("List(cursor mismatch) error = %v, want ErrInvalidArgument", err)
	}
	otherScope := testScope(t, "contact")
	if _, err := service.List(context.Background(), otherScope, SurfacePublic, ListOptions{
		Cursor: result.Next,
		Filters: []ListFilter{
			{Field: "status", Value: "open"},
			{Field: "email", Value: "ada@example.com"},
		},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("List(cross-scope cursor) error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceUpdateMergesTopLevelPatchBeforeValidation(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	id := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	store := &fakeStore{getRecord: Record{
		Scope: scope, ID: id, Version: 2,
		Data: json.RawMessage(`{"name":"Ada","remove":"yes","profile":{"city":"London"}}`),
	}}
	validator := validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
		if input.Surface != SurfacePublic || input.Mutation != MutationPatch {
			t.Fatalf("Validate() policy = %q/%q", input.Surface, input.Mutation)
		}
		if got := string(input.Data); got != `{"name":"Grace","profile":{"country":"US"},"remove":"yes"}` {
			t.Fatalf("Validate() data = %s", got)
		}
		if got := string(input.ExistingData); got != `{"name":"Ada","remove":"yes","profile":{"city":"London"}}` {
			t.Fatalf("Validate() existing data = %s", got)
		}
		if got := string(input.PatchData); got != `{"name":"Grace","profile":{"country":"US"}}` {
			t.Fatalf("Validate() patch data = %s", got)
		}
		return ValidatedData{Data: input.Data}, nil
	})
	service, err := NewService(
		store,
		validator,
		fixedClock{at: time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)},
		fixedIDGenerator{id: id},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Update(
		context.Background(), scope, id, 2, SurfacePublic,
		json.RawMessage(`{"name":"Grace","profile":{"country":"US"}}`),
	)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestServiceUpdateRejectsExplicitNull(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	id := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	store := &fakeStore{getRecord: Record{
		Scope: scope, ID: id, Version: 2, Data: json.RawMessage(`{"name":"Ada"}`),
	}}
	service, err := NewService(
		store,
		validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
			return ValidatedData{Data: input.Data}, nil
		}),
		fixedClock{},
		fixedIDGenerator{id: id},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	_, err = service.Update(
		context.Background(), scope, id, 2, SurfacePublic, json.RawMessage(`{"name":null}`),
	)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Update() error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceWriteSurfaceFailsClosed(t *testing.T) {
	t.Parallel()
	scope := testScope(t, "lead")
	id := testRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	service, err := NewService(
		&fakeStore{},
		validatorFunc(func(_ context.Context, input ValidationInput) (ValidatedData, error) {
			return ValidatedData{Data: input.Data}, nil
		}),
		fixedClock{},
		fixedIDGenerator{id: id},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := service.Create(context.Background(), scope, "", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Create() error = %v, want ErrInvalidArgument", err)
	}
}

func testScope(t *testing.T, resource string) Scope {
	t.Helper()
	projectID, err := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	if err != nil {
		t.Fatalf("ParseID() error = %v", err)
	}
	scope, err := NewScope(projectID, "crm.leads", resource, "sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}

func testRecordID(t *testing.T, value string) ID {
	t.Helper()
	id, err := ParseID(value)
	if err != nil {
		t.Fatalf("ParseID() error = %v", err)
	}
	return id
}

type validatorFunc func(context.Context, ValidationInput) (ValidatedData, error)

func (function validatorFunc) Validate(ctx context.Context, input ValidationInput) (ValidatedData, error) {
	return function(ctx, input)
}

func (validatorFunc) ValidateList(_ context.Context, input ListValidationInput) ([]ListFilter, error) {
	return append([]ListFilter(nil), input.Filters...), nil
}

type fixedClock struct {
	at time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.at
}

type fixedIDGenerator struct {
	id  ID
	err error
}

func (generator fixedIDGenerator) New(time.Time) (ID, error) {
	return generator.id, generator.err
}

type fakeStore struct {
	created    CreateCommand
	updated    UpdateCommand
	deleted    DeleteCommand
	listed     ListOptions
	listResult ListResult
	updateErr  error
	getRecord  Record
}

func (store *fakeStore) Create(_ context.Context, command CreateCommand) (Record, error) {
	store.created = command
	return Record{
		Scope: command.Scope, ID: command.ID, Version: 1, Data: command.Data,
		CreatedAt: command.At, UpdatedAt: command.At,
	}, nil
}

func (store *fakeStore) Get(_ context.Context, scope Scope, id ID) (Record, error) {
	if store.getRecord.ID.Valid() {
		return store.getRecord, nil
	}
	return Record{Scope: scope, ID: id, Version: 3, Data: json.RawMessage(`{"name":"Ada"}`)}, nil
}

func (store *fakeStore) List(_ context.Context, _ Scope, options ListOptions) (ListResult, error) {
	store.listed = options
	return store.listResult, nil
}

func (store *fakeStore) Update(_ context.Context, command UpdateCommand) (Record, error) {
	store.updated = command
	if store.updateErr != nil {
		return Record{}, store.updateErr
	}
	return Record{
		Scope: command.Scope, ID: command.ID, Version: command.ExpectedVersion + 1,
		Data: command.Data, UpdatedAt: command.At,
	}, nil
}

func (store *fakeStore) Delete(_ context.Context, command DeleteCommand) (Record, error) {
	store.deleted = command
	deletedAt := command.At
	return Record{
		Scope: command.Scope, ID: command.ID, Version: command.ExpectedVersion + 1,
		UpdatedAt: command.At, DeletedAt: &deletedAt,
	}, nil
}
