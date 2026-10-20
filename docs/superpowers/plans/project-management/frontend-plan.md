# T03 project/management: Frontend Plan (Svelte)

Tiket: `docs/tickets/T03-project-management.md`. Plan bersama: `overview.md` (D1-D6, bagian 4.7 DTO, 7 F9 i18n, 11 E2E, 12 body, 13 OpenAPI). Kontrak per endpoint: `usecase-createproject.md`, `usecase-listprojects.md`, `usecase-getproject.md`, `usecase-updateproject.md`, `usecase-deleteproject.md`, `usecase-listmembers.md`, `usecase-searchmembercandidates.md`, `usecase-addmember.md`, `usecase-removemember.md`, `usecase-changememberrole.md`. `checkaccess` dan `getprojectbus` adalah handler bus tanpa UI. Konvensi: `docs/tickets/README.md` bagian "Frontend (Svelte)" dan `rules.txt` bagian 12 (rules menang soal arsitektur).

Dokumen ini adalah **satu-satunya plan frontend** untuk T03. Bila bagian "Frontend" di plan usecase (terutama `usecase-listprojects.md` FE-1..FE-5, APP-1) berbeda dengan dokumen ini, **ikuti dokumen ini**. Selisihnya dicatat di bagian "Keputusan dan selisih dengan plan usecase".

Konteks yang dimuat: tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: `rules.txt` (12, 13), `docs/tickets/README.md`, tiket T03, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (peran Owner/Member/Super Admin, hapus permanen), `docs/superpowers/plans/foundation/overview.md`, `docs/superpowers/plans/core-auth/frontend-plan.md` (route `/` wajib login, `AppLayout` nav, `e2e/helpers/auth.ts`, smoke e2e K9), `docs/superpowers/plans/core-audit-log/frontend-plan.md` (pola URL sebagai sumber state, store per instance), dan kode FE di `main`: `shared/services/http.ts` (`get/post/patch/del`, `ApiError`, `isApiError`, CSRF otomatis untuk mutasi, handler 401 global), `shared/validation/error.ts` (`ErrorEnvelope`, `KnownErrorCode`, `fieldErrorsOf`), `shared/stores/{session,i18n,toast}.ts`, `shared/components/{Button,Input,Dialog,Table,Pagination,Spinner,EmptyState,ErrorState,Toast,LocaleSwitch}`, `app/routes/{router,guard,index}.ts` (pola `:param`, view menerima prop `params`), `app/App.svelte`, `app/layouts/AppLayout.svelte`, `app/views/AuditLogView.svelte`, `domain/core/audit-log/**` (pola feature yang sudah jadi), `.dependency-cruiser.cjs` (`domain-no-app`, `no-cross-feature-internals`, `outside-domain-only-via-index`), `e2e/smoke.spec.ts`, `playwright.config.ts`.

---

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack (dokumen ini: frontend saja; backend di `overview.md` dan `usecase-*.md`) |
| Domain | project |
| Module | management (folder FE `frontend/src/domain/project/management/`) |
| Feature FE | `listprojects`, `createproject`, `getproject`, `updateproject`, `deleteproject`, `members` (gabungan UI `listmembers` + `searchmembercandidates` + `addmember` + `removemember` + `changememberrole`, sesuai overview bagian 8 dan 15) |
| Route UI | `/` (beranda: daftar project), `/projects/:projectID` (redirect ke tab default), `/projects/:projectID/:tab` (workspace; tab T03: `members`, `settings`) |
| Akses UI | Semua route `meta: { auth: true, layout: 'app' }`. Tombol menyesuaikan `access_level` (UX saja; otorisasi di backend, rule 12.7) |
| Kontrak API | `api/modules/project/management.yml` (`info.version: 0.1.0`), schema `ProjectSummary`, `ProjectDetail`, `ProjectList`, `Member`, `MemberList`, `MemberCandidateList`, `Error` dari `api/modules/infra/foundation.yml` |
| Prasyarat | T00 (merged), T02 FE (merged), T01 FE (bootstrap sesi `/api/auth/me`, `/login`, `AppLayout` nav, `e2e/helpers/auth.ts`). Backend T03 F + U3-U12 |

Endpoint yang dipanggil frontend (semua prefix `/api`, tanpa versi di path; versi kontrak `0.1.0`; mutasi otomatis membawa `X-Requested-With: grimoire` dari `shared/services/http.ts`):

| Endpoint | Method | Version | Dipakai oleh (service FE) | Akses (backend) |
|---|---|---|---|---|
| `/api/projects` | GET | 0.1.0 | `listprojects/services/listprojects.service.ts` | developer (miliknya), super_admin (semua) |
| `/api/projects` | POST | 0.1.0 | `createproject/services/createproject.service.ts` | developer, super_admin |
| `/api/projects/{projectID}` | GET | 0.1.0 | `getproject/services/getproject.service.ts` | anggota, super_admin |
| `/api/projects/{projectID}` | PATCH | 0.1.0 | `updateproject/services/updateproject.service.ts` | anggota, super_admin |
| `/api/projects/{projectID}` | DELETE | 0.1.0 | `deleteproject/services/deleteproject.service.ts` | owner, super_admin |
| `/api/projects/{projectID}/members` | GET | 0.1.0 | `members/services/members.service.ts` (`listMembers`) | anggota, super_admin |
| `/api/projects/{projectID}/member-candidates` | GET | 0.1.0 | `members/services/members.service.ts` (`searchCandidates`) | owner, super_admin |
| `/api/projects/{projectID}/members` | POST | 0.1.0 | `members/services/members.service.ts` (`addMember`) | owner, super_admin |
| `/api/projects/{projectID}/members/{userID}` | DELETE | 0.1.0 | `members/services/members.service.ts` (`removeMember`) | owner, super_admin, diri sendiri |
| `/api/projects/{projectID}/members/{userID}` | PATCH | 0.1.0 | `members/services/members.service.ts` (`changeMemberRole`) | owner, super_admin |

Bus `project.management.check-access`, `project.management.get-project` dan bus yang dikonsumsi backend (`core.auth.get-user`, `core.auth.find-developers`, `core.audit.record`, `project.*.count-*`) tidak disentuh frontend. Frontend tidak menambah endpoint dan tidak memanggil endpoint module lain.

---

## Summary Plan

Frontend T03 menyediakan dua permukaan:

1. **Beranda `/`**: daftar project milik user (developer) atau semua project (Super Admin) sebagai tabel kartu responsif: nama, deskripsi ringkas, peran saya (`Owner`/`Member`, atau badge "Bukan anggota" untuk Super Admin yang bukan anggota), jumlah file dan spec (`null` tampil "-"), dan waktu update terakhir. Ada pencarian (`q`, debounce 300 ms) dan paginasi (`page`, 20 per halaman). **URL adalah sumber kebenaran** (`/?q=pay&page=2`), sehingga reload, Back, dan deep link bekerja (pola T02). Tombol "Project baru" membuka dialog `createproject`; sukses langsung masuk workspace project baru.
2. **Workspace `/projects/:projectID/:tab`**: header project (nama, deskripsi, peran saya, penanda "Bukan anggota") dan navigasi tab. Layout dan registri tab ada di `app/` (rule 12.4): T03 mendaftarkan tab **Members** dan **Settings**; T05/T06/T04 kelak menyisipkan **Knowledge**, **API Specs**, **API Keys** di depannya tanpa mengubah feature T03. Detail project dimuat sekali oleh view `getproject` (`ProjectContextView`) dan diteruskan ke tab sebagai konteks (`project`, `reload`, `replace`, `onprojectgone`).
   - Tab **Members**: tabel anggota (nama, email, peran, status nonaktif/tidak ditemukan, ditambahkan kapan/oleh siapa). Owner/Super Admin melihat tombol "Tambah anggota" (dialog dengan autocomplete developer aktif + pilihan peran), "Jadikan owner"/"Jadikan member", dan "Keluarkan". Member hanya melihat "Keluar" di barisnya sendiri.
   - Tab **Settings**: form ubah nama/deskripsi (semua level akses) dan "Zona bahaya" hapus project (owner/admin saja) dengan konfirmasi mengetik nama persis.

Pola lapisan mengikuti feature T02 yang sudah ada: `services/` memanggil `shared/services/http.ts` dengan schema Zod dan melempar `ApiError` apa adanya; `stores/` hanya state UI per instance (bukan singleton modul), tanpa fetch; `components/` presentasi murni; `views/` satu-satunya pemanggil service; `index.ts` satu-satunya pintu keluar feature. Feature tidak meng-import router (`domain-no-app`): navigasi dikirim balik lewat callback (`onopen`, `oncreated`, `ondeleted`, `onleft`, `onprojectgone`) ke `app/`.

Keamanan dan ketahanan:
- 404 diperlakukan seragam sebagai "Project tidak ditemukan" (tidak membedakan "tidak ada" dan "bukan milik Anda", rule 7.3).
- 403 pada mutasi (mis. peran baru saja diturunkan di tab lain) memunculkan toast `error.forbidden` lalu memuat ulang detail project agar tombol menyesuaikan.
- Pesan error bisnis dipetakan dari `details.reason` ke i18n `project.error.<REASON>`; error field dari `details.fields` ke input terkait; selain itu pesan server + trace id lewat `ErrorState`/toast.
- Semua nilai dari server dirender sebagai teks (tanpa `{@html}`).
- Respons yang tidak sesuai schema menjadi `ApiError` `INVALID_RESPONSE` dan ditampilkan sebagai error, bukan crash.

---

## Plan

Urutan wajib: FE-0 (prasyarat bersama) lebih dulu; lalu feature P1..P6 boleh paralel (tidak saling import internals); lalu komposisi `app/` (A1..A8); lalu e2e dan verifikasi (V). Semua langkah TDD: tulis test vitest feature lebih dulu.

### FE-0. Prasyarat bersama

1. **i18n** `frontend/src/shared/i18n/id.json` dan `en.json` (kunci flat, paritas id/en dijaga `shared/i18n/i18n-keys.test.ts`). Daftar lengkap di tabel "Kunci i18n". Bila backend F (overview F9) sudah menambah sebagian kunci, lengkapi saja; jangan duplikasi.
2. **`shared/validation/error.ts`** (ubah, generik, tanpa pengetahuan domain): tambah
   ```ts
   export const ReasonDetails = z.object({ reason: z.string().min(1) });
   export function reasonOf(details: unknown): string | null; // details.reason atau null
   ```
   Test di `shared/validation/error.test.ts`: `{reason:'LAST_OWNER'}` -> `'LAST_OWNER'`; `null`, `{fields:{}}`, `{reason:''}`, `{reason: 1}` -> `null`.
