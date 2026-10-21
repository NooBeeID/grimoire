# T10 project/api-spec (Swagger 2.0) — Usecase `getendpoint`

Overview (menang bila bertentangan sampai diperbarui): `docs/superpowers/plans/project-api-spec-swagger2/overview.md`
Tiket: `docs/tickets/T10-project-api-spec-swagger2.md`. Keputusan D1-D13 dan K1-K12 mengikat; plan ini hanya merinci bagian `getendpoint` + konsumen MCP-nya.

Prasyarat: foundation F1-F5 selesai (`openapi.Swagger2Version`, `Document.IsSwagger2()`, `swagger2Methods`, `methodsFor(doc)`, fixture `testdata/swagger-2.0-*.yaml`). Usecase ini tidak berbagi file produksi dengan `uploadspecs`.

Aturan arsitektur yang dipatuhi: `getendpoint` tetap hanya orkestrasi (cari spec -> parse -> FindOperation -> ResolveOperation -> salin ke respons). Logika 2.0 (rule bisnis murni) ada di `base/openapi` (paket tanpa I/O). Teks MCP ada di `discovery/.../apispectools/base`. Interaksi project <-> discovery hanya lewat bus `project.apispec.get-endpoint` yang sudah ada; tidak ada import lintas domain baru. Repository tidak disentuh (tanpa SQL baru).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T10 |
| Scope | backend (konsumen MCP ikut di dalam usecase ini) |
| Domain / Module | project / api-spec (lintas ke discovery / api-spec-tools hanya teks MCP) |
| Usecase | `getendpoint` (folder `backend/internal/domain/project/internal/apispec/getendpoint`) |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | bus `project.apispec.get-endpoint` (konstanta `BUS_PROJECT_APISPEC_GET_ENDPOINT`, tidak berubah) |
| Method | Call (bus, bukan HTTP) |
| Version | kontrak bus v1, perubahan aditif (3 field opsional di `Response`) |
| Konsumen | MCP `get_api_endpoint` dan `list_api_specs` (T09, `discovery/internal/apispectools`) |
| REST baru | tidak ada |
| Bergantung pada | foundation F1-F5, T06, T09 |

---

## 2. Summary Plan

1. Kunci dulu keluaran 3.x: sebelum mengubah kode, tulis tes karakterisasi (golden) `ResolveOperation` untuk 3.x dan jalankan hijau di kode lama.
2. `openapi.ResolveOperation` mendapat cabang 2.0: pemeriksaan method memakai `methodsFor(doc)`; `servers` dibentuk dari `schemes x host + basePath` (D10); `securitySchemes` dari `securityDefinitions` yang disebut `security` efektif; `consumes`/`produces` efektif; operasi di-inline dengan inliner, `maxInlineExpansions`, dan pemotongan bertahap yang SAMA. Struct `Resolved` bertambah `OpenAPIVersion`, `Consumes`, `Produces` (kosong untuk 3.x).
3. Kontrak bus `getendpoint.Response` bertambah `OpenAPIVersion`, `Consumes`, `Produces` (D11); usecase menyalinnya.
4. Konsumen MCP: `SpecVersionLabel` di `base/format.go` dipakai `catalog.go`; `detailJSON` menambah `openapi_version`, `consumes`, `produces` dengan `omitempty` hanya terisi untuk 2.0.
5. Tes mengunci: keluaran 3.x identik byte (resolve, usecase, catalog, detail, golden `tools_list.golden.json` tidak berubah).

Tidak ada: migrasi, konstanta bus baru, aksi audit baru, perubahan `limits.go`/`yamlguard.go`, perubahan `FindOperation` (sudah oleh foundation F4), perubahan skema JSON input tool MCP (golden `tools_list` tetap).

Putusan desain yang ditetapkan di sini (turunan D10/D11/K4):

