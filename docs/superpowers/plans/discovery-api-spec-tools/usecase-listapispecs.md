# T09 usecase `listapispecs` — Tool MCP `list_api_specs`

Tiket: `docs/tickets/T09-discovery-api-spec-tools.md`
Foundation (mengikat, menang bila bertentangan): `docs/superpowers/plans/discovery-api-spec-tools/overview.md`
Keputusan bersama yang dipakai: K1 (tanpa feature `register`), K2 (tanpa `structuredContent`), K3 (batas import), K4 (error bus menjadi error Go), K5 (argumen ketat, alias project), K7 (format `ArgError`), K9 (`TextBudget`), K11 (bahasa), K12 (teks spec tidak tepercaya), K13 (audit hanya metadata), K15 (`FormatTime`), K16 (tanpa state).

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` tidak relevan (tanpa migrasi); tidak ada knowledge doc module; tiket menyatakan UI "Tidak ada", jadi tidak ada frontend plan dan tidak ada bagian Visual. Kontrak T06 `backend/lib/contracts/project/apispec/listspecs` sudah di `main`: `Request{ProjectID}`, `Response{Items []Spec}` urut `Name`, `Spec{Name, Title, ApiVersion, OpenapiVersion string; EndpointCount int; UpdatedAt time.Time}`; project tanpa spec menghasilkan `Items` kosong (bukan nil). `Deps.Bus` bertipe `*bus.Registry`; `bus.CallTyped[T any](ctx, b *Registry, endpoint string, req any) (T, error)`; konstanta `constants.BUS_PROJECT_APISPEC_LIST_SPECS` sudah ada.

---

## Metadata

| Atribut | Nilai |
|---|---|
| Endpoint | `/mcp` (milik T07; tidak ada endpoint REST baru, `api/**` tidak berubah) |
| Method | `POST`, JSON-RPC 2.0 `tools/call` dengan `params.name = "list_api_specs"` (tampil juga di `tools/list`) |
| Version | MCP protocol `2025-06-18` (dinegosiasikan T07); tanpa versi REST |
| Scope | backend saja (workflow menyebut fullstack, tiket UI "Tidak ada"); `endpointsChanged = false` |
| Domain | discovery |
| Module | api-spec-tools (package `apispectools`) |
| Feature | `listapispecs` |
| Resource root | `backend/internal/domain/discovery/internal/apispectools` |
| Bus dikonsumsi | `project.apispec.list-specs` (Call sinkron, provider T06) via `constants.BUS_PROJECT_APISPEC_LIST_SPECS` |
| Bus disediakan | tidak ada |
| Aksi audit | `mcp.list_api_specs` (sudah ada di `record.Actions()`; ditulis gateway T07, bukan tool ini) |
| Migrasi | tidak ada |
| Gerbang | T06 dan T07 sudah di `main` (overview bagian 0). F3 overview (`base/{names,args,format,budget}.go`) harus selesai lebih dulu |

---

## Summary Plan

`list_api_specs` adalah titik awal penelusuran API: tanpa argumen, mengembalikan daftar spec project (nama, judul, versi API, versi OpenAPI, jumlah endpoint, waktu diperbarui) sebagai teks ringkas, lalu menyarankan langkah berikut `list_api_endpoints(spec=...)`.

Alur: handler memanggil `Parse` (tolak SEMUA argumen lewat `DecodeStrict` dengan allowlist kosong; alias `project_id`/`projectId`/`project` mendapat pesan khusus). Bila lolos, usecase memanggil port repository (satu `bus.CallTyped` ke T06 dengan `ProjectID` dari `call.ProjectID`) lalu membangun entity `base.SpecCatalog`. Entity itulah yang merender teks, menjaga budget byte, memberi penanda spec yang tidak ditampilkan, kasus kosong, dan metadata audit. Error bus tidak dipetakan menjadi `isError`; dibungkus `fmt.Errorf("list_api_specs: %w", err)` (K4) sehingga gateway menjawab `internal`/`timeout`.

Poin desain:
- Satu usecase, tidak dipecah. Rule (header, format baris, `-` untuk kosong, guard budget, penanda sisa, kasus kosong, ringkasan audit) ada di `SpecCatalog`, bukan di usecase atau handler.
- Tidak ada `Limit`/`MaxBytes` pada kontrak `listspecs`; jumlah spec per project kecil, namun guard budget tetap wajib karena judul/nama berasal dari file yang diunggah (K12).
- Tidak ada aturan rentang atau validasi nilai karena tidak ada argumen; seluruh validasi = "tidak ada argumen".
- Tidak ada state, cache, goroutine, SQL, atau audit di tool.

Deviasi kecil dari overview bagian 4 (disengaja): overview menulis `newHandler(uc, budget...)`, tetapi karena entity dibangun di usecase (rule: usecase membangun entity, handler hanya `Result()`), anggaran byte diberikan ke `newUsecase(repo, budget)`. Handler feature ini tidak butuh budget. Perilaku luar tidak berubah.

---

## Plan (langkah berurutan)

Prasyarat: F1 (gerbang) dan F3 overview selesai: `base/names.go`, `args.go`, `format.go`, `budget.go` (+ tes) tersedia, plus `apispectools/base.go` (F4, `Init` boleh stub). Semua jalur relatif terhadap `backend/internal/domain/discovery/internal/apispectools`. Kerjakan TDD: tulis tes dulu per file.

**S0. Penyesuaian aditif `base/args.go` (hanya bila belum ada di F3).** `DecodeStrict(raw, dst, allowed)` dengan `allowed` kosong (`len==0`) harus menghasilkan untuk field tak dikenal non-project:
`Argumen tidak valid: <field> tidak dikenal. Tool ini tidak menerima argumen.` (bukan daftar kosong). Tambah satu kasus di `base/args_test.go`. Perubahan aditif; tidak mengubah perilaku untuk `allowed` non-kosong.

**S1. Entity `base/catalog.go`** (tulis `base/catalog_test.go` dulu).

```go
package base

import (
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

// SpecCatalog adalah hasil list_api_specs. Murni, tanpa I/O.
type SpecCatalog struct {
	Items  []listspecs.Spec // urutan persis dari T06 (Name)
	Budget int              // batas byte teks, dari TextBudget(cfg)
}

func NewSpecCatalog(resp listspecs.Response, budget int) SpecCatalog
func (c SpecCatalog) EndpointTotal() int          // jumlah EndpointCount SEMUA item (termasuk yang tidak tampil)
func (c SpecCatalog) Text() string                // memanggil render()
func (c SpecCatalog) Result() toolkit.Result
// tak diekspor: render() (text string, shown int)
```

Konstanta tak diekspor: `specNameMaxRunes = 80`, `specTitleMaxRunes = 120`, `specVersionMaxRunes = 40`.

Aturan:

1. `NewSpecCatalog`: menyalin `resp.Items` (nil aman, diperlakukan kosong) dan `Budget`.
2. `EndpointTotal()`: penjumlahan `EndpointCount`. Dipakai Summary agar `endpoint_count` mencakup spec yang tidak tampil.
3. Kasus kosong (`len(Items)==0`): `Text()` = `Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.` `Result().IsError == false`. Summary `{count:0, endpoint_count:0, shown:0}`.
4. Format teks (urutan tetap, pemisah `\n`, tanpa baris kosong, tanpa newline akhir):
   - Header: `<N> API spec di project ini:` dengan `N = len(Items)` apa adanya (tanpa bentuk jamak khusus).
   - Satu baris per spec: `- <name>: <title> | versi <api_version> | OpenAPI <openapi_version> | <N> endpoint | diperbarui <UTC>`.
     - `name` = `Clip(OneLine(Name), 80)`. `title` = `Clip(OneLine(Title), 120)`, kosong menjadi `-`. `api_version` dan `openapi_version` = `Clip(OneLine(x), 40)`, kosong menjadi `-`. `N` = `EndpointCount`. Waktu = `FormatTime(UpdatedAt)` (K15; zero time menjadi `-`).
     - `Name` TIDAK pernah di-Clip hingga berbeda dari nama aslinya dalam keadaan normal (batas nama T06 = 80 rune, jadi Clip tidak berpengaruh pada data valid; Clip hanya pengaman).
   - Penanda sisa, hanya bila ada spec yang tidak ditampilkan: `[<R> spec lain tidak ditampilkan; gunakan list_api_endpoints(spec=...) dengan nama yang sudah diketahui]` dengan `R = len(Items)-shown`.
   - Penutup (selalu, baris terakhir): `Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.`
5. Guard budget (`render()`), algoritma eksak:
   - `text := header`; `footer := "\n" + penutup`; hitung dengan `len()` (byte, bukan rune).
   - Untuk tiap item `i` (0-based) berurutan: `line := "\n" + barisSpec(i)`; `rest := len(Items)-i-1`; `reserve := 0` bila `rest==0`, selain itu `len("\n" + penanda(rest))` (penanda dengan `R=rest`; ini batas atas karena digit `R` terbesar saat `rest` terbesar... dihitung eksak per iterasi dengan `rest` aktual).
   - Masukkan baris bila `len(text)+len(line)+reserve+len(footer) <= Budget`; jika tidak, berhenti (`break`, jangan lanjut mencoba item lebih pendek, agar urutan nama tetap berurutan tanpa lompatan).
   - `shown` = jumlah baris yang masuk. Bila `shown < len(Items)`: tambahkan `"\n" + penanda(len(Items)-shown)`. Lalu tambahkan `footer`.
   - Bila `Budget <= 0` (entity dibuat langsung di tes): diperlakukan tanpa batas.
   - `shown == 0` dengan `Items` tidak kosong dimungkinkan hanya bila budget amat kecil; teks tetap valid (header + penanda + penutup) dan `isError:false`.
6. `Result()`:
   - normal dan kosong: `toolkit.Result{Text: Text(), IsError: false, TargetType: TargetTypeSpec ("api_spec"), Target: "", Summary: {"count": len(Items), "endpoint_count": EndpointTotal(), "shown": shown}}`.
   - Summary DILARANG memuat nama/judul spec (hanya angka). `bytes` tidak ditulis tool; gateway menambah `bytes=len(Text)` (K13).
7. Tidak ada kasus `isError:true` di entity ini.

**S2. `listapispecs/params.go`** (tulis `params_test.go` dulu).

```go
package listapispecs

// rawArgs sengaja kosong: DecodeStrict menolak setiap field.
type rawArgs struct{}

// Params tidak memuat apa pun (tool tanpa argumen). Tidak ada project di sini.
type Params struct{}

var allowedFields = []string{} // kosong: tool tanpa argumen

func schema() json.RawMessage // lihat "InputSchema"
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var a rawArgs
	if e := base.DecodeStrict(raw, &a, allowedFields); e != nil {
		return Params{}, e
	}
	return Params{}, nil
}
```

Perilaku `Parse` (semua via `DecodeStrict`):
- `nil`, `""`, `" "`, `"null"`, `"{}"` diterima.
- `{"project_id":"x"}`, `{"projectId":"x"}`, `{"project":"x"}` ditolak dengan `Custom`: `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.` (K5; nama pada pesan mengikuti kunci yang dikirim: `projectId`/`project`). Bus tidak dipanggil.
- `{"foo":1}` ditolak: `Argumen tidak valid: foo tidak dikenal. Tool ini tidak menerima argumen.`
- `{"spec":"petstore"}` / `{"limit":10}` (argumen tool lain yang sering salah dipakai) ditolak dengan pesan yang sama berbentuk `... spec tidak dikenal. Tool ini tidak menerima argumen.`
- `[]`, `1`, `"x"` ditolak: `Argumen tidak valid: arguments harus object JSON ...`.
- `{}{"a":1}` (token sisa) ditolak; `{` (sintaks rusak) ditolak `bukan JSON valid`.
- `{"a":null}` tetap ditolak (field tak dikenal, bukan diabaikan).

`InputSchema` (dibekukan; sama dengan overview 12.1, kunci diserialisasi apa adanya):

```json
{"type":"object","properties":{},"additionalProperties":false}
```
Tanpa `required`. Tidak ada properti project. `schema()` mengembalikan konstanta `json.RawMessage` ini.

**S3. `listapispecs/repository.go`**

```go
// SpecLister adalah port ke provider API spec (T06). Satu-satunya tempat bus dipanggil.
type SpecLister interface {
	ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error)
}

type busSpecLister struct{ b *bus.Registry }

func newRepository(b *bus.Registry) SpecLister { return busSpecLister{b: b} }

func (r busSpecLister) ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error) {
	return bus.CallTyped[listspecs.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_LIST_SPECS, req)
}
```
Request dikirim sebagai NILAI (kontrak). Error dikembalikan apa adanya (termasuk `bus.ErrNotRegistered`, `bus.ErrBadResponseType`, `context.DeadlineExceeded`). Tanpa logika, tanpa SQL.

**S4. `listapispecs/usecase.go`**

```go
type usecase struct {
	specs  SpecLister
	budget int // base.TextBudget(MCPMaxResponseBytes)
}

func newUsecase(s SpecLister, budget int) usecase { return usecase{specs: s, budget: budget} }

// Execute: orkestrasi saja.
func (u usecase) Execute(ctx context.Context, projectID string, _ Params) (base.SpecCatalog, error) {
	resp, err := u.specs.ListSpecs(ctx, listspecs.Request{ProjectID: projectID})
	if err != nil {
		return base.SpecCatalog{}, err // diteruskan; handler membungkus (K4)
	}
	return base.NewSpecCatalog(resp, u.budget), nil
}
```
Tidak ada aturan format, penilaian "kosong", atau penghitungan budget di sini.

**S5. `listapispecs/handler.go`**

```go
func newHandler(u usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, base.TargetTypeSpec), nil // bus TIDAK dipanggil
		}
		cat, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolListAPISpecs, err)
		}
		return cat.Result(), nil
	}
}
```
Handler tidak memformat teks, tidak menyentuh bus langsung, dan `ProjectID` hanya dari `call.ProjectID`.

**S6. `listapispecs/init.go`**

```go
const (
	title       = "Daftar API spec"
	description = "Titik awal menjelajah API project (read-only): daftar spec OpenAPI beserta judul, versi API, versi OpenAPI, jumlah endpoint, dan waktu diperbarui. Kolom name dipakai sebagai argumen spec pada list_api_endpoints dan get_api_endpoint."
)

func Init(d app.Deps) toolkit.Tool {
	budget := base.TextBudget(d.Config.MCPMaxResponseBytes) // dihitung sekali (K9)
	h := newHandler(newUsecase(newRepository(d.Bus), budget))
	return toolkit.Tool{
		Name:        base.ToolListAPISpecs,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     h,
	}
}
```
`description` dibekukan dan identik dengan overview 12.1 serta golden `testdata/tools_list.golden.json` (golden dimiliki foundation F7). Tool tidak mendaftarkan endpoint bus dan tidak menulis audit.

**S7. Wiring.** Ganti stub `listapispecs.Init` pada `apispectools/base.go` (F4 overview) dengan implementasi ini. Tidak ada perubahan di `discovery/base.go` (F6 foundation), `mcpgateway/`, `lib/contracts/`, `lib/constants/`.

**S8. Tes integrasi** pada `backend/cmd/internal/server/apispectools_integration_test.go` (file milik foundation F8; tambahkan tes `list_api_specs` di sini sesuai Test Scenario). Tambahan di luar tabel overview 10.3: satu tes project tanpa spec.

**S9. Dokumen.** Kontribusi ke F9 foundation: bagian "Tool API spec" pada `docs/mcp-gateway.md` memuat contoh `tools/call` `list_api_specs` (bagian Request/Response di bawah). Tidak ada edit tiket/README oleh usecase ini.

---

## Migration

Tidak ada. Tidak ada tabel, kolom, atau index baru.

---

## Changes

| File | Status | Isi |
|---|---|---|
| `apispectools/base/catalog.go` | baru | `SpecCatalog`, `NewSpecCatalog`, `EndpointTotal`, `Text`, `Result`, `render` |
| `apispectools/base/catalog_test.go` | baru | tes entity (lihat Test Scenario) |
| `apispectools/base/args.go` + `args_test.go` | ubah (aditif, hanya bila F3 belum mencakup) | pesan `Tool ini tidak menerima argumen.` untuk `allowed` kosong |
| `apispectools/listapispecs/init.go` | baru | `Init`, `title`, `description` |
| `apispectools/listapispecs/params.go` | baru | `rawArgs`, `Params`, `schema`, `Parse` |
| `apispectools/listapispecs/handler.go` | baru | adapter `toolkit.Handler` |
| `apispectools/listapispecs/usecase.go` | baru | `Execute` |
| `apispectools/listapispecs/repository.go` | baru | port `SpecLister` + implementasi bus |
| `apispectools/listapispecs/{params,usecase,handler}_test.go` | baru | tes feature |
| `apispectools/base.go` | ubah | ganti stub `listapispecs.Init` dengan implementasi (dibuat F4) |
| `backend/cmd/internal/server/apispectools_integration_test.go` | ubah | tambah tes `list_api_specs` |

TIDAK berubah: `lib/constants/bus.go`, `lib/contracts/**`, `mcpgateway/**`, `project/**`, `infra/config`, `api/**`, `migrations/**`, `frontend/**`, `go.mod`, `discovery/base.go`.

Batas import file produksi feature (K3): stdlib, `grimoire/internal/infra/app`, `grimoire/internal/infra/bus`, `grimoire/lib/constants`, `grimoire/lib/contracts/project/apispec/listspecs`, `grimoire/lib/contracts/discovery/mcpgateway/toolkit`, `apispectools/base`. TIDAK memakai `lib/common/errs`. File `_test.go` boleh tambahan `infra/config` dan `lib/contracts/core/audit/record`. `base/catalog.go` hanya mengimpor stdlib, `toolkit`, `listspecs` (tidak mengimpor package feature, mencegah siklus).

---

## Test Scenario

### Unit: `base/catalog_test.go` (tabel, tanpa I/O)

Helper tes: `mkSpec(name, title, apiV, oasV string, n int, t time.Time) listspecs.Spec`; `mkItems(n int)` menghasilkan spec `s01..sNN` dengan `UpdatedAt` tetap `2026-10-20 08:15 UTC`.

| Tes | Input | Ekspektasi |
|---|---|---|
| `TestCatalog_Empty_Nil` | `Items=nil` | `Text` = `Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.`; `IsError=false`; Summary `{count:0,endpoint_count:0,shown:0}`; `TargetType=api_spec`, `Target=""` |
| `TestCatalog_Empty_NonNilSlice` | `Items=[]` | sama dengan nil |
| `TestCatalog_ThreeSpecs_Golden` | tiga spec contoh overview 12.2 | teks persis sama dengan blok contoh (header `3 API spec di project ini:`, tiga baris, penutup); Summary `{count:3,endpoint_count:6,shown:3}` |
| `TestCatalog_SingleSpecHeader` | satu spec | header `1 API spec di project ini:` |
| `TestCatalog_EmptyTitle` | `Title=""` | baris `- name: - \| versi ...` (judul `-`) |
| `TestCatalog_EmptyVersions` | `ApiVersion=""`, `OpenapiVersion=""` | `versi - \| OpenAPI -` |
| `TestCatalog_ZeroUpdatedAt` | `UpdatedAt` zero | `diperbarui -` |
| `TestCatalog_UpdatedAtUTC` | `UpdatedAt` zona `Asia/Jakarta` 15:15 | dirender `08:15 UTC` |
| `TestCatalog_TitleMultilineFlattened` | Title `"A\r\nB\tC   D"` | `A B C D`, satu baris (tidak ada `\n`/`\t` tambahan di baris spec) |
| `TestCatalog_TitleClipped` | Title 200 rune multibyte | tepat 120 rune berakhir `...`; UTF-8 valid |
| `TestCatalog_VersionClipped` | versi 100 rune | 40 rune berakhir `...` |
| `TestCatalog_ZeroEndpoints` | `EndpointCount=0` | `0 endpoint` |
| `TestCatalog_FooterAlwaysLast` | kasus normal, budget kecil, dan budget tanpa batas | baris terakhir selalu `Langkah berikut: list_api_endpoints(spec="<name>") ...` |
| `TestCatalog_BudgetSmall_Truncates` | 20 spec, `Budget` dipilih sehingga hanya sebagian muat | `len(Text()) <= Budget`; baris yang tampil adalah prefix berurutan (`s01..sK`); ada penanda `[<20-K> spec lain tidak ditampilkan; gunakan list_api_endpoints(spec=...) dengan nama yang sudah diketahui]` sebelum penutup; Summary `{count:20, endpoint_count:<total semua>, shown:K}` |
| `TestCatalog_BudgetExactFit` | `Budget` = panjang teks penuh | semua tampil, tanpa penanda; `Budget = penuh-1` -> `shown` berkurang dan penanda muncul, `len <= Budget` |
| `TestCatalog_BudgetNoMarkerForLastItem` | item terakhir tepat muat tanpa ruang penanda | item terakhir tetap tampil (reserve 0 bila `rest==0`) |
| `TestCatalog_BudgetTiny_ShownZero` | `Budget=200`, 3 spec | `shown=0`, teks = header + penanda `[3 spec lain ...]` + penutup, `IsError=false`, Summary `shown:0` |
| `TestCatalog_BudgetNonPositive` | `Budget=0` | tanpa batas, semua tampil |
| `TestCatalog_BudgetCountsBytes` | judul multibyte (CJK) | pembatasan memakai `len` byte: `len(Text()) <= Budget` meski rune lebih sedikit |
| `TestCatalog_SummaryNoSpecText` | spec bernama/berjudul unik | semua nilai Summary bertipe integer; tidak ada string judul/nama di Summary |
| `TestCatalog_EndpointTotalIncludesHidden` | 20 spec, sebagian tersembunyi | `EndpointTotal()` = jumlah SEMUA; `Summary.endpoint_count` sama |
| `TestCatalog_DefaultBudgetSane` | 200 spec dengan `Budget=TextBudget(102400)` | `len(Text()) <= 101888` |

### Unit: `listapispecs/params_test.go`

| Tes | Input | Ekspektasi |
|---|---|---|
| `TestParse_AcceptsEmpty` | `nil`, `""`, `"  "`, `"null"`, `"{}"`, `"{ }"` | tanpa error |
| `TestParse_RejectsAnyArgument` | `{"foo":1}`, `{"spec":"petstore"}`, `{"limit":5}`, `{"a":null}` | `*ArgError` bukan nil; `Message()` memuat `tidak dikenal` dan `Tool ini tidak menerima argumen.`; `Summary()` `error_code=invalid_argument` |
| `TestParse_ProjectAliases_AC6` | `{"project_id":"x"}`, `{"projectId":"x"}`, `{"project":"x"}` | `Message()` memuat `ditentukan oleh API key` dan `Hapus argumen ini`; menyebut kunci yang dikirim |
| `TestParse_NotObject` | `[]`, `1`, `"x"`, `true` | error field `arguments` (`harus object JSON`) |
| `TestParse_TrailingTokens` | `{}{"a":1}`, `{} x` | error `harus satu object JSON` |
| `TestParse_BrokenJSON` | `{`, `{"a":` | error `bukan JSON valid` |
| `TestSchema_Exact` | `schema()` | JSON valid, `type=object`, `properties` object kosong, `additionalProperties=false`, tidak ada `required`; `json.Unmarshal` ke `map[string]any` tidak punya kunci lain |
| `TestK4_NoProviderInvalidReachable` | semua input yang dapat dikirim agent | provider `listspecs` hanya invalid bila `ProjectID` bukan uuid; karena `ProjectID` bukan argumen, tes ini cukup menegaskan bahwa `Parse` tidak pernah menghasilkan `Params` dengan field apa pun (struct kosong, `unsafe.Sizeof==0` atau refleksi 0 field) |

### Unit: `listapispecs/usecase_test.go` (fake port)

Fake: `type fakeLister struct{ got listspecs.Request; calls int; resp listspecs.Response; err error }`.

| Tes | Ekspektasi |
|---|---|
| `TestExecute_PassesProjectID` | `Execute(ctx, "11111111-...", Params{})` -> `fake.got == listspecs.Request{ProjectID:"11111111-..."}`, `calls==1` |
| `TestExecute_BuildsCatalogWithBudget` | `newUsecase(fake, 4321)` -> hasil `Budget==4321` dan `Items` sama dengan `resp.Items` |
| `TestExecute_EmptyResponse` | `resp.Items=[]` -> catalog kosong, tanpa error |
| `TestExecute_PortErrorPropagated` | `err = fmt.Errorf("x: %w", bus.ErrNotRegistered)` -> `errors.Is(got, bus.ErrNotRegistered)`; hasil `SpecCatalog{}` |
| `TestExecute_ContextPassedThrough` | ctx berisi value -> diterima fake (untuk menjamin deadline MCP tidak hilang) |

### Unit: `listapispecs/handler_test.go` (`bus.NewRegistry(nil)` + fake handler)

Setup: `reg := bus.NewRegistry(nil)`; `reg.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, func(ctx, req any) (any, error){ calls++; last = req.(listspecs.Request); return resp, nil })`; `tool := Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: reg})`; panggil `tool.Handler(ctx, toolkit.Call{ProjectID: pid, Arguments: ...})`.

| Tes | Ekspektasi |
|---|---|
| `TestHandler_Success` | args `{}` -> `IsError=false`, teks memuat `3 API spec di project ini:` dan semua nama; `last.ProjectID == pid`; `calls==1`; `Summary["count"]==3` |
| `TestHandler_NoArgsRawNil` | `Arguments=nil` dan `"{}"` -> sukses, `calls==1` |
| `TestHandler_ArgRejected_BusNotCalled` | `{"foo":1}` -> `IsError=true`, `calls==0`, `err==nil`, `TargetType=api_spec` |
| `TestHandler_ProjectIDArg_AC6` | key project A (`pid`), args `{"project_id":"<B>"}` -> `IsError=true`, teks memuat `ditentukan oleh API key`, `calls==0`. Kemudian args `{}` -> `last.ProjectID == pid` (bukan `<B>`) |
| `TestHandler_BusNotRegistered` | registry tanpa `Register` -> `err != nil`, `errors.Is(err, bus.ErrNotRegistered)`, `result` nol, pesan `list_api_specs: ` di awal |
| `TestHandler_ProviderError` | fake mengembalikan `errors.New("boom")` -> error Go membungkus `boom`, bukan `isError` |
| `TestHandler_ProviderInvalidIsGoError_K4` | fake mengembalikan error yang bungkus `errs.ErrInvalid` (hanya di file tes, boleh import `errs`) -> tetap error Go (bukan `isError`) |
| `TestHandler_DeadlineExceeded` | fake mengembalikan `context.DeadlineExceeded` -> `errors.Is(err, context.DeadlineExceeded)` benar (agar gateway memberi kode `timeout`) |
| `TestHandler_EmptyProject` | `Items=[]` -> `IsError=false`, teks `Belum ada API spec di project ini. ...`, Summary `count:0` |
| `TestHandler_BudgetFromConfig` | `MCPMaxResponseBytes: 1024` dan 30 spec -> `len(Text) <= 512`; penanda sisa ada |
| `TestHandler_SummaryNoSpecText` | Summary tidak memuat nama/judul spec |
| `TestInit_ToolMetadata` | `Name=="list_api_specs"`, `Title`/`Description` tidak kosong, `Handler != nil`, `InputSchema` valid JSON |

### Integrasi (`//go:build integration`, `backend/cmd/internal/server/apispectools_integration_test.go`)

Memakai helper dan fixture overview 10.3 (`setupMCP`, `uploadSpecs`, `callTool`, `newProjectWithKey`; project A = `petstore`, `billing`, `huge`; project B = `solo`).

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT09_Int_AC1_ListSpecs` (sudah di overview) | 1 | key A, `list_api_specs {}` | `isError:false`; teks memuat `3 API spec`, `billing`, `huge`, `petstore`, versi, `OpenAPI 3.0.3`, jumlah endpoint, `Langkah berikut: list_api_endpoints` |
| `TestT09_Int_ListSpecs_OrderByName` | 1 | sama | urutan baris `billing`, `huge`, `petstore` |
| `TestT09_Int_ListSpecs_EmptyProject` | edge | `newProjectWithKey(t, e, "kosong")` tanpa upload, `list_api_specs {}` | `isError:false`, teks `Belum ada API spec di project ini...` |
| `TestT09_Int_AC6_ListSpecs_ProjectArg` (bagian dari tes AC6 overview) | 6 | key A, `list_api_specs {project_id:<B>}` | `isError:true`, `ditentukan oleh API key`, tidak memuat `solo` |
| `TestT09_Int_ListSpecs_IsolatedProject` | 6 | key B, `list_api_specs {}` | hanya `solo`; tidak ada `petstore` |
| `TestT09_Int_ListSpecs_AuditOneEntry` | K13 | satu panggilan sukses | tepat satu entri `mcp.list_api_specs`, `target_type=api_spec`, result memuat `count`, `endpoint_count`, `shown`, `bytes`; tidak memuat nama/judul spec |
| `TestT09_Int_ToolsList` (overview) | 7 | `tools/list` | `list_api_specs` ada dengan `annotations.readOnlyHint:true`, schema `properties:{}` |

---

## E2E Scenarios

Positif:
1. Agent memanggil `list_api_specs {}` pada project 3 spec -> melihat tiga baris dengan judul, versi, jumlah endpoint, dan tanggal; mengambil `petstore` dari kolom nama lalu memanggil `list_api_endpoints {spec:"petstore"}` (alur AC1).
2. Project tanpa spec -> `Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.` dengan `isError:false` (agent beralih ke tool knowledge).
3. Spec tanpa judul/versi -> baris memakai `-` sehingga format kolom tetap konsisten.
4. Project dengan ratusan spec (atau `MCP_MAX_RESPONSE_BYTES` kecil) -> daftar dipotong pada batas budget dengan penanda `[N spec lain tidak ditampilkan; ...]`; JSON-RPC envelope tetap utuh karena gateway tidak perlu memotong.
5. Dua panggilan berturut-turut -> hasil identik (idempoten, tanpa state).

Negatif:
1. `list_api_specs {"spec":"petstore"}` -> `Argumen tidak valid: spec tidak dikenal. Tool ini tidak menerima argumen.` (`isError:true`), bus tidak dipanggil.
2. `list_api_specs {"project_id":"<uuid lain>"}` (atau `projectId`/`project`) -> `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.`; data project lain tidak pernah muncul, project tidak berubah (AC6).
3. `arguments` bukan object (`[]`, `"x"`) -> `Argumen tidak valid: arguments harus object JSON ...` (`isError:true`).
4. Handler `project.apispec.list-specs` tidak terdaftar (modul project tidak ter-wire) -> error Go membungkus `bus.ErrNotRegistered`; gateway menjawab `Terjadi kesalahan internal saat menjalankan tool list_api_specs. trace_id=...`.
5. Provider timeout (`context.DeadlineExceeded`) -> error Go; gateway memberi teks timeout.
6. Provider mengembalikan error lain (DB) -> error Go; detail disembunyikan oleh gateway, `trace_id` untuk operator.

---

## Request/Response body

### Request sukses

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_api_specs","arguments":{}}}
```
`arguments` boleh dihilangkan; gateway meneruskan `{}`.

### Response sukses (3 spec)

```json
{"jsonrpc":"2.0","id":5,"result":{"content":[{"type":"text","text":"3 API spec di project ini:\n- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-20 08:15 UTC\n- huge: Huge Report | versi 1.0.0 | OpenAPI 3.0.3 | 1 endpoint | diperbarui 2026-10-20 08:16 UTC\n- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint | diperbarui 2026-10-20 08:17 UTC\nLangkah berikut: list_api_endpoints(spec=\"<name>\") untuk daftar endpoint, lalu get_api_endpoint untuk detail."}],"isError":false}}
```

`text` yang sama dalam bentuk terbaca:
```
3 API spec di project ini:
- billing: Billing API | versi 2.1.0 | OpenAPI 3.0.3 | 2 endpoint | diperbarui 2026-10-20 08:15 UTC
- huge: Huge Report | versi 1.0.0 | OpenAPI 3.0.3 | 1 endpoint | diperbarui 2026-10-20 08:16 UTC
- petstore: Swagger Petstore | versi 1.0.0 | OpenAPI 3.0.3 | 3 endpoint | diperbarui 2026-10-20 08:17 UTC
Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.
```

Metadata audit (ditulis gateway; tool hanya menyuplai `target_type`, `target`, dan Summary): `action=mcp.list_api_specs`, `target_type=api_spec`, `target=""`, result `{"count":3,"endpoint_count":6,"shown":3,"bytes":<len(Text)>}`.

### Response sukses dengan judul/versi kosong

```
1 API spec di project ini:
- legacy: - | versi - | OpenAPI 3.1.0 | 12 endpoint | diperbarui 2026-10-19 02:00 UTC
Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.
```

### Response terpotong budget

Contoh dengan `MCP_MAX_RESPONSE_BYTES=1024` (budget teks 512) dan 12 spec; hanya sebagian tampil:
```
12 API spec di project ini:
- s01: Service 01 | versi 1.0.0 | OpenAPI 3.0.3 | 4 endpoint | diperbarui 2026-10-20 08:15 UTC
- s02: Service 02 | versi 1.0.0 | OpenAPI 3.0.3 | 4 endpoint | diperbarui 2026-10-20 08:15 UTC
- s03: Service 03 | versi 1.0.0 | OpenAPI 3.0.3 | 4 endpoint | diperbarui 2026-10-20 08:15 UTC
[9 spec lain tidak ditampilkan; gunakan list_api_endpoints(spec=...) dengan nama yang sudah diketahui]
Langkah berikut: list_api_endpoints(spec="<name>") untuk daftar endpoint, lalu get_api_endpoint untuk detail.
```
Summary `{count:12, endpoint_count:48, shown:3}`; `isError:false`. (Angka `shown` aktual ditentukan algoritma guard; tes memverifikasi `len(text) <= Budget`, bukan angka tetap ini.)

### Response project tanpa spec

```json
{"jsonrpc":"2.0","id":6,"result":{"content":[{"type":"text","text":"Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files."}],"isError":false}}
```
Summary `{count:0, endpoint_count:0, shown:0}`.

### Error: argumen apa pun

Request `{"name":"list_api_specs","arguments":{"spec":"petstore"}}`:
```json
{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"Argumen tidak valid: spec tidak dikenal. Tool ini tidak menerima argumen."}],"isError":true}}
```
Summary audit: `{"error_code":"invalid_argument","field":"spec"}`. Bus tidak dipanggil.

### Error: argumen project

Request `{"name":"list_api_specs","arguments":{"project_id":"7c1f0f4e-0000-4000-8000-000000000000"}}`:
```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```

### Error: bukan object

```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"Argumen tidak valid: arguments harus object JSON (diterima: [] ). Kirim {} karena tool ini tidak menerima argumen."}],"isError":true}}
```
(Teks persis mengikuti `ArgError.Message()` hasil F3: `Argumen tidak valid: <field> <alasan> (diterima: <nilai ringkas>). <saran>`. Tes `TestParse_NotObject` hanya mengunci `arguments` dan `harus object JSON`, bukan spasi pada contoh ini. `Hint` untuk tool ini: `Kirim {} karena tool ini tidak menerima argumen.`)

### Error: bus/provider gagal (error Go, dipetakan gateway T07, teks tidak diubah T09)

```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool list_api_specs. trace_id=3b2f9c7e1a"}],"isError":true}}
```
Untuk `context.DeadlineExceeded`, gateway memakai teks timeout dengan kode `timeout`.
