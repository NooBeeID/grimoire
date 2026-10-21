# T10 / uploadspecs — Plan Usecase (jalur REST Swagger 2.0)

Mengikuti `docs/superpowers/plans/project-api-spec-swagger2/overview.md` (selanjutnya "overview", rujukan `OV <bagian>`). Bila bertentangan, overview menang. Plan T06 lama: `docs/superpowers/plans/project-api-spec/usecase-uploadspecs.md` (tetap berlaku untuk perilaku 3.x).

Usecase ini TIDAK mengubah kode produksi Go. Seluruh perilaku 2.0 datang dari foundation `base/openapi` (OV F0-F5). Pekerjaan di sini: membalik tes penolakan, menambah tes integration, memperbarui kontrak API, fixture e2e, catatan revisi dokumen, dan laporan uji nyata. Karena itu tidak ada pemecahan usecase dan tidak ada feature folder baru.

Cakupan "uploadspecs" di tiket ini mencakup seluruh jalur REST spec: `uploadspecs` (upload), `validatespec` (validate), `getspec` (detail, dokumen 2.0 apa adanya), `downloadspec` (raw, byte identik). Keempatnya bekerja tanpa perubahan signature.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T10 |
| Scope | fullstack (plan ini sisi backend + kontrak + fixture e2e; FE di `frontend-plan.md`) |
| Domain / Module | project / api-spec (folder Go `apispec`) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Feature | `uploadspecs` (+ tes di `validatespec`, integration; `getspec` dan `downloadspec` hanya diverifikasi lewat tes integration) |
| Bergantung pada | foundation OV F0-F5 selesai dan hijau; gerbang F0 (spike) lolos |
| Bus dikonsumsi / disediakan | tidak berubah (`project.management.check-access`, `core.audit.record`); tidak ada kontrak bus baru |
| Migrasi | tidak ada |

| Endpoint | Method | Version | Perubahan perilaku |
|---|---|---|---|
| `/api/projects/{projectID}/api-specs/upload` | POST (multipart, header `X-Requested-With: grimoire`) | tanpa prefix versi | `swagger: "2.0"` diterima: `created/updated/unchanged`, `openapi_version: "2.0"` |
| `/api/projects/{projectID}/api-specs/validate` | POST (multipart) | tanpa prefix versi | 2.0 valid -> `valid` |
| `/api/projects/{projectID}/api-specs/{name}` | GET | tanpa prefix versi | `document` berisi dokumen 2.0 apa adanya (kunci `swagger`) |
| `/api/projects/{projectID}/api-specs/{name}/raw` | GET | tanpa prefix versi | byte identik dengan unggahan (tanpa perubahan kode) |

Acceptance criteria tiket yang dipegang: AC1, AC2, AC3, AC6 (bagian REST), AC7, AC8.

---

## 2. Summary Plan

1. Pastikan foundation hijau dan gerbang F0 tahap a+b sudah dijalankan (OV F0). Jika batas D8 terlampaui, usecase ini BERHENTI (lihat langkah 1).
2. Balik tes penolakan 2.0 di tiga tempat (`uploadspecs/usecase_test.go`, `validatespec/usecase_test.go`, `apispec_integration_test.go`) agar mengunci perilaku baru.
3. Tambah tes integration baru: created/unchanged/updated/rejected 2.0, `openapi_version "2.0"`, urutan endpoint setara 3.x, unduh byte sama, issue berpointer + baris, `EXTERNAL_REF` tanpa request keluar, GET detail berisi `swagger`.
4. Perbarui kontrak `api/modules/project/api-spec.yml` (contoh dan deskripsi saja; bentuk skema tidak berubah), `make api-bundle`, commit `api/openapi.yml`, `make api-check`.
5. Jadikan fixture e2e `swagger-2.0.yaml` valid (petstore 2.0).
6. Catatan revisi di `docs/tickets/T06-project-api-spec.md` dan `docs/superpowers/plans/project-api-spec/overview.md`.
7. Uji nyata `hr-platform/api/openapi.yml` dan laporkan apa adanya, tanpa mengubah batas parser.

Keputusan khusus usecase (turunan OV 3, tidak mengubahnya):

