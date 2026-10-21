# T08 discovery/knowledge-tools — Frontend Plan

Mengikuti `docs/superpowers/plans/discovery-knowledge-tools/overview.md` (FOUNDATION) dan tiga plan usecase (`usecase-listfiles.md`, `usecase-readfile.md`, `usecase-searchknowledge.md`). Bila bertentangan, overview menang sampai diperbarui. Plan ini satu-satunya plan frontend T08; frontend tidak dipecah per usecase.

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/` hanya `migrations.md` (tidak relevan, T08 tanpa migrasi); `docs/tickets/README.md` bagian "Frontend (Svelte)" dan "Kontrak API"; `docs/superpowers/specs/2026-10-20-grimoire-design.md`; `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md` (bagian 2, 7, 11, 13); precedent `docs/superpowers/plans/discovery-mcp-gateway/frontend-plan.md` (T07). Kode frontend yang ada: `frontend/src/domain/core/audit-log/{listentries,getentry}/*` (T02), `frontend/src/domain/project/knowledge/*` (T05), `frontend/e2e/{audit-log,knowledge}.spec.ts`, `frontend/e2e/helpers/{auth,apikeys,projects,knowledge}.ts`.

**Inti keputusan.** Tiket T08 bagian UI menyatakan "Tidak ada", dan overview bagian 1 menyatakan frontend tidak berubah. Maka T08 **tidak menambah route, view, komponen, store, service, i18n, atau folder `frontend/src/domain/discovery/**`**. Katalog aksi `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge` sudah ada di `frontend/src/domain/core/audit-log/listentries/types.ts` (grup `mcp`), sehingga filter Audit Log tidak perlu diubah. Pekerjaan frontend T08 hanya **pengujian black-box di Playwright**: (1) memanggil tiga tool lewat `POST /mcp` dengan API key nyata dan memeriksa teks/`structuredContent`/`isError`, (2) memverifikasi jejak agent di layar `/audit-log` yang sudah ada (baris aktor `api_key`, aksi mono, target path mono, panel detail tanpa isi file).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T08 (ukuran S) |
| Scope | fullstack menurut tiket; plan ini: bagian frontend saja (e2e, tanpa kode aplikasi) |
| Domain | discovery |
| Module | knowledge-tools |
| Feature frontend baru | tidak ada (tanpa `frontend/src/domain/discovery/knowledge-tools/**`) |
| Kontrak REST | tidak berubah; tidak ada `api/modules/discovery/knowledge-tools.yml` (`/mcp` bukan REST `/api`). endpointsChanged = false |
| Bergantung pada | T05 (merged), T07 (BELUM merge; e2e T08 memakai helper `frontend/e2e/helpers/mcp.ts` dari plan frontend T07) |

Endpoint yang dipakai frontend (hanya oleh e2e, bukan oleh aplikasi Svelte):

| Endpoint | Method | Version | Pemilik | Dipakai untuk |
|---|---|---|---|---|
| `/mcp` (`initialize`, `tools/list`, `tools/call`) | POST | MCP `2025-11-25` / `2025-06-18` / `2025-03-26`, JSON-RPC 2.0 | T07; tool dari T08 | panggil `list_files`, `read_file`, `search_knowledge` |
| `/api/projects/{id}/knowledge/upload` | POST | REST, tanpa perubahan (`api/modules/project/knowledge.yml`) | T05 | menyiapkan fixture knowledge |
| `/api/projects/{id}/api-keys` | POST | REST, tanpa perubahan (`api/modules/project/api-key.yml`) | T04 | membuat key agent untuk project A |
| `/api/projects` | POST | REST, tanpa perubahan (`api/modules/project/management.yml`) | T03 | project A dan B |
| `/api/audit-log`, `/api/audit-log/{id}` | GET | REST, tanpa perubahan (`api/modules/core/audit-log.yml`) | T02 | verifikasi jejak + layar `/audit-log` |

Route aplikasi yang tersentuh (tanpa perubahan kode): `/audit-log` (baris `mcp.list_files` / `mcp.read_file` / `mcp.search_knowledge` kini muncul dari data nyata) dan `/projects/:id/knowledge` (tidak berubah; dipakai e2e hanya untuk memastikan file yang dibaca agent sama dengan yang terlihat manusia).

---

## 2. Summary Plan

1. `frontend/e2e/helpers/knowledge-tools.ts` (baru): pembungkus tipis di atas helper T07 `mcp.ts` untuk `tools/call` (`callTool(request, key, name, args)`), parser Zod `ToolCallResult` (`content[0].text`, `isError`, `structuredContent` opsional), dan pengikut penanda `[terpotong: ... offset=N ...]` / `[ada N file lagi ... offset=N ...]`.
2. `frontend/e2e/helpers/knowledge.ts`: tambah `bigText(bytes)` (isi campuran ASCII + multibyte 3/4 byte yang deterministik) untuk AC2. Tidak mengubah fungsi yang sudah ada.
3. `frontend/e2e/knowledge-tools.spec.ts` (baru): skenario positif/negatif AC1-AC6 lewat `/mcp`, lalu verifikasi jejak di `/audit-log` (AC audit D8).
4. `frontend/src/domain/core/audit-log/listentries/components/components.test.ts`: tambah satu asersi bahwa grup `mcp` memuat tiga aksi T08 (penjaga regresi katalog; tidak mengubah kode).

Tidak ada kunci i18n baru, tidak ada perubahan `app.css`, `DESIGN.md`, route, maupun store.

---

## 3. Plan (langkah berurutan)

Prasyarat: binary dengan `/mcp` aktif (T07 merged) dan tiga tool T08 terdaftar (backend F1-F9). Langkah 1-2 dan 4 bisa dikerjakan paralel dengan backend (FE slicing); langkah 3 butuh binary lengkap. Bila `frontend/e2e/helpers/mcp.ts` belum ada di `main` (T07 belum merge), developer berhenti dan melapor; jangan menyalin helper T07.

### Langkah 1. Helper `knowledge-tools.ts`

File baru `frontend/e2e/helpers/knowledge-tools.ts`. Mengimpor dari `./mcp` (T07): `mcpInitialize`, `mcpPost` (nama mengikuti implementasi T07; sesuaikan saat implementasi, jangan menduplikasi logika header `Authorization`/`MCP-Protocol-Version`).

```ts
export const ToolCallResult = z.object({
  content: z.array(z.object({ type: z.literal('text'), text: z.string() })).min(1),
  isError: z.boolean(),
  structuredContent: z.record(z.string(), z.unknown()).optional(),
  _meta: z.record(z.string(), z.unknown()).optional(),
});
export const ListStructured = z.object({
  items: z.array(z.object({ path: z.string(), size_bytes: z.number().int().nonnegative(), updated_at: z.string() })),
  folders: z.array(z.string()),
  total: z.number().int().nonnegative(),
  next_offset: z.number().int().nullable(),
  recursive: z.boolean(),
});
export const SearchStructured = z.object({
  items: z.array(z.object({ path: z.string(), score: z.number(), snippet: z.string() })),
  total: z.number().int().nonnegative(),
  query: z.string(),
});

export async function callTool(request: APIRequestContext, key: string, name: string, args: unknown): Promise<z.infer<typeof ToolCallResult>>;
/** Ambil N dari penanda akhir; null bila tidak ada penanda. */
export function nextOffset(text: string): number | null; // regex /offset=(\d+)/ pada baris terakhir yang diawali '['
/** Isi tanpa penanda (untuk menggabungkan potongan read_file). */
export function stripMarker(text: string): string;       // buang "\n\n[terpotong: ...]" di akhir
```

`callTool` gagal (`expect`) bila respons JSON-RPC berisi `error` (bukan `result`); skenario yang memang mengharapkan error JSON-RPC tidak memakai helper ini.

### Langkah 2. Fixture knowledge

`frontend/e2e/helpers/knowledge.ts`: tambah

```ts
/** Teks deterministik `bytes` byte: ASCII + 'é' (2B) + '€' (3B) + '𝄞' (4B) berulang, dipotong di batas karakter. */
export function bigText(bytes: number): string;
```

Fixture diunggah lewat `uploadApi` yang sudah ada (tanpa fixture file baru di repo). Project A:

| Path | Isi |
|---|---|
| `readme.md` | `# Payments API knowledge\nMulai dari docs/.` (±40 B) |
| `docs/auth/login-flow.md` | alur login, memuat kata `session cookie` |
| `docs/auth/readme.md` | ringkasan folder auth |
| `docs/guides/rate-limit.md` | `Rate limiting per key: 60 request per menit.` |
| `docs/big.md` | `bigText(200 * 1024)` |

