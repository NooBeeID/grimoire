# T07 discovery/mcp-gateway — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T07-discovery-mcp-gateway.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `.claude/rules/migrations.md`, `docs/tickets/README.md` (konvensi, katalog bus, katalog aksi audit, pola fail-open), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/project-api-key/overview.md` (T04: kontrak `verify`/`touch`, pola `base/` dengan subpackage I/O).
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/` hanya berisi `migrations.md` (tidak relevan, T07 tanpa migrasi). Tidak ada knowledge doc module mcp-gateway. Kode T00-T04 sudah di `main`: `lib/contracts/project/apikey/{verify,touch}`, `lib/contracts/project/management/getproject`, `lib/contracts/core/audit/record`, `infra/middleware/{ratelimit,origin,bodylimit}.go`, `infra/http/router.go` (sub-router `d.MCP` sudah di-mount di `/mcp` tanpa CSRF/session), `internal/domain/discovery/base.go` (masih no-op).

Dokumen ini adalah plan FOUNDATION. Isinya keputusan bersama, `base/`, kontrak, perubahan infra/lib, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/discovery-mcp-gateway/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila bertentangan, dokumen ini menang sampai diperbarui.

---

## 0. Prasyarat dan simbol yang dipakai

Signature publik yang sudah ada TIDAK diubah. T07 hanya MENAMBAH: field config, method `Limiter.Peek`, field `Deps.Version`, tiga konstanta aksi audit, satu paket kontrak netral (`toolkit`), dan module `mcpgateway`.

| Simbol | Import path | Package |
|---|---|---|
| `Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` (+ `Version`, langkah F3) | `grimoire/internal/infra/app` | `app` |
| `Registry.Call`, `Registry.Publish`, `NewRegistry`, `CallTyped[T]` | `grimoire/internal/infra/bus` | `bus` |
| `OriginGuard(allowed)`, `BodyLimit(n)`, `NewLimiter(ctx, Rule)`, `Rule{Name, Rate, Burst, KeyFunc}`, `Limiter.Allow`, `Limiter.Peek` (baru), `KeyByIP(r)` | `grimoire/internal/infra/middleware` | `middleware` |
| `Config` (+5 field, langkah F1) | `grimoire/internal/infra/config` | `config` |
| `TraceID(ctx)`, `WithAPIKeyID(ctx, id)`, `APIKeyID(ctx)` | `grimoire/lib/common/reqctx` | `reqctx` |
| `Request`, `Response`, `ActorAPIKey`, `ActorAnonymous`, `StatusOK`, `StatusError`, `StatusDenied`, `Actions()`, `ActionMCPAuthRejected`, `ActionMCPRateLimited`, `ActionMCPInitialize`/`ActionMCPToolsList`/`ActionMCPUnknownTool` (baru) | `grimoire/lib/contracts/core/audit/record` | `record` |
| `verify.Request{RawKey}`, `verify.Response{Found, KeyID, KeyName, Prefix, ProjectID, Reason}`, `ReasonMalformed/Unknown/Revoked/Mismatch` | `grimoire/lib/contracts/project/apikey/verify` | `verify` |
| `touch.Request{KeyID}` | `grimoire/lib/contracts/project/apikey/touch` | `touch` |
| `getproject.Request{ProjectID}`, `getproject.Response{Found, ID, Name}` | `grimoire/lib/contracts/project/management/getproject` | `getproject` |
| `BUS_PROJECT_APIKEY_VERIFY`, `BUS_PROJECT_APIKEY_TOUCH`, `BUS_PROJECT_MANAGEMENT_GET_PROJECT`, `BUS_CORE_AUDIT_RECORD` | `grimoire/lib/constants` | `constants` |
| `Version` | `grimoire/cmd/internal/buildinfo` | `buildinfo` (hanya dipakai di `cmd/internal/server`) |
| `rate.Limit`, `rate.Limiter.TokensAt` | `golang.org/x/time/rate` (sudah di `go.mod`) | `rate` |

- Tidak ada library baru; `go.mod` tidak berubah. Tidak ada library MCP (K2).
- Tidak ada tabel, tidak ada migrasi, tidak ada job terjadwal, tidak ada cache.
- Tidak ada kode error REST baru (`errs.Code`); semua error MCP memakai kode JSON-RPC di `base/jsonrpc.go`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T07 |
| Scope | fullstack (backend: plan ini + plan usecase; frontend: hanya katalog aksi + e2e, bagian 11) |
| Domain | discovery |
| Module | mcp-gateway (`mcpgateway`) |
| Resource root | `backend/internal/domain/discovery/internal/mcpgateway` |
| Kontrak REST | Tidak ada. `/mcp` bukan REST `/api` (rules 11); tidak ada `api/modules/**` baru. endpointsChanged = false |
| Versi protokol MCP | `2025-11-25`, `2025-06-18`, `2025-03-26` (dinegosiasikan saat `initialize`; default header `2025-03-26`) |
| Versi JSON-RPC | 2.0 (single message; batch ditolak) |
| Bergantung pada | T02 (merged), T04 (merged), T03 (merged, `get-project`) |
| Dibutuhkan oleh | T08, T09 (mendaftarkan tool lewat `toolkit`) |

Endpoint HTTP (sub-router `d.MCP`, sudah di-mount di `/mcp` oleh T00, tanpa CSRF/session):

| Endpoint | Method | Version | Middleware (urut) | Usecase |
|---|---|---|---|---|
| `/mcp` | POST | MCP 2025-11-25 / 2025-06-18 / 2025-03-26 | `OriginGuard(MCP_ALLOWED_ORIGINS)` -> `BodyLimit(MCP_MAX_REQUEST_BYTES)` -> `admit` (authenticate) | `endpoint` (+ `authenticate`, `callaudit`) |
| `/mcp` | GET | - | tanpa auth | `endpoint` -> 405 `Allow: POST` |
| `/mcp` | DELETE | - | tanpa auth | `endpoint` -> 405 `Allow: POST` |

Metode JSON-RPC di `POST /mcp`:

| Method | Jenis | Audit | Hasil |
|---|---|---|---|
| `initialize` | request | `mcp.initialize` | `protocolVersion`, `capabilities`, `serverInfo`, `instructions` |
| `notifications/initialized` (dan notification lain) | notification | tidak | HTTP 202 tanpa body |
| `ping` | request | tidak | `{}` |
| `tools/list` | request | `mcp.tools_list` | `{tools:[...]}` |
| `tools/call` | request | tepat 1: `mcp.<tool>` atau `mcp.unknown_tool` | `{content, isError, _meta?}` |
| lainnya | request | tidak | `-32601` |

Endpoint bus yang dikonsumsi (tidak ada endpoint/konstanta bus baru):

| Endpoint | Mode | Provider | Sifat di T07 | Dipakai oleh |
|---|---|---|---|---|
| `project.apikey.verify` | Call | T04 | fail-closed: error -> HTTP 503 `-32004`, log error, tanpa audit | `authenticate` |
| `project.apikey.touch` | Publish | T04 | error di-log warn | `authenticate` |
| `project.management.get-project` | Call | T03 | fail-soft: error/`Found=false` -> label `""`, log warn | `authenticate` |
| `core.audit.record` | Call (sinkron, rules 6.3) | T02 | fail-open: log error (tanpa Args/Result) | `authenticate`, `endpoint`, `callaudit` (lewat `base/auditwriter`) |

Mekanisme pendaftaran tool (bukan endpoint runtime): paket kontrak netral `lib/contracts/discovery/mcpgateway/toolkit` + wiring di `internal/domain/discovery/base.go` (baris katalog README `discovery.mcpgateway.register-tool | wiring`).

---

## 2. Summary Plan

T07 membangun pintu masuk agent: server MCP Streamable HTTP mode stateless di `POST /mcp`, ditulis sendiri (JSON-RPC 2.0, 5 metode, tanpa sesi, tanpa SSE). Setiap request:

1. Lolos `OriginGuard` (Origin asing -> 403, AC9) dan `BodyLimit` (lazy, 64 KB default).
2. Diautentikasi `authenticate`: key dari `Authorization: Bearer` atau `X-API-Key`, diverifikasi `project.apikey.verify` TANPA cache (AC5), gagal -> 401 seragam + audit `mcp.auth.rejected` (AC2). Rate limit per IP untuk gagal-auth (pre-check `Peek`) dan per key (AC6) -> 429 + `Retry-After` + audit `mcp.rate_limited` (dibatasi 1 entri/60 detik/subjek). Sukses -> `Principal` (key + project) di context, `touch` di-Publish.
3. Diproses `endpoint`: validasi transport (Content-Type, header versi, parse, batch), dispatch metode, audit ringan `mcp.initialize`/`mcp.tools_list`.
4. `tools/call` didelegasikan ke `callaudit` (implementasi `base.ToolInvoker`): lookup katalog, timeout per panggilan, recover panic, truncation respons (AC7), dan TEPAT satu entri audit `mcp.<tool>` tanpa isi respons (AC4). `ProjectID` tool selalu dari `Principal`, tidak pernah dari argumen (AC3).

Tool konkret belum ada di T07 (daftar kosong). T08/T09 mendaftarkan tool lewat `toolkit.Tool` dengan satu baris di `domain/discovery/base.go`; gateway tidak meng-import package tool dan sebaliknya. Nama tool duplikat atau tidak valid -> panic saat start dengan pesan berisi nama tool (AC8).

Tidak ada tabel, migrasi, OpenAPI, atau UI baru. Perubahan pendukung: 5 env baru, `Limiter.Peek`, `Deps.Version`, 3 aksi audit baru, dokumen `docs/mcp-gateway.md`, katalog README, filter aksi audit di frontend, dan e2e.

---

## 3. Keputusan bersama (mengikat semua usecase)

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Tiga feature: `authenticate`, `endpoint`, `callaudit`. Fitur tiket `toolregistry` menjadi foundation (`toolkit` + `base/catalog.go` + wiring), `ratelimit` digabung ke `authenticate` | `toolregistry` tidak punya handler/usecase. Limiter IP dipakai saat auth gagal, limiter key butuh identitas hasil verify, dan keduanya berbagi body penolakan + audit. Dipisah berarti feature saling import (dilarang 4.3) |
| K2 | JSON-RPC + MCP Streamable HTTP stateless ditulis sendiri, tanpa library MCP | Cakupan kecil; kontrol penuh atas audit, truncation, auth context; tanpa dependensi baru (1.5, 15.4) |
| K3 | Komposisi feature hanya di `mcpgateway/base.go` lewat tipe di `base/` (`ToolInvoker`, `Catalog`, `Principal`) dan `func(http.Handler) http.Handler` | Rules 4.3 |
| K4 | Basic Auth TIDAK didukung. Header: `Authorization: Bearer <key>` (skema case-insensitive) atau `X-API-Key: <key>`. Keduanya ada -> Bearer menang. Header kosong/tak ada -> reason `missing` tanpa memanggil verify | Rekomendasi tiket (pertanyaan terbuka 2); dicatat sebagai keputusan di tiket T07 |
| K5 | Semua penolakan level HTTP (401, 413, 415, 429, 503, 400 transport) memakai body JSON-RPC `{"jsonrpc":"2.0","id":null,"error":{...}}`. Pengecualian: 403 Origin memakai `middleware.OriginGuard` (format error REST), 405 GET/DELETE memakai body JSON-RPC | Klien MCP membaca JSON-RPC; klien sah tidak mengirim Origin asing |
| K6 | Body 401 IDENTIK byte-per-byte untuk semua alasan + header `WWW-Authenticate: Bearer realm="grimoire-mcp"`. `Reason`/`Prefix` hanya ke audit | Tiket + kontrak `verify` |
| K7 | `initialize` dengan versi tidak didukung -> error `-32602` `Unsupported protocol version`, `data {requested, supported}` (bukan fallback) | Keputusan tiket "tolak versi yang tidak didukung" |
| K8 | Header `MCP-Protocol-Version` (selain pada `initialize`): tidak dikenal -> 400 `-32600`; tidak ada -> dianggap `2025-03-26` | Spesifikasi MCP Streamable HTTP |
| K9 | Content-Type wajib `application/json` (parameter `charset` boleh); selain itu 415. Header `Accept` tidak dipaksakan | Toleran ke klien |
| K10 | Batch (array) -> 400 `-32600`. Parse error -> 400 `-32700`. Notification/response dari klien -> 202 tanpa body. Error per-method -> HTTP 200 dengan objek error JSON-RPC | JSON-RPC 2.0 + MCP |
| K11 | `id` request wajib string atau number; `id:null` atau tipe lain -> 400 `-32600` | MCP: id MUST NOT be null |
| K12 | Respons selalu `Content-Type: application/json`, tanpa `Mcp-Session-Id`, tanpa SSE | Stateless |
| K13 | `readOnlyHint:true` dipaksa gateway di `tools/list`; tool tidak bisa mengubahnya | Rules 9.1 |
| K14 | `toolkit.Call.ProjectID` SELALU `Principal.ProjectID`. Argumen `project_id` di `arguments` diteruskan apa adanya sebagai JSON opaque (schema tool T08/T09 yang menolaknya) dan tidak pernah mempengaruhi `ProjectID` | Rules 7.4, AC3 |
| K15 | `tools/call` dengan params tidak valid (name bukan string, kosong, > 128 byte; `arguments` ada tapi bukan object) -> `-32602` di endpoint, TANPA audit (bukan panggilan tool) | Audit hanya untuk panggilan tool sungguhan |
| K16 | Nama tool tak dikenal -> invoker menulis TEPAT 1 entri `mcp.unknown_tool` (status `error`), endpoint menjawab `-32602` `Unknown tool: <name>` | Terlihat di audit, tetap error protokol (spesifikasi MCP) |
| K17 | Setiap `tools/call` valid -> TEPAT 1 entri `mcp.<tool>`. Args = arguments ter-decode (disanitasi T02). Result = `{bytes, original_bytes, truncated, duration_ms, error_code?}` digabung `toolkit.Result.Summary` (key gateway menang). `Text` tidak pernah masuk audit | AC4, rules 9.4, 10.3 |
| K18 | Error Go dari Handler -> `isError:true`, `error_code:"internal"`, teks generik + `trace_id`. Timeout -> `error_code:"timeout"`. Panic -> `error_code:"panic"` + log error dengan stack. Server tidak jatuh | Tiket bagian Batas & ketahanan, rules 7.12 |
| K19 | Rate limit di memori satu instance, janitor berhenti lewat `d.Ctx`. Per key: `MCP_RATE_PER_MINUTE` (rate = n/60 per detik, burst n). Per IP gagal-auth: `MCP_AUTH_FAIL_PER_MINUTE`. Pre-check IP dengan `Peek` sebelum verify; token IP hanya dikonsumsi (`Allow`) saat auth gagal. Bila `Allow` saat gagal mengembalikan false, request itu tetap 401; request berikutnya kena 429 dari `Peek` | Anti brute force tanpa memblokir klien sah; tiket |
| K20 | Audit `mcp.rate_limited` dibatasi `AuditThrottle`: maks 1 entri per subjek (key id atau IP) per 60 detik; selebihnya log warn. Penolakan 429 tetap selalu dikirim | Cegah banjir tabel audit; AC6 terpenuhi entri pertama |
| K21 | Semua audit memakai `bus.Call` sinkron ke `core.audit.record`, fail-open, lewat satu writer bersama `base/auditwriter` (pola T04 `base/audit`). Durasi disimpan di `Result.duration_ms` (record.Request tidak punya field durasi) | Rules 6.3, 10.4 |
| K22 | `verify` error -> 503 `-32004` + log error, tanpa audit (fail-closed). `get-project` gagal -> label `""` + log warn, request tetap jalan | Kontrak `verify`; label hanya kosmetik audit |
| K23 | `touch` di-Publish hanya setelah lolos auth DAN rate limit key | Tiket; tidak memperbarui last_used untuk request yang ditolak |
| K24 | Urutan penolakan: Origin (403) -> auth/ratelimit (401/429/503) -> transport (415/413/400) -> protokol (JSON-RPC). Body limit dipasang sebelum auth tapi ditegakkan saat endpoint membaca body (lazy), sehingga request tanpa auth tidak pernah dibaca body-nya | `BodyLimit` T00 bersifat lazy |
| K25 | Bahasa: pesan error JSON-RPC level protokol memakai string standar Inggris (`Parse error`, `Invalid Request`, `Method not found`, `Invalid params`, `Unauthorized`, `Rate limit exceeded`, `Service unavailable`). Teks hasil tool dari gateway (internal/timeout/panic/truncation) memakai bahasa Indonesia, konsisten dengan penanda truncation | Klien MCP mengenali string standar; teks tool dibaca LLM |
| K26 | `serverInfo.version = d.Version` (dari `buildinfo.Version`); kosong -> `"dev"` | Tiket: `serverInfo` (nama, versi) |
| K27 | Tidak ada middleware `Timeout` di `/mcp`; batas waktu per tool dari `MCP_TOOL_TIMEOUT` (maks 60 detik). Server HTTP sudah tanpa WriteTimeout (T00) | Hindari 504 REST di tengah respons JSON-RPC |

### 3.1 Struktur module

```
backend/lib/contracts/discovery/mcpgateway/toolkit/
  toolkit.go                      # BARU: Tool, Handler, Call, Result (stdlib saja, tanpa tes; lihat F4)

backend/internal/domain/discovery/
  base.go                         # UBAH: Register(d) merakit []toolkit.Tool lalu mcpgateway.Register(d, tools)
  internal/mcpgateway/
    base.go                       # BARU: package mcpgateway; Register(d app.Deps, tools []toolkit.Tool)
    base_test.go                  # BARU: AC8 panic duplikat + AC1 JSON-RPC lintas feature (tool palsu read_file)
    base/                         # package base: entity + rule, TANPA I/O
      catalog.go      catalog_test.go
      principal.go    principal_test.go
      jsonrpc.go      jsonrpc_test.go
      protocol.go     protocol_test.go
      invoker.go
      truncate.go     truncate_test.go
      audit.go        audit_test.go
      throttle.go     throttle_test.go
      args.go         args_test.go
    base/auditwriter/             # package auditwriter: Call core.audit.record fail-open (I/O)
      auditwriter.go  auditwriter_test.go
    authenticate/                 # plan: usecase-authenticate.md
      init.go handler.go params.go usecase.go *_test.go
    endpoint/                     # plan: usecase-endpoint.md
      init.go handler.go params.go usecase.go *_test.go
    callaudit/                    # plan: usecase-callaudit.md
      init.go usecase.go *_test.go

backend/cmd/internal/server/
  mcpgateway_integration_test.go  # BARU: //go:build integration; core+project+discovery ke Postgres, key T04 asli
```

Tidak ada `repository.go` di module ini (tanpa SQL). Tes Postgres tidak boleh di dalam `mcpgateway` karena merakit domain `core`/`project` = pelanggaran 4.1 (archtest memindai `_test.go` dan tag `integration`); maka ditempatkan di `cmd/internal/server` yang memang tugasnya merakit.

---

## 4. Kontrak pendaftaran tool (paket netral, BARU)

File `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go` (rules 6.5: `lib/contracts/<domain>/<module>/<nama>`). Hanya meng-import `context` dan `encoding/json` (rules 4.5).

```go
// Package toolkit adalah kontrak pendaftaran tool MCP (provider wiring: T07 mcpgateway; pendaftar: T08, T09).
//
// Module tool membuat []toolkit.Tool dan domain/discovery/base.go meneruskannya ke mcpgateway.Register.
// Gateway tidak meng-import package tool dan sebaliknya. Urutan pendaftaran tidak berpengaruh.
// Validasi saat start (gagal = panic "mcpgateway: ..."):
//   - Name cocok ^[a-z][a-z0-9_]{0,63}$, bukan nama terlarang (initialize, tools_list, unknown_tool, auth, rate_limited),
//     unik, dan "mcp."+Name WAJIB ada di record.Actions().
//   - InputSchema JSON object dengan "type":"object". Handler tidak nil.
// Gateway menambahkan annotations.readOnlyHint=true; tool tidak bisa mengubahnya.
// Tool DILARANG menulis audit sendiri (rules 9.3); gateway menulis tepat satu entri mcp.<Name> per panggilan.
package toolkit

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

type Call struct {
	ProjectID string          // SELALU dari context gateway (Principal), tidak pernah dari argumen
	Arguments json.RawMessage // object JSON apa adanya ("{}" bila klien tidak mengirim arguments)
}

type Result struct {
	Text       string         // konten teks untuk LLM (dipotong gateway bila > MCP_MAX_RESPONSE_BYTES)
	IsError    bool           // kegagalan tool, pesan berguna bagi LLM
	TargetType string         // audit, mis. "file", "api_spec"
	Target     string         // audit, mis. path
	Summary    map[string]any // metadata saja (count, bytes, error_code); DILARANG berisi isi file
}
```

Wiring di `backend/internal/domain/discovery/base.go`:

```go
package discovery

// Register merakit module domain discovery. Tool dari module lain ditambahkan di sini (T08, T09)
// tanpa gateway meng-import package tool. Urutan append tidak berpengaruh (katalog diurutkan nama).
func Register(d app.Deps) {
	var tools []toolkit.Tool
	// T08: tools = append(tools, knowledgetools.Register(d)...)
	// T09: tools = append(tools, apispectools.Register(d)...)
	mcpgateway.Register(d, tools)
}
```

---

## 5. Entity bersama (`mcpgateway/base/`, tanpa I/O)

Semua fungsi murni (tanpa context, tanpa bus). `base` boleh meng-import `toolkit`, `record`, stdlib.

### 5.1 `catalog.go`

```go
type Catalog struct{ /* tools []toolkit.Tool terurut nama; index map[string]int */ }

