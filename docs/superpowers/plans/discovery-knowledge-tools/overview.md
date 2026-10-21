# T08 discovery/knowledge-tools — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T08-discovery-knowledge-tools.md`
Sumber aturan: `rules.txt` (bagian 4, 7.4, 9; menang untuk arsitektur), `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog aksi audit), `docs/superpowers/plans/discovery-mcp-gateway/{overview,usecase-callaudit,usecase-endpoint}.md` (T07: kontrak `toolkit`, `Outcome`, format hasil `tools/call`), kontrak T05 `backend/lib/contracts/project/knowledge/{listfiles,readfile,search}`.
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/migrations.md` tidak relevan karena T08 tidak punya migrasi. Tidak ada knowledge doc module. Tidak ada UI, OpenAPI, tabel, maupun aksi audit baru.

Dokumen ini adalah plan FOUNDATION. Isinya keputusan bersama, `base/`, kontrak, perubahan di luar module, urutan kerja, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/discovery-knowledge-tools/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila keduanya bertentangan, dokumen ini yang berlaku sampai diperbarui.

---

## 0. Prasyarat (gerbang F1, hentikan implementasi bila belum terpenuhi)

| Prasyarat | Status saat plan ditulis (2026-10-21) | Cara cek |
|---|---|---|
| T07 ter-merge ke `main` | BELUM. Baru ada di branch `feat/discovery-mcp-gateway` (commit `e4df0e3`, wip foundation; `callaudit/` dan `endpoint/` baru berisi `init.go`) | `git log main --oneline \| grep discovery-mcp-gateway` |
| `lib/contracts/discovery/mcpgateway/toolkit` (`Tool`, `Handler`, `Call`, `Result`) | ada di branch T07 | `go doc grimoire/lib/contracts/discovery/mcpgateway/toolkit` |
| `mcpgateway.Register(d, tools)` dipanggil dari `internal/domain/discovery/base.go` | ada di branch T07, dengan komentar `// T08: tools = append(tools, knowledgetools.Register(d)...)` | baca file |
| `base.Outcome` menyimpan `Result toolkit.Result` utuh | ada di branch T07 (`mcpgateway/base/invoker.go`) | baca file |
| Aksi audit `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge` di `record.Actions()` | SUDAH di `main` (`lib/contracts/core/audit/record/actions.go`) | `grep ActionMCPListFiles` |
| `Config.MCPMaxResponseBytes` (default 102400, minimal 1024) | SUDAH di `main` (`infra/config/config.go`) | `grep MCPMaxResponseBytes` |
| Kontrak T05 + konstanta `BUS_PROJECT_KNOWLEDGE_{LIST_FILES,READ_FILE,SEARCH}` | SUDAH di `main` | `grep KNOWLEDGE lib/constants/bus.go` |

Bila T07 belum merge, developer berhenti di F1 dan melapor. Jangan menyalin kode T07 ke branch T08.

Simbol yang dipakai:

| Simbol | Import path |
|---|---|
| `Deps{Config, Bus, ...}` | `grimoire/internal/infra/app` |
| `CallTyped[T]`, `NewRegistry`, `ErrNotRegistered` | `grimoire/internal/infra/bus` |
| `BUS_PROJECT_KNOWLEDGE_LIST_FILES`, `..._READ_FILE`, `..._SEARCH` | `grimoire/lib/constants` |
| `listfiles.{Request,Response,Item,MaxLimit}` | `grimoire/lib/contracts/project/knowledge/listfiles` |
| `readfile.{Request,Response}` | `grimoire/lib/contracts/project/knowledge/readfile` |
| `search.{Request,Response,Item,MaxLimit}` | `grimoire/lib/contracts/project/knowledge/search` |
| `toolkit.{Tool,Handler,Call,Result}` | `grimoire/lib/contracts/discovery/mcpgateway/toolkit` |
| `errs.ErrInvalid` | `grimoire/lib/common/errs` |

Tidak ada library baru. `go.mod` tidak berubah.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T08 (ukuran S) |
| Scope | fullstack menurut tiket, tetapi bagian UI tiket menyatakan "Tidak ada". Hanya backend; tidak ada frontend plan |
| Domain | discovery |
| Module | knowledge-tools (package `knowledgetools`) |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| Bergantung pada | T05 (merged), T07 (BELUM merge, lihat bagian 0) |
| Kontrak REST | Tidak ada. endpointsChanged = false |
| Kontrak bus disediakan | Tidak ada |
| Kontrak bus dikonsumsi | `project.knowledge.list-files`, `project.knowledge.read-file`, `project.knowledge.search` (semua Call sinkron) |
| Migrasi | Tidak ada |

Endpoint (milik T07, T08 hanya menambah tool di katalognya):

| Endpoint | Method | Version | Metode JSON-RPC | Tool T08 |
|---|---|---|---|---|
| `/mcp` | POST | MCP 2025-11-25 / 2025-06-18 / 2025-03-26, JSON-RPC 2.0 | `tools/list` | 3 entri baru: `list_files`, `read_file`, `search_knowledge` |
| `/mcp` | POST | sama | `tools/call` | `params.name` salah satu dari tiga tool di atas |

---

## 2. Summary Plan

T08 menambah tiga tool MCP read-only di atas gateway T07. Masing-masing memvalidasi argumen secara ketat, memanggil satu endpoint bus T05 dengan `ProjectID` dari `toolkit.Call` (bukan dari argumen), lalu merender keluaran yang ringkas untuk LLM: teks, `structuredContent`, dan metadata audit.

Module ini tipis. Hampir semua rule (normalisasi path, rentang angka, format teks, penanda lanjutan, pesan saran) tinggal di `base/` sebagai fungsi murni dan dites tanpa I/O. Tiap feature (`listfiles`, `readfile`, `searchknowledge`) punya lapisan `init -> handler -> usecase -> repository`, dan repository-nya hanya memanggil `bus.CallTyped`. Tidak ada SQL, state, cache, goroutine, atau job.

Ada satu perubahan aditif pada kontrak T07 (D1). `toolkit.Result.Structured` ditambahkan dan diteruskan endpoint sebagai `structuredContent`, karena tiket mewajibkan `structuredContent` sedangkan toolkit T07 belum punya field untuk itu.