- `Operation` 2.0 = operasi gabungan (parameter path-level digabung, override per `(name, in)`) dengan `$ref` internal di-inline. Bentuk 2.0 dibiarkan (`in: body` + `schema`, `consumes`/`produces` milik operasi tetap ada di dalamnya bila ditulis). Tidak dinormalisasi ke 3.x.
- Field `security` operasi tetap ada di `Operation` bila ada. `security` level root tidak disalin ke `Operation` (sama dengan 3.x); efeknya hanya memilih `SecuritySchemes`.
- `Consumes`/`Produces` amplop = array JSON efektif: bila operasi punya kunci `consumes` (walau `[]`) pakai itu; selain itu pakai root; tidak ada keduanya -> `[]`. Path item 2.0 tidak punya `consumes`; diabaikan.
- `Servers` 2.0: untuk tiap scheme (urut sesuai dokumen, duplikat dibuang) `{"url": "<scheme>://<host><basePath>"}`; `schemes` kosong/tidak ada -> `["https"]`; `host` kosong/tidak ada -> satu elemen `{"url": "<basePath>"}` (schemes diabaikan); `basePath` kosong/tidak ada -> `"/"`. Nilai bukan string diperlakukan kosong (validator F2 sudah menolak upload semacam itu; ini pertahanan untuk data lama). Elemen `schemes` bukan string dilewati.
- `OpenAPIVersion` amplop = `openapi.Swagger2Version` (`"2.0"`) hanya bila `doc.IsSwagger2()`; selain itu `""`.
- Pemeriksaan method: 2.0 memakai `swagger2Methods` (tanpa `trace`); `TRACE` pada spec 2.0 -> `ErrOperationNotFound` -> usecase mengembalikan `ENDPOINT_NOT_FOUND` + `Similar` (alur yang sudah ada).
- Ukuran untuk pemotongan (`size`) menjumlah `Operation + Servers + SecuritySchemes + Consumes + Produces`. Untuk 3.x dua yang terakhir nil sehingga nilainya sama dengan sekarang.

---

## 3. Plan (langkah berurutan)

Semua path relatif ke `backend/` kecuali disebut.

### G0. Karakterisasi 3.x (WAJIB sebelum G1)

File baru `internal/domain/project/internal/apispec/base/openapi/resolve_3x_test.go` + golden `testdata/resolve_3x.golden.json`.

- Tes `TestResolveOperation_3xUnchanged`: untuk kasus berikut panggil `ResolveOperation` lalu serialisasi `{"case":..., "operation":<raw>, "servers":<raw>, "securitySchemes":<raw>, "truncated":..., "effectiveDepth":...}` ke satu array JSON, bandingkan byte dengan golden.
  - `petstore-3.0.yaml` GET `/pets` depth 4 maxBytes 100000; depth 1; depth 0.
  - `nested-circular.yaml` GET `/nodes/{id}` depth 8 maxBytes 0 (circular + servers + security).
  - `nested-circular.yaml` maxBytes kecil (memicu pemotongan bertahap) dan maxBytes 10 (ringkasan `$truncated`).
  - `petstore-3.1.json` satu operasi.
  - YAML inline: `security: []` pada operasi, tanpa servers; servers per-operasi/path/root (pola `TestT06_AC5_ServersAndSecurity`).
- Flag `-update` (pola `base_test.go` di apispectools) untuk menulis golden. Generate golden SEKARANG di kode lama, periksa isinya, commit golden bersama tes (commit terpisah dari perubahan fungsi, agar diff-nya membuktikan golden tidak berubah).
- Tes tambahan di tempat yang sama: `Resolved` 3.x harus punya `OpenAPIVersion == ""`, `Consumes == nil`, `Produces == nil` (baris ini ditambahkan di G1 setelah field ada; sebelumnya tidak dapat dikompilasi).
- Commit: `test(project-api-spec): kunci keluaran ResolveOperation 3.x (golden)`.

### G1. `Resolved` + `ResolveOperation` cabang 2.0 (`base/openapi/resolve.go`)

1. `Resolved` menjadi:
   ```go
   type Resolved struct {
       Operation       json.RawMessage
       Servers         json.RawMessage // selalu array JSON
       SecuritySchemes json.RawMessage // selalu object JSON
       Truncated       bool
       EffectiveDepth  int
       // Khusus Swagger 2.0 (K8). Untuk 3.x: "", nil, nil.
       OpenAPIVersion string
       Consumes       json.RawMessage // array JSON
       Produces       json.RawMessage // array JSON
   }
   ```
2. Fungsi pembantu baru (privat, di `resolve.go`, tanpa import baru selain yang ada):
   - `swagger2Servers(root *Map) []any`: aturan D10 di atas; mengembalikan `[]any` berisi `*Map{"url": ...}` (urutan kunci tetap).
   - `effectiveStringList(op, root *Map, key string) []any`: `op` punya kunci `key` bertipe `[]any` -> itu; else root; else `[]any{}`. Hasil melewati `inline` (tanpa efek; hanya string) lalu `json.Marshal`.
   - `schemeDefs(doc Document) *Map`: 2.0 -> `securityDefinitions` root; 3.x -> `components.securitySchemes` (kode lama dipindah apa adanya, perilaku sama).
