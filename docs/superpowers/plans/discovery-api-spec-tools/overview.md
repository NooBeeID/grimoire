# T09 discovery/api-spec-tools — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T09-discovery-api-spec-tools.md`
Sumber aturan: `rules.txt` (bagian 4, 5, 7.4, 9; menang untuk arsitektur), `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog aksi audit), kontrak T07 `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go`, kontrak T06 `backend/lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}`, pola module tool T08 `docs/superpowers/plans/discovery-knowledge-tools/overview.md`.
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/migrations.md` tidak relevan (T09 tanpa migrasi). Tidak ada knowledge doc module. Tiket menyatakan UI "Tidak ada", jadi tidak ada frontend plan dan tidak ada bagian Visual. Tidak ada OpenAPI REST, tabel, maupun aksi audit baru.

Dokumen ini adalah plan FOUNDATION: keputusan bersama, `base/`, kontrak, perubahan di luar module, urutan kerja, dan daftar usecase. Plan rinci per usecase (`usecase-<slug>.md` di folder ini) WAJIB mengikuti dokumen ini. Bila bertentangan, dokumen ini berlaku sampai diperbarui.

---

## 0. Prasyarat (gerbang F1)

| Prasyarat | Status saat plan ditulis (2026-10-21) | Cara cek |
|---|---|---|
| T07 ter-merge ke `main` | SUDAH (`35c4fa4`) | `git log main --oneline \| grep discovery-mcp-gateway` |
| T06 ter-merge, handler bus `project.apispec.{list-specs,list-endpoints,get-endpoint}` terdaftar | SUDAH (`0a26586`, `9e1a864`) | `grep -rn BUS_PROJECT_APISPEC backend/internal/domain/project` |
| Konstanta `BUS_PROJECT_APISPEC_LIST_SPECS`, `..._LIST_ENDPOINTS`, `..._GET_ENDPOINT` | SUDAH (`lib/constants/bus.go` baris 50-52) | `grep APISPEC backend/lib/constants/bus.go` |
| Aksi `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint` di `record.Actions()` | SUDAH | `grep ActionMCP backend/lib/contracts/core/audit/record/actions.go` |
| `Config.MCPMaxResponseBytes` (default 102400, minimal 1024) | SUDAH | `grep MCPMaxResponseBytes backend/internal/infra/config/config.go` |
| `discovery/base.go` punya komentar `// T09: tools = append(tools, apispectools.Register(d)...)` | SUDAH | baca file |

T08 (knowledge-tools) dikerjakan paralel dan BELUM ada di `main`. T09 TIDAK bergantung pada T08 dan TIDAK memakai perubahan `toolkit.Result.Structured` milik plan T08 (lihat K2). Satu-satunya titik konflik merge dengan T08 adalah `backend/internal/domain/discovery/base.go` (satu baris append masing-masing) dan `docs/mcp-gateway.md` / `docs/tickets/README.md` (bagian berbeda). Siapa pun yang merge belakangan me-rebase dan mempertahankan kedua baris.

Simbol yang dipakai:

| Simbol | Import path |
|---|---|
| `Deps{Config, Bus}` | `grimoire/internal/infra/app` |
| `CallTyped[T]`, `NewRegistry(log)`, `Handler`, `ErrNotRegistered` | `grimoire/internal/infra/bus` |
| `BUS_PROJECT_APISPEC_LIST_SPECS`, `..._LIST_ENDPOINTS`, `..._GET_ENDPOINT` | `grimoire/lib/constants` |
| `listspecs.{Request,Response,Spec}` | `grimoire/lib/contracts/project/apispec/listspecs` |
| `listendpoints.{Request,Response,Endpoint,DefaultLimit,MaxLimit,MaxQueryRunes}` | `grimoire/lib/contracts/project/apispec/listendpoints` |
| `getendpoint.{Request,Response,EndpointRef,DefaultSchemaDepth,MaxSchemaDepth,Reason*}` | `grimoire/lib/contracts/project/apispec/getendpoint` |
| `toolkit.{Tool,Handler,Call,Result}` | `grimoire/lib/contracts/discovery/mcpgateway/toolkit` |

Tidak ada library baru. `go.mod` tidak berubah.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T09 (ukuran S) |
| Scope | fullstack menurut workflow, tetapi tiket menyatakan UI "Tidak ada". Hanya backend |
| Domain / Module | discovery / api-spec-tools (package `apispectools`) |
| Resource root | `backend/internal/domain/discovery/internal/apispectools` |
| Bergantung pada | T06 (merged), T07 (merged) |
| Kontrak REST | Tidak ada. `api/**` tidak berubah |
| Kontrak bus disediakan | Tidak ada |
| Kontrak bus dikonsumsi | `project.apispec.list-specs`, `project.apispec.list-endpoints`, `project.apispec.get-endpoint` (semua Call sinkron) |
| Migrasi | Tidak ada |

Endpoint (milik T07; T09 hanya menambah tool di katalog):

| Endpoint | Method | Version | Metode JSON-RPC | Tool T09 |
|---|---|---|---|---|
| `/mcp` | `POST` | MCP protocol `2025-06-18` (T07); tanpa versi REST | `tools/list` | 3 entri baru: `get_api_endpoint`, `list_api_endpoints`, `list_api_specs` (gateway menambah `annotations.readOnlyHint:true`) |
| `/mcp` | `POST` | MCP protocol `2025-06-18` (T07); tanpa versi REST | `tools/call` | `params.name` salah satu tool di atas |

---

## 2. Summary

T09 menambah tiga tool MCP read-only untuk menelusuri API spec project secara bertahap: `list_api_specs` lalu `list_api_endpoints` lalu `get_api_endpoint`. Tiap tool memvalidasi argumen secara ketat, memanggil tepat satu endpoint bus T06 dengan `ProjectID` dari `toolkit.Call` (tidak pernah dari argumen), lalu merender teks ringkas untuk LLM plus metadata audit.

Pekerjaan berat (parsing OpenAPI, inline `$ref`, penanda `$circular`, penurunan kedalaman agar muat `MaxBytes`, pencarian endpoint mirip) SUDAH dilakukan provider T06. T09 hanya: validasi argumen, memilih anggaran byte, render teks, dan memetakan hasil "tidak ketemu" ke pesan `isError` yang memandu. Semua rule render dan validasi ada di `base/` sebagai fungsi/method murni. Repository tiap feature hanya `bus.CallTyped`. Tidak ada SQL, state, cache, goroutine, job, atau perubahan pada T07.

---

## 3. Keputusan bersama (mengikat ketiga usecase)

