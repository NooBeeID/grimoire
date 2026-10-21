# T07 discovery/mcp-gateway — Usecase `endpoint`

Mengikuti `docs/superpowers/plans/discovery-mcp-gateway/overview.md` (FOUNDATION). Bila bertentangan, overview menang sampai diperbarui. Plan ini hanya untuk satu usecase: transport MCP Streamable HTTP stateless dan routing JSON-RPC.

Prasyarat: Fase F (F1-F8) overview sudah selesai. Artinya sudah ada `base/` (`jsonrpc.go`, `protocol.go`, `invoker.go`, `catalog.go`, `principal.go`, `audit.go`, `args.go`), `base/auditwriter`, config `MCP_*`, `Deps.Version`, tiga aksi audit baru, dan stub `endpoint.Init` yang akan diganti plan ini. `callaudit` (U1) dan `authenticate` (U2) tidak dibutuhkan untuk kode dan tes unit `endpoint` (memakai invoker dan admit palsu). Keduanya hanya dibutuhkan untuk tes lintas feature (langkah 7-8).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T07 |
| Usecase | `endpoint` |
| Scope | fullstack (feature ini backend; frontend hanya e2e, bagian 6) |
| Domain / Module | discovery / mcp-gateway (`mcpgateway`) |
| Folder | `backend/internal/domain/discovery/internal/mcpgateway/endpoint` |
| Endpoint | `/mcp` (sub-router `d.MCP`, route `"/"`) |
| Method | `POST` (JSON-RPC), `GET` dan `DELETE` (405) |
| Version | MCP 2025-11-25 / 2025-06-18 / 2025-03-26, JSON-RPC 2.0 single message |
| Middleware POST (urut) | `middleware.OriginGuard(cfg.MCPAllowedOrigins)` -> `middleware.BodyLimit(cfg.MCPMaxRequestBytes)` -> `admit` (dari `authenticate`) |
| Middleware GET/DELETE | tidak ada (tanpa auth) |
| Kontrak REST | tidak ada (`/mcp` bukan REST `/api`); `endpointsChanged = false` |
| Endpoint bus | Tidak menyediakan, tidak memanggil. Audit lewat `auditwriter.Writer` (yang memanggil `core.audit.record`, fail-open) |
| Migrasi | tidak ada |
| Repository | tidak ada (tanpa SQL) |

---

## 2. Summary Plan

`endpoint` adalah lapisan protokol. Tugasnya:

1. Mendaftarkan route di `d.MCP` dengan rantai middleware yang ditetapkan tiket.
2. Handler (transport): memeriksa Content-Type, membaca body (batas dari `BodyLimit`), mem-parse JSON-RPC lewat `base.ParseMessage`, memeriksa header `MCP-Protocol-Version`, menjawab 202 untuk notification/respons klien, dan menulis respons JSON.
3. Usecase (orkestrasi): dispatch per metode.
   - `initialize`: negosiasi versi (`base.Negotiate`), `serverInfo`, `capabilities`, `instructions` (`base.Instructions`), audit `mcp.initialize`.
   - `ping`: `{}`.
   - `tools/list`: katalog dari `base.Catalog.List()` dengan `readOnlyHint:true` dipaksa gateway, audit `mcp.tools_list`.
   - `tools/call`: validasi bentuk params (tanpa audit bila salah), delegasi ke `base.ToolInvoker`, ubah `base.Outcome` menjadi hasil MCP.
   - lainnya: `-32601`.
4. `GET`/`DELETE` dijawab 405 `Allow: POST`.

Rule bisnis (parse, negosiasi, builder audit, truncation, katalog) sudah di `base/`. Usecase tidak membuat rule baru. Bentuk JSON hasil (DTO MCP) dan pemetaan `Outcome` ke DTO ada di `params.go`. Penulisan audit `mcp.<tool>`/`mcp.unknown_tool` TIDAK dilakukan di sini (tanggung jawab `callaudit`); `endpoint` hanya menulis `mcp.initialize` dan `mcp.tools_list`.

Tidak memecah usecase: satu usecase `Usecase.Handle` dengan switch metode, karena semua metode berbagi parsing, Principal, dan format respons.

### Keputusan lokal (di bawah keputusan K1-K27 overview)

