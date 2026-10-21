# T10 project/api-spec (Swagger 2.0) — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T10-project-api-spec-swagger2.md`
Spec: `docs/superpowers/specs/2026-10-21-swagger2-support-design.md`
Plan lama (referensi detail, nama simbol konsisten): `docs/superpowers/plans/2026-10-21-swagger2-support.md`
Sumber aturan: `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog audit), `.claude/rules/migrations.md`, `rules.txt`.
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/` hanya berisi `migrations.md`. T06 (`0a26586`) dan T09 (`741a85c`) sudah ada di `main`. Karena itu Task 11 di plan lama (konsumen MCP) TIDAK lagi ditunda ke branch lain dan dikerjakan di usecase `getendpoint` T10.

Dokumen ini adalah plan FOUNDATION untuk T10. Isinya: keputusan bersama, perubahan `base/openapi` yang dipakai kedua usecase, perubahan kontrak bus, migrasi, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-api-spec-swagger2/usecase-<slug>.md`, dan plan frontend di `docs/superpowers/plans/project-api-spec-swagger2/frontend-plan.md`. Bila plan usecase bertentangan dengan dokumen ini, dokumen ini yang menang sampai diperbarui.

Prinsip pemilik: **"kalau 2.0 pakai 2.0, kalau 3.0 pakai 3.0."** File yang tersimpan dan diunduh = byte yang diunggah. Backend tidak mengonversi apa pun.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T10 |
| Scope | fullstack (backend di plan ini dan plan usecase; frontend di `frontend-plan.md`) |
| Domain | project (lintas ke discovery/api-spec-tools hanya untuk teks MCP) |
| Module | api-spec (folder Go `apispec`) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Versi kontrak REST | tanpa prefix versi (prefix `/api`). Bentuk respons tidak berubah; hanya contoh dan deskripsi di `api/modules/project/api-spec.yml` |
| Versi kontrak bus | v1, perubahan aditif (field opsional di `getendpoint.Response`) |
| Bergantung pada | T06, T09 (keduanya merged) |
| Feature folder baru | tidak ada |

Endpoint yang perilakunya berubah (tidak ada endpoint baru):

| Endpoint | Method | Version | Usecase T10 | Perubahan |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-specs/upload` | POST (multipart) | - | `uploadspecs` | `swagger: "2.0"` diterima (`created/updated/unchanged`), `openapi_version: "2.0"` |
| `/api/projects/{projectID}/api-specs/validate` | POST (multipart) | - | `uploadspecs` | 2.0 valid -> `valid` |
| `/api/projects/{projectID}/api-specs/{name}` | GET | - | `uploadspecs` | `document` berisi dokumen 2.0 apa adanya |
| `/api/projects/{projectID}/api-specs/{name}/raw` | GET | - | `uploadspecs` | byte identik (tanpa perubahan kode) |
| bus `project.apispec.get-endpoint` | Call | v1 | `getendpoint` | field amplop `OpenAPIVersion`, `Consumes`, `Produces` |
| MCP `list_api_specs` / `get_api_endpoint` | tool | - | `getendpoint` | teks "Swagger 2.0"; `openapi_version/consumes/produces` hanya untuk 2.0 |
| bus `project.apispec.list-specs` / `list-endpoints` | Call | v1 | (tanpa kode) | endpoint 2.0 ikut muncul karena `ExtractEndpoints` sudah tahu 2.0 |

---

## 2. Summary Plan

1. **Foundation (`base/openapi`, murni CPU, tanpa I/O):** spike ukur file hr-platform (gerbang), parser menerima `swagger` 2.0 dan menyimpan versi `"2.0"`, validator struktur 2.0 baru, pemeriksa `$ref` sadar 2.0, `methodsFor` untuk ekstraksi/pencarian operasi, dan seluruh fixture 2.0.
2. **Usecase `uploadspecs`** (jalur REST upload/validate/detail/unduh): tanpa perubahan kode produksi; balik tes penolakan, tambah tes integration, perbarui kontrak OpenAPI dan fixture e2e, catatan revisi dokumen, uji nyata hr-platform.
3. **Usecase `getendpoint`** (bus + MCP): cabang 2.0 di `ResolveOperation`, field baru di `Resolved` dan `getendpoint.Response`, penyalinan di usecase, label versi dan field 2.0 di `apispectools`.
4. **Frontend** (plan terpisah): adapter `adaptSwagger2` di `getspec/model`, zod, badge, teks hasil upload, i18n.

Yang TIDAK ada: feature folder baru, endpoint bus baru, konstanta bus baru di `lib/constants/bus.go`, migrasi, aksi audit baru, perubahan `limits.go` atau konfigurasi `apispec_max_file_bytes`.

Keluaran 3.x (urutan endpoint, pesan issue, bentuk `get-endpoint`, teks MCP) tidak berubah satu byte pun. Seluruh tes 3.0/3.1 lama lulus tanpa diubah, kecuali baris yang mengunci penolakan 2.0 (dibalik).

