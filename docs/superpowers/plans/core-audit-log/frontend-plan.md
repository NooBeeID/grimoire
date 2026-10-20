# T02 core/audit-log: Frontend Plan (Svelte)

Tiket: `docs/tickets/T02-core-audit-log.md`. Bagian bersama: `overview.md` (F5 OpenAPI, F6 i18n, F7). Kontrak per endpoint: `usecase-listentries.md`, `usecase-getentry.md`. Konvensi: `docs/tickets/README.md` bagian "Frontend (Svelte)" dan `rules.txt` bagian 12 (rules menang soal arsitektur).

Dokumen ini adalah **satu-satunya plan frontend** untuk T02. Bila bagian "Frontend" di `usecase-listentries.md` / `usecase-getentry.md` berbeda dengan dokumen ini, **ikuti dokumen ini** (selisihnya dicatat di bagian "Keputusan dan selisih dengan plan usecase").

## Metadata

| Field | Nilai |
|---|---|
| Scope | fullstack (dokumen ini: bagian frontend saja) |
| Domain | core |
| Module | audit-log (folder FE `domain/core/audit-log/`, URL `/audit-log`) |
| Feature FE | `listentries` (tabel + filter + cursor), `getentry` (panel detail). `record` dan `purgeexpired` tidak punya UI. |
| Route UI | `/audit-log` (query: filter + `entry=<id>` untuk panel detail) |
| Akses UI | `meta: { auth: true, roles: ['super_admin'] }` (UX saja; otorisasi di backend, rule 12.7) |
| Kontrak API | `api/modules/core/audit-log.yml` (schemas `AuditEntrySummary`, `AuditEntryList`, `AuditEntryDetail`) |

| Endpoint | Method | Version | Dipakai oleh | Akses |
|---|---|---|---|---|
| `/api/audit-log` | GET | v1 (tanpa versi di path, prefix `/api`) | `listentries/services/listentries.service.ts` | super_admin |
| `/api/audit-log/{id}` | GET | v1 | `getentry/services/getentry.service.ts` | super_admin |

Frontend tidak memanggil endpoint lain, tidak menambah endpoint, dan tidak pernah mengirim POST/PUT/PATCH/DELETE ke `/api/audit-log*` (AC7).

Konteks yang dimuat: tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: `rules.txt` (bagian 12, 13), `docs/tickets/README.md`, tiket T02, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (bagian "Audit log"), `overview.md`, `usecase-*.md`, dan kode foundation FE yang sudah ada di `main`: `shared/services/http.ts` (`get`, `ApiError`, `isApiError`, handler 401 global), `shared/components/{Table,Button,Input,Spinner,EmptyState,ErrorState,Dialog}`, `shared/stores/{i18n,session}`, `app/routes/{router,guard,index}.ts`, `app/layouts/AppLayout.svelte`, `app/App.svelte` (view menerima prop `params`), `.dependency-cruiser.cjs` (aturan `no-cross-feature-internals`, `outside-domain-only-via-index`, `domain-no-app`).

## Summary Plan

Satu halaman `/audit-log` khusus Super Admin. Halaman dikomposisikan di `app/views/AuditLogView.svelte` dari dua feature yang tidak saling import: `listentries` (FilterBar + AuditTable + "Muat berikutnya" dengan cursor stack) dan `getentry` (panel detail di samping kanan, overlay penuh di layar sempit). **URL adalah sumber kebenaran state**: filter disimpan di query string (`actor_type`, `actor_id`, `actor_label`, `project_id`, `action` berulang, `status` berulang, `range` atau `from`/`to`, `target`) dan entri terpilih di `entry=<id>`. Reload, tombol Back, dan deep link (mis. dari halaman API key nanti: `/audit-log?actor_type=api_key&actor_id=<id>`) bekerja tanpa state tambahan. Service memvalidasi respons dengan Zod dan melempar `ApiError` apa adanya; store hanya state UI (cursor stack, status muat); komponen murni presentasi; view feature satu-satunya yang memanggil service. Semua teks lewat i18n (`auditLog.*`), semua nilai dari server dirender sebagai teks (tanpa `{@html}`), dan setiap state loading/empty/error/forbidden/not-found punya tampilan yang jelas.

## Plan

Urutan wajib. FE-0 lebih dulu (prasyarat), lalu `getentry` (G-n) dan `listentries` (L-n) boleh paralel, lalu komposisi `app/` (A-n) paling akhir, lalu verifikasi (V).

### FE-0. Prasyarat bersama (bagian dari foundation F6, kerjakan bila belum ada)

1. **i18n** `frontend/src/shared/i18n/id.json` dan `en.json` (format kunci flat seperti file yang ada, mis. `"auditLog.columns.time"`). Daftar lengkap kunci ada di tabel "Kunci i18n" di bawah; tabel ini **menambah** beberapa kunci di luar daftar F6 overview (ditandai `+`). Tes i18n yang ada (`shared/stores/i18n.test.ts`) harus tetap hijau; tambah tes paritas kunci id/en bila belum ada.
2. Tidak ada perubahan lain di `shared/`. Tidak ada dependensi npm baru (`zod`, `svelte` 5, Testing Library, Playwright sudah ada).

### G. Feature `getentry` (`frontend/src/domain/core/audit-log/getentry/`)

**G1. `types.ts`**
- `ACTOR_TYPES = ['user','api_key','anonymous','system'] as const`, `ENTRY_STATUSES = ['ok','error','denied'] as const` (lokal, tidak import dari `listentries`, rule 12.4).
- `export type AuditEntryDetail = z.infer<typeof AuditEntryDetailSchema>` (schema didefinisikan di service, tipe diekspor ulang di sini).
- `export type DetailError = 'notFound' | 'forbidden' | 'load'`.

**G2. `services/getentry.service.ts`** (tanpa state)
```ts
export const AuditEntryDetailSchema = z.object({
  id: z.string().uuid(),
  occurred_at: z.string().datetime({ offset: true }),
  actor_type: z.enum(ACTOR_TYPES),
  actor_id: z.string().nullable(),
  actor_label: z.string(),
  project_id: z.string().nullable(),
  project_label: z.string().nullable(),
  action: z.string(),
  target_type: z.string().nullable(),
  target: z.string().nullable(),
  status: z.enum(ENTRY_STATUSES),
  args: z.record(z.string(), z.unknown()),
  result: z.record(z.string(), z.unknown()),
  ip: z.string().nullable(),
  user_agent: z.string().nullable(),
  trace_id: z.string(),
});

export function getAuditEntry(id: string, signal?: AbortSignal): Promise<AuditEntryDetail> {
  return get(`/api/audit-log/${encodeURIComponent(id)}`, { schema: AuditEntryDetailSchema, signal });
}

export function toDetailError(e: ApiError): DetailError // 404 -> notFound, 403 -> forbidden, lainnya -> load
```
- `get` dari `shared/services/http.ts`. Tidak ada retry/cache. `AbortError` diteruskan (pemanggil yang mengabaikan). 401 sudah memicu redirect global.
- Tidak memvalidasi format `id` di klien: id dari URL dikirim apa adanya (di-encode); backend mengembalikan 404 untuk id bukan uuid (tidak membocorkan format).