Project B: `secret/b.md` berisi `SECRET-B-7f3a`.

Batas ukuran upload T05 diperiksa saat implementasi (`knowledge-limits.spec.ts`); 200 KB berada di bawah batas file T05 yang sekarang. Bila tidak, turunkan ke ukuran terbesar yang diizinkan dan tetap minimal 3 potongan.

### Langkah 3. Spec `knowledge-tools.spec.ts`

File baru `frontend/e2e/knowledge-tools.spec.ts`. `test.describe.configure({ mode: 'serial' })` hanya di dalam describe yang berbagi fixture; tiap describe membuat project dengan nama unik bertag (`Payments e2e-<tag>`), key T04 (`claude-laptop-<tag>`), dan upload fixture di `beforeAll`. Rincian skenario ada di bagian 7.

Penutup tiap describe: verifikasi jejak di UI `/audit-log` (super admin):
- buka `/audit-log?actor_type=api_key&actor_id=<keyId>&action=mcp.read_file`;
- baris pertama: chip agent (kuning-pucat, label mono `claude-laptop-<tag> (<prefix>)`), aksi `mcp.read_file` mono, target `docs/auth/login-flow.md` mono, pill `Berhasil`;
- klik baris: panel detail kanan menampilkan `result` dengan `bytes`, `total_bytes`, `truncated:false`, dan TIDAK memuat teks isi file maupun `SECRET-B-7f3a`.

