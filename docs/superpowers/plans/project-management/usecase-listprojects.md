# T03 project/management — Usecase Plan: `listprojects`

Tiket: `docs/tickets/T03-project-management.md`. Plan bersama (keputusan mengikat, `base/`, `base/store`, `base/counts`, migrasi, OpenAPI): `docs/superpowers/plans/project-management/overview.md`. Dokumen ini hanya memuat usecase `listprojects` (U4, gelombang B). Bila bertentangan dengan overview, overview menang.
Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk arsitektur). Tidak ada `CLAUDE.md` maupun `.claude/rules/`.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack (backend Go + slice frontend Svelte untuk beranda daftar project; detail FE di `frontend-plan.md`) |
| Domain | project |
| Module | management |
| Feature | `listprojects` |
| Resource root | `backend/internal/domain/project/internal/management` |
| Endpoint | `/api/projects` |
| Method | GET |
| Version | tanpa prefix versi di URL (prefix `/api`); `api/modules/project/management.yml` `info.version: 0.1.0` |
| Akses | `RequireAuth`; developer hanya project yang ia ikuti, super_admin semua. Tidak ada 403 karena role (otorisasi = isi filter) |
| Audit | tidak ada (baca saja, tidak ada di katalog event) |
| Bus disediakan | tidak ada |
| Bus dikonsumsi | `project.knowledge.count-files`, `project.apispec.count-specs`, `project.apikey.count-keys` (kontrak `contentcount`, lewat `base/counts`, fail-soft). Tidak memakai `core.auth.get-user` (D6) maupun `core.audit.record` |
| AC | AC1 (developer hanya melihat miliknya), AC7 (super_admin melihat semua) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/project/internal/management/listprojects/`
- `backend/internal/domain/project/internal/management/base/projectfilter.go` dan `projectfilter_test.go` (file baru milik usecase ini; tidak mengubah file `base/` lain)
- `backend/internal/domain/project/internal/management/base.go` (tepat satu baris: `listprojects.Init(d)`)
- `backend/internal/domain/project/internal/management/management_integration_test.go` (hanya menambah test AC1/AC7 bagian daftar, bila file sudah ada; kalau belum, lewati dan cukup test di folder feature)
- `api/modules/project/management.yml`, `api/openapi.yml` (hanya bila BE-0 menemukan selisih)
- `frontend/src/domain/project/management/listprojects/`
- `frontend/src/app/` (route beranda dan menu, hanya bila belum ada)
- `frontend/src/shared/i18n/{id,en}.json` (kunci `project.list.*` bila belum ada dari F)

## Konteks yang dimuat

- Sumber: overview T03 (bagian 3.3 actor, 3.8 paginasi, 3.9 counts, 4.3 level, 4.7 DTO, 5.4 contentcount, 5.6 `base/counts`, 6 migrasi, 12 body), `docs/tickets/README.md`, `T03-project-management.md`, plan `core-auth/usecase-listusers.md` (pola paginasi `COUNT(*) OVER()` yang dipakai ulang).
- Infra T00 di `main` (nama final): `app.Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` (`internal/infra/app`), `respond.WriteJSON/WriteError` (`internal/infra/http/respond`), `middleware.RequireAuth()` (`internal/infra/middleware`), `db.Querier` (`internal/infra/db`), `errs` dan `reqctx` (`lib/common`).
- Prasyarat dari F (HARUS sudah ada; bila belum, berhenti dan laporkan, jangan membuatnya di folder feature):
  - Migrasi `projects` dan `project_members` beserta indeks `projects_updated_idx` dan `project_members_user_idx`.
  - `base/`: `Project`, `ProjectDTO`, `ProjectSummary`, `ContentCounts`, `NewSummary`, `Actor`, `ActorFrom`, `ResolveLevel`, `Level*`, `LikePattern`, `FieldErrors`, `InvalidFields`.
  - `base/counts` (`Counter`, `New`) dan kontrak `lib/contracts/project/management/contentcount` beserta tiga konstanta endpoint.
  - OpenAPI `GET /api/projects` (`operationId: listProjects`) dan schema `ProjectList` di `management.yml`.
- Catatan overview: `base.Project` tidak punya tag `db`; repository feature memakai row struct privat sendiri.

## Summary Plan

`listprojects` melayani `GET /api/projects?q=&limit=&offset=`. Usecase membangun aktor, memvalidasi filter lewat entity, meminta satu halaman project ke repository, meminta jumlah file/spec/key untuk id satu halaman itu lewat `counts.Counter` (tiga Call batch, fail-soft), lalu membentuk `{items, total, limit, offset}`.

Otorisasi di endpoint ini adalah isi filter, bukan penolakan:

| Aktor | Query |
|---|---|
| developer | `JOIN project_members m ON m.project_id = p.id AND m.user_id = <aktor>`: hanya project yang ia ikuti. Project lain tidak pernah muncul, tidak ada 403/404 (rule 7.3) |
| super_admin | `LEFT JOIN` keanggotaan aktor: semua project; `is_member` dari ada-tidaknya baris keanggotaan |

Pembagian tanggung jawab (satu usecase tetap satu folder, tanpa pemecahan buatan):

| Lapisan | Isi |
|---|---|
| `base/projectfilter.go` (rule murni) | `ProjectFilter`, `NewProjectFilter(...)`: default limit 20, batas 1..100, offset >= 0, panjang dan normalisasi `q`. Rule bisnis, jadi di entity |
| `listprojects/params.go` | Bentuk query string saja (`strconv.Atoi`). Tanpa rule nilai |
| `listprojects/usecase.go` | Orkestrasi: actor -> filter -> repo -> level per item -> counts -> DTO |
| `listprojects/repository.go` | Satu-satunya SQL feature ini |
| `listprojects/handler.go` | Parse, panggil usecase, `respond.WriteJSON`, header `Cache-Control: no-store` |

Keputusan di plan ini:

1. Tanpa migrasi dan tanpa indeks baru. `projects_updated_idx` melayani urutan super_admin; `project_members_user_idx` melayani JOIN developer. ILIKE `%q%` tidak memakai btree dan skala puluhan-ratusan project (overview 3.8).
2. `member_count` lewat subquery berkorelasi `(SELECT count(*) FROM project_members pm WHERE pm.project_id = p.id)` pada baris halaman saja di SELECT list (dievaluasi per baris keluaran). Dengan limit maks 100 ini murah dan PK `(project_id, user_id)` melayani.
3. `total` memakai `COUNT(*) OVER()`. Bila baris kosong dan `offset > 0`, repository menjalankan `SELECT count(*)` dengan fragmen JOIN dan WHERE yang sama (fungsi privat `build(f, actor)` dipakai kedua query agar tidak menyimpang).
4. Counts diminta SETELAH query selesai dan hanya untuk id halaman (<= 100 <= `contentcount.MaxProjectIDs`). Tepat tiga `bus.Call` per request, berapa pun jumlah item (tanpa N+1). Halaman kosong -> nol Call (`ForProjects` dengan ids kosong tidak memanggil bus).
5. Tidak ada `get-user` (D6) dan tidak ada audit. Usecase tidak butuh `Clock`.
6. Validasi dua tahap seperti `listusers`: bentuk angka oleh `params.go`, nilai oleh entity. Keduanya format `details.fields`. Bila bentuk salah, validasi nilai tidak dijalankan.
7. Parameter kosong (`q=`, `limit=`, `offset=`) sama dengan tidak dikirim. `q` hanya spasi dianggap tidak ada. Parameter tak dikenal diabaikan.
8. Route dipasang per feature dengan pola penuh (D4): `d.API.With(middleware.RequireAuth()).Get("/projects", h.ServeHTTP)`. Tidak memakai `Route("/projects", ...)` agar tidak bentrok dengan `createproject` (POST path sama) dan route T04-T06.
9. Role aktor selain `super_admin` dan `developer` (mustahil karena hanya dua role; defensif) -> `base.ErrForbidden`, tanpa query.
10. Respons memuat `access_level` dari `base.ResolveLevel(actor.Role, myRole, true)`. Karena JOIN developer dan CHECK role, level developer selalu `owner|member`; super_admin selalu `admin` (walau kebetulan ada baris keanggotaan, `my_role` tetap dilaporkan apa adanya dan `is_member=true`).
11. Id aktor yang bukan uuid (mustahil dari sesi) diperlakukan sebagai uuid nol di repository (`uuid.Nil`), sehingga developer mendapat daftar kosong dan super_admin mendapat semua, tanpa error cast.

## Plan

Urutan wajib. Kontrak respons (bagian Request/Response) dikunci dulu agar BE dan FE bisa paralel. Tulis test dulu (TDD) per langkah.

### Backend

#### BE-0. Verifikasi OpenAPI (sebelum kode, rule 11.6)

Pastikan `GET /api/projects` di `api/modules/project/management.yml` (`operationId: listProjects`, tag `projects`, security `sessionCookie`) memuat persis:

| Parameter | Aturan |
|---|---|
| `q` | query, string, `maxLength: 100`, opsional |
| `limit` | query, integer, `minimum: 1`, `maximum: 100`, `default: 20` |
| `offset` | query, integer, `minimum: 0`, `default: 0` |

Respons: 200 `ProjectList {items: ProjectSummary[], total, limit, offset}` (semua `required`; `ContentCounts` tiap field `type: [integer, "null"]`; `my_role` `type: [string, "null"]` enum `owner|member|null`; `access_level` enum `owner|member|admin`), 400 `BadRequest`, 401 `Unauthorized`, 500 `Internal`. Deskripsi menyebut "developer: hanya project yang diikuti; super_admin: semua". Jika ada selisih, perbaiki di `management.yml`, jalankan `make api-bundle`, pastikan `make api-check` lulus.

#### BE-1. `base/projectfilter.go` (entity rule, tanpa I/O, tanpa context)

```go
const (
    DefaultProjectLimit  = 20
    MaxProjectLimit      = 100
    MaxProjectQueryRunes = 100
    MaxProjectOffset     = 1_000_000
)

