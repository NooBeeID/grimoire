# T02 core/audit-log — Usecase Plan: `listentries`

Tiket: `docs/tickets/T02-core-audit-log.md`. Plan bersama (foundation, entity `base/`, kontrak, migrasi, OpenAPI, i18n): `docs/superpowers/plans/core-audit-log/overview.md`. Dokumen ini hanya memuat usecase `listentries`.
Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk soal arsitektur).

## Metadata

| Field | Nilai |
|---|---|
| Scope | fullstack (Go backend + Svelte frontend) |
| Domain | core |
| Module | audit-log (folder Go `auditlog`, URL/UI `audit-log`) |
| Feature | `listentries` (urutan: runLate; butuh `getentry/index.ts` untuk panel detail) |
| Resource root | `backend/internal/domain/core/internal/auditlog` |
| Endpoint | `/api/audit-log` |
| Method | GET |
| Version | v1 (prefix `/api`, tanpa versi di path) |
| Akses | super_admin saja (`middleware.RequireRole(resolvesession.RoleSuperAdmin)` dipasang di grup oleh `auditlog/base.go`, bukan di feature) |
| Bus | tidak menyediakan dan tidak mengonsumsi endpoint bus |
| Ref OpenAPI | `api/modules/core/audit-log.yml` (ditulis di foundation F5; usecase ini tidak mengubahnya, hanya memverifikasi kecocokan) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/core/internal/auditlog/listentries/`
- `frontend/src/domain/core/audit-log/listentries/`
- `frontend/src/app/routes/index.ts`
- `frontend/src/app/views/AuditLogView.svelte`
- `frontend/src/app/layouts/AppLayout.svelte`

## Konteks yang dimuat

- Tidak ada `CLAUDE.md` dan `.claude/rules/`. Sumber: `rules.txt`, `docs/tickets/README.md`, `docs/tickets/T02-core-audit-log.md`, overview T02.
- Kode foundation sudah ada di `main` (commit `45a7ad5`). Nama yang terverifikasi di kode: `grimoire/internal/infra/app` (`app.Deps`), `grimoire/internal/infra/http/respond` (`WriteJSON`, `WriteError`), `grimoire/internal/infra/middleware` (`RequireRole`), `grimoire/internal/infra/db` (`db.Querier`, `db.TestDB`, `db.Migrate`/`db.Up`), `grimoire/lib/common/errs`, `grimoire/lib/common/reqctx` (`WithIdentity`, `Identity{UserID,Email,Name,Role}`). Tugas bilang `httpx`; nama final di `main` adalah `respond`, pakai itu.
- FE: `shared/services/http.ts` (`get`, `ApiError`, `RequestOptions.query` hanya satu nilai per kunci, lihat Plan FE-2), `shared/components` (`Table`, `Button`, `Input`, `Spinner`, `EmptyState`, `ErrorState`), `shared/stores/i18n` (`t`, `locale`), `shared/stores/session`, router `app/routes/router.ts` (`current`, `navigate`, `RouteMeta.roles`), guard `app/routes/guard.ts` (role salah -> `notfound`).
- Ketergantungan pada foundation T02 (harus sudah ada sebelum usecase ini dikerjakan): `auditlog/base/{entry,filter,cursor}.go` (`Entry`, `Summary`, `Filter`, `Filter.Validate`, `EscapeLike`, `Cursor`, `EncodeCursor`, `DecodeCursor`, `Entry.Cursor`, `Entry.ToSummary`), migrasi `audit_log` + 4 indeks, kunci i18n `auditLog.*`, `api/modules/core/audit-log.yml`. Bila ada yang belum ada, berhenti dan laporkan; jangan membuatnya di folder feature.

## Summary Plan

`listentries` melayani `GET /api/audit-log`: parse query string menjadi `base.Filter`, validasi lewat `Filter.Validate` (rule ada di entity), ambil `limit+1` baris dari repository dengan predikat dinamis tetapi selalu parameterized dan keyset `(occurred_at, id)`, potong ke `limit`, bentuk `next_cursor` dari item terakhir halaman bila ada sisa, lalu petakan ke `Summary` (tanpa args/result/ip/user_agent). Respons `{items, next_cursor}`; `next_cursor` `null` di halaman terakhir. Tidak ada tulis ke DB, tidak ada audit untuk melihat audit.

Satu usecase tetap satu folder: `params -> handler -> usecase -> repository`, tanpa pemecahan buatan. Rule yang bisa dipakai feature lain (validasi filter, cursor, escape LIKE, ToSummary) sudah di `base/`; yang ada di folder ini hanya transport (parse query), orkestrasi (limit+1, potong, next_cursor), dan SQL.

Frontend: modul `listentries` berisi service (Zod), store filter yang tersinkron ke URL beserta cursor stack halaman yang sudah dimuat, komponen `FilterBar` (preset waktu, aksi multi dari katalog) dan `AuditTable`, serta view tipis. View di feature tidak meng-import router/app (rule 12.4); URL dibaca dan ditulis lewat props. Komposisi lintas feature (tabel + panel detail dari `getentry`, route, menu) dikerjakan di `app/`.

Keputusan desain yang diambil di plan ini:

1. Halaman memakai mode tambah ("Muat berikutnya" menambah baris di bawah), sesuai e2e dan kunci i18n `auditLog.loadMore`. Cursor stack menyimpan cursor tiap halaman yang sudah dimuat sehingga kegagalan satu halaman bisa dicoba ulang tanpa kehilangan halaman sebelumnya, dan reset filter cukup mengosongkan stack.
2. Filter disimpan di URL sebagai `from`/`to` absolut (RFC3339), bukan nama preset; preset hanya cara mengisi `from` saat diterapkan. Reload mempertahankan rentang yang sama persis.
3. `actor_id` hanya dikirim bersama `actor_type` oleh UI (indeks `audit_log_actor_idx` berawalan `actor_type`). Lihat Risiko di bawah.
4. Tanpa endpoint baru untuk autocomplete (rule 1.5); saran aktor/project diambil dari baris yang sudah dimuat.

## Plan

Urutan wajib. BE-n lalu FE-n lalu APP-n. Backend dan frontend feature bisa paralel setelah kontrak respons (bagian Request/Response) dikunci; APP-n terakhir karena butuh `getentry/index.ts`.

### Backend (`backend/internal/domain/core/internal/auditlog/listentries/`)

#### BE-1. `params.go` (parse query, tanpa rule bisnis)

```go
// ParseFilter mengubah query string menjadi base.Filter. Hanya memeriksa BENTUK
// (angka, RFC3339, cursor terdekode). Aturan nilai (enum, uuid, rentang, batas limit)
// ada di base.Filter.Validate.
func ParseFilter(q url.Values) (base.Filter, error)
```

Aturan parse:
- `actor_type`, `actor_id`, `actor_label`, `project_id`, `target`: `strings.TrimSpace(q.Get(k))`.
- `action` dan `status`: `q["action"]`, `q["status"]` (parameter berulang). Juga terima bentuk koma? Tidak: hanya berulang (sesuai OpenAPI `explode: true`). Nilai kosong (`action=`) tetap diteruskan sehingga ditolak `Validate` (`ValidAction("")` false).
- `from`, `to`: kosong -> zero `time.Time`; terisi -> `time.Parse(time.RFC3339Nano, v)` lalu `.UTC()`; gagal -> `invalidField("from")` / `invalidField("to")`.
- `limit`: kosong -> 0 (Validate mengubahnya jadi 50); terisi -> `strconv.Atoi`; gagal -> `invalidField("limit")`. Nilai negatif/di luar 1..200 ditolak `Validate`.
- `cursor`: kosong -> nil; terisi -> `base.DecodeCursor(v)`; error dikembalikan apa adanya (sudah `ErrInvalid`, field `cursor`).
- Parameter tak dikenal diabaikan (FE menambahkan `entry=` di URL halaman, bukan di API, tetapi tetap aman).
- `invalidField(name)` lokal di params.go: `&errs.Error{Code: errs.CodeInvalid, Message: name + " tidak valid", Details: map[string]string{"field": name}, Err: errs.ErrInvalid}`. Pesan tidak memuat nilai input.

#### BE-2. `repository.go` (satu-satunya yang menyentuh SQL)

```go
type Repository interface {
    // List mengembalikan paling banyak limit baris (kolom summary saja), urut
    // occurred_at DESC, id DESC, setelah f.Cursor bila ada.
    List(ctx context.Context, f base.Filter, limit int) ([]base.Entry, error)
}

