# T06 project/api-spec — Usecase Plan: `listendpoints`

Tiket: `docs/tickets/T06-project-api-spec.md`. Plan bersama (keputusan mengikat, `base/`, `base/store`, migrasi, kontrak bus): `docs/superpowers/plans/project-api-spec/overview.md`. Dokumen ini hanya memuat usecase `listendpoints` (U9, gelombang B). Bila bertentangan dengan overview, overview menang.
Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk arsitektur). Tidak ada `CLAUDE.md` maupun `.claude/rules/`. Tidak ada knowledge doc module apispec.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack pada level ticket; USECASE INI backend saja (handler bus, tidak ada UI/REST. Konsumen: T09) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `listendpoints` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | bus `project.apispec.list-endpoints` (konstanta `constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS`) |
| Method | Call (sinkron, request/response) |
| Version | v1 (endpoint bus baru) |
| Kontrak | `backend/lib/contracts/project/apispec/listendpoints` (`Request`, `Response`, `Endpoint`, `DefaultLimit=100`, `MaxLimit=300`, `MaxQueryRunes=200`) |
| Provider / Consumer | T06 / T09 (discovery/apispectools) |
| Akses | Handler bus TIDAK memeriksa akses user (T09 sudah terikat key -> project lewat T07). WAJIB membatasi semua query pada `ProjectID` request (rule 6.10) |
| Audit | tidak ada (baca saja) |
| Bus dikonsumsi | tidak ada |
| AC | AC4 (500 endpoint < 100 ms, filter tag/teks/paginasi), AC7 (isolasi project) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/project/internal/apispec/listendpoints/`
- `backend/internal/domain/project/internal/apispec/base.go` (tepat satu baris: `listendpoints.Init(d)`)
- `backend/internal/domain/project/internal/apispec/apispec_integration_test.go` (hanya menambah tes bagian ini bila file sudah ada)
- `backend/lib/contracts/project/apispec/listendpoints/listendpoints.go` dan `backend/lib/constants/bus.go` HANYA bila foundation F belum membuatnya (lihat prasyarat)

## Konteks yang dimuat

- Overview T06: 3.2 (struktur feature, feature bus tanpa `params.go`), 3.6 (urutan kanonik, clamp), 4.1 (`Endpoint`, `Methods`), 6.1 (`store.ListSpecNames`), 7.1 dan 7.3 (konstanta, kontrak), 9 (migrasi + catatan indeks), 15 (contoh bus).
- Infra T00 di `main`: `app.Deps{Ctx, Config, Log, DB, Bus, ...}` (`internal/infra/app`); `bus.Registry.Register`, `bus.Expect[T]`, `bus.ErrBadRequestType`, `bus.CallTyped[T]` (`internal/infra/bus`); `db.Querier`, `db.TestDB(t)` (`internal/infra/db`); `errs` (`lib/common/errs`).
- Baca dulu sebelum menulis (jangan menebak): signature persis `bus.Handler` dan `bus.Expect` di `internal/infra/bus`; cara `base/store` memindai kolom `tags text[]` pada `ListSpecEndpoints` (driver pgx atau lib/pq; pakai cara yang sama); apakah `github.com/google/uuid` sudah ada di `go.mod`.

Prasyarat dari foundation F (HARUS sudah ada; bila belum, berhenti dan laporkan, kecuali dua butir pertama yang boleh dibuat sesuai aturan "yang merge lebih dulu membuat kontrak"):
- `constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS = "project.apispec.list-endpoints"` di `lib/constants/bus.go`.
- `lib/contracts/project/apispec/listendpoints/listendpoints.go` persis overview 7.3 (isi ada di bagian Request/Response body di bawah, tipe + doc comment).
- Migrasi `api_specs` + `api_spec_endpoints` (`migrations/project/<VERSION>_create_api_specs.sql`) dengan `api_specs_project_name_key (project_id, name)` dan `api_spec_endpoints_pkey (spec_id, method, path)`.
- `base`: `Endpoint`, `Methods`, `MethodOrder`. `base/store`: `New`, `Store.ListSpecNames`.

## Summary Plan

`listendpoints` adalah handler bus Call yang dipakai agent (lewat T09) untuk mendaftar endpoint sebuah project tanpa memuat spec. Handler memeriksa tipe request (`bus.Expect[listendpoints.Request]`), usecase menormalkan parameter (clamp, trim, escape), memastikan `ProjectID` uuid valid, memeriksa nama spec bila diisi, lalu menjalankan SATU query parameterized yang men-JOIN `api_specs` (`project_id = $1`) dengan `api_spec_endpoints`, memfilter spec/tag/teks, mengurutkan secara kanonik, dan memaginasi.

Keputusan di plan ini:

1. Tanpa migrasi dan tanpa indeks baru. Jalur akses: `api_specs_project_name_key` (prefix `project_id`, lalu `name`) -> `api_spec_endpoints_pkey` (prefix `spec_id`). Per project maks 100 spec; spec sampai ratusan endpoint. Filter teks (`ILIKE '%x%'`) tidak bisa memakai btree dan tidak perlu trigram pada skala ini: baris yang dipindai dibatasi pada endpoint milik SATU project (puluhan ribu baris di kasus terburuk, 100 spec x 500). EXPLAIN dicatat sebagai bukti (rule 8.8, lihat Test Scenario AC4).
2. Clamp bukan error (kontrak): `Limit<=0 -> 100`, `Limit>300 -> 300`, `Offset<0 -> 0`. Query/Tag/Spec di-trim; byte NUL dan UTF-8 tidak valid dibuang (Postgres menolak keduanya sebagai parameter text dan akan jadi 500); Query dipotong `MaxQueryRunes` (200 rune) SEBELUM escape. Setelah normalisasi, string kosong berarti "tanpa filter".
3. Escape ILIKE: `\` -> `\\`, `%` -> `\%`, `_` -> `\_`, lalu dibungkus `%...%` di Go (bukan di SQL). SQL memakai `ILIKE $n ESCAPE '\'` eksplisit. Input pengguna TIDAK PERNAH disambung ke teks SQL; semua lewat placeholder.
4. Tag dicocokkan persis, case-sensitive, ke elemen array: `$n = ANY(e.tags)`. Bukan substring.
5. Spec: cocok persis dengan `api_specs.name` (case-sensitive; nama hanya huruf kecil). Bila `Spec != ""`, usecase memanggil `ListSpecNames(projectID)` (satu SELECT kecil, maks 100 baris, memakai unique index). Spec tidak ada -> `Response{SpecFound:false, Specs:<nama valid>, Items:[]}` dan err nil, QUERY ENDPOINT TIDAK DIJALANKAN. `Spec == ""` -> `SpecFound=true` dan tidak ada query nama.
6. Satu query, tiga peran: `COUNT(*) OVER()` memberi `Total` (dihitung sebelum LIMIT/OFFSET). Edge: bila Offset melewati total, hasil 0 baris sehingga `Total` tak terbaca; repository lalu menjalankan satu query `SELECT count(*)` dengan klausa WHERE yang SAMA (konstanta SQL dipakai bersama) hanya pada kasus `len(rows)==0 && offset>0`. Kasus umum tetap satu query.
7. Urutan kanonik: `s.name COLLATE "C"`, `e.path COLLATE "C"`, lalu `array_position(ARRAY['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'], e.method)`. `COLLATE "C"` membuat urutan deterministik lintas collation database. Paginasi stabil karena kunci urut (name, path, method) unik per project (PK spec_id, method, path + unique project/name).
8. `Truncated = Offset+len(Items) < Total`; `NextOffset = Offset+len(Items)` bila Truncated, selain itu 0.
9. `ProjectID` bukan uuid -> `errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)` (error Go, tanpa query). Uuid valid tapi project tidak ada -> bukan error: `Items` kosong, `Total` 0 (handler bus tidak membocorkan keberadaan; pemanggil sudah divalidasi T07). Jika `Spec` diisi pada project tak ada: `SpecFound=false`, `Specs=[]`.
10. `Items`, `Specs`, dan `Endpoint.Tags` tidak pernah `nil` (slice kosong). `OperationID` NULL -> `""`.
11. Pemisahan tanggung jawab: aturan normalisasi/escape murni di `query.go` (fungsi murni, diuji unit tanpa mock; tetap di folder feature karena hanya dipakai di sini dan melarang feature saling import); usecase hanya orkestrasi (validasi id, cek spec, panggil repo, hitung Truncated/NextOffset, mapping ke kontrak); `repository.go` satu-satunya yang menyentuh SQL. Tanpa transaksi (read-only). Tanpa log data.
12. Error infra dibungkus `fmt.Errorf("listendpoints: <langkah>: %w", err)`; tidak ada log isi query.

