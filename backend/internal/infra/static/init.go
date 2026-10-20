package static

import (
	"log/slog"
	"net/http"

	"grimoire/internal/infra/config"
)

// Register membuat handler static untuk dipasang sebagai NotFound root router
// (root.NotFound(h.ServeHTTP)). Dipanggil oleh wiring; fsys nil memakai Embedded().
func Register(cfg config.Config, log *slog.Logger) (http.Handler, error) {
	return New(cfg, nil, log)
}
