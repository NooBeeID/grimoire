# T09 discovery/api-spec-tools — Usecase `getapiendpoint` (tool MCP `get_api_endpoint`)

Tiket: `docs/tickets/T09-discovery-api-spec-tools.md`
Induk (mengikat): `docs/superpowers/plans/discovery-api-spec-tools/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview berlaku sampai diperbarui. Rujukan keputusan memakai ID overview (K1-K16).
Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` tidak relevan (tanpa migrasi); tidak ada knowledge doc module; tiket menyatakan UI "Tidak ada" jadi tidak ada frontend dan tidak ada bagian "Visual". Kontrak `backend/lib/contracts/project/apispec/getendpoint/getendpoint.go`, `toolkit.go`, dan provider `project/internal/apispec/base/openapi/resolve.go` sudah dibaca.

Perilaku provider T06 yang diandalkan (terverifikasi di `resolve.go`):
- Ukuran yang dibandingkan dengan `MaxBytes` = `len(Operation)+len(Servers)+len(SecuritySchemes)`. Provider mencoba kedalaman `SchemaDepth` turun sampai 0 hingga muat. `Truncated = EffectiveDepth < SchemaDepth`.
- Bila kedalaman 0 pun melebihi `MaxBytes`: `Operation = {"$truncated":true,"summary":...,"operationId":...}`, `Truncated=true`, `EffectiveDepth=0`.
- Kedalaman 0 yang muat (tanpa `$truncated`) mungkin terjadi: `Truncated=true`, `EffectiveDepth=0`, skema tampil sebagai `{"$ref": ...}`.
- `Servers` default `[]`, `SecuritySchemes` default `{}`. JSON di-marshal dengan HTML escape (`<`); entity T09 tidak meng-unescape.
- `Hint` bawaan provider (`hasil dipotong; turunkan schema_depth (dipakai N)`) TIDAK dipakai; entity membuat hint sendiri yang lebih memandu (bagian 3, G2).

---

## 0. Prasyarat (gerbang)

| Prasyarat | Cara cek |
|---|---|
| Gerbang F1 overview (T06 dan T07 di `main`, konstanta bus ada) | `git log main --oneline \| grep -E "project-api-spec\|discovery-mcp-gateway"`; `grep APISPEC_GET_ENDPOINT backend/lib/constants/bus.go` |
| `base/{names,args,format,budget,specmiss}.go` dari F3 overview beserta tesnya sudah ada | `ls backend/internal/domain/discovery/internal/apispectools/base` |
| `apispectools/base.go` (F4) memanggil `getapiendpoint.Init(d)` | baca file |

Bila F3/F4 belum ada, berhenti dan lapor. Jangan menyalin kode T07, jangan membuat ulang `args.go`/`specmiss.go` di sini.

Simbol bersama yang dipakai (signature tidak boleh diubah di sini):

| Simbol | Dari |
|---|---|
| `base.ToolGetAPIEndpoint`, `base.TargetTypeEndpoint`, `base.ProjectArgAliases`, `base.Methods`, `base.NormalizeMethod` | `base/names.go` |
| `base.ArgError`, `base.DecodeStrict(raw, dst, allowed)`, `base.StrArg(field, v, maxRunes, hint)`, `base.IntArg(field, v, def, min, max, hint)`, `base.ArgResult(e, targetType)`, `base.Brief` | `base/args.go` |
| `base.OneLine`, `base.Clip` | `base/format.go` |
| `base.ProviderMaxBytes(max)` | `base/budget.go` |
| `base.SpecMiss{Tool, Requested, Required, Valid}` dan `.Result()` | `base/specmiss.go` |

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T09 (S), usecase 3 dari 3 |
| Scope | fullstack menurut workflow; UI tiket "Tidak ada" jadi hanya backend |
| Domain / Module | discovery / api-spec-tools (package `apispectools`) |
| Feature folder | `backend/internal/domain/discovery/internal/apispectools/getapiendpoint/` |
| Endpoint (milik T07) | `/mcp` |
| Method | POST, JSON-RPC `tools/call` dengan `params.name="get_api_endpoint"` (dan `tools/list` memuat entri tool ini) |
| Version | MCP protocol 2025-06-18 (T07), JSON-RPC 2.0; tanpa versi REST |
| Endpoint REST baru | Tidak ada. `endpointsChanged = false` |
| Bus dikonsumsi | `project.apispec.get-endpoint` (`constants.BUS_PROJECT_APISPEC_GET_ENDPOINT`, Call sinkron, provider T06) |
| Bus disediakan | Tidak ada |
| Aksi audit | `mcp.get_api_endpoint` (sudah ada di `record.Actions()`, ditulis gateway; tool tidak menulis audit, K13) |
| Migrasi | Tidak ada |

---

## 2. Summary Plan

`get_api_endpoint` mengembalikan detail satu operasi API. Handler mem-parse argumen secara ketat (`spec`, `method`+`path` ATAU `operation_id`, `schema_depth`; tanpa `project_id`), usecase memanggil bus T06 lewat repository dengan `SchemaDepth` dan `MaxBytes=ProviderMaxBytes(MCPMaxResponseBytes)`, lalu `base.NewLookup` memilih bentuk hasil: JSON operasi (`EndpointDetail`), "spec wajib/tidak ketemu" (`SpecMiss`), atau "endpoint tidak ketemu" (`EndpointMiss`).

Keputusan kunci usecase ini:
1. Validasi `Parse` adalah superset aturan provider (K4). Tabel pemetaan ada di bagian 3 (P1). Karena `errs` tidak di-import, `errs.ErrInvalid` dari provider hanya mungkin karena bug T09 dan dipetakan gateway menjadi `internal`.
2. Satu usecase, satu `Execute`: panggil repository, bangun `Lookup`. Tidak dipecah. Rule bentuk hasil (marker, hint, JSON, saran) ada di entity `base/detail.go`.
3. `Text` sukses adalah SATU object JSON kompak (bukan teks bebas) agar agent bisa mem-parse. Marker `$circular`/`$ref` dikumpulkan ke `markers` supaya agent tidak perlu menelusuri sendiri.
4. Hint dibuat entity (bukan memakai `Resp.Hint`) dengan tiga cabang: normal (turunkan `schema_depth`), kedalaman 0 tanpa `$truncated`, dan operasi terlalu besar (`$truncated`).
5. Hasil tidak ketemu bukan error Go: `isError:true` dengan teks memandu. Hanya `Reason` tidak dikenal (pelanggaran kontrak) yang menjadi error Go.
6. `ProjectID` hanya dari `call.ProjectID` (AC6). Argumen `project_id` ditolak `DecodeStrict` sebelum bus dipanggil.
7. Batas ukuran: `MaxBytes` ke provider = `max - min(2048, max/4)`; sisa selisih dipakai pembungkus JSON, hint, dan markers. Agar pembungkus tidak melewati sisa itu, daftar markers dibatasi (G4). Dengan begitu pemotongan generik gateway (yang merusak JSON) praktis tidak terjadi (AC5).