| ID | Keputusan | Alasan |
|---|---|---|
| K1 | Folder `register` dari tiket TIDAK dibuat sebagai feature. Isinya wiring saja: `apispectools/base.go` (`Register`) + satu baris di `discovery/base.go` | Tidak ada handler/usecase; sama dengan K1 T07 dan D12 T08. Menghindari pemecahan artifisial |
| K2 | T09 TIDAK memakai `structuredContent` dan TIDAK mengubah kontrak/kode T07. Hasil tool hanya `Text` + metadata audit | Tiket tidak meminta `structuredContent`. Menghindari ketergantungan pada perubahan D1 plan T08 yang belum merge |
| K3 | Batas import module (lebih ketat dari archtest, sesuai tiket dan rules 4.4): stdlib, `infra/app`, `infra/bus`, `lib/constants`, `lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}`, `lib/contracts/discovery/mcpgateway/toolkit`, package module sendiri. TIDAK memakai `lib/common/errs`. File `_test.go` boleh tambahan `infra/config` dan `lib/contracts/core/audit/record` | Tiket: "Hanya mengimpor lib/constants, lib/contracts/..., infra/bus" (`app` hanya struct wiring, `toolkit` adalah lib/contracts) |
| K4 | Karena `errs` tidak di-import, SEMUA error Go dari bus dikembalikan sebagai `fmt.Errorf("<tool>: %w", err)` (gateway memetakan ke `internal`/`timeout`). Konsekuensinya validasi argumen T09 WAJIB superset validasi provider, sehingga `errs.ErrInvalid` dari T06 tidak pernah tercapai dari input agent (dibuktikan tes K4 di tiap `params_test.go`) | Error invalid dari provider hanya mungkin karena bug T09, dan itu layak menjadi `internal` |
| K5 | Argumen ketat: schema `"additionalProperties": false`; `base.DecodeStrict` menolak field tak dikenal dan token sisa. `project_id`/`projectId`/`project` mendapat pesan khusus `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.` Bus tidak dipanggil. Project tidak pernah berubah (AC6) | rules 7.4; konsisten dengan D3 T08 sehingga agent melihat perilaku sama di semua tool |
| K6 | Nilai di luar rentang DITOLAK (`isError`), tidak di-clamp, konsisten dengan `minimum`/`maximum` schema. `list_api_endpoints`: limit 1-300 (default 100), offset 0-1000000 (default 0). `get_api_endpoint`: schema_depth 1-8 (default 4). String di-`TrimSpace`; ditolak bila memuat karakter kontrol (`<0x20`, `0x7f`) atau melewati panjang maksimum (tabel 5.2) | Provider melakukan clamp diam-diam; T09 memilih eksplisit agar LLM tahu batasnya |
| K7 | Format `ArgError` sama dengan D6 T08: `Argumen tidak valid: <field> <alasan> (diterima: <nilai ringkas>). <saran>`. Bagian `(diterima: ...)` dihilangkan bila argumen tidak ada. Hanya error pertama, urutan field sesuai schema. Summary `{error_code:"invalid_argument", field}` | rules 9.5, konsistensi lintas tool |
| K8 | Method: `TrimSpace` + `ToUpper`, harus salah satu `GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE`. Path: hanya `TrimSpace`, wajib diawali `/`, dikirim persis (termasuk `{param}`), tidak dinormalisasi lebih jauh (tidak membuang slash akhir, tidak lowercase) | Tiket: path harus persis seperti di spec. Daftar method sama dengan `base.Methods` T06 |
| K9 | Anggaran byte (fungsi murni di `base/budget.go`): `TextBudget(max) = max - 512`; `ProviderMaxBytes(max) = max - min(2048, max/4)`. Dengan default 102400: teks list maks 101888 byte, `getendpoint.Request.MaxBytes = 100352`. Dengan minimum 1024: 512 dan 768. Nilai dihitung sekali di `init.go` dari `d.Config.MCPMaxResponseBytes` | Gateway memotong teks di atas `MCPMaxResponseBytes` dengan penanda generik dan merusak JSON. T09 menjaga agar pemotongan gateway praktis tidak pernah terjadi; pemotongan dilakukan provider (kedalaman) atau entity list (baris) dengan penanda yang bermakna (rules 9.6) |
| K10 | `MaxBytes` dan `Limit` selalu dikirim eksplisit ke provider (tidak mengandalkan default provider) | Default provider `DefaultMaxBytes=102400` bisa lebih besar dari batas gateway bila config diturunkan |
| K11 | Bahasa teks hasil dan deskripsi tool: Indonesia; nama tool, argumen, dan kunci JSON tetap Inggris/apa adanya. Setiap teks sukses dan error menyebut langkah berikutnya | rules 9.5, K25 T07 |
| K12 | Teks dari spec (title, summary, tag) tidak tepercaya: diratakan satu baris (`OneLine`) dan dipotong (`Clip`, rune) sebelum masuk daftar. JSON operasi di `get_api_endpoint` diteruskan apa adanya (sudah di-marshal provider, aman sebagai JSON) | rules 7.9, mencegah satu summary panjang merusak format baris |
| K13 | Audit hanya oleh gateway (rules 9.3). Tool mengisi `TargetType`/`Target`/`Summary` dengan metadata saja (tabel 5.6). Gateway menambah `bytes=len(Text)` sendiri (`mcpgateway/base/audit.go`), sehingga "ukuran byte" pada tiket otomatis terpenuhi; tool tidak menulis kunci `bytes` | Tiket: metadata ringkas (jumlah endpoint, ukuran byte) |
| K14 | Pesan "spec tidak ditemukan / spec wajib" dibangun SATU helper bersama (`base.SpecMiss`) dan dipakai `list_api_endpoints` dan `get_api_endpoint` | Rule sama, pesan sama, satu tempat tes |
| K15 | Waktu: `FormatTime(t) = t.UTC().Format("2006-01-02 15:04 UTC")`, zero time menjadi `-` | Deterministik untuk tes |
| K16 | Tidak ada state, cache, goroutine, atau job. Semua tool idempoten | Tiket "module tipis" |

---

## 4. Struktur module

```
backend/internal/domain/discovery/internal/apispectools/
  base.go                       Register(d app.Deps) []toolkit.Tool
  base_test.go                  kontrak tools/list (golden + validator schema), allowlist import, jumlah tool
  testdata/tools_list.golden.json
  base/                         entity + rule murni, tanpa I/O
    names.go      names_test.go         (F3)
    args.go       args_test.go          (F3)
    format.go     format_test.go        (F3)
    budget.go     budget_test.go        (F3)
    specmiss.go   specmiss_test.go      (F3)
    catalog.go    catalog_test.go       (usecase listapispecs)
    endpoints.go  endpoints_test.go     (usecase listapiendpoints)
    detail.go     detail_test.go        (usecase getapiendpoint)
  listapispecs/      init.go params.go handler.go usecase.go repository.go params_test.go handler_test.go usecase_test.go
  listapiendpoints/  (sama)
  getapiendpoint/    (sama)
```

Lapisan dalam feature (arah `handler -> usecase -> repository`, semua memakai `base`):