### Langkah 4. Penjaga katalog

`frontend/src/domain/core/audit-log/listentries/components/components.test.ts`: satu `it` baru: `ACTION_GROUPS.find(g => g.key === 'mcp').actions` memuat `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge`. Tidak ada perubahan pada `types.ts`.

### Langkah 5. Gerbang

```
cd frontend
npm run check
npm run test -- --run
npx playwright test e2e/knowledge-tools.spec.ts
```
Dari root: `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` (tidak ada file `src` berubah selain tes; harus tetap lolos).

---

## 4. Route, komponen, state

| Hal | T08 |
|---|---|
| Route baru | tidak ada |
| Komponen baru | tidak ada |
| Store baru | tidak ada |
| Service + Zod baru di `src/` | tidak ada (Zod hanya di helper e2e) |
| `index.ts` publik | tidak berubah |

State layar yang tampil dari data T08 (semuanya sudah ditangani T02, diverifikasi e2e, tidak diubah):

| State | Layar `/audit-log` dengan filter `action=mcp.*` |
|---|---|
| Loading | bar abu rata di posisi nilai, header tabel langsung tampil, tanpa shimmer (implementasi T02) |
| Empty | panel tabel tetap berbingkai, satu kalimat kosong T02 (mis. sebelum agent memanggil tool) |
| Error | di dalam panel tabel: satu kalimat + tombol "Coba lagi" (T02) |
| Sukses | baris `mcp.read_file` dkk, aktor chip agent |
| Gagal tool | baris dengan pill `Gagal` dan `result.error_code` (`invalid_argument`, `not_found`, `invalid_request`) di panel detail |

