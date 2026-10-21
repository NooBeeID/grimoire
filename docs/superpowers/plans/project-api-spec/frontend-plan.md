# T06 project/api-spec: Frontend Plan (Svelte)

Tiket: `docs/tickets/T06-project-api-spec.md` (bagian UI). Plan bersama: `overview.md` (D1-D8, 4.3 status/reason, 4.6 DTO, 14 E2E, 15 body). Kontrak per endpoint: `usecase-uploadspecs.md`, `usecase-validatespec.md`, `usecase-listspecs.md`, `usecase-getspec.md`, `usecase-downloadspec.md`, `usecase-renamespec.md`, `usecase-deletespec.md`. `listspecsbus`, `listendpoints`, `getendpoint`, `countspecs` adalah handler bus tanpa UI. Konvensi: `docs/tickets/README.md` bagian "Frontend (Svelte)" dan `rules.txt` bagian 12 (rules menang soal arsitektur).

Dokumen ini adalah **satu-satunya plan frontend** untuk T06; frontend tidak dipecah per usecase. Bila catatan FE di overview/plan usecase (mis. `usecase-listspecs.md` FE-1/FE-2: path `apispec/listspecs/services/listSpecs.ts`) berbeda dengan dokumen ini, **ikuti dokumen ini**. Selisihnya dicatat di "Keputusan".

Konteks yang dimuat: tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: tiket T06, `docs/tickets/README.md`, `rules.txt` (7.9, 12, 13), `docs/superpowers/specs/2026-10-20-grimoire-design.md`, `overview.md` + 11 plan usecase T06, `docs/superpowers/plans/project-management/frontend-plan.md` (workspace `/projects/:projectID/:tab`, registri tab `app/workspace/tabs.ts`, `WorkspaceTabProps`, `ProjectContext`, `reasonOf`, `shared/utils/text.ts`), `docs/superpowers/plans/project-api-key/frontend-plan.md` (K1 nama folder module, `Dialog` `dismissible`/`size`, `copyText`, `CopyField`), `docs/tickets/T05-project-knowledge.md` (sanitasi Markdown, "Upload campuran" dikomposisi di `app/`), dan kode FE di `main`: `shared/services/http.ts` (`get/post/patch/del`, `request`, `ApiError`, CSRF otomatis untuk mutasi, `FormData` dikirim apa adanya, handler 401 global), `shared/validation/error.ts` (`ErrorEnvelope`, `KnownErrorCode`, `fieldErrorsOf`, `reasonOf`), `shared/stores/{session,i18n,toast}.ts` (`t(key, params)`), `shared/utils/datetime.ts` (`formatDateTime(iso, locale)`), `shared/components/{Button,Input,Dialog,Table,Spinner,EmptyState,ErrorState,Toast}`, `app/routes/{index,router,guard}.ts` (pola `:param` per segmen, jumlah segmen harus sama), `.dependency-cruiser.cjs`, `e2e/helpers/auth.ts`, `playwright.config.ts` (binary `../bin/grimoire`, env default).

---

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (dokumen ini: frontend saja; backend di `overview.md` dan `usecase-*.md`) |
| Domain | project |
| Module | api-spec (folder FE `frontend/src/domain/project/api-spec/`, lihat K1; package Go tetap `apispec`) |
| Feature FE | `listspecs`, `uploadspecs` (upload + validasi tanpa menyimpan, satu dialog; K2), `getspec` (halaman detail + renderer dokumentasi), `downloadspec`, `renamespec`, `deletespec` |
| Route UI | `/projects/:projectID/api-specs` (tab daftar; route `/projects/:projectID/:tab` dari T03) dan `/projects/:projectID/api-specs/:name` (detail; route baru generik `/projects/:projectID/:tab/:item`, K4). Query detail: `?op=<METHOD> <path>` (operasi terpilih) |
| Akses UI | Route `meta: { auth: true, layout: 'app' }`. Semua level akses project (`member`/`owner`/`admin`) dan super_admin boleh semua aksi T06; tidak ada tombol yang disembunyikan per peran (otorisasi tetap di backend, rule 12.7) |
| Kontrak API | `api/modules/project/apispec.yml` (`info.version: 0.1.0`), schema `ApiSpec`, `ApiSpecList`, `ApiSpecDetail`, `ApiSpecEndpoint`, `ApiSpecIssue`, `ApiSpecFileResult`, `ApiSpecUploadResult`, `ApiSpecValidateResult`, `RenameApiSpecRequest`; `Error` dari `api/modules/infra/foundation.yml` |
| Prasyarat | T00, T01 FE, T02 FE (merged). **T03 FE** (workspace, `ProjectContextView`, `app/workspace/tabs.ts`, `reasonOf`, `shared/utils/text.ts`) wajib sudah di `main`. Backend T06 F + U1-U7 |

Endpoint yang dipanggil frontend (prefix `/api`, tanpa versi di path; versi kontrak `0.1.0`; mutasi otomatis membawa `X-Requested-With: grimoire` dari `shared/services/http.ts`):