| File | Isi | Dilarang |
|---|---|---|
| `init.go` | `func Init(d app.Deps) toolkit.Tool`. Merakit `newRepository(d.Bus)` lalu `newUsecase(repo)` lalu `newHandler(uc, budget...)`. Mengisi `Name` (dari `base`), `Title`, `Description` (teks bagian 12.1, persis), `InputSchema` (dari `params.go`). Membaca `d.Config.MCPMaxResponseBytes` sekali di sini | logika bisnis |
| `params.go` | struct argumen mentah (field pointer `*string`, `*int`), `func schema() json.RawMessage`, `func Parse(raw json.RawMessage) (Params, *base.ArgError)` | I/O |
| `handler.go` | adapter `toolkit.Handler`: `Parse` -> bila ArgError `base.ArgResult` -> `uc.Execute(ctx, call.ProjectID, p)` -> error Go dibungkus `fmt.Errorf("<tool>: %w", err)` -> `entity.Result()` | format teks sendiri, bus |
| `usecase.go` | `Execute(ctx, projectID string, p Params) (<entity>, error)`: panggil port repository dengan request lengkap (K10), bangun entity `base` | format teks, aturan rentang |
| `repository.go` | interface port kecil + implementasi `bus.CallTyped[contract.Response](ctx, b, constants.BUS_..., contract.Request{...})` (request dikirim sebagai NILAI) | logika |

`ProjectID` SELALU dari `call.ProjectID`, diteruskan handler ke `Execute`, lalu repository ke `Request.ProjectID`.

---

## 5. Base bersama (`base/`)

### 5.1 `names.go` (F3)

```go
const (
	ToolListAPISpecs     = "list_api_specs"
	ToolListAPIEndpoints = "list_api_endpoints"
	ToolGetAPIEndpoint   = "get_api_endpoint"
	TargetTypeSpec       = "api_spec"
	TargetTypeEndpoint   = "api_endpoint"
)

// ProjectArgAliases ditolak dengan pesan khusus (K5).
var ProjectArgAliases = []string{"project_id", "projectId", "project"}

// Methods urutan kanonik, sama dengan T06.
var Methods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}

// NormalizeMethod: TrimSpace + ToUpper; ok=false bila bukan anggota Methods.
func NormalizeMethod(s string) (string, bool)
```

Tes: nama cocok `^[a-z][a-z0-9_]{0,63}$` dan unik; `NormalizeMethod(" get ")=="GET"`, `"Patch"` ok, `"CONNECT"`, `"GETS"`, `""` ditolak.

### 5.2 `args.go` (F3)

Semantik identik dengan `knowledgetools/base/args.go` (D6 T08), ditulis ulang di sini karena rules 4.2 melarang import lintas module.

```go
type ArgError struct {
	Field, Reason, Got, Hint string // Got "" -> bagian (diterima: ...) dihilangkan
	Custom                   string // bila diisi, Message() mengembalikan ini apa adanya
}
func (e *ArgError) Message() string
func (e *ArgError) Summary() map[string]any // {"error_code":"invalid_argument","field":Field}

// ArgResult: toolkit.Result{Text: e.Message(), IsError: true, TargetType: targetType, Summary: e.Summary()}.
// Dipakai handler ketiga tool saat Parse gagal (bus tidak dipanggil).
func ArgResult(e *ArgError, targetType string) toolkit.Result

// DecodeStrict: raw kosong/spasi/"null" -> {}; bukan object -> Field "arguments" "harus object JSON";
// DisallowUnknownFields; token sisa -> "harus satu object JSON"; field alias project -> Custom K5;
// field lain tak dikenal -> Custom "Argumen tidak valid: <x> tidak dikenal. Argumen yang didukung: <allowed, koma>.";
// *json.UnmarshalTypeError -> Reason "harus string|integer"; sintaks rusak -> "bukan JSON valid".
func DecodeStrict(raw json.RawMessage, dst any, allowed []string) *ArgError

// IntArg: nil -> def; di luar [min,max] -> ArgError{Field, "harus <min>-<max>", Got, Hint}.
func IntArg(field string, v *int, def, min, max int, hint string) (int, *ArgError)

// StrArg: nil -> "" ; TrimSpace; karakter kontrol -> "memuat karakter kontrol";
// RuneCount > maxRunes -> "maksimal <n> karakter"; !utf8.ValidString -> "bukan UTF-8 valid".
func StrArg(field string, v *string, maxRunes int, hint string) (string, *ArgError)

func Brief(s string) string // maks 40 rune + "...", kontrol sebagai \uXXXX
```

Batas string (dipakai params tiap feature):

| Argumen | Tool | Maks | Alasan |
|---|---|---|---|
| `spec` | list_api_endpoints, get_api_endpoint | 80 rune | `NameMaxLen` T06 |
| `tag` | list_api_endpoints | 200 rune | wajar; tag dicocokkan persis oleh provider |
| `query` | list_api_endpoints | 200 rune (`listendpoints.MaxQueryRunes`) | ditolak, bukan dipotong diam-diam (K6) |
| `path` | get_api_endpoint | 2048 rune | wajar; path tetap dikirim persis |
| `operation_id` | get_api_endpoint | 200 rune | wajar |

Tes tabel: `""`/`"null"`/`" "` -> kosong tanpa error; `"[]"`, `"1"` -> `arguments`; `{"a":1}{"b":2}` token sisa; tiga alias project -> pesan memuat `ditentukan oleh API key`; `{"foo":1}` -> `foo tidak dikenal` + daftar allowed; `{"limit":"x"}` dan `{"limit":1.5}` -> `harus integer`; `{"spec":1}` -> `harus string`; `IntArg` nil/min/max/min-1/max+1; `StrArg` trim, `"a\tb"` ditolak, 81 rune multibyte ditolak dan 80 diterima; `Message()` dengan/tanpa Got/Hint; `Brief` 100 rune.

### 5.3 `format.go` (F3)

```go
func FormatTime(t time.Time) string     // K15
func OneLine(s string) string           // \r\n, \n, \r, \t -> spasi; spasi beruntun -> satu; TrimSpace
func Clip(s string, maxRunes int) string // > maxRunes -> potong + "..." (total maxRunes rune)
func JoinNames(names []string, max int) string // "a, b, c" ; > max -> "a, b, ... (+N lagi)"; kosong -> ""
```

Tes: zona `Asia/Jakarta` dirender UTC; `Clip` multibyte; `JoinNames` dengan 0, 3, dan 25 nama (max 20).

### 5.4 `budget.go` (F3)

```go
const textReserve = 512
func TextBudget(maxResponseBytes int) int       // max - 512, minimal 512
func ProviderMaxBytes(maxResponseBytes int) int // max - min(2048, max/4)
```

Tes: 102400 -> 101888 / 100352; 1024 -> 512 / 768; 4096 -> 3584 / 3072.

### 5.5 `specmiss.go` (F3, K14)