---

## 5. Migration

Tidak ada.

---

## 6. Changes (per file/folder)

| File | Status | Isi |
|---|---|---|
| `frontend/e2e/helpers/knowledge-tools.ts` | baru | `callTool`, `ToolCallResult`, `ListStructured`, `SearchStructured`, `nextOffset`, `stripMarker` |
| `frontend/e2e/helpers/knowledge.ts` | ubah | tambah `bigText(bytes)` |
| `frontend/e2e/knowledge-tools.spec.ts` | baru | e2e positif/negatif + verifikasi `/audit-log` |
| `frontend/src/domain/core/audit-log/listentries/components/components.test.ts` | ubah | asersi tiga aksi T08 di grup `mcp` |

TIDAK berubah: `frontend/src/**` selain tes di atas, `frontend/src/app.css`, `DESIGN.md`, `frontend/src/lib/i18n/*.json`, `frontend/playwright.config.ts` (env limiter MCP sudah ditambahkan T07), `api/modules/**`.

---

## 7. Test Scenario

### 7.1 Unit (Vitest)

| Tes | File | Ekspektasi |
|---|---|---|
| katalog aksi T08 | `components.test.ts` | grup `mcp` memuat `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge` |
| `nextOffset` | diuji di dalam spec Playwright sebagai `test('helper: nextOffset')` murni tanpa jaringan | `"...\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]"` -> 49998; `"...\n\n[ada 3 file lagi, panggil list_files dengan offset=2 recursive=true]"` -> 2; teks tanpa penanda -> null; `offset=` di tengah isi file (bukan baris penanda) -> null |
| `stripMarker` | sama | penanda dan dua `\n` di depannya terbuang; isi tanpa penanda tidak berubah |

### 7.2 Integration

Tidak ada tes integrasi frontend baru: tidak ada service Svelte. Integrasi Go (`backend/cmd/internal/server/knowledgetools_integration_test.go`) ada di overview bagian 10.

---

## 8. E2E Scenarios (`frontend/e2e/knowledge-tools.spec.ts`)

Setup bersama: super admin login (`apiLogin`), buat project A dan B, unggah fixture, buat key untuk A (`kA`), simpan `keyId` dan `prefix`.

### Positif

| # | AC | Langkah | Ekspektasi |
|---|---|---|---|
| P1 | 6 | `tools/list` | memuat `list_files`, `read_file`, `search_knowledge`, masing-masing `annotations.readOnlyHint === true`, `inputSchema.type === 'object'`, `additionalProperties === false`; tidak ada properti `project_id` di schema mana pun |
| P2 | 1 | `list_files {}` | `isError:false`; teks memuat `readme.md` dan `[folder] docs/`, tidak memuat `docs/auth/login-flow.md`; `ListStructured.parse`: `folders` = `["docs"]`, `next_offset === null` |
| P3 | 1 | `list_files {path:"docs"}` lalu `{path:"docs/auth"}` | tingkat demi tingkat: `[folder] docs/auth/`, `[folder] docs/guides/`, `docs/big.md`; lalu dua file auth |
| P4 | 1 | `list_files {recursive:true, limit:2}`, ikuti `nextOffset` sampai null | halaman pertama 2 item + penanda `offset=2`; gabungan path = 5 file, tanpa duplikat; `total === 5` di setiap halaman |
| P5 | 2 | `read_file {path:"docs/big.md", limit:50000}`, ikuti `nextOffset` | minimal 4 potongan; `stripMarker` digabung === `bigText(204800)` persis (tanpa duplikat/hilang, termasuk karakter 4 byte di batas); potongan terakhir tanpa penanda; tidak ada `_meta.truncated` |
| P6 | – | `read_file {path:"docs/auth/login-flow.md"}` | isi sama dengan `getFileApi` (T05) untuk path itu; tanpa penanda |
| P7 | 4 | `search_knowledge {query:"rate limit"}` | teks memuat `docs/guides/rate-limit.md` dan `**Rate**` atau `**rate**`; `SearchStructured.parse`: `total >= 1`, `items[0].path === 'docs/guides/rate-limit.md'`; lalu `read_file` pada path itu sukses |
| P8 | 4 | `search_knowledge {query:"rate", path:"docs/auth"}` | tidak ada hasil di luar `docs/auth/` |
| P9 | 4 | `search_knowledge {query:"zzqx-tidak-ada"}` | `isError:false`; teks memuat `Tidak ada hasil` dan saran `list_files`; `structuredContent.total === 0` |
| P10 | – | project kosong C + key C: `list_files {}` | `isError:false`, teks `Belum ada file knowledge di project ini.` |
| P11 | D8 | UI `/audit-log?actor_type=api_key&actor_id=<keyId>` | baris `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge` tampil dengan chip agent + label mono; filter `action=mcp.read_file` menyisakan hanya baris itu; panel detail `read_file` memuat `bytes`/`total_bytes` dan tidak memuat isi file |