func NewCatalog(tools []toolkit.Tool) (Catalog, error)
func (c Catalog) List() []toolkit.Tool                 // salinan, urut nama (urutan registrasi tidak berpengaruh)
func (c Catalog) Lookup(name string) (toolkit.Tool, bool)
func (c Catalog) Len() int
func (c Catalog) Names() []string                      // urut; dipakai instructions

var ReservedToolNames = []string{"initialize", "tools_list", "unknown_tool", "auth", "rate_limited"}
```

Validasi berurutan per tool; error pertama dikembalikan, pesan selalu memuat nama dengan `%q`:
- nama tidak cocok regex -> `nama tool MCP tidak valid: "Read-File"`
- nama terlarang -> `nama tool MCP terlarang: "auth"`
- `"mcp."+name` tidak ada di `record.Actions()` -> `aksi audit "mcp.foo" untuk tool "foo" belum ada di katalog record.Actions()`
- duplikat -> `tool MCP duplikat: "read_file"`
- Handler nil -> `tool MCP "read_file": handler nil`
- InputSchema bukan JSON object atau `type` bukan `"object"` -> `tool MCP "read_file": inputSchema harus JSON object dengan "type":"object"`

Daftar kosong valid (T07 berdiri sendiri). `mcpgateway.Register` melakukan `panic("mcpgateway: " + err.Error())`.

### 5.2 `principal.go`

```go
type Principal struct {
	KeyID, KeyName, KeyPrefix string
	ProjectID, ProjectName    string
	IP, UserAgent             string
}
func (p Principal) ActorLabel() string            // "<KeyName> (<KeyPrefix>)"; KeyName kosong -> KeyPrefix
func WithPrincipal(ctx context.Context, p Principal) context.Context
func PrincipalFrom(ctx context.Context) (Principal, bool) // key context privat (type principalKey struct{})
```

Catatan: `WithPrincipal`/`PrincipalFrom` hanya menyimpan nilai di context (bukan I/O), konsisten dengan `reqctx`. `authenticate` juga memanggil `reqctx.WithAPIKeyID(ctx, KeyID)` agar ikut terbawa saat Publish dan access log.

### 5.3 `jsonrpc.go`

```go
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	CodeUnauthorized   = -32001
	CodeForbidden      = -32002 // cadangan; 403 Origin memakai OriginGuard (K5)
	CodeRateLimited    = -32003
	CodeUnavailable    = -32004
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`          // selalu "2.0"
	ID      json.RawMessage `json:"id"`               // "null" bila tidak diketahui
	Result  json.RawMessage `json:"result,omitempty"` // sukses: selalu ada (minimal {})
	Error   *Error          `json:"error,omitempty"`
}

type Kind int
const (KindRequest Kind = iota + 1; KindNotification; KindResponse; KindBatch; KindInvalid; KindParseError)

func ParseMessage(body []byte) (Kind, Request, *Error)
func Success(id json.RawMessage, result any) (Response, error) // marshal result ke RawMessage
func Failure(id json.RawMessage, e *Error) Response
func HTTPReject(code int, msg string, data any) Response       // id:null
```

