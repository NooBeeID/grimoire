package listapiendpoints

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
		out, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolListAPIEndpoints, err)
		}
		return out.Result(), nil
	}
}