---

## 3. Keputusan bersama (mengikat kedua usecase dan frontend)

| ID | Keputusan |
|---|---|
| D1 | Versi tersimpan untuk Swagger 2.0 = string persis `"2.0"` (`openapi.Swagger2Version`) di `api_specs.openapi_version` (kolom `text NOT NULL`, tanpa CHECK). Tanpa migrasi. |
| D2 | Deteksi hanya di kunci root. Ada `openapi` -> jalur 3.x lama tanpa perubahan, walau `swagger` juga ada. Kunci `swagger` di dalam `info` atau `x-*` tidak berpengaruh. |
| D3 | Bentuk `swagger` yang diterima: string `"2.0"`, YAML `2.0` (float64), JSON `2`/`2.0` dan YAML `2` (`json.Number`). Semua disimpan `"2.0"`. `swagger: null` -> `NOT_OPENAPI` pointer `#`. Nilai lain (`"2.0.0"`, `"1.2"`, `"3.0"`, array, map) -> `UNSUPPORTED_VERSION`, pesan `versi Swagger "<x>" tidak didukung (hanya 2.0)`, pointer `#/swagger`. |
| D4 | `CodeNotOpenAPI3` / `NOT_OPENAPI_3` tetap didefinisikan di `issue.go`, kontrak API, dan i18n (K10), tetapi tidak lagi dihasilkan parser. |
| D5 | Method 2.0 yang diindeks (`swagger2Methods`): get, put, post, delete, options, head, patch. `trace` di 2.0 diabaikan (tidak error, tidak diindeks). Tidak ada `webhooks`. |
| D6 | `$ref` bukan `#...` -> `EXTERNAL_REF` tanpa fetch. `$ref` internal tak ditemukan -> `UNRESOLVED_REF`. Keduanya berpointer + baris. Resolver dan inliner 3.x dipakai ulang (`ResolvePointer`, `maxInlineExpansions`, `$circular`, pemotongan bertahap `maxBytes`). |
| D7 | `allOf`, `discriminator`, `collectionFormat`, `x-nullable` tidak divalidasi semantik (K7); lewat apa adanya. |
| D8 | Batas parser (`MaxNodes 500000`, `MaxAliases 100`, `MaxDepth 64`, file 5 MiB, `maxInlineExpansions 20000`) tidak diubah tanpa persetujuan eksplisit pemilik. Lihat gerbang F0. |
| D9 | Label versi tunggal: backend MCP `SpecVersionLabel(v)`, FE `openApiLabel(v)`. Teks: `"Swagger 2.0"` bila `v == "2.0"`, selain itu `"OpenAPI " + v`. Backend: `v == ""` -> `"OpenAPI -"` agar keluaran lama identik. |
| D10 | `servers` 2.0 (K3): per scheme `"<scheme>://<host><basePath>"`; `schemes` kosong -> `["https"]`; `host` kosong -> `[{"url": "<basePath>"}]`; `basePath` kosong -> `"/"`. Aturan sama di backend dan adapter FE. |
| D11 | Penanda versi di `get-endpoint` (K8) hanya di amplop (`Response.OpenAPIVersion`, `Consumes`, `Produces`), bukan di dalam `Operation`. Untuk 3.x: `""`, `nil`, `nil`. |
| D12 | Handler bus T06 tetap tidak memeriksa akses user dan selalu membatasi query pada `ProjectID` request. |
| D13 | Tidak ada permintaan jaringan keluar saat memproses spec (sudah dites; T10 menambah kasus 2.0). |

Keputusan pembangun K1-K12 di tiket masih berstatus "belum disetujui pemilik" (pertanyaan terbuka 1). Plan ini mengikuti K1-K12 apa adanya. Bila pemilik mengubah salah satunya, dokumen ini diperbarui lebih dulu.

---

## 4. Plan (urutan implementasi)

Semua path relatif ke `backend/internal/domain/project/internal/apispec/base/openapi/` kecuali disebut lain. Semua langkah foundation tidak menambah import baru ke paket (tes `imports_test.go` tetap hijau: tanpa I/O, tanpa net).

### F0. Spike pengukuran (gerbang, memblokir semua langkah lain)

File sementara `zz_spike_test.go` (TIDAK di-commit), `t.Skip` bila env `SPIKE_SPEC` kosong.
Target: `/Users/ascendiz/Documents/Projects/ascendiz/saas/hr-platform/api/openapi.yml`.

