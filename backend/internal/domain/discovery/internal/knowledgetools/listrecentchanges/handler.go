package listrecentchanges

import (
	"context"
	"time"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func newHandler(uc *Usecase, now func() time.Time) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments, now().UTC())
		if argErr != nil {
			return base.ArgResult(argErr, "", ""), nil
		}
		page, err := uc.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolListRecentChanges, err, "Periksa since dan path.", "", "")
		}
		return page.Result(), nil
	}
}
