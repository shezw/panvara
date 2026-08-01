/*
   Panvara
   internal/runtime/modelruntime/builder.go    2026-08-02
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package modelruntime

import (
	"context"
	"net/http"

	"github.com/shezw/panvara/internal/application/appmodule"
)

// Builder composes the complete immutable HTTP graph for one verified runtime
// revision and its independently selected Record persistence namespace.
// Implementations bind infrastructure dependencies in the composition root.
type Builder interface {
	Build(
		ctx context.Context,
		module *appmodule.CompiledModule,
		recordNamespaceRevision string,
	) (http.Handler, error)
}

// BuilderFunc adapts a function to Builder.
type BuilderFunc func(context.Context, *appmodule.CompiledModule, string) (http.Handler, error)

// Build calls function with the verified module and explicit Record namespace.
func (function BuilderFunc) Build(
	ctx context.Context,
	module *appmodule.CompiledModule,
	namespace string,
) (http.Handler, error) {
	return function(ctx, module, namespace)
}
