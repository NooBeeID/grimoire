# T06 project/api-spec — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T06-project-api-spec.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/project-management/overview.md` (plan T03: pola module, D1 contentcount, D4 route, D5 audit, D6 identitas).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. Tidak ada knowledge doc module apispec. Di `main` (`11838ec`): T00, T01, T02 sudah merge; T03 baru sampai plan (`backend/internal/domain/project/base.go` masih no-op, `backend/lib/contracts/project/` belum ada, `backend/migrations/project/` kosong). Konstanta `BUS_CORE_AUDIT_RECORD` dan aksi `record.ActionAPISpecUploaded|Updated|Deleted` SUDAH ada di `main`.

Dokumen ini adalah plan FOUNDATION untuk module `project/apispec`. Isinya keputusan bersama, `base/` (entity, parser OpenAPI, subpackage I/O), kontrak bus, migrasi, konfigurasi, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-api-spec/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila plan usecase bertentangan dengan dokumen ini, dokumen ini yang menang sampai diperbarui. Frontend ditulis di `docs/superpowers/plans/project-api-spec/frontend-plan.md` (terpisah).

---

## 0. Prasyarat dan nama infra yang dipakai

- T06 bergantung pada T03: FK `projects(id)`, bus `project.management.check-access`, konstanta `BUS_PROJECT_APISPEC_COUNT_SPECS`, dan kontrak `lib/contracts/project/management/contentcount`. Bila T03 belum merge saat T06 dikerjakan, file berikut dibuat PERSIS seperti plan T03 5.1-5.4 (aturan "yang merge lebih dulu membuat kontrak"): konstanta `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`, `BUS_PROJECT_APISPEC_COUNT_SPECS` (beserta dua konstanta count saudaranya agar blok T03 identik), `lib/contracts/project/management/checkaccess/checkaccess.go`, `lib/contracts/project/management/contentcount/contentcount.go`. Migrasi `projects` TIDAK dibuat T06: migrasi T06 butuh tabel `projects`, jadi T06 tidak boleh merge sebelum migrasi T03 ada di `main` (lihat risiko 16.1).
- Signature publik T00-T03 TIDAK diubah. T06 hanya MENAMBAH konstanta, kontrak, migrasi, module, config, dan OpenAPI.
- Nama infra di `main`:

| Simbol | Import path | Package |
|---|---|---|
| `Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` | `grimoire/internal/infra/app` | `app` |
| `Registry.Register`, `Registry.Call`, `NewRegistry`, `CallTyped[T]`, `Expect[T]`, `Handler`, `ErrNotRegistered`, `ErrBadRequestType` | `grimoire/internal/infra/bus` | `bus` |
| `WriteJSON`, `WriteError`, `DecodeJSON`, `StatusFor` | `grimoire/internal/infra/http/respond` | `respond` |
| `RequireAuth()`, `BodyLimit(n)`, `KeyByIP(r)` | `grimoire/internal/infra/middleware` | `middleware` |
| `DB` (embed `*sqlx.DB`), `DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `Querier`, `TestDB(t)` | `grimoire/internal/infra/db` | `db` |
| `New`, `Wrap`, `WithDetails`, `Error`, `CodeNotFound`, `CodeConflict`, `CodeInvalid`, `CodePayloadTooLarge`, `CodeUnauthorized`, `ErrInvalid` | `grimoire/lib/common/errs` | `errs` |
| `IdentityFrom(ctx)` (`Identity{UserID, Email, Name, Role}`), `TraceID(ctx)` | `grimoire/lib/common/reqctx` | `reqctx` |
| `Request`, `Response`, `ActorUser`, `StatusOK`, `ActionAPISpecUploaded`, `ActionAPISpecUpdated`, `ActionAPISpecDeleted` | `grimoire/lib/contracts/core/audit/record` | `record` |
| `Config.MaxUploadBytes`, `Config.MaxUploadFiles` (sudah ada) | `grimoire/internal/infra/config` | `config` |

- Tidak ada kode error baru. Semua kasus memakai `NOT_FOUND`, `CONFLICT`, `INVALID_ARGUMENT`, `PAYLOAD_TOO_LARGE`, dibedakan lewat `details.reason`.
- Satu library baru: `go.yaml.in/yaml/v3` (keputusan D2).
- Dua env var baru: `APISPEC_MAX_FILE_BYTES`, `APISPEC_MAX_SPECS` (bagian 10).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (backend Go di plan ini; frontend di `frontend-plan.md`) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Versi kontrak REST | Tanpa prefix versi di URL (prefix `/api`). `api/modules/project/apispec.yml` memakai `info.version: 0.1.0` |
| Versi kontrak bus | v1 (endpoint baru) |
| Bergantung pada | T00, T01, T02 (merged), T03 (plan; dibutuhkan sebelum merge) |
| Dibutuhkan oleh | T09 (`list-specs`, `list-endpoints`, `get-endpoint`), T03 (`count-specs`) |

Endpoint HTTP (semua di `d.API`, semua `middleware.RequireAuth()`, mutasi wajib header CSRF `X-Requested-With: grimoire` dari T00):

| Endpoint | Method | Version | Siapa | Usecase |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-specs/upload` | POST (multipart) | - | anggota, super_admin | `uploadspecs` |
| `/api/projects/{projectID}/api-specs/validate` | POST (multipart) | - | anggota, super_admin | `validatespec` |
| `/api/projects/{projectID}/api-specs` | GET | - | anggota, super_admin | `listspecs` |
| `/api/projects/{projectID}/api-specs/{name}` | GET | - | anggota, super_admin | `getspec` |
| `/api/projects/{projectID}/api-specs/{name}/raw` | GET | - | anggota, super_admin | `downloadspec` |
| `/api/projects/{projectID}/api-specs/{name}` | PATCH | - | anggota, super_admin | `renamespec` |
| `/api/projects/{projectID}/api-specs/{name}` | DELETE | - | anggota, super_admin | `deletespec` |

Endpoint bus (provider T06):

| Endpoint | Mode | Version | Consumer | Usecase |
|---|---|---|---|---|
| `project.apispec.list-specs` | Call | v1 | T09 | `listspecsbus` |
| `project.apispec.list-endpoints` | Call | v1 | T09 | `listendpoints` |
| `project.apispec.get-endpoint` | Call | v1 | T09 | `getendpoint` |
| `project.apispec.count-specs` | Call | v1 (kontrak T03 `contentcount`) | T03 | `countspecs` |

Endpoint bus yang dikonsumsi:

| Endpoint | Mode | Provider | Dipakai oleh |
|---|---|---|---|
| `project.management.check-access` | Call (fail-closed) | T03 | 7 usecase HTTP (lewat `base/access`) |
| `core.audit.record` | Call (fail-open, setelah commit) | T02 | `uploadspecs`, `renamespec`, `deletespec` (lewat `base/audit`) |

`core.auth.get-user` TIDAK dikonsumsi (D6).

---

## 2. Summary Plan

T06 menyimpan spesifikasi OpenAPI 3.0/3.1 (YAML/JSON) per project, memvalidasinya tanpa pernah melakukan fetch, mengindeks endpoint-nya ke tabel agar daftar endpoint cepat, menyajikan dokumen terparsir untuk dirender web, dan menyediakan akses per-endpoint yang hemat token untuk agent (T09) lewat bus.

Isinya:
- Dua tabel milik module (`api_specs`, `api_spec_endpoints`) dalam satu migrasi `migrations/project/`.
- `base/` (murni): entity `Spec`, `SpecSummary`, `Endpoint`; rule nama (`ValidateName`, `DeriveName`), path sumber, format, keputusan upload (`DecideUpload`), status per file, DTO, error.
- `base/openapi/` (murni, CPU only): parser YAML/JSON dengan penjaga billion-laughs, validator struktural OpenAPI 3.0/3.1, pemeriksa `$ref` (internal harus terselesaikan, eksternal DITOLAK tanpa fetch), ekstraksi endpoint, resolver operasi untuk `get-endpoint`, pencarian mirip (Levenshtein).
- Subpackage I/O bersama: `base/store` (SQL bersama), `base/access` (check-access via bus, fail-closed), `base/audit` (audit via bus, fail-open), `base/upload` (pembaca multipart streaming).
- 11 feature: 7 HTTP + 4 handler bus.
- Tiga kontrak bus baru (`listspecs`, `listendpoints`, `getendpoint`) dan provider untuk kontrak T03 `contentcount`.
- Dua env var, OpenAPI `api/modules/project/apispec.yml`, pembaruan README (katalog bus) dan tiket T06.

Hal penting soal keamanan:
- Parser tidak pernah mengakses jaringan atau filesystem: package `base/openapi` tidak meng-import `net`, `net/http`, `os`, `io/fs` (dibuktikan tes). `$ref` yang tidak diawali `#` ditolak dengan issue `EXTERNAL_REF`.
- YAML dibatasi kedalaman, jumlah alias, dan ukuran setelah ekspansi alias SEBELUM dikonversi (billion laughs).
- Non-anggota mendapat 404 di 7 route, dicek SEBELUM spec dicari. Handler bus selalu membatasi query pada `ProjectID` dari request.
- Isi spec dikirim sebagai data JSON, tidak pernah HTML; sanitasi Markdown di FE.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Keputusan baru (dicatat di PR; README dan tiket diperbarui di PR yang sama)

| # | Keputusan | Alasan | Tindakan dokumen |
|---|---|---|---|
| D1 | Feature `busreader` di tiket dipecah menjadi `listspecsbus`, `listendpoints`, `getendpoint`, ditambah `countspecs` (provider kontrak T03 D1) | Rule 3.4: satu feature = satu handler bus | Ubah tabel Features di T06 |
| D2 | Tidak memakai library OpenAPI (kin-openapi/libopenapi). Parser sendiri di `base/openapi` di atas `go.yaml.in/yaml/v3` | Kedua library punya loader `$ref` eksternal dan dukungan 3.1 berbeda; parser sendiri lebih mudah dibuktikan tanpa fetch (rule 7.10) dan cukup untuk validasi struktural. `go.yaml.in/yaml/v3` adalah fork resmi terpelihara `gopkg.in/yaml.v3` (rule 15.4) | Alasan ditulis di deskripsi PR |
| D3 | `$ref` eksternal (URL, file, relatif) DITOLAK (`rejected`, issue `EXTERNAL_REF`), bukan dibiarkan | Tiket memberi pilihan; menolak membuat `get-endpoint` selalu lengkap dan pesan ke user jelas | Kalimat di T06 "Validasi" dipertegas |
| D4 | Dokumen terparsir memakai map berurutan (`openapi.Map`) yang menjaga urutan kunci asli saat di-marshal ke JSON | `map[string]any` Go mengurutkan kunci alfabetis saat marshal; dokumentasi harus mengikuti urutan path/properti di spec | - |
| D5 | Upload diproses per file dalam transaksi sendiri; serialisasi per project memakai `pg_advisory_xact_lock(hashtextextended('apispec:'||project_id, 0))` | Rule 11.3 (gagal sebagian tidak membatalkan lainnya); batas `APISPEC_MAX_SPECS` harus akurat di bawah upload paralel tanpa mengunci tabel `projects` milik management (rule 2.4) | - |
| D6 | Actor dari `reqctx.IdentityFrom(ctx)`: `ID=UserID`, `Label=Name` (fallback `Email` bila `Name` kosong). Tidak memanggil `core.auth.get-user` | Pola T03 D6: middleware Session sudah menolak user nonaktif; label snapshot dari identitas | README: hapus T06 dari consumer `core.auth.get-user`; T06 "Dikonsumsi" diperbarui |
| D7 | Respons per file memakai `reason_code` (stabil, untuk i18n FE) di samping `reason` (teks Indonesia, bisa disalin) | Rule 12.8: UI lewat i18n; tiket meminta alasan yang bisa disalin | - |
| D8 | UTF-8 BOM di awal file dibuang sebelum parse dan simpan; hash dihitung dari konten yang disimpan. Konten dengan byte NUL ditolak | `encoding/json` menolak BOM; Postgres `text` menolak NUL | - |

### 3.2 Struktur module