| # | Keputusan | Alasan |
|---|---|---|
| E1 | Urutan pemeriksaan di handler POST: Principal ada di context -> Content-Type (415) -> baca body (413 / baca gagal 400) -> `ParseMessage` (400 `-32700`/`-32600`) -> header versi (400, dilewati bila method `initialize`) -> notification/respons klien (202) -> dispatch | Sesuai K24; header `MCP-Protocol-Version` tidak wajib pada `initialize` (K8) sehingga method harus diketahui lebih dulu |
| E2 | Header `MCP-Protocol-Version` juga diperiksa untuk notification dan respons klien (header tak dikenal tetap 400) | Spesifikasi MCP: berlaku untuk semua request berikutnya |
| E3 | Principal tidak ada di context pada handler POST = bug wiring: 500 `-32603` `Internal error`, log error, tanpa audit | `admit` selalu mendahului; jangan panic |
| E4 | `initialize` dengan params yang salah bentuk (bukan object, `protocolVersion` hilang/bukan string) -> `-32602 Invalid params` tanpa audit. Hanya versi tidak didukung yang diaudit (`status=error`) | Sama dengan K15: audit untuk keputusan protokol yang bermakna |
| E5 | `clientInfo.name` dan `clientInfo.version` opsional; bukan string atau lebih dari 128 byte -> dipotong (`base.TruncateName`) / dianggap kosong, tidak menggagalkan initialize | Metadata audit saja |
| E6 | `tools/call`: `arguments` literal `null` dianggap tidak valid (`-32602`, `arguments must be an object`); tidak ada `arguments` -> endpoint meneruskan `{}` ke invoker | Mengikuti perintah usecase: object atau tidak ada |
| E7 | `isError` hasil MCP = `Outcome.Result.IsError \|\| Outcome.ErrorCode != ""`; `_meta` hanya ada bila `Outcome.Truncated` (`{truncated:true, original_bytes}`) | Defensif terhadap invoker; kontrak MCP |
| E8 | `tools/list` tanpa paginasi: `nextCursor` tidak pernah dikirim, `params.cursor` diabaikan. Slice hasil selalu non-nil (`"tools":[]`) | Katalog kecil dan statis |
| E9 | Error serialisasi hasil (`base.Success` mengembalikan error) -> `-32603 Internal error`, HTTP 200, log error | Tidak mungkin pada DTO biasa, tetap ditangani |
| E10 | Method HTTP selain POST/GET/DELETE pada `/mcp` mengikuti perilaku default router (di luar cakupan usecase ini) | Tiket hanya menyebut GET dan DELETE |
| E11 | Respons JSON ditulis dengan `Content-Type: application/json` (tanpa `Mcp-Session-Id`, tanpa SSE). 202 ditulis tanpa body dan tanpa Content-Type | K12 |

---

## 3. Plan (langkah berurutan)

Semua path relatif ke `backend/internal/domain/discovery/internal/mcpgateway/endpoint/` kecuali disebut lain. Kerjakan TDD per file: tes dulu, lalu kode.

### Langkah 1 — `params.go`: DTO dan validasi params

Tipe (semua unexported kecuali dipakai tes dalam package yang sama):

```go
type initializeParams struct {
	ProtocolVersion string
	ClientName      string
	ClientVersion   string
}

type toolsCallParams struct {
	Name      string
	Arguments json.RawMessage // sudah dinormalisasi: "{}" bila tidak ada
}

// paramsError menjadi -32602 Invalid params dengan data {reason}.
type paramsError struct{ Reason string }
func (e *paramsError) Error() string

func parseInitializeParams(raw json.RawMessage) (initializeParams, *paramsError)
func parseToolsCallParams(raw json.RawMessage) (toolsCallParams, *paramsError)
```

Aturan `parseInitializeParams`:
- `raw` kosong atau bukan object -> `params must be an object`.
- `protocolVersion` hilang atau bukan string -> `protocolVersion must be a string`. String kosong dibiarkan lolos dan akan ditolak `Negotiate` (versi tidak didukung).
- `clientInfo` opsional; bila object, ambil `name` dan `version` bila string; selain itu kosong. Potong dengan `base.TruncateName(x, 128)`.
- `capabilities` diabaikan.

Aturan `parseToolsCallParams` (K15, E6):
- `raw` kosong atau bukan object -> `params must be an object`.
- `name` hilang atau bukan string -> `name must be a string`; string kosong -> `name must not be empty`; `len(name) > 128` (byte) -> `name too long (max 128 bytes)`.
- `arguments`: tidak ada -> `{}`; literal `null` atau bukan object -> `arguments must be an object`; object -> disimpan apa adanya (byte asli, tidak di-decode ulang, agar `project_id` tidak berubah-ubah; K14).
- Field lain (`_meta`) diabaikan.

DTO respons (urutan field JSON dipertahankan oleh struct):

```go
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    capabilities   `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
	Instructions    string         `json:"instructions"`
}
type capabilities struct{ Tools toolsCapability `json:"tools"` }
type toolsCapability struct{ ListChanged bool `json:"listChanged"` } // selalu false
type serverInfo struct{ Name, Version string }                       // tag json name/version

type toolEntry struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations toolAnnotations `json:"annotations"`
}
type toolAnnotations struct{ ReadOnlyHint bool `json:"readOnlyHint"` } // selalu true
type toolsListResult struct{ Tools []toolEntry `json:"tools"` }

type contentItem struct{ Type, Text string }                         // tag type/text
type callToolResult struct {
	Content []contentItem  `json:"content"`
	IsError bool           `json:"isError"`
	Meta    *callToolMeta  `json:"_meta,omitempty"`
}
type callToolMeta struct {
	Truncated     bool `json:"truncated"`
	OriginalBytes int  `json:"original_bytes"`
}

func toToolEntries(tools []toolkit.Tool) []toolEntry // ReadOnlyHint:true dipaksa; urutan input dipertahankan
func toCallToolResult(o base.Outcome) callToolResult  // E7
```

Tes: `params_test.go` (bagian 4.1).

### Langkah 2 — `usecase.go`: orkestrasi dispatch

```go
type Usecase struct {
	log     *slog.Logger
	version string          // d.Version; kosong -> "dev" (K26)
	cat     base.Catalog
	inv     base.ToolInvoker
	audit   auditwriter.Writer
	now     func() time.Time
}

