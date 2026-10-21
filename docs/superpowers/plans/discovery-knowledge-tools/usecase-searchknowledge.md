# T08 discovery/knowledge-tools — Usecase `searchknowledge` (tool MCP `search_knowledge`)

Tiket: `docs/tickets/T08-discovery-knowledge-tools.md`
Induk (WAJIB diikuti, menang bila bertentangan): `docs/superpowers/plans/discovery-knowledge-tools/overview.md` (selanjutnya "overview"), terutama D1-D13, bagian 5 (`base/`), dan bagian 9 (allowlist import).
Aturan arsitektur: `docs/tickets/README.md` dan `rules.txt`.

Prasyarat: gerbang F1 dan langkah F2-F4 overview selesai (T07 ter-merge, `toolkit.Result.Structured` ada, `base/{names,args,path,format,result}.go` ada, `knowledgetools/base.go` ada). Bila T07 belum merge, hentikan di F1 dan lapor; plan ini tidak bisa dikerjakan lebih dulu.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T08 (ukuran S) |
| Scope | fullstack menurut tiket; UI "Tidak ada", maka hanya backend. Tanpa frontend plan, tanpa bagian Visual |
| Domain / Module | discovery / knowledge-tools (package `knowledgetools`) |
| Usecase | `searchknowledge` |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| Endpoint | `/mcp` (milik T07, tidak dibuat di sini) |
| Method | POST, JSON-RPC 2.0 `tools/call` dengan `params.name="search_knowledge"`, serta `tools/list` (satu entri tambahan) |
| Version | MCP 2025-11-25 / 2025-06-18 / 2025-03-26 (dinegosiasikan T07) |
| Tool | name `search_knowledge`, title `Cari knowledge`, `readOnlyHint:true` (ditambahkan gateway) |
| Bus dikonsumsi | `project.knowledge.search` (`constants.BUS_PROJECT_KNOWLEDGE_SEARCH`), Call sinkron, provider T05 |
| Bus disediakan | tidak ada |
| Audit | aksi `mcp.search_knowledge` (sudah ada di `record.Actions()`), ditulis gateway, bukan tool |
| REST / endpointsChanged | tidak ada / false |

---

## 2. Summary Plan

`search_knowledge` mencari knowledge project secara full-text lewat satu panggilan bus ke provider T05. Tool hanya: (1) memvalidasi argumen (`query` 1-200 rune, `path` prefix opsional, `limit` 1-20 default 10), (2) memanggil bus dengan `ProjectID` dari `toolkit.Call`, (3) merender hasil menjadi teks ramah LLM (path, skor 3 desimal, cuplikan berisi `**term**`) plus `structuredContent`.

Pembagian tanggung jawab (rules 4 dan 7.4):

| Lapisan | Tanggung jawab |
|---|---|
| `params.go` | decode ketat + rentang. Karakter khusus query TIDAK ditolak dan TIDAK diubah (sanitasi tsquery milik T05, AC4) |
| `handler.go` | adaptor `toolkit.Handler`: Parse, Execute, pemetaan error. Tanpa format teks |
| `usecase.go` | orkestrasi: panggil `Searcher`, lalu `base.NewSearchHits` |
| `repository.go` | satu-satunya yang menyentuh bus (`bus.CallTyped`). Tidak ada SQL di module ini |
| `base/searching.go` | entity `SearchHits` murni: teks, Structured, Summary, `Result()` |

Satu usecase, tidak dipecah: parse, search, render adalah satu alur linear.

Penyimpangan kecil dari teks perintah: perintah menulis `base.NewSearchHits(params, resp)`, tetapi `base/` tidak boleh meng-import package feature (siklus dan melanggar layering). Konstruktor mengikuti kontrak yang dibekukan overview 5.6: `NewSearchHits(query, prefix string, resp search.Response)`. Usecase memanggilnya dengan `p.Query, p.Prefix`. Isi dan perilakunya sama.

---

## 3. Keputusan khusus usecase

| ID | Keputusan | Alasan |
|---|---|---|
| S1 | `Limit` selalu dikirim eksplisit ke bus (default 10 diisi di Parse). Nilai > 20 ditolak, bukan di-clamp (overview D5) | Kontrak T05 punya `MaxLimit=50` dan `DefaultLimit=20`; tool sengaja lebih ketat agar keluaran kecil |
| S2 | `Query` hanya di-`TrimSpace`. Tidak ada penghapusan atau escape karakter. Panjang dihitung `utf8.RuneCountInString` | AC4; satu sumber sanitasi di T05 |
| S3 | Tanpa hasil: `IsError=false`, `Structured={items:[],total:0,query}` | Tanpa hasil bukan kegagalan; LLM butuh saran, bukan error |
| S4 | `Structured` memuat `snippet` (bukan isi utuh file). `Summary` tidak memuat snippet maupun query | Overview 5.6; query sudah ada di `Args` audit gateway |
| S5 | `SearchHits.Total` dinormalisasi `max(resp.Total, len(resp.Items))` | Cegah penanda "ada -1 hasil lain" bila provider salah |
| S6 | Snippet di `Text()` diratakan ke satu baris (`base.OneLine`) lalu di-`TrimSpace`; di `Structured` apa adanya | Teks rapi; data asli tetap tersedia |
| S7 | Bila snippet kosong setelah diratakan dan di-trim, baris cuplikan dilewati (tidak ada baris berisi tiga spasi saja) | Hindari baris kosong yang tampak seperti bug |
| S8 | Hint `Permintaan ditolak` per tool: `Periksa path atau hapus filter path.` | Satu-satunya `ErrInvalid` yang mungkin dari T05 untuk tool ini adalah prefix tidak valid (ProjectID selalu valid dari gateway) |