### Negatif

| # | AC | Langkah | Ekspektasi |
|---|---|---|---|
| N1 | – | `read_file {}` | `isError:true`, teks diawali `Argumen tidak valid: path wajib diisi` |
| N2 | – | `list_files {limit:900}` | `isError:true`, teks memuat `limit harus 1-500 (diterima: 900)` |
| N3 | 5 | `kA`: `read_file {path:"secret/b.md", project_id:"<B>"}` dan `list_files {project_id:"<B>"}` | `isError:true`, teks memuat `ditentukan oleh API key`, tidak memuat `SECRET-B-7f3a`; tanpa `project_id`, `read_file {path:"secret/b.md"}` -> `File tidak ditemukan` (project A) |
| N4 | – | `search_knowledge {query:"   "}` dan `{query:"<201 karakter>"}` | `isError:true`, `query wajib diisi 1-200 karakter` |
| N5 | 3 | `read_file {path:"docs/auth/login-flw.md"}` | `isError:true`, teks memuat `File tidak ditemukan`, `Similar:`, `- docs/auth/login-flow.md`, `list_files` |
| N6 | – | `read_file {path:"docs/auth/login-flow.md", offset:999999999}` | `isError:true`, teks diawali `Permintaan ditolak:` |
| N7 | – | `list_files {path:"../etc"}` | `isError:true`, `tidak boleh memuat segmen ..` |
| N8 | 4 | `search_knowledge` dengan `a & b \| !c:* (x)`, `'"`, `%_\\` | respons berisi `result` (bukan `error` JSON-RPC), `isError:false` |
| N9 | – | `list_files {recursive:"true"}` | `isError:true`, `recursive harus boolean` |
| N10 | – | key yang sudah dicabut (T04 revoke) memanggil `list_files {}` | ditolak gateway (format penolakan T07), tidak ada entri `mcp.list_files` baru untuk key itu |
| N11 | D8 | UI `/audit-log?action=mcp.read_file&actor_id=<keyId>` setelah N5 | baris dengan pill `Gagal`; panel detail memuat `error_code: not_found`, `similar_count` |

---

## 9. Request/Response body

### 9.1 `tools/call` list_files (terpotong)

Request:
```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_files","arguments":{"recursive":true,"limit":2}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "result": {
    "content": [{"type":"text","text":"Folder: / (akar) | rekursif: ya | 5 file\ndocs/auth/login-flow.md  2.1 KB  2026-10-21 09:02 UTC\ndocs/auth/readme.md  0.4 KB  2026-10-21 09:02 UTC\n\n[ada 3 file lagi, panggil list_files dengan offset=2 recursive=true]"}],
    "isError": false,
    "structuredContent": {
      "items": [
        {"path":"docs/auth/login-flow.md","size_bytes":2150,"updated_at":"2026-10-21T09:02:11Z"},
        {"path":"docs/auth/readme.md","size_bytes":412,"updated_at":"2026-10-21T09:02:11Z"}
      ],
      "folders": [],
      "total": 5,
      "next_offset": 2,
      "recursive": true
    }
  }
}
```