func NewUsecase(log *slog.Logger, version string, cat base.Catalog, inv base.ToolInvoker, audit auditwriter.Writer) *Usecase

// Handle memproses SATU request JSON-RPC (Kind == KindRequest) dan selalu mengembalikan Response.
func (u *Usecase) Handle(ctx context.Context, p base.Principal, req base.Request) base.Response
```

`Handle`: `switch req.Method`:

- `"initialize"` -> `u.initialize(ctx, p, req)`.
- `"ping"` -> `base.Success(req.ID, struct{}{})`.
- `"tools/list"` -> `u.toolsList(ctx, p, req)`.
- `"tools/call"` -> `u.toolsCall(ctx, p, req)`.
- default -> `base.Failure(req.ID, &base.Error{Code: base.CodeMethodNotFound, Message: "Method not found", Data: map[string]string{"method": req.Method}})`. Tanpa audit.

`initialize` (urut):
1. `start := u.now()`.
2. `parseInitializeParams`; salah -> `Failure(-32602, "Invalid params", {reason})`, tanpa audit (E4).
3. `negotiated, ok := base.Negotiate(params.ProtocolVersion)`.
4. `!ok` -> `u.audit.Write(ctx, base.InitializeAudit(p, requested, "", clientName, clientVersion, false, since(start)))`, lalu `Failure(-32602, "Unsupported protocol version", {requested, supported: base.SupportedVersions})` (K7). HTTP tetap 200.
5. ok -> bangun `initializeResult` (`serverInfo{Name: base.ServerName, Version: u.version}`, `Instructions: base.Instructions(p.ProjectName, u.cat.Names())`), tulis `InitializeAudit(..., true, ...)`, kembalikan `base.Success`.

`toolsList`:
1. `start`; `tools := u.cat.List()`; `entries := toToolEntries(tools)` (non-nil, E8).
2. `u.audit.Write(ctx, base.ToolsListAudit(p, len(entries), since(start)))`.
3. `base.Success(req.ID, toolsListResult{entries})`.

`toolsCall`:
1. `parseToolsCallParams`; salah -> `Failure(-32602, "Invalid params", {reason})`, TANPA audit (K15).
2. `out := u.inv.Invoke(ctx, p, params.Name, params.Arguments)` (invoker menulis tepat satu audit, K17).
3. `out.UnknownTool` -> `Failure(-32602, "Unknown tool: " + base.TruncateName(params.Name, 128), nil)` (K16).
4. Selain itu `base.Success(req.ID, toCallToolResult(out))`.

Seluruh jalur `base.Success` yang mengembalikan error dipetakan ke `Failure(-32603, "Internal error")` + `u.log.ErrorContext(ctx, "mcp encode result failed", "method", req.Method, "err", err)` (E9). Usecase tidak menyentuh HTTP.

### Langkah 3 — `handler.go`: transport

```go
type dispatcher interface {
	Handle(ctx context.Context, p base.Principal, req base.Request) base.Response
}