Risiko yang dicatat: query dengan karakter NUL (`\u0000`) bisa ditolak driver Postgres. Bila T05 mengembalikan error non-`ErrInvalid`, hasilnya `internal`. Itu tanggung jawab sanitasi T05; di T08 tidak ada tes untuk kasus ini, tetapi developer yang menemukan perilaku itu melapor ke pemilik T05, bukan menambal di sini.

---

## 4. Plan (langkah berurutan)

Branch kerja mengikuti overview (`feat/discovery-knowledge-tools`). Semua path relatif terhadap `backend/internal/domain/discovery/internal/knowledgetools/` kecuali disebut lain. Urutan TDD: tulis tes dulu, lalu implementasi.

**U1. Cek prasyarat lokal.** `go doc grimoire/lib/contracts/project/knowledge/search` menampilkan `Request{ProjectID,Query,Prefix,Limit}`, `Response{Items,Total}`, `Item{Path,Score,Snippet}`. `base/args.go`, `base/path.go`, `base/format.go`, `base/result.go`, `base/names.go` sudah ada (F3). Jika belum, hentikan dan selesaikan F3 dulu.

**U2. Entity `base/searching.go` (+ `base/searching_test.go`).**

```go
package base

import (
	"fmt"
	"strings"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/search"
)

// SearchHits adalah hasil pencarian knowledge yang siap dirender untuk LLM. Murni: tanpa I/O.
type SearchHits struct {
	Query  string
	Prefix string // sudah ternormalisasi (tanpa "/" depan/belakang); "" = seluruh project
	Items  []search.Item
	Total  int // jumlah dokumen cocok sebelum limit; selalu >= len(Items)
}

func NewSearchHits(query, prefix string, resp search.Response) SearchHits // S5: Total = max(resp.Total, len(resp.Items))
func (h SearchHits) Text() string
func (h SearchHits) Structured() map[string]any
func (h SearchHits) Summary() map[string]any
func (h SearchHits) Result() toolkit.Result
```

Aturan `Text()` (baris digabung dengan `"\n"`, tanpa newline di akhir, tanpa baris kosong; kutip pada query adalah tanda kutip ganda literal `"`, query ditulis apa adanya):

Ada hasil (`len(Items) > 0`):
1. Baris 1: `<len(Items)> dari <Total> hasil untuk "<Query>"`, ditambah ` di <Prefix>/` bila `Prefix != ""`.
2. Per hasil ke-n (mulai 1): baris `<n>. <Path>  (skor <Score %.3f>)` (dua spasi sebelum tanda kurung). Lalu, bila snippet tidak kosong setelah `TrimSpace(OneLine(Snippet))`, satu baris `   <snippet>` (indentasi 3 spasi).
3. Bila `Total > len(Items)`: `[ada <Total-len(Items)> hasil lain; persempit query atau path, atau naikkan limit (maks 20)]`.
4. Selalu terakhir: `Baca isi lengkap dengan read_file(path).`

Tanpa hasil (`len(Items) == 0`):
1. `Tidak ada hasil untuk "<Query>"`, ditambah ` di <Prefix>/` bila ada prefix.
2. `Coba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files.`

`Structured()`:

```json
{"items":[{"path":"...","score":0.6079,"snippet":"..."}],"total":1,"query":"rate limit"}
```
`items` selalu slice (`[]map[string]any{}` bila kosong, bukan nil, supaya JSON `[]`). `score` float64 apa adanya (tidak dibulatkan). `snippet` string apa adanya. `total` int, `query` string.

`Summary()`: `{"count": len(Items), "total": Total}`. Dilarang ada snippet, path hasil, atau query.

`Result()`: `toolkit.Result{Text: Text(), IsError: false, Structured: Structured(), TargetType: "folder", Target: Prefix, Summary: Summary()}`.

Import `base/searching.go`: stdlib, `toolkit`, `search` (sesuai allowlist overview bagian 5 dan 9).

**U3. `searchknowledge/params.go` (+ `params_test.go`).**

```go
package searchknowledge

const (
	maxQueryRunes = 200
	defaultLimit  = 10
	maxLimit      = 20
	maxPathBytes  = 1024
)

// Params adalah argumen tervalidasi.
type Params struct {
	Query  string // sudah TrimSpace, 1-200 rune, karakter khusus apa adanya
	Prefix string // base.NormalizePrefix; "" = akar
	Limit  int    // 1-20
}

type rawArgs struct {
	Query *string `json:"query"`
	Path  *string `json:"path"`
	Limit *int    `json:"limit"`
}

var allowedArgs = []string{"query", "path", "limit"}

func schema() json.RawMessage
func Parse(raw json.RawMessage) (Params, *base.ArgError)
```

