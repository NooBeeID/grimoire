# T06 project/api-spec — Usecase Plan: `listspecsbus`

Tiket: `docs/tickets/T06-project-api-spec.md`. Plan bersama (keputusan mengikat, `base/`, `base/store`, migrasi, kontrak, konstanta): `docs/superpowers/plans/project-api-spec/overview.md`. Dokumen ini hanya memuat usecase `listspecsbus` (U8, gelombang B). Bila bertentangan dengan overview, overview menang.
Jangan tertukar dengan `listspecs` (HTTP `GET /api/projects/{projectID}/api-specs`, U3): di sini tanpa check-access, tanpa aktor, tanpa DTO HTTP, dan `ProjectID` bukan uuid adalah ERROR (`ErrInvalid`), bukan 404.
Konvensi: `docs/tickets/README.md` dan `rules.txt`. Tidak ada `CLAUDE.md`, `.claude/rules/`, maupun knowledge doc module apispec.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack pada tiket; usecase ini backend saja (tidak ada FE, tidak ada OpenAPI) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `listspecsbus` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | bus `project.apispec.list-specs` (konstanta `BUS_PROJECT_APISPEC_LIST_SPECS`); tidak ada endpoint HTTP |
| Method | Call (sinkron, `bus.Call` / `bus.CallTyped`) |
| Version | kontrak baru, v1 (perubahan breaking = nama endpoint baru) |
| Consumer | T09 (discovery/apispectools) |
| Auth | tidak ada pemeriksaan akses (jalur internal; pemanggil T09 sudah terikat key -> project lewat T07). Wajib membatasi query pada `ProjectID` request (rule 6.10) |
| Audit | tidak ada |
| Tabel / migrasi | membaca `api_specs` + `api_spec_endpoints`; tidak ada migrasi baru |
| AC | AC7 (handler bus tidak pernah mengembalikan data project lain) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/project/internal/apispec/listspecsbus/`
- `backend/internal/domain/project/internal/apispec/base.go` (tepat satu baris: `listspecsbus.Init(d)`)
- `backend/lib/constants/bus.go` (satu konstanta, hanya bila belum ada dari F)
- `backend/lib/contracts/project/apispec/listspecs/` (hanya bila belum ada dari F)

## Konteks yang dimuat

- Overview T06: 3.2 (struktur feature bus: tanpa `params.go`, `handler.go` memakai `bus.Expect`), 3.6 (urut `name`), 4.1 (`SpecSummary`), 6.1 (`store.ListSpecs`, id bukan uuid -> kosong tanpa query), 7.1-7.2 (konstanta dan kontrak), 11 (pola `Init` feature bus), 17 (baris #8).
- Pola referensi: `docs/superpowers/plans/project-management/usecase-getprojectbus.md` (handler bus, `bus.Expect`, nilai bukan pointer).
- Prasyarat dari foundation F (HARUS sudah ada; bila belum, kontrak dan konstanta dibuat persis seperti di bawah; selain itu hentikan dan laporkan, jangan membuat di luar allowedPaths):
  - `base.SpecSummary`, `base/store.New`, `Store.ListSpecs` (`LEFT JOIN` count, `ORDER BY name`, filter `project_id = $1`, id bukan uuid -> `nil, nil` tanpa query).
  - `apispec.Register` di `apispec/base.go`.
  - Migrasi `<VERSION>_create_api_specs.sql`.
- Infra di `main`: `bus.Registry.Register(endpoint string, h Handler)` (`Handler func(ctx, req any) (any, error)`; panik bila endpoint terdaftar dua kali), `bus.Expect[T]`, `bus.ErrBadRequestType`, `errs.Wrap`, `errs.CodeInvalid`, `errs.ErrInvalid` (`backend/lib/common/errs`). `github.com/google/uuid` sudah ada di `go.mod`.

## Summary Plan

`listspecsbus` adalah provider bus yang memberi T09 daftar spec sebuah project: `listspecs.Request{ProjectID}` menjadi `listspecs.Response{Items[]}`.

Keputusan:

1. Validasi uuid dilakukan di usecase (bukan diserahkan ke store), karena store mengembalikan kosong untuk id bukan uuid sehingga kasus itu tidak bisa dibedakan dari "project tanpa spec". Kontrak mensyaratkan error. Usecase memakai `uuid.Parse` dan, bila gagal, mengembalikan `errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)` (sesuai catatan overview 7.2; `errors.Is(err, errs.ErrInvalid)` benar). Tanpa `TrimSpace`/normalisasi: id dengan spasi dianggap tidak valid.
2. Isolasi (rule 6.10, AC7): `ProjectID` dari request diteruskan apa adanya ke `store.ListSpecs`, yang membatasi `WHERE s.project_id = $1`. Tidak ada jalur kode yang membaca spec tanpa filter itu. Tes isolasi wajib (lihat Test Scenario).
3. Project tanpa spec atau project yang tidak ada (uuid valid) -> `Items` kosong non-nil, tanpa error. Handler ini tidak memeriksa keberadaan project (tugas `get-project`/`check-access`).
4. Tipe request salah (pointer, string, `nil`) -> `bus.Expect` mengembalikan error yang membungkus `bus.ErrBadRequestType`; handler meneruskannya tanpa panic. Request dikirim sebagai nilai (`listspecs.Request{...}`), response dikembalikan sebagai nilai (`listspecs.Response{...}`) agar `bus.CallTyped[listspecs.Response]` cocok.
5. Pemetaan `base.SpecSummary` -> `listspecs.Spec` dilakukan di usecase: `Name, Title, ApiVersion (=APIVersion), OpenapiVersion (=OpenAPIVersion), EndpointCount, UpdatedAt (UTC)`. Tidak membawa `content`, `hash`, `source_path`, `*_by_label` (kontrak sengaja ramping; kolom `content` tidak dibaca karena `ListSpecs` tidak memilihnya).
6. Urutan `Items` = urutan store (`name`). Tanpa paginasi (maks `APISPEC_MAX_SPECS`).
7. Error infra dibungkus `fmt.Errorf("listspecsbus: list specs: %w", err)`. Tanpa log di handler (kasus normal tidak berisik; error dilaporkan lewat error return dan dicatat pemanggil).
8. Tanpa `repository.go`: SQL sudah di `base/store` (dipakai bersama `listspecs`). Usecase menerima interface lokal kecil (`specLister`) yang dipenuhi `*store.Store` (overview 3.2). Tanpa transaksi, `Clock`, `access`, `audit`, `params.go`.
9. Tanpa migrasi, tanpa perubahan OpenAPI/i18n/FE.

## Plan

Urutan wajib. Tulis tes dulu (TDD) per langkah.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F di atas ada. Baca `backend/internal/infra/bus/bus.go` dan `typed.go` untuk signature `Handler`, `Expect`, dan `ErrBadRequestType`; cek `app.Deps.Bus` bertipe `*bus.Registry`. Baca `store.ListSpecs` di F: pastikan filter `project_id`, `ORDER BY` deterministik, dan kembalian `[]` (bukan error) untuk id bukan uuid. Jika ada selisih, laporkan ke pemilik F (jangan ubah store dari feature ini).

**B1. Konstanta** `backend/lib/constants/bus.go` (hanya bila belum ada dari F):

```go
// Provider: T06 project/apispec. Consumer: T09 (discovery/apispectools).
// Handler TIDAK memeriksa akses user; WAJIB membatasi query pada ProjectID request (rule 6.10).
const BUS_PROJECT_APISPEC_LIST_SPECS = "project.apispec.list-specs"
```

Jangan menduplikasi. `lib/constants/bus_test.go` T00 harus tetap lulus.

**B2. Kontrak** `backend/lib/contracts/project/apispec/listspecs/listspecs.go` (hanya bila belum ada dari F; stdlib saja, rule 4.5; persis overview 7.2):

```go
// Package listspecs adalah kontrak bus project.apispec.list-specs (provider T06, consumer T09).
// ProjectID bukan uuid -> error membungkus errs.ErrInvalid. Project tanpa spec -> Items kosong (bukan nil).
// Tipe request salah -> bus.ErrBadRequestType. Request dikirim sebagai nilai.
package listspecs

