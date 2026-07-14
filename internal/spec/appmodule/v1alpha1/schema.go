/*
   Panvara
   internal/spec/appmodule/v1alpha1/schema.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package v1alpha1

import _ "embed"

//go:embed schema.json
var authoringSchema []byte

// AuthoringSchema returns a defensive copy of the AppModule JSON Schema.
func AuthoringSchema() []byte {
	return append([]byte(nil), authoringSchema...)
}