3. **`shared/components/Textarea.svelte`** (baru, generik): props sama dengan `Input` (`value` bindable, `label`, `name`, `hint`, `error`, `required`, `disabled`, `rows=4`, `maxlength?`), `aria-invalid`/`aria-describedby` untuk `error` dan `hint`. Ekspor di `shared/components/index.ts`. Test di `shared/components/components.test.ts`.
4. **`shared/utils/text.ts`** (baru, generik): `runeLength(s: string): number` (`[...s].length`, menghitung code point seperti `utf8.RuneCountInString` di backend) dan `hasControlChars(s: string, allow?: string[]): boolean` (`/\p{Cc}/u`, kecuali karakter di `allow`). Test di `shared/utils/text.test.ts` (emoji dihitung 1, `\n` terdeteksi, `allow=['\n','\r','\t']`).
5. Tidak ada dependensi npm baru.

### P1. Feature `listprojects` (`frontend/src/domain/project/management/listprojects/`)

**P1.1 `types.ts`**
```ts
export const PAGE_SIZE = 20;
export const MAX_QUERY = 100;
export const MY_ROLES = ['owner', 'member'] as const;
export const ACCESS_LEVELS = ['owner', 'member', 'admin'] as const;
export const ContentCountsSchema = z.object({
  files: z.number().int().nonnegative().nullable(),
  specs: z.number().int().nonnegative().nullable(),
  keys: z.number().int().nonnegative().nullable(),
});
export const ProjectSummarySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string(),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  updated_at: z.string().datetime({ offset: true }),
  my_role: z.enum(MY_ROLES).nullable(),
  access_level: z.enum(ACCESS_LEVELS),
  is_member: z.boolean(),
  member_count: z.number().int().nonnegative(),
  counts: ContentCountsSchema,
});
export const ProjectListSchema = z.object({
  items: z.array(ProjectSummarySchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
});
export type ProjectSummary = z.infer<typeof ProjectSummarySchema>;
export type ProjectList = z.infer<typeof ProjectListSchema>;
export interface ProjectListQuery { q: string; page: number } // page >= 1
```
Schema tidak `.strict()`: field tambahan dari server diabaikan (kompatibel maju).

**P1.2 `services/listprojects.service.ts`** (tanpa state)
```ts
export function parseListQuery(qs: URLSearchParams): ProjectListQuery;
//   q: trim, dipotong ke MAX_QUERY rune (mencegah 400 dari URL yang ditempel); page: integer >= 1, selain itu 1
export function toSearchParams(q: ProjectListQuery): URLSearchParams; // q kosong dan page 1 tidak ditulis
export function listProjects(q: ProjectListQuery, signal?: AbortSignal): Promise<ProjectList>;
//   get('/api/projects', { query: { q: q.q || undefined, limit: PAGE_SIZE, offset: (q.page-1)*PAGE_SIZE }, schema: ProjectListSchema, signal })
```

**P1.3 `stores/projectList.store.ts`** (per instance, tanpa fetch)
```ts
type ListState =
  | { status: 'idle' }
  | { status: 'loading'; key: string; previous: ProjectList | null }   // previous: data lama tetap tampil (redup) saat ganti halaman
  | { status: 'ready'; key: string; data: ProjectList }
  | { status: 'error'; key: string; error: ApiError };
export function createProjectListStore(): Readable<ListState> & {
  begin(key: string): void; resolve(key: string, d: ProjectList): void; fail(key: string, e: ApiError): void;
};
```
`key = toSearchParams(query).toString()`. `resolve`/`fail` dengan key basi diabaikan (proteksi race di samping abort).

**P1.4 `components/`** (presentasi; semua teks via `$t`)
- `format.ts`: `formatCount(n: number | null): string` (`null` -> `'-'`, angka -> `Intl.NumberFormat`), `formatRelative(iso, locale, now)` (`Intl.RelativeTimeFormat`, fallback `Intl.DateTimeFormat`; ISO invalid -> string asli), `excerpt(s, max = 140)` (potong di batas rune + "…").
- `ProjectSearchBar.svelte`: props `value: string`, `onsearch(q: string)`. `<form role="search">` + `Input` `type="search"` label `project.list.search`, `maxlength=100`. Debounce 300 ms di komponen (state lokal); Enter langsung memanggil `onsearch`; tombol hapus (`aria-label` `project.list.clearSearch`) memanggil `onsearch('')`.
- `RoleBadge.svelte`: props `myRole`, `isMember`, `accessLevel`. Teks `project.role.owner` / `project.role.member`; Super Admin bukan anggota -> `project.badge.notMember`. Teks + warna token (tidak mengandalkan warna saja).
- `ProjectTable.svelte`: props `items: ProjectSummary[]`, `onopen(id: string)`. Layar >= `md`: `Table` bersama dengan kolom `project.list.col.name` (nama sebagai `<a href="/projects/{id}">` agar ctrl/cmd-click membuka tab baru; klik biasa ditangani router global), `project.list.col.role`, `project.list.col.files`, `project.list.col.specs`, `project.list.col.updated` (`<time datetime>` + `title` ISO). Layar sempit: daftar kartu (`<ul>`) dengan isi yang sama. Deskripsi via `excerpt`. Baris punya `data-project-id`.
- Komponen tidak memanggil service dan tidak tahu router; tautan `href` adalah string biasa.

**P1.5 `views/ProjectListView.svelte`** (satu-satunya pemanggil service di feature ini)
Props: `query: URLSearchParams`, `onquerychange(next: URLSearchParams): void`, `canCreate: boolean`, `oncreate(): void`.
- `const parsed = $derived(parseListQuery(query))`; `$effect` pada `key`: abort controller lama, `store.begin(key)`, `listProjects(parsed, signal)` -> `resolve`; catch: abaikan abort; `ApiError` -> `fail`; error lain dibungkus `ApiError {status:0, code:'INTERNAL'}`.
- Setelah `ready`: bila `items.length === 0 && total > 0 && page > 1` -> `onquerychange` ke halaman terakhir (`Math.ceil(total/PAGE_SIZE)`) sekali (flag lokal agar tidak berulang).
- Header: `<h2>` `project.list.title`, tombol "Project baru" (`project.create.open`) bila `canCreate` -> `oncreate()`.
- State (lihat tabel "State UI"): loading pertama `Spinner` + `aria-busy`; loading ganti halaman: tabel lama `opacity-60` + `aria-busy`; kosong tanpa `q` -> `EmptyState` (`project.list.empty`, `project.list.emptyHint`, aksi "Project baru"); kosong dengan `q` -> `EmptyState` (`project.list.emptyFiltered` dengan `{q}`, aksi `project.list.reset`); error -> `ErrorState` + coba lagi (memuat ulang key sama); ready -> `ProjectTable` + `Pagination` (`page`, `PAGE_SIZE`, `total`; disembunyikan bila `total <= PAGE_SIZE`).
- Wilayah status `aria-live="polite"` mengumumkan `project.list.resultCount` (`{total}`) setelah pencarian.
- Ubah `q` -> `onquerychange` dengan `page` dihapus (kembali ke 1); ubah halaman -> `onquerychange` dengan `q` tetap.

**P1.6 `index.ts`**
```ts
export { default as ProjectListView } from './views/ProjectListView.svelte';
export type { ProjectSummary } from './types';
```

### P2. Feature `createproject` (`frontend/src/domain/project/management/createproject/`)

**P2.1 `types.ts`**: `NAME_MIN = 2`, `NAME_MAX = 100`, `DESCRIPTION_MAX = 1000`; `CreateProjectInput { name: string; description: string }`; `CreatedProjectSchema` (bentuk `ProjectSummary`, salinan lokal; feature tidak meng-import internals `listprojects`), `type CreatedProject`.

**P2.2 `services/createproject.service.ts`**
```ts
export function validateCreate(i: CreateProjectInput): Record<string, string>; // kunci i18n per field, mis. { name: 'project.validation.nameLength' }
//   name: trim, runeLength 2..100, hasControlChars -> 'project.validation.nameControl'
//   description: trim, runeLength <= 1000 -> 'project.validation.descriptionLength'; kontrol selain \n\r\t -> 'project.validation.descriptionControl'
export function createProject(i: CreateProjectInput): Promise<CreatedProject>;
//   post('/api/projects', { name: i.name.trim(), description: i.description.trim() }, { schema: CreatedProjectSchema })
```
Validasi klien hanya mencegah request yang pasti gagal; server tetap sumber kebenaran.

**P2.3 `components/CreateProjectForm.svelte`**: props `submitting`, `fieldErrors: Record<string,string>` (pesan siap tampil), `formError: string | null`, `showAdminHint: boolean`, `onsubmit(i)`, `oncancel()`. `Input` nama (`required`, `maxlength` tidak dipasang agar pesan validasi tampil), `Textarea` deskripsi dengan penghitung `{n}/1000` (`project.create.descriptionCounter`). `showAdminHint` -> paragraf `project.create.adminHint` (Super Admin tidak otomatis menjadi anggota, D3). Tombol Simpan `loading={submitting}` dan `disabled` saat submitting (mencegah submit ganda, usecase-createproject E2E negatif 7).

**P2.4 `views/CreateProjectDialog.svelte`**: props `open: boolean`, `isSuperAdmin: boolean`, `onclose()`, `oncreated(p: CreatedProject)`.
- `Dialog` `title=$t('project.create.title')`. State lokal: `submitting`, `fieldErrors`, `formError`; direset tiap kali dibuka.
- Submit: `validateCreate` -> bila ada error, tampilkan dan fokus input pertama yang salah, tanpa request. Selain itu `createProject`. Sukses: toast `project.create.success` (`{name}`), `oncreated(p)`.
- Error: 400 dengan `details.fields` -> peta ke field (pesan server ditampilkan apa adanya karena berbahasa server; bila kunci field dikenal `name`/`description` dan cocok dengan rule lokal, tampilkan kunci i18n lokal); 403 `CSRF_REQUIRED`/lainnya dan 5xx -> `formError` = `err.message` + trace id; dialog tetap terbuka dan input dipertahankan.

**P2.5 `index.ts`**: `export { default as CreateProjectDialog } from './views/CreateProjectDialog.svelte'; export type { CreatedProject } from './types';`

### P3. Feature `getproject` (`frontend/src/domain/project/management/getproject/`)

