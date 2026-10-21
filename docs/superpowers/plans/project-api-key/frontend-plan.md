# T04 project/api-key: Frontend Plan (Svelte)

Tiket: `docs/tickets/T04-project-api-key.md` (bagian UI). Plan bersama: `overview.md` (K1-K14, 4.4 DTO, 4.5 reason, 11 E2E, 12 body, 13 OpenAPI). Kontrak per endpoint: `usecase-createkey.md`, `usecase-listkeys.md`, `usecase-revokekey.md`. `verifykey`, `touchlastused`, `countkeys` adalah handler bus tanpa UI. Konvensi: `docs/tickets/README.md` bagian "Frontend (Svelte)" dan `rules.txt` bagian 12 (rules menang soal arsitektur).

Dokumen ini adalah **satu-satunya plan frontend** untuk T04; frontend tidak dipecah per usecase. Bila catatan FE di overview/plan usecase berbeda dengan dokumen ini, **ikuti dokumen ini** (selisih dicatat di "Keputusan").

Konteks yang dimuat: tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: tiket T04, `docs/tickets/README.md`, `docs/superpowers/specs/2026-10-20-grimoire-design.md`, `overview.md` + 6 plan usecase T04, `docs/superpowers/plans/project-management/frontend-plan.md` (workspace `/projects/:projectID/:tab`, registri tab `app/workspace/tabs.ts`, `ProjectContext`, `shared/utils/text.ts`), dan kode FE di `main`: `shared/services/http.ts` (`get/post`, `ApiError`, CSRF otomatis untuk mutasi, handler 401 global), `shared/validation/error.ts` (`fieldErrorsOf`, `reasonOf` sudah ada), `shared/components/{Dialog,Table,Button,Input,EmptyState,ErrorState,Spinner}`, `shared/stores/{session,i18n,toast}`, `shared/utils/datetime.ts` (`formatDateTime`), `domain/core/auth/users/components/OneTimeSecret.svelte` (pola salin satu kali, tidak di-import), `domain/core/audit-log/listentries/stores/filterQuery.ts` (param URL Audit Log: `project_id`, `action`, `target`), `app/routes/{index,router}.ts`, `.dependency-cruiser.cjs`, `e2e/helpers/auth.ts`, `e2e/users.spec.ts` (pola uji clipboard), `playwright.config.ts`.

---

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack (dokumen ini: frontend saja; backend di `overview.md` dan `usecase-*.md`) |
| Domain | project |
| Module | api-key (folder FE `frontend/src/domain/project/api-key/`, lihat K1) |
| Feature FE | `listkeys`, `createkey`, `revokekey` |
| Route UI | `/projects/:projectID/api-keys` (tab `api-keys` di workspace T03; route `/projects/:projectID/:tab` sudah ada dari T03, tidak menambah route baru) |
| Akses UI | Route `meta: { auth: true, layout: 'app' }` (dari T03). Semua level akses (`member`/`owner`/`admin`) boleh buat dan cabut; tautan Audit Log hanya bila `role === 'super_admin'` (UX saja; otorisasi di backend) |
| Kontrak API | `api/modules/project/api-key.yml` (`info.version: 0.1.0`), schema `ApiKey`, `CreateApiKeyRequest`, `CreateApiKeyResponse`, `ApiKeyList`, `ApiKeyEnvelope`; `Error` dari `api/modules/infra/foundation.yml` |
| Prasyarat | T00, T01 FE, T02 FE (merged). **T03 FE** (workspace, `ProjectContextView`, `app/workspace/tabs.ts`, `shared/utils/text.ts`) wajib sudah di `main`. Backend T04 F + U4-U6 |

Endpoint yang dipanggil frontend (prefix `/api`, tanpa versi di path; versi kontrak `0.1.0`; mutasi otomatis membawa `X-Requested-With: grimoire` dari `shared/services/http.ts`):

| Endpoint | Method | Version | Dipakai oleh (service FE) | Akses (backend) |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | GET | 0.1.0 | `listkeys/services/listkeys.service.ts` (`listKeys`) | anggota, super_admin |
| `/api/projects/{projectID}/api-keys` | POST | 0.1.0 | `createkey/services/createkey.service.ts` (`createKey`) | anggota, super_admin |
| `/api/projects/{projectID}/api-keys/{keyID}/revoke` | POST | 0.1.0 | `revokekey/services/revokekey.service.ts` (`revokeKey`) | anggota, super_admin |

Bus `project.apikey.verify`, `project.apikey.touch`, `project.apikey.count-keys` dan bus yang dikonsumsi backend (`check-access`, `get-project`, `core.audit.record`) tidak disentuh frontend. Frontend tidak menambah endpoint. Tautan Audit Log hanya berupa `href` ke route `/audit-log` yang sudah ada (T02), dibangun di `app/`.

---

## Summary Plan

Frontend T04 menambah tab **API Keys** ke workspace project (`/projects/:projectID/api-keys`) dengan tiga feature:

1. **`listkeys`**: tabel key project: nama, prefix (`gr_<prefix>_…`, monospace), dibuat oleh (email) dan kapan, terakhir dipakai (`-` bila `null`), status (aktif / dicabut). Key dicabut tampil redup beserta "dicabut oleh … pada …", tidak punya tombol aksi, dan tidak ada tombol hapus. Urutan mengikuti backend: aktif dulu, `created_at` turun, `id`. Store daftar dibuat per instance dan dibagikan ke tab agar hasil create/revoke langsung diterapkan tanpa GET ulang.
2. **`createkey`**: dialog dua langkah. Langkah 1: input nama (validasi klien 2-60 rune setelah trim, tanpa karakter kontrol). Langkah 2 (layar hasil): key lengkap dengan tombol salin, peringatan "tidak akan ditampilkan lagi", potongan konfigurasi **Claude Code** (`claude mcp add --transport http <nama> <URL>/mcp --header "Authorization: Bearer <key>"`) dan **JSON generik** untuk klien MCP lain, masing-masing dengan tombol salin. URL MCP = `mcp_url` dari respons, atau `window.location.origin + "/mcp"` bila `null`. Raw key hanya hidup di state lokal dialog dan dibuang saat dialog ditutup.
3. **`revokekey`**: dialog konfirmasi yang menyebut nama dan prefix key; sukses langsung memindahkan baris ke status dicabut.

Komposisi lintas module (tab, tautan Audit Log, navigasi saat project hilang) ada di `app/` (rule 12.4). Feature tidak meng-import router atau feature lain; komunikasi lewat props dan callback.

Keamanan dan ketahanan:
- Raw key tidak pernah masuk store, URL, `localStorage`, toast, log console, atau atribut `title`; hanya di `$state` dialog `createkey` dan di-`null`-kan saat ditutup. Dialog tidak bisa ditutup selama request create berjalan (mencegah key hilang).
- 404 project diperlakukan seragam (`onprojectgone` -> app ke beranda, pola T03). 404 `KEY_NOT_FOUND` dan 409 `ALREADY_REVOKED` -> pesan i18n + daftar dimuat ulang.
- Error bisnis dipetakan dari `details.reason` ke `project.apikey.error.<REASON>`; error field dari `details.fields.name` ke input; lainnya pesan server + trace id.
- Respons yang tidak sesuai schema -> `ApiError` `INVALID_RESPONSE`. Khusus create: pesan khusus "key mungkin sudah dibuat" + muat ulang daftar.
- Semua nilai server dirender sebagai teks (tanpa `{@html}`); potongan shell meng-quote nama dengan aman.

---

## Plan