**G3. `stores/entryDetail.store.ts`** (state UI, tanpa fetch)
```ts
type DetailState =
  | { status: 'idle' }
  | { status: 'loading'; id: string }
  | { status: 'ready'; id: string; entry: AuditEntryDetail }
  | { status: 'error'; id: string; error: DetailError; apiError: ApiError };
export function createEntryDetailStore(): Readable<DetailState> & {
  begin(id: string): void; resolve(id: string, e: AuditEntryDetail): void;
  fail(id: string, err: ApiError): void; reset(): void;
};
```
`resolve`/`fail` mengabaikan hasil bila `id` tidak sama dengan id yang sedang dimuat (proteksi race di samping abort). Dibuat per instance view (bukan singleton modul), sehingga tidak ada state global. Tidak ada localStorage.

**G4. `components/format.ts`**
- `formatJSON(v: unknown): string`: `JSON.stringify(v, null, 2)` dalam try/catch, fallback `String(v)`.
- `formatDateTime(iso: string, locale: string): string`: `Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' })`, zona lokal browser; ISO tidak valid -> kembalikan string asli.

**G5. `components/AuditEntryDetailPanel.svelte`** (presentasi murni)
Props: `state: DetailState`, `onclose: () => void`, `onretry: () => void`.
- Root `<aside aria-labelledby="audit-detail-title">`, header `<h2 id="audit-detail-title" tabindex="-1">` (`auditLog.detail.title`) + tombol tutup (`aria-label={$t('auditLog.detail.close')}`). Fokus dipindah ke heading saat panel dibuka/berganti id; tombol `Escape` memanggil `onclose`.
- `loading` -> `Spinner` + `aria-busy="true"`.
- `error.notFound` -> `EmptyState` judul `auditLog.detail.notFound`, deskripsi `auditLog.detail.notFoundHint`.
- `error.forbidden` -> `ErrorState` dengan pesan `error.forbidden`, tanpa retry.
- `error.load` -> `ErrorState` (pesan dari `ApiError` + trace id) dengan `onretry`.
- `ready`:
  - Daftar definisi (`<dl>`): waktu (`formatDateTime`, `title` = ISO penuh), aktor (`auditLog.actorType.<type>` + `actor_label`; `actor_id` monospace kecil bila ada), project (`project_label ?? '-'`, `project_id` monospace bila ada), aksi (monospace), target (`target_type` + `target`, `'-'` bila null), status (`auditLog.status.<status>` sebagai teks + warna token; tidak mengandalkan warna saja).
  - Seksi `auditLog.detail.args` dan `auditLog.detail.result`: `<pre class="overflow-auto whitespace-pre-wrap break-words">{formatJSON(value)}</pre>`. Objek kosong `{}` tampil `{}` (bukan disembunyikan). DILARANG `{@html}`/`innerHTML`; nilai `"<img src=x onerror=alert(1)>"` tampil literal.
  - Seksi teknis: `auditLog.detail.ip`, `auditLog.detail.userAgent`, `auditLog.detail.traceId` (null -> `-`).
  - Nilai `[REDACTED]`/`[OMITTED]`/`[TOO_DEEP]`/`_truncated` tampil apa adanya; teks bantu `auditLog.detail.sanitizedHint` di bawah seksi args.
- Lebar layout min 360 px tanpa scroll horizontal halaman (scroll di dalam `<pre>` saja).

**G6. `views/AuditEntryDetailView.svelte`** (satu-satunya pemanggil service di feature ini)
Props: `id: string | null`, `onclose: () => void`.
- `const store = createEntryDetailStore()`. `$effect` bergantung pada `id`: `id` null/kosong -> `store.reset()`, tidak ada request. Selain itu: abort controller sebelumnya, `store.begin(id)`, `getAuditEntry(id, signal)` -> `store.resolve`; catch: abaikan `AbortError`/`signal.aborted`, `ApiError` -> `store.fail(id, err)`, error non-ApiError -> `store.fail` dengan `ApiError` sintetis `{status: 0, code: 'INTERNAL'}`. Cleanup effect meng-abort.
- `onretry` memuat ulang id yang sama.
- Merender `AuditEntryDetailPanel` hanya bila `id` ada.

**G7. `index.ts`** (API publik, rule 12.3)
```ts
export { default as AuditEntryDetailView } from './views/AuditEntryDetailView.svelte';
export type { AuditEntryDetail } from './types';
```
Service, store, panel, dan format tidak diekspor.

### L. Feature `listentries` (`frontend/src/domain/core/audit-log/listentries/`)

**L1. `types.ts`**
- `ACTOR_TYPES`, `AUDIT_STATUSES` (salinan lokal), tipe turunan `ActorType`, `AuditStatus`.
- `AUDIT_ACTIONS`: 33 aksi katalog (sama persis dengan `lib/contracts/core/audit/record/actions.go`), dikelompokkan `ACTION_GROUPS = [{ key: 'mcp', actions: [...] }, { key: 'auth', actions: ['auth.*', 'user.*'] }, { key: 'project', ... }, { key: 'apikey', ... }, { key: 'content', actions: ['knowledge.*', 'apispec.*'] }]`. Komentar: "Sinkron dengan actions.go; provider tidak membatasi aksi. Aksi baru: tambahkan di sini dan di katalog README." Label grup via `auditLog.actionGroup.<key>`; nama aksi tampil sebagai kode (tidak diterjemahkan).
- `RANGE_PRESETS = [{ key: '1h', ms: 3_600_000 }, { key: '24h', ms: 86_400_000 }, { key: '7d', ms: 604_800_000 }, { key: '30d', ms: 2_592_000_000 }, { key: '90d', ms: 7_776_000_000 }] as const`; `RangeKey = typeof RANGE_PRESETS[number]['key'] | 'all' | 'custom'`.
- `AuditFilter { actorType: ActorType | ''; actorId: string; actorLabel: string; projectId: string; actions: string[]; statuses: AuditStatus[]; range: RangeKey; from: string; to: string; target: string }` (string kosong = tidak ada; `from`/`to` hanya terisi bila `range === 'custom'`).
- `EMPTY_FILTER`, `MAX_ACTIONS = 20`, `MAX_TEXT = 200`, `PAGE_SIZE = 50`, `UUID_RE`, `ACTION_RE = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/`.
- `AuditSummary = z.infer<typeof AuditSummarySchema>`.

