# T03 project/management — Usecase `createproject` (U3, gelombang A)

Melengkapi `docs/superpowers/plans/project-management/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `createproject`. Plan usecase lain tidak disentuh.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 (AC1, AC7) |
| Scope | fullstack pada tiket; usecase ini backend + kontrak OpenAPI. UI "Project baru" ada di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `createproject` |
| Endpoint | `/api/projects` |
| Method | POST |
| Version | tanpa prefix versi di URL (prefix `/api`); `management.yml` `info.version: 0.1.0` |
| Siapa | developer, super_admin (dicek di usecase; middleware hanya `RequireAuth()`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Respons sukses | 201 `ProjectSummary` |
| Audit | `project.created` (Call fail-open, setelah commit) |
| Bus dikonsumsi | `core.audit.record` saja (lewat `base/audit`). Tidak memakai `core.auth.get-user` (overview D6) dan tidak memakai `contentcount` (project baru pasti kosong) |
| Tabel / migrasi | menulis `projects`, `project_members`; migrasi dibuat di F overview, tidak ada migrasi baru di sini |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `.../management/createproject/`, `.../management/base.go` (satu baris `createproject.Init(d)`) |

Prasyarat (dari F overview; bila belum ada, hentikan dan laporkan, jangan dibuat di luar allowedPaths): migrasi `20261022090000000_create_projects.sql`; `base` (`Project`, `NewProject`, `Member`, `NewMember`, `RoleOwner`, `Actor`, `ActorFrom`, `RequestMeta`, `Clock`, `ResolveLevel`, `NewSummary`, `ProjectSummary`, `ContentCounts`, `ErrForbidden`, `FieldErrors`, `InvalidFields`); `base/audit` (`Recorder`, `Event`, `New`); konstanta `BUS_CORE_AUDIT_RECORD` dan `record.ActionProjectCreated` (T02). Path `POST /projects` dan schema `CreateProjectRequest`/`ProjectSummary` sudah ada di `api/modules/project/management.yml` dari F; usecase ini hanya memverifikasi.

## Summary Plan

`createproject` membuat project baru dalam satu transaksi.

Aturan:
- Validasi nama (2-100 rune setelah trim, tanpa karakter kontrol) dan deskripsi (<=1000 rune, default `""`) terjadi di entity `base.NewProject`, bukan di handler/usecase. Kedua error digabung dalam satu 400 `details.fields`.
- Handler hanya memvalidasi bentuk (JSON valid, field dikenal, tipe string). Field `name` tidak ada -> dianggap `""` lalu ditolak entity (`fields.name`).
- Satu transaksi: `INSERT projects` (`created_by` = id aktor, `created_by_label` = email aktor sebagai snapshot) lalu, bila aktor `developer`, `INSERT project_members` role `owner`, `added_by` = aktor, `added_by_label` = email aktor.
- `super_admin` TIDAK dijadikan anggota (overview D3). Project lahir tanpa owner; respons `my_role: null`, `access_level: "admin"`, `is_member: false`, `member_count: 0`.
- Role selain `developer`/`super_admin` (mustahil dari sesi normal) -> `base.ErrForbidden` (403) tanpa menyentuh DB.
- Level respons dihitung dengan `base.ResolveLevel(actor.Role, memberRole, true)` agar konsisten dengan rule akses; bukan di-hardcode.
- `counts` respons selalu `{files:0, specs:0, keys:0}` tanpa bus Call. `member_count` 1 (developer) atau 0 (super_admin).
- Audit dikirim SETELAH commit lewat `audit.Recorder` (fail-open, D5): kegagalan audit tidak menggagalkan request. Tepat satu entri per project yang berhasil dibuat; gagal validasi/rollback -> 0 entri. `Args` hanya `{"name": <nama>}`; deskripsi tidak dikirim. Tidak me-log body.
- Tidak ada panggilan bus di dalam transaksi.
- Tidak ada idempotensi/unik nama (nama boleh kembar; project dikenali lewat id).
- Error infra dibungkus `fmt.Errorf("createproject: <langkah>: %w", err)` dan menjadi 500 generik. Rollback otomatis bila fn mengembalikan error (`db.BeginTx`).

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol prasyarat ada. Baca `app.Deps`, `db.DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `respond.DecodeJSON/WriteJSON/WriteError`, `middleware.RequireAuth/KeyByIP`, dan pola `Init` feature yang sudah ada di module lain (mis. `core/audit` bila sudah merge) lalu ikuti. Pastikan `base/store` punya `WithTx(*sqlx.Tx)`. Bila `base/store` belum punya fungsi insert (memang tidak, SQL tulis khusus feature ini ada di repository feature, lihat B3).

