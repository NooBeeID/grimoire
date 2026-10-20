# T03 project/management — Usecase `getproject` (GET /api/projects/{projectID})

Melengkapi `docs/superpowers/plans/project-management/overview.md` (F: migrasi, `base/`, `base/store`, `base/counts`, `management/base.go`, `management.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `getproject` (U5, gelombang B). Jangan tertukar dengan `getprojectbus` (handler bus `project.management.get-project`, U2): folder, kontrak, dan semantik "tidak ada" berbeda (di sini 404, di sana `Found=false` tanpa error).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack pada tiket; plan usecase ini backend saja. FE (`getproject` view, header workspace) ada di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `getproject` (gelombang B, U5) |
| Endpoint | `/api/projects/{projectID}` |
| Method | GET |
| Version | tanpa prefix versi di URL; `management.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`. Otorisasi project di usecase: `FindAccess` -> `ResolveLevel` -> `Require(PermView)` |
| Siapa | anggota (owner/member), super_admin |
| Bus dikonsumsi | `project.knowledge.count-files`, `project.apispec.count-specs`, `project.apikey.count-keys` (via `base/counts`, fail-soft). Tidak memanggil `core.auth.get-user` (D6) |
| Bus disediakan | tidak ada |
| Audit | tidak ada (aksi baca) |
| Tabel / migrasi | membaca `projects`, `project_members`; tidak ada migrasi |
| AC | AC1 (bukan anggota tidak melihat project), AC2 (member boleh melihat) |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `.../management/getproject/`, `.../management/base.go` (satu baris `getproject.Init(d)`) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): `base.Project`, `base.Actor`/`ActorFrom`, `base.ResolveLevel`, `base.Require`/`PermView`, `base.ErrProjectNotFound`, `base.NewSummary`, `base.ProjectDetail`/`ProjectSummary`/`ContentCounts`, `base/store.Store.FindAccess` (id bukan uuid -> `found=false` tanpa query), `base/counts.Counter`, `management.Register`, path `GET /api/projects/{projectID}` + schema `ProjectDetail` di `api/modules/project/management.yml`, migrasi `20261022090000000_create_projects.sql`.

## Summary Plan

`getproject` mengembalikan detail satu project untuk anggota atau Super Admin. Alur usecase (urutan wajib overview 3.4):

1. Bangun aktor dari `reqctx.IdentityFrom(ctx)`; gagal -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `p, memberRole, found, err := repo.FindAccess(ctx, projectID, actor.ID)` (satu query LEFT JOIN).
3. `!found` (tidak ada, atau `projectID` bukan uuid) -> `base.ErrProjectNotFound` (404).
4. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermView)`. `none` -> 404 yang sama (tidak bocor). Tidak ada jalur 403 untuk `PermView`.
5. `repo.MemberStats(ctx, p.ID)` -> `member_count`, `owner_count` (satu query agregat).
6. `counts.ForProjects(ctx, []string{p.ID})` -> `ContentCounts` (fail-soft; tidak pernah error; `null` bila provider belum ada atau gagal).
7. Susun `base.ProjectDetail{ProjectSummary: base.NewSummary(p, level, memberRole, memberCount, c), OwnerCount: ownerCount}`; jawab 200.

Keputusan:
- Bukan anggota, project tidak ada, dan id bukan uuid menghasilkan respons byte-identik (kode, pesan, `details: null`). Klien tidak bisa membedakan (rule 7.3, AC1). Tidak ada log yang membedakan di level Info; boleh `Debug` dengan `reason` untuk diagnosis.
- Handler tidak memvalidasi uuid; hanya `chi.URLParam(r, "projectID")` diteruskan apa adanya (overview 3.3). Tidak ada `params.go` (hanya path param tunggal; query dan body diabaikan).
- `my_role`: `memberRole` bila tidak kosong, selain itu `null`. `is_member = memberRole != ""`. Super Admin bukan anggota: `my_role=null`, `is_member=false`, `access_level="admin"`. Itu semua dihitung `base.NewSummary`, bukan di handler/usecase.
- Developer nonaktif tidak sampai ke sini: middleware Session sudah menolaknya (D6). Karena itu `ResolveLevel(..., userActive=true)` tanpa panggilan `get-user`.
- `MemberStats` memakai satu query `COUNT(*)` dan `COUNT(*) FILTER (WHERE role='owner')`. Tidak memakai `FOR UPDATE` dan tidak di transaksi. Race ringan dengan hapus project (project terhapus di antara query 1 dan 5) menghasilkan hitungan 0 tetapi 200; dapat diterima untuk aksi baca.
- Hitungan isi satu Call per provider (3 Call berurutan, batch 1 id). Tidak pernah menggagalkan request (D1, overview 3.9).
- Error infra dibungkus `fmt.Errorf("getproject: <langkah>: %w", err)` dan dijawab 500 generik oleh `respond.WriteError`.
- Header `Cache-Control: no-store` dipasang pada 200 karena data bergantung pada keanggotaan saat itu. Kecil, tidak mengubah kontrak.
- Usecase hanya orkestrasi; rule akses ada di `base`, SQL ada di `base/store` dan repository feature.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Pastikan simbol F di atas ada. Baca signature `respond.WriteJSON(w, status, v)`, `respond.WriteError(w, r, err)`, `middleware.RequireAuth()`, `middleware.KeyByIP` dari kode T00 yang sebenarnya, dan pola `Init` feature lain yang sudah ada (mis. `createproject`/`checkaccess`). Ikuti persis. Pastikan `d.API` adalah `chi.Router` dan route didaftarkan dengan pola penuh (D4).

**B1. `getproject/repository.go`** (satu-satunya yang menyentuh SQL; `FindAccess` didelegasikan ke `store`)

```go
package getproject

type Repo struct {
    s *store.Store
    q db.Querier
}

func newRepo(q db.Querier, s *store.Store) *Repo { return &Repo{s: s, q: q} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
    return r.s.FindAccess(ctx, projectID, userID)
}

// MemberStats: jumlah anggota dan owner project. Project tanpa anggota -> (0, 0, nil).
func (r *Repo) MemberStats(ctx context.Context, projectID string) (members, owners int, err error) {
    const q = `SELECT COUNT(*), COUNT(*) FILTER (WHERE role = 'owner')
               FROM project_members WHERE project_id = $1`
    var row struct {
        Members int `db:"members"`
        Owners  int `db:"owners"`
    }
    if err := sqlx.GetContext(ctx, r.q, &row, q, projectID); err != nil {
        return 0, 0, err
    }
    return row.Members, row.Owners, nil
}
```

Alias kolom `AS members` / `AS owners` ditambahkan di query. `projectID` yang dikirim ke `MemberStats` selalu `p.ID` dari hasil `FindAccess` (uuid valid). Sesuaikan `db.Querier` dengan T00 (mis. `GetContext`).

**B2. `getproject/usecase.go`**

```go
type repository interface {
    FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error)
    MemberStats(ctx context.Context, projectID string) (members, owners int, err error)
}

type Usecase struct {
    repo   repository
    counts counts.Counter
}

func NewUsecase(repo repository, c counts.Counter) *Usecase { return &Usecase{repo: repo, counts: c} }

func (u *Usecase) Execute(ctx context.Context, projectID string) (base.ProjectDetail, error) {
    actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
    if !ok {
        return base.ProjectDetail{}, errs.New(errs.CodeUnauthorized, "perlu login")
    }
    p, memberRole, found, err := u.repo.FindAccess(ctx, projectID, actor.ID)
    if err != nil {
        return base.ProjectDetail{}, fmt.Errorf("getproject: find access: %w", err)
    }
    if !found {
        return base.ProjectDetail{}, base.ErrProjectNotFound
    }
    level := base.ResolveLevel(actor.Role, memberRole, true)
    if err := base.Require(level, base.PermView); err != nil {
        return base.ProjectDetail{}, err
    }
    members, owners, err := u.repo.MemberStats(ctx, p.ID)
    if err != nil {
        return base.ProjectDetail{}, fmt.Errorf("getproject: member stats: %w", err)
    }
    c := u.counts.ForProjects(ctx, []string{p.ID})[p.ID]
    return base.ProjectDetail{
        ProjectSummary: base.NewSummary(p, level, memberRole, members, c),
        OwnerCount:     owners,
    }, nil
}
```

`c` bernilai nol (`ContentCounts{nil,nil,nil}`) bila map tidak memuat id: semua `null`. Sesuaikan penyesuaian signature `reqctx.IdentityFrom` dengan T00 (mengembalikan `(Identity, bool)`).

**B3. `getproject/handler.go`**

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    d, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
    if err != nil {
        respond.WriteError(w, r, err)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respond.WriteJSON(w, http.StatusOK, d)
}
```

Tidak ada `RequestMeta` (tanpa audit), tidak ada decode body.

**B4. `getproject/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), counts.New(d.Bus, d.Log))
    d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}", newHandler(uc).ServeHTTP)
}
```

Nama param WAJIB `{projectID}` (D4). Hanya GET didaftarkan di sini; PATCH dan DELETE pada pola yang sama didaftarkan `updateproject` dan `deleteproject` lewat pemanggilan terpisah.

**B5. Wiring.** Tambah satu baris `getproject.Init(d)` di `management/base.go` dalam `Register` (satu baris, tanpa mengubah baris lain).

**B6. OpenAPI.** Seharusnya lengkap dari F. Verifikasi `api/modules/project/management.yml`: `GET /api/projects/{projectID}`, `operationId: getProject`, tag `projects`, parameter `$ref ProjectID` (uuid), respons 200 `ProjectDetail`, 401 `Unauthorized`, 404 `NotFound` (deskripsi: "juga untuk project yang bukan milik pemanggil atau id tidak valid; tidak bocor"), 500 `Internal`. Tidak ada `X-Requested-With` (GET). Bila ada perubahan: `make api-bundle` lalu `make api-check`.

**B7. Test** (bagian Test Scenario), lalu jalankan `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/internal/management/getproject/...`, archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. `getproject` hanya membaca `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (milik F).

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/management/getproject/init.go`
- `backend/internal/domain/project/internal/management/getproject/handler.go`
- `backend/internal/domain/project/internal/management/getproject/usecase.go`
- `backend/internal/domain/project/internal/management/getproject/repository.go`
- `backend/internal/domain/project/internal/management/getproject/usecase_test.go`
- `backend/internal/domain/project/internal/management/getproject/handler_test.go`
- `backend/internal/domain/project/internal/management/getproject/integration_test.go` (tag `integration`)

Backend (ubah, minimal):
- `backend/internal/domain/project/internal/management/base.go`: satu baris `getproject.Init(d)`.

API:
- `api/modules/project/management.yml`: verifikasi/lengkapi `getProject`; regenerasi `api/openapi.yml` hanya bila berubah.

Tidak dibuat: `params.go`, migrasi, kontrak bus, konstanta baru, entri audit, perubahan i18n (dimiliki F/frontend-plan).

Folder `getproject/` hanya meng-import `base`, `base/store`, `base/counts`, `lib/common/{errs,reqctx}`, `infra/{app,db}`, `infra/http/respond`, `infra/middleware`, `chi`, `sqlx`. Tidak meng-import feature lain (rule 4.3) dan tidak memanggil bus langsung (lewat `base/counts`).

## Test Scenario

Penamaan AC: `TestT03_ACn_<Deskripsi>`; lainnya `Test<Fungsi>_<Kasus>`.

### Unit usecase (repository dan counter palsu lewat interface)

Identitas disuntikkan dengan `reqctx.WithIdentity`.
- `TestUsecase_Execute_OwnerSees`: aktor developer, `memberRole=owner` -> `AccessLevel=owner`, `MyRole="owner"`, `IsMember=true`, `MemberCount`/`OwnerCount` dari repo.
- `TestUsecase_Execute_MemberSees`: `memberRole=member` -> `AccessLevel=member`, `MyRole="member"` (AC2).
- `TestUsecase_Execute_SuperAdminNotMember`: aktor super_admin, `memberRole=""` -> `AccessLevel=admin`, `MyRole=nil`, `IsMember=false`.
- `TestUsecase_Execute_NonMemberIs404`: developer, `memberRole=""`, `found=true` -> `base.ErrProjectNotFound`; `MemberStats` dan `counts` TIDAK dipanggil (pencatat panggilan) (AC1).
- `TestUsecase_Execute_NotFound`: repo `found=false` -> `base.ErrProjectNotFound` (pointer sama); sama persis dengan kasus non-anggota.
- `TestUsecase_Execute_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, repo tidak dipanggil.
- `TestUsecase_Execute_FindAccessError`: error repo dibungkus (`errors.Is` ke asal), bukan `*errs.Error`.
- `TestUsecase_Execute_MemberStatsError`: error dibungkus; counts tidak dipanggil.
- `TestUsecase_Execute_CountsFailSoft`: counter mengembalikan map kosong -> `Counts` semua `nil`, tanpa error; counter memberi `{id: {Files:&3}}` -> `files=3`, lainnya `nil`.
- `TestUsecase_Execute_CountsCalledOnceWithProjectID`: counter dipanggil tepat sekali dengan `[]string{p.ID}`.
- `TestUsecase_Execute_ZeroOwners`: `owners=0` (project tanpa owner) -> `OwnerCount=0` tanpa error.