**P3.1 `types.ts`**: `ProjectDetailSchema` = bentuk `ProjectSummary` + `owner_count: z.number().int().nonnegative()` (salinan lokal); `type ProjectDetail`; `type AccessLevel = 'owner' | 'member' | 'admin'`;
```ts
export interface ProjectContext {
  project: ProjectDetail;
  reload(): Promise<void>;          // GET ulang; dipakai setelah 403 atau perubahan peran sendiri
  replace(p: ProjectDetail): void;  // pakai hasil PATCH tanpa GET ulang
}
export function canManageMembers(l: AccessLevel): boolean; // owner | admin
export function canDeleteProject(l: AccessLevel): boolean; // owner | admin
```
Fungsi izin ini hanya UX (rule 12.7) dan sengaja mencerminkan tabel overview 3.4.

**P3.2 `services/getproject.service.ts`**: `getProject(id: string, signal?)` -> `get('/api/projects/' + encodeURIComponent(id), { schema: ProjectDetailSchema, signal })`. Id tidak divalidasi di klien (backend menjawab 404 untuk bukan-uuid; tidak membocorkan format).

**P3.3 `stores/projectDetail.store.ts`**: `idle | loading(id, previous?) | ready(id, project) | notFound(id) | error(id, ApiError)`; `begin`, `resolve`, `fail` (404 -> `notFound`), `replace(project)` (hanya bila id sama). Per instance.

**P3.4 `components/ProjectHeader.svelte`**: props `project: ProjectDetail`. `<h1>` nama (teks; `break-words`), deskripsi (`whitespace-pre-line`, teks), badge peran (`project.role.*` atau `project.badge.notMember` bila `access_level === 'admin' && !is_member`), ringkasan `project.detail.memberCount` (`{n}`). Tanpa tombol aksi.

**P3.5 `views/ProjectContextView.svelte`**: props `projectId: string`, `children: Snippet<[ProjectContext]>`, `onnotfound?(): void` (opsional, untuk judul dokumen).
- Memuat detail saat `projectId` berubah (abort + key).
- `loading` pertama -> skeleton header + `Spinner` (`aria-busy`). Saat `reload()` dengan data lama, konten tetap tampil (tanpa kedip).
- `notFound` -> `EmptyState` judul `project.detail.notFound`, deskripsi `project.detail.notFoundHint`, aksi tautan `<a href="/">` `project.detail.backHome`. Teks sama untuk "tidak ada", "bukan anggota", dan id bukan uuid (rule 7.3).
- `error` -> `ErrorState` + coba lagi.
- `ready` -> `ProjectHeader` lalu `{@render children(ctx)}`.
- `reload` mengembalikan Promise yang resolve setelah state terbarui (dipakai test).

**P3.6 `index.ts`**:
```ts
export { default as ProjectContextView } from './views/ProjectContextView.svelte';
export { canManageMembers, canDeleteProject } from './types';
export type { ProjectDetail, ProjectContext, AccessLevel } from './types';
```

### P4. Feature `updateproject` (`frontend/src/domain/project/management/updateproject/`)

**P4.1 `types.ts`**: konstanta panjang (salinan lokal), `UpdatedProjectSchema` (bentuk `ProjectDetail`, salinan lokal), `ProjectPatch { name?: string; description?: string }`, `EditableProject { id: string; name: string; description: string }`.

**P4.2 `services/updateproject.service.ts`**
```ts
export function diffPatch(current: EditableProject, form: { name: string; description: string }): ProjectPatch;
//   bandingkan setelah trim; hanya field berubah; tidak berubah -> {}
export function validatePatch(p: ProjectPatch): Record<string, string>; // rule sama dengan createproject (P2.2)
export function updateProject(id: string, p: ProjectPatch): Promise<UpdatedProject>;
//   patch('/api/projects/' + encodeURIComponent(id), p, { schema: UpdatedProjectSchema })
```

**P4.3 `components/ProjectSettingsForm.svelte`**: props `initial: EditableProject`, `submitting`, `fieldErrors`, `formError`, `onsubmit(form)`, `onreset()`. `Input` nama, `Textarea` deskripsi + penghitung. Tombol Simpan `disabled` bila `diffPatch` kosong, ada error lokal, atau `submitting` (usecase-updateproject E2E 3: simpan tanpa perubahan tidak mengirim request). Tombol "Batalkan" mengembalikan nilai `initial`. Bila `initial` berubah dari luar (mis. `reload`) dan form belum diubah, form mengikuti nilai baru; bila sudah diubah, nilai user dipertahankan.

**P4.4 `views/ProjectSettingsView.svelte`**: props `project: EditableProject`, `onupdated(p: UpdatedProject)`, `onprojectgone()`, `onforbidden()`.
- Submit: `validatePatch(diffPatch(...))`; kosong -> tidak ada request. Sukses -> toast `project.settings.saved`, `onupdated(p)` (app memanggil `ctx.replace`).
- 400 `details.fields` -> per field; `fields.body` -> `formError` `project.settings.nothingToSave`.
- 404 -> toast `project.detail.notFound` + `onprojectgone()` (app navigasi ke `/`, usecase-updateproject E2E negatif 6-7).
- 403 -> toast `error.forbidden` + `onforbidden()` (app memanggil `ctx.reload()`).
- 5xx/network -> `formError` dengan pesan + trace id; input dipertahankan.

**P4.5 `index.ts`**: `export { default as ProjectSettingsView } from './views/ProjectSettingsView.svelte'; export type { UpdatedProject } from './types';`

### P5. Feature `deleteproject` (`frontend/src/domain/project/management/deleteproject/`)

**P5.1 `types.ts`**: `DeletableProject { id: string; name: string; member_count: number; counts: { files: number | null; specs: number | null; keys: number | null } }`.

**P5.2 `services/deleteproject.service.ts`**
```ts
export function confirmMatches(typed: string, name: string): boolean; // typed === name (tanpa trim, case-sensitive; sama dengan base.ConfirmDeletion)
export function deleteProject(id: string, confirmName: string): Promise<void>;
//   del('/api/projects/' + encodeURIComponent(id), { body: { confirm_name: confirmName } }); 204 -> undefined
```

**P5.3 `components/DangerZone.svelte`**: props `onrequest()`. Kotak bergaris `border-danger` dengan judul `project.delete.zoneTitle`, penjelasan `project.delete.zoneDescription` (permanen, menghapus knowledge, spec, key, anggota), tombol `variant="danger"` `project.delete.open`.

**P5.4 `components/DeleteProjectForm.svelte`**: props `project`, `typed` (bindable), `submitting`, `error: string | null`, `onsubmit()`, `oncancel()`.
- Ringkasan dampak: `project.delete.impact` dengan `{members}`, `{files}`, `{specs}`, `{keys}` (nilai `null` -> "-").
- Instruksi `project.delete.typeToConfirm` dengan nama project ditampilkan dalam `<code>` (teks, bukan HTML). `Input` `autocomplete="off"`, `spellcheck=false`, label `project.delete.confirmLabel`. Paste diizinkan.
- Tombol hapus `variant="danger"` `disabled={!confirmMatches(typed, project.name) || submitting}`, `loading={submitting}`.

**P5.5 `views/DeleteProjectView.svelte`**: props `project: DeletableProject`, `ondeleted(name: string)`, `onprojectgone()`, `onforbidden()`.
- Menampilkan `DangerZone`; klik membuka `Dialog` (`project.delete.title`) berisi `DeleteProjectForm`. Fokus awal di input konfirmasi; Escape/batal menutup dan mengosongkan input.
- Submit -> `deleteProject(project.id, typed)`. Sukses -> tutup dialog, toast `project.delete.success` (`{name}`), `ondeleted(name)` (app navigasi `/`).
- 400 `reason=CONFIRMATION_MISMATCH` -> `error` `project.error.CONFIRMATION_MISMATCH` (mis. nama diubah orang lain di tab lain).
- 404 -> tutup, toast `project.delete.alreadyGone`, `onprojectgone()` (usecase-deleteproject E2E negatif 4).
- 403 -> tutup, toast `error.forbidden`, `onforbidden()`.
- 5xx -> `error` pesan server + trace id; dialog tetap terbuka.

**P5.6 `index.ts`**: `export { default as DeleteProjectView } from './views/DeleteProjectView.svelte'; export type { DeletableProject } from './types';`

### P6. Feature `members` (`frontend/src/domain/project/management/members/`)

**P6.1 `types.ts`**
```ts
export const MEMBER_ROLES = ['owner', 'member'] as const;
export type MemberRole = typeof MEMBER_ROLES[number];
export const MemberSchema = z.object({
  user_id: z.string().uuid(),
  name: z.string().nullable(),
  email: z.string().nullable(),
  is_active: z.boolean(),
  found: z.boolean(),
  role: z.enum(MEMBER_ROLES),
  added_at: z.string().datetime({ offset: true }),
  added_by: z.string().nullable(),
  added_by_label: z.string().nullable(),
});
export const MemberListSchema = z.object({ items: z.array(MemberSchema) });
export const CandidateSchema = z.object({ id: z.string().uuid(), name: z.string(), email: z.string() });
export const CandidateListSchema = z.object({ items: z.array(CandidateSchema) });
export type Member = z.infer<typeof MemberSchema>;
export type Candidate = z.infer<typeof CandidateSchema>;
export type AccessLevel = 'owner' | 'member' | 'admin'; // salinan lokal
export const CANDIDATE_LIMIT = 10;
export const CANDIDATE_DEBOUNCE_MS = 250;
export const MAX_CANDIDATE_QUERY = 100;
export const KNOWN_REASONS = ['LAST_OWNER', 'ALREADY_MEMBER', 'USER_NOT_FOUND', 'USER_NOT_DEVELOPER', 'USER_INACTIVE'] as const;
```

**P6.2 `services/members.service.ts`** (tanpa state; semua path di-`encodeURIComponent`)
```ts
export function listMembers(projectId: string, signal?: AbortSignal): Promise<Member[]>;
//   get(`/api/projects/${pid}/members`, { schema: MemberListSchema, signal }).then(r => r.items)
export function searchCandidates(projectId: string, q: string, signal?: AbortSignal): Promise<Candidate[]>;
//   q di-trim; kosong -> Promise.resolve([]) TANPA request (backend juga []); > 100 rune -> dipotong
//   get(`/api/projects/${pid}/member-candidates`, { query: { q, limit: CANDIDATE_LIMIT }, schema: CandidateListSchema, signal })
export function addMember(projectId: string, userId: string, role: MemberRole): Promise<Member>;
//   post(`/api/projects/${pid}/members`, { user_id: userId, role }, { schema: MemberSchema })
export function removeMember(projectId: string, userId: string): Promise<void>;
//   del(`/api/projects/${pid}/members/${uid}`)  (tanpa body; 204)
export function changeMemberRole(projectId: string, userId: string, role: MemberRole): Promise<Member>;
//   patch(`/api/projects/${pid}/members/${uid}`, { role }, { schema: MemberSchema })
export function memberErrorKey(e: ApiError): string | null;
//   reasonOf(e.details) di KNOWN_REASONS -> `project.error.${reason}`; 403 -> 'error.forbidden'; 404 dengan pesan
//   "anggota tidak ditemukan" tidak dibedakan dari pesan: status 404 -> 'project.members.error.gone'; lainnya null
```
Catatan 404: pada route members, 404 bisa berarti project hilang/akses dicabut ATAU anggota target hilang. View membedakannya dengan memuat ulang daftar anggota: bila `listMembers` juga 404 -> project hilang (`onprojectgone`); bila 200 -> anggota sudah tidak ada, daftar diperbarui + toast `project.members.error.gone`.