type Handler struct {
	log      *slog.Logger
	uc       dispatcher
	maxBytes int64 // cfg.MCPMaxRequestBytes; hanya untuk data limit_bytes pada 413
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request)
func (h *Handler) MethodNotAllowed(w http.ResponseWriter, r *http.Request)
```

`Post` (urut, E1):
1. `p, ok := base.PrincipalFrom(r.Context())`; `!ok` -> log error `"mcp principal missing"`, tulis 500 `HTTPReject(-32603, "Internal error", nil)`.
2. Content-Type: `mime.ParseMediaType(r.Header.Get("Content-Type"))`; error atau media type != `application/json` -> **415** `HTTPReject(-32600, "Invalid Request", {reason:"Content-Type must be application/json"})`. Parameter (`charset=utf-8`) boleh (K9). `Accept` tidak diperiksa.
3. `body, err := io.ReadAll(r.Body)`:
   - `errors.As(err, new(*http.MaxBytesError))` -> **413** `HTTPReject(-32600, "Invalid Request", {reason:"request body too large", limit_bytes:h.maxBytes})`.
   - error lain (klien memutus) -> 400 `-32700 Parse error` (tanpa log error; log debug).
4. `kind, req, perr := base.ParseMessage(body)`:
   - `KindParseError` -> **400** `-32700 Parse error`.
   - `KindBatch` -> **400** `-32600 Invalid Request` `{reason:"batch not supported"}`.
   - `KindInvalid` -> **400** `-32600 Invalid Request` (pakai `perr` bila ada; `data.reason` dari `perr.Data`).
5. Header versi: bila bukan `(kind == KindRequest && req.Method == "initialize")`, panggil `base.KnownHeaderVersion(r.Header.Get(base.HeaderProtocolVersion))`; `!ok` -> **400** `-32600 Invalid Request` `{reason:"unsupported MCP-Protocol-Version", supported: base.SupportedVersions}` (K8, E2).
6. `KindNotification` atau `KindResponse` -> `w.WriteHeader(202)` tanpa body (termasuk `notifications/initialized`).
7. `KindRequest` -> `resp := h.uc.Handle(r.Context(), p, req)`; tulis 200.

Helper `writeJSON(w, status, base.Response)`: `json.Marshal`; gagal -> log error dan tulis 500 dengan body statis `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"Internal error"}}`; sukses -> `Content-Type: application/json`, `WriteHeader(status)`, `Write(b)`.

`MethodNotAllowed`: header `Allow: POST`, status **405**, body `HTTPReject(-32600, "Invalid Request", {reason:"method not allowed; use POST"})`. Tidak membaca body, tidak butuh Principal.

Catatan: respons ke klien tidak pernah memuat `Reason` auth dan tidak pernah memuat pesan error Go mentah.

### Langkah 4 — `init.go`: pendaftaran route

```go
func Init(d app.Deps, cat base.Catalog, inv base.ToolInvoker,
	admit func(http.Handler) http.Handler, audit auditwriter.Writer) {

	cfg := d.Config
	uc := NewUsecase(d.Log, d.Version, cat, inv, audit)
	h := &Handler{log: d.Log, uc: uc, maxBytes: cfg.MCPMaxRequestBytes}

	d.MCP.With(
		middleware.OriginGuard(cfg.MCPAllowedOrigins),
		middleware.BodyLimit(cfg.MCPMaxRequestBytes),
		admit,
	).Post("/", h.Post)
	d.MCP.Get("/", h.MethodNotAllowed)
	d.MCP.Delete("/", h.MethodNotAllowed)
}
```

`init.go` hanya merakit; tidak ada logika. Package `endpoint` tidak meng-import `authenticate` atau `callaudit` (K3): `admit` dan `inv` masuk sebagai fungsi/interface dari `mcpgateway/base.go`.

### Langkah 5 — Ganti stub `Init`

Hapus stub `endpoint.Init` dari fase F. Pastikan `mcpgateway/base.go` memanggil `endpoint.Init(d, cat, inv, admit, audit)` sesuai kontrak overview bagian 8 (tanda tangan tidak berubah).

### Langkah 6 — Tes unit feature

Tulis `params_test.go`, `usecase_test.go`, `handler_test.go`, `init_test.go` (bagian 4). Jalankan `go test ./internal/domain/discovery/...` dan `go test ./internal/archtest/...`.

### Langkah 7 — Tes lintas feature (I1)

Buat `backend/internal/domain/discovery/internal/mcpgateway/base_test.go` (package `mcpgateway`, bagian 4.5). Butuh `callaudit` dan `authenticate` nyata (U1, U2) sehingga dikerjakan setelah keduanya selesai. Bila belum selesai, tulis tesnya dan tandai `t.Skip` HANYA sampai U1/U2 merge; jangan di-merge dengan skip.

### Langkah 8 — Tes integrasi Postgres (I2)

Buat `backend/cmd/internal/server/mcpgateway_integration_test.go` (`//go:build integration`, package `server`), memakai helper yang sudah ada di `run_integration_test.go` (`testDBURL`, `testCfg`, hook app). Overview menamai file ini `mcpgateway_integration_test.go`; permintaan awal menyebutnya `gateway_integration_test.go`, keduanya merujuk satu file yang sama. Tidak boleh di dalam `mcpgateway` (melanggar rules 4.1, archtest memindai tag `integration`). Isi di bagian 4.6.

### Langkah 9 — Verifikasi

`cd backend && go build ./... && go vet ./... && go test ./...`, `go test -tags integration ./cmd/internal/server/...` (dengan `TEST_DATABASE_URL`), `golangci-lint run`, archtest hijau. Smoke manual:

```
curl -s -X POST localhost:8080/mcp -H 'Authorization: Bearer <key>' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}'
```

---

## 4. Test Scenario

Penamaan `TestT07_ACn_...` untuk kriteria penerimaan. Semua tes unit memakai tabel (`[]struct{name ...}`), tanpa mock framework; fake ditulis tangan.

### 4.1 `params_test.go`

`parseInitializeParams`:
- valid penuh; valid tanpa `clientInfo`; `clientInfo.name` angka -> kosong; nama 200 byte -> dipotong 128.
- params kosong, array, string -> error `params must be an object`.
- `protocolVersion` hilang / angka / null -> error.
- `protocolVersion:""` -> lolos (ditolak di usecase).

`parseToolsCallParams`:
- `{"name":"read_file"}` -> `Arguments == {}`.
- `{"name":"read_file","arguments":{"path":"a"}}` -> byte `arguments` persis sama dengan input (termasuk spasi).
- `arguments` berisi `project_id` -> tetap utuh (K14), tidak dibuang.
- `name` hilang, angka, kosong, 128 byte (lolos), 129 byte (gagal), multibyte yang total byte > 128 (gagal).
- `arguments` `null`, array, string, angka -> `arguments must be an object`.
- params hilang / array -> `params must be an object`.

`toToolEntries`: `ReadOnlyHint` selalu true walau tool mencoba lain (tool tidak punya field itu; tes memastikan JSON hasil memuat `"readOnlyHint":true`); slice kosong -> JSON `[]` bukan `null`; `title` kosong dihilangkan; `inputSchema` tidak diubah.

`toCallToolResult`: ok; `Result.IsError` -> `isError:true`; `ErrorCode:"timeout"` dengan `IsError` false -> `isError:true` (E7); `Truncated` -> `_meta` ada; tidak truncated -> kunci `_meta` tidak ada; `Text` kosong -> `content[0].text == ""`.

### 4.2 `usecase_test.go`