Urutan wajib: FE-0 (prasyarat bersama) -> feature L (listkeys), C (createkey), R (revokekey) boleh paralel (tidak saling import) -> komposisi `app/` (A1-A3) -> e2e (E) -> verifikasi (V). Semua langkah TDD: test vitest ditulis lebih dulu.

### FE-0. Prasyarat bersama (generik, tanpa pengetahuan domain)

1. **Cek prasyarat T03 FE**: `app/workspace/tabs.ts`, `app/views/ProjectWorkspaceView.svelte`, `domain/project/management/getproject/index.ts` (ekspor `ProjectContext`), `shared/utils/text.ts` (`runeLength`, `hasControlChars`) ada di `main`. Bila belum ada, berhenti dan laporkan (T04 bergantung pada T03; jangan membuat ulang workspace T03 di PR ini).
2. **`shared/components/Dialog.svelte`** (ubah, kompatibel mundur): tambah props
   - `dismissible?: boolean = true`: bila `false`, event `cancel` (Esc) di-`preventDefault`, klik backdrop diabaikan, dan tombol tutup `disabled`.
   - `size?: 'md' | 'lg' = 'md'`: `md` = `max-w-lg` (perilaku lama), `lg` = `max-w-2xl` (layar hasil berisi potongan konfigurasi).
3. **`shared/components/Table.svelte`** (ubah, kompatibel mundur): tambah props `rowClass?: (row: T) => string` (kelas tambahan pada `<tr>`) dan `rowAttrs?: (row: T) => Record<string, string>` (atribut `data-*` pada `<tr>`, kunci yang tidak diawali `data-` diabaikan).
4. **`shared/utils/clipboard.ts`** (baru): `export async function copyText(text: string): Promise<boolean>`; `navigator.clipboard.writeText`, `false` bila API tidak ada atau menolak. Tanpa fallback `execCommand`.
5. **`shared/components/CopyField.svelte`** (baru, diekspor di `components/index.ts`): props `value: string`, `label: string`, `multiline?: boolean = false`, `rows?: number = 3`, `testid?: string`. Render `<input readonly>` atau `<textarea readonly>` (font mono, `spellcheck="false"`, `autocomplete="off"`) dengan label terhubung, tombol "Salin" (`common.copy`) yang berubah jadi "Tersalin" (`common.copied`) 2 detik. Bila `copyText` gagal: fokus + `select()` isi dan tampilkan `common.copyManual`. Status salin diumumkan lewat `<span role="status" aria-live="polite">`. Tidak menyentuh `OneTimeSecret.svelte` milik T01 (di luar lingkup).
6. **i18n** `shared/i18n/id.json` dan `en.json`: kunci di tabel "Kunci i18n" (paritas dijaga `i18n-keys.test.ts`). Pakai ulang `common.*`, `error.*` yang ada; tambah `common.copy`, `common.copied`, `common.copyManual` hanya bila belum ada.
7. Tidak ada dependensi npm baru.

### L. Feature `listkeys` (`frontend/src/domain/project/api-key/listkeys/`)

**L1 `types.ts`**
```ts
export const KEY_STATUSES = ['active', 'revoked'] as const;
export const ApiKeySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  prefix: z.string().regex(/^[a-z0-9]{8}$/),
  status: z.enum(KEY_STATUSES),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  last_used_at: z.string().datetime({ offset: true }).nullable(),
  revoked_at: z.string().datetime({ offset: true }).nullable(),
  revoked_by: z.string().nullable(),
  revoked_by_label: z.string().nullable(),
});
export const ApiKeyListSchema = z.object({ items: z.array(ApiKeySchema) });
export type ApiKey = z.infer<typeof ApiKeySchema>;
export type ApiKeyList = z.infer<typeof ApiKeyListSchema>;
export function sortKeys(items: ApiKey[]): ApiKey[]; // salinan baru: aktif dulu, created_at DESC, id ASC (cermin ORDER BY backend)
export function displayPrefix(prefix: string): string; // "gr_" + prefix + "_…"
```
Schema tidak `.strict()` (field tambahan diabaikan), tetapi test memastikan respons yang memuat `raw_key`/`secret_hash` tetap tidak pernah dirender (field tidak ada di tipe).

**L2 `services/listkeys.service.ts`** (tanpa state)
```ts
export function listKeys(projectId: string, signal?: AbortSignal): Promise<ApiKeyList>;
// get('/api/projects/' + encodeURIComponent(projectId) + '/api-keys', { schema: ApiKeyListSchema, signal })
```
projectId tidak divalidasi di klien (backend 404 untuk bukan-uuid).

**L3 `stores/keyList.store.ts`** (per instance, tanpa fetch)
```ts
export type KeyListState =
  | { status: 'idle' }
  | { status: 'loading'; projectId: string; previous: ApiKey[] | null }
  | { status: 'ready'; projectId: string; items: ApiKey[] }
  | { status: 'notFound'; projectId: string }
  | { status: 'error'; projectId: string; error: ApiError; previous: ApiKey[] | null };
export interface KeyListStore extends Readable<KeyListState> {
  version: Readable<number>;              // naik tiap invalidate(); view memuat ulang saat berubah
  begin(projectId: string): void;
  resolve(projectId: string, items: ApiKey[]): void; // diurutkan dengan sortKeys
  fail(projectId: string, e: ApiError): void;        // status 404 -> notFound
  upsert(key: ApiKey): void;              // ganti berdasarkan id atau sisipkan; urut ulang; hanya bila status ready
  invalidate(): void;
}
export function createKeyListStore(): KeyListStore;
export const activeCount: (s: KeyListState) => number;
```
`resolve`/`fail` untuk `projectId` yang bukan milik `begin` terakhir diabaikan (proteksi race di samping abort).

**L4 `components/`** (presentasi; tidak memanggil service)
- `KeyStatusBadge.svelte`: props `status`. Teks `project.apikey.status.active` / `.revoked` (bukan warna saja).
- `KeysTable.svelte`: props `items: ApiKey[]`, `loading: boolean`, `error: ApiError | null`, `onretry()`, `onrevoke(key: ApiKey)`, `auditHref?: (key: ApiKey) => string`. Memakai `Table` bersama:
  - Kolom: `name` (teks, `break-words`), `prefix` (`<code>` `displayPrefix`), `created` (`created_by_label ?? project.apikey.unknownUser` + `<time datetime>` `formatDateTime`), `lastUsed` (`<time>` atau `-` dengan `aria-label` `project.apikey.neverUsed`), `status` (`KeyStatusBadge`; untuk dicabut tambah baris kecil `project.apikey.revokedBy` `{by}` `{at}`), `actions`.
  - `rowClass`: `opacity-60` untuk `status === 'revoked'`. `rowAttrs`: `data-key-id`, `data-status`.
  - Aksi baris aktif: tombol "Cabut" (`project.apikey.revoke.open`, `aria-label` `project.apikey.revoke.aria` `{name}`). Baris dicabut: tanpa tombol. Tidak ada tombol hapus di mana pun.
  - Bila `auditHref` diberikan: tautan "Audit Log" (`project.apikey.auditLink`, `aria-label` `{name}`) di kolom aksi untuk semua baris (aktif dan dicabut).
  - Layar sempit (< `md`): `Table` dibungkus `overflow-x-auto` di kontainer kartu sendiri; tidak memecah layout halaman.
- `format.ts`: `revokedLine(key, locale)`, `createdLine(key, locale)` (murni, mudah diuji).

