// Package listrecentchanges adalah tool MCP list_recent_changes (T18).
package listrecentchanges

import (
	"time"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// Init merakit tool list_recent_changes.
func Init(d app.Deps) toolkit.Tool { return newTool(d, time.Now) }

func newTool(d app.Deps, now func() time.Time) toolkit.Tool {
	uc := newUsecase(busFiles{b: d.Bus}, busSpecs{b: d.Bus})
	return toolkit.Tool{
		Name:        base.ToolListRecentChanges,
		Title:       "Perubahan terbaru",
		Description: "Daftar file knowledge dan spec API yang berubah sejak waktu tertentu (read-only). Terbaru dulu; since wajib: RFC3339 atau durasi 24h, 7d, 30d (maks 90 hari). Setiap baris menandai dibuat atau diperbarui, tanpa isi dan tanpa identitas pengubah; file yang dihapus tidak muncul. Baca file yang berubah dengan get_file_outline atau read_file.",
		InputSchema: schema(),
		Handler:     newHandler(uc, now),
	}
}