## Plan

Urutan wajib; tulis tes dulu (TDD) per langkah.

### BE-0. Prasyarat

Verifikasi daftar prasyarat di atas. Konstanta dan kontrak: bila belum ada, buat persis seperti overview 7.1/7.3 (kontrak hanya stdlib, rule 4.5; hanya `time` tidak diperlukan di kontrak ini). Jangan membuat migrasi atau mengubah `base/*`.

### BE-1. `listendpoints/query.go` (aturan murni) + `query_test.go`

```go
package listendpoints

// Filter adalah parameter query yang sudah dinormalkan dan aman.
type Filter struct {
    ProjectID string
    Spec      string // "" = semua spec
    Tag       string // "" = tanpa filter
    Pattern   string // "" = tanpa filter; selain itu "%<escaped>%"
    Limit     int    // 1..MaxLimit
    Offset    int    // >= 0
}

// normalize: clamp Limit/Offset; sanitize (buang NUL & UTF-8 invalid, trim) Spec/Tag/Query;
// potong Query ke MaxQueryRunes rune; bentuk Pattern bila Query tidak kosong.
func normalize(req listendpoints.Request) Filter

func sanitize(s string) string          // strings.ToValidUTF8(s,"") lalu hapus "\x00" lalu TrimSpace
func escapeLike(s string) string        // "\" -> "\\", "%" -> "\%", "_" -> "\_" (backslash lebih dulu)
func validProjectID(s string) bool      // len==36 dan uuid.Parse ok (atau regexp bila google/uuid tidak di go.mod)
```

