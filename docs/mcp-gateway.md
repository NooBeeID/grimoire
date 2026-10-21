# MCP Gateway

Gateway MCP Grimoire melayani agent AI (Claude Code dan sejenisnya) lewat `POST /mcp`. Transport: MCP Streamable HTTP versi stateless. Satu pesan JSON-RPC 2.0 per request, respons selalu JSON biasa (tanpa SSE, tanpa `Mcp-Session-Id`). Seluruh tool bersifat read-only dan terikat ke satu project lewat API key.

## Ringkasan protokol

| Item | Nilai |
|---|---|
| Endpoint | `POST /mcp`. `GET` dan `DELETE` dijawab 405 dengan `Allow: POST` |
| Versi MCP | `2025-11-25`, `2025-06-18`, `2025-03-26` (negosiasi di `initialize`) |
| Auth | `Authorization: Bearer gr_<prefix>_<secret>` atau `X-API-Key` (Bearer menang) |
| Content-Type | `application/json` (parameter `charset` boleh) |
| Batch | tidak didukung (400) |
| Capabilities | `tools` saja, `listChanged: false` |

Metode yang ditangani: `initialize`, `ping`, `tools/list`, `tools/call`. Notification (`notifications/initialized`, dll.) dan respons klien dijawab `202` tanpa body. Metode lain menghasilkan `-32601 Method not found` (HTTP 200).

## Header

| Header | Keterangan |
|---|---|
| `Authorization` / `X-API-Key` | API key project |
| `MCP-Protocol-Version` | Opsional. Bila ada harus salah satu versi didukung, selain itu 400. Tidak diperiksa pada `initialize`. Tanpa header dianggap `2025-03-26` |
| `Origin` | Bila ada dan bukan origin yang sama dengan Host atau tidak ada di `MCP_ALLOWED_ORIGINS`, ditolak 403 (format error REST) |

Urutan pemeriksaan POST: Origin, batas body, autentikasi dan rate limit, Content-Type, baca body, parse, header versi, dispatch.

## Kode HTTP dan error

| HTTP | Kode JSON-RPC | Penyebab |
|---|---|---|
| 200 | hasil atau error JSON-RPC | Semua hasil dispatch, termasuk `-32601`, `-32602`, dan tool gagal (`isError: true`) |
| 202 | tanpa body | Notification atau respons klien |
| 400 | `-32700` | Body kosong atau JSON rusak |
| 400 | `-32600` | Batch, bukan object, `jsonrpc` salah, `id` null/bool/object, `params` bukan object, header versi tak dikenal |
| 401 | `-32001` | Key tidak ada, tidak valid, atau dicabut. Body identik untuk semua alasan |
| 403 | format REST | Origin asing |
| 405 | `-32600` | `GET`/`DELETE` |
| 413 | `-32600` | Body lebih besar dari `MCP_MAX_REQUEST_BYTES` (`data.limit_bytes`) |
| 415 | `-32600` | Content-Type bukan `application/json` |
| 429 | `-32003` | Rate limit |
| 500 | `-32603` | Bug internal; detail tidak dibocorkan |
| 503 | `-32004` | Verifikasi key tidak tersedia |

Error level protokol dalam HTTP 200: `-32602 Invalid params` (dengan `data.reason`), `-32602 Unsupported protocol version` (dengan `data.requested` dan `data.supported`), `-32602 Unknown tool: <nama>`, `-32601 Method not found` (dengan `data.method`).

## Contoh

`initialize`:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude-code","version":"2.1.0"}}}
```

```json
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{"listChanged":false}},"serverInfo":{"name":"grimoire","version":"1.4.0"},"instructions":"..."}}
```

`tools/list` memaksa `annotations.readOnlyHint: true` pada setiap tool; katalog kosong menghasilkan `{"tools":[]}`. Tidak ada paginasi.

`tools/call` (params `name` wajib string 1-128 byte, `arguments` opsional dan harus object):

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/auth/login.md"}}}
```

Hasil sukses: `{"content":[{"type":"text","text":"..."}],"isError":false}`. Kegagalan tool, timeout, dan panic menjadi `isError: true` dengan pesan yang aman bagi LLM. Respons yang dipotong membawa `_meta: {"truncated": true, "original_bytes": N}`. `project_id` selalu berasal dari API key; argumen `project_id` dari klien tidak pernah dipakai untuk menentukan project.

## Audit

`endpoint` menulis `mcp.initialize` (status `ok`, atau `error` bila versi tidak didukung) dan `mcp.tools_list`. Setiap `tools/call` menghasilkan tepat satu entri `mcp.<tool>` atau `mcp.unknown_tool` (ditulis `callaudit`). `ping`, `-32601`, notification, params tidak valid, dan penolakan transport tidak diaudit di sini. Penolakan auth menjadi `mcp.auth.rejected`, rate limit menjadi `mcp.rate_limited`. Audit fail-open dan tidak pernah memuat isi file.