import "time"

type Request struct{ ProjectID string }

type Response struct{ Items []Spec } // urut Name

type Spec struct {
    Name           string
    Title          string
    ApiVersion     string
    OpenapiVersion string
    EndpointCount  int
    UpdatedAt      time.Time
}
```

**B3. Tes dulu** (`usecase_test.go`, `handler_test.go`; lihat Test Scenario), pastikan gagal.

**B4. `listspecsbus/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package listspecsbus

// specLister dipenuhi *store.Store.
type specLister interface {
    ListSpecs(ctx context.Context, projectID string) ([]base.SpecSummary, error)
}

type Usecase struct{ specs specLister }

func NewUsecase(specs specLister) *Usecase { return &Usecase{specs: specs} }

func (u *Usecase) Execute(ctx context.Context, projectID string) (listspecs.Response, error) {
    if _, err := uuid.Parse(projectID); err != nil {
        return listspecs.Response{}, errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)
    }
    rows, err := u.specs.ListSpecs(ctx, projectID)
    if err != nil {
        return listspecs.Response{}, fmt.Errorf("listspecsbus: list specs: %w", err)
    }
    items := make([]listspecs.Spec, 0, len(rows))
    for _, s := range rows {
        items = append(items, listspecs.Spec{
            Name:           s.Name,
            Title:          s.Title,
            ApiVersion:     s.APIVersion,
            OpenapiVersion: s.OpenAPIVersion,
            EndpointCount:  s.EndpointCount,
            UpdatedAt:      s.UpdatedAt.UTC(),
        })
    }
    return listspecs.Response{Items: items}, nil
}
```

Catatan: `uuid.Parse` menerima beberapa bentuk non-kanonik (`urn:uuid:...`, `{...}`, tanpa tanda hubung). Agar sejalan dengan store (yang menjadi sumber kebenaran "uuid valid"), bandingkan: bila `uuid.Parse` sukses tetapi `store` menganggap kosong, hasilnya tetap `Items` kosong tanpa error (aman, tidak membocorkan data). Bila store memakai aturan lebih ketat, developer menyelaraskan dengan memakai helper yang SAMA yang dipakai store (mis. `base.IsUUID` bila F menyediakannya; periksa dulu, jangan membuat helper baru di luar allowedPaths). Cek `errs.Wrap` signature di `backend/lib/common/errs/errs.go` sebelum menulis (urutan argumen); pastikan `errors.Is(err, errs.ErrInvalid)` benar dan `Code == CodeInvalid`.

**B5. `listspecsbus/handler.go`**

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk project.apispec.list-specs.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[listspecs.Request](req)
    if err != nil {
        return nil, err // membungkus bus.ErrBadRequestType
    }
    return h.uc.Execute(ctx, in.ProjectID) // nilai listspecs.Response, bukan pointer
}
```