// ProjectFilter adalah filter daftar project yang sudah tervalidasi dan dinormalisasi.
type ProjectFilter struct {
    Query  string // sudah di-trim; "" = tanpa pencarian
    Limit  int    // 1..100
    Offset int    // 0..MaxProjectOffset
}

// ProjectFilterInput adalah nilai mentah dari transport. Limit/Offset nil = tidak dikirim.
type ProjectFilterInput struct {
    Query         string
    Limit, Offset *int
}

// NewProjectFilter memvalidasi semua field sekaligus dan mengembalikan FieldErrors gabungan (error).
func NewProjectFilter(in ProjectFilterInput) (ProjectFilter, error)

// LikePattern: "" bila Query kosong; selain itu LikePattern(Query) (escape \ % _).
func (f ProjectFilter) LikePattern() string
```

Aturan dan pesan (kunci = nama parameter):
- `q`: `strings.TrimSpace`; > 100 rune -> `"maksimal 100 karakter"`; `unicode.IsControl` -> `"mengandung karakter tidak valid"`.
- `limit`: nil -> 20; di luar 1..100 -> `"limit harus 1-100"`.
- `offset`: nil -> 0; `< 0` -> `"offset minimal 0"`; `> 1_000_000` -> `"offset terlalu besar"`.
- Error dikembalikan sebagai `FieldErrors`; usecase mengubahnya lewat `base.InvalidFields`.

#### BE-2. `listprojects/params.go` (bentuk query saja)

```go
// parseQuery hanya memeriksa BENTUK angka. Tidak ada rule nilai.
func parseQuery(q url.Values) (base.ProjectFilterInput, base.FieldErrors)
```
- `q`: `q.Get("q")` apa adanya.
- `limit`, `offset`: bila tidak kosong, `strconv.Atoi`; gagal -> `fe[k] = "harus berupa angka bulat"` (pesan tanpa nilai input); sukses -> pointer ke nilai.

#### BE-3. `listprojects/repository.go` (satu-satunya SQL)

```go
type Item struct {
    Project     base.Project
    MyRole      string // "" bila aktor bukan anggota (hanya mungkin untuk super_admin)
    MemberCount int
}