| # | Keputusan | Alasan |
|---|---|---|
| U1 | Tidak ada perubahan di `usecase.go`, `handler.go`, `repository.go`, `init.go` pada semua feature. Bila tes yang dibalik gagal karena kode produksi, perbaikannya milik foundation (F1-F4), bukan patch di usecase. | Signature usecase tidak berubah (ticket); mencegah logika versi bocor ke usecase. |
| U2 | `NOT_OPENAPI_3` tetap di `issue.go`, kontrak, i18n (OV D4), tetapi tes `NotOpenAPI3NeverEmitted` di foundation menjamin tidak muncul lagi. Usecase ini hanya mengganti contoh kontrak. | Kompatibilitas klien yang mungkin masih mengenal kode itu. |
| U3 | Tes integration memakai fixture dari `base/openapi/testdata/` lewat `readTestdata` yang sudah ada; tidak menduplikasi fixture. | Satu sumber fixture (OV F5). |
| U4 | Tidak ada pembacaan jaringan di tes selain listener `httptest` lokal yang HARUS menghitung 0 request. | OV D13, AC3. |

---

## 3. Plan (langkah berurutan)

Semua path relatif ke repo root kecuali disebut. `APIDIR` = `backend/internal/domain/project/internal/apispec`.

### Langkah 1 — Prasyarat dan gerbang

1. Cek foundation: `cd backend && go test ./internal/domain/project/internal/apispec/base/openapi/` hijau, fixture OV F5 ada (`swagger-2.0.yaml` berisi petstore 2.0 valid, `swagger-2.0-petstore.yaml/.json`, `swagger-2.0-unresolved-ref.yaml`, `swagger-2.0-dup-opid.yaml`, `swagger-2.0-bad-params.yaml`, `swagger-2.0-ref-http.yaml`, `swagger-2.0-ref-file.yaml`, `swagger-1.2.yaml`, `swagger-null.yaml`).
2. Gerbang F0: bila hasil spike menunjukkan batas D8 terlampaui atau parse > 2 detik, BERHENTI. Laporkan angka ukur + usulan angka baru + alasan ke pemilik dan tunggu persetujuan eksplisit. Jangan mengubah `limits.go`, `yamlguard.go`, atau `APISPEC_MAX_FILE_BYTES`. Langkah 2-6 (tes, kontrak, fixture, catatan) tetap boleh dikerjakan karena tidak bergantung pada angka batas; langkah 7 menunggu keputusan.

### Langkah 2 — Balik `uploadspecs/usecase_test.go`

File: `APIDIR/uploadspecs/usecase_test.go`.

- Di tabel `TestUploadSpecs_InvalidSpecs` HAPUS baris `{"swagger2", "s.yaml", "swagger: '2.0'...", "NOT_OPENAPI_3"}`. Gantikan dengan dua baris penolakan 2.0 yang tetap relevan di tabel ini (pola tabel: `name, path, content, code`):
  - `{"swagger 1.2", "s.yaml", "swagger: '1.2'\ninfo: {title: a, version: '1'}\npaths: {}\n", "UNSUPPORTED_VERSION"}`
  - `{"swagger2 unresolved", "s2.yaml", <swagger 2.0 dengan $ref '#/definitions/Hilang'>, "UNRESOLVED_REF"}`
  Tabel ini sudah memeriksa `r.Reason == r.Issues[0].Message`, `Name != ""`, `EndpointCount == 0`, tanpa tx dan tanpa audit; semuanya tetap berlaku.
- Tambah `TestUploadSpecs_Swagger2Created`: unggah petstore 2.0 inline (YAML) lewat `newEnv(10)` + `run(...)` seperti tes sukses 3.x yang ada. Ekspektasi: `Status == created`, `OpenAPIVersion == "2.0"`, `Title`, `APIVersion` dari `info`, `EndpointCount == 4`; repo menerima satu `InsertSpec` dengan `DocMeta.OpenAPIVersion == "2.0"` dan `ReplaceEndpoints` berisi 4 endpoint; audit `uploaded` satu kali. Tabel Swagger 2.0 tidak mengubah jalur `unchanged/updated` karena hash dihitung dari byte; tambah subcase: unggah ulang byte sama -> `unchanged`, tanpa audit baru.
- Tidak ada tes 3.x yang diubah.

### Langkah 3 — Balik `validatespec/usecase_test.go`

File: `APIDIR/validatespec/usecase_test.go`.

