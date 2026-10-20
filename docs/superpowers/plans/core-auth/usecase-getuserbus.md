# T01 core/auth — Usecase `getuserbus` (bus `core.auth.get-user`)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1: `base.User`, `base/store`, `auth.Register`/`buildShared`, konstanta bus). Bila overview bertentangan dengan dokumen ini, overview menang. Hanya satu usecase: `getuserbus` (U2). Jangan tertukar dengan `getuser` (HTTP `GET /api/users/{id}`, U10): folder, kontrak, dan semantik "tidak ada" berbeda (di sini `Found=false` tanpa error, di sana 404).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack pada tiket; usecase ini backend saja (tidak ada FE) |
| Domain | core |
| Module | auth |
| Feature | `getuserbus` (gelombang A, U2) |
| Endpoint | bus `core.auth.get-user` (konstanta `BUS_CORE_AUTH_GET_USER`); tidak ada endpoint HTTP |
| Method | Call (sinkron, `bus.Call`/`bus.CallTyped`) |
| Version | kontrak baru, v1 (belum ada prefix versi; perubahan breaking = ubah nama endpoint) |
| Consumer | T03, T04, T05, T06 (tidak ada consumer di T01) |
| Auth | tidak ada (jalur internal in-process) |
| Audit | tidak ada (aksi baca internal) |
| Tabel / migrasi | membaca `users`; tidak ada migrasi |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/getuserbus/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `getuserbus.Init`), `backend/lib/contracts/core/auth/getuser/`, `backend/lib/constants/bus.go` (satu konstanta, bila belum ada dari F1) |

Prasyarat dari F1 (bila belum ada, jangan dibuat ulang di luar allowedPaths; hentikan dan laporkan): `base.User`, `base/store.Store.FindUserByID` (id bukan uuid mengembalikan `(zero,false,nil)` tanpa query SQL), `auth.Register` + `buildShared`, tabel `users`. Kontrak `getuser` dan konstanta `BUS_CORE_AUTH_GET_USER` didefinisikan F1 langkah 3-4; bila belum ada, buat persis seperti di bawah (file kecil, stdlib saja).

## Summary Plan

`getuserbus` adalah provider bus yang memberi modul lain ringkasan user dari id: `{UserID}` menjadi `{Found, ID, Name, Email, Role, IsActive}`.

Aturan (dari tiket dan overview 5.2):
- `UserID` bukan uuid atau tidak ada di DB: `Found=false`, error `nil`. Bukan error, supaya consumer tidak perlu menguraikan error untuk kasus normal.
- User nonaktif: tetap `Found=true`, `IsActive=false`. Keputusan "nonaktif = tidak berlaku" ada di consumer (T03), bukan di sini. Usecase tidak menyaring `is_active`.
- Hanya error infra (DB gagal) yang menjadi error, dibungkus `fmt.Errorf("getuserbus: find: %w", err)`.
- Response tidak memuat `PasswordHash`, `MustChangePassword`, atau field sensitif lain: kontrak hanya enam field di atas.
- Response dikembalikan sebagai nilai (`getuser.Response`), bukan pointer, agar `bus.CallTyped[getuser.Response]` cocok (pola sama dengan `resolvesession`). Request juga dibaca sebagai nilai lewat `bus.Expect[getuser.Request]`.
- Tipe request salah (mis. string) menghasilkan error yang membungkus `bus.ErrBadRequestType` (rule 6.9), bukan panic.
- Usecase tidak memakai `base/audit`, tidak memakai hasher, clock, atau transaksi.
- Repository feature membungkus `*store.Store` dan hanya mengekspos `FindUserByID`. Tidak ada SQL baru.
- Tidak ada `params.go` (bukan HTTP). `handler.go` berisi handler bus. Tidak ada route HTTP, tidak ada perubahan OpenAPI.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F1 di atas ada. Baca `bus.Registry.Register(endpoint string, h bus.Handler)` (`type Handler func(ctx context.Context, req any) (any, error)`; panic bila endpoint terdaftar dua kali) dan `bus.Expect`. Baca `resolvesession` (U1) bila sudah ada untuk pola `Init` bus; ikuti persis. Cek `d.Bus` bertipe `*bus.Registry` di `app.Deps`.

**B1. Kontrak `backend/lib/contracts/core/auth/getuser/getuser.go`** (bila belum ada dari F1; hanya stdlib, rule 4.5)