- **Tahap a (sebelum F1):** jalankan guard YAML (`yamlguard.go`) dan `Parse` versi sekarang dengan `DefaultLimits`. Catat: ukuran byte, jumlah node, jumlah alias, kedalaman maksimum, waktu parse, kode issue pertama. Hasil `NOT_OPENAPI_3` = guard lolos (normal untuk tahap a). `YAML_LIMIT` / `FILE_TOO_LARGE` = batas terlampaui.
- **Tahap b (setelah F1-F4):** `Parse` + `ExtractEndpoints` + `ResolveOperation` untuk 3 operasi `POST` terbesar dengan `getendpoint.DefaultSchemaDepth` / `DefaultMaxBytes`. Catat: jumlah issue (+ kode), jumlah endpoint, waktu, ukuran hasil, `Truncated`, apakah budget `maxInlineExpansions` habis. Tahap b memakai `ResolveOperation` versi foundation (sebelum cabang 2.0 usecase `getendpoint`): bila cabang 2.0 belum ada, catat waktu dan ukuran setelah usecase `getendpoint` selesai, lalu ulangi tahap b.
- Perintah: `cd backend && SPIKE_SPEC=/Users/ascendiz/Documents/Projects/ascendiz/saas/hr-platform/api/openapi.yml go test ./internal/domain/project/internal/apispec/base/openapi/ -run TestSpikeHRPlatform -v`
- **Aturan berhenti:** bila ada batas D8 terlampaui atau parse > 2 detik -> BERHENTI, laporkan angka ukur + usulan angka baru + alasan ke pemilik, tunggu persetujuan eksplisit. Jangan ubah konstanta. Issue validasi lain (bukan batas) -> perbaiki cabang 2.0 di langkah asal (F2/F3), bukan batas, lalu ulangi.
- Setelah selesai: hapus `zz_spike_test.go`. Tidak ada commit. Angka dicatat di laporan usecase `uploadspecs` (kriteria 7).

### F1. Parser menerima `swagger` 2.0 (`parse.go`)

- Tambah `const Swagger2Version = "2.0"` dan `func (d Document) IsSwagger2() bool { return d.OpenAPIVersion == Swagger2Version }`.
- Ganti cabang `if _, sw := root.Get("swagger"); sw { ... CodeNotOpenAPI3 ... }` di `Parse`:
  - `swagger` ada dan nilainya `nil` -> `compose(CodeNotOpenAPI, "bukan dokumen OpenAPI", "#", 1)`.
  - `swaggerVersion(v any) (string, bool)`: `string "2.0"`, `float64 2.0`, `json.Number` bernilai `"2"`/`"2.0"` -> `("2.0", true)`; selain itu `(shown, false)` dengan `shown` dibentuk seperti cabang `openapi` (string, `json.Number.String()`, `strconv.FormatFloat`, lainnya `"?"`).
  - `false` -> `compose(CodeUnsupportedVersion, "versi Swagger "+strconv.Quote(truncRunes(shown, 40))+" tidak didukung (hanya 2.0)", "#/swagger", lineOfPointer(content, format, node, "#/swagger"))`.
  - `true` -> `doc := Document{OpenAPIVersion: Swagger2Version, Root: root, node: node}`; `raws := validateSwagger2(doc)`; `raws = append(raws, checkRefs(doc)...)`; lalu blok komposisi issue dan pengisian `Title/APIVersion` dari `info` yang SAMA dengan 3.x (refaktor kecil: ekstrak ekor `Parse` ke fungsi privat `finish(doc, raws, content)` agar tidak duplikat; perilaku 3.x identik).
- Jalur `openapi` tidak disentuh (D2).
- Commit: `feat(project-api-spec): parser menerima Swagger 2.0 dan menyimpan versi "2.0"`.

### F2. Validator struktur 2.0 (`validate_swagger2.go`, baru)

- `func validateSwagger2(doc Document) []rawIssue`, memakai receiver `validator` yang sama dengan `validate.go` (`add/bad`, `maxRawIssues`), kode `CodeInvalidStructure` dan `CodeDuplicateOpID`.
- `var swagger2Methods = []string{"get", "put", "post", "delete", "options", "head", "patch"}` (didefinisikan di file ini).
- Aturan:
  - `info` object; `info.title` dan `info.version` string.
  - `paths` wajib object; tiap kunci diawali `/` (kunci `x-` dilewati); path item object.
  - Method yang diperiksa = `swagger2Methods`; `trace` dan kunci tak dikenal dilewati.
  - Operasi: `operationId` string, unik (duplikat: issue di kemunculan kedua, pesan merujuk pointer pertama, sama seperti 3.x); `tags` array string; `deprecated` bool; `responses` wajib object, kode cocok `responseCodeRe` (kunci `x-` dilewati).
  - `parameters` (path-level dan operasi) array; item `$ref` dilewati; selain itu wajib `name` string dan `in` ∈ {query, header, path, formData, body}; `in: path` wajib `required: true`; `in: body` wajib `schema` object; maksimal satu body per operasi (gabungan path-level + operasi, override per `(name, in)` sama seperti `mergedOperation`); `body` dan `formData` tidak boleh bersamaan.
  - Root: `definitions`, `parameters`, `responses`, `securityDefinitions` bila ada object; `security` (root/operasi) array object; `consumes`/`produces` (root/operasi) array string; `host` string; `basePath` string diawali `/`; `schemes` array ⊆ {http, https, ws, wss}.
  - D7: `allOf`, `discriminator`, `collectionFormat`, `x-nullable` tidak divalidasi semantik.
