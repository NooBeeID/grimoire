# T06 project/api-spec — Usecase `getspec` (GET /api/projects/{projectID}/api-specs/{name})

Melengkapi `docs/superpowers/plans/project-api-spec/overview.md` (F: migrasi, `base/`, `base/openapi`, `base/store`, `base/access`, `apispec/base.go`, `apispec.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `getspec` (U4, gelombang B). Jangan tertukar dengan `getendpoint` (handler bus) atau `downloadspec` (teks mentah).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack pada tiket; plan usecase ini backend saja. FE (halaman detail, pohon tag, renderer) ada di `frontend-plan.md` |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `getspec` (gelombang B, U4) |
| Endpoint | `/api/projects/{projectID}/api-specs/{name}` |
| Method | GET |
| Version | tanpa prefix versi di URL; `api/modules/project/apispec.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; otorisasi project lewat `access.Checker.RequireMember` (bus `project.management.check-access`, fail-closed) |
| Siapa | anggota project, super_admin |
| Bus dikonsumsi | `project.management.check-access` (via `base/access`) |
| Bus disediakan | tidak ada |
| Audit | tidak ada (aksi baca) |
| Tabel / migrasi | membaca `api_specs`, `api_spec_endpoints`; tidak ada migrasi |
| AC | AC6 (3.0 dan 3.1 dapat dirender), AC7 (non-anggota 404) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| allowedPaths | `.../apispec/getspec/`, `.../apispec/base.go` (satu baris `getspec.Init(d)`) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): `base.Spec`, `base.Endpoint`, `base.SpecSummary`, `base.SpecDTO`, `base.EndpointDTO`, `base.ToSummaryDTO`, `base.ToEndpointDTOs`, `base.ErrSpecNotFound`, `base.ErrProjectNotFound`; `openapi.Parse`, `openapi.ToJSON`, `openapi.Format`, `openapi.DefaultLimits`, `openapi.Issue`; `store.Store.FindSpec` dan `ListSpecEndpoints`; `access.New`/`access.Checker`; `apispec/base.go`; path `GET .../api-specs/{name}` + schema `ApiSpecDetail` di `api/modules/project/apispec.yml`; migrasi `<VERSION>_create_api_specs.sql`.

## Summary Plan

`getspec` mengembalikan metadata lengkap satu spec, daftar endpoint dari tabel indeks, dan dokumen OpenAPI terparsir untuk dirender FE. Alur usecase (urutan wajib overview 3.4):

1. Aktor dari `base.ActorFrom(reqctx.IdentityFrom(ctx))`; gagal -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `guard.RequireMember(ctx, projectID)`. Error -> dikembalikan apa adanya (404 bila bukan anggota/project tidak ada/id bukan uuid; 500 bila bus gagal). Langkah ini SEBELUM spec dicari.
3. `repo.FindSpec(ctx, projectID, name)`; `!found` -> `base.ErrSpecNotFound` (404).
4. `repo.ListSpecEndpoints(ctx, spec.ID)` -> `endpoints` (urutan kanonik dari SQL). `endpoint_count = len(endpoints)` (tanpa query COUNT tambahan; tabel indeks adalah sumber tunggal).
5. `openapi.Parse([]byte(spec.Content), openapi.Format(spec.Format), openapi.DefaultLimits)`. `len(issues) > 0` -> data rusak: log error (tanpa isi spec) dan kembalikan `fmt.Errorf` -> 500 generik. Sukses -> `openapi.ToJSON(doc)` -> `json.RawMessage`.
6. Susun `Result{Spec, Endpoints, Document}`; handler menjawab 200.