- Di `TestValidate_PerFileCases`: baris `{"swagger-2.0.yaml", StatusRejected, "INVALID_SPEC", "NOT_OPENAPI_3"}` diubah menjadi `{"swagger-2.0.yaml", base.StatusValid, "", ""}` (fixture sekarang petstore 2.0 valid). Tambah baris:
  - `{"swagger-2.0-petstore.yaml", StatusValid, "", ""}`, `{"swagger-2.0-petstore.json", StatusValid, "", ""}`, `{"swagger-2.0-numeric.yaml", StatusValid, "", ""}`
  - `{"swagger-1.2.yaml", StatusRejected, "INVALID_SPEC", "UNSUPPORTED_VERSION"}`, `{"swagger-null.yaml", StatusRejected, "INVALID_SPEC", "NOT_OPENAPI"}`
  - `{"swagger-2.0-unresolved-ref.yaml", ..., "UNRESOLVED_REF"}`, `{"swagger-2.0-dup-opid.yaml", ..., "DUPLICATE_OPERATION_ID"}`, `{"swagger-2.0-bad-params.yaml", ..., "INVALID_STRUCTURE"}`
  - `{"swagger-2.0-ref-http.yaml", ..., "EXTERNAL_REF"}`, `{"swagger-2.0-ref-file.yaml", ..., "EXTERNAL_REF"}`
  Pemetaan fixture -> kode memakai pola tabel yang ada (loop yang sudah memeriksa `Issues` berisi pointer/line untuk kasus non-kosong; bila belum, tambah pemeriksaan `Line > 0` dan `Pointer != ""` untuk kode struktural/ref).
- Di `TestValidate_MixedKeepsOrderAndSummary`: urutan `petstore-3.0.yaml`, `README.md`, `swagger-2.0.yaml`, `broken.yaml` kini menghasilkan `valid, skipped, valid, rejected`; ubah `want` dan `Summary` menjadi `{Valid: 2, Skipped: 1, Rejected: 1}`. Agar tes tetap menguji penolakan berurutan, ganti `swagger-2.0.yaml` dengan `swagger-1.2.yaml` bila ingin mempertahankan angka lama; pilih SATU (disarankan: pertahankan `swagger-2.0.yaml` dan ubah ekspektasi, supaya kasus 2.0 valid tercakup di campuran).
- Validate tidak menyimpan apa pun: pastikan tes yang ada (`repo`/`audit` tidak tersentuh) mencakup fixture 2.0 valid melalui tabel yang sama.

### Langkah 4 — Balik dan tambah tes di `apispec_integration_test.go`

File: `APIDIR/apispec_integration_test.go` (build tag `integration`, DB test lewat `db.TestDB(t)`; jalankan dengan `TEST_DATABASE_URL` ke DB test dari `make dev-test-db`, BUKAN DB dev).

1. `TestT06_AC2_RejectedWithReasons`: baris `itFile{"old.yaml", readTestdata(t, "swagger-2.0.yaml")}` dan `want["old.yaml"] = "NOT_OPENAPI_3"` diganti dengan `itFile{"old.yaml", readTestdata(t, "swagger-1.2.yaml")}` dan `"UNSUPPORTED_VERSION"`. Sisanya (4 rejected, 0 hit listener, 0 baris tersimpan, 0 audit) tidak berubah.
2. Tambah helper kecil di file tes yang sama:
   - `func (e *itEnv) get(pid, name string) *httptest.ResponseRecorder` (GET `/api/projects/{pid}/api-specs/{name}`) dan `raw(pid, name)` (GET `.../raw`) memakai `e.do(...)`.
   - `func (e *itEnv) endpoints(pid, spec string) []itEndpoint` (method+path berurutan dari tabel `api_spec_endpoints` urut sesuai kolom urut yang dipakai `getspec`; ikuti pola `endpointPaths` yang sudah ada).