**L5 `views/ListKeysView.svelte`** (satu-satunya pemanggil `listKeys` di feature ini)
- Props: `projectId: string`, `store: KeyListStore`, `oncreate(): void`, `onrevoke(key: ApiKey): void`, `onprojectgone(): void`, `auditHref?: (key: ApiKey) => string`.
- `$effect` pada `projectId` dan `$store.version`: abort request sebelumnya, `store.begin`, `listKeys`, `resolve`/`fail`. `AbortError` diabaikan.
- Header tab: judul `project.apikey.title`, ringkasan `project.apikey.activeCount` `{n}`, tombol "Buat key" (`project.apikey.create.open`) selalu tampil (semua level akses boleh membuat). Paragraf bantuan `project.apikey.intro` (key read-only, hanya untuk MCP).
- State:
  - `loading` pertama -> `Table loading` (`aria-busy`). Muat ulang dengan data lama -> tabel lama tetap tampil (tanpa kedip) + `aria-busy`.
  - `ready` kosong -> `EmptyState` judul `project.apikey.empty`, deskripsi `project.apikey.emptyHint`, aksi tombol "Buat key".
  - `error` -> `ErrorState` (pesan + trace id) + coba lagi (`store.invalidate()`); tanpa daftar parsial.
  - `notFound` -> panggil `onprojectgone()` sekali.
- **index.ts**:
```ts
export { default as ListKeysView } from './views/ListKeysView.svelte';
export { createKeyListStore } from './stores/keyList.store';
export type { KeyListStore } from './stores/keyList.store';
export type { ApiKey } from './types';
```

### C. Feature `createkey` (`frontend/src/domain/project/api-key/createkey/`)

**C1 `types.ts`**
```ts
export const NAME_MIN = 2;
export const NAME_MAX = 60;
export const RAW_KEY_RE = /^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$/;
export const CreatedKeySchema = /* salinan lokal bentuk ApiKey (K4) */;
export const CreateKeyResponseSchema = z.object({
  key: CreatedKeySchema,
  raw_key: z.string().regex(RAW_KEY_RE),
  mcp_url: z.string().url().nullable(),
});
export type CreateKeyResponse = z.infer<typeof CreateKeyResponseSchema>;
export type CreatedKey = z.infer<typeof CreatedKeySchema>;
export type NameError = 'length' | 'control' | null;
export function validateName(raw: string): { name: string; error: NameError };
// trim; runeLength < 2 atau > 60 -> 'length'; hasControlChars(name) -> 'control' (cermin ValidateName backend)
```

**C2 `services/createkey.service.ts`** (tanpa state)
```ts
export function createKey(projectId: string, name: string): Promise<CreateKeyResponse>;
// post('/api/projects/' + encodeURIComponent(projectId) + '/api-keys', { name }, { schema: CreateKeyResponseSchema })
```
Tidak memakai `AbortSignal`: request create tidak dibatalkan setelah terkirim (hasilnya adalah satu-satunya kesempatan melihat key).

**C3 `snippets.ts`** (murni, tanpa DOM global; origin diinjeksi)
```ts
export function resolveMcpUrl(mcpUrl: string | null, origin: string): string;
//   mcpUrl ada -> mcpUrl apa adanya; selain itu origin (tanpa trailing slash) + '/mcp'
export function shellQuote(s: string): string;
//   /^[A-Za-z0-9._-]+$/ -> apa adanya; selain itu "'" + s.replaceAll("'", "'\\''") + "'"
export function claudeCodeSnippet(name: string, url: string, rawKey: string): string;
//   `claude mcp add --transport http ${shellQuote(name)} ${shellQuote(url)} --header "Authorization: Bearer ${rawKey}"`
export function genericJsonSnippet(name: string, url: string, rawKey: string): string;
//   JSON.stringify({ mcpServers: { [name]: { type: 'http', url, headers: { Authorization: `Bearer ${rawKey}` } } } }, null, 2)
```
Raw key aman di dalam tanda kutip ganda shell karena alfabetnya hanya `[A-Za-z0-9_-]` (dijamin `RAW_KEY_RE`).

**C4 `components/`**
- `CreateKeyForm.svelte`: props `value` (bindable), `submitting`, `fieldError: string | null`, `formError: string | null`, `traceId?: string`, `onsubmit(name)`. `Input` label `project.apikey.create.name`, hint `project.apikey.create.nameHint`, `maxlength` tidak dipakai (validasi rune), `autocomplete="off"`. Tombol submit `loading`/`disabled` saat `submitting` (mencegah submit ganda). Enter men-submit.
- `KeyResult.svelte`: props `rawKey`, `name`, `mcpUrl: string`. Isi berurutan: peringatan `role="alert"` `project.apikey.result.onceWarning` (menonjol, tidak hanya warna); `CopyField` label `project.apikey.result.keyLabel` (`testid="raw-key"`); judul `project.apikey.result.claudeCode` + `CopyField multiline` berisi `claudeCodeSnippet`; judul `project.apikey.result.genericJson` + `CopyField multiline rows=8` berisi `genericJsonSnippet`; catatan `project.apikey.result.urlNote` `{url}` (menjelaskan sumber URL). Semua sebagai teks.

**C5 `views/CreateKeyDialog.svelte`**
- Props: `open: boolean`, `projectId: string`, `onclose(): void`, `oncreated(key: CreatedKey): void`, `onprojectgone(): void`, `onstale(): void` (minta daftar dimuat ulang).
- State lokal: `step: 'form' | 'result'`, `name`, `submitting`, `fieldError`, `formError`, `traceId`, `result: { rawKey, name, mcpUrl } | null`.
- `Dialog` judul `project.apikey.create.title` (form) / `project.apikey.result.title` (hasil), `size = step === 'result' ? 'lg' : 'md'`, `dismissible = !submitting`.
- Submit: `validateName` -> error klien (`project.apikey.validation.nameLength` / `nameControl`) tanpa request. Lalu `createKey`:
  - 201 -> `result = { rawKey: res.raw_key, name: res.key.name, mcpUrl: resolveMcpUrl(res.mcp_url, window.location.origin) }`, `step = 'result'`, `oncreated(res.key)` (daftar langsung memuat baris baru). Fokus pindah ke tombol salin key.
  - 400 `fields.name` -> `fieldError` = pesan server (fallback bila validasi klien terlewati); field lain -> `formError`.
  - 409 `NAME_TAKEN` -> `fieldError` `project.apikey.error.NAME_TAKEN`; input dipertahankan, dialog tetap terbuka.
  - 409 `KEY_LIMIT_REACHED` -> `formError` `project.apikey.error.KEY_LIMIT_REACHED` `{limit}` (dari `details.limit`, fallback pesan server bila `limit` tidak ada).
  - 404 -> tutup dialog, `onprojectgone()`.
  - `INVALID_RESPONSE` -> `formError` `project.apikey.error.createUncertain` + `onstale()` (key mungkin sudah terbuat; pengguna diminta memeriksa daftar dan mencabut bila perlu).
  - Lainnya (403 CSRF, 429, 5xx, `NETWORK`) -> `formError` = `error.message` + trace id.
- Layar hasil: footer satu tombol utama `project.apikey.result.done` ("Saya sudah menyimpan key"). Menutup (tombol, Esc, backdrop, X) -> `result = null`, `name = ''`, `step = 'form'`, lalu `onclose()`. Tidak ada jalan kembali ke layar hasil.
- Membuka ulang dialog selalu mulai dari form kosong.
- **index.ts**: `export { default as CreateKeyDialog } from './views/CreateKeyDialog.svelte'; export type { CreatedKey } from './types';`

### R. Feature `revokekey` (`frontend/src/domain/project/api-key/revokekey/`)

