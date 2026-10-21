# Dukungan Swagger 2.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Spec Swagger 2.0 bisa diunggah, divalidasi, disimpan apa adanya, dibaca di UI, dan operasinya bisa diambil lewat bus `project.apispec.get-endpoint`, tanpa mengubah perilaku spec 3.x.

**Architecture:** Parser/validator/ekstraktor/resolver di `base/openapi` mendapat cabang native 2.0 (tanpa konversi di backend). Frontend memetakan dokumen 2.0 ke bentuk 3.x hanya di lapisan tampilan lewat fungsi murni `adaptSwagger2`, diterapkan pada salinan di memori setelah dokumen diambil.

**Tech Stack:** Go (`go.yaml.in/yaml/v3`, paket `grimoire/internal/domain/project/internal/apispec/...`), Svelte 5 + TypeScript + Zod + Vitest, OpenAPI kontrak di `api/modules/project/api-spec.yml` (`make api-bundle`).

**Spec:** `docs/superpowers/specs/2026-10-21-swagger2-support-design.md` (nomor "keputusan saya" di bawah merujuk ke daftar bernomor 1–13 di dokumen itu).

## Global Constraints

- File tersimpan dan diunduh = byte yang diunggah; backend tidak pernah menulis ulang isi spec.
- Nilai versi tersimpan untuk Swagger 2.0 persis `"2.0"` di kolom `api_specs.openapi_version` (tanpa migrasi).
- `swagger` diterima sebagai string `"2.0"`, angka `2.0`, dan integer `2`; `"2.0.0"`, `"1.2"`, nilai non-skalar → `UNSUPPORTED_VERSION`; `swagger: null` → `NOT_OPENAPI`. Dokumen yang punya `openapi` dan `swagger` sekaligus diperlakukan sebagai 3.x.
- Method yang diindeks untuk 2.0: get, put, post, delete, options, head, patch (tanpa trace, tanpa webhooks).
- `$ref` bukan `#...` → `EXTERNAL_REF`; `$ref` internal tak ditemukan → `UNRESOLVED_REF` (sama seperti 3.x). Tidak ada permintaan jaringan keluar.
- Batas parser (`MaxDepth 64`, `MaxAliases 100`, `MaxNodes 500_000`, `apispec_max_file_bytes` 5 MiB) tidak diubah tanpa persetujuan eksplisit pemilik.
- Kode `NOT_OPENAPI_3` tetap ada di kode dan kontrak API; kunci i18n `apispec.issue.NOT_OPENAPI_3` dibiarkan.
- Keluaran spec 3.x (bentuk `get-endpoint`, urutan endpoint, pesan issue) tidak berubah satu byte pun.
- Teks UI Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`. Teks versi: "Swagger 2.0" untuk 2.0, "OpenAPI x.y.z" untuk 3.x.
- Satu commit per task; pesan `feat(project-api-spec): ...` / `test(project-api-spec): ...`.

## Review Focus

1. Operasi 2.0 tanpa `consumes` di operasi maupun root, dengan parameter `in: body` → body tampil dengan MIME `application/json` (bukan hilang).
2. Respons 2.0 tanpa `schema` (mis. `204`) dan respons dengan `examples` MIME → tampil tanpa error; contoh tidak diperlakukan sebagai Example Object 3.x oleh pemeriksa `$ref`.
3. `definitions` sirkular (`Node.children[] -> Node`) → upload lolos, `get-endpoint` memberi `$circular`, adapter FE tidak berulang tak hingga.
4. `host` kosong atau `schemes` kosong → `servers` tetap terbentuk (`basePath` saja / `https`), tidak panik.
5. Dokumen 3.x yang kebetulan punya kunci `swagger` di dalam `info`/`x-*` tidak salah dikenali sebagai 2.0; dokumen dengan `openapi` dan `swagger` di root → 3.x.

Tiap butir di atas dipetakan ke tes di task pemiliknya (lihat "Review Focus" di task terkait).

---

### Task 1: Parser menerima `swagger` 2.0 dan membawa versi

**Files:**
- Modify: `backend/internal/domain/project/internal/apispec/base/openapi/parse.go` (blok `ver, has := root.Get("openapi")`, `versionRe`, `Document`)
- Modify: `backend/internal/domain/project/internal/apispec/base/openapi/parse_test.go` (baris ~49: `{"swagger-2.0.yaml", CodeNotOpenAPI3}`)
- Create: `backend/internal/domain/project/internal/apispec/base/openapi/testdata/swagger-2.0-petstore.yaml`, `swagger-2.0-petstore.json`, `swagger-2.0-numeric.yaml` (`swagger: 2.0`), `swagger-1.2.yaml`, `swagger-null.yaml`, `both-keys.yaml`

**Interfaces:**
- Produces: `const Swagger2Version = "2.0"`; `func (d Document) IsSwagger2() bool` (true bila `d.OpenAPIVersion == Swagger2Version`); `Parse` mengembalikan `Document{OpenAPIVersion: "2.0", Title, APIVersion, Root}` untuk 2.0 valid. Cabang 2.0 memanggil `validateSwagger2(doc)` (Task 2) lalu `checkRefs(doc)` (Task 3).

- [ ] **Step 1: Tulis tes gagal.** Di `parse_test.go`: `TestParse_Swagger2Accepted` (fixture `swagger-2.0.yaml` → tidak ada issue, `doc.OpenAPIVersion == "2.0"`, `doc.IsSwagger2()`); `TestParse_Swagger2VersionForms` (tabel: `"2.0"`, `2.0`, `2` → `"2.0"`; `"2.0.0"`, `"1.2"`, `[2]` → `CodeUnsupportedVersion`; `swagger: null` → `CodeNotOpenAPI`); `TestParse_BothKeysIs3x` (fixture `both-keys.yaml` memakai `openapi: 3.0.3` + `swagger: "2.0"` → versi `3.0.3`); `TestParse_SwaggerKeyInsideInfoIs3x` (fixture `swagger-in-info.yaml`: `openapi: 3.0.3` dengan `info.x-swagger: "2.0"` dan `info.swagger: "2.0"` → versi `3.0.3`, tidak dikenali sebagai 2.0). Ubah baris `swagger-2.0.yaml` di tabel penolakan lama menjadi kasus `swagger-1.2.yaml` → `CodeUnsupportedVersion`.
- [ ] **Step 2: Jalankan** `cd backend && go test ./internal/domain/project/internal/apispec/base/openapi/ -run 'TestParse_(Swagger2|BothKeys)' -v` → FAIL.
- [ ] **Step 3: Implementasikan** di `parse.go`: ganti cabang `Swagger 2.0 tidak didukung` dengan fungsi `swaggerVersion(v any) (string, bool)` yang menormalkan string/`json.Number`/`float64` bernilai 2/2.0 ke `Swagger2Version`; nilai lain → `CodeUnsupportedVersion` dengan pesan `versi Swagger "..." tidak didukung (hanya 2.0)` dan pointer `#/swagger`. Panggil validator cabang 2.0 (stub yang mengembalikan nil di task ini). `Document.Title/APIVersion` dibaca dari `info` seperti 3.x.
- [ ] **Step 4: Jalankan** perintah Step 2 plus `go test ./internal/domain/project/internal/apispec/base/openapi/` → PASS (tes 3.x lama tidak boleh berubah).
- [ ] **Step 5: Commit** `feat(project-api-spec): parser menerima Swagger 2.0 dan menyimpan versi "2.0"`.