type sqlRepository struct{ q db.Querier }
func NewRepository(q db.Querier) Repository
```

`buildQuery(f base.Filter, limit int) (string, []any)` (tidak diekspor, dites langsung):

- Kolom SELECT (tetap, literal): `id::text, occurred_at, actor_type, actor_id::text, actor_label, project_id::text, project_label, action, target_type, target, status`. Tidak pernah `args`, `result`, `ip`, `user_agent`, `trace_id` (jsonb besar tidak ikut daftar).
- `FROM audit_log`.
- Predikat dirakit dari fragmen literal tetap; nilai selalu lewat placeholder `$n` (counter lokal `add(v) string` yang menambah ke `args` dan mengembalikan `"$"+strconv.Itoa(len(args))`). Tidak ada concat nilai input ke SQL (rule 5.3):

| Filter | Fragmen |
|---|---|
| `ActorType` | `actor_type = $n` |
| `ActorID` | `actor_id = $n::uuid` |
| `ActorLabel` | `actor_label ILIKE $n ESCAPE '\'` dengan arg `"%" + base.EscapeLike(v) + "%"` |
| `ProjectID` | `project_id = $n::uuid` |
| `Actions` | `action = ANY($n::text[])` (arg `[]string`) |
| `Statuses` | `status = ANY($n::text[])` |
| `From` | `occurred_at >= $n` |
| `To` | `occurred_at < $n` (setengah terbuka; From < To sudah divalidasi) |
| `Target` | `target ILIKE $n ESCAPE '\'` dengan arg `"%" + base.EscapeLike(v) + "%"` |
| `Cursor` | `(occurred_at, id) < ($n::timestamptz, $m::uuid)` |

- Predikat digabung `AND` di `WHERE` hanya bila ada; urutan akhir `ORDER BY occurred_at DESC, id DESC LIMIT $n`.
- `ANY($n::text[])` dengan `[]string` melalui pgx stdlib: verifikasi di tes integrasi. Bila driver menolak, bungkus dengan `pgtype`/`pq.Array` yang sudah dipakai foundation; jangan ganti ke concat `IN (...)`.
- `List` memakai `q.SelectContext` ke slice struct `row` (tag `db:`; kolom nullable `*string`), lalu memetakan ke `base.Entry` (NULL -> `""`, `OccurredAt` -> `.UTC()`). Error DB dibungkus `fmt.Errorf("listentries: query: %w", err)`.
- Tanpa join, tanpa transaksi, tanpa `COUNT(*)` (tidak ada total; paginasi cursor, rule 11.5).

#### BE-3. `usecase.go` (orkestrasi)

```go
type Result struct {
    Items      []base.Summary `json:"items"`
    NextCursor *string        `json:"next_cursor"`
}

type Usecase struct{ repo Repository }          // menerima interface (rule 5.6)
func NewUsecase(r Repository) *Usecase
func (u *Usecase) List(ctx context.Context, f base.Filter) (Result, error)
```

