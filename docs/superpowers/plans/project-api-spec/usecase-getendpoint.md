# T06 project/api-spec — Usecase `getendpoint` (bus `project.apispec.get-endpoint`)

Melengkapi `docs/superpowers/plans/project-api-spec/overview.md` (F: migrasi, `base/`, `base/openapi`, `base/store`, kontrak bus, `apispec/base.go`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `getendpoint` (U10, gelombang B). Jangan tertukar dengan `getspec` (HTTP, dokumen penuh) atau `listendpoints` (daftar).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack pada tiket; usecase ini backend saja (tanpa HTTP, tanpa FE) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `getendpoint` (gelombang B, U10) |
| Endpoint | bus `project.apispec.get-endpoint` (konstanta `constants.BUS_PROJECT_APISPEC_GET_ENDPOINT`) |
| Method | Call (request/response, sinkron) |
| Version | v1 (endpoint baru) |
| Consumer | T09 (`discovery/apispectools`) |
| Auth | tidak ada di handler (pemanggil T09 sudah terikat key -> project lewat T07); WAJIB membatasi query pada `Request.ProjectID` (rule 6.10) |
| Bus dikonsumsi | tidak ada |
| Audit | tidak ada (baca) |
| Tabel / migrasi | membaca `api_specs`, `api_spec_endpoints`; tidak ada migrasi |
| AC | AC5 (inline, sirkular, tanpa skema lain), AC7 (isolasi project), sebagian AC6 (3.0/3.1) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| allowedPaths | `.../apispec/getendpoint/`, `.../apispec/base.go` (satu baris `getendpoint.Init(d)`) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): konstanta `BUS_PROJECT_APISPEC_GET_ENDPOINT`; kontrak `lib/contracts/project/apispec/getendpoint` (overview 7.4); `base.Spec`, `base.Endpoint`, `base.Methods`, `base.ValidMethod`; `openapi.Parse`, `openapi.Format`, `openapi.DefaultLimits`, `openapi.FindOperation`, `openapi.ResolveOperation`, `openapi.Similar`, `openapi.Candidate`, `openapi.Document`; `store.Store.ListSpecNames`, `FindSpec`, `ListSpecEndpoints`; `apispec/base.go`.

Catatan kontrak: file `getendpoint.go` kontrak sudah didefinisikan di overview 7.4 (F5). Jika saat implementasi file itu belum ada, hentikan; JANGAN mengarang bentuk lain. Pastikan isinya persis: konstanta `DefaultSchemaDepth=4`, `MaxSchemaDepth=8`, `DefaultMaxBytes=102400`, `ReasonSpecRequired|ReasonSpecNotFound|ReasonEndpointNotFound`, tipe `Request`, `Response`, `EndpointRef`.

## Summary Plan

`getendpoint` melayani agent: mengambil SATU operasi OpenAPI dari spec tersimpan, dengan `$ref` skema di-inline sampai kedalaman terbatas, penanda sirkular, hanya `servers` dan `securitySchemes` yang relevan, dan ukuran dibatasi (pemotongan bertahap). Tidak ada akses user; isolasi hanya lewat `ProjectID`.

Alur `Execute(ctx, req getendpoint.Request) (getendpoint.Response, error)`:

1. **Normalisasi + validasi request** (rule entity di usecase hanya memanggil fungsi murni; lihat Plan B2 untuk letak fungsi `normalize`):
   - `ProjectID` harus uuid -> else `errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)`.
   - `Method`, `Path`, `OperationID` di-trim. `Method` di-uppercase.
   - Tepat satu mode: (`Method` DAN `Path` keduanya terisi, `OperationID` kosong) ATAU (`OperationID` terisi, `Method` dan `Path` kosong). Selain itu (kosong semua; kedua mode terisi; hanya salah satu dari Method/Path) -> `ErrInvalid` dengan pesan spesifik.
   - `Method` (bila ada) harus `base.ValidMethod` setelah uppercase -> else `ErrInvalid`.
   - `Path` harus diawali `/` -> else `ErrInvalid` (path OpenAPI selalu begitu; menghindari pencarian sia-sia).
   - Clamp (bukan error): `SchemaDepth<=0` -> 4, `>8` -> 8; `MaxBytes<=0` -> 102400. (`MaxBytes` tanpa batas atas dari kontrak; tidak diclamp ke atas, tetapi dijaga `int` positif.)