### Task 2: Validasi struktur Swagger 2.0

**Files:**
- Create: `backend/internal/domain/project/internal/apispec/base/openapi/validate_swagger2.go`, `validate_swagger2_test.go`
- Modify: `parse.go` (memanggil `validateSwagger2`), `validate.go` (hanya bila helper dibagi; boleh dibiarkan)
- Create fixtures: `swagger-2.0-bad-params.yaml`, `swagger-2.0-dup-opid.yaml`

**Interfaces:**
- Produces: `func validateSwagger2(doc Document) []rawIssue` — memakai `v.add/v.bad`-style yang sama (`CodeInvalidStructure`, `CodeDuplicateOpID`), batas `maxRawIssues`.
- Aturan: `info.title` dan `info.version` string; `paths` wajib object; kunci path diawali `/` (kecuali `x-`); method yang dikenali = `swagger2Methods` (get put post delete options head patch); `operationId` string dan unik; `tags` array string; `deprecated` bool; `parameters` array dengan `name` string dan `in` ∈ {query, header, path, formData, body}; `in: path` wajib `required: true`; `in: body` wajib punya `schema` object dan paling banyak satu per operasi; kode respons cocok `responseCodeRe`; `responses` wajib ada di operasi; `definitions`, `parameters`, `responses`, `securityDefinitions` (bila ada) berupa object; `security` array object; `consumes`/`produces` (root dan operasi) array string; `host` string; `basePath` string diawali `/`; `schemes` array dari {http, https, ws, wss}.