3. Ubah `ResolveOperation`:
   - `ms := methodsFor(doc)`; pemeriksaan `!hasString(opMethods, ...)` diganti `!hasString(ms, strings.ToLower(method))`. Untuk 3.x `ms == opMethods` sehingga identik.
   - Setelah `merged`: `if doc.IsSwagger2() { servers = swagger2Servers(root) } else { servers = firstServers(op, item, root) }`.
   - `securityNames(op, root)` dipakai apa adanya (`security` operasi menimpa root; `security: []` -> tanpa skema; bentuk `[{name: [scopes]}]` sama di 2.0).
   - `schemes` diambil lewat `schemeDefs(doc)`. Nama dari `security` yang tidak ada di definisi dilewati (perilaku 3.x).
   - Di `build(d)`: bila `doc.IsSwagger2()`, marshal `in.inline(consumesList, 0, nil)` dan `produces` yang sama, isi `Resolved.OpenAPIVersion/Consumes/Produces`. Untuk 3.x biarkan nol.
   - `size` menambah `len(r.Consumes)+len(r.Produces)`.
   - Cabang ringkasan `$truncated` (depth 0 tetap terlalu besar): `r := first` sudah membawa `OpenAPIVersion/Consumes/Produces`; hanya `Operation` yang diganti. Tidak ada perubahan lain.
   - Inliner (`inliner.inline`), `maxInlineExpansions`, `paramKey`, `mergedOperation` TIDAK diubah: `ResolvePointer` sudah menangani `#/definitions/..`, `#/parameters/..`, `#/responses/..`; `paramKey` menyelesaikan `$ref: '#/parameters/X'` untuk override name+in.
4. Jalankan G0 (golden harus lulus tanpa `-update`).
5. Commit: `feat(project-api-spec): ResolveOperation mendukung Swagger 2.0`.

### G2. Kontrak bus (`lib/contracts/project/apispec/getendpoint/getendpoint.go`)

- Tambah ke `Response` (bentuk persis ditetapkan overview bagian 5):
  ```go
  OpenAPIVersion string          // "2.0" hanya untuk Swagger 2.0; "" untuk OpenAPI 3.x
  Consumes       json.RawMessage // array JSON efektif; nil untuk 3.x
  Produces       json.RawMessage // array JSON efektif; nil untuk 3.x
  ```
- Doc comment paket ditambah paragraf: untuk Swagger 2.0, `Servers` dibentuk dari `schemes` x `host` + `basePath`; `SecuritySchemes` dari `securityDefinitions` yang disebut `security` efektif; `Operation` adalah operasi 2.0 ter-inline apa adanya (tidak dinormalisasi ke 3.x); `OpenAPIVersion/Consumes/Produces` kosong untuk 3.x. Konsumen yang mengabaikan field baru tetap benar (aditif).
- `docs/tickets/README.md` (paragraf kontrak `project.apispec.*`): tambah SATU kalimat tentang field penanda versi. Tidak ada baris baru di katalog bus. (Catatan: working tree README sudah termodifikasi; sunting dengan Edit kecil, jangan menimpa perubahan lain.)
- Commit bersama G3.

### G3. Usecase (`getendpoint/usecase.go`)

Di `Execute`, setelah `res := getendpoint.Response{...}` tambahkan penyalinan:
```go
res.OpenAPIVersion = r.OpenAPIVersion
res.Consumes = r.Consumes
res.Produces = r.Produces
if res.OpenAPIVersion != "" {
    if len(res.Consumes) == 0 { res.Consumes = []byte("[]") }
    if len(res.Produces) == 0 { res.Produces = []byte("[]") }
}
```
Untuk 3.x ketiganya tetap `""`/nil (jangan diisi `[]`). Tidak ada perubahan lain: `normalize`, pemilihan spec, `Parse` ulang, `FindOperation` (sudah sadar 2.0 dari F4), `Similar`, `Hint`, isolasi `ProjectID` (D12) tidak disentuh. Commit: `feat(project-api-spec): get-endpoint mengembalikan penanda versi, consumes, produces untuk Swagger 2.0`.

### G4. Konsumen MCP (`internal/domain/discovery/internal/apispectools/base`)