```
backend/internal/domain/project/
  base.go                          # ubah: Register memanggil management.Register(d) (T03) dan apispec.Register(d)
  internal/apispec/
    base.go                        # package apispec: Register(d app.Deps) memanggil 11 <feature>.Init(d)
    apispec_integration_test.go    # (W) tes lintas feature, tag integration
    routes_test.go                 # (W) TestRoutesRegistered + tes urutan route chi
    base/                          # package base: entity + rule, TANPA I/O, TANPA context
      spec.go       spec_test.go
      name.go       name_test.go
      upload.go     upload_test.go
      errors.go     errors_test.go
      dto.go        dto_test.go
      actor.go  meta.go  clock.go
    base/openapi/                  # package openapi: parser/validator/resolver MURNI
      limits.go  issue.go  map.go  yamlguard.go  parse.go  validate.go  refs.go
      extract.go resolve.go similar.go tojson.go  pointer.go
      *_test.go  imports_test.go   testdata/
    base/store/                    # package store: SQL bersama
      store.go  store_integration_test.go
    base/access/                   # package access: Guard via bus check-access (fail-closed)
      access.go  access_test.go
    base/audit/                    # package audit: Recorder via bus core.audit.record (fail-open)
      audit.go   audit_test.go
    base/upload/                   # package upload: pembaca multipart streaming
      upload.go  upload_test.go
    uploadspecs/ validatespec/ listspecs/ getspec/ downloadspec/ renamespec/ deletespec/
    listspecsbus/ listendpoints/ getendpoint/ countspecs/
      init.go handler.go [params.go] usecase.go [repository.go] *_test.go
```

Aturan per feature:
- `init.go`: `func Init(d app.Deps)`. Merakit `store.New(d.DB)` -> repository feature -> usecase (menerima interface) -> handler, lalu mendaftarkan route atau handler bus feature itu sendiri.
- Feature bus (`listspecsbus`, `listendpoints`, `getendpoint`, `countspecs`) tanpa `params.go`; `handler.go` memanggil `bus.Expect[<kontrak>.Request](req)` (rule 6.9).
- Feature tanpa SQL sendiri (`validatespec`) tanpa `repository.go`. Feature yang hanya memakai fungsi store boleh memakai `*store.Store` lewat interface lokal di `usecase.go`, tanpa `repository.go`.
- Usecase menerima interface: repository/store, `access.Checker`, `audit.Recorder`, `base.Clock`. Bukan tipe konkret (rule 5.6).
- Feature DILARANG saling import (rule 4.3). Yang dipakai bersama lewat `base` dan `base/*`. Arah import: `base/openapi|store|access|audit|upload` -> `base`; `base` tidak meng-import subpackage-nya. `base/openapi` hanya boleh meng-import stdlib (selain larangan di 3.7), `go.yaml.in/yaml/v3`, dan `base` (untuk `base.Endpoint`).
- `BeginTx` dipanggil dari repository feature (`func (r *Repo) InTx(ctx, fn func(*Repo) error) error`), bukan dari usecase. Repository feature membungkus `*store.Store` (`r.s.WithTx(tx)` di dalam tx).

### 3.3 Konvensi handler, actor, request meta, log

- Handler membaca `chi.URLParam(r, "projectID")` dan `chi.URLParam(r, "name")` apa adanya. Tidak memvalidasi uuid di handler; store mengembalikan "tidak ketemu" untuk id bukan uuid tanpa query, dan check-access menjawab `ProjectExists=false` -> 404 yang sama.
- `base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`.
- `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")` (seharusnya mustahil karena `RequireAuth`).
- JSON body (hanya `renamespec`) di-decode dengan `respond.DecodeJSON` (field tak dikenal -> 400).
- Error infra dibungkus `fmt.Errorf("<feature>: <langkah>: %w", err)` -> 500 generik. Error bisnis selalu `*errs.Error` dari `base/errors.go`.
- Log: tidak pernah me-log isi spec, `Args` audit, atau body. Field log standar: `project_id`, `spec`, `feature`.

### 3.4 Akses (`base/access`, dipakai 7 usecase HTTP)

Urutan wajib di setiap usecase HTTP:
1. `actor` (3.3).
2. `if err := guard.RequireMember(ctx, projectID); err != nil { return err }`.
3. Baru setelah itu mencari spec. Spec tidak ada -> `base.ErrSpecNotFound` (404). Urutan ini mencegah non-anggota membedakan "project ada tapi spec tidak ada".

Perilaku `RequireMember`:

| Kondisi | Hasil |
|---|---|
| Identitas tidak ada di ctx | `errs.New(CodeUnauthorized, "perlu login")` |
| Call check-access error (termasuk `ErrNotRegistered`) | error terbungkus `access: check-access: %w` -> 500 (fail-closed) |
| `!ProjectExists` atau `Level == none` | `base.ErrProjectNotFound` (404, rule 7.3; termasuk id bukan uuid) |
| `member`, `owner`, `admin` | `nil` (semua aksi T06 cukup level member) |

Handler bus TIDAK memanggil check-access (pemanggil T09 sudah terikat key -> project lewat T07), tetapi WAJIB memfilter query dengan `ProjectID` request (rule 6.10).

### 3.5 Upload dan konkurensi (D5)

Alur `uploadspecs` (rinci di plan usecase), keputusan yang mengikat:
- Semua part multipart dibaca dulu (`upload.Read`) sampai body habis; baru setelah itu diproses. Bila body melebihi `MAX_UPLOAD_BYTES` di tengah jalan, seluruh request 413 dan tidak ada yang tersimpan.
- Parsing dan validasi dilakukan SEBELUM transaksi.
- Per file, satu transaksi: `LockProjectSpecs` -> `LockSpec(name)` -> `CountSpecs` -> `base.DecideUpload` -> INSERT atau UPDATE `api_specs` -> `ReplaceEndpoints`. Unchanged/RejectLimit -> rollback tanpa tulis.
- Nama ganda dalam satu request: file pertama (urutan part) diproses, berikutnya `rejected` reason_code `DUPLICATE_NAME_IN_UPLOAD`.
- Audit setelah commit, satu entri per file `created`/`updated`.
- Panggilan bus tidak pernah dilakukan di dalam tx.

### 3.6 Paginasi dan urutan

- `listspecs` (HTTP dan bus): tanpa paginasi (maks `APISPEC_MAX_SPECS`); urut `name`.
- Urutan endpoint kanonik di semua tempat: `spec name`, `path`, lalu method dengan urutan `GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE`. SQL: `array_position(ARRAY['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'], e.method)`. Go: `base.MethodOrder(method) int`.
- `list-endpoints`: `Limit` default 100, maks 300, `Offset` >= 0 (clamp, bukan error).

### 3.7 Batas parser tanpa fetch (rule 7.10)

- `base/openapi` DILARANG meng-import `net`, `net/http`, `net/url`, `os`, `io/fs`, `io/ioutil`, `path/filepath`, `os/exec`, `plugin`. Ditegakkan `imports_test.go` (go/parser atas file non-test di package itu).
- `$ref` non-`#` tidak pernah di-resolve; hanya dilaporkan.

---

## 4. Entity dan rule (`base/`, package `base`)

Semua fungsi murni (tanpa context, tanpa I/O). Diuji unit tanpa mock (rule 13.1). Import yang diizinkan: stdlib, `lib/common/errs`, `lib/common/reqctx` (tipe `Identity`).

### 4.1 `base/spec.go`

```go
const (
    FormatYAML = "yaml"
    FormatJSON = "json"
)

type Spec struct {
    ID, ProjectID, Name string
    SourcePath          *string
    Title, APIVersion, OpenAPIVersion string
    Content             string   // teks asli (tanpa BOM, D8)
    Format              string   // FormatYAML | FormatJSON
    Hash                [32]byte // SHA-256 dari Content
    SizeBytes           int      // len(Content)
    CreatedAt           time.Time
    CreatedBy           string
    CreatedByLabel      string
    UpdatedAt           time.Time
    UpdatedBy           string
    UpdatedByLabel      string
}

type SpecSummary struct {   // Spec tanpa Content/Hash + jumlah endpoint
    ID, ProjectID, Name string
    SourcePath          *string
    Title, APIVersion, OpenAPIVersion, Format string
    SizeBytes           int
    EndpointCount       int
    CreatedAt           time.Time
    CreatedByLabel      string
    UpdatedAt           time.Time
    UpdatedByLabel      string
}

type Endpoint struct {
    Method      string   // uppercase, salah satu Methods
    Path        string
    OperationID *string
    Summary     string   // dipotong 300 rune
    Tags        []string // dedup, urutan asli; tidak pernah nil (pakai []string{})
    Deprecated  bool
}

var Methods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}
func ValidMethod(m string) bool        // m sudah uppercase
func MethodOrder(m string) int         // indeks di Methods; tidak dikenal -> len(Methods)
func SortEndpoints(es []Endpoint)      // path, lalu MethodOrder

// NewSpec membangun Spec baru dari hasil parse. now di-UTC; Hash/SizeBytes dihitung dari content.
func NewSpec(projectID, name string, sourcePath *string, meta DocMeta, content, format string, actor Actor, now time.Time) Spec
// ApplyContent menimpa isi (dipakai saat updated); CreatedAt/By tetap, Updated* diisi actor/now.
func (s *Spec) ApplyContent(sourcePath *string, meta DocMeta, content, format string, actor Actor, now time.Time)
// Rename mengganti nama + Updated*; mengembalikan false bila nama sama (no-op).
func (s *Spec) Rename(newName string, actor Actor, now time.Time) bool
func HashContent(content string) [32]byte

type DocMeta struct{ Title, APIVersion, OpenAPIVersion string } // diisi dari openapi.Document
```

### 4.2 `base/name.go`

```go
const (
    NameMaxLen       = 80
    SourcePathMaxLen = 512
)
// ValidateName: ^[a-z0-9._-]{1,80}$ dan bukan "." / ".." -> nil, selain itu ErrInvalidName.
func ValidateName(s string) error
// DeriveName: basename (setelah "\"->"/"), buang ekstensi terakhir, lowercase, run karakter di luar
// [a-z0-9._-] -> "-", trim "-" dan "." di kedua sisi, potong 80 byte lalu trim lagi. Hasil kosong/"."/".." -> ErrInvalidName.
// Contoh: "specs/Payment API.v2.yaml" -> "payment-api.v2"; "Ünïcode.yaml" -> "n-code"; ".yaml" -> ErrInvalidName.
func DeriveName(fileName string) (string, error)
// NormalizeSourcePath: "\"->"/", buang "./" di depan, gabungkan "/" ganda. Tolak (ErrInvalidPath):
// kosong, diawali "/", huruf drive "C:", segmen "..", byte NUL / karakter kontrol, > 512 byte, bukan UTF-8 valid.
func NormalizeSourcePath(raw string) (string, error)
// FormatFromExt (case-insensitive): .yaml/.yml -> yaml; .json -> json; lainnya -> ok=false (skipped).
func FormatFromExt(fileName string) (format string, ok bool)
```

### 4.3 `base/upload.go`