Aturan `ParseMessage`:
- JSON tidak valid (termasuk body kosong) -> `KindParseError`, `-32700 Parse error`.
- Top-level array -> `KindBatch` (endpoint menjawab 400 `-32600 Invalid Request`, `data:{reason:"batch not supported"}`).
- Bukan object -> `KindInvalid`.
- Ada `result` atau `error` dan tidak ada `method` -> `KindResponse`.
- `jsonrpc != "2.0"` atau `method` bukan string tidak kosong -> `KindInvalid`.
- Tidak ada field `id` -> `KindNotification`.
- `id` ada: harus string atau number (bukan null/bool/object/array) -> `KindRequest`; selain itu `KindInvalid`.
- `params` bila ada harus object (array/skalar -> `KindInvalid`; MCP hanya memakai object).

### 5.4 `protocol.go`

```go
var SupportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"} // terbaru dulu
const DefaultHeaderVersion = "2025-03-26"
const ServerName = "grimoire"
const HeaderProtocolVersion = "MCP-Protocol-Version"
func Negotiate(requested string) (string, bool) // persis salah satu SupportedVersions -> (requested, true)
func KnownHeaderVersion(h string) (string, bool) // "" -> (DefaultHeaderVersion, true); dikenal -> (h, true)
func Instructions(projectName string, toolNames []string) string
```