**P6.3 `stores/`** (per instance, tanpa fetch)
- `membersList.store.ts`: `idle | loading | ready(items) | error(ApiError)`; `begin`, `resolve(items)` (diurutkan ulang dengan `sortMembers`), `fail`, `upsert(member)`, `remove(userId)`; derived `ownerCount`. `sortMembers` mengikuti backend (overview 3.8): owner dulu, `name` lowercase (`null` di akhir grup), lalu `user_id`.
- `candidateSearch.store.ts`: `{ q, status: 'idle' | 'loading' | 'ready' | 'error', items, error, activeIndex }`; `setQuery`, `begin(seq)`, `resolve(seq, items)`, `fail(seq, err)` (hasil dengan `seq` lama diabaikan), `move(delta)` (wrap), `clear()`.

**P6.4 `components/`**
- `MembersTable.svelte`: props `items`, `currentUserId`, `accessLevel`, `busyUserId: string | null`, `onrequestrole(m, role)`, `onrequestremove(m)`, `onrequestleave(m)`. Kolom: `project.members.col.name` (nama atau `project.members.unknownUser` bila `found=false`; akhiran `project.members.you` untuk baris sendiri), `col.email` (`-` bila null), `col.role` (badge teks), `col.status` (`project.members.status.inactive` bila `found && !is_active`; `project.members.status.notFound` bila `!found`; kosong bila aktif), `col.added` (`<time>` + `added_by_label`), `col.actions`.
  Aturan tombol (rule 12.7, hanya UX):
  | Baris | owner / admin (`canManage`) | member |
  |---|---|---|
  | diri sendiri | "Keluar" + "Jadikan member" bila role owner | "Keluar" |
  | anggota lain, role member | "Jadikan owner", "Keluarkan" | tidak ada |
  | anggota lain, role owner | "Jadikan member", "Keluarkan" | tidak ada |
  Super Admin tidak pernah punya baris sendiri (bukan anggota). Tombol baris yang sedang diproses `loading`; tombol lain di baris itu `disabled`. Label tombol menyertakan nama target untuk pembaca layar (`aria-label` `project.members.removeAria` dengan `{name}`).
- `CandidateCombobox.svelte`: pola ARIA combobox (WAI-ARIA 1.2): `<input role="combobox" aria-autocomplete="list" aria-expanded aria-controls aria-activedescendant>`, `<ul role="listbox">` dengan `<li role="option" aria-selected>` berisi nama + email (teks). Keyboard: ArrowDown/ArrowUp memindah `activeIndex`, Enter memilih, Escape menutup daftar (Escape kedua diteruskan ke Dialog). Props: `state`, `selected: Candidate | null`, `oninput(q)`, `onselect(c)`, `onclear()`. Status dalam listbox: `loading` -> `project.members.add.searching`; `ready` kosong dengan q -> `project.members.add.noCandidates`; `error` -> `project.members.add.searchError` (bukan "tidak ada hasil", usecase-searchmembercandidates E2E negatif 8). Region `aria-live="polite"` mengumumkan jumlah hasil.
- `AddMemberForm.svelte`: `CandidateCombobox` + pilihan peran (`<fieldset>` radio `member` default, `owner`) + petunjuk `project.members.add.ownerHint`; error inline (`role="alert"`); tombol Tambah `disabled` bila belum memilih kandidat atau `submitting` (usecase-addmember E2E negatif 8).
- `ConfirmActionDialog.svelte`: props `open`, `title`, `message`, `confirmLabel`, `variant`, `submitting`, `error`, `onconfirm`, `oncancel`. Dipakai untuk keluarkan, keluar, dan turunkan peran.

**P6.5 `views/MembersView.svelte`** (satu-satunya pemanggil service di feature)
Props: `projectId: string`, `accessLevel: AccessLevel`, `currentUserId: string`, `onleft(): void`, `onaccesschanged(): void`, `onprojectgone(): void`.
- `canManage = accessLevel === 'owner' || accessLevel === 'admin'`.
- Muat `listMembers` saat mount dan saat `projectId` berubah (abort + key). State: loading (`Table loading`), error (`ErrorState` + coba lagi; error `get-user` di backend = 500, tidak ada daftar parsial, usecase-listmembers E2E 4), 404 -> `onprojectgone()`, ready -> `MembersTable`. Daftar kosong secara teori mungkin (project tanpa owner yang dilihat admin) -> `EmptyState` `project.members.empty` + (bila `canManage`) tombol tambah.
- Header tab: judul `project.members.title` + jumlah; tombol "Tambah anggota" (`project.members.add.open`) hanya bila `canManage`.
- **Tambah**: `Dialog` `project.members.add.title` berisi `AddMemberForm`. Input -> debounce 250 ms -> `searchCandidates` (abort request sebelumnya, `seq` naik). Kandidat yang sudah ada di daftar lokal juga disaring di klien (pertahanan ganda). Submit -> `addMember`. Sukses: `store.upsert(m)`, tutup dialog, toast `project.members.add.success` (`{name}`), fokus ke tombol "Tambah anggota". Bila `role === 'owner'` dan peran saya tidak berubah, tidak perlu reload. Error: reason dikenal -> pesan inline i18n (`USER_INACTIVE`, `USER_NOT_DEVELOPER`, `USER_NOT_FOUND`, `ALREADY_MEMBER`); `ALREADY_MEMBER` juga memuat ulang daftar; `fields.user_id`/`fields.role` -> inline; 403 -> tutup, toast `error.forbidden`, `onaccesschanged()`; 404 -> cek ulang (P6.2) lalu `onprojectgone()` atau refresh.
- **Ubah peran**: promosi (`member -> owner`) langsung tanpa dialog; demosi (`owner -> member`) lewat `ConfirmActionDialog` (`project.members.demote.title`; pesan khusus bila target diri sendiri: `project.members.demote.selfWarning`). Kirim `changeMemberRole`. Sukses: `store.upsert(m)` (urutan ulang), toast `project.members.role.success`; bila target diri sendiri -> `onaccesschanged()` (app `ctx.reload()` sehingga tombol kelola hilang). 409 `LAST_OWNER` -> pesan `project.error.LAST_OWNER` di dialog / toast (promosi tidak pernah 409); 403 -> toast + `onaccesschanged()`; 404 -> cek ulang.
- **Keluarkan** (`target != saya`): `ConfirmActionDialog` (`project.members.remove.title`, pesan dengan `{name}`). `removeMember`. Sukses: `store.remove(id)`, toast `project.members.remove.success`. 409 `LAST_OWNER` (praktis tidak terjadi untuk owner lain) -> pesan; 403/404 seperti di atas.
- **Keluar** (`target == saya`): `ConfirmActionDialog` (`project.members.leave.title`, `project.members.leave.message`). Tombol tidak dinonaktifkan untuk owner terakhir; server yang memutuskan dan UI menampilkan `project.error.LAST_OWNER` di dialog (overview E2E negatif 3). Sukses: toast `project.members.leave.success`, `onleft()` (app navigasi ke `/`; project hilang dari daftar).
- Setelah dialog ditutup, fokus kembali ke tombol pemicu (Dialog bersama sudah menyimpan `trigger`).

**P6.6 `index.ts`**: `export { default as MembersView } from './views/MembersView.svelte'; export type { Member } from './types';`

### A. Komposisi `app/`

**A1. `app/workspace/tabs.ts`** (baru): registri tab workspace (rule 12.4: komposisi lintas module hanya di `app/`).
```ts
import type { Component } from 'svelte';
import type { ProjectContext } from '../../domain/project/management/getproject';
export interface WorkspaceTabProps { ctx: ProjectContext; currentUserId: string; onprojectgone(): void; onleft(): void }
export interface WorkspaceTab { key: string; labelKey: string; view: Component<WorkspaceTabProps> }
export const workspaceTabs: WorkspaceTab[] = [
  // T05: { key: 'knowledge', ... }, T06: { key: 'api-specs', ... }, T04: { key: 'api-keys', ... } disisipkan DI SINI
  { key: 'members', labelKey: 'project.tab.members', view: MembersTab },
  { key: 'settings', labelKey: 'project.tab.settings', view: SettingsTab },
];
export function defaultTabKey(tabs = workspaceTabs): string; // tab pertama
export function findTab(key: string, tabs = workspaceTabs): WorkspaceTab | null;
```
Komentar di file menjelaskan urutan akhir Knowledge, API Specs, API Keys, Members, Settings (tiket T03 bagian UI) dan bahwa T04-T06 cukup menambah satu entri.

**A2. `app/views/workspace/MembersTab.svelte`** (baru): `<MembersView projectId={ctx.project.id} accessLevel={ctx.project.access_level} {currentUserId} onleft={onleft} onaccesschanged={() => ctx.reload()} {onprojectgone} />`.

**A3. `app/views/workspace/SettingsTab.svelte`** (baru): `ProjectSettingsView` (`project={ctx.project}`, `onupdated={ctx.replace}`, `onforbidden={() => ctx.reload()}`, `onprojectgone`) lalu, hanya bila `canDeleteProject(ctx.project.access_level)`, `DeleteProjectView` (`project={ctx.project}`, `ondeleted` -> `navigate('/', { replace: true })`, `onforbidden`, `onprojectgone`). Member tidak melihat zona bahaya.

**A4. `app/views/ProjectWorkspaceView.svelte`** (baru): props `params: { projectID: string; tab?: string }`.
- `tab` kosong -> `navigate('/projects/' + encodeURIComponent(id) + '/' + defaultTabKey(), { replace: true })`.
- `tab` tidak dikenal -> di dalam konteks, tampilkan `EmptyState` `project.tab.notFound` + tautan ke tab default (header project tetap tampil).
- Render `<ProjectContextView projectId={params.projectID}>{#snippet children(ctx)} <WorkspaceTabsNav .../> <Tab ctx ... /> {/snippet}</ProjectContextView>`.
- `WorkspaceTabsNav` (komponen lokal `app/views/workspace/WorkspaceTabsNav.svelte`): `<nav aria-label={$t('project.tab.label')}>` berisi `<a href="/projects/{id}/{key}" aria-current={active ? 'page' : undefined}>`. Link (bukan `role="tablist"`) karena tiap tab adalah URL; scroll horizontal di layar sempit tanpa memecah halaman.
- `onprojectgone` -> `navigate('/', { replace: true })`. `onleft` -> `navigate('/', { replace: true })`.
- `$effect`: setelah `ctx.project` tersedia, `document.title = name + ' · ' + $t('app.name')`.
- `currentUserId` dari `$session.identity?.userId ?? ''` (route ber-`auth: true`, jadi selalu ada).