**L2. `services/listentries.service.ts`** (tanpa state)
- Schema:
  ```ts
  export const AuditSummarySchema = z.object({
    id: z.string().uuid(), occurred_at: z.string().datetime({ offset: true }),
    actor_type: z.enum(ACTOR_TYPES), actor_id: z.string().nullable(), actor_label: z.string(),
    project_id: z.string().nullable(), project_label: z.string().nullable(),
    action: z.string(), target_type: z.string().nullable(), target: z.string().nullable(),
    status: z.enum(AUDIT_STATUSES),
  });
  export const AuditListSchema = z.object({ items: z.array(AuditSummarySchema), next_cursor: z.string().nullable() });
  ```
- `resolveRange(f: AuditFilter, now: Date): { from?: string; to?: string }`: preset -> `from = now - ms` (ISO UTC, detik, tanpa milidetik), `to` kosong; `custom` -> `from`/`to` dari filter; `all` -> kosong.
- `buildListQuery(f, range, cursor: string | null, limit = PAGE_SIZE): string`: `URLSearchParams.append` hanya untuk nilai tidak kosong; `action` dan `status` di-append per nilai (karena `RequestOptions.query` di `http.ts` hanya satu nilai per kunci); `actor_id` hanya dikirim bersama `actor_type`. Mengembalikan `/api/audit-log?…` (tanpa `?` bila kosong). `range` TIDAK dikirim ke backend.
- `listEntries(f, range, cursor, signal): Promise<{ items: AuditSummary[]; nextCursor: string | null }>` via `get(url, { schema: AuditListSchema, signal })`.
- `ApiError` dilempar apa adanya.

**L3. `stores/filterQuery.ts`** (fungsi murni sinkron URL, tanpa fetch)
- `filterFromQuery(q: URLSearchParams): AuditFilter`: baca kunci filter; nilai enum liar (actor_type/status) dibuang; `action` yang tidak cocok `ACTION_RE` dibuang lalu dipotong ke `MAX_ACTIONS`; `actor_id`/`project_id` yang bukan uuid dibuang; teks dipotong ke `MAX_TEXT`; `range` dikenal -> dipakai; `from`/`to` ada (ISO valid) -> `range = 'custom'`; tanpa keduanya -> `range = 'all'`. Mengabaikan `entry`, `cursor`, `limit`.
- `filterToQuery(f, base?: URLSearchParams): URLSearchParams`: tulis ulang hanya kunci filter yang terisi dengan urutan stabil (`actor_type, actor_id, actor_label, project_id, action…, status…, range | from,to, target`), mempertahankan kunci non-filter dari `base` kecuali `entry` (apply filter menutup panel). `range = 'all'` tidak ditulis.
- `filterKey(f): string` kanonik (JSON terurut) untuk memutuskan reload.
- `validateFilter(f): Partial<Record<keyof AuditFilter, string>>` (kunci i18n error per field): uuid project/actor, `MAX_TEXT`, `MAX_ACTIONS`, `from < to` untuk custom. Mencerminkan validasi server; server tetap otoritatif.

**L4. `stores/listentries.store.ts`** (state UI, tanpa fetch)
```ts
interface PageState { cursor: string | null; items: AuditSummary[]; nextCursor: string | null }
interface ListState {
  key: string;                                  // filterKey saat halaman pertama dimuat
  range: { from?: string; to?: string };        // dibekukan per filterKey agar cursor konsisten
  pages: PageState[];                           // cursor stack
  status: 'idle' | 'loading' | 'ready' | 'error';
  error: ApiError | null;                       // error halaman pertama
  moreStatus: 'idle' | 'loading' | 'error';
  moreError: ApiError | null;                   // error "muat berikutnya" (halaman lama tetap)
}
```
Method: `reset(key, range)`, `begin()`, `pushPage(page)`, `fail(err)`, `beginMore()`, `failMore(err)`. Derived: `rows` (gabungan halaman, dedupe by `id`), `nextCursor` (halaman terakhir), `hasMore`, `suggestions` (`actors`: distinct `{actorType, actorId, actorLabel}` maks 50; `projects`: distinct `{projectId, projectLabel}` non-null maks 50). Dibuat per instance view.

**L5. Komponen** (presentasi; tidak memanggil service)
- `ActorIcon.svelte` (props `type`): SVG inline; ikon "agent" untuk `api_key` dan `anonymous`, "orang" untuk `user`, "gir" untuk `system`; `role="img"`, `aria-label`/`<title>` = `auditLog.actorType.<type>`.
- `StatusBadge.svelte` (props `status`): teks `auditLog.status.<status>` + kelas token (`ok` netral/sukses, `error` danger, `denied` warning). Teks selalu tampil.
- `AuditTable.svelte` (props `rows`, `status`, `error`, `selectedId`, `hasMore`, `moreStatus`, `moreError`, `filtered: boolean`, `onselect(id)`, `onloadmore()`, `onretry()`, `onretrymore()`, `onresetfilter()`):
  - Memakai `shared/components/Table.svelte` dengan `columns` `auditLog.columns.{time,actor,project,action,target,status}`, `caption` `auditLog.title`, `rowKey = r => r.id`, `cell` snippet.
  - Sel waktu berisi `<button type="button">` (Enter/Space) yang memanggil `onselect(id)`; teks waktu `formatDateTime`, `title` ISO. Baris terpilih `aria-current="true"` + highlight.
  - Aktor: `ActorIcon` + `actor_label`. Project: `project_label ?? '-'`. Aksi: monospace. Target: dipotong CSS (`truncate max-w-[24ch]`, `title` penuh). Status: `StatusBadge`.
  - Loading awal: `Table loading` (Spinner, `aria-busy`).
  - Empty: tanpa filter -> `EmptyState` `auditLog.empty` + deskripsi `auditLog.emptyHint`; dengan filter -> `auditLog.emptyFiltered` + tombol `auditLog.filter.reset` (`onresetfilter`).
  - Error awal: lihat matriks state (ErrorState; retry hanya untuk error yang bisa dicoba ulang).
  - Footer: tombol `auditLog.loadMore` hanya bila `hasMore`; saat `moreStatus='loading'` tombol disabled + Spinner kecil + `aria-live="polite"` `auditLog.loadingMore`; saat `moreStatus='error'` tampil pesan inline + tombol retry (`onretrymore`). Halaman terakhir (`hasMore=false` dan `rows.length>0`): teks kecil `auditLog.endOfList`.
  - Responsif: di bawah `md`, tabel di dalam wrapper `overflow-x-auto` (scroll di dalam tabel, bukan halaman); kolom target dan project disembunyikan di bawah `sm` (tetap terlihat di panel detail).
