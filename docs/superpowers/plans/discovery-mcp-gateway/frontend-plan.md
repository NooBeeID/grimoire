# T07 discovery/mcp-gateway — Frontend Plan

Mengikuti `docs/superpowers/plans/discovery-mcp-gateway/overview.md` (FOUNDATION, bagian 11 dan 13) dan tiga plan usecase (`usecase-authenticate.md`, `usecase-endpoint.md`, `usecase-callaudit.md`). Bila bertentangan, overview menang sampai diperbarui. Plan ini satu-satunya plan frontend T07; frontend tidak dipecah per usecase.

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/` hanya `migrations.md` (tidak relevan, T07 tanpa migrasi); `docs/tickets/README.md` bagian "Frontend (Svelte)" dan "Kontrak API"; `docs/superpowers/specs/2026-10-20-grimoire-design.md`; `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md` (bagian 2, 4.4, 7, 11, 12, 13). Kode frontend yang ada: `frontend/src/domain/core/audit-log/listentries/*` (T02), `frontend/src/domain/project/api-key/*` (T04), `frontend/e2e/{audit-log,project-api-key}.spec.ts`, `frontend/e2e/helpers/{auth,apikeys,projects}.ts`, `frontend/playwright.config.ts`.

**Inti keputusan:** tiket T07 menyatakan "Tidak ada UI sendiri. Potongan konfigurasi ada di T04; telusur aktivitas ada di T02." Maka T07 **tidak menambah route, view, komponen, store, service, atau folder `frontend/src/domain/discovery/**`**. Pekerjaan frontend T07 adalah: (1) katalog aksi audit di filter Audit Log, (2) helper + spec Playwright yang menguji `/mcp` black-box dan memverifikasi jejaknya di layar Audit Log dan tab API Keys yang sudah ada, (3) mengaktifkan skenario `test.fixme` T02 yang menunggu T07, (4) env e2e agar limiter gagal-auth per IP tidak saling mengunci antar tes paralel.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T07 |
| Scope | fullstack (plan ini: bagian frontend saja) |
| Domain | discovery |
| Module | mcp-gateway |
| Feature frontend baru | tidak ada (tanpa `frontend/src/domain/discovery/mcp-gateway/**`) |
| Kontrak REST | tidak berubah; tidak ada `api/modules/discovery/mcp-gateway.yml` (`/mcp` bukan REST `/api`, overview bagian 1). endpointsChanged = false |

Endpoint yang diuji/dipakai oleh frontend (e2e):

| Endpoint | Method | Version | Pemilik | Dipakai frontend untuk |
|---|---|---|---|---|
| `/mcp` | POST | MCP `2025-11-25` / `2025-06-18` / `2025-03-26`, JSON-RPC 2.0 | T07 (baru) | e2e black-box lewat Playwright `request` (bukan dari aplikasi Svelte) |
| `/mcp` | GET | - (405) | T07 (baru) | e2e negatif |
| `/mcp` | DELETE | - (405) | T07 (baru) | e2e negatif |
| `/api/audit-log` | GET | REST, tanpa perubahan (`api/modules/core/audit-log.yml`) | T02 | layar `/audit-log` + hitung baris di e2e |
| `/api/projects/{id}/api-keys` | GET/POST | REST, tanpa perubahan (`api/modules/project/api-key.yml`) | T04 | buat key, baca `last_used_at` |
| `/api/projects/{id}/api-keys/{keyId}/revoke` | POST | REST, tanpa perubahan | T04 | cabut key (AC5) |

Route aplikasi yang tersentuh (tanpa perubahan kode route): `/audit-log` (filter aksi grup MCP bertambah 3 opsi) dan `/projects/:id/api-keys` (kolom "Terakhir dipakai" kini terisi karena `touch` T07).

---

## 2. Summary Plan

1. `frontend/src/domain/core/audit-log/listentries/types.ts`: tambah `'mcp.initialize'`, `'mcp.tools_list'`, `'mcp.unknown_tool'` di grup `mcp` `ACTION_GROUPS` (sinkron dengan `record.Actions()` langkah F2 backend). Tes katalog 33 -> 36 aksi.
2. `frontend/e2e/helpers/mcp.ts` (baru): helper JSON-RPC untuk `POST /mcp` dan parser Zod untuk respons/penolakan.
3. `frontend/e2e/helpers/apikeys.ts`: tambah `last_used_at` pada tipe `ApiKeyApi` (dipakai untuk memverifikasi `touch`).
4. `frontend/e2e/mcp-gateway.spec.ts` (baru): alur positif/negatif endpoint + authenticate + callaudit, verifikasi jejak di `/audit-log` dan tab API Keys.
5. `frontend/e2e/audit-log.spec.ts`: skenario 5 `test.fixme` -> `test` dengan data nyata dari key + panggilan MCP.
6. `frontend/playwright.config.ts`: env `MCP_AUTH_FAIL_PER_MINUTE: '1000'` untuk `webServer` (setara pola `LOGIN_RATE_PER_MINUTE: '1000'` yang sudah ada). Brute force/429 IP diuji di Go (overview 12.5).

Tidak ada kunci i18n baru: aksi ditampilkan sebagai kode mentah mono, label grup `auditLog.actionGroup.mcp` sudah ada di `id.json`/`en.json`.

---

## 3. Plan (langkah berurutan)

Prasyarat: backend T07 fase F (F2 aksi audit) sudah ada agar katalog frontend dan backend sama. Langkah 1-2 bisa dikerjakan paralel dengan backend (FE slicing), langkah 3-6 butuh binary dengan `/mcp` aktif.

### Langkah 1. Katalog aksi audit

File: `frontend/src/domain/core/audit-log/listentries/types.ts`

- Grup `mcp` menjadi (urutan: tool, lalu kejadian gateway; tiga aksi baru setelah `mcp.rate_limited`):
  ```ts
  {
    key: 'mcp',
    actions: [
      'mcp.list_files',
      'mcp.read_file',
      'mcp.search_knowledge',
      'mcp.list_api_specs',
      'mcp.list_api_endpoints',
      'mcp.get_api_endpoint',
      'mcp.auth.rejected',
      'mcp.rate_limited',
      'mcp.initialize',
      'mcp.tools_list',
      'mcp.unknown_tool',
    ],
  },
  ```
- Komentar "Sinkron dengan lib/contracts/core/audit/record/actions.go" tetap. Tidak ada perubahan `ACTION_RE` (ketiganya cocok), `MAX_ACTIONS` (20, grup MCP 11 aksi tetap di bawah batas), `ActionPicker.svelte`, `FilterBar.svelte`, `AuditTable.svelte`.
- Kontrak publik `index.ts` feature `listentries` tidak berubah (konstanta sudah diekspos lewat `types.ts` yang dipakai internal; tidak ada import silang baru).

File: `frontend/src/domain/core/audit-log/listentries/types.test.ts`

- `toHaveLength(33)` dan `size).toBe(33)` -> `36`; nama tes "has 36 unique valid actions, each in exactly one group".
- Tambah satu kasus: tiga aksi baru ada di grup `mcp` dan `AUDIT_ACTIONS` tidak memuat nama tool terlarang dalam bentuk lain (`mcp.auth`, `mcp.tools/list`).

### Langkah 2. Helper e2e MCP

File baru: `frontend/e2e/helpers/mcp.ts`

```ts
import { expect, type APIRequestContext, type APIResponse } from '@playwright/test';
import { z } from 'zod';

export const MCP_VERSIONS = ['2025-11-25', '2025-06-18', '2025-03-26'] as const;
export const UNAUTHORIZED_BODY = '{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}';

export type McpId = string | number;

/** POST /mcp. rawKey null -> tanpa header auth. headers menimpa default (mis. X-API-Key, Origin, Content-Type). */
export async function mcpPost(
  request: APIRequestContext,
  rawKey: string | null,
  body: unknown,
  headers: Record<string, string> = {},
): Promise<APIResponse>;