```go
type FileStatus string
const (
    StatusCreated   FileStatus = "created"
    StatusUpdated   FileStatus = "updated"
    StatusUnchanged FileStatus = "unchanged"
    StatusSkipped   FileStatus = "skipped"
    StatusRejected  FileStatus = "rejected"
    StatusValid     FileStatus = "valid" // hanya validatespec
)

// reason_code per file (D7). Pesan Indonesia di Reasons[code] (fmt untuk SPEC_LIMIT_REACHED).
const (
    ReasonNotSpecFile            = "NOT_SPEC_FILE"            // skipped  "bukan file spec (.yaml/.yml/.json)"
    ReasonFileTooLarge           = "FILE_TOO_LARGE"           // rejected "ukuran file melebihi batas %d byte"
    ReasonTooManyFiles           = "TOO_MANY_FILES"           // rejected "melebihi batas jumlah file"
    ReasonInvalidPath            = "INVALID_PATH"             // rejected "path tidak valid"
    ReasonInvalidName            = "INVALID_NAME"             // rejected "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter"
    ReasonDuplicateNameInUpload  = "DUPLICATE_NAME_IN_UPLOAD" // rejected "nama duplikat dalam upload"
    ReasonInvalidSpec            = "INVALID_SPEC"             // rejected pesan = issue pertama; issues[] lengkap
    ReasonSpecLimitReached       = "SPEC_LIMIT_REACHED"       // rejected "batas %d spec per project tercapai"
)
func ReasonMessage(code string, args ...any) string

type UploadDecision int
const (
    DecisionCreate UploadDecision = iota + 1
    DecisionUpdate
    DecisionUnchanged
    DecisionRejectLimit
)
// DecideUpload (dicek berurutan):
//   existing != nil && existing.Hash == newHash -> Unchanged
//   existing != nil                             -> Update (tidak terkena batas)
//   specCount >= maxSpecs                       -> RejectLimit
//   lainnya                                     -> Create
func DecideUpload(existing *Spec, newHash [32]byte, specCount, maxSpecs int) UploadDecision

type UploadLimits struct{ MaxFileBytes int64; MaxFiles int; MaxSpecs int }

type Issue struct {      // salinan netral dari openapi.Issue untuk DTO
    Code    string
    Message string
    Pointer string // "" bila tidak ada
    Line    int    // 0 bila tidak diketahui
}

type FileResult struct {
    File           string     // nama file (basename) dari Content-Disposition
    SourcePath     *string    // path ternormalisasi; nil bila tidak valid
    Name           string     // "" bila belum bisa ditentukan
    Status         FileStatus
    ReasonCode     string
    Reason         string
    Issues         []Issue    // tidak pernah nil
    OpenAPIVersion string
    Title          string
    APIVersion     string
    EndpointCount  int
}

type UploadSummary struct{ Created, Updated, Unchanged, Skipped, Rejected int }
func Summarize(rs []FileResult) UploadSummary
```

### 4.4 `base/errors.go`

```go
const (
    ReasonNameTaken             = "NAME_TAKEN"
    ReasonNameWithMultipleFiles = "NAME_WITH_MULTIPLE_FILES"
    ReasonNoFiles               = "NO_FILES"
    ReasonNotMultipart          = "NOT_MULTIPART"
    // ReasonInvalidName (dari upload.go) juga dipakai untuk 400 renamespec.
)

var (
    ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
    ErrSpecNotFound    = errs.New(errs.CodeNotFound, "spec tidak ditemukan")
    ErrInvalidName     = errs.WithDetails(errs.CodeInvalid,
        "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter",
        map[string]any{"reason": ReasonInvalidName, "fields": map[string]string{"name": "nama spec tidak valid"}})
    ErrInvalidPath     = errs.New(errs.CodeInvalid, "path tidak valid") // hanya dipakai internal -> FileResult
    ErrNameTaken       = errs.WithDetails(errs.CodeConflict, "nama spec sudah dipakai di project ini",
        map[string]any{"reason": ReasonNameTaken})
    ErrNoFiles         = errs.WithDetails(errs.CodeInvalid, "tidak ada file",
        map[string]any{"reason": ReasonNoFiles})
    ErrNameWithMultipleFiles = errs.WithDetails(errs.CodeInvalid,
        "field name hanya boleh dipakai saat mengunggah tepat satu file",
        map[string]any{"reason": ReasonNameWithMultipleFiles})
    ErrNotMultipart    = errs.WithDetails(errs.CodeInvalid, "request harus multipart/form-data",
        map[string]any{"reason": ReasonNotMultipart})
    ErrPayloadTooLarge = errs.New(errs.CodePayloadTooLarge, "ukuran upload melebihi batas")
)
```

Sentinel dipakai bersama (pointer sama). Usecase TIDAK boleh memodifikasinya. Test memakai `errors.Is` atau memeriksa `Code` + `details.reason`.

### 4.5 `base/actor.go`, `base/meta.go`, `base/clock.go`

```go
type Actor struct{ ID, Label string }
// ActorFrom: ok=false atau UserID kosong -> false. Label = Name; Name kosong -> Email (D6).
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool)

type RequestMeta struct{ IP, UserAgent string }
type Clock func() time.Time // produksi: func() time.Time { return time.Now().UTC() }
```

### 4.6 `base/dto.go`

```go
type SpecDTO struct {
    Name           string    `json:"name"`
    Title          string    `json:"title"`
    OpenAPIVersion string    `json:"openapi_version"`
    APIVersion     string    `json:"api_version"`
    Format         string    `json:"format"`
    SourcePath     *string   `json:"source_path"`
    SizeBytes      int       `json:"size_bytes"`
    EndpointCount  int       `json:"endpoint_count"`
    CreatedAt      time.Time `json:"created_at"`
    CreatedByLabel string    `json:"created_by_label"`
    UpdatedAt      time.Time `json:"updated_at"`
    UpdatedByLabel string    `json:"updated_by_label"`
}
type EndpointDTO struct {
    Method      string   `json:"method"`
    Path        string   `json:"path"`
    OperationID *string  `json:"operation_id"`
    Summary     string   `json:"summary"`
    Tags        []string `json:"tags"`
    Deprecated  bool     `json:"deprecated"`
}
type IssueDTO struct {
    Code    string  `json:"code"`
    Message string  `json:"message"`
    Pointer *string `json:"pointer"` // null bila ""
    Line    *int    `json:"line"`    // null bila 0
}
type FileResultDTO struct {
    File           string     `json:"file"`
    SourcePath     *string    `json:"source_path"`
    Name           *string    `json:"name"`            // null bila ""
    Status         FileStatus `json:"status"`
    ReasonCode     *string    `json:"reason_code"`     // null bila ""
    Reason         *string    `json:"reason"`
    Issues         []IssueDTO `json:"issues"`
    OpenAPIVersion *string    `json:"openapi_version"`
    Title          *string    `json:"title"`
    APIVersion     *string    `json:"api_version"`
    EndpointCount  *int       `json:"endpoint_count"`  // null bila skipped/rejected sebelum parse
}
type UploadSummaryDTO struct {
    Created   int `json:"created"`
    Updated   int `json:"updated"`
    Unchanged int `json:"unchanged"`
    Skipped   int `json:"skipped"`
    Rejected  int `json:"rejected"`
}

type ValidateSummaryDTO struct { // dipakai validatespec
    Valid    int `json:"valid"`
    Skipped  int `json:"skipped"`
    Rejected int `json:"rejected"`
}

func ToSummaryDTO(s SpecSummary) SpecDTO
func ToEndpointDTOs(es []Endpoint) []EndpointDTO          // tidak pernah nil
func ToFileResultDTO(r FileResult) FileResultDTO
```

Waktu di JSON selalu UTC RFC3339 (store `.UTC()`).

---

## 5. Parser OpenAPI (`base/openapi/`, package `openapi`)

Murni: tanpa context, tanpa I/O, tanpa goroutine. Input selalu `[]byte` sudah di memori (maks `APISPEC_MAX_FILE_BYTES`).

### 5.1 `limits.go`, `issue.go`, `map.go`, `pointer.go`

```go
type Limits struct{ MaxDepth, MaxAliases, MaxNodes int }
var DefaultLimits = Limits{MaxDepth: 64, MaxAliases: 100, MaxNodes: 500_000}
const MaxIssues = 20

type Format string // "yaml" | "json"

const (
    CodeInvalidEncoding   = "INVALID_ENCODING"   // bukan UTF-8 valid / ada NUL
    CodeEmpty             = "EMPTY"              // kosong / hanya whitespace
    CodeSyntax            = "SYNTAX"             // YAML/JSON rusak (dengan baris)
    CodeMultiDocument     = "MULTI_DOCUMENT"     // YAML > 1 dokumen
    CodeYAMLLimit         = "YAML_LIMIT"         // "dokumen YAML terlalu kompleks (alias/kedalaman)"
    CodeNotOpenAPI        = "NOT_OPENAPI"        // "bukan dokumen OpenAPI"
    CodeNotOpenAPI3       = "NOT_OPENAPI_3"      // "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1"
    CodeUnsupportedVersion= "UNSUPPORTED_VERSION"// "versi OpenAPI %q tidak didukung (hanya 3.0.x/3.1.x)"
    CodeInvalidStructure  = "INVALID_STRUCTURE"  // tipe/field wajib salah
    CodeDuplicateOpID     = "DUPLICATE_OPERATION_ID"
    CodeUnresolvedRef     = "UNRESOLVED_REF"     // "referensi tidak ditemukan: <ref>"
    CodeExternalRef       = "EXTERNAL_REF"       // "referensi eksternal tidak didukung dan tidak di-fetch: <ref>"
)

type Issue struct {
    Code, Message, Pointer string // Pointer JSON pointer RFC 6901 berawalan "#", "" bila tidak relevan
    Line                   int    // 1-based, 0 bila tidak diketahui
}
// Message lengkap menyebut lokasi: "<pesan> (baris 12, #/paths/~1pets/get)"; bagian yang kosong dihilangkan.

// Map: map berurutan (D4). Values: string, json.Number, float64, bool, nil, *Map, []any.
type Map struct{ keys []string; vals map[string]any }
func (m *Map) Get(k string) (any, bool)
func (m *Map) Keys() []string
func (m *Map) Len() int
func (m *Map) MarshalJSON() ([]byte, error) // urutan kunci asli, escape seperti encoding/json

func EscapePointerToken(s string) string     // "~"->"~0", "/"->"~1"
func ResolvePointer(root any, ref string) (any, bool) // ref "#/a/b"; "#" = root; token di-unescape; index array numerik
```

### 5.2 `yamlguard.go`

```go
// Guard berjalan di *yaml.Node SEBELUM konversi. Mengembalikan Issue YAML_LIMIT bila:
//   - kedalaman node > MaxDepth;
//   - jumlah AliasNode > MaxAliases;
//   - total node SETELAH ekspansi alias > MaxNodes, dihitung dengan memo ukuran subtree per anchor
//     (biaya O(node asli));
//   - alias merujuk anchor yang sedang dikunjungi (siklus).
func guard(n *yaml.Node, lim Limits) *Issue
```

go-yaml v3 juga punya batas rasio alias internal saat decode ke nilai Go; itu lapis kedua, bukan andalan (decode ke `yaml.Node` tidak mengekspansi alias).

### 5.3 `parse.go`

```go
type Document struct {
    OpenAPIVersion string // "3.0.3" / "3.1.0"
    Title          string
    APIVersion     string
    Root           *Map
    node           *yaml.Node // dipakai untuk lookup baris secara lazy (lineOf(pointer))
}

// Parse: lihat alur. Issue dikembalikan maksimal MaxIssues; len(issues)>0 berarti dokumen ditolak.
func Parse(content []byte, format Format, lim Limits) (Document, []Issue)
// StripBOM membuang UTF-8 BOM di awal (D8). Dipanggil usecase sebelum Parse/hash/simpan.
func StripBOM(b []byte) []byte
```

Alur `Parse` (berhenti pada langkah pertama yang menghasilkan issue, kecuali langkah 6-7 yang mengumpulkan sampai 20 issue):
1. `utf8.Valid` dan tanpa byte `0x00` -> else `INVALID_ENCODING`. Hanya whitespace -> `EMPTY`.
2. Format JSON: `json.Valid`; bila tidak valid, decode dengan `json.Decoder` untuk mendapat `*json.SyntaxError.Offset` -> hitung baris -> `SYNTAX`. Setelah valid, JSON diparse dengan yaml.v3 (JSON adalah subset YAML 1.2) agar mendapat nomor baris.
3. `yaml.NewDecoder(bytes.NewReader(content))`: decode dokumen pertama ke `yaml.Node`; error -> `SYNTAX` (baris diambil dari pesan `yaml: line N:`); decode kedua tidak `io.EOF` -> `MULTI_DOCUMENT`. Kunci duplikat ditolak yaml.v3 -> `SYNTAX`.
4. `guard(node, lim)`.
5. Konversi Node -> nilai: mapping -> `*Map` (kunci skalar apa pun dijadikan string, mis. `200` -> `"200"`; kunci non-skalar -> `INVALID_STRUCTURE`); merge key `<<` (tag `!!merge`) digabung, kunci eksplisit menang; sequence -> `[]any`; skalar menurut tag: `!!str`, `!!timestamp`, `!!binary` -> string `Value`; `!!int` -> `json.Number(Value)` dinormalisasi (hex/oktal/underscore -> desimal via `strconv.ParseInt`, gagal -> string); `!!float` -> `float64` (`.inf`/`.nan` -> string `Value`); `!!bool` -> bool; `!!null` -> nil; tag lain -> string `Value`. Alias diekspansi (aman karena guard).
6. Klasifikasi: root bukan mapping -> `NOT_OPENAPI`. Tidak ada `openapi`: ada `swagger` -> `NOT_OPENAPI_3`; selain itu `NOT_OPENAPI`. `openapi` bukan string atau tidak cocok `^3\.(0|1)\.\d+$` -> `UNSUPPORTED_VERSION`.
7. `validate(doc)` (5.4) dan `checkRefs(doc)` (5.5). Kumpulkan issue; urutkan per baris; potong 20.
8. Sukses: isi `OpenAPIVersion`, `Title`, `APIVersion`.

