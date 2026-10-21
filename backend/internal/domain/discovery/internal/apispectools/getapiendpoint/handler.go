package getapiendpoint

import (
	"context"
	"fmt"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func newHandler(uc usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, aerr := Parse(call.Arguments)
		if aerr != nil {
			return base.ArgResult(aerr, base.TargetTypeEndpoint), nil
		}
		lk, err := uc.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolGetAPIEndpoint, err)
		}
		return lk.Result(), nil
	}
}