### 9.2 `tools/call` read_file (potongan)

```json
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/big.md","limit":50000}}}
```
```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"<49998 byte isi file>\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]"}],"isError":false}}
```

### 9.3 `tools/call` read_file (tidak ditemukan)

```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"File tidak ditemukan: docs/auth/login-flw.md\nSimilar:\n- docs/auth/login-flow.md\n- docs/auth/readme.md\nGunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi."}],"isError":true}}
```

### 9.4 `tools/call` search_knowledge

```json
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"rate limit","limit":5}}}
```
```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "content": [{"type":"text","text":"1 dari 1 hasil untuk \"rate limit\"\n1. docs/guides/rate-limit.md  (skor 0.608)\n   **Rate** **limiting** per key: 60 request per menit.\nBaca isi lengkap dengan read_file(path)."}],
    "isError": false,
    "structuredContent": {
      "items": [{"path":"docs/guides/rate-limit.md","score":0.6079,"snippet":"**Rate** **limiting** per key: 60 request per menit."}],
      "total": 1,
      "query": "rate limit"
    }
  }
}
```

### 9.5 Format error

Argumen tidak valid (hasil tool, bukan error JSON-RPC):
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-500 (diterima: 900). Pakai limit 1-500 dan lanjutkan dengan offset."}],"isError":true}}
```
`project_id` ditolak:
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Argumen tidak valid: project_id tidak dikenal; project ditentukan oleh API key dan tidak bisa diganti. Hapus argumen project_id."}],"isError":true}}
```
Ditolak T05 (`invalid_request`):
```json
{"jsonrpc":"2.0","id":14,"result":{"content":[{"type":"text","text":"Permintaan ditolak: offset melewati ukuran file. Mulai lagi dari offset=0 atau pakai offset dari penanda terpotong."}],"isError":true}}
```
Error internal (dipetakan gateway T07):
```json
{"jsonrpc":"2.0","id":15,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=01HZX3K9Q7"}],"isError":true}}
```
Teks persis untuk tiap pesan mengikuti plan usecase; e2e hanya mencocokkan frasa kunci yang tercantum di bagian 8.

### 9.6 Yang dibaca e2e dari `GET /api/audit-log` (kontrak T02, tidak berubah)

`GET /api/audit-log?actor_type=api_key&actor_id=0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c&action=mcp.read_file`:
```json
{
  "items": [
    {"id":"0192f0d1-2b00-7d11-8a00-000000000031","occurred_at":"2026-10-21T09:14:25.330Z","actor_type":"api_key","actor_id":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","actor_label":"claude-laptop (nbk_4f9a)","project_id":"0192f0c4-0000-7000-8000-00000000000a","project_label":"Payments API","action":"mcp.read_file","target_type":"file","target":"docs/auth/login-flow.md","status":"ok"}
  ],
  "next_cursor": null
}
```
`GET /api/audit-log/{id}` (bagian `result`, isi file tidak pernah ada):
```json
{"id":"0192f0d1-2b00-7d11-8a00-000000000031","action":"mcp.read_file","target_type":"file","target":"docs/auth/login-flow.md","status":"ok","args":{"path":"docs/auth/login-flow.md"},"result":{"found":true,"bytes":2150,"total_bytes":2150,"offset":0,"next_offset":-1,"truncated":false}}
```
(field lain mengikuti `api/modules/core/audit-log.yml`; field gateway seperti durasi ditambahkan T07.)

Error REST (format `foundation.yml`):
```json
{"error":{"code":"INVALID_ARGUMENT","message":"parameter tidak valid","details":{"field":"action"}},"trace_id":"01HZX3K9Q7"}
```

---

## 10. Visual