---

## 3. Plan (langkah berurutan)

Semua path relatif terhadap `backend/internal/domain/discovery/internal/apispectools/` kecuali disebut lain. Kerjakan dengan TDD: tes dulu, lalu implementasi.

**G0. Gerbang prasyarat.** Lihat bagian 0.

**G1. Entity `base/detail.go` (+ `base/detail_test.go`).** Murni, tanpa I/O. Import: stdlib (`bytes`, `encoding/json`, `fmt`, `sort`, `strconv`, `strings`), `toolkit`, `getendpoint`. Tulis tes dulu (bagian 5.1), lalu:

```go
package base

// EndpointAsk adalah permintaan agent setelah normalisasi Parse.
type EndpointAsk struct {
	Spec, Method, Path, OperationID string // Method sudah uppercase; Path persis; OperationID ""/diisi
}

// Lookup adalah hasil usecase get_api_endpoint: EndpointDetail, EndpointMiss, atau SpecMiss.
type Lookup interface{ Result() toolkit.Result }

const (
	maxMarkers      = 20  // per daftar
	markerBudget    = 700 // byte total string marker yang ditampilkan (G4)
	markerClipRunes = 100
	maxSimilar      = 5
	similarSummary  = 160
	suggestQueryMax = 40
	pathShownMax    = 200
)

// NewLookup memilih entity dari resp. error HANYA bila:
//  - Found=false dan Reason bukan SPEC_REQUIRED/SPEC_NOT_FOUND/ENDPOINT_NOT_FOUND, atau
//  - Found=true dan Operation/Servers/SecuritySchemes bukan JSON valid (json.Valid; kosong diganti default: Operation "null" ditolak, Servers "[]", SecuritySchemes "{}").
// Keduanya pelanggaran kontrak T06; handler membungkusnya sebagai error Go.
func NewLookup(ask EndpointAsk, requestedDepth, maxBytes int, resp getendpoint.Response) (Lookup, error)
```

Pemilihan di `NewLookup`:

| Kondisi | Hasil |
|---|---|
| `resp.Found` | `EndpointDetail{Resp, RequestedDepth, MaxBytes}` setelah `json.Valid` ketiga field (Operation kosong = error; Servers kosong dinormalkan `[]`; SecuritySchemes kosong dinormalkan `{}`) |
| `Reason == ReasonSpecRequired` | `SpecMiss{Tool: ToolGetAPIEndpoint, Requested: ask.Spec, Required: true, Valid: resp.Specs}` |
| `Reason == ReasonSpecNotFound` | `SpecMiss{Tool: ToolGetAPIEndpoint, Requested: ask.Spec, Required: false, Valid: resp.Specs}` |
| `Reason == ReasonEndpointNotFound` | `EndpointMiss{Spec: resp.Spec (fallback ask.Spec bila kosong), Ask: ask, Similar: resp.Similar dipotong ke 5, disalin}` |
| lainnya | `nil, fmt.Errorf("reason tidak dikenal %q (found=false)", resp.Reason)` |

`EndpointDetail`:

```go
type EndpointDetail struct {
	Resp           getendpoint.Response
	RequestedDepth int
	MaxBytes       int // MaxBytes yang dikirim ke provider
}
func (d EndpointDetail) Text() string
func (d EndpointDetail) Hint() string
func (d EndpointDetail) Markers() (circular, collapsed []string) // sudah dibatasi (G4)
func (d EndpointDetail) Summary() map[string]any
func (d EndpointDetail) Result() toolkit.Result
```

Aturan `Text()` (G3, bentuk JSON):
- Struct `detailJSON` dengan urutan kunci persis: `spec, method, path, schema_depth, truncated, hint (omitempty), markers (omitempty, pointer), operation, servers, securitySchemes`. Nilai `spec/method/path` dari `Resp` (bukan dari `ask`), `schema_depth = Resp.EffectiveDepth` (bisa 0), `operation/servers/securitySchemes` bertipe `json.RawMessage` apa adanya (setelah normalisasi default).
- Marshal dengan `json.NewEncoder(&buf)` + `SetEscapeHTML(false)`, lalu buang `\n` akhir. Hasil kompak satu baris, tanpa indent. Jika `Encode` gagal (tidak mungkin setelah `json.Valid`), kembalikan `{"error":"gagal menyusun hasil"}`; jangan panic.
- `markers` ditampilkan bila ada minimal satu `$circular`, satu `$ref`, atau `operation` root berisi `"$truncated":true`. Bentuk: `{"circular":[...],"collapsed":[...],"legend":"..."}`. Daftar kosong dirender `[]`, bukan `null`.
- `legend` = `"$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth"`, ditambah `"; $truncated: operasi terlalu besar, hanya ringkasan"` bila ada `$truncated` root.

Aturan pengumpulan marker (`collect()` internal, hasil penuh dipakai Summary; `Markers()` memakai versi terbatas):
- Decode `Operation`, `Servers`, `SecuritySchemes` ke `any` (decoder biasa), telusuri rekursif lewat `map[string]any` dan `[]any`.
- Pada kunci `$circular` dengan nilai string, tambahkan ke himpunan circular. Pada kunci `$ref` dengan nilai string, tambahkan ke himpunan collapsed. Nilai bukan string diabaikan. Kunci tersebut tetap ditelusuri nilainya bila bukan string (tidak ada efek).
- Unik, urut leksikografis, tiap ref di-`Clip(.., 100)`.
- `circular_count`/`collapsed_count` di Summary = jumlah unik PENUH (sebelum pembatasan G4).
- Root `$truncated`: `Operation` di-decode ke `map[string]any` dan `m["$truncated"] == true`.