1. `format.go`: tambah
   ```go
   // SpecVersionLabel: "Swagger 2.0" untuk "2.0"; "OpenAPI -" untuk kosong; selain itu "OpenAPI "+v.
   func SpecVersionLabel(v string) string
   ```
   Konstanta literal "2.0" didefinisikan lokal (`swagger2Version = "2.0"`); discovery TIDAK mengimpor `project/internal/...` (aturan modul internal). Tes memastikan label kontrak lewat nilai literal.
2. `catalog.go` `specLine`: ganti `ov := orDash(Clip(OneLine(s.OpenapiVersion), ...))` dan format `| OpenAPI %s |` menjadi `ov := SpecVersionLabel(Clip(OneLine(s.OpenapiVersion), specVersionMaxRunes))` dan `| %s |`. Kosong tetap menghasilkan `OpenAPI -` (identik dengan sekarang); 3.x tetap `OpenAPI 3.0.3`; 2.0 menjadi `Swagger 2.0`. Perhatikan: `orDash` masih dipakai untuk `title` dan `av`.
3. `detail.go`:
   - `NewLookup` (cabang `Found`): validasi `Consumes`/`Produces` — bila `len(TrimSpace)==0` dan `OpenAPIVersion != ""` set `[]`; bila terisi dan `!json.Valid` -> error `"consumes dari provider bukan JSON valid"` / `"produces ..."` (pola pesan yang ada). Bila `OpenAPIVersion == ""` biarkan (diabaikan saat render).
   - `detailJSON` tambah di AKHIR struct (setelah `SecuritySchemes`, urutan kunci JSON terjaga untuk 3.x):
     ```go
     OpenAPIVersion string          `json:"openapi_version,omitempty"`
     Consumes       json.RawMessage `json:"consumes,omitempty"`
     Produces       json.RawMessage `json:"produces,omitempty"`
     ```
   - `Text()`: isi tiga field HANYA bila `d.Resp.OpenAPIVersion != ""` (jangan pernah memancarkan `consumes` untuk 3.x walau provider mengirim). `RawMessage` dengan `omitempty` tidak dipancarkan bila nil sehingga 3.x identik byte.
   - `collect()` (penanda `$circular`/`$ref`), `Summary()` (audit), `Hint()` TIDAK diubah; consumes/produces hanya berisi string.
4. Deskripsi tool dan skema input TIDAK diubah (golden `testdata/tools_list.golden.json` tidak boleh berubah; jangan jalankan `-update`).
5. Grep akhir: `grep -rn '"OpenAPI ' backend/internal/domain/discovery` hanya boleh menemukan `SpecVersionLabel` (dan tes). Tool `list_api_endpoints` tidak mencetak versi (sudah dicek), tidak berubah.
6. Commit: `feat(discovery-api-spec-tools): label Swagger 2.0 dan field openapi_version/consumes/produces`.

### G5. Spike tahap b (gerbang overview F0)

Setelah G1, jalankan ulang `zz_spike_test.go` tahap b (`SPIKE_SPEC=... go test ./internal/domain/project/internal/apispec/base/openapi/ -run TestSpikeHRPlatform -v`): `ResolveOperation` untuk 3 operasi `POST` terbesar dengan `getendpoint.DefaultSchemaDepth` (4) dan `DefaultMaxBytes` (102400). Catat waktu, ukuran hasil, `Truncated`, dan apakah budget `maxInlineExpansions` habis (tambahkan penghitung sementara hanya di file spike, bukan di kode produksi). Bila batas D8 terlampaui atau waktu > 2 detik: BERHENTI dan laporkan ke pemilik; jangan ubah konstanta. Hapus file spike, tanpa commit. Angka dilaporkan di laporan usecase `uploadspecs`.

### G6. Verifikasi

- `cd backend && go test ./internal/domain/project/... ./internal/domain/discovery/... ./lib/...`
- `go test ./...`, `go vet ./...`, tes arsitektur `internal/archtest`.
- `go test -tags integration ./internal/domain/project/internal/apispec/...` dengan `TEST_DATABASE_URL` ke DB test (`make dev-test-db`), bukan DB dev.
- `git diff --stat -- internal/domain/discovery/internal/apispectools/testdata/` kosong.

---

## 4. Migration

