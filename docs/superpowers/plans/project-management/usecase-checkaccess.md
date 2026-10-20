# T03 project/management — Usecase `checkaccess`

Mengikuti `docs/superpowers/plans/project-management/overview.md` (overview menang bila bertentangan). Kontrak bus yang dikonsumsi (`core.auth.get-user`) persis dari `docs/superpowers/plans/core-auth/` (T01). Tidak ada kontrak yang didefinisikan ulang di sini kecuali kontrak milik T03 sendiri (`checkaccess`).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T03 |
| Scope | backend (handler bus saja; tanpa HTTP, tanpa frontend) |
| Domain | project |
| Module | management |
| Feature | `checkaccess` (gelombang A, dibutuhkan T04, T05, T06) |
| Endpoint | `project.management.check-access` (`constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`) |
| Method | Bus Call (bukan HTTP) |
| Version | v1 (endpoint baru) |
| Resource root | `backend/internal/domain/project/internal/management` |
| Folder feature | `backend/internal/domain/project/internal/management/checkaccess/` |
| Kontrak | `backend/lib/contracts/project/management/checkaccess` |
| Konsumsi | `core.auth.get-user` (Call, T01), fail-closed |
| Audit | tidak ada |
| AC | AC5 (anggota nonaktif tanpa akses), AC8 (project tak ada tanpa error) |

## 2. Summary Plan

`checkaccess` adalah satu-satunya sumber keputusan "level akses user X pada project Y" bagi module lain (rule 7.2). Usecase hanya orkestrasi:

1. Validasi input (`UserID` uuid, `UserRole` salah satu `super_admin|developer`). Gagal -> error membungkus `errs.ErrInvalid`.
2. Satu query `store.FindAccess` (LEFT JOIN `projects` + `project_members`). Project tak ada atau `ProjectID` bukan uuid -> `{ProjectExists:false, Level:none}`, err nil (AC8).
3. Keputusan level lewat `base.ResolveLevel` (rule bisnis di entity, murni):
   - `super_admin` -> `admin` tanpa memanggil `get-user`.
   - developer bukan anggota -> `none` tanpa `get-user`.
   - developer anggota -> `core.auth.get-user`; `!Found` atau `!IsActive` -> `none` (AC5); selain itu `owner|member`.
4. Error `get-user` (termasuk `bus.ErrNotRegistered`) atau DB -> error terbungkus (fail-closed, tidak pernah mengembalikan akses saat ragu).

Tidak ada cache, tidak ada audit, tidak ada tulis DB. Usecase tidak dipecah (satu alur).

Prasyarat dari foundation (overview bagian F): kontrak `checkaccess`, konstanta bus, migrasi `projects`/`project_members`, `base/` (`ResolveLevel`, `Level`), `base/store` (`FindAccess`), `base/users` (`Lookup`). Plan ini TIDAK membuatnya; bila belum ada di branch, selesaikan langkah F lebih dulu.

## 3. Plan (langkah berurutan)

Asumsi: langkah F overview sudah selesai. Bila ada yang belum ada, buat persis seperti overview bagian 4, 5, 6.

1. **Verifikasi prasyarat** (tanpa mengubahnya):
   - `lib/contracts/project/management/checkaccess/checkaccess.go` memuat `Level`, `LevelNone|Member|Owner|Admin`, `Request{ProjectID, UserID, UserRole}`, `Response{ProjectExists, Level}` (overview 5.2).
   - `lib/constants/bus.go` memuat `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS = "project.management.check-access"`, `BUS_CORE_AUTH_GET_USER`.
   - `base.ResolveLevel(userRole, membershipRole string, userActive bool) Level`, `base/store.Store.FindAccess`, `base/users.Lookup`.
