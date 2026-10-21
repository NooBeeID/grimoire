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