Urutan `Parse` (hanya error pertama dilaporkan, urutan sama dengan schema; overview D6):
1. `base.DecodeStrict(raw, &ra, allowedArgs)`. Ini menolak field tak dikenal dan `project_id`/`projectId`/`project` dengan pesan khusus (AC5), serta tipe salah (`query` bukan string menjadi `query harus string`).
2. Query: `q := strings.TrimSpace(deref(ra.Query))`. Kosong menjadi `ArgError{Field:"query", Reason:"wajib diisi 1-200 karakter", Hint:"Contoh: query=\"rate limit\"."}` (tanpa `Got`). Pesan akhir persis: `Argumen tidak valid: query wajib diisi 1-200 karakter. Contoh: query="rate limit".`
3. `utf8.RuneCountInString(q) > 200` menjadi `ArgError{Field:"query", Reason:"maksimal 200 karakter", Got: base.Brief(q), Hint:"Ringkas menjadi kata kunci utama, atau pecah menjadi beberapa pencarian."}`. Tepat 200 rune diterima.
4. Prefix: `base.NormalizePrefix("path", deref(ra.Path))` (error diteruskan apa adanya; Hint dari `base` cukup). Path kosong atau `/` menjadi `""`.
5. Limit: `base.IntArg("limit", ra.Limit, 10, 1, 20, "Persempit dengan path=<folder> atau ubah kata kunci, bukan menaikkan limit.")`.
6. Kembalikan `Params`.

`schema()` memakai `json.Marshal` dari `map[string]any` (kunci alfabetis, stabil untuk golden), isi persis overview 12.1:

```json
{
  "type": "object",
  "properties": {
    "query": {"type":"string","minLength":1,"maxLength":200,"description":"kata kunci atau frasa dalam tanda kutip, mis. \"rate limit\""},
    "path":  {"type":"string","maxLength":1024,"description":"prefix folder opsional, mis. docs/guides"},
    "limit": {"type":"integer","minimum":1,"maximum":20,"default":10,"description":"jumlah hasil maksimal"}
  },
  "required": ["query"],
  "additionalProperties": false
}
```
Konstanta schema memakai `maxQueryRunes`, `maxLimit`, `defaultLimit`, `maxPathBytes` agar satu sumber kebenaran dengan `Parse`.

**U4. `searchknowledge/repository.go`.**

```go
package searchknowledge

// Searcher adalah port ke pencarian knowledge (diimplementasikan lewat bus T05).
type Searcher interface {
	Search(ctx context.Context, req search.Request) (search.Response, error)
}

type busSearcher struct{ bus *bus.Registry }

func newRepository(b *bus.Registry) Searcher { return busSearcher{bus: b} }

func (r busSearcher) Search(ctx context.Context, req search.Request) (search.Response, error) {
	return bus.CallTyped[search.Response](ctx, r.bus, constants.BUS_PROJECT_KNOWLEDGE_SEARCH, req)
}
```
Hanya meneruskan error mentah (tanpa membungkus `ErrInvalid`) supaya `errors.Is` tetap bekerja. Tidak ada logika, tidak ada SQL.

**U5. `searchknowledge/usecase.go`.**

```go
type usecase struct{ searcher Searcher }

func newUsecase(s Searcher) *usecase { return &usecase{searcher: s} }

// Execute memanggil pencarian untuk projectID (dari call.ProjectID) lalu membentuk entity hasil.
func (u *usecase) Execute(ctx context.Context, projectID string, p Params) (base.SearchHits, error) {
	resp, err := u.searcher.Search(ctx, search.Request{ProjectID: projectID, Query: p.Query, Prefix: p.Prefix, Limit: p.Limit})
	if err != nil {
		return base.SearchHits{}, err
	}
	return base.NewSearchHits(p.Query, p.Prefix, resp), nil
}
```
Hanya orkestrasi. Tidak ada format teks, tidak ada aturan rentang.

**U6. `searchknowledge/handler.go`.**

```go
func newHandler(u *usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, "folder", ""), nil
		}
		hits, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolSearchKnowledge, err, "Periksa path atau hapus filter path.", "folder", p.Prefix)
		}
		return hits.Result(), nil
	}
}
```
`BusResult` (overview 5.5): `errs.ErrInvalid` menjadi `Result{IsError:true, Text:"Permintaan ditolak: <pesan>. Periksa path atau hapus filter path.", Summary:{error_code:"invalid_request"}}` dengan err nil. Error lain (termasuk `bus.ErrNotRegistered`, deadline, cancel) menjadi `fmt.Errorf("search_knowledge: %w", err)`; gateway memetakan ke `internal`/`timeout`. Handler tidak membaca `Arguments` untuk `project_id`; project hanya dari `call.ProjectID`.

Bila `handler` mengembalikan `Result` dengan `Structured` untuk `ArgResult`/`BusResult`: nil (helper tidak mengisinya).

**U7. `searchknowledge/init.go`.**