Langkah `List` (f sudah divalidasi, `f.Limit` 1..200):
1. `rows, err := u.repo.List(ctx, f, f.Limit+1)`; error -> `fmt.Errorf("listentries: %w", err)`.
2. Bila `len(rows) > f.Limit`: `rows = rows[:f.Limit]`, `next := base.EncodeCursor(rows[len(rows)-1].Cursor())`, `NextCursor = &next`. Selain itu `NextCursor = nil`.
3. `Items = make([]base.Summary, 0, len(rows))`, isi dengan `row.ToSummary()` (slice non-nil agar JSON `[]`, bukan `null`).
4. Tidak ada log di jalur sukses, tidak ada panggilan bus, tidak ada tulis audit.

#### BE-4. `handler.go`

```go
type Handler struct{ uc interface{ List(context.Context, base.Filter) (Result, error) } }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
    f, err := ParseFilter(r.URL.Query())
    if err == nil { err = f.Validate() }
    if err != nil { respond.WriteError(w, r, err); return }
    res, err := h.uc.List(r.Context(), f)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusOK, res)
}
```

Tanpa rule bisnis, tanpa SQL. Error 400 dipetakan oleh `respond.WriteError` ke `INVALID_ARGUMENT` + `details.field`; error DB menjadi 500 generik (detail hanya di log dengan trace_id, rule 7.12).

#### BE-5. `init.go`

```go
// Init merakit repository -> usecase -> handler dan mendaftarkan GET /.
// RequireRole(super_admin) dipasang di grup /audit-log oleh auditlog/base.go.
func Init(d app.Deps, r chi.Router) {
    h := NewHandler(NewUsecase(NewRepository(d.DB)))
    r.Get("/", h.List)
}
```

Tidak ada `r.Use` di sini: peran dijaga di grup agar getentry ikut terjaga. Tes HTTP (di bawah) membuktikan guard bekerja dengan memasang `RequireRole` seperti `base.go`.

#### BE-6. Tes backend (lihat Test Scenario)

Tulis tes bersama kodenya: `params_test.go`, `usecase_test.go`, `handler_test.go`, `repository_test.go` (build tag `integration`), `explain_integration_test.go` (build tag `integration`).

### Frontend feature (`frontend/src/domain/core/audit-log/listentries/`)

Struktur: `types.ts`, `index.ts`, `services/listentries.service.ts`, `stores/listentries.store.ts`, `components/{AuditTable,FilterBar,ActorIcon,StatusBadge}.svelte`, `views/ListEntriesView.svelte`, tes `*.test.ts` bersebelahan. Semua teks lewat `t()` kunci `auditLog.*` dari foundation F6; tidak ada string hard-coded (rule 12.8). Tidak ada `{@html}` di mana pun: semua nilai (label, target, aksi) dirender sebagai teks (rule 7.9).

#### FE-1. `types.ts`

- `ACTOR_TYPES = ['user','api_key','anonymous','system'] as const`, `AUDIT_STATUSES = ['ok','error','denied'] as const`, tipe turunan.
- `AUDIT_ACTIONS: readonly string[]`: salinan 33 aksi katalog README, dikelompokkan: `mcp.*`, `auth.*`+`user.*`, `project.*`, `apikey.*`, `knowledge.*`+`apispec.*`. Komentar: "Sinkron dengan `lib/contracts/core/audit/record/actions.go`; provider tidak membatasi aksi, jadi aksi baru tambahkan di sini." Tes memastikan 33 nilai unik dan cocok regex aksi.
- `RANGE_PRESETS = [{key:'1h',ms:3_600_000},{key:'24h',...},{key:'7d'},{key:'30d'},{key:'90d'}]` plus `all` (tanpa from/to). Kunci i18n `auditLog.range.<key>`.
- `AuditSummary` (tipe dari schema Zod), `AuditFilter { actorType: string; actorId: string; actorLabel: string; projectId: string; actions: string[]; statuses: string[]; from: string; to: string; target: string }` (string kosong = tidak ada), `MAX_ACTIONS = 20`, `PAGE_SIZE = 50`.

#### FE-2. `services/listentries.service.ts` (tanpa state)

- Schema Zod mengikuti `AuditEntrySummary`/`AuditEntryList` di OpenAPI:
  `AuditSummarySchema = z.object({ id: z.string().uuid(), occurred_at: z.string().datetime({offset:true}), actor_type: z.enum(ACTOR_TYPES), actor_id: z.string().nullable(), actor_label: z.string(), project_id: z.string().nullable(), project_label: z.string().nullable(), action: z.string(), target_type: z.string().nullable(), target: z.string().nullable(), status: z.enum(AUDIT_STATUSES) })`
  `AuditListSchema = z.object({ items: z.array(AuditSummarySchema), next_cursor: z.string().nullable() })`.
- `buildListUrl(filter, cursor?, limit = PAGE_SIZE): string`: `shared/services/http.ts` `RequestOptions.query` hanya satu nilai per kunci, padahal `action` dan `status` berulang. Maka query string dirakit sendiri dengan `URLSearchParams.append` (hanya nilai tidak kosong; `action` dan `status` di-append per nilai), lalu diteruskan sebagai bagian `url` (`buildUrl` di http.ts menerima URL relatif yang sudah berisi `?`). Jangan mengubah `shared/`.
- `listEntries(filter, cursor, signal): Promise<{items: AuditSummary[]; nextCursor: string | null}>` memakai `get(url, { schema: AuditListSchema, signal })`; hasil dipetakan ke camelCase `nextCursor`.
- Error `ApiError` dilempar apa adanya (403/401 ditangani http.ts/router).

#### FE-3. `stores/listentries.store.ts` (state UI, tanpa fetch)

