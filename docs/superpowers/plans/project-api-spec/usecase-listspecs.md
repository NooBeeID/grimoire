# T06 project/api-spec — Usecase Plan: `listspecs`

Tiket: `docs/tickets/T06-project-api-spec.md`. Plan bersama (keputusan mengikat, `base/`, `base/store`, `base/access`, migrasi, config, OpenAPI): `docs/superpowers/plans/project-api-spec/overview.md`. Dokumen ini hanya memuat usecase `listspecs` (U3, gelombang A). Bila bertentangan dengan overview, overview menang.
Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk arsitektur). Tidak ada `CLAUDE.md` maupun `.claude/rules/`. Tidak ada knowledge doc module apispec.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (backend Go di dokumen ini; slice FE daftar spec dirinci di `frontend-plan.md`, bagian ini hanya menetapkan kontrak) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `listspecs` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | `/api/projects/{projectID}/api-specs` |
| Method | GET |
| Version | tanpa prefix versi di URL (prefix `/api`); `api/modules/project/apispec.yml` `info.version: 0.1.0` |
| Akses | `RequireAuth`; anggota project atau super_admin. Non-anggota, project tidak ada, `projectID` bukan uuid: 404 yang sama (rule 7.3) |
| Audit | tidak ada (baca saja) |
| Bus disediakan | tidak ada (handler bus terpisah di `listspecsbus`) |
| Bus dikonsumsi | `project.management.check-access` (fail-closed, lewat `base/access`) |
| AC | AC7 (non-anggota 404, isolasi antar project) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/project/internal/apispec/listspecs/`
- `backend/internal/domain/project/internal/apispec/base.go` (tepat satu baris: `listspecs.Init(d)`)
- `backend/internal/domain/project/internal/apispec/apispec_integration_test.go` (hanya menambah tes bagian daftar, bila file sudah ada; bila belum, cukup tes di folder feature)
- `api/modules/project/apispec.yml`, `api/openapi.yml` (hanya bila BE-0 menemukan selisih)
- `frontend/src/domain/project/apispec/listspecs/` (kontrak/service; komposisi view di `frontend-plan.md`)
- `frontend/src/shared/i18n/{id,en}.json` (kunci `apispec.list.*` bila belum ada)

## Konteks yang dimuat

- Overview T06: 3.2 (struktur feature), 3.3 (konvensi handler/actor), 3.4 (urutan akses), 3.6 (urutan), 4.1 (`SpecSummary`), 4.4 (`ErrProjectNotFound`), 4.6 (`SpecDTO`, `ToSummaryDTO`), 6.1 (`store.ListSpecs`), 6.2 (`access.Checker`), 9 (migrasi), 10 (`APISPEC_MAX_SPECS`), 15 (body).
- Infra T00 di `main`: `app.Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` (`internal/infra/app`), `respond.WriteJSON/WriteError` (`internal/infra/http/respond`), `middleware.RequireAuth()` (`internal/infra/middleware`), `errs`, `reqctx` (`lib/common`).
- Prasyarat dari foundation F (HARUS sudah ada; bila belum, berhenti dan laporkan, jangan membuatnya di folder feature):
  - Migrasi `api_specs` dan `api_spec_endpoints` (`migrations/project/<VERSION>_create_api_specs.sql`).
  - `Config.APISpecMaxSpecs` (env `APISPEC_MAX_SPECS`, default 100).
  - `base`: `SpecSummary`, `SpecDTO`, `ToSummaryDTO`, `Actor`, `ActorFrom`, `ErrProjectNotFound`.
  - `base/store`: `New`, `Store.ListSpecs`. `base/access`: `Checker`, `New`.
  - OpenAPI `GET /api/projects/{projectID}/api-specs` (operationId `listApiSpecs`) dan schema `ApiSpec`, `ApiSpecList` di `apispec.yml`.

## Summary Plan

`listspecs` melayani `GET /api/projects/{projectID}/api-specs`. Usecase membangun aktor, memastikan pemanggil anggota project (atau super_admin) lewat `access.Checker.RequireMember`, meminta seluruh spec project ke store (`ORDER BY name`, `endpoint_count` lewat `LEFT JOIN` + `COUNT`, tanpa kolom `content`), lalu membentuk `{items, total, limit}` dengan `limit = APISPEC_MAX_SPECS`.

Keputusan di plan ini:

1. Tanpa migrasi, tanpa indeks baru. `api_specs_project_name_key (project_id, name)` melayani filter `project_id` dan urutan `name`; `api_spec_endpoints_pkey (spec_id, ...)` melayani agregat count. Maks 100 baris per project (batas `APISPEC_MAX_SPECS`), jadi tanpa paginasi (overview 3.6).
2. Tanpa `repository.go`: SQL sudah ada di `base/store` (`ListSpecs`, dipakai bersama `listspecsbus`). Usecase menerima interface lokal kecil (`specLister`) yang dipenuhi `*store.Store`, sesuai overview 3.2. Tidak ada transaksi (satu SELECT).
3. Urutan wajib (overview 3.4): `actor` -> `RequireMember` -> baru `ListSpecs`. Non-anggota tidak pernah memicu query ke `api_specs`.
4. `projectID` dipakai apa adanya dari `chi.URLParam`. Bukan uuid: check-access menjawab `ProjectExists=false` -> `ErrProjectNotFound` (404), store tidak dipanggil. Tidak ada validasi uuid di handler.
5. `items` tidak pernah `null`: store dan DTO mengembalikan slice kosong untuk project tanpa spec. Project kosong bukan error; respons 200 `{items: [], total: 0, limit: N}`.
6. `total = len(items)`, `limit` = konfigurasi (batas maksimum jumlah spec, bukan ukuran halaman). FE memakainya untuk menampilkan "x / limit" dan menonaktifkan upload saat penuh.
7. Respons memakai `base.SpecDTO` bersama (overview 4.6) yang merupakan superset dari kolom yang diminta tiket: `name, title, openapi_version, api_version, endpoint_count, source_path, size_bytes, updated_at, updated_by_label` plus `format, created_at, created_by_label`. Field tambahan tidak merugikan konsumen; kontrak OpenAPI tetap satu schema `ApiSpec` yang sama dengan `getspec`/`renamespec`.
8. Kolom `content` TIDAK dibaca dan TIDAK pernah muncul di respons (spec sampai 5 MB; daftar harus ringan).
9. Header `Cache-Control: no-store` (data per-user/proyek, berubah setelah upload).
10. Error infra dibungkus `fmt.Errorf("listspecs: list specs: %w", err)` -> 500 generik. Tanpa log isi data. Usecase tidak butuh `Clock` maupun `audit.Recorder`.
11. Urutan `ORDER BY s.name`: nama hanya `[a-z0-9._-]`. Agar urutan deterministik lintas collation database, store sebaiknya memakai `ORDER BY s.name COLLATE "C"`. Developer memeriksa `store.ListSpecs` di F; bila belum, laporkan ke pemilik foundation (jangan ubah store dari feature ini). Tes di bawah sengaja memakai nama alfanumerik sehingga lulus di kedua collation.

## Plan

Urutan wajib. Kontrak respons (bagian Request/Response) dikunci dulu agar BE dan FE bisa paralel. Tulis tes dulu (TDD) per langkah.

### Backend

#### BE-0. Verifikasi OpenAPI (sebelum kode, rule 11.6)

Pastikan `GET /api/projects/{projectID}/api-specs` di `api/modules/project/apispec.yml` memuat persis:
- `operationId: listApiSpecs`, tag `api-specs`, security `sessionCookie`, parameter `$ref: '#/components/parameters/ProjectID'` (path, string). Tanpa parameter query, tanpa header `X-Requested-With` (GET).
- Respons: 200 `ApiSpecList {items: ApiSpec[], total: integer, limit: integer}` (ketiganya `required`); 401 `Unauthorized`, 404 `NotFound`, 500 `Internal` dari `../infra/foundation.yml`.
- `ApiSpec` memuat `name, title, openapi_version, api_version, format, source_path (string|null), size_bytes, endpoint_count, created_at, created_by_label, updated_at, updated_by_label` (semua `required`). Deskripsi operasi menyebut "anggota project atau super_admin; non-anggota 404; urut nama".

Jika ada selisih, perbaiki di `apispec.yml`, jalankan `make api-bundle`, pastikan `make api-check` lulus.

#### BE-1. `listspecs/usecase.go` (orkestrasi saja; tidak ada rule bisnis baru)

```go
package listspecs