G4, pembatasan daftar marker (agar pembungkus tidak melewati sisa anggaran): potong tiap daftar ke 20 elemen pertama; lalu selama jumlah `len(ref)+3` seluruh elemen kedua daftar > 700, buang elemen terakhir dari daftar yang lebih panjang (jumlah elemen, bila sama circular dulu). Tampilan `markers` memakai hasil terbatas; tidak ada kunci tambahan untuk jumlah yang dibuang (agent tidak membutuhkannya; hint sudah mengarahkan menurunkan kedalaman). Alasan angka: pembungkus kunci (~150 byte) + legend (~230) + hint (~420) + markers (<= ~750) < 1,6 KB, di bawah sisa 2048 byte pada config default (K9).

Aturan `Hint()` (kosong bila `!Resp.Truncated`). Misalkan `eff = Resp.EffectiveDepth`, `req = RequestedDepth`:
1. `eff == 0` dan root `$truncated`: `"Operasi terlalu besar bahkan tanpa membuka skema; hanya ringkasan yang ditampilkan. Gunakan list_api_endpoints untuk konteks dan minta potongan spec ke pemilik project."`
2. `eff == 0` tanpa `$truncated`: `"Respons dibatasi <MaxBytes> byte; skema tidak dibuka sama sekali (kedalaman 0) dan tampil sebagai {\"$ref\": ...}. Detail skema yang dirujuk tidak dapat ditampilkan lebih jauh dalam batas ukuran respons."`
3. selain itu: `"Respons melebihi batas <MaxBytes> byte; skema hanya dibuka sampai kedalaman <eff> (diminta <req>). Bagian lebih dalam tampil sebagai {\"$ref\": ...}. Turunkan schema_depth (mis. <s>) agar respons ringkas dan konsisten."` dengan `s = max(1, eff)`; bila `s >= req` dan `req > 1` maka `s = req-1`.

Cabang 3 selalu mengandung kata `schema_depth` (AC5).

`Summary()` = `{found:true, schema_depth:eff, requested_depth:req, truncated:Resp.Truncated, circular_count:<penuh>, collapsed_count:<penuh>}`. DILARANG memuat isi operasi, summary, atau ref.

`Result()` = `toolkit.Result{Text: Text(), IsError:false, TargetType:"api_endpoint", Target: fmt.Sprintf("%s %s %s", Resp.Spec, Resp.Method, Resp.Path), Summary: Summary()}`.

`EndpointMiss`:

```go
type EndpointMiss struct {
	Spec    string
	Ask     EndpointAsk
	Similar []getendpoint.EndpointRef // maks 5
}
func (m EndpointMiss) Subject() string  // "GET /pets" atau "operation_id <id>" (Clip path 200 rune, OneLine)
func (m EndpointMiss) Target() string   // "<spec> <METHOD> <path>" atau "<spec> operation_id=<id>" (tanpa Clip)
func (m EndpointMiss) SuggestQuery() string
func (m EndpointMiss) Text() string
func (m EndpointMiss) Summary() map[string]any
func (m EndpointMiss) Result() toolkit.Result
```

Aturan `Text()` (baris dipisah `\n`, tanpa `\n` penutup):
```
Endpoint tidak ditemukan di spec <Spec>: <Subject>
Similar:                                   (hanya bila len(Similar) > 0)
- <METHOD> <path> — <Clip(OneLine(summary),160)> [<operationId>]
...
<Saran>
```
- Baris Similar: `- ` + `Method Path`; ` — <ringkasan>` hanya bila summary tidak kosong; ` [<operationId>]` hanya bila ada. Tag tidak ditampilkan. Urutan input dipertahankan.
- `Saran` bila ada Similar: `Gunakan salah satu di atas, atau list_api_endpoints(spec=<Quote(Spec)>, query=<Quote(q)>) untuk melihat endpoint yang ada.` Bila tidak ada Similar: `Periksa ejaan path atau operation_id, atau gunakan list_api_endpoints(spec=<Quote(Spec)>, query=<Quote(q)>) untuk melihat endpoint yang ada.` `Quote` = `strconv.Quote`. Bila `q == ""`, argumen `, query=...` dihilangkan.
- `SuggestQuery()`: `operation_id` bila diminta; selain itu segmen path pertama yang tidak kosong dan tidak diawali `{` (split `/`); hasil `Clip(OneLine(.), 40)`; kosong bila tidak ada (mis. path `/` atau `/{id}`).
- `Summary()` = `{found:false, error_code:"endpoint_not_found", similar_count:len(Similar)}`.
- `Result()` = `{Text, IsError:true, TargetType:"api_endpoint", Target: Target(), Summary}`.

`SpecMiss.Result()` (dari F3) dipakai apa adanya untuk SPEC_REQUIRED/SPEC_NOT_FOUND (AC3). `SpecMiss` memenuhi `Lookup` tanpa perubahan.

**G2. `getapiendpoint/params.go` (+ `params_test.go`).** Tulis tes dulu (bagian 5.2).

```go
type rawArgs struct {
	Spec        *string `json:"spec"`
	Method      *string `json:"method"`
	Path        *string `json:"path"`
	OperationID *string `json:"operation_id"`
	SchemaDepth *int    `json:"schema_depth"`
}

// Params adalah argumen tervalidasi.
type Params struct {
	Ask         base.EndpointAsk
	SchemaDepth int // 1..8, default getendpoint.DefaultSchemaDepth
}

var allowedArgs = []string{"spec", "method", "path", "operation_id", "schema_depth"}

func schema() json.RawMessage   // persis schema bagian 12.1 overview (get_api_endpoint)
func Parse(raw json.RawMessage) (Params, *base.ArgError)
```

