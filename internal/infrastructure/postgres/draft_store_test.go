/*
   Panvara
   internal/infrastructure/postgres/draft_store_test.go    2026-07-16
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

import "testing"

func TestDraftStoreRejectsNilPool(t *testing.T) {
	t.Parallel()
	if _, err := NewDraftStore(nil); err == nil {
		t.Fatal("NewDraftStore(nil) error = nil")
	}
}