- Helper dipakai ulang: `asMap`, `mustGet`, `joinPtr`, `isStr`, `isBool`, `quoteShort`, `responseCodeRe`. Boleh berbagi helper array dengan `validate.go` tanpa mengubah perilaku 3.x.
- Commit: `feat(project-api-spec): validasi struktur Swagger 2.0`.

### F3. Pemeriksa `$ref` sadar 2.0 (`refs.go`)

- `refChecker` mendapat field `swagger2 bool`, diisi `doc.IsSwagger2()` di `checkRefs`.
- Di `walk`: bila `swagger2` dan `!names && k == "examples"` -> seluruh nilai dilewati (peta MIME -> literal), tidak memanggil `r.examples`. Untuk 3.x cabang lama tetap.
- Name-map 2.0: bila `swagger2`, kunci `securityDefinitions` dan `scopes` diperlakukan sebagai name-map (`r.walk(v, child, true)`) di samping `nameMapKeys`. Variabel global `nameMapKeys` TIDAK diubah (3.x identik). `definitions`, `parameters`, `responses` root sudah aman karena kunci namanya tidak bernama `$ref`; bila ada kasus kunci definisi bernama `$ref` di fixture, perlakukan juga sebagai name-map hanya bila `swagger2`.
- `ResolvePointer` dipakai apa adanya untuk `#/definitions/`, `#/parameters/`, `#/responses/`.
- Commit: `feat(project-api-spec): pemeriksaan $ref untuk Swagger 2.0`.

### F4. Ekstraksi endpoint dan pencarian operasi (`extract.go`, `resolve.go`, `validate.go`)

- `func methodsFor(doc Document) []string` (di `validate.go` di samping `opMethods`): `doc.IsSwagger2()` -> `swagger2Methods`, selain itu `opMethods`.
- `ExtractEndpoints` (loop `for _, m := range opMethods`, extract.go:50) dan `FindOperation` (resolve.go:40, :59) memakai `methodsFor(doc)`. Urutan keluaran tetap `base.SortEndpoints`.
- Pemeriksaan method di `ResolveOperation` (resolve.go:238) TIDAK diubah di foundation; diubah oleh usecase `getendpoint` bersama cabang 2.0.
- Commit: `feat(project-api-spec): ekstraksi endpoint dan FindOperation untuk Swagger 2.0`.

### F5. Fixture `testdata/` (dibuat di foundation, dipakai kedua usecase)

| File | Isi |
|---|---|
| `swagger-2.0.yaml` | DIUBAH: dari "dokumen yang ditolak" menjadi petstore 2.0 valid (sama isinya dengan `swagger-2.0-petstore.yaml`) |
| `swagger-2.0-petstore.yaml` | petstore 2.0: `/pets` GET/POST, `/pets/{petId}` GET/DELETE, `definitions` Pet/Error, satu operasi `deprecated`, tags |
| `swagger-2.0-petstore.json` | petstore yang sama dalam JSON, `"swagger": "2.0"` |
| `swagger-2.0-numeric.yaml` | `swagger: 2.0` tanpa kutip |
| `swagger-1.2.yaml` | `swagger: "1.2"` |
| `swagger-null.yaml` | `swagger: null` |
| `both-keys.yaml` | `openapi: 3.0.3` + `swagger: "2.0"` di root |
| `swagger-in-info.yaml` | `openapi: 3.0.3` dengan `info.swagger: "2.0"` dan `info.x-swagger: "2.0"` |
| `swagger-2.0-unresolved-ref.yaml` | `$ref: '#/definitions/Hilang'` |
| `swagger-2.0-dup-opid.yaml` | dua operasi `operationId: listPets` |
| `swagger-2.0-bad-params.yaml` | parameter tanpa `in`, `in: cookie`, `in: path` tanpa `required: true`, dua `in: body` |
| `swagger-2.0-ref-http.yaml` | `$ref: 'http://127.0.0.1:<port>/x.yaml#/Pet'` (port diganti tes integration dengan listener lokal; untuk unit cukup host statis) |
| `swagger-2.0-ref-file.yaml` | `$ref: 'other.yaml#/definitions/Pet'` |
| `swagger-2.0-examples-ref-key.yaml` | respons `examples: {application/json: {"$ref": "#/nope"}}` |
| `swagger-2.0-circular.yaml` | `Node.children[] -> Node` |
| `swagger-2.0-full.yaml` | body tanpa `consumes`, formData + `type: file` (operasi lain), header/query/path, respons 204 tanpa schema, respons dengan `examples`, `x-nullable`, `allOf` + `discriminator`, `collectionFormat`, `securityDefinitions` basic/apiKey/oauth2, `host` + `basePath` + `schemes: [http, https]`, parameter `$ref` tingkat path, operasi `trace` |