Tes dulu (tabel) lalu implementasi.

### BE-2. `listendpoints/repository.go` (SQL) + `usecase.go` antarmuka

```go
// Row = satu baris hasil: nama spec + endpoint (base.Endpoint, Tags tidak nil).
type Row struct {
    Spec string
    base.Endpoint
}

type Repo struct {
    q db.Querier
    s *store.Store
}

func newRepo(q db.Querier, s *store.Store) *Repo

// ListSpecNames mendelegasikan ke store.ListSpecNames (ORDER BY name).
func (r *Repo) ListSpecNames(ctx context.Context, projectID string) ([]string, error)

// QueryEndpoints menjalankan query utama; mengembalikan baris halaman ini dan total.
func (r *Repo) QueryEndpoints(ctx context.Context, f Filter) (rows []Row, total int, err error)
```

SQL (raw string Go; klausa WHERE di konstanta `whereClause` agar sama persis dengan query fallback):

```sql
-- whereClause
WHERE s.project_id = $1::uuid
  AND ($2::text = '' OR s.name = $2)
  AND ($3::text = '' OR $3 = ANY(e.tags))
  AND ($4::text = '' OR e.path ILIKE $4 ESCAPE '\'
                     OR e.summary ILIKE $4 ESCAPE '\'
                     OR e.operation_id ILIKE $4 ESCAPE '\')

-- main
SELECT s.name AS spec, e.method, e.path, e.operation_id, e.summary, e.tags, e.deprecated,
       COUNT(*) OVER() AS total
FROM api_specs s
JOIN api_spec_endpoints e ON e.spec_id = s.id
<whereClause>
ORDER BY s.name COLLATE "C", e.path COLLATE "C",
         array_position(ARRAY['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'], e.method)
LIMIT $5 OFFSET $6

-- fallback (hanya bila rows kosong dan Offset > 0)
SELECT count(*) FROM api_specs s JOIN api_spec_endpoints e ON e.spec_id = s.id <whereClause>
```