**R1 `types.ts`**: `RevokedKeySchema` (salinan lokal bentuk `ApiKey`, K4), `RevokeKeyResponseSchema = z.object({ key: RevokedKeySchema })`, `type RevokedKey`, `interface RevokeTarget { id: string; name: string; prefix: string }`.

**R2 `services/revokekey.service.ts`**
```ts
export function revokeKey(projectId: string, keyId: string): Promise<RevokedKey>;
// post('/api/projects/' + encodeURIComponent(projectId) + '/api-keys/' + encodeURIComponent(keyId) + '/revoke',
//      undefined, { schema: RevokeKeyResponseSchema }).then(r => r.key)
```
Tanpa body (sesuai kontrak; `Content-Type` tidak dikirim karena body `undefined`).

**R3 `components/RevokeConfirm.svelte`**: props `target: RevokeTarget`, `error: string | null`, `traceId?: string`. Teks `project.apikey.revoke.message` `{name}` `{prefix}` (prefix dalam `<code>`), penjelasan `project.apikey.revoke.effect` (agent langsung ditolak, tidak bisa diaktifkan lagi), dan area error `role="alert"`.

**R4 `views/RevokeKeyDialog.svelte`**
- Props: `open`, `projectId`, `target: RevokeTarget | null`, `onclose()`, `onrevoked(key: RevokedKey)`, `onstale()`, `onprojectgone()`.
- `Dialog` judul `project.apikey.revoke.title`, `dismissible = !submitting`. Footer: "Batal" (`common.cancel`) dan tombol bahaya "Cabut key" (`project.apikey.revoke.submit`, `loading` saat submitting).
- Konfirmasi -> `revokeKey`:
  - 200 -> tutup, `onrevoked(key)`, toast sukses `project.apikey.revoke.success` `{name}`.
  - 409 `ALREADY_REVOKED` -> tutup, toast `project.apikey.error.ALREADY_REVOKED`, `onstale()`.
  - 404 `KEY_NOT_FOUND` (reason) -> tutup, toast `project.apikey.error.KEY_NOT_FOUND`, `onstale()`.
  - 404 tanpa reason (project) -> tutup, `onprojectgone()`.
  - Lainnya -> pesan di dialog (`error.message` + trace id), dialog tetap terbuka untuk coba lagi.
- **index.ts**: `export { default as RevokeKeyDialog } from './views/RevokeKeyDialog.svelte'; export type { RevokeTarget, RevokedKey } from './types';`

### A. Komposisi `app/`

**A1. `app/views/workspace/ApiKeysTab.svelte`** (baru). Props `WorkspaceTabProps` (`ctx`, `currentUserId`, `onprojectgone`, `onleft`).
- `const store = createKeyListStore()` (satu per instance tab).
- `<ListKeysView projectId={ctx.project.id} {store} oncreate={() => (createOpen = true)} onrevoke={(k) => (revokeTarget = k)} {onprojectgone} auditHref={isSuperAdmin ? auditHref : undefined} />`.
- `<CreateKeyDialog open={createOpen} projectId={ctx.project.id} onclose={() => (createOpen = false)} oncreated={(k) => store.upsert(k)} onstale={() => store.invalidate()} {onprojectgone} />`.
- `<RevokeKeyDialog open={revokeTarget !== null} projectId={ctx.project.id} target={revokeTarget} onclose={() => (revokeTarget = null)} onrevoked={(k) => store.upsert(k)} onstale={() => store.invalidate()} {onprojectgone} />`.
- `isSuperAdmin = $session.identity?.role === 'super_admin'`.
- `auditHref(key) = '/audit-log?' + new URLSearchParams([['project_id', ctx.project.id], ['action', 'apikey.created'], ['action', 'apikey.revoked'], ['target', key.id]])`. Parameter sesuai `filterFromQuery` T02 (`project_id` uuid, `action` regex, `target` teks).
- Tipe `CreatedKey`/`RevokedKey` struktural sama dengan `ApiKey`; adaptasi tipe (bila TS mengeluh) dilakukan di sini, bukan di feature.

**A2. `app/workspace/tabs.ts`** (ubah): sisipkan `{ key: 'api-keys', labelKey: 'project.tab.apiKeys', view: ApiKeysTab }` tepat sebelum `members` (urutan final tiket T03: Knowledge, API Specs, API Keys, Members, Settings). Konsekuensi: `defaultTabKey()` menjadi `api-keys` sampai T05 menambah `knowledge` (K5). `app/workspace/tabs.test.ts` (ubah): ekspektasi urutan `['api-keys', 'members', 'settings']` dan `defaultTabKey() === workspaceTabs[0].key`.

**A3. `app/views/ProjectWorkspaceView.svelte`**: tidak diubah (tab dirender lewat registri). `ProjectWorkspaceView.test.ts` (ubah): tambah kasus `/projects/<id>/api-keys` merender `ApiKeysTab` dan nav memberi `aria-current="page"` pada "API Keys".

### E. E2E

- `frontend/e2e/helpers/apikeys.ts` (baru): `createKeyApi(request, projectId, name)` -> `{ key, raw_key }` (membawa `CSRF`), `revokeKeyApi(request, projectId, keyId)`, `listKeysApi(request, projectId)`. Nama key unik per test (`k-${Date.now()}-${rand}`). Pakai `helpers/projects.ts` dari T03 untuk membuat project dan anggota.
- `frontend/e2e/project-api-key.spec.ts` (baru): skenario bagian "E2E Scenarios". Konteks browser diberi `permissions: ['clipboard-read', 'clipboard-write']` (pola `users.spec.ts`).

### V. Verifikasi

`npm run check`, `npm test`, `npm run lint`, `npm run depcruise`, `npm run test:arch`, `npm run build` (bundle di-embed backend), lalu `npm run test:e2e` terhadap binary (butuh backend T04 + T03 + T01 + T02).

---

## State UI (loading / empty / error per permukaan)

| Permukaan | Loading | Empty | Error | Khusus |
|---|---|---|---|---|
| Tab API Keys (header project) | Ditangani `ProjectContextView` T03 (skeleton) | - | `ErrorState` T03 | Bukan anggota / project tidak ada / id bukan uuid -> `project.detail.notFound` seragam (T03) |
| Tabel key | Pertama: `Table loading` + `aria-busy`. Muat ulang: tabel lama tetap tampil + `aria-busy` | `project.apikey.empty` + `emptyHint` + tombol "Buat key" | `ErrorState` (pesan + trace id) + coba lagi; tanpa daftar parsial | 404 -> `onprojectgone` (beranda). Baris dicabut redup, tanpa aksi. Respons rusak -> `INVALID_RESPONSE` di `ErrorState` |
| Dialog buat key (form) | Tombol "Buat" `loading`, input `disabled`, dialog tidak bisa ditutup | - | Field `name`: validasi klien, `NAME_TAKEN`, pesan server `fields.name`. `formError`: `KEY_LIMIT_REACHED {limit}`, `createUncertain`, 5xx/jaringan + trace id | Validasi klien sebelum request. 404 -> tutup + beranda |
| Dialog buat key (hasil) | - | - | Salin gagal -> isi terseleksi + `common.copyManual` | Peringatan sekali tampil; raw key dibuang saat ditutup; buka ulang = form kosong |
| Dialog cabut | Tombol "Cabut key" `loading`, dialog tidak bisa ditutup | - | 5xx/jaringan di dialog (bisa coba lagi) | `ALREADY_REVOKED`/`KEY_NOT_FOUND` -> tutup + toast + muat ulang daftar. 404 project -> beranda |
| Tautan Audit Log | - | - | - | Hanya bila `role === 'super_admin'`; developer tidak melihat tautan |
| Global | `PendingView` saat sesi `unknown` (T01) | - | 401 -> `/login?next=...` (handler T01) | `unhandledrejection` ApiError -> toast (T00) |