3. Tes baru (nama berawalan `TestT10_`):
   - `TestT10_AC1_Swagger2PetstoreYAMLAndJSON`: unggah `swagger-2.0-petstore.yaml` dan `swagger-2.0-petstore.json` (nama berbeda) -> `Summary.Created == 2`; tiap hasil `OpenAPIVersion == "2.0"`, `EndpointCount == 4`, `Title`/`APIVersion` terisi; kolom DB `api_specs.openapi_version == '2.0'` (query `e.count` atau `QueryRow`); audit `uploaded` 2 kali.
   - `TestT10_Swagger2Lifecycle_CreatedUpdatedUnchanged`: unggah -> `created`; unggah ulang byte sama -> `unchanged` tanpa audit baru; unggah versi berbeda dengan nama sama (tambah satu path) -> `updated`, endpoint lama diganti, audit `uploaded` dengan `overwritten == true`.
   - `TestT10_Swagger2EndpointOrderMatches3x`: unggah petstore 3.0 (`petstore-3.0.yaml`) dan petstore 2.0 dengan set path/method setara; urutan `(method, path)` hasil `e.endpoints` identik pola pengurutan (`base.SortEndpoints`). Bila isi dua fixture tidak setara, bandingkan 2.0 terhadap ekspektasi eksplisit hasil `SortEndpoints` untuk: GET /pets, POST /pets, GET /pets/{petId}, DELETE /pets/{petId}.
   - `TestT10_DownloadIsByteIdentical`: unggah `swagger-2.0-petstore.yaml`, `swagger-2.0-petstore.json`, dan `swagger-2.0-numeric.yaml` (`swagger: 2.0` tanpa kutip, membuktikan penyimpanan tidak menormalkan); GET `/raw` -> `bytes.Equal` dengan konten asli, header `Content-Type` sesuai format. Sertakan satu file dengan BOM/CRLF bila fixture tersedia untuk mengunci "byte sama" (periksa dulu bagaimana `upload` menyimpan BOM: ikuti perilaku 3.x yang ada, jangan mengubahnya).
   - `TestT10_GetSpecReturnsSwagger2Document`: GET detail petstore 2.0 -> 200; `spec.openapi_version == "2.0"`; `document.swagger == "2.0"`; `document.definitions.Pet` ada; `document` TIDAK memuat kunci `openapi`; `$ref` tetap `#/definitions/Pet` (tidak ditulis ulang); `endpoints` berisi 4 entri.
   - `TestT10_Swagger2Rejected_WithPointerAndLine`: unggah `swagger-2.0-unresolved-ref.yaml`, `swagger-2.0-dup-opid.yaml`, `swagger-2.0-bad-params.yaml` -> semua `rejected`, `reason_code == INVALID_SPEC`; kode issue pertama masing-masing `UNRESOLVED_REF`, `DUPLICATE_OPERATION_ID`, `INVALID_STRUCTURE`; setiap issue `Pointer` berawalan `#/` dan `Line > 0`; bad-params berisi >= 3 issue `INVALID_STRUCTURE`; tidak ada baris tersimpan dan tidak ada audit.
   - `TestT10_Swagger2ExternalRefNoOutboundRequest`: seperti `AC2`, `httptest.NewServer` penghitung hit; ganti placeholder port di `swagger-2.0-ref-http.yaml` (pakai placeholder yang dipakai fixture; konvensi yang sudah ada di `ref-http.yaml` adalah `__PORT__`) dan unggah juga `swagger-2.0-ref-file.yaml` -> keduanya `rejected` dengan `EXTERNAL_REF`; `hits == 0`.
   - `TestT10_Swagger2VersionErrors`: `swagger-1.2.yaml` -> `UNSUPPORTED_VERSION`, pesan memuat `versi Swagger "1.2" tidak didukung (hanya 2.0)`, pointer `#/swagger`, `Line > 0`; `swagger-null.yaml` -> `NOT_OPENAPI`.
   - `TestT10_ValidateEndpoint_Swagger2`: POST `/validate` untuk petstore 2.0 valid -> `valid`, tidak ada baris di `api_specs`, tidak ada audit; untuk `swagger-2.0-unresolved-ref.yaml` -> `rejected` dengan issue berpointer.
   - `TestT10_Swagger2Regression3x`: unggah `petstore-3.0.yaml` dan `petstore-3.1.json` bersama satu file 2.0 dalam satu request -> `created == 3`, `openapi_version` masing-masing `3.0.x`, `3.1.x`, `2.0`; GET detail 3.x tetap memuat `openapi` dan tidak memuat `swagger`.
   - `TestT10_ProjectIsolationSwagger2`: spec 2.0 di project A tidak terlihat dari project B (404), meniru `TestT06_AC7_ProjectIsolation`.
4. Tes lama `TestT06_AC6_OpenAPI30And31` tidak diubah.

### Langkah 5 — Kontrak API

File: `api/modules/project/api-spec.yml`. Hanya contoh dan deskripsi; bentuk skema, kode status, dan nama field TIDAK berubah.

