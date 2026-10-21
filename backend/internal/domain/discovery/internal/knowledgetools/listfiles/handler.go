package listfiles

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const hintInvalid = "Periksa path; panggil list_files tanpa path untuk melihat akar."

func newHandler(u usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, "folder", ""), nil
		}
		page, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolListFiles, err, hintInvalid, "folder", p.Prefix)
		}
		return page.Result(), nil
	}
}
