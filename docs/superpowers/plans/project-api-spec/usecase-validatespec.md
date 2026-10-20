# T06 project/apispec — Usecase `validatespec`

Mengikuti `docs/superpowers/plans/project-api-spec/overview.md` (overview menang bila bertentangan). Nomor bagian yang dirujuk (3.x, 4.x, 5.x, 6.x, 13.x, 15) adalah bagian overview itu. Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`; tidak ada knowledge doc module apispec; konvensi dari `docs/tickets/README.md`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (backend di plan ini; UI "Validasi tanpa menyimpan" di `frontend-plan.md`) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `validatespec` (gelombang A) |
| Endpoint | `/api/projects/{projectID}/api-specs/validate` |
| Method | POST (`multipart/form-data`) |
| Version | tanpa prefix versi; kontrak REST `info.version: 0.1.0` (endpoint baru) |
| Siapa | anggota project, super_admin (level `member`/`owner`/`admin` dari check-access) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Folder feature | `backend/internal/domain/project/internal/apispec/validatespec/` |
| Bus dikonsumsi | `project.management.check-access` (Call, fail-closed, lewat `base/access`) |
| Bus disediakan | tidak ada |
| Audit | tidak ada (tidak mengubah state) |
| DB | tidak ada (tanpa `repository.go`, tanpa `base/store`, tanpa `base/audit`) |
| AC | 2 (ditolak dengan alasan tepat, tanpa request keluar), 3 (billion laughs murah), 6 (3.0 dan 3.1) |

## 2. Summary Plan

`validatespec` memeriksa satu atau banyak file spec seperti `uploadspecs` memeriksanya, tetapi tidak menyimpan apa pun. Gunanya: pengguna melihat apakah file akan diterima (dan alasan penolakan yang bisa disalin) sebelum menimpa spec yang ada, dan ia menjadi pintu masuk uji logika validasi `base/openapi`.

Satu usecase, satu alur (tidak dipecah):

1. Handler membaca multipart dengan `upload.Read` (sama persis dengan upload; batas `APISPEC_MAX_FILE_BYTES`, `MAX_UPLOAD_FILES`; body dibatasi `BodyLimit(MAX_UPLOAD_BYTES)`).
2. Usecase: `guard.RequireMember(projectID)` (404 bila bukan anggota / project tak ada) lalu, per file berurutan: `FormatFromExt` -> ukuran/jumlah -> `NormalizeSourcePath` -> `DeriveName`/`ValidateName` -> nama ganda -> `StripBOM` -> `openapi.Parse` -> `openapi.ExtractEndpoints`.
3. Respons 200 `{results[], summary{valid, skipped, rejected}}`. Status per file: `valid | skipped | rejected`.

Keputusan khusus endpoint ini:
- `name_exists` TIDAK ada di respons (butuh SQL; dibuang dari kontrak endpoint ini). Pemanggil yang ingin tahu apakah nama sudah dipakai membandingkan dengan `GET /api-specs` (UI sudah memegang daftarnya).
- Pemeriksaan batas `APISPEC_MAX_SPECS` TIDAK dilakukan (butuh `CountSpecs`); `SPEC_LIMIT_REACHED` hanya muncul di upload. Ini dicatat di deskripsi OpenAPI operasi `validate`.
- Tidak ada tulis DB, tidak ada audit, tidak ada panggilan bus selain check-access. Parser tidak memakai jaringan maupun filesystem (3.7).
- `name` (field teks opsional, hanya bila tepat satu file) diperlakukan sama dengan upload: nama eksplisit tidak valid -> 400 `INVALID_NAME` untuk seluruh request (parameter request, bukan hasil per file). Nama turunan dari nama file yang tidak valid -> `rejected` per file.
- Urutan: `RequireMember` dijalankan SEBELUM pemrosesan file (agar non-anggota tidak mendapat hasil validasi). Pembacaan body multipart terjadi di handler sebelum usecase (sama seperti upload); konsekuensinya, non-anggota yang mengirim multipart rusak mendapat 400/413 alih-alih 404. Ini diterima (tidak membocorkan keberadaan project; body dibatasi `BodyLimit`). Tes non-anggota memakai multipart valid.

Cakupan unit test `base/openapi`: karena endpoint ini adalah pintu masuk logika validasi, bagian 8.2 merangkum tabel testdata utama dan pemetaan ke kode issue. Tes itu sendiri ditulis di langkah foundation F7 (overview 13.2); plan ini menambahkan tes tingkat usecase/HTTP yang memakai testdata yang sama.

## 3. Plan (langkah berurutan)

Prasyarat (foundation F, bukan bagian usecase ini; bila ada yang belum ada, buat persis seperti overview): `base/` (`FormatFromExt`, `DeriveName`, `ValidateName`, `NormalizeSourcePath`, `ReasonMessage`, `FileResult`, `Issue`, `FileStatus`, `ToFileResultDTO`, `ValidateSummaryDTO`, error sentinel), `base/openapi` (`Parse`, `StripBOM`, `ExtractEndpoints`, `DefaultLimits`), `base/upload` (`Read`, `File`, `Form`), `base/access` (`Checker`, `New`), config `APISpecMaxFileBytes` dan `MaxUploadFiles`, `api/modules/project/apispec.yml` berisi path `validate`, tabel `projects` (T03) hanya untuk tes integrasi.

1. **Verifikasi prasyarat** (tanpa mengubahnya): signature di atas ada; pada `base/upload.Read` field `name` yang kosong atau hanya whitespace diperlakukan sebagai tidak ada (`Form.Name == nil`). Bila belum, perbaiki di `base/upload` (milik foundation) dan tambahkan kasus tesnya di sana, jangan di feature ini.
2. **`validatespec/params.go`** (package `validatespec`):
   ```go
   type Params struct {
       ProjectID string
       Name      *string       // field "name" (sudah di-trim; nil bila kosong)
       Files     []upload.File // urutan part
   }
   ```
   Tanpa tag JSON (bukan body JSON). Tidak ada validasi di sini selain yang sudah dilakukan `upload.Read`.
3. **`validatespec/usecase.go`**:
   ```go
   type Usecase struct {
       guard  access.Checker
       limits base.UploadLimits   // hanya MaxFileBytes dan MaxFiles dipakai
       parse  func([]byte, openapi.Format, openapi.Limits) (openapi.Document, []openapi.Issue)  // default openapi.Parse; seam tes tidak diperlukan, hapus bila tak dipakai
   }
   func NewUsecase(guard access.Checker, limits base.UploadLimits) *Usecase

   type Result struct {
       Results []base.FileResult
       Summary base.ValidateSummaryDTO   // dihitung lokal (lihat langkah 3.4)
   }
   func (u *Usecase) Execute(ctx context.Context, p Params) (Result, error)
   ```
   (Jangan menambah field `parse` bila tidak dibutuhkan; tulis langsung `openapi.Parse`. Usecase menerima interface hanya untuk `access.Checker`.)

   Alur `Execute`, berurutan:
   1. `if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil { return Result{}, err }` (tabel 3.4: tanpa identitas 401; bukan anggota / project tak ada / id bukan uuid 404; error bus 500 fail-closed).
   2. Bila `p.Name != nil`: `base.ValidateName(*p.Name)`; gagal -> kembalikan `base.ErrInvalidName` apa adanya (400, `details.reason=INVALID_NAME`, `fields.name`).
   3. `seen := map[string]struct{}{}`; untuk tiap `f` di `p.Files` (urut), `ctx.Err() != nil` -> kembalikan `ctx.Err()` terbungkus; lalu `res := u.checkFile(f, p.Name, seen)`; tambahkan ke hasil. `checkFile` (privat, tanpa I/O, deterministik) menjalankan, berhenti pada langkah pertama yang menolak/melewati:
      1. `r := base.FileResult{File: f.FileName, Issues: []base.Issue{}}`. `sp, err := base.NormalizeSourcePath(f.RawPath)`; sukses -> `r.SourcePath = &sp` (best effort, juga untuk file yang di-skip); gagal -> `sp` nil, ingat `pathErr`.
      2. `_, ok := base.FormatFromExt(f.FileName)`; `!ok` -> `skipped`, `ReasonCode=NOT_SPEC_FILE`, `Reason=base.ReasonMessage(NOT_SPEC_FILE)`. Ekstensi dicek lebih dulu agar file non-spec (gambar, README) tidak pernah menjadi `rejected`.
      3. `f.Rejected == base.ReasonTooManyFiles` -> `rejected`, `TOO_MANY_FILES`.
      4. `f.TooLarge` -> `rejected`, `FILE_TOO_LARGE`, `Reason=ReasonMessage(code, u.limits.MaxFileBytes)`.
      5. `pathErr != nil` -> `rejected`, `INVALID_PATH`.
      6. Nama: `p.Name != nil` -> `name = *p.Name` (sudah divalidasi di 3.2); selain itu `name, err := base.DeriveName(f.FileName)`; err -> `rejected`, `INVALID_NAME` (Name tetap `""`). Sukses -> `r.Name = name`.
      7. Nama ganda: `name` sudah ada di `seen` -> `rejected`, `DUPLICATE_NAME_IN_UPLOAD`; selain itu masukkan ke `seen` (keputusan sama dengan upload: nama didaftarkan begitu ditentukan, sebelum parse; file pertama menang walau nanti ditolak karena isi).
      8. `content := openapi.StripBOM(f.Content)`; `format, _ := base.FormatFromExt(f.FileName)` -> `openapi.Format(format)`.
      9. `doc, issues := openapi.Parse(content, fmtVal, openapi.DefaultLimits)`. `len(issues) > 0` -> `rejected`, `ReasonCode=INVALID_SPEC`, `Reason=issues[0].Message`, `Issues` = semua issue dipetakan ke `base.Issue` (maks 20), field metadata (`OpenAPIVersion` dst.) dibiarkan kosong.
      10. `eps := openapi.ExtractEndpoints(doc)`. Sukses -> `Status=valid`, `OpenAPIVersion=doc.OpenAPIVersion`, `Title=doc.Title`, `APIVersion=doc.APIVersion`, `EndpointCount=len(eps)`, `ReasonCode/Reason` kosong.
   4. Ringkasan: hitung `Valid/Skipped/Rejected` dengan loop kecil lokal (`base.Summarize` tidak punya `valid`; tidak menambah rule baru ke `base`).
   5. Kembalikan `Result`. Tidak ada tulis, audit, atau panggilan bus lain.

   Catatan tipe: `f.Content` bisa `nil` hanya bila `TooLarge`/`Rejected` (sudah ditangani sebelum parse). `Parse` murni CPU; seluruh file diproses berurutan di goroutine request (tanpa goroutine tambahan, hasil deterministik).
4. **`validatespec/handler.go`**:
   ```go
   type Handler struct {
       uc           *Usecase
       maxFileBytes int64
       maxFiles     int
   }
   func newHandler(uc *Usecase, maxFileBytes int64, maxFiles int) *Handler

   type response struct {
       Results []base.FileResultDTO    `json:"results"`
       Summary base.ValidateSummaryDTO `json:"summary"`
   }

   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
   ```
   Alur `ServeHTTP`:
   1. `form, err := upload.Read(r, h.maxFileBytes, h.maxFiles)`; err -> `respond.WriteError(w, r, err)` (400 `NOT_MULTIPART|NO_FILES|NAME_WITH_MULTIPLE_FILES`, 413 `PAYLOAD_TOO_LARGE`, 400 multipart tidak valid). Gunakan helper yang dipakai handler T01/T03 di `infra/http/respond` (periksa signature `WriteError` sebelum menulis).
   2. `p := Params{ProjectID: chi.URLParam(r, "projectID"), Name: form.Name, Files: form.Files}` (apa adanya, tanpa validasi uuid; 3.3).
   3. `res, err := h.uc.Execute(r.Context(), p)`; err -> `respond.WriteError`.
   4. Petakan `res.Results` lewat `base.ToFileResultDTO` ke `[]FileResultDTO` (tidak pernah nil), `respond.WriteJSON(w, r, http.StatusOK, response{...})`.
   Handler tidak berisi rule; tidak me-log isi file. `Content-Type: application/json` standar `WriteJSON`.
5. **`validatespec/init.go`**:
   ```go
   func Init(d app.Deps) {
       limits := base.UploadLimits{MaxFileBytes: d.Config.APISpecMaxFileBytes, MaxFiles: d.Config.MaxUploadFiles}
       uc := NewUsecase(access.New(d.Bus), limits)
       h := newHandler(uc, limits.MaxFileBytes, limits.MaxFiles)
       d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
           Post("/projects/{projectID}/api-specs/validate", h.ServeHTTP)
   }
   ```
   `Init` TIDAK menyentuh `d.DB` (tes menjamin lewat `Deps{DB: nil}`).
6. **Wiring**: tambah satu baris `validatespec.Init(d)` di `apispec/base.go` (`Register`). Jangan mengubah `Init` feature lain.
7. **OpenAPI** (diperiksa, bukan ditulis ulang; aslinya langkah F11): operasi `POST /projects/{projectID}/api-specs/validate` ada di `api/modules/project/apispec.yml` dengan parameter `XRequestedWith`, `ProjectID`; request multipart (`files` array binary, `name` string); respons 200 `ApiSpecValidateResult` (`results[]: ApiSpecFileResult`, `summary{valid,skipped,rejected}`), 400/401/403/404/413 dari `foundation.yml`; deskripsi menyebut: "tidak menyimpan; tidak memeriksa nama bentrok dan batas jumlah spec". `status` enum untuk validate: `valid|skipped|rejected` (enum bersama `ApiSpecFileResult.status` memuat semuanya). `make api-check` lulus.
8. **Tes** (bagian 8) ditulis bersama kode.
9. **Verifikasi akhir**: `go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/...`, `go test -tags integration` pada tes feature ini, archtest `TestT00_AC4_RepoIsClean`.

Kontrak bus: tidak ada yang baru. Konsumsi: `project.management.check-access` (`Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists, Level}`) lewat `access.Guard`.

## 4. Migration

Tidak ada. Feature ini tidak menyentuh tabel apa pun.

## 5. Changes (per file/folder)

Baru, semuanya di `backend/internal/domain/project/internal/apispec/validatespec/`:
- `init.go`: perakitan dan route (langkah 5).
- `handler.go`: multipart -> `Params` -> `Execute` -> DTO.
- `params.go`: `Params`.
- `usecase.go`: `Usecase`, `Execute`, `checkFile`, hitung ringkasan.
- `usecase_test.go`, `handler_test.go`, `init_test.go`, `validatespec_integration_test.go` (tag `integration`).

Diubah:
- `backend/internal/domain/project/internal/apispec/base.go`: tambah `validatespec.Init(d)`.

Diperiksa tanpa perubahan (milik foundation): `base/*`, `base/openapi/*` (+ `testdata/`), `base/upload`, `base/access`, `api/modules/project/apispec.yml`, `docs/configuration.md`.

Tidak ada: `repository.go`, migrasi, konstanta bus, kontrak bus, perubahan config, perubahan `base/store`, `base/audit`.

## 6. Aturan yang ditegakkan

- Usecase hanya orkestrasi; semua rule (nama, path, format, alasan, parsing) ada di `base` dan `base/openapi`.
- Tanpa SQL: tidak mengimpor `db`, `sqlx`, `base/store`, `base/audit`, `infra/bus` (bus hanya lewat `access`). Ditegakkan tes (bagian 8.1).
- Tidak mengimpor feature lain.
- Isi file tidak pernah di-log; hanya `project_id` dan jumlah file pada log debug bila ada.
- Konkurensi: nol goroutine; memori terbatas oleh `MAX_UPLOAD_BYTES` (sudah dibaca penuh oleh `upload.Read`) ditambah ekspansi parser yang dibatasi `DefaultLimits` (64/100/500000).

## 7. Request/Response body

Header wajib: `X-Requested-With: grimoire` (CSRF T00), cookie sesi.

### POST /api/projects/{projectID}/api-specs/validate

Request (`multipart/form-data`, field `files` berulang; `name` opsional hanya bila tepat satu file):

```
--b
Content-Disposition: form-data; name="files"; filename="specs/auth.yaml"
Content-Type: application/yaml

openapi: 3.1.0
...
--b
Content-Disposition: form-data; name="files"; filename="specs/legacy.yaml"
Content-Type: application/yaml

swagger: "2.0"
...
--b
Content-Disposition: form-data; name="files"; filename="specs/README.md"
Content-Type: text/markdown

# docs
--b--
```

Response 200:

```json
{
  "results": [
    {
      "file": "auth.yaml",
      "source_path": "specs/auth.yaml",
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
      "source_path": "specs/legacy.yaml",
      "name": "legacy",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)",
      "issues": [
        { "code": "NOT_OPENAPI_3", "message": "Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)", "pointer": "#/swagger", "line": 1 }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    },
    {
      "file": "README.md",
      "source_path": "specs/README.md",
      "name": null,
      "status": "skipped",
      "reason_code": "NOT_SPEC_FILE",
      "reason": "bukan file spec (.yaml/.yml/.json)",
      "issues": [],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "valid": 1, "skipped": 1, "rejected": 1 }
}
```

Catatan: tidak ada field `name_exists`. `reason` untuk `INVALID_SPEC` adalah `Message` issue pertama (sudah memuat lokasi), jadi bisa disalin apa adanya.

Contoh `$ref` eksternal (tanpa fetch):

```json
{
  "file": "ext.yaml", "source_path": "ext.yaml", "name": "ext", "status": "rejected",
  "reason_code": "INVALID_SPEC",
  "reason": "referensi eksternal tidak didukung dan tidak di-fetch: http://127.0.0.1:9/x.yaml (baris 12, #/paths/~1pets/get/responses/200/content/application~1json/schema)",
  "issues": [
    { "code": "EXTERNAL_REF", "message": "referensi eksternal tidak didukung dan tidak di-fetch: http://127.0.0.1:9/x.yaml (baris 12, #/paths/~1pets/get/responses/200/content/application~1json/schema)", "pointer": "#/paths/~1pets/get/responses/200/content/application~1json/schema", "line": 12 }
  ],
  "openapi_version": null, "title": null, "api_version": null, "endpoint_count": null
}
```

Contoh alasan lain per file (`reason_code` -> `reason`): `FILE_TOO_LARGE` "ukuran file melebihi batas 5242880 byte"; `INVALID_NAME` "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter"; `DUPLICATE_NAME_IN_UPLOAD` "nama duplikat dalam upload"; `INVALID_PATH` "path tidak valid"; `TOO_MANY_FILES` "melebihi batas jumlah file".

### Error (format seragam, rule 11.1)

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
{ "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "request harus multipart/form-data", "details": { "reason": "NOT_MULTIPART" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "field name hanya boleh dipakai saat mengunggah tepat satu file", "details": { "reason": "NAME_WITH_MULTIPLE_FILES" } }, "trace_id": "..." }
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter", "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } } }, "trace_id": "..." }
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran upload melebihi batas" }, "trace_id": "..." }
{ "error": { "code": "FORBIDDEN", "message": "...", "details": { "reason": "CSRF_REQUIRED" } }, "trace_id": "..." }
```

Kode `UNAUTHORIZED` dan `FORBIDDEN/CSRF_REQUIRED` datang dari middleware T00/T01 (nilai persis mengikuti `errs`/middleware yang ada; verifikasi saat menulis tes). Error check-access/bus -> 500 `INTERNAL` generik.

## 8. Test Scenario

Penamaan: `TestT06_ACn_<Deskripsi>` untuk AC; lainnya `Test<Unit>_<Kasus>`.

### 8.1 Unit usecase (`usecase_test.go`, tanpa DB; fake `access.Checker`)

Fixture dibaca dari `../base/openapi/testdata/` (satu sumber kebenaran dengan tes parser) lewat helper `loadFixture(t, name)` yang membentuk `upload.File{FileName, RawPath, Content}`.

Akses dan urutan:
- `TestValidate_NonMember_404`: checker mengembalikan `base.ErrProjectNotFound` -> `Execute` mengembalikan error itu; `openapi.Parse` tidak dijalankan (buktikan lewat file berisi billion-laughs yang tetap kembali < 10 ms dan tanpa hasil).
- `TestValidate_CheckerError_FailClosed`: checker error infra -> error dikembalikan (bukan hasil kosong).
- `TestValidate_ExplicitNameInvalid_400`: `Name="Bad Name"` -> `errors.Is(err, base.ErrInvalidName)`; tidak ada hasil.

Per file (tabel kasus; asersi `Status`, `ReasonCode`, kode issue pertama, `EndpointCount`):

| Input | Status | ReasonCode / issue |
|---|---|---|
| `petstore-3.0.yaml` | valid | `OpenAPIVersion=3.0.3`, endpoint sesuai fixture, `nullable` tidak mengganggu (AC6) |
| `petstore-3.1.json` | valid | `3.1.0`, `type: ["string","null"]` diterima (AC6) |
| `webhooks-only-3.1.yaml` | valid | `EndpointCount=0` |
| `bom.json` | valid | BOM dibuang sebelum parse |
| `merge-keys.yaml` | valid | |
| `ref-in-example.yaml` | valid | `$ref` di `example`/`x-*` tidak diperiksa |
| `README.md`, `notes.txt`, `spec.yaml.bak`, tanpa ekstensi | skipped | `NOT_SPEC_FILE`; `Name` kosong |
| `package.json` | rejected | `INVALID_SPEC`, `NOT_OPENAPI` (AC2) |
| `broken.yaml` / `broken.json` | rejected | `SYNTAX`, `Line > 0` (AC2) |
| `swagger-2.0.yaml` | rejected | `NOT_OPENAPI_3` (AC2) |
| `ref-http.yaml` | rejected | `EXTERNAL_REF` (AC2) |
| `ref-file.yaml` | rejected | `EXTERNAL_REF` |
| `ref-unresolved.yaml` | rejected | `UNRESOLVED_REF` dengan pointer dan baris |
| `dup-operationid.yaml` | rejected | `DUPLICATE_OPERATION_ID` |
| `billion-laughs.yaml`, `deep.yaml`, `alias-cycle.yaml` | rejected | `YAML_LIMIT` (AC3) |
| `invalid-utf8.yaml`, `nul.json` | rejected | `INVALID_ENCODING` |
| `multi-doc.yaml` | rejected | `MULTI_DOCUMENT` |
| `many-errors.yaml` | rejected | tepat 20 issue, urut baris; `Reason` = `issues[0].Message` |
| `File{TooLarge:true}` | rejected | `FILE_TOO_LARGE`, pesan memuat batas; `Content` nil tidak di-parse |
| `File{Rejected:"TOO_MANY_FILES"}` | rejected | `TOO_MANY_FILES` |
| `RawPath="/etc/x.yaml"` atau `a/../b.yaml` | rejected | `INVALID_PATH`, `SourcePath` nil |
| `FileName=".yaml"` | rejected | `INVALID_NAME`, `Name` kosong |

Kasus tambahan:
- `TestValidate_DuplicateNameInUpload`: `v1/payment.yaml` + `v2/payment.yaml` -> yang pertama `valid`, yang kedua `rejected DUPLICATE_NAME_IN_UPLOAD`; `payment.yaml` pertama invalid + kedua valid -> kedua tetap `DUPLICATE_NAME_IN_UPLOAD` (nama terdaftar saat ditentukan; sama dengan upload).
- `TestValidate_ExplicitName`: satu file + `Name="payment-v2"` -> `Name=payment-v2`.
- `TestValidate_SourcePathNormalized`: `a\b\c.yaml` -> `SourcePath=a/b/c.yaml`; file skipped dengan path baik tetap memuat `SourcePath`.
- `TestValidate_MixedKeepsOrderAndSummary`: 1 valid + 1 skipped + 2 rejected -> urutan hasil = urutan part; `Summary{1,1,2}`; satu file buruk tidak membatalkan file lain.
- `TestValidate_ResultsNeverNil`: `Issues` selalu slice (`[]`), `Results` tidak nil.
- `TestT06_AC3_BillionLaughsCheap`: `Execute` dengan `billion-laughs.yaml` selesai < 1 detik dan `TotalAlloc` naik < 50 MB.
- `TestValidate_ContextCancelled`: ctx dibatalkan sebelum file kedua -> error ctx, tidak panik.
- `TestValidate_NoNameExistsField`: marshal respons ke JSON dan pastikan tidak ada kunci `name_exists`.

### 8.2 Ringkasan unit test `base/openapi` (ditulis di foundation F7; dirangkum karena endpoint ini pintu masuknya)

| Area | Kasus kunci | Harapan |
|---|---|---|
| Versi | petstore 3.0 (`nullable`), petstore 3.1 (`type` array, kunci `200` -> `"200"`) | valid; endpoint terekstrak urut kanonik (`path`, lalu GET,PUT,POST,DELETE,OPTIONS,HEAD,PATCH,TRACE) |
| Klasifikasi | `package.json`; `swagger-2.0.yaml`; `openapi-4.0.yaml`; `openapi: 3.0` numerik | `NOT_OPENAPI`; `NOT_OPENAPI_3`; `UNSUPPORTED_VERSION`; `UNSUPPORTED_VERSION` |
| Sintaks | YAML rusak; JSON rusak; multi-dokumen; kunci duplikat | `SYNTAX` (baris > 0; JSON dari offset); `MULTI_DOCUMENT`; `SYNTAX` |
| Struktur | 3.0 tanpa `paths`; `info.title` bukan string; `in: path` tanpa `required: true`; responses kode salah | `INVALID_STRUCTURE` dengan pointer dan baris |
| `$ref` | `http://...`, `./common.yaml#/X`, `/etc/passwd`, `//host/x` | `EXTERNAL_REF`, nol I/O (listener lokal menghitung 0 request) |
| `$ref` internal | tak terselesaikan; di dalam `example`/`default`/`enum`/`x-*` | `UNRESOLVED_REF`; dilewati |
| operationId | duplikat di `paths` dan `webhooks` | `DUPLICATE_OPERATION_ID` menyebut dua lokasi |
| Batas YAML | billion laughs (9 x 10 alias); kedalaman 200; siklus anchor | `YAML_LIMIT`; parse < 1 detik, alokasi < 50 MB; JSON valid 5 MB < 1 detik |
| Encoding | UTF-8 tidak valid; NUL; BOM; kosong/whitespace | `INVALID_ENCODING`; `INVALID_ENCODING`; valid setelah `StripBOM`; `EMPTY` |
| Batas issue | > 20 kesalahan | tepat 20, urut baris |
| Keamanan paket | `imports_test.go` | tidak ada `net`, `net/http`, `net/url`, `os`, `io/fs`, `io/ioutil`, `path/filepath`, `os/exec`, `plugin` |
| Peta | `Map.MarshalJSON` | urutan kunci asli; escape sama dengan `encoding/json` |

### 8.3 Handler dan init (`handler_test.go`, `init_test.go`; `httptest`, fake `Usecase` lewat checker palsu)

- `TestValidateHandler_OK`: multipart 3 file (valid, rusak, `.md`) -> 200; JSON sesuai bagian 7 (`summary`, `null` untuk field kosong, `issues: []`).
- `TestValidateHandler_NotMultipart` -> 400 `NOT_MULTIPART`; tanpa part file -> 400 `NO_FILES`; `name` + 2 file -> 400 `NAME_WITH_MULTIPLE_FILES`; `name=Bad Name` + 1 file -> 400 `INVALID_NAME` dengan `fields.name`; `name=` kosong + 1 file -> diperlakukan tidak ada (nama dari file).
- `TestValidateHandler_BodyTooLarge`: body > `BodyLimit` -> 413 `PAYLOAD_TOO_LARGE`.
- `TestValidateHandler_FileTooLarge`: satu file > `APISPEC_MAX_FILE_BYTES` + satu file valid -> 200; yang pertama `rejected FILE_TOO_LARGE`, yang kedua tetap `valid` (part berikutnya tetap terbaca).
- `TestValidateHandler_NonMember404`: bus check-access palsu menjawab `ProjectExists=false`; juga `projectID` bukan uuid -> 404 `NOT_FOUND` "project tidak ditemukan" (multipart valid).
- `TestValidateHandler_NoSession401`: tanpa identitas -> 401 (via `RequireAuth`).
- `TestInit_DoesNotTouchDB`: `Init(app.Deps{DB: nil, Bus: reg, API: router, Config: cfg})` tidak panik; route `POST .../validate` terdaftar.
- `TestValidatespec_ImportsNoPersistence`: `go/parser` atas file non-test di package ini; gagal bila mengimpor `internal/infra/db`, `github.com/jmoiron/sqlx`, `base/store`, `base/audit`, `database/sql`, atau `net/http/httputil`/klien HTTP keluar (`net/http` hanya boleh untuk handler).

### 8.4 Integrasi (`validatespec_integration_test.go`, tag `integration`, `db.TestDB(t)`)

- `TestT06_AC2_ValidateRejectedNoOutbound`: dalam satu request kirim `package.json`, YAML rusak, Swagger 2.0, dan spec dengan `$ref: http://<listener lokal>/x.yaml` -> 4 `rejected` dengan kode issue tepat; listener menghitung 0 request; `store.CountSpecs(project)` tetap 0.
- `TestValidate_DoesNotPersistNorAudit`: kirim 2 spec valid -> 200 `valid`; `CountSpecs` = 0, tabel `api_spec_endpoints` kosong, handler audit palsu menerima 0 panggilan.
- `TestValidate_DoesNotOverwrite`: simpan `payment` lewat store (atau `uploadspecs` bila sudah ada), validasi `payment.yaml` berbeda -> `valid`; `FindSpec("payment")` tidak berubah (hash sama).
- `TestT06_AC7_ValidateIsolation`: anggota project A memvalidasi di project B (bukan anggota) -> 404; super_admin bukan anggota -> 200.
- `TestT06_AC6_Validate30And31`: kedua petstore dalam satu request -> dua `valid`.
- Pemeriksaan route (dimiliki W `routes_test.go`, dicatat di sini): `GET .../api-specs/validate` diarahkan ke `getspec` (spec bernama `validate`); `POST .../api-specs/validate` ke feature ini; `PUT/PATCH/DELETE .../validate` -> `PATCH`/`DELETE` ke `renamespec`/`deletespec` dengan `{name}=validate`.

## 9. E2E Scenarios (Playwright; butuh T03 merge dan UI dari `frontend-plan.md`)

Positif:
1. Anggota membuka tab "API Specs" -> "Validasi" (atau tombol "Periksa dulu" di dialog upload), memilih `auth.yaml` valid -> baris hasil `valid` dengan judul, versi OpenAPI, versi API, jumlah endpoint; tab daftar spec tidak berubah (tidak ada kartu baru) setelah menutup dialog dan memuat ulang.
2. Memilih folder berisi `auth.yaml`, `payment.yaml`, `README.md` -> ringkasan "2 valid, 1 dilewati"; tombol "Unggah" aktif hanya untuk file valid (bila UI menawarkan lanjut upload).
3. Spec 3.0 dengan `nullable` dan spec 3.1 dengan `type: [string, "null"]` dinyatakan valid.
4. Super Admin yang bukan anggota dapat memvalidasi.
5. Tombol "Salin alasan" pada baris ditolak menyalin teks `reason` yang memuat baris/pointer.

Negatif:
1. `package.json`, YAML rusak, Swagger 2.0, dan spec `$ref` URL -> 4 baris `rejected` dengan alasan terpisah dan bisa disalin (memuat baris/pointer); tidak ada permintaan keluar dari server (dipastikan di tes backend).
2. File `.txt` -> `skipped` "bukan file spec".
3. File 6 MB -> `rejected` "ukuran file melebihi batas".
4. Dua file ber-nama sama dari folder berbeda -> yang kedua `rejected` "nama duplikat dalam upload".
5. Developer bukan anggota membuka URL halaman project lain -> halaman 404; permintaan `validate` langsung ke project itu -> 404.
6. Request tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`.
7. YAML billion laughs -> `rejected` "dokumen YAML terlalu kompleks" dalam waktu wajar; halaman tetap responsif.

## 10. Risiko dan koordinasi dengan plan usecase lain

1. **Duplikasi orkestrasi per file dengan `uploadspecs`.** Urutan langkah 3.3.3 (ekstensi -> ukuran -> path -> nama -> duplikat -> BOM -> parse -> ekstrak) harus identik di `uploadspecs` agar hasil validate dan upload konsisten. Rule-nya ada di `base`/`base/openapi` (tidak diduplikasi); yang diduplikasi hanya urutan panggilan. Bila plan/implementasi `uploadspecs` sudah memindahkan urutan ini ke satu fungsi bersama di `base`, pakai fungsi itu dan hapus `checkFile`; jangan membuat package bersama baru tanpa memperbarui overview (feature dilarang saling impor).
2. Perbedaan yang disengaja dari upload: tidak ada `SPEC_LIMIT_REACHED`, `unchanged`, `created`, `updated`, tidak ada `name_exists`.
3. Non-anggota dengan multipart rusak mendapat 400/413 sebelum 404 (lihat bagian 2); tidak membuka informasi project.
4. Validasi struktural bukan validasi skema JSON penuh (overview risiko 4): `valid` berarti "akan diterima upload" (kecuali batas jumlah/nama bentrok), bukan jaminan skema bebas cacat.