1. Upload (`POST .../upload`), contoh respons 200: tambahkan satu entri hasil `created` untuk Swagger 2.0 (mis. `file: petstore.yaml`, `name: petstore`, `status: created`, `openapi_version: "2.0"`, `title: Swagger Petstore`, `api_version: 1.0.0`, `endpoint_count: 4`) dan sesuaikan `summary` (`created: 1, updated: 1, unchanged: 0, skipped: 0, rejected: 1`). Entri `package.json` rejected tetap.
2. Validate (`POST .../validate`), contoh 200: ganti contoh `NOT_OPENAPI_3` (`Swagger 2.0 tidak didukung...`) dengan contoh Swagger 2.0 yang `status: valid`, `reason_code: null`, `reason: null`, `issues: []`, `openapi_version: "2.0"`, `endpoint_count: 4`; `summary: { valid: 1, skipped: 0, rejected: 0 }`. Sertakan entri kedua `rejected` dengan `UNRESOLVED_REF` berpointer dan baris untuk menunjukkan issue 2.0 (`#/paths/~1pets/get/responses/200/schema/$ref`, `line: 21`). Sesuaikan summary menjadi `{ valid: 1, skipped: 0, rejected: 1 }`. Teks message: ambil dari keluaran nyata tes `TestT10_Swagger2Rejected_WithPointerAndLine` agar contoh tidak ilustratif saja.
3. Detail (`GET .../{name}`): deskripsi `document` diperbarui menjadi "Dokumen spec sebagai JSON, apa adanya dari file unggahan: `openapi` untuk 3.x atau `swagger: "2.0"` untuk Swagger 2.0 (tidak dikonversi). `$ref` internal dipertahankan..." Tambah satu contoh kedua (`examples.swagger2`) dengan `openapi_version: "2.0"` dan `document` berkunci `swagger`, `host`, `basePath`, `definitions`. Skema `ApiSpecDetail.document` (deskripsi di bagian schemas) diperbarui sejalan.
4. Skema `ApiSpec.openapi_version`: deskripsi "versi dokumen: nomor `openapi` untuk 3.x, atau `2.0` untuk Swagger 2.0", `example` tetap `3.0.3`; tambah `examples: ["3.0.3", "3.1.0", "2.0"]` bila gaya file memakai OpenAPI 3.1 (`type: [string, "null"]` bentuk dipertahankan). Properti serupa di `ApiSpecFileResult.openapi_version` diberi deskripsi sama.
5. Daftar kode issue: bila ada deskripsi yang menyebut `NOT_OPENAPI_3`, ubah menjadi "kode dipertahankan untuk kompatibilitas, tidak lagi dihasilkan sejak T10". Kode `NOT_OPENAPI_3` TIDAK dihapus dari enum.
6. `make api-bundle`, commit `api/openapi.yml` hasil generate (jangan edit manual), lalu `make api-check` harus lulus. Catatan: pada snapshot repo ini target `api-bundle`/`api-check` belum terlihat di `Makefile` root (hanya dirujuk di `docs/tickets/README.md`); bila target memang tidak ada, laporkan dan jangan membuat bundle manual (aturan `.claude/agents/api-design.md`).

### Langkah 6 — Fixture e2e

- Path yang dituju tiket: `frontend/e2e/fixtures/apispec/swagger-2.0.yaml`. Pada snapshot ini folder `frontend/e2e/fixtures/apispec` belum ada (hanya `fixtures/knowledge`) dan fixture spec e2e saat ini berupa string di `frontend/e2e/helpers/apispecs.ts`. Buat `frontend/e2e/fixtures/apispec/swagger-2.0.yaml` berisi petstore Swagger 2.0 VALID (isi sama dengan `base/openapi/testdata/swagger-2.0-petstore.yaml`: `/pets` GET/POST, `/pets/{petId}` GET/DELETE, `definitions` Pet/Error, satu operasi `deprecated`, `host`, `basePath`, `schemes`, satu `securityDefinitions` apiKey, `POST /pets` dengan parameter `in: body` agar skenario e2e baca-operasi berbody punya data). Jika pembangun menemukan fixture ini sudah ada di branch lain sebagai dokumen ditolak, ubah isinya menjadi valid, bukan membuat file kedua.
- Pembaca fixture: spec e2e yang mengunggah file 2.0 membaca file ini (mis. lewat `fs.readFileSync`) dan memakai `uploadSpecsApi`. Perubahan spec e2e itu sendiri milik plan frontend; di sini hanya file fixture.

### Langkah 7 — Catatan revisi dokumen

Tambah catatan revisi di BAGIAN ATAS (tanpa menulis ulang isi lama):

1. `docs/tickets/T06-project-api-spec.md`: blok `> Revisi T10 (2026-10-21): ...` di bawah judul/tabel, berisi bahwa "Swagger 2.0" di bagian Non-goals dan penolakan Swagger 2.0 pada skenario (baris yang menyebut spec Swagger 2.0 `rejected`, sekitar skenario 2) tidak lagi berlaku; dukungan 2.0 dibahas di T10; ketentuan lain tidak berubah.
2. `docs/superpowers/plans/project-api-spec/overview.md`: blok revisi serupa menyebut OV lama yang memperlakukan 2.0 sebagai `NOT_OPENAPI_3` digantikan `docs/superpowers/plans/project-api-spec-swagger2/overview.md`.
3. Tambah blok revisi singkat di `docs/superpowers/plans/project-api-spec/usecase-uploadspecs.md` dan `usecase-validatespec.md` HANYA bila bagian rujukannya menyebut penolakan 2.0; selain itu cukup dua file di atas (tiket: "diberi catatan revisi, tidak ditulis ulang"). Tetap jangan mengubah isi lama.