Fungsi murni untuk sinkron URL:
- `filterFromQuery(q: URLSearchParams): AuditFilter`: membaca `actor_type, actor_id, actor_label, project_id, action (getAll), status (getAll), from, to, target`; membuang nilai di luar enum (actor_type/status) dan memotong `action` ke `MAX_ACTIONS`. Mengabaikan `entry`, `cursor`, `limit`.
- `filterToQuery(f: AuditFilter, base?: URLSearchParams): URLSearchParams`: menulis kembali hanya kunci filter yang terisi dan mempertahankan kunci lain di `base` (mis. `entry`). Urutan kunci stabil.
- `filterKey(f): string` kanonik (dipakai view untuk memutuskan reload; perubahan hanya `entry` tidak memicu reload).
- `presetToRange(key, now = new Date()): {from: string; to: string}` (`to` kosong, `from` = now - durasi, ISO UTC tanpa milidetik; `all` -> keduanya kosong).

Store `createListEntriesStore()` (writable):
```ts
interface PageState { cursor: string | null; nextCursor: string | null; items: AuditSummary[] }
interface State {
  pages: PageState[];          // cursor stack: satu elemen per halaman yang sudah dimuat
  status: 'idle' | 'loading' | 'ready' | 'error';
  error: ApiError | null;
}
```
Method: `reset()` (kosongkan stack), `begin()` (status loading), `pushPage(page)` (tambah ke stack, status ready), `fail(err)`, derived `rows` (gabungan `items` semua halaman), `nextCursor` (halaman terakhir), `hasMore`. Tidak ada `fetch`/service di store (rule 12.2). Daftar saran aktor/project diturunkan dari `rows` (distinct `actor_label`, distinct `{project_id, project_label}`), maks 50 entri.

#### FE-4. Komponen (presentasi; tidak memanggil service)

- `ActorIcon.svelte`: SVG inline kecil, ikon "agent" untuk `api_key`/`anonymous`, "orang" untuk `user`, "sistem" untuk `system`; `aria-label` dan `<title>` dari `auditLog.actorType.<type>`; `role="img"`.
- `StatusBadge.svelte`: teks `auditLog.status.<status>` dengan warna token tema (ok/error/denied), tidak mengandalkan warna saja.
- `AuditTable.svelte` (props: `rows`, `loading`, `error`, `selectedId`, `hasMore`, `loadingMore`, `onselect(id)`, `onloadmore()`, `onretry()`): memakai `shared/components/Table.svelte` dengan kolom `auditLog.columns.{time,actor,project,action,target,status}`. Waktu: `new Intl.DateTimeFormat($locale, { dateStyle: 'medium', timeStyle: 'medium' })` dari ISO UTC (zona lokal browser; `title` berisi ISO penuh). Aktor: `ActorIcon` + `actor_label`. Project: `project_label ?? '-'`. Target: teks, dipotong CSS (`truncate`, `title` penuh). Baris dapat dipilih: sel pertama berisi `<button>` (fokusable, Enter/Space) yang memanggil `onselect(id)`; baris terpilih `aria-current="true"`. Empty: `EmptyState` `auditLog.empty`. Error: `ErrorState` + retry. Tombol "Muat berikutnya" (`auditLog.loadMore`) hanya bila `hasMore`.
- `FilterBar.svelte` (props: `value: AuditFilter`, `suggestions`, `onapply(f)`, `onreset()`): form dengan `onsubmit` (apply) dan tombol reset (`auditLog.filter.apply` / `reset`).
  - Jenis aktor: `<select>` dengan opsi "Semua" (`auditLog.filter.any`) + 4 tipe.
  - Aktor: input teks `actor_label` + `<datalist>` dari saran; bila `actorId` terisi (deep link), tampil chip "actor_id: <8 char pertama>" yang bisa dihapus; UI selalu mengirim `actor_type` bersama `actor_id`.
  - Project: input + `<datalist>` label -> `project_id` dari saran; menempel uuid langsung diterima. Nilai terpilih disimpan sebagai `projectId`.
  - Aksi (multi): daftar checkbox dalam `<details>`/popover dengan kotak cari, dikelompokkan; maksimum `MAX_ACTIONS` (checkbox sisanya `disabled` dengan teks bantu); ringkasan "n dipilih".
  - Status: tiga checkbox.
  - Rentang waktu: `<select>`/segmented dengan preset `1h, 24h, 7d, 30d, 90d, all`; memilih preset memanggil `presetToRange` saat Apply. Bila `from` URL tidak cocok preset, tampil "Kustom" (read-only).
  - Teks target: input (maks 200 karakter, `maxlength`).
  - Semua kontrol berlabel (`<label for>`), dapat dioperasikan keyboard; layout responsif (grid 1 kolom di mobile).

#### FE-5. `views/ListEntriesView.svelte` (komposisi tipis)

Props: `{ query: URLSearchParams; selectedId?: string | null; onquerychange: (q: URLSearchParams) => void; onselect: (id: string) => void }`. Tidak meng-import router atau `app/`.

Perilaku:
- `filter = $derived(filterFromQuery(query))`; `$effect` bergantung pada `filterKey(filter)` saja: abort request berjalan (`AbortController`), `store.reset()`, `store.begin()`, `listEntries(filter, null, signal)` -> `pushPage`. Respons usang diabaikan (cek `signal.aborted`).
- `loadMore`: `listEntries(filter, nextCursor, signal)` -> `pushPage`; gagal -> `fail` tanpa membuang halaman sebelumnya (tombol retry memuat cursor yang sama).
- `FilterBar.onapply(f)` -> `onquerychange(filterToQuery(f, query))` (hapus juga `entry`); `onreset` -> query tanpa kunci filter.
- Error `ApiError` 400/500 -> `ErrorState` memakai `auditLog.error.load` + trace id bila ada; 401 ditangani otomatis oleh http.ts.
- Judul `<h1>` `auditLog.title`.

#### FE-6. `index.ts`

Ekspor publik saja: `ListEntriesView`, tipe `AuditSummary`, `AuditFilter`, dan fungsi `filterFromQuery`/`filterToQuery` (agar `app/` bisa mendeteksi filter bila perlu). Tidak mengekspor service/store/komponen internal.

