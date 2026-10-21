# T09 discovery/api-spec-tools — Frontend Plan

Mengikuti `docs/superpowers/plans/discovery-api-spec-tools/overview.md` (FOUNDATION) dan tiga plan usecase (`usecase-listapispecs.md`, `usecase-listapiendpoints.md`, `usecase-getapiendpoint.md`). Bila bertentangan, overview menang sampai diperbarui. Plan ini satu-satunya plan frontend T09; frontend tidak dipecah per usecase.

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/` hanya `migrations.md` (tidak relevan, T09 tanpa migrasi); `docs/tickets/README.md` bagian "Frontend (Svelte)" dan "Kontrak API"; `docs/superpowers/specs/2026-10-20-grimoire-design.md`; tiket `docs/tickets/T09-discovery-api-spec-tools.md`; `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md` (bagian 2.2, 7, 11, 13); precedent `discovery-mcp-gateway/frontend-plan.md` (T07) dan `discovery-knowledge-tools/frontend-plan.md` (T08). Kode frontend yang ada di `main`: `frontend/e2e/helpers/{auth,apikeys,projects,knowledge,mcp}.ts`, `frontend/e2e/mcp-gateway.spec.ts`, `frontend/playwright.config.ts`, `frontend/src/domain/core/audit-log/listentries/{types.ts,components/components.test.ts}`, `frontend/src/domain/project/api-spec/getspec/components/*` (tab API Spec T06), `api/modules/project/api-spec.yml` (`POST /api/projects/{projectID}/api-specs/upload`, multipart `files`, nama spec = nama file tanpa ekstensi).

**Inti keputusan.** Tiket T09 bagian UI menyatakan "Tidak ada", dan overview bagian 1 dan 9 menyatakan `frontend/**` tidak berubah. Maka T09 **tidak menambah route, view, komponen, store, service aplikasi, kunci i18n, atau folder `frontend/src/domain/discovery/**`**. Katalog aksi `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint` sudah ada di `frontend/src/domain/core/audit-log/listentries/types.ts` (grup `mcp`), sehingga filter Audit Log tidak perlu diubah. Pekerjaan frontend T09 hanya **pengujian black-box Playwright**: (1) memanggil tiga tool lewat `POST /mcp` dengan API key asli dan memeriksa `content[0].text` serta `isError` (T09 tidak memakai `structuredContent`, K2 overview), (2) memastikan endpoint yang dilihat agent sama dengan yang dilihat manusia di tab API Spec, (3) memverifikasi jejak agent di layar `/audit-log` yang sudah ada. Ditambah satu asersi unit penjaga katalog.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T09 (ukuran S) |
| Scope | fullstack menurut workflow; plan ini: bagian frontend saja (e2e + satu tes unit, tanpa kode aplikasi) |
| Domain | discovery |
| Module | api-spec-tools (package backend `apispectools`) |
| Feature frontend baru | tidak ada (tanpa `frontend/src/domain/discovery/api-spec-tools/**`) |
| Kontrak REST | tidak berubah; tidak ada `api/modules/discovery/api-spec-tools.yml` (`/mcp` bukan REST `/api`). endpointsChanged = false |
| Bergantung pada | T06 (merged `0a26586`, `9e1a864`), T07 (merged `35c4fa4`, helper `frontend/e2e/helpers/mcp.ts` sudah ada). Tidak bergantung pada T08 |

Endpoint yang dipakai frontend (hanya oleh e2e, bukan oleh aplikasi Svelte):

| Endpoint | Method | Version | Pemilik | Dipakai untuk |
|---|---|---|---|---|
| `/mcp` (`initialize`, `tools/list`, `tools/call`) | POST | MCP `2025-06-18` (juga `2025-11-25`, `2025-03-26`), JSON-RPC 2.0 | T07; tool dari T09 | panggil `list_api_specs`, `list_api_endpoints`, `get_api_endpoint` |
| `/api/projects/{projectID}/api-specs/upload` | POST | REST, tanpa perubahan (`api/modules/project/api-spec.yml`) | T06 | menyiapkan fixture spec |
| `/api/projects/{projectID}/api-keys` | POST | REST, tanpa perubahan (`api/modules/project/api-key.yml`) | T04 | key agent per project |
| `/api/projects/{projectID}/api-keys/{id}/revoke` (sesuai helper `revokeKeyApi`) | POST | REST, tanpa perubahan | T04 | N10 (key dicabut) |
| `/api/projects` | POST | REST, tanpa perubahan (`api/modules/project/management.yml`) | T03 | project A, B, C |
| `/api/audit-log`, `/api/audit-log/{id}` | GET | REST, tanpa perubahan (`api/modules/core/audit-log.yml`) | T02 | verifikasi jejak + layar `/audit-log` |

Route aplikasi yang tersentuh (tanpa perubahan kode): `/audit-log` (baris `mcp.list_api_specs` / `mcp.list_api_endpoints` / `mcp.get_api_endpoint` kini muncul dari data nyata) dan `/projects/:projectID/api-specs/:item` (tab API Spec T06; dipakai e2e hanya untuk membandingkan endpoint yang dilihat manusia dengan yang dilihat agent).

---

## 2. Summary Plan

1. `frontend/e2e/helpers/mcp.ts` (ubah, aditif): tambah `ToolCallResult` (Zod) dan `callTool(request, key, name, args)` generik. Bila saat implementasi T08 sudah menaruh `callTool` di `mcp.ts` atau `knowledge-tools.ts`, pakai yang ada dan jangan menduplikasi.
2. `frontend/e2e/helpers/apispecs.ts` (baru): `uploadSpecsApi` (multipart `files` ke endpoint upload T06), fixture spec sebagai string YAML (`petstoreYaml`, `billingYaml`, `soloYaml`), generator deterministik `manyEndpointsYaml(n)` dan `hugeYaml()`, serta parser teks murni `parseEndpointLines`, `nextOffset`, `parseDetail`.
3. `frontend/e2e/api-spec-tools.spec.ts` (baru): skenario positif/negatif AC1-AC7 lewat `/mcp`, perbandingan dengan tab API Spec, dan verifikasi jejak di `/audit-log`.
4. `frontend/src/domain/core/audit-log/listentries/components/components.test.ts` (ubah): satu `it` penjaga bahwa grup `mcp` memuat tiga aksi T09.

Tidak ada kunci i18n baru, tidak ada perubahan `app.css`, `DESIGN.md`, route, store, maupun `playwright.config.ts`.

---

## 3. Plan (langkah berurutan)

Prasyarat: binary berisi tiga tool T09 terdaftar (backend F1-F9 overview). Langkah 1, 2, dan 4 boleh dikerjakan paralel dengan backend (FE slicing; helper diuji dengan tes murni tanpa jaringan). Langkah 3 butuh binary lengkap (`make build` lalu `../bin/grimoire` dijalankan `webServer` Playwright).

### Langkah 1. `callTool` di `frontend/e2e/helpers/mcp.ts`

Tambahan aditif; fungsi yang ada tidak diubah.

```ts
export const ToolCallResult = z.object({
  content: z.array(z.object({ type: z.literal('text'), text: z.string() })).min(1),
  isError: z.boolean(),
  structuredContent: z.unknown().optional(), // T09 tidak mengisinya; dicek undefined di P1
});
export type ToolCallResult = z.infer<typeof ToolCallResult>;

/** initialize (sekali per key, di-cache per proses) lalu tools/call. Gagal (expect) bila respons berisi `error` JSON-RPC. */
export async function callTool(request: APIRequestContext, rawKey: string, name: string, args?: unknown): Promise<ToolCallResult>;
```

Implementasi memakai `mcpPost`, `initializeBody`, `toolsCallBody`, `expectRpcResult` yang sudah ada; `id` dari counter modul. Cache `initialize` per key mengurangi permintaan agar tidak mendekati `MCP_RATE_PER_MINUTE` (default 120/menit per key). Skenario yang memang mengharapkan error JSON-RPC atau penolakan gateway (N10) tidak memakai helper ini.

### Langkah 2. Helper `frontend/e2e/helpers/apispecs.ts`

```ts
export interface SpecFile { file: string; content: string } // file: "petstore.yaml" -> spec "petstore"
export interface SpecUploadResult { results: { file: string; name: string; status: string; reason_code: string | null; endpoint_count: number | null }[]; summary: Record<'created'|'updated'|'unchanged'|'skipped'|'rejected', number> }