- `ActionPicker.svelte` (props `value: string[]`, `onchange`): `<details>` dengan ringkasan `auditLog.filter.selectedCount` (`{n}`), kotak cari (`auditLog.filter.searchAction`), checkbox per aksi dikelompokkan `ACTION_GROUPS`; bila `value.length >= MAX_ACTIONS`, checkbox yang belum dipilih `disabled` + teks `auditLog.filter.actionLimit`. Aksi dari URL yang tidak ada di katalog tetap ditampilkan di grup `auditLog.actionGroup.other` agar bisa dilepas.
- `FilterBar.svelte` (props `value: AuditFilter`, `suggestions`, `errors`, `onapply(f)`, `onreset()`, `disabled`): `<form role="search" aria-label={$t('auditLog.filter.label')}>` dengan draft lokal (`$state` salinan `value`; disinkron ulang saat `value` berubah dari URL).
  - Jenis aktor `<select>`: `auditLog.filter.any` + 4 tipe.
  - Aktor: `Input` `actor_label` + `<datalist>` dari `suggestions.actors`; memilih saran yang punya `actorId` mengisi `actorType` + `actorId` (filter persis, pakai indeks). Bila `actorId` terisi tampil chip `auditLog.filter.actorIdChip` (`{id}` 8 karakter pertama) dengan tombol hapus (`aria-label` `auditLog.filter.removeActorId`).
  - Project: `Input` + `<datalist>` label dari `suggestions.projects`; label cocok -> `projectId`; UUID ditempel langsung diterima; teks lain -> error inline `auditLog.filter.projectInvalid`.
  - Aksi: `ActionPicker`. Status: tiga checkbox.
  - Rentang waktu `<select>`: `1h, 24h, 7d, 30d, 90d, all`; opsi `custom` hanya muncul (disabled, label `auditLog.range.custom` + rentang terformat) bila URL membawa `from`/`to`.
  - Target: `Input maxlength=200`.
  - Tombol `auditLog.filter.apply` (submit) dan `auditLog.filter.reset`. Error validasi (`validateFilter`) mencegah submit, ditampilkan per field dengan `aria-invalid` + `aria-describedby`.
  - Layout: grid 1 kolom di mobile, 3 kolom di `lg`. Semua kontrol punya `<label for>`.

**L6. `views/ListEntriesView.svelte`** (komposisi tipis, satu-satunya pemanggil service di feature ini)
Props: `{ query: URLSearchParams; selectedId: string | null; onquerychange: (q: URLSearchParams) => void; onselect: (id: string) => void }`. Tidak meng-import router atau `app/` (aturan `domain-no-app`).
- `filter = $derived(filterFromQuery(query))`, `key = $derived(filterKey(filter))`.
- `$effect` hanya bergantung pada `key`: abort request berjalan, `range = resolveRange(filter, new Date())`, `store.reset(key, range)`, `store.begin()`, `listEntries(filter, range, null, signal)` -> `pushPage` / `fail`. Hasil usang diabaikan (`signal.aborted`). Perubahan hanya `entry` tidak memicu request.
- `loadMore()`: `store.beginMore()`, `listEntries(filter, state.range, nextCursor, signal)` -> `pushPage` / `failMore` (halaman sebelumnya tidak dibuang). `retryMore` memakai cursor yang sama. Tombol tidak bisa ditekan ganda (disabled saat loading).
- `refresh()` (tombol `auditLog.refresh` di header): reset + muat ulang halaman pertama dengan `resolveRange` baru.
- `onapply(f)` -> `onquerychange(filterToQuery(f, query))`; `onreset()` -> `onquerychange(filterToQuery(EMPTY_FILTER, query))`.
- Header: `<h1>` `auditLog.title` + subjudul `auditLog.subtitle` (retensi 90 hari, hanya-baca).
- `aria-live="polite"` region yang mengumumkan `auditLog.resultCount` (`{n}` baris dimuat) setelah tiap halaman.

**L7. `index.ts`**
```ts
export { default as ListEntriesView } from './views/ListEntriesView.svelte';
export type { AuditSummary, AuditFilter } from './types';
```

### A. Komposisi `app/` (terakhir, setelah kedua `index.ts` ada)

**A1. `frontend/src/app/views/AuditLogView.svelte`**
- Import hanya lewat index: `import { ListEntriesView } from '../../domain/core/audit-log/listentries'`, `import { AuditEntryDetailView } from '../../domain/core/audit-log/getentry'`; `import { current, navigate } from '../routes/router'`. Menerima prop `params` (diabaikan) agar cocok dengan `App.svelte`.
- `query = $derived($current.location.query)`, `entryId = $derived(query.get('entry') || null)`.
- `onquerychange(q)` -> `navigate('/audit-log' + (q.size ? '?' + q : ''), { replace: true })` (ubah filter tidak menambah history; reload menyimpan filter).
- `onselect(id)` -> salin query, `set('entry', id)`, `navigate(...)` **push** (Back menutup panel). Simpan id tombol baris pemicu untuk mengembalikan fokus.
- Panel `onclose` -> hapus `entry`, `navigate(...)` replace, kembalikan fokus ke tombol baris (`document.querySelector('[data-entry-id="<id>"]')`, bila ada).
- Layout: `lg` ke atas grid `[1fr_28rem]` (tabel + `<AuditEntryDetailView>` sticky); di bawah `lg` panel menjadi overlay `fixed inset-0` dengan backdrop, `role="dialog" aria-modal="true"`, body tidak scroll. Panel hanya dirender bila `entryId` ada.

**A2. `frontend/src/app/routes/index.ts`**
```ts
import AuditLogView from '../views/AuditLogView.svelte';
{ path: '/audit-log', view: AuditLogView, meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'auditLog.title' } },
```
Guard yang ada: `unknown` -> pending, anonim -> redirect `/login?next=/audit-log%3F...`, developer -> `notfound`. Router mencocokkan path saja sehingga query tidak mengganggu. Jangan mengubah route lain (T01 juga menambah route; konflik merge simpan keduanya).

**A3. `frontend/src/app/layouts/AppLayout.svelte`**
Tambah satu `<nav aria-label={$t('nav.main')}>` (bila T01 sudah menambah nav, sisipkan satu `<a>` saja) berisi `<a href="/audit-log">{$t('auditLog.menu')}</a>` hanya bila `$session.identity?.role === 'super_admin'`; `aria-current="page"` bila `$current.location.path` diawali `/audit-log`. UX saja (rule 12.7).

### V. Verifikasi

`npm run check`, `npx vitest run src/domain/core/audit-log src/app`, `npm run test:arch` (depcruise), `npm run build`, lalu `npm run test:e2e` terhadap binary bila T01 (login + create-admin) sudah di-merge. Grep wajib kosong: `grep -rn "{@html\|innerHTML" frontend/src/domain/core/audit-log frontend/src/app/views/AuditLogView.svelte`; `grep -rnE "\b(post|put|patch|del)\(" frontend/src/domain/core/audit-log` (tidak ada mutasi, AC7).

### Matriks state UI