/** Kirim body mentah (string) untuk kasus parse error, batch, body besar. */
export async function mcpPostRaw(request: APIRequestContext, rawKey: string | null, raw: string, headers?: Record<string, string>): Promise<APIResponse>;

export const initializeBody = (id: McpId, version = '2025-06-18') => ({ jsonrpc: '2.0', id, method: 'initialize',
  params: { protocolVersion: version, capabilities: {}, clientInfo: { name: 'grimoire-e2e', version: '1.0.0' } } });
export const initializedNotification = () => ({ jsonrpc: '2.0', method: 'notifications/initialized' });
export const toolsListBody = (id: McpId) => ({ jsonrpc: '2.0', id, method: 'tools/list' });
export const pingBody = (id: McpId) => ({ jsonrpc: '2.0', id, method: 'ping' });
export const toolsCallBody = (id: McpId, name: unknown, args?: unknown) =>
  ({ jsonrpc: '2.0', id, method: 'tools/call', params: args === undefined ? { name } : { name, arguments: args } });

/** Key berformat benar tapi tidak terdaftar; prefix unik agar baris audit bisa difilter lewat target. */
export function randomWellFormedKey(): { rawKey: string; prefix: string };

export const RpcError = z.object({ code: z.number().int(), message: z.string(), data: z.unknown().optional() });
export const RpcSuccess = z.object({ jsonrpc: z.literal('2.0'), id: z.union([z.string(), z.number()]), result: z.record(z.unknown()) });
export const RpcFailure = z.object({ jsonrpc: z.literal('2.0'), id: z.union([z.string(), z.number(), z.null()]), error: RpcError });
export const InitializeResult = z.object({
  protocolVersion: z.enum(MCP_VERSIONS),
  capabilities: z.object({ tools: z.object({ listChanged: z.literal(false) }) }),
  serverInfo: z.object({ name: z.literal('grimoire'), version: z.string().min(1) }),
  instructions: z.string().min(1),
});
export const ToolsListResult = z.object({ tools: z.array(z.object({
  name: z.string(), description: z.string(), inputSchema: z.record(z.unknown()),
  annotations: z.object({ readOnlyHint: z.literal(true) }),
})) });

