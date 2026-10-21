# T09 usecase `listapiendpoints` — Tool MCP `list_api_endpoints`

Tiket: `docs/tickets/T09-discovery-api-spec-tools.md`
Foundation (mengikat, menang bila bertentangan): `docs/superpowers/plans/discovery-api-spec-tools/overview.md`
Keputusan bersama yang dipakai: K3 (batas import), K4 (error Go dibungkus, validasi superset provider), K5 (argumen ketat, tolak `project_id`), K6 (tolak, bukan clamp), K7 (format `ArgError`), K8 (tidak dipakai di tool ini), K9 (anggaran byte), K10 (`Limit` eksplisit), K11 (bahasa), K12 (teks spec tidak tepercaya), K13 (audit metadata saja), K14 (`SpecMiss`), K16 (tanpa state).

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` tidak relevan (tanpa migrasi); tidak ada knowledge doc module; tiket menyatakan UI "Tidak ada", jadi tidak ada frontend plan dan tidak ada bagian Visual. Kontrak T06 `lib/contracts/project/apispec/listendpoints` sudah di `main`: `Request{ProjectID, Spec, Tag, Query string; Limit, Offset int}`, `Response{SpecFound, Specs, Items, Total, Truncated, NextOffset}`, `Endpoint{Spec, Method, Path, OperationID, Summary, Tags, Deprecated}`, `DefaultLimit=100`, `MaxLimit=300`, `MaxQueryRunes=200`. Provider melakukan clamp diam-diam; tool ini menolak nilai di luar rentang sebelum memanggil bus.

---

## Metadata

| Atribut | Nilai |
|---|---|
| Endpoint | `/mcp` (milik T07; tidak ada endpoint REST baru) |
| Method | `POST`, JSON-RPC 2.0 `tools/call` dengan `params.name = "list_api_endpoints"` (tampil juga di `tools/list`) |
| Version | MCP protocol `2025-06-18` (dinegosiasikan T07); tanpa versi REST |
| Scope | backend saja (workflow menyebut fullstack, tetapi tiket UI "Tidak ada"); `endpointsChanged = false` |
| Domain | discovery |
| Module | api-spec-tools (package `apispectools`) |
| Feature | `listapiendpoints` |
| Resource root | `backend/internal/domain/discovery/internal/apispectools` |
| Bus dikonsumsi | `project.apispec.list-endpoints` (Call sinkron, provider T06) via `constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS` |
| Bus disediakan | tidak ada |
| Aksi audit | `mcp.list_api_endpoints` (sudah ada di `record.Actions()`; ditulis gateway T07, bukan tool ini) |
| Migrasi | tidak ada |
| Gerbang | T06 dan T07 sudah di `main` (overview bagian 0). Prasyarat foundation F3 (`base/{names,args,format,budget,specmiss}.go` + tes) dan F4 (`apispectools/base.go` dengan stub `Init`) harus selesai lebih dulu |

---

## Summary Plan

`list_api_endpoints` adalah langkah kedua penelusuran API spec: daftar ringkas satu baris per operasi, dikelompokkan per spec, dengan penyaring `spec`, `tag`, `query` dan paginasi `limit`/`offset`. Alur: handler mem-parse argumen (ketat, tolak `project_id`, tolak nilai di luar rentang), usecase memanggil port repository (satu `bus.CallTyped` ke T06 dengan `Limit` eksplisit), lalu membangun entity `base.EndpointPage`. Entity murni itulah yang menjadi pemilik semua rule: header, pengelompokan `== <spec> ==`, format baris, pemotongan berdasar `TextBudget`, penghitungan `NextOffset` dari baris yang benar-benar dirender, penanda lanjutan, pesan total 0, `offset_out_of_range`, dan ringkasan audit. `!SpecFound` dialihkan ke `base.SpecMiss` (K14).

Poin desain:
- Satu usecase, tidak dipecah. Handler dan usecase tidak berisi rule bisnis atau format teks.
- `NextOffset` provider diabaikan. Entity menghitung `Offset + shown` karena baris bisa terpotong oleh anggaran byte walau provider mengirim semua `Items`.
- Urutan item dari provider (Spec, Path, method kanonik) dipertahankan; entity tidak mengurutkan ulang. Pengelompokan cukup dengan membuka grup baru saat `Spec` berbeda dari baris sebelumnya yang dirender.
- Minimal satu baris selalu dirender (jaminan progres paginasi), meskipun anggaran sangat kecil. Path dipotong 300 rune dalam baris supaya satu baris tidak bisa melewati cadangan gateway.
- Tidak ada state, cache, goroutine, atau SQL. `ProjectID` hanya dari `call.ProjectID`.
- Perubahan kecil terhadap overview 4: anggaran teks (`base.TextBudget(d.Config.MCPMaxResponseBytes)`) dihitung sekali di `init.go` lalu diteruskan ke `newUsecase(repo, budget)` (bukan ke handler), karena usecase yang membangun entity. Kontrak entity di overview 5.6 tidak berubah.

---

## Plan (langkah berurutan)

Semua jalur relatif terhadap resource root `backend/internal/domain/discovery/internal/apispectools`. Urutan TDD: tulis tes dulu per langkah.

**E1. Entity `base/endpoints.go`** (tulis `base/endpoints_test.go` dulu).

```go
package base