2. **Pilih spec**:
   - `names := repo.ListSpecNames(ctx, projectID)`.
   - `Spec == ""`: `len(names)==1` -> pakai itu; selainnya (0 atau >=2) -> `Response{Found:false, Reason: SPEC_REQUIRED, Specs: names}` (Specs tidak pernah nil).
   - `Spec != ""` dan tidak ada di `names` -> `Response{Found:false, Reason: SPEC_NOT_FOUND, Specs: names}`. Pencocokan persis (nama sudah slug lowercase; tidak ada case-folding agar konsisten dengan HTTP).
3. **Ambil content**: `repo.FindSpec(ctx, projectID, specName)` (dibatasi ProjectID). `!found` (race: dihapus di antara dua query) -> perlakukan sebagai `SPEC_NOT_FOUND` dengan `Specs` dimuat ulang dari `names` tanpa spec itu.
4. **Parse**: `openapi.Parse([]byte(spec.Content), openapi.Format(spec.Format), openapi.DefaultLimits)`. Issue > 0 (data rusak, hampir mustahil) -> log error tanpa isi spec (`feature`, `project_id`, `spec`, `issue_code` pertama) dan `fmt.Errorf("getendpoint: reparse stored spec: %s", code)` -> error infra (T09 yang mengubahnya menjadi pesan generik).
5. **Cari operasi**: `openapi.FindOperation(doc, method, path, operationID)`. `ok=false` -> `repo.ListSpecEndpoints(ctx, spec.ID)` -> `[]openapi.Candidate` -> `openapi.Similar(cands, method, path, operationID, 5)` -> `Response{Found:false, Reason: ENDPOINT_NOT_FOUND, Spec: specName, Similar: [...]}` (Similar tidak pernah nil).
6. **Resolve**: `openapi.ResolveOperation(doc, method2, path2, depth, maxBytes)` -> `Response{Found:true, Spec, Method: method2 (uppercase), Path: path2, Operation, Servers, SecuritySchemes, Truncated, EffectiveDepth}`. Bila `Truncated`: `Hint = fmt.Sprintf("hasil dipotong; turunkan schema_depth (dipakai %d)", EffectiveDepth)`.

Keputusan:
- **Tidak ada check-access** (overview 3.4). Satu-satunya pengaman: setiap query store memakai `ProjectID` request. Tes AC7 membuktikan.
- **Tidak ada error untuk "tidak ketemu"**: semua hasil bisnis lewat `Found=false` + `Reason` (kontrak 7.4). Error Go hanya untuk request tidak valid (membungkus `errs.ErrInvalid`) dan kegagalan infra.
- **`ErrInvalid` dibungkus**, bukan dikembalikan sebagai sentinel mentah: `errs.Wrap(errs.CodeInvalid, "<pesan>", errs.ErrInvalid)` agar `errors.Is(err, errs.ErrInvalid)` benar (kontrak kontrak hanya stdlib; pembungkusan terjadi di provider).
- **Dua query untuk spec (ListSpecNames lalu FindSpec)**: ListSpecNames diperlukan untuk SPEC_REQUIRED/SPEC_NOT_FOUND (daftar nama). Untuk jalur umum (`Spec` diisi dan benar) dapat dihemat: bila `Spec != ""`, panggil `FindSpec` lebih dulu; hanya bila `!found` panggil `ListSpecNames`. Bila `Spec == ""`, panggil `ListSpecNames` dulu. Implementasikan optimasi ini (satu query pada jalur sukses dengan nama diisi) karena T09 memanggil endpoint ini berulang.
- **Tanpa cache parse**: spec <= 5 MB; parse ulang diterima (overview risiko 16.3).
- **`Spec` pada Response** selalu diisi bila spec terpilih (Found true atau ENDPOINT_NOT_FOUND); kosong pada SPEC_REQUIRED/SPEC_NOT_FOUND.
- **Method pada ENDPOINT_NOT_FOUND**: `Response.Method/Path` dikosongkan (hanya `Similar`); agen memakai `Similar`.
- **`EffectiveDepth`** pada ENDPOINT_NOT_FOUND = 0 (tidak relevan).
- Log: tidak pernah me-log isi spec, operation, atau path/operationId mentah dari agent pada level di atas debug.
- Usecase hanya orkestrasi; rule bisnis (resolusi, truncation, similar, validasi method) di `base` / `base/openapi`; SQL hanya di `base/store`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Cek simbol F ada (daftar di Metadata). Baca signature nyata `bus.Expect[T](req any) (T, error)` (`backend/internal/infra/bus/typed.go`), `bus.Handler = func(ctx, req any) (any, error)`, `d.Bus.Register(endpoint, handler)`, `errs.Wrap`, `errs.ErrInvalid`. Baca pola `Init` feature bus gelombang A (`countspecs`) bila sudah ada agar identik.