### Unit handler (`httptest`, usecase palsu atau repo palsu, router chi nyata)

- `TestHandler_Get_OK`: 200, `Content-Type: application/json`, `Cache-Control: no-store`, body memuat semua field detail termasuk `owner_count`; `counts.files` bertipe `null` bila nil.
- `TestHandler_Get_PassesRawParam`: route `/projects/{projectID}` meneruskan `"bukan-uuid"` apa adanya ke usecase (tanpa validasi di handler).
- `TestHandler_Get_NotFound404Body`: error `ErrProjectNotFound` -> 404, body `{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan","details":null},"trace_id":...}`.
- `TestHandler_Get_InternalIs500Generic`: error infra -> 500 tanpa membocorkan pesan error asal.
- `TestHandler_Get_IgnoresQueryAndBody`: `?x=1` dan body sembarang tidak memengaruhi hasil.

### Integrasi (tag `integration`, `db.TestDB(t)`, bus nyata dengan handler palsu)

- `TestRepo_MemberStats`: 0 anggota -> (0,0); 1 owner + 2 member -> (3,1); 2 owner -> (2,2); project lain tidak ikut terhitung.
- `TestRepo_FindAccess_Delegates`: anggota -> `memberRole` benar; bukan anggota -> `""`, `found=true`.
- `TestT03_AC1_GetProject_NonMemberGets404`: A membuat project (INSERT langsung); B (developer, bukan anggota) `GET /projects/{id}` -> 404; body identik dengan `GET /projects/<uuid acak>` dan `GET /projects/bukan-uuid` (bandingkan kode, pesan, details; `trace_id` diabaikan).
- `TestT03_AC2_GetProject_MemberSees`: B ditambahkan sebagai `member` -> 200, `my_role="member"`, `access_level="member"`, `member_count=2`, `owner_count=1`.
- `TestT03_AC7_GetProject_SuperAdminSeesAny`: S (bukan anggota) -> 200, `access_level="admin"`, `my_role=null`, `is_member=false`.
- `TestT03_GetProject_NoOwnerProject`: project tanpa anggota diakses S -> 200, `member_count=0`, `owner_count=0`.
- `TestT03_GetProject_CountsFailSoft`: tanpa provider count terdaftar -> 200, `counts` semua `null`; provider palsu files=3 saja -> `files:3`, `specs:null`, `keys:null`; satu provider mengembalikan error -> 200 (hanya field itu `null`).
- `TestT03_GetProject_NoAudit`: setelah request sukses maupun 404, handler palsu `core.audit.record` tidak menerima panggilan.
- `TestT03_Security_GetProject_Isolation` (rule 13.6): developer C (anggota project lain saja) -> 404 untuk project A; tidak ada kebocoran lewat kode/pesan/timing struktur body.
- `TestT03_GetProject_Unauthenticated`: tanpa sesi -> 401 dari `RequireAuth` (tidak sampai usecase).