Tidak ada. `api_specs.openapi_version` sudah `text NOT NULL` tanpa CHECK dan menyimpan `"2.0"` apa adanya (D1). Aturan `.claude/rules/migrations.md` tidak relevan.

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `internal/domain/project/internal/apispec/base/openapi/resolve.go` | `Resolved` +3 field; `swagger2Servers`, `effectiveStringList`, `schemeDefs`; cabang 2.0 di `ResolveOperation` (method via `methodsFor`, servers, security defs, consumes/produces, `size`) |
| `.../base/openapi/resolve_3x_test.go` (baru) | tes golden 3.x + asersi field baru nol |
| `.../base/openapi/testdata/resolve_3x.golden.json` (baru) | golden dibuat dari kode SEBELUM perubahan |
| `.../base/openapi/resolve_swagger2_test.go` (baru) | tes cabang 2.0 (bagian 6) |
| `lib/contracts/project/apispec/getendpoint/getendpoint.go` | `Response` +`OpenAPIVersion/Consumes/Produces`; doc paket |
| `internal/domain/project/internal/apispec/getendpoint/usecase.go` | penyalinan tiga field; default `[]` hanya untuk 2.0 |
| `internal/domain/project/internal/apispec/getendpoint/usecase_test.go` | tes 2.0 dan 3.x tak berubah (bagian 6) |
| `internal/domain/discovery/internal/apispectools/base/format.go` | `SpecVersionLabel` |
| `.../apispectools/base/catalog.go` | `specLine` memakai `SpecVersionLabel` |
| `.../apispectools/base/detail.go` | validasi `Consumes/Produces`; `detailJSON` +3 field `omitempty`; `Text()` mengisi hanya untuk 2.0 |
| `.../apispectools/base/{format_test,catalog_test,detail_test}.go` | tes baru (bagian 6) |
| `.../apispectools/getapiendpoint/usecase_test.go` | satu tes 2.0 melalui fake bus (opsional, bagian 6) |
| `docs/tickets/README.md` | satu kalimat tentang field penanda versi pada kontrak `project.apispec.get-endpoint` |

Tidak berubah: `lib/constants/bus.go`, `listspecs`/`listendpoints` kontrak, repository, `limits.go`, `yamlguard.go`, `testdata/tools_list.golden.json`, katalog audit.

---

## 6. Test Scenario

### 6.1 `base/openapi` (unit; fixture foundation F5 + YAML inline lewat helper `parseSwagger2(t, src string) Document` di `resolve_swagger2_test.go`)

- `TestResolveOperation_3xUnchanged` (G0): golden byte identik; `OpenAPIVersion==""`, `Consumes==nil`, `Produces==nil`.
- `TestResolveOperation_Swagger2_InlinesDefinitions`: petstore 2.0 POST `/pets` (body `schema: {$ref: '#/definitions/Pet'}`) depth 4 -> `Operation` memuat properti Pet ter-inline, tidak ada kunci `definitions`/`Unused`; parameter `in: body` tetap ber-bentuk 2.0; `OpenAPIVersion == "2.0"`; `Truncated=false`, `EffectiveDepth=4`.
- `..._Swagger2_InlinesParametersAndResponses`: `$ref: '#/parameters/Limit'` dan `$ref: '#/responses/NotFound'` ter-inline.
- `..._Swagger2_PathParamsMerged`: parameter level path digabung; operasi menang untuk `(name, in)` sama, termasuk parameter path-level via `$ref: '#/parameters/X'` (pola `ParamRefIdentity`).
- `..._Swagger2_DepthLeavesRef`: depth 1 menyisakan `{"$ref":"#/definitions/..."}` pada level 2; depth 0 mempertahankan `$ref`.
- `..._Swagger2_Circular`: `swagger-2.0-circular.yaml` -> `{"$circular":"#/definitions/Node"}`.
- `..._Swagger2_Servers` (tabel, YAML inline): `schemes:[http,https] + host + basePath:/v1` -> dua url `http://h/v1`, `https://h/v1`; tanpa `schemes` -> `https://h/v1`; tanpa `host` -> `[{"url":"/v1"}]` (schemes diabaikan); tanpa `basePath` -> `https://h/`; tanpa host dan basePath -> `[{"url":"/"}]`; `schemes` duplikat dibuang; tanpa apapun tidak pernah `null`.
- `..._Swagger2_Security`: `security` root `[{basic: []}]` memilih `basic` dari `securityDefinitions`; operasi `security: []` -> `{}`; operasi `security: [{oauth: [read]}]` menimpa root dan memuat definisi `oauth2` dengan `scopes`; nama tak terdefinisi dilewati; definisi tanpa `security` efektif tidak muncul.
- `..._Swagger2_ConsumesProduces`: operasi menimpa root; operasi `consumes: []` menimpa root menjadi `[]`; tanpa keduanya -> `[]`, bukan `null`; `Produces` simetris.
- `..._Swagger2_TraceNotFound`: `TRACE` pada `swagger-2.0-full.yaml` (ada kunci `trace`) -> `ErrOperationNotFound`; sementara `TRACE` pada dokumen 3.x tetap bisa di-resolve (kontrol).
- `..._Swagger2_TruncationStepsDown`: dokumen 2.0 dalam; maxBytes turun -> `Truncated`, `EffectiveDepth` turun; maxBytes 10 -> `$truncated` dengan `summary`/`operationId`, dan `OpenAPIVersion/Consumes/Produces` tetap terisi.
- `..._Swagger2_InlineBudget`: dokumen 2.0 sintetis fan-out besar menghabiskan `maxInlineExpansions` -> sisa `$ref` tidak di-inline, tidak hang (batas waktu tes).
- `..._Swagger2_NotFound`: path tidak ada, method tidak ada pada path, `paths` hilang.