Fake: `fakeInvoker` (mencatat `name`, `args`, `Principal`; mengembalikan `Outcome` terprogram), `fakeAudit` (menyimpan `[]record.Request`), katalog nyata dari `base.NewCatalog`.

Tabel per metode:
- `initialize`: tiap versi didukung -> `protocolVersion` sama dengan yang diminta, `serverInfo.name == "grimoire"`, `serverInfo.version == "1.4.0"` (kosong -> `"dev"`), `capabilities.tools.listChanged == false`, `instructions` memuat nama project dari Principal dan nama tool katalog; tepat 1 audit `mcp.initialize` status `ok` dengan Args `protocol_version/client_name/client_version`, `Result.protocol_version`.
- `initialize` `2024-11-05` -> error `-32602`, `message "Unsupported protocol version"`, `data.requested == "2024-11-05"`, `data.supported` = tiga versi urut terbaru dulu; 1 audit `mcp.initialize` status `error`, `Result.error_code == "unsupported_version"`.
- `initialize` params salah bentuk -> `-32602` dan 0 audit (E4).
- `initialize` katalog kosong -> instructions memuat "Belum ada tool yang terdaftar."
- `ping` -> `result` persis `{}`, 0 audit, invoker tidak dipanggil. `id` string dan number dipertahankan byte-per-byte.
- `tools/list`: katalog tiga tool dengan urutan registrasi acak -> urut nama; semua `readOnlyHint:true`; tepat 1 audit `mcp.tools_list` dengan `Result.count == 3`; katalog kosong -> `{"tools":[]}` dan `count == 0`.
- `tools/call` valid -> invoker dipanggil sekali dengan `name` dan `args` persis; hasil `content[0].text`, `isError:false`; 0 audit dari endpoint (audit dilakukan invoker).
- `tools/call` argumen `{"project_id":"B"}` -> invoker menerima args utuh dan Principal A (`endpoint` tidak membaca/menyentuh `project_id`; AC3 sisi endpoint).
- `tools/call` `Outcome.UnknownTool` -> `-32602`, `message == "Unknown tool: write_file"`; nama 300 byte dipotong 128 pada pesan.
- `tools/call` `Outcome{Result.IsError:true}` -> HTTP-level sukses dengan `isError:true`.
- `tools/call` `Outcome{ErrorCode:"timeout", Result.IsError:true, Text:"..."}` -> `isError:true`.
- `tools/call` `Truncated` -> `_meta.truncated:true`, `original_bytes` sesuai.
- `tools/call` params salah (name bukan string, kosong, 129 byte, arguments array, arguments null) -> `-32602` + `data.reason`, invoker TIDAK dipanggil, 0 audit.
- `resources/list`, `prompts/list`, `completion/complete`, `""` tidak mungkin (ParseMessage menolak) -> `-32601` dengan `data.method`; 0 audit; invoker tidak dipanggil.
- Audit writer dipanggil sebelum respons dikembalikan (urutan: tulis audit lalu return) dan kegagalan audit (writer palsu yang tidak menyimpan apa-apa) tidak mengubah hasil (fail-open terjamin di `auditwriter`; tes memakai writer palsu yang mencatat panggilan).
- Durasi: `now` palsu maju 25 ms -> `Result.duration_ms == 25` pada `mcp.initialize` dan `mcp.tools_list`.

### 4.3 `handler_test.go`

Memakai `httptest.NewRecorder` dengan `dispatcher` palsu (mencatat jumlah panggilan dan request) dan Principal disuntik lewat `base.WithPrincipal`. Tabel transport:

| Kasus | Input | Ekspektasi |
|---|---|---|
| Content-Type kosong | tanpa header | 415, `-32600`, `reason` Content-Type, dispatcher 0 panggilan |
| Content-Type `text/plain` | | 415 |
| Content-Type `application/json; charset=utf-8` | | lolos |
| Content-Type `APPLICATION/JSON` | | lolos |
| Body kosong | `""` | 400 `-32700` |
| JSON rusak | `{"jsonrpc":` | 400 `-32700` |
| Batch | `[{"jsonrpc":"2.0","id":1,"method":"ping"}]` | 400 `-32600`, `data.reason == "batch not supported"` |
| Bukan object | `123`, `"x"`, `null` | 400 `-32600` |
| `jsonrpc` salah | `{"jsonrpc":"1.0",...}` | 400 `-32600` |
| `id:null` | `{"jsonrpc":"2.0","id":null,"method":"ping"}` | 400 `-32600` (K11) |
| `id` bool/object | | 400 `-32600` |
| `params` array | | 400 `-32600` |
| Body > batas | body 70 KB dengan `BodyLimit(64KB)` di depan handler | 413 `-32600`, `data.limit_bytes == 65536` |
| Body tepat di batas | | tidak 413 |
| Notification | `notifications/initialized` | 202, body kosong (`rec.Body.Len()==0`), tanpa `Content-Type`, dispatcher 0 panggilan |
| Notification lain | `notifications/cancelled` | 202 |
| Respons klien | `{"jsonrpc":"2.0","id":1,"result":{}}` | 202 |
| Header versi tak dikenal | `MCP-Protocol-Version: 2024-11-05` + `ping` | 400 `-32600`, `data.supported` tiga versi |
| Header versi tak dikenal + `initialize` | | lolos (dilewati, E1) |
| Header versi tak dikenal + notification | | 400 (E2) |
| Header versi tiap versi didukung | | lolos |
| Header versi tidak ada + `tools/list` | | lolos (dianggap `2025-03-26`) |
| Principal tidak ada | tanpa `WithPrincipal` | 500 `-32603`, dispatcher 0 panggilan |
| Request sukses | `ping` | 200, `Content-Type: application/json`, tanpa header `Mcp-Session-Id`, body `{"jsonrpc":"2.0","id":1,"result":{}}` |
| `id` string | | `id` kembali string persis |
| Dispatcher mengembalikan error JSON-RPC | | tetap HTTP 200 (K10) |
| `MethodNotAllowed` GET dan DELETE | | 405, `Allow: POST`, body `-32600` `reason "method not allowed; use POST"`, Principal tidak diperlukan |