```go
const (
	title       = "Cari knowledge"
	description = "Cari file knowledge berdasarkan kata kunci (full-text, read-only, bukan semantik). Frasa dalam tanda kutip dicari utuh. Hasil berisi path, skor, dan cuplikan dengan kata kunci bertanda **...**; baca file utuh dengan read_file(path). Persempit dengan path=<folder>."
)

// Init merakit tool search_knowledge: repository -> usecase -> handler.
func Init(d app.Deps) toolkit.Tool {
	uc := newUsecase(newRepository(d.Bus))
	return toolkit.Tool{
		Name:        base.ToolSearchKnowledge,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     newHandler(uc),
	}
}
```
Teks `description` disalin persis (dibekukan, ikut golden overview 12.1).

**U8. Wiring.** Tidak ada langkah baru: `knowledgetools/base.go` (F4) sudah memanggil `searchknowledge.Init(d)`; `discovery/base.go` (F6) sudah menambah `knowledgetools.Register(d)`. Jika F4 masih memakai stub `searchknowledge.Init`, ganti stub dengan U7 dan pastikan tidak ada stub tersisa untuk tool ini.

**U9. Golden dan tes module.** Setelah U7, `knowledgetools/base_test.go` (F7) mencakup entri `search_knowledge` pada `testdata/tools_list.golden.json`. Bandingkan hasil `-update` dengan overview 12.1 untuk blok `search_knowledge` (selisih hanya urutan kunci). Tidak ada tes baru di file ini oleh usecase ini.

**U10. Tes integrasi** (file `backend/cmd/internal/server/knowledgetools_integration_test.go` dibuat di F8; bagian usecase ini adalah tiga tes di bagian 7.5). Jika file belum ada, tambahkan tiga fungsi itu saat F8 dikerjakan; jangan membuat file kedua.

**U11. Gerbang.**
```
cd backend
go build ./...
go vet ./...
go test ./internal/domain/discovery/internal/knowledgetools/...
go test -race ./internal/domain/discovery/...
go test -tags integration -run 'TestT08_Int_AC4' ./cmd/internal/server/...
golangci-lint run ./internal/domain/discovery/...
```
Cek impor hanya yang diizinkan allowlist overview bagian 9 (`TestT08_AC7_ImportAllowlist` lulus).

---

## 5. Migration

Tidak ada. Tidak ada tabel, kolom, atau index. Data dibaca lewat bus T05.

---

## 6. Changes (per file/folder)

| File (relatif `knowledgetools/`) | Status | Isi |
|---|---|---|
| `base/searching.go` | baru | entity `SearchHits` (U2) |
| `base/searching_test.go` | baru | tes entity |
| `searchknowledge/init.go` | baru | `Init`, konstanta title/description |
| `searchknowledge/params.go` | baru | `Params`, `Parse`, `schema` |
| `searchknowledge/handler.go` | baru | adaptor `toolkit.Handler` |
| `searchknowledge/usecase.go` | baru | `Execute` |
| `searchknowledge/repository.go` | baru | `Searcher` + implementasi bus |
| `searchknowledge/params_test.go` | baru | tes Parse |
| `searchknowledge/usecase_test.go` | baru | tes usecase (fake `Searcher`) |
| `searchknowledge/handler_test.go` | baru | tes handler (registry bus palsu) |
| `testdata/tools_list.golden.json` | ubah (milik F7) | entri `search_knowledge` |
| `backend/cmd/internal/server/knowledgetools_integration_test.go` | ubah/baru (F8) | tiga tes `AC4` |

Tidak berubah oleh usecase ini: `lib/constants/bus.go`, `lib/contracts/project/**`, `lib/contracts/core/audit/record`, `infra/config`, `api/**`, `migrations/**`, `frontend/**`, `go.mod`, dan base bersama (`args/path/format/result/names`). Bila ternyata butuh helper baru di base bersama, tambahkan secara aditif dengan tes (overview F5).

Impor tiap file mengikuti allowlist overview bagian 9: stdlib, `infra/app`, `infra/bus`, `lib/constants`, `lib/contracts/project/knowledge/search`, `toolkit`, `lib/common/errs`, dan package module sendiri. Dilarang `internal/domain/project/**` dan `.../mcpgateway/**`.

---

## 7. Test Scenario

### 7.1 `searchknowledge/params_test.go` (tabel, tanpa I/O)