`Instructions`: teks singkat (Indonesia) yang menyebut nama project (kosong -> "project ini") dan alur jelajah. Bila `toolNames` memuat `list_files`/`read_file`/`search_knowledge` atau tool API spec, sebut berurutan list -> read -> search dengan nama tool aslinya; bila katalog kosong: "Belum ada tool yang terdaftar."

### 5.5 `invoker.go`

```go
// ToolInvoker dijalankan endpoint untuk setiap tools/call yang bentuk params-nya valid.
// Implementasi (callaudit) WAJIB menulis tepat satu entri audit per panggilan dan tidak pernah panic.
type ToolInvoker interface {
	Invoke(ctx context.Context, p Principal, name string, args json.RawMessage) Outcome
}

type Outcome struct {
	UnknownTool   bool
	Result        toolkit.Result // Text sudah dipotong
	Truncated     bool
	OriginalBytes int
	ErrorCode     string // diisi gateway: "internal" | "timeout" | "panic"; "" bila tidak ada
}
```

### 5.6 `truncate.go`

```go
func Truncate(text string, max int) (out string, truncated bool, original int)
func TruncationMarker(limit, original int) string
```
- `len(text) <= max` -> apa adanya.
- Selainnya: potong di batas rune UTF-8 sehingga `len(prefix)+len(marker) <= max`, lalu tambah penanda:
  `\n\n[truncated: respons dipotong pada N byte dari M byte. Persempit permintaan dengan argumen offset/limit.]`
  (`N` = byte prefix yang dipertahankan, `M` = original). Hasil selalu UTF-8 valid dan `len(out) <= max`.

### 5.7 `args.go`

```go
func DecodeArgs(raw json.RawMessage) map[string]any // kosong/null -> {}; object -> map; selain itu {"_invalid": true}
func TruncateName(name string, max int) string        // aman UTF-8, dipakai audit unknown tool (max 128)
```

### 5.8 `audit.go` (builder `record.Request`)

Semua builder mengisi `IP`, `UserAgent` dari argumen; `TraceID` dibiarkan kosong (diisi provider dari ctx). Durasi -> `Result["duration_ms"]` (int64 milidetik).

| Builder | Action | Actor | Project | Target | Args | Result | Status |
|---|---|---|---|---|---|---|---|
| `ToolCallAudit(p, tool, args map[string]any, o Outcome, dur)` | `mcp.<tool>` | `api_key`, ID=KeyID, Label=ActorLabel | ID+Name | `o.Result.TargetType/Target` | `args` | `Summary` tool lalu ditimpa `{bytes, original_bytes, truncated, duration_ms, error_code?}` | `error` bila `o.Result.IsError`, selain itu `ok` |
| `UnknownToolAudit(p, name, dur)` | `mcp.unknown_tool` | `api_key` | ID+Name | `tool`/name | `{name}` (maks 128) | `{error_code:"unknown_tool", duration_ms}` | `error` |
| `InitializeAudit(p, requested, negotiated, clientName, clientVersion, ok bool, dur)` | `mcp.initialize` | `api_key` | ID+Name | - | `{protocol_version: requested, client_name, client_version}` | ok: `{protocol_version: negotiated, duration_ms}`; gagal: `{error_code:"unsupported_version", duration_ms}` | `ok` / `error` |
| `ToolsListAudit(p, count, dur)` | `mcp.tools_list` | `api_key` | ID+Name | - | `{}` | `{count, duration_ms}` | `ok` |
| `AuthRejectedAudit(ip, ua, prefix, reason)` | `mcp.auth.rejected` | `anonymous`, ID kosong, Label=prefix | kosong | `api_key`/prefix (kosong bila tak ada) | `{reason, key_prefix}` | `{}` | `denied` |
| `RateLimitedKeyAudit(p, retryAfter int)` | `mcp.rate_limited` | `api_key` | ID+Name | `api_key`/KeyID | `{scope:"key", retry_after_seconds}` | `{}` | `denied` |
| `RateLimitedIPAudit(ip, ua string, retryAfter int)` | `mcp.rate_limited` | `anonymous` | kosong | `ip`/ip | `{scope:"auth_ip", retry_after_seconds}` | `{}` | `denied` |

`bytes` = panjang `Text` setelah dipotong; `original_bytes` = sebelum dipotong. Reason yang mungkin di `mcp.auth.rejected`: `missing`, `malformed`, `unknown`, `mismatch`, `revoked`. Tiga aksi baru (`mcp.initialize`, `mcp.tools_list`, `mcp.unknown_tool`) ditambahkan di F2.

### 5.9 `throttle.go`

```go
type AuditThrottle struct{ /* mu, last map[string]time.Time, window, now func() time.Time */ }
func NewAuditThrottle(window time.Duration, now func() time.Time) *AuditThrottle
func (t *AuditThrottle) Allow(subject string) bool // true bila belum ada entri untuk subject dalam window
```
Window 60 detik. Pembersihan entri kedaluwarsa dilakukan di dalam `Allow` saat map > 1024 entri (tanpa goroutine). Subjek: `"key:"+KeyID` atau `"ip:"+IP`.

### 5.10 `base/auditwriter/` (I/O bersama, pola T04 `base/audit`)

```go
type Writer interface{ Write(ctx context.Context, req record.Request) }
func New(b *bus.Registry, log *slog.Logger) Writer
```
`Write` memanggil `bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)`. Error -> `log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err)`; tidak pernah me-log `Args`/`Result`; tidak mengembalikan error (fail-open). Ctx yang dipakai: `context.WithoutCancel(ctx)` agar audit tetap tertulis walau klien memutus koneksi.

---

## 6. Kontrak bus dan konstanta