T08 tidak menambah layar. Bagian ini mencatat bagaimana hasil kerja T08 tampil di layar yang sudah ada, supaya asersi e2e dan Design check tidak menyimpang dari DESIGN.md.

### 10.1 Pertanyaan per layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| `/audit-log` (T02) | Siapa (manusia atau agent) melakukan apa, kapan? Khusus T08: file knowledge mana yang dibaca atau dicari agent ini? | tabel entri (panel radius 8) dengan baris `mcp.read_file` / `mcp.list_files` / `mcp.search_knowledge` dan kolom target berisi path mono |
| Tab Knowledge (T05) | File mana yang agent baca, dan apa isinya? | pohon file mono + panel baca; T08 tidak mengubahnya, hanya menjamin isi yang dibaca agent identik (P6) |

Temuan utama yang didukung data T08: "agent `claude-laptop` membaca `docs/auth/login-flow.md` pukul 09:14" (baris pertama hasil filter `actor_id`). Tidak ada blok baru untuknya.

### 10.2 Pola DESIGN.md dan kerangka

- `/audit-log`: pola **list + filter bar + panel detail kanan** (overlay di layar sempit), di kerangka `AppLayout` framed-sheet (sidebar 228px di `bg-ground`, sheet putih inset 8px, radius 16).
- Tab Knowledge: pola **dua kolom** (pohon + panel baca) di workspace project (title row + tab garis bawah 2px).
- Tidak ada pola list/detail/settings baru.

### 10.3 Elemen khas produk di layar pertama

- **Jejak agent** (Wajah produk #4): setiap baris T08 beraktor `api_key`, sehingga tampil dengan chip kotak kuning-pucat + ikon agent + label mono `claude-laptop (nbk_4f9a)`. E2E P11 memastikan chip ini, bukan chip bulat manusia.
- **Mono untuk satu pekerjaan** (#1): aksi `mcp.read_file`, target `docs/auth/login-flow.md`, prefix `nbk_4f9a` mono; label kolom dan judul tidak.
- Satu penanda status per baris: pill `Berhasil` / `Gagal` (pasangan `pill-*`, disertai teks). `not_found` dan `invalid_argument` tampil sebagai `Gagal`, alasannya hanya di panel detail (merah sebagai sinyal, bukan tekstur).

### 10.4 Token/utility yang dipakai

Hanya yang sudah ada di `app.css` dan dipakai T02/T05: `bg-ground`, `bg-surface`, `bg-raised`, `bg-active`, `border-border`, `border-rule`, `border-control`, `text-fg`, `text-muted`, `bg-accent/20` + `text-accent-ink` (chip agent), `bg-pill-{positive,negative,neutral}-bg` + `text-pill-…-ink`, `rounded-base`, `rounded-card`, `rounded-sheet`, `shadow-panel`, `font-mono`, `tabular-nums`. Tidak ada hex, radius, font, shadow, atau komponen dekoratif baru. Tidak ada usulan perubahan DESIGN.md.

Catatan (bukan perubahan, sama dengan catatan T07): `AuditTable` memberi chip kuning-pucat ke semua aktor non-`user`. Untuk T08 tidak berdampak karena semua baris T08 beraktor `api_key`.

### 10.5 Data contoh

Kata dan angka produk ini (dilarang Acme/John Doe/lorem):
- project `Payments API` (A), `Billing Internal` (B), `Onboarding Docs` (C, kosong);
- key `claude-laptop`, prefix `nbk_4f9a`;
- path `readme.md`, `docs/auth/login-flow.md`, `docs/auth/readme.md`, `docs/guides/rate-limit.md`, `docs/big.md` (200 KB = 204800 byte);
- kueri `rate limit`, salah ketik `docs/auth/login-flw.md`;
- ukuran `2.1 KB` = `2150` byte (satu variabel; teks dan `structuredContent` harus sama), `total` 5 file = jumlah baris fixture.