type Repository interface {
    // List mengembalikan satu halaman urut updated_at DESC, id DESC dan total baris cocok filter.
    // restrictToMember=true (developer): JOIN keanggotaan aktor. false (super_admin): LEFT JOIN.
    // Halaman kosong dan Offset>0: total lewat COUNT(*) terpisah dengan WHERE/JOIN yang sama.
    List(ctx context.Context, actorID string, restrictToMember bool, f base.ProjectFilter) (items []Item, total int, err error)
}

type Repo struct{ q db.Querier }
func NewRepo(q db.Querier) *Repo
```

Row privat:

```go
type projectRow struct {
    ID             string    `db:"id"`
    Name           string    `db:"name"`
    Description    string    `db:"description"`
    CreatedBy      *string   `db:"created_by"`
    CreatedByLabel *string   `db:"created_by_label"`
    CreatedAt      time.Time `db:"created_at"`
    UpdatedAt      time.Time `db:"updated_at"`
    MyRole         *string   `db:"my_role"`
    MemberCount    int       `db:"member_count"`
    Total          int       `db:"total"`
}
```

Query dibangun dari fragmen konstan; nilai pengguna HANYA lewat argumen posisional. `$1` = id aktor (`uuid.Nil` bila bukan uuid):

```sql
SELECT p.id::text AS id, p.name, p.description,
       p.created_by::text AS created_by, p.created_by_label,
       p.created_at, p.updated_at,
       m.role AS my_role,
       (SELECT count(*) FROM project_members pm WHERE pm.project_id = p.id) AS member_count,
       COUNT(*) OVER() AS total