Commit fixture bersama langkah yang pertama memakainya (F1 untuk fixture versi, F2 untuk bad-params/dup-opid, F3 untuk ref/examples/circular, F4 untuk full).

### Setelah foundation

- Usecase `uploadspecs` dan `getendpoint` dapat dikerjakan paralel (tidak berbagi file produksi; berbagi hanya fixture F5). Frontend dapat dikerjakan paralel setelah F1 (kontrak REST tidak berubah bentuk).
- Verifikasi akhir T10:
  - `cd backend && go test ./...`
  - `go test -tags integration ./internal/domain/project/internal/apispec/...` dengan `TEST_DATABASE_URL` ke DB test (`make dev-test-db`), BUKAN DB dev.
  - `go vet ./...`; tes arsitektur T00 (`internal/archtest`) lulus.
  - `make api-bundle` lalu `make api-check`.

---

## 5. Kontrak bus dan konstanta

- `lib/constants/bus.go`: **tidak berubah.** Konstanta yang dipakai sudah ada: `BUS_PROJECT_APISPEC_LIST_SPECS`, `BUS_PROJECT_APISPEC_LIST_ENDPOINTS`, `BUS_PROJECT_APISPEC_GET_ENDPOINT`.
- `lib/contracts/project/apispec/listspecs`, `listendpoints`: **tidak berubah.** `OpenAPIVersion` spec 2.0 berisi `"2.0"` lewat jalur data yang sudah ada.
- `lib/contracts/project/apispec/getendpoint/getendpoint.go`: perubahan aditif (dikerjakan usecase `getendpoint`, bentuknya ditetapkan di sini):

```go
type Response struct {
	Found              bool
	Reason             string
	Specs              []string
	Similar            []EndpointRef
	Spec, Method, Path string
	Operation          json.RawMessage
	Servers            json.RawMessage // array
	SecuritySchemes    json.RawMessage // object
	Truncated          bool
	EffectiveDepth     int
	Hint               string
	// Penanda versi (K8). "2.0" hanya untuk Swagger 2.0; "" untuk OpenAPI 3.x.
	OpenAPIVersion string
	// Array JSON consumes/produces efektif (operasi menimpa root; tidak ada keduanya -> []).
	// Hanya untuk Swagger 2.0; nil untuk OpenAPI 3.x.
	Consumes json.RawMessage
	Produces json.RawMessage
}
```

  Doc comment paket ditambah paragraf: untuk Swagger 2.0, `Servers` dibentuk dari `schemes` x `host` + `basePath` (D10), `SecuritySchemes` dari `securityDefinitions` yang disebut `security` efektif, dan `Operation` adalah operasi 2.0 ter-inline apa adanya (tidak dinormalisasi ke 3.x).
- Katalog endpoint bus di `docs/tickets/README.md`: tidak ada baris baru. Paragraf kontrak `project.apispec.*` boleh ditambah satu kalimat tentang field penanda versi (dikerjakan usecase `getendpoint`).
- Aksi audit: tidak ada yang baru. `Summary` audit MCP `mcp.get_api_endpoint` tidak berubah.

---

## 6. Migration

Tidak ada. Kolom `api_specs.openapi_version` sudah `text NOT NULL` tanpa CHECK; nilai `"2.0"` disimpan apa adanya.

---

## 7. Changes (ringkasan per file/folder)

Foundation (dokumen ini):

| File | Perubahan |
|---|---|
| `base/openapi/parse.go` | `Swagger2Version`, `IsSwagger2`, `swaggerVersion`, cabang 2.0, ekstrak `finish` (perilaku 3.x identik) |
| `base/openapi/validate_swagger2.go` | baru: `swagger2Methods`, `validateSwagger2` |
| `base/openapi/validate.go` | `methodsFor` |
| `base/openapi/refs.go` | `refChecker.swagger2`, `examples` dilewati, name-map 2.0 |
| `base/openapi/extract.go` | `methodsFor(doc)` |
| `base/openapi/resolve.go` | `FindOperation` memakai `methodsFor(doc)` |
| `base/openapi/issue.go` | tidak berubah (`CodeNotOpenAPI3` tetap) |
| `base/openapi/limits.go`, `yamlguard.go` | tidak berubah |
| `base/openapi/testdata/*` | fixture F5 |
| `base/openapi/parse_test.go`, `validate_swagger2_test.go` (baru), `resolve_test.go`, `external_ref_test.go`, `extract_test.go` atau `resolve_test.go` | tes foundation (bagian 8) |

Usecase `uploadspecs` (plan terpisah): tes `uploadspecs/usecase_test.go`, `validatespec/usecase_test.go`, `apispec_integration_test.go`; `api/modules/project/api-spec.yml`, `api/openapi.yml` (bundle); `frontend/e2e/fixtures/apispec/swagger-2.0.yaml`; catatan revisi di `docs/tickets/T06-project-api-spec.md` dan `docs/superpowers/plans/project-api-spec/overview.md`.