- [ ] **Step 1: Tulis tes gagal** (`validate_swagger2_test.go`), semua lewat `Parse`: `TestSwagger2_Petstore_OK`; `TestSwagger2_MissingInfoTitle` (pointer `#/info/title`); `TestSwagger2_PathParamNotRequired`; `TestSwagger2_TwoBodyParams`; `TestSwagger2_BadParamIn` (`in: cookie` ditolak untuk 2.0); `TestSwagger2_DuplicateOperationID` (kode `CodeDuplicateOpID`, ada nomor baris); `TestSwagger2_BadBasePath`; `TestSwagger2_NoTraceIndexed` (operasi `trace` diabaikan, tidak error).
- [ ] **Step 2: Jalankan** `go test ./internal/domain/project/internal/apispec/base/openapi/ -run TestSwagger2 -v` → FAIL.
- [ ] **Step 3: Implementasikan** `validateSwagger2` sesuai aturan di atas. Pakai ulang `asMap`, `mustGet`, `joinPtr`, `isStr`, `isBool`, `quoteShort`, `responseCodeRe`; boleh membagi `securityArray`/`objectArray` dengan `validate.go` lewat receiver `validator` yang sama.
- [ ] **Step 4: Jalankan** seluruh paket `base/openapi` → PASS.
- [ ] **Step 5: Commit** `feat(project-api-spec): validasi struktur Swagger 2.0`.

### Task 3: Pemeriksa `$ref` untuk 2.0

**Files:**
- Modify: `backend/internal/domain/project/internal/apispec/base/openapi/refs.go` (`checkRefs`, `refChecker`), `parse.go` (memanggil `checkRefs` untuk 2.0)
- Test: `refs_test.go`; fixtures `swagger-2.0-unresolved-ref.yaml` (`$ref: '#/definitions/Hilang'`), `swagger-2.0-ref-http.yaml`, `swagger-2.0-example-with-ref-key.yaml`, `swagger-2.0-circular.yaml`

**Interfaces:**
- Consumes: `Document.IsSwagger2()` (Task 1).
- Produces: `checkRefs(doc)` tidak memperlakukan `examples` sebagai peta Example Object 3.x bila `doc.IsSwagger2()` (di 2.0 `examples` di respons adalah peta MIME → nilai literal; seluruh isinya dilewati). Untuk 2.0 `ResolvePointer` yang sama dipakai (`#/definitions/X`, `#/parameters/X`, `#/responses/X`).

- [ ] **Step 1: Tulis tes gagal:** `TestRefs_Swagger2Unresolved` (kode `CodeUnresolvedRef`, pointer ke lokasi `$ref`, baris terisi); `TestRefs_Swagger2External` (`CodeExternalRef`); `TestRefs_Swagger2ExamplesAreLiteral` (respons dengan `examples: {application/json: {"$ref": "#/nope"}}` tidak menghasilkan issue); `TestRefs_Swagger2CircularOK`.
- [ ] **Step 2: Jalankan** → FAIL pada kasus examples.
- [ ] **Step 3: Implementasikan** penyesuaian `refChecker` (field `swagger2 bool`; cabang `case !names && k == "examples"` memanggil `r.examples` hanya bila bukan 2.0, selain itu dilewati).
- [ ] **Step 4: Jalankan** paket `base/openapi` → PASS.
- [ ] **Step 5: Commit** `feat(project-api-spec): pemeriksaan $ref untuk Swagger 2.0`.