import (
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

// Filter adalah filter yang dipakai agent (sudah divalidasi/di-trim oleh Parse).
type Filter struct{ Spec, Tag, Query string }

// EndpointPage adalah satu halaman hasil list_api_endpoints. Murni, tanpa I/O.
type EndpointPage struct {
	Filter Filter
	Offset int
	Total  int
	Items  []listendpoints.Endpoint // urutan persis dari provider
	Budget int                      // TextBudget(MCPMaxResponseBytes)
}

// NewEndpointPage: bila !resp.SpecFound mengembalikan (EndpointPage{}, &SpecMiss{Tool: ToolListAPIEndpoints,
// Requested: f.Spec, Required: false, Valid: resp.Specs}). Selain itu menyalin Items/Total dan mengembalikan (page, nil).
func NewEndpointPage(f Filter, offset int, resp listendpoints.Response, budget int) (EndpointPage, *SpecMiss)

func (p EndpointPage) Lines() (text string, shown int) // badan: grup + baris yang muat Budget
func (p EndpointPage) NextOffset() int                 // Offset+shown bila Offset+shown < Total, selain itu -1
func (p EndpointPage) Text() string
func (p EndpointPage) Result() toolkit.Result
```

Konstanta privat: `closingReserve = 200` (ruang untuk penanda lanjutan + baris petunjuk), `summaryMaxRunes = 160`, `pathMaxRunes = 300`, `tagsMaxRunes = 120`, `opIDMaxRunes = 100`.

Aturan (urutan cabang di `Text()`/`Result()`):

1. **Total 0** (`p.Total == 0`, `SpecFound` true): `IsError:false`.
   - Tanpa filter sama sekali (`Spec`, `Tag`, `Query` semuanya kosong): teks `Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.` (teks sama dengan cabang 1 `SpecMiss.Text()`; gunakan konstanta bersama atau panggil `SpecMiss{}.Text()`, jangan menyalin literal).
   - Ada filter: `Tidak ada endpoint yang cocok (<filterDesc>). Hapus filter tag/query, periksa nama tag dari list_api_endpoints tanpa filter, atau lihat spec dengan list_api_specs.`
     `filterDesc` = `spec: <Spec atau "semua">`, ditambah `, tag: <Tag>` bila diisi, ditambah `, query: "<Query>"` bila diisi.
   - Offset > 0 dengan Total 0 diperlakukan sama (bukan `offset_out_of_range`).
   - Summary `{count:0, total:0, next_offset:-1, spec_groups:0, truncated:false}`.
2. **Offset di luar rentang** (`Total > 0 && len(Items) == 0`): `IsError:true`, teks `Argumen tidak valid: offset <Offset> melewati total <Total> endpoint. Gunakan offset 0 sampai <Total-1>.`, Summary `{error_code:"offset_out_of_range", total:Total}`.
3. **Normal** (`Total > 0 && len(Items) > 0`), teks tersusun:
   - Header: `Endpoint <Offset+1>-<Offset+shown> dari <Total> | spec: <Spec atau "semua">` + ` | tag: <Tag>` bila diisi + ` | query: "<Query>"` bila diisi.
   - Badan dari `Lines()` (di bawah). Antar grup tidak ada baris kosong.
   - Bila `NextOffset() >= 0`: satu baris kosong, lalu `[ada <Total-Offset-shown> endpoint lagi, panggil list_api_endpoints dengan offset=<NextOffset> dan argumen lain yang sama]`.
   - Baris petunjuk selalu terakhir (didahului satu baris kosong bila tidak ada penanda; bila ada penanda, ditaruh persis di baris berikutnya tanpa baris kosong): `Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id).`
   - `IsError:false`, `TargetType: base.TargetTypeSpec` (`api_spec`), `Target: Filter.Spec`.
   - Summary `{count: shown, total: Total, next_offset: NextOffset() (int, -1 bila habis), spec_groups: jumlah grup yang dirender, truncated: NextOffset() >= 0}`. DILARANG memuat query, tag, summary, atau path (query sudah ada di `Args` audit gateway).

`Lines()`:

- `used := len(header dengan shown = len(Items)) + 1` (batas atas; angka `shown` sebenarnya paling banyak sama panjang). Header final dihitung ulang dengan `shown` sebenarnya.
- Untuk tiap item i (urut):
  - `newGroup := (i == 0) || item.Spec != prevSpec`.
  - `row := formatRow(item)`; `piece := (newGroup ? "== "+item.Spec+" ==\n" : "") + row + "\n"`.
  - Bila `i > 0 && used + len(piece) + closingReserve > Budget`: berhenti. Baris pertama (i == 0) SELALU dirender.
  - `used += len(piece)`; `shown++`; bila `newGroup` maka `groups++`; `prevSpec = item.Spec`.
- Kembalikan badan tanpa `\n` akhir dan `shown`. (`groups` disimpan via method privat `render()` yang dipakai `Lines()`, `Text()`, dan `Result()` agar satu sumber kebenaran.)
- Pada halaman lanjutan (`Offset > 0`) baris pertama tetap diawali `== <spec> ==` (agent tahu spec asal baris).

`formatRow(e)` (K12, semua teks dari spec diratakan dan dipotong):

```
<METHOD> <Clip(OneLine(Path), 300)>
  + " — " + Clip(OneLine(Summary), 160)          bila hasil OneLine tidak kosong
  + " [" + Clip(OneLine(OperationID), 100) + "]"  bila OperationID tidak kosong
  + " (" + Clip(join(tag yang sudah OneLine & tidak kosong, ", "), 120) + ")"  bila ada tag
  + " [deprecated]"                               bila Deprecated
```

Pemisah " — " memakai em dash (U+2014) persis seperti spesifikasi tiket. `METHOD` dipakai apa adanya dari provider (sudah huruf besar).

**E2. `listapiendpoints/params.go`** (tulis `params_test.go` dulu).

```go
package listapiendpoints

type rawArgs struct {
	Spec   *string `json:"spec"`
	Tag    *string `json:"tag"`
	Query  *string `json:"query"`
	Limit  *int    `json:"limit"`
	Offset *int    `json:"offset"`
}

// Params hasil validasi. Tidak memuat project.
type Params struct {
	Spec, Tag, Query string // sudah di-TrimSpace
	Limit            int    // 1-300
	Offset           int    // 0-1000000
}

const (
	maxSpecRunes  = 80
	maxTagRunes   = 200
	maxQueryRunes = 200 // = listendpoints.MaxQueryRunes
	defaultLimit  = 100 // = listendpoints.DefaultLimit
	maxLimit      = 300 // = listendpoints.MaxLimit
	maxOffset     = 1_000_000
)