| Endpoint | Method | Version | Dipakai oleh (service FE) | Akses (backend) |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-specs` | GET | 0.1.0 | `listspecs/services/listspecs.service.ts` (`listSpecs`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/upload` | POST (multipart) | 0.1.0 | `uploadspecs/services/uploadspecs.service.ts` (`uploadSpecs`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/validate` | POST (multipart) | 0.1.0 | `uploadspecs/services/uploadspecs.service.ts` (`validateSpecs`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/{name}` | GET | 0.1.0 | `getspec/services/getspec.service.ts` (`getSpec`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/{name}/raw` | GET | 0.1.0 | `downloadspec/services/downloadspec.service.ts` (`downloadSpec`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/{name}` | PATCH | 0.1.0 | `renamespec/services/renamespec.service.ts` (`renameSpec`) | anggota, super_admin |
| `/api/projects/{projectID}/api-specs/{name}` | DELETE | 0.1.0 | `deletespec/services/deletespec.service.ts` (`deleteSpec`) | anggota, super_admin |

Bus `project.apispec.list-specs|list-endpoints|get-endpoint|count-specs` dan bus yang dikonsumsi backend (`project.management.check-access`, `core.audit.record`) tidak disentuh frontend. Frontend tidak menambah endpoint. Jumlah spec per project di beranda (`spec_count`) sudah ditangani T03 FE.

---

## Summary Plan

Frontend T06 menambah tab **API Specs** ke workspace project dengan dua permukaan:

1. **Daftar spec** (`/projects/:projectID/api-specs`, feature `listspecs`): tabel/kartu responsif per spec: nama (tautan ke detail), judul, versi OpenAPI (lencana `3.0`/`3.1`), versi API, jumlah endpoint, `source_path` (bila ada), diperbarui (waktu + `updated_by_label`). Header menampilkan "x / limit spec" dari respons (`total`, `limit`) dan tombol **Unggah spec**. Urutan mengikuti backend (nama).
2. **Dialog unggah** (feature `uploadspecs`): pilih file (`multiple`, `.yaml,.yml,.json`) atau folder (`webkitdirectory`). File dikirim dengan path relatif (`FormData.append('files', file, file.webkitRelativePath || file.name)`). Field opsional **nama** aktif hanya bila tepat satu file spec dipilih. Dua aksi: **Validasi saja** (`/validate`, tanpa menyimpan) dan **Unggah** (`/upload`). Hasil ditampilkan per file: status (dibuat/diperbarui/tidak berubah/dilewati/ditolak/valid), nama spec, versi, jumlah endpoint, alasan (`reason_code` -> label i18n, `reason` + `issues[]` dengan baris/pointer sebagai teks yang **bisa disalin**), plus ringkasan jumlah. Setelah validasi, baris valid yang namanya sudah ada di daftar ditandai "akan menimpa".
3. **Halaman detail spec** (`/projects/:projectID/api-specs/:name`, feature `getspec` + aksi `downloadspec`, `renamespec`, `deletespec`): header metadata + tombol **Unduh**, **Ubah nama**, **Hapus**; panel samping berisi pencarian endpoint dan pohon **tag -> endpoint** (lencana method, path, ringkasan, coret bila deprecated); panel utama menampilkan dokumentasi terender: ringkasan API (info, deskripsi Markdown, servers, skema keamanan, external docs, webhooks 3.1) bila belum ada operasi terpilih, atau satu operasi (parameter, request body per media type, response per kode, skema yang dapat dilipat, contoh, security, servers efektif, status deprecated). Tanpa tombol eksekusi request.

Renderer dokumentasi ditulis sendiri sebagai komponen Svelte (K5), di atas modul model murni (`getspec/model/*`) yang menelusuri `$ref` internal secara malas dengan penanda sirkular, menormalkan tampilan tipe 3.0 (`nullable`) dan 3.1 (`type: [.., "null"]`), dan menggabungkan parameter level path. Deskripsi Markdown dirender lewat komponen bersama tersanitasi (`shared/components/Markdown.svelte`, K6): HTML mentah tidak dirender, tautan hanya `http:`/`https:`/`mailto:`/anchor `#`, gambar tidak dimuat.

Komposisi lintas module (tab, route detail, navigasi setelah rename/delete, kembali ke beranda saat project hilang) ada di `app/` (rule 12.4). Feature tidak meng-import router atau feature lain; komunikasi lewat props dan callback. Store daftar dibuat per instance tab dan dibagikan agar hasil upload/rename/delete langsung tercermin.

Keamanan dan ketahanan:
- Isi spec tidak tepercaya (rule 7.9, AC8): semua nilai dirender sebagai teks; satu-satunya `{@html}` ada di `Markdown.svelte` setelah sanitasi; contoh/skema ditampilkan sebagai JSON teks dalam `<pre>`; URL dari spec (`servers[].url`, `externalDocs.url`, `contact.url`, `license.url`) hanya menjadi tautan bila `http(s):`, selain itu teks.
- Penelusuran `$ref` di FE hanya pointer internal `#/...`, tanpa fetch apa pun; ref yang tidak bisa diselesaikan tampil sebagai penanda, bukan crash.
- Renderer tahan terhadap spec besar (500 endpoint, 5 MB): hanya satu operasi dirender di panel utama, skema dibuka malas (klik), kedalaman tampilan default 1 level.
- 404 dari daftar/upload -> `onprojectgone` (beranda, pola T03). 404 detail -> state "spec tidak ditemukan" + tautan ke daftar. 404 saat rename/delete/unduh -> toast + kembali ke daftar.
- Error bisnis dari `details.reason` -> `apispec.error.<REASON>`; error field `details.fields.name` -> input nama; lainnya pesan server + trace id lewat `ErrorState`/`formError`.
- Respons tidak sesuai schema -> `ApiError` `INVALID_RESPONSE`. Khusus upload: pesan "unggahan mungkin sudah tersimpan" + muat ulang daftar.

---

## Plan

Urutan wajib: FE-0 (prasyarat bersama) -> feature S1 `listspecs`, S2 `uploadspecs`, S3 `getspec`, S4 `downloadspec`, S5 `renamespec`, S6 `deletespec` boleh paralel (tidak saling import) -> komposisi `app/` (A1-A5) -> e2e (E) -> verifikasi (V). Semua langkah TDD: test vitest ditulis lebih dulu.

### FE-0. Prasyarat bersama (generik, tanpa pengetahuan domain)

1. **Cek prasyarat T03 FE**: `app/workspace/tabs.ts` (`WorkspaceTab`, `WorkspaceTabProps`, `findTab`, `defaultTabKey`), `app/views/ProjectWorkspaceView.svelte`, `domain/project/management/getproject/index.ts` (ekspor `ProjectContext`), `shared/validation/error.ts` `reasonOf`. Bila belum ada, berhenti dan laporkan (T06 bergantung pada T03; jangan membuat ulang workspace T03 di PR ini).
2. **Prasyarat T04 FE yang dipakai ulang** (aturan "yang merge lebih dulu membuat"; bila T04 FE belum merge, buat PERSIS seperti `project-api-key/frontend-plan.md` FE-0.2 dan FE-0.4):
   - `shared/components/Dialog.svelte` props `dismissible?: boolean = true` dan `size?: 'md' | 'lg' = 'md'`.
   - `shared/utils/clipboard.ts` `copyText(text): Promise<boolean>`.
3. **`shared/services/http.ts`** (ubah, kompatibel mundur, generik): tambah
   ```ts
   export interface BlobResult { blob: Blob; contentType: string | null }
   // GET non-JSON (unduhan). Sukses -> Blob. Gagal -> ApiError dengan parsing ErrorEnvelope dan handler 401 yang sama dengan request().
   export async function getBlob(url: string, o?: { signal?: AbortSignal; skipAuthRedirect?: boolean }): Promise<BlobResult>;
   ```
   Implementasi berbagi `buildUrl`, penanganan jaringan, envelope error, dan 401 dengan `request` (ekstrak helper privat `toApiError(res, requestId)`; perilaku `request` tidak berubah). Header `Accept: */*`.
   Plus `shared/utils/download.ts` (baru): `saveBlob(blob: Blob, fileName: string): void` (object URL + `<a download>` sementara + `URL.revokeObjectURL` pada tick berikutnya). Nama file disaring `[^A-Za-z0-9._-]` -> `_`.
4. **Markdown tersanitasi** (aturan "yang merge lebih dulu membuat"; T05 FE juga membutuhkannya. Bila T05 FE sudah menambahkannya, pakai yang ada dan hanya pastikan opsi di bawah tersedia):
   - Dependensi baru (versi exact, rule 15.4, alasan di PR): `markdown-it` (parser CommonMark, `html: false` sehingga HTML mentah di-escape, `validateLink` bawaan menolak `javascript:`/`vbscript:`/`file:`/`data:`) dan `dompurify` (lapisan kedua). `@types/markdown-it` di devDependencies. Pin ke versi stabil terbaru saat implementasi (mis. `markdown-it@14.1.0`, `dompurify@3.2.6`), dicatat di `package.json` tanpa `^`.
   - `shared/utils/markdown.ts` (baru): `renderMarkdown(src: string, opts?: { allowImages?: boolean }): string`. `MarkdownIt({ html: false, linkify: false, typographer: false })`; hasil lewat `DOMPurify.sanitize` dengan `ALLOWED_TAGS` (p, br, strong, em, code, pre, blockquote, ul, ol, li, a, h1-h6, table, thead, tbody, tr, th, td, hr, del) dan `ALLOWED_ATTR` (`href`, `title`); hook `afterSanitizeAttributes`: `href` hanya `http:`, `https:`, `mailto:` atau diawali `#` (lainnya atribut dihapus), tautan eksternal diberi `rel="noopener noreferrer nofollow"` dan `target="_blank"`. `allowImages` default `false` (gambar diganti teks alt; mencegah request ke host luar dari isi spec). Input > 200 000 karakter dipotong + penanda.
   - `shared/components/Markdown.svelte` (baru, ekspor di `components/index.ts`): props `source: string | null | undefined`, `inline?: boolean = false`, `class?: string`. Render `{@html renderMarkdown(source)}` di `<div class="prose-sm">` (`inline` -> `<span>`, hanya paragraf pertama). Satu-satunya pemakaian `{@html}` yang diizinkan di T06.
5. **`shared/utils/url.ts`** (baru, generik): `safeHref(raw: unknown): string | null` (string, `new URL(raw)` berhasil, protokol `http:`/`https:` -> href; lainnya `null`). Dipakai renderer untuk `servers`, `externalDocs`, `contact`, `license`.
6. **i18n** `shared/i18n/id.json` dan `en.json`: kunci di tabel "Kunci i18n" (paritas dijaga `i18n-keys.test.ts`). Pakai ulang `common.*`, `error.*`, `project.*` yang ada.

### S1. Feature `listspecs` (`frontend/src/domain/project/api-spec/listspecs/`)

**`types.ts`**
```ts
export const ApiSpec = z.object({
  name: z.string().min(1), title: z.string(), openapi_version: z.string(), api_version: z.string(),
  format: z.enum(['yaml', 'json']), source_path: z.string().nullable(), size_bytes: z.number().int().nonnegative(),
  endpoint_count: z.number().int().nonnegative(), created_at: z.string().datetime({ offset: true }),
  created_by_label: z.string(), updated_at: z.string().datetime({ offset: true }), updated_by_label: z.string(),
});
export const ApiSpecList = z.object({ items: z.array(ApiSpec), total: z.number().int().nonnegative(), limit: z.number().int().positive() });
export type ApiSpec = z.infer<typeof ApiSpec>; export type ApiSpecList = z.infer<typeof ApiSpecList>;
export function sortSpecs(xs: ApiSpec[]): ApiSpec[];          // salinan terurut name (cermin ORDER BY name, perbandingan byte/kode unit)
export function openapiMajorMinor(v: string): '3.0' | '3.1' | string; // "3.1.0" -> "3.1"
export function formatBytes(n: number, locale: Locale): string;      // 18234 -> "17,8 KB"
```

**`services/listspecs.service.ts`**: `listSpecs(projectId, signal?) => get('/api/projects/' + encodeURIComponent(projectId) + '/api-specs', { schema: ApiSpecList, signal })`. Tanpa state.

**`stores/specList.store.ts`**: `createSpecListStore()` (per instance, bukan singleton) dengan state `{ status: 'idle'|'loading'|'ready'|'error', items: ApiSpec[], total, limit, error: unknown|null, version: number }` dan aksi tanpa fetch: `start()`, `set(list)`, `fail(err)`, `upsert(spec)` (ganti berdasarkan `name`, lalu `sortSpecs`), `rename(oldName, spec)`, `remove(name)`, `invalidate()` (menaikkan `version` agar view memuat ulang), `names(): Set<string>`. `total` diperbarui saat upsert/remove.

**`components/`**
- `SpecTable.svelte`: props `items`, `projectId`, `loading`. Kolom: nama (`<a href="/projects/{id}/api-specs/{name}">`, monospace) + judul di bawahnya, `OpenApiBadge`, versi API, endpoint, `source_path` (dipotong tengah, `title` berisi path utuh), diperbarui (`formatDateTime` + `updated_by_label`). Di layar < `md` berubah menjadi kartu (CSS grid, tanpa scroll horizontal halaman).
- `OpenApiBadge.svelte`: `3.0` / `3.1` dengan `aria-label` `apispec.openapiVersion`.
- `SpecQuota.svelte`: "x / limit spec"; `total >= limit` -> teks peringatan `apispec.list.limitReached` (upload tetap bisa untuk menimpa).

**`views/ListSpecsView.svelte`**: props `projectId`, `store`, `onupload()`, `onprojectgone()`. `$effect` pada `projectId` + `$store.version`: `AbortController`, `store.start()`, `listSpecs` -> `set` / `fail`; hasil basi diabaikan (seq). 404 -> `onprojectgone()`. Render header (judul, `SpecQuota`, tombol "Unggah spec") + `SpecTable` / `EmptyState` / `ErrorState`.

**`index.ts`**: ekspor `ListSpecsView`, `createSpecListStore`, `type SpecListStore`, `type ApiSpec`, `ApiSpec` (schema, dipakai ulang oleh komposisi untuk adaptasi tipe).

### S2. Feature `uploadspecs` (`frontend/src/domain/project/api-spec/uploadspecs/`)

**`types.ts`**
```ts
export const FileStatus = z.enum(['created', 'updated', 'unchanged', 'skipped', 'rejected', 'valid']);
export const ApiSpecIssue = z.object({ code: z.string(), message: z.string(), pointer: z.string().nullable(), line: z.number().int().positive().nullable() });
export const ApiSpecFileResult = z.object({
  file: z.string(), source_path: z.string().nullable(), name: z.string().nullable(), status: FileStatus,
  reason_code: z.string().nullable(), reason: z.string().nullable(), issues: z.array(ApiSpecIssue),
  openapi_version: z.string().nullable(), title: z.string().nullable(), api_version: z.string().nullable(),
  endpoint_count: z.number().int().nonnegative().nullable(),
});
export const ApiSpecUploadResult = z.object({ results: z.array(ApiSpecFileResult),
  summary: z.object({ created: z.number().int(), updated: z.number().int(), unchanged: z.number().int(), skipped: z.number().int(), rejected: z.number().int() }) });
export const ApiSpecValidateResult = z.object({ results: z.array(ApiSpecFileResult),
  summary: z.object({ valid: z.number().int(), skipped: z.number().int(), rejected: z.number().int() }) });
export interface PickedFile { file: File; path: string }       // path = webkitRelativePath || name
export const SPEC_EXTENSIONS = ['.yaml', '.yml', '.json'];
export const NAME_RE = /^[a-z0-9._-]{1,80}$/;
```

**`model.ts`** (murni, diuji tanpa DOM):
- `partitionPicked(files: PickedFile[]): { spec: PickedFile[]; local: FileResult[] }`: file dengan ekstensi selain `SPEC_EXTENSIONS` (case-insensitive) tidak dikirim dan menjadi hasil lokal `{status: 'skipped', reason_code: 'NOT_SPEC_FILE', reason: null, issues: [], ...}` dengan penanda `local: true` (hanya UX: mengurangi bandwidth/batas `MAX_UPLOAD_FILES`; backend tetap otoritatif untuk file yang dikirim). Path `node_modules/**` dan segmen tersembunyi (`.git/**`, `.x/**`) TIDAK difilter (perilaku sama dengan backend).
- `validateNameInput(name: string, specCount: number): 'ok' | 'INVALID_NAME' | 'NAME_WITH_MULTIPLE_FILES'`: kosong -> ok (pakai nama turunan); `specCount !== 1` -> `NAME_WITH_MULTIPLE_FILES`; tidak cocok `NAME_RE` atau `.`/`..` -> `INVALID_NAME`.
- `mergeResults(server: FileResult[], local: FileResult[]): FileResult[]`: hasil server dulu (urutan part), lalu lokal; ringkasan dihitung ulang dengan `summarize`.
- `summarize(rs): Record<FileStatus, number>`.
- `issueText(r: FileResult): string` dan `rejectedText(rs: FileResult[]): string`: teks salin `"<source_path ?? file> [<status>/<reason_code>]: <reason>\n  - <CODE> <message>"` per issue (message server sudah memuat baris/pointer). Tanpa isi spec.
- `markOverwrite(rs: FileResult[], existing: Set<string>): Array<FileResult & { willOverwrite: boolean }>`: hanya untuk status `valid`.

**`services/uploadspecs.service.ts`**:
```ts
function toForm(files: PickedFile[], name?: string): FormData; // append('files', f.file, f.path); name (bila diisi) append('name', name)
export const uploadSpecs = (projectId: string, files: PickedFile[], name?: string, signal?: AbortSignal) =>
  post(`/api/projects/${enc(projectId)}/api-specs/upload`, toForm(files, name), { schema: ApiSpecUploadResult, signal });
export const validateSpecs = (projectId: string, files: PickedFile[], name?: string, signal?: AbortSignal) =>
  post(`/api/projects/${enc(projectId)}/api-specs/validate`, toForm(files, name), { schema: ApiSpecValidateResult, signal });
```
`FormData` dikirim apa adanya oleh `http.ts` (browser menentukan boundary; header CSRF otomatis).

**`stores/uploadDialog.store.ts`**: `createUploadDialogStore()` per instance dialog: `{ phase: 'pick'|'validating'|'validated'|'uploading'|'done', picked: PickedFile[], local: FileResult[], name: string, nameError: string|null, formError: {key, params?, traceId?}|null, results: FileResult[], summary }`; aksi murni `pick(files)`, `setName(v)`, `startValidate()`, `validated(res)`, `startUpload()`, `uploaded(res)`, `fail(err)`, `reset()`. Memilih file baru setelah hasil -> kembali ke `pick` dan hasil lama dibuang.

**`components/`**
- `FilePicker.svelte`: dua `<input type="file">` tersembunyi dengan tombol berlabel: "Pilih file" (`multiple`, `accept=".yaml,.yml,.json,application/json,application/yaml"`) dan "Pilih folder" (`webkitdirectory`, `multiple`). Area drop (drag & drop) menerima file (folder lewat `DataTransferItem.webkitGetAsEntry`, rekursif, path dari `fullPath` tanpa `/` awal). Menampilkan daftar terpilih: jumlah file spec yang akan dikirim + jumlah dilewati lokal. `data-testid="apispec-file-input"` dan `apispec-folder-input` untuk e2e.
- `NameField.svelte`: `Input` dengan hint `apispec.upload.nameHint`; `disabled` + hint `apispec.upload.nameOnlySingle` bila jumlah file spec != 1.
- `ResultSummary.svelte`: lencana jumlah per status (hanya yang > 0), `role="status"`.
- `ResultTable.svelte`: baris per file: file + `source_path`, `StatusBadge`, nama spec, judul + versi OpenAPI/API, jumlah endpoint, penanda "akan menimpa"/"menimpa" (`updated`), dan untuk `skipped`/`rejected`: label `apispec.reason.<reason_code>` (fallback `reason` server bila kunci tidak ada) + blok `<pre>` berisi `reason` dan daftar issue (`apispec.issue.<code>` sebagai label pendek, `message` server utuh termasuk baris/pointer) + tombol salin (`copyText(issueText(r))`, status "Tersalin" 2 detik, gagal -> seleksi teks + `common.copyManual`). Baris `created`/`updated`/`unchanged` dengan nama menautkan ke detail (`href`) hanya setelah upload (bukan validasi).
- `StatusBadge.svelte`: warna + teks (tidak hanya warna): created/updated/valid hijau, unchanged abu, skipped kuning, rejected merah.

**`views/UploadSpecsDialog.svelte`**: props `open`, `projectId`, `existingNames: Set<string>`, `onclose()`, `onuploaded(res: ApiSpecUploadResult)`, `onstale()`, `onprojectgone()`.
- `Dialog size="lg"`, `dismissible={phase !== 'validating' && phase !== 'uploading'}`.
- Tombol: "Validasi saja" dan "Unggah" (nonaktif bila tidak ada file spec atau `nameError`), "Tutup". Setelah `done`: "Unggah lagi" (reset) dan "Tutup".
- Alur Validasi: validasi klien nama -> `validateSpecs` -> `validated(res)` -> tabel hasil dengan `markOverwrite(existingNames)`; tombol "Unggah" tetap aktif (mengirim file yang sama).
- Alur Unggah: `uploadSpecs` -> `uploaded` -> `onuploaded(res)` (komposisi memuat ulang daftar). Tombol salin "Salin semua penolakan" bila ada `rejected`.
- Error request (bukan per file): 400 `NO_FILES`/`NAME_WITH_MULTIPLE_FILES`/`NOT_MULTIPART` -> `formError` `apispec.error.<REASON>`; 400 `INVALID_NAME` -> `nameError` dari `fieldErrorsOf(details).name` atau `apispec.error.INVALID_NAME`; 413 -> `apispec.upload.error.tooLarge`; 404 -> tutup + `onprojectgone()`; `INVALID_RESPONSE` pada upload -> `apispec.upload.error.uncertain` + `onstale()`; jaringan/5xx -> pesan server + trace id, file tetap terpilih untuk coba lagi.
- Menutup dialog membuang file terpilih dan hasil (`reset`).

**`index.ts`**: ekspor `UploadSpecsDialog`, `uploadSpecs`, `validateSpecs`, `ResultTable`, `ResultSummary`, tipe `PickedFile`, `ApiSpecUploadResult`, `ApiSpecFileResult`. Ekspor service + komponen hasil disediakan untuk "Upload campuran" T05 yang dikomposisi di `app/` (tiket T05 bagian upload campuran); T06 sendiri tidak membuat dialog campuran.

### S3. Feature `getspec` (`frontend/src/domain/project/api-spec/getspec/`)

**`types.ts`**: `ApiSpec` (salinan lokal, K3), `ApiSpecEndpoint = z.object({ method: z.enum(['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE']), path: z.string(), operation_id: z.string().nullable(), summary: z.string(), tags: z.array(z.string()), deprecated: z.boolean() })`, `OpenApiDocument = z.object({ openapi: z.string(), info: z.object({ title: z.string(), version: z.string() }).passthrough() }).passthrough()`, `ApiSpecDetail = z.object({ spec: ApiSpec, endpoints: z.array(ApiSpecEndpoint), document: OpenApiDocument })`. Isi `document` di luar itu tidak divalidasi Zod (bentuk bebas); model membacanya secara defensif (`isObj`, `asStr`, `asArr`).

**`model/`** (murni, tanpa Svelte, diuji unit):
- `refs.ts`: `resolvePointer(doc, ref): unknown | undefined` (hanya `#`/`#/...`, unescape `~1` -> `/`, `~0` -> `~`, index array); `deref(doc, node, stack: string[]): { node: unknown; ref: string | null; circular: boolean; unresolved: boolean }` (mengikuti rantai `$ref` maksimal 16 langkah; ref di `stack` -> `circular`; kunci saudara `$ref` 3.1 `description`/`summary` digabung di atas hasil). Ref non-`#` -> `unresolved` (seharusnya mustahil, upload menolaknya).
- `operations.ts`: `METHOD_ORDER`; `buildTagTree(doc, endpoints): TagGroup[]` (`{ name, description: string|null, endpoints: EndpointKey[] }`): urutan tag = `doc.tags[]` lalu urutan kemunculan; endpoint dengan banyak tag muncul di tiap tag; tanpa tag -> grup `apispec.detail.untagged` (key `__untagged`, terakhir); di dalam grup urut path lalu `METHOD_ORDER`. `endpointKey(m, p) = m + ' ' + p`; `parseOpParam(op: string|null): {method, path} | null`. `getOperation(doc, method, path): { pathItem, operation, params: Param[], servers, security } | null` (path item `$ref` diselesaikan; parameter level path digabung, operasi menang untuk `name`+`in` yang sama; `servers` operasi ?? path ?? root ?? []; `security` operasi ?? root ?? []).
- `schema.ts`: `typeLabel(schema): string` (`type` string/array; 3.0 `nullable: true` -> tambah `null`; 3.1 `["string","null"]` -> `string | null`; `items` -> `array<..>`; `format` dalam kurung; `enum`/`const`; `oneOf`/`anyOf`/`allOf` -> label komposisi; tanpa tipe -> `any`); `schemaChildren(doc, schema, stack): SchemaRow[]` (`properties` dengan penanda `required`, `readOnly`, `writeOnly`, `deprecated`; `items`; `additionalProperties`; cabang `oneOf/anyOf/allOf`; `discriminator.propertyName` sebagai info), setiap anak membawa `ref`, `circular`, `unresolved`. Tidak pernah membuka rekursi sendiri: anak dimuat saat baris dibuka.
- `search.ts`: `filterEndpoints(endpoints, q): ApiSpecEndpoint[]` (trim, lowercase, semua token harus cocok pada gabungan `method path operation_id summary tags`; q kosong -> semua). Ditampilkan sebagai daftar datar bila q terisi.
- `security.ts`: `usedSchemes(doc, security): Array<{ name, scheme: unknown | null, scopes: string[] }>` (skema dari `components.securitySchemes`, ref diselesaikan; nama yang tidak ada -> `scheme: null`); `security: []` -> "tanpa autentikasi".
- `examples.ts`: `examplesOf(mediaOrParam): Array<{ name, summary, value }>` (`example` tunggal dan `examples` map dengan `$ref`; `externalValue` hanya ditampilkan sebagai teks URL, tidak di-fetch); `toPrettyJson(v, maxChars = 20000)` (dipotong + penanda).

**`services/getspec.service.ts`**: `getSpec(projectId, name, signal?) => get('/api/projects/' + enc(projectId) + '/api-specs/' + enc(name), { schema: ApiSpecDetail, signal })`.

**`stores/specDetail.store.ts`**: `createSpecDetailStore()`: `{ status, detail: ApiSpecDetail|null, error, tree: TagGroup[] (derived), query: string }`; aksi `start`, `set`, `fail`, `setQuery`, `replaceSpec(spec)` (setelah rename: metadata diganti tanpa GET ulang).

**`components/`** (presentasi; tidak memanggil service):
- `SpecHeader.svelte`: nama (mono), judul, `OpenApiBadge` lokal, versi API, format, `source_path`, ukuran, jumlah endpoint, dibuat/diperbarui oleh + waktu; slot `actions` (snippet) untuk tombol dari komposisi.
- `EndpointSidebar.svelte`: props `tree`, `endpoints`, `query`, `selected`, `hrefFor(ep)`, `onquery(q)`. Input cari (`role="searchbox"`, debounce 150 ms) + jumlah hasil; `nav aria-label` berisi grup tag (`<details open>` per tag, ringkasan tag dari deskripsi Markdown `inline`) -> tautan endpoint (`MethodBadge`, path mono, ringkasan, `<s>`/lencana deprecated, `aria-current` untuk terpilih). Hasil cari kosong -> `apispec.detail.searchEmpty`. Di < `md` sidebar menjadi panel yang bisa dilipat di atas konten (`<details>`), tanpa scroll horizontal halaman.
- `MethodBadge.svelte`: warna per method + teks method (bukan warna saja).
- `ApiOverview.svelte`: `info` (judul, versi, `Markdown` deskripsi, `termsOfService`/`contact`/`license` lewat `safeHref`), `ServersList`, `SecuritySchemesList` (semua `components.securitySchemes`: tipe, scheme, `bearerFormat`, `in`/`name` untuk apiKey, flow OAuth2 + scope; URL OAuth sebagai teks/`safeHref`), `externalDocs`, daftar tag + deskripsi, dan bagian **Webhooks** (3.1) yang memakai `OperationView` per webhook (dilipat).
- `OperationView.svelte`: props `doc`, `method`, `path`, `op` (hasil `getOperation`). Menampilkan judul (`MethodBadge` + path + `summary`), lencana deprecated, `operationId`, tag, `Markdown` deskripsi, **Security** (`usedSchemes`), **Servers** efektif, **Parameters** (dikelompokkan `path/query/header/cookie`: nama, `required`, `deprecated`, `typeLabel`, deskripsi Markdown, contoh), **Request body** (`required`, deskripsi, tab per media type -> `SchemaTree` + `ExampleBlock`), **Responses** (per kode, urutan kunci objek; `default` terakhir; deskripsi, header, tab per media type -> `SchemaTree` + `ExampleBlock`, `links` sebagai teks), **Callbacks** (nama + daftar operasi, dilipat). Tanpa tombol "Try it out" atau form apa pun.
- `SchemaTree.svelte` (rekursif): props `doc`, `schema`, `stack`, `depth`, `name?`, `required?`. Baris: nama, `typeLabel`, penanda required/readOnly/writeOnly/deprecated/nullable, `default`/`enum` (teks JSON), deskripsi Markdown; anak dimuat hanya saat baris dibuka (`<button aria-expanded>`); level 0 terbuka, sisanya tertutup. Ref sirkular -> lencana `apispec.schema.circular` + nama skema, tanpa tombol buka. Ref tak terselesaikan -> lencana `apispec.schema.unresolved`. Nama skema `$ref` ditampilkan (`#/components/schemas/Customer` -> `Customer`).
- `ExampleBlock.svelte`: daftar contoh (pilih bila > 1), `<pre><code>` teks dari `toPrettyJson`, tombol salin (`copyText`).
- `ServersList.svelte`, `SecuritySchemesList.svelte`, `OpenApiBadge.svelte`.

**`views/SpecDetailView.svelte`**: props `projectId`, `name`, `selectedOp: string | null` (dari query `op`), `hrefForOp(key: string | null): string`, `actions?: Snippet<[ApiSpec]>`, `store?` (opsional; default dibuat sendiri), `onnotfound?()`. `$effect` pada `projectId`+`name`: `AbortController`, `getSpec` -> store. Layout dua kolom (`md:grid-cols-[18rem_1fr]`): `EndpointSidebar` | panel utama (`OperationView` bila `selectedOp` valid dan ada di dokumen; `selectedOp` tidak dikenal -> `apispec.detail.opNotFound` + tautan ringkasan; tanpa `selectedOp` -> `ApiOverview`). Saat operasi berganti, fokus dipindah ke judul operasi (`tabindex="-1"`) untuk pembaca layar dan panel utama di-scroll ke atas. `document.title` diatur komposisi.
- 404 -> `EmptyState` `apispec.detail.notFound` + tautan `apispec.detail.backToList` (atau `onnotfound`). Dokumen tanpa endpoint (`endpoints.length === 0`, mis. webhooks-only) -> sidebar `apispec.detail.noEndpoints`, panel ringkasan tetap tampil.

**`index.ts`**: ekspor `SpecDetailView`, `createSpecDetailStore`, `type SpecDetailStore`, `type ApiSpecDetail`.

### S4. Feature `downloadspec` (`frontend/src/domain/project/api-spec/downloadspec/`)

- `services/downloadspec.service.ts`: `downloadSpec(projectId, name, format: 'yaml'|'json', signal?) => getBlob('/api/projects/' + enc(projectId) + '/api-specs/' + enc(name) + '/raw')` lalu `{ blob, fileName: name + (format === 'json' ? '.json' : '.yaml') }` (nama file dari nama spec dan format, cermin `Content-Disposition` backend; header tidak di-parse).
- `views/DownloadSpecButton.svelte`: props `projectId`, `spec: { name, format }`, `onspecgone()`, `onprojectgone()`. Klik -> tombol `loading` -> `downloadSpec` -> `saveBlob`. 404 -> toast `apispec.download.gone` + `onspecgone()`; error lain -> `toast.fromError`.
- `index.ts`: ekspor `DownloadSpecButton`, `downloadSpec`.

### S5. Feature `renamespec` (`frontend/src/domain/project/api-spec/renamespec/`)

- `types.ts`: `ApiSpec` (salinan lokal), `RenameResponse = z.object({ spec: ApiSpec })`, `NAME_RE`, `validateName(v): 'ok'|'required'|'invalid'`.
- `services/renamespec.service.ts`: `renameSpec(projectId, name, newName) => patch('/api/projects/' + enc(projectId) + '/api-specs/' + enc(name), { name: newName }, { schema: RenameResponse })`.
- `components/RenameForm.svelte`: `Input` (nilai awal nama lama, `autocomplete="off"`, `spellcheck=false`, hint aturan nama), pratinjau URL baru. Tombol Simpan nonaktif bila tidak berubah atau tidak valid.
- `views/RenameSpecDialog.svelte`: props `open`, `projectId`, `spec`, `onclose()`, `onrenamed(spec)`, `onspecgone()`, `onprojectgone()`. `Dialog dismissible={!submitting}`. Validasi klien (`required` / `invalid`) -> pesan i18n. Respons: 200 -> `onrenamed(res.spec)` + toast `apispec.rename.success`; 409 `NAME_TAKEN` -> error field `apispec.error.NAME_TAKEN`; 400 `INVALID_NAME`/`fields.name` -> error field; 404 -> tutup + toast `apispec.error.specGone` + `onspecgone()` (tidak membedakan project/spec; daftar akan memuat ulang dan menangani project hilang). Nama sama dengan lama tidak mengirim request.
- `index.ts`: ekspor `RenameSpecDialog`.

### S6. Feature `deletespec` (`frontend/src/domain/project/api-spec/deletespec/`)

- `services/deletespec.service.ts`: `deleteSpec(projectId, name) => del('/api/projects/' + enc(projectId) + '/api-specs/' + enc(name))` (204, tanpa schema).
- `components/DeleteConfirm.svelte`: pesan `apispec.delete.message {name}` + efek (`apispec.delete.effect`: endpoint tidak lagi bisa dibaca agent; tidak bisa dibatalkan; unduh dulu bila perlu). Tanpa mengetik ulang nama (spec bisa diunggah ulang; berbeda dengan hapus project).
- `views/DeleteSpecDialog.svelte`: props `open`, `projectId`, `spec`, `onclose()`, `ondeleted(name)`, `onprojectgone()`. `dismissible={!submitting}`. 204 -> `ondeleted` + toast `apispec.delete.success`; 404 -> diperlakukan sudah terhapus: toast `apispec.delete.alreadyGone` + `ondeleted(name)`; 5xx/jaringan -> pesan di dialog, bisa coba lagi.
- `index.ts`: ekspor `DeleteSpecDialog`.

### A. Komposisi `app/`

**A1. Route detail generik `app/routes/index.ts`** (ubah): tambah
```ts
{ path: '/projects/:projectID/:tab/:item', view: ProjectWorkspaceView, meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' } },
```
Router mencocokkan per jumlah segmen, jadi tidak bentrok dengan `/projects/:projectID/:tab`. Test `router.test.ts`: `/projects/p1/api-specs/payment.v2` -> `params.item === 'payment.v2'`; `%2F` di-decode.

**A2. `app/workspace/tabs.ts`** (ubah, generik):
- `WorkspaceTabProps` ditambah `item: string | null` dan `query: URLSearchParams` (tab lama mengabaikannya).
- `WorkspaceTab` ditambah `supportsItem?: boolean` (default `false`). `ProjectWorkspaceView` merender `project.tab.notFound` bila `item` terisi untuk tab tanpa `supportsItem`.
- Sisipkan `{ key: 'api-specs', labelKey: 'project.tab.apiSpecs', view: ApiSpecsTab, supportsItem: true }` setelah `knowledge` (bila sudah ada) dan sebelum `api-keys`/`members` (urutan final tiket T03: Knowledge, API Specs, API Keys, Members, Settings). Default tab tetap entri pertama (K5 T03).
- `tabs.test.ts` (ubah): urutan entri sesuai yang ada di `main` saat implementasi + `api-specs` di posisi benar; `findTab('api-specs')?.supportsItem === true`.

**A3. `app/views/ProjectWorkspaceView.svelte`** (ubah kecil): teruskan `item={params.item ?? null}` dan `query={$current.location.query}` ke tab; tab aktif dihitung dari `params.tab` (nav tab `aria-current` tetap benar di halaman detail).

**A4. `app/views/workspace/ApiSpecsTab.svelte`** (baru). Props `WorkspaceTabProps`.
- `const list = createSpecListStore()` (satu per instance tab, bertahan saat berpindah daftar <-> detail karena tab sama).
- `base = '/projects/' + enc(ctx.project.id) + '/api-specs'`.
- `item === null` -> `<ListSpecsView projectId={ctx.project.id} store={list} onupload={() => (uploadOpen = true)} {onprojectgone} />` + `<UploadSpecsDialog open={uploadOpen} projectId={ctx.project.id} existingNames={list.names()} onclose={() => (uploadOpen = false)} onuploaded={() => list.invalidate()} onstale={() => list.invalidate()} {onprojectgone} />`.
- `item !== null` -> tautan "Kembali ke daftar" + `<SpecDetailView projectId name={item} selectedOp={query.get('op')} hrefForOp={(k) => base + '/' + enc(item) + (k ? '?op=' + encodeURIComponent(k) : '')} actions={actionsSnippet} />`, di mana `actionsSnippet(spec)` merender `DownloadSpecButton`, tombol "Ubah nama" (membuka `RenameSpecDialog`), tombol "Hapus" (membuka `DeleteSpecDialog`).
  - `onrenamed(spec)` -> `list.rename(item, spec)`; `navigate(base + '/' + enc(spec.name) + location.search, { replace: true })`.
  - `ondeleted(name)` -> `list.remove(name)`; `navigate(base, { replace: true })`.
  - `onspecgone()` -> `list.invalidate()`; `navigate(base, { replace: true })`.
- `$effect`: di detail, `document.title = item + ' · ' + ctx.project.name + ' · ' + $t('app.name')`.
- Pemilihan operasi lewat tautan biasa (`href`) sehingga Back/Forward dan deep link bekerja (router menangkap klik `<a>`).
- Upload campuran T05: tidak dibuat di T06. Bila T05 FE sudah merge, `KnowledgeTab` T05 yang mengimpor `uploadSpecs`/`ResultTable` dari `uploadspecs/index.ts` (di `app/`).

**A5. `app/views/workspace/ApiSpecsTab.test.ts`** (baru) dan `ProjectWorkspaceView.test.ts` (ubah): kasus tab daftar, tab detail dengan `item`, `item` pada tab tanpa `supportsItem` -> `project.tab.notFound`, nav `aria-current` pada "API Specs" di halaman detail.

### E. E2E

- `frontend/e2e/fixtures/apispec/` (baru): `auth.yaml` (3.1, 5 endpoint), `payment.yaml` (3.0, 2 endpoint: `GET /payments`, `POST /payments`, skema bersarang `Payment -> Customer -> Address`, skema sirkular `Node.children[] -> Node`, `nullable`, parameter level path, securityScheme bearer, `deprecated: true` pada satu operasi), `payment-updated.yaml` (endpoint `GET /payments` dihapus, tambah `POST /payments/{id}/refund`), `notification.yaml` (3.0), `petstore-3.1.json` (`type: ["string","null"]`), `README.md`, `package.json`, `broken.yaml`, `swagger-2.0.yaml`, `ref-http.template.yaml` (`$ref: http://127.0.0.1:{PORT}/common.yaml#/X`, PORT diisi saat test), `xss.yaml` (deskripsi `<script>alert(1)</script>`, `<img src=x onerror=alert(1)>`, `[klik](javascript:alert(1))`, `servers[0].url: javascript:alert(1)`), `folder/v1/payment.yaml` + `folder/v2/payment.yaml` (nama dasar sama). File 6 MB dibuat saat test di direktori temp (`os.tmpdir()`), tidak di-commit.
- `frontend/e2e/helpers/apispecs.ts` (baru): `uploadSpecsApi(request, projectId, files: {path, name}[])` (multipart + CSRF), `listSpecsApi`, `deleteSpecApi`. Memakai `helpers/projects.ts` T03 untuk membuat project dan anggota, `helpers/auth.ts` untuk login.
- `frontend/e2e/project-api-spec.spec.ts` (baru): skenario bagian "E2E Scenarios". Folder dipilih dengan `setInputFiles('<dir>')` pada input `webkitdirectory` (didukung Playwright 1.64). Konteks diberi `permissions: ['clipboard-read', 'clipboard-write']` untuk uji salin. Listener lokal (`http.createServer`) menghitung request untuk skenario `$ref` URL.

### V. Verifikasi

`npm run check`, `npm test`, `npm run depcruise`, `npm run test:arch`, `npm run build` (bundle di-embed backend; pastikan ukuran bundle naik wajar setelah `markdown-it` + `dompurify`), lalu `npm run test:e2e` terhadap binary (butuh backend T06 + T03 + T01 + T02).

---

## State UI (loading / empty / error per permukaan)

| Permukaan | Loading | Empty | Error | Khusus |
|---|---|---|---|---|
| Header workspace | Ditangani `ProjectContextView` T03 | - | `ErrorState` T03 | Bukan anggota / project tidak ada / id bukan uuid -> `project.detail.notFound` seragam (T03) |
| Daftar spec | Pertama: `Table loading` + `aria-busy`. Muat ulang (setelah upload): daftar lama tetap tampil + `aria-busy` | `apispec.list.empty` + `emptyHint` + tombol "Unggah spec" | `ErrorState` (pesan + trace id) + coba lagi; tanpa daftar parsial | 404 -> `onprojectgone` (beranda). `total >= limit` -> peringatan kuota. Respons rusak -> `INVALID_RESPONSE` di `ErrorState` |
| Dialog unggah: pilih | - | Belum ada file: area drop + `apispec.upload.pickHint`. Semua file non-spec: tombol aksi nonaktif + `apispec.upload.noSpecFiles` | Nama tidak valid / nama untuk banyak file: pesan di bawah input (klien) | Jumlah file spec dan dilewati lokal ditampilkan |
| Dialog unggah: validasi/unggah berjalan | Tombol aktif `loading`, tombol lain dan input `disabled`, dialog tidak bisa ditutup | - | `formError`: `NO_FILES`, `NAME_WITH_MULTIPLE_FILES`, `NOT_MULTIPART`, 413 `tooLarge`, 5xx/jaringan + trace id (file tetap terpilih) | 404 -> tutup + beranda. `INVALID_RESPONSE` saat unggah -> `uncertain` + daftar dimuat ulang |
| Dialog unggah: hasil | - | - | Per baris `rejected`/`skipped` dengan alasan + issue + salin. Salin gagal -> teks terseleksi + `common.copyManual` | Ringkasan per status. Validasi: penanda "akan menimpa". Unggah: daftar dimuat ulang, nama menautkan ke detail |
| Detail spec | `Spinner` + kerangka dua kolom, `aria-busy` | Spec tanpa endpoint: sidebar `apispec.detail.noEndpoints`, panel ringkasan tetap. Cari tanpa hasil: `apispec.detail.searchEmpty` | `ErrorState` + coba lagi | 404 -> `apispec.detail.notFound` + tautan daftar. `?op=` tidak dikenal -> `apispec.detail.opNotFound` + tautan ringkasan. Respons rusak -> `INVALID_RESPONSE` |
| Renderer operasi/skema | Anak skema dimuat saat dibuka (sinkron, tanpa spinner) | Operasi tanpa parameter/body/response: bagian tidak dirender (bukan "kosong") | `$ref` tidak terselesaikan -> lencana `unresolved` | Sirkular -> lencana `circular`, tanpa tombol buka. Contoh > 20 000 karakter dipotong + penanda |
| Unduh | Tombol `loading` | - | Toast dari `ApiError` | 404 -> toast `download.gone` + ke daftar |
| Dialog ubah nama | Tombol Simpan `loading`, dialog tidak bisa ditutup | - | Field: `required`, `invalid`, `NAME_TAKEN`, pesan server `fields.name`. `formError`: 5xx/jaringan | Tidak berubah -> Simpan nonaktif. 404 -> toast + ke daftar. Sukses -> URL diganti ke nama baru (replace) |
| Dialog hapus | Tombol Hapus `loading`, dialog tidak bisa ditutup | - | 5xx/jaringan di dialog (bisa coba lagi) | 404 -> dianggap sudah terhapus + toast + ke daftar |
| Global | `PendingView` saat sesi `unknown` (T01) | - | 401 -> `/login?next=...` (handler T01) | `unhandledrejection` ApiError -> toast (T00) |

---

## Kunci i18n

Ditambahkan ke `id.json` dan `en.json` (flat; parameter `{x}`). Kunci yang sudah ada (`common.*`, `error.*`, `project.*` T03, `common.copy*` T04) dipakai ulang; `common.copy`, `common.copied`, `common.copyManual` hanya ditambah bila belum ada (isi sama dengan plan T04).

| Kunci | id | en |
|---|---|---|
| `project.tab.apiSpecs` | API Specs | API Specs |
| `apispec.title` | API Specs | API Specs |
| `apispec.openapiVersion` | Versi OpenAPI | OpenAPI version |
| `apispec.list.quota` | {total} / {limit} spec | {total} / {limit} specs |
| `apispec.list.limitReached` | Batas spec tercapai. Spec baru akan ditolak; menimpa spec yang ada tetap bisa. | Spec limit reached. New specs will be rejected; overwriting existing specs still works. |
| `apispec.list.empty` | Belum ada API spec | No API specs yet |
| `apispec.list.emptyHint` | Unggah file OpenAPI 3.0/3.1 (.yaml, .yml, .json) satu per satu atau satu folder sekaligus. | Upload OpenAPI 3.0/3.1 files (.yaml, .yml, .json) one by one or a whole folder. |
| `apispec.list.col.name` | Nama | Name |
| `apispec.list.col.version` | Versi | Version |
| `apispec.list.col.endpoints` | Endpoint | Endpoints |
| `apispec.list.col.source` | Sumber | Source |
| `apispec.list.col.updated` | Diperbarui | Updated |
| `apispec.list.updatedBy` | {by} · {at} | {by} · {at} |
| `apispec.upload.open` | Unggah spec | Upload specs |
| `apispec.upload.title` | Unggah API spec | Upload API specs |
| `apispec.upload.pickFiles` | Pilih file | Choose files |
| `apispec.upload.pickFolder` | Pilih folder | Choose folder |
| `apispec.upload.pickHint` | Seret file atau folder ke sini. Spec dengan nama yang sama akan ditimpa. | Drop files or a folder here. Specs with the same name are overwritten. |
| `apispec.upload.selected` | {spec} file spec dipilih, {skipped} file lain dilewati | {spec} spec files selected, {skipped} other files skipped |
| `apispec.upload.noSpecFiles` | Tidak ada file .yaml, .yml, atau .json | No .yaml, .yml, or .json files |
| `apispec.upload.name` | Nama spec (opsional) | Spec name (optional) |
| `apispec.upload.nameHint` | Kosongkan untuk memakai nama file. Huruf kecil, angka, '-', '_', '.'; 1-80 karakter. | Leave empty to use the file name. Lowercase letters, digits, '-', '_', '.'; 1-80 characters. |
| `apispec.upload.nameOnlySingle` | Nama hanya bisa diisi saat mengunggah tepat satu file | A name can only be set when uploading exactly one file |
| `apispec.upload.validate` | Validasi saja | Validate only |
| `apispec.upload.submit` | Unggah | Upload |
| `apispec.upload.again` | Unggah lagi | Upload more |
| `apispec.upload.copyRejected` | Salin semua penolakan | Copy all rejections |
| `apispec.upload.willOverwrite` | Akan menimpa spec yang ada | Will overwrite existing spec |
| `apispec.upload.error.tooLarge` | Ukuran unggahan melebihi batas. Tidak ada yang disimpan. Unggah lebih sedikit file sekaligus. | The upload exceeds the size limit. Nothing was saved. Upload fewer files at once. |
| `apispec.upload.error.uncertain` | Respons server tidak valid. Sebagian spec mungkin sudah tersimpan; periksa daftar. | The server response was invalid. Some specs may have been saved; check the list. |
| `apispec.status.created` | Dibuat | Created |
| `apispec.status.updated` | Diperbarui | Updated |
| `apispec.status.unchanged` | Tidak berubah | Unchanged |
| `apispec.status.skipped` | Dilewati | Skipped |
| `apispec.status.rejected` | Ditolak | Rejected |
| `apispec.status.valid` | Valid | Valid |
| `apispec.reason.NOT_SPEC_FILE` | Bukan file spec (.yaml/.yml/.json) | Not a spec file (.yaml/.yml/.json) |
| `apispec.reason.FILE_TOO_LARGE` | Ukuran file melebihi batas | File exceeds the size limit |
| `apispec.reason.TOO_MANY_FILES` | Melebihi batas jumlah file per unggahan | Exceeds the per-upload file limit |
| `apispec.reason.INVALID_PATH` | Path file tidak valid | Invalid file path |
| `apispec.reason.INVALID_NAME` | Nama spec tidak valid | Invalid spec name |
| `apispec.reason.DUPLICATE_NAME_IN_UPLOAD` | Nama duplikat dalam unggahan ini | Duplicate name in this upload |
| `apispec.reason.INVALID_SPEC` | Bukan spec OpenAPI 3.0/3.1 yang valid | Not a valid OpenAPI 3.0/3.1 spec |
| `apispec.reason.SPEC_LIMIT_REACHED` | Batas spec per project tercapai | Per-project spec limit reached |
| `apispec.issue.INVALID_ENCODING` | Encoding bukan UTF-8 | Not UTF-8 |
| `apispec.issue.EMPTY` | File kosong | Empty file |
| `apispec.issue.SYNTAX` | Sintaks rusak | Syntax error |
| `apispec.issue.MULTI_DOCUMENT` | Lebih dari satu dokumen YAML | More than one YAML document |
| `apispec.issue.YAML_LIMIT` | YAML terlalu kompleks | YAML too complex |
| `apispec.issue.NOT_OPENAPI` | Bukan OpenAPI | Not OpenAPI |
| `apispec.issue.NOT_OPENAPI_3` | Swagger 2.0 tidak didukung | Swagger 2.0 not supported |
| `apispec.issue.UNSUPPORTED_VERSION` | Versi tidak didukung | Unsupported version |
| `apispec.issue.INVALID_STRUCTURE` | Struktur tidak valid | Invalid structure |
| `apispec.issue.DUPLICATE_OPERATION_ID` | operationId ganda | Duplicate operationId |
| `apispec.issue.UNRESOLVED_REF` | Referensi tidak ditemukan | Unresolved reference |
| `apispec.issue.EXTERNAL_REF` | Referensi eksternal tidak didukung | External reference not supported |
| `apispec.detail.backToList` | Kembali ke daftar spec | Back to spec list |
| `apispec.detail.notFound` | Spec tidak ditemukan | Spec not found |
| `apispec.detail.notFoundHint` | Spec mungkin sudah dihapus atau diganti namanya. | The spec may have been deleted or renamed. |
| `apispec.detail.search` | Cari endpoint | Search endpoints |
| `apispec.detail.searchCount` | {n} endpoint | {n} endpoints |
| `apispec.detail.searchEmpty` | Tidak ada endpoint yang cocok | No matching endpoints |
| `apispec.detail.noEndpoints` | Spec ini tidak punya endpoint di paths | This spec has no endpoints under paths |
| `apispec.detail.untagged` | Tanpa tag | Untagged |
| `apispec.detail.overview` | Ringkasan API | API overview |
| `apispec.detail.opNotFound` | Endpoint tidak ada di spec ini | Endpoint not found in this spec |
| `apispec.detail.meta` | {format} · {size} · {count} endpoint · sumber {source} | {format} · {size} · {count} endpoints · source {source} |
| `apispec.detail.createdBy` | Dibuat oleh {by} pada {at} | Created by {by} on {at} |
| `apispec.detail.updatedBy` | Diperbarui oleh {by} pada {at} | Updated by {by} on {at} |
| `apispec.doc.servers` | Server | Servers |
| `apispec.doc.security` | Keamanan | Security |
| `apispec.doc.securitySchemes` | Skema keamanan | Security schemes |
| `apispec.doc.noAuth` | Tanpa autentikasi | No authentication |
| `apispec.doc.parameters` | Parameter | Parameters |
| `apispec.doc.requestBody` | Request body | Request body |
| `apispec.doc.responses` | Response | Responses |
| `apispec.doc.callbacks` | Callback | Callbacks |
| `apispec.doc.webhooks` | Webhook | Webhooks |
| `apispec.doc.headers` | Header | Headers |
| `apispec.doc.examples` | Contoh | Examples |
| `apispec.doc.externalDocs` | Dokumentasi eksternal | External docs |
| `apispec.doc.deprecated` | Usang | Deprecated |
| `apispec.doc.required` | wajib | required |
| `apispec.doc.readOnly` | hanya-baca | read-only |
| `apispec.doc.writeOnly` | hanya-tulis | write-only |
| `apispec.doc.in.path` | Path | Path |
| `apispec.doc.in.query` | Query | Query |
| `apispec.doc.in.header` | Header | Header |
| `apispec.doc.in.cookie` | Cookie | Cookie |
| `apispec.doc.truncated` | Dipotong ({n} karakter) | Truncated ({n} characters) |
| `apispec.schema.expand` | Buka {name} | Expand {name} |
| `apispec.schema.collapse` | Tutup {name} | Collapse {name} |
| `apispec.schema.circular` | Referensi sirkular ke {name} | Circular reference to {name} |
| `apispec.schema.unresolved` | Referensi tidak ditemukan: {ref} | Unresolved reference: {ref} |
| `apispec.download.open` | Unduh | Download |
| `apispec.download.gone` | Spec sudah tidak ada | The spec no longer exists |
| `apispec.rename.open` | Ubah nama | Rename |
| `apispec.rename.title` | Ubah nama spec | Rename spec |
| `apispec.rename.label` | Nama baru | New name |
| `apispec.rename.preview` | URL baru: {url} | New URL: {url} |
| `apispec.rename.success` | Spec diganti nama menjadi "{name}" | Spec renamed to "{name}" |
| `apispec.validation.nameRequired` | Nama wajib diisi | Name is required |
| `apispec.validation.nameInvalid` | Huruf kecil, angka, '-', '_', '.'; 1-80 karakter | Lowercase letters, digits, '-', '_', '.'; 1-80 characters |
| `apispec.delete.open` | Hapus | Delete |
| `apispec.delete.title` | Hapus spec? | Delete spec? |
| `apispec.delete.message` | Spec "{name}" dan indeks endpoint-nya akan dihapus permanen. | Spec "{name}" and its endpoint index will be permanently deleted. |
| `apispec.delete.effect` | Agent tidak bisa lagi membaca endpoint spec ini. Unduh spec lebih dulu bila masih diperlukan. | Agents can no longer read this spec's endpoints. Download the spec first if you still need it. |
| `apispec.delete.submit` | Hapus spec | Delete spec |
| `apispec.delete.success` | Spec "{name}" dihapus | Spec "{name}" deleted |
| `apispec.delete.alreadyGone` | Spec "{name}" sudah tidak ada | Spec "{name}" no longer exists |
| `apispec.error.NAME_TAKEN` | Nama ini sudah dipakai spec lain di project ini | This name is already used by another spec in this project |
| `apispec.error.INVALID_NAME` | Nama spec tidak valid | Invalid spec name |
| `apispec.error.NO_FILES` | Tidak ada file yang dikirim | No files were sent |
| `apispec.error.NAME_WITH_MULTIPLE_FILES` | Nama hanya boleh diisi saat mengunggah tepat satu file | A name can only be set when uploading exactly one file |
| `apispec.error.NOT_MULTIPART` | Format unggahan tidak valid | Invalid upload format |
| `apispec.error.specGone` | Spec sudah tidak ada. Daftar diperbarui. | The spec no longer exists. The list has been refreshed. |

---

## Keputusan dan selisih dengan overview / plan usecase

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Folder FE `frontend/src/domain/project/api-spec/` (bukan `apispec/` seperti tiket, overview 12, dan `usecase-listspecs.md`) | README: `domain/<domain>/<module>/<feature>` dengan module `api-spec`; preseden T04 K1 (`api-key/`) dan `core/audit-log/`. Package Go tetap `apispec` |
| K2 | Satu feature FE `uploadspecs` untuk usecase `uploadspecs` + `validatespec` (dua service di satu file) | Satu permukaan UI (dialog yang sama, file yang sama, tabel hasil yang sama); memisah akan memaksa import silang feature (rule 12.4). Pola T03 K1 (`members`) |
| K3 | Schema `ApiSpec` disalin lokal di `listspecs`, `getspec`, `renamespec` | Pola T02/T03 K4/T04 K4; tipe kompatibel struktural, fixture JSON sama (overview 15) |
| K4 | Route detail generik `/projects/:projectID/:tab/:item` + `WorkspaceTabProps.item/query` + `WorkspaceTab.supportsItem` di `app/` | Router T00 mencocokkan per jumlah segmen; tab perlu sub-halaman yang bisa di-deep-link dan tetap di bawah header workspace. Generik (T05 bisa memakainya), tetap di `app/` (rule 12.4) |
| K5 | Renderer OpenAPI ditulis sendiri (komponen Svelte + model murni), bukan Swagger UI/Redoc/RapiDoc | Tiket menyerahkan ke builder. Pustaka populer berbasis React/Web Component berukuran besar, punya "Try it out"/loader `$ref` eksternal yang harus dimatikan, dan memakai renderer Markdown sendiri yang sulit dibuktikan aman. Renderer sendiri: tanpa eksekusi request (non-goal), sanitasi seragam (AC8), lazy untuk spec besar |
| K6 | Markdown lewat `shared/utils/markdown.ts` + `shared/components/Markdown.svelte` (`markdown-it` `html:false` + `dompurify`, gambar tidak dimuat). Dibuat oleh tiket yang merge lebih dulu (T05 atau T06) | Rule 12.6/7.9, tiket T06 "disanitasi seperti T05". Dua lapis (escape HTML + sanitizer) agar regresi konfigurasi satu lapis tidak membuka XSS. Gambar remote dari isi spec bisa dipakai pelacakan; tidak ada kebutuhan gambar (spec desain: tanpa gambar) |
| K7 | FE tidak membedakan 404 "project" vs "spec" di halaman detail (backend tidak memberi `details.reason`); 404 detail -> "spec tidak ditemukan" + tautan daftar. Daftar yang 404 -> beranda | Project sudah divalidasi `ProjectContextView`; bila project hilang di tengah sesi, membuka daftar menghasilkan 404 -> beranda. Tidak bergantung pada teks pesan server |
| K8 | File non-spec disaring di klien sebelum dikirim dan dilaporkan sebagai `skipped` lokal (`NOT_SPEC_FILE`) | Menghindari mengirim isi folder besar (mis. README, gambar) dan batas `MAX_UPLOAD_FILES`/`MAX_UPLOAD_BYTES`. Hasil identik dengan perilaku backend (backend tetap memvalidasi semua yang dikirim) |
| K9 | Unduhan lewat `getBlob` + `saveBlob` (bukan `<a href download>` langsung) | Error 404/401 tetap tampil sebagai pesan, bukan file JSON error yang terunduh; 401 memakai handler global |
| K10 | Operasi terpilih disimpan di query `?op=<METHOD> <path>` (tautan biasa, push); pencarian endpoint hanya state lokal | Deep link dan Back/Forward per operasi; pencarian bersifat sementara dan tidak memenuhi riwayat |
| K11 | Dokumen `document` hanya divalidasi Zod di level atas (`openapi`, `info.title`, `info.version`, `passthrough`); sisanya dibaca defensif oleh model | Bentuk OpenAPI terlalu luas untuk schema Zod penuh; validator struktural ada di backend. Model tidak pernah melempar pada bentuk tak terduga (diuji dengan fixture rusak) |
| K12 | Urutan kode response mengikuti urutan kunci objek JS (kunci integer naik, lalu `default`) | Overview risiko 6 (`JSON.parse` mengurutkan kunci integer-like); dapat diterima |
| K13 | Validasi nama di klien hanya UX; server tetap sumber kebenaran (pesan `fields.name` server ditampilkan bila lolos klien) | Rule 12.7 |
| K14 | Tidak membuat dialog "Upload campuran" Markdown + spec | Milik T05 (dikomposisi di `app/`); T06 cukup mengekspor `uploadSpecs` + komponen hasil di `index.ts` |

---

## Migration

Tidak ada. Frontend tidak punya migrasi. Migrasi backend T06 (`backend/migrations/project/<VERSION>_create_api_specs.sql`) dibuat di langkah F `overview.md`.

---

## Changes

`frontend/package.json`, `frontend/package-lock.json` (ubah, bila belum ditambahkan T05 FE): `markdown-it`, `dompurify` (dependencies, versi exact), `@types/markdown-it` (devDependencies).

`frontend/src/shared/` (generik):
- `services/http.ts`, `services/http.test.ts` (ubah): `getBlob`, helper privat `toApiError`.
- `utils/download.ts`, `utils/download.test.ts` (baru): `saveBlob`.
- `utils/url.ts`, `utils/url.test.ts` (baru): `safeHref`.
- `utils/markdown.ts`, `utils/markdown.test.ts` (baru, bila belum ada dari T05).
- `components/Markdown.svelte` (baru, bila belum ada dari T05), `components/index.ts` (ubah: ekspor), `components/components.test.ts` (ubah).
- `components/Dialog.svelte` (`dismissible`, `size`) dan `utils/clipboard.ts` (`copyText`): hanya bila T04 FE belum merge (isi persis plan T04).
- `i18n/id.json`, `i18n/en.json` (ubah): kunci tabel "Kunci i18n".

`frontend/src/domain/project/api-spec/` (baru):
- `listspecs/{types.ts, types.test.ts, index.ts, services/listspecs.service.ts, services/listspecs.service.test.ts, stores/specList.store.ts, stores/specList.store.test.ts, components/{SpecTable.svelte, OpenApiBadge.svelte, SpecQuota.svelte, components.test.ts}, views/ListSpecsView.svelte, views/ListSpecsView.test.ts, mocks/specs.ts}`.
- `uploadspecs/{types.ts, types.test.ts, model.ts, model.test.ts, index.ts, services/uploadspecs.service.ts, services/uploadspecs.service.test.ts, stores/uploadDialog.store.ts, stores/uploadDialog.store.test.ts, components/{FilePicker.svelte, NameField.svelte, ResultSummary.svelte, ResultTable.svelte, StatusBadge.svelte, components.test.ts}, views/UploadSpecsDialog.svelte, views/UploadSpecsDialog.test.ts, mocks/results.ts}`.
- `getspec/{types.ts, types.test.ts, index.ts, model/{refs.ts, refs.test.ts, operations.ts, operations.test.ts, schema.ts, schema.test.ts, search.ts, search.test.ts, security.ts, security.test.ts, examples.ts, examples.test.ts}, services/getspec.service.ts, services/getspec.service.test.ts, stores/specDetail.store.ts, stores/specDetail.store.test.ts, components/{SpecHeader.svelte, EndpointSidebar.svelte, MethodBadge.svelte, OpenApiBadge.svelte, ApiOverview.svelte, OperationView.svelte, SchemaTree.svelte, ExampleBlock.svelte, ServersList.svelte, SecuritySchemesList.svelte, components.test.ts, security.test.ts}, views/SpecDetailView.svelte, views/SpecDetailView.test.ts, mocks/{detail-3.0.ts, detail-3.1.ts, detail-xss.ts, detail-circular.ts, detail-webhooks.ts}}`.
- `downloadspec/{index.ts, services/downloadspec.service.ts, services/downloadspec.service.test.ts, views/DownloadSpecButton.svelte, views/DownloadSpecButton.test.ts}`.
- `renamespec/{types.ts, types.test.ts, index.ts, services/renamespec.service.ts, services/renamespec.service.test.ts, components/RenameForm.svelte, components/RenameForm.test.ts, views/RenameSpecDialog.svelte, views/RenameSpecDialog.test.ts}`.
- `deletespec/{index.ts, services/deletespec.service.ts, services/deletespec.service.test.ts, components/DeleteConfirm.svelte, views/DeleteSpecDialog.svelte, views/DeleteSpecDialog.test.ts}`.

`frontend/src/app/`:
- `routes/index.ts` (ubah: route `/projects/:projectID/:tab/:item`), `routes/router.test.ts` (ubah).
- `workspace/tabs.ts`, `workspace/tabs.test.ts` (ubah: `item`, `query`, `supportsItem`, entri `api-specs`).
- `views/ProjectWorkspaceView.svelte`, `views/ProjectWorkspaceView.test.ts` (ubah: teruskan `item`/`query`, tab tanpa `supportsItem`).
- `views/workspace/ApiSpecsTab.svelte`, `views/workspace/ApiSpecsTab.test.ts` (baru).

`frontend/e2e/`:
- `project-api-spec.spec.ts` (baru).
- `helpers/apispecs.ts` (baru).
- `fixtures/apispec/*` (baru, daftar di langkah E).

Tidak diubah: `shared/validation/error.ts` (`reasonOf`/`fieldErrorsOf` sudah ada), `shared/stores/*`, `app/App.svelte`, `app/layouts/*`, `app/routes/{router,guard}.ts`, `.dependency-cruiser.cjs`, `scripts/test-arch.mjs`, `playwright.config.ts`, `domain/core/**`, `domain/project/management/**`, `domain/project/api-key/**`, backend.

---

## Test Scenario

Vitest + Testing Library (jsdom). `fetch` di-stub per test (`vi.stubGlobal`); fixture JSON di `mocks/` mengikuti overview bagian 15. Test penerimaan memuat `T06-ACn` di judul `describe`/`it`.

### shared
- `getBlob`: 200 -> `Blob` dengan isi dan `contentType`; 404 envelope -> `ApiError` status 404 pesan server; 401 -> handler unauthorized dipanggil; jaringan gagal -> `ApiError NETWORK`; tanpa header CSRF (GET); `request` lama tetap lulus seluruh test-nya (regresi).
- `saveBlob`: membuat object URL, klik `<a download="payment.yaml">`, `revokeObjectURL` dipanggil; nama berbahaya `../x"y.yaml` -> `.._x_y.yaml`.
- `safeHref`: `https://a.b/x` -> sama; `http://a` ok; `javascript:alert(1)`, `JaVaScRiPt:`, `data:text/html,..`, `//evil`, `/relative`, `ftp://x`, angka, `null` -> `null`.
- `renderMarkdown` (T06-AC8): `<script>alert(1)</script>` -> di-escape sebagai teks (tidak ada elemen `script`); `<img src=x onerror=alert(1)>` -> teks, tidak ada `img`; `[x](javascript:alert(1))` -> tidak ada `href` berbahaya (anchor tanpa `href` atau teks); `[x](JAVASCRIPT:alert(1))`, `[x](data:text/html,...)`, `[x](vbscript:..)` sama; `[x](https://a.b)` -> `href` + `rel="noopener noreferrer nofollow"` + `target="_blank"`; `[x](#anchor)` dipertahankan; `![a](https://t.example/p.png)` -> teks `a` tanpa `img` (default) dan `img` bila `allowImages`; `**b**`, tabel, kode blok dirender; input 300 000 karakter dipotong + penanda.
- `Markdown.svelte`: tidak ada `script`/`onerror`/`javascript:` di DOM hasil; `inline` merender `span`; `source` null -> tidak merender apa pun.
- `Dialog` (bila dibuat di sini): sesuai test plan T04.
- Paritas kunci i18n id/en hijau.

### listspecs
- `types.test.ts`: fixture list valid lolos; `source_path: null` valid; `format: 'xml'` ditolak; `limit` hilang ditolak; `endpoint_count: -1` ditolak. `sortSpecs` stabil, tidak memutasi input. `openapiMajorMinor('3.1.0') === '3.1'`. `formatBytes(18234, 'id')`.
- `listspecs.service.test.ts`: `GET /api/projects/<id>/api-specs` (id di-`encodeURIComponent`, `a/b` -> `a%2Fb`), tanpa header CSRF; respons rusak -> `ApiError INVALID_RESPONSE`; 404 envelope -> `ApiError` 404.
- `specList.store.test.ts`: `upsert` spec baru masuk urut nama dan `total` naik; `upsert` nama sama mengganti; `rename('payment', {name:'payment-v2'})` memindah posisi; `remove` menurunkan `total`; `invalidate` menaikkan `version`; `names()`.
- `components.test.ts`: `SpecTable` merender tautan `/projects/p1/api-specs/payment`, judul, lencana `3.0`/`3.1`, jumlah endpoint, `source_path`, `updated_by_label`; nilai `<b>x</b>` di judul tampil sebagai teks. `SpecQuota` `total >= limit` -> peringatan.
- `ListSpecsView.test.ts`: loading (`aria-busy`) -> tabel; kosong -> `EmptyState` + tombol unggah memanggil `onupload`; 500 -> `ErrorState` + coba lagi memanggil ulang; 404 -> `onprojectgone`; `invalidate` memuat ulang dengan daftar lama tetap tampil; respons basi (projectId berubah cepat) diabaikan.

### uploadspecs
- `types.test.ts`: fixture upload (overview 15) dan validate lolos; `status: 'deleted'` ditolak; `line: 0` ditolak; `issues` hilang ditolak; `summary` validate dengan kunci upload ditolak.
- `model.test.ts`: `partitionPicked` (`a.YAML`, `b.yml`, `c.json` dikirim; `README.md`, `x.yaml.bak`, `Makefile` lokal `skipped NOT_SPEC_FILE`; path relatif dipertahankan); `validateNameInput` (`''` ok, `payment-v2` ok, `Bad Name`/`a/b`/`..`/81 karakter -> `INVALID_NAME`, nama + 2 file -> `NAME_WITH_MULTIPLE_FILES`); `mergeResults` + `summarize`; `issueText` memuat file, `source_path`, `reason_code`, setiap issue dengan baris/pointer dari message; `markOverwrite` hanya menandai `valid` yang namanya ada.
- `uploadspecs.service.test.ts`: `POST .../upload` dan `.../validate` dengan body `FormData` (tanpa `Content-Type` manual), header `X-Requested-With: grimoire`; part `files` memakai nama file = path relatif (`specs/payment.yaml`); `name` hanya ditambahkan bila diisi; respons rusak -> `INVALID_RESPONSE`.
- `uploadDialog.store.test.ts`: transisi fase; `pick` baru setelah `done` mereset hasil; `fail` menyimpan `formError` dan mempertahankan file.
- `components.test.ts`: `FilePicker` input folder ber-atribut `webkitdirectory`; drop file memanggil `pick`; `NameField` nonaktif untuk 2 file; `ResultTable` merender label status (teks, bukan hanya warna), label `apispec.reason.INVALID_SPEC`, pesan issue server utuh (`(baris 40, #/paths/~1pets/...)`), tombol salin memanggil `navigator.clipboard.writeText(issueText(r))`, salin gagal -> teks terseleksi + `common.copyManual`; nilai berbahaya di `reason`/`file` tampil sebagai teks.
- `UploadSpecsDialog.test.ts`:
  - T06-AC1: unggah 3 file -> tabel 3 `created`, ringkasan "3 dibuat", `onuploaded` dipanggil sekali.
  - T06-AC2: respons 4 `rejected` (`NOT_OPENAPI`, `SYNTAX`, `NOT_OPENAPI_3`, `EXTERNAL_REF`) -> 4 baris dengan alasan; "Salin semua penolakan" menyalin keempatnya.
  - Validasi saja -> `POST /validate`, status `valid`, penanda "akan menimpa" untuk nama di `existingNames`; tidak memanggil `onuploaded`.
  - Folder dengan `README.md` -> file itu tidak ada di `FormData` dan tampil `skipped` lokal.
  - Nama + 2 file -> tombol nonaktif, tanpa request. `Bad Name` -> pesan klien, tanpa request.
  - 400 `INVALID_NAME` dengan `fields.name` -> pesan di input; 400 `NO_FILES` -> `formError`; 413 -> `apispec.upload.error.tooLarge`; 404 -> `onprojectgone`; `INVALID_RESPONSE` -> `uncertain` + `onstale`; 500 -> pesan + trace id dan file tetap terpilih.
  - Selama request: Esc tidak menutup dialog, tombol `disabled`.

### getspec
- `types.test.ts`: fixture detail 3.0 dan 3.1 lolos; `document.openapi` hilang ditolak; method `get` (lowercase) ditolak; kunci tambahan di `document` dipertahankan (`passthrough`).
- `model/refs.test.ts`: `#/components/schemas/Pet`; `~1` dan `~0`; index array; `#` root; ref tak ada -> `unresolved`; rantai ref A -> B -> C diikuti; A -> B -> A -> `circular`; kunci saudara `description` (3.1) menimpa; ref `http://...` -> `unresolved` tanpa fetch (fetch spy 0 panggilan).
- `model/operations.test.ts`: `buildTagTree` urut sesuai `doc.tags` lalu kemunculan; endpoint 2 tag muncul di keduanya; tanpa tag -> grup terakhir; urutan method kanonik dalam path; `getOperation` menggabungkan parameter path-level dan override operasi (`name`+`in` sama, termasuk via `$ref`); path item `$ref` diselesaikan; servers fallback operasi > path > root > `[]`; security operasi `[]` -> kosong (tanpa auth) berbeda dengan tidak didefinisikan (pakai root); `parseOpParam('POST /payments/{id}')`, input rusak -> `null`.
- `model/schema.test.ts` (T06-AC6): 3.0 `{type:'string', nullable:true}` -> `string | null`; 3.1 `{type:['string','null']}` -> `string | null`; `{type:'array', items:{$ref:'#/components/schemas/Pet'}}` -> `array<Pet>`; `format: date-time` tampil; `enum`/`const`; `oneOf`/`allOf`; tanpa tipe -> `any`; `schemaChildren` menandai required/readOnly/writeOnly/deprecated; anak sirkular `Node.children.items -> Node` ditandai `circular` (T06-AC5 sisi tampilan); bentuk tak terduga (`properties: 5`, `required: 'x'`) tidak melempar.
- `model/search.test.ts`: cocok pada path, method, operationId, summary, tag; multi-token AND; case-insensitive; q kosong -> semua; 500 endpoint < 5 ms.
- `model/security.test.ts`, `model/examples.test.ts`: skema dipakai saja, nama tak dikenal -> `scheme: null`; `examples` dengan `$ref`; `externalValue` hanya teks; `toPrettyJson` memotong.
- `getspec.service.test.ts`: `GET /api/projects/<id>/api-specs/<name>` (nama di-encode), schema, 404 -> `ApiError` 404.
- `specDetail.store.test.ts`: `set` menghitung `tree`; `setQuery`; `replaceSpec` mengganti metadata tanpa mengubah dokumen.
- `components.test.ts`: `EndpointSidebar` merender grup tag dan tautan `?op=GET%20%2Fpayments`, `aria-current` pada terpilih, deprecated dicoret + label; `OperationView` menampilkan parameter per lokasi, request body per media type, response per kode (`default` terakhir), security dan servers efektif, TIDAK ada tombol "Try it out"/form/`button` eksekusi (query `getByRole('button', {name: /try|kirim|execute/i})` kosong); `SchemaTree` hanya level 0 terbuka, klik membuka anak, sirkular menampilkan lencana tanpa tombol buka, render spec 500 endpoint + skema dalam tidak membuat > 1 operasi di DOM.
- `security.test.ts` (T06-AC8): render `detail-xss.ts` (deskripsi `info`, tag, operasi, parameter, skema, contoh berisi `<script>`, `<img onerror>`, `javascript:`; `servers[0].url: javascript:alert(1)`, `externalDocs.url: javascript:..`) -> DOM tidak mengandung elemen `script`/`img`/atribut `on*`, tidak ada `href` diawali `javascript:`; URL server tampil sebagai teks; `window.alert` spy tidak dipanggil.
- `SpecDetailView.test.ts`: loading -> ringkasan API (tanpa `op`); `selectedOp='POST /payments'` -> `OperationView` operasi itu dan fokus pindah ke judul; `op` tidak dikenal -> `opNotFound`; spec webhooks-only -> `noEndpoints` + bagian Webhooks; 404 -> `notFound` + tautan daftar; 500 -> `ErrorState` + coba lagi; ganti `name` membatalkan request lama.

### downloadspec / renamespec / deletespec
- `downloadspec.service.test.ts`: `GET .../payment/raw`, nama file `payment.yaml` / `petstore.json` sesuai format; `DownloadSpecButton.test.ts`: loading, `saveBlob` dipanggil, 404 -> toast + `onspecgone`.
- `renamespec`: `validateName` (`''` required, `Payment` invalid, `payment-v2` ok); service `PATCH` dengan body `{name}` + CSRF; dialog: tidak berubah -> Simpan nonaktif; 409 `NAME_TAKEN` -> pesan di input; 400 `fields.name` -> pesan server di input; 404 -> `onspecgone` + toast; sukses -> `onrenamed(spec)` + toast; Esc nonaktif saat submit.
- `deletespec`: service `DELETE` + CSRF, 204 tanpa schema; dialog: pesan menyebut nama; 204 -> `ondeleted('payment')` + toast; 404 -> `ondeleted` + toast `alreadyGone`; 500 -> pesan di dialog, coba lagi berhasil.

### app
- `router.test.ts`: route tiga segmen workspace cocok dan tidak menelan `/projects/p1/api-specs` (dua segmen setelah `projects`).
- `tabs.test.ts`: entri `api-specs` di posisi benar, `supportsItem`.
- `ProjectWorkspaceView.test.ts`: `item` diteruskan; `item` untuk `members` -> `project.tab.notFound`.
- `ApiSpecsTab.test.ts`: `item=null` -> daftar + buka dialog unggah; `onuploaded` -> daftar dimuat ulang; `item='payment'` -> detail dengan tombol Unduh/Ubah nama/Hapus; `onrenamed` -> `navigate('/projects/p1/api-specs/payment-v2', {replace:true})` dan store daftar ter-rename; `ondeleted` -> `navigate('/projects/p1/api-specs', {replace:true})` dan item hilang dari store; `onspecgone` -> `invalidate` + navigate daftar.
- `npm run depcruise`: tidak ada import dari `domain/project/api-spec/*/` ke feature lain, tidak ada import `app/` dari domain, `app/` hanya mengimpor `index.ts`.

---

## E2E Scenarios

`frontend/e2e/project-api-spec.spec.ts` (Playwright, terhadap binary; butuh T01, T02, T03, T06 backend + FE). Setiap test membuat project sendiri lewat helper T03; user developer anggota, developer bukan anggota, dan super_admin dari `global-setup`.

Positif:
1. **Unggah folder (T06-AC1)**: anggota membuka `/projects/<id>/api-specs` (kosong: "Belum ada API spec") -> Unggah spec -> pilih folder berisi `auth.yaml`, `payment.yaml`, `notification.yaml`, `README.md` -> Unggah -> ringkasan "3 dibuat, 1 dilewati" (`README.md` "Bukan file spec") -> tutup -> daftar menampilkan 3 baris urut nama dengan judul, lencana versi OpenAPI, versi API, jumlah endpoint, kuota "3 / 100 spec".
2. **Timpa (T06-AC1)**: unggah `payment-updated.yaml` dengan nama `payment` (field nama) -> baris "Diperbarui"; buka detail `payment` -> `POST /payments/{id}/refund` ada di pohon, `GET /payments` tidak ada lagi; buka `?op=GET%20%2Fpayments` -> "Endpoint tidak ada di spec ini".
3. **Unggah identik**: unggah ulang `auth.yaml` -> "Tidak berubah"; waktu "Diperbarui" di daftar tidak berubah.
4. **Validasi saja**: pilih `payment.yaml` + `newspec.yaml` -> Validasi saja -> 2 "Valid", `payment` bertanda "Akan menimpa"; tutup -> daftar tidak berubah (tetap 3 spec).
5. **Detail dan renderer (T06-AC5/AC6 tampilan)**: buka `payment` -> pohon tag -> endpoint dengan lencana method; ketik `refund` di pencarian -> hanya endpoint refund; klik `POST /payments` -> parameter, request body `application/json`, skema `Payment` dapat dibuka -> `Customer` -> `Address`; skema `Node.children` menampilkan "Referensi sirkular ke Node"; properti `nullable` tampil `string | null`; operasi deprecated berlabel "Usang"; bagian Security menampilkan skema bearer; tidak ada tombol eksekusi request. Reload halaman dengan URL yang sama -> operasi yang sama terbuka (deep link). Back -> kembali ke ringkasan.
6. **OpenAPI 3.1 (T06-AC6)**: unggah `petstore-3.1.json` -> buka detail -> lencana `3.1`, `type: ["string","null"]` tampil `string | null`.
7. **Unduh**: tombol Unduh di detail `payment` -> event download bernama `payment.yaml`, isinya byte-identik dengan fixture yang terakhir diunggah.
8. **Ubah nama**: `payment` -> `payment-v2` -> toast sukses, URL menjadi `/projects/<id>/api-specs/payment-v2`, header menampilkan nama baru; buka URL lama -> "Spec tidak ditemukan" + tautan ke daftar.
9. **Hapus**: detail `notification` -> Hapus -> konfirmasi -> kembali ke daftar, `notification` hilang, kuota turun; spec lain tetap.
10. **Super Admin bukan anggota**: membuka tab API Specs project orang lain, mengunggah `auth.yaml` -> "Dibuat".
11. **Salin alasan**: setelah negatif 1, tombol salin di baris `swagger-2.0.yaml` -> clipboard berisi nama file, `INVALID_SPEC`, dan pesan issue dengan baris.

Negatif:
1. **Penolakan dengan alasan (T06-AC2)**: listener lokal dijalankan; unggah `package.json`, `broken.yaml`, `swagger-2.0.yaml`, `ref-http.yaml` (ke listener) -> 4 baris "Ditolak" dengan label `Bukan OpenAPI`, `Sintaks rusak` (memuat "baris"), `Swagger 2.0 tidak didukung`, `Referensi eksternal tidak didukung` (memuat pointer `#/...`); listener menerima 0 request; daftar spec tidak berubah.
2. **Bukan file spec**: pilih `notes.txt` saja -> tombol Unggah nonaktif, pesan "Tidak ada file .yaml, .yml, atau .json".
3. **File terlalu besar**: `huge.yaml` 6 MB + `auth.yaml` -> `huge.yaml` "Ditolak: Ukuran file melebihi batas", `auth.yaml` tetap diproses.
4. **Nama duplikat dalam unggahan**: folder `folder/` (`v1/payment.yaml`, `v2/payment.yaml`) -> satu "Dibuat", satu "Ditolak: Nama duplikat dalam unggahan ini".
5. **Nama tidak valid / untuk banyak file**: satu file + nama `Bad Name` -> pesan validasi, tanpa request (dicek dengan `page.waitForRequest` tidak terjadi); dua file -> field nama nonaktif dengan penjelasan.
6. **Ubah nama bentrok**: ubah `auth` menjadi `payment-v2` (sudah ada) -> "Nama ini sudah dipakai spec lain di project ini", dialog tetap terbuka.
7. **Batas spec**: `page.route` men-stub respons upload dengan baris `rejected SPEC_LIMIT_REACHED` dan list dengan `total == limit` -> baris "Batas spec per project tercapai" dan peringatan kuota tampil (batas nyata diuji di integrasi backend; binary e2e memakai default 100).
8. **Bukan anggota (T06-AC7)**: developer bukan anggota membuka `/projects/<idLain>/api-specs` dan `/projects/<idLain>/api-specs/payment` -> "Project tidak ditemukan" (T03), tidak ada data spec di DOM.
9. **XSS (T06-AC8)**: unggah `xss.yaml`, buka detail dan operasinya -> teks `<script>alert(1)</script>` tampil sebagai teks; handler `page.on('dialog')` tidak terpanggil; tidak ada elemen `img` dari deskripsi; tautan `javascript:` tidak memiliki `href` aktif; URL server `javascript:alert(1)` tampil sebagai teks.
10. **CSRF**: `request.post('/api/projects/<id>/api-specs/upload', { multipart })` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; `request.delete(...)` tanpa header -> 403.
11. **Spec dihapus di tab lain**: buka detail `auth`, hapus `auth` lewat API helper, klik Unduh -> toast "Spec sudah tidak ada" dan kembali ke daftar tanpa `auth`.

---

## Request/Response body

Format error seragam (rule 11.1), diparse `ErrorEnvelope` di `shared/validation/error.ts`; `details.reason` dibaca `reasonOf`, `details.fields` dibaca `fieldErrorsOf`:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

Error yang dibangun FE sendiri (tidak dari server) memakai `ApiError` dengan `code` `NETWORK` (status 0) atau `INVALID_RESPONSE` (status asli, `details` = issue Zod).

### GET /api/projects/{projectID}/api-specs (`listSpecs`)

Response 200 (`ApiSpecList`):

```json
{
  "items": [
    {
      "name": "auth",
      "title": "Auth API",
      "openapi_version": "3.1.0",
      "api_version": "2.0.0",
      "format": "yaml",
      "source_path": "specs/auth.yaml",
      "size_bytes": 18234,
      "endpoint_count": 5,
      "created_at": "2026-10-23T09:00:00Z",
      "created_by_label": "Budi",
      "updated_at": "2026-10-23T09:00:00Z",
      "updated_by_label": "Budi"
    },
    {
      "name": "payment",
      "title": "Payment API",
      "openapi_version": "3.0.3",
      "api_version": "1.4.0",
      "format": "yaml",
      "source_path": null,
      "size_bytes": 40211,
      "endpoint_count": 2,
      "created_at": "2026-10-23T09:00:00Z",
      "created_by_label": "Budi",
      "updated_at": "2026-10-24T10:00:00Z",
      "updated_by_label": "Sari"
    }
  ],
  "total": 2,
  "limit": 100
}
```

Error: 401 (handler global -> login), 404 (non-anggota / project tidak ada / id bukan uuid; FE -> `onprojectgone`):

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### POST /api/projects/{projectID}/api-specs/upload (`uploadSpecs`)

Request: `multipart/form-data` dibangun FE dengan `FormData` (boundary dari browser), header `X-Requested-With: grimoire` (otomatis). Nama file part = path relatif (`webkitRelativePath`) agar backend menyimpan `source_path`:

```
--b
Content-Disposition: form-data; name="files"; filename="specs/payment.yaml"
Content-Type: application/yaml

openapi: 3.0.3
...
--b
Content-Disposition: form-data; name="files"; filename="specs/package.json"
Content-Type: application/json

{"name":"x","version":"1.0.0"}
--b--
```

Opsional (hanya bila tepat 1 file): part teks `name` -> `payment-v2`.

Response 200 (`ApiSpecUploadResult`):

```json
{
  "results": [
    {
      "file": "payment.yaml",
      "source_path": "specs/payment.yaml",
      "name": "payment",
      "status": "updated",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "3.0.3",
      "title": "Payment API",
      "api_version": "1.4.0",
      "endpoint_count": 12
    },
    {
      "file": "package.json",
      "source_path": "specs/package.json",
      "name": "package",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "bukan dokumen OpenAPI",
      "issues": [
        { "code": "NOT_OPENAPI", "message": "bukan dokumen OpenAPI", "pointer": "#", "line": 1 }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    },
    {
      "file": "common-ref.yaml",
      "source_path": "specs/common-ref.yaml",
      "name": "common-ref",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "referensi eksternal tidak didukung dan tidak di-fetch: http://example.com/common.yaml (baris 40, #/paths/~1pets/get/responses/200/content/application~1json/schema)",
      "issues": [
        {
          "code": "EXTERNAL_REF",
          "message": "referensi eksternal tidak didukung dan tidak di-fetch: http://example.com/common.yaml (baris 40, #/paths/~1pets/get/responses/200/content/application~1json/schema)",
          "pointer": "#/paths/~1pets/get/responses/200/content/application~1json/schema",
          "line": 40
        }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    },
    {
      "file": "huge.yaml",
      "source_path": "huge.yaml",
      "name": null,
      "status": "rejected",
      "reason_code": "FILE_TOO_LARGE",
      "reason": "ukuran file melebihi batas 5242880 byte",
      "issues": [],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "created": 0, "updated": 1, "unchanged": 0, "skipped": 0, "rejected": 3 }
}
```

Hasil lokal FE (tidak dikirim ke server, K8) yang digabung ke tabel memakai bentuk yang sama:

```json
{ "file": "README.md", "source_path": "specs/README.md", "name": null, "status": "skipped", "reason_code": "NOT_SPEC_FILE", "reason": null, "issues": [], "openapi_version": null, "title": null, "api_version": null, "endpoint_count": null }
```

Error request (FE -> `formError`/`nameError`/navigasi):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "field name hanya boleh dipakai saat mengunggah tepat satu file", "details": { "reason": "NAME_WITH_MULTIPLE_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter", "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "request harus multipart/form-data", "details": { "reason": "NOT_MULTIPART" } }, "trace_id": "..." }
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran upload melebihi batas" }, "trace_id": "..." }
{ "error": { "code": "CSRF_REQUIRED", "message": "..." }, "trace_id": "..." }
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

### POST /api/projects/{projectID}/api-specs/validate (`validateSpecs`)

Request sama dengan upload. Response 200 (`ApiSpecValidateResult`):

```json
{
  "results": [
    {
      "file": "auth.yaml",
      "source_path": "auth.yaml",
      "name": "auth",
      "status": "valid",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "3.1.0",
      "title": "Auth API",
      "api_version": "2.0.0",
      "endpoint_count": 5
    },
    {
      "file": "legacy.yaml",
      "source_path": "legacy.yaml",
      "name": "legacy",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1",
      "issues": [
        { "code": "NOT_OPENAPI_3", "message": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)", "pointer": "#/swagger", "line": 1 }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "valid": 1, "skipped": 0, "rejected": 1 }
}
```

Error: sama dengan upload.

### GET /api/projects/{projectID}/api-specs/{name} (`getSpec`)

Response 200 (`ApiSpecDetail`; `document` mempertahankan `$ref` internal dan urutan kunci file asli; FE yang menelusuri):

```json
{
  "spec": {
    "name": "payment",
    "title": "Payment API",
    "openapi_version": "3.0.3",
    "api_version": "1.4.0",
    "format": "yaml",
    "source_path": "specs/payment.yaml",
    "size_bytes": 40211,
    "endpoint_count": 2,
    "created_at": "2026-10-23T09:00:00Z",
    "created_by_label": "Budi",
    "updated_at": "2026-10-24T10:00:00Z",
    "updated_by_label": "Sari"
  },
  "endpoints": [
    { "method": "GET", "path": "/payments", "operation_id": "listPayments", "summary": "Daftar pembayaran", "tags": ["payments"], "deprecated": false },
    { "method": "POST", "path": "/payments", "operation_id": "createPayment", "summary": "Buat pembayaran", "tags": ["payments"], "deprecated": false }
  ],
  "document": {
    "openapi": "3.0.3",
    "info": { "title": "Payment API", "version": "1.4.0", "description": "Deskripsi **Markdown** (disanitasi di FE)" },
    "servers": [ { "url": "https://api.example.com" } ],
    "tags": [ { "name": "payments", "description": "Operasi pembayaran" } ],
    "paths": {
      "/payments": {
        "get": {
          "operationId": "listPayments",
          "tags": ["payments"],
          "summary": "Daftar pembayaran",
          "parameters": [ { "name": "limit", "in": "query", "schema": { "type": "integer", "nullable": true } } ],
          "responses": {
            "200": { "description": "OK", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Payment" } } } } }
          }
        },
        "post": {
          "operationId": "createPayment",
          "tags": ["payments"],
          "summary": "Buat pembayaran",
          "security": [ { "bearer": [] } ],
          "requestBody": { "$ref": "#/components/requestBodies/Payment" },
          "responses": { "201": { "description": "Created" }, "default": { "description": "Error" } }
        }
      }
    },
    "components": {
      "schemas": {
        "Payment": { "type": "object", "required": ["amount"], "properties": { "amount": { "type": "integer" }, "customer": { "$ref": "#/components/schemas/Customer" } } },
        "Customer": { "type": "object", "properties": { "name": { "type": "string" }, "address": { "$ref": "#/components/schemas/Address" } } },
        "Address": { "type": "object", "properties": { "city": { "type": "string", "nullable": true } } },
        "Node": { "type": "object", "properties": { "children": { "type": "array", "items": { "$ref": "#/components/schemas/Node" } } } }
      },
      "requestBodies": { "Payment": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Payment" } } } } },
      "securitySchemes": { "bearer": { "type": "http", "scheme": "bearer" } }
    }
  }
}
```

Error: 404 non-anggota / project tidak ada / id bukan uuid (`project tidak ditemukan`) atau spec tidak ada (`spec tidak ditemukan`); FE memperlakukan keduanya sebagai "spec tidak ditemukan" di halaman detail (K7):

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### GET /api/projects/{projectID}/api-specs/{name}/raw (`downloadSpec`)

Response 200 (dibaca sebagai `Blob` oleh `getBlob`), header:

```
Content-Type: application/yaml; charset=utf-8
Content-Disposition: attachment; filename="payment.yaml"
X-Content-Type-Options: nosniff
Cache-Control: no-store
```

Body: teks spec asli (tanpa BOM). Error tetap JSON seragam (diparse `getBlob`):

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### PATCH /api/projects/{projectID}/api-specs/{name} (`renameSpec`)

Request (header `X-Requested-With: grimoire`, `Content-Type: application/json`; `{name}` di path = nama lama):

```json
{ "name": "payment-v2" }
```

Response 200 (`{ spec: ApiSpec }`):

```json
{
  "spec": {
    "name": "payment-v2",
    "title": "Payment API",
    "openapi_version": "3.0.3",
    "api_version": "1.4.0",
    "format": "yaml",
    "source_path": "specs/payment.yaml",
    "size_bytes": 40211,
    "endpoint_count": 2,
    "created_at": "2026-10-23T09:00:00Z",
    "created_by_label": "Budi",
    "updated_at": "2026-10-25T08:00:00Z",
    "updated_by_label": "Sari"
  }
}
```

Error:

```json
{ "error": { "code": "CONFLICT", "message": "nama spec sudah dipakai di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter", "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "wajib diisi" } } }, "trace_id": "..." }
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "..." }
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

### DELETE /api/projects/{projectID}/api-specs/{name} (`deleteSpec`)

Request tanpa body (header `X-Requested-With: grimoire`). Response 204 tanpa body. Error:

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