| Tes | Input `arguments` | Ekspektasi |
|---|---|---|
| `TestParse_QueryRequired` | `{}` | `Field=="query"`; `Message()` persis `Argumen tidak valid: query wajib diisi 1-200 karakter. Contoh: query="rate limit".` |
| `TestParse_QueryEmptyOrBlank` | `{"query":""}`, `{"query":"   "}`, `{"query":"\t\n"}` | error sama seperti di atas |
| `TestParse_QueryWrongType` | `{"query":5}` | `Field=="query"`, `Reason=="harus string"` |
| `TestParse_QueryTrimmed` | `{"query":"  rate limit  "}` | `Query=="rate limit"` |
| `TestParse_Query200RunesMultibyte` | 200 x `"é"` (400 byte), juga 200 x `"日"` | diterima, `Query` identik |
| `TestParse_Query201Runes` | 201 x `"é"` | ditolak, `Field=="query"`, `Reason` memuat `maksimal 200`, `Hint` memuat `Ringkas` |
| `TestParse_Query200AsciiAccepted_201Rejected` | 200 dan 201 x `"a"` | 200 diterima, 201 ditolak |
| `TestParse_QueryTrimBeforeCount` | 200 `a` diapit 5 spasi tiap sisi | diterima (hitung setelah trim) |
| `TestT08_AC4_SpecialCharsAccepted` | query `a & b \| !c:* (x) 'y' "z" <tag>` (dikirim sebagai JSON string yang di-escape benar) | `err==nil`; `Query` sama persis dengan input setelah trim (tidak ada karakter yang diubah/dihapus). Tambahan tabel: `"`, `'"`, `*`, `:*`, `\\`, `%`, `_`, `--`, `;`, `<>`, emoji |
| `TestParse_LimitDefault` | `{"query":"a"}` | `Limit==10` |
| `TestParse_LimitBounds` | `limit` 1 dan 20 | diterima |
| `TestParse_LimitOutOfRange` | `limit` 0, -1, 21, 100 | ditolak, `Field=="limit"`, pesan memuat `harus 1-20` dan `(diterima: 21)` |
| `TestParse_LimitWrongType` | `{"query":"a","limit":"5"}`, `{"query":"a","limit":1.5}` | `Reason=="harus integer"` |
| `TestParse_PathNormalized` | `path`: `"/docs/guides/"`, `"  docs/guides  "`, `"docs"`, `"/"`, `""`, tanpa field | `Prefix`: `docs/guides`, `docs/guides`, `docs`, `""`, `""`, `""` |
| `TestParse_PathInvalid` | `"../etc"`, `"a/../b"`, `"a\u0000b"`, 1025 byte | ditolak, `Field=="path"` |
| `TestParse_Path1024Accepted` | 1024 byte | diterima |
| `TestT08_AC5_ProjectIDRejected` | `{"query":"a","project_id":"x"}`, `projectId`, `project` | error; `Message()` memuat `ditentukan oleh API key`; `Params` kosong |
| `TestParse_UnknownField` | `{"query":"a","foo":1}` | `Message()` memuat `foo tidak dikenal` dan `query, path, limit` |
| `TestParse_NotObject` | `[]`, `"x"`, `1`, `{"query":"a"}{"x":1}`, `{` | `Field=="arguments"` |
| `TestParse_NullAndEmptyRaw` | `null`, ``, ` ` | diperlakukan `{}`: error `query` wajib (bukan panic) |
| `TestParse_ErrorOrder` | `{"query":"","limit":99}` | yang dilaporkan `query` (urutan schema) |
| `TestSchema_MatchesConstants` | `schema()` | JSON valid; `required==["query"]`; `additionalProperties==false`; `query.maxLength==200`; `limit.default==10`, `maximum==20`; tidak ada properti project |

### 7.2 `base/searching_test.go` (tabel, tanpa I/O)

| Tes | Skenario | Ekspektasi |
|---|---|---|
| `TestSearchHits_TextWithResults` | 2 item, Total 2, prefix `""`, skor 0.6079 dan 1 | persis: baris 1 `2 dari 2 hasil untuk "rate limit"`; `1. docs/guides/b.md  (skor 0.608)`; `   **rate**  **limiting** ...` sesuai snippet; `2. ...  (skor 1.000)`; baris akhir `Baca isi lengkap dengan read_file(path).`; tidak ada `[ada` |
| `TestSearchHits_ScoreThreeDecimals` | skor 0.6, 0.12345, 0.9996, 12 | `0.600`, `0.123`, `1.000`, `12.000` |
| `TestSearchHits_PrefixInHeader` | prefix `docs/guides` | baris 1 berakhir ` di docs/guides/`; tanpa prefix tidak ada ` di ` |
| `TestSearchHits_SnippetHighlightKept` | snippet `**rate** **limiting** per key` | tampil apa adanya setelah indentasi 3 spasi |
| `TestSearchHits_SnippetNewlinesFlattened` | snippet `a\nb\r\nc\td` | teks `a b c d` (tidak ada `\n` di dalam baris cuplikan; jumlah baris teks sama dengan 1 + 2*n + penutup) |
| `TestSearchHits_EmptySnippetSkipsLine` | snippet `""` atau spasi | tidak ada baris cuplikan untuk item itu |
| `TestSearchHits_MoreMarkerOnlyWhenTotalGreater` | Items 3, Total 7 | memuat `[ada 4 hasil lain; persempit query atau path, atau naikkan limit (maks 20)]` sebelum `Baca isi lengkap`; Items 3, Total 3: tidak ada penanda |
| `TestSearchHits_TotalNormalized` | Items 3, Total 0 atau 1 | `Total==3`, tidak ada penanda, header `3 dari 3` |
| `TestSearchHits_NoResultsMessage` | Items kosong/nil, query `xyz` | teks persis `Tidak ada hasil untuk "xyz"\nCoba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files.`; dengan prefix: `Tidak ada hasil untuk "xyz" di docs/` |
| `TestSearchHits_NoResultsIsNotError` | Items kosong | `Result().IsError==false` |
| `TestSearchHits_QueryEchoedVerbatim` | query `a & b \| !c:* "z"` | muncul tak berubah di header |
| `TestSearchHits_Structured` | 1 item | `items[0]=={path,score:0.6079,snippet}` (snippet asli, tidak diratakan), `total`, `query`; di-marshal JSON menghasilkan kunci tepat `items,total,query` |
| `TestSearchHits_StructuredEmptyItemsIsArray` | tanpa item | JSON memuat `"items":[]` bukan `null` |
| `TestSearchHits_SummaryNoSnippetNoQuery` | 2 item, Total 5 | `Summary()=={"count":2,"total":5}` tepat dua kunci; marshal tidak memuat snippet, path hasil, maupun query |
| `TestSearchHits_Result` | prefix `docs` | `TargetType=="folder"`, `Target=="docs"`, `IsError==false`, `Text`/`Structured`/`Summary` terisi; prefix kosong menghasilkan `Target==""` |