```go
// Package getuser adalah kontrak bus core.auth.get-user.
// UserID bukan uuid / tidak ada -> Found=false, err=nil.
// User nonaktif -> Found=true, IsActive=false (konsumen yang memutuskan).
package getuser

type Request struct {
    UserID string
}

type Response struct {
    Found    bool
    ID       string
    Name     string
    Email    string
    Role     string // resolvesession.RoleSuperAdmin | resolvesession.RoleDeveloper
    IsActive bool
}
```

**B2. Konstanta** `backend/lib/constants/bus.go`: pastikan ada

```go
const BUS_CORE_AUTH_GET_USER = "core.auth.get-user"
```

Jangan menduplikasi bila F1 sudah menambahkannya. `lib/constants/bus_test.go` T00 harus tetap lulus.

**B3. `getuserbus/repository.go`** (satu-satunya yang menyentuh SQL, lewat `store`)

```go
package getuserbus

type userFinder interface {
    FindUserByID(ctx context.Context, id string) (base.User, bool, error)
}

type Repository struct{ s *store.Store }

func NewRepository(q db.Querier) *Repository { return &Repository{s: store.New(q)} }

func (r *Repository) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
    return r.s.FindUserByID(ctx, id)
}
```

**B4. `getuserbus/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
type Usecase struct{ repo userFinder }

func NewUsecase(repo userFinder) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, userID string) (getuser.Response, error) {
    user, found, err := u.repo.FindUserByID(ctx, strings.TrimSpace(userID))
    if err != nil {
        return getuser.Response{}, fmt.Errorf("getuserbus: find: %w", err)
    }
    if !found {
        return getuser.Response{Found: false}, nil
    }
    return getuser.Response{
        Found: true, ID: user.ID, Name: user.Name, Email: user.Email,
        Role: user.Role, IsActive: user.IsActive,
    }, nil
}
```

Catatan: `TrimSpace` hanya pembersihan input toleran; uuid tetap divalidasi oleh `store.FindUserByID` (satu tempat yang memutuskan "uuid valid", sama seperti `getuser`). Pemetaan entity ke Response sederhana dan tidak membawa rule bisnis, jadi boleh tinggal di usecase (tidak perlu method entity baru).

**B5. `getuserbus/handler.go`** (handler bus)

```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk core.auth.get-user.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[getuser.Request](req)
    if err != nil {
        return nil, err // membungkus bus.ErrBadRequestType
    }
    return h.uc.Execute(ctx, in.UserID) // nilai getuser.Response, bukan pointer
}
```

Handler tidak menulis log untuk kasus `Found=false` (kasus normal, hindari noise). Kegagalan infra dilog oleh pemanggil/bus sesuai T00.

**B6. `getuserbus/init.go`**

```go
func Init(d app.Deps, _ base.Shared) {
    h := NewHandler(NewUsecase(NewRepository(d.DB)))
    d.Bus.Register(constants.BUS_CORE_AUTH_GET_USER, h.Handle)
}
```

Ikuti tipe `d.DB` yang dipakai feature lain. Tidak ada route HTTP.

**B7. Wiring.** Tambah satu baris `getuserbus.Init(d, shared)` di `auth/base.go` dalam `Register`. Satu baris, tanpa mengubah baris lain (minimal konflik merge).

**B8. Test** (bagian Test Scenario).

**B9. Verifikasi akhir.** `go vet ./...`, archtest `TestT00_AC4_RepoIsClean` (folder `getuserbus` hanya meng-import `base`, `base/store`, `lib/contracts/...`, `lib/constants`, `infra/app`, `infra/bus`, `infra/db`; tidak meng-import feature lain, rule 4.3), `go test ./internal/domain/core/... ./lib/...`.

## Migration

Tidak ada. `getuserbus` hanya membaca tabel `users` dari `backend/migrations/core/20261021090000000_create_auth_tables.sql` (milik F1).

## Changes

Backend (baru):
- `backend/internal/domain/core/internal/auth/getuserbus/init.go`
- `backend/internal/domain/core/internal/auth/getuserbus/handler.go`
- `backend/internal/domain/core/internal/auth/getuserbus/usecase.go`
- `backend/internal/domain/core/internal/auth/getuserbus/repository.go`
- `backend/internal/domain/core/internal/auth/getuserbus/usecase_test.go`
- `backend/internal/domain/core/internal/auth/getuserbus/handler_test.go`
- `backend/internal/domain/core/internal/auth/getuserbus/integration_test.go`
- `backend/lib/contracts/core/auth/getuser/getuser.go` (bila belum dibuat F1)