| Area | Kondisi | Tampilan | Aksi pengguna |
|---|---|---|---|
| Halaman | sesi `unknown` | `PendingView` (guard) | - |
| Halaman | anonim / 401 di tengah sesi | redirect `/login?next=…` (guard / handler 401 global `http.ts`) | login lalu kembali ke URL yang sama + filter |
| Halaman | role developer | `NotFoundView` (guard); menu tidak tampil | - |
| Tabel | loading halaman 1 | `Table loading` (Spinner, `aria-busy=true`), FilterBar tetap bisa dipakai | ubah filter membatalkan request lama |
| Tabel | `ready`, 0 baris, tanpa filter | `EmptyState auditLog.empty` + `auditLog.emptyHint` | - |
| Tabel | `ready`, 0 baris, ada filter | `EmptyState auditLog.emptyFiltered` | tombol reset filter |
| Tabel | 400 `INVALID_ARGUMENT` (deep link rusak, mis. `cursor`/`limit` dari luar) | `ErrorState` pesan server + `auditLog.error.invalidFilter` (`{field}` dari `details.field`) | tombol reset filter (tanpa retry) |
| Tabel | 403 `FORBIDDEN` (role dicabut saat halaman terbuka) | `ErrorState` `error.forbidden` | tanpa retry |
| Tabel | `NETWORK` (status 0), 5xx, `INVALID_RESPONSE` | `ErrorState` `auditLog.error.load` + pesan + trace id | retry |
| Tabel | muat berikutnya loading | tombol disabled + Spinner + live region | - |
| Tabel | muat berikutnya gagal | halaman lama tetap; pesan inline + retry | retry cursor yang sama |
| Tabel | halaman terakhir | tombol hilang, `auditLog.endOfList` | - |
| Panel | loading | Spinner, `aria-busy` | tutup |
| Panel | 404 (id bukan uuid / tidak ada / sudah dipurge) | `EmptyState auditLog.detail.notFound` + hint | tutup |
| Panel | 403 | `ErrorState error.forbidden` | tutup |
| Panel | network/5xx/`INVALID_RESPONSE` | `ErrorState` + trace id | retry, tutup |
| Panel | `args`/`result` kosong `{}` | `<pre>{}</pre>` | - |
| Filter | input tidak valid | error inline per field, submit dicegah | perbaiki |

### Kunci i18n

Kunci bertanda `+` belum ada di daftar F6 overview dan ditambahkan oleh FE-0.

| Kunci | id | en |
|---|---|---|
| `auditLog.menu` | Audit Log | Audit Log |
| `auditLog.title` | Audit Log | Audit Log |
| `auditLog.subtitle` + | Aktivitas agent dan pengguna, disimpan 90 hari. Hanya-baca. | Agent and user activity, kept for 90 days. Read-only. |
| `auditLog.refresh` + | Muat ulang | Refresh |
| `auditLog.empty` | Belum ada entri audit | No audit entries yet |
| `auditLog.emptyHint` + | Entri muncul saat agent memanggil MCP atau pengguna melakukan aksi sensitif. | Entries appear when agents call MCP or users perform sensitive actions. |
| `auditLog.emptyFiltered` + | Tidak ada entri yang cocok dengan filter | No entries match the filter |
| `auditLog.loadMore` | Muat berikutnya | Load more |
| `auditLog.loadingMore` + | Memuat entri berikutnya… | Loading more entries… |
| `auditLog.endOfList` + | Semua entri sudah dimuat | All entries loaded |
| `auditLog.resultCount` + | {n} entri dimuat | {n} entries loaded |
| `auditLog.columns.{time,actor,project,action,target,status}` | Waktu, Aktor, Project, Aksi, Target, Status | Time, Actor, Project, Action, Target, Status |
| `auditLog.actorType.{user,api_key,anonymous,system}` | Pengguna, API key (agent), Anonim, Sistem | User, API key (agent), Anonymous, System |
| `auditLog.status.{ok,error,denied}` | Berhasil, Gagal, Ditolak | OK, Error, Denied |
| `auditLog.actionGroup.{mcp,auth,project,apikey,content,other}` + | MCP, Autentikasi & pengguna, Project, API key, Knowledge & API spec, Lainnya | MCP, Auth & users, Project, API keys, Knowledge & API specs, Other |
| `auditLog.filter.label` + | Filter audit log | Audit log filter |
| `auditLog.filter.{actorType,actor,project,action,status,range,target}` | Jenis aktor, Aktor, Project, Aksi, Status, Rentang waktu, Teks target | Actor type, Actor, Project, Action, Status, Time range, Target text |
| `auditLog.filter.{apply,reset,any}` | Terapkan, Reset, Semua | Apply, Reset, Any |
| `auditLog.filter.selectedCount` + | {n} dipilih | {n} selected |
| `auditLog.filter.searchAction` + | Cari aksi | Search actions |
| `auditLog.filter.actionLimit` + | Maksimal 20 aksi | Up to 20 actions |
| `auditLog.filter.actorIdChip` + | ID aktor: {id} | Actor ID: {id} |
| `auditLog.filter.removeActorId` + | Hapus filter ID aktor | Remove actor ID filter |
| `auditLog.filter.projectInvalid` + | Pilih project dari saran atau tempel UUID project | Pick a suggested project or paste a project UUID |
| `auditLog.filter.textTooLong` + | Maksimal 200 karakter | At most 200 characters |
| `auditLog.filter.rangeInvalid` + | Waktu mulai harus sebelum waktu akhir | Start must be before end |
| `auditLog.range.{1h,24h,7d,30d,90d,all}` | 1 jam, 24 jam, 7 hari, 30 hari, 90 hari, Semua | 1 hour, 24 hours, 7 days, 30 days, 90 days, All |
| `auditLog.range.custom` + | Kustom: {from} - {to} | Custom: {from} - {to} |
| `auditLog.detail.{title,args,result,ip,userAgent,traceId,close,notFound}` | Detail entri, Argumen, Ringkasan hasil, Alamat IP, User agent, Trace ID, Tutup, Entri tidak ditemukan | Entry detail, Arguments, Result summary, IP address, User agent, Trace ID, Close, Entry not found |
| `auditLog.detail.notFoundHint` + | Entri mungkin sudah melewati masa retensi. | The entry may have passed its retention period. |
| `auditLog.detail.sanitizedHint` + | Nilai rahasia ditutup dan isi file tidak disimpan. | Secret values are redacted and file contents are never stored. |
| `auditLog.error.load` | Gagal memuat audit log | Failed to load the audit log |
| `auditLog.error.invalidFilter` + | Filter tidak valid ({field}) | Invalid filter ({field}) |
| `nav.main` + (hanya bila belum ada dari T01) | Navigasi utama | Main navigation |

### Keputusan dan selisih dengan plan usecase