- `lib/constants/bus.go`: TIDAK berubah. Komentar `BUS_PROJECT_MANAGEMENT_GET_PROJECT` dan `BUS_PROJECT_APIKEY_*` sudah menyebut T07 sebagai consumer.
- Kontrak yang dikonsumsi (tidak diubah): `verify`, `touch`, `getproject`, `record` (lihat tabel bagian 1).
- Kontrak baru (wiring, bukan bus): `lib/contracts/discovery/mcpgateway/toolkit` (bagian 4).
- `lib/contracts/core/audit/record/actions.go` (tambah):
  ```go
  ActionMCPInitialize  = "mcp.initialize"
  ActionMCPToolsList   = "mcp.tools_list"
  ActionMCPUnknownTool = "mcp.unknown_tool"
  ```
  dan masuk ke `Actions()` setelah `ActionMCPRateLimited`.

Pola pemanggilan (diulang di plan usecase):

```go
res, err := bus.CallTyped[verify.Response](ctx, d.Bus, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
// err != nil -> 503 -32004 + log error (fail-closed). !res.Found -> 401 seragam + AuthRejectedAudit(ip, ua, res.Prefix, res.Reason).

gp, err := bus.CallTyped[getproject.Response](ctx, d.Bus, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: res.ProjectID})
// err != nil || !gp.Found -> ProjectName "" + log warn.

if err := d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: res.KeyID}); err != nil { /* log warn */ }
```

---

## 7. Migration

Tidak ada. T07 tidak punya tabel (tiket bagian Model data); rate limiter dan throttle audit di memori satu instance.

---

## 8. Plan (langkah berurutan)

### Fase F — Foundation (WAJIB selesai sebelum usecase)

**F1. Config** — `backend/internal/infra/config/config.go`
- Field baru di `Config` (setelah `PublicURL`, blok komentar `// MCP gateway (T07).`):
  ```go
  MCPRatePerMinute     int           `env:"MCP_RATE_PER_MINUTE"`
  MCPAuthFailPerMinute int           `env:"MCP_AUTH_FAIL_PER_MINUTE"`
  MCPMaxRequestBytes   int64         `env:"MCP_MAX_REQUEST_BYTES"`
  MCPToolTimeout       time.Duration `env:"MCP_TOOL_TIMEOUT"`
  MCPAllowedOrigins    []string      `env:"MCP_ALLOWED_ORIGINS"`
  ```
- `Load`:
  - `c.MCPRatePerMinute, _ = l.intr("MCP_RATE_PER_MINUTE", 120, 1, 10000)`
  - `c.MCPAuthFailPerMinute, _ = l.intr("MCP_AUTH_FAIL_PER_MINUTE", 20, 1, 1000)`
  - `c.MCPMaxRequestBytes, _ = l.int64("MCP_MAX_REQUEST_BYTES", 65536, 1024)`
  - `c.MCPToolTimeout = l.dur("MCP_TOOL_TIMEOUT", 10*time.Second)`; bila `> 60*time.Second` -> `l.fail("MCP_TOOL_TIMEOUT", "maksimal 60s (sekarang %s)", ...)`.
  - `MCP_ALLOWED_ORIGINS`: split koma, trim spasi, buang entri kosong. Tiap entri: `url.Parse` sukses, scheme `http`/`https`, host tidak kosong, path kosong, tanpa query/fragment/userinfo; selain itu `l.fail("MCP_ALLOWED_ORIGINS", "entri %q harus berbentuk scheme://host[:port]", e)`. Simpan apa adanya (pencocokan `CheckOrigin` sudah case-insensitive). Default `nil`.
- `LogValue`: tambah `mcp_rate_per_minute`, `mcp_auth_fail_per_minute`, `mcp_max_request_bytes`, `mcp_tool_timeout`, `mcp_allowed_origins` (bukan rahasia).
- `config_test.go`: default, batas bawah/atas tiap env, `MCP_TOOL_TIMEOUT=61s` gagal, `0s` gagal, origin valid/invalid (`example.com`, `https://a.com/path`, `ftp://a`, `https://a.com?x=1`), daftar dengan spasi dan koma ganda.
- `docs/configuration.md`: 5 baris baru di tabel (format kolom yang sudah ada). `.env.example`: 5 baris komentar `# MCP_RATE_PER_MINUTE=120` dst. `docs_test.go` dan `envexample_test.go` harus lolos; bila `formatDefault` tidak mendukung `[]string`, tidak perlu diubah karena default `MCPAllowedOrigins` nol (dilewati `IsZero`).

**F2. Aksi audit** — `backend/lib/contracts/core/audit/record/actions.go` + `actions_test.go`
- Tambah 3 konstanta (bagian 6) dan masukkan ke `Actions()`. Perbarui `actions_test.go` (jumlah/keanggotaan).

**F3. Infra kecil**
- `backend/internal/infra/middleware/ratelimit.go`: tambah
  ```go
  // Peek memeriksa apakah key masih punya token tanpa mengonsumsi dan tanpa membuat entri.
  // Key belum dikenal -> (true, 0).
  func (l *Limiter) Peek(key string) (ok bool, wait time.Duration)
  ```
  Implementasi: lock, ambil entri; tidak ada -> `(true, 0)`; `tokens := e.lim.TokensAt(now)`; `tokens >= 1` -> `(true, 0)`; selainnya `wait = time.Duration((1-tokens)/float64(l.rule.Rate) * float64(time.Second))`, `(false, wait)`. Tidak mengubah `lastSeen`.
  Tes di `ratelimit_test.go`: Peek tidak membuat entri (`size()` tetap 0), Peek tidak mengonsumsi (N kali Peek lalu Allow masih sukses), setelah burst habis Peek false dengan wait > 0, setelah waktu maju (clock palsu `newLimiter`) Peek true lagi.
- `backend/internal/infra/app/deps.go`: tambah field `Version string // versi build untuk serverInfo MCP; diisi cmd/internal/server dari buildinfo.Version`.
- `backend/cmd/internal/server/run.go` dan `domains.go`: isi `Version: buildinfo.Version` di literal `app.Deps`. Tes yang ada (`domains_test.go`, `run_test.go`) harus tetap lolos.

**F4. Kontrak toolkit** — `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go` (bagian 4). Tanpa logika; tidak wajib ada tes. Archtest harus tetap lolos (hanya stdlib).

**F5. `mcpgateway/base/`** — file pada bagian 5.1-5.9 beserta tes unit tanpa mock (bagian 12.1).

**F6. `mcpgateway/base/auditwriter/`** — bagian 5.10 + tes dengan `bus.NewRegistry` dan handler palsu.

**F7. Wiring module** — `backend/internal/domain/discovery/internal/mcpgateway/base.go`:
```go
package mcpgateway

// Register memasang gateway MCP di d.MCP. tools berasal dari domain/discovery/base.go.
// Katalog tidak valid (nama duplikat dll.) -> panic saat start (AC8).
func Register(d app.Deps, tools []toolkit.Tool) {
	cat, err := base.NewCatalog(tools)
	if err != nil {
		panic("mcpgateway: " + err.Error())
	}
	audit := auditwriter.New(d.Bus, d.Log)
	inv := callaudit.Init(d, cat, audit)    // base.ToolInvoker
	admit := authenticate.Init(d, audit)    // func(http.Handler) http.Handler
	endpoint.Init(d, cat, inv, admit, audit)
}
```
Selama usecase belum ada, developer foundation boleh membuat stub `Init` di tiap folder feature yang hanya mengembalikan nilai minimal agar build hijau; stub diganti di fase usecase.

**F8. Wiring domain** — `backend/internal/domain/discovery/base.go` (bagian 4). Hapus komentar "No-op pada T00".

### Fase U — Usecase (paralel setelah F; plan rinci di file usecase)