### Komposisi `app/` (dikerjakan terakhir, setelah `getentry/index.ts` ada)

#### APP-1. `frontend/src/app/views/AuditLogView.svelte`

- Meng-import `ListEntriesView` dari `../../domain/core/audit-log/listentries` dan panel detail dari `../../domain/core/audit-log/getentry` (nama ekspor dan props mengikuti plan `getentry`; yang diharapkan `EntryDetailPanel` dengan props `id` dan `onclose`. Baca `getentry/index.ts` lebih dulu dan sesuaikan hanya di file ini).
- `import { current, navigate } from '../routes/router'`. `query = $derived($current.location.query)`; `entryId = $derived(query.get('entry'))`.
- `onquerychange(q)` -> `navigate('/audit-log' + (q.size ? '?' + q : ''), { replace: true })` (perubahan filter tidak menambah entri history).
- `onselect(id)` -> set `entry=id` dan `navigate(...)` dengan push (tombol Back menutup panel). Panel `onclose` -> hapus `entry`, `navigate` replace.
- Layout: tabel di kolom utama; panel detail di sisi kanan pada layar lebar, overlay penuh pada layar sempit. Panel hanya dirender bila `entryId` ada.

#### APP-2. `frontend/src/app/routes/index.ts`

Tambah import `AuditLogView` dan route:
```ts
{ path: '/audit-log', view: AuditLogView, meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'auditLog.title' } },
```
`guard.ts` sudah mengembalikan `notfound` untuk role lain dan redirect login untuk anonim; router mencocokkan path saja sehingga `?entry=` tidak mengganggu. Jangan mengubah route lain (T01 juga menambah route di file ini; konflik merge cukup disimpan keduanya).

#### APP-3. `frontend/src/app/layouts/AppLayout.svelte`

Tambah item menu "Audit Log" (`$t('auditLog.menu')`) berupa `<a href="/audit-log">` di header/nav, hanya bila `$session.identity?.role === 'super_admin'` (UX saja; otorisasi tetap di backend, rule 12.7). `aria-current="page"` bila `$current.location.path` diawali `/audit-log`. Perubahan sekecil mungkin (satu blok `<nav>`), karena T01 juga mengubah layout ini.

## Migration

Tidak ada. Tabel `audit_log` dan keempat indeks dibuat oleh foundation F1 (`backend/migrations/core/20261021090000000_create_audit_log.sql`). Usecase ini hanya membaca. Indeks yang dipakai planner: `audit_log_occurred_idx`, `audit_log_project_idx`, `audit_log_actor_idx`, `audit_log_action_idx`; semuanya sudah berakhiran `occurred_at DESC, id DESC` sehingga cocok untuk keyset.

## Changes

| File / folder | Aksi |
|---|---|
| `backend/internal/domain/core/internal/auditlog/listentries/init.go` | buat |
| `.../listentries/params.go`, `params_test.go` | buat |
| `.../listentries/handler.go`, `handler_test.go` | buat |
| `.../listentries/usecase.go`, `usecase_test.go` | buat |
| `.../listentries/repository.go`, `repository_test.go` (tag `integration`), `explain_integration_test.go` (tag `integration`) | buat |
| `frontend/src/domain/core/audit-log/listentries/types.ts`, `index.ts` | buat |
| `.../listentries/services/listentries.service.ts` + `.test.ts` | buat |
| `.../listentries/stores/listentries.store.ts` + `.test.ts` | buat |
| `.../listentries/components/{AuditTable,FilterBar,ActorIcon,StatusBadge}.svelte` + `components.test.ts` | buat |
| `.../listentries/views/ListEntriesView.svelte` + `ListEntriesView.test.ts` | buat |
| `frontend/src/app/views/AuditLogView.svelte` | buat |
| `frontend/src/app/routes/index.ts` | ubah (route `/audit-log`) |
| `frontend/src/app/layouts/AppLayout.svelte` | ubah (item menu) |
| `frontend/e2e/audit-log.spec.ts` | opsional (Playwright); di luar allowedPaths, buat hanya bila orchestrator mengizinkan, jika tidak lewati |

Tidak diubah: `auditlog/base/*`, `auditlog/base.go` (dipanggil foundation/wiring W1: `listentries.Init(d, r)`), OpenAPI, i18n, `shared/*`, migrasi. Tidak ada env baru, dependensi baru, endpoint bus baru.

## Test Scenario

Penamaan `TestT02_ACn_*` untuk bukti kriteria, selain itu `Test<Unit>_<Kasus>` (rule 13.4). Integrasi memakai `db.TestDB(t)` dan build tag `integration` (rule 13.3); migrasi diterapkan ke schema tes dengan helper migrasi foundation yang sama dengan tes module lain.

### Unit backend (tanpa mock SQL)

`params_test.go` (table-driven, `ParseFilter` + `Validate`):
- query kosong -> Filter zero, `Validate` mengisi `Limit = 50`.
- `action=mcp.read_file&action=mcp.search_knowledge&status=ok&status=denied` -> slice berurutan sesuai input.
- `from`/`to` RFC3339 dengan offset `+07:00` -> tersimpan UTC. `from=kemarin` -> `ErrInvalid`, `details.field == "from"`; `to=2026-10-20` (bukan RFC3339) -> invalid `to`.
- `limit=abc` -> invalid `limit`; `limit=0` -> 50 setelah Validate; `limit=200` valid; `limit=201` dan `limit=-1` -> invalid (via Validate).
- `cursor=rusak!` -> `ErrInvalid` field `cursor`; cursor valid (hasil `EncodeCursor`) -> `Filter.Cursor` terisi dengan nanodetik dan id sama.
- `actor_type=admin`, `actor_id=abc`, `project_id=1`, `action=BAD`, `status=x`, `from` >= `to`, 21 `action`, `target` 201 byte -> semuanya `errors.Is(err, errs.ErrInvalid)` dengan field benar (lewat `Validate`).
- Pesan error tidak memuat nilai input (cek `!strings.Contains(err.Error(), "rusak!")`).
- Parameter asing (`entry=...`) diabaikan.