1. **Ekspor `getentry`**: hanya `AuditEntryDetailView` (props `id`, `onclose`) + tipe. `usecase-listentries.md` menyebut `EntryDetailPanel`, `usecase-getentry.md` juga mengekspor panel/store/service; keduanya diganti dengan ekspor ini (rule 12.3, API publik minimal).
2. **Tidak ada store `selectedEntry` global**: id terpilih hidup di URL (`?entry=`), diteruskan lewat props. Store detail dibuat per instance view. Lebih sederhana (rule 1.5) dan menghindari dua sumber kebenaran.
3. **Rentang waktu relatif di URL sebagai `range=<preset>`**, bukan `from` absolut. Alasan: reload "7 hari" tetap berarti 7 hari terakhir, dan pilihan preset tetap tampil benar setelah reload. `from` dihitung saat memuat halaman pertama dan dibekukan untuk cursor berikutnya. `from`/`to` absolut tetap diterima dari URL (deep link) dan tampil sebagai "Kustom". Backend tidak menerima `range`; parameter ini murni UI.
4. **Default tanpa filter rentang (`all`)**, sama dengan default backend (indeks `occurred_at`).
5. **Validasi Zod `listentries` dan `getentry` diseragamkan** (`uuid`, `datetime({offset:true})`); `action` tetap `z.string()` karena provider tidak membatasi katalog.
6. **Autocomplete tanpa endpoint baru**: saran aktor/project dari baris yang sudah dimuat (overview, "Keputusan lintas usecase").
7. Helper `formatDateTime` diduplikasi kecil di masing-masing feature (bukan `shared/`), agar feature tetap self-contained tanpa mengubah `shared/`.

## Migration

Tidak ada (frontend). Tabel `audit_log` dibuat foundation F1: `backend/migrations/core/20261021090000000_create_audit_log.sql`.

## Changes

| File / folder | Aksi | Langkah |
|---|---|---|
| `frontend/src/shared/i18n/id.json`, `en.json` | ubah (kunci `auditLog.*`, `nav.main` bila belum ada) | FE-0 |
| `frontend/src/shared/i18n/i18n-keys.test.ts` | buat bila belum ada (paritas kunci id/en) | FE-0 |
| `frontend/src/domain/core/audit-log/getentry/types.ts` | buat | G1 |
| `.../getentry/services/getentry.service.ts` + `getentry.service.test.ts` | buat | G2 |
| `.../getentry/stores/entryDetail.store.ts` + `entryDetail.store.test.ts` | buat | G3 |
| `.../getentry/components/format.ts` + `format.test.ts` | buat | G4 |
| `.../getentry/components/AuditEntryDetailPanel.svelte` + `AuditEntryDetailPanel.test.ts` | buat | G5 |
| `.../getentry/views/AuditEntryDetailView.svelte` + `AuditEntryDetailView.test.ts` | buat | G6 |
| `.../getentry/index.ts` | buat | G7 |
| `frontend/src/domain/core/audit-log/listentries/types.ts` + `types.test.ts` | buat | L1 |
| `.../listentries/services/listentries.service.ts` + `listentries.service.test.ts` | buat | L2 |
| `.../listentries/stores/filterQuery.ts` + `filterQuery.test.ts` | buat | L3 |
| `.../listentries/stores/listentries.store.ts` + `listentries.store.test.ts` | buat | L4 |
| `.../listentries/components/format.ts` | buat | L5 |
| `.../listentries/components/{ActorIcon,StatusBadge,AuditTable,ActionPicker,FilterBar}.svelte` + `components.test.ts` | buat | L5 |
| `.../listentries/views/ListEntriesView.svelte` + `ListEntriesView.test.ts` | buat | L6 |
| `.../listentries/index.ts` | buat | L7 |
| `frontend/src/app/views/AuditLogView.svelte` + `AuditLogView.test.ts` | buat | A1 |
| `frontend/src/app/routes/index.ts` | ubah (route `/audit-log`) | A2 |
| `frontend/src/app/layouts/AppLayout.svelte` | ubah (item menu super_admin) | A3 |
| `frontend/e2e/audit-log.spec.ts` | buat | V |
| `frontend/e2e/helpers/auth.ts` | buat bila T01 belum menyediakan helper login | V |

Tidak ada perubahan `shared/` selain i18n. Tidak ada dependensi baru.

## Test Scenario

Vitest + Testing Library (jsdom), `fetch` di-stub via `vi.stubGlobal('fetch')` seperti `shared/services/http.test.ts`; view diuji dengan service di-mock (`vi.mock`). Tes yang membuktikan kriteria memakai prefiks `T02-AC<n>` di nama (rule 13.4).

### getentry
- `getentry.service.test.ts`: URL `/api/audit-log/<id>` dengan `encodeURIComponent` (id `a/b` -> `a%2Fb`); respons valid lolos dan tipe benar; `actor_type: "robot"` / `occurred_at: "kemarin"` / `args: []` -> `ApiError` `INVALID_RESPONSE`; 404 envelope -> `ApiError` status 404 code `NOT_FOUND`; `toDetailError` memetakan 404/403/500/0 dengan benar; signal abort meneruskan `AbortError`.
- `entryDetail.store.test.ts`: `begin(A)` -> loading A; `resolve(B, …)` saat memuat A diabaikan; `fail(A, 404)` -> `notFound`; `reset()` -> idle.
- `format.test.ts`: objek bersarang terformat 2 spasi; siklus/`BigInt` -> fallback `String(v)` tanpa throw; `formatDateTime` dengan locale `id` dan `en`; ISO invalid dikembalikan apa adanya.
- `AuditEntryDetailPanel.test.ts`: `T02-AC4` nilai `"[REDACTED]"` tampil literal dan tidak ada teks rahasia; `T02-AC3` entri dengan `project_label` snapshot tanpa project aktif tetap menampilkan label; XSS: `target`/`args.path` = `"<img src=x onerror=alert(1)>"` muncul sebagai teks, `container.querySelector('img')` null; nilai null -> `-`; state loading/notFound/forbidden/load (retry hanya pada `load`); tombol tutup dan `Escape` memanggil `onclose`; fokus berpindah ke heading saat dibuka.
- `AuditEntryDetailView.test.ts`: `id=null` -> tidak ada request; `id` A -> B: request A di-abort dan respons lambat A tidak menimpa B; 404 -> panel notFound; 500 -> load + retry memanggil ulang service dengan id sama.

