# T08 usecase `listfiles` — Tool MCP `list_files`

Tiket: `docs/tickets/T08-discovery-knowledge-tools.md`
Foundation (mengikat, menang bila bertentangan): `docs/superpowers/plans/discovery-knowledge-tools/overview.md`
Keputusan bersama yang dipakai: D1 (Structured), D3 (argumen ketat, AC5), D4 (normalisasi path), D5 (tolak, bukan clamp), D6 (format ArgError), D8 (audit hanya metadata), D9 (format ukuran/waktu), D10 (penanda), D11, D13 (pemetaan error bus).

Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` tidak relevan (tanpa migrasi); tidak ada knowledge doc module; tidak ada frontend (tiket: UI "Tidak ada"), jadi tidak ada bagian Visual. Kontrak T05 `lib/contracts/project/knowledge/listfiles` sudah di `main` (`Request{ProjectID,Prefix,Recursive,Limit,Offset}`, `Response{Items,Folders,Total,Truncated}`, `DefaultLimit=100`, `MaxLimit=500`).

---

## Metadata

| Atribut | Nilai |
|---|---|
| Endpoint | `/mcp` (milik T07; tidak ada endpoint REST baru) |
| Method | `POST`, JSON-RPC 2.0 `tools/call` dengan `params.name = "list_files"` (tampil juga di `tools/list`) |
| Version | MCP 2025-11-25 / 2025-06-18 / 2025-03-26 (dinegosiasikan T07) |
| Scope | backend saja (tiket fullstack, tetapi tanpa UI); `endpointsChanged = false` |
| Domain | discovery |
| Module | knowledge-tools (package `knowledgetools`) |
| Feature | `listfiles` |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| Bus dikonsumsi | `project.knowledge.list-files` (Call sinkron, provider T05) via `constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES` |
| Bus disediakan | tidak ada |
| Aksi audit | `mcp.list_files` (sudah ada di `record.Actions()`; ditulis gateway T07, bukan tool ini) |
| Migrasi | tidak ada |
| Gerbang | T07 belum merge ke `main` (lihat overview bagian 0). Mulai implementasi hanya setelah F1-F3 overview selesai |

---

## Summary Plan

`list_files` adalah tool read-only untuk menjelajah struktur knowledge project. Alurnya: handler mem-parse argumen (ketat, tolak `project_id`), usecase memanggil `Lister` (satu `bus.CallTyped` ke T05) lalu membangun entity `base.ListPage`; entity murni itu yang merender teks, `structuredContent`, ringkasan audit, dan kasus kosong/offset di luar rentang. `ProjectID` hanya berasal dari `call.ProjectID`.

Poin desain:
- Satu usecase, tidak dipecah. Rule (kapan folder tampil, penanda lanjutan, pesan kosong, offset di luar rentang) ada di `ListPage`, bukan di usecase atau handler.
- `NewListPage` menerima nilai primitif `(prefix, recursive, offset, resp)` sesuai kontrak beku overview 5.6, bukan struct `Params`, karena `base/` tidak boleh mengimpor package `listfiles` (siklus impor).
- Folder hanya ditampilkan bila `!Recursive && Offset == 0` supaya halaman lanjutan tidak mengulang folder.
- `Limit` ke T05 selalu eksplisit (1-500 hasil `Parse`), jadi tidak bergantung pada `DefaultLimit=100` T05 yang berbeda dari default tool (200).
- Tidak ada state, cache, goroutine, atau SQL.

---

## Plan (langkah berurutan)

Prasyarat: F1 (gerbang T07), F2 (`toolkit.Result.Structured` + `structuredContent` di endpoint), dan F3 (`base/{names,args,path,format,result}.go` beserta tes) dari overview sudah selesai. Semua jalur di bawah relatif terhadap resource root `backend/internal/domain/discovery/internal/knowledgetools`.

**L1. Entity `base/listing.go`** (TDD: tulis `base/listing_test.go` dulu).

```go
package base