2. **`checkaccess/usecase.go`** (package `checkaccess`):
   ```go
   type AccessFinder interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
   }
   type Usecase struct { access AccessFinder; users users.Lookup }
   func NewUsecase(a AccessFinder, u users.Lookup) *Usecase
   func (u *Usecase) Execute(ctx context.Context, req checkaccess.Request) (checkaccess.Response, error)
   ```
   Alur `Execute`, berurutan:
   1. `validate(req)`:
      - `uuid.Parse(req.UserID)` gagal -> `errs.Wrap(errs.CodeInvalid, "user_id tidak valid", errs.ErrInvalid)`.
      - `req.UserRole` bukan `resolvesession.RoleSuperAdmin` / `RoleDeveloper` -> `errs.Wrap(errs.CodeInvalid, "user_role tidak valid", errs.ErrInvalid)`.
      - `ProjectID` TIDAK divalidasi di sini (bukan uuid = project tak ada, AC8; store sudah menangani tanpa query).
   2. `_, memberRole, found, err := u.access.FindAccess(ctx, req.ProjectID, req.UserID)`; err -> `fmt.Errorf("checkaccess: find access: %w", err)`.
   3. `!found` -> `Response{ProjectExists:false, Level:LevelNone}, nil`.
   4. `req.UserRole == super_admin` -> `Response{true, base.ResolveLevel(role, memberRole, true)}` (= `admin`), tanpa get-user.
   5. `memberRole == ""` -> `Response{true, LevelNone}` tanpa get-user.
   6. Developer anggota: `usr, err := u.users.Get(ctx, req.UserID)`; err -> `fmt.Errorf("checkaccess: get-user: %w", err)` (fail-closed).
   7. `active := usr.Found && usr.IsActive`; `Response{true, base.ResolveLevel(req.UserRole, memberRole, active)}`.
3. **`checkaccess/handler.go`**:
   ```go
   type handler struct{ uc *Usecase }
   func newHandler(uc *Usecase) *handler
   func (h *handler) Handle(ctx context.Context, req any) (any, error) {
       r, err := bus.Expect[checkaccess.Request](req)   // tipe salah -> error membungkus bus.ErrBadRequestType
       if err != nil { return nil, err }
       return h.uc.Execute(ctx, r)                       // Response dikembalikan sebagai nilai
   }
   ```
   Tanpa `params.go` (feature bus, rule 6.9). Gunakan signature handler yang dipakai `bus.Registry.Register` di T00 (periksa `backend/internal/infra/bus` sebelum menulis; sesuaikan bentuk fungsi bila beda dari contoh).