var allowedFields = []string{"spec", "tag", "query", "limit", "offset"}

func schema() json.RawMessage // InputSchema di bawah
func Parse(raw json.RawMessage) (Params, *base.ArgError)
```

Urutan `Parse` (error pertama menang, urutan field sesuai schema, K7):

1. `base.DecodeStrict(raw, &a, allowedFields)`: kosong/`null` menjadi `{}`; bukan object, token sisa, tipe salah (`limit:"x"` atau `limit:1.5` menjadi `limit harus integer`; `spec:1` menjadi `spec harus string`), dan field tak dikenal. `project_id`/`projectId`/`project` menghasilkan pesan khusus K5: `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.`
2. `spec`: `base.StrArg("spec", a.Spec, 80, "Panggil list_api_specs untuk melihat nama spec yang valid.")`.
3. `tag`: `base.StrArg("tag", a.Tag, 200, "Panggil list_api_endpoints tanpa tag untuk melihat tag yang ada.")`.
4. `query`: `base.StrArg("query", a.Query, 200, "Persingkat kata kunci.")`. Ditolak, tidak dipotong diam-diam (K6); provider yang memotong dengan `MaxQueryRunes` tidak pernah tercapai.
5. `limit`: `base.IntArg("limit", a.Limit, 100, 1, 300, "Gunakan offset untuk halaman berikutnya.")`. Contoh: `Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya.`
6. `offset`: `base.IntArg("offset", a.Offset, 0, 1_000_000 ... )` yaitu `IntArg("offset", a.Offset, 0, 0, 1_000_000, "Gunakan nilai offset dari penanda [ada N endpoint lagi ...].")`.

Tidak ada normalisasi lain: `tag` dikirim persis (peka huruf besar/kecil, provider mencocokkan persis); `spec` dikirim persis setelah trim (nama spec peka huruf besar/kecil menurut T06).

`InputSchema` (disalin persis dari overview 12.1; golden menjadi acuan akhir):

```json
{
  "type": "object",
  "properties": {
    "spec":   { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; kosong = semua spec" },
    "tag":    { "type": "string", "maxLength": 200, "description": "tag persis (peka huruf besar/kecil)" },
    "query":  { "type": "string", "maxLength": 200, "description": "kata yang dicari pada path, ringkasan, atau operationId" },
    "limit":  { "type": "integer", "minimum": 1, "maximum": 300, "default": 100, "description": "jumlah endpoint per halaman" },
    "offset": { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah endpoint; pakai nilai dari penanda [ada N endpoint lagi ...]" }
  },
  "additionalProperties": false
}
```

Tanpa `required`. Tidak ada properti project.

**E3. `listapiendpoints/repository.go`**

```go
// Lister adalah port ke provider API spec (T06). Satu-satunya tempat bus dipanggil.
type Lister interface {
	List(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error)
}

type busLister struct{ b *bus.Registry } // tipe mengikuti field app.Deps.Bus

func newRepository(b *bus.Registry) Lister { return busLister{b: b} }

func (r busLister) List(ctx context.Context, req listendpoints.Request) (listendpoints.Response, error) {
	return bus.CallTyped[listendpoints.Response](ctx, r.b, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, req)
}
```

Request dikirim sebagai NILAI (bukan pointer), sesuai asersi `bus.Expect[listendpoints.Request]` di provider. Cek tipe `Deps.Bus` dengan `go doc grimoire/internal/infra/app` sebelum menulis. Tanpa logika; error dikembalikan apa adanya (termasuk `bus.ErrNotRegistered`).

**E4. `listapiendpoints/usecase.go`**

```go
type usecase struct {
	lister Lister
	budget int // base.TextBudget(cfg.MCPMaxResponseBytes), dihitung di init.go
}

func newUsecase(l Lister, budget int) usecase { return usecase{lister: l, budget: budget} }

// Outcome adalah penyeleksi tipis: tepat satu dari Page atau Miss. Tanpa rule bisnis.
type Outcome struct {
	Page base.EndpointPage
	Miss *base.SpecMiss
}

func (o Outcome) Result() toolkit.Result {
	if o.Miss != nil {
		return o.Miss.Result()
	}
	return o.Page.Result()
}

// Execute: orkestrasi saja.
func (u usecase) Execute(ctx context.Context, projectID string, p Params) (Outcome, error) {
	resp, err := u.lister.List(ctx, listendpoints.Request{
		ProjectID: projectID,
		Spec:      p.Spec,
		Tag:       p.Tag,
		Query:     p.Query,
		Limit:     p.Limit,  // selalu eksplisit (K10)
		Offset:    p.Offset,
	})
	if err != nil {
		return Outcome{}, err // diteruskan; handler membungkus (K4)
	}
	page, miss := base.NewEndpointPage(base.Filter{Spec: p.Spec, Tag: p.Tag, Query: p.Query}, p.Offset, resp, u.budget)
	return Outcome{Page: page, Miss: miss}, nil
}
```

Tidak ada aturan rentang, format, pengelompokan, atau penilaian "kosong" di sini.

**E5. `listapiendpoints/handler.go`**

```go
func newHandler(u usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, base.TargetTypeSpec), nil // bus tidak dipanggil
		}
		out, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return toolkit.Result{}, fmt.Errorf("%s: %w", base.ToolListAPIEndpoints, err)
		}
		return out.Result(), nil
	}
}
```

Handler tidak memformat teks, tidak menyentuh bus, dan tidak memeriksa jenis error (K3 melarang `errs`; K4: semua error Go dibungkus, gateway memetakan ke `internal`/`timeout`; `errors.Is(err, context.DeadlineExceeded)` tetap berlaku lewat `%w`).

**E6. `listapiendpoints/init.go`**

```go
const (
	title       = "Daftar endpoint API"
	description = "Daftar endpoint API (read-only), satu baris per operasi: METHOD /path — ringkasan [operationId] (tag). Tanpa spec = semua spec, dikelompokkan per spec. Saring dengan tag (persis) atau query (cocok sebagian pada path, ringkasan, operationId). Lanjutkan daftar terpotong dengan offset. Ambil detail dengan get_api_endpoint."
)