### Task 4: Ekstraksi endpoint dan pencarian operasi untuk 2.0

**Files:**
- Modify: `base/openapi/extract.go` (`ExtractEndpoints`), `base/openapi/resolve.go` (`FindOperation`), `base/openapi/validate.go` (var `opMethods`)
- Test: `extract_test.go`, `resolve_test.go`

**Interfaces:**
- Produces: `func methodsFor(doc Document) []string` (2.0 → tujuh method tanpa trace; selain itu `opMethods` yang ada). `ExtractEndpoints` dan `FindOperation` memakainya; urutan keluaran tetap lewat `base.SortEndpoints`.

- [ ] **Step 1: Tulis tes gagal:** `TestExtractEndpoints_Swagger2` (petstore 2.0 → method/path/operationId/tags/summary/deprecated benar, urutan kanonik sama seperti 3.x untuk set yang setara); `TestExtractEndpoints_Swagger2IgnoresTrace`; `TestFindOperation_Swagger2ByOperationID`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasikan** `methodsFor` dan ganti pemakaian `opMethods` langsung di `ExtractEndpoints` dan `FindOperation` (tes 3.x yang ada tetap hijau).
- [ ] **Step 4: Jalankan** paket → PASS.
- [ ] **Step 5: Commit** `feat(project-api-spec): ekstraksi endpoint dan FindOperation untuk 2.0`.

### Task 5: Gerbang ukur file hr-platform (tidak di-commit)

**Files:**
- Create sementara: `backend/internal/domain/project/internal/apispec/base/openapi/zz_spike_test.go` (dihapus setelah task; tidak di-commit)

**Interfaces:** tidak ada (read-only terhadap `Parse`).

- [ ] **Step 1: Tulis tes sementara** `TestSpikeHRPlatform` yang melewati sendiri bila env `SPIKE_SPEC` kosong; membaca file, memanggil `Parse(content, FormatYAML, DefaultLimits)` dan `ExtractEndpoints`, lalu mencetak: ukuran byte, jumlah issue (+ kode issue pertama), jumlah endpoint, waktu parse.
- [ ] **Step 2: Jalankan** `SPIKE_SPEC=/Users/ascendiz/Documents/Projects/ascendiz/saas/hr-platform/api/openapi.yml go test ./internal/domain/project/internal/apispec/base/openapi/ -run TestSpikeHRPlatform -v`.
- [ ] **Step 3: Putuskan.** Lolos tanpa issue → hapus file sementara, lanjut. Ada issue `YAML_LIMIT`, `FILE_TOO_LARGE`, atau waktu parse > 2 detik → **berhenti**, laporkan angka ukur dan usulan batas baru ke pemilik; jangan ubah `limits.go` atau konfigurasi sebelum ada persetujuan. Issue validasi lain → perbaiki cabang 2.0 (bukan batas) di task asal, lalu ulangi.
- [ ] **Step 4:** Hapus `zz_spike_test.go`. Tidak ada commit.

### Task 6: Resolusi operasi 2.0 (`servers`, keamanan, consumes/produces, penanda versi)

**Files:**
- Modify: `base/openapi/resolve.go` (`ResolveOperation`, `Resolved`, `firstServers`, `securityNames`), `backend/lib/contracts/project/apispec/getendpoint/getendpoint.go` (`Response`), `backend/internal/domain/project/internal/apispec/getendpoint/usecase.go` (menyalin field baru), `getendpoint/handler.go`
- Test: `base/openapi/resolve_test.go`, `getendpoint/usecase_test.go`