import (
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

// ListPage adalah satu halaman hasil list_files. Murni, tanpa I/O.
type ListPage struct {
	Prefix    string           // sudah dinormalisasi, "" = akar
	Recursive bool
	Offset    int
	Items     []listfiles.Item // urutan persis dari respons T05
	Folders   []string         // HANYA terisi bila !Recursive && Offset == 0; selain itu nil
	Total     int              // jumlah file yang cocok (bukan hanya halaman ini)
}

func NewListPage(prefix string, recursive bool, offset int, resp listfiles.Response) ListPage
func (p ListPage) NextOffset() int
func (p ListPage) OffsetOutOfRange() bool
func (p ListPage) Text() string
func (p ListPage) Structured() map[string]any
func (p ListPage) Summary() map[string]any
func (p ListPage) Result() toolkit.Result
```

Aturan:

1. `NewListPage`: menyalin `resp.Items` dan `resp.Total`. `Folders = resp.Folders` hanya bila `!recursive && offset == 0`, selain itu `nil`. Setiap folder dinormalisasi (`strings.Trim(f, "/")`) dan entri kosong dibuang, supaya Text dan Structured konsisten apa pun gaya slash dari T05.
2. `NextOffset()`: `n := p.Offset + len(p.Items)`; `n < p.Total` mengembalikan `n`, selain itu `-1`. Tidak memakai `resp.Truncated` supaya satu sumber kebenaran (hasilnya sama dengan `Truncated` pada respons yang konsisten).
3. `OffsetOutOfRange()`: `p.Total > 0 && p.Offset >= p.Total`.
4. `Text()` (urutan baris tetap):
   - Baris 1: `Folder: <prefix atau "/ (akar)"> | rekursif: <ya|tidak> | <Total> file`.
   - Tiap folder: `[folder] <path>/`.
   - Tiap file: `<path>  <HumanSize(SizeBytes)>  <FormatTime(UpdatedAt)>` (dua spasi pemisah; `HumanSize` dan `FormatTime` dari `base/format.go`, D9).
   - Bila `NextOffset() >= 0`: baris kosong, lalu `[ada <Total-next> file lagi, panggil list_files dengan offset=<next><sufiks>]`. Sufiks (agar argumen bisa disalin): `" path=<prefix>"` bila `Prefix != ""`, lalu `" recursive=true"` bila `Recursive`. Urutan: offset, path, recursive. Contoh: `offset=2 path=docs recursive=true`.
   - Kasus kosong (tidak ada file dan tidak ada folder yang ditampilkan, `Total == 0`): menggantikan SELURUH teks dengan satu pesan:
     - `Prefix == ""`: `Belum ada file knowledge di project ini.`
     - selain itu: `Tidak ada file di folder '<prefix>'. Panggil list_files tanpa path untuk melihat struktur dari akar, atau gunakan search_knowledge.`
     - `Result().IsError == false` untuk keduanya.
   - Catatan edge: akar yang hanya berisi subfolder (`Total == 0`, `Folders != nil`) BUKAN kosong; teks tetap header (`0 file`) + entri folder. Offset > 0 dengan `Total == 0` (folder disembunyikan) diperlakukan kosong.
5. `Text()` untuk `OffsetOutOfRange()`: `Offset <n> melewati jumlah file (<Total>). Gunakan offset 0-<Total-1>.`
6. `Structured()`: `{"items":[{"path","size_bytes","updated_at"(RFC3339 UTC)}], "folders": [...], "total", "next_offset", "recursive"}`.
   - `items` dan `folders` SELALU slice non-nil (`[]` bila kosong), supaya JSON memuat `[]`, bukan `null`.
   - `next_offset`: `int` atau `nil` (JSON `null`) bila `NextOffset() == -1`.
   - Nilai byte tetap integer (`size_bytes`), tidak berformat manusia.
   - Folder di `Structured` tanpa slash akhir (`"docs"`), berbeda dengan teks (`docs/`).
7. `Summary()`: `{"count": len(Items), "total": Total, "folders": len(Folders), "next_offset": NextOffset() (int; -1 bila habis), "recursive": Recursive}`. Hanya metadata, tanpa path/isi. (Overview 12.2 menampilkan `next_offset:2`; untuk habis dipakai `-1` agar tipe tetap integer di audit.)
8. `Result()`:
   - normal: `toolkit.Result{Text: Text(), IsError: false, Structured: Structured(), TargetType: "folder", Target: Prefix, Summary: Summary()}`.
   - `OffsetOutOfRange()`: `toolkit.Result{Text: <pesan 5>, IsError: true, Structured: nil, TargetType: "folder", Target: Prefix, Summary: {"error_code":"offset_out_of_range","count":0,"total":Total,"folders":0,"next_offset":-1,"recursive":Recursive}}`.
   - kosong: `IsError:false`, `Structured` tetap diisi (`items:[]`, `folders:[]`, `total:0`, `next_offset:null`) agar klien terstruktur konsisten.

**L2. `listfiles/params.go`**

```go
package listfiles

type rawArgs struct {
	Path      *string `json:"path"`
	Recursive *bool   `json:"recursive"`
	Limit     *int    `json:"limit"`
	Offset    *int    `json:"offset"`
}

// Params hasil validasi. Tidak memuat project.
type Params struct {
	Prefix    string // sudah base.NormalizePrefix
	Recursive bool
	Limit     int // 1-500
	Offset    int // 0-1000000
}

const (
	defaultLimit = 200
	maxLimit     = 500
	maxOffset    = 1_000_000
)

var allowedFields = []string{"path", "recursive", "limit", "offset"}

func schema() json.RawMessage // JSON Schema bagian "InputSchema" di bawah
func Parse(raw json.RawMessage) (Params, *base.ArgError)
```

Urutan `Parse` (error pertama menang, urutan field sesuai schema, D6):
1. `base.DecodeStrict(raw, &a, allowedFields)`: `null`/kosong menjadi `{}`; bukan object, token sisa, tipe salah (`limit:"x"` menjadi `Field:"limit"`, `Reason:"harus integer"`; `recursive:"true"` menjadi `recursive harus boolean`), dan field tak dikenal. `project_id`/`projectId`/`project` menghasilkan pesan khusus AC5: `Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang.`
2. `path`: `nil` menjadi `""`; selain itu `base.NormalizePrefix("path", *a.Path)` (trim, buang satu `/` depan dan belakang, tolak >1024 byte, UTF-8 tidak valid, karakter kontrol/NUL, segmen `..`).
3. `recursive`: `base.BoolArg(a.Recursive, false)`.
4. `limit`: `base.IntArg("limit", a.Limit, 200, 1, 500, "Gunakan offset untuk halaman berikutnya.")`. Contoh: `Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya.`
5. `offset`: `base.IntArg("offset", a.Offset, 0, 0, 1_000_000, "Gunakan nilai offset dari penanda [ada N file lagi ...].")`.

Konstanta `maxLimit` dijaga sinkron dengan `listfiles.MaxLimit` (T05) lewat satu tes (`TestMaxLimitMatchesContract`), tanpa mengimpor T05 di non-test code selain tipe kontrak yang sudah diizinkan.

`InputSchema` (disalin ke golden, overview 12.1; kunci diserialisasi alfabetis oleh golden):

```json
{
  "type": "object",
  "properties": {
    "path":      { "type": "string",  "maxLength": 1024, "description": "prefix folder, mis. docs/guides; kosong = akar" },
    "recursive": { "type": "boolean", "default": false,  "description": "true = semua file di bawah path; false = satu tingkat beserta entri [folder]" },
    "limit":     { "type": "integer", "minimum": 1, "maximum": 500, "default": 200, "description": "jumlah file per halaman" },
    "offset":    { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah file; pakai nilai dari penanda [ada N file lagi ...]" }
  },
  "additionalProperties": false
}
```
Tanpa `required`. Tidak ada properti project.

**L3. `listfiles/repository.go`**

```go
// Lister adalah port ke provider knowledge (T05). Satu-satunya tempat bus dipanggil.
type Lister interface {
	List(ctx context.Context, req listfiles.Request) (listfiles.Response, error)
}

type busLister struct{ b *bus.Registry } // tipe bus mengikuti app.Deps.Bus

func newRepository(b *bus.Registry) Lister { return busLister{b: b} }

func (r busLister) List(ctx context.Context, req listfiles.Request) (listfiles.Response, error) {
	return bus.CallTyped[listfiles.Response](ctx, r.b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, req)
}
```
Gunakan tipe `Bus` persis seperti field `app.Deps.Bus` dan signature `CallTyped` yang ada di `main` (cek dengan `go doc grimoire/internal/infra/bus` sebelum menulis). Tidak ada logika di repository. Error dikembalikan apa adanya (termasuk `bus.ErrNotRegistered` dan `errs.ErrInvalid`).

**L4. `listfiles/usecase.go`**

```go
type usecase struct{ lister Lister }

func newUsecase(l Lister) usecase { return usecase{lister: l} }

// Execute: orkestrasi saja.
func (u usecase) Execute(ctx context.Context, projectID string, p Params) (base.ListPage, error) {
	resp, err := u.lister.List(ctx, listfiles.Request{
		ProjectID: projectID,
		Prefix:    p.Prefix,
		Recursive: p.Recursive,
		Limit:     p.Limit,
		Offset:    p.Offset,
	})
	if err != nil {
		return base.ListPage{}, err // diteruskan; handler yang memetakan (D13)
	}
	return base.NewListPage(p.Prefix, p.Recursive, p.Offset, resp), nil
}
```
Tidak ada aturan rentang, format, atau penilaian "kosong" di sini.

**L5. `listfiles/handler.go`**

```go
const hintInvalid = "Periksa path; panggil list_files tanpa path untuk melihat akar."

func newHandler(u usecase) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments)
		if argErr != nil {
			return base.ArgResult(argErr, "folder", ""), nil
		}
		page, err := u.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolListFiles, err, hintInvalid, "folder", p.Prefix)
		}
		return page.Result(), nil
	}
}
```
- `BusResult` (foundation 5.5): `errors.Is(err, errs.ErrInvalid)` menjadi `Result{IsError:true, Text:"Permintaan ditolak: <err>. Periksa path; panggil list_files tanpa path untuk melihat akar.", Summary:{error_code:"invalid_request"}}` dengan error Go nil; error lain menjadi `fmt.Errorf("list_files: %w", err)` (gateway memetakan ke `internal`/`timeout`).
- Handler tidak memformat teks sendiri dan tidak menyentuh bus.

**L6. `listfiles/init.go`**

```go
const (
	title       = "Daftar file knowledge"
	description = "Daftar file knowledge project (read-only). Mulai dari akar (tanpa path) untuk memahami struktur sebelum membaca. Default hanya satu tingkat: subfolder tampil sebagai entri [folder]; panggil lagi dengan path=<folder> untuk masuk, atau recursive=true untuk semua file di bawah path. Lanjutkan daftar yang terpotong dengan offset. Baca isi dengan read_file, cari berdasarkan isi dengan search_knowledge."
)