### 5.4 `validate.go` (validasi struktural, toleran terhadap field tak dikenal dan `x-*`)

- `info` object; `info.title` string; `info.version` string.
- 3.0: `paths` wajib. 3.1: minimal satu dari `paths`, `webhooks`, `components`.
- `paths` object; tiap kunci diawali `/` (kecuali `x-*`); path item object (boleh `$ref`).
- Operasi pada 8 method (lowercase key): object; `operationId` string bila ada; `tags` array of string; `deprecated` bool; `summary`/`description` string; `parameters` array; `requestBody` object; `responses` object (3.0 wajib; 3.1 opsional) dengan kunci `^([1-5][0-9]{2}|[1-5]XX|default)$` atau `x-*`.
- Parameter (bukan `$ref`): `name` string, `in` salah satu `query|header|path|cookie`; `in: path` -> `required: true`. Kecocokan template `{param}` dengan parameter TIDAK dicek (terlalu banyak spec nyata yang longgar).
- `components` object; tiap sub-key yang dikenal (`schemas`, `responses`, `parameters`, `examples`, `requestBodies`, `headers`, `securitySchemes`, `links`, `callbacks`, `pathItems`) object.
- `servers` array of object dengan `url` string; `tags` array of object dengan `name` string; `security` array of object.
- `operationId` unik di seluruh `paths` dan `webhooks`; duplikat -> `DUPLICATE_OPERATION_ID` dengan pointer lokasi kedua dan pesan menyebut pointer lokasi pertama.
- Tipe skema (`type: string` vs `type: [string, "null"]`, `nullable`) TIDAK dinormalkan atau divalidasi (AC6; render di FE).

### 5.5 `refs.go`

- Walk seluruh dokumen; setiap kunci `$ref` dengan nilai string diperiksa. Subtree yang berisi data literal DILEWATI: nilai di bawah kunci `example`, `examples.<n>.value`, `default`, `enum`, `const`, dan kunci berawalan `x-`.
- `$ref` bukan string -> `INVALID_STRUCTURE`.
- Diawali `#` -> `ResolvePointer(root, ref)` harus berhasil, else `UNRESOLVED_REF`.
- Tidak diawali `#` (URL, `file.yaml#/x`, `./a.yaml`, `//host`, `$id`-relatif 3.1) -> `EXTERNAL_REF` (D3). Tidak ada resolver, loader, atau HTTP client di package ini.
- `discriminator.mapping` dan `$dynamicRef` tidak diperiksa (string biasa).

### 5.6 `extract.go`, `tojson.go`

```go
// ExtractEndpoints: hanya `paths` (webhooks tidak diindeks). Path item $ref internal diselesaikan.
// Method uppercase; OperationID nil bila kosong; Summary = summary (fallback "") dipotong 300 rune;
// Tags dedup urutan asli; Deprecated bool. Hasil terurut base.SortEndpoints.
func ExtractEndpoints(doc Document) []base.Endpoint
// ToJSON: json.Marshal(doc.Root) dengan urutan kunci asli (Map.MarshalJSON).
func ToJSON(doc Document) ([]byte, error)
```

### 5.7 `resolve.go` (dipakai `getendpoint`)

```go
type Resolved struct {
    Operation       json.RawMessage
    Servers         json.RawMessage // selalu array JSON
    SecuritySchemes json.RawMessage // selalu object JSON
    Truncated       bool
    EffectiveDepth  int
}
// FindOperation: (method,path) persis (path termasuk "{param}") atau operationId. ok=false bila tidak ada.
func FindOperation(doc Document, method, path, operationID string) (method2, path2 string, ok bool)
func ResolveOperation(doc Document, method, path string, depth, maxBytes int) (Resolved, error)
```

Aturan resolusi:
- Operasi = path item (path item `$ref` internal diselesaikan) + objek operasi. `parameters` level path digabung ke operasi; operasi menang untuk pasangan `name`+`in` yang sama (parameter `$ref` diselesaikan dulu untuk membandingkan).
- Inline `$ref` internal secara rekursif: setiap masuk ke objek tujuan `$ref` menambah kedalaman 1. Kedalaman > `depth` -> objek dibiarkan `{"$ref": "..."}`. Ref yang sedang ada di stack -> `{"$circular": "#/components/schemas/X"}`. Kunci saudara `$ref` (3.1: `description`, `summary`) digabung di atas objek ter-inline. Hanya yang dirujuk operasi yang muncul; `components` tidak disalin.
- `servers`: `operation.servers` ?? `pathItem.servers` ?? `root.servers` ?? `[]`.
- `securitySchemes`: nama skema dari `operation.security` ?? `root.security`, diambil dari `components.securitySchemes` (ref diselesaikan); hanya yang dipakai.
- Ukuran = `len(Operation)+len(Servers)+len(SecuritySchemes)`. Bila > `maxBytes`: ulangi dengan `depth-1` sampai muat atau 0; `Truncated=true`, `EffectiveDepth` = depth akhir. Bila depth 0 masih > `maxBytes`: `Operation` = `{"$truncated": true, "summary": ..., "operationId": ...}`, `Truncated=true`, `EffectiveDepth=0`.
- Tidak truncated: `EffectiveDepth = depth`.

### 5.8 `similar.go`

```go
type Candidate struct{ Method, Path, OperationID, Summary string }
// Similar: skor = min(Levenshtein(lower("METHOD path"), lower(query)), Levenshtein(lower(opID), lower(queryOpID)) bila ada);
// string dipotong 200 rune sebelum dihitung; urut skor lalu urutan kanonik; ambil n (n=5).
func Similar(cands []Candidate, method, path, operationID string, n int) []Candidate
```

---

## 6. Subpackage I/O bersama

### 6.1 `base/store` (package `store`)

```go
type Store struct{ q db.Querier }
func New(q db.Querier) *Store
func (s *Store) WithTx(tx *sqlx.Tx) *Store
var ErrNameTaken = errors.New("store: spec name taken") // 23505 pada constraint api_specs_project_name_key

// projectID/specID bukan uuid -> hasil kosong / (zero,false,nil) tanpa query.
func (s *Store) FindSpec(ctx, projectID, name string) (base.Spec, bool, error)          // termasuk content
func (s *Store) FindSpecMeta(ctx, projectID, name string) (base.SpecSummary, bool, error) // tanpa content, + endpoint_count
func (s *Store) LockSpec(ctx, projectID, name string) (base.Spec, bool, error)          // FOR UPDATE; wajib di tx
func (s *Store) ListSpecs(ctx, projectID string) ([]base.SpecSummary, error)           // LEFT JOIN COUNT, ORDER BY name
func (s *Store) ListSpecNames(ctx, projectID string) ([]string, error)                 // ORDER BY name
func (s *Store) CountSpecs(ctx, projectID string) (int, error)
func (s *Store) LockProjectSpecs(ctx, projectID string) error
//   SELECT pg_advisory_xact_lock(hashtextextended('apispec:' || $1::text, 0)); wajib di tx
func (s *Store) ListSpecEndpoints(ctx, specID string) ([]base.Endpoint, error)         // urutan kanonik
func (s *Store) ReplaceEndpoints(ctx, specID string, es []base.Endpoint) error
//   DELETE FROM api_spec_endpoints WHERE spec_id=$1; lalu satu INSERT ... SELECT FROM unnest($2::text[], $3::text[],
//   $4::text[], $5::text[], $6::text[] /*tags sebagai JSON array string*/, $7::bool[]) bila len>0. Wajib di tx.
func (s *Store) CountEndpoints(ctx, specID string) (int, error)
```

- Fungsi yang hanya dipakai satu feature (INSERT/UPDATE content -> `uploadspecs`; UPDATE name -> `renamespec`; DELETE -> `deletespec`; query filter endpoint -> `listendpoints`; count per project batch -> `countspecs`) ditulis di `repository.go` feature itu, bukan di store.
- Tags di `unnest`: kirim `[]string` JSON per endpoint, lalu `ARRAY(SELECT jsonb_array_elements_text(t::jsonb))` di SQL (unnest array 2D tidak cocok untuk panjang berbeda).
- Row struct privat bertag `db`; entity tanpa tag. Waktu `.UTC()`. `content_hash` `bytea` <-> `[32]byte`.
- Error pgx `23505` dengan `ConstraintName == "api_specs_project_name_key"` -> `ErrNameTaken` (via `errors.As(err, &pgconn.PgError)`).

### 6.2 `base/access` (package `access`)

```go
type Checker interface{ RequireMember(ctx context.Context, projectID string) error }
type Guard struct{ b *bus.Registry }
func New(b *bus.Registry) *Guard
// RequireMember: tabel 3.4. Call:
//   bus.CallTyped[checkaccess.Response](ctx, g.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
//       checkaccess.Request{ProjectID: projectID, UserID: id.UserID, UserRole: id.Role})
```

### 6.3 `base/audit` (package `audit`, pola T03 5.6)

```go
type Recorder interface{ Record(ctx context.Context, ev Event) }
type Event struct {
    Action    string         // record.ActionAPISpec*
    ProjectID string
    Target    string         // nama spec (setelah aksi; rename: nama baru)
    Args      map[string]any // TANPA isi spec
    Meta      base.RequestMeta
}
type BusRecorder struct{ b *bus.Registry; log *slog.Logger; now base.Clock }
func New(b *bus.Registry, log *slog.Logger, now base.Clock) *BusRecorder
// Record: identitas dari reqctx.IdentityFrom(ctx) -> ActorType record.ActorUser, ActorID UserID,
//   ActorLabel base.ActorFrom(...).Label; ProjectLabel ""; TargetType "api_spec"; Status record.StatusOK;
//   OccurredAt now(); TraceID reqctx.TraceID(ctx); IP/UserAgent dari Meta.
//   Identitas tidak ada -> log error, tidak mengirim.
//   bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
//   error -> log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "error", err)
//   TIDAK mengembalikan error (fail-open).
```

Args per aksi:
- `apispec.uploaded`: `{name, source_path, openapi_version, endpoint_count, size_bytes, overwritten}`; satu entri per file `created` (overwritten=false) / `updated` (overwritten=true).
- `apispec.updated` (rename): `{from, to}`; no-op tidak diaudit.
- `apispec.deleted`: `{name, endpoint_count}`.

### 6.4 `base/upload` (package `upload`)