Backend (ubah, minimal):
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `getuserbus.Init(d, shared)`.
- `backend/lib/constants/bus.go`: `BUS_CORE_AUTH_GET_USER` (hanya bila belum ada).

Tidak dibuat: `params.go`, migrasi, route HTTP, perubahan `auth.yml`, kode frontend, event audit.

Dokumen: katalog `docs/tickets/README.md` sudah memuat baris `core.auth.get-user` (Call, T01, consumer T03/T04/T05/T06); tidak perlu diubah.

## Test Scenario

Penamaan mengikuti rule 13.4. Usecase ini tidak memiliki AC bernomor di tiket; test memakai nama `Test...` biasa.

### Unit usecase (repository fake lewat interface `userFinder`)
- `TestUsecase_Execute_Found`: user ada menghasilkan `Found=true` dan lima field (`ID`, `Name`, `Email`, `Role`, `IsActive`) sama dengan entity.
- `TestUsecase_Execute_NotFound`: repo `(zero,false,nil)` menghasilkan `Response{Found:false}` (semua field kosong) dan error `nil`.
- `TestUsecase_Execute_InactiveUserStillFound`: `IsActive=false` menghasilkan `Found=true, IsActive=false`.
- `TestUsecase_Execute_SuperAdminRole`: role `super_admin` diteruskan apa adanya.
- `TestUsecase_Execute_RepoError`: repo mengembalikan error; hasil `Response{}` dan error membungkus (`errors.Is`) error asal; bukan `Found=false`.
- `TestUsecase_Execute_TrimsID`: `" <uuid> "` diteruskan ke repo sebagai `<uuid>` (fake mencatat argumen).
- `TestUsecase_Execute_EmptyID`: `""` diteruskan, repo `(zero,false,nil)` menghasilkan `Found=false`.

### Handler (bus nyata `bus.NewRegistry`, usecase memakai repo fake)
- `TestHandler_Handle_ReturnsValueResponse`: lewat `bus.CallTyped[getuser.Response]` berhasil (membuktikan handler mengembalikan nilai, bukan pointer).
- `TestHandler_Handle_BadRequestType`: `registry.Call(ctx, BUS_CORE_AUTH_GET_USER, "string-salah")` dan pointer `&getuser.Request{}` menghasilkan error yang `errors.Is(err, bus.ErrBadRequestType)`, tanpa panic.
- `TestHandler_Handle_NotFoundIsNotError`: `Found=false`, `err == nil`.
- `TestHandler_Handle_PropagatesInfraError`: error infra sampai ke pemanggil.
- `TestInit_RegistersEndpoint`: setelah `Init`, `Call` ke endpoint tidak menghasilkan `bus.ErrNotRegistered`; memanggil `Init` dua kali pada registry sama panik (perilaku `Register`; didokumentasikan, bukan diubah).

### Integrasi (Postgres nyata `db.TestDB(t)` + migrasi, bus nyata, `getuserbus.Init`)
- `TestGetUserBus_Integration_Found`: sisip user via `store.InsertUser`; `CallTyped` mengembalikan field sama dengan baris DB.
- `TestGetUserBus_Integration_Inactive`: user nonaktif (`IsActive=false` di DB) menghasilkan `Found=true, IsActive=false`.
- `TestGetUserBus_Integration_UnknownUUID`: uuid valid tidak ada menghasilkan `Found=false`, `err=nil`.
- `TestGetUserBus_Integration_InvalidUUID`: `"abc"`, `"123"`, `"'; DROP TABLE users;--"`, string kosong, 1000 karakter menghasilkan `Found=false`, `err=nil`, tanpa error DB (membuktikan validasi uuid di store dan tidak ada injeksi).
- `TestGetUserBus_Integration_NoSensitiveFields`: `reflect` atas `getuser.Response` hanya memiliki enam field yang disepakati; `fmt.Sprintf("%+v")` hasil tidak memuat `$argon2id$`.
- `TestGetUserBus_Integration_Concurrent`: 20 goroutine memanggil paralel; semua hasil konsisten (tidak ada data race, jalankan dengan `-race`).
- `TestGetUserBus_Integration_NoAudit`: handler `core.audit.record` palsu menghitung panggilan; hasilnya 0.