### Langkah 8 — Verifikasi

```
cd backend
go test ./...
go vet ./...
TEST_DATABASE_URL=<DB test dari make dev-test-db> go test -tags integration ./internal/domain/project/internal/apispec/...
go test ./internal/archtest/...    # tes arsitektur T00
cd .. && make api-bundle && make api-check
```

Seluruh tes 3.0/3.1 yang ada lulus tanpa diubah, kecuali tiga titik penolakan 2.0 yang dibalik.

### Langkah 9 — Uji nyata hr-platform (kriteria 7)

1. Jalankan tahap b spike (OV F0) dengan `SPIKE_SPEC=/Users/ascendiz/Documents/Projects/ascendiz/saas/hr-platform/api/openapi.yml` bila belum dilakukan setelah usecase `getendpoint` selesai; angkanya dilaporkan di sini.
2. Unggah file itu lewat jalur REST nyata (backend lokal dengan DB dev, `curl`/UI, atau tes integration sementara berflag env yang TIDAK di-commit). Nama spec `hr-platform`.
3. Laporkan apa adanya (di pesan akhir/PR dan, jika disetujui pemilik, di catatan tiket T10): ukuran byte, jumlah node/alias/kedalaman, waktu parse, status upload (`created` atau `rejected` beserta kode issue pertama), `endpoint_count`, waktu GET detail, ukuran respons detail. Jika `rejected` karena batas (`YAML_LIMIT`, `FILE_TOO_LARGE`) atau karena issue struktur/ref, tulis kode dan pointer persis.
4. Bila gagal karena batas: BERHENTI dan eskalasi ke pemilik (usulan angka + data + alasan). Jika karena issue validasi 2.0 (bukan batas): perbaiki cabang di foundation (F2/F3), ulangi. Jangan mengubah konstanta batas.

---

## 4. Migration

Tidak ada. `api_specs.openapi_version` sudah `text NOT NULL` tanpa CHECK; `"2.0"` disimpan apa adanya (OV D1, OV 6).

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `APIDIR/uploadspecs/usecase_test.go` | hapus baris `swagger2`->`NOT_OPENAPI_3`; tambah baris `UNSUPPORTED_VERSION` dan `UNRESOLVED_REF` 2.0; tambah `TestUploadSpecs_Swagger2Created` (+ subcase unchanged) |
| `APIDIR/validatespec/usecase_test.go` | baris `swagger-2.0.yaml` jadi `valid`; baris fixture 2.0 baru; `TestValidate_MixedKeepsOrderAndSummary` disesuaikan |
| `APIDIR/apispec_integration_test.go` | `AC2` dibalik ke `swagger-1.2.yaml`/`UNSUPPORTED_VERSION`; helper `get/raw/endpoints`; tes `TestT10_*` |
| `api/modules/project/api-spec.yml` | contoh upload, contoh validate, deskripsi dan contoh detail, deskripsi `openapi_version`, catatan `NOT_OPENAPI_3` tetap ada |
| `api/openapi.yml` | hasil `make api-bundle` (di-commit, tidak diedit manual) |
| `frontend/e2e/fixtures/apispec/swagger-2.0.yaml` | petstore 2.0 valid |
| `docs/tickets/T06-project-api-spec.md` | catatan revisi T10 |
| `docs/superpowers/plans/project-api-spec/overview.md` | catatan revisi T10 |
| `APIDIR/**/usecase.go, handler.go, repository.go, init.go` | TIDAK diubah |
| `lib/constants/bus.go`, `lib/contracts/**`, `backend/migrations/**`, `limits.go`, `yamlguard.go`, `issue.go` | TIDAK diubah |

Pesan commit: `test(project-api-spec): balik tes penolakan Swagger 2.0 dan tambah integration`, `docs(project-api-spec): kontrak contoh Swagger 2.0 dan bundle`, `test(project-api-spec): fixture e2e swagger-2.0 valid`, `docs(tickets): catatan revisi T10 di T06 dan plan project-api-spec`.

---

## 6. Test Scenario

Unit (`go test ./internal/domain/project/internal/apispec/...`):