```go
type File struct {
    FileName  string // basename untuk tampilan (setelah "\"->"/")
    RawPath   string // filename mentah dari Content-Disposition (bisa berisi direktori)
    Content   []byte // nil bila TooLarge/Rejected
    TooLarge  bool
    Rejected  string // reason_code (TOO_MANY_FILES) bila part dibuang
}
type Form struct {
    Files []File
    Name  *string // field teks "name" (opsional, maks 1 KiB)
}
// Read:
//  - Content-Type bukan multipart/form-data -> base.ErrNotMultipart (400).
//  - r.MultipartReader(); iterasi NextPart. Field file = "files" (boleh berulang). Field "name" teks.
//    Field lain di-drain dan diabaikan.
//  - Path asli: mime.ParseMediaType(part.Header.Get("Content-Disposition")) -> params["filename"]
//    (BUKAN part.FileName() yang membuang direktori). Kosong -> part.FileName().
//  - Konten: io.ReadAll(io.LimitReader(part, maxFileBytes+1)); > maxFileBytes -> TooLarge, sisa di-drain.
//  - Part file ke-(maxFiles+1) dan seterusnya -> File{Rejected: TOO_MANY_FILES}, isi di-drain.
//  - Error baca dengan errors.As(err, *http.MaxBytesError) -> base.ErrPayloadTooLarge (413 seluruh request).
//    Error multipart lain -> 400 "multipart tidak valid".
//  - Nol part file -> base.ErrNoFiles. Name != nil && len(Files) != 1 -> base.ErrNameWithMultipleFiles.
func Read(r *http.Request, maxFileBytes int64, maxFiles int) (Form, error)
```

`base/upload` meng-import `net/http` dan `mime`; package ini bukan parser (larangan 3.7 hanya untuk `base/openapi`).

---

## 7. Kontrak bus dan konstanta endpoint

### 7.1 `backend/lib/constants/bus.go` (ditambah)

```go
// Provider: T06 project/apispec. Consumer: T09 (discovery/apispectools).
// Kontrak: lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}.
// Handler TIDAK memeriksa akses user; WAJIB membatasi query pada ProjectID request (rule 6.10).
const BUS_PROJECT_APISPEC_LIST_SPECS     = "project.apispec.list-specs"
const BUS_PROJECT_APISPEC_LIST_ENDPOINTS = "project.apispec.list-endpoints"
const BUS_PROJECT_APISPEC_GET_ENDPOINT   = "project.apispec.get-endpoint"
```

`BUS_PROJECT_APISPEC_COUNT_SPECS` didefinisikan T03 (blok contentcount) — JANGAN duplikat. Bila T03 belum merge, blok T03 5.1 disalin persis (termasuk `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`).

### 7.2 `lib/contracts/project/apispec/listspecs/listspecs.go` (baru)

```go
// Package listspecs adalah kontrak bus project.apispec.list-specs (provider T06, consumer T09).
// ProjectID bukan uuid -> error membungkus errs.ErrInvalid. Project tanpa spec -> Items kosong (bukan nil).
// Tipe request salah -> bus.ErrBadRequestType. Request dikirim sebagai nilai.
package listspecs

type Request struct{ ProjectID string }
type Response struct{ Items []Spec } // urut Name
type Spec struct {
    Name, Title, ApiVersion, OpenapiVersion string
    EndpointCount int
    UpdatedAt     time.Time
}
```

Catatan: kontrak hanya stdlib (rule 4.5); "membungkus errs.ErrInvalid" berarti provider memakai `errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)`; kontrak tidak meng-import errs.

### 7.3 `lib/contracts/project/apispec/listendpoints/listendpoints.go` (baru)

```go
// Package listendpoints adalah kontrak bus project.apispec.list-endpoints (provider T06, consumer T09).
// Clamp (bukan error): Limit<=0 -> DefaultLimit, >MaxLimit -> MaxLimit; Offset<0 -> 0.
// Query di-trim, dipotong MaxQueryRunes; dicocokkan ILIKE (escape % _ \) pada path, summary, operation_id.
// Tag dicocokkan persis (case-sensitive) ke salah satu tag endpoint. Spec "" = semua spec project.
// Spec diisi tapi tidak ada -> SpecFound=false, Specs berisi nama spec valid, Items kosong, err nil.
// Urut: Spec, Path, method kanonik (GET,PUT,POST,DELETE,OPTIONS,HEAD,PATCH,TRACE).
// Truncated = Offset+len(Items) < Total; NextOffset = Offset+len(Items) bila Truncated, else 0.
// ProjectID bukan uuid -> error membungkus errs.ErrInvalid.
package listendpoints

const (
    DefaultLimit  = 100
    MaxLimit      = 300
    MaxQueryRunes = 200
)

type Request struct {
    ProjectID, Spec, Tag, Query string
    Limit, Offset               int
}
type Response struct {
    SpecFound  bool
    Specs      []string // diisi bila !SpecFound
    Items      []Endpoint
    Total      int
    Truncated  bool
    NextOffset int
}
type Endpoint struct {
    Spec, Method, Path string
    OperationID        string // "" bila tidak ada
    Summary            string
    Tags               []string
    Deprecated         bool
}
```

`SpecFound` bernilai `true` bila `Spec == ""`.

### 7.4 `lib/contracts/project/apispec/getendpoint/getendpoint.go` (baru)

```go
// Package getendpoint adalah kontrak bus project.apispec.get-endpoint (provider T06, consumer T09).
//
// Error Go (membungkus errs.ErrInvalid) hanya untuk request tidak valid: ProjectID bukan uuid;
// tidak ada (Method+Path) maupun OperationID; keduanya diisi; hanya salah satu dari Method/Path;
// Method bukan GET|PUT|POST|DELETE|OPTIONS|HEAD|PATCH|TRACE (provider menormalkan uppercase).
// Clamp: SchemaDepth<=0 -> DefaultSchemaDepth, >MaxSchemaDepth -> MaxSchemaDepth; MaxBytes<=0 -> DefaultMaxBytes.
//
// Hasil "tidak ketemu" (err nil, Found=false):
//   Spec "" dan project punya != 1 spec -> Reason SPEC_REQUIRED, Specs = nama spec project.
//   Spec tidak ada                      -> Reason SPEC_NOT_FOUND, Specs.
//   Operasi tidak ada                   -> Reason ENDPOINT_NOT_FOUND, Spec terisi, Similar (maks 5).
// Spec "" dan project punya tepat 1 spec -> spec itu dipakai.
package getendpoint

const (
    DefaultSchemaDepth = 4
    MaxSchemaDepth     = 8
    DefaultMaxBytes    = 102400

    ReasonSpecRequired     = "SPEC_REQUIRED"
    ReasonSpecNotFound     = "SPEC_NOT_FOUND"
    ReasonEndpointNotFound = "ENDPOINT_NOT_FOUND"
)

type Request struct {
    ProjectID, Spec, Method, Path, OperationID string
    SchemaDepth int
    MaxBytes    int
}
type Response struct {
    Found           bool
    Reason          string
    Specs           []string
    Similar         []EndpointRef
    Spec, Method, Path string
    Operation       json.RawMessage
    Servers         json.RawMessage // array
    SecuritySchemes json.RawMessage // object
    Truncated       bool
    EffectiveDepth  int
    Hint            string // "hasil dipotong; turunkan schema_depth (dipakai N)" bila Truncated
}
type EndpointRef struct{ Method, Path, OperationID, Summary string }
```

### 7.5 `lib/contracts/project/management/contentcount` (milik T03, di-provide T06)

Provider `countspecs` mematuhi doc comment T03 5.4: `bus.Expect[contentcount.Request]`; `len(ProjectIDs) > contentcount.MaxProjectIDs` -> error membungkus `errs.ErrInvalid`; id bukan uuid diabaikan; satu query `SELECT project_id, count(*) FROM api_specs WHERE project_id = ANY($1::uuid[]) GROUP BY project_id`; project tanpa spec boleh absen dari `Counts`; `Counts` tidak pernah nil.

### 7.6 Kontrak yang dikonsumsi

- `lib/contracts/project/management/checkaccess` (T03 5.2): `Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists bool; Level}`; `LevelNone|Member|Owner|Admin`.
- `lib/contracts/core/audit/record` (sudah di `main`).

### 7.7 Katalog README

Baris `list-specs`, `list-endpoints`, `get-endpoint` sudah ada; tambah catatan kontrak di `lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}`. Konfirmasi/tambah baris (bila T03 belum menambahkannya):

```
| `project.apispec.count-specs` | Call | T06 | T03 |
```

Ubah baris `core.auth.get-user`: consumer `T03, T04, T05` (T06 dihapus, D6).

---

## 8. Route (`init.go` per feature, pola T03 D4)

```go
// uploadspecs
d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
    Post("/projects/{projectID}/api-specs/upload", h.ServeHTTP)
// validatespec
d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
    Post("/projects/{projectID}/api-specs/validate", h.ServeHTTP)
// listspecs, getspec, downloadspec, renamespec, deletespec
d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs", h.ServeHTTP)
d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs/{name}", h.ServeHTTP)
d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs/{name}/raw", h.ServeHTTP)
d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}/api-specs/{name}", h.ServeHTTP)
d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}/api-specs/{name}", h.ServeHTTP)
```

Route statis `upload`/`validate` hanya POST; `{name}` hanya GET/PATCH/DELETE. Spec bernama `upload`/`validate` tetap bisa di-GET/PATCH/DELETE karena chi melanjutkan pencarian ke node param bila method tidak cocok di node statis. Ini DIWAJIBKAN tes router (`routes_test.go`).

---

## 9. Migration