### Tambahan lintas usecase (dimiliki `auth_integration_test.go`, W)
- Baris di tabel AC8/AC9: `getuserbus` menghasilkan 0 `record.Request` dan tidak ada plaintext/hash di log.
- Pemeriksaan wiring: setelah `core.Register(d)`, `Call` ke `BUS_CORE_AUTH_GET_USER` tidak mengembalikan `ErrNotRegistered`.

## E2E Scenarios

Endpoint ini tidak punya antarmuka HTTP/UI, sehingga E2E Playwright tidak berlaku langsung. Verifikasinya lewat test integrasi in-process di atas dan, begitu consumer ada (T03), lewat E2E consumer tersebut. Skenario perilaku yang harus tetap benar:

Positif:
1. Consumer memanggil dengan id developer aktif: `Found=true`, `IsActive=true`, nama/email/role benar.
2. Super Admin menonaktifkan developer lewat `setuserstatus`; panggilan berikutnya mengembalikan `Found=true, IsActive=false` (consumer T03 yang menolak keanggotaan). Dicek di integrasi lintas feature setelah U12 ada.
3. Setelah `updateuser` mengubah nama atau email, panggilan berikutnya mengembalikan data terbaru (tidak ada cache).
4. Server menyala dengan semua domain terdaftar (smoke): `Call` ke endpoint ini dari `cmd/internal/server.WithDomains` tidak `ErrNotRegistered`.

Negatif:
1. Id tidak ada: `Found=false`, tanpa error, dan consumer tidak crash.
2. Id bukan uuid atau kosong: sama, `Found=false`.
3. Tipe request salah: error `ErrBadRequestType`, bukan panic.
4. DB tidak tersedia: error terbungkus, bukan `Found=false` (supaya consumer tidak salah menyimpulkan user tidak ada).
5. Respons tidak pernah memuat hash password atau flag `must_change_password`.

## Request/Response body

Bus bersifat in-process (Go), bukan JSON di wire. Tidak ada endpoint HTTP dan tidak ada format error HTTP; error bus dikembalikan sebagai `error` Go.

### Pemanggilan oleh consumer

```go
res, err := bus.CallTyped[getuser.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_GET_USER,
    getuser.Request{UserID: "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02"})
```

Hasil user aktif:
```go
getuser.Response{
    Found: true, ID: "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02",
    Name: "Sari", Email: "sari@tim.id", Role: "developer", IsActive: true,
}
```

Hasil user nonaktif (tetap ditemukan; consumer yang memutuskan):
```go
getuser.Response{
    Found: true, ID: "9a2f4c6e-1b3d-4e5f-8a7b-0c1d2e3f4a5b",
    Name: "Andi", Email: "andi@tim.id", Role: "developer", IsActive: false,
}
```

Hasil id tidak ada atau bukan uuid (`err == nil`):
```go
getuser.Request{UserID: "bukan-uuid"}   // -> getuser.Response{Found: false}
getuser.Request{UserID: "00000000-0000-4000-8000-000000000000"} // -> getuser.Response{Found: false}
```

### Error

Tipe request salah:
```go
_, err := d.Bus.Call(ctx, constants.BUS_CORE_AUTH_GET_USER, "string-salah")
// errors.Is(err, bus.ErrBadRequestType) == true
```

Endpoint belum terdaftar (salah wiring, bukan kasus normal):
```go
// errors.Is(err, bus.ErrNotRegistered) == true
```

Kegagalan infra:
```go
// err = fmt.Errorf("getuserbus: find: %w", <error DB>); Response zero value
```

Kontrak CRUD lain pada module ini (HTTP) memakai format error seragam T00 `{ "error": { "code", "message", "details" }, "trace_id" }`; tidak berlaku di sini karena tidak ada lapisan HTTP.

## Definisi selesai

- Semua test di atas hijau (termasuk `-race`); `go vet`, archtest `TestT00_AC4_RepoIsClean` lulus.
- Id non-uuid dan id tak dikenal menghasilkan `Found=false` tanpa error; user nonaktif tetap `Found=true`.
- Tidak ada perubahan di luar allowedPaths selain satu baris di `auth/base.go`.