// specLister dipenuhi *store.Store.
type specLister interface {
    ListSpecs(ctx context.Context, projectID string) ([]base.SpecSummary, error)
}

type Result struct {
    Items []base.SpecSummary // tidak pernah nil
    Limit int
}

type Usecase struct {
    guard    access.Checker
    specs    specLister
    maxSpecs int
}

func NewUsecase(guard access.Checker, specs specLister, maxSpecs int) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error) {
    if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
        return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
    }
    if err := u.guard.RequireMember(ctx, projectID); err != nil {
        return Result{}, err
    }
    items, err := u.specs.ListSpecs(ctx, projectID)
    if err != nil {
        return Result{}, fmt.Errorf("listspecs: list specs: %w", err)
    }
    if items == nil {
        items = []base.SpecSummary{}
    }
    return Result{Items: items, Limit: u.maxSpecs}, nil
}
```

Catatan: `ActorFrom` dipanggil hanya untuk menjaga konsistensi (401 bila identitas hilang); hasil actor tidak dipakai karena tidak ada audit. `RequireMember` sendiri juga menolak identitas kosong; pemeriksaan eksplisit dipertahankan agar urutan error jelas dan teruji tanpa guard.

#### BE-2. `listspecs/handler.go`

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
    if err != nil {
        respond.WriteError(w, r, err)
        return
    }
    items := make([]base.SpecDTO, 0, len(res.Items))
    for _, s := range res.Items {
        items = append(items, base.ToSummaryDTO(s))
    }
    w.Header().Set("Cache-Control", "no-store")
    respond.WriteJSON(w, http.StatusOK, listResponse{Items: items, Total: len(items), Limit: res.Limit})
}

type listResponse struct {
    Items []base.SpecDTO `json:"items"`
    Total int            `json:"total"`
    Limit int            `json:"limit"`
}
```