func Init(d app.Deps) toolkit.Tool {
	budget := base.TextBudget(d.Config.MCPMaxResponseBytes)
	h := newHandler(newUsecase(newRepository(d.Bus), budget))
	return toolkit.Tool{
		Name:        base.ToolListAPIEndpoints,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     h,
	}
}
```

`description` dibekukan dan harus identik dengan entri `list_api_endpoints` di `testdata/tools_list.golden.json`. Tool tidak mendaftarkan endpoint bus dan tidak menulis audit (rules 9.3).

**E7. Tes** per bagian Test Scenario. Urutan TDD: `base/endpoints_test.go` (E1), `params_test.go` (E2), `usecase_test.go` (E4), `handler_test.go` (E5).

**E8. Wiring ke module.** Ganti stub `listapiendpoints.Init` di `apispectools/base.go` (F4 overview) dengan implementasi ini. `discovery/base.go` tidak disentuh (F6 overview milik foundation).

**E9. Tes integrasi** pada `backend/cmd/internal/server/apispectools_integration_test.go` (file milik foundation F8; tambahkan tes `list_api_endpoints` di bawah, memakai helper `uploadSpecs`, `callTool`, `newProjectWithKey` dan fixture `petstore`/`billing`/`huge`/`solo` dari overview 10.3).

**E10. Gerbang usecase.**

```
cd backend
go build ./... && go vet ./...
go test ./internal/domain/discovery/internal/apispectools/...
go test ./internal/archtest/...
go test -tags integration ./cmd/internal/server/... -run 'TestT09_Int_(AC1|ListEndpoints|UnknownSpecList|AC6)'
```

Entri `list_api_endpoints` di golden harus persis sama dengan bagian ini (verifikasi manual saat `-update`, foundation F7).

---

## Migration

Tidak ada. Tidak ada tabel, kolom, atau index baru; data dibaca lewat bus T06.

---

## Changes (per file/folder)

Resource root: `backend/internal/domain/discovery/internal/apispectools`.

| File | Status | Isi |
|---|---|---|
| `base/endpoints.go` | baru | `Filter`, `EndpointPage`, `NewEndpointPage`, `Lines`, `NextOffset`, `Text`, `Result`, helper privat `render`, `formatRow`, `filterDesc` |
| `base/endpoints_test.go` | baru | tes entity (lihat Test Scenario) |
| `listapiendpoints/init.go` | baru | `Init(d app.Deps) toolkit.Tool` (nama, title, description, schema, handler, anggaran) |
| `listapiendpoints/params.go` | baru | `rawArgs`, `Params`, konstanta batas, `schema()`, `Parse` |
| `listapiendpoints/handler.go` | baru | adapter `toolkit.Handler` |
| `listapiendpoints/usecase.go` | baru | `usecase`, `Outcome`, `Execute` |
| `listapiendpoints/repository.go` | baru | interface `Lister` + implementasi `bus.CallTyped` |
| `listapiendpoints/params_test.go` | baru | tabel validasi argumen |
| `listapiendpoints/usecase_test.go` | baru | fake `Lister` |
| `listapiendpoints/handler_test.go` | baru | `bus.NewRegistry(nil)` + handler bus palsu |
| `base.go` | ubah (milik foundation F4) | `Register` memanggil `listapiendpoints.Init(d)` (stub diganti) |
| `testdata/tools_list.golden.json` | diperbarui lewat `-update` (foundation F7) | entri `list_api_endpoints` harus cocok dengan bagian ini |
| `backend/cmd/internal/server/apispectools_integration_test.go` | ubah (foundation F8) | tes `list_api_endpoints` (E9) |

Dipakai tanpa diubah: `base/{names,args,format,budget,specmiss}.go` (foundation F3), `lib/constants/bus.go` (`BUS_PROJECT_APISPEC_LIST_ENDPOINTS`), `lib/contracts/project/apispec/listendpoints`, `lib/contracts/discovery/mcpgateway/toolkit`.

Tidak berubah: `api/**`, `migrations/**`, `frontend/**`, `infra/config`, `lib/contracts/core/audit/record`, `mcpgateway/**`, `go.mod`.

Kontrak bus yang dikonsumsi:

| Konstanta | Nama | Request | Response | Error |
|---|---|---|---|---|
| `BUS_PROJECT_APISPEC_LIST_ENDPOINTS` | `project.apispec.list-endpoints` | `listendpoints.Request{ProjectID: call.ProjectID, Spec, Tag, Query, Limit (selalu eksplisit 1-300), Offset}` (nilai) | `listendpoints.Response{SpecFound, Specs, Items, Total, Truncated, NextOffset}` | semua `err != nil` (termasuk `bus.ErrNotRegistered`, `errs.ErrInvalid`, deadline) dibungkus `fmt.Errorf("list_api_endpoints: %w", err)`; `errs.ErrInvalid` tidak tercapai dari input agent (K4) |

Import yang diizinkan di package `listapiendpoints` dan `base/endpoints.go` (K3, AC7): stdlib, `grimoire/internal/infra/app`, `grimoire/internal/infra/bus`, `grimoire/lib/constants`, `grimoire/lib/contracts/project/apispec/listendpoints`, `grimoire/lib/contracts/discovery/mcpgateway/toolkit`, dan package module sendiri. Dilarang `lib/common/errs`, `internal/domain/project/**`, dan `internal/domain/discovery/internal/mcpgateway/**`. File `_test.go` boleh tambah `infra/config`.

---

## Test Scenario

Nama tes yang memetakan AC memakai awalan `TestT09_ACn_`.

### `base/endpoints_test.go` (tabel, tanpa I/O)

Fixture: helper `ep(spec, method, path, opID, summary string, tags []string, dep bool) listendpoints.Endpoint`. Budget besar (100000) kecuali disebut lain.

| Tes | Skenario | Ekspektasi |
|---|---|---|
| `TestT09_AC1_GroupedBySpec` | Spec kosong; Items: billing x2, petstore x1; Total 3 | teks persis: `Endpoint 1-3 dari 3 \| spec: semua`, `== billing ==`, dua baris, `== petstore ==`, satu baris, baris petunjuk `Detail: ...`; `spec_groups == 2`; tanpa penanda |
| `TestEndpointPage_SingleSpecStillHasGroupHeader` | `Filter.Spec = "petstore"` | header `spec: petstore`; tetap ada `== petstore ==` |
| `TestEndpointPage_GroupHeaderRepeatsOnLaterPage` | Offset 2, item pertama petstore | baris badan pertama `== petstore ==`; header `Endpoint 3-4 dari ...` |
| `TestEndpointPage_HeaderWithTagAndQuery` | Tag `pets`, Query `pet` | header berakhir ` \| tag: pets \| query: "pet"` |
| `TestEndpointPage_Row_Full` | summary, opId, dua tag, deprecated | baris persis `POST /pets — Create a pet [createPet] (pets, admin) [deprecated]` |
| `TestEndpointPage_Row_NoTagNoOpID` | tanpa tag, tanpa operationId | `GET /health — Health check`; tanpa `[]`, tanpa `()` |
| `TestEndpointPage_Row_NoSummary` | summary kosong atau hanya spasi/newline | tanpa ` — `; baris `GET /health [healthCheck]` |
| `TestEndpointPage_Row_SummaryOneLineAndClip` | summary memuat `\n`, `\t`, spasi ganda, dan 300 rune | satu baris tanpa newline; panjang summary tepat 160 rune berakhir `...` |
| `TestEndpointPage_Row_TagsSanitized` | tag memuat newline, tag kosong, 200 tag | tag diratakan, yang kosong dibuang, gabungan dipotong 120 rune |
| `TestEndpointPage_Row_LongPathClipped` | path 1000 rune | path dalam baris 300 rune |
| `TestEndpointPage_Row_Deprecated` | `Deprecated=true` vs false | penanda `[deprecated]` hanya pada yang true, di akhir baris |
| `TestEndpointPage_NextOffset_AllShown` | Offset 0, 3 item, Total 3 | `NextOffset() == -1`; tanpa penanda; `truncated:false` di Summary |
| `TestEndpointPage_Marker_ProviderLimited` | Offset 0, 2 item, Total 6, budget besar | `NextOffset() == 2`; penanda persis `[ada 4 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]` setelah satu baris kosong, lalu baris `Detail:`; `Summary.truncated == true` |
| `TestT09_AC5ish_BudgetTruncatesRows` | 50 item, Total 50, budget kecil sehingga hanya sebagian muat | `shown < 50`; `NextOffset() == Offset+shown`; angka `ada N endpoint lagi` = `Total-Offset-shown`; `len(Text()) <= Budget`; `Summary.count == shown`; header menyebut `1-<shown>` |
| `TestEndpointPage_Budget_AlwaysAtLeastOneRow` | budget 10 (lebih kecil dari satu baris) | tepat satu baris dirender, `NextOffset() == Offset+1`, tidak panik |
| `TestEndpointPage_Budget_ReserveSufficient` | sapu budget 700..3000 dengan Offset 1000000 dan Total 1000299 (angka terpanjang) | selama `shown >= 2`, `len(Text()) <= Budget` |
| `TestEndpointPage_Budget_GroupHeaderCountedWithRow` | baris pertama grup kedua tidak muat | header grup kedua TIDAK muncul tanpa barisnya |
| `TestEndpointPage_NextOffsetFromRendered_NotProvider` | resp.Truncated/NextOffset sengaja salah (mis. `NextOffset: 999`) | hasil mengikuti `Offset+shown` |
| `TestEndpointPage_Total0_NoFilter` | Total 0, SpecFound true, tanpa filter | teks `Belum ada API spec di project ini. ...`; `IsError == false`; Summary `{count:0,total:0,next_offset:-1,spec_groups:0,truncated:false}` |
| `TestEndpointPage_Total0_WithFilter` | Total 0, Spec `petstore`, Tag `x`, Query `y` | teks `Tidak ada endpoint yang cocok (spec: petstore, tag: x, query: "y"). Hapus filter tag/query, ...`; `IsError == false` |
| `TestEndpointPage_Total0_OnlySpecFilter` | Total 0, Spec `petstore` saja | `Tidak ada endpoint yang cocok (spec: petstore). ...` (bukan pesan "belum ada API spec") |
| `TestEndpointPage_Total0_WithOffsetIsNotRangeError` | Total 0, Offset 5 | pesan total 0, `IsError == false` |
| `TestEndpointPage_OffsetOutOfRange` | Total 6, Items kosong, Offset 6 dan 5000 | `IsError == true`; teks `Argumen tidak valid: offset 5000 melewati total 6 endpoint. Gunakan offset 0 sampai 5.`; `Summary == {error_code:"offset_out_of_range", total:6}` |
| `TestEndpointPage_SpecMiss` | `SpecFound=false`, Specs `[billing, petstore]`, Spec `petstor` | `NewEndpointPage` mengembalikan `*SpecMiss{Tool:"list_api_endpoints", Requested:"petstor", Required:false, Valid:[billing petstore]}`; `Result()` isError, teks memuat `Spec tidak ditemukan: petstor` dan `billing, petstore` |
| `TestEndpointPage_SpecMiss_NoSpecsInProject` | `SpecFound=false`, Specs kosong | teks cabang `no_specs` (`Project ini belum punya API spec`) |
| `TestEndpointPage_ItemOrderPreserved` | Items tidak terurut alfabet | urutan teks sama dengan input |
| `TestEndpointPage_Result_Metadata` | halaman normal | `TargetType == "api_spec"`, `Target == Filter.Spec`; kunci Summary tepat `count,total,next_offset,spec_groups,truncated`; tidak memuat string summary/path/query |
| `TestEndpointPage_UnicodeSafe` | summary dan path multibyte (huruf Jawa/emoji) | `Clip` memotong per rune, hasil UTF-8 valid |

### `listapiendpoints/params_test.go` (tabel)

| Input `arguments` | Ekspektasi |
|---|---|
| `""`, `null`, `{}`, ` ` | `Params{Limit:100, Offset:0}`, tanpa error |
| `{"spec":"  petstore "}` | `Spec == "petstore"` |
| `{"spec":"` + 80 rune multibyte + `"}` | diterima |
| `{"spec":"` + 81 rune + `"}` | `spec maksimal 80 karakter`, `Field == "spec"` |
| `{"spec":"a\tb"}` | `spec memuat karakter kontrol` |
| `{"tag":"` + 201 rune + `"}` | `tag maksimal 200 karakter` |
| `{"query":"` + 200 rune + `"}` | diterima; 201 rune ditolak `query maksimal 200 karakter` |
| `{"limit":1}`, `{"limit":300}` | diterima |
| `{"limit":0}`, `{"limit":301}`, `{"limit":-5}`, `{"limit":500}` | `Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya.` (bukan clamp) |
| `{"limit":"x"}`, `{"limit":1.5}`, `{"limit":null}` | `limit harus integer` untuk dua pertama; `null` dianggap tidak diisi (default 100) |
| `{"offset":0}`, `{"offset":1000000}` | diterima |
| `{"offset":-1}`, `{"offset":1000001}` | `offset harus 0-1000000` |
| `{"spec":1}` | `spec harus string` |
| `[]`, `1`, `"x"` | `arguments harus object JSON` |
| `{"limit":1}{"x":1}` | `harus satu object JSON` |
| `{"foo":1}` | `Argumen tidak valid: foo tidak dikenal. Argumen yang didukung: spec, tag, query, limit, offset.` |
| `{"project_id":"x"}`, `{"projectId":"x"}`, `{"project":"x"}` | pesan K5 (`ditentukan oleh API key`) — `TestT09_AC6_ProjectArgRejected` |
| `{"spec":"a","limit":0,"offset":-1}` | hanya error pertama (`limit`), sesuai urutan schema |
| `{"tag":"pets"}` | `Tag == "pets"`, tanpa normalisasi huruf |
| `TestLimitsMatchContract` | `maxLimit == listendpoints.MaxLimit`, `defaultLimit == listendpoints.DefaultLimit`, `maxQueryRunes == listendpoints.MaxQueryRunes` |
| `TestK4_ParseNeverYieldsProviderClampedValues` | untuk semua input tabel yang lolos: `1 <= Limit <= 300`, `Offset >= 0`, `RuneCount(Query) <= 200` (provider tidak pernah perlu clamp) |
| `TestSchema_Valid` | `schema()` JSON valid, `additionalProperties:false`, `minimum/maximum/default` limit dan offset sama dengan konstanta |

### `listapiendpoints/usecase_test.go` (fake `Lister` mencatat request)

- `TestExecute_ForwardsRequest`: `Execute(ctx, "proj-1", Params{Spec:"petstore", Tag:"pets", Query:"pet", Limit:50, Offset:10})` menghasilkan `Request == {ProjectID:"proj-1", Spec:"petstore", Tag:"pets", Query:"pet", Limit:50, Offset:10}`.
- `TestExecute_LimitAlwaysExplicit`: `Params` hasil `Parse("{}")` meneruskan `Limit == 100` (bukan 0).
- `TestExecute_ProjectIDFromArgumentOnly`: `ProjectID` hanya dari parameter `projectID`.
- `TestExecute_BuildsPage`: resp 3 item + Total 3 menghasilkan `Outcome.Page.Total == 3`, `Miss == nil`, `Result().IsError == false`, budget dari konstruktor dipakai (budget kecil memotong baris).
- `TestExecute_SpecNotFound_ReturnsMiss`: resp `SpecFound=false, Specs:[a b]` menghasilkan `Outcome.Miss != nil`, `Outcome.Result().IsError == true`.
- `TestExecute_PortError_Propagated`: port mengembalikan error; `errors.Is(err, sentinel)` benar, Outcome nol.

### `listapiendpoints/handler_test.go` (`bus.NewRegistry(nil)` + `Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, fake)`; handler dirakit lewat `Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: reg}).Handler`)

- `TestHandler_InvalidArgs_BusNotCalled`: `{"limit":500}` menghasilkan `IsError`, teks memuat `limit harus 1-300`, counter fake `0`, `err == nil`, `Summary["error_code"] == "invalid_argument"`, `Summary["field"] == "limit"`.
- `TestT09_AC6_ProjectIDIgnored`: `Call{ProjectID:"P-A", Arguments: {"project_id":"P-B"}}` ditolak dengan pesan K5, counter `0`. Varian argumen valid: `req.(listendpoints.Request).ProjectID == "P-A"`.
- `TestHandler_Success_Text`: fake mengembalikan 2 item pada 1 spec; `Text` memuat `Endpoint 1-2 dari 2 | spec: semua`, `== petstore ==`, `GET /pets — List all pets [listPets] (pets)`; `IsError == false`; `TargetType == "api_spec"`.
- `TestHandler_Success_RequestAsserted`: fake memverifikasi `Limit == 100` ketika `arguments` `{}`.
- `TestHandler_SpecNotFound`: fake `SpecFound=false, Specs:["billing","petstore"]`; `IsError`, teks `Spec tidak ditemukan: petstor`, `billing, petstore`; `Target == "petstor"`; `err == nil`.
- `TestHandler_Total0`: fake Total 0 dengan filter menghasilkan pesan saran, `IsError == false`.
- `TestHandler_OffsetOutOfRange`: fake Total 12, Items kosong, `{"offset":5000}` menghasilkan `IsError`, `offset_out_of_range`.
- `TestHandler_ProviderError_Wrapped`: fake error menghasilkan `Result{}` + error yang memuat `list_api_endpoints:` dan `errors.Is(err, sentinel)`.
- `TestHandler_DeadlineExceeded_Wrapped`: fake mengembalikan `context.DeadlineExceeded`; `errors.Is(err, context.DeadlineExceeded)` benar.
- `TestHandler_EndpointNotRegistered`: bus tanpa handler menghasilkan `errors.Is(err, bus.ErrNotRegistered)`.
- `TestHandler_Summary_NoSpecText`: `Summary` tidak memuat string summary/path spec (scan nilai bertipe string).

### Integrasi (`backend/cmd/internal/server/apispectools_integration_test.go`, `//go:build integration`)

Memakai fixture overview 10.3. Project A: `petstore` (3 endpoint: `GET /pets`, `POST /pets` deprecated, `GET /pets/{petId}`), `billing` (2: `GET /invoices`, `POST /invoices/{id}/pay`), `huge` (1: `GET /report`); total 6. Urutan keluaran: billing, huge, petstore.

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT09_Int_AC1_ListEndpointsGrouped` | 1 | key A, `list_api_endpoints {}` | `isError:false`; `Endpoint 1-6 dari 6 \| spec: semua`; `== billing ==`, `== huge ==`, `== petstore ==` berurutan; `POST /pets — ... [createPet] (pets) [deprecated]` |
| `TestT09_Int_AC1_TagAndQuery` | 1 | `{tag:"pets"}`; `{query:"invoice"}`; `{spec:"petstore", query:"petId"}` | 3 endpoint (hanya petstore); 2 endpoint (hanya billing); `Endpoint 1-1 dari 1` |
| `TestT09_Int_ListEndpointsPaged` | 1 | `{limit:2}`, `{limit:2, offset:2}`, `{limit:2, offset:4}` | halaman 1 memuat `[ada 4 endpoint lagi, panggil list_api_endpoints dengan offset=2 ...]`; halaman 2 memuat `== huge ==` lalu `== petstore ==`; halaman 3 tanpa penanda; gabungan semua halaman = 6 endpoint tanpa duplikat |
| `TestT09_Int_UnknownSpecList` | 1 | `{spec:"petstor"}` | `isError:true`; `Spec tidak ditemukan: petstor`; `billing, huge, petstore` |
| `TestT09_Int_ListEndpoints_NoMatch` | 1 | `{tag:"tidak-ada"}` | `isError:false`; teks `Tidak ada endpoint yang cocok (spec: semua, tag: tidak-ada)` |
| `TestT09_Int_ListEndpoints_OffsetOutOfRange` | 1 | `{offset:5000}` | `isError:true`; `offset 5000 melewati total 6 endpoint` |
| `TestT09_Int_ListEndpoints_InvalidLimit` | 1 | `{limit:500}` | `isError:true`; `limit harus 1-300 (diterima: 500)` |
| `TestT09_Int_ListEndpoints_NoSpecsProject` | 1 | project tanpa spec (`newProjectWithKey` baru), `{}` | `isError:false`; `Belum ada API spec di project ini` |
| `TestT09_Int_AC6_ListEndpointsProjectArg` | 6 | key A, `{project_id:<B>}` | `isError:true`, `ditentukan oleh API key`; hasil tanpa `project_id` dengan key B hanya memuat `GET /health` (project tetap sesuai key) |
| `TestT09_Int_AuditListEndpoints` | K13 | satu panggilan sukses | tepat satu entri `mcp.list_api_endpoints`, `target_type=api_spec`, result memuat `count`, `total`, `bytes`; tidak memuat summary/path endpoint |

---

## E2E Scenarios

Positif:
1. Agent: `list_api_specs {}` melihat `petstore`; `list_api_endpoints {spec:"petstore"}` menampilkan `GET /pets/{petId} — Info for a specific pet [showPetById] (pets)`; lalu `get_api_endpoint` untuk detail.
2. Tanpa `spec`: seluruh spec tampil dikelompokkan `== <spec> ==`, diurut nama spec.
3. Penyaringan `tag:"pets"` dan `query:"invoice"` menghasilkan subset yang benar; keduanya digabung bersifat AND (ditangani provider).
4. Spec besar: `{limit:100}` terpotong, penanda `offset=100`; agent memanggil ulang `{limit:100, offset:100}` sampai tanpa penanda. Angka offset selalu `Offset + baris yang benar-benar tampil`, jadi tidak ada endpoint yang terlewat walau teks terpotong anggaran byte.
5. `MCP_MAX_RESPONSE_BYTES` diturunkan (mis. 4096): teks tetap `<= 3584` byte, baris dipotong dengan penanda, tidak ada pemotongan generik gateway.
6. Endpoint `deprecated` tampil dengan `[deprecated]`; endpoint tanpa tag/operationId tampil tanpa `()`/`[]`.
7. Total 0 dengan filter menghasilkan saran (bukan error) sehingga agent mencoba tanpa filter.
8. Project tanpa spec menghasilkan `Belum ada API spec di project ini. ...` dengan `isError:false`.

Negatif:
1. `{limit:500}` ditolak (`limit harus 1-300`), bukan di-clamp; bus tidak dipanggil.
2. `{limit:"x"}` menghasilkan `limit harus integer`.
3. `{offset:-1}` menghasilkan `offset harus 0-1000000`.
4. `{query:<201 rune>}` ditolak, tidak dipotong diam-diam.
5. `{offset:5000}` pada 12 endpoint menghasilkan `isError` `offset_out_of_range`.
6. `{spec:"petstor"}` menghasilkan `isError` dengan daftar spec valid.
7. `{project_id:"..."}`/`{projectId:...}`/`{project:...}` menghasilkan pesan K5; project tidak pernah berubah (AC6).
8. Field tak dikenal (`{"sort":"asc"}`) menghasilkan `sort tidak dikenal` beserta daftar argumen yang didukung.
9. Provider tidak terdaftar atau timeout menghasilkan error Go; gateway menjawab `Terjadi kesalahan internal saat menjalankan tool list_api_endpoints. trace_id=...` atau teks timeout.

---

## Request/Response body

Semua respons dibungkus JSON-RPC 2.0 oleh gateway T07; di bawah ini `result.content[0].text` dan field audit tool.

### Sukses, semua spec (tanpa filter)

Request:
```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_api_endpoints","arguments":{}}}
```

Respons:
```json
{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"Endpoint 1-6 dari 6 | spec: semua\n== billing ==\nGET /invoices — List invoices [listInvoices] (invoices)\nPOST /invoices/{id}/pay — Pay an invoice (invoices)\n== huge ==\nGET /report — Big report\n== petstore ==\nGET /pets — List all pets [listPets] (pets)\nPOST /pets — Create a pet [createPet] (pets) [deprecated]\nGET /pets/{petId} — Info for a specific pet [showPetById] (pets)\n\nDetail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id)."}],"isError":false}}
```

Metadata audit dari tool (gateway menambah `bytes`): `TargetType:"api_spec"`, `Target:""`, `Summary:{"count":6,"total":6,"next_offset":-1,"spec_groups":3,"truncated":false}`.

### Sukses, terpotong (filter tag + limit)

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
Audit: `Summary:{"count":2,"total":3,"next_offset":2,"spec_groups":1,"truncated":true}`.

### Sukses, halaman lanjutan dengan query

Request: `{"name":"list_api_endpoints","arguments":{"spec":"petstore","query":"pet","limit":1,"offset":1}}`

Text:
```
Endpoint 2-2 dari 3 | spec: petstore | query: "pet"
== petstore ==
POST /pets — Create a pet [createPet] (pets) [deprecated]

[ada 1 endpoint lagi, panggil list_api_endpoints dengan offset=2 dan argumen lain yang sama]
Detail: get_api_endpoint(spec, method, path) atau get_api_endpoint(spec, operation_id).
```
Audit: `Target:"petstore"`.

### Total 0 (isError false)

Request: `{"name":"list_api_endpoints","arguments":{"spec":"petstore","tag":"tidak-ada"}}`

Text:
```
Tidak ada endpoint yang cocok (spec: petstore, tag: tidak-ada). Hapus filter tag/query, periksa nama tag dari list_api_endpoints tanpa filter, atau lihat spec dengan list_api_specs.
```
Audit: `Summary:{"count":0,"total":0,"next_offset":-1,"spec_groups":0,"truncated":false}`.

### Project tanpa spec (isError false)

```
Belum ada API spec di project ini. Spec diunggah anggota project lewat web UI; untuk dokumentasi lain gunakan list_files.
```

### Error: spec tidak dikenal (isError true)

Request: `{"name":"list_api_endpoints","arguments":{"spec":"petstor"}}`
```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"Spec tidak ditemukan: petstor. Spec yang valid: billing, huge, petstore. Nama spec persis seperti di list_api_specs."}],"isError":true}}
```
Audit: `TargetType:"api_spec"`, `Target:"petstor"`, `Summary:{"error_code":"spec_not_found","spec_count":3}`.

### Error: offset melewati total (isError true)

Request: `{"name":"list_api_endpoints","arguments":{"offset":5000}}`
```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"Argumen tidak valid: offset 5000 melewati total 6 endpoint. Gunakan offset 0 sampai 5."}],"isError":true}}
```
Audit: `Summary:{"error_code":"offset_out_of_range","total":6}`.

### Error: argumen tidak valid (bus tidak dipanggil)

Request: `{"name":"list_api_endpoints","arguments":{"limit":500}}`
```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya."}],"isError":true}}
```
Audit: `Summary:{"error_code":"invalid_argument","field":"limit"}`.

Request: `{"name":"list_api_endpoints","arguments":{"query":"<201 karakter>"}}` menghasilkan teks `Argumen tidak valid: query maksimal 200 karakter (diterima: <40 rune pertama>...). Persingkat kata kunci.`

### Error: argumen project (AC6)

Request: `{"name":"list_api_endpoints","arguments":{"project_id":"b3f1..."}}`
```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```

### Error: field tak dikenal

Request: `{"name":"list_api_endpoints","arguments":{"sort":"asc"}}`

Text: `Argumen tidak valid: sort tidak dikenal. Argumen yang didukung: spec, tag, query, limit, offset.`

### Error internal / timeout

Provider error atau bus tidak terdaftar: tool mengembalikan error Go `list_api_endpoints: <err>`; gateway T07 menjawab `Terjadi kesalahan internal saat menjalankan tool list_api_endpoints. trace_id=...` (atau teks timeout bila `context.DeadlineExceeded`). T09 tidak mengubah teks tersebut.

### Kontrak `tools/list` (entri tool ini, dari overview 12.1)

```json
{
  "name": "list_api_endpoints",
  "title": "Daftar endpoint API",
  "description": "Daftar endpoint API (read-only), satu baris per operasi: METHOD /path — ringkasan [operationId] (tag). Tanpa spec = semua spec, dikelompokkan per spec. Saring dengan tag (persis) atau query (cocok sebagian pada path, ringkasan, operationId). Lanjutkan daftar terpotong dengan offset. Ambil detail dengan get_api_endpoint.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "spec":   { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; kosong = semua spec" },
      "tag":    { "type": "string", "maxLength": 200, "description": "tag persis (peka huruf besar/kecil)" },
      "query":  { "type": "string", "maxLength": 200, "description": "kata yang dicari pada path, ringkasan, atau operationId" },
      "limit":  { "type": "integer", "minimum": 1, "maximum": 300, "default": 100, "description": "jumlah endpoint per halaman" },
      "offset": { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah endpoint; pakai nilai dari penanda [ada N endpoint lagi ...]" }
    },
    "additionalProperties": false
  },
  "annotations": { "readOnlyHint": true }
}
```