```go
// SpecMiss adalah hasil "spec tidak ada / spec wajib" untuk tool tool.
type SpecMiss struct {
	Tool      string   // tool pemanggil, untuk teks saran
	Requested string   // nama spec dari agent; "" bila spec tidak diisi
	Required  bool     // true = spec tidak diisi dan project punya != 1 spec
	Valid     []string // nama spec valid dari provider (sudah urut)
}
func (m SpecMiss) Text() string
func (m SpecMiss) Result() toolkit.Result
```

Teks (urutan cabang):
1. `len(Valid)==0`: `Project ini belum punya API spec. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.` Summary `{error_code:"no_specs"}`.
2. `Required`: `Project ini punya <N> API spec; isi argumen spec. Spec yang valid: <JoinNames(Valid,20)>. Lihat detailnya dengan list_api_specs.` Summary `{error_code:"spec_required", spec_count:N}`.
3. selain itu: `Spec tidak ditemukan: <Brief(Requested)>. Spec yang valid: <JoinNames(Valid,20)>. Nama spec persis seperti di list_api_specs.` Summary `{error_code:"spec_not_found", spec_count:N}`.

`Result()`: `IsError:true`, `TargetType:"api_spec"`, `Target:Requested`.

### 5.6 Entity per tool (dibuat di plan usecase; kontraknya dibekukan di sini)

| File | Entity | Konstruktor | Method wajib | TargetType / Target / Summary (K13) |
|---|---|---|---|---|
| `catalog.go` | `SpecCatalog{Items []listspecs.Spec, Budget int}` | `NewSpecCatalog(resp listspecs.Response, budget int)` | `Text()`, `Result()`, `EndpointTotal() int` | `api_spec` / `""` / `{count, endpoint_count, shown}` |
| `endpoints.go` | `EndpointPage{Filter, Offset, Total, Items []listendpoints.Endpoint, Budget}` dengan `Filter{Spec, Tag, Query}` | `NewEndpointPage(f Filter, offset int, resp listendpoints.Response, budget int) (EndpointPage, *SpecMiss)` (SpecMiss bila `!resp.SpecFound`) | `Lines() (text string, shown int)`, `NextOffset() int` (-1 bila habis), `Text()`, `Result()` (termasuk `offset_out_of_range`) | `api_spec` / `Filter.Spec` / `{count:shown, total, next_offset, spec_groups, truncated}` |
| `detail.go` | `EndpointDetail{Resp getendpoint.Response, RequestedDepth, MaxBytes int}`, `EndpointMiss{Spec string, Ask EndpointAsk, Similar []getendpoint.EndpointRef}`, `type Lookup interface{ Result() toolkit.Result }` | `NewLookup(ask EndpointAsk, requestedDepth, maxBytes int, resp getendpoint.Response) (Lookup, error)` (error hanya untuk `Reason` tak dikenal = pelanggaran kontrak) | `Text()`, `Result()`, `Markers() (circular, collapsed []string)` | `api_endpoint` / `<spec> <METHOD> <path>` / `{found:true, schema_depth, requested_depth, truncated, circular_count, collapsed_count}`; miss `{found:false, error_code:"endpoint_not_found", similar_count}` |

`EndpointAsk{Spec, Method, Path, OperationID string}` didefinisikan di `detail.go` dan dipakai `getapiendpoint/params.go`.

Handler cukup memanggil `entity.Result()`; semua rule (format, penanda, pesan) di entity. `Summary` DILARANG memuat summary/deskripsi spec, isi skema, atau query (query sudah tercatat di `Args` audit oleh gateway).

---

## 6. Kontrak bus yang dikonsumsi (tidak ada perubahan pada `lib/constants` atau `lib/contracts/**`)

| Konstanta | Request yang dikirim T09 | Response yang dipakai | Catatan |
|---|---|---|---|
| `BUS_PROJECT_APISPEC_LIST_SPECS` | `listspecs.Request{ProjectID: call.ProjectID}` | `Items[{Name,Title,ApiVersion,OpenapiVersion,EndpointCount,UpdatedAt}]` urut Name | Items kosong (bukan nil) untuk project tanpa spec |
| `BUS_PROJECT_APISPEC_LIST_ENDPOINTS` | `listendpoints.Request{ProjectID, Spec, Tag, Query, Limit (1-300, eksplisit), Offset}` | `SpecFound, Specs, Items[{Spec,Method,Path,OperationID,Summary,Tags,Deprecated}], Total, Truncated, NextOffset` | Urut Spec, Path, method kanonik, sehingga pengelompokan per spec cukup dengan memecah saat `Spec` berganti. `NextOffset` provider TIDAK dipakai langsung; entity menghitung ulang dari jumlah baris yang benar-benar dirender (K9) |
| `BUS_PROJECT_APISPEC_GET_ENDPOINT` | `getendpoint.Request{ProjectID, Spec, Method (uppercase), Path, OperationID, SchemaDepth (1-8), MaxBytes: ProviderMaxBytes(cfg)}` | `Found, Reason, Specs, Similar, Spec, Method, Path, Operation, Servers, SecuritySchemes, Truncated, EffectiveDepth, Hint` | Provider sudah: inline `$ref` internal, `{"$circular": ref}` untuk siklus, `{"$ref": ref}` bila melewati kedalaman, menurunkan kedalaman sampai muat `MaxBytes`, `{"$truncated":true, summary, operationId}` bila depth 0 pun terlalu besar, hanya skema yang dirujuk, `Servers` default `[]`, `SecuritySchemes` default `{}` |

Pemetaan error bus (K4): semua `err != nil` -> handler mengembalikan `toolkit.Result{}` dan `fmt.Errorf("<tool>: %w", err)`; `errors.Is` terhadap `context.DeadlineExceeded` tetap berlaku sehingga gateway memberi kode `timeout`.

Katalog bus README sudah mencantumkan T09 sebagai consumer ketiga endpoint; tidak perlu baris baru.

---

## 7. Migration

Tidak ada. T09 tidak membuat tabel, kolom, maupun index.

---

## 8. Plan (langkah berurutan)

**F1. Gerbang prasyarat.** Cek tabel bagian 0. Buat branch `feat/discovery-api-spec-tools` dari `main`.

**F2. (tidak ada perubahan T07).** Konfirmasi dengan `git diff main -- backend/internal/domain/discovery/internal/mcpgateway backend/lib/contracts/discovery` kosong sampai akhir.

**F3. `base/` bersama.** Buat `names.go`, `args.go`, `format.go`, `budget.go`, `specmiss.go` beserta tes (5.1-5.5). `go test ./internal/domain/discovery/internal/apispectools/base/...` hijau.

**F4. `apispectools/base.go`.**
```go
// Package apispectools menyediakan tool MCP API spec (T09) yang didaftarkan ke gateway T07 lewat toolkit.
package apispectools

// Register membangun tiga tool read-only. Tidak mendaftarkan endpoint bus dan tidak punya state.
func Register(d app.Deps) []toolkit.Tool {
	return []toolkit.Tool{listapispecs.Init(d), listapiendpoints.Init(d), getapiendpoint.Init(d)}
}
```
Selama feature belum selesai, `Init` boleh stub (nama + schema `{"type":"object"}` + handler yang mengembalikan error). Wajib terisi sebelum F9.