Keputusan:
- Non-anggota, project tidak ada, id bukan uuid menghasilkan respons byte-identik (404 `project tidak ditemukan`). Spec yang ada di project lain dan nama yang tidak ada: 404 `spec tidak ditemukan` hanya setelah lolos check-access.
- Store sudah memfilter `project_id` + `name`; spec project lain tidak mungkin terbaca.
- Dokumen dikirim sebagai `json.RawMessage` hasil `Map.MarshalJSON` (D4): urutan kunci sama dengan file asli, kunci non-string YAML (mis. `200:` integer) sudah dijadikan string oleh `Parse` langkah 5 sehingga `{"200": ...}` valid JSON. `$ref` internal dibiarkan; FE yang menelusuri. Skema 3.0 `nullable` dan 3.1 `type: [..., "null"]` tidak dinormalkan (AC6).
- Isi spec tidak tepercaya: dikirim sebagai data JSON dengan `Content-Type: application/json`, tidak pernah HTML. Sanitasi Markdown di FE.
- Parse ulang gagal hampir mustahil (spec divalidasi saat upload dengan parser yang sama, hash tersimpan). Bila terjadi, 500 generik + log `project_id`, `spec`, `feature`, kode issue pertama saja (bukan pesan yang bisa memuat potongan isi).
- Konsistensi baca: `FindSpec` dan `ListSpecEndpoints` dua query tanpa transaksi. Upload paralel di antara keduanya dapat menghasilkan endpoint dari versi lebih baru daripada content (race ringan). Dapat diterima untuk aksi baca; bila ingin ketat, repository membungkus keduanya dalam satu `BeginTx` read-only (`REPEATABLE READ`) — TIDAK dilakukan sekarang (dicatat sebagai risiko).
- Header `Cache-Control: no-store` (bergantung keanggotaan saat itu).
- Tidak ada `params.go`: hanya dua path param, query dan body diabaikan.
- Error infra dibungkus `fmt.Errorf("getspec: <langkah>: %w", err)`.
- Usecase hanya orkestrasi; SQL hanya di `base/store`; rule nama/format tidak diulang di sini (nama tidak divalidasi: nama tak valid cukup tidak ditemukan -> 404).

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Pastikan simbol F di atas ada. Baca signature nyata `respond.WriteJSON(w, status, v)`, `respond.WriteError(w, r, err)`, `middleware.RequireAuth()`, `reqctx.IdentityFrom`, dan pola `Init` feature gelombang A (mis. `listspecs`) agar identik. Pastikan nama param route `{projectID}` dan `{name}`.

**B1. `getspec/repository.go`** (tanpa SQL sendiri; mendelegasikan ke store)

```go
package getspec

type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
    return r.s.FindSpec(ctx, projectID, name)
}

func (r *Repo) ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error) {
    return r.s.ListSpecEndpoints(ctx, specID)
}
```

Overview 3.2 mengizinkan feature tanpa SQL memakai interface lokal di `usecase.go` tanpa `repository.go`; file ini dipertahankan hanya agar pola gelombang B seragam. Boleh dihapus dan `*store.Store` dipasang langsung ke interface `repository`.

**B2. `getspec/usecase.go`**

```go
type repository interface {
    FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
    ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error)
}

type Result struct {
    Spec      base.SpecSummary
    Endpoints []base.Endpoint
    Document  json.RawMessage
}

type Usecase struct {
    repo  repository
    guard access.Checker
    log   *slog.Logger
}

func NewUsecase(repo repository, guard access.Checker, log *slog.Logger) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID, name string) (Result, error) {
    if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
        return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
    }
    if err := u.guard.RequireMember(ctx, projectID); err != nil {
        return Result{}, err
    }
    sp, found, err := u.repo.FindSpec(ctx, projectID, name)
    if err != nil { return Result{}, fmt.Errorf("getspec: find spec: %w", err) }
    if !found { return Result{}, base.ErrSpecNotFound }
    eps, err := u.repo.ListSpecEndpoints(ctx, sp.ID)
    if err != nil { return Result{}, fmt.Errorf("getspec: list endpoints: %w", err) }

    doc, issues := openapi.Parse([]byte(sp.Content), openapi.Format(sp.Format), openapi.DefaultLimits)
    if len(issues) > 0 {
        u.log.ErrorContext(ctx, "getspec: parse ulang spec tersimpan gagal",
            "feature", "getspec", "project_id", projectID, "spec", name, "issue_code", issues[0].Code)
        return Result{}, fmt.Errorf("getspec: reparse stored spec %s: %s", sp.ID, issues[0].Code)
    }
    raw, err := openapi.ToJSON(doc)
    if err != nil { return Result{}, fmt.Errorf("getspec: marshal document: %w", err) }

    return Result{Spec: summaryOf(sp, len(eps)), Endpoints: eps, Document: raw}, nil
}
```