### listentries
- `types.test.ts`: `AUDIT_ACTIONS` 33 nilai unik, semua cocok `ACTION_RE`, sama dengan daftar literal katalog README; setiap aksi muncul tepat di satu grup.
- `listentries.service.test.ts`: `buildListQuery` meng-append `action` dan `status` berulang, membuang nilai kosong, meng-encode `actor_label`/`target` berisi `&`, spasi, `%`, tidak mengirim `range`, tidak mengirim `actor_id` tanpa `actor_type`, menyertakan `cursor` dan `limit=50`; `resolveRange('7d', fixedNow)` -> `from = fixedNow - 7d` (ISO detik), `all` -> kosong, `custom` -> `from`/`to` asli; respons valid -> `nextCursor` camelCase; `next_cursor: null` -> `null`; item rusak -> `INVALID_RESPONSE`; 400 envelope -> `ApiError` dengan `details.field`; 403 -> status 403.
- `filterQuery.test.ts`: round-trip `filterFromQuery(filterToQuery(f))` untuk filter lengkap (aksi berulang, urutan stabil); `entry` dihapus oleh `filterToQuery(f, base)` sedangkan kunci asing lain dipertahankan; enum liar dibuang; 25 aksi -> 20; aksi `BAD` dibuang; `project_id=abc` dibuang; `from`+`to` -> `range=custom`; `range=7d` dikenali; `filterKey` sama untuk query yang hanya berbeda `entry`; `validateFilter` (uuid, 201 karakter, from >= to).
- `listentries.store.test.ts`: `pushPage` dua kali -> `rows` berurutan, dedupe id duplikat di batas halaman, `nextCursor` dari halaman terakhir, `hasMore` false saat `null`; `failMore` mempertahankan halaman; `reset` mengosongkan; `range` dibekukan sampai `reset` berikutnya; saran distinct maks 50 dan mengabaikan `project_id` null.
- `components.test.ts`:
  - `AuditTable`: kolom berlabel i18n; `T02-AC1` baris `api_key` menampilkan ikon agent dengan nama aksesibel "API key (agent)", `user` ikon orang; XSS `actor_label` literal tanpa elemen `img`; klik / Enter pada tombol waktu memanggil `onselect(id)`; `selectedId` memberi `aria-current`; empty tanpa/dengan filter (tombol reset hanya saat filter); error 403 tanpa retry, error 500 dengan retry dan trace id; tombol muat berikutnya hanya saat `hasMore`, disabled saat loading, error inline + retry saat `moreStatus='error'`; `endOfList` tampil di halaman terakhir.
  - `ActionPicker`: memilih 20 aksi menonaktifkan sisanya; pencarian menyaring; aksi di luar katalog tampil di grup "Lainnya".
  - `FilterBar`: apply menghasilkan `AuditFilter` benar (preset 7 hari -> `range='7d'`); memilih saran aktor mengisi `actorType`+`actorId`; chip actor id bisa dihapus; project teks bebas -> error inline dan `onapply` tidak dipanggil; target 201 karakter dicegah (`maxlength`); reset memanggil `onreset`; semua kontrol punya label (`getByLabelText`).
- `ListEntriesView.test.ts` (service di-mock): render awal memanggil `listEntries` sekali dengan filter dari `query`; perubahan `query` filter -> request baru dan request lama di-abort; perubahan hanya `entry` -> tidak ada request; "Muat berikutnya" memanggil service dengan `nextCursor` dan `range` yang sama dengan halaman 1; gagal halaman 2 mempertahankan halaman 1 dan retry memakai cursor sama; apply filter memanggil `onquerychange` tanpa `entry`; tombol refresh memuat ulang halaman 1.

### app/
- `AuditLogView.test.ts` (memakai `startRouter` + `App` seperti `app/views/app.test.ts`, `fetch` di-stub):
  - `T02-AC5` developer membuka `/audit-log` -> `notfound-title` tampil dan tidak ada fetch ke `/api/audit-log`; menu tidak tampil.
  - Super admin: menu "Audit Log" tampil dengan `aria-current="page"`; route merender judul; `document.title` = `Audit Log · Grimoire`.
  - Anonim -> redirect `/login?next=%2Faudit-log…` (query ikut).
  - Klik baris -> URL berisi `entry=<id>` (pushState), panel memuat detail; Back (`popstate`) menutup panel; tombol tutup menghapus `entry` (replaceState) dan fokus kembali ke tombol baris.
  - Apply filter -> URL berubah via replaceState (panjang `history` tidak bertambah).
  - Deep link `/audit-log?actor_type=api_key&actor_id=<uuid>` -> request berisi kedua param dan chip actor id tampil.
- Tes arsitektur (`npm run test:arch`): lolos tanpa pengecualian baru; fixture depcruise yang ada tetap membuktikan `listentries` -> `getentry/components/...` ditolak.

## E2E Scenarios

File `frontend/e2e/audit-log.spec.ts` (Playwright terhadap binary). Prasyarat: T01 (`create-admin`, `/login`, user management) sudah di-merge; DB uji bersih per run (`grimoire migrate up`). Seed memakai alur nyata yang menulis audit lewat `core.audit.record`: login sukses (`auth.login.success`), login gagal (`auth.login.failed`), membuat user developer (`user.created`). Skenario yang butuh entri MCP (`mcp.*`) ditandai `test.fixme` sampai T07 tersedia.

Positif:
1. Super admin login -> menu "Audit Log" tampil -> klik -> `/audit-log` menampilkan tabel urut terbaru; baris `auth.login.success` milik admin dengan ikon pengguna; tidak ada console error dan tidak ada respons >= 400.
2. Filter aksi `auth.login.failed` + status `denied`/`error` + preset 24 jam -> hanya baris login gagal; URL berisi `action=auth.login.failed&status=…&range=24h`; request jaringan berisi `from=` dan tanpa `range`; reload -> filter dan hasil sama.
3. Paginasi: seed > 50 entri (55 login gagal via `request.post('/api/auth/login')`) -> 50 baris, klik "Muat berikutnya" -> 55 baris tanpa id duplikat, tombol hilang dan teks "Semua entri sudah dimuat" tampil.
4. Klik baris -> URL `?entry=<id>`, panel menampilkan argumen (password login gagal tampil `[REDACTED]` atau tidak ada; AC4), ringkasan hasil, IP, user agent, trace id; tombol Back browser menutup panel; reload dengan `?entry=` membuka panel langsung.
5. Deep link `/audit-log?actor_type=user&actor_id=<id admin>` -> hanya entri admin; chip actor id tampil dan bisa dihapus.
6. Viewport 375x812: tabel scroll di dalam wrapper, halaman tanpa scroll horizontal, panel detail tampil sebagai overlay dan `Escape` menutupnya.
7. Ganti bahasa ke `en` -> judul "Audit Log", kolom "Time/Actor/…", status "Denied".
8. (fixme sampai T07) `read_file` via MCP -> baris `mcp.read_file` dengan aktor `api_key`, nama key, project, target path; detail `result.bytes` (AC1). Key dicabut -> baris `mcp.auth.rejected` status Ditolak, aktor Anonim dengan prefix key (AC2). Setelah project/key dihapus baris tetap berlabel snapshot (AC3).