Pastikan pada error, nilai kembalian pertama `nil` (bukan `Response{}` kosong), sehingga konsumen tidak keliru membaca respons pada jalur error. Konsekuensi: `return h.uc.Execute(...)` langsung tidak cocok karena mengembalikan struct; tulis eksplisit `res, err := ...; if err != nil { return nil, err }; return res, nil`.

**B6. `listspecsbus/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(store.New(d.DB))
    d.Bus.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, newHandler(uc).Handle)
}
```

Tidak ada route HTTP.

**B7. Wiring.** Tambah tepat satu baris `listspecsbus.Init(d)` di `apispec/base.go` (`Register`). Jangan menyentuh baris feature lain (minimal konflik merge dengan usecase paralel).

**B8. Verifikasi akhir.** `go vet ./...`; `golangci-lint`; archtest `TestT00_AC4_RepoIsClean` (folder `listspecsbus` hanya meng-import `base`, `base/store`, `lib/contracts/project/apispec/listspecs`, `lib/constants`, `lib/common/errs`, `infra/app`, `infra/bus`, `google/uuid`; tidak meng-import feature lain, rule 4.3); `go test -race ./internal/domain/project/internal/apispec/... ./lib/...`; `go test -tags integration` untuk tes integrasi. EXPLAIN `ListSpecs` sudah dicatat di plan `listspecs`; tidak perlu diulang.

## Migration