**B1. `getendpoint/repository.go`** (tanpa SQL sendiri; delegasi ke store)

```go
package getendpoint

type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) ListSpecNames(ctx context.Context, projectID string) ([]string, error) {
    return r.s.ListSpecNames(ctx, projectID)
}
func (r *Repo) FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
    return r.s.FindSpec(ctx, projectID, name)
}
func (r *Repo) ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error) {
    return r.s.ListSpecEndpoints(ctx, specID)
}
```

Overview 3.2 mengizinkan feature tanpa SQL sendiri memakai interface lokal di `usecase.go` tanpa `repository.go`; file ini dipertahankan agar pola feature bus seragam dan boleh dihapus (pasang `*store.Store` langsung ke interface `repository`).

**B2. `getendpoint/usecase.go`**

```go
type repository interface {
    ListSpecNames(ctx context.Context, projectID string) ([]string, error)
    FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
    ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error)
}

type Usecase struct {
    repo repository
    log  *slog.Logger
}

func NewUsecase(repo repository, log *slog.Logger) *Usecase

func (u *Usecase) Execute(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error)
```

Isi `Execute` mengikuti Summary Plan langkah 1-6. Fungsi privat `normalize(req) (params, error)` di `usecase.go` (hanya memanggil `uuid.Parse`, `strings`, `base.ValidMethod`, konstanta kontrak; tidak ada aturan baru): mengembalikan struct `params{projectID, spec, method, path, operationID string; depth, maxBytes int}`. Pesan error (Indonesia):

| Kondisi | Pesan |
|---|---|
| ProjectID bukan uuid | `project_id tidak valid` |
| tidak ada Method+Path maupun OperationID | `isi method dan path, atau operation_id` |
| keduanya terisi | `isi method dan path, atau operation_id; tidak boleh keduanya` |
| hanya salah satu Method/Path | `method dan path harus diisi bersamaan` |
| method tidak dikenal | `method tidak valid` |
| path tidak diawali `/` | `path harus diawali '/'` |

Semua dibungkus `errs.Wrap(errs.CodeInvalid, msg, errs.ErrInvalid)`.

Bila F belum menyediakan `Similar`/`FindOperation` dengan signature persis overview 5.7/5.8, hentikan; jangan menulis ulang logika resolusi di usecase.

Pemetaan `[]base.Endpoint` -> `[]openapi.Candidate{Method, Path, OperationID (deref ""), Summary}` dilakukan di fungsi privat `toCandidates` (pemetaan murni, bukan rule bisnis). Pemetaan `openapi.Candidate` -> `getendpoint.EndpointRef` di `toRefs`.