**B1. `createproject/params.go`**

```go
type CreateRequest struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}
```
Hanya bentuk. Tidak ada validasi isi di sini. `respond.DecodeJSON` menolak field tak dikenal dan body kosong/rusak (400).

**B2. `createproject/usecase.go`**

```go
type Tx interface {
    InsertProject(ctx context.Context, p base.Project) (base.Project, error) // RETURNING id, created_at, updated_at
    InsertMember(ctx context.Context, m base.Member) error
}
type Repository interface {
    InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
    repo  Repository
    audit audit.Recorder
    clock base.Clock
}
func NewUsecase(r Repository, a audit.Recorder, c base.Clock) *Usecase

func (u *Usecase) Execute(ctx context.Context, in CreateRequest, meta base.RequestMeta) (base.ProjectSummary, error)
```

Alur `Execute` (orkestrasi saja):
1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `actor.Role` bukan `resolvesession.RoleDeveloper`/`RoleSuperAdmin` -> `base.ErrForbidden`.
3. `now := u.clock()`; `p, err := base.NewProject(in.Name, in.Description, actor, now)`; err (`*errs.Error` dari `InvalidFields`) dikembalikan apa adanya.
4. `memberRole := ""`; `memberCount := 0`.
5. `u.repo.InTx(ctx, func(tx Tx) error { ... })`:
   - `created, err := tx.InsertProject(ctx, p)`; simpan ke variabel luar `p = created`.
   - bila `actor.Role == developer`: `m, err := base.NewMember(p.ID, actor.ID, base.RoleOwner, actor, now)`; `tx.InsertMember(ctx, m)`; set `memberRole = base.RoleOwner`, `memberCount = 1`.
   - error dibungkus `fmt.Errorf("createproject: insert project: %w", err)` / `"insert owner: %w"`.
6. `level := base.ResolveLevel(actor.Role, memberRole, true)`.
7. `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectCreated, ProjectID: p.ID, ProjectLabel: p.Name, TargetType: "project", Target: p.ID, Args: map[string]any{"name": p.Name}, Meta: meta})`. Tidak mengembalikan error.
8. `return base.NewSummary(p, level, memberRole, memberCount, base.ContentCounts{Files: ptr(0), Specs: ptr(0), Keys: ptr(0)}), nil`.

Tidak memecah usecase menjadi service lain; seluruh orkestrasi di satu `Execute`. Tidak ada rule bisnis di sini (validasi di `NewProject`, role/level di `base`).

**B3. `createproject/repository.go`** (satu-satunya yang menyentuh SQL untuk feature ini)

```go
type Repo struct {
    db *db.DB
    q  db.Querier // *sqlx.DB di luar tx, *sqlx.Tx di dalam tx
}
func newRepo(d *db.DB) *Repo
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error { return fn(&Repo{db: r.db, q: tx}) })
}
```
SQL:
```sql
INSERT INTO projects (name, description, created_by, created_by_label, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id, created_at, updated_at;

INSERT INTO project_members (project_id, user_id, role, added_by, added_by_label, added_at)
VALUES ($1, $2, $3, $4, $5, $6);
```
`created_by`/`added_by` diisi `uuid` aktor (string -> kolom `uuid`); `created_by_label` dari `Project.CreatedByLabel`. Row struct privat bertag `db`; waktu hasil `RETURNING` di-`.UTC()`. Nama parameter pakai placeholder posisional; tidak ada string concat. Tidak ada `ON CONFLICT` (PK baru, uuid dari DB).

**B4. `createproject/handler.go`**

```go
type Handler struct{ uc *Usecase }
func newHandler(uc *Usecase) *Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var in CreateRequest
    if err := respond.DecodeJSON(r, &in); err != nil { respond.WriteError(w, r, err); return }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    res, err := h.uc.Execute(r.Context(), in, meta)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusCreated, res)
}
```
`truncate` helper lokal kecil (potong rune ke 512); bila sudah ada di `base` pakai itu, jangan buat dua salinan di feature.

**B5. `createproject/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(
        newRepo(d.DB),
        audit.New(d.Bus, d.Log),
        base.Clock(func() time.Time { return time.Now().UTC() }),
    )
    d.API.With(middleware.RequireAuth()).Post("/projects", newHandler(uc).ServeHTTP)
}
```
Route pola penuh langsung di `d.API` (D4), tanpa `Route("/projects", ...)`.

**B6. Wiring**: tambah satu baris `createproject.Init(d)` di `management/base.go` (`Register`). Jangan menyentuh baris Init feature lain.