---

## 3. Keputusan bersama (mengikat ketiga usecase)

| ID | Keputusan | Alasan |
|---|---|---|
| D1 | Perubahan aditif pada T07: (a) `toolkit.Result` ditambah `Structured map[string]any`; (b) `endpoint.callToolResult` ditambah `StructuredContent map[string]any \`json:"structuredContent,omitempty"\`` yang diisi dari `Outcome.Result.Structured`; (c) `callaudit` meneruskan `Result` apa adanya, cukup tambah satu asersi tes. `Structured` tidak masuk audit dan tidak dipotong gateway. Tidak ada `outputSchema` di `tools/list` | Tiket mewajibkan `structuredContent`. Perubahan aditif, signature lama tidak berubah |
| D2 | `offset`/`limit` `read_file` memakai satuan BYTE, bukan karakter seperti tertulis di tiket | Kontrak T05 berbasis byte dan memotong di batas UTF-8, sehingga `offset=NextOffset` menyambung tanpa duplikat atau celah (AC2). Konversi karakter ke byte butuh membaca file utuh. Dicatat di bagian Keputusan tiket T08 |
| D3 | Argumen ketat: schema `"additionalProperties": false`, `base.DecodeStrict` menolak field tak dikenal dan token sisa. `project_id`/`projectId`/`project` mendapat pesan khusus. Project tidak pernah berubah | AC5, rules 7.4 |
| D4 | Path, prefix, dan query di-`TrimSpace`. Path/prefix: satu `/` di depan dibuang (prefix juga satu `/` di belakang), maksimal 1024 byte, ditolak bila memuat NUL/karakter kontrol atau segmen `..`. Query 1-200 rune setelah trim. Validasi karakter path lainnya diserahkan ke T05 (dipetakan lewat `ErrInvalid`) | Satu sumber kebenaran validasi path tetap di T05 |
| D5 | Nilai di luar rentang DITOLAK (`isError`), tidak di-clamp. `list_files`: limit 1-500 (default 200), offset 0-1000000 (default 0). `read_file`: offset 0-2147483647, limit 1 sampai `MaxReadLimit` (default `min(40000, MaxReadLimit)`), dengan `MaxReadLimit = max(1024, MCPMaxResponseBytes-512)`. `search_knowledge`: limit 1-20 (default 10). Tipe salah menjadi `ArgError{Field, "harus <tipe>"}` | Konsisten dengan `minimum`/`maximum` di schema. Cadangan 512 byte memastikan gateway tidak pernah memotong isi file |
| D6 | Format pesan `ArgError`: `Argumen tidak valid: <field> <alasan> (diterima: <nilai ringkas>). <saran>`. Bagian `(diterima: ...)` dihilangkan bila argumen tidak ada. Hanya error pertama yang dilaporkan, dengan urutan field sesuai schema. Summary `{error_code:"invalid_argument", field}` | LLM tahu persis apa yang harus diperbaiki (rules 9.5) |
| D7 | Bahasa teks hasil dan deskripsi tool: Indonesia (konsisten K25 T07). Nama tool dan argumen tetap apa adanya. Deskripsi menyebut langkah berikutnya | rules 9.5 |
| D8 | Audit hanya dilakukan gateway. Tool mengisi `TargetType`/`Target`/`Summary` dengan metadata saja (lihat 5.6) | rules 9.3 |
| D9 | `FormatTime(t) = t.UTC().Format("2006-01-02 15:04 UTC")`. Di `Structured` waktu memakai RFC3339 UTC. `HumanSize`: `<1024` menjadi `"N B"`, lalu `"1.5 KB"`, `"2.3 MB"`, `"1.0 GB"` (satu desimal, basis 1024). `Structured` selalu memakai byte integer | Deterministik untuk golden/tes |
| D10 | Penanda di akhir teks dipisah satu baris kosong dan memakai kurung siku. `read_file`: `[terpotong: lanjutkan dengan offset=<NextOffset> dari total <TotalBytes> byte]`. `list_files`: `[ada <sisa> file lagi, panggil list_files dengan offset=<next>]` | Mudah dibedakan dari isi |
| D11 | Tidak ada state, cache, goroutine, atau job. Semua tool idempoten | Tipis sesuai tiket |
| D12 | Folder `register` dari tiket TIDAK dibuat sebagai feature. Isinya hanya wiring tanpa handler/usecase, jadi ditaruh di `knowledgetools/base.go` ditambah satu baris di `discovery/base.go` | Sama dengan K1 T07, tidak memecah usecase secara artifisial |
| D13 | Pemetaan error bus (sama untuk ketiganya): `errors.Is(err, errs.ErrInvalid)` menjadi `Result{IsError:true, Text:"Permintaan ditolak: <err.Error()>. <saran per tool>", Summary:{error_code:"invalid_request"}}` dengan err nil. Error lain (termasuk `bus.ErrNotRegistered` dan ctx deadline/cancel) dikembalikan sebagai error Go `fmt.Errorf("<tool>: %w", err)`, lalu gateway memetakannya ke `internal`/`timeout` dan menyembunyikan detailnya | Pesan T05 sudah memandu. Error infrastruktur tidak boleh bocor |

---

## 4. Struktur module

```
backend/internal/domain/discovery/internal/knowledgetools/
  base.go                      Register(d app.Deps) []toolkit.Tool
  base_test.go                 AC6 (kontrak tools/list + validator schema), AC7 (allowlist import), jumlah tool
  testdata/tools_list.golden.json
  base/                        entity dan rule murni, tanpa I/O
    names.go       names_test.go
    args.go        args_test.go
    path.go        path_test.go
    format.go      format_test.go
    result.go      result_test.go
    listing.go     listing_test.go        (dibuat di usecase listfiles)
    reading.go     reading_test.go        (dibuat di usecase readfile)
    searching.go   searching_test.go      (dibuat di usecase searchknowledge)
  listfiles/        init.go params.go handler.go usecase.go repository.go params_test.go handler_test.go usecase_test.go
  readfile/         (sama)
  searchknowledge/  (sama)
```

Lapisan di dalam feature (arah dependensi `handler -> usecase -> repository`, semua memakai `base`):