Urutan `Parse` (error pertama menang, K7):
1. `base.DecodeStrict(raw, &r, allowedArgs)` (menolak field tak dikenal, alias project dengan pesan khusus K5, tipe salah).
2. `spec`: `base.StrArg("spec", r.Spec, 80, "Nama spec dari list_api_specs; boleh dikosongkan bila project hanya punya satu spec.")`. Kosong setelah trim = tidak diisi.
3. `method`: `base.StrArg("method", r.Method, 32, ...)` lalu bila tidak kosong `base.NormalizeMethod`; gagal -> `ArgError{Field:"method", Reason:"harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE", Got: base.Brief(nilai), Hint:"Huruf besar/kecil bebas."}`. Batas 32 (bukan 7 seperti `maxLength` schema) agar pesan daftar method yang tampil, bukan "maksimal 7 karakter"; schema tetap `maxLength 7` sebagai panduan klien.
4. `path`: `base.StrArg("path", r.Path, 2048, ...)`; bila tidak kosong dan tidak diawali `/` -> `ArgError{Field:"path", Reason:"harus diawali /", Got: Brief, Hint:"Tulis path persis seperti di list_api_endpoints, mis. /pets/{petId}."}`. Path TIDAK dinormalisasi lain (K8).
5. `operation_id`: `base.StrArg("operation_id", r.OperationID, 200, ...)`.
6. `schema_depth`: `base.IntArg("schema_depth", r.SchemaDepth, 4, 1, 8, "Turunkan nilai bila hasil terpotong; maksimum 8.")`.
7. Aturan eksklusif (setelah semua field valid secara individual), `hasMP := method != "" || path != ""`, `hasOp := operationID != ""`:
   - `hasMP && hasOp` -> `ArgError{Field:"operation_id", Reason:"tidak boleh diisi bersama method/path (pilih salah satu: method + path atau operation_id)", Hint:"Hapus operation_id atau hapus method dan path."}`.
   - `!hasMP && !hasOp` -> `ArgError{Field:"method", Reason:"wajib diisi bersama path, atau isi operation_id", Hint:"Cari endpoint dengan list_api_endpoints."}`.
   - `method != "" && path == ""` -> `ArgError{Field:"path", Reason:"wajib diisi bersama method", Hint:...}`.
   - `path != "" && method == ""` -> `ArgError{Field:"method", Reason:"wajib diisi bersama path", Hint:...}`.

Tabel pemetaan superset (K4), setiap kondisi `errs.ErrInvalid` provider tidak boleh lolos:

| Kondisi provider `ErrInvalid` | Pencegah di `Parse` |
|---|---|
| ProjectID bukan uuid | berasal dari gateway (Principal), bukan dari agent |
| tidak ada (Method+Path) maupun OperationID | langkah 7, cabang `!hasMP && !hasOp` |
| keduanya diisi | langkah 7, cabang pertama |
| hanya salah satu dari Method/Path | langkah 7, cabang ketiga dan keempat |
| Method bukan 8 metode HTTP | langkah 3 |

**G3. `getapiendpoint/repository.go`.**

```go
type repository interface {
	Get(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error)
}
type busRepository struct{ b *bus.Registry }
func newRepository(b *bus.Registry) repository
func (r busRepository) Get(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error) {
	return bus.CallTyped[getendpoint.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, req) // request sebagai NILAI
}
```
Tipe parameter `b` mengikuti tipe `app.Deps.Bus` yang sebenarnya (sesuaikan bila bukan `*bus.Registry`). Tanpa logika; error dikembalikan apa adanya.

**G4. `getapiendpoint/usecase.go` (+ `usecase_test.go`).**

```go
type usecase struct {
	repo     repository
	maxBytes int // base.ProviderMaxBytes(cfg.MCPMaxResponseBytes), dihitung di init.go
}
func newUsecase(repo repository, maxBytes int) usecase

func (u usecase) Execute(ctx context.Context, projectID string, p Params) (base.Lookup, error) {
	resp, err := u.repo.Get(ctx, getendpoint.Request{
		ProjectID: projectID, Spec: p.Ask.Spec, Method: p.Ask.Method, Path: p.Ask.Path,
		OperationID: p.Ask.OperationID, SchemaDepth: p.SchemaDepth, MaxBytes: u.maxBytes, // K10: eksplisit
	})
	if err != nil { return nil, err }
	return base.NewLookup(p.Ask, p.SchemaDepth, u.maxBytes, resp)
}
```
Usecase hanya orkestrasi: tidak ada format teks, tidak ada aturan rentang.

**G5. `getapiendpoint/handler.go`.**

```go
func newHandler(uc usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, aerr := Parse(call.Arguments)
		if aerr != nil { return base.ArgResult(aerr, base.TargetTypeEndpoint), nil } // bus tidak dipanggil
		lk, err := uc.Execute(ctx, call.ProjectID, p)
		if err != nil { return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolGetAPIEndpoint, err) }
		return lk.Result(), nil
	}
}
```
Handler tidak memformat teks dan tidak menyentuh bus. Pembungkus `%w` menjaga `errors.Is(err, context.DeadlineExceeded)` dan `bus.ErrNotRegistered`.

**G6. `getapiendpoint/init.go`.**

```go
func Init(d app.Deps) toolkit.Tool {
	maxBytes := base.ProviderMaxBytes(d.Config.MCPMaxResponseBytes) // sekali, K9
	uc := newUsecase(newRepository(d.Bus), maxBytes)
	return toolkit.Tool{
		Name: base.ToolGetAPIEndpoint, Title: "Detail endpoint API",
		Description: <teks persis overview 12.1 untuk get_api_endpoint>,
		InputSchema: schema(), Handler: newHandler(uc),
	}
}
```

**G7. Gerbang.** Jalankan dari `backend/`: `go build ./...`, `go vet ./...`, `go test ./internal/domain/discovery/internal/apispectools/...`, `go test -race` pada paket yang sama, `go test ./internal/archtest/...`, lalu tes integrasi bagian 5.4. Pastikan golden `tools/list` (milik `base_test.go` overview F7) memuat entri `get_api_endpoint` sesuai overview 12.1; bila berbeda, perbaiki `schema()`/`Description`, bukan golden.

---

## 4. Migration

Tidak ada. Tanpa tabel, kolom, index, atau perubahan `migrations/**`.

## 4b. Changes (per file/folder)

| File | Status | Isi |
|---|---|---|
| `.../apispectools/base/detail.go` | baru | `EndpointAsk`, `Lookup`, `NewLookup`, `EndpointDetail`, `EndpointMiss`, pengumpul marker, hint |
| `.../apispectools/base/detail_test.go` | baru | bagian 5.1 |
| `.../apispectools/getapiendpoint/init.go` | baru | `Init(d)` (G6) |
| `.../apispectools/getapiendpoint/params.go` | baru | `rawArgs`, `Params`, `schema()`, `Parse` (G2) |
| `.../apispectools/getapiendpoint/handler.go` | baru | adapter `toolkit.Handler` (G5) |
| `.../apispectools/getapiendpoint/usecase.go` | baru | `Execute` (G4) |
| `.../apispectools/getapiendpoint/repository.go` | baru | port + `bus.CallTyped` (G3) |
| `.../apispectools/getapiendpoint/params_test.go` | baru | bagian 5.2 |
| `.../apispectools/getapiendpoint/usecase_test.go` | baru | bagian 5.3 |
| `.../apispectools/getapiendpoint/handler_test.go` | baru | bagian 5.3 |
| `backend/cmd/internal/server/apispectools_integration_test.go` | ubah/baru (berbagi dengan usecase lain, overview F8) | tes bagian 5.4; bila file belum ada, buat dengan helper `uploadSpecs`, `callTool`, `newProjectWithKey` dan fixture overview 10.3, jangan duplikasi bila sudah ada |