**F5. Usecase (paralel; hanya berbagi `base/`).**
- `listapispecs`: `usecase-listapispecs.md` (+ `base/catalog.go`).
- `listapiendpoints`: `usecase-listapiendpoints.md` (+ `base/endpoints.go`).
- `getapiendpoint`: `usecase-getapiendpoint.md` (+ `base/detail.go`).
Helper baru di `args.go`/`format.go` ditambahkan aditif beserta tes.

**F6. Wiring domain.** `backend/internal/domain/discovery/base.go`: ganti komentar `// T09: ...` dengan `tools = append(tools, apispectools.Register(d)...)` dan import `grimoire/internal/domain/discovery/internal/apispectools`. Baris/komentar T08 tidak disentuh.

**F7. Tes module.** `apispectools/base_test.go` + `testdata/tools_list.golden.json` (10.2). Jalankan sekali dengan `-update`, bandingkan manual dengan 12.1.

**F8. Tes integrasi.** `backend/cmd/internal/server/apispectools_integration_test.go` (`//go:build integration`), lihat 10.3.

**F9. Dokumen.**
- `docs/mcp-gateway.md`: bagian baru "Tool API spec": alur tiga langkah, tabel argumen, satu contoh `tools/call` per tool, penjelasan penanda `$circular`/`$ref`/`$truncated` dan K9.
- `docs/tickets/T09-discovery-api-spec-tools.md`: bagian `## Keputusan` (K1, K2, K4, K5, K6, K9).
- `docs/tickets/README.md`: status T09 di tabel tiket.

**F10. Gerbang akhir.**
```
cd backend
go build ./...
go vet ./...
go test ./...                                    # termasuk internal/archtest
go test -race ./internal/domain/discovery/...
go test -tags integration ./cmd/internal/server/...
golangci-lint run
```
Tidak ada stub `Init`. Golden stabil (dua kali jalan tanpa `-update` hijau). `git diff main --stat` tidak menyentuh `mcpgateway/`, `lib/contracts/`, `lib/constants/`, `api/`, `migrations/`, `frontend/`.

---

## 9. Changes (per file/folder)

| File | Status | Isi |
|---|---|---|
| `backend/internal/domain/discovery/base.go` | ubah | append `apispectools.Register(d)` + import |
| `backend/internal/domain/discovery/internal/apispectools/base.go` | baru | `Register` |
| `.../apispectools/base_test.go`, `testdata/tools_list.golden.json` | baru | kontrak tools/list, allowlist import, jumlah tool |
| `.../apispectools/base/{names,args,format,budget,specmiss}.go` (+ `_test.go`) | baru | base bersama (F3) |
| `.../apispectools/base/{catalog,endpoints,detail}.go` (+ `_test.go`) | baru | entity per tool (F5) |
| `.../apispectools/listapispecs/*` | baru | usecase `listapispecs` |
| `.../apispectools/listapiendpoints/*` | baru | usecase `listapiendpoints` |
| `.../apispectools/getapiendpoint/*` | baru | usecase `getapiendpoint` |
| `backend/cmd/internal/server/apispectools_integration_test.go` | baru | e2e AC1-AC7 |
| `docs/mcp-gateway.md` | ubah | bagian "Tool API spec" |
| `docs/tickets/T09-discovery-api-spec-tools.md` | ubah | bagian Keputusan |
| `docs/tickets/README.md` | ubah | status T09 |

TIDAK berubah: `lib/constants/bus.go`, `lib/contracts/**`, `internal/domain/discovery/internal/mcpgateway/**`, `internal/domain/project/**`, `infra/config`, `api/**`, `migrations/**`, `frontend/**`, `go.mod`.

---

## 10. Test Scenario

### 10.1 Unit

| Lapisan | Alat | Fokus |
|---|---|---|
| `base/*_test.go` | tabel, tanpa I/O | decode ketat, batas string/angka, format, anggaran, SpecMiss, render entity (baris, grup, penanda, budget, JSON detail, markers) |
| `<feature>/params_test.go` | tabel | default, rentang, tipe, alias project (AC6), normalisasi method, aturan eksklusif method+path/operation_id, dan TES K4: untuk setiap input yang ditolak provider (`method` tak valid, path tanpa `/`, hanya method, keduanya diisi, tidak ada keduanya) `Parse` mengembalikan ArgError |
| `<feature>/usecase_test.go` | fake port repository | request diteruskan persis: `ProjectID` dari `Execute`, `Limit`/`MaxBytes`/`SchemaDepth` eksplisit; error port diteruskan dengan `errors.Is` |
| `<feature>/handler_test.go` | `bus.NewRegistry(nil)` + `Register(constants.BUS_PROJECT_APISPEC_..., fake)` | bus TIDAK dipanggil bila argumen salah (counter 0); `req.(contract.Request).ProjectID == call.ProjectID` walau argumen memuat `project_id` (ditolak); provider error dan endpoint tidak terdaftar -> error Go (`errors.Is(err, bus.ErrNotRegistered)`); `context.DeadlineExceeded` tetap terbungkus; Summary tanpa teks spec |

### 10.2 Tes module `apispectools/base_test.go`

- `TestRegister_ThreeReadOnlyTools`: `Register(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: bus.NewRegistry(nil)})` -> 3 tool, nama `get_api_endpoint`, `list_api_endpoints`, `list_api_specs`, `Handler != nil`, `Title`/`Description` tidak kosong, `InputSchema` JSON valid.
- `TestT09_AC7_ToolsListContract`: proyeksi `{name,title,description,inputSchema}` urut nama, `inputSchema` di-unmarshal ke `any` lalu `json.MarshalIndent` 2 spasi, dibandingkan byte-per-byte dengan golden; flag `-update`.
- `TestT09_AC7_SchemasValid` (validator minimal tanpa dependensi): root `type=object`, `additionalProperties=false`, keyword root hanya `{type, properties, required, additionalProperties}`; tiap properti `type` di `{string, integer}`, `description` tidak kosong, keyword hanya `{type, description, default, minimum, maximum, minLength, maxLength}`; `minimum <= default <= maximum`; `required ⊆ properties`; tidak ada properti `project`/`project_id`/`projectId`; `"mcp."+name` ada di `record.Actions()`; tidak ada properti yang ditandai `required` pada `get_api_endpoint` (aturan eksklusif ditangani `Parse`).
- `TestT09_AC7_ImportAllowlist`: `filepath.WalkDir` dari direktori module, `parser.ParseFile(..., parser.ImportsOnly)` untuk setiap `.go`; import harus stdlib atau ada di allowlist K3 (tes boleh tambahan `infra/config`, `lib/contracts/core/audit/record`). Pesan gagal `<file>: import terlarang <path>`.