Usecase `getendpoint` (plan terpisah): `base/openapi/resolve.go` (`Resolved`, cabang 2.0, `swagger2Servers`, security efektif, consumes/produces efektif, `size()`), `lib/contracts/project/apispec/getendpoint/getendpoint.go`, `getendpoint/usecase.go`, `discovery/internal/apispectools/base/{format.go,catalog.go,detail.go}` + tes.

Frontend (plan terpisah): `getspec/model/swagger2.ts`, `getspec/types.ts`, `getspec/services/getspec.service.ts`, `shared/components/OpenApiBadge.svelte`, `uploadspecs/components/ResultTable.svelte`, `shared/i18n/{id,en}.json`, tes uploadspecs yang dibalik.

---

## 8. Test Scenario

### Unit (foundation, `go test ./internal/domain/project/internal/apispec/base/openapi/`)

`parse_test.go`:
- Baris tabel lama `{"swagger-2.0.yaml", CodeNotOpenAPI3}` dibalik menjadi `{"swagger-1.2.yaml", CodeUnsupportedVersion}`.
- `TestParse_Swagger2Accepted`: `swagger-2.0.yaml`, `swagger-2.0-petstore.yaml`, `swagger-2.0-petstore.json` -> tanpa issue, `OpenAPIVersion == "2.0"`, `IsSwagger2()`, `Title`/`APIVersion` dari `info`.
- `TestParse_Swagger2VersionForms` (tabel, konten inline): YAML `"2.0"`, YAML `2.0`, YAML `2`, JSON `2`, JSON `2.0` -> `"2.0"`; `"2.0.0"`, `"1.2"`, `"3.0"`, `[2]`, `{a: 1}` -> `CodeUnsupportedVersion`, pointer `#/swagger`, line > 0, pesan `versi Swagger "2.0.0" tidak didukung (hanya 2.0)`; `swagger: null` -> `CodeNotOpenAPI` pointer `#`.
- `TestParse_BothKeysIs3x`: `both-keys.yaml` -> `OpenAPIVersion == "3.0.3"`, `!IsSwagger2()`.
- `TestParse_SwaggerKeyInsideInfoIs3x`: `swagger-in-info.yaml` -> `"3.0.3"`.
- `TestParse_NotOpenAPI3NeverEmitted`: semua fixture `testdata/` tidak menghasilkan `CodeNotOpenAPI3`.

`validate_swagger2_test.go` (semua lewat `Parse`):
- `TestSwagger2_Petstore_OK`, `TestSwagger2_Full_OK` (`swagger-2.0-full.yaml` tanpa issue; `allOf`/`discriminator`/`collectionFormat`/`x-nullable` lolos).
- `TestSwagger2_MissingInfoTitle` (pointer `#/info/title`), `TestSwagger2_PathsNotObject`, `TestSwagger2_PathWithoutSlash`.
- `TestSwagger2_BadParams` (`swagger-2.0-bad-params.yaml`): issue `INVALID_STRUCTURE` untuk parameter tanpa `in`, `in: cookie`, path tidak required, dua body; semua berpointer `#/paths/.../parameters/<i>` dan line > 0.
- `TestSwagger2_BodyAndFormData`, `TestSwagger2_BodyWithoutSchema`, `TestSwagger2_PathLevelBodyPlusOperationBody` (dua body gabungan -> issue).
- `TestSwagger2_DuplicateOperationID` (`CodeDuplicateOpID`, pointer + line).
- `TestSwagger2_MissingResponses`, `TestSwagger2_BadResponseCode`.
- `TestSwagger2_BadBasePath` (`basePath: v1`), `TestSwagger2_BadScheme` (`schemes: [ftp]`), `TestSwagger2_ConsumesNotStringArray`, `TestSwagger2_SecurityNotArray`.
- `TestSwagger2_TraceIgnored` (operasi `trace` dengan isi rusak -> tanpa issue).
- `TestSwagger2_MaxRawIssues` (banyak error -> dipotong sama seperti 3.x).

refs (`resolve_test.go` / `external_ref_test.go`):
- `TestRefs_Swagger2Unresolved`: `UNRESOLVED_REF`, pointer ke lokasi `$ref`, line > 0.
- `TestRefs_Swagger2External`: `swagger-2.0-ref-http.yaml` dan `swagger-2.0-ref-file.yaml` -> `EXTERNAL_REF` tanpa fetch (memakai pola uji tanpa jaringan yang sudah ada di `external_ref_test.go`).
- `TestRefs_Swagger2ExamplesAreLiteral`: `swagger-2.0-examples-ref-key.yaml` tanpa issue.
- `TestRefs_Swagger2SecurityDefinitionsNameMap`: `securityDefinitions` / `scopes` berkunci aneh (mis. `$ref`) tidak dianggap referensi.
- `TestRefs_Swagger2CircularOK`: `swagger-2.0-circular.yaml` tanpa issue.
- Kontrol 3.x: `ref-in-example.yaml` tetap menghasilkan issue yang sama seperti sebelumnya (tes lama tidak diubah).