Semua 4xx/5xx: body adalah JSON valid, `jsonrpc:"2.0"`, `id:null`. Tidak ada string `Reason` auth, trace Go, atau path internal pada body mana pun (tes pencarian substring pada body kasus 500).

### 4.4 `init_test.go`

Router chi nyata sebagai `d.MCP`, `app.Deps{Config, Log, MCP}` dengan `admit` palsu yang menghitung panggilan dan menyuntik Principal; invoker palsu.
- `Origin: https://evil.example` -> 403 (format REST `OriginGuard`), `admit` 0 panggilan, dispatcher 0 (AC9).
- `Origin` di `MCPAllowedOrigins` -> lolos; tanpa `Origin` -> lolos; `Origin` sama dengan Host -> lolos.
- Urutan rantai: dengan `admit` palsu yang menolak 401, body besar (> batas) tidak pernah dibaca (pembaca body palsu mencatat `Read` tidak dipanggil) — bukti K24.
- `GET /` dan `DELETE /` -> 405 `Allow: POST` dan `admit` 0 panggilan (tanpa auth).
- `POST /` -> `admit` 1 panggilan.
- `BodyLimit` memakai `cfg.MCPMaxRequestBytes` (set 1024, kirim 2 KB -> 413).

### 4.5 Lintas feature — `mcpgateway/base_test.go` (I1)

Rakit `app.Deps{Ctx, Config (default + MCP_TOOL_TIMEOUT kecil), Log, Bus: bus.NewRegistry(), MCP: chi router, Version: "1.4.0"}`, handler bus palsu `verify` (key `gr_aaaaaaaa_<secret>` -> project A, lainnya `Found=false`), `get-project`, `touch`, dan audit palsu yang menyimpan semua `record.Request`; tool palsu `read_file` (mencatat `Call` yang diterima). Lalu `mcpgateway.Register(d, []toolkit.Tool{readFileFake})`.

- `TestT07_AC1_JSONRPCFlow`: `initialize` (200, `protocolVersion` = yang diminta, `serverInfo.name == "grimoire"`) -> `notifications/initialized` (202, body kosong) -> `tools/list` (berisi `read_file`, `annotations.readOnlyHint == true`) -> `tools/call read_file` (200, `content[0].text`); audit berisi `mcp.initialize`, `mcp.tools_list`, `mcp.read_file` (tepat satu masing-masing).
- `TestT07_AC3_ProjectIDFromKeyOnly`: arguments `{"path":"x","project_id":"<B>"}` -> tool palsu menerima `Call.ProjectID == A`, `Call.Arguments` masih memuat `project_id` B apa adanya; audit `ProjectID == A`.
- `TestT07_AC4_ExactlyOneAuditNoContent`: tool mengembalikan teks `SECRET-CONTENT-123` -> tepat 1 entri `mcp.read_file`, string itu tidak ada di entri mana pun.
- `TestT07_AC8_RegisterPanicsOnDuplicate`: dua `read_file` -> panic, pesan diawali `mcpgateway:` dan memuat `"read_file"`.
- `TestT07_AC9_ForeignOrigin`: `Origin: https://evil.example` -> 403 dan `verify` TIDAK dipanggil; Origin di `MCP_ALLOWED_ORIGINS` lolos; tanpa Origin lolos.
- `GET`/`DELETE /mcp` -> 405 `Allow: POST` dan `verify` tidak dipanggil.
- `tools/call` nama tak dikenal -> `-32602 Unknown tool: write_file`, tepat 1 audit `mcp.unknown_tool`.
- `tools/call` params tidak valid -> `-32602`, 0 audit tool.
- `tools/list` pada katalog kosong (`Register(d, nil)`) -> `{"tools":[]}`.
- Mix: tool yang mengembalikan 500 KB -> respons memuat `_meta.truncated:true` dan `len(text) <= MCPMaxResponseBytes` (AC7 lewat jalur penuh).

### 4.6 Integrasi Postgres — `cmd/internal/server/mcpgateway_integration_test.go` (I2)

Tag `integration`; Postgres nyata (`TEST_DATABASE_URL`), `RUN_MIGRATIONS_ON_START=true`, `COOKIE_SECURE=false`. Buat admin lewat bus `core.auth.create-admin` (hook app), login `/api/auth/login`, buat project dan key lewat REST T03/T04 (key asli), lalu hit `POST /mcp` pada server nyata.