/** Assert status + parse body sebagai RpcFailure; mengembalikan error. */
export async function expectRpcFailure(res: APIResponse, status: number, code: number): Promise<z.infer<typeof RpcError>>;
/** Assert 200 + RpcSuccess; mengembalikan result. */
export async function expectRpcResult(res: APIResponse): Promise<Record<string, unknown>>;
```

Aturan helper:
- Default header: `Content-Type: application/json`; `Authorization: Bearer <rawKey>` hanya bila `rawKey` tidak null. Tidak mengirim `Origin` dan tidak mengirim cookie (pemanggil memakai fixture `request` Playwright yang bersih, bukan `admin.request`), sehingga tes membuktikan `/mcp` tidak butuh sesi/CSRF.
- `randomWellFormedKey()`: prefix 8 karakter `[a-z0-9]`, secret 43 karakter `[A-Za-z0-9_-]` (cocok `RAW_KEY_RE` T04 `^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$`).
- Zod dipakai untuk memvalidasi respons (sama seperti `services/` di aplikasi), sehingga perubahan bentuk respons gagal dengan pesan jelas.

File: `frontend/e2e/helpers/apikeys.ts`
- `ApiKeyApi` tambah `last_used_at: string | null`. Tanpa perubahan fungsi.

### Langkah 3. Env e2e

File: `frontend/playwright.config.ts`
- `webServer.env` tambah `MCP_AUTH_FAIL_PER_MINUTE: '1000'`. Alasan: `fullyParallel: true` dan semua tes memakai IP `127.0.0.1`; dengan default 20/menit, beberapa tes 401 (tanpa key, key acak, key dicabut, Basic) yang berjalan bersama dapat memicu pre-check `Peek` IP (K19) sehingga tes lain yang memakai key sah ikut 429. Tidak ada env lain yang diubah: `MCP_RATE_PER_MINUTE` default 120 per key aman karena tiap tes membuat key sendiri; `MCP_ALLOWED_ORIGINS` kosong (Origin asing tetap 403, permintaan tanpa Origin lolos).
- CI job `e2e` memakai `webServer` (tanpa `E2E_BASE_URL`), jadi env ini berlaku di CI tanpa mengubah `.github/workflows/ci.yml`. Bila `E2E_BASE_URL` dipakai (server eksternal), operator wajib menyetel env yang sama; dicatat di komentar config.

### Langkah 4. Spec `mcp-gateway.spec.ts`

File baru: `frontend/e2e/mcp-gateway.spec.ts`. Struktur:

```ts
// E2E T07 discovery/mcp-gateway terhadap binary sungguhan (butuh T01-T04 + T07).
// Panggilan MCP memakai fixture `request` tanpa cookie; verifikasi jejak lewat UI audit log (admin) dan tab API Keys.
// Tool nyata (read_file, list_files) ditambah T08/T09; skenarionya test.fixme sampai merge.

async function setupKey(browser: Browser) {
  const admin = await adminContext(browser, 'id-ID');       // helpers/auth
  const proj = await createProjectApi(admin.request);       // helpers/projects
  const created = await createKeyApi(admin.request, proj.id, uniqueKeyName('mcp'));
  return { admin, proj, key: created.key, rawKey: created.raw_key };
}

