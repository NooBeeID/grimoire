# T03 project/management — Usecase `getprojectbus` (bus `project.management.get-project`)

Melengkapi `docs/superpowers/plans/project-management/overview.md` (F: `base.Project`, `base/store`, kontrak, konstanta, kerangka `management.Register`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `getprojectbus` (U2, gelombang A, dibutuhkan T07). Jangan tertukar dengan `getproject` (HTTP `GET /api/projects/{projectID}`, U5): folder, kontrak, dan semantik "tidak ada" berbeda (di sini `Found=false` tanpa error dan tanpa pemeriksaan aktor; di sana 404 dan Require(PermView)). Juga bedakan paket kontrak `lib/contracts/project/management/getproject` (nama paket kontrak) dari folder feature `getprojectbus`.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack pada tiket; usecase ini backend saja (tidak ada FE) |
| Domain | project |
| Module | management |
| Feature | `getprojectbus` (gelombang A, U2) |
| Endpoint | bus `project.management.get-project` (konstanta `BUS_PROJECT_MANAGEMENT_GET_PROJECT`); tidak ada endpoint HTTP |
| Method | Call (sinkron, `bus.Call` / `bus.CallTyped`) |
| Version | kontrak baru, v1 (perubahan breaking = ubah nama endpoint) |
| Consumer | T07 (setelah verifikasi API key). Tidak ada consumer di T03 |
| Auth | tidak ada (jalur internal in-process; tidak memeriksa aktor, hanya mengembalikan id + nama) |
| Audit | tidak ada (aksi baca internal) |
| Tabel / migrasi | membaca `projects`; tidak ada migrasi baru |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `backend/internal/domain/project/internal/management/getprojectbus/`, `backend/internal/domain/project/internal/management/base.go` (satu baris `getprojectbus.Init(d)`), `backend/lib/contracts/project/management/getproject/`, `backend/lib/constants/bus.go` (satu konstanta, bila belum ada dari F) |

Prasyarat dari F (bila belum ada, hentikan dan laporkan; jangan membuatnya di luar allowedPaths): `base.Project`, `base/store.Store.FindProject` (id bukan uuid mengembalikan `(zero,false,nil)` tanpa query SQL), `management.Register` di `management/base.go`, `project.Register` memanggil `management.Register`, tabel `projects`. Kontrak `getproject` dan konstanta `BUS_PROJECT_MANAGEMENT_GET_PROJECT` didefinisikan F langkah 2-3; bila belum ada, buat persis seperti di bawah (file kecil, stdlib saja, rule 4.5).

## Summary Plan

`getprojectbus` adalah provider bus yang memberi module lain (T07) identitas ringkas project: `{ProjectID}` menjadi `{Found, ID, Name}`.

Aturan (overview 5.3, 9):
- `ProjectID` bukan uuid atau tidak ada di DB: `Found=false`, error `nil`. Bukan error, supaya consumer tidak perlu menguraikan error untuk kasus normal.
- Hanya error infra (DB gagal) yang menjadi error, dibungkus `fmt.Errorf("getprojectbus: find: %w", err)`. Consumer tidak boleh menyimpulkan "project tidak ada" dari kegagalan DB.
- Tidak ada pemeriksaan aktor atau level akses. Endpoint ini internal; otorisasi project tetap lewat `check-access`. Respons hanya `ID` dan `Name` (tanpa deskripsi, created_by, atau data anggota).
- Request dibaca sebagai nilai lewat `bus.Expect[getproject.Request]`; pointer atau tipe lain menghasilkan error yang membungkus `bus.ErrBadRequestType` (rule 6.9), bukan panic.
- Response dikembalikan sebagai nilai (`getproject.Response`), bukan pointer, agar `bus.CallTyped[getproject.Response]` cocok.
- `ID` pada respons berasal dari baris DB (`base.Project.ID`, bentuk kanonik), bukan menggemakan input.
- Tanpa audit, tanpa `users`/`counts`/`audit` subpackage, tanpa clock, tanpa transaksi, tanpa HTTP, tanpa `params.go`, tanpa `repository.go` (tidak ada query khusus; usecase menerima interface yang dipenuhi `*store.Store`, pola `checkaccess` di overview bagian 7).
- Tanpa perubahan OpenAPI, i18n, atau frontend.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F di atas ada. Baca `backend/internal/infra/bus/bus.go` (`Registry.Register(endpoint string, h Handler)`; `type Handler func(ctx context.Context, req any) (any, error)`; panik bila endpoint terdaftar dua kali) dan `typed.go` (`Expect`, `CallTyped`). Cek `app.Deps.Bus` bertipe `*bus.Registry` dan `app.Deps.DB` memenuhi `db.Querier`. Bila `checkaccess` (U1) sudah ada, ikuti pola `Init`-nya persis.

**B1. Kontrak `backend/lib/contracts/project/management/getproject/getproject.go`** (bila belum ada dari F; hanya stdlib)

```go
// Package getproject adalah kontrak bus project.management.get-project (provider T03, consumer T07).
// ProjectID bukan uuid atau tidak ada -> Found=false, err nil. Tipe request salah -> ErrBadRequestType.
// Tidak memeriksa aktor; otorisasi project memakai check-access.
package getproject

type Request struct{ ProjectID string }

type Response struct {
    Found bool
    ID    string
    Name  string
}
```

**B2. Konstanta** `backend/lib/constants/bus.go`: pastikan ada

```go
const BUS_PROJECT_MANAGEMENT_GET_PROJECT = "project.management.get-project"
```

Jangan menduplikasi bila F sudah menambahkannya. `lib/constants/bus_test.go` T00 harus tetap lulus.

**B3. `getprojectbus/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package getprojectbus

type projectFinder interface {
    FindProject(ctx context.Context, id string) (base.Project, bool, error)
}

type Usecase struct{ repo projectFinder }

func NewUsecase(repo projectFinder) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, projectID string) (getproject.Response, error) {
    p, found, err := u.repo.FindProject(ctx, projectID)
    if err != nil {
        return getproject.Response{}, fmt.Errorf("getprojectbus: find: %w", err)
    }
    if !found {
        return getproject.Response{Found: false}, nil
    }
    return getproject.Response{Found: true, ID: p.ID, Name: p.Name}, nil
}
```

Catatan: uuid divalidasi di `store.FindProject` (satu tempat yang memutuskan "uuid valid", sama seperti `checkaccess` dan route HTTP), jadi usecase tidak melakukan `uuid.Parse` sendiri dan tidak melakukan `TrimSpace` (id dengan spasi dianggap bukan uuid -> `Found=false`; sisi consumer wajib mengirim id apa adanya dari data tersimpan). Pemetaan entity ke Response sederhana dan tidak membawa rule bisnis, jadi tidak perlu method entity baru.

**B4. `getprojectbus/handler.go`** (handler bus)

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk project.management.get-project.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[getproject.Request](req)
    if err != nil {
        return nil, err // membungkus bus.ErrBadRequestType
    }
    return h.uc.Execute(ctx, in.ProjectID) // nilai getproject.Response, bukan pointer
}
```

Handler tidak menulis log untuk `Found=false` (kasus normal, hindari noise). Handler tidak membaca `reqctx` (tidak ada aktor).

**B5. `getprojectbus/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(store.New(d.DB))
    d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, newHandler(uc).Handle)
}
```

Tidak ada route HTTP.

**B6. Wiring.** Tambah satu baris `getprojectbus.Init(d)` di `management/base.go` dalam `Register`. Satu baris, tanpa mengubah baris lain (minimal konflik merge dengan U1/U3 yang paralel di gelombang A).

**B7. Test** (bagian Test Scenario).

**B8. Verifikasi akhir.** `go vet ./...`; archtest `TestT00_AC4_RepoIsClean` (folder `getprojectbus` hanya meng-import `base`, `base/store`, `lib/contracts/project/management/getproject`, `lib/constants`, `infra/app`, `infra/bus`; tidak meng-import feature lain, rule 4.3); `go test -race ./internal/domain/project/... ./lib/...`; `go test -tags integration` untuk test integrasi.

## Migration

Tidak ada. `getprojectbus` hanya membaca tabel `projects` dari `backend/migrations/project/20261022090000000_create_projects.sql` (milik F).

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/management/getprojectbus/init.go`
- `backend/internal/domain/project/internal/management/getprojectbus/handler.go`
- `backend/internal/domain/project/internal/management/getprojectbus/usecase.go`
- `backend/internal/domain/project/internal/management/getprojectbus/usecase_test.go`
- `backend/internal/domain/project/internal/management/getprojectbus/handler_test.go`
- `backend/internal/domain/project/internal/management/getprojectbus/integration_test.go` (tag `integration`)
- `backend/lib/contracts/project/management/getproject/getproject.go` (bila belum dibuat F)