- `TestT07_AC1_RealKeyInitializeAndToolsList`: `initialize` 200; `tools/list` 200 dengan `tools: []` (T08 menambah asersi `read_file`); baris `audit_log` `mcp.initialize` dengan `actor_type=api_key`, `actor_id=<keyId>`, `project_label=<nama project>`; `last_used_at` key terisi (tunggu publish `touch` selesai dengan polling singkat berbatas 2 detik).
- `TestT07_AC2_UniformRejection` (versi DB): tanpa key, key acak, key dicabut -> body identik dan 3 baris `mcp.auth.rejected`.
- `TestT07_AC5_RevokeTakesEffectNextRequest`: `tools/list` 200 -> revoke via REST -> request berikutnya 401.
- `TestT07_EndpointTransportOverRealServer`: dengan key valid: `Content-Type: text/plain` -> 415; JSON rusak -> 400 `-32700`; batch -> 400; `ping` -> `{}`; `resources/list` -> `-32601`; `GET /mcp` -> 405 tanpa key; `Origin` asing -> 403.

Tes yang memanggil `core.audit.record` memastikan hanya `initialize`/`tools_list` yang ditulis `endpoint` (tidak ada audit untuk `ping`, `-32601`, notification).

### 4.7 Cek otomatis

`go vet`, `golangci-lint`, archtest (package `endpoint` hanya meng-import `app`, `middleware`, `base`, `base/auditwriter`, `toolkit`, `record`, stdlib, `chi` bila perlu; tidak meng-import `authenticate`, `callaudit`, domain lain).

---

## 5. Migration

Tidak ada. `endpoint` tidak menyentuh SQL, tidak ada tabel, tidak ada `repository.go`.

---

## 6. E2E Scenarios (Playwright)

File `frontend/e2e/mcp-gateway.spec.ts` dan helper `frontend/e2e/helpers/mcp.ts` (struktur di overview bagian 11/13). Kasus yang dimiliki `endpoint` (kasus penolakan auth 401/429 dimiliki plan `authenticate`, dikerjakan pada file yang sama di langkah I6 overview). Skenario memakai `request` Playwright (API), bukan UI, kecuali verifikasi audit-log.

Helper `mcp.ts`: `mcpPost(request, rawKey | null, body, headers?)` (default `Content-Type: application/json`, `Authorization: Bearer <rawKey>` bila ada), `initializeBody(id, version?)`, `toolsListBody(id)`, `pingBody(id)`.

Positif:
1. Login admin -> buat project -> buat key (`createKeyApi`) -> `initialize` (200, `result.serverInfo.name == "grimoire"`, `result.protocolVersion == "2025-06-18"`, `result.instructions` memuat nama project, `result.capabilities.tools.listChanged == false`) -> `notifications/initialized` (202, body kosong) -> `tools/list` (200, `result.tools` array) -> buka `/audit-log?actor_type=api_key&actor_id=<keyId>` dan lihat baris `mcp.initialize` dan `mcp.tools_list`.
2. `X-API-Key` memberi hasil sama dengan Bearer.
3. `ping` -> `result` `{}`; `id` string `"p-1"` kembali sebagai string.
4. `initialize` tiga versi (`2025-11-25`, `2025-06-18`, `2025-03-26`) masing-masing mengembalikan versi yang diminta.
5. `Content-Type: application/json; charset=utf-8` diterima.
6. Header `MCP-Protocol-Version: 2025-06-18` pada `tools/list` diterima.

Negatif:
7. `initialize` dengan `2024-11-05` -> 200 dengan `error.code == -32602`, `error.data.requested == "2024-11-05"`, `error.data.supported` tiga versi; audit-log menampilkan `mcp.initialize` berstatus error.
8. `Origin: https://evil.example` -> 403.
9. `GET /mcp` -> 405 dengan header `allow: POST`; `DELETE /mcp` sama. Keduanya tanpa key.
10. `resources/list` -> 200 dengan `error.code == -32601`.
11. `Content-Type: text/plain` -> 415 dengan `error.code == -32600`.
12. Body bukan JSON -> 400 `error.code == -32700`.
13. Batch `[{...}]` -> 400 `-32600`.
14. Body 70 KB -> 413 `-32600`.
15. `MCP-Protocol-Version: 1999-01-01` -> 400 `-32600`.
16. `tools/call` `name:"nope"` -> 200 `error.code == -32602`, `message` "Unknown tool: nope"; audit-log menampilkan `mcp.unknown_tool`.
17. `tools/call` `params:{"name":""}` -> `-32602` dan tidak ada baris audit baru (hitung baris filter aksi sebelum/sesudah).

`tools/call` ke tool nyata (`read_file`) ditambahkan oleh T08, di luar T07.

---

## 7. Request/Response body

Semua `POST /mcp` memakai `Content-Type: application/json` dan header auth. Contoh key: `gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA`.