**U1. `callaudit`** (`usecase-callaudit.md`) — implementasi `base.ToolInvoker`. Bisa paralel dengan U2.
**U2. `authenticate`** (`usecase-authenticate.md`) — middleware admisi (auth + rate limit). Bisa paralel dengan U1.
**U3. `endpoint`** (`usecase-endpoint.md`) — transport + dispatch; butuh U1/U2 untuk tes lintas feature di `mcpgateway/base_test.go` (tes per feature memakai invoker/admit palsu, jadi kodenya bisa mulai paralel).

Kontrak `Init` yang mengikat:

| Feature | Signature | Catatan |
|---|---|---|
| `callaudit` | `func Init(d app.Deps, cat base.Catalog, audit auditwriter.Writer) base.ToolInvoker` | Membaca `d.Config.MCPToolTimeout`, `d.Config.MCPMaxResponseBytes`, `d.Log` |
| `authenticate` | `func Init(d app.Deps, audit auditwriter.Writer) func(http.Handler) http.Handler` | Membuat dua `middleware.NewLimiter(d.Ctx, ...)` dan satu `base.NewAuditThrottle(time.Minute, time.Now)` |
| `endpoint` | `func Init(d app.Deps, cat base.Catalog, inv base.ToolInvoker, admit func(http.Handler) http.Handler, audit auditwriter.Writer)` | Mendaftarkan `d.MCP.With(middleware.OriginGuard(cfg.MCPAllowedOrigins), middleware.BodyLimit(cfg.MCPMaxRequestBytes), admit).Post("/", h)`, `d.MCP.Get("/", h405)`, `d.MCP.Delete("/", h405)` |

Usecase tiap feature menerima interface (`auditwriter.Writer`, `base.ToolInvoker`, interface kecil untuk bus/limiter) agar bisa dites (rules 5.6).

### Fase I — Integrasi dan dokumen

**I1.** `backend/internal/domain/discovery/internal/mcpgateway/base_test.go`: tes lintas feature lewat `httptest` + `bus.NewRegistry` nyata berisi handler palsu `verify`/`get-project`/`touch`/`audit` dan tool palsu `read_file` (AC1, AC3, AC4, AC8, AC9; bagian 12.3).
**I2.** `backend/cmd/internal/server/mcpgateway_integration_test.go` (`//go:build integration`): Postgres nyata, `RUN_MIGRATIONS_ON_START=true`, `COOKIE_SECURE=false`; buat admin lewat bus `core.auth.create-admin` (via `WithAppHook`), login `/api/auth/login`, buat project dan key lewat REST T03/T04, lalu `POST /mcp` (bagian 12.4).
**I3.** `docs/mcp-gateway.md` (BARU): ringkasan protokol (bagian 14), header, kode error, env terkait, cara menambah tool (toolkit + satu baris di `discovery/base.go` + aksi audit di `record.Actions()`), tes manual AC1:
  ```
  claude mcp add --transport http grimoire https://<host>/mcp --header "Authorization: Bearer <key>"
  ```
  lalu `/mcp` di Claude Code menampilkan server terhubung; daftar tool dan `read_file` diverifikasi penuh setelah T08 merge (dicatat sebagai langkah tertunda).
**I4.** `docs/tickets/README.md`: status T07; baris katalog bus `discovery.mcpgateway.register-tool | wiring | T07 | T08, T09` merujuk `lib/contracts/discovery/mcpgateway/toolkit` + `domain/discovery/base.go`; katalog aksi agent tambah `mcp.initialize`, `mcp.tools_list`, `mcp.unknown_tool`; pertanyaan terbuka 2 (Basic Auth) ditandai diputuskan: tidak didukung. Catatan T02 "1 skenario menunggu T04/T07" diperbarui.
**I5.** `docs/tickets/T07-discovery-mcp-gateway.md`: tabel Features dicatat bahwa `toolregistry` = foundation/wiring dan `ratelimit` digabung ke `authenticate` (K1); pertanyaan terbuka 2 diputuskan (K4); env baru disebut di bagian Batas.
**I6.** Frontend + e2e (bagian 11).

---

## 9. Daftar usecase

| Slug | Plan | Ringkasan | Tanggung jawab audit |
|---|---|---|---|
| `authenticate` | `docs/superpowers/plans/discovery-mcp-gateway/usecase-authenticate.md` | Middleware admisi `POST /mcp`: ekstrak key (Bearer/X-API-Key), pre-check limiter IP (`Peek`), verify tanpa cache (fail-closed 503), 401 seragam, limiter per key, label project fail-soft, `Principal` + `reqctx.WithAPIKeyID`, Publish `touch` | `mcp.auth.rejected`, `mcp.rate_limited` (key/auth_ip, throttle) |
| `endpoint` | `docs/superpowers/plans/discovery-mcp-gateway/usecase-endpoint.md` | Route POST/GET/DELETE, validasi transport (415/413/400/202), dispatch `initialize`/`ping`/`tools/list`/`tools/call`/-32601, negosiasi versi, `instructions`, `readOnlyHint`, validasi params `tools/call`, pemetaan `Outcome` -> hasil MCP | `mcp.initialize`, `mcp.tools_list` |
| `callaudit` | `docs/superpowers/plans/discovery-mcp-gateway/usecase-callaudit.md` | `base.ToolInvoker`: lookup, timeout, recover panic, `ProjectID` dari Principal, truncation, pemetaan error | tepat 1 `mcp.<tool>` atau `mcp.unknown_tool` per panggilan |

Tidak ada usecase `toolregistry` dan `ratelimit` (K1).

---

## 10. Changes (per file/folder)

| File/folder | Aksi | Isi |
|---|---|---|
| `backend/internal/infra/config/config.go` | ubah | 5 field + validasi + `LogValue` (F1) |
| `backend/internal/infra/config/config_test.go` | ubah | tes env baru |
| `docs/configuration.md`, `.env.example` | ubah | 5 env baru |
| `backend/internal/infra/middleware/ratelimit.go` (+`_test.go`) | ubah | `Limiter.Peek` (F3) |
| `backend/internal/infra/app/deps.go` | ubah | field `Version` |
| `backend/cmd/internal/server/run.go`, `domains.go` | ubah | isi `Version: buildinfo.Version` |
| `backend/lib/contracts/core/audit/record/actions.go` (+`actions_test.go`) | ubah | 3 aksi baru |
| `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go` | baru | kontrak tool |
| `backend/internal/domain/discovery/base.go` | ubah | wiring tool + `mcpgateway.Register` |
| `backend/internal/domain/discovery/internal/mcpgateway/base.go` (+`base_test.go`) | baru | wiring module, tes lintas feature |
| `.../mcpgateway/base/*.go` (+tes) | baru | entity bagian 5.1-5.9 |
| `.../mcpgateway/base/auditwriter/*.go` (+tes) | baru | writer audit fail-open |
| `.../mcpgateway/authenticate/` | baru | lihat plan usecase |
| `.../mcpgateway/endpoint/` | baru | lihat plan usecase |
| `.../mcpgateway/callaudit/` | baru | lihat plan usecase |
| `backend/cmd/internal/server/mcpgateway_integration_test.go` | baru | tes Postgres end-to-end |
| `docs/mcp-gateway.md` | baru | dokumentasi protokol + tes manual |
| `docs/tickets/README.md`, `docs/tickets/T07-discovery-mcp-gateway.md` | ubah | katalog, status, keputusan |
| `frontend/src/domain/core/audit-log/listentries/types.ts` | ubah | 3 aksi di grup `mcp` |
| `frontend/e2e/helpers/mcp.ts` | baru | helper panggilan MCP |
| `frontend/e2e/audit-log.spec.ts` | ubah | aktifkan skenario 5 |
| `frontend/e2e/mcp-gateway.spec.ts` | baru | alur MCP |
| `api/**`, `backend/migrations/**`, `lib/constants/bus.go`, `go.mod` | tidak berubah | - |

---

## 11. Frontend (tanpa UI baru)