Backend (ubah, minimal):
- `backend/internal/domain/project/internal/management/base.go`: satu baris `getprojectbus.Init(d)`.
- `backend/lib/constants/bus.go`: `BUS_PROJECT_MANAGEMENT_GET_PROJECT` (hanya bila belum ada).

Tidak dibuat: `params.go`, `repository.go`, migrasi, route HTTP, perubahan `management.yml`/`root.yml`, i18n, kode frontend, event audit.

Dokumen: katalog `docs/tickets/README.md` sudah memuat baris `project.management.get-project` (Call, T03, consumer T07); catatan lokasi kontrak ditambahkan di langkah F (bukan di usecase ini).

## Test Scenario

Penamaan mengikuti rule 13.4. Usecase ini tidak memiliki AC bernomor di tiket (dipakai T07); test memakai nama `Test...` biasa.

### Unit usecase (repository fake lewat interface `projectFinder`)
- `TestUsecase_Execute_Found`: repo mengembalikan project; hasil `Found=true`, `ID` dan `Name` sama dengan entity.
- `TestUsecase_Execute_NotFound`: repo `(zero,false,nil)` menghasilkan `Response{Found:false}` (semua field kosong) dan error `nil`.
- `TestUsecase_Execute_RepoError`: repo mengembalikan error; hasil `Response{}` dan error membungkus (`errors.Is`) error asal; bukan `Found=false`.
- `TestUsecase_Execute_PassesIDVerbatim`: fake mencatat argumen; `" <uuid> "` dan `""` diteruskan apa adanya (tanpa trim), repo fake `(zero,false,nil)` menghasilkan `Found=false`.
- `TestUsecase_Execute_ReturnsOnlyIDAndName`: `reflect` atas `getproject.Response` hanya memiliki tiga field (`Found`, `ID`, `Name`).