### 10.3 Integrasi (`backend/cmd/internal/server/apispectools_integration_test.go`, `//go:build integration`)

Setup memakai helper T07 di package yang sama (`setupMCP`, `e.rest`, `e.mcp`, `e.newKey`, `e.countAudit`, konstanta `initBody`). Tambah helper lokal:
- `uploadSpecs(t, e, projectID, files map[string]string)`: multipart field `files` ke `POST /api/projects/{projectID}/api-specs/upload` dengan cookie sesi admin dan header `X-Requested-With: grimoire`, harap 200;
- `callTool(t, e, key, name string, args any) (text string, isError bool)`: kirim `initialize` lalu `tools/call`, decode `result.content[0].text` dan `result.isError`.
- `newProjectWithKey(t, e, name string) (projID, rawKey string)`: `POST /api/projects` (harap 201) lalu `POST /api/projects/{id}/api-keys` (harap 201, ambil `raw_key`). Dipakai untuk project B karena `e.newKey` T07 selalu memakai `e.projID`. Helper T07 TIDAK diubah.
- Project A = project bawaan `setupMCP` (`e.projID`, `e.rawKey`). Nama spec = nama file tanpa ekstensi (aturan upload T06), jadi `petstore.yaml` menjadi `petstore`.

Fixture (YAML OpenAPI 3.0.3 di `testdata/` package server atau string konstanta di file tes):
- `petstore.yaml`: `GET /pets` (`listPets`, tag `pets`), `POST /pets` (`createPet`, tag `pets`, deprecated), `GET /pets/{petId}` (`showPetById`, tag `pets`, respons `Pet` yang merujuk `Owner` yang merujuk `Address`; `Pet.children` -> `Pet` (sirkular)); `components.schemas.Unused` tidak dirujuk; `servers` satu entri; `securitySchemes.apiKey` dipakai `GET /pets/{petId}`.
- `billing.yaml`: `GET /invoices` (`listInvoices`, tag `invoices`), `POST /invoices/{id}/pay` (tag `invoices`).
- `huge.yaml`: `GET /report` dengan skema bersarang 8 tingkat dan banyak properti sehingga hasil depth 8 > 100352 byte.
Project A: tiga spec di atas (AC1). Project B: satu spec `solo.yaml` (`GET /health`). Key asli T04 untuk A dan B.

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT09_Int_ToolsList` | 7 | `tools/list` | ada 3 tool T09, masing-masing `annotations.readOnlyHint:true` |
| `TestT09_Int_AC1_ListSpecs` | 1 | key A, `list_api_specs {}` | teks memuat `3 API spec`, `billing`, `huge`, `petstore`, versi, `OpenAPI 3.0.3`, jumlah endpoint; `isError:false` |
| `TestT09_Int_AC1_ListEndpointsGrouped` | 1 | `list_api_endpoints {}` | teks memuat `== billing ==`, `== huge ==`, `== petstore ==` berurutan; `POST /pets — ... [createPet] (pets) [deprecated]` |
| `TestT09_Int_AC1_TagAndQuery` | 1 | `{tag:"pets"}`; `{query:"invoice"}`; `{spec:"petstore", query:"petId"}` | hanya endpoint yang cocok; `Endpoint 1-1 dari 1` untuk kasus ketiga |
| `TestT09_Int_ListEndpointsPaged` | 1 | `{limit:2}` lalu `{limit:2, offset:2}` | halaman 1 memuat penanda `offset=2`; gabungan semua halaman = semua endpoint tanpa duplikat |
| `TestT09_Int_UnknownSpecList` | 1 | `{spec:"petstor"}` | `isError:true`, memuat `Spec tidak ditemukan: petstor` dan `billing, huge, petstore` |
| `TestT09_Int_AC2_NestedCircular` | 2 | `get_api_endpoint {spec:"petstore", method:"get", path:"/pets/{petId}"}` | `isError:false`; teks JSON valid; `operation` memuat properti `Owner` dan `Address` ter-inline, memuat `"$circular":"#/components/schemas/Pet"`, TIDAK memuat `Unused`; `markers.circular` berisi ref Pet; `servers` dan `securitySchemes.apiKey` ada |
| `TestT09_Int_AC2_ByOperationID` | 2 | `{spec:"petstore", operation_id:"showPetById"}` | hasil sama dengan kasus method+path (`method:"GET"`, `path:"/pets/{petId}"`) |
| `TestT09_Int_AC3_SpecRequired` | 3 | key A, `{method:"GET", path:"/pets"}` | `isError:true`, memuat `punya 3 API spec` dan ketiga nama |
| `TestT09_Int_AC3_SingleSpecOptional` | 3 | key B, `{method:"GET", path:"/health"}` | `isError:false`, `"spec":"solo"` |
| `TestT09_Int_AC4_Similar` | 4 | `{spec:"petstore", method:"GET", path:"/pet/{petId}"}` | `isError:true`, memuat `Endpoint tidak ditemukan`, `Similar:`, `GET /pets/{petId}`, `list_api_endpoints` |
| `TestT09_Int_AC5_TruncatedDepth` | 5 | `{spec:"huge", method:"GET", path:"/report", schema_depth:8}` | `isError:false`; JSON valid (tidak dipotong gateway: tidak memuat `[truncated: respons dipotong`); `"truncated":true`, `schema_depth < 8`, `hint` memuat `schema_depth`; `len(text) <= 102400` |
| `TestT09_Int_AC6_ProjectArgIgnoredForData` | 6 | key A: `list_api_specs {project_id:<B>}`, `list_api_endpoints {project_id:<B>}`, `get_api_endpoint {project_id:<B>, method:"GET", path:"/health"}` | ketiganya `isError:true` memuat `ditentukan oleh API key`, tidak memuat `solo`; tanpa `project_id`, `get_api_endpoint {spec:"solo", ...}` dengan key A -> `Spec tidak ditemukan` (project tetap A) |
| `TestT09_Int_AuditOneEntryPerCall` | K13 | satu `get_api_endpoint` sukses | tepat satu entri `mcp.get_api_endpoint`, `target_type=api_endpoint`, `target="petstore GET /pets/{petId}"`, result memuat `bytes` dan `schema_depth`, tidak memuat isi skema |

---

## 11. E2E Scenarios

Positif:
1. Agent: `list_api_specs {}` -> melihat `petstore`; `list_api_endpoints {spec:"petstore"}` -> melihat `GET /pets/{petId}`; `get_api_endpoint {spec:"petstore", method:"GET", path:"/pets/{petId}"}` -> JSON operasi lengkap.
2. Project satu spec: `get_api_endpoint {operation_id:"healthCheck"}` tanpa `spec` berhasil.
3. Spec besar: `list_api_endpoints {limit:100}` -> penanda `offset=100`; agent mengikuti sampai habis.
4. `get_api_endpoint` pada skema sirkular -> `$circular` + `markers.circular` + legend.
5. `get_api_endpoint` pada operasi raksasa -> `truncated:true` + hint `schema_depth`.
6. Project tanpa spec: `list_api_specs {}` -> `Belum ada API spec di project ini...` (`isError:false`).

Negatif:
1. `list_api_endpoints {limit:500}` -> `Argumen tidak valid: limit harus 1-300 (diterima: 500). ...`.
2. `get_api_endpoint {spec:"petstore"}` -> `Argumen tidak valid: method wajib diisi bersama path, atau isi operation_id. ...`.
3. `get_api_endpoint {spec:"petstore", method:"GET", path:"/pets", operation_id:"listPets"}` -> `pilih salah satu`.
4. `get_api_endpoint {method:"FETCH", path:"/pets"}` -> `method harus salah satu GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE`.
5. `get_api_endpoint {method:"GET", path:"pets"}` -> `path harus diawali /`.
6. `get_api_endpoint {..., schema_depth:9}` -> `schema_depth harus 1-8`.
7. `list_api_endpoints {offset:5000}` pada 12 endpoint -> `isError` `offset 5000 melewati total 12 endpoint` (`offset_out_of_range`).
8. Argumen `project_id` di tool mana pun -> pesan khusus, bus tidak dipanggil.
9. Provider tidak terdaftar / timeout -> error Go; gateway menjawab `Terjadi kesalahan internal saat menjalankan tool <name>. trace_id=...` atau teks timeout.

---

## 12. Request/Response body

### 12.1 `tools/list` (isi golden, config default)

Teks `description` dan deskripsi properti dibekukan dan disalin persis ke `init.go`/`params.go`. Golden yang dihasilkan `-update` (urut nama, kunci alfabetis) adalah acuan akhir.

```json
[
  {
    "name": "get_api_endpoint",
    "title": "Detail endpoint API",
    "description": "Detail satu operasi API (read-only) dalam JSON: parameter, request body, response, dengan skema $ref yang dibuka sampai schema_depth, servers, dan securitySchemes yang dipakai. Pilih operasi dengan method + path (path persis seperti di list_api_endpoints, termasuk {param}) ATAU operation_id. spec wajib bila project punya lebih dari satu spec. {\"$circular\": ref} = skema merujuk dirinya; {\"$ref\": ref} = tidak dibuka karena batas kedalaman. Bila endpoint tidak ketemu, gunakan saran Similar atau list_api_endpoints.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "spec": { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; boleh kosong bila project hanya punya satu spec" },
        "method": { "type": "string", "maxLength": 7, "description": "GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, atau TRACE (huruf besar/kecil bebas); wajib bersama path" },
        "path": { "type": "string", "maxLength": 2048, "description": "path persis seperti di spec, mis. /pets/{petId}; wajib bersama method" },
        "operation_id": { "type": "string", "maxLength": 200, "description": "operationId; alternatif dari method + path" },
        "schema_depth": { "type": "integer", "minimum": 1, "maximum": 8, "default": 4, "description": "kedalaman pembukaan $ref; turunkan bila hasil terpotong" }
      },
      "additionalProperties": false
    }
  },
  {
    "name": "list_api_endpoints",
    "title": "Daftar endpoint API",
    "description": "Daftar endpoint API (read-only), satu baris per operasi: METHOD /path — ringkasan [operationId] (tag). Tanpa spec = semua spec, dikelompokkan per spec. Saring dengan tag (persis) atau query (cocok sebagian pada path, ringkasan, operationId). Lanjutkan daftar terpotong dengan offset. Ambil detail dengan get_api_endpoint.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "spec": { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; kosong = semua spec" },
        "tag": { "type": "string", "maxLength": 200, "description": "tag persis (peka huruf besar/kecil)" },
        "query": { "type": "string", "maxLength": 200, "description": "kata yang dicari pada path, ringkasan, atau operationId" },
        "limit": { "type": "integer", "minimum": 1, "maximum": 300, "default": 100, "description": "jumlah endpoint per halaman" },
        "offset": { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah endpoint; pakai nilai dari penanda [ada N endpoint lagi ...]" }
      },
      "additionalProperties": false
    }
  },
  {
    "name": "list_api_specs",
    "title": "Daftar API spec",
    "description": "Titik awal menjelajah API project (read-only): daftar spec OpenAPI beserta judul, versi API, versi OpenAPI, jumlah endpoint, dan waktu diperbarui. Kolom name dipakai sebagai argumen spec pada list_api_endpoints dan get_api_endpoint.",
    "inputSchema": {
      "type": "object",
      "properties": {},
      "additionalProperties": false
    }
  }
]
```

### 12.2 `list_api_specs`

Request: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_api_specs","arguments":{}}}`

