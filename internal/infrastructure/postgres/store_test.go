/*
   Panvara
   internal/infrastructure/postgres/store_test.go    2026-07-14
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
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shezw/panvara/internal/application/record"
)

func TestConstructorsRejectNilPool(t *testing.T) {
	t.Parallel()
	if _, err := NewStore(nil); err == nil {
		t.Fatal("NewStore(nil) error = nil")
	}
	if err := Migrate(context.Background(), nil); err == nil {
		t.Fatal("Migrate(nil) error = nil")
	}
}

func TestTranslateWriteErrorMapsStableApplicationSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		code       string
		constraint string
		want       error
	}{
		{name: "record identity", code: "23505", constraint: "flex_record_pkey", want: record.ErrAlreadyExists},
		{name: "unique value", code: "23505", constraint: "flex_unique_scope_value_key", want: record.ErrUniqueConflict},
		{name: "reference target", code: "23503", constraint: "flex_reference_target_record_fk", want: record.ErrNotFound},
		{name: "check constraint", code: "23514", constraint: "flex_record_data_object_check", want: record.ErrInvalidArgument},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := translateWriteError("test", &pgconn.PgError{
				Code: test.code, ConstraintName: test.constraint,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("translateWriteError() error = %v, want %v", err, test.want)
			}
		})
	}
}