**Interfaces:**
- Produces: `Resolved` mendapat `OpenAPIVersion string` (terisi `"2.0"` hanya untuk 2.0, selain itu kosong), `Consumes json.RawMessage`, `Produces json.RawMessage` (array JSON; nil untuk 3.x). Contract `getendpoint.Response` mendapat field yang sama persis (`OpenAPIVersion string`, `Consumes, Produces json.RawMessage`). Untuk 2.0: `Servers` = array `[{"url": "<scheme>://<host><basePath>"}]` per scheme (`schemes` kosong → `https`; `host` kosong → `[{"url": "<basePath>"}]`, `basePath` kosong → `/`); `SecuritySchemes` = entri `securityDefinitions` yang disebut `security` efektif (operasi menimpa root); `Consumes/Produces` efektif (operasi menimpa root, default kosong `[]`). `$ref` di-inline dengan `inliner` yang sama (`#/definitions`, `#/parameters`, `#/responses`), parameter level path digabung dengan `mergedOperation`.

- [ ] **Step 1: Tulis tes gagal:** `TestResolveOperation_Swagger2_InlinesDefinitions`; `..._ServersFromHostBasePathSchemes` (dua scheme → dua server); `..._ServersNoHost` (hanya `basePath`); `..._ServersNoSchemes` (→ `https`); `..._SecurityFromDefinitions`; `..._EffectiveConsumesProduces` (operasi menimpa root); `..._Circular` (`$circular` ada, tidak hang); `TestResolveOperation_3xUnchanged` (Response 3.x: `OpenAPIVersion == ""`, `Consumes == nil`, `Produces == nil` — kunci kompatibilitas). Di `getendpoint/usecase_test.go`: `TestExecute_Swagger2SetsOpenAPIVersion`.
- [ ] **Step 2: Jalankan** `go test ./internal/domain/project/internal/apispec/... ./lib/contracts/project/apispec/...` pada tes baru → FAIL.
- [ ] **Step 3: Implementasikan** cabang `doc.IsSwagger2()` di `ResolveOperation` (fungsi pembantu `swagger2Servers(root *Map) []any`, `swagger2Security(op, root *Map) (names []string, defs *Map)`, `effectiveList(op, root *Map, key string) []any`), isi field baru di `Resolved` dan teruskan di `getendpoint/usecase.go` ke `Response`. Komentar paket kontrak diperbarui: field baru opsional, kosong untuk 3.x.
- [ ] **Step 4: Jalankan** seluruh `./internal/domain/project/...` dan `./lib/...` → PASS.
- [ ] **Step 5: Commit** `feat(project-api-spec): resolusi operasi Swagger 2.0 dan penanda versi pada get-endpoint`.

### Task 7: Upload/validate/getspec memakai cabang 2.0; kontrak API dan tes yang dibalik

**Files:**
- Modify: `apispec/uploadspecs/usecase_test.go:258`, `apispec/validatespec/usecase_test.go:89,224`, `apispec/apispec_integration_test.go:284`, `api/modules/project/api-spec.yml` (contoh `NOT_OPENAPI_3` di sekitar baris 109–114 diganti contoh 2.0 `created` dengan `openapi_version: "2.0"`; deskripsi `openapi_version` menyebut `"2.0"`), lalu `make api-bundle`
- Test: `apispec/apispec_integration_test.go` (tambah)

**Interfaces:**
- Consumes: `Parse` (Task 1–4). Tidak ada perubahan signature di `uploadspecs`/`validatespec`/`getspec`; `getspec` mengembalikan `Document` JSON utuh seperti sekarang.