Satu file: `backend/migrations/project/<VERSION>_create_api_specs.sql` (setelah `20261022090000000_create_projects.sql` T03). `<VERSION>` bukan nilai tetap: di-generate saat implementasi dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`) (format `YYYYMMDDHHMMSSmmm`, 17 digit sesuai validasi runner). Karena dibuat saat implement, otomatis lebih besar dari migrasi T03 (`create_projects`). Sebelum commit, pastikan tidak ada versi kembar di `backend/migrations/**`; bila kembar, generate ulang.

```sql
-- T06 project/apispec. Butuh tabel projects (T03).

CREATE TABLE api_specs (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             text        NOT NULL,
    source_path      text        NULL,
    title            text        NOT NULL,
    api_version      text        NOT NULL,
    openapi_version  text        NOT NULL,
    content          text        NOT NULL,
    content_format   text        NOT NULL,
    content_hash     bytea       NOT NULL,
    size_bytes       int         NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,  -- tanpa FK (rule 8.5)
    created_by_label text        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    updated_by_label text        NOT NULL,
    CONSTRAINT api_specs_project_name_key UNIQUE (project_id, name),
    CONSTRAINT api_specs_name_check CHECK (name ~ '^[a-z0-9._-]{1,80}$' AND name NOT IN ('.', '..')),
    CONSTRAINT api_specs_format_check CHECK (content_format IN ('yaml', 'json')),
    CONSTRAINT api_specs_size_check CHECK (size_bytes >= 0),
    CONSTRAINT api_specs_hash_len CHECK (octet_length(content_hash) = 32)
);

CREATE TABLE api_spec_endpoints (
    spec_id      uuid    NOT NULL REFERENCES api_specs(id) ON DELETE CASCADE,
    method       text    NOT NULL,
    path         text    NOT NULL,
    operation_id text    NULL,
    summary      text    NOT NULL DEFAULT '',
    tags         text[]  NOT NULL DEFAULT '{}',
    deprecated   boolean NOT NULL DEFAULT false,
    CONSTRAINT api_spec_endpoints_pkey PRIMARY KEY (spec_id, method, path),
    CONSTRAINT api_spec_endpoints_method_check
        CHECK (method IN ('GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'))
);
CREATE INDEX api_spec_endpoints_opid_idx ON api_spec_endpoints (spec_id, operation_id)
    WHERE operation_id IS NOT NULL;
```

Catatan:
- Forward-only, tanpa `CONCURRENTLY` (runner T00 membungkus file dalam transaksi).
- `api_specs_project_name_key` melayani filter `project_id` (prefix) dan lookup nama; tidak perlu indeks `project_id` terpisah.
- `api_spec_endpoints_pkey` (prefix `spec_id`) melayani JOIN `list-endpoints`; spec 500 endpoint dibaca lewat index scan per spec lalu difilter/diurutkan di memori (< 100 ms; EXPLAIN dicatat di plan `listendpoints`, rule 8.8). Tidak ada indeks trigram (skala kecil per project).
- Hard delete (rule 8.2); hapus project (T03) meng-cascade `api_specs` lalu `api_spec_endpoints`. Tes pg_constraint T03 (AC6) akan memverifikasi FK ber-cascade.

---

## 10. Konfigurasi (`infra/config` + `docs/configuration.md`)

| Env | Tipe | Default | Validasi | Keterangan |
|---|---|---|---|---|
| `APISPEC_MAX_FILE_BYTES` | int64 | `5242880` | `>= 1` dan `<= MAX_UPLOAD_BYTES` | Ukuran maksimum satu file spec |
| `APISPEC_MAX_SPECS` | int | `100` | `>= 1` | Jumlah maksimum spec per project |

- Field `Config.APISpecMaxFileBytes int64`, `Config.APISpecMaxSpecs int`, dibaca dengan helper loader yang sudah ada (`l.int64`, `l.intv`) di `config.go`; cek relasi dengan `MAX_UPLOAD_BYTES` setelah keduanya terbaca (pola `MaxUploadBytes < MaxFileBytes`).
- Batas upload total dan jumlah file memakai `MAX_UPLOAD_BYTES` / `MAX_UPLOAD_FILES` yang sudah ada.
- Batas YAML (64/100/500000) konstanta di `base/openapi/limits.go`, tidak dikonfigurasi.
- `config_test.go` diperbarui: default, override, dan penolakan (`0`, `> MAX_UPLOAD_BYTES`).

---

## 11. Plan (langkah berurutan)

```
F  foundation (migrasi, config, konstanta, kontrak, base/, base/openapi, base/store|access|audit|upload, OpenAPI yml, dokumen)
 ├── gelombang A (paralel): validatespec, uploadspecs, listspecs, countspecs
 ├── gelombang B (paralel): getspec, downloadspec, renamespec, deletespec, listspecsbus, listendpoints, getendpoint
W  wiring final: apispec/base.go (11 Init), project/base.go, tes integrasi lintas feature, TestRoutesRegistered
FE frontend-plan.md (terpisah, setelah kontrak REST F7 final)
```

Secara kode semua feature hanya bergantung pada F, jadi gelombang B boleh dimulai bersamaan dengan A; urutan gelombang hanya agar data uji (spec tersimpan lewat `uploadspecs`) tersedia untuk tes integrasi gelombang B. Tiap usecase menambah satu baris `<feature>.Init(d)` di `apispec/base.go` di PR-nya.

### F — foundation

1. **Migrasi** (generate `<VERSION>` dulu dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`)): `backend/migrations/project/<VERSION>_create_api_specs.sql` (bagian 9). Validasi dengan `make migrate-up` lokal di atas migrasi T03.
2. **Config**: tambah 2 env var (bagian 10) ke `backend/internal/infra/config/config.go`, `config_test.go`, `docs/configuration.md`, dan `.env.example` bila ada.
3. **Dependensi**: `go get go.yaml.in/yaml/v3@<versi stabil terbaru>`; `go mod tidy`. Catat alasan di PR (D2).
4. **Konstanta**: 3 konstanta di `backend/lib/constants/bus.go` (7.1). Bila T03 belum merge: blok konstanta T03 5.1 persis.
5. **Kontrak** (stdlib saja, rule 4.5): `backend/lib/contracts/project/apispec/listspecs/listspecs.go`, `.../listendpoints/listendpoints.go`, `.../getendpoint/getendpoint.go` (7.2-7.4). Bila T03 belum merge: `lib/contracts/project/management/checkaccess/checkaccess.go` dan `.../contentcount/contentcount.go` persis T03 5.2/5.4.
6. **base/** (package `base`): semua file bagian 4 + unit test.
7. **base/openapi/**: bagian 5 + `testdata/` (13.2) + unit test + `imports_test.go`.
8. **base/store**: 6.1 + `store_integration_test.go` (tag `integration`, `db.TestDB(t)`, menerapkan migrasi T03 + T06).
9. **base/access**, **base/audit**, **base/upload**: 6.2-6.4 + test (bus nyata `bus.NewRegistry(log)` dengan handler palsu; `httptest` untuk upload).
10. **Kerangka wiring**:
    - `backend/internal/domain/project/internal/apispec/base.go`: `package apispec; func Register(d app.Deps) { /* <feature>.Init(d) ditambah tiap usecase */ }`;
    - `backend/internal/domain/project/base.go`: tambah `apispec.Register(d)` di samping `management.Register(d)` (bila T03 belum merge, file berisi hanya `apispec.Register(d)` dan T03 menambah barisnya; konflik merge diselesaikan dengan menyimpan keduanya).
11. **OpenAPI** (SEBELUM kode handler, rule 11.6): `api/modules/project/apispec.yml` dengan 5 path (7 operasi) dan schema bagian 15 (`ApiSpec`, `ApiSpecList`, `ApiSpecDetail`, `ApiSpecEndpoint`, `ApiSpecIssue`, `ApiSpecFileResult`, `ApiSpecUploadResult`, `ApiSpecValidateResult`, `RenameApiSpecRequest`); parameter `XRequestedWith`, `ProjectID`, `SpecName` di `components.parameters` file itu (pola `management.yml`); request multipart `files` (array binary) + `name`; `/raw` dengan content `application/yaml` dan `application/json`; `Error` dan responses dari `../infra/foundation.yml`. Tambah `$ref` path dan schema ke `api/root.yml` (escape `~1`), `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus.
12. **Dokumen**:
    - `docs/tickets/README.md`: katalog bus (7.7);
    - `docs/tickets/T06-project-api-spec.md`: tabel Features (D1: `listspecsbus`, `listendpoints`, `getendpoint`, `countspecs`), "Dikonsumsi" tanpa `core.auth.get-user` (D6), keputusan `$ref` eksternal ditolak (D3), "Disediakan" `project.apispec.count-specs`.

### U — usecase (plan rinci terpisah)

Tiap usecase: folder feature, `Init` di `apispec/base.go`, cek path/schema di `apispec.yml`, tes AC yang menjadi tanggung jawabnya (bagian 13).

Contoh pola `init.go` HTTP:

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    clock := base.Clock(func() time.Time { return time.Now().UTC() })
    uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log, clock), clock, d.Config.APISpecMaxSpecs)
    h := newHandler(uc, d.Config.APISpecMaxFileBytes, d.Config.MaxUploadFiles)
    d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
        Post("/projects/{projectID}/api-specs/upload", h.ServeHTTP)
}
```

Feature bus:

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(store.New(d.DB)))
    d.Bus.Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, newHandler(uc).Handle)
}
```

### W — wiring final

1. `apispec/base.go` memanggil ke-11 `Init` (urutan bebas).
2. `apispec_integration_test.go` (tag `integration`): bus nyata, `db.TestDB(t)`, router `chi.NewRouter()` + middleware identitas uji; handler palsu untuk `project.management.check-access` (peta project -> level) dan `core.audit.record` (merekam) bila belum terdaftar di registry test.
3. `routes_test.go` / `TestRoutesRegistered`: `chi.Walk` memuat tepat 7 pasangan method+pola bagian 1; bus memuat 4 endpoint (Call dengan request valid tidak `ErrNotRegistered`); `GET /api/projects/{id}/api-specs/upload` dan `.../validate` diarahkan ke `getspec`, `POST .../{name}` -> 405.
4. `go vet ./...`, `golangci-lint`, archtest `TestT00_AC4_RepoIsClean`, `make api-check` lulus.

---

## 12. Changes (per file/folder)

Lib, config, migrasi (F):
- `backend/go.mod`, `backend/go.sum` (ubah): `go.yaml.in/yaml/v3`.
- `backend/lib/constants/bus.go` (ubah): 3 konstanta T06 (+ blok T03 bila belum ada).
- `backend/lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}/*.go` (baru).
- `backend/lib/contracts/project/management/{checkaccess,contentcount}/*.go` (baru HANYA bila T03 belum merge; isi persis plan T03).
- `backend/migrations/project/<VERSION>_create_api_specs.sql` (baru).
- `backend/internal/infra/config/config.go`, `config_test.go` (ubah): 2 env var.

Domain:
- `backend/internal/domain/project/base.go` (ubah): `apispec.Register(d)`.
- `backend/internal/domain/project/internal/apispec/base.go` (baru).
- `backend/internal/domain/project/internal/apispec/base/{spec,name,upload,errors,dto,actor,meta,clock}.go` + `*_test.go` (baru).
- `backend/internal/domain/project/internal/apispec/base/openapi/{limits,issue,map,pointer,yamlguard,parse,validate,refs,extract,resolve,similar,tojson}.go` + `*_test.go` + `imports_test.go` + `testdata/*` (baru).
- `backend/internal/domain/project/internal/apispec/base/store/{store.go,store_integration_test.go}` (baru).
- `backend/internal/domain/project/internal/apispec/base/access/{access.go,access_test.go}` (baru).
- `backend/internal/domain/project/internal/apispec/base/audit/{audit.go,audit_test.go}` (baru).
- `backend/internal/domain/project/internal/apispec/base/upload/{upload.go,upload_test.go}` (baru).
- `backend/internal/domain/project/internal/apispec/<11 feature>/...` (baru, per plan usecase).
- `backend/internal/domain/project/internal/apispec/{apispec_integration_test.go,routes_test.go}` (baru, W).

API dan dokumen:
- `api/modules/project/apispec.yml` (baru), `api/root.yml` (ubah), `api/openapi.yml` (generated).
- `docs/configuration.md` (ubah), `docs/tickets/README.md` (ubah), `docs/tickets/T06-project-api-spec.md` (ubah).

Frontend: rinci di `frontend-plan.md` (`frontend/src/domain/project/apispec/...`, i18n `apispec.*` termasuk `apispec.reason.<REASON_CODE>` dan `apispec.issue.<CODE>`).

Tidak diubah: `internal/infra/**` selain `config`, `cmd/**`, module lain.

---

## 13. Test Scenario

Penamaan: `TestT06_ACn_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### 13.1 Unit `base/` (tanpa mock)

- `ValidateName`: valid `payment`, `a`, `v1.2_x-y`, 80 karakter; tidak valid `""`, `.`, `..`, `Payment`, `a b`, `a/b`, 81 karakter, unicode.
- `DeriveName`: `auth.yaml`->`auth`; `specs/Payment API.v2.yaml`->`payment-api.v2`; `C:\x\Notif.YML`->`notif`; `---.json`->error; `.yaml`->error; nama 200 karakter dipotong 80 tanpa `-`/`.` di ujung.
- `NormalizeSourcePath`: `a\b\c.yaml`->`a/b/c.yaml`; tolak `/etc/x.yaml`, `C:/x.yaml`, `a/../b.yaml`, NUL, 513 byte.
- `FormatFromExt`: `.YAML`, `.yml`, `.json` ok; `.txt`, tanpa ekstensi, `.yaml.bak` skipped.
- `DecideUpload`: 4 cabang termasuk "update saat specCount == maxSpecs tetap Update", "hash sama saat batas penuh -> Unchanged".
- `Spec.Rename` no-op; `NewSpec` hash/size; `SortEndpoints`/`MethodOrder`; `ActorFrom` fallback Email; `Summarize`; DTO null-handling (`Pointer ""`->null, `Line 0`->null).

### 13.2 Unit `base/openapi/` (`testdata/`)

| File testdata | Harapan |
|---|---|
| `petstore-3.0.yaml` | valid; 3.0.3; endpoint terekstrak lengkap, urut kanonik; `nullable` dibiarkan |
| `petstore-3.1.json` | valid; 3.1.0; `type: ["string","null"]` dibiarkan; kunci `200` jadi `"200"` |
| `webhooks-only-3.1.yaml` | valid (tanpa `paths`); 0 endpoint |
| `components-only-3.0.yaml` | `INVALID_STRUCTURE` (3.0 butuh `paths`) |
| `package.json` | `NOT_OPENAPI` |
| `broken.yaml` | `SYNTAX` dengan `Line` > 0 |
| `broken.json` | `SYNTAX` dengan `Line` dari offset |
| `swagger-2.0.yaml` | `NOT_OPENAPI_3` |
| `openapi-4.0.yaml`, `openapi-number.yaml` (`openapi: 3.0` numerik) | `UNSUPPORTED_VERSION` |
| `ref-http.yaml` (`$ref: http://127.0.0.1:PORT/x.yaml`), `ref-file.yaml` (`./common.yaml#/X`, `/etc/passwd`) | `EXTERNAL_REF`, tanpa I/O |
| `ref-unresolved.yaml` | `UNRESOLVED_REF` dengan pointer dan baris |
| `ref-in-example.yaml` (`$ref` di dalam `example`/`x-foo`) | valid (tidak diperiksa) |
| `dup-operationid.yaml` | `DUPLICATE_OPERATION_ID` menyebut kedua lokasi |
| `billion-laughs.yaml` (9 level x 10 alias) | `YAML_LIMIT` |
| `deep.yaml` (kedalaman 200) | `YAML_LIMIT` |
| `alias-cycle.yaml` | `YAML_LIMIT` |
| `multi-doc.yaml` | `MULTI_DOCUMENT` |
| `invalid-utf8.yaml`, `nul.json` | `INVALID_ENCODING` |
| `bom.json` | valid setelah `StripBOM` |
| `merge-keys.yaml` (`<<: *base`) | valid, kunci eksplisit menang |
| `many-errors.yaml` (> 20 kesalahan) | tepat 20 issue, urut baris |
| `nested-circular.yaml` | untuk resolver (13.2 bawah) |