### 6.2 `getendpoint` usecase (`usecase_test.go`, fakeRepo yang ada; tambah `swagger2YAML` konstanta)

- `TestExecute_Swagger2SetsOpenAPIVersion`: `Found`, `OpenAPIVersion=="2.0"`, `Consumes`/`Produces` JSON array benar, `Servers` dari host/basePath/schemes, `SecuritySchemes` object, `Operation` valid JSON.
- `TestExecute_Swagger2ByOperationID`: pencarian via `operation_id` pada spec 2.0.
- `TestExecute_Swagger2TraceNotFound`: `Reason==ENDPOINT_NOT_FOUND`, `Spec` terisi, `Similar` dari `ListSpecEndpoints` (epCalls==1).
- `TestExecute_3xFieldsEmpty`: spec 3.x (`specYAML`) -> `OpenAPIVersion==""`, `Consumes==nil`, `Produces==nil`; `Operation/Servers/SecuritySchemes` sama dengan hasil sebelum T10 (bandingkan dengan literal JSON yang dikunci).
- `TestExecute_Swagger2ProjectIsolation`: spec 2.0 hanya di `pidB`, request `pidA` -> `SPEC_NOT_FOUND` (D12; query selalu memakai `ProjectID` request).
- Tes yang ada tidak diubah dan lulus.

### 6.3 `apispectools/base` (unit)

- `TestSpecVersionLabel`: `"2.0"` -> `Swagger 2.0`; `"3.0.3"` -> `OpenAPI 3.0.3`; `""` -> `OpenAPI -`; `"3.1.0"` -> `OpenAPI 3.1.0`.
- Catalog: baris spec 2.0 `... | versi 1.0.0 | Swagger 2.0 | 4 endpoint | diperbarui ...`; baris 3.x identik dengan sebelumnya (tes catalog lama tidak diubah dan hijau); `OpenapiVersion` kosong tetap `OpenAPI -`; versi sangat panjang tetap diklip sebelum dilabeli; anggaran byte (`Budget`) tetap menghitung baris dengan benar karena label dihitung di `specLine`.
- Detail 2.0: `Text()` memuat `"openapi_version":"2.0","consumes":["application/json"],"produces":["application/json"]` setelah `securitySchemes` (urutan kunci dikunci); `consumes: []` tetap dipancarkan sebagai `[]` untuk 2.0.
- Detail 3.x: `Text()` dari `foundResp()` TIDAK mengandung `openapi_version`, `consumes`, `produces`; string persis sama dengan sebelum perubahan (bandingkan dengan literal). Bila provider salah mengirim `Consumes` untuk respons 3.x (`OpenAPIVersion==""`), field tetap tidak dipancarkan.
- `NewLookup` menolak `Consumes` bukan JSON valid (2.0); respons 2.0 dengan `Consumes` kosong -> `[]`.
- `Summary()` tidak berubah (kunci audit yang sama); `Markers`/`Hint` tidak terpengaruh.
- Golden `apispectools/base_test.go` (`tools_list.golden.json`) lulus tanpa `-update` dan file tidak berubah di git.
- Opsional `getapiendpoint/usecase_test.go`: fake bus mengembalikan `Response` 2.0 -> teks hasil memuat tiga field; fake 3.x -> tidak.

