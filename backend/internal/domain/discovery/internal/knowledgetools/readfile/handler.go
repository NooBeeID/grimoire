package readfile

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func newHandler(uc *Usecase, maxLimit int) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments, maxLimit)
		if argErr != nil {
			return base.ArgResult(argErr, "file", ""), nil
		}
		out, err := uc.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolReadFile, err,
				"Gunakan offset dari penanda terpotong atau 0.", "file", p.Path)
		}
		return out.Result(), nil
	}
}