---

## Kunci i18n

Ditambahkan ke `id.json` dan `en.json` (flat). Kunci yang sudah ada (`common.*`, `error.*`, `project.*` T03) dipakai ulang. `common.copy`, `common.copied`, `common.copyManual` hanya ditambah bila belum ada.

| Kunci | id | en |
|---|---|---|
| `common.copy` | Salin | Copy |
| `common.copied` | Tersalin | Copied |
| `common.copyManual` | Tidak bisa menyalin otomatis. Teks sudah diseleksi, tekan Ctrl/Cmd+C. | Could not copy automatically. The text is selected, press Ctrl/Cmd+C. |
| `project.tab.apiKeys` | API Keys | API Keys |
| `project.apikey.title` | API Keys | API Keys |
| `project.apikey.intro` | Key dipakai agent (mis. Claude Code) untuk membaca project ini lewat MCP. Key hanya bisa membaca dan tidak bisa dipakai untuk login. | Keys let agents (e.g. Claude Code) read this project over MCP. Keys are read-only and cannot be used to sign in. |
| `project.apikey.activeCount` | {n} key aktif | {n} active keys |
| `project.apikey.empty` | Belum ada API key | No API keys yet |
| `project.apikey.emptyHint` | Buat key untuk menghubungkan agent ke project ini. | Create a key to connect an agent to this project. |
| `project.apikey.col.name` | Nama | Name |
| `project.apikey.col.prefix` | Prefix | Prefix |
| `project.apikey.col.created` | Dibuat | Created |
| `project.apikey.col.lastUsed` | Terakhir dipakai | Last used |
| `project.apikey.col.status` | Status | Status |
| `project.apikey.col.actions` | Aksi | Actions |
| `project.apikey.createdBy` | {by} · {at} | {by} · {at} |
| `project.apikey.neverUsed` | Belum pernah dipakai | Never used |
| `project.apikey.unknownUser` | Tidak diketahui | Unknown |
| `project.apikey.status.active` | Aktif | Active |
| `project.apikey.status.revoked` | Dicabut | Revoked |
| `project.apikey.revokedBy` | Dicabut oleh {by} pada {at} | Revoked by {by} on {at} |
| `project.apikey.auditLink` | Audit Log | Audit log |
| `project.apikey.auditLinkAria` | Lihat Audit Log untuk key {name} | View audit log for key {name} |
| `project.apikey.create.open` | Buat key | Create key |
| `project.apikey.create.title` | Buat API key | Create API key |
| `project.apikey.create.name` | Nama key | Key name |
| `project.apikey.create.nameHint` | 2-60 karakter, unik di antara key aktif. Contoh: claude-code-budi | 2-60 characters, unique among active keys. Example: claude-code-budi |
| `project.apikey.create.submit` | Buat | Create |
| `project.apikey.validation.nameLength` | Nama key wajib 2-60 karakter | Key name must be 2-60 characters |
| `project.apikey.validation.nameControl` | Nama key tidak boleh mengandung karakter kontrol | Key name must not contain control characters |
| `project.apikey.result.title` | API key dibuat | API key created |
| `project.apikey.result.onceWarning` | Salin dan simpan key ini sekarang. Key tidak akan ditampilkan lagi. | Copy and store this key now. It will not be shown again. |
| `project.apikey.result.keyLabel` | API key | API key |
| `project.apikey.result.claudeCode` | Claude Code | Claude Code |
| `project.apikey.result.genericJson` | Konfigurasi JSON untuk klien MCP lain | JSON config for other MCP clients |
| `project.apikey.result.urlNote` | URL server MCP: {url} | MCP server URL: {url} |
| `project.apikey.result.done` | Saya sudah menyimpan key | I have stored the key |
| `project.apikey.revoke.open` | Cabut | Revoke |
| `project.apikey.revoke.aria` | Cabut key {name} | Revoke key {name} |
| `project.apikey.revoke.title` | Cabut API key? | Revoke API key? |
| `project.apikey.revoke.message` | Key "{name}" (prefix {prefix}) akan dicabut. | Key "{name}" (prefix {prefix}) will be revoked. |
| `project.apikey.revoke.effect` | Agent yang memakai key ini langsung ditolak. Key yang dicabut tidak bisa diaktifkan lagi. | Agents using this key are rejected immediately. A revoked key cannot be re-enabled. |
| `project.apikey.revoke.submit` | Cabut key | Revoke key |
| `project.apikey.revoke.success` | Key "{name}" dicabut | Key "{name}" revoked |
| `project.apikey.error.NAME_TAKEN` | Nama ini sudah dipakai key aktif lain di project ini | This name is already used by another active key in this project |
| `project.apikey.error.KEY_LIMIT_REACHED` | Batas {limit} key aktif per project tercapai. Cabut key yang tidak dipakai. | The limit of {limit} active keys per project has been reached. Revoke unused keys. |
| `project.apikey.error.ALREADY_REVOKED` | Key ini sudah dicabut. Daftar diperbarui. | This key was already revoked. The list has been refreshed. |
| `project.apikey.error.KEY_NOT_FOUND` | Key tidak ditemukan. Daftar diperbarui. | Key not found. The list has been refreshed. |
| `project.apikey.error.createUncertain` | Respons server tidak valid. Key mungkin sudah dibuat; periksa daftar dan cabut bila tidak dipakai. | The server response was invalid. The key may have been created; check the list and revoke it if unused. |

---

## Keputusan dan selisih dengan overview / plan usecase

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Folder FE `frontend/src/domain/project/api-key/` (bukan `apikey/` seperti tiket dan overview 8/15) | README: `domain/<domain>/<module>/<feature>` dengan module `api-key`; preseden `domain/core/audit-log/`. Nama package Go `apikey` tetap di backend |
| K2 | Store daftar (`createKeyListStore`) diekspor dari `listkeys` dan dibuat oleh `ApiKeysTab` di `app/`; create/revoke menerapkan hasil lewat `store.upsert`, muat ulang lewat `store.invalidate` | Feature tidak boleh saling import (rule 12.4). Store tanpa fetch; hanya `ListKeysView` yang memanggil `listKeys` |
| K3 | Urutan baris diurutkan ulang di klien dengan `sortKeys` (cermin `ORDER BY` backend) setelah `upsert` | Baris baru/dicabut langsung di posisi yang sama dengan hasil reload (E2E listkeys positif 3) |
| K4 | Schema `ApiKey` disalin lokal di tiap feature (`listkeys`, `createkey`, `revokekey`) | Pola T02/T03 K4; tipe kompatibel secara struktural, fixture JSON sama (overview 12) |
| K5 | Tab `api-keys` disisipkan sebelum `members`; default tab menjadi `api-keys` sampai T05 | Urutan final tiket T03 dan T03 FE K5 (default = entri pertama). Tidak menambah logika default khusus |
| K6 | URL MCP: `mcp_url` dipakai apa adanya; `null` -> `window.location.origin + '/mcp'` | Overview 12 dan K8. Origin diinjeksi ke `snippets.ts` agar teruji tanpa DOM |
| K7 | Nama key di potongan shell di-quote (`shellQuote`); JSON dibangun dengan `JSON.stringify` | Nama boleh berisi spasi/kutip (hanya karakter kontrol yang dilarang); mencegah potongan rusak atau injeksi saat ditempel |
| K8 | `Dialog` mendapat `dismissible` dan `size`; `Table` mendapat `rowClass` dan `rowAttrs`; komponen baru `CopyField` dan util `copyText` | Generik, tanpa pengetahuan domain (rule 12.5). Mencegah key hilang karena dialog tertutup saat request berjalan; baris dicabut redup |
| K9 | Tidak ada konfirmasi tambahan saat menutup layar hasil; tombol utama berlabel "Saya sudah menyimpan key" | Konsisten dengan `OneTimeSecret` T01; peringatan sudah menonjol |
| K10 | Tautan Audit Log memakai `project_id` + `action=apikey.created&action=apikey.revoked` + `target=<keyID>` | Param yang dipahami `filterFromQuery` T02; route `/audit-log` sudah dibatasi `super_admin` |
| K11 | Validasi nama di klien hanya UX; server tetap sumber kebenaran (pesan `fields.name` server ditampilkan bila lolos klien) | Rule 12.7 |