Tanpa `params.go` (tidak ada query/body). Sesuaikan pemanggilan `respond.WriteError` dengan signature sebenarnya di `main` (baca `internal/infra/http/respond` sebelum menulis; jangan menebak).

#### BE-3. `listspecs/init.go`

```go
func Init(d app.Deps) {
    uc := NewUsecase(access.New(d.Bus), store.New(d.DB), d.Config.APISpecMaxSpecs)
    h := newHandler(uc)
    d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-specs", h.ServeHTTP)
}
```

Pola route penuh per feature (T03 D4), bukan `Route("/projects", ...)`, agar tidak bentrok dengan route T03/T04/T05. Tidak ada `BodyLimit` (GET). Tambahkan satu baris `listspecs.Init(d)` ke `apispec/base.go` (jangan menyentuh baris feature lain).

#### BE-4. Verifikasi

`go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/listspecs/...`, tes integrasi (tag `integration`), archtest `TestT00_AC4_RepoIsClean` (feature tidak meng-import feature lain, hanya `base/*`), `make api-check`.

### Frontend (kontrak; rincian view/komponen di `frontend-plan.md`)

#### FE-1. Service dan skema
- `frontend/src/domain/project/apispec/listspecs/services/listSpecs.ts`: `listSpecs(projectId): Promise<SpecList>` memanggil `GET /api/projects/${projectId}/api-specs` lewat `shared/services/http.ts`; respons divalidasi Zod: `{items: ApiSpec[], total: number, limit: number}` dengan `source_path` nullable, waktu string RFC3339.
- `types.ts`: tipe `ApiSpec`, `SpecList`. `index.ts` mengekspos `listSpecs`, tipe, dan komponen daftar.

#### FE-2. Tampilan daftar (tab "API Specs")
- Kartu/tabel per spec: nama (tautan ke detail `/projects/{id}/api-specs/{name}`), judul, versi OpenAPI, versi API, jumlah endpoint, diperbarui (waktu relatif + `updated_by_label`), `source_path` bila ada.
- Penghitung "`total` / `limit` spec"; tombol upload nonaktif bila `total >= limit` (aksi upload sendiri di usecase `uploadspecs`).
- State: loading, error (tombol coba lagi), kosong ("Belum ada spec. Unggah file OpenAPI pertama."), 404 -> halaman not-found shell (bukan pesan khusus yang membocorkan keberadaan project).
- i18n `id/en`: `apispec.list.title`, `apispec.list.empty`, `apispec.list.count`, `apispec.list.error`. Tanpa string literal di komponen (rule 12.8).
- Tipe data pada tampilan memakai teks biasa (judul spec tidak tepercaya; render sebagai teks, bukan HTML).

## Migration