`usecase_test.go` (repository palsu yang merekam `limit` yang diminta):
- Repo mengembalikan `limit+1` baris -> `Items` tepat `limit`, `NextCursor` = `EncodeCursor` dari baris ke-`limit` (bukan baris ekstra); decode kembali menghasilkan `(OccurredAt, ID)` yang sama.
- Repo mengembalikan `<= limit` baris -> `NextCursor == nil`.
- Tepat `limit` baris -> `NextCursor == nil` (tidak ada sisa).
- Repo kosong -> `Items` non-nil dan panjang 0, JSON `{"items":[],"next_cursor":null}`.
- Limit default: Filter dari `ParseFilter(kosong)`+`Validate` -> repo dipanggil dengan `51`; limit 200 -> 201.
- Mapping `ToSummary`: JSON item tidak punya kunci `args`, `result`, `ip`, `user_agent`, `trace_id`; field nullable kosong menjadi `null`.
- Error repo dibungkus dan dikembalikan (`errors.Is` ke error asli).
- Tidak ada panggilan bus/log (usecase tidak punya dependensi tersebut).

`handler_test.go` (usecase palsu atau repo palsu; `httptest`):
- filter invalid -> 400 `INVALID_ARGUMENT` dengan `error.details.field` dan `trace_id`; usecase tidak dipanggil.
- cursor rusak -> 400, field `cursor`.
- error usecase non-`errs` -> 500 `INTERNAL`, body tidak memuat teks error asli.
- sukses -> 200, `Content-Type: application/json`, bentuk `{items, next_cursor}`.

`buildQuery` (unit, tanpa DB): untuk kombinasi filter, assert (a) SQL hanya mengandung placeholder `$n` dan jumlah `$` sama dengan `len(args)`; (b) nilai input (mis. `"x'; DROP TABLE audit_log; --"` pada `Target`) tidak muncul di string SQL, hanya di `args`; (c) tanpa filter -> tidak ada `WHERE`, berakhir `ORDER BY occurred_at DESC, id DESC LIMIT $1`; (d) `ActorLabel` memakai `ILIKE ... ESCAPE '\'` dan arg ter-escape (`50%_` -> `%50\%\_%`).

### Integrasi repository (Postgres nyata)

`repository_test.go`:
- Urutan: sisip 10 entri dengan `occurred_at` berbeda -> `List` mengembalikan `occurred_at DESC`.
- Keyset stabil dengan `occurred_at` sama: 7 entri `occurred_at` identik dengan id berurutan + beberapa entri lain; telusuri halaman demi halaman (limit 3) lewat usecase+repo nyata sambil memakai `next_cursor`; gabungan semua halaman = urutan lengkap tanpa duplikat dan tanpa loncatan, dan sama dengan query tanpa limit.
- Sisipan baru antar halaman (entri lebih baru dari cursor) tidak menggeser halaman berikutnya (keyset, bukan offset).
- Filter gabungan: project A + aksi `mcp.read_file`/`mcp.search_knowledge` + rentang `[from,to)` -> hanya entri cocok; batas `from` inklusif dan `to` eksklusif diuji dengan entri tepat di batas.
- `actor_id` (+ `actor_type`) -> hanya entri aktor itu.
- `status` multi (`ok`,`denied`) -> `error` tidak ikut.
- `actor_label` contains, case-insensitive (`"BUDI"` cocok `claude-code-budi (gk_ab12)`).
- ILIKE escape: target `"50%_done"`, `"500 done"`, `"50x_done"`; filter `target="50%_"` hanya cocok yang literal `50%_`; `%` saja tidak cocok semua baris; backslash di input tidak merusak query.
- Hanya kolom summary yang dibaca: entri dengan `args` besar tetap cepat dan `Entry.Args == nil`.
- Nilai NULL (`project_id`, `target`, ...) dipetakan ke `""` tanpa panic.
- `ANY($n::text[])` bekerja di driver (aksi 20 nilai).

`TestT02_AC3_SnapshotLabelsSurviveDeletion` (HTTP+repo): entri dengan `project_id` dan `actor_id` uuid acak yang tidak ada di tabel mana pun (module project/api-key tidak ada di tes; membuktikan tidak ada FK) tetap muncul dengan `project_label` dan `actor_label` snapshot, termasuk saat difilter `project_id`.

### Integrasi HTTP (AC5)

`handler_test.go` (tag `integration`, router chi nyata): `r.Route("/audit-log", func(r){ r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin)); listentries.Init(deps, r) })` dengan middleware tes yang menaruh `reqctx.WithIdentity` sesuai header tes:
- `TestT02_AC5_DeveloperForbidden`: role `developer` -> 403 `FORBIDDEN`.
- `TestT02_AC5_AnonymousUnauthorized`: tanpa identitas -> 401 `UNAUTHORIZED`.
- `TestT02_AC5_SuperAdminOK`: `super_admin` -> 200, `items` berurutan, `next_cursor` benar.
- Paginasi end-to-end via HTTP: `limit=2` pada 5 entri -> halaman 2+2+1; halaman terakhir `next_cursor: null`.
- `POST/PUT/PATCH/DELETE /audit-log` -> 405 (bukan rute feature ini; dicek bersama arch test W2).

### AC9 EXPLAIN (1 juta baris)