## E2E Scenarios

Butuh T01 dan T02 merge (Playwright terhadap binary).

Positif:
1. Developer A membuat project "Payments", lalu membuka `/projects/<id>` -> workspace tampil dengan nama, deskripsi, badge peran "owner", jumlah anggota 1; hitungan file/spec/key tampil "-" bila T04-T06 belum ada.
2. A menambahkan B sebagai member; B membuka project dari beranda -> detail tampil, badge "member", tombol kelola anggota/hapus tidak tampil (otorisasi tetap di backend).
3. Super Admin membuka project yang bukan miliknya lewat beranda -> detail tampil dengan penanda "bukan anggota" (`access_level=admin`, `is_member=false`).
4. Setelah T04-T06 merge: header workspace menampilkan jumlah file/spec/key sesuai data.

Negatif:
1. Developer C membuka URL `/projects/<id PA>` langsung -> halaman "project tidak ditemukan" (API 404), tanpa membedakan dari id yang memang tidak ada.
2. Membuka `/projects/bukan-uuid` -> halaman "tidak ditemukan" yang sama (API 404).
3. Tanpa login `GET /api/projects/<id>` -> 401 dan UI mengarahkan ke login.
4. Developer dinonaktifkan Super Admin saat sesi berjalan -> request berikutnya ditolak middleware (401), bukan menampilkan project.
5. Provider hitungan gagal -> detail tetap tampil, hitungan "-" (tidak ada error page).

## Request/Response body

Request: tanpa body, tanpa query. Header `Cookie` sesi. Tanpa `X-Requested-With` (GET).

```
GET /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001
```

### 200 (developer A, owner; provider hitungan belum ada)

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

### 200 (developer B, member; sebagian provider tersedia)

```json
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
  "owner_count": 1,
  "counts": { "files": 12, "specs": 3, "keys": 0 }
}
```

### 200 (Super Admin S, bukan anggota, project tanpa owner)

```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "name": "Payments",
  "description": "",
  "created_by": null,
  "created_by_label": null,
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T09:00:00Z",
  "my_role": null,
  "access_level": "admin",
  "is_member": false,
  "member_count": 0,
  "owner_count": 0,
  "counts": { "files": 0, "specs": 0, "keys": 0 }
}
```

### 404 (bukan anggota, project tidak ada, atau `projectID` bukan uuid; ketiganya identik)

```json
{
  "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

### 401 (tanpa sesi, dari middleware)

```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

### 500 (error infra, pesan generik)

```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

Catatan: teks pesan 401/500 mengikuti `respond`/middleware T00 yang sebenarnya; test hanya mengunci `code` dan status. Waktu selalu UTC RFC3339. Tidak ada entri audit untuk endpoint ini.