| Tes | Ekspektasi |
|---|---|
| `TestUploadSpecs_Swagger2Created` | `created`, `OpenAPIVersion "2.0"`, 4 endpoint, 1 insert, 1 audit; unggah ulang -> `unchanged`, 0 audit baru |
| `TestUploadSpecs_InvalidSpecs` (baris 2.0) | `swagger: '1.2'` -> `UNSUPPORTED_VERSION`; 2.0 dengan `$ref` hilang -> `UNRESOLVED_REF`; reason == pesan issue pertama; tanpa tx dan audit |
| `TestValidate_PerFileCases` (baris 2.0) | petstore yaml/json/numeric -> `valid`; 1.2 -> `UNSUPPORTED_VERSION`; null -> `NOT_OPENAPI`; unresolved/dup/bad-params/ref-http/ref-file -> kode masing-masing dengan pointer dan `Line > 0` |
| `TestValidate_MixedKeepsOrderAndSummary` | `valid, skipped, valid, rejected`; summary `{2,1,1}` |

Integration (`-tags integration`, DB test): daftar `TestT10_*` di langkah 4. Inti kasus:

- created/updated/unchanged dengan `openapi_version "2.0"` di DB dan respons.
- urutan endpoint sama dengan aturan `SortEndpoints` 3.x.
- unduh `/raw` byte identik (YAML, JSON, `swagger: 2.0` tanpa kutip).
- GET detail: `document.swagger == "2.0"`, tanpa kunci `openapi`, `$ref` tidak ditulis ulang.
- rejected: `UNRESOLVED_REF`, `DUPLICATE_OPERATION_ID`, `INVALID_STRUCTURE` masing-masing berpointer dan baris; tidak ada baris DB dan audit.
- `EXTERNAL_REF` (http dan file): `rejected`, listener lokal 0 hit.
- validate: tidak menyimpan apa pun.
- regresi 3.x: 3.0/3.1 tetap benar dalam request campuran.
- isolasi project untuk spec 2.0.

Kontrol regresi: tes 3.0/3.1 lama lulus tanpa diubah; `imports_test.go` dan tes arsitektur T00 hijau; `make api-check` lulus.

---

## 7. E2E Scenarios

Positif:
1. Unggah `swagger-2.0-petstore.yaml` (fixture e2e `swagger-2.0.yaml`) -> `created`, `openapi_version "2.0"`, `endpoint_count 4`.
2. Unggah `swagger-2.0-petstore.json` dengan nama lain -> `created`; GET `/raw` = byte file asli.
3. Unggah ulang file sama -> `unchanged`.
4. POST `/validate` file 2.0 valid -> `valid`, tidak tersimpan.
5. GET detail -> `document.swagger == "2.0"` apa adanya; `endpoints` 4 entri berurutan.
6. Spec 3.0.3 yang ada: upload, detail, dan unduh identik dengan sebelum T10.
7. Uji nyata hr-platform (langkah 9) dilaporkan apa adanya.

Negatif:
1. `swagger-2.0-unresolved-ref.yaml` -> `rejected`, `UNRESOLVED_REF` dengan pointer + baris.
2. `swagger-2.0-dup-opid.yaml` -> `rejected`, `DUPLICATE_OPERATION_ID`.
3. `swagger-2.0-bad-params.yaml` -> `rejected`, beberapa `INVALID_STRUCTURE`.
4. `swagger-2.0-ref-http.yaml` / `swagger-2.0-ref-file.yaml` -> `rejected`, `EXTERNAL_REF`, 0 permintaan keluar.
5. `swagger-1.2.yaml` -> `rejected`, `UNSUPPORTED_VERSION`; `swagger-null.yaml` -> `rejected`, `NOT_OPENAPI`.
6. File 2.0 > 5 MiB -> `rejected`, `FILE_TOO_LARGE` (batas tidak berubah).
7. Non-anggota project -> 404; tanpa header `X-Requested-With` -> ditolak CSRF (perilaku T00 tidak berubah).

---

## 8. Request/Response body

### Upload (POST `/api/projects/{projectID}/api-specs/upload`, multipart, header `X-Requested-With: grimoire`)

Field: `files` (berulang, wajib), `name` (opsional, hanya bila satu file, pola `^[a-z0-9._-]{1,80}$`).

Respons 200 (campuran created 2.0, rejected ref hilang, rejected versi):