- [ ] **Step 1: Balik tes:** kasus `swagger2` di `uploadspecs/usecase_test.go` → status `created` (bukan rejected), `OpenAPIVersion == "2.0"`; `validatespec` fixture `swagger-2.0.yaml` → `StatusOK` (bukan `INVALID_SPEC`); integration: unggah `swagger-2.0-petstore.yaml` → `created`, `GET` detail mengembalikan `openapi_version: "2.0"`, daftar endpoint terisi, unduh = byte sama; unggah `swagger-2.0-unresolved-ref.yaml` → `rejected` + issue `UNRESOLVED_REF` berpointer dan berbaris; unggah `swagger-2.0-ref-http.yaml` → `rejected` `EXTERNAL_REF` dengan listener lokal menghitung 0 request.
- [ ] **Step 2: Jalankan** `cd backend && go test ./internal/domain/project/internal/apispec/...` dan `go test -tags integration ./internal/domain/project/internal/apispec/... ` dengan `TEST_DATABASE_URL` menunjuk `grimoire_test` (bukan DB dev) → FAIL sebelum perubahan, lalu pastikan penyebabnya benar.
- [ ] **Step 3: Implementasikan** perubahan kontrak YAML dan `make api-bundle`; periksa tidak ada kode produksi lain yang menolak 2.0 (`grep -rn "NOT_OPENAPI_3\|CodeNotOpenAPI3" backend`; kode tetap didefinisikan, hanya tidak dipakai).
- [ ] **Step 4: Jalankan** seluruh paket apispec (unit + integration) dan `go vet ./...` → PASS. Pastikan tes 3.x tidak diubah kecuali baris yang disebut di atas.
- [ ] **Step 5: Commit** `feat(project-api-spec): upload dan baca spec Swagger 2.0 (tes penolakan dibalik, kontrak diperbarui)`.

### Task 8: Adapter FE `adaptSwagger2`

**Files:**
- Create: `frontend/src/domain/project/api-spec/getspec/model/swagger2.ts`, `swagger2.test.ts`
- Test fixtures: `frontend/src/domain/project/api-spec/getspec/mocks/detail-swagger2.ts` (dokumen 2.0 kecil: body, formData+file, header/query/path, respons dengan dan tanpa `schema`, `examples`, `definitions` sirkular, `x-nullable`, `securityDefinitions` basic/apiKey/oauth2, `host/basePath/schemes`)

**Interfaces:**
- Produces: `export const isSwagger2 = (doc: unknown): boolean` (true bila objek dengan `swagger` bernilai `"2.0"`/`2`/`2.0` dan tanpa kunci `openapi`); `export function adaptSwagger2(doc: Obj): Obj` — fungsi murni, tidak memutasi masukan, mengembalikan dokumen berbentuk 3.x untuk model yang ada (`operations.ts`, `refs.ts`, `schema.ts`, `security.ts`, `examples.ts`). Tidak melempar untuk bentuk tak dikenal: bagian yang tak dipahami disalin apa adanya.
- Aturan pemetaan (butir 8 di desain): `#/definitions/`→`#/components/schemas/`, `#/parameters/`→`#/components/parameters/`, `#/responses/`→`#/components/responses/` (di semua nilai `$ref`); `in: body` → `requestBody.content[mime].schema` untuk tiap MIME efektif (`consumes` operasi, lalu root, default `["application/json"]`), `required` ikut; `in: formData` → `requestBody` dengan `multipart/form-data` bila ada `type: file`, selain itu `application/x-www-form-urlencoded`, properti per parameter, `type: file` → `{type: string, format: binary}`; `responses.*.schema` → `content[mime].schema` per MIME `produces` efektif (default `["application/json"]`), respons tanpa `schema` → tanpa `content`; `responses.*.examples` → `content[mime].example`; `x-nullable: true` → `nullable: true`; parameter non-body: `type/format/items/enum/default` dipindah ke `schema`; `securityDefinitions` → `components.securitySchemes` (`basic`→`{type: http, scheme: basic}`, `apiKey` tetap, `oauth2` dengan `flow`/`authorizationUrl`/`tokenUrl`/`scopes` → `flows.{implicit|password|clientCredentials|authorizationCode}`); `host`/`basePath`/`schemes` → `servers` dengan aturan sama seperti backend (Task 6); `definitions` → `components.schemas`.