**B7. OpenAPI**: verifikasi operasi `createProject` di `api/modules/project/management.yml` (request `CreateProjectRequest`, respons 201 `ProjectSummary`, error `BadRequest/Unauthorized/Forbidden/Internal` via `../infra/foundation.yml`, parameter `XRequestedWith`). Bila ada selisih dengan bagian "Request/Response body" di bawah, perbaiki di sana, jalankan `make api-bundle`, pastikan `make api-check` lulus.

**B8. Verifikasi akhir**: `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/...`, archtest `TestT00_AC4_RepoIsClean` (tidak ada import lintas module/feature; `createproject` hanya import `base`, `base/audit`, infra, lib).

## Migration

Tidak ada di usecase ini. Memakai tabel `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (dibuat di F overview bagian 6). Usecase ini bergantung pada CHECK `projects_name_len`, `projects_description_len`, `project_members_role_check` sebagai jaring pengaman (bukan sumber pesan error).

## Changes

Baru:
- `backend/internal/domain/project/internal/management/createproject/init.go`
- `.../createproject/handler.go`
- `.../createproject/params.go`
- `.../createproject/usecase.go`
- `.../createproject/repository.go`
- `.../createproject/usecase_test.go` (unit, repo/audit palsu)
- `.../createproject/handler_test.go` (httptest, usecase dengan repo palsu)
- `.../createproject/repository_integration_test.go` (tag `integration`, `db.TestDB(t)`)

Ubah:
- `backend/internal/domain/project/internal/management/base.go`: tambah `createproject.Init(d)`.
- `api/modules/project/management.yml` hanya bila verifikasi B7 menemukan selisih (lalu `api/openapi.yml` regenerasi).

Tidak diubah: `lib/constants/bus.go`, `lib/contracts/**`, migrasi, `internal/infra/**`, `go.mod`, usecase lain.

## Test Scenario

Penamaan: `TestT03_AC<n>_...` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

Unit usecase (`usecase_test.go`, repo palsu pencatat urutan panggilan, audit palsu pencatat event, clock tetap):
- developer: `InsertProject` lalu `InsertMember` dipanggil dalam satu `InTx`; member berisi `Role=owner`, `UserID=actor.ID`, `AddedBy=actor.ID`, `AddedByLabel=email`; summary `my_role="owner"`, `access_level="owner"`, `is_member=true`, `member_count=1`, counts `0/0/0` (bukan nil).
- super_admin: `InsertMember` TIDAK dipanggil; summary `my_role=nil`, `access_level="admin"`, `is_member=false`, `member_count=0`.
- `CreatedBy`/`CreatedByLabel` pada project yang dikirim ke repo = id dan email aktor (kedua role).
- Nama `"A"`, spasi saja, `"a\nb"`, 101 rune: error `*errs.Error` `INVALID_ARGUMENT` dengan `fields.name`; repo tidak dipanggil; audit 0.
- Deskripsi 1001 rune + nama invalid sekaligus: kedua key ada di `fields`.
- Nama `"  Payments  "` disimpan `"Payments"`; nama 100 rune multibyte lulus; deskripsi kosong lulus.
- Aktor tanpa identitas di ctx: `errs.CodeUnauthorized` (`UNAUTHORIZED`); role `"agent"`: `ErrForbidden`; repo tidak dipanggil.
- Error `InsertProject` -> error dibungkus, bukan `*errs.Error`; audit 0. Error `InsertMember` -> idem (rollback ditegaskan di integrasi).
- Audit sukses: tepat 1 event `Action=project.created`, `ProjectID=p.ID`, `ProjectLabel=nama`, `TargetType="project"`, `Target=p.ID`, `Args=={"name": nama}` (tanpa deskripsi), `Meta` diteruskan; dipanggil setelah `InTx` selesai (urutan pencatatan).
- Audit gagal (recorder palsu tidak bisa melempar karena interface tanpa error; verifikasi dengan `audit.BusRecorder` + bus yang handlernya error, lihat integrasi).

Handler (`handler_test.go`, httptest):
- 201 + JSON tepat sesuai contoh (kunci snake_case, `my_role` null untuk super_admin, waktu RFC3339 UTC).
- Body `{"nama":"x"}` -> 400 (field tak dikenal); body kosong / JSON rusak -> 400; `name` bertipe angka -> 400.
- Error validasi -> 400 dengan `error.details.fields.name`; `ErrForbidden` -> 403; error biasa -> 500 generik tanpa bocor pesan internal; `trace_id` ada di semua error.
- `UserAgent` > 512 rune dipotong di `RequestMeta`; `IP` dari `KeyByIP`.

Repository integrasi (`repository_integration_test.go`, Postgres nyata):
- `InTx` sukses: baris `projects` dan `project_members` ada; `id` dibangkitkan DB; `created_at == updated_at`; `created_by_label` tersimpan.
- Atomik: `InsertMember` dipaksa gagal (role `"admin"` melanggar CHECK) -> `projects` tetap kosong (rollback). Idem fn mengembalikan error setelah `InsertProject`.
- CHECK `projects_name_len` menolak nama 1 karakter bila entity dilewati (jaring pengaman).
- `created_by` bertipe uuid: id aktor valid tersimpan; tidak ada FK ke `users`.

Integrasi lintas feature (ditambahkan ke `management_integration_test.go` oleh W, tag `integration`; usecase ini menyumbang kasus berikut dan memastikan lulus dengan 12 Init atau minimal Init yang ada):
- `TestT03_AC1_CreatorIsOwnerAndInvisibleToOthers` bagian create: A `POST /api/projects` -> 201, `my_role=owner`; `project_members` berisi A sebagai owner. (Bagian `GET` oleh B dibuktikan U4/U5.)
- `TestT03_AC7_SuperAdminCreate`: S `POST /api/projects` -> 201, `member_count=0`, tidak ada baris `project_members`; S bukan anggota.
- `TestT03_Audit_Created`: tepat satu entri `project.created` dengan `actor_label` email, `project_label` nama, `args.name`; audit handler yang error -> request tetap 201 (fail-open) dan error ter-log tanpa `args`; request gagal validasi -> 0 entri.
- CSRF: `POST` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED` (middleware T00); tanpa sesi -> 401.
- Kebocoran: respons tidak memuat field di luar `ProjectSummary` (snapshot JSON).
- `TestRoutesRegistered` (W) memuat `POST /api/projects`.

## E2E Scenarios

Positif (Playwright terhadap binary; butuh T01 dan T02 merge, UI dari frontend-plan):
1. Developer A login -> beranda kosong -> klik "Project baru" -> isi nama "Payments" dan deskripsi -> submit -> masuk workspace project; tab Members memuat A sebagai owner; beranda menampilkan "Payments" dengan peran owner.
2. Nama dengan spasi di tepi ("  Payments  ") tersimpan "Payments".
3. Super Admin login -> "Project baru" -> project terbuat; tab Members kosong (tanpa anggota), beranda menampilkan penanda "bukan anggota"; Super Admin lalu menunjuk owner lewat tambah anggota.
4. Audit log (Super Admin) memuat `project.created` dengan nama project dan email pembuat.
5. Developer B tidak melihat project A di beranda; `GET /api/projects/<id>` oleh B -> 404 (AC1).

Negatif:
1. Nama 1 karakter -> pesan error pada field `name` (i18n), project tidak terbuat, tidak ada entri audit.
2. Nama kosong / hanya spasi -> pesan field `name`; deskripsi 1001 karakter -> pesan field `description`; keduanya sekaligus -> dua pesan.
3. Nama mengandung baris baru (lewat API langsung) -> 400 `fields.name`.
4. `POST /api/projects` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`.
5. Tanpa login -> 401.
6. Body dengan field tak dikenal (`{"nama": "x"}`) -> 400.
7. Submit ganda cepat (double click) menghasilkan dua project terpisah (tidak idempotent); UI menonaktifkan tombol saat request berjalan sehingga hanya satu.

## Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama project wajib 2-100 karakter" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie sesi.

Request:
```json
{ "name": "Payments", "description": "Layanan pembayaran internal" }
```
`description` opsional (default `""`).

201, aktor developer A (owner):
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

201, aktor super_admin S (bukan anggota, D3):
```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000002",
  "name": "Payments",
  "description": "",
  "created_by": "0192f5a0-ffff-7aaa-8bbb-00000000000f",
  "created_by_label": "admin@tim.id",
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T09:00:00Z",
  "my_role": null,
  "access_level": "admin",
  "is_member": false,
  "member_count": 0,
  "counts": { "files": 0, "specs": 0, "keys": 0 }
}
```

400, validasi nama dan deskripsi:
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

400, karakter kontrol pada nama:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama project tidak boleh mengandung karakter kontrol" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, field tak dikenal (`{"nama": "x"}`) atau JSON rusak: format error 400 dari `respond.DecodeJSON` (`code: INVALID_ARGUMENT`).

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (header CSRF hilang, dari middleware T00):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
Pesan dan kode persis mengikuti middleware T00 di `main`; test mengecek `code`, bukan teks pesan.

403 (role tak dikenal, praktis tidak tercapai):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500: `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "..." }` (pesan generik; detail hanya di log).

### Audit yang dikirim (`core.audit.record`, Go, bukan wire JSON)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "a@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectCreated, TargetType: "project", Target: "0192f5a0-1111-7aaa-8bbb-000000000001",
    Args: map[string]any{"name": "Payments"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```