### Handler (bus nyata `bus.NewRegistry`, usecase memakai repo fake)
- `TestHandler_Handle_ReturnsValueResponse`: lewat `bus.CallTyped[getproject.Response]` berhasil (membuktikan handler mengembalikan nilai, bukan pointer).
- `TestHandler_Handle_BadRequestType`: `registry.Call(ctx, BUS_PROJECT_MANAGEMENT_GET_PROJECT, "string-salah")` dan pointer `&getproject.Request{}` menghasilkan error yang `errors.Is(err, bus.ErrBadRequestType)`, tanpa panic.
- `TestHandler_Handle_NotFoundIsNotError`: `Found=false`, `err == nil`.
- `TestHandler_Handle_PropagatesInfraError`: error infra sampai ke pemanggil, bukan `Found=false`.
- `TestInit_RegistersEndpoint`: setelah `Init` pada registry baru, `Call` ke endpoint tidak menghasilkan `bus.ErrNotRegistered`; memanggil `Init` dua kali pada registry yang sama panik (perilaku `Register`; didokumentasikan, bukan diubah).

### Integrasi (Postgres nyata `db.TestDB(t)` + migrasi, bus nyata, `getprojectbus.Init`)
- `TestGetProjectBus_Integration_Found`: sisip project via SQL fixture; `CallTyped` mengembalikan `ID` dan `Name` sama dengan baris DB (nama sudah ter-trim saat disimpan).
- `TestGetProjectBus_Integration_UnknownUUID`: uuid valid tidak ada menghasilkan `Found=false`, `err=nil`.
- `TestGetProjectBus_Integration_InvalidUUID`: `"abc"`, `"123"`, `"'; DROP TABLE projects;--"`, string kosong, 1000 karakter menghasilkan `Found=false`, `err=nil`, tanpa error DB (membuktikan validasi uuid di store dan tidak ada injeksi); tabel `projects` masih utuh.
- `TestGetProjectBus_Integration_NoActorRequired`: konteks tanpa identitas (`context.Background()`) tetap berhasil; hasil tidak bergantung keanggotaan.
- `TestGetProjectBus_Integration_ReflectsRename`: setelah `UPDATE projects SET name=...`, panggilan berikutnya mengembalikan nama baru (tidak ada cache).
- `TestGetProjectBus_Integration_AfterDelete`: setelah project dihapus, `Found=false`.
- `TestGetProjectBus_Integration_Concurrent`: 20 goroutine memanggil paralel; semua hasil konsisten (jalankan dengan `-race`).
- `TestGetProjectBus_Integration_NoAudit`: handler `core.audit.record` palsu menghitung panggilan; hasilnya 0.