Catatan implementasi:
- Argumen: `f.ProjectID, f.Spec, f.Tag, f.Pattern, f.Limit, f.Offset` (fallback hanya 4 argumen pertama; pastikan placeholder `$5/$6` tidak muncul di query fallback). `operation_id` NULL aman: `NULL ILIKE ...` bernilai NULL pada OR.
- Row struct privat bertag `db` (`OperationID *string`, tags dipindai dengan cara yang sama seperti `store.ListSpecEndpoints`; bila ragu gunakan `to_jsonb(e.tags)` dipindai ke `[]byte` lalu `json.Unmarshal`). Entity tanpa tag. `Tags == nil` -> `[]string{}`.
- `Total` dari baris pertama; tidak ada baris dan `Offset==0` -> `Total=0`.
- `sqlx` `SelectContext` pada `r.q` (Querier); tidak ada `BeginTx` (read-only).
- Error DB dibungkus `fmt.Errorf("listendpoints: query endpoints: %w", err)`.

### BE-3. `listendpoints/usecase.go` (orkestrasi saja)

```go
type endpointReader interface {
    ListSpecNames(ctx context.Context, projectID string) ([]string, error)
    QueryEndpoints(ctx context.Context, f Filter) ([]Row, int, error)
}

type Usecase struct{ repo endpointReader }

func NewUsecase(repo endpointReader) *Usecase

func (u *Usecase) Execute(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error) {
    if !validProjectID(req.ProjectID) {
        return listendpoints.Response{}, errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)
    }
    f := normalize(req)
    if f.Spec != "" {
        names, err := u.repo.ListSpecNames(ctx, f.ProjectID)
        if err != nil { return ..., fmt.Errorf("listendpoints: list spec names: %w", err) }
        if !slices.Contains(names, f.Spec) {
            if names == nil { names = []string{} }
            return listendpoints.Response{SpecFound: false, Specs: names, Items: []listendpoints.Endpoint{}}, nil
        }
    }
    rows, total, err := u.repo.QueryEndpoints(ctx, f)
    if err != nil { return ..., fmt.Errorf("listendpoints: query endpoints: %w", err) } // jangan membungkus dua kali bila repo sudah menambah prefix; satu prefix cukup
    items := make([]listendpoints.Endpoint, 0, len(rows))
    for _, r := range rows { /* map: OperationID nil -> "", Tags dijaga non-nil */ }
    next := f.Offset + len(items)
    truncated := next < total
    if !truncated { next = 0 }
    return listendpoints.Response{SpecFound: true, Items: items, Total: total, Truncated: truncated, NextOffset: next}, nil
}
```

Tidak ada `Clock`, `audit`, `access`. Tidak ada rule bisnis baru di usecase (clamp/escape ada di `query.go`).

### BE-4. `listendpoints/handler.go`

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler

// Handle memenuhi bus.Handler. Sesuaikan signature dengan infra/bus di main.
func (h *Handler) Handle(ctx context.Context, req bus.Request /* tipe sebenarnya */) (any, error) {
    in, err := bus.Expect[listendpoints.Request](req)   // tipe salah -> error membungkus bus.ErrBadRequestType
    if err != nil { return nil, err }
    return h.uc.Execute(ctx, in)                         // Response dikembalikan sebagai NILAI, bukan pointer
}
```

Tanpa `params.go` (rule 3.4/overview 3.2). Tidak ada pembungkusan error tambahan di handler selain yang dari `Expect`.

### BE-5. `listendpoints/init.go` dan wiring

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(d.DB, store.New(d.DB)))
    d.Bus.Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, newHandler(uc).Handle)
}
```

Tambahkan satu baris `listendpoints.Init(d)` di `apispec/base.go` (jangan menyentuh baris feature lain).

### BE-6. Verifikasi

`go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/listendpoints/...`, tes integrasi (tag `integration`), archtest `TestT00_AC4_RepoIsClean` (tidak meng-import feature lain; hanya `base/*`, `lib/contracts`, `lib/constants`, infra). Catat hasil EXPLAIN di deskripsi PR (lihat AC4).

### Frontend

Tidak ada. Usecase ini tidak punya UI. T09 yang merender hasil untuk agent.

## Migration