**A5. `app/views/HomeView.svelte`** (ubah; versi T01 menampilkan sapaan): pertahankan `<h1 data-testid="home-title">` (sapaan T01 tetap), lalu
- `<ProjectListView query={$current.location.query} onquerychange={(q) => navigate('/' + (q.size ? '?' + q : ''), { replace: true })} canCreate oncreate={() => (createOpen = true)} />`;
- `<CreateProjectDialog open={createOpen} isSuperAdmin={$session.identity?.role === 'super_admin'} onclose={() => (createOpen = false)} oncreated={(p) => { createOpen = false; navigate('/projects/' + encodeURIComponent(p.id) + '/' + defaultTabKey()); }} />`.
Pencarian memakai `replace` (tidak memenuhi riwayat); pindah halaman memakai `push` agar Back kembali ke halaman sebelumnya.

**A6. `app/routes/index.ts`** (ubah): tambah
```ts
{ path: '/projects/:projectID', view: ProjectWorkspaceView, meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' } },
{ path: '/projects/:projectID/:tab', view: ProjectWorkspaceView, meta: { auth: true, layout: 'app', titleKey: 'project.workspace.title' } },
```
Route `/` tetap `meta: { auth: true, layout: 'app', titleKey: 'home.title' }` (dari T01). Tidak ada `roles` (developer dan super_admin keduanya boleh).

**A7. `app/layouts/AppLayout.svelte`** (ubah kecil): tautan "Beranda" (`/`, dari T01) mendapat `aria-current="page"` juga untuk path `/projects/*`, dengan label `nav.projects` ("Project"). Tidak ada menu lain.

**A8. `e2e/smoke.spec.ts`** (ubah): test "deep link falls back to SPA" memakai path yang tidak cocok route mana pun (`/zzz/abc`), karena `/projects/abc` sekarang route ber-auth (redirect ke login). Selebihnya sesuai versi T01 (K9).

### V. Verifikasi

`npm run check`, `npm test`, `npm run depcruise`, `npm run test:arch`, `npm run build` (bundle di-embed backend), lalu `npm run test:e2e` terhadap binary (butuh backend T03 + T01 + T02).

---

## State UI (loading / empty / error per permukaan)

| Permukaan | Loading | Empty | Error | Khusus |
|---|---|---|---|---|
| Beranda daftar | Pertama: `Spinner` + `aria-busy`. Ganti halaman/pencarian: tabel lama redup + `aria-busy` | Tanpa `q`: `project.list.empty` + tombol "Project baru". Dengan `q`: `project.list.emptyFiltered` + reset | `ErrorState` (pesan + trace id) + coba lagi | Halaman di luar jangkauan -> lompat ke halaman terakhir sekali. Respons rusak -> `INVALID_RESPONSE` di `ErrorState` |
| Dialog buat project | Tombol Simpan `loading`, input `disabled` | - | Field: pesan per input. Lainnya: `formError` di atas tombol | Validasi klien sebelum request. Super Admin melihat `adminHint` |
| Header workspace | Skeleton + `Spinner` | - | `ErrorState` + coba lagi | 404 -> `project.detail.notFound` + kembali ke beranda (seragam untuk tidak ada / bukan milik / bukan uuid) |
| Tab tidak dikenal | - | `project.tab.notFound` + tautan tab default | - | Header tetap tampil |
| Tab Members | `Table loading` | `project.members.empty` (+ tambah bila boleh) | `ErrorState` + coba lagi (tanpa daftar parsial) | 404 -> kembali ke beranda. 403 mutasi -> toast + reload konteks |
| Dialog tambah anggota | Autocomplete: `searching`; submit: tombol `loading` | `noCandidates` (hanya bila q terisi) | Pencarian: `searchError` (bukan "tidak ada hasil"). Submit: pesan reason inline | q kosong -> daftar kosong tanpa request. Debounce 250 ms; hanya hasil request terakhir |
| Ubah peran / keluarkan / keluar | Tombol baris `loading`; dialog `submitting` | - | `LAST_OWNER` dan lainnya di dialog | 404 anggota -> refresh daftar + toast `gone` |
| Tab Settings | Tombol Simpan `loading` | - | Field + `formError` | Simpan nonaktif bila tidak ada perubahan. 404 -> beranda |
| Zona bahaya | Tombol hapus `loading` | - | `CONFIRMATION_MISMATCH` dan 5xx di dialog | Tombol aktif hanya bila nama persis sama. 404 -> toast `alreadyGone` + beranda |
| Global | `PendingView` saat sesi `unknown` (T01) | - | 401 -> `/login?next=...` (handler T01) | `unhandledrejection` ApiError -> toast (T00) |

---

## Kunci i18n

Ditambahkan ke `id.json` dan `en.json` (flat). Kunci yang sudah ada (`common.*`, `error.*`, `nav.*`) dipakai ulang.

| Kunci | id | en |
|---|---|---|
| `nav.projects` | Project | Projects |
| `project.list.title` | Project | Projects |
| `project.list.search` | Cari project | Search projects |
| `project.list.clearSearch` | Hapus pencarian | Clear search |
| `project.list.empty` | Belum ada project | No projects yet |
| `project.list.emptyHint` | Buat project pertama atau minta owner project menambahkan Anda. | Create your first project or ask a project owner to add you. |
| `project.list.emptyFiltered` | Tidak ada project yang cocok dengan "{q}" | No projects match "{q}" |
| `project.list.reset` | Reset pencarian | Reset search |
| `project.list.resultCount` | {total} project | {total} projects |
| `project.list.col.name` | Nama | Name |
| `project.list.col.role` | Peran saya | My role |
| `project.list.col.files` | File | Files |
| `project.list.col.specs` | Spec | Specs |
| `project.list.col.updated` | Diperbarui | Updated |
| `project.role.owner` | Owner | Owner |
| `project.role.member` | Member | Member |
| `project.badge.notMember` | Bukan anggota | Not a member |
| `project.create.open` | Project baru | New project |
| `project.create.title` | Buat project | Create project |
| `project.create.name` | Nama project | Project name |
| `project.create.description` | Deskripsi (opsional) | Description (optional) |
| `project.create.descriptionCounter` | {n}/1000 karakter | {n}/1000 characters |
| `project.create.submit` | Buat | Create |
| `project.create.success` | Project "{name}" dibuat | Project "{name}" created |
| `project.create.adminHint` | Sebagai Super Admin Anda tidak otomatis menjadi anggota. Tunjuk owner lewat tab Members. | As Super Admin you are not added as a member. Assign an owner from the Members tab. |
| `project.validation.nameLength` | Nama project wajib 2-100 karakter | Project name must be 2-100 characters |
| `project.validation.nameControl` | Nama project tidak boleh mengandung karakter kontrol | Project name must not contain control characters |
| `project.validation.descriptionLength` | Deskripsi maksimal 1000 karakter | Description must be at most 1000 characters |
| `project.validation.descriptionControl` | Deskripsi mengandung karakter tidak valid | Description contains invalid characters |
| `project.workspace.title` | Project | Project |
| `project.detail.memberCount` | {n} anggota | {n} members |
| `project.detail.notFound` | Project tidak ditemukan | Project not found |
| `project.detail.notFoundHint` | Project mungkin sudah dihapus atau Anda bukan anggotanya. | The project may have been deleted or you are not a member. |
| `project.detail.backHome` | Kembali ke daftar project | Back to projects |
| `project.tab.label` | Bagian project | Project sections |
| `project.tab.members` | Members | Members |
| `project.tab.settings` | Settings | Settings |
| `project.tab.notFound` | Bagian ini tidak tersedia | This section is not available |
| `project.settings.title` | Pengaturan project | Project settings |
| `project.settings.save` | Simpan | Save |
| `project.settings.reset` | Batalkan perubahan | Discard changes |
| `project.settings.saved` | Perubahan disimpan | Changes saved |
| `project.settings.nothingToSave` | Tidak ada perubahan untuk disimpan | Nothing to save |
| `project.delete.zoneTitle` | Zona bahaya | Danger zone |
| `project.delete.zoneDescription` | Menghapus project bersifat permanen: semua knowledge, API spec, API key, dan anggota ikut terhapus. | Deleting a project is permanent: all knowledge, API specs, API keys and members are removed. |
| `project.delete.open` | Hapus project | Delete project |
| `project.delete.title` | Hapus project ini? | Delete this project? |
| `project.delete.impact` | Akan dihapus: {members} anggota, {files} file, {specs} spec, {keys} key. | Will be removed: {members} members, {files} files, {specs} specs, {keys} keys. |
| `project.delete.typeToConfirm` | Ketik nama project untuk konfirmasi: | Type the project name to confirm: |
| `project.delete.confirmLabel` | Nama project | Project name |
| `project.delete.submit` | Hapus permanen | Delete permanently |
| `project.delete.success` | Project "{name}" dihapus | Project "{name}" deleted |
| `project.delete.alreadyGone` | Project sudah tidak ada | The project no longer exists |
| `project.members.title` | Anggota | Members |
| `project.members.empty` | Project ini belum punya anggota | This project has no members |
| `project.members.you` | (Anda) | (you) |
| `project.members.unknownUser` | User tidak dikenal | Unknown user |
| `project.members.col.name` | Nama | Name |
| `project.members.col.email` | Email | Email |
| `project.members.col.role` | Peran | Role |
| `project.members.col.status` | Status | Status |
| `project.members.col.added` | Ditambahkan | Added |
| `project.members.col.actions` | Aksi | Actions |
| `project.members.status.inactive` | Nonaktif | Inactive |
| `project.members.status.notFound` | Tidak ditemukan | Not found |
| `project.members.makeOwner` | Jadikan owner | Make owner |
| `project.members.makeMember` | Jadikan member | Make member |
| `project.members.remove` | Keluarkan | Remove |
| `project.members.removeAria` | Keluarkan {name} | Remove {name} |
| `project.members.leave` | Keluar | Leave |
| `project.members.add.open` | Tambah anggota | Add member |
| `project.members.add.title` | Tambah anggota | Add member |
| `project.members.add.search` | Cari developer (nama atau email) | Search developers (name or email) |
| `project.members.add.searching` | Mencari... | Searching... |
| `project.members.add.noCandidates` | Tidak ada developer aktif yang cocok | No matching active developers |
| `project.members.add.searchError` | Pencarian gagal. Coba lagi. | Search failed. Try again. |
| `project.members.add.resultCount` | {n} kandidat | {n} candidates |
| `project.members.add.role` | Peran | Role |
| `project.members.add.ownerHint` | Owner dapat mengelola anggota dan menghapus project. | Owners can manage members and delete the project. |
| `project.members.add.submit` | Tambah | Add |
| `project.members.add.success` | {name} ditambahkan | {name} added |
| `project.members.role.success` | Peran diperbarui | Role updated |
| `project.members.demote.title` | Turunkan menjadi member? | Change to member? |
| `project.members.demote.message` | {name} tidak lagi bisa mengelola anggota atau menghapus project. | {name} will no longer be able to manage members or delete the project. |
| `project.members.demote.selfWarning` | Anda akan kehilangan akses kelola anggota dan hapus project. | You will lose the ability to manage members and delete the project. |
| `project.members.remove.title` | Keluarkan anggota? | Remove member? |
| `project.members.remove.message` | {name} akan kehilangan akses ke project ini. | {name} will lose access to this project. |
| `project.members.remove.success` | Anggota dikeluarkan | Member removed |
| `project.members.leave.title` | Keluar dari project? | Leave project? |
| `project.members.leave.message` | Anda akan kehilangan akses ke project ini. | You will lose access to this project. |
| `project.members.leave.success` | Anda telah keluar dari project | You left the project |
| `project.members.error.gone` | Anggota sudah tidak ada; daftar diperbarui | The member no longer exists; list refreshed |
| `project.error.LAST_OWNER` | Owner terakhir tidak bisa keluar atau diturunkan. Angkat owner lain lebih dulu. | The last owner cannot leave or be demoted. Promote another owner first. |
| `project.error.ALREADY_MEMBER` | User sudah menjadi anggota | The user is already a member |
| `project.error.CONFIRMATION_MISMATCH` | Nama konfirmasi tidak cocok | The confirmation name does not match |
| `project.error.USER_NOT_FOUND` | User tidak ditemukan | User not found |
| `project.error.USER_NOT_DEVELOPER` | Hanya developer yang bisa ditambahkan. Super Admin tidak perlu menjadi anggota. | Only developers can be added. Super Admins do not need membership. |
| `project.error.USER_INACTIVE` | User nonaktif tidak bisa ditambahkan | Inactive users cannot be added |