- `frontend/src/domain/core/audit-log/listentries/types.ts`: tambah `'mcp.initialize'`, `'mcp.tools_list'`, `'mcp.unknown_tool'` ke grup `mcp` di `ACTION_GROUPS` (setelah `mcp.rate_limited`). Aksi ditampilkan sebagai kode mentah (tidak ada kunci i18n per aksi saat ini; grup `auditLog.actionGroup.mcp` sudah ada di `id.json`/`en.json`), jadi tidak ada kunci i18n baru. Jalankan `npm run test` (tes `filterQuery`/mock yang memakai daftar aksi harus tetap lolos; perbarui snapshot jumlah aksi bila ada).
- `frontend/e2e/helpers/mcp.ts` (baru): `mcpPost(request, rawKey | null, body, headers?)` mengirim `POST /mcp` dengan `Content-Type: application/json` dan `Authorization: Bearer <rawKey>` bila ada; `initializeBody(id)`, `toolsListBody(id)`.
- `frontend/e2e/audit-log.spec.ts` skenario 5: ganti `test.fixme` menjadi `test`. Buat project (`createProjectApi`) dan key (`createKeyApi`, UI pembuatan key sudah dicakup `project-api-key.spec.ts`), panggil `initialize` lalu `tools/list` lewat `request`, buka `/audit-log?actor_type=api_key&actor_id=<keyId>`, harapkan baris `mcp.initialize` dan `mcp.tools_list`, dan tidak ada baris aktor lain.
- `frontend/e2e/mcp-gateway.spec.ts` (baru, rules 13.7; langkah upload ditambah T08): lihat bagian 13.

### Visual

Tidak ada layar baru dan tidak ada perubahan token/komponen. Satu-satunya efek visual: tiga opsi baru di filter aksi grup "MCP" pada layar Audit Log (pola dropdown filter yang sudah ada di DESIGN.md, tanpa elemen khas baru). Data contoh untuk mock/tes: `{action: "mcp.initialize", actor_type: "api_key", actor_label: "claude-laptop (a1b2c3d4)", project_label: "Payments", status: "ok"}`.

---

## 12. Test Scenario

Penamaan `TestT07_ACn_...` untuk kriteria penerimaan; tes lain bebas deskriptif.

### 12.1 Unit `base/` (tanpa mock)

- `catalog_test.go`: daftar kosong valid; urutan input berbeda -> `List()` sama (urut nama); `Lookup` ada/tidak; `TestT07_AC8_DuplicateToolName` error memuat `"read_file"`; nama invalid (`Read`, `1abc`, `a-b`, 65 karakter); nama terlarang (5 nama); aksi `mcp.foo` tidak ada di `record.Actions()`; handler nil; schema bukan object/`type` bukan `"object"`/JSON rusak.
- `principal_test.go`: `ActorLabel` normal dan nama kosong; `PrincipalFrom` pada ctx kosong -> false.
- `jsonrpc_test.go`: tabel `ParseMessage` (parse error, body kosong, batch, skalar, response klien, notification, request id string/number, id null/bool/object, jsonrpc salah, method kosong, params array); `Success` dengan `{}`; `HTTPReject` menghasilkan `id:null`; `Failure` mempertahankan id.
- `protocol_test.go`: `Negotiate` tiap versi + `"2024-11-05"` false; `KnownHeaderVersion("")` default; `Instructions` memuat nama project dan nama tool; katalog kosong.
- `truncate_test.go`: tidak dipotong; `TestT07_AC7_Truncate500KB` (500 KB ASCII ke 100 KB: `len(out) <= max`, memuat penanda, `original == 512000`); string multibyte (emoji/aksara) di batas -> `utf8.ValidString(out)`; `max` lebih kecil dari penanda tetap tidak panic (keluaran = penanda yang dipotong aman).
- `args_test.go`: kosong/null -> `{}`, object, array -> `{_invalid:true}`.
- `audit_test.go`: tiap builder menghasilkan field sesuai tabel 5.8; `ToolCallAudit` key gateway menimpa `Summary` (`bytes` dari tool diabaikan); `Text` tidak ada di `Args`/`Result` (cari string rahasia di hasil `fmt.Sprintf("%v", req)`).
- `throttle_test.go`: subjek sama dalam 60 detik -> false; setelah 60 detik (clock palsu) -> true; subjek berbeda independen; pembersihan saat > 1024 entri.

### 12.2 `auditwriter`

- Handler audit sukses -> dipanggil sekali dengan request yang sama.
- Handler error -> tidak panic, log berisi `audit write failed` dan action, TIDAK berisi nilai Args/Result.
- Handler tidak terdaftar (`ErrNotRegistered`) -> log error, tetap lanjut.
- Ctx yang dibatalkan sebelum `Write` -> audit tetap ditulis (`WithoutCancel`).

### 12.3 Lintas feature (`mcpgateway/base_test.go`, httptest + bus nyata dengan handler palsu)

Rakit `app.Deps{Ctx, Config (default + MCP_TOOL_TIMEOUT kecil), Log, Bus, MCP: chi router}` lalu `mcpgateway.Register(d, []toolkit.Tool{readFileFake})`. Handler palsu `verify`: key `gr_aaaaaaaa_<secret>` -> Found project A; lainnya Found=false. Audit palsu menyimpan semua `record.Request`.

- `TestT07_AC1_JSONRPCFlow`: `initialize` (200, `protocolVersion` = yang diminta, `serverInfo.name=grimoire`) -> `notifications/initialized` (202, body kosong) -> `tools/list` (berisi `read_file`, `annotations.readOnlyHint=true`) -> `tools/call read_file` (200, `content[0].text`), audit berisi `mcp.initialize`, `mcp.tools_list`, `mcp.read_file`.
- `TestT07_AC3_ProjectIDFromKeyOnly`: arguments `{"path":"x","project_id":"<B>"}` -> tool palsu menerima `Call.ProjectID == A`; audit `ProjectID == A`.
- `TestT07_AC4_ExactlyOneAuditNoContent`: tool mengembalikan teks berisi `SECRET-CONTENT-123` -> tepat 1 entri `mcp.read_file`, string itu tidak ada di entri mana pun.
- `TestT07_AC8_RegisterPanicsOnDuplicate`: `mcpgateway.Register` dengan dua `read_file` -> panic, pesan diawali `mcpgateway:` dan memuat `"read_file"`.
- `TestT07_AC9_ForeignOrigin`: `Origin: https://evil.example` -> 403, verify TIDAK dipanggil; Origin di `MCP_ALLOWED_ORIGINS` -> lolos; tanpa Origin -> lolos.
- GET/DELETE `/mcp` -> 405 `Allow: POST`, tanpa memanggil verify.

### 12.4 Integrasi Postgres (`cmd/internal/server/mcpgateway_integration_test.go`, tag `integration`)

- `TestT07_AC1_RealKeyInitializeAndToolsList`: key asli T04 -> `initialize` 200, `tools/list` 200 (`tools: []` pada T07; T08 menambah asersi `read_file`), baris `audit_log` `mcp.initialize` dengan `actor_type=api_key`, `actor_id=<keyId>`, `project_label=<nama project>`; `last_used_at` key terisi (tunggu publish selesai).
- `TestT07_AC2_UniformRejection` (versi DB): tanpa key, key acak, key dicabut -> body identik, 3 baris `mcp.auth.rejected` (reason `missing`, `malformed`/`unknown`, `revoked`).
- `TestT07_AC5_RevokeTakesEffectNextRequest`: `tools/list` 200 -> revoke via REST -> request berikutnya 401.

### 12.5 Usecase (rinci di plan masing-masing; ringkasan cakupan wajib)

- `authenticate`: AC2 (body identik byte, 3 entri), AC5, AC6 (`TestT07_AC6_KeyRateLimit`: burst habis -> 429 + `Retry-After` + 1 entri `mcp.rate_limited` walau ditolak berkali-kali dalam 60 detik; key lain tetap 200), pre-check IP memblokir walau key berikutnya benar (verify tidak dipanggil), verify error -> 503 tanpa audit, get-project gagal tetap lolos, touch hanya saat sukses, Reason tidak muncul di body.
- `endpoint`: tabel protokol (Content-Type, header versi, parse, batch, id null, notification, ping, method tidak dikenal, initialize versi tidak didukung, params `tools/call` tidak valid tanpa audit, UnknownTool -> -32602, 413 body terlalu besar).
- `callaudit`: AC4 per jalur (ok, isError tool, error Go, timeout, panic, unknown), AC7, AC3, timeout dengan tool yang mengabaikan ctx, audit gagal tetap mengembalikan hasil + log error.