### 7.1 initialize

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
Params tidak valid (HTTP 200, tanpa audit), request `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`:
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid params","data":{"reason":"protocolVersion must be a string"}}}
```

### 7.2 notifications/initialized

```json
{"jsonrpc":"2.0","method":"notifications/initialized"}
```
Response: `202 Accepted`, tanpa body. Respons klien (`{"jsonrpc":"2.0","id":1,"result":{}}`) juga 202 tanpa body.

### 7.3 ping

```json
{"jsonrpc":"2.0","id":"p-1","method":"ping"}
```
```json
{"jsonrpc":"2.0","id":"p-1","result":{}}
```

### 7.4 tools/list

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
Katalog kosong (T07): `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`.

### 7.5 tools/call

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
Kegagalan tool:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"File \"docs/auth/logn.md\" tidak ditemukan. Mungkin maksudnya: docs/auth/login.md"}],"isError":true}}
```
Timeout / internal / panic (dibuat `callaudit`, dibungkus `endpoint`):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Tool read_file melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```
Tool tidak dikenal (HTTP 200):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Unknown tool: write_file"}}
```
Params tidak valid (HTTP 200, tanpa audit):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Invalid params","data":{"reason":"arguments must be an object"}}}
```
Variasi `reason`: `params must be an object`, `name must be a string`, `name must not be empty`, `name too long (max 128 bytes)`.

### 7.6 Metode tidak dikenal

```json
{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"Method not found","data":{"method":"resources/list"}}}
```

### 7.7 Penolakan level HTTP milik endpoint

Semua berbentuk JSON-RPC dengan `id:null`; header `Content-Type: application/json`.

400 parse error:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}
```
400 batch / invalid / `id:null`:
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
500 Principal hilang / serialisasi gagal (bug internal, tidak membocorkan detail):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"Internal error"}}
```
403 Origin asing (format REST dari `OriginGuard`, bukan JSON-RPC, K5):
```json
{"error":{"code":"FORBIDDEN","message":"origin tidak diizinkan"},"trace_id":"01HZX3K9Q7"}
```
401/429/503 dihasilkan `authenticate` (lihat plan `usecase-authenticate.md`), bukan `endpoint`.

### 7.8 Audit yang ditulis `endpoint` (dikirim ke `core.audit.record` lewat `auditwriter`)

`mcp.initialize` sukses:
```json
{
  "ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)",
  "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments",
  "Action": "mcp.initialize",
  "Args": {"protocol_version": "2025-06-18", "client_name": "claude-code", "client_version": "2.1.0"},
  "Result": {"protocol_version": "2025-06-18", "duration_ms": 1},
  "Status": "ok", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"
}
```
`mcp.initialize` versi tidak didukung: sama, `Args.protocol_version = "2024-11-05"`, `Result = {"error_code":"unsupported_version","duration_ms":1}`, `Status = "error"`.

`mcp.tools_list`:
```json
{
  "ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)",
  "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments",
  "Action": "mcp.tools_list", "Args": {}, "Result": {"count": 3, "duration_ms": 1},
  "Status": "ok", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"
}
```

---

## 8. Changes (per file/folder)

| File | Aksi | Isi |
|---|---|---|
| `.../mcpgateway/endpoint/init.go` | baru (ganti stub F7) | `Init(d, cat, inv, admit, audit)`; route POST/GET/DELETE |
| `.../mcpgateway/endpoint/handler.go` | baru | `Handler.Post`, `Handler.MethodNotAllowed`, `writeJSON` |
| `.../mcpgateway/endpoint/params.go` | baru | parser params, DTO respons, `toToolEntries`, `toCallToolResult` |
| `.../mcpgateway/endpoint/usecase.go` | baru | `Usecase.Handle` + `initialize`/`toolsList`/`toolsCall` |
| `.../mcpgateway/endpoint/{params,usecase,handler,init}_test.go` | baru | bagian 4.1-4.4 |
| `.../mcpgateway/base_test.go` | baru | tes lintas feature (bagian 4.5) |
| `backend/cmd/internal/server/mcpgateway_integration_test.go` | baru | tes Postgres (bagian 4.6) |
| `frontend/e2e/helpers/mcp.ts` | baru | helper panggilan MCP |
| `frontend/e2e/mcp-gateway.spec.ts` | baru | skenario bagian 6 (skenario auth 401/429 dari plan `authenticate`) |
| `docs/mcp-gateway.md` | dikerjakan langkah I3 overview | bagian protokol memakai contoh bagian 7 |

Tidak berubah oleh usecase ini: `base/*` (hanya dipakai), `authenticate/`, `callaudit/`, `lib/constants/bus.go`, `api/**`, `backend/migrations/**`, `go.mod`.

## 9. Risiko dan catatan untuk implementer

- `BodyLimit` T00 bersifat lazy dan membungkus body asli: root router sudah memasang `HTTPDefaultBodyBytes`, jadi batas `/mcp` bisa lebih besar atau lebih kecil dari batas root. Tes 413 harus memakai konfigurasi `MCP_MAX_REQUEST_BYTES` yang berbeda dari default root untuk membuktikan batas terdalam menang.
- `http.MaxBytesReader` menandai koneksi untuk ditutup setelah melewati batas; respons 413 tetap ditulis normal.
- Jangan me-log body request atau argumen tool (rules 10.3). Log error endpoint hanya memuat `method` dan `err`.
- `Principal.IP` dan `UserAgent` diisi `authenticate`; `endpoint` tidak membaca `r.RemoteAddr` sendiri.
- Bila `Handle` dipanggil dengan `KindRequest` yang `ID`-nya kosong, itu bug pemanggil; `ParseMessage` menjamin `ID` terisi untuk `KindRequest`, sehingga tidak perlu penanganan khusus.