`result.content[0].text`:
```
3 API spec di project ini:
- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-20 08:15 UTC
- huge: Huge Report | versi 1.0.0 | OpenAPI 3.0.3 | 1 endpoint | diperbarui 2026-10-20 08:16 UTC
- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint | diperbarui 2026-10-20 08:17 UTC
Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.
```
Aturan: title `Clip(OneLine, 120)`, kosong -> `-`; versi kosong -> `-`. Bila baris berikutnya membuat teks melewati `Budget`, berhenti dan tambahkan `[<N> spec lain tidak ditampilkan; gunakan list_api_endpoints(spec=...) dengan nama yang sudah diketahui]`. `isError:false`. Summary `{count:3, endpoint_count:6, shown:3}`.

Project tanpa spec (`Items` kosong), `isError:false`, Summary `{count:0, endpoint_count:0, shown:0}`:
```
Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.
```
Header `1 API spec di project ini:` memakai angka apa adanya (tanpa bentuk jamak khusus).

### 12.3 `list_api_endpoints`

Request: `{"name":"list_api_endpoints","arguments":{"tag":"pets","limit":2}}`

Text:
```
Endpoint 1-2 dari 3 | spec: semua | tag: pets
== petstore ==
GET /pets — List all pets [listPets] (pets)
POST /pets — Create a pet [createPet] (pets) [deprecated]

[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]
Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id).
```
Aturan entity `EndpointPage`:
- Header: `Endpoint <Offset+1>-<Offset+shown> dari <Total> | spec: <Spec atau "semua">` + ` | tag: <tag>` bila diisi + ` | query: "<query>"` bila diisi.
- Header grup `== <spec> ==` setiap kali `Spec` berganti (juga saat `spec` diisi). Antar grup tidak ada baris kosong tambahan.
- Baris: `<METHOD> <Path>` + ` — <Clip(OneLine(Summary),160)>` bila ada + ` [<OperationID>]` bila ada + ` (<tags dipisah ", ">)` bila ada + ` [deprecated]` bila Deprecated.
- Baris dirender selama `len(teks)+len(baris)+cadangan penutup (200) <= Budget`; `shown` = baris yang masuk; `NextOffset = Offset+shown` bila `Offset+shown < Total`, selain itu -1.
- Penanda (dipisah satu baris kosong) hanya bila `NextOffset >= 0`: `[ada <Total-Offset-shown> endpoint lagi, panggil list_api_endpoints dengan offset=<NextOffset> dan argumen lain yang sama]`.
- `Total==0`: `Tidak ada endpoint yang cocok (spec: ..., tag: ..., query: "..."). Hapus filter tag/query, periksa nama tag dari list_api_endpoints tanpa filter, atau lihat spec dengan list_api_specs.` (`isError:false`). Bila tanpa filter dan tanpa spec: `Belum ada API spec di project ini. ...` (teks sama dengan SpecMiss cabang 1, tetapi `isError:false`).
- `Total>0 && len(Items)==0`: `isError:true`, `Argumen tidak valid: offset <Offset> melewati total <Total> endpoint. Gunakan offset 0 sampai <Total-1>.` Summary `{error_code:"offset_out_of_range", total}`.
- `!SpecFound` -> `SpecMiss{Tool, Requested: spec, Required:false, Valid: resp.Specs}.Result()`.

