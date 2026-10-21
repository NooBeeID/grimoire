package renamespec

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
)

type renameRequest struct {
	Name *string `json:"name"`
}

// parseRename mendekode body dan memeriksa bentuk saja; format nama adalah rule entity.
func parseRename(r *http.Request) (string, error) {
	var req renameRequest
	if err := respond.DecodeJSON(r, &req); err != nil {
		return "", err
	}
	if req.Name == nil {
		return "", errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
			map[string]any{"fields": map[string]string{"name": "wajib diisi"}})
	}
	return *req.Name, nil
}
