# T05 project/knowledge: Frontend Plan (Svelte)

Tiket: `docs/tickets/T05-project-knowledge.md`. Plan bersama backend: `overview.md` (D1-D12, bagian 4 DTO/reason, 13 body, 14 OpenAPI). Kontrak per endpoint: `usecase-uploadfiles.md`, `usecase-createfile.md`, `usecase-updatefile.md`, `usecase-getfile.md`, `usecase-listtree.md`, `usecase-deletefile.md`, `usecase-deletefolder.md`, `usecase-movefile.md`, `usecase-search.md`. `buslistfiles`, `busreadfile`, `bussearch`, `countfiles` adalah handler bus tanpa UI.

Dokumen ini adalah **satu-satunya plan frontend** T05 (frontend tidak dipecah per usecase). Bila E2E di plan usecase menyebut perilaku UI yang berbeda dengan dokumen ini, **ikuti dokumen ini**; selisihnya ada di bagian "Keputusan".

Konteks yang dimuat: tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: `rules.txt` (12, 13), `docs/tickets/README.md` (bagian Frontend, katalog aksi), tiket T05, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (tidak ada file `*knowledge-hub-design.md`; Grimoire = Knowledge Hub), `docs/superpowers/plans/project-management/frontend-plan.md` (workspace `/projects/:projectID/:tab`, `app/workspace/tabs.ts`, `ProjectContext`, `reasonOf`, `shared/utils/text.ts`, `Textarea`), `docs/superpowers/plans/project-api-key/frontend-plan.md` (pola sisip tab), `docs/superpowers/plans/project-api-spec/overview.md` (upload `.yaml/.json` T06 di `/api/projects/{id}/api-specs/upload`), dan kode FE di `main`: `shared/services/http.ts` (`get/post/put/del`, `ApiError`, CSRF otomatis, handler 401), `shared/validation/error.ts` (`ErrorEnvelope`, `KnownErrorCode`, `reasonOf`), `shared/components/*` (`Button`, `Input`, `Dialog`, `Spinner`, `EmptyState`, `ErrorState`, `Toast`), `shared/stores/{session,i18n,toast}.ts`, `shared/utils/datetime.ts`, `app/routes/{router,guard,index}.ts`, `.dependency-cruiser.cjs`, `scripts/test-arch.mjs`, `playwright.config.ts`, `e2e/helpers/auth.ts`, dan CSP backend `infra/middleware/securityheaders.go` (`script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:`).

---

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (dokumen ini: frontend saja; backend di `overview.md` + `usecase-*.md`) |
| Domain | project |
| Module | knowledge (`frontend/src/domain/project/knowledge/`) |
| Feature FE | `base` (rule murni + schema bersama, tanpa I/O), `listtree`, `getfile`, `createfile`, `updatefile`, `uploadfiles`, `movefile`, `deletefile`, `deletefolder`, `search` |
| Route UI | `/projects/:projectID/knowledge` (tab workspace T03), state di query: `?path=` (file), `?dir=` (folder), `?mode=edit`, `?new=1`, `?q=` + `?in=` (pencarian) |
| Akses UI | Route `meta: { auth: true, layout: 'app' }` (dari T03). Semua level `member|owner|admin` boleh semua aksi T05, jadi tidak ada tombol yang disembunyikan per peran. Otorisasi di backend (rule 12.7) |
| Kontrak API | `api/modules/project/knowledge.yml` (`info.version: 0.1.0`), schema `KnowledgeFile`, `KnowledgeTree`, `KnowledgeUploadResult`, `KnowledgeSearchResult`, `KnowledgeConflictDetails`, `KnowledgeDeleteFolderResult`; `Error` dari `api/modules/infra/foundation.yml` |
| Prasyarat | T00, T01 FE, T02 FE (merged). **T03 FE wajib di `main`** (`app/workspace/tabs.ts`, `app/views/ProjectWorkspaceView.svelte`, `domain/project/management/getproject/index.ts` dengan `ProjectContext`, `shared/utils/text.ts`, `shared/components/Textarea.svelte`). Backend T05 F + U1-U3, U5-U10 |
| Dependensi npm baru | `marked@17.0.6`, `dompurify@3.4.16`, `mermaid@11.17.2`, `codemirror@6.0.2`, `@codemirror/lang-markdown@6.5.2`, `@codemirror/state@6.7.6`, `@codemirror/view@6.43.13` (versi pasti, semuanya dirilis > 2 minggu sebelum 2026-10-20; lihat K3) |

Endpoint yang dipanggil frontend (prefix `/api`, tanpa versi di path; versi kontrak `0.1.0`; mutasi membawa `X-Requested-With: grimoire` otomatis):