Negatif:
1. Developer login -> menu tidak tampil; buka `/audit-log` langsung -> halaman tidak ditemukan; `page.request.get('/api/audit-log')` -> 403 `FORBIDDEN` (AC5).
2. Tanpa sesi buka `/audit-log?action=auth.logout` -> redirect `/login?next=%2Faudit-log%3Faction%3Dauth.logout`; setelah login kembali ke URL itu dengan filter terpasang.
3. Deep link `/audit-log?project_id=bukan-uuid&status=x&action=BAD` -> nilai liar dibuang di klien, tidak ada request 400, tabel tampil tanpa filter tersebut.
4. `page.route('/api/audit-log?**')` dipaksa 500 -> ErrorState dengan trace id + tombol coba lagi; lepas route lalu coba lagi -> tabel tampil.
5. `page.route` mengembalikan JSON tidak sesuai schema -> ErrorState "Respons server tidak sesuai format".
6. Halaman 2 dipaksa gagal (route untuk request ber-`cursor`) -> halaman 1 tetap, pesan inline + retry berhasil.
7. `?entry=00000000-0000-7000-8000-000000000000` dan `?entry=abc` -> panel "Entri tidak ditemukan".
8. Seed entri dengan `target`/`args` berisi `<img src=x onerror=window.__xss=1>` (via login gagal dengan email tersebut) -> tampil literal, `window.__xss` undefined.
9. Respons lambat: ubah filter dua kali cepat -> hanya hasil filter terakhir yang tampil (request pertama di-abort).
10. UI tidak menyediakan tombol ubah/hapus entri; `page.request.delete('/api/audit-log/<id>')` -> 405 `METHOD_NOT_ALLOWED` (AC7).

## Request/Response body

Kontrak mengikuti `overview.md` dan `api/modules/core/audit-log.yml`. Envelope error dari foundation: `{ "error": { "code", "message", "details" }, "trace_id" }`, dipetakan `http.ts` ke `ApiError { status, code, message, details, traceId }`.

### Pemetaan URL UI -> request API

| URL UI `/audit-log?…` | Request `GET /api/audit-log?…` |
|---|---|
| `actor_type=api_key&actor_id=<uuid>` | `actor_type=api_key&actor_id=<uuid>` |
| `actor_label=budi` | `actor_label=budi` (contains, ILIKE) |
| `project_id=<uuid>` | `project_id=<uuid>` |
| `action=mcp.read_file&action=mcp.search_knowledge` | sama (berulang) |
| `status=ok&status=error` | sama (berulang) |
| `range=7d` | `from=<now-7d ISO>` (dibekukan untuk cursor) |
| `from=…&to=…` | sama |
| `target=login.md` | `target=login.md` |
| `entry=<id>` | tidak dikirim; memicu `GET /api/audit-log/<id>` |
| (muat berikutnya) | `cursor=<next_cursor>` |
| (selalu) | `limit=50` |

### `GET /api/audit-log?project_id=0192f0c4-0000-7000-8000-000000000001&action=mcp.read_file&action=mcp.search_knowledge&status=ok&from=2026-10-13T00:00:00Z&limit=50`

200:
```json
{
  "items": [
    {
      "id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f",
      "occurred_at": "2026-10-19T08:14:22.123456Z",
      "actor_type": "api_key",
      "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
      "actor_label": "claude-code-budi (gk_ab12)",
      "project_id": "0192f0c4-0000-7000-8000-000000000001",
      "project_label": "Payments",
      "action": "mcp.read_file",
      "target_type": "file",
      "target": "docs/auth/login.md",
      "status": "ok"
    },
    {
      "id": "0192f0c4-7aff-7c00-8b00-0f0e0d0c0b0a",
      "occurred_at": "2026-10-19T08:14:20Z",
      "actor_type": "api_key",
      "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
      "actor_label": "claude-code-budi (gk_ab12)",
      "project_id": "0192f0c4-0000-7000-8000-000000000001",
      "project_label": "Payments",
      "action": "mcp.search_knowledge",
      "target_type": null,
      "target": null,
      "status": "ok"
    }
  ],
  "next_cursor": "MjAyNi0xMC0xOVQwODoxNDoyMFp8MDE5MmYwYzQtN2FmZi03YzAwLThiMDAtMGYwZTBkMGMwYjBh"
}
```

Halaman terakhir: `"next_cursor": null`. Tanpa hasil: `{ "items": [], "next_cursor": null }` (UI: empty state).

Hasil service setelah map: `{ items: AuditSummary[], nextCursor: "MjAy…" | null }`.

400 (filter invalid, mis. `limit=500`):
```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "limit tidak valid", "details": { "field": "limit" } },
  "trace_id": "4f1c2b7e9a0d4e6f"
}
```

401:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f1c2b7e9a0d4e70" }
```

403 (developer):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f1c2b7e9a0d4e71" }
```

### `GET /api/audit-log/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f`

200:
```json
{
  "id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f",
  "occurred_at": "2026-10-19T08:14:22.123456Z",
  "actor_type": "api_key",
  "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
  "actor_label": "claude-code-budi (gk_ab12)",
  "project_id": "0192f0c4-0000-7000-8000-000000000001",
  "project_label": "Payments",
  "action": "mcp.read_file",
  "target_type": "file",
  "target": "docs/auth/login.md",
  "status": "ok",
  "args": { "path": "docs/auth/login.md", "api_key": "[REDACTED]" },
  "result": { "bytes": 4812, "content": "[OMITTED]" },
  "ip": "203.0.113.5",
  "user_agent": "claude-code/2.1",
  "trace_id": "9c2e41aa07b34f15"
}
```

Entri AC2 (detail):
```json
{
  "id": "0192f0c4-7c10-7000-8000-00000000aaaa",
  "occurred_at": "2026-10-19T09:00:00Z",
  "actor_type": "anonymous",
  "actor_id": null,
  "actor_label": "gk_zz99",
  "project_id": null,
  "project_label": null,
  "action": "mcp.auth.rejected",
  "target_type": null,
  "target": null,
  "status": "denied",
  "args": {},
  "result": { "reason": "invalid_key" },
  "ip": "198.51.100.7",
  "user_agent": null,
  "trace_id": "1a2b3c4d5e6f7a8b"
}
```

Entri dengan argumen terpotong total (> 8 KB):
```json
{ "args": { "_truncated": true, "_original_bytes": 15234, "_keys": ["path", "query"] } }
```

404 (id bukan uuid, tidak ada, atau sudah dipurge):
```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "4f1c2b7e9a0d4e72" }
```

405 (mis. `DELETE /api/audit-log/{id}`, tidak pernah dipanggil UI):
```json
{ "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null }, "trace_id": "4f1c2b7e9a0d4e73" }
```

### Error sisi klien (`ApiError` dari `http.ts`, tanpa respons server yang valid)

```json
{ "status": 0, "code": "NETWORK", "message": "Tidak dapat terhubung ke server", "details": null }
```
```json
{ "status": 200, "code": "INVALID_RESPONSE", "message": "Respons server tidak sesuai format", "details": [{ "path": ["items", 0, "actor_type"], "message": "Invalid enum value" }], "traceId": "req-7f3a" }
```
```json
{ "status": 502, "code": "INTERNAL", "message": "Terjadi kesalahan pada server", "details": null, "traceId": "req-7f3b" }
```
