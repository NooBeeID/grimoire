package listapispecs

import (
	"context"
	"fmt"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func newHandler(u usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, base.TargetTypeSpec), nil
		}
		cat, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolListAPISpecs, err)
		}
		return cat.Result(), nil
	}
}