Tidak berubah oleh usecase ini: `base.go` module (F4) dan `discovery/base.go` (F6) milik overview; `lib/constants`, `lib/contracts/**`, `mcpgateway/**`, `project/**`, `api/**`, `migrations/**`, `frontend/**`, `go.mod`. Import `getapiendpoint/*.go` non-test hanya: stdlib, `infra/app`, `infra/bus`, `lib/constants`, `lib/contracts/project/apispec/getendpoint`, `lib/contracts/discovery/mcpgateway/toolkit`, `apispectools/base` (K3; dijaga `TestT09_AC7_ImportAllowlist`).

---

## 5. Test Scenario

### 5.1 `base/detail_test.go` (tabel, tanpa I/O)

Fixture: `Response` Found dengan `Operation` bersarang yang memuat `{"$circular":"#/components/schemas/Pet"}`, `{"$ref":"#/components/schemas/Owner"}` ganda, `Servers=[{"url":"https://x"}]`, `SecuritySchemes={"apiKey":{...}}`.

| Tes | Ekspektasi |
|---|---|
| `TestNewLookup_Found_JSONValidCompact` | `Text()` lolos `json.Valid`, tanpa `\n`, kunci utama berurutan `spec,method,path,schema_depth,truncated,markers,operation,servers,securitySchemes`; `spec/method/path` dari `Resp`; `schema_depth = EffectiveDepth` |
| `TestDetail_NoHTMLEscape` | operasi berisi `<` dan `&` mentah/`<` dari provider tidak diubah encoder (substring yang sama muncul) |
| `TestDetail_Markers_Collected` | circular = `["#/components/schemas/Pet"]`, collapsed unik dan urut walau `$ref` muncul berkali-kali dan di `servers`/`securitySchemes`; nilai non-string pada kunci `$ref` diabaikan |
| `TestDetail_Markers_ArraysNeverNull` | hanya circular ada -> `"collapsed":[]` bukan `null` |
| `TestDetail_NoMarkers_OmitsKey` | tanpa `$circular`/`$ref`/`$truncated` -> tidak ada kunci `markers`; `hint` juga tidak ada bila tidak terpotong |
| `TestDetail_Markers_CapAndByteBudget` | 50 ref unik -> maksimal 20 per daftar; ref panjang 100 rune -> total string marker <= 700 byte; `circular_count`/`collapsed_count` di Summary tetap 50 |
| `TestDetail_Hint_Depth` | `Truncated`, Effective 3, Requested 8 -> hint memuat `schema_depth`, `100352` (MaxBytes), `kedalaman 3`, `(diminta 8)`, `mis. 3` |
| `TestDetail_Hint_SuggestBelowRequested` | Effective 4, Requested 4 -> saran `3`; Requested 1, Effective 1 -> saran `1` |
| `TestDetail_Hint_Depth0Truncated` | Effective 0, Operation `{"$truncated":true,"summary":"x"}` -> hint khusus `Operasi terlalu besar`; `markers` ada dengan legend memuat `$truncated`; circular/collapsed `[]` |
| `TestDetail_Hint_Depth0NoTruncatedMarker` | Effective 0, Truncated, operasi biasa -> hint cabang 2, tidak menyarankan menurunkan `schema_depth` |
| `TestDetail_NotTruncated_NoHint` | `Truncated=false` -> kunci `hint` tidak ada, `truncated:false` |
| `TestDetail_Summary` | `{found:true, schema_depth, requested_depth, truncated, circular_count, collapsed_count}`; tidak ada kunci lain; tidak memuat isi/summary spec |
| `TestDetail_Result_Target` | `TargetType=="api_endpoint"`, `Target=="petstore GET /pets/{petId}"`, `IsError==false` |
| `TestNewLookup_Defaults` | `Servers` kosong -> `"servers":[]`; `SecuritySchemes` kosong -> `{}` |
| `TestNewLookup_InvalidProviderJSON` | Found dengan `Operation` bukan JSON valid atau kosong -> error |
| `TestNewLookup_SpecRequired` | `Reason=SPEC_REQUIRED`, `Specs=[a,b,c]` -> `IsError`, teks memuat `punya 3 API spec` dan `a, b, c`, Summary `error_code:"spec_required"` |
| `TestNewLookup_SpecNotFound` | `SPEC_NOT_FOUND` ask.Spec `petstor` -> `Spec tidak ditemukan: petstor` + nama valid |
| `TestNewLookup_SpecNotFound_NoSpecs` | `Specs` kosong -> teks `belum punya API spec`, `error_code:"no_specs"` |
| `TestNewLookup_EndpointNotFound_WithSimilar` | teks persis format bagian 7.2: baris `Similar:`, tiap baris `- METHOD /path — ringkasan [opId]`, saran `list_api_endpoints(spec="petstore", query="pet")`; Summary `{found:false, error_code:"endpoint_not_found", similar_count:2}`; `Target=="petstore GET /pet/{petId}"` |
| `TestNewLookup_EndpointNotFound_NoSimilar` | tanpa baris `Similar:` dan `- `; saran berawalan `Periksa ejaan`; `similar_count:0` |
| `TestEndpointMiss_OperationID` | ask dengan operation_id -> baris pertama `operation_id showPetByID`, query saran `showPetByID`, Target `petstore operation_id=showPetByID` |
| `TestEndpointMiss_SuggestQuery` | `/pets/{petId}` -> `pets`; `/{id}` dan `/` -> tanpa `, query=`; `/v1/pets` -> `v1`; segmen 60 rune -> di-Clip 40 |
| `TestEndpointMiss_SimilarCapAndSanitize` | 7 similar -> 5 baris; summary multi-baris/tab diratakan `OneLine` dan di-`Clip` 160 |
| `TestEndpointMiss_SpecFallback` | `resp.Spec` kosong -> memakai `ask.Spec` |
| `TestNewLookup_UnknownReason` | `Found=false, Reason="WEIRD"` dan `Reason=""` -> error |