| File | Isi | Dilarang |
|---|---|---|
| `init.go` | `func Init(d app.Deps) toolkit.Tool`. Merakit `repository(d.Bus)`, lalu `usecase`, lalu `handler`. Mengisi `Name` (dari `base`), `Title`, `Description`, dan `InputSchema` (dari `params.go`). Membaca config sekali di sini | logika bisnis |
| `params.go` | struct argumen mentah dengan field pointer (`*string`, `*int`, `*bool`), `func schema(...) json.RawMessage`, dan `func Parse(raw json.RawMessage) (Params, *base.ArgError)` (readfile: `Parse(raw, maxLimit int)`) | I/O |
| `handler.go` | adapter `toolkit.Handler`: Parse, lalu Execute, lalu pemetaan ke `toolkit.Result` lewat helper `base` | format teks sendiri, SQL, bus |
| `usecase.go` | `Execute(ctx, projectID string, p Params) (<entity>, error)`. Hanya orkestrasi: panggil port repository, lalu bangun entity `base` | format teks, aturan rentang |
| `repository.go` | interface kecil (`Lister`/`Reader`/`Searcher`) beserta implementasi `bus.CallTyped[contract.Response](ctx, b, constants.BUS_..., contract.Request{...})` | SQL, logika |

`ProjectID` SELALU berasal dari `call.ProjectID`. Handler meneruskannya ke `Execute`, lalu repository memasukkannya ke `Request.ProjectID`.

---

## 5. Base bersama (`base/`) — dibuat di foundation (F3)

Import `base/` hanya boleh: stdlib, `toolkit`, `lib/common/errs` (hanya di `result.go`), dan tipe kontrak T05 (dipakai oleh `listing/reading/searching.go`, yang dibuat di usecase).

### 5.1 `names.go`

```go
const (
	ToolListFiles       = "list_files"
	ToolReadFile        = "read_file"
	ToolSearchKnowledge = "search_knowledge"
)

// ProjectArgAliases adalah nama argumen project yang ditolak dengan pesan khusus (AC5).
var ProjectArgAliases = []string{"project_id", "projectId", "project"}
```

Tes: ketiga nama cocok `^[a-z][a-z0-9_]{0,63}$` dan unik.

### 5.2 `args.go`

```go
// ArgError adalah kegagalan validasi argumen yang dikembalikan ke LLM sebagai isError.
type ArgError struct {
	Field  string // nama argumen, mis. "limit"; "arguments" untuk masalah bentuk object
	Reason string // mis. "harus 1-500", "wajib diisi", "harus integer"
	Got    string // nilai ringkas (maks 40 rune, sisanya diganti "..."); "" berarti bagian (diterima: ...) dihilangkan
	Hint   string // saran langkah berikut; boleh kosong
	Custom string // bila diisi, Message() mengembalikan ini apa adanya (pesan project_id)
}

func (e *ArgError) Message() string        // D6: "Argumen tidak valid: <Field> <Reason> (diterima: <Got>). <Hint>"
func (e *ArgError) Summary() map[string]any // {"error_code":"invalid_argument","field":Field}

// DecodeStrict mendekode object JSON ke dst (pointer ke struct argumen mentah).
// - raw kosong, spasi, atau literal null dianggap {} (defensif; endpoint T07 sudah menolak null).
// - bukan object -> ArgError{Field:"arguments", Reason:"harus object JSON"}.
// - json.Decoder + DisallowUnknownFields; token sisa setelah object -> ArgError{Field:"arguments", Reason:"harus satu object JSON"}.
// - field tak dikenal (parse `json: unknown field "x"`):
//     x di ProjectArgAliases -> Custom: "Argumen <x> tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."
//     selain itu -> Custom: "Argumen tidak valid: <x> tidak dikenal. Argumen yang didukung: <allowed dipisah koma>."
// - *json.UnmarshalTypeError -> ArgError{Field: e.Field, Reason: "harus " + typeName(e.Type)}; typeName: string->"string",
//   int->"integer", bool->"boolean"; Got = e.Value (mis. "string", "number").
// - error JSON lain (sintaks) -> ArgError{Field:"arguments", Reason:"bukan JSON valid"}.
func DecodeStrict(raw json.RawMessage, dst any, allowed []string) *ArgError

// IntArg memvalidasi argumen integer opsional. nil -> def. Di luar [min,max] -> ArgError{Field, "harus <min>-<max>", Got:itoa, Hint}.
// Bila max == math.MaxInt32 maka Reason menjadi "harus >= <min>".
func IntArg(field string, v *int, def, min, max int, hint string) (int, *ArgError)

// BoolArg: nil -> def.
func BoolArg(v *bool, def bool) bool

// Brief meringkas nilai untuk Got: maks 40 rune, karakter kontrol ditampilkan sebagai \uXXXX, lalu ditambah "..." bila terpotong.
func Brief(s string) string
```

Catatan: `json.Decoder` menolak angka pecahan pada `*int` lewat `UnmarshalTypeError`. Angka besar di luar `int` juga menjadi `UnmarshalTypeError`, jadi tidak perlu kasus khusus.

Tes `args_test.go` (tabel):
- `""`, `"null"`, dan `" "` menjadi struct kosong tanpa error;
- `"[]"` dan `"1"` menghasilkan `Field=="arguments"`;
- `{"a":1}{"b":2}` menghasilkan error token sisa;
- `{"project_id":"x"}`, `{"projectId":"x"}`, dan `{"project":"x"}` menghasilkan pesan khusus yang memuat `ditentukan oleh API key`;
- `{"foo":1}` menghasilkan pesan yang memuat `foo tidak dikenal` dan daftar allowed;
- `{"limit":"x"}` menghasilkan `Field=="limit"` dan `Reason=="harus integer"`;
- `{"limit":1.5}` menghasilkan `harus integer`;
- `{"recursive":"true"}` menghasilkan `harus boolean`;
- `IntArg` untuk nil/default/min/max/min-1/max+1;
- `Message()` dengan dan tanpa `Got`/`Hint`;
- `Brief` untuk 100 rune multibyte menghasilkan 40 rune ditambah `...`.

### 5.3 `path.go`