Tes lain:
- `TestT06_AC3_BillionLaughsCheap`: parse `billion-laughs.yaml` < 1 detik dan alokasi < 50 MB (`runtime.MemStats.TotalAlloc` sebelum/sesudah); juga varian JSON besar 5 MB valid < 1 detik (baseline).
- `TestT06_AC2_NoNetworkImports` (`imports_test.go`): parse semua file non-test package `openapi` dengan `go/parser`; gagal bila ada import dari daftar 3.7.
- `TestT06_AC2_ExternalRefNoRequest`: `httptest.NewServer` menghitung request; parse spec ber-`$ref` ke URL server -> `EXTERNAL_REF` dan hitungan 0.
- `Map.MarshalJSON` menjaga urutan; escape karakter khusus sama dengan `encoding/json`.
- `ResolvePointer`: `~0`, `~1`, index array, `#`.
- `ResolveOperation` (`TestT06_AC5_*`): skema bersarang 3 level inline pada depth 4; depth 1 menyisakan `$ref` di level 2; sirkular `Node -> children -> Node` menghasilkan `{"$circular": "#/components/schemas/Node"}`; skema tak dirujuk (`Unused`) tidak muncul di JSON; path-level parameter digabung dan dioverride operasi; servers fallback operasi > path > root > `[]`; securitySchemes hanya yang dipakai (operasi `security: []` -> `{}`); truncation bertahap (maxBytes kecil -> `EffectiveDepth` turun, `Truncated`) dan fallback `$truncated` pada depth 0.
- `FindOperation`: by method+path (path persis dengan `{id}`), by operationId, method lowercase input dinormalkan pemanggil.
- `Similar`: `GET /pet/{id}` menemukan `GET /pets/{petId}` di urutan atas; maks 5.

### 13.3 Subpackage I/O

- `store` (integration): CRUD dasar; `ListSpecs` urut nama + endpoint_count benar (0 untuk spec tanpa endpoint); `ReplaceEndpoints` mengganti seluruhnya (endpoint lama hilang); `LockProjectSpecs` men-serialisasi dua tx paralel (tx kedua menunggu); 23505 -> `ErrNameTaken`; id bukan uuid -> kosong tanpa query; isolasi: spec project A tidak terlihat dari project B; hapus project meng-cascade spec dan endpoint.
- `access`: `ProjectExists=false` -> 404; `LevelNone` -> 404; member/owner/admin -> nil; handler error / `ErrNotRegistered` -> error (bukan nil); tanpa identitas -> 401.
- `audit`: request yang dikirim (ActorLabel Name/Email, TargetType `api_spec`, tanpa content); handler error -> tidak panik, tidak mengembalikan error, log level error.
- `upload`: dua file dengan direktori (`filename="v1/payment.yaml"` -> `RawPath` utuh); file > batas -> `TooLarge` dan part berikutnya tetap terbaca; part ke-(max+1) -> `TOO_MANY_FILES`; body > `BodyLimit` -> `ErrPayloadTooLarge`; bukan multipart -> `ErrNotMultipart`; nol file -> `ErrNoFiles`; `name` + 2 file -> `ErrNameWithMultipleFiles`; field tak dikenal diabaikan.

### 13.4 Integrasi lintas usecase (`apispec_integration_test.go`, tag `integration`)

- `TestT06_AC1_UploadThreeThenOverwrite`: upload 3 spec -> 3 `created`; upload ulang `payment.yaml` dengan endpoint berbeda -> `updated`; `list-endpoints` spec `payment` tidak lagi memuat endpoint yang dihapus; upload ulang identik -> `unchanged`, tanpa audit.
- `TestT06_AC2_RejectedWithReasons`: `package.json`, YAML rusak, Swagger 2.0, `$ref: http://<listener lokal>` dalam satu request -> 4 `rejected` dengan issue code tepat; listener menghitung 0 request; tidak ada baris tersimpan.
- `TestT06_AC4_ListEndpoints500Fast`: spec 500 endpoint; `list-endpoints` < 100 ms (median 5 kali); filter tag, teks (`%`/`_` ter-escape), paginasi (`Truncated`, `NextOffset`).
- `TestT06_AC5_GetEndpointInlines`: via bus pada spec tersimpan.
- `TestT06_AC6_OpenAPI30And31`: upload `petstore-3.0.yaml` dan `petstore-3.1.json`, `getspec` keduanya 200.
- `TestT06_AC7_ProjectIsolation`: non-anggota mendapat 404 di 7 route (termasuk `projectID` bukan uuid dan spec yang ada di project lain); bus `list-specs`/`list-endpoints`/`get-endpoint`/`count-specs` dengan `ProjectID` B tidak mengembalikan data project A.
- Batas spec: `APISPEC_MAX_SPECS=2` -> file ketiga `rejected SPEC_LIMIT_REACHED`, menimpa spec yang ada tetap boleh.
- Rename lalu upload nama lama -> spec baru `created`; delete -> endpoint indeks hilang.
- Audit: `apispec.uploaded` per created/updated dengan `overwritten`; `apispec.updated {from,to}`; `apispec.deleted {name, endpoint_count}`; audit handler gagal -> respons tetap 200/204.

AC8 (deskripsi HTML/skrip tidak dieksekusi) diuji di FE (`frontend-plan.md`) dan E2E; backend hanya menjamin `Content-Type: application/json` dan tanpa HTML.

---

## 14. E2E Scenarios (Playwright; butuh T03 merge)

Positif:
1. Developer anggota membuka tab "API Specs", mengunggah folder berisi `auth.yaml`, `payment.yaml`, `notification.yaml` -> ringkasan 3 `created`; daftar menampilkan 3 kartu dengan judul, versi, jumlah endpoint.
2. Mengunggah ulang `payment.yaml` yang diperbarui -> `updated`; halaman detail menampilkan endpoint baru dan endpoint lama hilang.
3. Halaman detail: pohon tag -> endpoint, panel dokumentasi (parameter, body, response, skema dapat dilipat, servers, security, lencana deprecated); pencarian endpoint.
4. Unduh raw -> file `payment.yaml` identik dengan yang diunggah.
5. Ubah nama `payment` -> `payment-v2`; URL detail berubah; nama lama 404.
6. Hapus spec -> hilang dari daftar.
7. Validasi tanpa menyimpan: hasil `valid` dengan jumlah endpoint; daftar spec tidak berubah.
8. Super Admin (bukan anggota) dapat melihat dan mengunggah.

Negatif:
1. Unggah `package.json`, YAML rusak, Swagger 2.0, spec `$ref` URL -> 4 baris `rejected` dengan alasan yang bisa disalin (termasuk baris/pointer).
2. Unggah file `.txt` -> `skipped`.
3. Unggah file 6 MB -> `rejected FILE_TOO_LARGE`.
4. Ubah nama ke nama yang sudah dipakai -> pesan konflik; nama `Bad Name` -> pesan tidak valid.
5. Developer bukan anggota membuka URL spec project lain -> halaman 404.
6. Spec dengan deskripsi `<script>alert(1)</script>` dan `[x](javascript:alert(1))` -> teks tampil aman, tidak ada dialog, tautan tidak aktif (AC8).
7. Request mutasi tanpa header CSRF -> 403 `CSRF_REQUIRED`.

---

## 15. Request/Response body

Format error seragam (rule 11.1), contoh:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### POST /api/projects/{projectID}/api-specs/upload

Request: `multipart/form-data`, header `X-Requested-With: grimoire`.

```
--b
Content-Disposition: form-data; name="files"; filename="specs/payment.yaml"
Content-Type: application/yaml

openapi: 3.0.3
...
--b
Content-Disposition: form-data; name="files"; filename="specs/package.json"
Content-Type: application/json

{"name":"x","version":"1.0.0"}
--b--
```

Opsional field `name` (hanya bila tepat 1 file): `Content-Disposition: form-data; name="name"` -> `payment-v2`.

Response 200:

```json
{
  "results": [
    {
      "file": "payment.yaml",
      "source_path": "specs/payment.yaml",
      "name": "payment",
      "status": "updated",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "3.0.3",
      "title": "Payment API",
      "api_version": "1.4.0",
      "endpoint_count": 12
    },
    {
      "file": "package.json",
      "source_path": "specs/package.json",
      "name": "package",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "bukan dokumen OpenAPI",
      "issues": [
        { "code": "NOT_OPENAPI", "message": "bukan dokumen OpenAPI", "pointer": "#", "line": 1 }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "created": 0, "updated": 1, "unchanged": 0, "skipped": 0, "rejected": 1 }
}
```

Contoh issue `$ref` eksternal: `{"code":"EXTERNAL_REF","message":"referensi eksternal tidak didukung dan tidak di-fetch: http://example.com/common.yaml (baris 40, #/paths/~1pets/get/responses/200/content/application~1json/schema)","pointer":"#/paths/~1pets/get/responses/200/content/application~1json/schema","line":40}`.