- [ ] **Step 1: Tulis tes gagal** (nama tes = ringkasan aturan): `isSwagger2 true/false (3.x, kunci ganda)`; `adaptSwagger2 menulis ulang $ref`; `body tanpa consumes → application/json`; `body dengan consumes dua MIME → dua entri content`; `formData file → multipart`; `respons tanpa schema → tanpa content`; `respons examples → example per MIME`; `x-nullable → nullable`; `securityDefinitions oauth2 → flows`; `servers dari host+basePath+schemes`; `host kosong → servers hanya basePath`; `definitions sirkular tidak membuat adapter berulang tak hingga` (adapter tidak meng-inline `$ref`); `tidak memutasi masukan` (deep-freeze); `bentuk tak dikenal tidak melempar`.
- [ ] **Step 2: Jalankan** `cd frontend && npx vitest run src/domain/project/api-spec/getspec/model/swagger2.test.ts` → FAIL.
- [ ] **Step 3: Implementasikan** `swagger2.ts` memakai helper `guards.ts` (`asObj/asArr/asStr/isObj`); satu fungsi kecil per aturan, `adaptSwagger2` menyusunnya.
- [ ] **Step 4: Jalankan** tes adapter plus tes `model/*` lain → PASS.
- [ ] **Step 5: Commit** `feat(project-api-spec): adapter Swagger 2.0 ke bentuk tampilan di frontend`.

### Task 9: FE — skema, service, lencana, teks hasil upload

**Files:**
- Modify: `frontend/src/domain/project/api-spec/getspec/types.ts` (`OpenApiDocumentSchema`), `getspec/services/getspec.service.ts`, `frontend/src/shared/components/OpenApiBadge.svelte`, `frontend/src/domain/project/api-spec/uploadspecs/components/ResultTable.svelte:48`, `frontend/src/shared/i18n/id.json`, `en.json`
- Test: `getspec/services/getspec.service.test.ts`, `getspec/views/SpecDetailView.test.ts`, `uploadspecs/components/components.test.ts`, komponen badge (`listspecs/components/components.test.ts`)
- Modify (balik): `uploadspecs/model.test.ts`, `uploadspecs/mocks/results.ts`, `uploadspecs/views/UploadSpecsDialog.test.ts` (baris yang mengunci penolakan `NOT_OPENAPI_3` untuk Swagger 2.0)

**Interfaces:**
- Consumes: `adaptSwagger2`, `isSwagger2` (Task 8).
- Produces: `OpenApiDocumentSchema` menerima dokumen 3.x (`openapi: string`) atau 2.0 (`swagger`), tetap `.passthrough()`; `getSpec(projectId, name, signal)` mengembalikan `ApiSpecDetail` yang `document`-nya sudah dipetakan bila `spec.openapi_version === "2.0"` (atau `isSwagger2(raw.document)`); fungsi `openApiLabel(version: string): string` (di `OpenApiBadge.svelte` atau modul kecil di `shared/`) → `"Swagger 2.0"` bila `version === "2.0"`, selain itu `"OpenAPI " + version`; kunci i18n baru `apispec.swaggerVersion` (id: "Swagger", en: "Swagger") dipakai `aria-label` lencana untuk 2.0.

- [ ] **Step 1: Tulis tes gagal:** `getSpec memetakan dokumen 2.0 sebelum dikembalikan` (respons mock `swagger: "2.0"` → `document.openapi`/`components.schemas` ada, `spec.openapi_version` tetap `"2.0"`); `getSpec membiarkan dokumen 3.x identik`; `OpenApiBadge menampilkan "2.0" dengan aria-label Swagger`; `ResultTable menulis "Swagger 2.0" untuk versi 2.0 dan "OpenAPI 3.0.3" untuk 3.x`; `SpecDetailView menampilkan daftar endpoint dari spec 2.0 tanpa error zod` (memakai `detail-swagger2.ts`). Balik tes upload yang menolak 2.0 menjadi `created`.
- [ ] **Step 2: Jalankan** `cd frontend && npx vitest run src/domain/project/api-spec src/shared` → FAIL pada tes baru.
- [ ] **Step 3: Implementasikan** perubahan di atas. Zod tidak boleh melempar untuk dokumen 2.0 yang valid; pemetaan terjadi setelah parse zod dan sebelum disimpan di store.
- [ ] **Step 4: Jalankan** `npx vitest run`, `npm run check`, dan `npm run build` → PASS tanpa peringatan baru.
- [ ] **Step 5: Commit** `feat(project-api-spec): UI membaca Swagger 2.0 (skema, adapter, lencana, teks hasil upload)`.

### Task 10: Fixture e2e, dokumen tiket, dan uji nyata