```go
const MaxPathBytes = 1024

// NormalizePath untuk read_file: TrimSpace, buang satu "/" di depan. Kosong -> ArgError "wajib diisi".
// Berakhir "/" -> ArgError{Field, Reason:"menunjuk folder", Hint:"Gunakan list_files dengan path=<tanpa slash akhir>."}.
func NormalizePath(field, raw string) (string, *ArgError)

// NormalizePrefix untuk list_files/search_knowledge: TrimSpace, buang satu "/" di depan dan satu di belakang. Kosong = akar (valid).
func NormalizePrefix(field, raw string) (string, *ArgError)
```

Aturan bersama (dicek setelah trim):
- `len > MaxPathBytes` ditolak dengan Reason `"maksimal 1024 byte"`;
- `!utf8.ValidString` ditolak dengan `"bukan UTF-8 valid"`;
- rune `< 0x20` atau `0x7f` (termasuk NUL) ditolak dengan `"memuat karakter kontrol"`;
- segmen `..` (hasil `strings.Split(p, "/")`) ditolak dengan `"tidak boleh memuat segmen .."` dan Hint `"Gunakan path relatif dari akar knowledge, mis. docs/guides/a.md."`.

Tes `path_test.go`:
- `"  /docs/  "` menjadi `"docs"` (prefix);
- `"/docs/a.md"` menjadi `"docs/a.md"`;
- `"//docs"` menjadi `"/docs"` (hanya satu slash dibuang; sisanya dinilai T05);
- `"docs/"` ditolak di `NormalizePath`;
- 1025 byte ditolak dan 1024 byte diterima;
- `"../a"` dan `"a/../b"` ditolak, tetapi `"a..b"` diterima;
- `"a\x00b"` dan `"a\tb"` ditolak;
- prefix `""` dan `"/"` menjadi `""`.

### 5.4 `format.go`

```go
func HumanSize(n int) string        // D9: 0 -> "0 B", 1023 -> "1023 B", 1536 -> "1.5 KB", 2411724 -> "2.3 MB"
func FormatTime(t time.Time) string // t.UTC().Format("2006-01-02 15:04 UTC"); zero time -> "-"
func RFC3339(t time.Time) string    // t.UTC().Format(time.RFC3339); zero time -> ""
func OneLine(s string) string       // ganti \r\n, \n, \r, \t dengan satu spasi (dipakai snippet)
```

Tes: tabel batas 1023/1024/1048575/1048576. Zona waktu non-UTC (`Asia/Jakarta`) dirender UTC.

### 5.5 `result.go` (pemetaan hasil bersama, dipakai ketiga handler)

```go
// ArgResult membangun hasil isError untuk ArgError.
func ArgResult(e *ArgError, targetType, target string) toolkit.Result
// -> Result{IsError:true, Text:e.Message(), TargetType, Target, Summary:e.Summary()}

// BusResult memetakan error bus sesuai D13. ok=false berarti caller harus mengembalikan error Go.
func BusResult(tool string, err error, hint, targetType, target string) (res toolkit.Result, goErr error)
// errors.Is(err, errs.ErrInvalid) -> (Result{IsError:true, Text:"Permintaan ditolak: "+err.Error()+". "+hint,
//                                   Summary:{"error_code":"invalid_request"}}, nil)
// selain itu                       -> (Result{}, fmt.Errorf("%s: %w", tool, err))
```

Tes: ErrInvalid terbungkus (`fmt.Errorf("x: %w", errs.ErrInvalid)`) menjadi isError. `bus.ErrNotRegistered`-like dan `context.DeadlineExceeded` dikembalikan sebagai error Go dengan `errors.Is` tetap berlaku (gateway perlu `DeadlineExceeded` untuk kode `timeout`).

### 5.6 Entity per tool (dibuat di plan usecase, kontraknya dibekukan di sini)

| File | Entity | Konstruktor | Method wajib | Target/Summary (D8) |
|---|---|---|---|---|
| `listing.go` | `ListPage{Prefix, Recursive, Offset, Items []listfiles.Item, Folders []string, Total}` | `NewListPage(prefix string, recursive bool, offset int, resp listfiles.Response) ListPage` | `NextOffset() int` (-1 bila habis), `Text()`, `Structured()`, `Summary()`, `Result() toolkit.Result` (termasuk `offset_out_of_range`) | `folder`, prefix, `{count,total,folders,next_offset,recursive}` |
| `reading.go` | `ReadChunk{Path, Offset, Content, SizeBytes, TotalBytes, NextOffset, Truncated}`, `NotFound{Path, Similar}`, `type ReadOutcome interface{ Result() toolkit.Result }` | `NewReadChunk(resp readfile.Response, offset int)`, `NewNotFound(path string, similar []string)`, `MaxReadLimit(maxResponseBytes int) int`, `DefaultReadLimit(max int) int` | `Text()`, `Summary()`, `Result()` (`Structured` nil) | `file`, path, `{found,bytes,total_bytes,offset,next_offset,truncated}` / `{found:false,similar_count,error_code:"not_found"}` |
| `searching.go` | `SearchHits{Query, Prefix, Items []search.Item, Total}` | `NewSearchHits(query, prefix string, resp search.Response)` | `Text()`, `Structured()`, `Summary()`, `Result()` | `folder`, prefix, `{count,total}` |

`Result()` milik entity merangkai `Text/IsError/Structured/TargetType/Target/Summary`. Dengan begitu handler cukup memanggil `entity.Result()` (rule tetap di entity). Rincian teks ada di plan usecase masing-masing.

`Summary` DILARANG memuat isi file, snippet, maupun query. Query sudah tercatat di `Args` audit oleh gateway.

---

## 6. Kontrak bus yang dikonsumsi (tidak ada perubahan di `lib/constants` atau `lib/contracts/project`)