`explain_integration_test.go`, `TestT02_AC9_ExplainUsesIndex` (tag `integration`, dilewati dengan `-short`):
- Seed satu `INSERT ... SELECT FROM generate_series(1, 1000000)`: `id = gen_random_uuid()`, `occurred_at = now() - (g * interval '4 seconds')`, 20 project (`md5((g % 20)::text)::uuid`), 50 aktor `api_key` (`md5('a' || (g % 50)::text)::uuid`), 8 nilai aksi (`(ARRAY[...])[1 + g % 8]`), `status` bergilir, `args/result` default. Lalu `ANALYZE audit_log`.
- Untuk tiap kasus ambil SQL+args dari `buildQuery` (query yang sama dengan produksi) dan jalankan `EXPLAIN (FORMAT TEXT) <sql>`:
  1. project + aksi + rentang waktu (7 hari) -> rencana memuat `Index Scan`/`Bitmap Index Scan`/`Index Only Scan` dan TIDAK memuat `Seq Scan on audit_log`.
  2. `actor_type=api_key` + `actor_id` -> memakai indeks (`audit_log_actor_idx`), tanpa seq scan.
  3. tanpa filter (halaman pertama, `LIMIT 51`) -> `audit_log_occurred_idx`, tanpa seq scan.
  4. tambahan: halaman lanjut dengan cursor tanpa filter -> indeks, tanpa seq scan.
- Gagal bila string `Seq Scan on audit_log` ada. Bila planner CI menolak indeks karena statistik, perbaiki distribusi seed (jangan menambah `SET enable_seqscan = off`, itu melemahkan bukti AC9).
- Waktu seed 1 juta baris sekitar belasan detik; dijalankan di job integrasi, bukan unit.

### Frontend (Vitest + Testing Library)

- `listentries.service.test.ts`: `buildListUrl` meng-append `action` dan `status` berulang, menghilangkan nilai kosong, meng-encode `cursor` dan `actor_label` berisi `&`/spasi; respons valid lolos Zod; respons dengan `actor_type` tak dikenal atau `occurred_at` bukan datetime -> `ApiError` `INVALID_RESPONSE`; `next_cursor: null` dipetakan ke `nextCursor: null`; 403 -> `ApiError` status 403 (fetch di-mock lewat `vi.stubGlobal('fetch')` seperti `http.test.ts`).
- `listentries.store.test.ts`: `filterFromQuery`/`filterToQuery` round-trip (termasuk `action` berulang dan urutan stabil); kunci `entry` dipertahankan oleh `filterToQuery(f, base)`; nilai enum liar dibuang; `action` >20 dipotong; `presetToRange('24h', fixedNow)` -> `from = fixedNow - 24h`, `all` -> kosong; cursor stack: `pushPage` dua kali -> `rows` gabungan berurutan, `nextCursor` dari halaman terakhir, `reset()` mengosongkan, `fail()` tidak membuang halaman yang sudah dimuat; saran aktor distinct dan terbatas.
- `components.test.ts`: `AuditTable` merender baris sebagai teks (`actor_label = "<img src=x onerror=alert(1)>"` muncul literal, tidak ada elemen `img`), ikon agent vs manusia punya nama aksesibel, klik/Enter pada tombol baris memanggil `onselect(id)`, tombol "Muat berikutnya" hanya saat `hasMore`, empty/error state; `FilterBar` menghasilkan `AuditFilter` yang benar saat apply (preset 7 hari mengisi `from`), reset mengosongkan, batas 20 aksi menonaktifkan checkbox sisanya.
- `ListEntriesView.test.ts` (service di-mock): perubahan `query` -> request baru dan request lama di-abort; perubahan hanya `entry` tidak memicu request ulang; "Muat berikutnya" memanggil service dengan `next_cursor`; gagal muat halaman ke-2 mempertahankan halaman 1 dan menawarkan retry.
- `app/views/app.test.ts` (atau tes baru `AuditLogView.test.ts` bersebelahan, di dalam path yang diizinkan): super_admin melihat item menu dan route render; developer -> guard `notfound` (pakai `resolve()` dari guard.ts); `?entry=<id>` membuka panel `getentry` dan klik tutup menghapus `entry` dari URL.

### Playwright (opsional)

`frontend/e2e/audit-log.spec.ts`: login super_admin, seed entri via bus/SQL helper, terapkan filter project + aksi + preset 7 hari, verifikasi URL query dan hasil, reload mempertahankan filter. Hanya dikerjakan bila infrastruktur e2e foundation mendukung seed; bukan gerbang.

## E2E Scenarios

Positif:
1. Super admin login -> menu "Audit Log" tampil -> `/audit-log` memuat tabel urut terbaru; ikon agent untuk `api_key`/`anonymous`, ikon manusia untuk `user`.
2. Filter project X + aksi `mcp.read_file` + preset 7 hari -> hanya entri cocok; URL menjadi `/audit-log?project_id=...&action=mcp.read_file&from=...`; reload mempertahankan filter; tombol Back/Forward memulihkan filter sebelumnya.
3. "Muat berikutnya" memakai `next_cursor`; dua entri dengan `occurred_at` sama tidak terduplikasi atau terlompati; halaman terakhir -> tombol hilang (`next_cursor: null`).
4. Klik baris -> URL `?entry=<id>` (panel detail dari `getentry`), filter tidak berubah; klik tutup/Back menutup panel tanpa memuat ulang tabel. Entri berargumen `"<img onerror>"` tampil sebagai teks.
5. Deep link `/audit-log?actor_type=api_key&actor_id=<id>` -> terfilter ke key itu; chip aktor tampil dan bisa dihapus.
6. Beberapa aksi dipilih (`mcp.read_file`, `mcp.search_knowledge`) + status `ok`,`denied` -> request memuat parameter berulang; hasil gabungan benar.
7. Pencarian `target` berisi `%` atau `_` diperlakukan literal (tidak mencocokkan semua baris).
8. Project dan key sudah dihapus -> entri lama tetap tampil dengan label snapshot (AC3).
9. Tanpa hasil -> `EmptyState` "belum ada entri"; `{"items":[],"next_cursor":null}`.