### 12.6 Infra/lib

- `ratelimit_test.go` (Peek), `config_test.go` (env baru), `actions_test.go`, `docs_test.go`, `envexample_test.go`, archtest (tanpa pelanggaran baru), `go vet`, `golangci-lint` (job `lint`).

---

## 13. E2E Scenarios (Playwright)

`frontend/e2e/mcp-gateway.spec.ts`:

Positif:
1. Login admin -> buat project -> buat key (`createKeyApi`) -> `POST /mcp initialize` (200, `result.serverInfo.name == "grimoire"`, `result.protocolVersion == "2025-06-18"`) -> `notifications/initialized` (202) -> `tools/list` (200, `result.tools` array) -> buka `/audit-log?actor_type=api_key&actor_id=<keyId>` dan lihat `mcp.initialize`.
2. `X-API-Key` header menghasilkan hasil yang sama dengan Bearer.
3. `ping` -> `{}`.

Negatif:
4. Tanpa header -> 401, header `www-authenticate` diawali `Bearer`, body `error.code == -32001`.
5. Key acak format benar -> 401 body identik dengan skenario 4.
6. Cabut key (`revokeKeyApi`) -> panggilan berikutnya 401; audit-log filter aksi `mcp.auth.rejected` menampilkan baris dengan label prefix key.
7. `Origin: https://evil.example` -> 403.
8. `GET /mcp` -> 405 dengan header `allow: POST`.
9. Method `resources/list` -> 200 dengan `error.code == -32601`.

`frontend/e2e/audit-log.spec.ts` skenario 5 aktif (bagian 11).

---

## 14. Request/Response body

Semua `POST /mcp` memakai `Content-Type: application/json` dan header auth. Contoh key: `gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA`.

### 14.1 initialize

Request:
```http
POST /mcp
Authorization: Bearer gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA
Content-Type: application/json
```
```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude-code","version":"2.1.0"}}}
```
Response 200:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-06-18",
    "capabilities": {"tools": {"listChanged": false}},
    "serverInfo": {"name": "grimoire", "version": "1.4.0"},
    "instructions": "Server ini menyajikan knowledge dan API spec project \"Payments\" (read-only). Alur jelajah: list_files untuk melihat isi, read_file untuk membaca, search_knowledge untuk mencari."
  }
}
```
Versi tidak didukung (HTTP 200):
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Unsupported protocol version","data":{"requested":"2024-11-05","supported":["2025-11-25","2025-06-18","2025-03-26"]}}}
```

### 14.2 notifications/initialized

```json
{"jsonrpc":"2.0","method":"notifications/initialized"}
```
Response: `202 Accepted`, body kosong.

### 14.3 ping

```json
{"jsonrpc":"2.0","id":"p-1","method":"ping"}
```
```json
{"jsonrpc":"2.0","id":"p-1","result":{}}
```

### 14.4 tools/list

Request (header `MCP-Protocol-Version: 2025-06-18`):
```json
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
```
Response 200 (contoh dengan tool T08):
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "read_file",
        "title": "Baca file knowledge",
        "description": "Membaca isi file Markdown berdasarkan path. Gunakan list_files dulu bila path belum diketahui.",
        "inputSchema": {"type": "object", "properties": {"path": {"type": "string"}, "offset": {"type": "integer"}, "limit": {"type": "integer"}}, "required": ["path"], "additionalProperties": false},
        "annotations": {"readOnlyHint": true}
      }
    ]
  }
}
```
Pada T07 (katalog kosong): `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`.

### 14.5 tools/call

Request:
```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/auth/login.md"}}}
```
Sukses:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"# Login\n\nAlur login memakai cookie session..."}],"isError":false}}
```
Dipotong:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"...\n\n[truncated: respons dipotong pada 102280 byte dari 512000 byte. Persempit permintaan dengan argumen offset/limit.]"}],"isError":false,"_meta":{"truncated":true,"original_bytes":512000}}}
```
Kegagalan tool (dari tool):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"File \"docs/auth/logn.md\" tidak ditemukan. Mungkin maksudnya: docs/auth/login.md"}],"isError":true}}
```
Timeout / internal / panic (dari gateway):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Tool read_file melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=01HZX3K9Q7"}],"isError":true}}
```
Tool tidak dikenal (HTTP 200):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Unknown tool: write_file"}}
```
Params tidak valid (HTTP 200, tanpa audit):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Invalid params","data":{"reason":"arguments must be an object"}}}
```
Metode tidak dikenal (HTTP 200):
```json
{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"Method not found","data":{"method":"resources/list"}}}
```

### 14.6 Penolakan level HTTP

401 (identik untuk missing/malformed/unknown/mismatch/revoked), header `WWW-Authenticate: Bearer realm="grimoire-mcp"`:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}
```
429, header `Retry-After: 1`:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32003,"message":"Rate limit exceeded","data":{"retry_after_seconds":1}}}
```
503 (verify error):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32004,"message":"Service unavailable"}}
```
400 parse error:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}
```
400 batch / invalid / id null:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"batch not supported"}}}
```
400 header versi tidak dikenal:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"unsupported MCP-Protocol-Version","supported":["2025-11-25","2025-06-18","2025-03-26"]}}}
```
413 body > `MCP_MAX_REQUEST_BYTES`:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"request body too large","limit_bytes":65536}}}
```
415 Content-Type salah:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"Content-Type must be application/json"}}}
```
405 `GET`/`DELETE /mcp`, header `Allow: POST`:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"method not allowed; use POST"}}}
```
403 Origin asing (format REST dari `OriginGuard`):
```json
{"error":{"code":"FORBIDDEN","message":"origin tidak diizinkan"},"trace_id":"01HZX3K9Q7"}
```

### 14.7 Contoh entri audit (dikirim ke `core.audit.record`)

`mcp.read_file`:
```json
{
  "ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)",
  "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments",
  "Action": "mcp.read_file", "TargetType": "file", "Target": "docs/auth/login.md",
  "Args": {"path": "docs/auth/login.md"},
  "Result": {"bytes": 2048, "original_bytes": 2048, "truncated": false, "duration_ms": 12},
  "Status": "ok", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"
}
```
`mcp.auth.rejected`:
```json
{"ActorType": "anonymous", "ActorID": "", "ActorLabel": "a1b2c3d4", "Action": "mcp.auth.rejected", "TargetType": "api_key", "Target": "a1b2c3d4", "Args": {"reason": "revoked", "key_prefix": "a1b2c3d4"}, "Result": {}, "Status": "denied", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"}
```
`mcp.rate_limited` (scope key):
```json
{"ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)", "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments", "Action": "mcp.rate_limited", "TargetType": "api_key", "Target": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "Args": {"scope": "key", "retry_after_seconds": 1}, "Result": {}, "Status": "denied", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"}
```

---

## 15. Urutan implementasi (ringkas)

1. Foundation: F1 config -> F2 aksi audit -> F3 Peek + Deps.Version -> F4 toolkit -> F5 `base/` + tes -> F6 `auditwriter` -> F7 `mcpgateway/base.go` (dengan stub feature) -> F8 `discovery/base.go`. Build + `go test ./...` + archtest hijau.
2. `callaudit` dan `authenticate` paralel.
3. `endpoint`, lalu I1 (`mcpgateway/base_test.go`) dan I2 (integrasi Postgres).
4. I3-I5 dokumen (`docs/mcp-gateway.md`, README, tiket), I6 frontend + e2e.

Definition of done: semua AC1-AC9 punya tes bernama `TestT07_ACn_*` (AC1 manual penuh menunggu T08, didokumentasikan), CI `lint`, `test`, `test-integration`, `api-check` (tanpa perubahan), `depcruise`, `build`, `e2e` hijau.