| Konstanta (`lib/constants/bus.go`) | Nama | Request yang dikirim T08 | Response | Catatan |
|---|---|---|---|---|
| `BUS_PROJECT_KNOWLEDGE_LIST_FILES` | `project.knowledge.list-files` | `listfiles.Request{ProjectID: call.ProjectID, Prefix, Recursive, Limit (selalu eksplisit, 1-500), Offset}` | `{Items[{Path,SizeBytes,UpdatedAt}], Folders, Total, Truncated}` | `Folders` nil bila Recursive |
| `BUS_PROJECT_KNOWLEDGE_READ_FILE` | `project.knowledge.read-file` | `readfile.Request{ProjectID: call.ProjectID, Path, Offset, Limit}` (byte, D2; Limit selalu eksplisit) | `{Found, Path, Content, SizeBytes, TotalBytes, NextOffset, Truncated, Similar(<=5)}` | Offset di luar file menghasilkan `ErrInvalid` |
| `BUS_PROJECT_KNOWLEDGE_SEARCH` | `project.knowledge.search` | `search.Request{ProjectID: call.ProjectID, Query, Prefix, Limit (1-20)}` | `{Items[{Path,Score,Snippet}], Total}` | sanitasi tsquery dilakukan T05 |

Error dari ketiganya dipetakan lewat `base.BusResult` (D13). Konsumer T08 sudah tercantum di katalog bus README dan di komentar paket kontrak T05.

Kontrak T07 yang diubah secara aditif (D1), di `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go`:

```go
type Result struct {
	Text       string
	IsError    bool
	TargetType string
	Target     string
	Summary    map[string]any
	// Structured opsional, menjadi structuredContent hasil MCP. Wajib JSON object (map dengan nilai
	// yang bisa di-marshal), berisi metadata/daftar saja. Tidak masuk audit dan tidak dipotong gateway;
	// tool bertanggung jawab menjaga ukurannya kecil (daftar dibatasi limit tool).
	Structured map[string]any
}
```

---

## 7. Migration

Tidak ada. T08 tidak membuat tabel, kolom, maupun index. Seluruh data dibaca lewat bus T05.

---

## 8. Plan (langkah berurutan)

**F1. Gerbang prasyarat.** Cek semua baris tabel bagian 0. Bila T07 belum ada di `main`, hentikan dan laporkan. Buat branch `feat/discovery-knowledge-tools` dari `main`.

**F2. D1: field `Structured` pada kontrak T07.**
1. `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go`: tambah field `Structured` beserta komentar (bagian 6).
2. `backend/internal/domain/discovery/internal/mcpgateway/endpoint/` (file struct hasil, `toCallToolResult`): tambah `StructuredContent map[string]any \`json:"structuredContent,omitempty"\`` dan isi dari `o.Result.Structured`. Untuk hasil buatan gateway (internal/timeout/panic) `Structured` tetap nil.
3. Tes endpoint (`params_test.go` atau tempat tes `toCallToolResult` berada): `Structured` nil berarti JSON tidak memuat kunci `structuredContent`; `Structured{"total":3}` berarti `"structuredContent":{"total":3}` ada.
4. Tes `callaudit`: tool palsu mengembalikan `Structured{"items":[...]}`, lalu `Outcome.Result.Structured` sama persis dan entri audit tidak memuat kunci `items`/`structured`.
5. `go build ./... && go test ./internal/domain/discovery/...` harus hijau.

**F3. `base/` bersama.** Buat `names.go`, `args.go`, `path.go`, `format.go`, dan `result.go` beserta tesnya (bagian 5.1-5.5). `listing.go`, `reading.go`, dan `searching.go` dibuat di F5 oleh usecase masing-masing.

**F4. `knowledgetools/base.go`.**
```go
// Package knowledgetools menyediakan tool MCP knowledge (T08) yang didaftarkan ke gateway T07 lewat toolkit.
package knowledgetools

// Register membangun tiga tool read-only. Tidak mendaftarkan endpoint bus dan tidak punya state.
func Register(d app.Deps) []toolkit.Tool {
	return []toolkit.Tool{listfiles.Init(d), readfile.Init(d), searchknowledge.Init(d)}
}
```
Selama feature belum selesai, `Init` boleh berupa stub. Semua stub wajib terisi sebelum merge (gerbang akhir F10).

**F5. Usecase (paralel, hanya berbagi `base/`).**
- `listfiles`: lihat `usecase-listfiles.md` (+ `base/listing.go`).
- `readfile`: lihat `usecase-readfile.md` (+ `base/reading.go`).
- `searchknowledge`: lihat `usecase-searchknowledge.md` (+ `base/searching.go`).
Konflik antar usecase hanya mungkin di `base/` bersama. Bila butuh helper baru di `args.go`/`path.go`, tambahkan secara aditif dan beri tes.

**F6. Wiring domain.** `backend/internal/domain/discovery/base.go`: ganti komentar `// T08: ...` dengan `tools = append(tools, knowledgetools.Register(d)...)` dan tambahkan import `grimoire/internal/domain/discovery/internal/knowledgetools`. Komentar T09 tetap.

**F7. Tes module.** Buat `knowledgetools/base_test.go` dan `testdata/tools_list.golden.json` (bagian 10.2). Jalankan sekali dengan `-update`, lalu periksa diff golden secara manual terhadap teks di bagian 12.1.

**F8. Tes integrasi.** Buat `backend/cmd/internal/server/knowledgetools_integration_test.go` (`//go:build integration`), lihat bagian 10.3.

**F9. Dokumen.**
- `docs/mcp-gateway.md` (milik T07): tambah bagian "Tool knowledge" yang memuat tiga tool, tabel argumen, satu contoh `tools/call` per tool, dan catatan D2 (byte).
- `docs/tickets/T08-discovery-knowledge-tools.md`: tambah bagian `## Keputusan` (D1, D2, D3, D5, D12).
- `docs/tickets/README.md`: status T08, catatan `toolkit.Result.Structured` di paragraf register-tool, dan pastikan katalog bus mencantumkan T08 sebagai consumer ketiga endpoint.

**F10. Gerbang akhir.**
```
cd backend
go build ./...
go vet ./...
go test ./...                                   # termasuk internal/archtest
go test -race ./internal/domain/discovery/...
go test -tags integration ./cmd/internal/server/...
golangci-lint run
```
Tidak boleh ada stub `Init` yang tersisa. Golden harus stabil (dua kali jalan tanpa `-update` tetap hijau).

---

## 9. Changes (per file/folder)