**B3. `getendpoint/handler.go`**

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    r, err := bus.Expect[getendpoint.Request](req)
    if err != nil { return nil, err }
    return h.uc.Execute(ctx, r)   // Response nilai, bukan pointer
}
```

Request dan response adalah NILAI (bukan pointer), sesuai kontrak. Tanpa `params.go` (rule 6.9). Return `getendpoint.Response` sebagai `any`; jangan `*Response`.

**B4. `getendpoint/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(store.New(d.DB)), d.Log)
    d.Bus.Register(constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, newHandler(uc).Handle)
}
```

**B5. Wiring**: tambah satu baris `getendpoint.Init(d)` di `apispec/base.go` (tambah import). Jangan mengubah baris lain.

**B6. Verifikasi**: `go build ./...`, `go vet ./...`, `go test ./internal/domain/project/internal/apispec/getendpoint/...`, `go test -tags integration` untuk integrasi, `golangci-lint run ./internal/domain/project/...`, archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. Membaca tabel `api_specs` dan `api_spec_endpoints` dari migrasi F (`backend/migrations/project/<VERSION>_create_api_specs.sql`).

## Changes

Baru:
- `backend/internal/domain/project/internal/apispec/getendpoint/init.go`
- `.../getendpoint/handler.go`
- `.../getendpoint/usecase.go`
- `.../getendpoint/repository.go`
- `.../getendpoint/usecase_test.go` (unit, repo palsu)
- `.../getendpoint/handler_test.go` (bus.Expect: tipe salah)
- `.../getendpoint/getendpoint_integration_test.go` (tag `integration`)
- `.../getendpoint/testdata/` bila diperlukan (spec uji untuk integrasi; boleh memakai `base/openapi/testdata` via salinan, tidak import lintas package test)

Diubah:
- `backend/internal/domain/project/internal/apispec/base.go`: satu baris `getendpoint.Init(d)` (+ import).

Tidak diubah: `base/`, `base/openapi`, `base/store`, kontrak, konstanta, migrasi, OpenAPI yml (endpoint bus tidak masuk `api/`), README (katalog sudah memuat baris `get-endpoint`; catatan kontrak ditambah di F12).

## Test Scenario

Penamaan: `TestT06_AC5_*`, `TestT06_AC7_*`; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `usecase_test.go` (repo palsu; tanpa DB)

Validasi request (tabel kasus; semua `errors.Is(err, errs.ErrInvalid)`):
- ProjectID `"x"`, `""` -> invalid.
- Method/Path/OperationID semua kosong -> invalid.
- Method+Path+OperationID semua terisi -> invalid.
- Hanya Method; hanya Path -> invalid.
- Method `"FETCH"` -> invalid; Method `"get"` -> diterima, dinormalisasi `GET`; spasi di-trim.
- Path tanpa `/` -> invalid.
- Repo tidak dipanggil sama sekali pada semua kasus invalid (assert counter 0).

Clamp:
- `SchemaDepth` 0, -1 -> 4; 100 -> 8; 3 -> 3. `MaxBytes` 0/-5 -> 102400; 500 -> 500 (verifikasi nilai yang diteruskan ke resolver lewat spec uji yang hasilnya bergantung depth/bytes, bukan lewat seam palsu).

Pemilihan spec:
- `Spec ""`, 1 spec -> spec itu dipakai, `Found=true`, `Response.Spec` = nama.
- `Spec ""`, 0 spec -> `Found=false`, `SPEC_REQUIRED`, `Specs=[]` (bukan nil).
- `Spec ""`, 2 spec -> `SPEC_REQUIRED`, `Specs=[a b]` urut.
- `Spec "nope"` -> `SPEC_NOT_FOUND`, `Specs` berisi nama valid.
- `Spec` benar: `ListSpecNames` TIDAK dipanggil pada jalur sukses (assert counter).
- Race: `FindSpec` !found setelah `ListSpecNames` menyebut ada -> `SPEC_NOT_FOUND`, `Specs` tanpa spec itu.

Pencarian:
- Lookup by method+path -> Found. Lookup by operationId -> Found; `Response.Method` uppercase dan `Path` terisi dari spec.
- operationId tidak ada -> `ENDPOINT_NOT_FOUND`, `Similar` maks 5, `Spec` terisi, `Operation` nil.
- `GET /pet/{id}` (typo) -> `Similar` memuat `GET /pets/{petId}`; urutan skor.
- Spec tersimpan rusak (content tak bisa di-parse) -> error non-invalid (bukan `ErrInvalid`), log tidak memuat isi spec.
- Error repo -> dibungkus `getendpoint: ...`, `errors.Is` ke error asal.

Resolusi (memakai spec uji kecil in-memory lewat repo palsu; logika dalamnya sudah diuji di `base/openapi`, di sini cukup pemetaan ke Response):
- `Truncated=true` -> `Hint` berisi `"turunkan schema_depth"` dan `EffectiveDepth` yang dipakai; `Truncated=false` -> `Hint ""`.
- `Servers` selalu array JSON (`[]` bila tidak ada), `SecuritySchemes` selalu object (`{}`), tidak pernah nil.

### Unit `handler_test.go`
- Request bertipe salah (`string`, `*getendpoint.Request`, nil) -> `errors.Is(err, bus.ErrBadRequestType)`.
- Request nilai valid -> return bertipe `getendpoint.Response` (bukan pointer).

### Integrasi `getendpoint_integration_test.go` (tag `integration`, `db.TestDB(t)`, bus nyata `bus.NewRegistry(log)`, `Init(d)` nyata)

Spec uji disimpan langsung lewat `store`/SQL fixture (isi dari `petstore-3.0.yaml`-setara dan `nested.yaml` di bawah), endpoint diindeks via `ExtractEndpoints`.

- `TestT06_AC5_GetEndpointInlinesNested`: spec dengan `POST /orders` -> `requestBody` `$ref Order` -> `Order.customer` `$ref Customer` -> `Customer.address` `$ref Address`. Depth 4: `Operation` berisi skema Order, Customer, Address ter-inline (tanpa `$ref` tersisa). Depth 1: level kedua tetap `{"$ref": "#/components/schemas/Customer"}`. Assert via `json.Unmarshal` ke `map[string]any`, bukan string compare.
- `TestT06_AC5_GetEndpointMarksCircular`: `Node.children.items -> Node` -> hasil memuat `{"$circular": "#/components/schemas/Node"}`, selesai < 1 detik, tidak loop.
- `TestT06_AC5_GetEndpointExcludesUnreferencedSchemas`: `components.schemas.Unused` / `Secret` tidak muncul (`bytes.Contains(Operation,"Unused")` false; juga tidak ada kunci `components` di Operation). `securitySchemes` hanya skema yang dipakai operasi (operasi `security: [{bearer: []}]` -> hanya `bearer`; operasi `security: []` -> `{}`).
- `TestT06_AC5_GetEndpointPathLevelParametersMerged`: parameter level path ikut, override operasi menang untuk pasangan `name`+`in` sama.
- `TestT06_AC5_GetEndpointServersFallback`: operasi > path item > root > `[]`.
- `TestGetEndpoint_Truncation`: `MaxBytes` kecil -> `Truncated=true`, `EffectiveDepth < depth awal`, `Hint` terisi; ukuran `len(Operation)+len(Servers)+len(SecuritySchemes) <= MaxBytes`. Depth 0 masih terlalu besar -> `Operation` berisi `"$truncated": true`, `operationId`, `summary`.
- `TestGetEndpoint_LookupByOperationID`: `OperationID: "createOrder"` menghasilkan operasi sama dengan `POST /orders`; `Method`/`Path` pada Response terisi.
- `TestGetEndpoint_Spec31AndSpec30`: spec 3.0 (`nullable: true`) dan 3.1 (`type: ["string","null"]`) sama-sama di-resolve, kedua bentuk tipe dibiarkan apa adanya (AC6).
- `TestGetEndpoint_DefaultSpecSingle`: project dengan 1 spec, `Spec ""` -> Found. Project dengan 2 spec -> `SPEC_REQUIRED` + 2 nama. Project dengan 0 spec -> `SPEC_REQUIRED` + `[]`.
- `TestGetEndpoint_SpecNotFoundListsNames`, `TestGetEndpoint_EndpointNotFoundSimilar`.
- `TestT06_AC7_GetEndpointProjectIsolation`: project A punya `payment`; panggil dengan `ProjectID` B + `Spec: "payment"` -> `SPEC_NOT_FOUND` dan `Specs` tidak memuat data A; panggil B dengan `Spec ""` -> `SPEC_REQUIRED` `Specs=[]` (tidak bocor spec A walau A punya tepat 1 spec). Pastikan `Operation` tidak pernah berisi isi A.
- Registrasi: `d.Bus.Call(ctx, constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, getendpoint.Request{...})` tidak mengembalikan `bus.ErrNotRegistered`.
- Performa sanity: spec 5 MB (banyak path) `get-endpoint` untuk satu operasi selesai < 2 detik (tanpa SLA formal; mencegah regresi O(n^2)).

## E2E Scenarios

Usecase ini tidak punya UI atau route HTTP; E2E Playwright tidak berlaku. Skenario end-to-end dijalankan sebagai tes integrasi bus (di atas) dan dilanjutkan di E2E T09 (agent MCP memanggil `get_endpoint`).

Positif:
1. Agent meminta `POST /payments` dari spec `payment` yang punya skema bersarang -> operasi lengkap dengan skema ter-inline, `servers`, `securitySchemes` yang dipakai.
2. Agent meminta by `operation_id` -> hasil sama dengan by method+path.
3. Project dengan satu spec, agent tidak menyebut `spec` -> otomatis dipakai.
4. Hasil besar -> `Truncated=true` + `Hint`; agent menurunkan `schema_depth`, hasil lebih kecil dan `Truncated=false`.

Negatif:
1. Typo path (`GET /payment`) -> `ENDPOINT_NOT_FOUND` + `Similar` berisi `GET /payments`.
2. Nama spec salah -> `SPEC_NOT_FOUND` + daftar nama spec.
3. Beberapa spec dan `spec` kosong -> `SPEC_REQUIRED` + daftar nama.
4. Request tidak valid (method `FETCH`, tanpa path, kedua mode sekaligus, project_id bukan uuid) -> error bus membungkus `errs.ErrInvalid` (T09 mengubah menjadi pesan jelas).
5. Agent project B meminta spec project A -> `SPEC_NOT_FOUND`; tidak ada data A.
6. Spec berisi `$ref` sirkular -> penanda `$circular`, tidak hang.

## Request/Response body

Bus bertipe Go (nilai, bukan JSON di wire). Format error bus: `error` Go membungkus `errs.ErrInvalid` (T09 yang memetakan ke JSON-RPC/MCP). Bila error ini bocor ke HTTP lewat `respond.WriteError`, bentuk seragamnya:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "isi method dan path, atau operation_id" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### Sukses by method+path

```go
getendpoint.Request{ProjectID: "0b6c1e2a-4d3f-4a5b-9c7d-1e2f3a4b5c6d", Spec: "payment", Method: "post", Path: "/payments", SchemaDepth: 2}
```

```go
getendpoint.Response{
    Found: true, Spec: "payment", Method: "POST", Path: "/payments",
    Operation: json.RawMessage(`{
      "operationId": "createPayment",
      "summary": "Buat pembayaran",
      "tags": ["payments"],
      "parameters": [{"name": "Idempotency-Key", "in": "header", "required": false, "schema": {"type": "string"}}],
      "requestBody": {"required": true, "content": {"application/json": {"schema": {
        "type": "object",
        "properties": {
          "amount": {"type": "integer"},
          "customer": {"type": "object", "properties": {"name": {"type": "string"}, "address": {"$ref": "#/components/schemas/Address"}}}
        }}}}},
      "responses": {"201": {"description": "Created"}},
      "security": [{"bearer": []}]
    }`),
    Servers:         json.RawMessage(`[{"url":"https://api.example.com"}]`),
    SecuritySchemes: json.RawMessage(`{"bearer":{"type":"http","scheme":"bearer"}}`),
    Truncated: false, EffectiveDepth: 2,
}
```

### Sukses by operationId, referensi sirkular

```go
getendpoint.Request{ProjectID: "0b6c1e2a-...", Spec: "", OperationID: "getTree"}   // project punya tepat 1 spec
```

```go
getendpoint.Response{
    Found: true, Spec: "catalog", Method: "GET", Path: "/tree",
    Operation: json.RawMessage(`{"operationId":"getTree","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{
      "type":"object","properties":{"children":{"type":"array","items":{"$circular":"#/components/schemas/Node"}}}}}}}}}`),
    Servers: json.RawMessage(`[]`), SecuritySchemes: json.RawMessage(`{}`),
    Truncated: false, EffectiveDepth: 4,
}
```

### Terpotong

```go
getendpoint.Request{ProjectID: "0b6c1e2a-...", Spec: "payment", Method: "GET", Path: "/reports", SchemaDepth: 8, MaxBytes: 2000}
// ->
getendpoint.Response{Found: true, Spec: "payment", Method: "GET", Path: "/reports",
    Operation: json.RawMessage(`{...}`), Servers: json.RawMessage(`[]`), SecuritySchemes: json.RawMessage(`{}`),
    Truncated: true, EffectiveDepth: 3,
    Hint: "hasil dipotong; turunkan schema_depth (dipakai 3)"}