/** multipart `files` (satu part per file, type application/yaml), header CSRF; expect 200 dan rejected === 0. */
export async function uploadSpecsApi(request: APIRequestContext, projectId: string, files: SpecFile[]): Promise<SpecUploadResult>;

export const petstoreYaml: string;   // lihat tabel fixture
export const billingYaml: string;
export const soloYaml: string;
/** n operasi GET /items/{i} (i = 000..n-1, tiga digit), operationId getItem<i>, tag "items", summary "Get item <i>". */
export function manyEndpointsYaml(n: number): string;
/** GET /report dengan skema bersarang 8 tingkat; tiap tingkat 40 properti string berdeskripsi 60 karakter, sehingga hasil depth 8 > 100352 byte tetapi file < 1 MiB. */
export function hugeYaml(): string;

/** Baris endpoint dari teks list_api_endpoints: [{group, method, path, operationId, tags, deprecated}]. Baris "== x ==" mengganti group. */
export function parseEndpointLines(text: string): EndpointLine[];
/** N dari penanda "[ada K endpoint lagi, panggil list_api_endpoints dengan offset=N ...]"; null bila tidak ada. */
export function nextOffset(text: string): number | null;
/** JSON.parse teks get_api_endpoint lalu Zod EndpointDetail (bagian 9.4). */
export function parseDetail(text: string): EndpointDetail;
```

Zod di helper (hanya untuk e2e, bukan `src/`):

```ts
export const EndpointDetail = z.object({
  spec: z.string(), method: z.enum(['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE']), path: z.string().startsWith('/'),
  schema_depth: z.number().int().min(0).max(8), truncated: z.boolean(), hint: z.string().optional(),
  markers: z.object({ circular: z.array(z.string()), collapsed: z.array(z.string()), legend: z.string() }).optional(),
  operation: z.record(z.string(), z.unknown()),
  servers: z.array(z.record(z.string(), z.unknown())),
  securitySchemes: z.record(z.string(), z.unknown()),
});
```

Fixture (semua OpenAPI 3.0.3, string di helper, tanpa file baru di `e2e/fixtures`):

| Project | File | Isi penting |
|---|---|---|
| A `Payments e2e-<tag>` | `petstore.yaml` | title `Swagger Petstore`, versi `1.0.0`; `GET /pets` (`listPets`, tag `pets`, "List all pets"), `POST /pets` (`createPet`, tag `pets`, deprecated, "Create a pet"), `GET /pets/{petId}` (`showPetById`, tag `pets`, "Info for a specific pet", respons `Pet` -> `owner: Owner` -> `address: Address`, `Pet.children: Pet[]` sirkular, `security: [{apiKey: []}]`); `components.schemas.Unused` tidak dirujuk; `servers: [{url: https://petstore.example.com/v1}]`; `securitySchemes.apiKey` header `X-API-Key` |
| A | `billing.yaml` | title `Billing API`, versi `2.1.0`; `GET /invoices` (`listInvoices`, tag `invoices`, "List invoices"), `POST /invoices/{id}/pay` (`payInvoice`, tag `invoices`, "Pay an invoice") |
| A | `huge.yaml` | `hugeYaml()`, title `Huge Report`, versi `1.0.0`, satu operasi `GET /report` (`getReport`) |
| B `Ledger e2e-<tag>` | `solo.yaml` | title `Ledger Health`, versi `0.9.0`; `GET /health` (`healthCheck`, tag `ops`, "Health check") |
| C `Catalog e2e-<tag>` | `catalog.yaml` | `manyEndpointsYaml(120)`, title `Catalog API` |
| D `Onboarding e2e-<tag>` | (tidak ada) | project tanpa spec |

Jumlah endpoint project A = 3 + 2 + 1 = 6 (satu variabel `A_ENDPOINTS`, dihitung dari fixture, dipakai di asersi teks dan audit).

### Langkah 3. Spec `frontend/e2e/api-spec-tools.spec.ts`

- `test.describe.configure({ mode: 'serial' })` hanya di describe yang berbagi fixture. `beforeAll`: `apiLogin` super admin (`ADMIN_EMAIL`), `createProjectApi` A-D (nama unik bertag), `uploadSpecsApi`, `createKeyApi` per project (nama `claude-laptop-<tag>`, `ledger-bot-<tag>`, dst.); simpan `keyId`, `prefix`, `raw_key`.
- Describe: `tools/list` (P1), `alur penelusuran` (P2-P8, project A), `satu spec` (P9, B), `paginasi` (P10, C), `tanpa spec` (P11, D), `argumen salah` (N1-N9), `key dicabut` (N10, key terpisah agar tidak mengganggu describe lain), `tab API Spec` (P12), `audit` (P13, N11).
- Tiap key melakukan paling banyak sekitar 40 permintaan `/mcp`; describe argumen salah memakai key A kedua (`claude-laptop-neg-<tag>`) agar tetap di bawah batas per key.
- Rincian skenario ada di bagian 8.

Penutup (P13): verifikasi jejak di UI `/audit-log` (super admin, `adminContext`):
- buka `/audit-log?actor_type=api_key&actor_id=<keyIdA>&action=mcp.get_api_endpoint`;
- baris pertama: chip agent (kuning-pucat, label mono `claude-laptop-<tag> (<prefix>)`), aksi `mcp.get_api_endpoint` mono, target `petstore GET /pets/{petId}` mono, pill `Berhasil`;
- klik baris: panel detail kanan memuat `result` dengan `found`, `schema_depth`, `circular_count`, `bytes`, dan TIDAK memuat isi skema (`Owner`, `Address`, `X-API-Key`, `Info for a specific pet`).

### Langkah 4. Penjaga katalog (unit)

`frontend/src/domain/core/audit-log/listentries/components/components.test.ts`: satu `it` baru: `ACTION_GROUPS.find(g => g.key === 'mcp')!.actions` memuat `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint`. Tidak ada perubahan pada `types.ts`.

### Langkah 5. Gerbang

```
cd frontend
npm run check
npm run test -- --run
npx playwright test e2e/api-spec-tools.spec.ts e2e/mcp-gateway.spec.ts
```
Dari root: `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` (tidak ada file `src` berubah selain tes; harus tetap lolos). `git diff main --stat -- frontend/src` hanya menampilkan `components.test.ts`.

---

## 4. Route, komponen, state

| Hal | T09 |
|---|---|
| Route baru | tidak ada |
| Komponen baru | tidak ada |
| Store baru | tidak ada |
| Service + Zod baru di `src/` | tidak ada (Zod hanya di helper e2e) |
| `index.ts` publik | tidak berubah |
| i18n | tidak berubah |

State layar yang menampilkan data T09 (semuanya sudah ditangani T02/T06, diverifikasi e2e, tidak diubah):

| State | `/audit-log` dengan filter `action=mcp.*_api_*` | Tab API Spec (pembanding P12) |
|---|---|---|
| Loading | bar abu rata di posisi nilai, header tabel langsung tampil, tanpa shimmer (T02) | perilaku T06 |
| Empty | panel tabel tetap berbingkai, satu kalimat kosong T02 (sebelum agent memanggil tool) | perilaku T06 (project D: daftar spec kosong dengan aksi unggah) |
| Error | di dalam panel tabel: satu kalimat + "Coba lagi" (T02) | perilaku T06 |
| Sukses | baris `mcp.list_api_specs` (target `–`), `mcp.list_api_endpoints` (target nama spec atau `–`), `mcp.get_api_endpoint` (target `petstore GET /pets/{petId}`), aktor chip agent | daftar endpoint: method pil netral + path mono |
| Gagal tool | pill `Gagal`; panel detail memuat `result.error_code` (`invalid_argument`, `spec_required`, `spec_not_found`, `endpoint_not_found`, `offset_out_of_range`, `no_specs`) | – |

---

## 5. Migration

Tidak ada.

---

## 6. Changes (per file/folder)

| File | Status | Isi |
|---|---|---|
| `frontend/e2e/helpers/mcp.ts` | ubah (aditif) | `ToolCallResult`, `callTool` (lewati bila sudah ada dari T08) |
| `frontend/e2e/helpers/apispecs.ts` | baru | `uploadSpecsApi`, fixture YAML, `manyEndpointsYaml`, `hugeYaml`, `parseEndpointLines`, `nextOffset`, `parseDetail`, `EndpointDetail` |
| `frontend/e2e/api-spec-tools.spec.ts` | baru | e2e positif/negatif + pembanding tab API Spec + verifikasi `/audit-log` |
| `frontend/src/domain/core/audit-log/listentries/components/components.test.ts` | ubah | asersi tiga aksi T09 di grup `mcp` |

TIDAK berubah: `frontend/src/**` selain tes di atas, `frontend/src/app.css`, `DESIGN.md`, file i18n, `frontend/playwright.config.ts`, `frontend/e2e/mcp-gateway.spec.ts`, `api/modules/**`.

---

## 7. Test Scenario

### 7.1 Unit (Vitest)

| Tes | File | Ekspektasi |
|---|---|---|
| katalog aksi T09 | `components.test.ts` | grup `mcp` memuat `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint` |

### 7.2 Tes helper murni (di dalam spec Playwright, `test('helper: ...')`, tanpa jaringan)

| Tes | Ekspektasi |
|---|---|
| `nextOffset` | teks 12.3 overview -> `2`; teks tanpa penanda -> `null`; `offset=` di dalam summary endpoint (bukan baris penanda `[ada ...`) -> `null` |
| `parseEndpointLines` | teks 9.2 menghasilkan 2 baris grup `petstore`; `POST /pets` `deprecated:true`, `operationId:'createPet'`, `tags:['pets']`; baris tanpa summary/operationId/tag ter-parse dengan field kosong; header dan penanda diabaikan |
| `parseDetail` | contoh 9.4 lolos Zod; teks bukan JSON -> lempar dengan pesan jelas |
| `manyEndpointsYaml(120)` | 120 path unik, deterministik (dua panggilan identik) |
| `hugeYaml()` | ukuran < `APISPEC_MAX_FILE_BYTES` default (5242880) dan < 1 MiB; deterministik |

### 7.3 Integration

Tidak ada tes integrasi frontend baru: tidak ada service Svelte. Integrasi Go (`backend/cmd/internal/server/apispectools_integration_test.go`) ada di overview bagian 10.3.

---

## 8. E2E Scenarios (`frontend/e2e/api-spec-tools.spec.ts`)

Setup bersama: lihat langkah 3. `kA` = key project A, `kA2` = key A kedua (negatif), `kB`, `kC`, `kD`.

### Positif

| # | AC | Langkah | Ekspektasi |
|---|---|---|---|
| P1 | 7 | `tools/list` (`ToolsListResult.parse`) | memuat `list_api_specs`, `list_api_endpoints`, `get_api_endpoint`; masing-masing `annotations.readOnlyHint === true`, `inputSchema.type === 'object'`, `additionalProperties === false`; tidak ada properti `project_id`/`projectId`/`project`; `list_api_endpoints.inputSchema.properties.limit` `maximum === 300`, `default === 100`; `get_api_endpoint` `schema_depth` `maximum === 8`, `default === 4` |
| P2 | 1 | `kA`: `list_api_specs {}` | `isError:false`; teks diawali `3 API spec di project ini:`; memuat baris `- billing: Billing API \| versi 2.1.0 \| OpenAPI 3.0.3 \| 2 endpoint`, `- huge: Huge Report`, `- petstore: Swagger Petstore \| versi 1.0.0 \| OpenAPI 3.0.3 \| 3 endpoint` berurutan; baris `Langkah berikut: list_api_endpoints`; `structuredContent` undefined |
| P3 | 1 | `list_api_endpoints {}` | `== billing ==`, `== huge ==`, `== petstore ==` berurutan; `parseEndpointLines` = 6 baris = `A_ENDPOINTS`; header `Endpoint 1-6 dari 6 \| spec: semua`; `POST /pets — Create a pet [createPet] (pets) [deprecated]`; tanpa penanda `[ada` |
| P4 | 1 | `{tag:"pets"}`; `{query:"invoice"}`; `{spec:"petstore", query:"petId"}` | hanya 3 baris `pets` (grup `petstore` saja); hanya baris billing (`listInvoices`, `payInvoice`); tepat `GET /pets/{petId}` dengan header `Endpoint 1-1 dari 1 \| spec: petstore \| query: "petId"` |
| P5 | 1 | `{tag:"pets", limit:2}` lalu ikuti `nextOffset` | halaman 1: 2 baris + `[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2 ...]`; gabungan halaman = 3 endpoint tanpa duplikat |
| P6 | 2 | `get_api_endpoint {spec:"petstore", method:"get", path:"/pets/{petId}"}` | `isError:false`; `parseDetail`: `method === 'GET'`, `path === '/pets/{petId}'`, `truncated === false`, `schema_depth === 4`; JSON `operation` memuat `owner` dan `address` ter-inline, memuat `"$circular":"#/components/schemas/Pet"`, TIDAK memuat `Unused`; `markers.circular` memuat `#/components/schemas/Pet`, `markers.legend` tidak kosong; `servers[0].url === 'https://petstore.example.com/v1'`; `securitySchemes.apiKey.name === 'X-API-Key'` |
| P7 | 2 | `{spec:"petstore", operation_id:"showPetById"}` | `operation` dan `method/path` sama dengan P6 (bandingkan objek hasil `parseDetail`) |
| P8 | 5 | `{spec:"huge", method:"GET", path:"/report", schema_depth:8}` | `isError:false`; teks JSON valid (tidak memuat `[truncated: respons dipotong`); `truncated === true`; `schema_depth < 8`; `hint` memuat `schema_depth`; `Buffer.byteLength(text) <= 102400` |
| P9 | 3 | `kB`: `get_api_endpoint {method:"GET", path:"/health"}` dan `{operation_id:"healthCheck"}` | keduanya `isError:false`, `spec === 'solo'` |
| P10 | 1 | `kC`: `list_api_endpoints {spec:"catalog"}` lalu ikuti `nextOffset` | halaman 1: 100 baris + penanda `offset=100`; halaman 2: 20 baris tanpa penanda, header `Endpoint 101-120 dari 120`; gabungan 120 path unik |
| P11 | – | `kD`: `list_api_specs {}` dan `list_api_endpoints {}` | keduanya `isError:false`, teks `Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.` |
| P12 | – | UI super admin `/projects/<A>/api-specs/petstore` | sidebar endpoint menampilkan tiga endpoint yang sama dengan P4 (method + path mono `/pets/{petId}`); operationId `showPetById` yang tampil di detail UI sama dengan `operation.operationId` di P6. Selector memakai role/teks (`getByRole('navigation', { name: <apispec.detail.endpoints> })`), disesuaikan saat implementasi |
| P13 | K13 | UI `/audit-log?actor_type=api_key&actor_id=<keyIdA>` | baris `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint` tampil dengan chip agent + label mono; filter `action=mcp.get_api_endpoint` menyisakan baris aksi itu; panel detail sesuai langkah 3; `GET /api/audit-log/{id}` untuk `mcp.list_api_specs` P2 memuat `result.count === 3`, `result.endpoint_count === A_ENDPOINTS`, `result.bytes > 0` |

### Negatif

| # | AC | Langkah | Ekspektasi |
|---|---|---|---|
| N1 | 3 | `kA2`: `get_api_endpoint {method:"GET", path:"/pets"}` (tanpa spec, project 3 spec) | `isError:true`, teks `Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs.` |
| N2 | 1 | `list_api_endpoints {spec:"petstor"}` | `isError:true`, memuat `Spec tidak ditemukan: petstor` dan `billing, huge, petstore` |
| N3 | 4 | `get_api_endpoint {spec:"petstore", method:"GET", path:"/pet/{petId}"}` | `isError:true`, memuat `Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}`, `Similar:`, `- GET /pets/{petId}`, `list_api_endpoints` |
| N4 | 4 | `{spec:"petstore", operation_id:"showPetByID"}` | `isError:true`, baris pertama `... : operation_id showPetByID`, `Similar:` memuat `showPetById` |
| N5 | 6 | `kA2`: `list_api_specs {project_id:"<B>"}`, `list_api_endpoints {projectId:"<B>"}`, `get_api_endpoint {project:"<B>", method:"GET", path:"/health"}` | ketiganya `isError:true`, teks `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.`, tidak memuat `solo`/`Ledger`; lalu `get_api_endpoint {spec:"solo", method:"GET", path:"/health"}` dengan `kA2` -> `Spec tidak ditemukan: solo` (project tetap A) |
| N6 | – | `list_api_endpoints {limit:500}`; `{limit:0}`; `{offset:-1}`; `{limit:"10"}` | `isError:true`: `Argumen tidak valid: limit harus 1-300 (diterima: 500)`; `limit harus 1-300 (diterima: 0)`; `offset harus 0-1000000`; `limit harus integer` |
| N7 | – | `list_api_endpoints {spec:"billing", offset:5000}` | `isError:true`, `Argumen tidak valid: offset 5000 melewati total 2 endpoint. Gunakan offset 0 sampai 1.` |
| N8 | – | `get_api_endpoint`: `{spec:"petstore"}`; `{spec:"petstore", method:"GET", path:"/pets", operation_id:"listPets"}`; `{spec:"petstore", method:"FETCH", path:"/pets"}`; `{spec:"petstore", method:"GET", path:"pets"}`; `{spec:"petstore", operation_id:"listPets", schema_depth:9}`; `{spec:"petstore", method:"GET"}` | berturut: `method wajib diisi bersama path, atau isi operation_id`; `pilih salah satu`; `method harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE`; `path harus diawali /`; `schema_depth harus 1-8 (diterima: 9)`; `method wajib diisi bersama path`. Semua `isError:true` dan diawali `Argumen tidak valid:` |
| N9 | – | `list_api_specs {spec:"petstore"}`; `list_api_endpoints {foo:1}`; `tools/call` dengan `arguments: []` | `spec tidak dikenal. Tool ini tidak menerima argumen.`; `foo tidak dikenal. Argumen yang didukung: spec, tag, query, limit, offset.`; `arguments harus object JSON`. Respons berisi `result` (bukan `error` JSON-RPC) |
| N10 | – | key A ketiga dicabut (`revokeKeyApi`), lalu `tools/call list_api_specs` via `mcpPost` | ditolak gateway: `expectRpcFailure(res, 401, -32001)` (format T07, body `UNAUTHORIZED_BODY`); tidak ada entri `mcp.list_api_specs` baru untuk key itu (`GET /api/audit-log?actor_id=<keyId>&action=mcp.list_api_specs` kosong) |
| N11 | K13 | UI `/audit-log?actor_type=api_key&actor_id=<keyIdA2>&action=mcp.get_api_endpoint` setelah N3 | ada baris pill `Gagal` dengan target `petstore GET /pet/{petId}`; panel detail memuat `error_code` `endpoint_not_found` dan `similar_count`; tidak memuat teks summary operasi |

---

## 9. Request/Response body

Semua hasil tool adalah `result` JSON-RPC sukses; kegagalan tool ditandai `isError:true` (bukan `error` JSON-RPC). Teks persis mengikuti overview bagian 12 dan plan usecase; e2e hanya mencocokkan frasa kunci di bagian 8.

### 9.1 `tools/call` list_api_specs

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_api_specs","arguments":{}}}
```
```json
{"jsonrpc":"2.0","id":5,"result":{"content":[{"type":"text","text":"3 API spec di project ini:\n- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-21 09:02 UTC\n- huge: Huge Report | versi 1.0.0 | OpenAPI 3.0.3 | 1 endpoint | diperbarui 2026-10-21 09:02 UTC\n- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint | diperbarui 2026-10-21 09:02 UTC\nLangkah berikut: list_api_endpoints(spec=\"<name>\") untuk daftar endpoint, lalu get_api_endpoint untuk detail."}],"isError":false}}
```

### 9.2 `tools/call` list_api_endpoints (terpotong)

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_api_endpoints","arguments":{"tag":"pets","limit":2}}}
```
```json
{"jsonrpc":"2.0","id":6,"result":{"content":[{"type":"text","text":"Endpoint 1-2 dari 3 | spec: semua | tag: pets\n== petstore ==\nGET /pets — List all pets [listPets] (pets)\nPOST /pets — Create a pet [createPet] (pets) [deprecated]\n\n[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]\nDetail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id)."}],"isError":false}}
```

### 9.3 `tools/call` get_api_endpoint (request)

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_api_endpoint","arguments":{"spec":"petstore","method":"get","path":"/pets/{petId}"}}}
```

### 9.4 `get_api_endpoint` sukses (isi `content[0].text`, diformat agar terbaca; aslinya satu baris JSON kompak)

```json
{
  "spec": "petstore",
  "method": "GET",
  "path": "/pets/{petId}",
  "schema_depth": 4,
  "truncated": false,
  "markers": {
    "circular": ["#/components/schemas/Pet"],
    "collapsed": [],
    "legend": "$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth"
  },
  "operation": {
    "operationId": "showPetById",
    "summary": "Info for a specific pet",
    "tags": ["pets"],
    "parameters": [{"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}],
    "responses": {
      "200": {
        "description": "Expected response to a valid request",
        "content": {"application/json": {"schema": {
          "type": "object",
          "required": ["id", "name"],
          "properties": {
            "id": {"type": "integer", "format": "int64"},
            "name": {"type": "string"},
            "owner": {"type": "object", "properties": {
              "name": {"type": "string"},
              "address": {"type": "object", "properties": {"city": {"type": "string"}, "country": {"type": "string"}}}
            }},
            "children": {"type": "array", "items": {"$circular": "#/components/schemas/Pet"}}
          }
        }}}
      }
    },
    "security": [{"apiKey": []}]
  },
  "servers": [{"url": "https://petstore.example.com/v1"}],
  "securitySchemes": {"apiKey": {"type": "apiKey", "in": "header", "name": "X-API-Key"}}
}
```

### 9.5 `get_api_endpoint` terpotong (P8)

```json
{"spec":"huge","method":"GET","path":"/report","schema_depth":3,"truncated":true,"hint":"Respons melebihi batas 100352 byte; skema hanya dibuka sampai kedalaman 3 (diminta 8). Bagian lebih dalam tampil sebagai {\"$ref\": ...}. Turunkan schema_depth (mis. 3) agar respons ringkas dan konsisten.","markers":{"circular":[],"collapsed":["#/components/schemas/Level4"],"legend":"..."},"operation":{"operationId":"getReport","...":"..."},"servers":[],"securitySchemes":{}}
```
(Kedalaman efektif bergantung ukuran `hugeYaml()`; e2e hanya menuntut `< 8`.)

### 9.6 Tidak ketemu dan spec

```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}\nSimilar:\n- GET /pets/{petId} — Info for a specific pet [showPetById]\n- GET /pets — List all pets [listPets]\nGunakan salah satu di atas, atau list_api_endpoints(spec=\"petstore\", query=\"pet\") untuk melihat endpoint yang ada."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"Spec tidak ditemukan: petstor. Spec yang valid: billing, huge, petstore. Nama spec persis seperti di list_api_specs."}],"isError":true}}
```

### 9.7 Format error

Argumen tidak valid (bus tidak dipanggil):
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya."}],"isError":true}}
```
Argumen project:
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```
Error internal (dipetakan gateway T07):
```json
{"jsonrpc":"2.0","id":14,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool get_api_endpoint. trace_id=01HZX3K9Q7"}],"isError":true}}
```
Key dicabut / tidak valid (gateway T07, HTTP 401):
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}
```

### 9.8 Yang dibaca e2e dari REST (kontrak T02/T06, tidak berubah)

Upload fixture `POST /api/projects/{projectID}/api-specs/upload` (multipart `files`: `petstore.yaml`, `billing.yaml`, `huge.yaml`), respons 200:
```json
{"results":[
  {"file":"petstore.yaml","source_path":"petstore.yaml","name":"petstore","status":"created","reason_code":null,"reason":null,"issues":[],"openapi_version":"3.0.3","title":"Swagger Petstore","api_version":"1.0.0","endpoint_count":3},
  {"file":"billing.yaml","source_path":"billing.yaml","name":"billing","status":"created","reason_code":null,"reason":null,"issues":[],"openapi_version":"3.0.3","title":"Billing API","api_version":"2.1.0","endpoint_count":2},
  {"file":"huge.yaml","source_path":"huge.yaml","name":"huge","status":"created","reason_code":null,"reason":null,"issues":[],"openapi_version":"3.0.3","title":"Huge Report","api_version":"1.0.0","endpoint_count":1}
],"summary":{"created":3,"updated":0,"unchanged":0,"skipped":0,"rejected":0}}
```

`GET /api/audit-log?actor_type=api_key&actor_id=0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c&action=mcp.get_api_endpoint`:
```json
{
  "items": [
    {"id":"0192f0d1-2b00-7d11-8a00-000000000041","occurred_at":"2026-10-21T09:14:25.330Z","actor_type":"api_key","actor_id":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","actor_label":"claude-laptop (nbk_4f9a)","project_id":"0192f0c4-0000-7000-8000-00000000000a","project_label":"Payments API","action":"mcp.get_api_endpoint","target_type":"api_endpoint","target":"petstore GET /pets/{petId}","status":"ok"}
  ],
  "next_cursor": null
}
```
`GET /api/audit-log/{id}` (bagian relevan; isi skema tidak pernah ada):
```json
{"id":"0192f0d1-2b00-7d11-8a00-000000000041","action":"mcp.get_api_endpoint","target_type":"api_endpoint","target":"petstore GET /pets/{petId}","status":"ok","args":{"spec":"petstore","method":"get","path":"/pets/{petId}"},"result":{"found":true,"schema_depth":4,"requested_depth":4,"truncated":false,"circular_count":1,"collapsed_count":0,"bytes":1873}}
```
Error REST (format `foundation.yml`), mis. upload ditolak:
```json
{"error":{"code":"INVALID_ARGUMENT","message":"parameter tidak valid","details":{"field":"files"}},"trace_id":"01HZX3K9Q7"}
```

---

## 10. Visual

T09 tidak menambah layar. Bagian ini mencatat bagaimana hasil kerja T09 tampil di layar yang sudah ada, supaya asersi e2e dan Design check tidak menyimpang dari DESIGN.md.

### 10.1 Pertanyaan per layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| `/audit-log` (T02) | Siapa (manusia atau agent) melakukan apa, kapan? Khusus T09: endpoint API mana yang ditelusuri agent ini? | tabel entri (panel radius 8) dengan baris `mcp.list_api_specs` / `mcp.list_api_endpoints` / `mcp.get_api_endpoint`, kolom target berisi `petstore GET /pets/{petId}` mono |
| Tab API Spec (T06) | Endpoint apa yang tersedia? | daftar endpoint (method pil netral + path mono) dan detail operasi; T09 tidak mengubahnya, hanya menjamin endpoint yang dilihat agent identik (P12) |

Temuan utama yang didukung data T09: "agent `claude-laptop` membuka `petstore GET /pets/{petId}` pukul 09:14" (baris pertama hasil filter `actor_id`). Tidak ada blok baru untuknya.

### 10.2 Pola DESIGN.md dan kerangka

- `/audit-log`: pola **list + filter bar + panel detail kanan** (overlay di layar sempit), di kerangka `AppLayout` framed-sheet (sidebar 228px di `bg-ground`, sheet putih inset 8px, radius 16).
- Tab API Spec: pola **list spec -> detail** (daftar endpoint + detail operasi) di workspace project (title row + tab garis bawah 2px).
- Tidak ada pola list/detail/settings baru.

### 10.3 Elemen khas produk di layar pertama

- **Jejak agent** (Wajah produk #4): setiap baris T09 beraktor `api_key`, tampil dengan chip kotak kuning-pucat + ikon agent + label mono `claude-laptop (nbk_4f9a)`. P13 memastikan chip ini, bukan chip bulat manusia.
- **Mono untuk satu pekerjaan** (#1): aksi `mcp.get_api_endpoint`, target `petstore GET /pets/{petId}`, `operationId` `showPetById`, prefix `nbk_4f9a` mono; label kolom dan judul tidak.
- Satu penanda status per baris: pill `Berhasil` / `Gagal` (pasangan `pill-*`, disertai teks). `endpoint_not_found`, `spec_required`, `invalid_argument` tampil sebagai `Gagal`; alasannya hanya di panel detail (merah sebagai sinyal, bukan tekstur).

### 10.4 Token/utility yang dipakai

Hanya yang sudah ada di `app.css` dan dipakai T02/T06: `bg-ground`, `bg-surface`, `bg-raised`, `bg-active`, `border-border`, `border-rule`, `border-control`, `text-fg`, `text-muted`, `bg-accent/20` + `text-accent-ink` (chip agent), `bg-pill-{positive,negative,neutral}-bg` + `text-pill-…-ink`, `rounded-base`, `rounded-card`, `rounded-sheet`, `shadow-panel`, `font-mono`, `tabular-nums`. Tidak ada hex, radius, font, shadow, atau komponen dekoratif baru. Tidak ada usulan perubahan DESIGN.md.

Catatan (bukan perubahan, sama dengan catatan T07/T08): `AuditTable` memberi chip kuning-pucat ke semua aktor non-`user`. Untuk T09 tidak berdampak karena semua baris T09 beraktor `api_key`. Target `list_api_specs` kosong dirender `–` abu (aturan "nilai hilang").

### 10.5 Data contoh

Kata dan angka produk ini (dilarang Acme/John Doe/lorem):
- project `Payments API` (A, 3 spec), `Ledger` (B, 1 spec `solo`), `Catalog` (C, 120 endpoint), `Onboarding Docs` (D, kosong);
- key `claude-laptop`, prefix `nbk_4f9a`;
- spec `petstore` (`Swagger Petstore` 1.0.0, 3 endpoint), `billing` (`Billing API` 2.1.0, 2 endpoint), `huge` (`Huge Report` 1.0.0, 1 endpoint), total A = 6 = jumlah baris fixture (satu variabel `A_ENDPOINTS`);
- endpoint `GET /pets/{petId}` (`showPetById`), salah ketik `GET /pet/{petId}`, `operation_id` salah huruf `showPetByID`;
- server `https://petstore.example.com/v1`, header keamanan `X-API-Key`.
