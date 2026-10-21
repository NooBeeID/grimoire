// Package toolkit adalah kontrak pendaftaran tool MCP (provider wiring: T07 mcpgateway; pendaftar: T08, T09).
//
// Module tool membuat []toolkit.Tool dan domain/discovery/base.go meneruskannya ke mcpgateway.Register.
// Gateway tidak meng-import package tool dan sebaliknya. Urutan pendaftaran tidak berpengaruh.
// Validasi saat start (gagal = panic "mcpgateway: ..."):
//   - Name cocok ^[a-z][a-z0-9_]{0,63}$, bukan nama terlarang (initialize, tools_list, unknown_tool, auth, rate_limited),
//     unik, dan "mcp."+Name WAJIB ada di record.Actions().
//   - InputSchema JSON object dengan "type":"object". Handler tidak nil.
//
// Gateway menambahkan annotations.readOnlyHint=true; tool tidak bisa mengubahnya.
// Tool DILARANG menulis audit sendiri (rules 9.3); gateway menulis tepat satu entri mcp.<Name> per panggilan.
package toolkit

import (
	"context"
	"encoding/json"
)

// Tool mendeskripsikan satu tool MCP beserta handler-nya.
type Tool struct {
	Name        string          // ^[a-z][a-z0-9_]{0,63}$
	Title       string          // opsional, judul untuk manusia
	Description string          // memandu LLM (rules 9.5)
	InputSchema json.RawMessage // JSON Schema object, wajib "type":"object"
	Handler     Handler
}

// Handler menjalankan tool. ctx membawa deadline MCP_TOOL_TIMEOUT dan trace_id; tool WAJIB menghormati ctx.
// Error Go = kegagalan internal (gateway menyembunyikan detailnya). Kegagalan yang berguna bagi LLM
// (argumen salah, file tidak ada) dikembalikan sebagai Result{IsError: true, Text: "..."} dengan err nil.
type Handler func(ctx context.Context, call Call) (Result, error)

// Call adalah masukan Handler.
type Call struct {
	ProjectID string          // SELALU dari context gateway (Principal), tidak pernah dari argumen
	Arguments json.RawMessage // object JSON apa adanya ("{}" bila klien tidak mengirim arguments)
}

// Result adalah keluaran Handler.
type Result struct {
	Text       string         // konten teks untuk LLM (dipotong gateway bila > MCP_MAX_RESPONSE_BYTES)
	IsError    bool           // kegagalan tool, pesan berguna bagi LLM
	TargetType string         // audit, mis. "file", "api_spec"
	Target     string         // audit, mis. path
	Summary    map[string]any // metadata saja (count, bytes, error_code); DILARANG berisi isi file
	// Structured opsional, menjadi structuredContent hasil MCP. Wajib JSON object (map dengan nilai
	// yang bisa di-marshal), berisi metadata/daftar saja. Tidak masuk audit dan tidak dipotong gateway;
	// tool bertanggung jawab menjaga ukurannya kecil (daftar dibatasi limit tool).
	Structured map[string]any
}