---

## Migration

Tidak ada. Frontend tidak punya migrasi. Migrasi backend T04 (`backend/migrations/project/<VERSION>_create_api_keys.sql`) dibuat di langkah F `overview.md`.

---

## Changes

`frontend/src/shared/` (generik):
- `components/Dialog.svelte` (ubah): props `dismissible`, `size`.
- `components/Table.svelte` (ubah): props `rowClass`, `rowAttrs`.
- `components/CopyField.svelte` (baru), `components/index.ts` (ubah: ekspor `CopyField`), `components/components.test.ts` (ubah).
- `utils/clipboard.ts`, `utils/clipboard.test.ts` (baru).
- `i18n/id.json`, `i18n/en.json` (ubah): kunci tabel "Kunci i18n".

`frontend/src/domain/project/api-key/` (baru):
- `listkeys/{types.ts, types.test.ts, index.ts, services/listkeys.service.ts, services/listkeys.service.test.ts, stores/keyList.store.ts, stores/keyList.store.test.ts, components/{KeysTable.svelte, KeyStatusBadge.svelte, format.ts, format.test.ts, components.test.ts}, views/ListKeysView.svelte, views/ListKeysView.test.ts, mocks/keys.ts}`.
- `createkey/{types.ts, types.test.ts, snippets.ts, snippets.test.ts, index.ts, services/createkey.service.ts, services/createkey.service.test.ts, components/{CreateKeyForm.svelte, KeyResult.svelte, components.test.ts}, views/CreateKeyDialog.svelte, views/CreateKeyDialog.test.ts, mocks/created.ts}`.
- `revokekey/{types.ts, index.ts, services/revokekey.service.ts, services/revokekey.service.test.ts, components/RevokeConfirm.svelte, components/RevokeConfirm.test.ts, views/RevokeKeyDialog.svelte, views/RevokeKeyDialog.test.ts, mocks/revoked.ts}`.

`frontend/src/app/`:
- `views/workspace/ApiKeysTab.svelte`, `views/workspace/ApiKeysTab.test.ts` (baru).
- `workspace/tabs.ts`, `workspace/tabs.test.ts` (ubah: entri `api-keys`).
- `views/ProjectWorkspaceView.test.ts` (ubah: kasus tab `api-keys`).

`frontend/e2e/`:
- `project-api-key.spec.ts` (baru).
- `helpers/apikeys.ts` (baru).

Tidak diubah: `shared/services/http.ts`, `shared/validation/error.ts` (`reasonOf`/`fieldErrorsOf` sudah ada), `shared/stores/*`, `app/routes/*` (route workspace dari T03), `app/App.svelte`, `app/layouts/*`, `.dependency-cruiser.cjs`, `scripts/test-arch.mjs`, `package.json`, `domain/core/**`, `domain/project/management/**`, `playwright.config.ts` (tanpa `PUBLIC_URL`, jadi E2E menguji jalur origin; jalur `mcp_url` diuji unit).

---

## Test Scenario

Vitest + Testing Library (jsdom). `fetch` di-stub per test (`vi.stubGlobal`); fixture JSON di `mocks/` mengikuti overview bagian 12. Test penerimaan memuat `T04-ACn` di judul `describe`/`it`.

### shared
- `Dialog`: `dismissible=false` -> event `cancel` di-`preventDefault` (dialog tetap terbuka), klik backdrop diabaikan, tombol X `disabled`; default tetap bisa ditutup (regresi T01/T02). `size='lg'` memberi kelas `max-w-2xl`.
- `Table`: `rowClass` diterapkan per baris; `rowAttrs` hanya menulis atribut `data-*`.
- `copyText`: `navigator.clipboard` tidak ada -> `false`; `writeText` reject -> `false`; sukses -> `true` dengan teks persis.
- `CopyField`: label terhubung (`getByLabelText`); klik salin -> `writeText(value)` dan teks tombol "Tersalin" lalu kembali setelah 2 detik (fake timers); gagal -> isi terseleksi (`selectionStart=0`, `selectionEnd=value.length`) dan pesan manual; `multiline` merender `textarea`.
- Paritas kunci i18n id/en hijau.

### listkeys
- `types.test.ts`: fixture list valid lolos; `status: 'disabled'` ditolak; `prefix: 'ABCDEFGH'` ditolak; `last_used_at: null` valid; `items` hilang ditolak. `sortKeys`: aktif sebelum dicabut, `created_at` turun, tie-break `id` naik; input tidak dimutasi. `displayPrefix('k3x9q2ma') === 'gr_k3x9q2ma_…'`.
- `listkeys.service.test.ts`: memanggil `GET /api/projects/<id>/api-keys` (id di-`encodeURIComponent`, `a/b` -> `a%2Fb`), tanpa header CSRF; respons rusak -> `ApiError INVALID_RESPONSE`; 404 envelope -> `ApiError` status 404 dengan pesan server.
- `keyList.store.test.ts`: `begin`/`resolve` -> `ready` terurut; `fail` 404 -> `notFound`; `fail` 500 -> `error` dengan `previous`; `resolve` untuk projectId basi diabaikan; `upsert` key baru -> masuk paling atas di antara aktif; `upsert` versi dicabut -> pindah ke bawah semua aktif; `invalidate` menaikkan `version`; `activeCount` menghitung hanya aktif.
- `components.test.ts`:
  - `T04-AC2` `KeysTable` tidak pernah merender string `gr_<prefix>_<secret>` walau fixture diberi field `raw_key` tambahan (DOM tidak memuat nilai itu).
  - Baris dicabut punya kelas `opacity-60`, `data-status="revoked"`, teks "Dicabut oleh a@tim.id pada …", tanpa tombol "Cabut".
  - Baris aktif punya tombol "Cabut" dengan `aria-label` "Cabut key ci-pipeline"; klik memanggil `onrevoke(key)`.
  - `last_used_at: null` -> "-" dengan `aria-label` "Belum pernah dipakai"; terisi -> `<time datetime>` ISO.
  - `created_by_label: null` -> "Tidak diketahui".
  - `auditHref` tidak diberikan -> tidak ada tautan "Audit Log"; diberikan -> tautan dengan `href` hasil fungsi di semua baris.
  - Nama `<img src=x onerror=alert(1)>` tampil literal (tidak ada elemen `img`).
  - Tidak ada tombol/teks "Hapus" di tabel.
- `ListKeysView.test.ts`: loading pertama -> `aria-busy="true"`; kosong -> `EmptyState` "Belum ada API key" + tombol "Buat key" memanggil `oncreate`; error 500 -> `ErrorState` dengan trace id, coba lagi memicu GET kedua; 404 -> `onprojectgone` dipanggil tepat sekali; `store.invalidate()` dari luar memicu GET ulang dan data lama tetap tampil selama loading; ganti `projectId` -> request lama di-abort.

