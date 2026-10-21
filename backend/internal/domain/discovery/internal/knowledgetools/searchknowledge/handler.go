package searchknowledge

import (
	"context"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

func newHandler(u *usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, "folder", ""), nil
		}
		hits, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolSearchKnowledge, err, "Periksa path atau hapus filter path.", "folder", p.Prefix)
		}
		return hits.Result(), nil
	}
}