extract/find:
- `TestExtractEndpoints_Swagger2`: petstore 2.0 -> method/path/operationId/tags/summary/deprecated benar; urutan sama dengan `ExtractEndpoints` petstore 3.0 untuk set path/method setara.
- `TestExtractEndpoints_Swagger2IgnoresTrace`: `swagger-2.0-full.yaml` tidak memuat `TRACE`.
- `TestFindOperation_Swagger2ByOperationID` dan `..._ByMethodPath`; `TestFindOperation_Swagger2TraceNotFound`.
- Seluruh tes 3.0/3.1 lama lulus tanpa diubah; `imports_test.go` hijau.

### Unit/integration per usecase

Dirinci di plan usecase. Ringkas:
- `uploadspecs`: tes usecase dibalik (2.0 -> `created`, `OpenAPIVersion "2.0"`; validate -> `valid`), integration created/unchanged/rejected, unduh byte identik, `EXTERNAL_REF` dengan listener lokal menghitung 0 request, GET detail berisi kunci `swagger`.
- `getendpoint`: `TestResolveOperation_Swagger2_*` (inline definitions, servers 2 scheme / tanpa host / tanpa schemes / tanpa basePath, security efektif termasuk `security: []`, consumes/produces efektif, circular, pemotongan), `TestResolveOperation_3xUnchanged`, `TestExecute_Swagger2SetsOpenAPIVersion`, tes `apispectools` (label, detail 2.0, keluaran 3.x identik, golden `tools_list.golden.json` tidak berubah).

---

## 9. E2E Scenarios

Positif:
1. Unggah `swagger-2.0-petstore.yaml` -> `created`, `openapi_version "2.0"`, `endpoint_count 4`; daftar spec menampilkan "Swagger 2.0"; detail membuka daftar endpoint dan satu operasi dengan body (fixture e2e `frontend/e2e/fixtures/apispec/swagger-2.0.yaml` yang dijadikan valid).
2. Unggah `swagger-2.0-petstore.json` (nama lain) -> `created`; unduh `/raw` = byte file asli.
3. Unggah ulang file yang sama -> `unchanged`.
4. Validate (`/validate`) file 2.0 valid -> `valid`, tidak tersimpan.
5. MCP `list_api_specs` menulis `| Swagger 2.0 |`; `get_api_endpoint` untuk `POST /pets` mengembalikan operasi ter-inline + `openapi_version: "2.0"`, `consumes`, `produces`, `servers` dari host/basePath/schemes.
6. Spec 3.0.3 yang ada: semua keluaran (upload, detail, MCP) identik dengan sebelum T10.
7. Uji nyata `hr-platform/api/openapi.yml`: hasil dilaporkan apa adanya.

Negatif:
1. `swagger-2.0-unresolved-ref.yaml` -> `rejected`, `UNRESOLVED_REF` dengan pointer + baris.
2. `swagger-2.0-dup-opid.yaml` -> `rejected`, `DUPLICATE_OPERATION_ID`.
3. `swagger-2.0-bad-params.yaml` -> `rejected`, beberapa `INVALID_STRUCTURE`.
4. `swagger-2.0-ref-http.yaml` / `swagger-2.0-ref-file.yaml` -> `rejected`, `EXTERNAL_REF`, 0 permintaan keluar.
5. `swagger-1.2.yaml` -> `rejected`, `UNSUPPORTED_VERSION`.
6. `swagger-null.yaml` -> `rejected`, `NOT_OPENAPI`.
7. File 2.0 > 5 MiB -> `rejected` `FILE_TOO_LARGE` (batas tidak berubah).
8. MCP `get_api_endpoint` method `TRACE` pada spec 2.0 -> `ENDPOINT_NOT_FOUND` dengan `Similar`.

---

## 10. Request/Response body

### Upload Swagger 2.0 (POST `/api/projects/{projectID}/api-specs/upload`, multipart, header `X-Requested-With: grimoire`)

Respons 200:

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

Teks `message`/`reason` persis mengikuti `compose` dan pesan resolver yang sudah ada; contoh di atas ilustratif untuk bentuk, plan usecase `uploadspecs` mengunci teks persisnya dari tes.