### 7.3 `searchknowledge/usecase_test.go` (fake `Searcher`)

| Tes | Ekspektasi |
|---|---|
| `TestExecute_ForwardsRequest` | `Execute(ctx, "proj-1", Params{Query:"rate limit", Prefix:"docs", Limit:7})` memanggil fake dengan `search.Request{ProjectID:"proj-1", Query:"rate limit", Prefix:"docs", Limit:7}` tepat satu kali |
| `TestExecute_ProjectIDFromArgumentNotParams` | `Params` tidak punya field project; `ProjectID` request == argumen `projectID` Execute |
| `TestExecute_BuildsHits` | fake mengembalikan 2 item Total 5; hasil `Query`, `Prefix`, `len(Items)==2`, `Total==5` |
| `TestExecute_PropagatesError` | fake error `errs.ErrInvalid` dibungkus; `errors.Is(err, errs.ErrInvalid)` true dan `SearchHits` nol |
| `TestExecute_PropagatesCtx` | ctx yang sama diteruskan ke fake (cek lewat nilai context key) |
| `TestExecute_EmptyResponse` | fake `Response{}` menghasilkan `SearchHits` dengan Items kosong, err nil |

### 7.4 `searchknowledge/handler_test.go` (registry bus palsu)

Setup: `bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))`, daftarkan handler palsu pada `constants.BUS_PROJECT_KNOWLEDGE_SEARCH` yang mencatat request dan jumlah panggilan. Rakit lewat `Init(app.Deps{Bus: reg}).Handler`.

| Tes | Ekspektasi |
|---|---|
| `TestHandler_Success` | `call{ProjectID:"p1", Arguments:{"query":"rate limit","path":"/docs/","limit":5}}`; request bus == `{ProjectID:"p1", Query:"rate limit", Prefix:"docs", Limit:5}`; `IsError==false`; teks memuat path dan `(skor 0.608)`; `Structured["total"]` terisi |
| `TestHandler_DefaultLimitSentExplicit` | tanpa `limit` menghasilkan `Request.Limit==10` |
| `TestHandler_NoResults` | provider mengembalikan `Response{}`; `IsError==false`; teks memuat `Tidak ada hasil` dan `list_files` |
| `TestHandler_InvalidArgsDoNotCallBus` | `{}`, `{"query":"  "}`, `{"query":"a","limit":0}`, `{"query":"a","project_id":"x"}`, `{"query":"a","extra":1}`: `IsError==true`, err nil, `Summary["error_code"]=="invalid_argument"`, jumlah panggilan bus == 0 |
| `TestHandler_ProjectIDArgIgnored` (AC5) | argumen `project_id:"B"` pada call `ProjectID:"A"` ditolak; bus tidak dipanggil; tidak pernah ada request dengan project `B` |
| `TestHandler_ErrInvalidBecomesToolError` | provider mengembalikan `fmt.Errorf("prefix tidak valid: %w", errs.ErrInvalid)`; hasil `IsError==true`, err nil; teks diawali `Permintaan ditolak:` dan diakhiri `Periksa path atau hapus filter path.`; `Summary["error_code"]=="invalid_request"` |
| `TestHandler_OtherErrorBecomesGoError` | provider mengembalikan `errors.New("boom")`; `err != nil`, `errors.Is(err, boom)`; teks hasil kosong |
| `TestHandler_NotRegisteredIsGoError` | registry tanpa handler; `errors.Is(err, bus.ErrNotRegistered)` true |
| `TestHandler_DeadlineIsGoError` | provider mengembalikan `context.DeadlineExceeded`; `errors.Is(err, context.DeadlineExceeded)` (gateway butuh ini untuk kode `timeout`) |
| `TestHandler_BadRequestTypeIsGoError` | provider terdaftar menolak tipe (`bus.ErrBadRequestType`); error Go, bukan isError |
| `TestHandler_SummaryHasNoSnippetOrQuery` | hasil sukses: `Summary` hanya `count,total`; marshal Summary tidak memuat snippet maupun query |
| `TestHandler_AuditTarget` | `TargetType=="folder"`, `Target==prefix` |

### 7.5 Integrasi (`//go:build integration`, memakai setup overview 10.3)