Tidak ada. Feature ini hanya membaca `api_specs` dan `api_spec_endpoints` dari `backend/migrations/project/<VERSION>_create_api_specs.sql` (milik F).

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/apispec/listspecsbus/init.go`
- `backend/internal/domain/project/internal/apispec/listspecsbus/handler.go`
- `backend/internal/domain/project/internal/apispec/listspecsbus/usecase.go`
- `backend/internal/domain/project/internal/apispec/listspecsbus/usecase_test.go`
- `backend/internal/domain/project/internal/apispec/listspecsbus/handler_test.go`
- `backend/internal/domain/project/internal/apispec/listspecsbus/listspecsbus_integration_test.go` (tag `integration`)
- `backend/lib/contracts/project/apispec/listspecs/listspecs.go` (bila belum dibuat F)

Backend (ubah, minimal):
- `backend/internal/domain/project/internal/apispec/base.go`: satu baris `listspecsbus.Init(d)`.
- `backend/lib/constants/bus.go`: konstanta `BUS_PROJECT_APISPEC_LIST_SPECS` (bila belum ada dari F).

Tidak diubah: `base/*`, feature lain, `infra/**`, `api/**`, frontend, migrasi, `docs/tickets/README.md` (baris katalog `list-specs` sudah ada; catatan kontrak ditangani F).

## Test Scenario

Penamaan: `TestT06_AC7_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `usecase_test.go` (fake `specLister`, tanpa DB)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestExecute_MapsFields` | store mengembalikan satu `SpecSummary` lengkap (APIVersion "1.4.0", OpenAPIVersion "3.0.3", EndpointCount 12) | `Spec{Name, Title, ApiVersion:"1.4.0", OpenapiVersion:"3.0.3", EndpointCount:12, UpdatedAt}` benar; `UpdatedAt` berlokasi UTC |
| `TestExecute_PreservesStoreOrder` | store: `auth, notification, payment` | `Items` persis urutan itu |
| `TestExecute_EmptyReturnsEmptySliceNotNil` | store mengembalikan `nil` dan `[]` | `Items != nil`, `len == 0`, tanpa error |
| `TestExecute_InvalidUUID` | `""`, `"bukan-uuid"`, `"123"`, `"0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e1"` (kurang 1 digit), `" 0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"` (spasi depan) | `errors.Is(err, errs.ErrInvalid)`; `Code == CodeInvalid`; store TIDAK dipanggil (hitung panggilan = 0) |
| `TestExecute_PassesProjectIDVerbatim` | uuid valid huruf kecil | store menerima string yang sama persis |
| `TestExecute_StoreErrorWrapped` | store error | `errors.Is` ke error asli; prefix `listspecsbus: list specs`; BUKAN `ErrInvalid` |
| `TestExecute_DoesNotExposeContent` | `SpecSummary` terisi `SourcePath`/label | `listspecs.Spec` hanya memuat 6 field kontrak (cek via refleksi jumlah field = 6) |

### Unit `handler_test.go` (bus nyata `bus.NewRegistry(log)`, usecase dengan fake)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT06_AC7_Handle_WrongRequestType` | `Handle(ctx, &listspecs.Request{...})` (pointer), `"string"`, `nil`, `listendpoints.Request{}` | error `errors.Is(err, bus.ErrBadRequestType)`; return pertama `nil`; tidak panic; store tidak dipanggil |
| `TestHandle_ReturnsValueNotPointer` | `bus.CallTyped[listspecs.Response](ctx, reg, constants.BUS_PROJECT_APISPEC_LIST_SPECS, listspecs.Request{ProjectID: uuid})` | berhasil (tidak error tipe); `Items` sesuai fake |
| `TestHandle_InvalidProjectID` | lewat `CallTyped` dengan `"x"` | error `errors.Is(err, errs.ErrInvalid)` |
| `TestHandle_ErrorReturnsNilResponse` | usecase error | return pertama `nil` |
| `TestInit_RegistersEndpoint` | `Init(app.Deps{Bus: reg, DB: ...})` | `reg.Call` ke endpoint tidak `ErrNotRegistered` |

### Integration `listspecsbus_integration_test.go` (tag `integration`, `db.TestDB(t)`, migrasi T03 + T06, bus nyata)

Setup: seed 2 project (A, B) dan spec/endpoint langsung lewat `store`/SQL (tidak bergantung pada `uploadspecs`).

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT06_AC7_ListSpecsBus_IsolationBetweenProjects` | A punya `auth`, `payment`; B punya `billing`; nama `shared` ada di keduanya dengan jumlah endpoint berbeda (A: 3, B: 7) | `ProjectID=A` hanya `auth, payment, shared(3)`; `ProjectID=B` hanya `billing, shared(7)`; tidak pernah ada nama milik project lain |
| `TestT06_AC7_ListSpecsBus_UnknownProjectEmpty` | uuid valid yang tidak ada | `Items` kosong non-nil, tanpa error (tidak membocorkan keberadaan project) |
| `TestT06_AC7_ListSpecsBus_NonUUIDIsError` | `ProjectID: "bukan-uuid"` | error `ErrInvalid`; tidak ada error SQL cast (tidak 500) |
| `TestListSpecsBus_OrderedByName` | sisip `payment`, `auth`, `notification` acak | urut `auth, notification, payment` |
| `TestListSpecsBus_EndpointCount` | `auth`: 5 endpoint; `payment`: 0 endpoint | `EndpointCount` 5 dan 0 (spec tanpa endpoint tetap muncul) |
| `TestListSpecsBus_Fields` | spec dengan title "Payment API", api_version "1.4.0", openapi_version "3.1.0" | field kontrak terisi benar; `UpdatedAt` = `updated_at` baris (UTC) |
| `TestListSpecsBus_NoAccessCheckNoAudit` | registry TANPA handler `project.management.check-access` dan `core.audit.record` | panggilan tetap sukses (tidak bergantung keduanya); tidak ada panggilan audit |
| `TestListSpecsBus_AfterRenameAndDelete` | rename lalu hapus spec lewat store | `Items` mengikuti perubahan (tanpa cache) |
| `TestListSpecsBus_LargeContentNotRead` | spec 5 MB | cepat (< 100 ms); tidak ada kolom content terbaca (cukup asersi waktu + ringkas) |

`TestRoutesRegistered` (W) memastikan `Call` ke endpoint ini dengan request valid tidak `ErrNotRegistered`; tidak diulang di sini.

## E2E Scenarios

Usecase ini tidak punya UI atau endpoint HTTP, jadi tidak ada E2E Playwright sendiri. Perilakunya diverifikasi E2E tidak langsung lewat consumer T09 (setelah T07/T09 ada) dan skenario backend end-to-end berikut (dijalankan di integrasi lintas feature `apispec_integration_test.go` bila ada):

Positif:
1. Setelah `uploadspecs` mengunggah `auth.yaml`, `payment.yaml`, `notification.yaml` ke project A, `bus.Call(project.apispec.list-specs, {ProjectID: A})` mengembalikan tiga item urut nama dengan judul, versi, dan jumlah endpoint sesuai hasil upload.
2. Setelah `payment.yaml` diunggah ulang dengan endpoint lebih sedikit, `EndpointCount` dan `UpdatedAt` item `payment` ikut berubah.
3. Setelah `renamespec` `payment` -> `payment-v2`, daftar memuat `payment-v2`, bukan `payment`; setelah `deletespec`, item hilang.
4. Project baru tanpa spec -> `Items` kosong.
5. (Setelah T09 ada) Tool agent `list_specs` dengan key project A menampilkan spec project A saja.

Negatif:
1. `ProjectID` project B dipakai: tidak ada nama spec project A di respons (AC7).
2. `ProjectID` bukan uuid: error `ErrInvalid` (T09 menerjemahkan menjadi pesan tool, bukan 500).
3. Request bertipe salah: `ErrBadRequestType`, tidak panic, server tetap melayani request lain.
4. DB tidak tersedia (di-stub): error terbungkus `listspecsbus: list specs`, bukan `Items` kosong palsu.

## Request/Response body

Tidak ada JSON di wire (bus in-process); berikut nilai Go.

### Request

```go
listspecs.Request{ProjectID: "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"}
```

Pemanggil:

```go
res, err := bus.CallTyped[listspecs.Response](ctx, reg,
    constants.BUS_PROJECT_APISPEC_LIST_SPECS,
    listspecs.Request{ProjectID: projectID})
```

### Response sukses

```go
listspecs.Response{Items: []listspecs.Spec{
    {Name: "auth",    Title: "Auth API",    ApiVersion: "2.0.0", OpenapiVersion: "3.1.0", EndpointCount: 5,  UpdatedAt: time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)},
    {Name: "payment", Title: "Payment API", ApiVersion: "1.4.0", OpenapiVersion: "3.0.3", EndpointCount: 12, UpdatedAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)},
}}
```

Project tanpa spec (atau uuid valid yang tidak ada):

```go
listspecs.Response{Items: []listspecs.Spec{}} // bukan nil
```

### Error (nilai Go, bukan JSON HTTP)

| Kondisi | Error |
|---|---|
| `ProjectID` bukan uuid | `*errs.Error` `Code == CodeInvalid`, pesan "project_id tidak valid", `errors.Is(err, errs.ErrInvalid) == true` |
| Tipe request salah (pointer/string/nil/kontrak lain) | error yang membungkus `bus.ErrBadRequestType` (`errors.Is` benar) |
| Error DB | `fmt.Errorf("listspecsbus: list specs: %w", dbErr)` (bukan `ErrInvalid`) |

Konsumen (T09) memetakan `ErrInvalid` menjadi pesan tool yang jelas dan error lain menjadi kegagalan generik; tidak ada format error HTTP karena endpoint ini tidak terekspos lewat HTTP. Bila suatu saat error ini sampai ke `respond.WriteError`, bentuknya mengikuti format seragam:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "project_id tidak valid" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
