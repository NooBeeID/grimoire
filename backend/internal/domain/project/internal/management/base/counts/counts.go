// Package counts mengambil jumlah isi project (file/spec/key) lewat bus. Fail-soft:
// kegagalan apa pun menghasilkan hitungan nil, tidak pernah error.
package counts

import (
	"context"
	"errors"
	"log/slog"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

type Counter interface {
	ForProjects(ctx context.Context, ids []string) map[string]base.ContentCounts
}

type BusCounter struct {
	b   *bus.Registry
	log *slog.Logger
}

var _ Counter = (*BusCounter)(nil)

func New(b *bus.Registry, log *slog.Logger) *BusCounter {
	if log == nil {
		log = slog.Default()
	}
	return &BusCounter{b: b, log: log}
}

// ForProjects memanggil tiap provider sekali (batch). ids di-dedupe; kosong -> map kosong tanpa bus.
func (c *BusCounter) ForProjects(ctx context.Context, ids []string) map[string]base.ContentCounts {
	out := map[string]base.ContentCounts{}
	seen := make(map[string]struct{}, len(ids))
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out
	}
	files := c.fetch(ctx, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, uniq)
	specs := c.fetch(ctx, constants.BUS_PROJECT_APISPEC_COUNT_SPECS, uniq)
	keys := c.fetch(ctx, constants.BUS_PROJECT_APIKEY_COUNT_KEYS, uniq)
	for _, id := range uniq {
		out[id] = base.ContentCounts{Files: pick(files, id), Specs: pick(specs, id), Keys: pick(keys, id)}
	}
	return out
}

func pick(m map[string]int, id string) *int {
	if m == nil {
		return nil
	}
	v := m[id]
	return &v
}

// fetch mengembalikan nil bila provider tidak tersedia atau gagal.
func (c *BusCounter) fetch(ctx context.Context, endpoint string, ids []string) map[string]int {
	res, err := bus.CallTyped[contentcount.Response](ctx, c.b, endpoint, contentcount.Request{ProjectIDs: ids})
	if err != nil {
		if errors.Is(err, bus.ErrNotRegistered) {
			c.log.DebugContext(ctx, "content count belum tersedia", "endpoint", endpoint)
		} else {
			c.log.WarnContext(ctx, "content count gagal", "endpoint", endpoint, "error", err)
		}
		return nil
	}
	if res.Counts == nil {
		return map[string]int{}
	}
	return res.Counts
}