Tidak ada. Tabel dan indeks berasal dari migrasi foundation `backend/migrations/project/<VERSION>_create_api_specs.sql`. Alasan tidak menambah indeks (rule 8.8): query selalu diawali `project_id` (unique index `api_specs_project_name_key`), JOIN ke `api_spec_endpoints_pkey` via prefix `spec_id`; filter tag/teks dikerjakan sebagai filter pada baris satu project. Bukti EXPLAIN WAJIB dicatat (AC4). Bila pada 100 spec x 500 endpoint median > 100 ms, itu temuan: laporkan dan usulkan indeks lewat perubahan migrasi foundation, jangan menambah migrasi dari feature ini.

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/apispec/listendpoints/init.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/handler.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/usecase.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/query.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/repository.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/query_test.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/usecase_test.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/handler_test.go`
- `backend/internal/domain/project/internal/apispec/listendpoints/listendpoints_integration_test.go` (tag `integration`)

Backend (ubah):
- `backend/internal/domain/project/internal/apispec/base.go`: tambah `listendpoints.Init(d)`.

Hanya bila F belum membuatnya:
- `backend/lib/constants/bus.go`: `BUS_PROJECT_APISPEC_LIST_ENDPOINTS` (+ komentar provider/consumer overview 7.1).
- `backend/lib/contracts/project/apispec/listendpoints/listendpoints.go` persis overview 7.3.

Tidak diubah: `base/*`, feature lain, `infra/**`, migrasi, `api/**` (tidak ada REST), frontend.

## Test Scenario

Penamaan: `TestT06_AC4_<Deskripsi>` / `TestT06_AC7_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `query_test.go` (fungsi murni, tanpa mock)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestNormalize_LimitClamp` | Limit `-5, 0, 1, 100, 300, 301, 100000` | `100, 100, 1, 100, 300, 300, 300` |
| `TestNormalize_OffsetClamp` | Offset `-1, 0, 42` | `0, 0, 42` |
| `TestNormalize_TrimsAndEmpties` | Spec/Tag/Query `"  x  "`, `"   "` | `"x"`; spasi saja -> `""`; Pattern `""` bila Query kosong |
| `TestNormalize_QueryTruncatedToRunes` | Query 250 rune multibyte (`é`) | tepat 200 rune sebelum escape, tidak memotong di tengah rune |
| `TestNormalize_StripsNULAndInvalidUTF8` | Query `"a\x00b\xffc"` | `"abc"` (tanpa NUL / byte invalid) |
| `TestEscapeLike` | `%`, `_`, `\`, `100%_x\y`, `plain` | `\%`, `\_`, `\\`, `100\%\_x\\y`, `plain` (backslash di-escape lebih dulu, tidak ter-escape dua kali) |
| `TestNormalize_PatternWrapped` | Query `a_b` | Pattern `%a\_b%` |
| `TestValidProjectID` | uuid valid; `""`; `bukan-uuid`; `0b6c...` salah panjang; `urn:uuid:...`; `{uuid}`; uuid + spasi | hanya bentuk kanonik 36 karakter yang `true` |

### Unit `usecase_test.go` (fake `endpointReader`, tanpa DB)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestExecute_InvalidProjectID` | `ProjectID` `"x"` | error `errors.Is(err, errs.ErrInvalid)`, `Code == INVALID_ARGUMENT`; repo tidak dipanggil sama sekali |
| `TestExecute_NoSpecFilterSkipsNameLookup` | `Spec==""` | `ListSpecNames` dipanggil 0 kali; `SpecFound==true` |
| `TestExecute_UnknownSpecReturnsNamesNoQuery` | `Spec="paymnt"`, nama `auth,payment` | `SpecFound=false`, `Specs=[auth payment]`, `Items` non-nil kosong, `QueryEndpoints` 0 panggilan, err nil |
| `TestExecute_UnknownSpecProjectWithoutSpecs` | nama `nil` | `Specs` = `[]string{}` (bukan nil) |
| `TestExecute_SpecIsCaseSensitive` | `Spec="Payment"`, nama `payment` | `SpecFound=false` |
| `TestExecute_KnownSpecQueriesWithFilter` | `Spec="payment"` | repo menerima Filter dengan Spec `payment`, ProjectID verbatim |
| `TestExecute_PassesNormalizedFilterToRepo` | Limit 9999, Offset -3, Query `50%` | Filter: Limit 300, Offset 0, Pattern `%50\%%` |
| `TestExecute_TruncatedAndNextOffset` | total 250, Offset 100, 100 baris | `Truncated=true`, `NextOffset=200` |
| `TestExecute_LastPageNotTruncated` | total 250, Offset 200, 50 baris | `Truncated=false`, `NextOffset=0` |
| `TestExecute_ExactFitNotTruncated` | total 100, Offset 0, 100 baris | `Truncated=false`, `NextOffset=0` |
| `TestExecute_EmptyResultNonNil` | repo 0 baris, total 0 | `Items != nil`, `len==0`, `Total==0`, `SpecFound=true` |
| `TestExecute_MapsRow` | `OperationID=nil`, `Tags=nil`; satu lagi dengan nilai | `OperationID==""`, `Tags` non-nil kosong; nilai dipetakan 1:1 termasuk `Spec`, `Deprecated` |
| `TestExecute_RepoErrorsWrapped` | `ListSpecNames` error; `QueryEndpoints` error | `errors.Is` ke error asli; bukan `errs.Error` bisnis |

### Unit `handler_test.go`

- `TestHandle_WrongRequestType`: kirim `string`, `*listendpoints.Request` (pointer), dan struct tipe lain -> error `errors.Is(err, bus.ErrBadRequestType)`; usecase tidak dipanggil. (Request dikirim sebagai NILAI; pointer ditolak sesuai kontrak.)
- `TestHandle_ReturnsResponseByValue`: hasil bertipe `listendpoints.Response` (bukan pointer), bisa dipakai `bus.CallTyped[listendpoints.Response]`.
- `TestHandle_PropagatesUsecaseError`.

### Integration `listendpoints_integration_test.go` (tag `integration`, `db.TestDB(t)`, bus nyata `bus.NewRegistry(log)` + `Init(d)`; migrasi T03 + T06 diterapkan; seed lewat SQL langsung/`store.ReplaceEndpoints` — feature ini tidak bergantung pada `uploadspecs`)

Semua pemanggilan lewat `bus.CallTyped[listendpoints.Response](ctx, reg, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, listendpoints.Request{...})`.

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT06_AC4_ListEndpoints500Fast` | project dengan 1 spec 500 endpoint (seed `generate_series`, method campur, tag campur); panggil `Spec="big", Limit=300` | median dari 5 panggilan (setelah 1 warm-up) < 100 ms; `Total==500`, `len(Items)==300`, `Truncated`, `NextOffset==300`. Juga skenario lebih berat: 100 spec x 500 endpoint di satu project, `Spec==""` dengan `Query="refund"` < 100 ms |
| `TestT06_AC4_ExplainRecorded` | jalankan `EXPLAIN (ANALYZE, BUFFERS)` untuk query utama pada seed 100x500 | `t.Log` mencetak rencana (disalin ke PR sebagai bukti rule 8.8); assert lunak: tidak ada `Seq Scan on api_specs` yang membaca project lain (filter `project_id` terpakai, mis. `Index Cond: (project_id = ...)` atau `Filter` pada scan kecil); tidak meng-assert bentuk plan lain karena planner bisa memilih seq scan pada tabel kecil |
| `TestT06_AC4_FilterByTag` | tag `payments`, `refunds`, `Payments`, tanpa tag | `Tag="payments"` hanya endpoint ber-tag persis itu (bukan `Payments`, bukan substring `pay`); endpoint dengan banyak tag cocok bila salah satu cocok |
| `TestT06_AC4_FilterByText` | cari `refund` | cocok di path, summary, ATAU operation_id; case-insensitive (`REFUND`); operation_id NULL tidak menyebabkan error/baris hilang bila path cocok |
| `TestT06_AC4_TextEscapePercent` | path `/discount/100%`, `/discount/1000`, `/a_b`, `/axb`, summary berisi `\` | `Query="%"` hanya yang mengandung `%` literal; `Query="100%"` hanya `/discount/100%`; bukan semua baris |
| `TestT06_AC4_TextEscapeUnderscore` | `Query="a_b"` | hanya `/a_b` (bukan `/axb`); `Query="_"` hanya yang punya `_` literal |
| `TestT06_AC4_TextEscapeBackslash` | summary `C:\temp`, `Query="\"` dan `Query="c:\t"` | cocok literal, tanpa error SQL ("LIKE pattern must not end with escape character" tidak boleh terjadi) |
| `TestT06_AC4_SQLInjectionStringsAreLiteral` | `Query`/`Tag`/`Spec` = `' OR 1=1 --`, `"; DROP TABLE api_specs;--` | `Query/Tag`: 0 hasil (kecuali seed memang berisi string itu), tabel utuh; `Spec`: `SpecFound=false`; tanpa error |
| `TestT06_AC4_CombinedFiltersAreAnded` | Spec + Tag + Query bersamaan | irisan semua filter |
| `TestT06_AC4_Pagination` | 2 spec (a: 7, b: 6 endpoint), `Limit=5` berulang mengikuti `NextOffset` | gabungan semua halaman = 13 baris tanpa duplikat/hilang, urutan sama dengan panggilan `Limit=300`; halaman terakhir `Truncated=false`, `NextOffset=0`; `Total==13` di tiap halaman |
| `TestT06_AC4_OffsetBeyondTotal` | `Offset=1000` pada total 13 | `Items` kosong non-nil, `Total==13` (via query fallback), `Truncated=false`, `NextOffset=0` |
| `TestT06_AC4_ClampOverBus` | `Limit=100000` pada spec 500 endpoint; `Limit=0`; `Offset=-10` | 300 item; 100 item; Offset diperlakukan 0 |
| `TestT06_AC4_CanonicalOrder` | seed acak: path `/b` & `/a`, method semua 8 | urut spec, path (byte-order, `/Z` < `/a`), lalu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE |
| `TestT06_AC4_UnknownSpec` | `Spec="nope"` pada project dengan `auth,payment` | `SpecFound=false`, `Specs=[auth payment]`, `Items=[]`, err nil |
| `TestT06_AC4_EmptyProject` | project tanpa spec; `Spec==""` | `SpecFound=true`, `Items=[]`, `Total=0` |
| `TestT06_AC4_NonUUIDProjectID` | `ProjectID="bukan-uuid"` | error membungkus `errs.ErrInvalid` (tanpa error cast SQL) |
| `TestT06_AC7_ProjectIsolation` | project A dan B, spec bernama sama `payment` di keduanya dengan endpoint berbeda; B punya spec `billing` | request `ProjectID=A` hanya mengembalikan endpoint A (baik `Spec="payment"` maupun `Spec==""`); `Spec="billing"` dengan `ProjectID=A` -> `SpecFound=false`, `Specs` hanya nama milik A (tidak membocorkan `billing`); `Total` hanya menghitung A |
| `TestT06_AC7_UnknownProjectUUID` | uuid valid tak ada | `Items=[]`, `Total=0`, tanpa error |
| `TestListEndpoints_NotRegisteredWithoutInit` | registry tanpa `Init` | `bus.ErrNotRegistered` (sanity wiring); dan setelah `Init` tidak |
| `TestListEndpoints_DeleteSpecRemovesFromIndex` | hapus spec lalu panggil lagi | endpoint spec tersebut hilang (regresi cascade FK) |

`TestRoutesRegistered` (W, bukan di feature ini) memuat pengecekan bahwa bus `project.apispec.list-endpoints` terdaftar.

## E2E Scenarios

Tidak ada UI atau REST untuk usecase ini; konsumen satu-satunya adalah T09 (tool MCP agent). Skenario E2E dijalankan di level bus (integration di atas) dan, setelah T07/T09 merge, dari sisi agent lewat tool `list_endpoints`. Skenario black-box:

Positif:
1. Agent memanggil `list_endpoints` untuk spec `payment` (3 spec terunggah via UI): menerima daftar urut path/method, `Total`, `Truncated=false`.
2. Agent memfilter `tag=payments` lalu `query=refund`: hanya endpoint relevan; spec 500 endpoint dibaca per halaman 100 dengan mengikuti `NextOffset` sampai `Truncated=false`.
3. Setelah developer mengunggah ulang `payment.yaml` (endpoint dihapus), panggilan berikutnya tidak lagi memuat endpoint yang dihapus.
4. Agent mengetik `%` atau `_` pada pencarian: tidak ada hasil "semua baris" yang menyesatkan.

Negatif:
1. Agent salah mengetik nama spec (`paymnt`): mendapat daftar nama spec valid (`SpecFound=false`), bukan error mentah.
2. Project tanpa spec: daftar kosong tanpa error.
3. Key project A tidak pernah melihat endpoint project B, bahkan bila nama spec sama atau menebak nama spec B.
4. `ProjectID` korup (tidak mungkin dari T07; diuji di level bus): error `INVALID_ARGUMENT`-setara, tidak ada 500 dari SQL.
5. Limit sangat besar atau negatif: dijepit, bukan ditolak.

## Request/Response body

Endpoint bus (nilai Go; tidak ada wire JSON). Kontrak `backend/lib/contracts/project/apispec/listendpoints/listendpoints.go` (persis overview 7.3):

```go
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

(Doc comment paket mengulang aturan: clamp, escape ILIKE, tag persis, `SpecFound`, urutan kanonik, `Truncated`/`NextOffset`, `ProjectID` bukan uuid -> error membungkus `errs.ErrInvalid`, request dikirim sebagai nilai.)

Contoh 1, filter spec + tag + teks:

```go
req := listendpoints.Request{ProjectID: "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10", Spec: "payment", Tag: "payments", Query: "refund", Limit: 50}
res, err := bus.CallTyped[listendpoints.Response](ctx, reg, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, req)
// res = listendpoints.Response{
//   SpecFound: true,
//   Items: []listendpoints.Endpoint{
//     {Spec: "payment", Method: "POST", Path: "/payments/{id}/refund", OperationID: "refundPayment",
//      Summary: "Refund pembayaran", Tags: []string{"payments"}, Deprecated: false},
//   },
//   Total: 1, Truncated: false, NextOffset: 0,
// }
```

Setara JSON bila T09 menyerialisasi:

```json
{
  "spec_found": true,
  "specs": [],
  "items": [
    { "spec": "payment", "method": "POST", "path": "/payments/{id}/refund", "operation_id": "refundPayment",
      "summary": "Refund pembayaran", "tags": ["payments"], "deprecated": false }
  ],
  "total": 1,
  "truncated": false,
  "next_offset": 0
}
```

Contoh 2, halaman terpotong (semua spec, Limit 100, total 250):

```go
listendpoints.Request{ProjectID: "0b6c...", Limit: 100, Offset: 0}
// -> Response{SpecFound: true, Items: <100 item>, Total: 250, Truncated: true, NextOffset: 100}
listendpoints.Request{ProjectID: "0b6c...", Limit: 100, Offset: 200}
// -> Response{SpecFound: true, Items: <50 item>, Total: 250, Truncated: false, NextOffset: 0}
```

Contoh 3, clamp (bukan error): `Limit: 100000, Offset: -5` -> dijalankan sebagai `Limit 300, Offset 0`.

Contoh 4, spec tidak ada (err nil):

```go
listendpoints.Request{ProjectID: "0b6c...", Spec: "paymnt"}
// -> Response{SpecFound: false, Specs: []string{"auth", "notification", "payment"}, Items: []listendpoints.Endpoint{}, Total: 0}
```

Contoh 5, offset melewati total:

```go
listendpoints.Request{ProjectID: "0b6c...", Spec: "payment", Offset: 1000}
// -> Response{SpecFound: true, Items: []listendpoints.Endpoint{}, Total: 12, Truncated: false, NextOffset: 0}
```

Contoh 6, project tanpa spec / uuid tidak dikenal:

```go
// -> Response{SpecFound: true, Items: []listendpoints.Endpoint{}, Total: 0}
```

Error (hanya Go error pada bus; tidak ada format HTTP):

| Kasus | Hasil |
|---|---|
| Request bukan `listendpoints.Request` (nilai) | error membungkus `bus.ErrBadRequestType` |
| `ProjectID` bukan uuid | `errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)` -> `errors.Is(err, errs.ErrInvalid)`, `Code == INVALID_ARGUMENT` |
| Error DB | `fmt.Errorf("listendpoints: query endpoints: %w", err)` (T09 memetakannya ke error generik untuk agent) |
| Handler belum terdaftar | `bus.ErrNotRegistered` (dari pemanggil) |

Bila T09 meneruskan error ke klien MCP, bentuk seragam rule 11.1 tetap dikendalikan T09, contoh:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "project_id tidak valid" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