Fixture project A (dari overview): `docs/guides/b.md` berisi `rate limiting per key`; tambahkan ke fixture `docs/a.md` bila perlu konten non-"rate" agar filter path bermakna (mis. `docs/a.md` berisi `rate card harga`, `readme.md` tanpa kata "rate"). Fixture project B: `secret/b.md` berisi `SECRET-B`.

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT08_Int_AC4_SearchSnippet` | 4 | `search_knowledge {query:"rate limit"}` | tidak ada error JSON-RPC; `isError:false`; teks memuat `docs/guides/b.md`, `(skor ` dan `**rate**`; baris pertama cocok `^\d+ dari \d+ hasil untuk "rate limit"`; `structuredContent.items[0].snippet` memuat `**rate**`; `structuredContent.total>=1` |
| `TestT08_Int_AC4_SpecialChars` | 4 | masing-masing query `a & b \| !c:* (x)` dan `'"` (juga `)(`, `:*`, `\\`, `<->` sebagai pelengkap) | respons tanpa `error` JSON-RPC; `isError:false` (hasil boleh kosong); `structuredContent.query` sama dengan input |
| `TestT08_Int_AC4_PrefixFilter` | 4 | `{query:"rate", path:"docs/guides"}`, `{query:"rate", path:"/docs/guides/"}`, `{query:"rate", path:"other"}` | dua pertama: hanya path berawalan `docs/guides/` (tidak ada `docs/a.md`); ketiga: `isError:false`, teks `Tidak ada hasil ... di other/` |
| `TestT08_Int_AC5_SearchProjectIsolation` | 5 | key A, `{query:"SECRET-B"}`; lalu `{query:"rate", project_id:<B>}` | pertama: tanpa hasil (A tidak melihat B), teks tidak memuat `SECRET-B` selain di echo query; kedua: `isError:true`, teks memuat `ditentukan oleh API key` |
| `TestT08_Int_AuditSearch` | D8 | satu `search_knowledge` sukses | tepat satu entri `mcp.search_knowledge`, `target_type=folder`; Result audit memuat `count`/`total`; tidak memuat snippet; query ada di `Args` |

---

## 8. E2E Scenarios

Positif:
1. Agent memanggil `search_knowledge {query:"rate limit"}`, mendapat daftar path + skor + cuplikan `**rate**`, lalu `read_file {path:"docs/guides/b.md"}` dari hasil. Alur dua langkah berjalan tanpa `list_files`.
2. Frasa dalam kutip: `{query:"\"rate limiting\""}` mengembalikan dokumen yang memuat frasa utuh (perilaku T05, diverifikasi sekilas di integrasi).
3. Persempit: `{query:"rate", path:"docs/guides"}` hanya mengembalikan hasil di bawah folder itu; header teks berisi ` di docs/guides/`.
4. Banyak hasil: `{query:"rate", limit:1}` pada >1 dokumen cocok menampilkan `1 dari N hasil` dan penanda `[ada N-1 hasil lain; ...]`.
5. Tanpa hasil: `{query:"zzzqqq"}` menghasilkan `isError:false` + saran kata kunci lain/`list_files`.
6. Karakter khusus: `{query:"a & b | !c:* (x)"}` tidak menghasilkan error (AC4); hasil boleh kosong.
7. Query 200 karakter (termasuk multibyte) diterima.

Negatif:
1. `{}` atau `{"query":"   "}`: `isError:true`, `query wajib diisi 1-200 karakter. Contoh: query="rate limit".` Bus tidak dipanggil.
2. Query 201 karakter: `isError:true`, `query maksimal 200 karakter`, saran meringkas.
3. `{"query":"a","limit":0}` dan `{"query":"a","limit":21}`: `isError:true`, `limit harus 1-20`.
4. `{"query":"a","limit":"5"}`: `isError:true`, `limit harus integer`.
5. `{"query":"a","project_id":"<B>"}`: `isError:true`, pesan khusus project; project tidak berubah dan bus tidak dipanggil (AC5).
6. `{"query":"a","path":"../etc"}`: `isError:true`, `path tidak boleh memuat segmen ..`.
7. Prefix ditolak provider (`ErrInvalid`): `isError:true`, `Permintaan ditolak: ... Periksa path atau hapus filter path.`, audit `error_code=invalid_request`.
8. Provider T05 tidak terdaftar atau timeout: error Go; gateway mengembalikan teks internal/timeout (format overview 12.6), audit `error_code=internal|timeout`.
9. Argumen tak dikenal `{"query":"a","foo":1}`: `isError:true`, `foo tidak dikenal. Argumen yang didukung: query, path, limit.`

---

## 9. Request/Response body

Semua melalui `POST /mcp` dengan header kunci API (diurus T07). Contoh `id` hanyalah ilustrasi.

### 9.1 `tools/list` (entri tool ini; gateway menambah `annotations`)

```json
{
  "name": "search_knowledge",
  "title": "Cari knowledge",
  "description": "Cari file knowledge berdasarkan kata kunci (full-text, read-only, bukan semantik). Frasa dalam tanda kutip dicari utuh. Hasil berisi path, skor, dan cuplikan dengan kata kunci bertanda **...**; baca file utuh dengan read_file(path). Persempit dengan path=<folder>.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "query": {"type": "string", "minLength": 1, "maxLength": 200, "description": "kata kunci atau frasa dalam tanda kutip, mis. \"rate limit\""},
      "path": {"type": "string", "maxLength": 1024, "description": "prefix folder opsional, mis. docs/guides"},
      "limit": {"type": "integer", "minimum": 1, "maximum": 20, "default": 10, "description": "jumlah hasil maksimal"}
    },
    "required": ["query"],
    "additionalProperties": false
  },
  "annotations": {"readOnlyHint": true}
}
```

### 9.2 Sukses, banyak hasil dan terpotong oleh limit

Request:
```json
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"rate limit","path":"docs","limit":2}}}
```
Bus: `search.Request{ProjectID:"<dari key>", Query:"rate limit", Prefix:"docs", Limit:2}`; respons provider `{Items:[{docs/guides/b.md, 0.6079, "**rate** **limiting** per key"},{docs/a.md, 0.3, "kartu **rate**\ndan harga"}], Total:5}`.

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "content": [{"type":"text","text":"2 dari 5 hasil untuk \"rate limit\" di docs/\n1. docs/guides/b.md  (skor 0.608)\n   **rate** **limiting** per key\n2. docs/a.md  (skor 0.300)\n   kartu **rate** dan harga\n[ada 3 hasil lain; persempit query atau path, atau naikkan limit (maks 20)]\nBaca isi lengkap dengan read_file(path)."}],
    "isError": false,
    "structuredContent": {
      "items": [
        {"path":"docs/guides/b.md","score":0.6079,"snippet":"**rate** **limiting** per key"},
        {"path":"docs/a.md","score":0.3,"snippet":"kartu **rate**\ndan harga"}
      ],
      "total": 5,
      "query": "rate limit"
    }
  }
}
```
Audit (gateway): `action=mcp.search_knowledge`, `target_type=folder`, `target="docs"`, summary `{count:2,total:5}` ditambah field gateway; `Args` memuat query.

### 9.3 Sukses, satu hasil tanpa prefix

```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"1 dari 1 hasil untuk \"rate limit\"\n1. docs/guides/b.md  (skor 0.608)\n   **rate** **limiting** per key\nBaca isi lengkap dengan read_file(path)."}],"isError":false,"structuredContent":{"items":[{"path":"docs/guides/b.md","score":0.6079,"snippet":"**rate** **limiting** per key"}],"total":1,"query":"rate limit"}}}
```

### 9.4 Tanpa hasil (bukan error)

Request: `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"xyz","path":"/other/"}}}`

```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Tidak ada hasil untuk \"xyz\" di other/\nCoba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files."}],"isError":false,"structuredContent":{"items":[],"total":0,"query":"xyz"}}}
```

### 9.5 Karakter khusus (AC4, tidak error)

Request:
```json
{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"a & b | !c:* (x)"}}}
```
Response (contoh bila tidak ada kecocokan):
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Tidak ada hasil untuk \"a & b | !c:* (x)\"\nCoba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files."}],"isError":false,"structuredContent":{"items":[],"total":0,"query":"a & b | !c:* (x)"}}}
```