```

Fallback depth 0 masih terlalu besar:

```go
Operation: json.RawMessage(`{"$truncated":true,"operationId":"getReports","summary":"Laporan"}`), Truncated: true, EffectiveDepth: 0,
Hint: "hasil dipotong; turunkan schema_depth (dipakai 0)"
```

### Tidak ketemu

```go
// ENDPOINT_NOT_FOUND
getendpoint.Request{ProjectID: "0b6c1e2a-...", Spec: "payment", Method: "GET", Path: "/payment"}
getendpoint.Response{Found: false, Reason: "ENDPOINT_NOT_FOUND", Spec: "payment",
    Similar: []getendpoint.EndpointRef{
        {Method: "GET", Path: "/payments", OperationID: "listPayments", Summary: "Daftar pembayaran"},
        {Method: "POST", Path: "/payments", OperationID: "createPayment", Summary: "Buat pembayaran"},
    }}

// SPEC_NOT_FOUND
getendpoint.Request{ProjectID: "0b6c1e2a-...", Spec: "paymnt", Method: "GET", Path: "/payments"}
getendpoint.Response{Found: false, Reason: "SPEC_NOT_FOUND", Specs: []string{"auth", "notification", "payment"}}

// SPEC_REQUIRED
getendpoint.Request{ProjectID: "0b6c1e2a-...", OperationID: "createPayment"}   // project punya 3 spec
getendpoint.Response{Found: false, Reason: "SPEC_REQUIRED", Specs: []string{"auth", "notification", "payment"}}
```

### Request tidak valid (error Go, `errors.Is(err, errs.ErrInvalid)`)

```go
getendpoint.Request{ProjectID: "bukan-uuid", Method: "GET", Path: "/x"}                       // "project_id tidak valid"
getendpoint.Request{ProjectID: "0b6c1e2a-...", Method: "GET"}                                  // "method dan path harus diisi bersamaan"
getendpoint.Request{ProjectID: "0b6c1e2a-...", Method: "GET", Path: "/x", OperationID: "op"}   // "isi method dan path, atau operation_id; tidak boleh keduanya"
getendpoint.Request{ProjectID: "0b6c1e2a-...", Method: "FETCH", Path: "/x"}                    // "method tidak valid"
getendpoint.Request{ProjectID: "0b6c1e2a-..."}                                                  // "isi method dan path, atau operation_id"
```

Tipe request salah (mis. `"string"`): `bus.ErrBadRequestType`.