### 5.2 `getapiendpoint/params_test.go`

| Kelompok | Kasus |
|---|---|
| Default | `{}`/`""`/`"null"` tanpa method/path/operation_id -> ArgError field `method` (`wajib diisi bersama path, atau isi operation_id`); `{"method":"GET","path":"/pets"}` -> `SchemaDepth==4`, `Ask{Method:"GET", Path:"/pets"}` |
| Eksklusivitas | method+path+operation_id -> field `operation_id`, pesan memuat `pilih salah satu`; hanya method -> field `path`; hanya path -> field `method`; hanya `operation_id` -> ok; method `"  "` + operation_id -> ok (kosong setelah trim = tidak diisi) |
| Method | `"get"`, `" Patch "`, `"tRaCe"` ok -> uppercase; `"FETCH"`, `"CONNECT"`, `"GETS"` ditolak dengan pesan memuat `GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE`; `"GET\t"` dengan tab dalam ditolak karakter kontrol; method 40 rune -> `maksimal 32 karakter` |
| Path | `"pets"` -> `harus diawali /`; `" /pets/{petId} "` -> `Path=="/pets/{petId}"` (hanya trim); `"/pets/"` tetap `"/pets/"` (slash akhir tidak dibuang); `"/Pets/{ID}"` tidak di-lowercase; `"/"` ok; path 2049 rune ditolak, 2048 diterima; karakter kontrol ditolak |
| Spec | 80 rune diterima, 81 ditolak; `"  petstore "` -> `petstore`; `spec:1` -> `harus string` |
| operation_id | 200 rune ok, 201 ditolak; trim; karakter kontrol ditolak |
| Depth | 1 dan 8 diterima; 0, 9, -1 ditolak `schema_depth harus 1-8 (diterima: 9)`; `"x"` dan `1.5` -> `harus integer`; tidak ada clamp |
| Alias project (AC6) | `project_id`, `projectId`, `project` -> pesan memuat `ditentukan oleh API key`; field tak dikenal `{"foo":1}` -> `foo tidak dikenal` + daftar `spec, method, path, operation_id, schema_depth`; token sisa `{}{}` ditolak; `[]` -> field `arguments` |
| Urutan error | input dengan `spec` terlalu panjang DAN `schema_depth:9` -> error `spec` (urutan schema); eksklusivitas hanya dicek setelah semua field valid |
| TES K4 (superset provider) | untuk tiap input berikut `Parse` HARUS mengembalikan `*ArgError` (bukan Params): `{}`; `{"method":"GET"}`; `{"path":"/pets"}`; `{"method":"GET","path":"/pets","operation_id":"x"}`; `{"method":"FETCH","path":"/pets"}`; `{"method":"GET","path":"pets"}`. Tes tambahan berbasis properti: untuk 8 method x {path valid, kosong} x {operation_id valid, kosong}, hasil sukses HANYA bila memenuhi aturan provider (tepat satu dari `method+path` atau `operation_id`, method di 8 metode) |
| Schema | `schema()` JSON valid; kunci properti persis `{spec, method, path, operation_id, schema_depth}`; `additionalProperties:false`; tidak ada `required`; `schema_depth` min 1, max 8, default 4 |

### 5.3 `usecase_test.go` dan `handler_test.go`

`usecase_test.go` (fake `repository` merekam request):
- Request diteruskan persis: `ProjectID` dari argumen `Execute`, `Spec/Method/Path/OperationID` dari `Params`, `SchemaDepth` dari `Params`, `MaxBytes` = nilai yang disuntik ke `newUsecase` (K10; bukan 0).
- Fake mengembalikan Found -> `Lookup` bertipe `EndpointDetail`; `SPEC_REQUIRED`/`SPEC_NOT_FOUND` -> `SpecMiss`; `ENDPOINT_NOT_FOUND` -> `EndpointMiss`.
- Error repository dikembalikan dengan `errors.Is(err, sentinel)` utuh; `Reason` tidak dikenal -> error; `context.DeadlineExceeded` dari fake tetap `errors.Is`.
- Tidak ada panggilan kedua ke repository (tepat satu `Get` per `Execute`).

`handler_test.go` (registry bus asli `bus.NewRegistry(nil)`, handler palsu didaftarkan pada `constants.BUS_PROJECT_APISPEC_GET_ENDPOINT` dengan penghitung panggilan; tool dirakit lewat `Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: reg})`):
- Argumen salah (method `FETCH`, `schema_depth:9`, alias `project_id`, tanpa method/operation_id) -> `IsError:true`, `TargetType:"api_endpoint"`, Summary `{error_code:"invalid_argument", field}`, penghitung bus tetap 0, err nil.
- `call.ProjectID="P1"` dengan argumen berisi `project_id:"P2"` -> ditolak, bus 0 kali; tanpa `project_id`, request ke bus membawa `ProjectID=="P1"` (AC6), `MaxBytes==100352`, `SchemaDepth==4` default, `Method=="GET"` dari input `"get"`, `Path` persis.
- Fake Found -> `IsError:false`, `Text` JSON valid, `Target=="petstore GET /pets/{petId}"`, Summary kunci persis `{found, schema_depth, requested_depth, truncated, circular_count, collapsed_count}`.
- Fake `ENDPOINT_NOT_FOUND` dengan Similar -> `IsError:true`, teks memuat `Similar:` dan `list_api_endpoints`; Summary `{found:false, error_code:"endpoint_not_found", similar_count}`.
- Fake `SPEC_REQUIRED` -> `IsError:true`, teks memuat nama spec valid.
- Config `MCPMaxResponseBytes:1024` -> `MaxBytes==768`.
- Endpoint bus tidak terdaftar -> `err` dengan `errors.Is(err, bus.ErrNotRegistered)`, `Result{}` kosong, pesan berawalan `get_api_endpoint:`.
- Fake mengembalikan error biasa dan `context.DeadlineExceeded` -> error Go terbungkus (`errors.Is` benar); fake mengembalikan `Reason` aneh -> error Go.
- Summary hasil mana pun tidak memuat summary/ref/isi operasi dari fake (asersi: marshal Summary tidak mengandung string unik yang ditanam di `Operation`).