---

## Keputusan dan selisih dengan plan usecase

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Satu folder FE `members/` untuk lima usecase anggota (bukan lima folder) | Overview bagian 8/15 dan plan usecase anggota merujuk `members/`. Kelimanya satu permukaan UI (tab Members) yang berbagi daftar lokal; memecah akan memaksa import silang |
| K2 | Daftar project: URL (`q`, `page`) sebagai sumber state, bukan store `q/page` seperti `usecase-listprojects.md` FE-2. `refreshProjects` tidak diekspor | Pola T02; reload/Back/deep link bekerja. Beranda selalu memuat ulang saat dikunjungi, sehingga refresh eksplisit setelah buat/hapus/keluar tidak diperlukan |
| K3 | Nama file service `listprojects.service.ts` (bukan `projects.service.ts`) dan store `projectList.store.ts` | Konsisten dengan feature T02 (`<feature>.service.ts`, `<nama>.store.ts`) |
| K4 | Schema Zod disalin lokal per feature (`ProjectSummary` di `listprojects` dan `createproject`, `ProjectDetail` di `getproject` dan `updateproject`) | Rule 12.3/12.4 dan keputusan serupa T02. Tipe kompatibel secara struktural; test tiap feature memakai fixture JSON yang sama bentuknya (overview bagian 12) |
| K5 | Workspace memakai registri tab di `app/workspace/tabs.ts`; tab default = entri pertama (sekarang `members`, setelah T05 `knowledge`) | Rule 12.4. T04-T06 cukup menambah entri tanpa menyentuh feature T03 |
| K6 | Owner terakhir tidak diblokir di klien (tombol "Keluar"/"Jadikan member" tetap aktif); pesan `LAST_OWNER` dari server | Satu sumber kebenaran (backend, termasuk race). E2E overview negatif 3 mengharapkan pesan muncul setelah klik |
| K7 | Tambah `reasonOf` (shared/validation), `Textarea` (shared/components), `runeLength`/`hasControlChars` (shared/utils) | Generik, tanpa pengetahuan domain (rule 12.5). Dipakai T04-T06 juga |
| K8 | Pembatalan request lewat `AbortController` + key/seq di store; hasil basi diabaikan | Pencarian beranda dan autocomplete (usecase-searchmembercandidates E2E positif 5) |
| K9 | Smoke e2e "deep link" pindah ke `/zzz/abc` | `/projects/:projectID` kini route ber-auth |

---

## Migration

Tidak ada. Frontend tidak punya migrasi. Migrasi backend T03 (`backend/migrations/project/20261022090000000_create_projects.sql`) dibuat di langkah F `overview.md`.

---

## Changes

`frontend/src/shared/` (generik):
- `i18n/id.json`, `i18n/en.json` (ubah): kunci tabel "Kunci i18n".
- `validation/error.ts` (ubah): `ReasonDetails`, `reasonOf`; `validation/error.test.ts` (ubah).
- `components/Textarea.svelte` (baru), `components/index.ts` (ubah: ekspor), `components/components.test.ts` (ubah).
- `utils/text.ts`, `utils/text.test.ts` (baru).

`frontend/src/domain/project/management/` (baru; hapus `frontend/src/domain/project/.gitkeep`):
- `listprojects/{types.ts, index.ts, services/listprojects.service.ts, services/listprojects.service.test.ts, stores/projectList.store.ts, stores/projectList.store.test.ts, components/{format.ts, format.test.ts, ProjectSearchBar.svelte, RoleBadge.svelte, ProjectTable.svelte, components.test.ts}, views/ProjectListView.svelte, views/ProjectListView.test.ts, mocks/projects.ts}`.
- `createproject/{types.ts, index.ts, services/createproject.service.ts, services/createproject.service.test.ts, components/CreateProjectForm.svelte, components/CreateProjectForm.test.ts, views/CreateProjectDialog.svelte, views/CreateProjectDialog.test.ts}`.
- `getproject/{types.ts, types.test.ts, index.ts, services/getproject.service.ts, services/getproject.service.test.ts, stores/projectDetail.store.ts, stores/projectDetail.store.test.ts, components/ProjectHeader.svelte, components/ProjectHeader.test.ts, views/ProjectContextView.svelte, views/ProjectContextView.test.ts, mocks/project.ts}`.
- `updateproject/{types.ts, index.ts, services/updateproject.service.ts, services/updateproject.service.test.ts, components/ProjectSettingsForm.svelte, components/ProjectSettingsForm.test.ts, views/ProjectSettingsView.svelte, views/ProjectSettingsView.test.ts}`.
- `deleteproject/{types.ts, index.ts, services/deleteproject.service.ts, services/deleteproject.service.test.ts, components/DangerZone.svelte, components/DeleteProjectForm.svelte, components/components.test.ts, views/DeleteProjectView.svelte, views/DeleteProjectView.test.ts}`.
- `members/{types.ts, index.ts, services/members.service.ts, services/members.service.test.ts, stores/membersList.store.ts, stores/membersList.store.test.ts, stores/candidateSearch.store.ts, stores/candidateSearch.store.test.ts, components/{MembersTable.svelte, CandidateCombobox.svelte, AddMemberForm.svelte, ConfirmActionDialog.svelte, components.test.ts, CandidateCombobox.test.ts}, views/MembersView.svelte, views/MembersView.test.ts, mocks/members.ts}`.

`frontend/src/app/`:
- `workspace/tabs.ts`, `workspace/tabs.test.ts` (baru).
- `views/ProjectWorkspaceView.svelte`, `views/ProjectWorkspaceView.test.ts` (baru).
- `views/workspace/{WorkspaceTabsNav.svelte, MembersTab.svelte, SettingsTab.svelte}` (baru).
- `views/HomeView.svelte` (ubah: daftar project + dialog buat), `views/app.test.ts` (ubah: beranda memuat daftar).
- `routes/index.ts` (ubah: dua route workspace), `routes/router.test.ts` (ubah: match `/projects/:projectID` dan `/projects/:projectID/:tab`).
- `layouts/AppLayout.svelte` (ubah: label/aktif `nav.projects`).

`frontend/e2e/`:
- `project-management.spec.ts` (baru).
- `helpers/projects.ts` (baru): `createProjectApi(request, name, description?)`, `addMemberApi(request, projectId, userId, role?)`, `deleteProjectApi(request, id, name)` (membawa `X-Requested-With: grimoire`), nama project unik per test (`P-${Date.now()}-${rand}`).
- `smoke.spec.ts` (ubah, K9).

Tidak diubah: `shared/services/http.ts` (`del` sudah menerima `body` lewat `RequestOptions`), `shared/stores/*`, `app/routes/{router,guard}.ts`, `app/App.svelte`, `.dependency-cruiser.cjs`, `scripts/test-arch.mjs`, `package.json`, feature `domain/core/**`.

---

## Test Scenario

Vitest + Testing Library (jsdom). `fetch` di-stub per test (`vi.stubGlobal`); fixture JSON di `mocks/` mengikuti overview bagian 12. Test penerimaan dinamai `T03-ACn` di judul `describe`/`it` (rule 13.4).

### shared
- `reasonOf`: kasus FE-0.2.
- `runeLength('👍a') === 2`; `hasControlChars('a\nb') === true`; `hasControlChars('a\nb', ['\n']) === false`; `hasControlChars('a\u0000') === true`.
- `Textarea`: label terhubung (`getByLabelText`), `error` memberi `aria-invalid="true"` dan `aria-describedby` menunjuk pesan.
- Paritas kunci i18n id/en (test yang ada) hijau dengan kunci baru.

### listprojects
- `listprojects.service.test.ts`:
  - `listProjects({q:'pay', page:2})` memanggil `/api/projects?q=pay&limit=20&offset=20`; `q` kosong tidak dikirim.
  - Respons fixture valid lolos; `counts.files: null` valid; `my_role: 'admin'` ditolak -> `ApiError` `INVALID_RESPONSE`; tanpa `total` ditolak.
  - 400 `details.fields.limit` -> `ApiError` dengan `details` utuh.
  - `parseListQuery`: `page=abc|0|-1` -> 1; `q` 150 rune dipotong 100; `toSearchParams` tidak menulis `page=1` dan `q` kosong.
