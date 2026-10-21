package searchquality

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/scanentries"
)

// Scanner adalah port ke provider core.audit.scan-entries. Tidak ada SQL di feature ini.
type Scanner interface {
	Scan(ctx context.Context, req scanentries.Request) (scanentries.Response, error)
}

type busScanner struct{ b *bus.Registry }

// NewRepository membuat adapter bus ke core.audit.scan-entries.
func NewRepository(b *bus.Registry) Scanner { return &busScanner{b: b} }

func (s *busScanner) Scan(ctx context.Context, req scanentries.Request) (scanentries.Response, error) {
	return bus.CallTyped[scanentries.Response](ctx, s.b, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, req)
}