### 5.4 Integrasi (`//go:build integration`, bagian milik usecase ini; fixture dan helper dari overview 10.3)

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT09_Int_ToolsList` (bagian get) | 7 | `tools/list` | entri `get_api_endpoint` ada, `annotations.readOnlyHint:true` |
| `TestT09_Int_AC2_NestedCircular` | 2 | key A: `{spec:"petstore", method:"get", path:"/pets/{petId}"}` | `isError:false`; teks JSON valid; `operation` memuat `Owner` dan `Address` ter-inline, `"$circular":"#/components/schemas/Pet"`, TIDAK memuat `Unused`; `markers.circular` berisi ref Pet; `servers` dan `securitySchemes.apiKey` ada |
| `TestT09_Int_AC2_ByOperationID` | 2 | `{spec:"petstore", operation_id:"showPetById"}` | hasil sama dengan method+path (`method:"GET"`, `path:"/pets/{petId}"`) |
| `TestT09_Int_AC3_SpecRequired` | 3 | key A: `{method:"GET", path:"/pets"}` | `isError:true`, memuat `punya 3 API spec` dan `billing, huge, petstore` |
| `TestT09_Int_AC3_SpecNotFound` | 3 | key A: `{spec:"petstor", method:"GET", path:"/pets"}` | `isError:true`, `Spec tidak ditemukan: petstor`, nama valid |
| `TestT09_Int_AC3_SingleSpecOptional` | 3 | key B: `{method:"GET", path:"/health"}` | `isError:false`, `"spec":"solo"` |
| `TestT09_Int_AC4_Similar` | 4 | `{spec:"petstore", method:"GET", path:"/pet/{petId}"}` | `isError:true`, `Endpoint tidak ditemukan`, `Similar:`, `GET /pets/{petId}`, `list_api_endpoints` |
| `TestT09_Int_AC4_OperationIDTypo` | 4 | `{spec:"petstore", operation_id:"showPetByID"}` | `isError:true`, baris pertama memuat `operation_id showPetByID` |
| `TestT09_Int_AC5_TruncatedDepth` | 5 | `{spec:"huge", method:"GET", path:"/report", schema_depth:8}` | `isError:false`; JSON valid (tidak ada `[truncated: respons dipotong`); `truncated:true`; `schema_depth < 8`; `hint` memuat `schema_depth`; `len(text) <= 102400` |
| `TestT09_Int_AC6_ProjectArg` | 6 | key A: `{project_id:<B>, method:"GET", path:"/health"}`; lalu `{spec:"solo", method:"GET", path:"/health"}` | pertama `isError:true` memuat `ditentukan oleh API key` dan tidak memuat `solo`; kedua `Spec tidak ditemukan` (project tetap A) |
| `TestT09_Int_InvalidArgs` | K4 | `{method:"FETCH",path:"/pets"}`, `{method:"GET",path:"pets"}`, `{spec:"petstore"}`, kombinasi lengkap, `schema_depth:9` | semua `isError:true` dengan pesan `Argumen tidak valid:`; BUKAN `Terjadi kesalahan internal` (membuktikan superset provider) |
| `TestT09_Int_AuditOneEntryPerCall` | K13 | satu panggilan sukses | tepat satu entri `mcp.get_api_endpoint`, `target_type=api_endpoint`, `target="petstore GET /pets/{petId}"`; result memuat `bytes` (dari gateway), `schema_depth`, `found`; tidak memuat isi skema |

---

## 6. E2E Scenarios

Positif:
1. Alur penuh agent: `list_api_specs {}` -> `list_api_endpoints {spec:"petstore"}` -> `get_api_endpoint {spec:"petstore", method:"GET", path:"/pets/{petId}"}` -> JSON operasi lengkap.
2. Project satu spec: `get_api_endpoint {operation_id:"healthCheck"}` tanpa `spec` berhasil.
3. Method huruf kecil `"get"` dinormalisasi; hasil `method:"GET"`.
4. Skema sirkular: `markers.circular` terisi dan `legend` menjelaskan `$circular`.
5. Skema dalam dengan `schema_depth:2`: bagian lebih dalam tampil `{"$ref": ...}` dan terdaftar di `markers.collapsed`; `truncated:false` (batas kedalaman diminta agent bukan pemotongan ukuran), tanpa hint.
6. Operasi raksasa `schema_depth:8` -> `truncated:true`, `schema_depth` efektif lebih kecil, hint menyarankan nilai tertentu; agent memanggil ulang dengan nilai itu dan mendapat hasil `truncated:false`.
7. Operasi super besar sehingga kedalaman 0 pun melebihi batas -> operasi `{"$truncated":true,...}` + hint "Operasi terlalu besar".

Negatif:
1. `{spec:"petstore"}` -> `Argumen tidak valid: method wajib diisi bersama path, atau isi operation_id. ...`.
2. method+path+operation_id -> `... pilih salah satu ...`.
3. `{method:"FETCH", path:"/pets"}` -> `method harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE (diterima: FETCH)`.
4. `{method:"GET", path:"pets"}` -> `path harus diawali /`.
5. `schema_depth:9` -> `schema_depth harus 1-8 (diterima: 9)`.
6. Project >1 spec tanpa `spec` -> `SpecMiss` dengan nama spec valid.
7. `spec` salah ketik -> `Spec tidak ditemukan: <nama>` + nama valid; project tanpa spec -> teks `belum punya API spec` + saran `list_files`.
8. Path salah ketik -> `Endpoint tidak ditemukan` + `Similar` + saran `list_api_endpoints`; tanpa similar -> hanya saran `Periksa ejaan ...`.
9. Argumen `project_id`/`projectId`/`project` -> pesan khusus; bus tidak dipanggil.
10. Provider tidak terdaftar / timeout / `Reason` tidak dikenal -> error Go; gateway menjawab `Terjadi kesalahan internal saat menjalankan tool get_api_endpoint. trace_id=...` atau teks timeout.

---

## 7. Request/Response body

### 7.1 `tools/call` sukses

Request:
```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_api_endpoint","arguments":{"spec":"petstore","method":"get","path":"/pets/{petId}"}}}
```
Bus request yang dikirim (K10; ProjectID dari gateway):
```json
{"ProjectID":"<uuid dari API key>","Spec":"petstore","Method":"GET","Path":"/pets/{petId}","OperationID":"","SchemaDepth":4,"MaxBytes":100352}
```
Response:
```json
{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"{\"spec\":\"petstore\",\"method\":\"GET\",\"path\":\"/pets/{petId}\",\"schema_depth\":4,\"truncated\":false,\"markers\":{\"circular\":[\"#/components/schemas/Pet\"],\"collapsed\":[],\"legend\":\"$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth\"},\"operation\":{\"operationId\":\"showPetById\",\"summary\":\"Info for a specific pet\",\"tags\":[\"pets\"],\"parameters\":[...],\"responses\":{\"200\":{...}},\"security\":[{\"apiKey\":[]}]},\"servers\":[{\"url\":\"https://petstore.example.com/v1\"}],\"securitySchemes\":{\"apiKey\":{\"type\":\"apiKey\",\"in\":\"header\",\"name\":\"X-API-Key\"}}}"}],"isError":false}}
```
`text` setelah di-unescape (kompak satu baris; di sini diformat agar terbaca):
```json
{"spec":"petstore","method":"GET","path":"/pets/{petId}","schema_depth":4,"truncated":false,
 "markers":{"circular":["#/components/schemas/Pet"],"collapsed":[],"legend":"$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth"},
 "operation":{"operationId":"showPetById","summary":"Info for a specific pet","tags":["pets"],"parameters":[...],"responses":{"200":{"content":{"application/json":{"schema":{"type":"object","properties":{"owner":{...},"children":{"type":"array","items":{"$circular":"#/components/schemas/Pet"}}}}}}}},"security":[{"apiKey":[]}]},
 "servers":[{"url":"https://petstore.example.com/v1"}],
 "securitySchemes":{"apiKey":{"type":"apiKey","in":"header","name":"X-API-Key"}}}