| File | Status | Isi |
|---|---|---|
| `backend/lib/contracts/discovery/mcpgateway/toolkit/toolkit.go` | ubah (milik T07) | field `Result.Structured` (D1a) |
| `backend/internal/domain/discovery/internal/mcpgateway/endpoint/*.go` | ubah (milik T07) | `callToolResult.StructuredContent`, pemetaan, dan tes (D1b) |
| `backend/internal/domain/discovery/internal/mcpgateway/callaudit/*_test.go` | ubah (milik T07) | asersi Structured diteruskan dan tidak diaudit (D1c) |
| `backend/internal/domain/discovery/base.go` | ubah | append `knowledgetools.Register(d)` dan import |
| `backend/internal/domain/discovery/internal/knowledgetools/base.go` | baru | `Register` |
| `backend/internal/domain/discovery/internal/knowledgetools/base_test.go` | baru | AC6, AC7, jumlah tool |
| `backend/internal/domain/discovery/internal/knowledgetools/testdata/tools_list.golden.json` | baru | kontrak `tools/list` |
| `.../knowledgetools/base/{names,args,path,format,result}.go` (+ `_test.go`) | baru | base bersama (foundation) |
| `.../knowledgetools/base/{listing,reading,searching}.go` (+ `_test.go`) | baru | entity per tool (plan usecase) |
| `.../knowledgetools/listfiles/*` | baru | usecase `listfiles` |
| `.../knowledgetools/readfile/*` | baru | usecase `readfile` |
| `.../knowledgetools/searchknowledge/*` | baru | usecase `searchknowledge` |
| `backend/cmd/internal/server/knowledgetools_integration_test.go` | baru | e2e AC1-AC5 |
| `docs/mcp-gateway.md` | ubah | bagian "Tool knowledge" |
| `docs/tickets/T08-discovery-knowledge-tools.md` | ubah | bagian Keputusan |
| `docs/tickets/README.md` | ubah | status, catatan Structured |

TIDAK berubah: `lib/constants/bus.go`, `lib/contracts/project/**`, `lib/contracts/core/audit/record`, `infra/config`, `api/modules/**`, `migrations/**`, `frontend/**`, `go.mod`.

Allowlist import module (AC7, lebih ketat dari archtest):
- stdlib;
- `grimoire/internal/infra/app`;
- `grimoire/internal/infra/bus`;
- `grimoire/lib/constants`;
- `grimoire/lib/contracts/project/knowledge/{listfiles,readfile,search}`;
- `grimoire/lib/contracts/discovery/mcpgateway/toolkit`;
- `grimoire/lib/common/errs`;
- package internal module ini sendiri (`.../knowledgetools/...`).

DILARANG: `internal/domain/project/**` dan `internal/domain/discovery/internal/mcpgateway/**`.

---

## 10. Test Scenario

### 10.1 Unit (per file `base/`, dirinci di bagian 5; per feature, dirinci di plan usecase)

| Lapisan | Alat | Fokus |
|---|---|---|
| `base/*_test.go` | tabel, tanpa I/O | decode ketat, path, format, pemetaan error, render entity |
| `<feature>/params_test.go` | tabel | default, rentang, tipe, `project_id` (AC5), normalisasi |
| `<feature>/usecase_test.go` | fake `Lister`/`Reader`/`Searcher` | request diteruskan persis (ProjectID dari Execute, Limit eksplisit), error diteruskan |
| `<feature>/handler_test.go` | `bus.NewRegistry()` + handler palsu terdaftar pada konstanta T05 | AC5 (bus tidak dipanggil bila argumen salah; `Request.ProjectID == call.ProjectID`), ErrInvalid menjadi isError, error lain dan `ErrNotRegistered` menjadi error Go, Summary tanpa isi |

### 10.2 Tes module `knowledgetools/base_test.go`

- `TestRegister_ThreeReadOnlyTools`: `Register(app.Deps{Config: cfg, Bus: bus.NewRegistry()})` menghasilkan 3 tool, nama unik (`list_files`, `read_file`, `search_knowledge`), `Handler != nil`, `Title`/`Description` tidak kosong, dan `InputSchema` adalah JSON valid.
- `TestT08_AC6_ToolsListContract`: config default (`MCPMaxResponseBytes=102400`). Proyeksikan setiap tool ke `{name,title,description,inputSchema}` (inputSchema di-unmarshal ke `any` lalu di-marshal ulang dengan indentasi 2 spasi supaya diff stabil), urutkan berdasarkan nama, lalu bandingkan byte-per-byte dengan `testdata/tools_list.golden.json`. Flag `-update` (`flag.Bool`) menulis ulang golden.
- `TestT08_AC6_SchemasAreValidJSONSchema` (validator meta minimal, tanpa dependensi):
  - root `type=="object"` dan `additionalProperties==false`;
  - keyword root hanya `{type, properties, required, additionalProperties}`;
  - setiap `properties[k]` punya `type` di `{string, integer, boolean}`, `description` tidak kosong, dan keyword hanya dari `{type, description, default, minimum, maximum, minLength, maxLength}`;
  - `minimum`/`maximum` hanya untuk integer dan `minLength`/`maxLength` hanya untuk string;
  - `default` sesuai tipe dan `minimum <= default <= maximum`;
  - `required ⊆ properties`;
  - tidak ada properti `project`, `project_id`, maupun `projectId`;
  - nama cocok `^[a-z][a-z0-9_]{0,63}$` dan `"mcp."+name` ada di `record.Actions()` (import `record` hanya di file tes).
  - Varian: `Register` dengan `MCPMaxResponseBytes=1024` menghasilkan `read_file.limit.maximum==1024` dan `default==1024`.
- `TestT08_AC7_ImportAllowlist`: `filepath.WalkDir` dari direktori module, lalu `parser.ParseFile(..., parser.ImportsOnly)` untuk setiap `.go` (termasuk `_test.go`). Setiap import harus stdlib (tidak ada titik di segmen pertama) atau ada di allowlist bagian 9. File `_test.go` boleh tambahan `grimoire/lib/contracts/core/audit/record` (stdlib seperti `testing`, `flag`, `os`, `go/parser`, dan `go/token` sudah tercakup aturan stdlib). Gagal dengan pesan `<file>: import terlarang <path>`.