### Tambahan lintas usecase (dimiliki `management_integration_test.go`, W)
- `TestRoutesRegistered`: setelah `project.Register(d)`, `Call` ke `BUS_PROJECT_MANAGEMENT_GET_PROJECT` dengan request valid tidak mengembalikan `ErrNotRegistered`.
- `TestT03_Security_ProjectIsolation`: `get-project` tidak dibatasi aktor dan hanya mengembalikan id + nama (sudah tercantum di overview bagian 10; dicek di sana, bukan diulang di sini).

## E2E Scenarios

Endpoint ini tidak punya antarmuka HTTP/UI, sehingga E2E Playwright tidak berlaku langsung. Verifikasinya lewat test integrasi in-process di atas dan, begitu consumer ada (T07), lewat E2E consumer tersebut. Perilaku yang harus tetap benar:

Positif:
1. Consumer memanggil dengan id project yang ada: `Found=true`, `ID` dan `Name` benar.
2. Developer A membuat project lewat `createproject` lalu consumer memanggil dengan id itu: `Found=true` dengan nama sama.
3. Setelah `updateproject` mengubah nama, panggilan berikutnya mengembalikan nama terbaru.
4. Server menyala dengan semua domain terdaftar (smoke): `Call` ke endpoint ini dari `cmd/internal/server.WithDomains` tidak `ErrNotRegistered`.

Negatif:
1. Id tidak ada: `Found=false`, tanpa error, consumer tidak crash (T07 menjawab 404/401 sesuai aturannya).
2. Id bukan uuid atau kosong: sama, `Found=false`.
3. Setelah `deleteproject`, panggilan mengembalikan `Found=false`.
4. Tipe request salah: error `ErrBadRequestType`, bukan panic.
5. DB tidak tersedia: error terbungkus, bukan `Found=false` (supaya consumer tidak salah menyimpulkan project tidak ada).
6. Respons tidak pernah memuat deskripsi, `created_by`, atau data anggota.

## Request/Response body

Bus bersifat in-process (Go), bukan JSON di wire. Tidak ada endpoint HTTP dan tidak ada format error HTTP; error bus dikembalikan sebagai `error` Go.

### Pemanggilan oleh consumer

```go
res, err := bus.CallTyped[getproject.Response](ctx, d.Bus, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT,
    getproject.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"})
```

Hasil project ada:
```go
getproject.Response{
    Found: true, ID: "0192f5a0-1111-7aaa-8bbb-000000000001", Name: "Payments",
}
```

Hasil id tidak ada atau bukan uuid (`err == nil`):
```go
getproject.Request{ProjectID: "bukan-uuid"}                          // -> getproject.Response{Found: false}
getproject.Request{ProjectID: "00000000-0000-4000-8000-000000000000"} // -> getproject.Response{Found: false}
```

### Error

Tipe request salah:
```go
_, err := d.Bus.Call(ctx, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, "string-salah")
// errors.Is(err, bus.ErrBadRequestType) == true
```

Endpoint belum terdaftar (salah wiring, bukan kasus normal):
```go
// errors.Is(err, bus.ErrNotRegistered) == true
```

Kegagalan infra:
```go
// err = fmt.Errorf("getprojectbus: find: %w", <error DB>); Response zero value
```

Fitur HTTP lain pada module ini memakai format error seragam T00 `{ "error": { "code", "message", "details" }, "trace_id" }`; tidak berlaku di sini karena tidak ada lapisan HTTP.

## Definisi selesai

- Semua test di atas hijau (termasuk `-race`); `go vet` dan archtest `TestT00_AC4_RepoIsClean` lulus.
- Id non-uuid dan id tak dikenal menghasilkan `Found=false` tanpa error; error infra tidak pernah disamarkan menjadi `Found=false`.
- Tidak ada perubahan di luar allowedPaths selain satu baris di `management/base.go`.