**Files:**
- Modify: `frontend/e2e/fixtures/apispec/swagger-2.0.yaml` (jadikan petstore 2.0 valid) dan spec e2e yang mengharapkan penolakan 2.0 (cari `swagger-2.0` di `frontend/e2e`); `docs/tickets/T06-project-api-spec.md` (tambahkan catatan revisi di bagian non-goal: Swagger 2.0 kini didukung, rujuk spec ini); `docs/superpowers/plans/project-api-spec/*` tidak ditulis ulang

**Interfaces:** tidak ada.

- [ ] **Step 1: Ubah e2e** agar unggah `swagger-2.0.yaml` → `created` dan detailnya terbuka (daftar endpoint + satu operasi dengan body).
- [ ] **Step 2: Jalankan** `make dev` (DB dev), `make seed`, lalu `cd frontend && npx playwright test -g "api-spec"` terhadap backend dev → PASS.
- [ ] **Step 3: Uji nyata:** unggah `/Users/ascendiz/Documents/Projects/ascendiz/saas/hr-platform/api/openapi.yml` ke satu project lewat UI; laporkan hasilnya apa adanya (lolos/gagal, jumlah endpoint, apakah detail satu operasi `POST` terbuka, tangkapan layar bila bisa). Jika gagal karena batas, ikuti gerbang Task 5.
- [ ] **Step 4: Commit** `test(project-api-spec): e2e dan dokumen untuk dukungan Swagger 2.0`.

### Task 11: Konsumen MCP (branch terpisah; tidak dikerjakan di `main`)

Tool MCP `list_api_specs` dan `get_api_endpoint` belum ada di `main`; mereka dibangun di branch/worktree `feat/discovery-api-spec-tools` (`worktree/feat-discovery-api-spec-tools`). Setelah branch itu digabung (atau di dalamnya, atas keputusan pemilik):

- [ ] Teks daftar spec menulis `Swagger 2.0` bila `openapi_version == "2.0"` dan `OpenAPI x.y.z` selain itu (satu fungsi format bersama); keluaran `get_api_endpoint` menyertakan `openapi_version: "2.0"`, `consumes`, `produces` hanya bila `Response.OpenAPIVersion != ""`; tes yang ada untuk 3.x tidak berubah (`OpenAPI 3.0.3 | N endpoint` tetap persis).
- [ ] Commit di branch itu: `feat(discovery-api-spec-tools): dukung keluaran Swagger 2.0`.

---

## Self-review

- **Cakupan spec:** tujuan 1 → Task 1–3, 7; tujuan 2 → Task 4, 7, 9; tujuan 3 → Task 8–9; tujuan 4 → Task 6, 11; tujuan 5 → tes 3.x tak berubah di Task 1, 6, 7, 9. Keputusan 1 (versi `"2.0"`) → Task 1; 2 → Task 1; 3, 12 → Task 7 (kode tetap); 4 → Task 2, 4; 5 → Task 6, 8; 6 → Task 6; 7 → Task 9, 11; 8 → Task 8; 9 → Task 2, 3, 6; 10 → Task 6; 11 → Task 1; 13 → Task 9, 11. Kriteria penerimaan 1–6 → Task 7, 9; 7 → Task 5, 10. Batas parser → Global Constraints + gerbang Task 5.
- **Konsistensi tipe:** `Swagger2Version`, `IsSwagger2`, `methodsFor`, `validateSwagger2`, `Resolved.OpenAPIVersion/Consumes/Produces` dan `getendpoint.Response` dengan nama sama; FE: `isSwagger2`, `adaptSwagger2`, `openApiLabel`.
- **Review Focus → tes:** 1 → Task 8 (`body tanpa consumes`) dan Task 6 (`EffectiveConsumesProduces`); 2 → Task 3 (`ExamplesAreLiteral`), Task 8; 3 → Task 3 (`CircularOK`), Task 6 (`Circular`), Task 8; 4 → Task 6 (`ServersNoHost/NoSchemes`), Task 8; 5 → Task 1 (`BothKeysIs3x`) dan fixture dengan `swagger` di `x-*` di Task 1.