test.describe('mcp-gateway: protokol', () => { ... });        // endpoint
test.describe('mcp-gateway: autentikasi', () => { ... });     // authenticate
test.describe('mcp-gateway: panggilan tool', () => { ... });  // callaudit
```

Isi skenario: bagian 7. Verifikasi UI memakai locator berbasis peran dan teks (`getByRole('table')`, `getByRole('row').filter({ hasText: 'mcp.initialize' })`, `getByRole('img', { name: 'API key (agent)' })`), bukan kelas CSS.

Menghitung baris audit (skenario "tanpa audit"): `admin.request.get('/api/audit-log?actor_type=api_key&actor_id=<keyId>&limit=100')` sebelum dan sesudah; key unik per tes sehingga aman paralel.

### Langkah 5. Aktifkan skenario T02

File: `frontend/e2e/audit-log.spec.ts`
- Hapus komentar "Satu skenario masih fixme..." di header dan komentar di atas skenario 5.
- Skenario 5 menjadi `test('5. deep link actor_type+actor_id terfilter ke key itu', ...)`: buat project + key (`createProjectApi`, `createKeyApi`), panggil `initialize` lalu `tools/list` lewat `mcpPost` dengan fixture `request`, `page.goto('/audit-log?actor_type=api_key&actor_id=<keyId>')`, harapkan dua baris (`mcp.initialize`, `mcp.tools_list`), setiap baris memuat label `<nama key> (<prefix>)` dan project label = nama project, tidak ada baris dengan aktor lain (semua baris punya ikon `API key (agent)`).

### Langkah 6. Verifikasi

- `npm run test` (vitest; `types.test.ts`, `filterQuery.test.ts`, `components.test.ts` hijau).
- `npm run check`, `npm run lint`, `npm run depcruise` (tidak ada import baru di `src/`).
- `npx playwright test e2e/mcp-gateway.spec.ts e2e/audit-log.spec.ts` terhadap binary hasil `make build`.
- `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` tetap tanpa FAIL baru (tidak ada file `.svelte` yang diubah).

Kontrak bus: frontend tidak menyentuh bus. Untuk kelengkapan, rantai yang dibuktikan e2e: `POST /mcp` -> `project.apikey.verify` (Call) -> `project.management.get-project` (Call) -> `project.apikey.touch` (Publish, dibuktikan lewat `last_used_at`) -> `core.audit.record` (Call, dibuktikan lewat baris `/audit-log`).

---

## 4. Migration

Tidak ada. T07 tidak punya tabel (overview bagian 0); frontend tidak pernah membuat migrasi.

---

## 5. Changes

| File/folder | Aksi | Isi |
|---|---|---|
| `frontend/src/domain/core/audit-log/listentries/types.ts` | ubah | 3 aksi baru di grup `mcp` |
| `frontend/src/domain/core/audit-log/listentries/types.test.ts` | ubah | 33 -> 36, kasus keanggotaan grup `mcp` |
| `frontend/e2e/helpers/mcp.ts` | baru | `mcpPost`, `mcpPostRaw`, body builder, `randomWellFormedKey`, skema Zod, `expectRpcFailure`/`expectRpcResult` |
| `frontend/e2e/helpers/apikeys.ts` | ubah | `ApiKeyApi.last_used_at` |
| `frontend/e2e/mcp-gateway.spec.ts` | baru | skenario bagian 7 |
| `frontend/e2e/audit-log.spec.ts` | ubah | skenario 5 aktif, komentar header diperbarui |
| `frontend/playwright.config.ts` | ubah | `MCP_AUTH_FAIL_PER_MINUTE: '1000'` + komentar |
| `frontend/src/domain/discovery/**` | tidak dibuat | T07 tanpa UI (tiket bagian UI) |
| `frontend/src/shared/i18n/{id,en}.json` | tidak berubah | aksi tampil sebagai kode mentah; grup MCP sudah ada |
| `frontend/src/app/routes/**`, komponen/store/service | tidak berubah | - |
| `DESIGN.md`, `frontend/src/app.css` | tidak berubah | tanpa token/komponen baru |
| `api/**` | tidak berubah | `/mcp` bukan REST |

---

## 6. Komponen, state, dan status layar (yang sudah ada, ditinjau untuk T07)

T07 tidak menambah komponen atau store. Perilaku yang wajib tetap benar dengan data dari gateway:

| Layar / komponen | State | Perilaku dengan data T07 |
|---|---|---|
| `/audit-log` `ListEntriesView` + `listentries.store` | loading | kerangka tabel dengan bar abu per sel (perilaku T02), tidak berubah |
| | empty | filter `actor_id` key baru yang belum pernah memanggil MCP: bingkai tabel tetap, kalimat `auditLog.emptyHint` ("Entri muncul saat agent memanggil MCP...") |
| | error | error di dalam blok tabel + tombol Coba lagi (T02), tidak berubah |
| | data | baris `mcp.initialize`/`mcp.tools_list`/`mcp.unknown_tool`: chip agent kotak kuning-pucat + label mono `claude-laptop (a1b2c3d4)`, aksi mono, status pill (`ok` -> Berhasil, `error` -> Gagal, `denied` -> Ditolak) |
| `ActionPicker` | opsi | grup "MCP" menampilkan 11 checkbox; cari "tools" menemukan `mcp.tools_list`; batas 20 tidak tercapai oleh satu grup |
| `filterQuery` | URL | `?action=mcp.unknown_tool` dan `?action=mcp.tools_list` lolos `ACTION_RE` dan dipertahankan saat round-trip |
| `AuditTable` baris `mcp.auth.rejected` | data | aktor `anonymous` (chip non-manusia yang sudah ada), label = prefix key mono bila terbaca, target `a1b2c3d4`, status Ditolak |
| Tab API Keys `listkeys` | data | kolom "Terakhir dipakai" berisi waktu relatif setelah panggilan MCP sukses; key yang belum pernah dipakai tetap `–` abu |

---

## 7. Test Scenario (unit/integration frontend)

Vitest:
1. `types.test.ts`: 36 aksi unik, masing-masing cocok `ACTION_RE`, tepat di satu grup; `mcp.initialize`, `mcp.tools_list`, `mcp.unknown_tool` di grup `mcp`.
2. `filterQuery.test.ts` (tambah satu kasus): `filterFromQuery(new URLSearchParams('action=mcp.unknown_tool&action=mcp.tools_list'))` -> `actions` berisi keduanya; `filterToQuery` mengembalikan query yang sama.
3. `components.test.ts` (tambah satu kasus `ActionPicker`): render dengan `value=[]`, ketik `unknown` di pencarian -> hanya checkbox `mcp.unknown_tool` di grup MCP yang terlihat; centang -> `onchange(['mcp.unknown_tool'])`.
4. `AuditTable` dengan `mockSummary(1, { action: 'mcp.tools_list', target_type: null, target: null })`: kolom target menampilkan `–` abu, chip agent punya nama aksesibel `API key (agent)`.

Tidak ada tes service/store baru (tidak ada service/store baru). Tes integrasi backend (Postgres) ada di plan backend (overview 12.4).

---

## 8. E2E Scenarios (Playwright)

File `frontend/e2e/mcp-gateway.spec.ts` kecuali disebut lain. Tiap tes membuat project + key sendiri (`setupKey`). "Audit UI" = admin membuka URL `/audit-log?...` lalu memeriksa baris tabel.

### Positif

| # | Usecase | Langkah | Harapan |
|---|---|---|---|
| P1 | endpoint, authenticate (AC1) | `initialize` (Bearer, `2025-06-18`) -> `notifications/initialized` -> `tools/list` | 200 `InitializeResult` (`serverInfo.name=grimoire`, `protocolVersion=2025-06-18`, `instructions` memuat nama project); 202 body kosong; 200 `ToolsListResult` (`tools` array, setiap tool `readOnlyHint:true`); Audit UI `actor_type=api_key&actor_id=<keyId>` menampilkan `mcp.initialize` dan `mcp.tools_list` status Berhasil, label `<nama key> (<prefix>)`, project = nama project |
| P2 | authenticate | sama dengan P1 memakai `X-API-Key` | hasil identik P1 (kecuali `id`) |
| P3 | authenticate | Bearer valid + `X-API-Key` acak | 200 (Bearer menang) |
| P4 | authenticate (touch) | setelah P1, `expect.poll(() => listKeysApi(...))` `last_used_at` tidak null (timeout 5 s); buka `/projects/<id>/api-keys` | baris key menampilkan waktu "Terakhir dipakai", bukan `–` |
| P5 | endpoint | `ping` id `"p-1"` | `{"jsonrpc":"2.0","id":"p-1","result":{}}`, id tetap string |
| P6 | endpoint | `initialize` untuk tiap `MCP_VERSIONS` | `protocolVersion` = yang diminta |
| P7 | endpoint | `Content-Type: application/json; charset=utf-8`; `tools/list` dengan header `MCP-Protocol-Version: 2025-06-18` | 200 |
| P8 | audit-log.spec skenario 5 | lihat langkah 5 | dua baris key itu saja |
| P9 | callaudit (`test.fixme` sampai T08) | `tools/call read_file {"path":"docs/auth/login-flow.md"}` | 200 `isError:false`; Audit UI tepat satu baris `mcp.read_file` Berhasil, target `docs/auth/login-flow.md`; panel detail tidak memuat isi file |
| P10 | callaudit AC3 (`test.fixme` sampai T08) | arguments ditambah `project_id` project lain | hasil dari project key; baris audit berproject milik key |

### Negatif

| # | Usecase | Langkah | Harapan |
|---|---|---|---|
| N1 | authenticate (AC2) | tanpa header auth, `tools/list` | 401, `www-authenticate` diawali `Bearer`, body teks persis `UNAUTHORIZED_BODY` |
| N2 | authenticate (AC2) | `randomWellFormedKey()` | 401, body identik N1; Audit UI `action=mcp.auth.rejected&target=<prefix>` menampilkan 1 baris Ditolak, aktor Anonim, label prefix mono |
| N3 | authenticate (AC5) | P1 sukses -> `revokeKeyApi` -> `tools/list` | 401 body identik N1; Audit UI `action=mcp.auth.rejected&target=<prefix key>` 1 baris |
| N4 | authenticate | `Authorization: Basic Zm9vOmJhcg==` saja | 401 body identik N1 |
| N5 | authenticate | Bearer acak + `X-API-Key` valid | 401 |
| N6 | endpoint (AC9) | key valid + `Origin: https://evil.example` | 403, body REST `error.code == "FORBIDDEN"`, `trace_id` ada; hitung baris audit key sebelum/sesudah sama (tidak ada `mcp.*`) |
| N7 | endpoint | `GET /mcp` dan `DELETE /mcp` tanpa key | 405, header `allow: POST`, `error.code == -32600` |
| N8 | endpoint | `resources/list` | 200, `error.code == -32601`, `error.data.method == "resources/list"` |
| N9 | endpoint (K7) | `initialize` `2024-11-05` | 200, `error.code == -32602`, `data.requested == "2024-11-05"`, `data.supported` = 3 versi; Audit UI baris `mcp.initialize` status Gagal |
| N10 | endpoint | `Content-Type: text/plain` | 415, `-32600` |
| N11 | endpoint | body `{bukan json` | 400, `-32700`, `id: null` |
| N12 | endpoint | batch `[initializeBody(1)]` | 400, `-32600`, `data.reason == "batch not supported"` |
| N13 | endpoint | body 70 KB (padding di `params.clientInfo.name`) | 413, `-32600`, `data.limit_bytes == 65536` |
| N14 | endpoint | header `MCP-Protocol-Version: 1999-01-01` pada `tools/list` | 400, `-32600` |
| N15 | callaudit (K16) | `tools/call` `name:"write_file"` | 200, `error.code == -32602`, `message == "Unknown tool: write_file"`; Audit UI `actor_id=<keyId>&action=mcp.unknown_tool` tepat 1 baris Gagal, target `write_file` |
| N16 | endpoint (K15) | `tools/call` `{"name":""}` dan `{"name":"x","arguments":"str"}` | `-32602` `Invalid params`; jumlah baris audit key sebelum/sesudah sama |

Di luar e2e (tidak dapat diprovokasi dengan aman lewat Playwright, dicakup tes Go overview 12.5): 429 per key (AC6), brute force IP 429 + `mcp.rate_limited`, verify tidak tersedia 503, timeout/panic tool, truncation 500 KB (AC7, butuh tool T08), duplikasi nama tool (AC8, gagal start).

---

## 9. Request/Response body

Contoh key: `gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA`. Semua `POST /mcp` dengan `Content-Type: application/json`.

### 9.1 initialize

```http
POST /mcp
Authorization: Bearer gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA
Content-Type: application/json
```
```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"grimoire-e2e","version":"1.0.0"}}}
```
200:
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
Versi tidak didukung (200):
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Unsupported protocol version","data":{"requested":"2024-11-05","supported":["2025-11-25","2025-06-18","2025-03-26"]}}}
```

### 9.2 notifications/initialized, ping, tools/list

```json
{"jsonrpc":"2.0","method":"notifications/initialized"}
```
-> `202 Accepted`, body kosong.

```json
{"jsonrpc":"2.0","id":"p-1","method":"ping"}
```
```json
{"jsonrpc":"2.0","id":"p-1","result":{}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
```
T07 (katalog kosong):
```json
{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}
```
Setelah T08:
```json
{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"read_file","title":"Baca file knowledge","description":"Membaca isi file Markdown berdasarkan path. Gunakan list_files dulu bila path belum diketahui.","inputSchema":{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"],"additionalProperties":false},"annotations":{"readOnlyHint":true}}]}}
```

### 9.3 tools/call

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"docs/auth/login-flow.md"}}}
```
Tool tidak dikenal (200):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Unknown tool: write_file"}}
```
Params tidak valid (200, tanpa audit):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Invalid params","data":{"reason":"arguments must be an object"}}}
```
Sukses (setelah T08):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"# Login flow\n\nAlur login memakai cookie session..."}],"isError":false}}
```
Kegagalan tool:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"File \"docs/auth/logn-flow.md\" tidak ditemukan. Mungkin maksudnya: docs/auth/login-flow.md"}],"isError":true}}
```
Metode tidak dikenal (200):
```json
{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"Method not found","data":{"method":"resources/list"}}}
```

### 9.4 Format error level HTTP

401 (identik untuk tanpa key/acak/dicabut/Basic), header `WWW-Authenticate: Bearer realm="grimoire-mcp"`:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}
```
429, header `Retry-After: 1` (hanya tes Go):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32003,"message":"Rate limit exceeded","data":{"retry_after_seconds":1}}}
```
503 (hanya tes Go):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32004,"message":"Service unavailable"}}
```
400 parse / batch / header versi:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}
```
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"batch not supported"}}}
```
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"unsupported MCP-Protocol-Version","supported":["2025-11-25","2025-06-18","2025-03-26"]}}}
```
413:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"request body too large","limit_bytes":65536}}}
```
415:
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"Content-Type must be application/json"}}}
```
405 (header `Allow: POST`):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request","data":{"reason":"method not allowed; use POST"}}}
```
403 Origin asing (format REST `OriginGuard`):
```json
{"error":{"code":"FORBIDDEN","message":"origin tidak diizinkan"},"trace_id":"01HZX3K9Q7"}
```

### 9.5 Yang dibaca frontend dari `GET /api/audit-log` (kontrak T02, tidak berubah)

`GET /api/audit-log?actor_type=api_key&actor_id=0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c`:
```json
{
  "items": [
    {"id":"0192f0c5-1a00-7d11-8a00-000000000002","occurred_at":"2026-10-21T09:14:22.481Z","actor_type":"api_key","actor_id":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","actor_label":"claude-laptop (a1b2c3d4)","project_id":"0192f0c4-0000-7000-8000-00000000000a","project_label":"Payments","action":"mcp.tools_list","target_type":null,"target":null,"status":"ok"},
    {"id":"0192f0c5-1a00-7d11-8a00-000000000001","occurred_at":"2026-10-21T09:14:22.102Z","actor_type":"api_key","actor_id":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","actor_label":"claude-laptop (a1b2c3d4)","project_id":"0192f0c4-0000-7000-8000-00000000000a","project_label":"Payments","action":"mcp.initialize","target_type":null,"target":null,"status":"ok"}
  ],
  "next_cursor": null
}
```
Error REST (format `foundation.yml`), mis. filter tidak valid:
```json
{"error":{"code":"INVALID_ARGUMENT","message":"parameter tidak valid","details":{"field":"action"}},"trace_id":"01HZX3K9Q7"}
```

---

## 10. Visual

T07 tidak menambah layar. Bagian ini mencatat bagaimana jejak gateway tampil di dua layar yang sudah ada, agar implementasi dan Design check tidak menyimpang.

### 10.1 Pertanyaan per layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| `/audit-log` (T02) | Siapa (manusia atau agent) melakukan apa, kapan? Untuk T07 secara khusus: kapan agent ini terhubung dan apa yang ia panggil atau coba? | tabel entri (panel radius 8), dengan baris `mcp.initialize` / `mcp.tools_list` / `mcp.unknown_tool` / `mcp.auth.rejected` |
| `/projects/:id/api-keys` (T04) | Key mana yang aktif dan kapan terakhir dipakai? | tabel key; kolom "Terakhir dipakai" kini terisi oleh `touch` T07 |

Temuan utama (one finding) yang didukung data T07: baris `mcp.auth.rejected` dengan prefix key yang sudah dicabut, yaitu "ada yang mencoba memakai key yang sudah dicabut" (user story tiket). Tidak ada blok baru untuknya; filter `action=mcp.auth.rejected` + target prefix adalah jalannya, dipakai dari deep link.

### 10.2 Pola DESIGN.md dan kerangka

- `/audit-log`: pola **list + filter bar + panel detail kanan** (tabel DESIGN.md "Pola per layar"), di kerangka `AppLayout` framed-sheet (sidebar di `bg-ground`, sheet putih inset 8px). Perubahan hanya di isi `ActionPicker` (dropdown filter yang ada; app-screens 4.4: scope/filter di kiri).
- Tab API Keys: pola **list** di workspace project (title row + tab garis bawah 2px).
- Tidak ada pola detail/settings baru.

### 10.3 Elemen khas produk di layar pertama

- **Jejak agent** (DESIGN.md "Wajah produk" #4): setiap baris `mcp.*` beraktor `api_key` memakai chip kotak kuning-pucat dengan ikon agent dan label mono `claude-laptop (a1b2c3d4)`; manusia tetap chip bulat netral. Baris gateway T07 harus terbaca sekilas sebagai agent.
- **Mono untuk satu pekerjaan** (#1): nama aksi `mcp.tools_list`, prefix key `a1b2c3d4`, target `write_file` dan path `docs/auth/login-flow.md` tampil mono; label kolom dan judul tidak.
- Status satu penanda per baris: pill `Berhasil` / `Gagal` / `Ditolak` (pasangan pucat `pill-*`), tidak warna saja.

### 10.4 Token/utility yang dipakai

Hanya yang sudah ada di `app.css`: `bg-ground`, `bg-surface`, `bg-raised`, `bg-active`, `border-border`, `border-rule`, `border-control`, `text-fg`, `text-muted`, `bg-accent/20` + `text-accent-ink` (chip agent), `bg-pill-{positive,negative,warning,neutral}-bg` + `text-pill-…-ink`, `rounded-base`, `rounded-card`, `shadow-panel`, `font-mono`, `tabular-nums`. Tidak ada hex, radius, font, shadow, atau komponen dekoratif baru. Tidak ada usulan perubahan DESIGN.md.

Catatan (bukan perubahan): `AuditTable` saat ini memberi chip kuning-pucat ke semua aktor non-`user`, termasuk `anonymous` pada `mcp.auth.rejected`. DESIGN.md #4 menyebut chip kuning untuk agent (`actor_type=api_key`). Plan ini tidak mengubahnya (di luar cakupan T07); bila ingin `anonymous` memakai chip netral, itu perubahan T02 yang perlu diputuskan manusia lewat DESIGN.md.

### 10.5 Data contoh

Untuk mock/fixture vitest dan teks contoh (bukan Acme/John Doe/lorem):

```ts
// Satu agent, satu project; urutan waktu naik; angka kecil sesuai akun baru.
const key = { id: '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c', name: 'claude-laptop', prefix: 'a1b2c3d4' };
const project = { id: '0192f0c4-0000-7000-8000-00000000000a', name: 'Payments' };
const label = `${key.name} (${key.prefix})`; // satu variabel, dipakai di semua baris
export const t07Rows = [
  { occurred_at: '2026-10-21T09:14:22.102Z', action: 'mcp.initialize',   status: 'ok',     target: null },
  { occurred_at: '2026-10-21T09:14:22.481Z', action: 'mcp.tools_list',   status: 'ok',     target: null },
  { occurred_at: '2026-10-21T09:15:03.990Z', action: 'mcp.unknown_tool', status: 'error',  target: 'write_file' },
  { occurred_at: '2026-10-21T11:02:47.310Z', action: 'mcp.auth.rejected', status: 'denied', target: 'a1b2c3d4', actor_type: 'anonymous', actor_label: 'a1b2c3d4' },
];
// Terakhir dipakai di tab API Keys = occurred_at baris sukses terakhir (09:14:22.481Z), bukan nilai terpisah.
```

State layar (app-screens 11): empty = bingkai tabel + `auditLog.emptyHint`; loading = bar abu seukuran sel tanpa shimmer; error = kalimat di dalam blok + "Coba lagi"; nilai hilang (target `mcp.tools_list`, "Terakhir dipakai" key baru) = `–` abu.