4. **`checkaccess/init.go`**:
   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(store.New(d.DB), users.New(d.Bus))
       d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, newHandler(uc).Handle)
   }
   ```
   Tidak ada repository feature (query hanya `FindAccess` dari `base/store`). Tidak ada `BeginTx`.
5. **Wiring**: tambah satu baris `checkaccess.Init(d)` di `backend/internal/domain/project/internal/management/base.go` (fungsi `Register`).
6. **Test** (bagian 7): `usecase_test.go`, `handler_test.go`, dan `checkaccess_integration_test.go` (tag `integration`).
7. **Verifikasi**: `cd backend && go vet ./... && go test ./internal/domain/project/... && go test -tags integration ./internal/domain/project/internal/management/checkaccess/...`; archtest T00 (`TestT00_AC4_RepoIsClean`) lulus (feature tidak meng-import feature lain; hanya `base`, `base/store`, `base/users`, `lib/*`).

Kontrak bus yang dilayani (provider, v1):

| Kondisi | Hasil |
|---|---|
| `req` bukan `checkaccess.Request` (mis. pointer atau tipe lain) | error membungkus `bus.ErrBadRequestType` |
| `UserID` kosong/bukan uuid, atau `UserRole` bukan `super_admin`/`developer` | `errs.Wrap(errs.CodeInvalid, "<field> tidak valid", errs.ErrInvalid)` |
| `ProjectID` bukan uuid atau tidak ada | `{ProjectExists:false, Level:none}`, err nil |
| `super_admin` (project ada) | `{true, admin}`, tanpa get-user |
| developer bukan anggota | `{true, none}`, tanpa get-user |
| developer anggota, get-user `!Found` atau `!IsActive` | `{true, none}` |
| developer anggota aktif | `{true, owner}` atau `{true, member}` |
| error get-user (termasuk `ErrNotRegistered`) / error DB | error terbungkus (fail-closed) |

Kontrak yang dikonsumsi (persis plan T01, tidak didefinisikan ulang): `bus.CallTyped[getuser.Response](ctx, b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID})` -> `Response{Found, ID, Name, Email, Role, IsActive}`; dipanggil via `users.Lookup.Get` (sudah di foundation).

## 4. Migration

Tidak ada. Memakai tabel `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (foundation). Hanya BACA.

Query tunggal (di `base/store.FindAccess`, milik foundation; feature ini tidak menulis SQL):

```sql
SELECT p.id, p.name, p.description, p.created_by, p.created_by_label, p.created_at, p.updated_at,
       COALESCE(m.role, '') AS member_role
FROM projects p
LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = $2
WHERE p.id = $1
```

`ProjectID` bukan uuid -> store mengembalikan `found=false` tanpa query. `UserID` sudah divalidasi uuid oleh usecase sebelum store dipanggil.

## 5. Changes

Baru:
- `backend/internal/domain/project/internal/management/checkaccess/init.go`
- `backend/internal/domain/project/internal/management/checkaccess/handler.go`
- `backend/internal/domain/project/internal/management/checkaccess/usecase.go`
- `backend/internal/domain/project/internal/management/checkaccess/usecase_test.go`
- `backend/internal/domain/project/internal/management/checkaccess/handler_test.go`
- `backend/internal/domain/project/internal/management/checkaccess/checkaccess_integration_test.go`

Ubah:
- `backend/internal/domain/project/internal/management/base.go`: tambah `checkaccess.Init(d)` di `Register`.

Tidak diubah oleh usecase ini (sudah dari foundation): kontrak `checkaccess`, `lib/constants/bus.go`, `base/*`, migrasi, `api/*` (tidak ada route HTTP; OpenAPI tidak terpengaruh), i18n, frontend.

Catatan arsitektur: tidak ada `repository.go` karena tidak ada query khusus feature; usecase menerima interface `AccessFinder` dan `users.Lookup` (rule 5.6), bukan tipe konkret.

## 6. Pola consumer (dokumentasi untuk T04-T06, sudah ada di doc comment kontrak)

```go
res, err := bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
    checkaccess.Request{ProjectID: pid, UserID: id.UserID, UserRole: id.Role})
if err != nil { return err }                                   // 500, fail-closed
if !res.ProjectExists || res.Level == checkaccess.LevelNone {
    return errs.New(errs.CodeNotFound, "project tidak ditemukan") // 404, bukan 403
}
// aksi yang butuh owner/admin: Level member -> 403 FORBIDDEN
```

## 7. Test Scenario

Penamaan: `TestT03_ACn_...` untuk AC; lainnya `Test<Fungsi>_<Kasus>`.

### Unit usecase (`usecase_test.go`, repo dan `users.Lookup` palsu, hitung panggilan)

- `TestExecute_SuperAdminGetsAdminWithoutGetUser`: project ada, role `super_admin` (anggota maupun bukan) -> `{true, admin}`, panggilan get-user = 0.
- `TestExecute_DeveloperNotMemberIsNone`: `memberRole=""` -> `{true, none}`, get-user = 0.
- `TestExecute_DeveloperOwner` / `..._Member`: get-user aktif -> `{true, owner}` / `{true, member}`, get-user dipanggil tepat 1x dengan `UserID` benar.
- `TestT03_AC5_DeactivatedMemberIsNone`: get-user `Found=true, IsActive=false` -> `{true, none}`.
- `TestT03_AC5_MemberNotFoundInAuthIsNone`: get-user `Found=false` -> `{true, none}`.
- `TestT03_AC8_ProjectNotFound`: `found=false` -> `{false, none}`, err nil, get-user = 0.
- `TestExecute_GetUserErrorIsFailClosed`: get-user error -> error terbungkus, `errors.Is` ke error asal, response nol (bukan akses); varian `bus.ErrNotRegistered` tetap `errors.Is` benar.
- `TestExecute_FindAccessErrorIsWrapped`: error repo -> error terbungkus, bukan `errs.ErrInvalid`.
- `TestExecute_InvalidInput`: `UserID` kosong, `"bukan-uuid"`; `UserRole` `""`, `"agent"`, `"admin"` -> `errors.Is(err, errs.ErrInvalid)`; repo tidak dipanggil.
- `TestExecute_ProjectIDNotUUIDPassedToStore`: `ProjectID="bukan-uuid"` tidak ditolak usecase; repo palsu `found=false` -> `{false, none}`.
- `TestExecute_RoleFromMembershipUnknownIsNone`: `memberRole="aneh"` (data korup) -> `none` (via `ResolveLevel`).

### Handler (`handler_test.go`)

- Tipe salah (`&checkaccess.Request{}`, `"x"`, `nil`) -> `errors.Is(err, bus.ErrBadRequestType)`.
- Request nilai valid -> response bertipe `checkaccess.Response` (nilai, bukan pointer) sehingga `bus.CallTyped[checkaccess.Response]` berhasil.

### Integrasi (`checkaccess_integration_test.go`, tag `integration`, `db.TestDB(t)`, bus nyata `bus.NewRegistry(log)`, `get-user` palsu terdaftar di registry test)

Fixture: A (owner), B (member), C (bukan anggota), D (anggota nonaktif), S (super_admin); project P dibuat langsung via SQL.

- `TestT03_AC5_DeactivatedMemberHasNoAccess`: B member aktif -> `member`; get-user palsu diubah B `IsActive=false` -> `{true, none}`; A tetap `owner`; S -> `admin` dan penghitung panggilan get-user untuk S = 0.
- `TestT03_AC8_CheckAccessUnknownProject`: uuid acak -> `{false, none}`, err nil; `"bukan-uuid"` -> sama; tipe request salah via `bus.CallTyped` -> `ErrBadRequestType`; `UserRole="agent"` -> `errors.Is(err, errs.ErrInvalid)`.
- `TestCheckAccess_NonMemberDeveloper`: C -> `{true, none}`, get-user tidak dipanggil.
- `TestCheckAccess_FailClosedWhenGetUserUnregistered`: registry tanpa get-user, A (anggota) -> error `errors.Is(err, bus.ErrNotRegistered)`; S tetap `admin` (tidak butuh get-user).
- `TestCheckAccess_SingleQuery`: Querier pencatat menunjukkan tepat 1 query untuk developer anggota dan 0 untuk `ProjectID` bukan uuid.
- `TestCheckAccess_NoAuditCall`: handler palsu `core.audit.record` tidak pernah dipanggil.
- `TestCheckAccess_AfterMemberRemoved`: hapus baris `project_members` B -> `none` pada panggilan berikutnya (tanpa cache).
- Keamanan (rule 13.6): developer C meminta akses ke project A dengan `UserID` C -> tidak pernah `member/owner`; request dengan `UserID` A dari konteks C tidak dapat dipalsukan lewat handler ini karena identitas diberikan consumer (dokumentasikan: consumer WAJIB memakai `reqctx.IdentityFrom(ctx)`, bukan input klien).

## 8. E2E Scenarios

Feature ini tidak punya HTTP/UI sendiri; diverifikasi lewat konsumennya (T04-T06) dan lewat route T03 yang mengandalkan keputusan setara. Skenario saat T04/T05/T06 sudah ada (tandai `skip` sampai mereka merge):

Positif:
1. Developer owner membuka tab API Keys/Knowledge/API Specs project -> 200 (level `owner`).
2. Developer member membuka tab yang sama (baca/edit) -> 200; aksi khusus owner -> 403.
3. Super Admin membuka tab project yang bukan miliknya -> 200 (level `admin`).

Negatif:
1. Developer bukan anggota membuka endpoint T04-T06 untuk project itu -> 404 `project tidak ditemukan` (bukan 403).
2. Id project acak/bukan uuid pada endpoint T04-T06 -> 404 yang sama (AC8).
3. Developer yang dinonaktifkan Super Admin, dengan sesi yang masih ada, tidak lolos middleware sesi (401); lewat jalur bus murni, anggota nonaktif -> `none` -> 404 (AC5).
4. Layanan `core.auth.get-user` gagal -> endpoint konsumen developer-anggota menjawab 500 (fail-closed), Super Admin tetap lolos.

## 9. Request/Response body

Tidak ada HTTP. Kontrak bus (Go, bukan JSON di wire):

Request (nilai, bukan pointer):

```go
checkaccess.Request{
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001",
    UserID:    "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    UserRole:  "developer",
}
```

Response:

```go
// developer anggota aktif
checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}
// owner
checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelOwner}
// super_admin
checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}
// bukan anggota ATAU anggota nonaktif/tidak ditemukan di auth
checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}
// project tidak ada / ProjectID bukan uuid (err nil)
checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelNone}
```

Panggilan konsumsi ke T01 (hanya developer anggota):

```go
getuser.Request{UserID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b"}
// -> getuser.Response{Found: true, ID: "...", Name: "Budi", Email: "b@tim.id", Role: "developer", IsActive: false}
```

Error (dikembalikan sebagai `error` Go; bila consumer meneruskannya ke HTTP, `respond.WriteError` memetakan sebagai berikut):

| Sebab | Error | Status bila diteruskan |
|---|---|---|
| Tipe request salah | membungkus `bus.ErrBadRequestType` | 500 generik (bug programmer consumer) |
| `UserID`/`UserRole` tidak valid | `errs.Wrap(CodeInvalid, "user_id tidak valid", errs.ErrInvalid)` | 400 `INVALID_ARGUMENT` |
| get-user / DB gagal | `fmt.Errorf("checkaccess: get-user: %w", err)` | 500 `INTERNAL` generik |

Contoh JSON error seragam T00 bila 400 diteruskan consumer:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user_role tidak valid", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Contoh 404 yang dihasilkan consumer dari `Level=none` atau `ProjectExists=false`:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