`summaryOf(sp base.Spec, n int) base.SpecSummary` adalah pemetaan field-ke-field (ID, ProjectID, Name, SourcePath, Title, APIVersion, OpenAPIVersion, Format, SizeBytes, EndpointCount=n, CreatedAt/By label, UpdatedAt/By label). Bila F sudah menyediakan konstruktor setara di `base` (mis. `Spec.Summary(n)`), pakai itu dan hapus fungsi lokal. `guard` adalah interface `access.Checker`; `sp.Content` tidak pernah di-log.

**B3. `getspec/handler.go`**

```go
type response struct {
    Spec      base.SpecDTO        `json:"spec"`
    Endpoints []base.EndpointDTO  `json:"endpoints"`
    Document  json.RawMessage     `json:"document"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "name"))
    if err != nil {
        respond.WriteError(w, r, err)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respond.WriteJSON(w, http.StatusOK, response{
        Spec:      base.ToSummaryDTO(res.Spec),
        Endpoints: base.ToEndpointDTOs(res.Endpoints),
        Document:  res.Document,
    })
}
```

Pastikan `respond.WriteJSON` memakai `json.Marshal` biasa (RawMessage disalin apa adanya, tanpa re-sort). Bila `WriteJSON` melakukan `SetEscapeHTML(false)` atau sejenisnya, tidak masalah; yang penting urutan kunci dokumen tidak berubah. Uji di Test Scenario.

**B4. `getspec/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(s), access.New(d.Bus), d.Log)
    d.API.With(middleware.RequireAuth()).
        Get("/projects/{projectID}/api-specs/{name}", newHandler(uc).ServeHTTP)
}
```

Hanya GET; PATCH dan DELETE pada pola yang sama didaftarkan `renamespec` dan `deletespec`. Route statis `upload`/`validate` hanya POST, sehingga GET `.../api-specs/upload` jatuh ke handler ini (dibuktikan `routes_test.go` di W).

**B5. Wiring.** Tambah satu baris `getspec.Init(d)` di `apispec/base.go` (`Register`), tanpa mengubah baris lain.

**B6. OpenAPI.** Verifikasi `api/modules/project/apispec.yml`: `GET /api/projects/{projectID}/api-specs/{name}`, `operationId: getApiSpec`, parameter `ProjectID` (uuid) dan `SpecName`, 200 `ApiSpecDetail` (`spec: ApiSpec`, `endpoints: ApiSpecEndpoint[]`, `document: object, additionalProperties: true`), 401, 404 (deskripsi: "juga untuk non-anggota dan id tidak valid"), 500. Tanpa `X-Requested-With`. Bila ada perubahan: `make api-bundle` lalu `make api-check`.

**B7. Verifikasi.** `go vet ./...`, `go test ./internal/domain/project/internal/apispec/getspec/...`, tes integrasi (`-tags integration`), `make api-check`, archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. Memakai tabel `api_specs` dan `api_spec_endpoints` dari `backend/migrations/project/<VERSION>_create_api_specs.sql` (foundation).

## Changes

Baru:
- `backend/internal/domain/project/internal/apispec/getspec/init.go`
- `.../getspec/handler.go`
- `.../getspec/usecase.go`
- `.../getspec/repository.go`
- `.../getspec/usecase_test.go` (unit, fake repository + fake `access.Checker`)
- `.../getspec/handler_test.go` (httptest + chi router)
- `.../getspec/getspec_integration_test.go` (tag `integration`)

Diubah:
- `backend/internal/domain/project/internal/apispec/base.go`: satu baris `getspec.Init(d)`.
- `api/modules/project/apispec.yml`, `api/root.yml`, `api/openapi.yml`: hanya bila verifikasi B6 menemukan selisih.

Tidak diubah: `base/**`, feature lain, `infra/**`, `lib/**`, migrasi.

## Test Scenario

Unit `usecase_test.go` (fake repository mencatat panggilan; fake guard):
- `TestGetSpec_NonMemberNotFoundBeforeSpecLookup`: guard mengembalikan `base.ErrProjectNotFound`; hasil error itu; `FindSpec` dan `ListSpecEndpoints` TIDAK dipanggil (counter 0). Ini inti AC7.
- `TestGetSpec_GuardInfraError`: guard mengembalikan error biasa -> error diteruskan, bukan nil; repo tidak dipanggil.
- `TestGetSpec_NoIdentity`: ctx tanpa identitas -> `CodeUnauthorized`.
- `TestGetSpec_SpecNotFound`: guard nil, `FindSpec` found=false -> `errors.Is(err, base.ErrSpecNotFound)`; `ListSpecEndpoints` tidak dipanggil.
- `TestGetSpec_RepoErrors`: error `FindSpec`/`ListSpecEndpoints` dibungkus `getspec:` dan bukan `*errs.Error`.
- `TestGetSpec_OK30`: content `petstore-3.0.yaml` (salinan kecil inline) -> `Spec.EndpointCount == len(endpoints)`, `Document` JSON valid, `openapi == "3.0.3"`.
- `TestGetSpec_OK31`: content 3.1 JSON dengan `type: ["string","null"]` -> dokumen memuat array itu apa adanya; `webhooks` ikut dokumen.
- `TestGetSpec_NonStringKeysBecomeStrings`: YAML dengan `responses: { 200: {description: OK}, 404: {...} }` (kunci integer) -> `json.Unmarshal(Document, &map[string]any)` sukses dan `paths./x.get.responses` punya kunci string `"200"` dan `"404"`.
- `TestGetSpec_DocumentKeyOrderPreserved`: spec dengan path `/zebra` sebelum `/alpha` -> `bytes.Index(doc, "/zebra") < bytes.Index(doc, "/alpha")`.
- `TestGetSpec_ReparseFailure`: content tersimpan rusak (`not: [valid`) -> error biasa (bukan `*errs.Error`, jadi 500 generik), log berisi `feature=getspec` dan TIDAK berisi potongan content (handler slog uji menangkap output).
- `TestGetSpec_EndpointsEmptyNotNil`: spec 3.1 hanya `webhooks` -> `Endpoints` kosong; DTO `endpoints` berupa `[]` (bukan `null`).

Handler `handler_test.go` (httptest, router chi dengan pola penuh, middleware identitas uji):
- 200 memuat tepat tiga kunci `spec`, `endpoints`, `document`; `Content-Type: application/json`; `Cache-Control: no-store`.
- 404 non-anggota: body identik byte-per-byte dengan 404 project tidak ada dan id bukan uuid (`details` tidak ada).
- 404 spec tidak ada: pesan `spec tidak ditemukan`.
- 500: body generik `{"error":{"code":"INTERNAL",...}}` tanpa teks error internal.
- `Content-Type` tidak pernah `text/html`; `X-Content-Type-Options: nosniff` bila disediakan middleware global.

Integrasi `getspec_integration_test.go` (tag `integration`, `db.TestDB(t)`, migrasi T03 + T06, bus nyata dengan handler palsu `check-access`):
- `TestT06_AC6_GetSpec30And31`: sisipkan `petstore-3.0.yaml` dan `petstore-3.1.json` lewat `store` (atau `uploadspecs` bila sudah ada), GET keduanya -> 200; `endpoint_count` sama dengan jumlah baris `api_spec_endpoints`; `document.openapi` sesuai.
- `TestT06_AC7_GetSpecNonMember404`: spec `payment` ada di project A; pengguna bukan anggota A -> 404 `project tidak ditemukan`; anggota A meminta spec `payment` lewat `projectID` B (ia anggota B, spec hanya di A) -> 404 `spec tidak ditemukan`; `projectID` bukan uuid -> 404 project; super_admin non-anggota -> 200.
- Kunci non-string melalui DB: spec YAML dengan `200:` integer tersimpan -> GET 200 dan `document` valid JSON dengan `"200"`.
- Urutan `endpoints` kanonik: path lalu `GET, PUT, POST, DELETE, ...`.
- Check-access tidak terdaftar di bus (`ErrNotRegistered`) -> 500 (fail-closed), bukan 200.
- Tidak ada panggilan `core.audit.record` (handler palsu audit mencatat 0 request).

## E2E Scenarios

Positif (Playwright, butuh T03 merge dan FE T06):
1. Anggota membuka `/projects/{id}/api-specs/payment`: header menampilkan judul, versi OpenAPI/API; pohon tag -> endpoint tampil sesuai `endpoints`; klik endpoint menampilkan panel dokumentasi (parameter, body, response, skema dapat dilipat, servers, security).
2. Spec 3.0 dengan `nullable: true` dan spec 3.1 dengan `type: ["string","null"]` keduanya tampil tanpa error (AC6).
3. Spec dengan response code `200`/`404` menampilkan kode status dengan benar.
4. Super Admin (bukan anggota) membuka halaman yang sama -> tampil.
5. Pencarian endpoint di pohon memfilter tanpa request baru ke server.

Negatif:
1. Developer bukan anggota membuka URL spec project lain -> halaman 404 (tanpa membedakan project vs spec).
2. Anggota membuka nama spec yang tidak ada -> halaman 404 spec.
3. Spec dengan deskripsi `<script>alert(1)</script>` dan tautan `javascript:` -> tidak ada dialog, tautan tidak aktif (AC8, diuji di FE; backend hanya menjamin JSON).
4. Pengguna belum login -> 401 dan diarahkan ke login.
5. Gangguan bus check-access (disimulasikan di tes backend, bukan E2E) -> pesan galat generik, bukan konten spec.

## Request/Response body

Request: `GET /api/projects/0b6c3f5e-2d4a-4a8e-9f3b-1c2d3e4f5a6b/api-specs/payment` (tanpa body, tanpa header CSRF karena GET).

Response 200:

```json
{
  "spec": {
    "name": "payment",
    "title": "Payment API",
    "openapi_version": "3.1.0",
    "api_version": "1.4.0",
    "format": "yaml",
    "source_path": "specs/payment.yaml",
    "size_bytes": 40211,
    "endpoint_count": 2,
    "created_at": "2026-10-23T09:00:00Z",
    "created_by_label": "Budi",
    "updated_at": "2026-10-24T10:00:00Z",
    "updated_by_label": "Sari"
  },
  "endpoints": [
    { "method": "GET", "path": "/payments", "operation_id": "listPayments", "summary": "Daftar pembayaran", "tags": ["payments"], "deprecated": false },
    { "method": "POST", "path": "/payments", "operation_id": "createPayment", "summary": "Buat pembayaran", "tags": ["payments"], "deprecated": false }
  ],
  "document": {
    "openapi": "3.1.0",
    "info": { "title": "Payment API", "version": "1.4.0", "description": "Deskripsi **Markdown** (disanitasi di FE)" },
    "servers": [ { "url": "https://api.example.com" } ],
    "paths": {
      "/payments": {
        "get": {
          "operationId": "listPayments",
          "tags": ["payments"],
          "responses": { "200": { "description": "OK" }, "404": { "description": "Tidak ada" } }
        },
        "post": {
          "operationId": "createPayment",
          "requestBody": { "$ref": "#/components/requestBodies/Payment" },
          "responses": { "201": { "description": "Created" } }
        }
      }
    },
    "components": {
      "requestBodies": {
        "Payment": { "content": { "application/json": { "schema": { "type": ["object", "null"] } } } }
      },
      "securitySchemes": { "bearer": { "type": "http", "scheme": "bearer" } }
    }
  }
}
```

Catatan: kunci `"200"`/`"404"` pada contoh berasal dari YAML `200:`/`404:` (integer) yang dikonversi menjadi string oleh parser; urutan kunci mengikuti file asli.

Error (format seragam rule 11.1):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
404 non-anggota, project tidak ada, atau `projectID` bukan uuid (byte-identik).

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
404 anggota, spec tidak ada di project itu.

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
401 tanpa sesi.

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server" }, "trace_id": "6f1c2a0b9d8e4f17" }
```
500 generik: check-access gagal, query gagal, atau parse ulang spec tersimpan gagal (detail hanya di log server).

Risiko dan catatan:
- Dua query tanpa transaksi (lihat Keputusan); race ringan dengan upload paralel diterima.
- Parse ulang per request bertambah CPU untuk spec hingga 5 MB; tidak ada syarat performa pada endpoint ini. Cache hasil parse adalah perubahan terpisah (overview 16.3).
- Respons `document` bisa beberapa MB; kompresi gzip diserahkan ke reverse proxy/middleware T00 bila ada.