### 10.3 Integrasi (`backend/cmd/internal/server/knowledgetools_integration_test.go`, `//go:build integration`)

Setup mengikuti pola tes integrasi T07 di package yang sama:
- Postgres tes dan migrasi;
- rakit `core` + `project` + `discovery` dengan bus nyata;
- buat project A dan B lewat bus/repository T03;
- buat API key T04 asli untuk A;
- unggah file T05 lewat bus/repository T05;
- jalankan `httptest.Server`;
- helper `mcpCall(t, key, name string, args any) callToolResult`, yang mengirim `initialize` lalu `tools/call`.

Fixture project A:
- `readme.md` (100 B);
- `docs/a.md`;
- `docs/readme.md`;
- `docs/guides/b.md` (isi "rate limiting per key");
- `docs/big.txt` 200 KB (campuran ASCII dan multibyte 3/4 byte).

Fixture project B: `secret/b.md` berisi `SECRET-B`.

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT08_Int_ToolsList` | 6 | `tools/list` | ada 3 tool T08 dengan `annotations.readOnlyHint:true` |
| `TestT08_Int_AC1_ListTopLevel` | 1 | `list_files {}` | teks memuat `readme.md` dan `[folder] docs/`, tanpa `docs/a.md`; `structuredContent.folders == ["docs"]`; `next_offset == null` |
| `TestT08_Int_AC1_ListRecursivePaged` | 1 | `{recursive:true, limit:2}` lalu `{recursive:true, limit:2, offset:2}` | halaman 1 berisi 2 file dan penanda `offset=2`; halaman 2 berisi sisanya; gabungan path == semua 5 file tanpa duplikat |
| `TestT08_Int_AC2_ReadChunks` | 2 | `read_file {path:"docs/big.txt", limit:50000}` lalu ikuti `offset=(\d+)` sampai tidak ada penanda | gabungan == isi asli byte-per-byte; tidak ada respons yang memuat `_meta.truncated` |
| `TestT08_Int_AC3_SimilarPaths` | 3 | `read_file {path:"docs/redme.md"}` | `isError:true`; teks memuat `File tidak ditemukan`, `Similar:`, `- docs/readme.md`, `list_files` |
| `TestT08_Int_AC4_SearchSnippet` | 4 | `search_knowledge {query:"rate limit"}` | `isError:false`; teks memuat `docs/guides/b.md` dan `**rate` |
| `TestT08_Int_AC4_SpecialChars` | 4 | query `a & b \| !c:* (x)` dan `'"` | tidak ada error JSON-RPC; `isError:false` (hasil boleh kosong) |
| `TestT08_Int_AC4_PrefixFilter` | 4 | `{query:"rate", path:"docs/guides"}` vs `{query:"rate", path:"other"}` | hanya hasil di bawah prefix |
| `TestT08_Int_AC5_ProjectArgRejected` | 5 | key A, `read_file {path:"secret/b.md", project_id:<B>}`; `list_files {project_id:<B>}` | `isError:true`, teks memuat `ditentukan oleh API key`, tidak memuat `SECRET-B`; tanpa `project_id`, `read_file secret/b.md` menghasilkan not found di A |
| `TestT08_Int_AuditOneEntryPerCall` | D8 | setelah satu `read_file` sukses | tepat satu entri `mcp.read_file`, `target_type=file`, `target=docs/a.md`, Result tidak memuat isi file |

---

## 11. E2E Scenarios

Positif:
1. Agent memanggil `list_files {}`, melihat folder `docs`, lalu `list_files {path:"docs"}`, `read_file {path:"docs/a.md"}`, dan mendapat isi utuh tanpa penanda.
2. Agent membaca file 200 KB: `read_file {limit:50000}` menghasilkan penanda `offset=50000` (atau sedikit kurang di batas UTF-8). Agent mengikuti penanda sampai selesai dan isinya utuh.
3. Agent mencari `search_knowledge {query:"rate limit"}`, mendapat path, skor, dan cuplikan `**rate**`, lalu `read_file` pada path itu.
4. `list_files {recursive:true, limit:2}` menghasilkan penanda, lalu `offset=2` mengembalikan sisanya.
5. Pencarian tanpa hasil menghasilkan `isError:false` dengan saran kata kunci lain.
6. Project kosong: `list_files {}` menghasilkan `Belum ada file knowledge di project ini.` (`isError:false`).

Negatif:
1. `read_file {}` menghasilkan `isError`: `Argumen tidak valid: path wajib diisi...`.
2. `list_files {limit:900}` menghasilkan `isError`: `limit harus 1-500 (diterima: 900)`.
3. `list_files {project_id:"<B>"}` menghasilkan `isError` dengan pesan khusus. Project tidak berubah dan bus tidak dipanggil.
4. `search_knowledge {query:"   "}` menghasilkan `isError` `query wajib diisi 1-200 karakter`.
5. `read_file {path:"docs/redme.md"}` menghasilkan `isError` beserta Similar.
6. `read_file {path:"docs/a.md", offset:999999999}` (melewati file) membuat T05 mengembalikan ErrInvalid, sehingga hasilnya `isError` `Permintaan ditolak: ...` dengan `error_code=invalid_request`.
7. `list_files {path:"../etc"}` menghasilkan `isError` `tidak boleh memuat segmen ..`.
8. Provider T05 tidak terdaftar atau timeout menghasilkan error Go. Gateway mengembalikan `Terjadi kesalahan internal saat menjalankan tool <name>. trace_id=<id>` atau teks timeout, dan audit `error_code=internal|timeout`.
9. `list_files {recursive:"true"}` menghasilkan `isError` `recursive harus boolean`.

---

## 12. Request/Response body

### 12.1 `tools/list` (isi golden `testdata/tools_list.golden.json`, config default)

Golden berisi array (bukan respons JSON-RPC), diurutkan berdasarkan nama:

```json
[
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
    }
  },
  {
    "name": "read_file",
    "title": "Baca file knowledge",
    "description": "Baca isi satu file knowledge (read-only). path persis seperti dari list_files atau search_knowledge. File besar dikembalikan per potongan: bila ada penanda [terpotong: ... offset=N ...], panggil lagi dengan offset=N untuk lanjutan tanpa duplikat. offset dan limit dalam byte.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "path": { "type": "string", "minLength": 1, "maxLength": 1024, "description": "path file, mis. docs/guides/a.md" },
        "offset": { "type": "integer", "minimum": 0, "maximum": 2147483647, "default": 0, "description": "posisi byte awal; pakai nilai dari penanda terpotong" },
        "limit": { "type": "integer", "minimum": 1, "maximum": 101888, "default": 40000, "description": "jumlah byte maksimal per potongan" }
      },
      "required": ["path"],
      "additionalProperties": false
    }
  },
  {
    "name": "search_knowledge",
    "title": "Cari knowledge",
    "description": "Cari file knowledge berdasarkan kata kunci (full-text, read-only, bukan semantik). Frasa dalam tanda kutip dicari utuh. Hasil berisi path, skor, dan cuplikan dengan kata kunci bertanda **...**; baca file utuh dengan read_file(path). Persempit dengan path=<folder>.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": { "type": "string", "minLength": 1, "maxLength": 200, "description": "kata kunci atau frasa dalam tanda kutip, mis. \"rate limit\"" },
        "path": { "type": "string", "maxLength": 1024, "description": "prefix folder opsional, mis. docs/guides" },
        "limit": { "type": "integer", "minimum": 1, "maximum": 20, "default": 10, "description": "jumlah hasil maksimal" }
      },
      "required": ["query"],
      "additionalProperties": false
    }
  }
]
```

Teks deskripsi tool di atas dibekukan dan disalin persis ke `init.go`. Deskripsi properti yang tidak didefinisikan di plan usecase memakai teks di atas. Urutan kunci dalam golden mengikuti hasil `json.MarshalIndent` dari `map` (alfabetis). Contoh di atas ditulis berurutan supaya mudah dibaca, dan file golden yang dihasilkan `-update` adalah acuannya.

Di respons `tools/list` T07, tiap entri ditambah `"annotations":{"readOnlyHint":true}` oleh gateway.

### 12.2 `tools/call` list_files (sukses, terpotong)

Request:
```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_files","arguments":{"recursive":true,"limit":2}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 7,
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
Audit (oleh gateway): `action=mcp.list_files`, `target_type=folder`, `target=""`, result memuat `{count:2,total:5,folders:0,next_offset:2,recursive:true}` ditambah field gateway.

### 12.3 `tools/call` read_file (potongan)

Request:
```json
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/big.txt","limit":50000}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "result": {
    "content": [{"type":"text","text":"<50000 byte isi file>\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]"}],
    "isError": false
  }
}
```
Tidak ada `structuredContent` (Structured nil) dan tidak ada `_meta` (gateway tidak memotong). Summary audit: `{found:true,bytes:49998,total_bytes:204800,offset:0,next_offset:49998,truncated:true}`.

### 12.4 `tools/call` read_file (tidak ditemukan)

```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"File tidak ditemukan: docs/redme.md\nSimilar:\n- docs/readme.md\n- readme.md\nGunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi."}],"isError":true}}
```
Summary: `{found:false, similar_count:2, error_code:"not_found"}`.

### 12.5 `tools/call` search_knowledge

Request:
```json
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"rate limit","limit":5}}}
```
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "content": [{"type":"text","text":"1 dari 1 hasil untuk \"rate limit\"\n1. docs/guides/b.md  (skor 0.608)\n   **rate** **limiting** per key\nBaca isi lengkap dengan read_file(path)."}],
    "isError": false,
    "structuredContent": {
      "items": [{"path":"docs/guides/b.md","score":0.6079,"snippet":"**rate** **limiting** per key"}],
      "total": 1,
      "query": "rate limit"
    }
  }
}
```
Tanpa hasil:
```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"Tidak ada hasil untuk \"xyz\"\nCoba kata kunci lain yang lebih umum atau sinonim, kurangi jumlah kata, hapus filter path, atau jelajahi struktur dengan list_files."}],"isError":false,"structuredContent":{"items":[],"total":0,"query":"xyz"}}}
```

### 12.6 Format error

Argumen tidak valid (`invalid_argument`, bus tidak dipanggil):
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya."}],"isError":true}}
```
Summary audit: `{error_code:"invalid_argument", field:"limit"}`.

Argumen project (AC5):
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```

Ditolak provider (`invalid_request`, `errs.ErrInvalid` dari T05):
```json
{"jsonrpc":"2.0","id":14,"result":{"content":[{"type":"text","text":"Permintaan ditolak: offset 999999999 melewati ukuran file 1229 byte: invalid argument. Gunakan offset dari penanda terpotong atau 0."}],"isError":true}}
```

Error internal (error Go dari tool, dipetakan gateway T07, detail disembunyikan):
```json
{"jsonrpc":"2.0","id":15,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=7f3c2a9e1b4d"}],"isError":true}}
```

Timeout (ctx deadline, dipetakan gateway):
```json
{"jsonrpc":"2.0","id":16,"result":{"content":[{"type":"text","text":"Tool search_knowledge melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```

Nama tool tidak dikenal tetap ditangani T07 (`-32602 Unknown tool: <name>`). T08 tidak mengubah perilaku ini.

---

## 13. Daftar usecase

| Slug | Folder | Tool MCP | Bus | Entity base | Plan |
|---|---|---|---|---|---|
| `listfiles` | `knowledgetools/listfiles/` | `list_files` | `project.knowledge.list-files` | `base/listing.go` (`ListPage`) | `docs/superpowers/plans/discovery-knowledge-tools/usecase-listfiles.md` |
| `readfile` | `knowledgetools/readfile/` | `read_file` | `project.knowledge.read-file` | `base/reading.go` (`ReadChunk`, `NotFound`, `MaxReadLimit`) | `docs/superpowers/plans/discovery-knowledge-tools/usecase-readfile.md` |
| `searchknowledge` | `knowledgetools/searchknowledge/` | `search_knowledge` | `project.knowledge.search` | `base/searching.go` (`SearchHits`) | `docs/superpowers/plans/discovery-knowledge-tools/usecase-searchknowledge.md` |

`register` (dari tabel features tiket) bukan usecase. Fungsinya dijalankan F4 + F6 (D12).