- `projectList.store.test.ts`: `resolve` dengan key basi diabaikan; `begin` menyimpan `previous`; `fail` -> `error`.
- `components.test.ts`:
  - `formatCount(null) === '-'`, `formatCount(0) === '0'`.
  - `ProjectTable`: peran tampil sebagai teks `Owner`/`Member`; `access_level=admin, is_member=false` menampilkan "Bukan anggota" (T03-AC7); nama adalah link `/projects/<id>`; nama berisi `<img src=x onerror=alert(1)>` tampil literal (tidak ada elemen `img`).
  - `ProjectSearchBar`: mengetik memanggil `onsearch` sekali setelah 300 ms (fake timer); Enter langsung.
- `ProjectListView.test.ts`:
  - Loading pertama menampilkan spinner dan `aria-busy`.
  - Kosong tanpa `q` vs dengan `q` menampilkan pesan berbeda; reset memanggil `onquerychange` tanpa `q`.
  - Error 500 menampilkan `ErrorState` dengan trace id; coba lagi memanggil fetch lagi.
  - `items: [], total: 45, page=5` memanggil `onquerychange` ke `page=3` tepat sekali.
  - Ganti `query` cepat dua kali: request pertama di-abort, hanya hasil kedua yang tampil.
  - `canCreate` menampilkan tombol "Project baru".

### createproject
- `validateCreate`: `'A'` -> nameLength; `'AB'` lolos; 100 emoji lolos, 101 gagal; `'a\nb'` -> nameControl; deskripsi 1001 rune gagal; `\n` di deskripsi lolos.
- `createProject` mengirim body ter-trim, method POST, header `X-Requested-With: grimoire`.
- `CreateProjectDialog`:
  - Nama 1 karakter: pesan field tampil, fetch tidak dipanggil (usecase-createproject E2E negatif 1).
  - Sukses: `oncreated` menerima project dan toast sukses muncul.
  - 400 `fields.name` dari server tampil di input nama; input tidak dikosongkan.
  - Klik Simpan dua kali cepat hanya menghasilkan satu request.
  - `isSuperAdmin` menampilkan `adminHint`.

### getproject
- `canManageMembers`/`canDeleteProject`: tabel owner/member/admin.
- `getproject.service`: path di-encode (`'a/b'` -> `/api/projects/a%2Fb`); `owner_count` wajib.
- `projectDetail.store`: 404 -> `notFound`; `replace` dengan id berbeda diabaikan.
- `ProjectHeader`: badge "Bukan anggota" untuk admin non-anggota; deskripsi multi-baris sebagai teks.
- `ProjectContextView`:
  - T03-AC1: 404 menampilkan "Project tidak ditemukan" + tautan ke `/` (tanpa membedakan alasan).
  - Ready merender snippet `children` dengan `ctx.project`.
  - `ctx.reload()` memanggil GET lagi tanpa menghilangkan konten lama; `ctx.replace(p)` memperbarui header tanpa fetch.
  - Error 500 -> `ErrorState` + coba lagi.

### updateproject
- `diffPatch`: tidak berubah -> `{}`; nama berubah setelah trim -> `{name}`; `' Payments '` dengan nama `Payments` -> `{}`.
- `ProjectSettingsView`:
  - T03-AC2: member (form tampil untuk semua level) mengubah nama -> PATCH body `{name}` saja; sukses memanggil `onupdated` dan toast.
  - Tombol Simpan nonaktif tanpa perubahan (tidak ada fetch).
  - 400 `fields.name` tampil di input; `fields.body` -> `nothingToSave`.
  - 404 -> `onprojectgone`; 403 -> toast `error.forbidden` + `onforbidden`.
  - `initial` berubah dari luar saat form bersih -> form ikut; saat form kotor -> nilai user dipertahankan.

### deleteproject
- `confirmMatches('Payments', 'Payments') === true`; `'payments'`, `'Payments '`, `''` -> false.
- `deleteProject` mengirim `DELETE` dengan body `{"confirm_name":"Payments"}` dan header CSRF; 204 -> resolve `undefined`.
- `DeleteProjectView`:
  - T03-AC6: tombol hapus nonaktif sampai nama persis; huruf besar/kecil berbeda tetap nonaktif.
  - Ringkasan dampak menampilkan "-" untuk counts `null`.
  - Sukses -> `ondeleted('Payments')` + toast.
  - 400 `CONFIRMATION_MISMATCH` -> pesan i18n di dialog.
  - 404 -> toast `alreadyGone` + `onprojectgone`; 403 -> toast + `onforbidden`.
  - Escape menutup dialog dan mengosongkan input.

### members
- `members.service.test.ts`:
  - `searchCandidates(pid, '  ')` tidak memanggil fetch dan resolve `[]`.
  - `searchCandidates(pid, 'bud')` -> `/api/projects/<pid>/member-candidates?q=bud&limit=10`.
  - `addMember` body `{user_id, role}`; `removeMember` DELETE tanpa body; `changeMemberRole` PATCH `{role}`; semua membawa header CSRF.
  - `memberErrorKey`: reason `LAST_OWNER` -> `project.error.LAST_OWNER`; reason tak dikenal -> null; 403 -> `error.forbidden`.
  - Respons member dengan `role: 'admin'` ditolak Zod.
- `membersList.store.test.ts`: `sortMembers` owner dulu, nama case-insensitive, `name=null` di akhir grup, tie-break `user_id`; `upsert` mengurutkan ulang; `ownerCount` benar.
- `candidateSearch.store.test.ts`: `resolve` dengan seq lama diabaikan; `move` wrap di ujung.
- `components.test.ts` (`MembersTable`):
  - T03-AC2: `accessLevel=member` -> tidak ada tombol "Keluarkan"/"Jadikan owner"; hanya "Keluar" di baris sendiri.
  - `accessLevel=owner` -> baris lain punya "Keluarkan" dan tombol peran; baris sendiri punya "Keluar" + "Jadikan member".
  - T03-AC7: `accessLevel=admin` tanpa baris sendiri -> semua baris punya tombol kelola.
  - Status `Nonaktif` untuk `is_active=false`; `User tidak dikenal` + `Tidak ditemukan` untuk `found=false`.
- `CandidateCombobox.test.ts`: atribut ARIA (`role=combobox`, `aria-expanded`, `aria-activedescendant`); ArrowDown/Enter memilih; Escape menutup; state error menampilkan `searchError`, bukan `noCandidates`.
- `MembersView.test.ts`:
  - Memuat daftar; error 500 -> `ErrorState` tanpa baris; 404 -> `onprojectgone`.
  - Tambah: debounce 250 ms satu request; mengetik lagi meng-abort request lama; kandidat yang sudah anggota tidak tampil; tombol Tambah nonaktif sampai kandidat dipilih; sukses menambah baris dan toast.
  - T03-AC4: 400 `USER_INACTIVE` / `USER_NOT_DEVELOPER` / `USER_NOT_FOUND` menampilkan pesan i18n masing-masing di dialog; 409 `ALREADY_MEMBER` menampilkan pesan dan memuat ulang daftar.
  - T03-AC3: "Keluar" sebagai owner terakhir -> 409 `LAST_OWNER` -> pesan i18n di dialog, `onleft` tidak dipanggil, baris tetap.
  - Demosi diri sendiri sukses -> `onaccesschanged` dipanggil.
  - Keluar sukses -> `onleft` dipanggil.
  - 404 saat keluarkan (anggota sudah hilang): `listMembers` dipanggil ulang; bila 200 -> toast `gone`; bila 404 -> `onprojectgone`.
  - 403 saat aksi -> toast `error.forbidden` + `onaccesschanged`.

### app
- `tabs.test.ts`: `defaultTabKey()` = `members` (sebelum T05); `findTab('xyz') === null`; key unik.
- `router.test.ts`: `/projects/abc` cocok route workspace dengan `params.projectID='abc'`; `/projects/abc/members` dengan `tab='members'`; `/projects/a%2Fb/settings` -> `projectID='a/b'`.
- `ProjectWorkspaceView.test.ts`:
  - Tanpa `tab` -> `navigate` replace ke `/projects/<id>/members`.
  - Tab tidak dikenal -> `project.tab.notFound` dengan header tetap tampil.
  - Member tidak melihat zona bahaya di Settings; owner dan admin melihatnya (rule 12.7, UX).
  - Nav tab memberi `aria-current="page"` pada tab aktif.
- `app.test.ts`: beranda (`/`) untuk sesi developer memuat `GET /api/projects` dan menampilkan judul; tombol "Project baru" membuka dialog; `oncreated` menavigasi ke `/projects/<id>/members`.

### Arsitektur
- `npm run depcruise` lulus: feature `project/management/*` hanya meng-import `shared/` dan file internalnya; `app/` hanya meng-import `index.ts` feature; tidak ada `domain -> app`.
- `npm run test:arch` (`scripts/test-arch.mjs`) lulus tanpa perubahan fixture.
- `npm run check` (svelte-check + tsc) tanpa error; `npm run build` sukses.

---

## E2E Scenarios

File `frontend/e2e/project-management.spec.ts` (Playwright terhadap binary). Prasyarat: T01 (`create-admin`, login, `createUserApi`, `setuserstatus`) dan T02 merged; backend T03 lengkap; DB bersih per run (`global-setup.ts` T01). Setiap test membuat user dan project sendiri dengan nama unik (paralel aman). Hitungan file/spec/key `null` sampai T04-T06 merge, sehingga UI menampilkan "-" (D1).