FROM projects p
{JOIN | LEFT JOIN} project_members m ON m.project_id = p.id AND m.user_id = $1
WHERE 1=1
  [AND p.name ILIKE $2 ESCAPE '\']
ORDER BY p.updated_at DESC, p.id DESC
LIMIT $n OFFSET $n
```

Catatan implementasi:
- Kata `JOIN` atau `LEFT JOIN` dipilih dari dua konstanta Go (bukan dari input pengguna). Escape `\` ditulis di string raw Go.
- `total` = `rows[0].Total` bila ada baris; kosong dan `Offset > 0` -> `SELECT count(*) FROM projects p {JOIN} ... WHERE ...` (fungsi privat `build` menghasilkan klausa JOIN, WHERE, dan args untuk kedua query); kosong dan `Offset == 0` -> 0.
- Slice hasil tidak pernah nil. Waktu di-`.UTC()`.
- Error dibungkus `fmt.Errorf("listprojects: query projects: %w", err)`. Tanpa transaksi (satu SELECT, READ COMMITTED cukup).
- Kolom `description` disertakan (dipakai kartu beranda).

#### BE-4. `listprojects/usecase.go` (orkestrasi)

```go
type Input = base.ProjectFilterInput

type Output struct {
    Items  []base.ProjectSummary `json:"items"`
    Total  int                   `json:"total"`
    Limit  int                   `json:"limit"`
    Offset int                   `json:"offset"`
}

type Usecase struct {
    repo   Repository
    counts counts.Counter
}
func NewUsecase(repo Repository, c counts.Counter) *Usecase
func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error)
```

Langkah `Execute`:
1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `actor.Role` bukan `resolvesession.RoleDeveloper`/`RoleSuperAdmin` -> `base.ErrForbidden`.
3. `f, err := base.NewProjectFilter(in)`; `FieldErrors` -> `base.InvalidFields(fe)` (400). Repository tidak dipanggil.
4. `items, total, err := u.repo.List(ctx, actor.ID, actor.Role == developer, f)`; error dibungkus `fmt.Errorf("listprojects: list: %w", err)` (500 generik).
5. `ids` dari `items`; `cm := u.counts.ForProjects(ctx, ids)` (tidak pernah error; id tanpa entri -> `ContentCounts{}` semua nil).
6. Per item: `level := base.ResolveLevel(actor.Role, it.MyRole, true)`; `base.NewSummary(it.Project, level, it.MyRole, it.MemberCount, cm[it.Project.ID])` (myRole `""` -> `my_role` null, `is_member` false).
7. `Output{Items, Total: total, Limit: f.Limit, Offset: f.Offset}` (nilai efektif setelah default). `Items` tidak pernah nil (`make([]base.ProjectSummary, 0, n)`).

Tidak ada audit, tidak ada pemanggilan `get-user`, tidak ada `Clock`.

#### BE-5. `listprojects/handler.go`

```go
type Handler struct{ uc *Usecase }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    in, fe := parseQuery(r.URL.Query())
    if len(fe) > 0 { respond.WriteError(w, r, base.InvalidFields(fe)); return }
    out, err := h.uc.Execute(r.Context(), in)
    if err != nil { respond.WriteError(w, r, err); return }
    w.Header().Set("Cache-Control", "no-store")
    respond.WriteJSON(w, http.StatusOK, out)
}
```

#### BE-6. `listprojects/init.go` dan wiring

```go
func Init(d app.Deps) {
    uc := NewUsecase(NewRepo(d.DB), counts.New(d.Bus, d.Log))
    h := &Handler{uc: uc}
    d.API.With(middleware.RequireAuth()).Get("/projects", h.ServeHTTP)
}
```

Tambahkan tepat satu baris `listprojects.Init(d)` di `management/base.go` pada PR ini. `*db.DB` memenuhi `db.Querier`.

#### BE-7. Test

Lihat Test Scenario. Entity dan usecase dulu (TDD), lalu integrasi repository dan HTTP.

### Frontend (slice daftar project / beranda)

Mengikuti `frontend-plan.md` (folder `frontend/src/domain/project/management/listprojects/`). Fitur lain (buat project, workspace) ditambah usecase masing-masing; `index.ts` hanya ditambah ekspor.

- FE-1 `types.ts` dan `services/projects.service.ts`: Zod `projectSummarySchema` (id, name, description, created_by nullable, created_by_label nullable, created_at, updated_at, `my_role` enum `owner|member` nullable, `access_level` enum `owner|member|admin`, `is_member`, `member_count`, `counts {files, specs, keys}` tiap field `number | null`) dan `projectListSchema {items, total, limit, offset}`. `listProjects({q, limit, offset}, {signal})` memanggil `get('/api/projects', {query, schema, signal})`; nilai kosong dibuang dari query. Periksa signature `RequestOptions` di `shared/services/http.ts`; jangan membuat client HTTP baru.
- FE-2 `stores/projectsList.ts`: state `q`, `page`, `pageSize` (20), `data`, `loading`, `error`; `offset=(page-1)*pageSize`; ubah `q` reset `page=1`, debounce 300 ms; fetch baru membatalkan sebelumnya (`AbortController`) dan respons basi diabaikan; halaman kosong dengan `total>0` dan `page>1` lompat ke halaman terakhir sekali; `refresh()` diekspos (dipanggil setelah buat/hapus project).
- FE-3 `components/`: `ProjectSearchBar.svelte`; `ProjectCard`/`ProjectTable.svelte` (nama, deskripsi ringkas, peran saya sebagai teks `Owner`/`Member`, jumlah file dan spec, update terakhir). Hitungan `null` ditampilkan "-" (bukan 0). Super_admin dengan `is_member=false` menampilkan badge "Bukan anggota" (teks, bukan hanya warna). Keadaan: memuat (`Spinner`), kosong tanpa pencarian ("Belum ada project" + tombol "Project baru" bila memungkinkan), kosong dengan pencarian ("Tidak ada project yang cocok" + reset), error (`ErrorState` + coba lagi). Paginasi memakai `shared/components/Pagination`.
- FE-4 `views/ProjectListView.svelte` + `index.ts` (ekspor `ProjectListView`, `refreshProjects`, tipe `ProjectSummary`). Klik kartu menuju `/projects/{id}` lewat callback dari `app/` (feature tidak meng-import router).
- FE-5 i18n `project.list.*` (title, search, empty, emptyFiltered, reset, col.name, col.role, col.files, col.specs, col.updated, notMember, error.load) di `id.json` dan `en.json` dengan paritas kunci.
- APP-1 `app/`: route beranda (`/`) memakai `ProjectListView`; tombol "Project baru" membuka form `createproject` bila sudah ada. Jangan duplikasi bila sudah ditambah usecase lain.

## Migration

Tidak ada. Tabel dan indeks dibuat F (`backend/migrations/project/20261022090000000_create_projects.sql`). Tidak ada indeks `lower(name)`/trigram (overview 3.8).

## Changes

Backend:
- `backend/internal/domain/project/internal/management/base/projectfilter.go` (baru): `ProjectFilter`, `ProjectFilterInput`, `NewProjectFilter`, `LikePattern`, konstanta.
- `backend/internal/domain/project/internal/management/base/projectfilter_test.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/init.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/handler.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/params.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/usecase.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/repository.go` (baru).
- `backend/internal/domain/project/internal/management/listprojects/{params,usecase,handler}_test.go`, `repository_integration_test.go`, `listprojects_integration_test.go` (baru).
- `backend/internal/domain/project/internal/management/base.go` (ubah, satu baris): `listprojects.Init(d)`.
- `backend/internal/domain/project/internal/management/management_integration_test.go` (tambah bila ada): bagian daftar dari `TestT03_AC1_*` dan `TestT03_AC7_*`.
- `api/modules/project/management.yml`, `api/openapi.yml`: hanya bila BE-0 menemukan selisih.

Frontend:
- `frontend/src/domain/project/management/listprojects/{types.ts,services/projects.service.ts,stores/projectsList.ts,components/*,views/ProjectListView.svelte,index.ts}` (baru) beserta `*.test.ts`.
- `frontend/src/app/` route beranda dan menu (ubah bila perlu).
- `frontend/src/shared/i18n/id.json`, `en.json` (ubah).

Tidak berubah: `base/store`, `base/counts`, `base/audit`, `base/users`, konstanta dan kontrak bus, migrasi, `docs/tickets/README.md`, plan usecase lain.

## Test Scenario

Penamaan integrasi mengikuti rule 13.4 (`TestT03_ACn_...`). Integrasi memakai `db.TestDB(t)` dan menerapkan migrasi `migrations.FS`.

### Unit entity (`base/projectfilter_test.go`, tanpa mock)
- `TestNewProjectFilter_Defaults`: input kosong -> `Limit=20`, `Offset=0`, `Query=""`.
- `TestNewProjectFilter_LimitBounds` (tabel): 1 dan 100 lolos; 0, -1, 101, 500 -> `fields.limit = "limit harus 1-100"`.
- `TestNewProjectFilter_OffsetBounds`: 0 lolos; -1 -> `offset minimal 0`; 1_000_001 -> `offset terlalu besar`.
- `TestNewProjectFilter_Query`: `"  pay  "` -> `"pay"`; spasi saja -> `""`; 100 rune multibyte lolos; 101 rune ditolak; `"a\nb"` dan `"a\x00b"` ditolak.
- `TestNewProjectFilter_AggregatesErrors`: `q` 101 rune + `limit=999` + `offset=-1` -> satu `FieldErrors` berisi tiga kunci.
- `TestProjectFilter_LikePattern`: `Query="50%_a\\"` -> `"%50\%\_a\\\\%"`; kosong -> `""`.

### Unit feature (`listprojects/*_test.go`, repository dan counter palsu)
- `TestParseQuery_Shape`: `limit=abc`, `offset=1.5` -> `fe["limit"]`/`fe["offset"]` = `harus berupa angka bulat`; `limit=` kosong -> pointer nil, tanpa error; `limit=20` -> pointer 20; param tak dikenal diabaikan.
- `TestUsecase_InvalidFilterIs400NoRepoNoCounts`: `limit=500` -> `*errs.Error` `CodeInvalid` dengan `details.fields.limit`; repository dan counter tidak dipanggil.
- `TestUsecase_DeveloperUsesRestrictedQuery`: aktor developer -> repo menerima `restrictToMember=true` dan `actor.ID` dari identitas; super_admin -> `false`.
- `TestUsecase_LevelAndMembership`: developer item `my_role=owner` -> `access_level=owner`, `is_member=true`; `member` -> `member`; super_admin tanpa keanggotaan -> `access_level=admin`, `my_role=nil`, `is_member=false`; super_admin dengan baris keanggotaan -> `admin`, `is_member=true`.
- `TestUsecase_CountsBatchedOnce`: counter palsu mencatat panggilan; 20 item -> tepat satu `ForProjects` dengan 20 id (urutan sama dengan item); hasil kosong -> `ForProjects` dipanggil dengan ids kosong dan mengembalikan map kosong tanpa Call bus (perilaku `base/counts`, overview 3.9); assert tidak ada error.
- `TestUsecase_CountsMissingIsNull`: counter mengembalikan map tanpa entri untuk satu id -> `counts.files/specs/keys` JSON `null` untuk id itu; entri `{Files:&3}` -> `files:3`, lainnya `null`.
- `TestUsecase_EmptyItemsIsNotNil`: marshal -> `"items":[]`.
- `TestUsecase_EffectiveLimitOffsetEchoed`: tanpa limit -> `Output.Limit==20`, `Offset==0`.
- `TestUsecase_UnknownRoleForbidden`: role `agent` -> `base.ErrForbidden`, repo tidak dipanggil.
- `TestUsecase_NoIdentityUnauthorized`: konteks tanpa identitas -> `CodeUnauthorized`.
- `TestUsecase_RepoErrorWrapped500`: error repo -> error non-`*errs.Error` berawalan `listprojects:`; `respond.WriteError` -> 500 `INTERNAL` tanpa teks SQL.
- `TestHandler_Headers`: 200 membawa `Cache-Control: no-store` dan `Content-Type: application/json`.

### Integrasi repository (`repository_integration_test.go`, Postgres nyata, seed lewat SQL langsung)
Seed: user A, B, C (uuid tetap, tanpa tabel users); project P1..P5 dengan `updated_at` eksplisit berbeda dan dua project sama `updated_at` (tie-break id); A owner P1 dan P2, A member P3, B owner P4, tidak ada anggota di P5 (tanpa owner); P2 punya 3 anggota; satu project bernama `50%_off`.
- `TestListProjects_DeveloperOnlyOwnMemberships` (AC1): A melihat P1, P2, P3 saja; B hanya P4; C kosong, `total=0`.
- `TestListProjects_SuperAdminSeesAll_LeftJoin` (AC7): aktor non-anggota dengan `restrictToMember=false` melihat P1..P5, `MyRole=""`; aktor yang juga punya baris keanggotaan memiliki `MyRole` terisi.
- `TestListProjects_OrderUpdatedAtDescThenID`: urutan `updated_at DESC, id DESC` stabil, tie-break by id.
- `TestListProjects_MemberCountCorrect`: `MemberCount` P2 = 3, P5 = 0; hitungan tidak terpengaruh filter aktor (semua anggota, bukan hanya aktor).
- `TestListProjects_SearchCaseInsensitive`: `q=PAY` cocok `Payments`.
- `TestListProjects_SearchEscapesWildcards`: `q=%` hanya mengembalikan `50%_off`; `q=_` dan `q=\` literal.
- `TestListProjects_SQLInjectionInQuery`: `q="'; DROP TABLE projects; --"` -> 0 item, tabel masih ada.
- `TestListProjects_SearchDoesNotLeakOtherProjects`: B mencari nama project milik A -> kosong (filter keanggotaan tetap berlaku bersama ILIKE).
- `TestListProjects_Pagination`: limit 2 offset 0/2/4 -> gabungan sama dengan daftar penuh tanpa duplikat atau lompatan; `total` sama di tiap halaman.
- `TestListProjects_OffsetBeyondEnd_TotalStillCorrect`: `offset=100` -> `items=[]`, `total` = jumlah cocok (dengan dan tanpa `q`, untuk developer maupun super_admin).
- `TestListProjects_EmptyDB`: `items=[]`, `total=0`.
- `TestListProjects_NonUUIDActorID`: aktor `"bukan-uuid"` -> developer: kosong tanpa error; super_admin: semua project.
- `TestListProjects_DescriptionAndLabelsMapped`: `created_by` NULL dan `created_by_label` NULL terpetakan ke pointer nil; waktu UTC.

### Integrasi HTTP (`listprojects_integration_test.go`, router chi + middleware identitas uji, bus nyata, SQL seed)
Identitas disuntikkan lewat middleware uji (`reqctx.WithIdentity`); tanpa identitas, `RequireAuth` memberi 401.
- `TestT03_AC1_ListProjects_DeveloperSeesOnlyOwn`: A 200 memuat P1; B tidak memuatnya; `total` benar.
- `TestT03_AC7_ListProjects_SuperAdminSeesAllWithIsMember`: super_admin memuat semua project dengan `access_level=admin`, `is_member=false`, `my_role=null`.
- `TestT03_ListProjects_RequiresAuth`: tanpa sesi -> 401 `UNAUTHORIZED`.
- `TestT03_ListProjects_ValidationErrors`: `limit=500` -> 400 `details.fields.limit`; `limit=abc`; `offset=-1`; `q` 101 karakter -> 400 `details.fields.q`; dua kesalahan sekaligus -> dua kunci.
- `TestT03_Counts_FailSoft_List`: tanpa provider count -> 200, `counts` semua `null`; dengan provider palsu files=3 untuk satu id -> `files:3`, `specs:null`, `keys:null`; id lain tanpa entri di respons provider -> `files:0`.
- `TestT03_Counts_ProviderError_List`: provider specs mengembalikan error -> 200, hanya `specs:null`, files dan keys terisi.
- `TestT03_ListProjects_CountsOneCallPerProvider`: 25 project, halaman 20 -> setiap provider palsu dipanggil tepat sekali dengan 20 id.
- `TestT03_ListProjects_NoAuditEntry`: setelah beberapa GET, handler audit palsu tidak menerima `record.Request`.
- `TestT03_ListProjects_NoStoreHeader`: `Cache-Control: no-store`.
- `TestT03_ListProjects_ReflectsMembershipChangeImmediately`: hapus baris keanggotaan A di SQL -> `GET` berikutnya tidak memuat project itu.
- `TestRoutesRegistered` (milik W) mencakup `GET /projects`; usecase ini memastikan lewat `chi.Walk` pada `Init` bahwa ada tepat satu `GET /projects`.

### Frontend (vitest)
- `projects.service.test.ts`: query hanya memuat nilai terisi; respons valid lolos Zod; `counts.files: null` valid; respons tanpa `total` atau `my_role` di luar enum ditolak; error 400 -> `ApiError` dengan `details.fields`.
- `projectsList.test.ts`: ubah `q` reset halaman ke 1; debounce 300 ms (fake timer, satu fetch); respons lama tidak menimpa respons baru; halaman kosong dengan `total>0` lompat sekali; `refresh()` memuat ulang.
- `ProjectTable.test.ts`: peran tampil sebagai teks; hitungan `null` tampil "-" (bukan 0); `is_member=false` menampilkan badge "Bukan anggota"; kosong tanpa pencarian vs dengan pencarian menampilkan pesan berbeda.
- Paritas i18n `project.list.*` id/en.

### Arsitektur dan kualitas
- `TestT00_AC4_RepoIsClean` (archtest) lulus: `listprojects` tidak meng-import feature lain, hanya `base`, `base/counts`, `lib`, dan infra; tidak ada SQL di luar `repository.go`.
- `go vet`, `golangci-lint`, `make api-check` lulus; `npm run depcruise`, `npm run lint`, `npm test` lulus.

## E2E Scenarios

Playwright terhadap binary; prasyarat: `migrate up`, admin dan developer A, B, C tersedia (T01).

Positif:
1. Developer A login, beranda kosong menampilkan "Belum ada project"; A membuat project "Payments" -> muncul di daftar dengan peran Owner.
2. Developer B (bukan anggota) membuka beranda: project A tidak tampil. A menambahkan B -> setelah muat ulang, B melihat "Payments" dengan peran Member.
3. Pencarian `pay` (debounce) menyempit ke project yang namanya mengandung `pay` tanpa memandang huruf besar kecil; mengosongkan pencarian mengembalikan daftar penuh.
4. Pencarian `%` atau `_` tidak menampilkan semua project (literal).
5. Dengan lebih dari 20 project: halaman 1 berisi 20 kartu, "Berikutnya" membuka halaman 2 dengan sisa; mengubah pencarian kembali ke halaman 1. Urutan: yang terakhir diubah paling atas (ubah nama project di Settings lalu kembali ke beranda: project itu naik ke atas).
6. Super Admin login: beranda menampilkan semua project; yang bukan miliknya diberi badge "Bukan anggota".
7. Hitungan file/spec/key menampilkan "-" selama T04-T06 belum ada, dan angka setelah provider tersedia.

Negatif:
1. Tanpa login: `/` mengarah ke `/login`; `GET /api/projects` manual -> 401.
2. `GET /api/projects?limit=500`, `?limit=abc`, `?offset=-1` manual -> 400 dengan `details.fields` per kolom.
3. Pencarian tanpa hasil menampilkan "Tidak ada project yang cocok" dan tombol reset berfungsi.
4. API gagal (500): `ErrorState` dengan tombol coba lagi; setelah server pulih, klik coba lagi memuat daftar.
5. Pencarian `'; DROP TABLE projects; --` -> daftar kosong, aplikasi tetap normal.
6. Developer C dikeluarkan dari project di tab lain: muat ulang beranda tidak lagi memuat project itu (tanpa error).

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. `GET` tidak butuh header `X-Requested-With`; cookie sesi wajib.

### GET /api/projects

Contoh: `GET /api/projects?q=pay&limit=20&offset=0`

200 developer (header `Cache-Control: no-store`; counts belum tersedia -> `null`):

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

200 super_admin (project yang bukan miliknya, counts tersedia sebagian):

```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000002",
      "name": "Billing",
      "description": "",
      "created_by": "0192f5a0-cccc-7aaa-8bbb-00000000000c",
      "created_by_label": "c@tim.id",
      "created_at": "2026-10-22T08:00:00Z",
      "updated_at": "2026-10-22T08:30:00Z",
      "my_role": null,
      "access_level": "admin",
      "is_member": false,
      "member_count": 0,
      "counts": { "files": 12, "specs": null, "keys": 0 }
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

200 tanpa parameter (default): `limit: 20`, `offset: 0`, urut `updated_at DESC, id DESC`.

200 hasil kosong (developer tanpa project, atau offset melewati data; `total` tetap total yang cocok):

```json
{ "items": [], "total": 3, "limit": 20, "offset": 100 }
```

400 nilai tidak valid (`limit=500&offset=-1`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "limit": "limit harus 1-100", "offset": "offset minimal 0" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

400 bentuk salah (`limit=abc`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "limit": "harus berupa angka bulat" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0B"
}
```

400 `q` terlalu panjang:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "q": "maksimal 100 karakter" } }
  },
  "trace_id": "..."
}
```

401 tanpa sesi atau sesi tidak valid:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

500 (kegagalan DB; tanpa detail internal; test memeriksa kode `INTERNAL` dan pesan tanpa SQL):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

Tidak ada 403 dan tidak ada 404 untuk endpoint ini: project yang bukan milik developer tidak muncul di daftar.

### Bus

Tidak ada endpoint bus yang disediakan. Yang dikonsumsi (lewat `base/counts`, sekali per provider per request, nilai request, fail-soft):

```go
// BUS_PROJECT_KNOWLEDGE_COUNT_FILES, BUS_PROJECT_APISPEC_COUNT_SPECS, BUS_PROJECT_APIKEY_COUNT_KEYS
bus.CallTyped[contentcount.Response](ctx, d.Bus, ep,
    contentcount.Request{ProjectIDs: []string{"0192f5a0-1111-7aaa-8bbb-000000000001"}})
// -> contentcount.Response{Counts: map[string]int{"0192f5a0-1111-7aaa-8bbb-000000000001": 12}}
// bus.ErrNotRegistered -> Debug log, field null; error lain -> Warn log, field null; tidak pernah menggagalkan request.
```

### Catatan keamanan ringkas
- Isolasi project ditegakkan di SQL (JOIN keanggotaan), bukan di lapisan tampilan; pencarian tidak pernah melebar ke project lain.
- Input `q` hanya masuk sebagai argumen terparameter dengan wildcard ter-escape; fragmen SQL (`JOIN`/`LEFT JOIN`) dipilih dari konstanta, bukan dari input.
- Respons tidak di-cache (`no-store`). Log tidak memuat nilai `q` maupun isi respons.
- Hanya field ringkasan yang keluar; tidak ada data user lain selain `created_by` dan label snapshot.