### createkey
- `types.test.ts`: `validateName`: `'a'` -> length; `'ab'` -> ok; `'  claude  '` -> name `'claude'`; 60 emoji -> ok; 61 -> length; `'a\tb'` -> control; spasi saja -> length. `CreateKeyResponseSchema`: fixture valid lolos; `raw_key` 54 karakter atau berawalan `GR_` ditolak; `mcp_url: null` valid; `mcp_url: 'bukan url'` ditolak.
- `snippets.test.ts`:
  - `resolveMcpUrl('https://grimoire.tim.id/mcp', 'http://localhost:8080') === 'https://grimoire.tim.id/mcp'`; `resolveMcpUrl(null, 'http://localhost:8080') === 'http://localhost:8080/mcp'`; origin dengan trailing slash tidak menggandakan `/`.
  - `claudeCodeSnippet('claude-code-budi', 'https://g.tim.id/mcp', K) === 'claude mcp add --transport http claude-code-budi https://g.tim.id/mcp --header "Authorization: Bearer ' + K + '"'`.
  - Nama `budi's key` -> `'budi'\''s key'`; nama `a;rm -rf ~` ter-quote utuh.
  - `genericJsonSnippet`: `JSON.parse` hasilnya `{ mcpServers: { [name]: { type: 'http', url, headers: { Authorization: 'Bearer ' + K } } } }`; nama berisi `"` tetap JSON valid.
- `createkey.service.test.ts`: `POST /api/projects/<id>/api-keys` dengan body `{"name":"claude-code-budi"}` dan header `X-Requested-With: grimoire`; 201 fixture -> objek tervalidasi; 409 `NAME_TAKEN` -> `ApiError` dengan `reasonOf(details) === 'NAME_TAKEN'`.
- `components.test.ts`: `CreateKeyForm` tombol `disabled` saat `submitting`; Enter men-submit; `fieldError` memberi `aria-invalid`. `KeyResult` menampilkan peringatan `role="alert"`, raw key di `CopyField` "API key", potongan Claude Code dan JSON berisi raw key dan URL.
- `CreateKeyDialog.test.ts`:
  - `T04-AC1` sukses: isi nama -> submit -> layar hasil memuat raw key, potongan dengan `mcp_url` respons; `oncreated` dipanggil dengan `key` (tanpa `raw_key`).
  - `mcp_url: null` -> potongan memakai `window.location.origin + '/mcp'`.
  - Validasi klien: nama 1 karakter -> pesan "Nama key wajib 2-60 karakter", `fetch` tidak dipanggil.
  - `T04-AC4` 409 `NAME_TAKEN` -> pesan field i18n, input tetap berisi nama, dialog tetap di form.
  - `T04-AC5` 409 `KEY_LIMIT_REACHED` `limit: 20` -> `formError` memuat "20".
  - 400 `fields.name` dari server -> pesan server di field.
  - 404 -> `onprojectgone`, dialog tertutup.
  - Respons 201 rusak -> `createUncertain` + `onstale` dipanggil.
  - 500 -> `formError` dengan trace id.
  - Submit ganda cepat -> `fetch` dipanggil sekali.
  - Selama request berjalan dialog tidak bisa ditutup (Esc/X tidak memanggil `onclose`).
  - `T04-AC2` setelah "Saya sudah menyimpan key": `onclose` dipanggil, DOM tidak lagi memuat raw key; buka ulang -> form kosong, tidak ada raw key.
  - Raw key tidak pernah dikirim ke `toast` maupun `console` (spy).

### revokekey
- `revokekey.service.test.ts`: `POST /api/projects/<id>/api-keys/<keyId>/revoke` dengan CSRF dan tanpa body; id di-encode; 200 -> `RevokedKey` dengan `status: 'revoked'`; respons `status: 'active'` tetap lolos schema (validasi bentuk, bukan nilai).
- `RevokeConfirm.test.ts`: menyebut nama dan prefix; nama berisi HTML tampil literal.
- `RevokeKeyDialog.test.ts`:
  - Sukses -> `onrevoked(key)`, toast sukses, dialog tertutup.
  - 409 `ALREADY_REVOKED` -> toast i18n + `onstale`.
  - 404 `KEY_NOT_FOUND` -> toast i18n + `onstale`.
  - 404 tanpa reason -> `onprojectgone`.
  - 500 -> pesan di dialog, dialog tetap terbuka, tombol bisa ditekan lagi.
  - "Batal" tidak memanggil `fetch`.
  - Selama submitting dialog tidak bisa ditutup.

### app
- `tabs.test.ts`: urutan `['api-keys', 'members', 'settings']`; key unik; `findTab('api-keys')` mengembalikan `ApiKeysTab`.
- `ApiKeysTab.test.ts` (fetch di-stub, `ProjectContext` palsu):
  - Developer -> tidak ada tautan "Audit Log"; Super Admin -> tautan `href` = `/audit-log?project_id=<pid>&action=apikey.created&action=apikey.revoked&target=<keyId>`.
  - Buat key -> tutup dialog hasil -> baris baru tampil di atas tanpa GET kedua.
  - Cabut -> baris pindah ke bawah, redup, tanpa GET kedua.
  - Cabut dengan 409 -> GET daftar dipanggil ulang.
  - 404 dari GET daftar -> `onprojectgone` diteruskan.
- `ProjectWorkspaceView.test.ts`: `/projects/<id>/api-keys` merender judul "API Keys" dan `aria-current="page"` pada tautan tab "API Keys".
- `npm run depcruise`: tidak ada import `app/` dari `domain/`, tidak ada import silang antar feature `api-key/*`, `app/` hanya meng-import `index.ts` feature.

---

## E2E Scenarios

Playwright terhadap binary (`frontend/e2e/project-api-key.spec.ts`). Fixture per test: developer A (owner P1), B (member P1, bukan owner), C (developer bukan anggota), Super Admin `e2e-admin@tim.id`; P1 dan P2 dibuat lewat `helpers/projects.ts` (T03). `PUBLIC_URL` tidak diset di `webServer`, jadi URL MCP = origin browser. Konteks diberi izin clipboard.

