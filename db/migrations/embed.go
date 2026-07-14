/*
   Panvara
   db/migrations/embed.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package migrations exposes immutable SQL migrations to infrastructure.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var embedded embed.FS

// Files returns the embedded, read-only migration filesystem.
func Files() fs.FS {
	return embedded
}