### Ringkasan audit `search_knowledge`

Entri `mcp.search_knowledge` membawa `result` berikut (selain `bytes`, `original_bytes`, `truncated`, `duration_ms`, dan `error_code` dari gateway):

| Kunci | Tipe | Keterangan |
|---|---|---|
| `count` | integer | Jumlah hasil yang dikembalikan ke agent |
| `total` | integer | Jumlah dokumen cocok sebelum `limit` |
| `result_paths` | array string | Path hasil teratas, urut sama dengan keluaran, maksimal 10. Tiap path dipotong pada batas UTF-8 1024 byte (sama dengan batas `target` audit `read_file`, sehingga bisa dicocokkan persis). Tanpa hasil: `[]` (bukan `null`) |
| `result_paths_truncated` | boolean | Hanya ada (`true`) bila sebagian path dibuang karena anggaran ukuran (6144 byte JSON) agar `result` tidak melewati batas 8192 byte `Sanitize` |

Contoh:

```json
{"count": 15, "total": 42, "result_paths": ["docs/a.md", "docs/b.md", "...hingga 10 path"], "bytes": 3120, "original_bytes": 3120, "truncated": false, "duration_ms": 21}
```

Snippet, skor, isi file, dan query tidak masuk ke `result` (query tercatat di `args.query`). Path adalah metadata yang juga tercatat sebagai `target` saat `read_file`, bukan isi file.

Alasan membalik keputusan T08 ("tanpa path hasil"): metrik kualitas pencarian T11 (follow-up `read_file`, rank yang dibuka, reformulasi) hanya dapat dihitung bila urutan hasil tercatat. Batas 10 path dan anggaran 6144 byte menjaga baris audit kecil; `result` yang melewati 8192 byte akan diganti seluruhnya oleh `{_truncated, ...}` dan `count`/`total` hilang, sehingga entity memotong sebelum itu terjadi. Entri sebelum T11 tidak memiliki `result_paths` dan dilewati oleh laporan kualitas pencarian.

## Environment

| Env | Default | Fungsi |
|---|---|---|
| `MCP_RATE_PER_MINUTE` | 120 | Batas request per key per menit |
| `MCP_AUTH_FAIL_PER_MINUTE` | 20 | Batas auth gagal per IP per menit |
| `MCP_MAX_REQUEST_BYTES` | 65536 | Batas body request |
| `MCP_MAX_RESPONSE_BYTES` | 102400 | Batas teks hasil tool sebelum dipotong |
| `MCP_TOOL_TIMEOUT` | 10s | Batas waktu satu tool (maks 60s) |
| `MCP_ALLOWED_ORIGINS` | kosong | Daftar origin `scheme://host[:port]` yang diizinkan |

## Menambah tool

1. Buat `[]toolkit.Tool` di module tool (`lib/contracts/discovery/mcpgateway/toolkit`). Nama cocok `^[a-z][a-z0-9_]{0,63}$`, bukan nama terlarang (`initialize`, `tools_list`, `unknown_tool`, `auth`, `rate_limited`), dan unik.
2. Tambahkan aksi `mcp.<nama>` ke `record.Actions()` (`lib/contracts/core/audit/record/actions.go`).
3. Tambahkan satu baris `tools = append(tools, ...)` di `backend/internal/domain/discovery/base.go`.

Katalog tidak valid membuat server panic saat start dengan pesan berawalan `mcpgateway:`. Tool tidak boleh menulis audit sendiri; hormati `ctx` (deadline `MCP_TOOL_TIMEOUT`).

## Tool knowledge

Module `discovery/knowledge-tools` (T08) mendaftarkan tiga tool read-only. Project selalu dari API key; tidak ada argumen `project_id` (argumen itu ditolak). Argumen ketat (`additionalProperties: false`): field tak dikenal atau tipe salah menghasilkan `isError` dengan penjelasan field yang salah. Nilai di luar rentang ditolak, bukan dipotong. Hasil sukses membawa teks, `structuredContent` (object JSON), dan metadata audit tanpa isi file (path hasil pencarian dicatat sebagai `result_paths`, lihat Audit).