func Init(d app.Deps) toolkit.Tool {
	h := newHandler(newUsecase(newRepository(d.Bus)))
	return toolkit.Tool{
		Name:        base.ToolListFiles,
		Title:       title,
		Description: description,
		InputSchema: schema(),
		Handler:     h,
	}
}
```
Teks `description` dibekukan; harus identik dengan golden (`testdata/tools_list.golden.json`). Tool tidak mendaftarkan endpoint bus dan tidak menulis audit.

**L7. Tes feature dan entity** (lihat Test Scenario). Urutan TDD per file: `base/listing_test.go` (L1), `params_test.go` (L2), `usecase_test.go` (L4), `handler_test.go` (L5).

**L8. Wiring ke module.** Ganti stub `listfiles.Init` pada `knowledgetools/base.go` (F4 overview) dengan implementasi ini. Tidak ada perubahan di `discovery/base.go` (itu F6 overview, milik foundation).

**L9. Tes integrasi AC1** pada `backend/cmd/internal/server/knowledgetools_integration_test.go` (file milik foundation F8; tambahkan dua tes `list_files` di sini).

**L10. Gerbang usecase.**
```
cd backend
go build ./... && go vet ./...
go test ./internal/domain/discovery/internal/knowledgetools/...
go test ./internal/archtest/...
go test -tags integration ./cmd/internal/server/... -run 'TestT08_Int_AC1'
```
Golden `tools_list.golden.json` harus memuat entri `list_files` yang persis sama dengan bagian ini (diverifikasi manual saat `-update` di F7 overview).

---

## Migration

Tidak ada. Tidak ada tabel, kolom, atau index baru; data dibaca lewat bus T05.

---

## Changes (per file/folder)

Resource root: `backend/internal/domain/discovery/internal/knowledgetools`.

| File | Status | Isi |
|---|---|---|
| `base/listing.go` | baru | entity `ListPage` + `NewListPage`, `NextOffset`, `OffsetOutOfRange`, `Text`, `Structured`, `Summary`, `Result` |
| `base/listing_test.go` | baru | tes entity (lihat Test Scenario) |
| `listfiles/init.go` | baru | `Init(d app.Deps) toolkit.Tool` (nama, title, description, schema, handler) |
| `listfiles/params.go` | baru | `rawArgs`, `Params`, `schema()`, `Parse` |
| `listfiles/handler.go` | baru | adapter `toolkit.Handler` |
| `listfiles/usecase.go` | baru | `Execute` (orkestrasi) |
| `listfiles/repository.go` | baru | interface `Lister` + implementasi `bus.CallTyped` |
| `listfiles/params_test.go` | baru | tabel validasi argumen |
| `listfiles/usecase_test.go` | baru | fake `Lister` |
| `listfiles/handler_test.go` | baru | `bus.NewRegistry()` + handler palsu |
| `base.go` | ubah (milik foundation F4) | `Register` memanggil `listfiles.Init(d)` (stub diganti) |
| `testdata/tools_list.golden.json` | diperbarui lewat `-update` (foundation F7) | entri `list_files` harus cocok dengan bagian ini |
| `backend/cmd/internal/server/knowledgetools_integration_test.go` | ubah (foundation F8) | `TestT08_Int_AC1_ListTopLevel`, `TestT08_Int_AC1_ListRecursivePaged` |

Dipakai tanpa diubah: `base/{names,args,path,format,result}.go` (foundation F3), `lib/constants/bus.go` (`BUS_PROJECT_KNOWLEDGE_LIST_FILES`), `lib/contracts/project/knowledge/listfiles`, `lib/contracts/discovery/mcpgateway/toolkit` (dengan `Result.Structured` dari F2).

Tidak berubah: `api/**`, `migrations/**`, `frontend/**`, `infra/config`, `lib/contracts/core/audit/record`, `go.mod`.

Kontrak bus yang dikonsumsi:

| Konstanta | Nama | Request | Response | Error |
|---|---|---|---|---|
| `BUS_PROJECT_KNOWLEDGE_LIST_FILES` | `project.knowledge.list-files` | `listfiles.Request{ProjectID: call.ProjectID, Prefix, Recursive, Limit (selalu eksplisit 1-500), Offset}` | `listfiles.Response{Items, Folders (nil bila Recursive), Total, Truncated}` | `errs.ErrInvalid` (prefix/ProjectID tidak valid) menjadi isError; `bus.ErrNotRegistered`, ctx cancel/deadline menjadi error Go |

Import yang diizinkan di package `listfiles` dan `base/listing.go` (AC7): stdlib, `grimoire/internal/infra/app`, `grimoire/internal/infra/bus`, `grimoire/lib/constants`, `grimoire/lib/contracts/project/knowledge/listfiles`, `grimoire/lib/contracts/discovery/mcpgateway/toolkit`, `grimoire/lib/common/errs`, dan package module ini. Dilarang `internal/domain/project/**` dan `internal/domain/discovery/internal/mcpgateway/**`.

---

## Test Scenario

Semua nama tes yang memetakan AC memakai awalan `TestT08_ACn_`.

### `base/listing_test.go` (tabel, tanpa I/O; fixture waktu UTC tetap, mis. `2026-10-20T08:15:00Z`; gunakan juga satu waktu `Asia/Jakarta` untuk bukti render UTC)

| Tes | Skenario | Ekspektasi |
|---|---|---|
| `TestT08_AC1_TopLevel` | `!Recursive`, offset 0, Folders `["docs"]`, Items `[readme.md]`, Total 1 | Teks persis: baris 1 `Folder: / (akar) \| rekursif: tidak \| 1 file`, lalu `[folder] docs/`, lalu `readme.md  100 B  2026-10-20 08:15 UTC`; tidak ada penanda; `NextOffset() == -1` |
| `TestListPage_Recursive_Truncated` | `Recursive`, Total 5, 2 item, offset 0, prefix kosong | Penanda `[ada 3 file lagi, panggil list_files dengan offset=2 recursive=true]` didahului satu baris kosong; `Structured()["next_offset"] == 2` |
| `TestListPage_Truncated_WithPrefix` | prefix `docs`, `Recursive`, Total 5, offset 2, 2 item | penanda `offset=4 path=docs recursive=true`; sisa `1 file lagi` |
| `TestListPage_LastPage` | offset 3, 2 item, Total 5 | tanpa penanda; `NextOffset() == -1`; `Structured()["next_offset"] == nil`; JSON memuat `"next_offset":null` |
| `TestListPage_FoldersNotRepeatedOnLaterPages` | `!Recursive`, offset 2, resp.Folders berisi `docs` | `p.Folders == nil`; teks tanpa `[folder]`; `Structured()["folders"]` adalah `[]string{}` (JSON `[]`) |
| `TestListPage_FoldersHiddenWhenRecursive` | `Recursive`, resp.Folders `nil` | `folders` di Structured `[]`; tanpa `[folder]` |
| `TestListPage_FolderSlashNormalized` | resp.Folders `["docs/", "docs/guides"]` | teks `[folder] docs/` dan `[folder] docs/guides/`; Structured `["docs","docs/guides"]` |
| `TestListPage_EmptyRoot` | Total 0, tanpa folder, prefix kosong | teks persis `Belum ada file knowledge di project ini.`; `IsError == false`; Structured `items:[]` |
| `TestListPage_EmptyPrefix` | Total 0, prefix `docs/x` | teks persis `Tidak ada file di folder 'docs/x'. Panggil list_files tanpa path untuk melihat struktur dari akar, atau gunakan search_knowledge.`; `IsError == false` |
| `TestListPage_RootWithOnlyFolders` | Total 0, Folders `["docs"]`, offset 0 | BUKAN pesan kosong: header `... \| 0 file` + `[folder] docs/` |
| `TestListPage_OffsetOutOfRange` | Total 3, offset 3 (dan offset 10) | teks `Offset 3 melewati jumlah file (3). Gunakan offset 0-2.`; `IsError == true`; `Summary["error_code"] == "offset_out_of_range"`; `Structured == nil` |
| `TestListPage_OffsetWithZeroTotalIsEmptyNotError` | Total 0, offset 5 | pesan kosong, `IsError == false` (bukan `offset_out_of_range`) |
| `TestListPage_FormatDeterministic` | item 1536 B pada waktu `Asia/Jakarta` 15:15 | baris `... 1.5 KB  2026-10-20 08:15 UTC`; `updated_at` Structured `2026-10-20T08:15:00Z` |
| `TestListPage_TextSeparatorsTwoSpaces` | satu file | pemisah tepat dua spasi, tanpa tab |
| `TestListPage_Structured_Shape` | halaman penuh | kunci tepat `items, folders, total, next_offset, recursive`; tiap item tepat `path, size_bytes, updated_at`; `size_bytes` bertipe int |
| `TestListPage_Summary_MetadataOnly` | halaman penuh | kunci tepat `count, total, folders, next_offset, recursive`; tidak ada string path/isi; `Result().TargetType == "folder"`, `Target == prefix` |
| `TestListPage_ItemOrderPreserved` | resp tidak terurut alfabet | urutan teks/Structured sama dengan respons (entity tidak mengurutkan ulang) |

### `listfiles/params_test.go` (tabel)

| Input `arguments` | Ekspektasi |
|---|---|
| `{}` | `Params{"", false, 200, 0}`, tanpa error |
| `null` (RawMessage `null`) dan RawMessage kosong | dinormalisasi ke `{}` (hasil sama dengan baris pertama) |
| `{"path":"  /docs/  "}` | `Prefix == "docs"` |
| `{"path":"/"}` | `Prefix == ""` |
| `{"limit":0}`, `{"limit":501}` | ArgError `Field=="limit"`, pesan memuat `harus 1-500`; untuk 501: `(diterima: 501)` |
| `{"limit":900}` | pesan persis `Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya.` |
| `{"limit":"x"}`, `{"limit":1.5}` | `Field=="limit"`, `Reason=="harus integer"` |
| `{"limit":1}`, `{"limit":500}` | diterima (batas) |
| `{"offset":-1}` | ArgError `Field=="offset"` |
| `{"offset":1000001}` | ArgError `Field=="offset"`; `{"offset":1000000}` diterima |
| `{"recursive":"true"}` | ArgError `Field=="recursive"`, `harus boolean` |
| `{"recursive":true}` | `Recursive == true` |
| `{"project_id":"x"}`, `{"projectId":"x"}`, `{"project":"x"}` | pesan khusus memuat `ditentukan oleh API key` (AC5) |
| `{"foo":1}` | pesan memuat `foo tidak dikenal` dan `path, recursive, limit, offset` |
| path 1025 byte | ArgError `Field=="path"`, `maksimal 1024 byte`; 1024 byte diterima |
| `{"path":"../a"}`, `{"path":"a/../b"}` | ArgError `Field=="path"`, memuat `..` |
| `{"path":"a..b"}` | diterima |
| `{"path":"a\u0000b"}` | ArgError `memuat karakter kontrol` |
| `[]`, `1`, `"x"`, `{` | ArgError `Field=="arguments"` |
| `{"limit":900,"offset":-1}` | hanya error pertama (limit) yang dilaporkan |
| `TestMaxLimitMatchesContract` | `maxLimit == listfiles.MaxLimit` (T05) |
| `TestSchema_Valid` | `schema()` JSON valid; `additionalProperties:false`; tidak ada `required`; tidak ada properti project; default 200/0/false |

### `listfiles/usecase_test.go` (fake `Lister` yang merekam request)

- `TestExecute_ForwardsRequest`: `Execute(ctx, "proj-1", Params{Prefix:"docs", Recursive:true, Limit:2, Offset:4})` mengirim `Request{ProjectID:"proj-1", Prefix:"docs", Recursive:true, Limit:2, Offset:4}` persis; `Limit` eksplisit (bukan 0).
- `TestExecute_DefaultParamsSendExplicitLimit200`: `Params{Limit:200}` menghasilkan `Request.Limit == 200`.
- `TestExecute_BuildsListPage`: fake mengembalikan `Items/Folders/Total`; hasil `ListPage` memiliki `Prefix/Recursive/Offset` dari Params dan `Total` dari respons; folder disembunyikan bila `Offset > 0`.
- `TestExecute_PropagatesError`: fake mengembalikan `errs.ErrInvalid` terbungkus; `errors.Is` tetap berlaku dan `ListPage` nol. Sama untuk error biasa dan `context.DeadlineExceeded`.

### `listfiles/handler_test.go` (`bus.NewRegistry()` + handler palsu terdaftar pada `constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES`, merekam jumlah panggilan dan `Request`)

- `TestT08_AC5_ProjectFromCallOnly`:
  - `arguments={"project_id":"B"}` menghasilkan `IsError == true`, teks memuat `ditentukan oleh API key`, `Summary["error_code"]=="invalid_argument"`, jumlah panggilan bus 0.
  - tanpa `project_id`, dengan `call.ProjectID="A"`: `Request.ProjectID == "A"` tepat satu panggilan.
- `TestHandler_ArgErrorSkipsBus`: `{"limit":900}` menghasilkan `IsError`, bus tidak dipanggil, `Summary["field"]=="limit"`.
- `TestHandler_Success`: handler palsu mengembalikan 2 item + 1 folder; `Result.Text` memuat file dan `[folder]`, `Structured["total"]`, `TargetType=="folder"`, `Target==prefix`.
- `TestHandler_ErrInvalidBecomesIsError`: handler palsu mengembalikan `fmt.Errorf("x: %w", errs.ErrInvalid)`; hasil `IsError == true`, error Go nil, teks memuat `Permintaan ditolak:` dan `Periksa path; panggil list_files tanpa path untuk melihat akar.`, `Summary["error_code"]=="invalid_request"`.
- `TestHandler_OtherErrorBecomesGoError`: handler palsu mengembalikan `errors.New("db down")`; handler mengembalikan error Go yang membungkusnya (`errors.Is`), bukan Result error.
- `TestHandler_BusNotRegisteredIsGoError`: registry kosong menghasilkan error Go; `errors.Is(err, bus.ErrNotRegistered)`.
- `TestHandler_ContextDeadlinePreserved`: handler palsu mengembalikan `context.DeadlineExceeded`; `errors.Is(err, context.DeadlineExceeded)` tetap benar (gateway butuh ini untuk kode `timeout`).
- `TestHandler_ResultMetadataOnly`: dengan item berisi path unik `SECRET-PATH-XYZ`, `Summary` tidak memuat string itu dan seluruh nilainya hanya number/bool; `Structured` hanya berisi kunci metadata/daftar yang dibekukan.
- `TestHandler_OffsetOutOfRange`: handler palsu `Total=3`, argumen `offset=5` menghasilkan `IsError`, `error_code=offset_out_of_range`, bus dipanggil satu kali (validasi rentang offset terhadap Total hanya mungkin setelah T05 menjawab).

### Tes level module (milik foundation, tetapi wajib mencakup tool ini)
- `TestT08_AC6_ToolsListContract`: entri `list_files` pada golden cocok byte-per-byte.
- `TestT08_AC6_SchemasAreValidJSONSchema`: schema `list_files` lolos validator meta (tipe `string/boolean/integer`, `minimum<=default<=maximum`, tanpa properti project; `mcp.list_files` ada di `record.Actions()`).
- `TestT08_AC7_ImportAllowlist`: tidak ada impor terlarang di `listfiles/` dan `base/listing*.go`.

### Integrasi (`//go:build integration`, `backend/cmd/internal/server`, Postgres tes + bus nyata + API key nyata)
Fixture project A: `readme.md`, `docs/a.md`, `docs/guides/b.md` (lima file lengkap di fixture overview boleh dipakai; asersi di bawah memakai urutan T05).

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT08_Int_AC1_ListTopLevel` | 1 | `list_files {}` | `isError:false`; teks memuat `readme.md` dan `[folder] docs/`, tidak memuat `docs/a.md` maupun `docs/guides/b.md`; `structuredContent.folders == ["docs"]`; `next_offset == null`; tidak ada penanda |
| `TestT08_Int_AC1_ListRecursivePaged` | 1 | `{recursive:true, limit:2}` lalu `{recursive:true, limit:2, offset:2}` (lanjut hingga habis) | halaman 1: 2 file dan penanda `offset=2`; `structuredContent.next_offset==2`; halaman berikutnya berisi sisanya tanpa `[folder]`; gabungan path sama dengan semua file project tanpa duplikat/hilang; halaman terakhir `next_offset == null` |
| `TestT08_Int_ListFiles_PrefixAndEmpty` | 1 | `{path:"docs"}`; `{path:"tidak-ada"}` | `docs`: file di bawah docs + `[folder] docs/guides/`; `tidak-ada`: pesan `Tidak ada file di folder 'tidak-ada'...`, `isError:false` |
| `TestT08_Int_ListFiles_OffsetPastEnd` | 1 | `{offset:999}` | `isError:true`, teks `Offset 999 melewati jumlah file (...)` |
| `TestT08_Int_AC5_ListFilesProjectArgRejected` | 5 | key A, `list_files {project_id:<B>}` | `isError:true`, teks memuat `ditentukan oleh API key`, tidak memuat `secret/b.md` |
| `TestT08_Int_ListFiles_Audit` | D8 | satu `list_files` sukses | tepat satu entri audit `mcp.list_files`, `target_type=folder`, Result berisi `count,total,folders,next_offset,recursive` tanpa path file |

---

## E2E Scenarios

Positif:
1. Agent memanggil `list_files {}` pada project berstruktur, melihat `readme.md` dan `[folder] docs/`, lalu `list_files {path:"docs"}` untuk masuk satu tingkat.
2. Agent memanggil `list_files {recursive:true}` pada project kecil (total <= 200) dan mendapat semua file tanpa penanda.
3. Agent memanggil `list_files {recursive:true, limit:2}`, melihat penanda `offset=2 recursive=true`, lalu memanggil ulang dengan argumen itu dan mendapat sisanya; folder tidak diulang di halaman lanjutan.
4. `list_files {path:" /docs/ "}` (spasi dan slash) berperilaku sama dengan `{path:"docs"}`.
5. Project kosong: `list_files {}` menghasilkan `Belum ada file knowledge di project ini.` dengan `isError:false`.
6. Folder tidak ada: `list_files {path:"nope"}` menghasilkan pesan saran (list tanpa path / search_knowledge), `isError:false`.
7. Akar hanya berisi subfolder: header `0 file` dan entri `[folder]`, bukan pesan kosong.
8. Halaman terakhir: `structuredContent.next_offset` bernilai `null` dan teks tanpa penanda.

Negatif:
1. `list_files {limit:900}` menghasilkan `isError`: `Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya.` (tidak di-clamp, bus tidak dipanggil).
2. `list_files {limit:0}` dan `{limit:"x"}` menghasilkan `isError` pada field `limit`.
3. `list_files {offset:-1}` menghasilkan `isError` pada field `offset`.
4. `list_files {recursive:"true"}` menghasilkan `isError`: `recursive harus boolean`.
5. `list_files {project_id:"<B>"}` (juga `projectId`, `project`) menghasilkan `isError` pesan khusus; project tidak berubah dan bus tidak dipanggil.
6. `list_files {foo:1}` menghasilkan `isError` yang menyebut argumen yang didukung.
7. `list_files {path:"../etc"}` menghasilkan `isError`: `path tidak boleh memuat segmen ..`.
8. `list_files {path:<1025 byte>}` menghasilkan `isError`: `path maksimal 1024 byte`.
9. `list_files {offset:50}` pada project dengan 3 file menghasilkan `isError` `offset_out_of_range` dengan petunjuk `offset 0-2`.
10. T05 menolak prefix (`errs.ErrInvalid`) menghasilkan `isError` `Permintaan ditolak: ... Periksa path; panggil list_files tanpa path untuk melihat akar.` dengan `error_code=invalid_request`.
11. Provider T05 tidak terdaftar atau timeout menghasilkan error Go; gateway mengembalikan `Terjadi kesalahan internal saat menjalankan tool list_files. trace_id=<id>` atau teks timeout, dan audit `error_code=internal|timeout`.
12. `arguments: null` dikirim langsung ke handler diperlakukan sebagai `{}` (default); di endpoint T07 `null` sudah ditolak lebih awal.

---

## Request/Response body

### `tools/list` (entri `list_files`; gateway menambah `annotations`)

```json
{
  "name": "list_files",
  "title": "Daftar file knowledge",
  "description": "Daftar file knowledge project (read-only). Mulai dari akar (tanpa path) untuk memahami struktur sebelum membaca. Default hanya satu tingkat: subfolder tampil sebagai entri [folder]; panggil lagi dengan path=<folder> untuk masuk, atau recursive=true untuk semua file di bawah path. Lanjutkan daftar yang terpotong dengan offset. Baca isi dengan read_file, cari berdasarkan isi dengan search_knowledge.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "path": { "type": "string", "maxLength": 1024, "description": "prefix folder, mis. docs/guides; kosong = akar" },
      "recursive": { "type": "boolean", "default": false, "description": "true = semua file di bawah path; false = satu tingkat beserta entri [folder]" },
      "limit": { "type": "integer", "minimum": 1, "maximum": 500, "default": 200, "description": "jumlah file per halaman" },
      "offset": { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah file; pakai nilai dari penanda [ada N file lagi ...]" }
    },
    "additionalProperties": false
  },
  "annotations": { "readOnlyHint": true }
}
```

### Level satu (tanpa argumen)

Request:
```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files","arguments":{}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{"type":"text","text":"Folder: / (akar) | rekursif: tidak | 1 file\n[folder] docs/\nreadme.md  100 B  2026-10-20 08:10 UTC"}],
    "isError": false,
    "structuredContent": {
      "items": [{"path":"readme.md","size_bytes":100,"updated_at":"2026-10-20T08:10:00Z"}],
      "folders": ["docs"],
      "total": 1,
      "next_offset": null,
      "recursive": false
    }
  }
}
```
Audit (gateway): `action=mcp.list_files`, `target_type=folder`, `target=""`, result `{count:1,total:1,folders:1,next_offset:-1,recursive:false}` ditambah field gateway.

### Rekursif, terpotong

Request:
```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_files","arguments":{"recursive":true,"limit":2}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [{"type":"text","text":"Folder: / (akar) | rekursif: ya | 5 file\ndocs/a.md  1.2 KB  2026-10-20 08:15 UTC\ndocs/big.txt  200.0 KB  2026-10-20 08:16 UTC\n\n[ada 3 file lagi, panggil list_files dengan offset=2 recursive=true]"}],
    "isError": false,
    "structuredContent": {
      "items": [
        {"path":"docs/a.md","size_bytes":1229,"updated_at":"2026-10-20T08:15:00Z"},
        {"path":"docs/big.txt","size_bytes":204800,"updated_at":"2026-10-20T08:16:00Z"}
      ],
      "folders": [],
      "total": 5,
      "next_offset": 2,
      "recursive": true
    }
  }
}
```

### Halaman lanjutan dengan prefix

Request:
```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_files","arguments":{"path":"docs","recursive":true,"limit":2,"offset":2}}}
```
Teks hasil (halaman terakhir, tanpa penanda):
```
Folder: docs | rekursif: ya | 3 file
docs/guides/b.md  23 B  2026-10-20 08:17 UTC
```
dengan `"next_offset": null`.

### Folder kosong / project kosong (`isError:false`)

```json
{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text","text":"Belum ada file knowledge di project ini."}],"isError":false,"structuredContent":{"items":[],"folders":[],"total":0,"next_offset":null,"recursive":false}}}
```
```json
{"jsonrpc":"2.0","id":5,"result":{"content":[{"type":"text","text":"Tidak ada file di folder 'nope'. Panggil list_files tanpa path untuk melihat struktur dari akar, atau gunakan search_knowledge."}],"isError":false,"structuredContent":{"items":[],"folders":[],"total":0,"next_offset":null,"recursive":false}}}
```

### Format error

Argumen tidak valid (`invalid_argument`, bus tidak dipanggil):
```json
{"jsonrpc":"2.0","id":6,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya."}],"isError":true}}
```
Summary audit: `{"error_code":"invalid_argument","field":"limit"}`.

Argumen project (AC5):
```json
{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```

Argumen tidak dikenal:
```json
{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"Argumen tidak valid: foo tidak dikenal. Argumen yang didukung: path, recursive, limit, offset."}],"isError":true}}
```

Tipe salah:
```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"Argumen tidak valid: recursive harus boolean (diterima: string)."}],"isError":true}}
```

Offset di luar rentang (`offset_out_of_range`, tanpa `structuredContent`):
```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"Offset 5 melewati jumlah file (3). Gunakan offset 0-2."}],"isError":true}}
```
Summary audit: `{"error_code":"offset_out_of_range","count":0,"total":3,"folders":0,"next_offset":-1,"recursive":false}`.

Ditolak provider (`invalid_request`, `errs.ErrInvalid` dari T05):
```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"Permintaan ditolak: prefix tidak valid: invalid argument. Periksa path; panggil list_files tanpa path untuk melihat akar."}],"isError":true}}
```
Summary audit: `{"error_code":"invalid_request"}`. (Teks sebelum saran mengikuti `err.Error()` aktual dari T05.)

Error internal (error Go dari tool, dipetakan gateway; detail disembunyikan):
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool list_files. trace_id=7f3c2a9e1b4d"}],"isError":true}}
```

Timeout (ctx deadline, dipetakan gateway):
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Tool list_files melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```

---

## Catatan bagi developer (asumsi dan keputusan kecil)

- Signature `NewListPage` mengikuti overview 5.6 (`prefix, recursive, offset, resp`), bukan `NewListPage(params, resp)`, karena `base/` tidak boleh mengimpor `listfiles` (siklus). Usecase meneruskan field dari `Params`.
- Gaya slash folder dari T05 (dengan atau tanpa `/` akhir, relatif akar) tidak dijamin kontraknya; entity menormalisasi (trim `/` lalu menambah `/` hanya di teks). Saat L1, baca tes provider T05 di `internal/domain/project/internal/knowledge` untuk memastikan `Folders` berisi path penuh dari akar (bukan nama segmen). Bila hanya nama segmen, usecase TIDAK boleh diubah; tambahkan penggabungan prefix di `NewListPage` dan tes.
- Sufiks penanda memakai urutan `offset`, `path`, `recursive` (cocok dengan contoh overview 12.2 `offset=2 recursive=true`). `limit` tidak disertakan karena default sudah wajar; bila agent memakai limit kustom, ia mengulangnya sendiri.
- `Summary.next_offset` memakai `-1` saat habis (tipe integer stabil untuk audit), sedangkan `Structured.next_offset` memakai `null`.
- Validasi `offset` terhadap `Total` hanya dilakukan di entity setelah jawaban T05 (satu kali panggilan bus); bukan di `Parse`.