### Error request (tidak berubah)

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
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
    "endpoint_count": 4
  },
  "document": {
    "swagger": "2.0",
    "info": { "title": "Swagger Petstore", "version": "1.0.0" },
    "host": "petstore.example.com",
    "basePath": "/v1",
    "schemes": ["https"],
    "paths": { "/pets": { "get": { "operationId": "listPets", "responses": { "200": { "description": "ok", "schema": { "type": "array", "items": { "$ref": "#/definitions/Pet" } } } } } } },
    "definitions": { "Pet": { "type": "object", "required": ["id", "name"], "properties": { "id": { "type": "integer" }, "name": { "type": "string" } } } }
  },
  "endpoints": [
    { "method": "GET", "path": "/pets", "operation_id": "listPets", "summary": "List pets", "tags": ["pets"], "deprecated": false }
  ]
}
```

(Nama field `spec`/`document`/`endpoints` mengikuti DTO getspec yang ada; T10 tidak mengubah bentuknya.)

### Bus `project.apispec.get-endpoint` (Swagger 2.0)

Request:

```json
{ "ProjectID": "7d1f0c8e-2b4a-4c1e-9f3a-5e6d7c8b9a01", "Spec": "petstore", "Method": "POST", "Path": "/pets", "SchemaDepth": 4, "MaxBytes": 102400 }
```

Response:

```json
{
  "Found": true,
  "Spec": "petstore",
  "Method": "POST",
  "Path": "/pets",
  "Operation": {
    "operationId": "createPet",
    "parameters": [
      { "name": "body", "in": "body", "required": true, "schema": { "type": "object", "required": ["id", "name"], "properties": { "id": { "type": "integer" }, "name": { "type": "string" } } } }
    ],
    "responses": { "201": { "description": "created" } },
    "security": [{ "api_key": [] }]
  },
  "Servers": [{ "url": "https://petstore.example.com/v1" }],
  "SecuritySchemes": { "api_key": { "type": "apiKey", "name": "X-API-Key", "in": "header" } },
  "Truncated": false,
  "EffectiveDepth": 4,
  "OpenAPIVersion": "2.0",
  "Consumes": ["application/json"],
  "Produces": ["application/json"]
}
```

Untuk 3.x: `OpenAPIVersion` `""`, `Consumes`/`Produces` `nil` (bentuk dan isi lain identik sebelum T10).

### MCP `get_api_endpoint` (2.0), isi teks JSON

```json
{
  "spec": "petstore",
  "method": "POST",
  "path": "/pets",
  "schema_depth": 4,
  "truncated": false,
  "operation": { "operationId": "createPet", "parameters": [ { "name": "body", "in": "body", "required": true, "schema": { "type": "object" } } ], "responses": { "201": { "description": "created" } } },
  "servers": [{ "url": "https://petstore.example.com/v1" }],
  "securitySchemes": { "api_key": { "type": "apiKey", "name": "X-API-Key", "in": "header" } },
  "openapi_version": "2.0",
  "consumes": ["application/json"],
  "produces": ["application/json"]
}
```

Untuk 3.x tiga field terakhir tidak muncul (`omitempty`). Baris `list_api_specs`: `- petstore: Swagger Petstore | versi 1.0.0 | Swagger 2.0 | 4 endpoint | diperbarui ...`; 3.x tetap `| OpenAPI 3.0.3 |`.

### MCP error (tidak berubah)

`get_api_endpoint` operasi tidak ada -> hasil tool `isError: false` dengan teks `ENDPOINT_NOT_FOUND` + daftar `Similar` seperti T09.

---

## 11. Daftar usecase

| Slug | Plan | Ringkas |
|---|---|---|
| `uploadspecs` | `docs/superpowers/plans/project-api-spec-swagger2/usecase-uploadspecs.md` | Jalur REST 2.0 di atas foundation: upload, validate, detail (dokumen 2.0 apa adanya), unduh (byte identik); balik tes penolakan; tes integration; kontrak `api-spec.yml` + bundle + `api-check`; fixture e2e; catatan revisi T06; laporan uji nyata hr-platform. |
| `getendpoint` | `docs/superpowers/plans/project-api-spec-swagger2/usecase-getendpoint.md` | `ResolveOperation` cabang 2.0 (servers, security, consumes/produces, inline), field `Resolved` dan `getendpoint.Response` baru, penyalinan di usecase, `SpecVersionLabel` + detail 2.0 di `apispectools`; keluaran 3.x terkunci identik. |
| (frontend) | `docs/superpowers/plans/project-api-spec-swagger2/frontend-plan.md` | Adapter `adaptSwagger2`, zod, badge, ResultTable, i18n, tes FE dibalik. |

Pesan commit: `feat(project-api-spec): ...`, `feat(discovery-api-spec-tools): ...`, `test(project-api-spec): ...`.

---

## 12. Risiko

1. File hr-platform (3,5 MB, ~77 ribu baris, 2.481 `$ref` internal) dapat melampaui `MaxNodes` atau `maxInlineExpansions`. Mitigasi: gerbang F0; tidak ada perubahan batas tanpa persetujuan.
2. Refaktor ekor `Parse` (`finish`) bisa menggeser perilaku 3.x. Mitigasi: refaktor murni, semua tes 3.x lama tidak diubah.
3. Name-map 2.0 di `refs.go` salah cakupan bisa membuat `$ref` asli terlewat. Mitigasi: hanya `securityDefinitions` dan `scopes`, dan hanya bila `swagger2`; tes `Unresolved` di dalam `definitions` bersarang.
4. K1-K12 belum disetujui pemilik; perubahan keputusan memengaruhi D1-D11.