### 6.4 Integration (`-tags integration`, DB test)

Ditambah di `apispec_integration_test.go` milik `uploadspecs` bila plan itu mengerjakannya; jika tidak ada, tambahkan satu tes di sini: unggah petstore 2.0 -> panggil handler bus `project.apispec.get-endpoint` (POST `/pets`) -> `OpenAPIVersion "2.0"`, `Servers`, `Consumes`; lalu 3.0 -> field kosong. Wajib memakai DB test.

---

## 7. E2E Scenarios

Positif:
1. Project dengan spec `petstore` (2.0, `host: petstore.example.com`, `basePath: /v1`, `schemes: [https]`): MCP `list_api_specs` memuat `| Swagger 2.0 |`.
2. MCP `get_api_endpoint(spec=petstore, method=POST, path=/pets)`: teks JSON memuat `operation` ter-inline (body Pet terbuka), `servers:[{"url":"https://petstore.example.com/v1"}]`, `securitySchemes`, `openapi_version:"2.0"`, `consumes`, `produces`.
3. `get_api_endpoint(operation_id=createPet)` pada spec 2.0 -> hasil sama.
4. Spec 2.0 bercabang dua scheme (`http`, `https`) -> `servers` berisi dua entri.
5. Operasi 2.0 dengan `$ref` melingkar -> `$circular` muncul beserta blok `markers` (mekanisme T09 sama).
6. Operasi 2.0 besar dengan `max_bytes` kecil -> `truncated:true`, `hint`, `schema_depth` turun; `openapi_version/consumes/produces` tetap ada.
7. Spec 3.0.3 yang sudah ada: `list_api_specs` tetap `| OpenAPI 3.0.3 |`; `get_api_endpoint` tidak memuat `openapi_version/consumes/produces`; seluruh keluaran identik dengan sebelum T10.
8. `tools/list` MCP identik dengan sebelum T10 (golden).
9. Uji nyata hr-platform (setelah upload lolos): `get_api_endpoint` untuk satu operasi `POST` besar; laporkan waktu, ukuran, `truncated` apa adanya.

Negatif:
1. `get_api_endpoint(method=TRACE, path=/pets)` pada spec 2.0 -> `ENDPOINT_NOT_FOUND` dengan daftar `Similar` (hasil tool `isError` sesuai alur T09 yang ada; tidak ada panic).
2. Method/path tidak ada pada spec 2.0 -> `ENDPOINT_NOT_FOUND` + saran `list_api_endpoints`.
3. `spec` tidak diberi pada project dengan >1 spec -> `SPEC_REQUIRED` (tidak berubah); nama salah -> `SPEC_NOT_FOUND`.
4. Spec milik project lain tidak dapat dibaca (isolasi `ProjectID`).
5. Spec 2.0 tersimpan yang gagal parse ulang (data rusak) -> error internal (`getendpoint: reparse stored spec`), tidak bocor isi.
6. Request tidak valid (`method` tanpa `path`, `schema_depth` di luar rentang di-clamp) -> perilaku lama.

---

## 8. Request/Response body

### Bus request `project.apispec.get-endpoint` (tidak berubah)

```json
{ "ProjectID": "7d1f0c8e-2b4a-4c1e-9f3a-5e6d7c8b9a01", "Spec": "petstore", "Method": "POST", "Path": "/pets", "SchemaDepth": 4, "MaxBytes": 102400 }
```

### Bus response, Swagger 2.0 ditemukan

```json
{
  "Found": true,
  "Reason": "",
  "Specs": [],
  "Similar": [],
  "Spec": "petstore",
  "Method": "POST",
  "Path": "/pets",
  "Operation": {
    "operationId": "createPet",
    "parameters": [
      { "name": "body", "in": "body", "required": true,
        "schema": { "type": "object", "required": ["id", "name"],
          "properties": { "id": { "type": "integer" }, "name": { "type": "string" } } } }
    ],
    "responses": { "201": { "description": "created" } },
    "security": [{ "api_key": [] }]
  },
  "Servers": [{ "url": "https://petstore.example.com/v1" }],
  "SecuritySchemes": { "api_key": { "type": "apiKey", "name": "X-API-Key", "in": "header" } },
  "Truncated": false,
  "EffectiveDepth": 4,
  "Hint": "",
  "OpenAPIVersion": "2.0",
  "Consumes": ["application/json"],
  "Produces": ["application/json"]
}
```

