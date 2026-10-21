package base

import (
	"context"
	"encoding/json"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// ToolInvoker dijalankan endpoint untuk setiap tools/call yang bentuk params-nya valid.
// Implementasi (callaudit) WAJIB menulis tepat satu entri audit per panggilan dan tidak pernah panic.
type ToolInvoker interface {
	Invoke(ctx context.Context, p Principal, name string, args json.RawMessage) Outcome
}

// Outcome adalah hasil satu panggilan tool.
type Outcome struct {
	UnknownTool   bool
	Result        toolkit.Result // Text sudah dipotong
	Truncated     bool
	OriginalBytes int
	ErrorCode     string // diisi gateway: "internal" | "timeout" | "panic"; "" bila tidak ada
}