Tidak ada. Tabel `api_specs` dan `api_spec_endpoints` dibuat oleh foundation F (`backend/migrations/project/<VERSION>_create_api_specs.sql`). Feature ini hanya membaca. EXPLAIN query `ListSpecs` pada project dengan 100 spec x 500 endpoint dicatat di PR (rule 8.8): harus index scan pada `api_specs_project_name_key` dan aggregate lewat `api_spec_endpoints_pkey`, tanpa seq scan pada `api_spec_endpoints` untuk project lain.

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/apispec/listspecs/init.go`
- `backend/internal/domain/project/internal/apispec/listspecs/handler.go`
- `backend/internal/domain/project/internal/apispec/listspecs/usecase.go`
- `backend/internal/domain/project/internal/apispec/listspecs/usecase_test.go`
- `backend/internal/domain/project/internal/apispec/listspecs/handler_test.go`
- `backend/internal/domain/project/internal/apispec/listspecs/listspecs_integration_test.go` (tag `integration`)

Backend (ubah):
- `backend/internal/domain/project/internal/apispec/base.go`: tambah `listspecs.Init(d)`.

API (ubah hanya bila BE-0 menemukan selisih):
- `api/modules/project/apispec.yml`, `api/openapi.yml` (generated).

Frontend (baru, kontrak):
- `frontend/src/domain/project/apispec/listspecs/{services/listSpecs.ts,types.ts,index.ts}` + tes Vitest untuk service/skema.
- `frontend/src/shared/i18n/{id,en}.json`: kunci `apispec.list.*`.

Tidak diubah: `base/*` (termasuk `store`), feature lain, `infra/**`, `lib/**`, migrasi.

## Test Scenario

Penamaan: `TestT06_AC7_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `usecase_test.go` (fake `access.Checker` dan `specLister`, tanpa DB)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestExecute_ReturnsItemsInStoreOrder` | store mengembalikan `auth, notification, payment` | `Items` persis urutan itu; `Limit` = nilai konfigurasi |
| `TestExecute_EmptyProjectReturnsEmptySliceNotNil` | store mengembalikan `nil` | `Items != nil`, `len == 0`, tanpa error |
| `TestExecute_GuardCalledBeforeStore` | guard menolak | store tidak dipanggil (hitung panggilan = 0); error = `base.ErrProjectNotFound` |
| `TestT06_AC7_Execute_NonMemberGets404` | guard mengembalikan `ErrProjectNotFound` | `errors.Is`/`Code == NOT_FOUND`, pesan "project tidak ditemukan" |
| `TestExecute_GuardInfraErrorPropagates` | guard error biasa (fail-closed) | error dikembalikan, bukan nil, store tidak dipanggil |
| `TestExecute_NoIdentity` | ctx tanpa identitas | `CodeUnauthorized`; guard dan store tidak dipanggil |
| `TestExecute_StoreErrorWrapped` | store error | error membungkus (`errors.Is` ke error asli), prefix `listspecs: list specs` |
| `TestExecute_PassesProjectIDVerbatim` | `projectID` = "bukan-uuid" | guard dan store menerima string yang sama (tanpa trim/normalisasi) |

### Unit `handler_test.go` (`httptest` + `chi`, usecase dengan fake)

- 200: body `{"items":[...],"total":N,"limit":L}`; `Content-Type: application/json`; `Cache-Control: no-store`; urutan item dipertahankan.
- Project kosong: body memuat literal `"items":[]` (bukan `null`) — periksa string mentah, bukan hanya hasil decode.
- Body TIDAK memuat kunci `content` maupun `content_hash` (decode ke `map[string]any` per item dan periksa kunci yang ada = persis 12 kunci `SpecDTO`).
- 404: guard `ErrProjectNotFound` -> status 404, body `{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan"},"trace_id":...}`.
- 500: store error -> 500 `INTERNAL`, pesan generik, tanpa detail error asli.
- `source_path` null dirender sebagai `null`; waktu UTC RFC3339.
- Param `projectID` diambil dari URL chi (uji lewat router nyata, bukan mengisi context manual).

### Integration `listspecs_integration_test.go` (tag `integration`, `db.TestDB(t)`, bus nyata dengan handler palsu `project.management.check-access`)

Setup: terapkan migrasi T03 + T06; buat 2 project (A, B) dan user (anggota A, anggota B, super_admin, user luar); handler palsu check-access: peta `(project, user)` -> level, `ProjectExists=false` untuk id tak dikenal atau bukan uuid, `LevelAdmin` untuk super_admin; seed spec langsung lewat SQL/`store` (feature ini tidak bergantung pada `uploadspecs`).

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT06_AC7_List_OrderedByName` | seed `payment`, `auth`, `notification` (urutan sisip acak) | 200; item urut `auth, notification, payment` |
| `TestList_EndpointCountViaJoin` | `auth`: 5 endpoint, `payment`: 0 endpoint | `endpoint_count` 5 dan 0 (spec tanpa endpoint tetap muncul, bukan hilang karena JOIN) |
| `TestList_EmptyProject` | project A tanpa spec | 200, `items == []`, `total == 0`, `limit == APISPEC_MAX_SPECS` |
| `TestT06_AC7_List_NonMemberGets404` | user luar memanggil project A yang punya spec | 404 `NOT_FOUND`; body identik dengan kasus project tak ada (tidak membocorkan keberadaan) |
| `TestT06_AC7_List_NonUUIDProjectID` | `/projects/bukan-uuid/api-specs` | 404 `NOT_FOUND`, tanpa 500 (tidak ada error cast SQL) |
| `TestT06_AC7_List_UnknownProject404` | uuid valid yang tidak ada | 404 |
| `TestT06_AC7_List_IsolationBetweenProjects` | A punya `auth`, B punya `billing` | anggota A hanya melihat `auth`; anggota B hanya `billing`; nama sama di dua project muncul masing-masing dengan count milik project-nya |
| `TestList_SuperAdminNonMember` | super_admin (bukan anggota) | 200 dengan daftar lengkap |
| `TestList_NoContentColumnRead` | spec 5 MB seed | 200 cepat; respons < 4 KB per item; tidak ada `content` |
| `TestList_Unauthenticated` | tanpa sesi | 401 dari `RequireAuth` (guard tidak dipanggil) |
| `TestList_CheckAccessNotRegistered` | registry tanpa handler check-access | 500 (fail-closed), bukan 200 |
| `TestList_NoAuditEmitted` | panggil sukses | handler palsu `core.audit.record` tidak dipanggil |
| `TestList_AfterProjectDeleteCascades` | hapus project A | `store.ListSpecs` kosong (cascade); sekadar regresi cascade, bukan kontrak endpoint |

Tes `TestRoutesRegistered` (W, bukan di feature ini) harus memuat pasangan `GET /projects/{projectID}/api-specs`.

## E2E Scenarios

Playwright, butuh T03 merge; dijalankan melalui `frontend/e2e/apispec-list.spec.ts` (atau digabung ke spec E2E apispec bila sudah ada).

Positif:
1. Developer anggota membuka tab "API Specs" project yang punya `payment`, `auth`, `notification` (seed lewat API): tiga kartu muncul urut `auth, notification, payment`, masing-masing menampilkan judul, versi OpenAPI, versi API, jumlah endpoint, dan waktu pembaruan; penghitung "3 / 100".
2. Project tanpa spec: tampil keadaan kosong dengan ajakan unggah; tombol upload aktif.
3. Klik kartu `payment` membuka halaman detail `/projects/{id}/api-specs/payment` (navigasi saja; isi detail diuji di `getspec`).
4. Super Admin (bukan anggota) membuka tab project mana pun dan melihat daftar.
5. Setelah upload spec baru (via UI `uploadspecs`), daftar menyegarkan dan menampilkan spec baru pada posisi urut nama yang benar.

Negatif:
1. Developer bukan anggota membuka URL tab API Specs project lain: halaman 404 shell (tidak ada nama project atau data spec yang bocor).
2. `projectID` tidak valid (`/projects/abc/api-specs`): halaman 404, tanpa error mentah atau layar putih.
3. Sesi kedaluwarsa (401) saat memuat: diarahkan ke login dan kembali ke halaman ini setelah masuk.
4. Server error (500, di-stub): state error dengan tombol "coba lagi"; setelah pulih, daftar tampil.
5. Judul spec berisi `<script>alert(1)</script>`: tampil sebagai teks literal, tidak ada dialog (selaras AC8).
6. Daftar penuh (`total == limit`, seed via API dengan limit kecil di env uji): tombol upload nonaktif dengan penjelasan.

## Request/Response body

Request: `GET /api/projects/0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10/api-specs`, cookie sesi. Tanpa body, tanpa query, tanpa header CSRF (GET).

### 200 OK

Header: `Content-Type: application/json`, `Cache-Control: no-store`.

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
    },
    {
      "name": "payment",
      "title": "Payment API",
      "openapi_version": "3.0.3",
      "api_version": "1.4.0",
      "format": "json",
      "source_path": null,
      "size_bytes": 40211,
      "endpoint_count": 12,
      "created_at": "2026-10-23T09:00:00Z",
      "created_by_label": "Budi",
      "updated_at": "2026-10-24T10:00:00Z",
      "updated_by_label": "Sari"
    }
  ],
  "total": 2,
  "limit": 100
}
```

### 200 OK, project tanpa spec

```json
{ "items": [], "total": 0, "limit": 100 }
```

### Error (format seragam, rule 11.1)

401 (tanpa sesi, dari `RequireAuth`):

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

404 (non-anggota, project tidak ada, atau `projectID` bukan uuid; ketiganya identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

500 (check-access gagal/tidak terdaftar atau error DB; pesan generik, detail hanya di log):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

Kode dan pesan 401/500 mengikuti `respond.WriteError`/`errs` di `main`; sesuaikan contoh di atas dengan keluaran aktual saat menulis tes handler.