### Bus response, OpenAPI 3.x (identik dengan sebelum T10)

```json
{
  "Found": true, "Specs": [], "Similar": [], "Spec": "billing", "Method": "POST", "Path": "/orders",
  "Operation": { "operationId": "createOrder", "requestBody": { "content": { "application/json": { "schema": { "type": "object" } } } }, "responses": { "201": { "description": "ok" } } },
  "Servers": [], "SecuritySchemes": {},
  "Truncated": false, "EffectiveDepth": 4, "Hint": "",
  "OpenAPIVersion": "", "Consumes": null, "Produces": null
}
```

### Bus response, operasi tidak ada (2.0, mis. TRACE)

```json
{
  "Found": false,
  "Reason": "ENDPOINT_NOT_FOUND",
  "Specs": [],
  "Similar": [
    { "Method": "GET", "Path": "/pets", "OperationID": "listPets", "Summary": "List pets" },
    { "Method": "POST", "Path": "/pets", "OperationID": "createPet", "Summary": "Create pet" }
  ],
  "Spec": "petstore",
  "OpenAPIVersion": "", "Consumes": null, "Produces": null
}
```

### Error request tidak valid (kontrak lama, `errs.ErrInvalid`)

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "method dan path harus diisi bersamaan" }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(Bentuk error HTTP/MCP mengikuti amplop standar; bus mengembalikan error Go yang membungkus `errs.ErrInvalid`.)

### MCP `list_api_specs` (teks)

```
2 API spec di project ini:
- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-20 08:15 UTC
- petstore: Swagger Petstore | versi 1.0.0 | Swagger 2.0 | 4 endpoint | diperbarui 2026-10-21 09:00 UTC
Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.
```

### MCP `get_api_endpoint` Swagger 2.0 (isi teks, JSON satu baris; ditampilkan berlipat untuk dibaca)

```json
{
  "spec": "petstore", "method": "POST", "path": "/pets", "schema_depth": 4, "truncated": false,
  "operation": { "operationId": "createPet", "parameters": [ { "name": "body", "in": "body", "required": true, "schema": { "type": "object" } } ], "responses": { "201": { "description": "created" } } },
  "servers": [{ "url": "https://petstore.example.com/v1" }],
  "securitySchemes": { "api_key": { "type": "apiKey", "name": "X-API-Key", "in": "header" } },
  "openapi_version": "2.0",
  "consumes": ["application/json"],
  "produces": ["application/json"]
}
```

### MCP `get_api_endpoint` 3.x

Sama persis seperti sebelum T10: tiga kunci terakhir tidak muncul.

### MCP endpoint tidak ketemu (tidak berubah)

```
Endpoint tidak ditemukan di spec petstore: TRACE /pets
Similar:
- GET /pets — List pets [listPets]
- POST /pets — Create pet [createPet]
Gunakan salah satu di atas, atau list_api_endpoints(spec="petstore", query="pets") untuk melihat endpoint yang ada.
```
(hasil tool `isError: true` sesuai `EndpointMiss.Result()` T09; catatan: overview menyebut `isError:false` pada bagian "MCP error", tetapi kode `EndpointMiss.Result()` saat ini memakai `IsError: true`. Perilaku kode tidak diubah oleh T10; tes mengikuti kode.)

---

## 9. Risiko dan catatan

1. Perbedaan dokumentasi: overview bagian 10 menulis `isError: false` untuk `ENDPOINT_NOT_FOUND`, sedangkan `detail.go` memakai `IsError: true`. T10 tidak mengubah ini; perlu koreksi kalimat di overview (bukan kode).
2. Operasi 2.0 besar (hr-platform) dapat memakai banyak anggaran inline; dicakup G5 dan tes `InlineBudget`. Tidak ada batas yang diubah.
3. `Operation` 2.0 mempertahankan `consumes`/`produces` milik operasi di dalam objek sekaligus di amplop (duplikasi kecil, disengaja agar objek tetap "apa adanya").
4. Kunci penanda versi di `detailJSON` ditaruh di akhir agar urutan kunci 3.x tidak bergeser; mengubah urutan akan memecah tes `JSONCompactOrdered`.
5. Bila pemilik mengubah K3/K8 (bentuk servers atau penanda versi), plan ini dan overview diperbarui lebih dulu.