| Endpoint | Method | Version | Service FE | Akses (backend) |
|---|---|---|---|---|
| `/api/projects/{projectID}/knowledge/tree?prefix=` | GET | 0.1.0 | `listtree/services/listtree.service.ts` (`getTreeLevel`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/files?path=` | GET | 0.1.0 | `getfile/services/getfile.service.ts` (`getFile`); juga dipakai `updatefile` untuk muat ulang setelah konflik lewat `getfile/index.ts` | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/files` | POST | 0.1.0 | `createfile/services/createfile.service.ts` (`createFile`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/files?path=` | PUT | 0.1.0 | `updatefile/services/updatefile.service.ts` (`updateFile`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/files?path=` | DELETE | 0.1.0 | `deletefile/services/deletefile.service.ts` (`deleteFile`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/folders?path=` | DELETE | 0.1.0 | `deletefolder/services/deletefolder.service.ts` (`deleteFolder`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/files/move` | POST | 0.1.0 | `movefile/services/movefile.service.ts` (`moveFile`) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/upload` | POST (multipart) | 0.1.0 | `uploadfiles/services/uploadfiles.service.ts` (`uploadKnowledgeFiles`, lewat XHR untuk progres) | anggota, super_admin |
| `/api/projects/{projectID}/knowledge/search?q=&prefix=&limit=` | GET | 0.1.0 | `search/services/search.service.ts` (`searchKnowledge`) | anggota, super_admin |

Bus `project.knowledge.*` dan `count-files` tidak disentuh frontend. Frontend tidak menambah endpoint. Endpoint T06 (`/api-specs/upload`) TIDAK dipanggil oleh module knowledge; upload campuran disusun di `app/` lewat registri target (A3).

---

## Summary Plan

Tab **Knowledge** di workspace project (`/projects/:projectID/knowledge`, menjadi tab pertama dan default) terdiri dari:

1. **Panel kiri**: kotak cari cepat + pohon folder/file lazy (`tree?prefix=` per folder yang dibuka, cache per prefix, `file_count` di folder). Saat kotak cari terisi, pohon diganti daftar hasil pencarian dengan cuplikan tersorot (segmen `{text, highlight}` dirender sebagai `<mark>`, bukan HTML). Tombol **Upload** dan **File baru** di atas pohon. Tiap node punya menu aksi (salin path, file baru di sini, upload ke sini, pindah/ganti nama, hapus file/folder).
2. **Panel kanan** (bergantung URL):
   - `?path=` -> **viewer**: breadcrumb (segmen folder bisa diklik), tombol salin path, judul, metadata (ukuran, diubah oleh/kapan), Markdown tersanitasi + Mermaid (`securityLevel: 'strict'`), tombol Edit, Pindah, Hapus. File `.txt` ditampilkan sebagai teks pra-format.
   - `?path=&mode=edit` -> **editor** CodeMirror + pratinjau berdampingan (tab Tulis/Pratinjau di layar sempit), indikator "belum disimpan", Ctrl/Cmd+S, draf tersimpan di `sessionStorage` (selamat dari 401/reload), penanganan **409**: dialog konflik dengan "Muat versi terbaru", "Bandingkan" (diff baris), dan dari layar banding "Timpa dengan versi saya" (eksplisit).
   - `?new=1[&dir=]` -> **file baru**: input path (validasi klien mencerminkan `base/path.go`), editor + pratinjau, Simpan -> POST.
   - `?dir=` -> **folder**: daftar isi folder dengan aksi folder (file baru, upload ke sini, hapus folder).
   - kosong -> ringkasan project knowledge atau `EmptyState` (belum ada file: CTA Upload / File baru).
3. **Dialog Upload**: pilih folder (`webkitdirectory`), pilih file, atau drag-drop (rekursif lewat `webkitGetAsEntry`). Layar ringkasan pra-upload (jumlah file per tujuan, ukuran total, yang dilewati karena tersembunyi, yang tidak didukung), progres (byte terkirim via XHR, lalu "memproses"), batal, dan laporan akhir per file dengan filter (semua, gagal, ditimpa, dibuat, tidak berubah, dilewati). Upload campuran: `.md/.markdown/.txt` ke T05, `.yaml/.yml/.json` ke target tambahan yang didaftarkan `app/` (T06 kelak); tanpa target T06 file itu dilaporkan "tidak didukung".
4. **Dialog** pindah/ganti nama, hapus file, hapus folder (menampilkan jumlah file), semuanya dengan konfirmasi.

Pola lapisan mengikuti T02/T03: `services/` memanggil `shared/services/http.ts` (atau `shared/services/upload.ts` untuk multipart berprogres) dengan schema Zod dan melempar `ApiError` apa adanya; `stores/` state UI per instance tanpa fetch; `components/` presentasi; `views/` satu-satunya pemanggil service; `index.ts` satu-satunya pintu keluar. Feature tidak meng-import router: navigasi lewat callback ke `app/`. URL adalah sumber kebenaran untuk file/folder/mode/pencarian, sehingga reload, Back, dan deep link bekerja.

Keamanan (AC5, rule 12.6, 13.6):
- Satu-satunya `{@html}` ada di `shared/components/MarkdownView.svelte`, dan masukannya selalu keluaran `renderMarkdown()` (marked tanpa HTML mentah -> DOMPurify dengan allowlist). Tautan `javascript:`/`data:`/`vbscript:` dibuang (teks saja). Gambar tidak dimuat (non-goal; ditampilkan sebagai teks alt).
- Mermaid dimuat lazy, `startOnLoad: false`, `securityLevel: 'strict'`, `htmlLabels: false`; SVG hasilnya disanitasi ulang DOMPurify profil SVG sebelum disisipkan. Gagal render -> blok kode teks + pesan.
- Semua nilai server lain (path, judul, label, cuplikan, alasan) dirender sebagai teks.
- 404 knowledge diperlakukan seragam; bila project ternyata hilang/akses dicabut, `ctx.reload()` T03 yang menampilkan "Project tidak ditemukan".

---

## Plan

Urutan: FE-0 (prasyarat bersama) -> K0 (`knowledge/base`) -> K1..K9 feature (boleh paralel; tidak saling import internals, hanya `base/index.ts` dan `getfile/index.ts` seperti dicatat) -> A (komposisi `app/`) -> E (e2e) -> V (verifikasi). Semua langkah TDD: test vitest ditulis lebih dulu.

### FE-0. Prasyarat bersama (`frontend/src/shared/`, generik, tanpa pengetahuan domain)

1. **Cek prasyarat T03 FE** (`app/workspace/tabs.ts`, `app/views/ProjectWorkspaceView.svelte`, `domain/project/management/getproject/index.ts`, `shared/utils/text.ts`, `shared/components/Textarea.svelte`). Bila belum ada di `main`, berhenti dan laporkan; jangan membuat ulang workspace T03 di PR ini.
2. **npm** (`frontend/package.json`, `package-lock.json`): tambah dependensi versi pasti di tabel Metadata (`npm install --save-exact`). Tidak ada dependensi lain.
3. **`shared/services/http.ts`** (ubah, aditif): ekspor dua helper yang kini privat agar dipakai ulang tanpa duplikasi:
   ```ts
   export function toApiError(status: number, bodyText: string, requestId?: string): ApiError; // parse ErrorEnvelope, fallback INTERNAL
   export function notifyUnauthorized(): void; // = handleUnauthorized() yang sudah ada
   ```
   `request()` memakai keduanya (perilaku tidak berubah; test `http.test.ts` yang ada tetap lulus).
4. **`shared/services/upload.ts`** (baru): multipart dengan progres (fetch tidak memberi progres unggah).
   ```ts
   export interface UploadProgress { loaded: number; total: number; phase: 'sending' | 'processing' }
   export function postMultipart<T>(url: string, form: FormData, o: {
     schema: z.ZodType<T>; signal?: AbortSignal; onProgress?: (p: UploadProgress) => void;
   }): Promise<T>;
   ```
   - `XMLHttpRequest`, `withCredentials` same-origin, header `X-Requested-With: grimoire` dan `Accept: application/json`; tidak mengatur `Content-Type` (browser menulis boundary). URL wajib relatif same-origin (aturan sama `buildUrl`).
   - `xhr.upload.onprogress` -> `phase: 'sending'`; `xhr.upload.onload` -> `phase: 'processing'`.
   - Sukses: parse JSON + `schema.safeParse`; gagal -> `ApiError INVALID_RESPONSE`. Error HTTP -> `toApiError`; 401 -> `notifyUnauthorized()`. `onerror` -> `ApiError NETWORK`. `signal.abort()` -> `xhr.abort()` dan reject `DOMException('AbortError')`.
5. **`shared/utils/markdown/`** (baru):
   - `render.ts`: `renderMarkdown(src: string, o?: { resolveLink?: (href: string) => string | null }): { html: string; mermaid: string[] }`.
     - `marked` instance lokal (bukan global) `{ gfm: true, breaks: false, async: false }`, renderer khusus: `html` token -> di-escape sebagai teks (tidak ada HTML mentah); `image` -> `<span class="md-img-alt">[alt]</span>` (gambar non-goal, CSP `img-src 'self' data:`); `code` dengan lang `mermaid` -> `<div class="md-mermaid" data-mermaid-index="i"><pre><code>escaped</code></pre></div>` dan kode dicatat di `mermaid[i]`; `link` -> href diperiksa `isSafeHref` (hanya `http:`, `https:`, `mailto:`, `#...`, atau relatif tanpa skema); tidak aman -> teks saja; relatif dan `resolveLink` ada -> href = hasil `resolveLink` (null -> teks saja); eksternal -> `target="_blank" rel="noopener noreferrer nofollow"`. Heading id diberi awalan `md-` (cegah DOM clobbering).
     - Hasil `marked` dilewatkan `sanitizeHtml` sebelum dikembalikan. Fungsi ini satu-satunya sumber HTML untuk `{@html}`.
   - `sanitize.ts`: `sanitizeHtml(html)` dengan DOMPurify: `ALLOWED_TAGS` (p, h1-h6, ul, ol, li, blockquote, pre, code, em, strong, del, a, table, thead, tbody, tr, th, td, hr, br, span, div, input[type=checkbox disabled] untuk task list), `ALLOWED_ATTR` (href, title, target, rel, id, class, data-mermaid-index, align, type, checked, disabled), `ALLOWED_URI_REGEXP = /^(?:https?:|mailto:|#|\/(?!\/)|\.{0,2}\/|[^:]*$)/i`, `FORBID_TAGS` (style, script, iframe, object, embed, form, img, svg, math), `FORBID_ATTR` (style, on*); hook `uponSanitizeAttribute` memaksa input checkbox `disabled`. `sanitizeSvg(svg)` profil `{ svg: true, svgFilters: true }`, `FORBID_TAGS: ['foreignObject', 'script']`, tanpa atribut `on*`/`href` ber-`javascript:`.
   - `mermaid.ts`: `renderMermaid(code: string, id: string): Promise<{ ok: true; svg: string } | { ok: false }>`; `import('mermaid')` lazy sekali (cache promise); `initialize({ startOnLoad: false, securityLevel: 'strict', flowchart: { htmlLabels: false }, theme: 'neutral' })` sekali; hasil `render` -> `sanitizeSvg`. Kode > 50 000 karakter -> `{ ok: false }` tanpa render.
   - `links.ts`: `isSafeHref(href)`, `resolveRelativePath(baseDir: string, href: string): string | null` (gabung `baseDir` + href relatif, `..` dihitung, keluar dari root -> null, buang `#frag`/`?q`; tidak memvalidasi ekstensi).
6. **`shared/components/MarkdownView.svelte`** (baru): props `source: string`, `resolveLink?`, `mermaidLabel` (teks aksesibel). `$derived` `renderMarkdown(source, { resolveLink })`; `{@html rendered.html}` (komentar: "satu-satunya @html; masukan sudah lewat sanitizeHtml"). `$effect` setelah render: untuk tiap `.md-mermaid[data-mermaid-index]` panggil `renderMermaid` dan ganti isi dengan SVG (container `role="img"` + `aria-label`); gagal -> biarkan `<pre>` + catatan `md.mermaidError`. Render dibatalkan (token) bila `source` berubah sebelum selesai. Gaya prose di `app.css` (kelas `.md-body`).
7. **`shared/components/MarkdownEditor.svelte`** (baru): props `value` (bindable), `label`, `readonly?`, `onsave?()`, `disabled?`. Render `<textarea>` dulu (dapat dipakai segera dan di test), lalu `import('./markdownEditor/codemirror.ts')` lazy membuat `EditorView` (`basicSetup`, `markdown()`, line wrapping, keymap `Mod-s` -> `onsave`) menggantikan textarea; sinkron dua arah dengan `value`. Gagal muat CodeMirror -> tetap textarea (Mod-s tetap ditangani). `shared/components/markdownEditor/codemirror.ts` berisi satu-satunya import `codemirror`/`@codemirror/*`.
8. **`shared/utils/clipboard.ts`** (baru): `copyText(text): Promise<boolean>` (`navigator.clipboard.writeText`, fallback `document.execCommand('copy')` lewat textarea sementara; false bila keduanya gagal).
9. **`shared/utils/bytes.ts`** (baru): `utf8Length(s)` (`new TextEncoder().encode(s).length`) dan `formatBytes(n, locale)` (B/KB/MB, basis 1024).
10. **`shared/components/index.ts`**: ekspor `MarkdownView`, `MarkdownEditor`.
11. **i18n** `shared/i18n/id.json` dan `en.json`: kunci di tabel "Kunci i18n" (paritas dijaga `i18n-keys.test.ts`).
12. **`app.css`**: gaya `.md-body` (prose, tabel, kode, `mark`), `.md-mermaid`, layout dua panel.

### K0. `frontend/src/domain/project/knowledge/base/` (rule murni + schema bersama; tanpa services/stores/views)

Folder ini mencerminkan `backend/.../knowledge/base/` dan diimpor feature lain HANYA lewat `base/index.ts` (lolos aturan `no-cross-feature-internals`). Tidak ada fetch, store, atau komponen.

**`base/types.ts`**
```ts
export const VERSION_RE = /^[0-9a-f]{64}$/;
export const UserRefSchema = z.object({ id: z.string(), label: z.string() });
export const KnowledgeFileSchema = z.object({
  path: z.string().min(1), title: z.string(), content: z.string(),
  size_bytes: z.number().int().nonnegative(), version: z.string().regex(VERSION_RE),
  created_at: z.string().datetime({ offset: true }), created_by: UserRefSchema,
  updated_at: z.string().datetime({ offset: true }), updated_by: UserRefSchema,
});
export type KnowledgeFile = z.infer<typeof KnowledgeFileSchema>;
export const ConflictCurrentSchema = z.object({
  content: z.string(), version: z.string().regex(VERSION_RE),
  updated_at: z.string().datetime({ offset: true }), updated_by_label: z.string(),
});
export const ConflictDetailsSchema = z.object({ reason: z.literal('version_conflict'), current: ConflictCurrentSchema });
export const TooLargeDetailsSchema = z.object({ reason: z.string(), max_bytes: z.number().int().positive() });
export const REASONS = [ 'path_empty','path_absolute','path_traversal','path_empty_segment','path_control_char',
  'path_too_long','segment_too_long','hidden','unsupported_type','too_large','invalid_encoding',
  'duplicate_in_request','too_many_files','project_file_limit','invalid_version','same_path',
  'prefix_required','invalid_limit','no_files','upload_too_large','path_exists','version_conflict','not_multipart' ] as const;
export type Reason = (typeof REASONS)[number];
```

**`base/path.ts`** (salinan perilaku `base/path.go`; backend tetap otoritatif):
```ts
export const MAX_PATH_RUNES = 512; export const MAX_SEGMENT_RUNES = 255;
export const SUPPORTED_EXTS = ['.md', '.markdown', '.txt'] as const;
export function validatePath(raw: string): Reason | null;   // urutan cek sama dengan NormalizePath (tanpa trim)
export function validatePrefix(raw: string): Reason | null; // "" root valid; satu "/" di akhir ditoleransi
export function normalizePrefix(raw: string): string;       // buang satu "/" di akhir
export function isHidden(p: string): boolean;
export function ext(p: string): string;                     // huruf kecil
export function isSupported(p: string): boolean;
export function baseName(p: string): string;
export function dirName(p: string): string;                 // "" untuk root
export function ancestors(p: string): string[];             // "docs/auth/x.md" -> ["docs", "docs/auth"]
export function joinPath(dir: string, rel: string): string; // dir "" -> rel
export function validateNewFilePath(raw: string): Reason | null; // validatePath -> hidden -> unsupported_type (urutan createfile)
```
Panjang memakai `runeLength` dari `shared/utils/text.ts`; karakter kontrol `/\p{Cc}/u`.

**`base/errors.ts`**:
```ts
export function reasonMessageKey(reason: string | null): string | null; // 'knowledge.reason.<r>' bila r di REASONS
export function errorMessage(e: unknown, tr: (k: string, p?: Record<string, unknown>) => string): string;
//   ApiError dengan reason dikenal -> tr('knowledge.reason.<r>', { max: formatBytes(details.max_bytes) })
//   ApiError tanpa reason -> e.message (pesan server, sudah Indonesia) ; selain itu tr('error.internal')
export function isNotFound(e: unknown): boolean; // ApiError status 404
```

**`base/index.ts`**: ekspor semua di atas (schema, tipe, fungsi path, helper error).

### K1. Feature `listtree` (`knowledge/listtree/`)

- **`types.ts`**: `FolderNodeSchema {name, path, file_count}`, `FileNodeSchema {name, path, size_bytes, updated_at}`, `TreeLevelSchema {prefix, folders[], files[], total_files}`; `type TreeLevel`; `TreeNodeRef = { kind: 'folder' | 'file'; path: string; name: string; fileCount?: number }`.
- **`services/listtree.service.ts`**: `getTreeLevel(projectId, prefix, signal?)` -> `get('/api/projects/' + enc(projectId) + '/knowledge/tree', { query: { prefix: prefix || undefined }, schema: TreeLevelSchema, signal })`.
- **`stores/tree.store.ts`** (`createTreeStore()`, per instance):
  - State: `levels: Map<prefix, { status: 'loading' | 'ready' | 'error'; level?: TreeLevel; error?: ApiError }>`, `expanded: Set<prefix>`, `selected: string | null` (path file atau folder), `focused: string | null`.
  - Aksi: `beginLoad(prefix)`, `resolve(prefix, level)`, `fail(prefix, err)`, `toggle(prefix)`, `expand(prefixes[])`, `select(path)`, `invalidateAll(): string[]` (mengembalikan prefix yang harus dimuat ulang = root + semua yang `expanded`; level lain dibuang), `visibleRows()` (derived: daftar baris datar berurutan untuk navigasi keyboard, termasuk baris loading/error/kosong per folder).
- **`components/`**:
  - `TreeView.svelte`: props `rows`, `selected`, `onselect(ref)`, `ontoggle(prefix)`, `onaction(ref, action)`, `onretry(prefix)`. `role="tree"` + `aria-label`; tiap baris `role="treeitem"`, `aria-level`, `aria-expanded` (folder), `aria-selected`; roving tabindex; tombol panah Atas/Bawah (pindah), Kanan (buka/masuk), Kiri (tutup/ke induk), Enter/Spasi (pilih/toggle), Home/End. Folder menampilkan `file_count`. Nama dirender teks (`break-all`), path lengkap di `title`.
  - `NodeMenu.svelte`: tombol `aria-haspopup="menu"` + menu (`role="menu"`, Esc menutup, fokus kembali ke tombol). Aksi folder: `newFileHere`, `uploadHere`, `copyPath`, `deleteFolder`. Aksi file: `open`, `move`, `copyPath`, `deleteFile`.
  - Baris status per folder: loading (`Spinner` kecil), error (teks + "Coba lagi"), kosong (`knowledge.tree.folderEmpty`).
- **`views/KnowledgeTreeView.svelte`**: props `projectId`, `selectedPath: string | null` (file atau folder dari URL), `refreshKey: number`, `onopenfile(path)`, `onopendir(path)`, `onaction(ref, action)`, `onprojectgone()`, `onempty?(isEmpty: boolean)`.
  - Mount: muat root. `selectedPath` berubah -> `expand(ancestors(path))` dan muat level yang belum ada (berurutan dari atas agar tidak memuat folder yang tidak ada; berhenti bila folder hilang dari level induk).
  - Buka folder yang sudah ada di cache tidak memanggil ulang request (usecase-listtree E2E 2).
  - `refreshKey` berubah -> `invalidateAll()` lalu muat ulang paralel (`Promise.all`, abort controller per prefix); baris lama tetap tampil sampai data baru masuk (tanpa kedip).
  - Error 404 dari tree -> `onprojectgone()` (tree tidak pernah 404 untuk prefix kosong). Error lain -> baris error per folder; error root -> `ErrorState` + coba lagi.
  - Root `total_files === 0` -> `onempty(true)`.
- **`index.ts`**: `export { default as KnowledgeTreeView } from './views/KnowledgeTreeView.svelte'; export type { TreeNodeRef, TreeAction } from './types';`

### K2. Feature `getfile` (`knowledge/getfile/`)

- **`types.ts`**: re-export `KnowledgeFile` dari `base`. `type ViewerState = idle | loading(path) | ready(file) | notFound(path) | error(path, ApiError)`.
- **`services/getfile.service.ts`**: `getFile(projectId, path, signal?)` -> `get('/api/projects/' + enc(projectId) + '/knowledge/files', { query: { path }, schema: KnowledgeFileSchema, signal })`.
- **`stores/viewer.store.ts`**: `createViewerStore()` dengan `begin(path)`, `resolve(file)` (abaikan bila path tidak sama dengan yang terakhir diminta), `fail(path, err)` (404 -> `notFound`), `replace(file)`.
- **`components/`**:
  - `Breadcrumb.svelte`: props `path`, `ondir(path)`; `<nav aria-label>` + `<ol>`; segmen folder adalah tombol, segmen terakhir `aria-current="page"`; tombol "Salin path" (`copyText`, toast `knowledge.copy.done` / `knowledge.copy.failed`).
  - `FileMeta.svelte`: ukuran (`formatBytes`), `updated_by.label`, `<time datetime>` (`formatDateTime`).
  - `FileContent.svelte`: props `file`, `resolveLink`. `.md`/`.markdown` -> `MarkdownView`; `.txt` -> `<pre class="whitespace-pre-wrap">{content}</pre>`. File kosong -> `knowledge.viewer.emptyFile`.
- **`views/FileViewerView.svelte`**: props `projectId`, `path`, `reloadKey`, `resolveLink(href, fromDir) => string | null`, `onedit(file)`, `onmove(file)`, `ondelete(file)`, `ondir(path)`, `onnotfound(path)`.
  - Muat saat `path`/`reloadKey` berubah (abort sebelumnya). Loading -> skeleton + `Spinner` (`aria-busy`). `notFound` -> `EmptyState` `knowledge.viewer.notFound` + aksi kembali ke folder induk, lalu panggil `onnotfound(path)` sekali (app memuat ulang pohon dan konteks project). Error -> `ErrorState` + coba lagi.
  - Ready: `Breadcrumb`, judul (`title` atau `baseName(path)`), `FileMeta`, tombol Edit/Pindah/Hapus, `FileContent`.
- **`index.ts`**: `FileViewerView`, `getFile` (dipakai `updatefile` untuk muat ulang setelah konflik dan pemulihan draf; dicatat K5), tipe `KnowledgeFile`.

### K3. Feature `createfile` (`knowledge/createfile/`)

- **`types.ts`**: `CreateFileInput { path: string; content: string }`.
- **`services/createfile.service.ts`**: `createFile(projectId, input)` -> `post(url + '/knowledge/files', input, { schema: KnowledgeFileSchema })`.
- **`stores/newFile.store.ts`**: `createNewFileStore(initialDir)`: `path` (awal `joinPath(dir, '')` + `/` bila dir ada), `content` (awal `''`), `submitting`, `pathError: Reason | null` (derived dari `validateNewFilePath` setelah field disentuh), `formError: string | null`, `dirty`.
- **`components/NewFileForm.svelte`**: `Input` path (hint `knowledge.new.pathHint`: contoh `docs/auth/login.md`), pesan alasan inline (`knowledge.reason.*`), `MarkdownEditor` + `MarkdownView` pratinjau (debounce 250 ms), tombol Simpan (`disabled` bila path tidak valid atau `submitting`) dan Batal.
- **`views/NewFileView.svelte`**: props `projectId`, `dir`, `oncreated(file)`, `oncancel()`, `ondirtychange(dirty)`.
  - Simpan -> `createFile`. 201 -> toast `knowledge.new.created` + `oncreated(file)`.
  - 400 dengan reason path (`path_*`, `segment_too_long`, `hidden`, `unsupported_type`) -> error di input path. 409 `path_exists` -> error input path `knowledge.reason.path_exists` + tautan "Buka file yang ada" (`oncreated` tidak dipanggil; app menyediakan tautan via `resolveLink`). 413 `too_large` -> `formError` dengan batas dari `details.max_bytes`; isi editor tetap. 400 `project_file_limit`/`invalid_encoding` -> `formError`. 404 -> `formError` project hilang + `ondirtychange(false)` lalu app memanggil `ctx.reload()`.
- **`index.ts`**: `NewFileView`.

### K4. Feature `updatefile` (`knowledge/updatefile/`)

- **`types.ts`**: `UpdateFileInput { content: string; version: string }`; `type SaveOutcome = saved(file) | noop(file) | conflict(current) | notFound | tooLarge(max) | invalid(reason) | failed(err)`; `Draft { content: string; baseVersion: string; savedAt: string }`.
- **`services/updatefile.service.ts`**: `updateFile(projectId, path, input)` -> `put(url + '/knowledge/files', input, { query: { path }, schema: KnowledgeFileSchema })`. Helper `parseConflict(err): ConflictCurrent | null` (409 + `ConflictDetailsSchema.safeParse(details)`).
- **`stores/editor.store.ts`** (`createEditorStore(file)`): `base: { content, version }`, `draft` (isi editor), `dirty` (derived `draft !== base.content`), `saving`, `conflict: ConflictCurrent | null`, `compareOpen`, `error: string | null`. Aksi: `edit(text)`, `saved(file)` (base = file, draft tetap bila user mengetik selama simpan dan berbeda), `setConflict(c)`, `acceptCurrent()` (base = current, draft = current.content), `rebaseOnCurrent()` (base.version = current.version, draft tetap -> simpan ulang = timpa sadar), `discard()`.
- **`stores/draft.ts`**: `loadDraft(projectId, path): Draft | null`, `saveDraft(...)` (debounce 1 s dari view), `clearDraft(...)`. Kunci `grimoire.knowledge.draft:<projectId>:<path>` di `sessionStorage`, semua akses dibungkus try/catch (mode privat/kuota -> tanpa draf, tidak error). Hanya isi knowledge (bukan rahasia; rule 12.9 tidak berlaku).
- **`components/`**:
  - `EditorToolbar.svelte`: indikator `knowledge.editor.unsaved` / `knowledge.editor.saved` (`aria-live="polite"`), tombol Simpan (`loading` saat `saving`), Batal, toggle Tulis/Pratinjau (layar < `lg`).
  - `EditorSplit.svelte`: `MarkdownEditor` kiri, `MarkdownView` kanan (debounce 250 ms; isi > 200 KB -> pratinjau manual dengan tombol `knowledge.editor.refreshPreview`).
  - `ConflictDialog.svelte`: `Dialog` judul `knowledge.conflict.title`, teks `knowledge.conflict.body` dengan `{name}` (= `updated_by_label`) dan `{time}`; tombol `knowledge.conflict.reload` (muat versi terbaru, buang perubahan saya; konfirmasi kedua bila dirty), `knowledge.conflict.compare`, `knowledge.conflict.keepEditing`.
  - `CompareView.svelte`: dua kolom "Versi saya" vs "Versi terbaru" + diff baris (`diff.ts`); tombol `knowledge.conflict.overwrite` (rebase lalu simpan), `knowledge.conflict.useTheirs` (= reload), `knowledge.copy.mine` (salin draf saya).
  - `diff.ts`: `diffLines(a, b): { op: 'eq' | 'add' | 'del'; text: string }[]` (LCS Myers sederhana; > 5000 baris salah satu sisi -> `null`, UI hanya menampilkan dua kolom tanpa sorotan).
- **`views/FileEditorView.svelte`**: props `projectId`, `path`, `onclose(file | null)`, `ondirtychange(dirty)`, `onnotfound(path)`.
  - Mount: `getFile` (dari `getfile/index.ts`). Loading/404/error seperti viewer. Ready: cek `loadDraft`; bila ada dan isinya beda -> banner `knowledge.editor.draftFound` (`{time}`) dengan "Pulihkan" / "Buang". Pulihkan memakai `draft.baseVersion` sebagai `version` (bila beda dengan versi server, simpan berikutnya akan 409 dan masuk alur konflik, sengaja).
  - Simpan (tombol atau Mod-S): bila tidak dirty dan versi sama -> tetap kirim PUT (backend no-op terverifikasi D12, 200 tanpa audit) agar konsisten; respons -> `saved`. Sukses -> `clearDraft`, toast `knowledge.editor.savedToast`, `ondirtychange(false)`. Simpan kedua memakai `version` dari respons pertama (usecase-updatefile E2E 2).
  - 409 `version_conflict` -> `setConflict` + `ConflictDialog`. 409 tanpa detail valid -> `formError`.
  - 404 -> banner `knowledge.editor.deletedRemotely` (draf tetap di editor dan sessionStorage, tombol "Simpan sebagai file baru" membuka `?new=1` dengan isi draf lewat callback `onsaveasnew(content)`), lalu `onnotfound(path)`.
  - 413 -> `formError` dengan batas; 400 `invalid_encoding`/`invalid_version` -> `formError`. 401 -> handler global ke `/login?next=...` (draf sudah di sessionStorage; setelah login kembali ke `?path=&mode=edit` dan banner pemulihan tampil).
  - Draf disimpan (debounce 1 s) setiap `draft` berubah saat dirty; `beforeunload` dipasang saat dirty (`event.preventDefault()`), dilepas saat bersih/unmount.
  - Batal saat dirty -> konfirmasi `knowledge.editor.discardConfirm`; ya -> `clearDraft` + `onclose(null)`.
- **`index.ts`**: `FileEditorView`, tipe `SaveOutcome`.

### K5. Feature `uploadfiles` (`knowledge/uploadfiles/`)

- **`types.ts`**:
  ```ts
  export const UploadStatusSchema = z.enum(['created', 'updated', 'unchanged', 'skipped', 'rejected']);
  export const UploadItemSchema = z.object({ path: z.string(), status: UploadStatusSchema, reason: z.string().optional() });
  export const UploadResultSchema = z.object({
    summary: z.object({ created: z.number().int().nonnegative(), updated: z.number().int().nonnegative(),
      unchanged: z.number().int().nonnegative(), skipped: z.number().int().nonnegative(), rejected: z.number().int().nonnegative() }),
    items: z.array(UploadItemSchema),
  });
  export interface LocalFile { path: string; file: File }           // path relatif (webkitRelativePath / entry.fullPath tanpa "/" depan)
  export type ReportStatus = z.infer<typeof UploadStatusSchema> | 'unsupported';
  export interface ReportItem { path: string; status: ReportStatus; reasonText?: string; target: string }
  export interface UploadTarget {                                   // titik ekstensi untuk app/ (upload campuran)
    key: string; labelKey: string;
    accepts(path: string): boolean;
    upload(projectId: string, files: LocalFile[], o: { signal: AbortSignal; onProgress(p: UploadProgress): void }): Promise<ReportItem[]>;
  }
  export interface UploadPlan { byTarget: Map<string, LocalFile[]>; skipped: ReportItem[]; unsupported: ReportItem[]; totalBytes: number }
  ```
- **`services/uploadfiles.service.ts`**: `uploadKnowledgeFiles(projectId, files, o)` -> `FormData`; `form.append('files', f.file, f.path)` untuk tiap file (nama ketiga = path relatif; backend membaca `filename` mentah); `postMultipart(url + '/knowledge/upload', form, { schema: UploadResultSchema, ... })`. `knowledgeTarget: UploadTarget` (`key: 'knowledge'`, `accepts = isSupported`, `upload` memetakan item ke `ReportItem` dengan `reasonText` dari `knowledge.reason.*`).
- **`services/collect.ts`** (I/O browser, tanpa state):
  - `fromInput(fileList: FileList): LocalFile[]` (`webkitRelativePath || name`).
  - `fromDrop(dt: DataTransfer): Promise<LocalFile[]>`: `items[i].webkitGetAsEntry()`; direktori dibaca rekursif dengan `createReader().readEntries()` diulang sampai kosong (batch 100 Chrome); `fullPath` tanpa `/` depan. Fallback `dt.files` bila API entri tidak ada. Batas aman 10 000 entri (lebih -> berhenti + flag `truncated`).
- **`stores/upload.store.ts`** (`createUploadStore()`): fase `pick | review | uploading | done | failed`; `plan`, `progress { target, loaded, total, phase }`, `report: ReportItem[]`, `targetErrors: Map<target, string>`, `filter: 'all' | 'failed' | 'updated' | 'created' | 'unchanged' | 'skipped'`; derived `counts` dan `filtered`.
- **`stores/plan.ts`** (murni): `planUpload(files: LocalFile[], targets: UploadTarget[], targetDir: string): UploadPlan`. Per file: path = `joinPath(targetDir, path)`; `isHidden` -> `skipped` (`reasonText` `knowledge.reason.hidden`, target `client`, tidak dikirim, contoh `.git/` besar tidak diunggah); target pertama yang `accepts` -> masuk `byTarget`; tidak ada -> `unsupported`. Path lain TIDAK divalidasi di klien (backend otoritatif atas penolakan, tiket "Upload campuran").
- **`components/`**:
  - `DropZone.svelte`: area drag-drop (`dragenter/over/leave/drop`, `aria-describedby`), tombol "Pilih folder" (`<input type="file" webkitdirectory multiple hidden>`) dan "Pilih file" (`accept=".md,.markdown,.txt"` + ekstensi target lain dari props `acceptExts`).
  - `UploadReview.svelte`: ringkasan per target (`{n}` file, `formatBytes`), daftar dilewati dan tidak didukung (dilipat bila > 10), folder tujuan (`targetDir` atau root), tombol Unggah (`disabled` bila tidak ada file untuk target mana pun) dan Batal.
  - `UploadProgress.svelte`: `<progress>` + teks `knowledge.upload.sending` (`{percent}`) / `knowledge.upload.processing` (indeterminate), nama target aktif, tombol Batalkan.
  - `UploadReport.svelte`: kartu ringkasan (dibuat, ditimpa, tidak berubah, dilewati, gagal, tidak didukung), filter (`role="radiogroup"` tombol), tabel item (path teks, status badge teks, alasan), banner error per target (mis. 413 `upload_too_large` dengan batas). Daftar > 500 item dirender bertahap ("Tampilkan lebih banyak", 200 per langkah).
- **`views/UploadDialogView.svelte`**: props `open`, `projectId`, `targetDir`, `extraTargets: UploadTarget[] = []`, `onclose()`, `onuploaded()`.
  - Target efektif = `[knowledgeTarget, ...extraTargets]`.
  - Jalankan target berurutan (knowledge dulu), satu request per target (K6). Error seluruh request satu target (413, 400 `no_files`/`not_multipart`, 5xx, NETWORK) -> `targetErrors` + semua file target itu tampil `rejected` dengan alasan error tersebut; target lain tetap jalan. 404 -> `targetErrors` "project tidak ditemukan" + `onuploaded()` (app memuat ulang konteks).
  - Abort -> fase `failed` dengan `knowledge.upload.cancelled`; laporan parsial target yang sudah selesai tetap tampil.
  - Selesai (minimal satu target sukses) -> `onuploaded()` (app menaikkan `refreshKey` pohon). Menutup dialog saat `uploading` -> konfirmasi batal.
  - Laporan menggabungkan item klien (`skipped`, `unsupported`) + item server, urut sesuai urutan file asli.
- **`index.ts`**: `UploadDialogView`, tipe `UploadTarget`, `LocalFile`, `ReportItem`.

### K6. Feature `movefile` (`knowledge/movefile/`)

- **`services/movefile.service.ts`**: `moveFile(projectId, { from, to })` -> `post(url + '/knowledge/files/move', body, { schema: KnowledgeFileSchema })`.
- **`components/MoveFileForm.svelte`**: `Input` path tujuan (awal = `from`, kursor memilih nama file tanpa ekstensi), validasi `validateNewFilePath(to)` + `to === from` -> `same_path`.
- **`views/MoveFileDialog.svelte`**: props `open`, `projectId`, `from`, `onmoved(file)`, `onclose()`, `onnotfound()`.
  - 200 -> toast `knowledge.move.done` + `onmoved(file)`. 409 `path_exists` / 400 reason -> error inline di input. 404 -> pesan `knowledge.move.sourceGone` + `onnotfound()` (pohon dimuat ulang). Tombol Simpan `disabled` bila tidak valid/sama.
- **`index.ts`**: `MoveFileDialog`.

### K7. Feature `deletefile` (`knowledge/deletefile/`)

- **`services/deletefile.service.ts`**: `deleteFile(projectId, path)` -> `del(url + '/knowledge/files', { query: { path } })` (204, tanpa schema).
- **`views/DeleteFileDialog.svelte`**: props `open`, `projectId`, `path`, `ondeleted(path)`, `onclose()`. `Dialog` judul `knowledge.deleteFile.title`, isi path (teks, `break-all`) + `knowledge.deleteFile.body` (permanen, tanpa riwayat versi). Tombol `variant="danger"`. Batal tidak mengirim request. 204 -> toast + `ondeleted`. 404 -> toast `knowledge.deleteFile.gone` + `ondeleted` (dianggap sudah tidak ada; app menyegarkan pohon). Error lain -> pesan di dialog.
- **`index.ts`**: `DeleteFileDialog`.

### K8. Feature `deletefolder` (`knowledge/deletefolder/`)

- **`types.ts`**: `DeleteFolderResultSchema = z.object({ deleted: z.number().int().nonnegative() })`.
- **`services/deletefolder.service.ts`**: `deleteFolder(projectId, path)` -> `del(url + '/knowledge/folders', { query: { path }, schema: DeleteFolderResultSchema })`.
- **`views/DeleteFolderDialog.svelte`**: props `open`, `projectId`, `path`, `fileCount`, `ondeleted(path, deleted)`, `onclose()`. Isi: path folder + `knowledge.deleteFolder.body` dengan `{count}` file (dari `file_count` pohon) + kotak centang wajib `knowledge.deleteFolder.ack` sebelum tombol hapus aktif. `path` kosong tidak pernah dikirim (tidak ada aksi hapus root di UI; guard di view melempar error pemrograman). 200 -> toast `knowledge.deleteFolder.done` (`{count}` = `deleted`). 404 -> toast `knowledge.deleteFolder.gone` + `ondeleted(path, 0)`.
- **`index.ts`**: `DeleteFolderDialog`.

### K9. Feature `search` (`knowledge/search/`)

- **`types.ts`**: `SegmentSchema {text, highlight: boolean}`, `SearchItemSchema {path, title, score: number, snippet: Segment[]}`, `SearchResultSchema {items, total}`; `DEFAULT_LIMIT = 20`, `MAX_LIMIT = 50`, `MAX_QUERY_CHARS = 200`.
- **`services/search.service.ts`**: `searchKnowledge(projectId, { q, prefix, limit }, signal?)` -> `get(url + '/knowledge/search', { query: { q, prefix: prefix || undefined, limit }, schema: SearchResultSchema, signal })`. Kueri dikirim apa adanya (dipotong 200 karakter); pembersihan di backend.
- **`stores/search.store.ts`** (`createSearchStore()`): `idle | loading(key, previous?) | ready(key, result) | error(key, err)`; `key = q|prefix|limit`; hasil usang diabaikan.
- **`components/`**:
  - `SearchBox.svelte`: `<input type="search">` dengan label tersembunyi, tombol hapus, Esc mengosongkan, `/` memfokuskan (bila fokus tidak di input lain). Mengirim `onchange(q)` dengan debounce 300 ms (diatur view).
  - `SearchResults.svelte`: daftar `<ul>`; tiap item: judul (atau `baseName`), path kecil, cuplikan `{#each snippet as s}{#if s.highlight}<mark>{s.text}</mark>{:else}{s.text}{/if}{/each}` (tanpa `{@html}`). Klik/Enter -> `onopen(path)`. Total `knowledge.search.total` (`{shown}` dari `{total}`); bila `total > items.length` dan `limit < 50` -> tombol `knowledge.search.more` (limit 50).
  - `ScopeToggle.svelte`: bila ada folder konteks -> checkbox `knowledge.search.onlyIn` (`{dir}`).
- **`views/KnowledgeSearchView.svelte`**: props `projectId`, `q`, `scope: string` (prefix aktif atau ''), `contextDir`, `onquerychange(q, scope)`, `onopen(path)`, `onprojectgone()`.
  - `q.trim() === ''` -> tidak ada request, tidak dirender (app menampilkan pohon). Kueri yang tidak menghasilkan term di backend -> `{items: [], total: 0}` -> `EmptyState` `knowledge.search.empty` (bukan error).
  - Loading pertama -> `Spinner`; berikutnya hasil lama redup + `aria-busy`. Error 400 (mis. `invalid_limit`, `path_traversal` pada scope) -> pesan alasan; 404 -> `onprojectgone()`; lain -> `ErrorState` + coba lagi.
- **`index.ts`**: `KnowledgeSearchView`, `SearchBox` (dipakai app di header panel kiri).

### A. Komposisi `app/` (rule 12.4)

**A1. `app/workspace/knowledgeUrl.ts`** (baru, murni) + test:
```ts
export interface KnowledgeUrlState { path: string | null; dir: string | null; mode: 'view' | 'edit' | 'new'; q: string; scope: string }
export function parseKnowledgeQuery(q: URLSearchParams): KnowledgeUrlState; // mode=edit tanpa path -> view; new=1 menang atas path
export function buildKnowledgeHref(projectId: string, s: Partial<KnowledgeUrlState>): string; // '/projects/<enc>/knowledge?...' (urutan param stabil)
```

**A2. `app/workspace/tabs.ts`** (ubah): sisipkan `{ key: 'knowledge', labelKey: 'project.tab.knowledge', view: KnowledgeTab }` di indeks 0 (urutan final tiket T03: Knowledge, API Specs, API Keys, Members, Settings; bila T06 sudah ada, tetap di depan `api-specs`). `defaultTabKey()` menjadi `knowledge`. `tabs.test.ts`: ekspektasi urutan diperbarui.

**A3. `app/workspace/uploadTargets.ts`** (baru): `export const extraUploadTargets: UploadTarget[] = [];` dengan komentar: "T06 menambah target api-spec (`accepts` .yaml/.yml/.json, memanggil service upload T06 lewat index.ts-nya) DI SINI". Tipe `UploadTarget` di-import dari `domain/project/knowledge/uploadfiles` (index.ts). Selama kosong, `.yaml/.yml/.json` dilaporkan `unsupported` dengan `knowledge.upload.unsupportedHere`.

**A4. `app/views/workspace/knowledge/KnowledgeTab.svelte`** (baru): props `WorkspaceTabProps` (`ctx`, `currentUserId`, `onprojectgone`, `onleft`).
- State URL: `parseKnowledgeQuery($current.location.query)`. Navigasi: `navigate(buildKnowledgeHref(...))` (push untuk buka file/folder/mode; `replace` untuk perubahan kueri cari).
- State lokal: `treeRefreshKey`, `viewerReloadKey`, `dirty` (dari editor/new), dialog terbuka (`upload { dir }`, `move { from }`, `deleteFile { path }`, `deleteFolder { path, count }`), `isEmpty`.
- Layout: grid dua kolom >= `lg` (kiri 20rem, bisa diubah lebarnya tidak termasuk), < `lg` panel kiri menjadi laci yang dibuka tombol `knowledge.panel.files` dan tertutup otomatis setelah memilih file.
- Komposisi: `KnowledgeSidebar` + `KnowledgeMain` + dialog-dialog (A5-A7).
- Penjaga perubahan: bila `dirty` dan user memilih file/folder lain atau membuka pencarian -> `Dialog` `knowledge.editor.discardConfirm`; Ya -> lanjut (draf tetap di sessionStorage sampai dibuang eksplisit).
- 404 dari viewer/editor/move/delete -> `treeRefreshKey++` dan `ctx.reload()` (bila project hilang, `ProjectContextView` T03 menampilkan "Project tidak ditemukan"). `onprojectgone` dari tree/search -> `ctx.reload()`.
- `resolveLink(href, fromDir)`: `resolveRelativePath(fromDir, href)` -> path; bila `isSupported(path)` -> `buildKnowledgeHref(id, { path })`, selain itu `null` (teks). Router global menangani klik tautan same-origin.

**A5. `app/views/workspace/knowledge/KnowledgeSidebar.svelte`** (baru): tombol Upload + File baru, `SearchBox`; `q` kosong -> `KnowledgeTreeView` (`selectedPath = path ?? dir`, `refreshKey`), selain itu `KnowledgeSearchView` (`scope`, `contextDir = dir ?? dirName(path)`). `onaction` memetakan aksi menu ke navigasi atau pembukaan dialog; `copyPath` -> `copyText` + toast.

**A6. `app/views/workspace/knowledge/KnowledgeMain.svelte`** (baru): `mode=new` -> `NewFileView` (`dir`); `mode=edit` + path -> `FileEditorView`; path -> `FileViewerView`; dir -> `FolderPane.svelte` (lokal app: judul folder, breadcrumb sederhana, tombol file baru/upload ke sini/hapus folder; isi folder ditampilkan pohon di kiri, panel ini tidak memuat ulang data); kosong + `isEmpty` -> `EmptyState` `knowledge.empty.title` + aksi Upload / File baru; kosong + tidak empty -> `EmptyState` `knowledge.pickFile`.
- `oncreated(file)` -> `treeRefreshKey++`, navigasi `{ path: file.path }`. `onclose(file)` editor -> `{ path }` mode view + `viewerReloadKey++`. `onmoved(file)` -> `{ path: file.path }` + refresh. `ondeleted` -> `{ dir: dirName(path) || null }` + refresh. `ondeleted` folder -> `{ dir: dirName(path) || null }` + refresh.

**A7. `app/views/workspace/knowledge/KnowledgeDialogs.svelte`** (baru): `UploadDialogView` (`extraTargets={extraUploadTargets}`, `targetDir`, `onuploaded` -> refresh), `MoveFileDialog`, `DeleteFileDialog`, `DeleteFolderDialog`.

**A8. `app/routes/index.ts`**: tidak berubah (route `/projects/:projectID/:tab` dari T03 sudah menampung `knowledge`). `router.test.ts`: tambah kasus `/projects/abc/knowledge?path=docs%2Fa.md` -> `tab='knowledge'` dan `query.get('path') === 'docs/a.md'`.

**A9. `domain/core/audit-log/listentries/types.ts`**: pastikan `'knowledge.file.moved'` ada di grup `content` (ditambah backend F6; bila belum, tambahkan di sini). Tidak ada perubahan lain di T02.

### E. E2E (`frontend/e2e/`)

1. `e2e/helpers/project.ts` (baru bila T03 belum menyediakan; bila sudah ada pakai yang ada): `createProjectApi(request, name)`, `addMemberApi`.
2. `e2e/helpers/knowledge.ts` (baru): `uploadApi(request, projectId, files: {path, content | Buffer}[])` (multipart lewat `request.post` + `multipart`), `getFileApi`, `openKnowledge(page, projectId, query?)`.
3. `e2e/fixtures/knowledge/docs/` (baru): `a.md`, `auth/login.md` (berisi blok ```mermaid), `auth/flow/x.md`, `guide.md` (panjang, menyebut "login" sekali), `.git/config`, `logo.png` (kecil), `xss.md` (`<script>`, `[x](javascript:alert(1))`, `<img src=x onerror=alert(1)>`). File 2 MB dan biner `.md` dibuat di test (`Buffer`), tidak di-commit.
4. `e2e/knowledge.spec.ts` (baru): skenario bagian "E2E Scenarios" (server default).
5. `e2e/knowledge-limits.spec.ts` (baru) + `playwright.config.ts` (ubah): project Playwright kedua `knowledge-limits` dengan `testMatch: /knowledge-limits\.spec\.ts/`, `baseURL http://localhost:8081`, dan entri `webServer` kedua (`HTTP_ADDR=127.0.0.1:8081`, `MAX_UPLOAD_BYTES=2097152`, `MAX_PROJECT_FILES=5`, DB yang sama). Project `chromium` mengecualikan file itu (`testIgnore`). `webServer` menjadi array; bila `E2E_BASE_URL` di-set (CI eksternal), project limits di-skip dengan pesan.

### V. Verifikasi

`npm run check`, `npm test`, `npm run depcruise`, `npm run test:arch`, `npm run build` (cek chunk: `mermaid` dan `codemirror` terpisah dari chunk utama; ukuran chunk utama tidak naik > 60 KB gzip), lalu `npm run test:e2e` terhadap binary (backend T05 + T01-T03). E2E memasang listener `page.on('console')` untuk pelanggaran CSP (gagal bila ada `Refused to`), membuktikan Mermaid dan CodeMirror jalan di CSP produksi.

---

## State UI (loading / empty / error per permukaan)

| Permukaan | Loading | Empty | Error | Khusus |
|---|---|---|---|---|
| Pohon root | `Spinner` + `aria-busy` di panel kiri | `knowledge.empty.title` di panel kanan + CTA Upload/File baru; panel kiri `knowledge.tree.empty` | `ErrorState` + coba lagi | 404 -> `ctx.reload()` (project hilang) |
| Folder di pohon | `Spinner` kecil di bawah node | `knowledge.tree.folderEmpty` (folder dihapus di tab lain, 200 kosong) | Baris error + "Coba lagi" (folder lain tetap) | Cache per prefix; refresh setelah mutasi tanpa kedip |
| Viewer file | Skeleton + `Spinner` | `knowledge.viewer.emptyFile` (isi kosong) | `ErrorState` + coba lagi | 404 -> `knowledge.viewer.notFound` + kembali ke folder induk + refresh pohon. Mermaid gagal -> kode + `md.mermaidError`. `INVALID_RESPONSE` -> `ErrorState` |
| Editor | Skeleton saat memuat file; tombol Simpan `loading` | - | `formError` di atas editor (413 dengan batas, `invalid_encoding`, jaringan) | 409 -> `ConflictDialog`/`CompareView`; 404 -> banner terhapus + "Simpan sebagai file baru"; 401 -> login, draf dipulihkan; dirty -> `beforeunload` + konfirmasi pindah |
| File baru | Tombol Simpan `loading` | Editor kosong | Error inline path / `formError` | 409 `path_exists` + tautan ke file yang ada |
| Upload | `UploadProgress` (persen byte, lalu "memproses") | Tanpa file valid -> tombol Unggah nonaktif + `knowledge.upload.nothing` | Banner per target (413 `upload_too_large` + batas, 400, 5xx, jaringan) | Abort -> `knowledge.upload.cancelled` + laporan parsial; filter laporan |
| Pencarian | Pertama `Spinner`; berikutnya hasil lama redup + `aria-busy` | `knowledge.search.empty` (termasuk kueri tanpa kata) | Pesan alasan (400) / `ErrorState` + coba lagi | Kueri kosong -> pohon kembali; > 200 karakter dipotong klien |
| Pindah | Tombol `loading` | - | Error inline (409/400) | 404 -> `knowledge.move.sourceGone` + refresh |
| Hapus file/folder | Tombol `loading` | - | Pesan di dialog | 404 -> toast "sudah tidak ada" + refresh; folder wajib centang konfirmasi |

---

## Kunci i18n (id / en)

Kunci flat; paritas id/en dijaga `i18n-keys.test.ts`. Tidak ada string hard-coded di komponen (rule 12.8).

| Kunci | id | en |
|---|---|---|
| `project.tab.knowledge` | Knowledge | Knowledge |
| `knowledge.panel.files` | File | Files |
| `knowledge.actions.upload` | Upload | Upload |
| `knowledge.actions.newFile` | File baru | New file |
| `knowledge.actions.edit` / `.move` / `.delete` / `.copyPath` / `.newFileHere` / `.uploadHere` / `.deleteFolder` / `.open` / `.menu` | Edit / Pindah atau ganti nama / Hapus / Salin path / File baru di sini / Upload ke sini / Hapus folder / Buka / Aksi untuk {name} | Edit / Move or rename / Delete / Copy path / New file here / Upload here / Delete folder / Open / Actions for {name} |
| `knowledge.tree.label` | Pohon file knowledge | Knowledge file tree |
| `knowledge.tree.empty` | Belum ada file | No files yet |
| `knowledge.tree.folderEmpty` | Folder kosong | Empty folder |
| `knowledge.tree.fileCount` | {count} file | {count} files |
| `knowledge.tree.loadError` | Gagal memuat folder | Failed to load folder |
| `knowledge.empty.title` | Belum ada knowledge di project ini | This project has no knowledge yet |
| `knowledge.empty.hint` | Unggah folder docs/ atau buat file Markdown baru. | Upload a docs/ folder or create a new Markdown file. |
| `knowledge.pickFile` | Pilih file di pohon untuk melihat isinya | Select a file in the tree to view it |
| `knowledge.copy.done` / `.failed` / `.mine` | Path disalin / Gagal menyalin; salin manual / Salin versi saya | Path copied / Copy failed; copy manually / Copy my version |
| `knowledge.breadcrumb.label` / `.root` | Lokasi file / Root | File location / Root |
| `knowledge.viewer.notFound` / `.notFoundHint` / `.backToFolder` | File tidak ditemukan / File mungkin sudah dihapus atau dipindah. / Kembali ke folder | File not found / The file may have been deleted or moved. / Back to folder |
| `knowledge.viewer.emptyFile` | File ini kosong | This file is empty |
| `knowledge.viewer.updatedBy` | Diubah oleh {name}, {time} | Updated by {name}, {time} |
| `knowledge.viewer.size` | Ukuran {size} | Size {size} |
| `md.mermaidError` / `md.mermaidLabel` / `md.imageOmitted` | Diagram Mermaid tidak bisa dirender / Diagram Mermaid / Gambar tidak ditampilkan: {alt} | Mermaid diagram could not be rendered / Mermaid diagram / Image not shown: {alt} |
| `knowledge.editor.unsaved` / `.saved` / `.savedToast` | Belum disimpan / Tersimpan / Perubahan disimpan | Unsaved / Saved / Changes saved |
| `knowledge.editor.save` / `.cancel` / `.write` / `.preview` / `.refreshPreview` | Simpan / Batal / Tulis / Pratinjau / Perbarui pratinjau | Save / Cancel / Write / Preview / Refresh preview |
| `knowledge.editor.discardConfirm` / `.discard` | Buang perubahan yang belum disimpan? / Buang | Discard unsaved changes? / Discard |
| `knowledge.editor.draftFound` / `.restore` | Ada draf belum disimpan dari {time}. / Pulihkan | An unsaved draft from {time} exists. / Restore |
| `knowledge.editor.deletedRemotely` / `.saveAsNew` | File ini sudah dihapus orang lain. Draf Anda masih ada. / Simpan sebagai file baru | This file was deleted by someone else. Your draft is kept. / Save as new file |
| `knowledge.editor.label` | Isi Markdown | Markdown content |
| `knowledge.conflict.title` / `.body` | File sudah diubah orang lain / {name} menyimpan perubahan pada {time}. Perubahan Anda belum disimpan. | File was changed by someone else / {name} saved changes at {time}. Your changes are not saved. |
| `knowledge.conflict.reload` / `.compare` / `.keepEditing` / `.overwrite` / `.useTheirs` / `.mine` / `.theirs` | Muat versi terbaru / Bandingkan / Lanjut mengedit / Timpa dengan versi saya / Pakai versi terbaru / Versi saya / Versi terbaru | Load latest version / Compare / Keep editing / Overwrite with my version / Use latest version / My version / Latest version |
| `knowledge.new.title` / `.path` / `.pathHint` / `.created` / `.openExisting` | File baru / Path file / Contoh: docs/auth/login.md (.md, .markdown, .txt) / File dibuat / Buka file yang ada | New file / File path / Example: docs/auth/login.md (.md, .markdown, .txt) / File created / Open existing file |
| `knowledge.upload.title` / `.drop` / `.pickFolder` / `.pickFiles` / `.targetDir` / `.root` | Upload knowledge / Tarik folder atau file ke sini / Pilih folder / Pilih file / Tujuan: {dir} / root project | Upload knowledge / Drop a folder or files here / Choose folder / Choose files / Destination: {dir} / project root |
| `knowledge.upload.review` / `.toTarget` / `.skippedList` / `.unsupportedList` / `.nothing` / `.start` | Periksa sebelum upload / {count} file ke {target} ({size}) / {count} file dilewati (tersembunyi) / {count} file tidak didukung / Tidak ada file yang bisa diunggah / Unggah | Review before upload / {count} files to {target} ({size}) / {count} files skipped (hidden) / {count} files not supported / Nothing to upload / Upload |
| `knowledge.upload.sending` / `.processing` / `.cancel` / `.cancelled` / `.cancelConfirm` | Mengirim {percent}% / Memproses di server... / Batalkan / Upload dibatalkan / Batalkan upload yang sedang berjalan? | Sending {percent}% / Processing on server... / Cancel / Upload cancelled / Cancel the running upload? |
| `knowledge.upload.report` / `.filter.all` / `.filter.failed` / `.filter.updated` / `.filter.created` / `.filter.unchanged` / `.filter.skipped` / `.showMore` | Laporan upload / Semua / Gagal / Ditimpa / Dibuat / Tidak berubah / Dilewati / Tampilkan lebih banyak | Upload report / All / Failed / Overwritten / Created / Unchanged / Skipped / Show more |
| `knowledge.upload.status.created` / `.updated` / `.unchanged` / `.skipped` / `.rejected` / `.unsupported` | Dibuat / Ditimpa / Tidak berubah / Dilewati / Ditolak / Tidak didukung | Created / Overwritten / Unchanged / Skipped / Rejected / Not supported |
| `knowledge.upload.unsupportedHere` | Tipe file ini tidak didukung di sini | This file type is not supported here |
| `knowledge.upload.targetFailed` | Upload ke {target} gagal: {message} | Upload to {target} failed: {message} |
| `knowledge.upload.target.knowledge` | Knowledge | Knowledge |
| `knowledge.move.title` / `.to` / `.save` / `.done` / `.sourceGone` | Pindah atau ganti nama / Path baru / Pindahkan / File dipindahkan / File asal sudah tidak ada | Move or rename / New path / Move / File moved / Source file no longer exists |
| `knowledge.deleteFile.title` / `.body` / `.confirm` / `.done` / `.gone` | Hapus file? / File dihapus permanen; tidak ada riwayat versi. / Hapus / File dihapus / File sudah tidak ada | Delete file? / The file is deleted permanently; there is no version history. / Delete / File deleted / File no longer exists |
| `knowledge.deleteFolder.title` / `.body` / `.ack` / `.confirm` / `.done` / `.gone` | Hapus folder? / {count} file di bawah folder ini akan dihapus permanen. / Saya mengerti semua file ini akan dihapus / Hapus folder / {count} file dihapus / Folder sudah tidak ada | Delete folder? / {count} files under this folder will be deleted permanently. / I understand all these files will be deleted / Delete folder / {count} files deleted / Folder no longer exists |
| `knowledge.search.label` / `.placeholder` / `.clear` / `.total` / `.more` / `.empty` / `.onlyIn` | Cari knowledge / Cari kata atau "frasa" / Hapus pencarian / {shown} dari {total} hasil / Tampilkan lebih banyak / Tidak ada hasil / Hanya di {dir} | Search knowledge / Search words or "a phrase" / Clear search / {shown} of {total} results / Show more / No results / Only in {dir} |
| `knowledge.reason.<reason>` (23 kunci, satu per `REASONS`) | Pesan sama dengan tabel `Message()` overview 4.1; `too_large` = `Ukuran file melebihi batas {max}`, `upload_too_large` = `Ukuran total upload melebihi batas {max}. Pecah menjadi beberapa upload.`, `too_many_files` = `Jumlah file per upload melebihi batas; unggah sisanya lagi.` | Terjemahan Inggris setara (mis. `path_traversal` = `Path must not contain . or .. segments`) |

---

## Keputusan

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Folder `knowledge/base/` (rule murni + schema Zod bersama, tanpa services/stores/views), diimpor hanya lewat `base/index.ts` | `KnowledgeFile` dipakai 4 feature dan validasi path dipakai 3 feature; menyalin di tiap feature mudah menyimpang. Mencerminkan `base/` backend; lolos `no-cross-feature-internals` karena lewat `index.ts` |
| K2 | `updatefile` meng-import `getFile` dari `getfile/index.ts` | Editor butuh memuat file dan memuat ulang setelah konflik; API publik, bukan internals (rule 12.3) |
| K3 | Library: `marked` (parser, HTML mentah di-escape lewat renderer), `dompurify` (sanitasi), `mermaid` 11.x (bukan 12.x yang baru 2 minggu: API `initialize/render` stabil di 11), CodeMirror 6 (`codemirror` + `@codemirror/lang-markdown`/`state`/`view`). Mermaid dan CodeMirror dimuat lazy (chunk terpisah, same-origin, cocok CSP `script-src 'self'`) | Tiket: "CodeMirror atau setara", Mermaid wajib. Lazy agar halaman lain tidak membayar ukuran bundle |
| K4 | Renderer Markdown dan editor di `shared/` (`shared/utils/markdown/`, `MarkdownView`, `MarkdownEditor`) | Generik dan juga dibutuhkan T06 (deskripsi spec, rule 12.6 berlaku untuk spec juga). `shared/` tidak meng-import domain |
| K5 | Satu-satunya `{@html}` di `MarkdownView`, masukan selalu `sanitizeHtml(marked(...))`; gambar tidak dirender | AC5, rule 12.6; gambar adalah non-goal dan CSP memblokir gambar eksternal |
| K6 | Upload: satu request per target, tanpa pemecahan batch di klien; progres lewat XHR (`shared/services/upload.ts`) | AC7 (satu entri audit per upload); batas `MAX_UPLOAD_*` hanya diketahui server, kelebihan dilaporkan per item (`too_many_files`) atau 413 seluruh request dengan `max_bytes` |
| K7 | File tersembunyi dilewati di klien (tidak diunggah) dan tidak didukung tidak dikirim; validasi path lain hanya di backend | Folder `.git/` bisa sangat besar; tiket "Backend tetap otoritatif atas penolakan" |
| K8 | Upload campuran lewat `UploadTarget` yang didaftarkan `app/workspace/uploadTargets.ts`; module knowledge tidak tahu T06 | Rule 12.4; T06 cukup menambah satu entri |
| K9 | URL query (`path`, `dir`, `mode`, `new`, `q`, `in`) sebagai sumber state tab; route tetap `/projects/:projectID/:tab` T03 | Path memuat `/`; deep link, reload, Back. Tidak mengubah router |
| K10 | Draf editor di `sessionStorage` (per tab, bukan `localStorage`) | usecase-updatefile E2E negatif 7 (401 tanpa kehilangan draf); hilang saat tab ditutup, tidak tersebar ke perangkat lain |
| K11 | Konflik 409: muat ulang, bandingkan, dan "timpa dengan versi saya" hanya dari layar banding (simpan ulang dengan `version` terbaru) | Tiket: tidak menimpa "tanpa sadar"; timpa sadar setelah melihat diff tetap diizinkan |
| K12 | 404 di knowledge memicu `ctx.reload()` T03 alih-alih membedakan "project" vs "file" dari teks pesan | Backend memakai `NOT_FOUND` tanpa reason untuk keduanya; pesan server tidak boleh jadi kontrak |
| K13 | Tidak ada tombol yang disembunyikan per peran | Overview 1: semua level boleh semua aksi T05 |
| K14 | Hapus folder wajib centang konfirmasi (bukan mengetik nama) | Destruktif massal tapi tidak se-final hapus project; cukup satu langkah eksplisit |

---

## Migration

Tidak ada (frontend tidak menyentuh database). Migrasi backend `backend/migrations/project/<VERSION>_create_knowledge_files.sql` ada di `overview.md` bagian 6.

---

## Changes

`frontend/`:
- `package.json`, `package-lock.json` (ubah): 7 dependensi pasti (Metadata).
- `playwright.config.ts` (ubah): `webServer` array + project `knowledge-limits` (E5).
- `src/app.css` (ubah): `.md-body`, `.md-mermaid`, layout dua panel.

`frontend/src/shared/`:
- `services/http.ts` (ubah, aditif): ekspor `toApiError`, `notifyUnauthorized`; `http.test.ts` (ubah: test ekspor baru).
- `services/upload.ts`, `services/upload.test.ts` (baru).
- `utils/markdown/{render.ts, sanitize.ts, mermaid.ts, links.ts}` + `render.test.ts`, `sanitize.test.ts`, `links.test.ts`, `mermaid.test.ts` (baru).
- `utils/clipboard.ts`, `utils/clipboard.test.ts`, `utils/bytes.ts`, `utils/bytes.test.ts` (baru).
- `components/MarkdownView.svelte`, `components/MarkdownEditor.svelte`, `components/markdownEditor/codemirror.ts` (baru); `components/index.ts` (ubah); `components/markdown.test.ts` (baru).
- `i18n/id.json`, `i18n/en.json` (ubah): kunci tabel i18n.

`frontend/src/domain/project/knowledge/` (baru seluruhnya; `domain/project/.gitkeep` boleh dibiarkan):
- `base/{types.ts, path.ts, errors.ts, index.ts, path.test.ts, errors.test.ts, types.test.ts}`.
- `listtree/{types.ts, index.ts, services/listtree.service.ts, services/listtree.service.test.ts, stores/tree.store.ts, stores/tree.store.test.ts, components/{TreeView.svelte, NodeMenu.svelte, TreeView.test.ts}, views/KnowledgeTreeView.svelte, views/KnowledgeTreeView.test.ts, mocks/tree.ts}`.
- `getfile/{types.ts, index.ts, services/getfile.service.ts, services/getfile.service.test.ts, stores/viewer.store.ts, stores/viewer.store.test.ts, components/{Breadcrumb.svelte, FileMeta.svelte, FileContent.svelte, components.test.ts}, views/FileViewerView.svelte, views/FileViewerView.test.ts, mocks/files.ts}`.
- `createfile/{types.ts, index.ts, services/createfile.service.ts, services/createfile.service.test.ts, stores/newFile.store.ts, stores/newFile.store.test.ts, components/NewFileForm.svelte, views/NewFileView.svelte, views/NewFileView.test.ts}`.
- `updatefile/{types.ts, index.ts, services/updatefile.service.ts, services/updatefile.service.test.ts, stores/editor.store.ts, stores/editor.store.test.ts, stores/draft.ts, stores/draft.test.ts, components/{EditorToolbar.svelte, EditorSplit.svelte, ConflictDialog.svelte, CompareView.svelte, diff.ts, diff.test.ts, components.test.ts}, views/FileEditorView.svelte, views/FileEditorView.test.ts}`.
- `uploadfiles/{types.ts, index.ts, services/uploadfiles.service.ts, services/uploadfiles.service.test.ts, services/collect.ts, services/collect.test.ts, stores/upload.store.ts, stores/upload.store.test.ts, stores/plan.ts, stores/plan.test.ts, components/{DropZone.svelte, UploadReview.svelte, UploadProgress.svelte, UploadReport.svelte, components.test.ts}, views/UploadDialogView.svelte, views/UploadDialogView.test.ts}`.
- `movefile/{types.ts, index.ts, services/movefile.service.ts, services/movefile.service.test.ts, components/MoveFileForm.svelte, views/MoveFileDialog.svelte, views/MoveFileDialog.test.ts}`.
- `deletefile/{index.ts, services/deletefile.service.ts, services/deletefile.service.test.ts, views/DeleteFileDialog.svelte, views/DeleteFileDialog.test.ts}`.
- `deletefolder/{types.ts, index.ts, services/deletefolder.service.ts, services/deletefolder.service.test.ts, views/DeleteFolderDialog.svelte, views/DeleteFolderDialog.test.ts}`.
- `search/{types.ts, index.ts, services/search.service.ts, services/search.service.test.ts, stores/search.store.ts, stores/search.store.test.ts, components/{SearchBox.svelte, SearchResults.svelte, ScopeToggle.svelte, components.test.ts}, views/KnowledgeSearchView.svelte, views/KnowledgeSearchView.test.ts}`.

`frontend/src/app/`:
- `workspace/knowledgeUrl.ts`, `workspace/knowledgeUrl.test.ts` (baru).
- `workspace/uploadTargets.ts` (baru).
- `workspace/tabs.ts`, `workspace/tabs.test.ts` (ubah: entri `knowledge` di depan).
- `views/workspace/knowledge/{KnowledgeTab.svelte, KnowledgeSidebar.svelte, KnowledgeMain.svelte, KnowledgeDialogs.svelte, FolderPane.svelte, KnowledgeTab.test.ts}` (baru).
- `routes/router.test.ts` (ubah: kasus query knowledge).

`frontend/src/domain/core/audit-log/listentries/types.ts`: verifikasi/tambah `knowledge.file.moved` (A9).

`frontend/e2e/`: `helpers/knowledge.ts`, `helpers/project.ts` (bila belum ada), `fixtures/knowledge/docs/**`, `knowledge.spec.ts`, `knowledge-limits.spec.ts` (baru).

`frontend/.dependency-cruiser.cjs`, `scripts/test-arch.mjs`: tidak diubah (aturan yang ada cukup).

---

## Test Scenario (vitest + @testing-library/svelte, jsdom)

Nama test yang membuktikan kriteria tiket memakai awalan `T05-ACn` (rule 13.4).

### shared
- `http.test.ts`: `toApiError(409, '{"error":{"code":"CONFLICT","message":"m","details":{"reason":"version_conflict"}},"trace_id":"t"}')` -> code/details/traceId; body bukan JSON -> `INTERNAL`; `request()` lama tetap lulus.
- `upload.test.ts` (XHR palsu): header CSRF + Accept terpasang, tanpa `Content-Type` manual; progres `sending` lalu `processing`; 200 valid -> data; 200 tidak sesuai schema -> `INVALID_RESPONSE`; 413 envelope -> `ApiError` dengan `details.max_bytes`; 401 -> `notifyUnauthorized` dipanggil; `onerror` -> `NETWORK`; abort -> `AbortError` dan `xhr.abort()`; URL absolut ditolak.
- `sanitize.test.ts` / `render.test.ts` (**T05-AC5**): `<script>alert(1)</script>` tampil sebagai teks (tidak ada elemen `script`); `[x](javascript:alert(1))`, `[x](JaVaScRiPt:...)`, `[x](data:text/html,...)`, `[x](vbscript:...)` -> tidak ada `href` berbahaya (teks saja); `<img src=x onerror=alert(1)>` -> tidak ada `img`, tidak ada atribut `onerror`; `<a href="https://x" onclick=...>` mentah -> di-escape; tautan eksternal mendapat `rel="noopener noreferrer nofollow"` dan `target="_blank"`; heading id berawalan `md-`; task list checkbox `disabled`; tabel GFM dirender; blok ```mermaid menjadi `.md-mermaid` dan kode tercatat di `mermaid[]`; `![alt](x.png)` -> teks alt tanpa `img`; tautan relatif dengan `resolveLink` -> href hasil resolver, resolver `null` -> teks.
- `links.test.ts`: `resolveRelativePath('docs/auth', '../a.md') === 'docs/a.md'`; `('', '../x.md') === null`; `('docs', './b.md#h') === 'docs/b.md'`; `isSafeHref` tabel skema.
- `mermaid.test.ts` (modul `mermaid` di-mock): `initialize` dipanggil sekali dengan `securityLevel: 'strict'` dan `startOnLoad: false`; SVG hasil berisi `<script>`/`onload`/`foreignObject` dibersihkan; `render` melempar -> `{ ok: false }`; kode > 50 000 karakter tidak dirender.
- `markdown.test.ts` (`MarkdownView`, `MarkdownEditor`): **T05-AC5** blok mermaid dirender menjadi SVG (mock) di dalam `role="img"`; gagal -> `<pre>` + `md.mermaidError`; ganti `source` cepat tidak menyisipkan SVG usang; `MarkdownEditor` fallback textarea bila import CodeMirror gagal, `bind:value` dua arah, Mod-S memanggil `onsave`.
- `bytes.test.ts`, `clipboard.test.ts` (clipboard API ada/tidak/gagal).

### base
- `path.test.ts` (tabel sama dengan `path_test.go` backend): `docs/auth/login.md` valid; `''` path_empty; `/a.md` path_absolute; `../x.md`, `a/./b.md` path_traversal; `a//b.md`, `a/` path_empty_segment; `a\u0000b.md`, `a\nb.md` path_control_char; 513 rune path_too_long, 512 valid; segmen 256 rune segment_too_long; emoji dihitung 1 rune; `Docs/A.MD` dipertahankan; `a\b.md` valid. `validateNewFilePath('.env')` -> hidden, `('config.yaml')` -> unsupported_type, `('docs/x.MD')` valid. `validatePrefix('docs/')` valid, `normalizePrefix('docs/') === 'docs'`. `ancestors`, `dirName`, `joinPath`.
- `errors.test.ts`: `errorMessage` memakai `knowledge.reason.too_large` dengan `{max}` dari `details.max_bytes`; reason tak dikenal -> pesan server; non-ApiError -> `error.internal`.
- `types.test.ts`: `KnowledgeFileSchema` menolak `version` huruf besar/63 karakter, menerima contoh body overview; `ConflictDetailsSchema` menerima contoh 409.

### listtree
- Service: URL `/api/projects/p%2F1/knowledge/tree?prefix=docs%2Fauth` (id dan prefix di-encode); root tanpa `prefix`; respons rusak -> `INVALID_RESPONSE`.
- Store: `toggle` + cache (buka-tutup-buka tidak meminta ulang); `invalidateAll` mengembalikan root + expanded; `visibleRows` urutan folder lalu file, kedalaman benar.
- `TreeView.test.ts`: `role="tree"`/`treeitem`/`aria-expanded`/`aria-level`; panah Bawah/Atas/Kanan/Kiri/Home/End; nama `<img src=x onerror=alert(1)>.md` tampil literal; `file_count` tampil; menu aksi folder vs file berbeda; Esc menutup menu dan fokus kembali.
- `KnowledgeTreeView.test.ts` (**T05-AC1** bentuk UI): root -> `docs`; buka `docs` -> satu request `prefix=docs` -> `auth`, `a.md`; `selectedPath='docs/auth/flow/x.md'` memuat `docs`, `docs/auth`, `docs/auth/flow` berurutan lalu menandai `aria-selected`; `refreshKey` naik -> memuat ulang hanya root + expanded; folder kosong -> `folderEmpty`; error satu folder -> baris error + coba lagi tanpa merusak folder lain; 404 -> `onprojectgone`; `total_files=0` -> `onempty(true)`.

### getfile
- Service: query `path=catatan%20rapat%2F%C3%A9.md` (encode unicode/spasi); schema.
- Store: respons usang (path lama) diabaikan; 404 -> `notFound`.
- Components: breadcrumb segmen folder memanggil `ondir('docs/auth')`; salin path -> toast; `.txt` dirender `<pre>` tanpa Markdown; `.md` lewat `MarkdownView`.
- `FileViewerView.test.ts`: loading -> ready (judul, meta, tombol); `title` kosong -> nama file; 404 -> `knowledge.viewer.notFound` + `onnotfound` sekali; error 500 -> `ErrorState` + coba lagi memanggil ulang; tautan relatif ke `.md` memakai `resolveLink`; **T05-AC5** file berisi `<script>`/`javascript:` tidak menghasilkan `script`/href berbahaya di DOM.

### createfile
- Store: `pathError` hanya setelah disentuh; `validateNewFilePath` untuk `../x.md`, `.env`, `config.yaml`; awal `dir` `docs` -> path `docs/`.
- `NewFileView.test.ts`: simpan valid -> POST body `{path, content}` -> `oncreated`; 409 `path_exists` -> error inline + tautan buka; 413 -> pesan dengan batas, isi editor tetap; 400 `project_file_limit` -> `formError`; tombol Simpan nonaktif saat path tidak valid; `ondirtychange(true)` saat mengetik.

### updatefile
- Service: PUT ke `.../knowledge/files?path=docs%2Fa.md` body `{content, version}`; `parseConflict` hanya untuk 409 dengan detail valid.
- `editor.store.test.ts`: `dirty`; `saved` memperbarui base/version; `acceptCurrent`; `rebaseOnCurrent` mempertahankan draf dengan version baru.
- `draft.test.ts`: simpan/muat/hapus; `sessionStorage` melempar (mock) -> tidak error, `loadDraft` null.
- `diff.test.ts`: tambah/hapus/sama; input besar -> `null`.
- `FileEditorView.test.ts`:
  - **T05-AC3**: muat (version A) -> edit -> simpan -> 409 `version_conflict` dengan `current` -> `ConflictDialog` menampilkan label + waktu; isi editor tidak berubah dan tidak ada PUT lanjutan otomatis; "Muat versi terbaru" -> editor berisi `current.content`, simpan berikutnya memakai `current.version`; "Bandingkan" -> `CompareView` dua kolom; "Timpa dengan versi saya" -> PUT dengan `version = current.version` dan isi draf.
  - Dua simpan berturut-turut memakai version dari respons pertama.
  - Simpan tanpa perubahan tetap PUT dan sukses diam-diam (no-op D12).
  - 404 -> banner `deletedRemotely`, draf tetap, `onnotfound`.
  - 413 -> `formError` dengan batas; isi tetap.
  - Draf: mengetik menyimpan draf (timer palsu); remount dengan draf berbeda -> banner pulihkan; "Pulihkan" mengisi editor dengan `baseVersion` draf; sukses simpan menghapus draf.
  - `beforeunload` terpasang hanya saat dirty; Batal saat dirty -> konfirmasi.

### uploadfiles
- `plan.test.ts`: `.git/config`, `a/.DS_Store` -> skipped (tidak dikirim); `logo.png` -> unsupported; `.md/.markdown/.txt` -> target knowledge; dengan target palsu `accepts(.yaml)` -> `.yaml` ke target itu; `targetDir='docs'` -> path `docs/<rel>`; `totalBytes` benar.
- `collect.test.ts`: `fromInput` memakai `webkitRelativePath`, fallback `name`; `fromDrop` dengan entri palsu 3 tingkat dan `readEntries` yang mengembalikan batch berulang -> semua file dengan `fullPath` tanpa `/` depan; fallback `dt.files`; batas 10 000.
- `uploadfiles.service.test.ts`: `FormData` berisi field `files` dengan nama file = path relatif (`docs/auth/login.md`), bukan basename; item server dipetakan ke `ReportItem` dengan `reasonText` i18n.
- `upload.store.test.ts`: fase, `counts`, filter `failed` = rejected + unsupported, `updated` = ditimpa.
- `components.test.ts`: `UploadReview` angka per target dan tombol nonaktif bila kosong; `UploadReport` filter radiogroup; path ber-HTML tampil literal; banner error target.
- `UploadDialogView.test.ts`:
  - **T05-AC1** (bentuk UI): pilih folder 3 tingkat -> review -> unggah -> laporan 3 dibuat -> `onuploaded`; respons kedua semua `unchanged` -> kartu "Tidak berubah" 3.
  - **T05-AC2** (bentuk UI): respons dengan `path_traversal`, `path_too_long`, `too_large`, `invalid_encoding`, `hidden` -> alasan terbaca per item; item valid "Dibuat".
  - 413 `upload_too_large` -> banner dengan batas, semua file target itu "Ditolak", `onuploaded` tidak dipanggil.
  - Dua target (palsu): knowledge sukses, target kedua 500 -> laporan gabungan + banner target kedua; urutan item = urutan file asli.
  - Batalkan saat `sending` -> `knowledge.upload.cancelled`.

### movefile / deletefile / deletefolder
- `MoveFileDialog.test.ts`: awal = `from`; sama -> tombol nonaktif + `same_path`; `docs/a.yaml` -> unsupported; 409 `path_exists` -> inline; 404 -> `sourceGone` + `onnotfound`; 200 -> `onmoved` dengan path baru.
- `DeleteFileDialog.test.ts`: Batal tidak memanggil service; konfirmasi -> DELETE `?path=` -> `ondeleted`; 404 -> toast `gone` + `ondeleted`.
- `DeleteFolderDialog.test.ts`: menampilkan `{count}`; tombol nonaktif sampai centang; 200 -> toast dengan `deleted`; 404 -> `gone`; path kosong tidak pernah dikirim.

### search
- Service: query `q`, `prefix` (dihilangkan bila kosong), `limit`; kueri 300 karakter dipotong 200.
- Store: hasil usang diabaikan.
- `components.test.ts`: cuplikan `[{text:'Alur '},{text:'login',highlight:true}]` -> satu `<mark>login</mark>`; segmen berisi `<script>` tampil literal (**T05-AC5**); total + tombol "lebih banyak" muncul bila `total > items.length`.
- `KnowledgeSearchView.test.ts` (**T05-AC4** bentuk UI): urutan item sesuai respons (`login.md` pertama); `q=''` tidak request; `{items:[], total:0}` -> `empty` tanpa error; 400 `path_traversal` -> pesan alasan; 404 -> `onprojectgone`; "lebih banyak" -> `limit=50`.

### app
- `knowledgeUrl.test.ts`: parse/build bolak-balik; `mode=edit` tanpa `path` -> view; `new=1` menang; path dengan `/`, spasi, unicode ter-encode; urutan param stabil.
- `tabs.test.ts`: `knowledge` di indeks 0, `defaultTabKey() === 'knowledge'`, key unik.
- `router.test.ts`: `/projects/abc/knowledge?path=docs%2Fa.md` cocok tab `knowledge`.
- `KnowledgeTab.test.ts` (service di-mock lewat `vi.mock` index feature): URL `?path=` -> viewer; `?mode=edit` -> editor; `?new=1&dir=docs` -> form baru dengan `docs/`; `?q=login` -> hasil pencarian menggantikan pohon, kosongkan -> pohon kembali; dirty + klik file lain -> konfirmasi; 404 viewer -> `ctx.reload` dipanggil + pohon refresh; `extraUploadTargets` diteruskan ke dialog upload.

### Arsitektur
- `npm run depcruise` dan `npm run test:arch` lulus: tidak ada import dari `knowledge/<feature>/` ke internals feature lain (hanya `base/index.ts`, `getfile/index.ts`); `app/` hanya meng-import `index.ts`; `shared/` tidak meng-import domain/app.
- Test grep (di `components.test.ts` shared): hanya `MarkdownView.svelte` yang memuat `{@html` di `src/domain/project/knowledge/**` dan `src/shared/**` (memindai sumber via `import.meta.glob('?raw')`).

---

## E2E Scenarios (Playwright terhadap binary; butuh T01, T02, T03, T05 merge)

`e2e/knowledge.spec.ts` (server default). Tiap test membuat project sendiri lewat API (paralel aman). Listener konsol gagal pada pelanggaran CSP.

Positif:
1. **T05-AC1 upload folder**: developer A login -> buat project -> workspace terbuka di tab Knowledge (default) -> `EmptyState` -> Upload -> "Pilih folder" (`setInputFiles` direktori `fixtures/knowledge/docs`) -> review menampilkan file knowledge, `.git/config` dilewati, `logo.png` tidak didukung -> Unggah -> laporan "Dibuat" sesuai jumlah `.md` -> pohon: buka `docs` > `auth` > `flow` menampilkan `x.md` (request `tree?prefix=` satu per folder).
2. **Upload ulang** folder yang sama -> laporan semua "Tidak berubah". Ubah satu file (fixture salinan temp) lalu upload ulang -> 1 "Ditimpa", sisanya tidak berubah; buka file -> isi baru tampil.
3. **Viewer + Mermaid**: klik `docs/auth/login.md` -> URL `?path=docs%2Fauth%2Flogin.md`; breadcrumb `docs / auth / login.md`; diagram Mermaid dirender sebagai `svg` di dalam `role="img"`; "Salin path" -> clipboard berisi `docs/auth/login.md` (izin clipboard di context).
4. **Edit**: Edit -> ubah teks -> indikator "Belum disimpan" -> Ctrl+S -> "Tersimpan", viewer menampilkan isi baru; reload halaman tetap. Edit lagi dan simpan lagi tanpa reload -> sukses (version baru dipakai).
5. **File baru**: "File baru" -> path `notes/todo.md`, isi `# Todo` -> Simpan -> pohon menampilkan `notes/todo.md`, viewer judul "Todo"; reload: masih ada.
6. **Pindah**: menu `notes/todo.md` -> Pindah -> `archive/todo.md` -> pohon: `archive/todo.md` ada, folder `notes` hilang; viewer di path baru.
7. **Pencarian (T05-AC4)**: ketik `login` -> hasil menggantikan pohon, `docs/auth/login.md` di urutan pertama di atas `docs/guide.md`, kata tersorot dalam `<mark>`; klik hasil -> viewer membuka file; kosongkan -> pohon kembali. Frasa `"reset password"` hanya file yang memuatnya. Centang "Hanya di docs/auth" -> hanya file di folder itu.
8. **Hapus folder**: menu `archive` -> Hapus folder -> dialog menampilkan jumlah file; centang + konfirmasi -> `archive` hilang, `docs/` tetap.
9. **Hapus file**: buka `docs/a.md` -> Hapus -> konfirmasi -> hilang dari pohon, panel kanan ke folder `docs`; membuka URL lama -> "File tidak ditemukan".
10. **Super Admin non-anggota**: admin membuka project A -> tab Knowledge -> lihat, edit, upload berhasil.
11. **Upload ke folder**: menu folder `docs` -> Upload ke sini -> pilih file `extra.md` -> tersimpan di `docs/extra.md`.
12. **Drag-drop file**: `dispatchEvent('drop')` dengan `DataTransfer` berisi 2 file `.md` -> review -> unggah -> tersimpan di root.
13. **Audit**: setelah skenario upload 100 file (dibuat lewat API helper), admin membuka Audit Log -> satu entri `knowledge.upload.completed` untuk upload itu (**T05-AC7**).

Negatif:
1. **T05-AC2**: upload lewat UI berisi `../x.md` (lewat helper API karena browser tidak bisa membuat nama itu; laporan dicek dari respons) dan lewat UI: file 2 MB `.md`, biner `.md`, `.git/config`, `logo.png` + 2 file valid -> laporan: "Ditolak" dengan alasan ukuran/encoding, "Dilewati" (tersembunyi), "Tidak didukung"; 2 file valid tampil di pohon. Filter "Gagal" hanya menampilkan yang ditolak/tidak didukung.
2. **T05-AC3 konflik**: dua context browser (A dan B, anggota) membuka `docs/auth/login.md` mode edit; A simpan; B simpan -> dialog konflik dengan nama A; isi tersimpan tetap milik A (dicek via API); B "Bandingkan" -> dua kolom; "Muat versi terbaru" -> editor B berisi isi A.
3. **T05-AC5 XSS**: buka `xss.md` -> `page.on('dialog')` tidak pernah terpicu; tidak ada elemen `script` di dalam `.md-body`; tautan `javascript:` tidak memiliki `href`; tidak ada `img`.
4. **Non-anggota (T05-AC6)**: developer C membuka `/projects/<A>/knowledge?path=docs%2Fa.md` -> "Project tidak ditemukan"; `page.request.get/put/post/delete` ke 9 endpoint project A (dengan CSRF) -> 404.
5. **Path tidak valid**: File baru `../x.md` -> pesan traversal, tombol nonaktif; `.env` -> pesan tersembunyi; `config.yaml` -> tipe tidak didukung. Lewat `page.request.post` (melewati validasi klien) -> 400 dengan `details.reason` yang sama.
6. **Path sudah ada**: File baru `docs/a.md` (sudah ada) -> 409 `file dengan path ini sudah ada` + tautan "Buka file yang ada"; isi lama tidak berubah.
7. **Pindah ke path yang ada** -> pesan 409 inline; kedua file utuh.
8. **Hapus dua kali**: tab 1 dan tab 2 membuka file; tab 1 hapus; tab 2 hapus -> toast "File sudah tidak ada", pohon menyegarkan, tanpa crash.
9. **Folder dihapus di tab lain** lalu dibuka di pohon tab pertama -> "Folder kosong" tanpa error.
10. **Pencarian karakter aneh**: `" & : or -`, `"unclosed`, kueri 300 karakter -> tidak ada toast error, "Tidak ada hasil" atau hasil valid.
11. **CSRF**: `page.request.post(.../knowledge/files)` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
12. **Sesi kedaluwarsa saat edit**: hapus cookie sesi -> simpan -> diarahkan ke `/login?next=...mode%3Dedit`; login -> editor menampilkan banner draf; "Pulihkan" -> isi draf kembali; simpan -> sukses.
13. **Batal**: dialog hapus file/folder -> Batal -> tidak ada request DELETE (`page.waitForRequest` tidak terjadi), pohon tidak berubah.

`e2e/knowledge-limits.spec.ts` (server kedua `MAX_UPLOAD_BYTES=2097152`, `MAX_PROJECT_FILES=5`):
14. Upload total 3 MB (tiga file 1 MB) -> banner "Ukuran total upload melebihi batas 2 MB", pohon tidak berubah.
15. Project berisi 4 file -> upload 3 file baru + 1 file yang ada diubah -> 1 dibuat, 2 ditolak `project_file_limit`, 1 ditimpa. File baru dari editor -> pesan "project sudah mencapai batas jumlah file".

---

## Request/Response body

Format error seragam (dari `foundation.yml`, dipetakan ke `ApiError {status, code, message, details, traceId}` oleh `shared/services/http.ts`):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

Kode yang ditangani UI: `INVALID_ARGUMENT` (400, `details.reason`), `UNAUTHORIZED` (401 -> login), `CSRF_REQUIRED` (403, tidak terjadi lewat UI), `NOT_FOUND` (404, tanpa reason), `CONFLICT` (409, `path_exists` / `version_conflict` + `current`), `PAYLOAD_TOO_LARGE` (413, `reason` + `max_bytes`), `TIMEOUT`/`INTERNAL` (`ErrorState`/toast + trace id), `NETWORK` dan `INVALID_RESPONSE` (dibuat klien).

### GET /api/projects/{projectID}/knowledge/tree?prefix=docs

200 (`TreeLevelSchema`):
```json
{
  "prefix": "docs",
  "folders": [ { "name": "auth", "path": "docs/auth", "file_count": 2 } ],
  "files": [ { "name": "a.md", "path": "docs/a.md", "size_bytes": 120, "updated_at": "2026-10-24T09:00:00Z" } ],
  "total_files": 3
}
```
Root (tanpa `prefix`) kosong: `{ "prefix": "", "folders": [], "files": [], "total_files": 0 }`. 400: `{"error":{"code":"INVALID_ARGUMENT","message":"path tidak boleh memuat segmen . atau ..","details":{"reason":"path_traversal"}},"trace_id":"..."}`. 404 (non-anggota): `{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan"},"trace_id":"..."}`.

### GET /api/projects/{projectID}/knowledge/files?path=docs%2Fauth%2Flogin.md

200 (`KnowledgeFileSchema`):
```json
{
  "path": "docs/auth/login.md",
  "title": "Login",
  "content": "# Login\n\nAlur login...\n\n```mermaid\nflowchart LR\n  A-->B\n```\n",
  "size_bytes": 58,
  "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea",
  "created_at": "2026-10-24T09:00:00Z",
  "created_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" },
  "updated_at": "2026-10-24T09:00:00Z",
  "updated_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" }
}
```
404: `{"error":{"code":"NOT_FOUND","message":"file tidak ditemukan"},"trace_id":"..."}`. 400 tanpa path: `{"error":{"code":"INVALID_ARGUMENT","message":"path wajib diisi, mis. docs/auth/login.md","details":{"reason":"path_empty"}},"trace_id":"..."}`.

### POST /api/projects/{projectID}/knowledge/files

Request (header `X-Requested-With: grimoire`, `Content-Type: application/json`):
```json
{ "path": "notes/todo.md", "content": "# Todo\n\n- [ ] tulis plan\n" }
```
201: `KnowledgeFile` (bentuk seperti GET, `path` = `notes/todo.md`, `title` = `Todo`).
409: `{"error":{"code":"CONFLICT","message":"file dengan path ini sudah ada","details":{"reason":"path_exists"}},"trace_id":"..."}`
400: `{"error":{"code":"INVALID_ARGUMENT","message":"path yang diawali titik (mis. .git/) tidak didukung","details":{"reason":"hidden"}},"trace_id":"..."}` / `unsupported_type` / `project_file_limit` / `invalid_encoding`.
413: `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"ukuran file melebihi batas 1048576 byte","details":{"reason":"too_large","max_bytes":1048576}},"trace_id":"..."}`

### PUT /api/projects/{projectID}/knowledge/files?path=docs%2Fauth%2Flogin.md

Request:
```json
{ "content": "# Login\n\nAlur login v2", "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea" }
```
200: `KnowledgeFile` dengan `version` baru. 409 (UI: `ConflictDialog`):
```json
{
  "error": {
    "code": "CONFLICT",
    "message": "file sudah diubah orang lain; muat ulang sebelum menyimpan",
    "details": {
      "reason": "version_conflict",
      "current": {
        "content": "# Login\n\nAlur login versi Sari",
        "version": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
        "updated_at": "2026-10-24T09:05:00Z",
        "updated_by_label": "Sari"
      }
    }
  },
  "trace_id": "..."
}
```
400: `{"error":{"code":"INVALID_ARGUMENT","message":"version wajib diisi dengan nilai dari getfile","details":{"reason":"invalid_version"}},"trace_id":"..."}`. 404 `file tidak ditemukan`. 413 seperti createfile.

### DELETE /api/projects/{projectID}/knowledge/files?path=docs%2Fa.md

204 tanpa body (`del` mengembalikan `undefined`). 404 `{"error":{"code":"NOT_FOUND","message":"file tidak ditemukan"},"trace_id":"..."}`.

### DELETE /api/projects/{projectID}/knowledge/folders?path=archive

200 (`DeleteFolderResultSchema`): `{ "deleted": 3 }`. 404: `{"error":{"code":"NOT_FOUND","message":"folder tidak ditemukan atau kosong"},"trace_id":"..."}`. 400 (tidak terjadi dari UI): `{"error":{"code":"INVALID_ARGUMENT","message":"path folder wajib diisi; root tidak bisa dihapus sekaligus","details":{"reason":"prefix_required"}},"trace_id":"..."}`.

### POST /api/projects/{projectID}/knowledge/files/move

Request:
```json
{ "from": "notes/todo.md", "to": "archive/todo.md" }
```
200: `KnowledgeFile` dengan `path` baru dan `version` sama. 409 `path_exists`; 404 `file tidak ditemukan`; 400 `{"error":{"code":"INVALID_ARGUMENT","message":"path tujuan sama dengan path asal","details":{"reason":"same_path"}},"trace_id":"..."}` / `hidden` / `unsupported_type`.

### POST /api/projects/{projectID}/knowledge/upload (XHR multipart)

Request: `Content-Type: multipart/form-data; boundary=...` (ditulis browser), `X-Requested-With: grimoire`. Dibangun dengan `form.append('files', file, 'docs/auth/login.md')`:
```
--b
Content-Disposition: form-data; name="files"; filename="docs/auth/login.md"
Content-Type: text/markdown

# Login
...
--b
Content-Disposition: form-data; name="files"; filename="docs/big.md"
Content-Type: text/markdown

...
--b--
```
200 (`UploadResultSchema`):
```json
{
  "summary": { "created": 2, "updated": 1, "unchanged": 3, "skipped": 0, "rejected": 2 },
  "items": [
    { "path": "docs/auth/login.md", "status": "created" },
    { "path": "docs/auth/flow/x.md", "status": "created" },
    { "path": "docs/big.md", "status": "rejected", "reason": "too_large" },
    { "path": "docs/bin.md", "status": "rejected", "reason": "invalid_encoding" },
    { "path": "docs/readme.md", "status": "updated" },
    { "path": "docs/a.md", "status": "unchanged" },
    { "path": "docs/b.md", "status": "unchanged" },
    { "path": "docs/c.md", "status": "unchanged" }
  ]
}
```
Laporan UI menambahkan item klien (tidak dikirim): `{ "path": ".git/config", "status": "skipped", "reasonText": "path yang diawali titik ... tidak didukung", "target": "client" }`, `{ "path": "docs/logo.png", "status": "unsupported", "target": "client" }`.
413: `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"ukuran total upload melebihi batas","details":{"reason":"upload_too_large","max_bytes":52428800}},"trace_id":"..."}`
400: `{"error":{"code":"INVALID_ARGUMENT","message":"upload tidak berisi file","details":{"reason":"no_files"}},"trace_id":"..."}`

### GET /api/projects/{projectID}/knowledge/search?q=login&prefix=docs&limit=20

200 (`SearchResultSchema`):
```json
{
  "items": [
    {
      "path": "docs/auth/login.md",
      "title": "Login",
      "score": 0.83,
      "snippet": [ { "text": "Alur ", "highlight": false }, { "text": "login", "highlight": true }, { "text": " memakai email dan password", "highlight": false } ]
    },
    {
      "path": "docs/guide.md",
      "title": "Panduan",
      "score": 0.12,
      "snippet": [ { "text": "… setelah ", "highlight": false }, { "text": "login", "highlight": true }, { "text": " berhasil …", "highlight": false } ]
    }
  ],
  "total": 2
}
```
Kueri tanpa kata: `{ "items": [], "total": 0 }`. 400: `{"error":{"code":"INVALID_ARGUMENT","message":"limit harus bilangan bulat","details":{"reason":"invalid_limit"}},"trace_id":"..."}`.

### Umum

Tanpa sesi (semua route): `401 {"error":{"code":"UNAUTHORIZED","message":"perlu login"},"trace_id":"..."}` -> handler global ke `/login?next=<path+query>`. Mutasi tanpa CSRF: `403 {"error":{"code":"CSRF_REQUIRED","message":"..."},"trace_id":"..."}`.

Respons tidak sesuai schema (dibuat klien, bukan server): `ApiError { status: 200, code: "INVALID_RESPONSE", message: t('error.invalid_response'), details: <zod issues> }` -> `ErrorState`.

---

## Risiko dan catatan

- **T03 FE belum merge** saat plan ini ditulis (tidak ada `app/workspace/` di `main`). FE-0.1 menghentikan pekerjaan bila prasyarat belum ada.
- **Ukuran bundle**: Mermaid (~800 KB gzip) dan CodeMirror (~150 KB gzip) hanya dimuat saat ada blok mermaid / editor dibuka. V mengecek chunk utama.
- **CSP**: Mermaid menyisipkan `<style>` di SVG dan CodeMirror memakai style inline; diizinkan `style-src 'unsafe-inline'`. Keduanya tidak butuh `unsafe-eval` (dibuktikan listener CSP di e2e). Bila ternyata butuh, JANGAN melonggarkan CSP di PR ini; laporkan.
- **File besar di pratinjau**: isi hingga 1 MiB; pratinjau otomatis dimatikan > 200 KB.
- **Folder sangat besar di pohon**: satu level dimuat utuh (backend tanpa paging untuk `tree`); di atas ~2000 entri satu level render bisa lambat. Diterima pada skala desain (maks 5000 file per project).
- **`webkitGetAsEntry`** non-standar tapi didukung Chrome/Edge/Firefox/Safari modern; fallback `dt.files` tanpa folder.
- **macOS NFD**: nama file dari Finder bisa NFD; dikirim apa adanya (overview D11).
- **T06 FE** cukup menambah target di `app/workspace/uploadTargets.ts`; tidak ada perubahan module knowledge.