Positif:
1. **T04-AC1 member membuat key**: B login -> `/projects/<P1>/api-keys` -> keadaan kosong "Belum ada API key" -> "Buat key" -> isi `claude-code-budi` -> "Buat" -> layar hasil: peringatan "tidak akan ditampilkan lagi", input "API key" berisi nilai cocok `^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$`; klik "Salin" -> "Tersalin" dan `navigator.clipboard.readText()` sama dengan key; potongan Claude Code = `claude mcp add --transport http claude-code-budi <origin>/mcp --header "Authorization: Bearer <key>"`; JSON generik ter-parse dengan `url = <origin>/mcp`.
2. **T04-AC2 sekali tampil**: klik "Saya sudah menyimpan key" -> dialog tertutup; tabel memuat baris `claude-code-budi`, prefix `gr_<prefix>_…`, dibuat oleh B (email), terakhir dipakai "-", status "Aktif"; `page.content()` dan URL tidak memuat key; reload -> key tetap tidak ada; respons jaringan `GET /api/projects/<P1>/api-keys` (ditangkap `page.waitForResponse`) tidak memuat `raw_key` maupun string key.
3. **Key tidak terikat pembuat**: A (owner) membuat key `ci-pipeline` lewat API; B me-reload tab -> melihat dua key, `ci-pipeline` di atas (terbaru), dibuat oleh A.
4. **Cabut**: B menekan "Cabut" pada `claude-code-budi` -> dialog menyebut nama dan prefix -> "Cabut key" -> toast sukses; baris redup, status "Dicabut", "Dicabut oleh <B> pada …", tanpa tombol "Cabut", pindah ke bawah key aktif; reload -> tetap sama; tidak ada tombol "Hapus".
5. **T04-AC4 nama dipakai ulang**: setelah langkah 4, buat key `claude-code-budi` lagi -> sukses; tabel memuat dua baris bernama sama (satu aktif, satu dicabut).
6. **Anggota lain mencabut key rekannya**: B membuat key `b-tool` lewat API; A (anggota, bukan pembuat) mencabutnya lewat UI -> sukses, "Dicabut oleh <A>".
7. **Super Admin**: admin (bukan anggota) membuka `/projects/<P1>/api-keys` -> daftar sama; membuat dan mencabut key sukses; tautan "Audit Log" tampil pada baris -> klik -> `/audit-log` menampilkan entri `apikey.created` dan `apikey.revoked` dengan nama + prefix, tanpa key lengkap.
8. **Developer tidak melihat tautan Audit Log**: B di tab yang sama -> tidak ada tautan "Audit Log".
9. **Isolasi project**: P2 punya key `p2-key`; tab API Keys P1 tidak memuatnya, dan sebaliknya.
10. **Nav tab**: dari `/projects/<P1>/members`, klik tab "API Keys" -> URL `/projects/<P1>/api-keys`, `aria-current="page"` pada tab itu; Back kembali ke Members.

Negatif:
1. **T04-AC1 bukan anggota**: C membuka `/projects/<P1>/api-keys` -> "Project tidak ditemukan" (seragam T03); `GET` dan `POST /api/projects/<P1>/api-keys` lewat `request` C -> 404 `project tidak ditemukan`.
2. **Id bukan uuid**: `/projects/bukan-uuid/api-keys` -> "Project tidak ditemukan"; API 404.
3. **Validasi nama**: nama `a` -> pesan "Nama key wajib 2-60 karakter" tanpa request (`page.on('request')` tidak mencatat POST); nama 61 karakter lewat API -> 400 `fields.name`.
4. **NAME_TAKEN**: buat key dengan nama key aktif yang ada -> pesan "Nama ini sudah dipakai key aktif lain di project ini"; dialog tetap terbuka, input terjaga.
5. **T04-AC5 batas**: 20 key aktif dibuat lewat API (default `API_KEY_MAX_ACTIVE_PER_PROJECT=20`) -> buat key ke-21 lewat UI -> pesan memuat "20"; cabut satu lewat UI -> buat lagi sukses.
6. **ALREADY_REVOKED (tab basi)**: dua page B membuka tab yang sama; page 1 mencabut key; page 2 menekan "Cabut" pada key itu -> toast "Key ini sudah dicabut. Daftar diperbarui." dan baris tampil dicabut.
7. **Akses dicabut saat terbuka**: A mengeluarkan B lewat API; B menekan "Buat key" -> submit -> diarahkan ke beranda (404 -> `onprojectgone`), tanpa error mentah.
8. **Submit ganda**: double-click "Buat" -> tepat satu `POST` tercatat; satu baris baru.
9. **CSRF**: `POST /api/projects/<P1>/api-keys` dan `/revoke` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; key tetap aktif.
10. **Tanpa login**: `GET /api/projects/<P1>/api-keys` -> 401; membuka `/projects/<P1>/api-keys` tanpa sesi -> `/login?next=%2Fprojects%2F<P1>%2Fapi-keys`, setelah login kembali ke tab API Keys.
11. **Revoke lintas project**: `POST /api/projects/<P1>/api-keys/<keyID P2>/revoke` -> 404 `KEY_NOT_FOUND`; tab P2 tetap menampilkan key aktif.

---

## Request/Response body

Format error seragam T00 (`api/modules/infra/foundation.yml#/components/schemas/Error`), divalidasi FE oleh `ErrorEnvelope` di `shared/validation/error.ts`:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Header mutasi (otomatis dari `http.ts`): `X-Requested-With: grimoire`; `Content-Type: application/json` hanya bila ada body. Cookie sesi `same-origin`.

### GET /api/projects/{projectID}/api-keys (`listKeys`)

Tanpa body, tanpa query. 200:
```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000003",
      "name": "ci-pipeline",
      "prefix": "a1b2c3d4",
      "status": "active",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-23T10:00:00Z",
      "last_used_at": "2026-10-23T11:42:00Z",
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
      "name": "claude-code-budi",
      "prefix": "k3x9q2ma",
      "status": "revoked",
      "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "created_by_label": "b@tim.id",
      "created_at": "2026-10-23T09:00:00Z",
      "last_used_at": null,
      "revoked_at": "2026-10-23T12:00:00Z",
      "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "revoked_by_label": "a@tim.id"
    }
  ]
}
```
Kosong: `{ "items": [] }` -> `EmptyState`.

404 (bukan anggota, project tidak ada, id bukan uuid) -> `onprojectgone`:
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

401 (sesi habis) -> handler global ke `/login?next=...`:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects/{projectID}/api-keys (`createKey`)

Request:
```json
{ "name": "claude-code-budi" }
```

201 (header `Cache-Control: no-store`):
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
    "name": "claude-code-budi",
    "prefix": "k3x9q2ma",
    "status": "active",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "b@tim.id",
    "created_at": "2026-10-23T09:00:00Z",
    "last_used_at": null,
    "revoked_at": null,
    "revoked_by": null,
    "revoked_by_label": null
  },
  "raw_key": "gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY",
  "mcp_url": "https://grimoire.tim.id/mcp"
}
```
`mcp_url: null` bila `PUBLIC_URL` tidak diset -> FE memakai `window.location.origin + "/mcp"`.

Potongan yang dihasilkan FE dari contoh di atas:
```sh
claude mcp add --transport http claude-code-budi https://grimoire.tim.id/mcp --header "Authorization: Bearer gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY"
```
```json
{
  "mcpServers": {
    "claude-code-budi": {
      "type": "http",
      "url": "https://grimoire.tim.id/mcp",
      "headers": {
        "Authorization": "Bearer gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY"
      }
    }
  }
}
```

400 validasi nama (-> pesan di field `name`):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key tidak boleh mengandung karakter kontrol" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 nama dipakai key aktif (-> `project.apikey.error.NAME_TAKEN` di field):
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 batas key aktif (-> `project.apikey.error.KEY_LIMIT_REACHED` dengan `{limit}`):
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (-> `onprojectgone`):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 tanpa header CSRF (FE selalu mengirim; hanya muncul lewat API langsung):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects/{projectID}/api-keys/{keyID}/revoke (`revokeKey`)

Request: tanpa body.

200:
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
    "name": "claude-code-budi",
    "prefix": "k3x9q2ma",
    "status": "revoked",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "b@tim.id",
    "created_at": "2026-10-23T09:00:00Z",
    "last_used_at": "2026-10-23T11:00:00Z",
    "revoked_at": "2026-10-23T12:00:00Z",
    "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "revoked_by_label": "a@tim.id"
  }
}
```

404 key tidak ada / bukan uuid / milik project lain (-> toast `KEY_NOT_FOUND` + muat ulang):
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 project (-> `onprojectgone`):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 sudah dicabut (-> toast `ALREADY_REVOKED` + muat ulang):
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Error klien yang dibuat `http.ts` (bukan dari server): `NETWORK` (status 0, `error.network`), `INVALID_RESPONSE` (respons tidak cocok schema Zod, `details` = issue Zod), `INTERNAL` (body error bukan envelope).