Negatif:
1. Developer membuka `/audit-log` (UI) -> halaman tidak ditemukan (guard `meta.roles`); menu tidak tampil; `GET /api/audit-log` -> 403 `FORBIDDEN` (AC5).
2. Tanpa sesi -> `GET /api/audit-log` 401 `UNAUTHORIZED`; UI redirect ke `/login?next=%2Faudit-log...`.
3. `limit=500`, `limit=abc`, `action=BAD`, `status=x`, `from` > `to`, `from=bukan-tanggal`, `actor_id=abc`, `project_id=1`, `cursor=rusak` -> 400 `INVALID_ARGUMENT` dengan `details.field`; UI menampilkan error state tanpa crash. URL halaman berisi nilai tak valid (mis. `?status=x`) dibersihkan oleh `filterFromQuery` sehingga tidak pernah dikirim.
4. Lebih dari 20 aksi -> UI menolak memilih; request manual ke API -> 400 `action`.
5. `POST/PUT/PATCH/DELETE /api/audit-log` -> 405 `METHOD_NOT_ALLOWED` (AC7).
6. DB mati -> 500 `INTERNAL` generik, UI error state dengan trace id dan tombol coba lagi; tidak ada detail SQL bocor.
7. Perpindahan filter cepat -> hanya respons terakhir yang tampil (request lama di-abort, tidak menimpa).

## Request/Response body

### `GET /api/audit-log`

Query (semua opsional): `actor_type` (`user|api_key|anonymous|system`), `actor_id` (uuid), `actor_label` (contains, case-insensitive, maks 200 byte), `project_id` (uuid), `action` (berulang, maks 20, format `a.b[.c]`), `status` (berulang, `ok|error|denied`), `from` (RFC3339, inklusif), `to` (RFC3339, eksklusif), `target` (contains, case-insensitive, maks 200 byte), `cursor` (opaque dari `next_cursor`), `limit` (1..200, default 50).

Contoh:

```
GET /api/audit-log?project_id=0192f0c4-0000-7000-8000-000000000001&action=mcp.read_file&action=mcp.search_knowledge&status=ok&from=2026-10-13T00:00:00Z&to=2026-10-20T00:00:00Z&limit=2
```

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
      "occurred_at": "2026-10-19T08:14:20.000000Z",
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

Halaman berikutnya: ulangi query yang sama ditambah `&cursor=<next_cursor>`. Halaman terakhir:

```json
{ "items": [ { "id": "0192f0c4-7afe-7a00-8c00-010203040506", "occurred_at": "2026-10-19T08:10:00.000000Z", "actor_type": "anonymous", "actor_id": null, "actor_label": "gk_zz99", "project_id": null, "project_label": null, "action": "mcp.auth.rejected", "target_type": null, "target": null, "status": "denied" } ], "next_cursor": null }
```

Tanpa hasil:

```json
{ "items": [], "next_cursor": null }
```

Catatan kontrak: `args`, `result`, `ip`, `user_agent`, `trace_id` TIDAK ada di daftar (hanya di `GET /api/audit-log/{id}`). Urutan selalu `occurred_at DESC, id DESC`. `next_cursor` adalah string opak (base64url dari `"<RFC3339Nano UTC>|<uuid>"`); klien tidak boleh menyusunnya.

400 (filter invalid, contoh `limit=500`):

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "limit tidak valid", "details": { "field": "limit" } },
  "trace_id": "4f1c2b7e9a0d4e6f"
}
```

400 (cursor rusak):

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "cursor tidak valid", "details": { "field": "cursor" } },
  "trace_id": "4f1c2b7e9a0d4e74"
}
```

401 (tanpa sesi):

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f1c2b7e9a0d4e70" }
```

403 (developer):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f1c2b7e9a0d4e71" }
```

500 (kegagalan DB; detail hanya di log):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f1c2b7e9a0d4e75" }
```

Contoh URL halaman FE (bukan API): `/audit-log?project_id=0192f0c4-0000-7000-8000-000000000001&action=mcp.read_file&status=ok&from=2026-10-13T00:00:00Z&entry=0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f`.

## Risiko dan catatan untuk implementer/reviewer

1. `actor_id` tanpa `actor_type`: indeks `audit_log_actor_idx (actor_type, actor_id, occurred_at DESC, id DESC)` tidak efisien bila hanya `actor_id` yang diberikan (tanpa kolom awal), sehingga planner dapat memindai `audit_log_occurred_idx` dan menyaring, lambat untuk aktor langka di tabel besar. Mitigasi di plan ini: UI dan deep link selalu mengirim keduanya; tes AC9 memakai pasangan itu. Rekomendasi ke pemilik foundation (di luar allowedPaths, jangan diubah dari feature ini): tambahkan di `base.Filter.Validate` aturan "`actor_id` mensyaratkan `actor_type`" (field `actor_type`) beserta tesnya, agar API tidak bisa memicu pola lambat. Bila tidak ditambahkan, perilaku tetap benar, hanya berpotensi lambat.
2. `actor_label`/`target` ILIKE contains tidak memakai indeks teks (tanpa `pg_trgm`, sesuai migrasi foundation). Query selalu punya `ORDER BY ... LIMIT`, jadi planner memindai indeks waktu dan menyaring; wajar untuk retensi 90 hari, dan UI mendorong penggunaan rentang waktu. `pg_trgm` adalah tindak lanjut opsional di luar T02.
3. Cursor berisi `occurred_at` dengan presisi mikrodetik dari DB; cursor dibentuk hanya dari baris hasil query sehingga tidak ada selisih presisi dengan nilai kolom.
4. Nama ekspor panel `getentry` belum final saat plan ini ditulis; penyesuaian terbatas pada `AuditLogView.svelte`.
5. `AppLayout.svelte` dan `routes/index.ts` juga disentuh T01; ubah minimal dan selesaikan konflik merge dengan menyimpan kedua sisi.