Summary sukses: `{count:shown, total, next_offset (int atau -1), spec_groups, truncated: NextOffset>=0}`.

### 12.4 `get_api_endpoint` (sukses)

Request: `{"name":"get_api_endpoint","arguments":{"spec":"petstore","method":"get","path":"/pets/{petId}"}}`

Text (satu object JSON kompak; di sini diformat agar mudah dibaca; urutan kunci mengikuti struct):
```json
{"spec":"petstore","method":"GET","path":"/pets/{petId}","schema_depth":4,"truncated":false,
 "markers":{"circular":["#/components/schemas/Pet"],"collapsed":[],"legend":"$circular: skema merujuk dirinya dan tidak dibuka lagi; $ref: tidak dibuka karena batas schema_depth"},
 "operation":{"operationId":"showPetById","summary":"Info for a specific pet","tags":["pets"],"parameters":[...],"responses":{"200":{"content":{"application/json":{"schema":{"type":"object","properties":{"owner":{...},"children":{"type":"array","items":{"$circular":"#/components/schemas/Pet"}}}}}}}},"security":[{"apiKey":[]}]},
 "servers":[{"url":"https://petstore.example.com/v1"}],
 "securitySchemes":{"apiKey":{"type":"apiKey","in":"header","name":"X-API-Key"}}}
```
Aturan entity `EndpointDetail`:
- Marshal struct `detailJSON{Spec, Method, Path, SchemaDepth (EffectiveDepth), Truncated, Hint (omitempty), Markers (*markers, omitempty), Operation, Servers, SecuritySchemes json.RawMessage}` dengan `json.Marshal` (kompak, tanpa HTML escape: pakai `json.Encoder` + `SetEscapeHTML(false)`, buang newline akhir).
- `Markers()`: decode `Operation`, `Servers`, `SecuritySchemes` ke `any`, telusuri rekursif; kumpulkan nilai string kunci `$circular` (circular) dan kunci `$ref` (collapsed), unik, urut, maks 20 per daftar. `markers` dihilangkan bila keduanya kosong dan tidak ada `$truncated`.
- `Truncated` dan `Hint`: bila `Resp.Truncated`: `Hint = "Respons melebihi batas <MaxBytes> byte; skema hanya dibuka sampai kedalaman <EffectiveDepth> (diminta <RequestedDepth>). Bagian lebih dalam tampil sebagai {\"$ref\": ...}. Turunkan schema_depth (mis. <max(1,EffectiveDepth)>) agar respons ringkas dan konsisten."`. Bila `EffectiveDepth==0` dan `Operation` memuat `$truncated`: `Hint = "Operasi terlalu besar bahkan tanpa membuka skema; hanya ringkasan yang ditampilkan. Gunakan list_api_endpoints untuk konteks dan minta potongan spec ke pemilik project."`
- Summary `{found:true, schema_depth:EffectiveDepth, requested_depth, truncated, circular_count, collapsed_count}`; Target `"<Spec> <Method> <Path>"`.

### 12.5 `get_api_endpoint` (tidak ketemu)

`ENDPOINT_NOT_FOUND`:
```
Endpoint tidak ditemukan di spec petstore: GET /pet/{petId}
Similar:
- GET /pets/{petId} — Info for a specific pet [showPetById]
- GET /pets — List all pets [listPets]
Gunakan salah satu di atas, atau list_api_endpoints(spec="petstore", query="pet") untuk melihat endpoint yang ada.
```
- Baris pertama memakai `operation_id <id>` bila agent memakai operation_id: `Endpoint tidak ditemukan di spec petstore: operation_id showPetByID`.
- `Similar` kosong -> bagian `Similar:` dihilangkan.
- Saran `query` = segmen path pertama non-param (mis. `pet`) atau `operation_id` yang diminta, `Clip` 40 rune; bila tidak ada, saran tanpa `query`.
- `isError:true`, TargetType `api_endpoint`, Target `"<spec> <METHOD> <path>"` atau `"<spec> operation_id=<id>"`, Summary `{found:false, error_code:"endpoint_not_found", similar_count}`.

`SPEC_REQUIRED` / `SPEC_NOT_FOUND` -> `SpecMiss{Tool:"get_api_endpoint", Requested: ask.Spec, Required: Reason==SPEC_REQUIRED, Valid: resp.Specs}.Result()`. Contoh (key A, tanpa spec):
```
Project ini punya 3 API spec; isi argumen spec. Spec yang valid: billing, huge, petstore. Lihat detailnya dengan list_api_specs.
```
`Reason` lain (tidak dikenal) -> `NewLookup` mengembalikan error, handler mengembalikan error Go (pelanggaran kontrak T06).

### 12.6 Format error umum

Argumen tidak valid (bus tidak dipanggil):
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya."}],"isError":true}}
```
Argumen project:
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```
Error internal / timeout: dipetakan gateway T07 (detail disembunyikan, `trace_id`), T09 tidak mengubah teksnya.

---

## 13. Daftar usecase

| Slug | Folder | Tool MCP | Bus | Entity base | Plan |
|---|---|---|---|---|---|
| `listapispecs` | `apispectools/listapispecs/` | `list_api_specs` | `project.apispec.list-specs` | `base/catalog.go` (`SpecCatalog`) | `usecase-listapispecs.md` |
| `listapiendpoints` | `apispectools/listapiendpoints/` | `list_api_endpoints` | `project.apispec.list-endpoints` | `base/endpoints.go` (`EndpointPage`, memakai `SpecMiss`) | `usecase-listapiendpoints.md` |
| `getapiendpoint` | `apispectools/getapiendpoint/` | `get_api_endpoint` | `project.apispec.get-endpoint` | `base/detail.go` (`EndpointDetail`, `EndpointMiss`, `EndpointAsk`, memakai `SpecMiss`) | `usecase-getapiendpoint.md` |

`register` (tabel features tiket) bukan usecase; fungsinya dijalankan F4 + F6 (K1).