| Tool | Argumen | Keterangan |
|---|---|---|
| `list_files` | `path` (string, maks 1024, kosong = akar), `recursive` (boolean, default `false`), `limit` (1-500, default 200), `offset` (0-1000000, default 0) | Tanpa `recursive`, satu tingkat dengan subfolder sebagai entri `[folder]`. Terpotong: `[ada N file lagi, panggil list_files dengan offset=...]`. |
| `read_file` | `path` (wajib, 1-1024), `offset` (0-2147483647, default 0), `limit` (1 sampai `MCP_MAX_RESPONSE_BYTES-512`, default 40000) | `offset` dan `limit` dalam BYTE (bukan karakter), dipotong pada batas UTF-8. Terpotong: `[terpotong: lanjutkan dengan offset=N dari total M byte]`. File tidak ada: `isError` dengan saran path mirip. |
| `search_knowledge` | `query` (wajib, 1-200), `path` (prefix opsional), `limit` (1-20, default 10) | Full-text, bukan semantik. Frasa dalam kutip dicari utuh. Kata kunci di cuplikan bertanda `**...**`. |

Contoh `tools/call`:

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_files","arguments":{"path":"docs","recursive":true,"limit":50}}}
```

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/guides/a.md","offset":40000,"limit":40000}}}
```

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"\"rate limit\"","path":"docs","limit":5}}}
```

Kontrak `tools/list` dibekukan di `backend/internal/domain/discovery/internal/knowledgetools/testdata/tools_list.golden.json` (perbarui dengan `go test ./internal/domain/discovery/internal/knowledgetools/ -update` lalu periksa diff).

## Tool API spec

Module `discovery/api-spec-tools` (T09) mendaftarkan tiga tool read-only untuk menelusuri API spec project secara bertahap: `list_api_specs` lalu `list_api_endpoints` lalu `get_api_endpoint`. Aturan umum sama dengan tool knowledge: project dari API key (argumen `project_id` ditolak), argumen ketat, nilai di luar rentang ditolak. Hasil berupa teks (tanpa `structuredContent`) plus metadata audit; gateway menambah `bytes` ke audit.

| Tool | Argumen | Keterangan |
|---|---|---|
| `list_api_specs` | tidak ada | Daftar spec: nama, judul, versi API, versi OpenAPI, jumlah endpoint, waktu diperbarui. Kolom nama dipakai sebagai argumen `spec`. |
| `list_api_endpoints` | `spec` (maks 80), `tag` (maks 200, persis), `query` (maks 200, cocok sebagian), `limit` (1-300, default 100), `offset` (0-1000000, default 0) | Satu baris per operasi, dikelompokkan per spec (`== nama ==`). Terpotong: `[ada N endpoint lagi, panggil list_api_endpoints dengan offset=...]`. Spec tak dikenal: `isError` dengan daftar spec valid. |
| `get_api_endpoint` | `spec` (wajib bila project punya lebih dari satu spec), `method` + `path` (path persis seperti di spec, termasuk `{param}`) ATAU `operation_id`, `schema_depth` (1-8, default 4) | Detail operasi sebagai JSON satu baris: operasi dengan `$ref` ter-inline, `servers`, `securitySchemes`, dan `markers`. Tidak ketemu: `isError` dengan saran `Similar:`. |

Penanda pada JSON detail:

- `{"$circular": ref}`: skema merujuk dirinya sendiri dan tidak dibuka lagi (didaftar di `markers.circular`).
- `{"$ref": ref}`: tidak dibuka karena batas `schema_depth` (didaftar di `markers.collapsed`).
- `{"$truncated": true, ...}`: operasi terlalu besar bahkan tanpa membuka skema; hanya ringkasan.
- `truncated: true` dengan `hint`: kedalaman diturunkan otomatis agar muat batas respons; `schema_depth` pada hasil adalah kedalaman efektif.

Anggaran byte dihitung dari `MCP_MAX_RESPONSE_BYTES` (default 102400): teks daftar maksimal `max-512` byte, dan `get_api_endpoint` meminta provider memuat hasil dalam `max - min(2048, max/4)` byte, sehingga pemotongan generik gateway praktis tidak terjadi.

Contoh `tools/call`:

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_api_specs","arguments":{}}}
```

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_api_endpoints","arguments":{"spec":"petstore","tag":"pets","limit":50}}}
```

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_api_endpoint","arguments":{"spec":"petstore","method":"GET","path":"/pets/{petId}","schema_depth":4}}}
```

Kontrak `tools/list` dibekukan di `backend/internal/domain/discovery/internal/apispectools/testdata/tools_list.golden.json` (perbarui dengan `go test ./internal/domain/discovery/internal/apispectools/ -update` lalu periksa diff).

## Tes manual

```
claude mcp add --transport http grimoire https://<host>/mcp --header "Authorization: Bearer <key>"
```

Lalu `/mcp` di Claude Code harus menampilkan server terhubung. Verifikasi penuh daftar tool dan `read_file` dilakukan setelah T08 merge (langkah tertunda).

Smoke dengan curl:

```
curl -s -X POST localhost:8080/mcp -H 'Authorization: Bearer <key>' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}'
```