Positif:
1. **T03-AC1 buat project**: developer A login -> beranda kosong (`project.list.empty`) -> "Project baru" -> nama "Payments", deskripsi -> diarahkan ke `/projects/<id>/members`; header menampilkan "Payments" dan badge Owner; tabel anggota memuat A sebagai owner dengan "(Anda)". Beranda menampilkan Payments dengan peran Owner dan kolom File/Spec "-".
2. **Tambah anggota lewat autocomplete**: A membuka "Tambah anggota", mengetik "bud" -> daftar menampilkan developer aktif B (nama + email), tidak menampilkan A (sudah anggota) maupun developer nonaktif D -> pilih B (keyboard ArrowDown + Enter) -> Tambah -> B muncul sebagai Member; toast sukses.
3. **T03-AC2 member**: B login -> beranda menampilkan Payments dengan peran Member -> tab Settings: ubah nama menjadi "Payments v2" -> toast tersimpan, header berubah; zona bahaya tidak tampil; tab Members hanya menampilkan tombol "Keluar" di baris B.
4. **Serah terima owner**: A menekan "Jadikan owner" pada B -> B menjadi Owner; A menekan "Keluar" -> konfirmasi -> diarahkan ke beranda, Payments hilang dari daftar A; B (reload) tetap Owner.
5. **T03-AC7 Super Admin**: admin login -> beranda menampilkan project A dan project developer C, masing-masing dengan badge "Bukan anggota" -> membuka project C -> tab Members: mengeluarkan anggota, lalu menurunkan owner terakhir C menjadi member (berhasil, project tanpa owner) -> menambah A dengan peran Owner.
6. **Super Admin membuat project**: dialog menampilkan `adminHint`; setelah dibuat, tab Members kosong (`project.members.empty`) dan admin bisa menambahkan owner.
7. **T03-AC6 hapus**: B (owner) membuka Settings -> zona bahaya -> dialog menampilkan dampak -> tombol nonaktif saat mengetik "payments v2" -> mengetik "Payments v2" persis -> Hapus permanen -> kembali ke beranda tanpa project itu, toast sukses. Admin membuka `/audit-log?action=project.deleted` dan melihat entri dengan label project.
8. **Pencarian dan paginasi**: developer dengan 25 project -> halaman 1 berisi 20, Pagination ke halaman 2 berisi 5, URL `?page=2`; ketik "alpha" -> URL `?q=alpha`, hasil tersaring; reload mempertahankan pencarian; Back kembali ke halaman sebelumnya.

Negatif:
1. **T03-AC1 isolasi**: developer C membuka `/projects/<id Payments>/members` langsung -> "Project tidak ditemukan" + tautan ke beranda; `/projects/bukan-uuid` -> pesan sama; tidak ada data Payments di DOM.
2. **T03-AC2 member via API**: sebagai B (member), `page.request` `POST /api/projects/<id>/members` -> 403; `DELETE /api/projects/<id>` -> 403. Di UI, B tidak melihat tombol tambah/keluarkan/hapus.
3. **T03-AC3 owner terakhir**: A (satu-satunya owner) menekan "Keluar" -> dialog menampilkan `project.error.LAST_OWNER`; A tetap owner setelah reload. A menekan "Jadikan member" pada dirinya -> pesan yang sama.
4. **T03-AC4 kandidat tidak layak via API**: `POST members` dengan user nonaktif D -> 400 `USER_INACTIVE`; dengan admin -> 400 `USER_NOT_DEVELOPER`. Dengan `page.route` yang membalas 400 `USER_INACTIVE` untuk submit UI -> dialog menampilkan pesan i18n `project.error.USER_INACTIVE`.
5. **Duplikat**: dua tab menambahkan B bersamaan; tab kedua mendapat 409 dan menampilkan `project.error.ALREADY_MEMBER`, daftar dimuat ulang dengan satu baris B.
6. **Nama tidak valid**: buat project dengan nama "A" -> pesan field nama, tidak ada request `POST /api/projects` (dipantau `page.on('request')`).
7. **Hapus dengan nama salah via API** -> 400 `CONFIRMATION_MISMATCH`; project masih ada.
8. **Tab lama**: tab 1 menghapus project, tab 2 menyimpan Settings -> toast "Project tidak ditemukan" dan kembali ke beranda.
9. **Akses dicabut saat terbuka**: A mengeluarkan B; B (tab masih terbuka di Members) menekan "Keluar" -> 404 -> diarahkan ke beranda tanpa error mentah.
10. **Pencarian kandidat gagal**: `page.route('**/member-candidates*')` membalas 500 -> listbox menampilkan `project.members.add.searchError`, bukan "tidak ada developer".
11. **Respons rusak**: `page.route('/api/projects?*')` membalas 200 `{"items":"x"}` -> beranda menampilkan `ErrorState` "Respons server tidak sesuai format", tanpa crash dan tanpa console error yang tidak tertangani.
12. **Tanpa login**: membuka `/projects/<id>` tanpa sesi -> `/login?next=%2Fprojects%2F<id>`; setelah login kembali ke workspace.
13. **CSRF**: `page.request.post('/api/projects', { data: {name:'X Y'} })` tanpa header -> 403 `CSRF_REQUIRED` (UI selalu mengirim header; diverifikasi tidak ada 403 pada alur positif).
14. **XSS**: project bernama `<img src=x onerror=window.__xss=1>` -> tampil literal di beranda dan header; `window.__xss` tetap undefined.

---

## Request/Response body

Bentuk persis dari `overview.md` bagian 12 (sumber kebenaran). Semua error memakai envelope T00 yang divalidasi `ErrorEnvelope`:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Header yang dikirim FE pada mutasi: `X-Requested-With: grimoire`, `Content-Type: application/json`, `Accept: application/json`. Cookie sesi `same-origin`.

### GET /api/projects?q=pay&limit=20&offset=0 (`listProjects`)

200:
```json
{
  "items": [
    {
      "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
      "name": "Payments",
      "description": "Layanan pembayaran internal",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-22T09:00:00Z",
      "updated_at": "2026-10-22T10:15:00Z",
      "my_role": "member",
      "access_level": "member",
      "is_member": true,
      "member_count": 2,
      "counts": { "files": null, "specs": null, "keys": null }
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

Super Admin bukan anggota: `"my_role": null`, `"access_level": "admin"`, `"is_member": false`.

400 (`limit=500`, tidak terjadi dari UI karena `limit` tetap 20):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "limit": "limit harus 1-100" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

401 (sesi habis; FE mengarahkan ke `/login?next=`):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects (`createProject`)

Request:
```json
{ "name": "Payments", "description": "Layanan pembayaran internal" }
```

201 (developer):
```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "name": "Payments",
  "description": "Layanan pembayaran internal",
  "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "created_by_label": "a@tim.id",
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T09:00:00Z",
  "my_role": "owner",
  "access_level": "owner",
  "is_member": true,
  "member_count": 1,
  "counts": { "files": 0, "specs": 0, "keys": 0 }
}
```

201 (Super Admin): sama, tetapi `"my_role": null`, `"access_level": "admin"`, `"is_member": false`, `"member_count": 0`.

400:
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "name": "nama project wajib 2-100 karakter", "description": "deskripsi maksimal 1000 karakter" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

403 (tanpa header CSRF; tidak terjadi dari UI):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### GET /api/projects/{projectID} (`getProject`)

200 (`ProjectDetail`):
```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "name": "Payments",
  "description": "Layanan pembayaran internal",
  "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "created_by_label": "a@tim.id",
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T10:15:00Z",
  "my_role": "owner",
  "access_level": "owner",
  "is_member": true,
  "member_count": 2,
  "owner_count": 1,
  "counts": { "files": null, "specs": null, "keys": null }
}
```

404 (tidak ada, bukan anggota, atau bukan uuid; FE menampilkan pesan yang sama):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### PATCH /api/projects/{projectID} (`updateProject`)

Request (FE hanya mengirim field yang berubah):
```json
{ "name": "Payments v2" }
```

200: `ProjectDetail` seperti di atas dengan `"name": "Payments v2"` dan `updated_at` baru.

400 (body kosong; FE tidak mengirimnya karena tombol nonaktif):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "body": "minimal satu field (name, description) wajib diisi" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### DELETE /api/projects/{projectID} (`deleteProject`)

Request:
```json
{ "confirm_name": "Payments v2" }
```

204 tanpa body.

400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama konfirmasi tidak cocok", "details": { "reason": "CONFIRMATION_MISMATCH" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (member):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### GET /api/projects/{projectID}/members (`listMembers`)

200:
```json
{
  "items": [
    {
      "user_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "name": "Andi",
      "email": "a@tim.id",
      "is_active": true,
      "found": true,
      "role": "owner",
      "added_at": "2026-10-22T09:00:00Z",
      "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "added_by_label": "a@tim.id"
    },
    {
      "user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "name": "Budi",
      "email": "b@tim.id",
      "is_active": false,
      "found": true,
      "role": "member",
      "added_at": "2026-10-22T09:05:00Z",
      "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "added_by_label": "a@tim.id"
    }
  ]
}
```

User yang tidak ditemukan `get-user`: `"name": null, "email": null, "found": false, "is_active": false` (FE: "User tidak dikenal").

500 (get-user gagal; FE: `ErrorState` + coba lagi):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### GET /api/projects/{projectID}/member-candidates?q=bud&limit=10 (`searchCandidates`)

200:
```json
{ "items": [ { "id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "name": "Budiman", "email": "budiman@tim.id" } ] }
```

`q` kosong -> `{ "items": [] }` (FE tidak mengirim request). Member -> 403 `FORBIDDEN`. Bukan anggota -> 404 `NOT_FOUND`.

### POST /api/projects/{projectID}/members (`addMember`)

Request:
```json
{ "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "role": "member" }
```

201:
```json
{
  "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c",
  "name": "Budiman",
  "email": "budiman@tim.id",
  "is_active": true,
  "found": true,
  "role": "member",
  "added_at": "2026-10-22T11:00:00Z",
  "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "added_by_label": "a@tim.id"
}
```

400 (AC4; FE memetakan `details.reason` ke `project.error.<REASON>`):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user nonaktif tidak bisa ditambahkan sebagai anggota", "details": { "reason": "USER_INACTIVE" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "hanya developer yang bisa ditambahkan sebagai anggota; Super Admin tidak perlu menjadi anggota", "details": { "reason": "USER_NOT_DEVELOPER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user tidak ditemukan", "details": { "reason": "USER_NOT_FOUND" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400 (`user_id` kosong):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "user_id": "wajib diisi" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota", "details": { "reason": "ALREADY_MEMBER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### DELETE /api/projects/{projectID}/members/{userID} (`removeMember`)

Tanpa request body. 204 tanpa body.

409:
```json
{ "error": { "code": "CONFLICT", "message": "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu", "details": { "reason": "LAST_OWNER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (target bukan anggota; FE memuat ulang daftar):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (member mengeluarkan orang lain):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### PATCH /api/projects/{projectID}/members/{userID} (`changeMemberRole`)

Request:
```json
{ "role": "owner" }
```

200: `Member` (bentuk sama dengan 201 `addMember`) dengan `"role": "owner"`. No-op juga 200.

409 `LAST_OWNER` dan 403 `FORBIDDEN` seperti di atas. Role invalid (tidak terjadi dari UI):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus owner atau member" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Respons tidak sesuai schema (dibuat FE, bukan server)

`ApiError` yang dilempar `shared/services/http.ts` saat Zod gagal, ditampilkan lewat `ErrorState`:
```json
{ "status": 200, "code": "INVALID_RESPONSE", "message": "Respons server tidak sesuai format", "details": [ { "path": ["items"], "message": "Expected array, received string" } ] }
```