### 9.6 Format error

Query kosong (`invalid_argument`, bus tidak dipanggil):
```json
{"jsonrpc":"2.0","id":14,"result":{"content":[{"type":"text","text":"Argumen tidak valid: query wajib diisi 1-200 karakter. Contoh: query=\"rate limit\"."}],"isError":true}}
```
Summary audit: `{error_code:"invalid_argument", field:"query"}`.

Query terlalu panjang:
```json
{"jsonrpc":"2.0","id":15,"result":{"content":[{"type":"text","text":"Argumen tidak valid: query maksimal 200 karakter (diterima: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa...). Ringkas menjadi kata kunci utama, atau pecah menjadi beberapa pencarian."}],"isError":true}}
```

Limit di luar rentang:
```json
{"jsonrpc":"2.0","id":16,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-20 (diterima: 21). Persempit dengan path=<folder> atau ubah kata kunci, bukan menaikkan limit."}],"isError":true}}
```

Argumen project (AC5):
```json
{"jsonrpc":"2.0","id":17,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```

Argumen tak dikenal:
```json
{"jsonrpc":"2.0","id":18,"result":{"content":[{"type":"text","text":"Argumen tidak valid: foo tidak dikenal. Argumen yang didukung: query, path, limit."}],"isError":true}}
```

Ditolak provider (`errs.ErrInvalid` dari T05, `error_code=invalid_request`):
```json
{"jsonrpc":"2.0","id":19,"result":{"content":[{"type":"text","text":"Permintaan ditolak: prefix tidak valid: invalid argument. Periksa path atau hapus filter path."}],"isError":true}}
```
Isi kalimat sebelum `Periksa path` adalah `err.Error()` dari T05 apa adanya (contoh di atas ilustratif).

Error internal (error Go dari tool, dipetakan gateway, detail disembunyikan):
```json
{"jsonrpc":"2.0","id":20,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool search_knowledge. trace_id=7f3c2a9e1b4d"}],"isError":true}}
```

Timeout (ctx deadline, dipetakan gateway):
```json
{"jsonrpc":"2.0","id":21,"result":{"content":[{"type":"text","text":"Tool search_knowledge melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```

Nama tool tidak dikenal, auth, dan rate limit tetap ditangani T07; tidak berubah oleh usecase ini.