```
Audit (dibuat gateway): `action=mcp.get_api_endpoint`, `target_type=api_endpoint`, `target="petstore GET /pets/{petId}"`, result `{found:true, schema_depth:4, requested_depth:4, truncated:false, circular_count:1, collapsed_count:0, bytes:<len(text)>}`.

### 7.2 Endpoint tidak ketemu (`isError:true`)

Request: `{"name":"get_api_endpoint","arguments":{"spec":"petstore","method":"GET","path":"/pet/{petId}"}}`
```
Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}
Similar:
- GET /pets/{petId} — Info for a specific pet [showPetById]
- GET /pets — List all pets [listPets]
Gunakan salah satu di atas, atau list_api_endpoints(spec="petstore", query="pet") untuk melihat endpoint yang ada.
```
Dengan `operation_id` salah (tanpa Similar):
```
Endpoint tidak ditemukan di spec petstore: operation_id showPetByID
Periksa ejaan path atau operation_id, atau gunakan list_api_endpoints(spec="petstore", query="showPetByID") untuk melihat endpoint yang ada.
```
Summary: `{found:false, error_code:"endpoint_not_found", similar_count:2}`; `Target`: `petstore GET /pet/{petId}` (atau `petstore operation_id=showPetByID`).

### 7.3 Spec wajib / tidak ditemukan (`isError:true`, dari `base.SpecMiss`)

```
Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs.
```
```
Spec tidak ditemukan: petstor. Spec yang valid: billing, huge, petstore. Nama spec persis seperti di list_api_specs.
```
Summary: `{error_code:"spec_required", spec_count:3}` / `{error_code:"spec_not_found", spec_count:3}`; `TargetType:"api_spec"`, `Target:<spec diminta>`.

### 7.4 Terpotong karena ukuran

Request: `{"spec":"huge","method":"GET","path":"/report","schema_depth":8}`. Teks (ringkas, bagian operation dipangkas):
```json
{"spec":"huge","method":"GET","path":"/report","schema_depth":3,"truncated":true,
 "hint":"Respons melebihi batas 100352 byte; skema hanya dibuka sampai kedalaman 3 (diminta 8). Bagian lebih dalam tampil sebagai {\"$ref\": ...}. Turunkan schema_depth (mis. 3) agar respons ringkas dan konsisten.",
 "markers":{"circular":[],"collapsed":["#/components/schemas/Row"],"legend":"$circular: ...; $ref: ..."},
 "operation":{...},"servers":[],"securitySchemes":{}}
```
Kedalaman 0 dan terlalu besar:
```json
{"spec":"huge","method":"GET","path":"/report","schema_depth":0,"truncated":true,
 "hint":"Operasi terlalu besar bahkan tanpa membuka skema; hanya ringkasan yang ditampilkan. Gunakan list_api_endpoints untuk konteks dan minta potongan spec ke pemilik project.",
 "markers":{"circular":[],"collapsed":[],"legend":"$circular: ...; $ref: ...; $truncated: operasi terlalu besar, hanya ringkasan"},
 "operation":{"$truncated":true,"summary":"Laporan besar","operationId":"getReport"},"servers":[],"securitySchemes":{}}
```

### 7.5 Format error argumen (bus tidak dipanggil)

```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"Argumen tidak valid: method wajib diisi bersama path, atau isi operation_id. Cari endpoint dengan list_api_endpoints."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"Argumen tidak valid: method harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE (diterima: FETCH). Huruf besar/kecil bebas."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"Argumen tidak valid: operation_id tidak boleh diisi bersama method/path (pilih salah satu: method + path atau operation_id) (diterima: showPetById). Hapus operation_id atau hapus method dan path."}],"isError":true}}
```
```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"Argumen tidak valid: schema_depth harus 1-8 (diterima: 9). Turunkan nilai bila hasil terpotong; maksimum 8."}],"isError":true}}
```
Argumen project:
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```
Summary semua kasus ini: `{error_code:"invalid_argument", field:"<field>"}`; `TargetType:"api_endpoint"`.

Error internal / timeout (provider mati, `Reason` tidak dikenal, JSON provider rusak) dipetakan gateway T07 (detail disembunyikan, `trace_id`); usecase ini tidak mengubah teksnya.

---

## 8. Catatan risiko untuk implementer

- Pesan eksklusivitas bila `operation_id` ikut tertulis sebagai `(diterima: ...)`: `Got` diisi `Brief(operation_id)` pada cabang pertama; pada cabang lain `Got` kosong (argumen tidak ada).
- Config `MCPMaxResponseBytes` di bawah ~8 KiB menyisakan kurang dari ~2 KiB untuk pembungkus; marker dan hint pada kasus ekstrem masih bisa memicu pemotongan generik gateway. Ini diterima (default 102400, minimal 1024 hanya untuk tes); tidak ada tambahan logika.
- Bila provider T06 kelak mengubah penghitungan `MaxBytes` (tidak lagi menjumlah ketiga field), tes `TestT09_Int_AC5_TruncatedDepth` akan gagal lebih dulu; perbaiki `ProviderMaxBytes`/G4 di overview, bukan di sini.