```json
{
  "results": [
    {
      "file": "petstore.yaml",
      "source_path": "petstore.yaml",
      "name": "petstore",
      "status": "created",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "2.0",
      "title": "Swagger Petstore",
      "api_version": "1.0.0",
      "endpoint_count": 4
    },
    {
      "file": "broken.yaml",
      "source_path": "broken.yaml",
      "name": "broken",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "$ref tidak ditemukan: #/definitions/Hilang",
      "issues": [
        {
          "code": "UNRESOLVED_REF",
          "message": "$ref tidak ditemukan: #/definitions/Hilang (baris 21, #/paths/~1pets/get/responses/200/schema/$ref)",
          "pointer": "#/paths/~1pets/get/responses/200/schema/$ref",
          "line": 21
        }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    },
    {
      "file": "legacy.yaml",
      "source_path": "legacy.yaml",
      "name": "legacy",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "versi Swagger \"1.2\" tidak didukung (hanya 2.0)",
      "issues": [
        {
          "code": "UNSUPPORTED_VERSION",
          "message": "versi Swagger \"1.2\" tidak didukung (hanya 2.0) (baris 1, #/swagger)",
          "pointer": "#/swagger",
          "line": 1
        }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 2 }
}
```

Teks persis `message`/`reason` dikunci dari keluaran tes integration; contoh di atas harus disalin dari hasil tes saat mengisi `api-spec.yml`.

### Validate (POST `/api/projects/{projectID}/api-specs/validate`)

```json
{
  "results": [
    {
      "file": "petstore.yaml",
      "source_path": "petstore.yaml",
      "name": "petstore",
      "status": "valid",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "2.0",
      "title": "Swagger Petstore",
      "api_version": "1.0.0",
      "endpoint_count": 4
    }
  ],
  "summary": { "valid": 1, "skipped": 0, "rejected": 0 }
}
```

### Detail (GET `/api/projects/{projectID}/api-specs/petstore`), dipotong

```json
{
  "spec": {
    "name": "petstore",
    "title": "Swagger Petstore",
    "openapi_version": "2.0",
    "api_version": "1.0.0",
    "format": "yaml",
    "source_path": "petstore.yaml",
    "size_bytes": 2048,
    "endpoint_count": 4,
    "created_at": "2026-10-21T09:00:00Z",
    "created_by_label": "Budi",
    "updated_at": "2026-10-21T09:00:00Z",
    "updated_by_label": "Budi"
  },
  "endpoints": [
    { "method": "GET", "path": "/pets", "operation_id": "listPets", "summary": "List pets", "tags": ["pets"], "deprecated": false }
  ],
  "document": {
    "swagger": "2.0",
    "info": { "title": "Swagger Petstore", "version": "1.0.0" },
    "host": "petstore.example.com",
    "basePath": "/v1",
    "schemes": ["https"],
    "paths": { "/pets": { "get": { "operationId": "listPets", "responses": { "200": { "description": "ok", "schema": { "type": "array", "items": { "$ref": "#/definitions/Pet" } } } } } } },
    "definitions": { "Pet": { "type": "object", "required": ["id", "name"], "properties": { "id": { "type": "integer" }, "name": { "type": "string" } } } }
  }
}
```

### Unduh (GET `/api/projects/{projectID}/api-specs/petstore/raw`)

Body = byte persis unggahan; `Content-Type` mengikuti format tersimpan (`application/yaml` atau `application/json`), tanpa perubahan kode.

### Format error (tidak berubah)

Request tanpa file:

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Spec tidak ditemukan / bukan anggota:

```json
{
  "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Payload terlalu besar: `413` `PAYLOAD_TOO_LARGE` (envelope sama). File 2.0 melebihi 5 MiB bukan error HTTP: hasil per file `rejected` dengan `reason_code: "FILE_TOO_LARGE"`.

---

## 9. Risiko dan catatan

1. Hasil spike hr-platform bisa menghentikan langkah 9 (batas D8). Langkah 2-8 tidak terhalang.
2. Pada snapshot ini target `make api-bundle`/`make api-check` tidak terlihat di `Makefile` root; verifikasi sebelum langkah 5. Bila tidak ada, laporkan.
3. Fixture e2e `frontend/e2e/fixtures/apispec/` belum ada di repo; langkah 6 membuatnya. Overview menyebut fixture "dijadikan valid"; ini diselaraskan di sini.
4. Placeholder port di `swagger-2.0-ref-http.yaml`: konvensi repo adalah `__PORT__` (OV F5 menulis `<port>`); sesuaikan `strings.ReplaceAll` dengan yang benar-benar dipakai fixture foundation.
5. K1-K12 belum disetujui pemilik (OV 3); perubahan keputusan memengaruhi kode versi `"2.0"` dan teks pesan `UNSUPPORTED_VERSION` yang dikunci tes di sini.