Error request:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "field name hanya boleh dipakai saat mengunggah tepat satu file", "details": { "reason": "NAME_WITH_MULTIPLE_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter", "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } } }, "trace_id": "..." }
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran upload melebihi batas" }, "trace_id": "..." }
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

### POST /api/projects/{projectID}/api-specs/validate

Request sama dengan upload. Response 200:

```json
{
  "results": [
    {
      "file": "auth.yaml",
      "source_path": "auth.yaml",
      "name": "auth",
      "status": "valid",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "3.1.0",
      "title": "Auth API",
      "api_version": "2.0.0",
      "endpoint_count": 5
    },
    {
      "file": "legacy.yaml",
      "source_path": "legacy.yaml",
      "name": "legacy",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1",
      "issues": [
        { "code": "NOT_OPENAPI_3", "message": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)", "pointer": "#/swagger", "line": 1 }
      ],
      "openapi_version": null, "title": null, "api_version": null, "endpoint_count": null
    }
  ],
  "summary": { "valid": 1, "skipped": 0, "rejected": 1 }
}
```

### GET /api/projects/{projectID}/api-specs

```json
{
  "items": [
    {
      "name": "auth",
      "title": "Auth API",
      "openapi_version": "3.1.0",
      "api_version": "2.0.0",
      "format": "yaml",
      "source_path": "specs/auth.yaml",
      "size_bytes": 18234,
      "endpoint_count": 5,
      "created_at": "2026-10-23T09:00:00Z",
      "created_by_label": "Budi",
      "updated_at": "2026-10-23T09:00:00Z",
      "updated_by_label": "Budi"
    }
  ],
  "total": 1,
  "limit": 100
}
```

### GET /api/projects/{projectID}/api-specs/{name}

```json
{
  "spec": { "name": "payment", "title": "Payment API", "openapi_version": "3.0.3", "api_version": "1.4.0", "format": "yaml", "source_path": "specs/payment.yaml", "size_bytes": 40211, "endpoint_count": 2, "created_at": "2026-10-23T09:00:00Z", "created_by_label": "Budi", "updated_at": "2026-10-24T10:00:00Z", "updated_by_label": "Sari" },
  "endpoints": [
    { "method": "GET", "path": "/payments", "operation_id": "listPayments", "summary": "Daftar pembayaran", "tags": ["payments"], "deprecated": false },
    { "method": "POST", "path": "/payments", "operation_id": "createPayment", "summary": "Buat pembayaran", "tags": ["payments"], "deprecated": false }
  ],
  "document": {
    "openapi": "3.0.3",
    "info": { "title": "Payment API", "version": "1.4.0", "description": "Deskripsi **Markdown** (disanitasi di FE)" },
    "servers": [ { "url": "https://api.example.com" } ],
    "paths": {
      "/payments": {
        "get": { "operationId": "listPayments", "tags": ["payments"], "responses": { "200": { "description": "OK" } } },
        "post": { "operationId": "createPayment", "requestBody": { "$ref": "#/components/requestBodies/Payment" }, "responses": { "201": { "description": "Created" } } }
      }
    },
    "components": { "requestBodies": { "Payment": { "content": { "application/json": { "schema": { "type": "object" } } } } } }
  }
}
```

`document` mempertahankan `$ref` internal apa adanya (FE yang menelusurinya); urutan kunci sama dengan file asli (D4).

### GET /api/projects/{projectID}/api-specs/{name}/raw

Response 200, header:

```
Content-Type: application/yaml; charset=utf-8
Content-Disposition: attachment; filename="payment.yaml"
X-Content-Type-Options: nosniff
Cache-Control: no-store
```

Body: teks spec asli (tanpa BOM). Error tetap JSON seragam (404 `spec tidak ditemukan`).

### PATCH /api/projects/{projectID}/api-specs/{name}

Request:

```json
{ "name": "payment-v2" }
```

Response 200:

```json
{ "spec": { "name": "payment-v2", "title": "Payment API", "openapi_version": "3.0.3", "api_version": "1.4.0", "format": "yaml", "source_path": "specs/payment.yaml", "size_bytes": 40211, "endpoint_count": 2, "created_at": "2026-10-23T09:00:00Z", "created_by_label": "Budi", "updated_at": "2026-10-25T08:00:00Z", "updated_by_label": "Sari" } }
```

Error:

```json
{ "error": { "code": "CONFLICT", "message": "nama spec sudah dipakai di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "wajib diisi" } } }, "trace_id": "..." }
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "..." }
```

### DELETE /api/projects/{projectID}/api-specs/{name}

Response 204 tanpa body. Error 404 seperti di atas.

### Bus (nilai Go, bukan JSON di wire)

```go
listspecs.Request{ProjectID: "0b6c..."}
// -> listspecs.Response{Items: []listspecs.Spec{{Name: "auth", Title: "Auth API", ApiVersion: "2.0.0", OpenapiVersion: "3.1.0", EndpointCount: 5, UpdatedAt: t}}}

listendpoints.Request{ProjectID: "0b6c...", Spec: "payment", Tag: "payments", Query: "refund", Limit: 50}
// -> listendpoints.Response{SpecFound: true, Items: []listendpoints.Endpoint{{Spec: "payment", Method: "POST", Path: "/payments/{id}/refund", OperationID: "refundPayment", Summary: "Refund", Tags: []string{"payments"}}}, Total: 1}

listendpoints.Request{ProjectID: "0b6c...", Spec: "paymnt"}
// -> listendpoints.Response{SpecFound: false, Specs: []string{"auth", "notification", "payment"}, Items: []listendpoints.Endpoint{}}

getendpoint.Request{ProjectID: "0b6c...", Spec: "payment", Method: "post", Path: "/payments", SchemaDepth: 2}
// -> getendpoint.Response{Found: true, Spec: "payment", Method: "POST", Path: "/payments",
//      Operation: {"operationId":"createPayment","parameters":[...],"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"customer":{"$ref":"#/components/schemas/Customer"}}}}}},"responses":{...}},
//      Servers: [{"url":"https://api.example.com"}], SecuritySchemes: {"bearer":{"type":"http","scheme":"bearer"}},
//      Truncated: false, EffectiveDepth: 2}

getendpoint.Request{ProjectID: "0b6c...", Spec: "payment", Method: "GET", Path: "/payment"}
// -> getendpoint.Response{Found: false, Reason: "ENDPOINT_NOT_FOUND", Spec: "payment",
//      Similar: []getendpoint.EndpointRef{{Method: "GET", Path: "/payments", OperationID: "listPayments", Summary: "Daftar pembayaran"}}}

getendpoint.Request{ProjectID: "bukan-uuid", Method: "GET", Path: "/x"}
// -> error membungkus errs.ErrInvalid

contentcount.Request{ProjectIDs: []string{"0b6c...", "9a1f...", "x"}}
// -> contentcount.Response{Counts: map[string]int{"0b6c...": 3}}
```

### Audit (`record.Request` yang dikirim, contoh `apispec.uploaded`)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "<user uuid>", ActorLabel: "Budi",
    ProjectID: "0b6c...", ProjectLabel: "",
    Action: record.ActionAPISpecUploaded, TargetType: "api_spec", Target: "payment",
    Args: map[string]any{"name": "payment", "source_path": "specs/payment.yaml", "openapi_version": "3.0.3",
        "endpoint_count": 12, "size_bytes": 40211, "overwritten": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "6f1c2a0b9d8e4f17",
}
```

---

## 16. Risiko dan catatan

1. **Urutan merge T03.** Migrasi T06 mereferensikan `projects(id)`; T06 tidak boleh merge sebelum migrasi T03. Kontrak `checkaccess`/`contentcount` boleh dibuat T06 bila T03 belum merge (isi persis plan T03).
2. **Memori upload.** Semua part dibaca ke memori sebelum diproses (maks `MAX_UPLOAD_BYTES`, default 50 MB per request) agar 413 tidak meninggalkan simpanan parsial. Diterima untuk tim internal; dicatat di `docs/configuration.md`.
3. **Parse ulang di `getspec`/`get-endpoint`.** Spec <= 5 MB dan sudah tervalidasi; tidak ada syarat performa untuk keduanya. Bila kelak lambat, cache hasil parse adalah perubahan terpisah.
4. **Validasi struktural, bukan validasi skema penuh.** Validator tidak memeriksa JSON Schema di dalam `schemas` atau kecocokan `{param}`. Ini sengaja (toleran terhadap spec nyata).
5. **3.1 `$id`-relatif** ditolak sebagai `EXTERNAL_REF` (D3); dicatat di pesan dan tiket.
6. **Kunci integer di JSON browser.** `JSON.parse` menaruh kunci integer-like (`"200"`, `"404"`) lebih dulu dalam urutan menaik; urutan response code di FE mengikuti itu (dapat diterima).
7. **Advisory lock hash collision** antar project hanya menyebabkan serialisasi berlebih, tidak salah hasil.

---

## 17. Daftar usecase (plan rinci ditulis terpisah di `docs/superpowers/plans/project-api-spec/usecase-<slug>.md`)

| # | Slug | Jenis | Titik masuk | Store/repo | Bus dikonsumsi | Audit | AC |
|---|---|---|---|---|---|---|---|
| 1 | `uploadspecs` | HTTP | `POST /api/projects/{projectID}/api-specs/upload` | `LockProjectSpecs`, `LockSpec`, `CountSpecs`, `ReplaceEndpoints`; repo: `InsertSpec`, `UpdateSpecContent`, `InTx` | check-access, audit | `apispec.uploaded` | 1, 2, 3, 6, 7 |
| 2 | `validatespec` | HTTP | `POST /api/projects/{projectID}/api-specs/validate` | - (tanpa repository) | check-access | - | 2, 3, 6 |
| 3 | `listspecs` | HTTP | `GET /api/projects/{projectID}/api-specs` | `ListSpecs` | check-access | - | 7 |
| 4 | `getspec` | HTTP | `GET /api/projects/{projectID}/api-specs/{name}` | `FindSpec`, `ListSpecEndpoints` | check-access | - | 6, 7 |
| 5 | `downloadspec` | HTTP | `GET /api/projects/{projectID}/api-specs/{name}/raw` | `FindSpec` | check-access | - | 7 |
| 6 | `renamespec` | HTTP | `PATCH /api/projects/{projectID}/api-specs/{name}` | `LockSpec`; repo: `RenameSpec`, `InTx` | check-access, audit | `apispec.updated` | 7 |
| 7 | `deletespec` | HTTP | `DELETE /api/projects/{projectID}/api-specs/{name}` | `LockSpec`, `CountEndpoints`; repo: `DeleteSpec`, `InTx` | check-access, audit | `apispec.deleted` | 7 |
| 8 | `listspecsbus` | bus | `project.apispec.list-specs` | `ListSpecs` | - | - | 7 |
| 9 | `listendpoints` | bus | `project.apispec.list-endpoints` | `ListSpecNames`; repo: `QueryEndpoints` | - | - | 4, 7 |
| 10 | `getendpoint` | bus | `project.apispec.get-endpoint` | `ListSpecNames`, `FindSpec`, `ListSpecEndpoints` | - | - | 5, 7 |
| 11 | `countspecs` | bus | `project.apispec.count-specs` | repo: `CountByProjects` | - | - | 7 |

Ringkasan per usecase:
- **uploadspecs**: multipart (`files` berulang, `name` opsional bila 1 file), `base/upload.Read`; `RequireMember`; per file: `NormalizeSourcePath`, `FormatFromExt` (skipped), TooLarge/TooMany (rejected), `name` atau `DeriveName` + `ValidateName`, dedup nama, `StripBOM`, `openapi.Parse` (rejected + issues), `ExtractEndpoints`, hash -> tx per file (3.5) -> created/updated/unchanged/rejected `SPEC_LIMIT_REACHED`; audit setelah commit; 200 `{results, summary}`.
- **validatespec**: sama dengan upload sampai `ExtractEndpoints`, tanpa DB dan audit; status `valid|skipped|rejected`; `name_exists` tidak diisi. Plan usecase ini merangkum unit test `base/openapi` utama (13.2).
- **listspecs**: `RequireMember` -> `ListSpecs`; 200 `{items, total, limit: APISPEC_MAX_SPECS}`.
- **getspec**: `RequireMember` -> `FindSpec` (404) -> `openapi.Parse` ulang (gagal -> 500 + log) -> 200 `{spec, endpoints, document}`.
- **downloadspec**: `RequireMember` -> `FindSpec` -> tulis teks asli dengan header 15; bukan `WriteJSON`.
- **renamespec**: params `{name}` wajib; `ValidateName` (400); `RequireMember` -> tx `LockSpec` (404) -> sama -> 200 tanpa tulis/audit -> UPDATE (23505 -> 409 `NAME_TAKEN`) -> audit `{from,to}` -> 200 `{spec}`.
- **deletespec**: `RequireMember` -> tx `LockSpec` (404) -> `CountEndpoints` -> DELETE (cascade) -> audit `{name, endpoint_count}` -> 204.
- **listspecsbus**: `bus.Expect[listspecs.Request]`; ProjectID bukan uuid -> `ErrInvalid`; `ListSpecs` -> `Response{Items}`.
- **listendpoints**: clamp; `Spec` tidak ada -> `SpecFound=false` + `Specs`; satu query parameterized JOIN `api_specs` (`project_id = $1`) dengan filter opsional spec/tag/query (ILIKE ter-escape), `COUNT(*) OVER()`, urutan kanonik, LIMIT/OFFSET; `Truncated`/`NextOffset`; EXPLAIN dicatat.
- **getendpoint**: validasi request (7.4) -> clamp -> pilih spec (SPEC_REQUIRED / SPEC_NOT_FOUND) -> `FindSpec` -> `Parse` -> `FindOperation` (tidak ada -> ENDPOINT_NOT_FOUND + `Similar` dari `ListSpecEndpoints`) -> `ResolveOperation` -> Response (`Hint` bila Truncated).
- **countspecs**: `bus.Expect[contentcount.Request]`; > `MaxProjectIDs` -> `ErrInvalid`; buang id bukan uuid; satu query GROUP BY -> `Response{Counts}`.
