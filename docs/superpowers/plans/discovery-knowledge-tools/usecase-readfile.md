# T08 discovery/knowledge-tools — Usecase `readfile` (tool MCP `read_file`)

Tiket: `docs/tickets/T08-discovery-knowledge-tools.md`
Induk (mengikat): `docs/superpowers/plans/discovery-knowledge-tools/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview yang berlaku sampai diperbarui. Rujukan keputusan memakai ID overview (D1-D13).
Konteks yang dicek: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` tidak relevan (tanpa migrasi); tidak ada knowledge doc module; tidak ada UI (frontend tidak direncanakan, bagian "Visual" tidak berlaku). Kontrak T05 `backend/lib/contracts/project/knowledge/readfile` dan implementasi provider `project/internal/knowledge/base/slice.go` (`SliceUTF8`) sudah dibaca.

Perilaku provider T05 yang diandalkan plan ini:
- `Offset`/`Limit` dalam byte. `Offset` di tengah rune maju ke awal rune berikutnya. Akhir potongan mundur ke batas rune, tetapi selalu maju minimal satu rune, jadi `NextOffset > Offset` selama masih ada sisa. Potongan bisa melebihi `Limit` paling banyak 3 byte (limit lebih kecil dari satu rune).
- `Offset == TotalBytes` menghasilkan `Content ""`, `Truncated false`. `Offset > TotalBytes` menghasilkan `errs.ErrInvalid` ("offset N di luar ukuran file (M byte)").
- Tidak ketemu: `Found=false`, `Similar` maks 5, err nil. `Limit` provider dibatasi `Config.MCPMaxResponseBytes`.

---

## 0. Prasyarat (gerbang)

Usecase ini bergantung pada foundation overview (F1-F4). Sebelum mulai, cek:

| Prasyarat | Cara cek |
|---|---|
| Gerbang F1 overview terpenuhi: T07 sudah ada di `main` (`toolkit.{Tool,Handler,Call,Result}`) | `git log main --oneline \| grep discovery-mcp-gateway` |
| `base/{names,args,path,format,result}.go` dari F3 sudah ada beserta tesnya | `ls backend/internal/domain/discovery/internal/knowledgetools/base` |
| `knowledgetools/base.go` (F4) memanggil `readfile.Init(d)` | baca file |

Bila T07 belum merge atau `base/` bersama belum ada, berhenti dan lapor. Jangan menyalin kode T07, jangan membuat ulang `args.go`/`path.go` di sini.

Simbol bersama yang dipakai (semua dari overview bagian 5, signature tidak boleh diubah di sini):

| Simbol | Dari |
|---|---|
| `base.ToolReadFile`, `base.ProjectArgAliases` | `base/names.go` |
| `base.ArgError`, `base.DecodeStrict(raw, dst, allowed)`, `base.IntArg(field, v, def, min, max, hint)`, `base.Brief` | `base/args.go` |
| `base.NormalizePath(field, raw)` | `base/path.go` |
| `base.ArgResult(e, targetType, target)`, `base.BusResult(tool, err, hint, targetType, target)` | `base/result.go` |
| `base.HumanSize` | tidak dipakai di tool ini |

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T08 (S), usecase 1 dari 3 |
| Scope | fullstack menurut tiket; UI tiket "Tidak ada" jadi hanya backend |
| Domain / Module | discovery / knowledge-tools (package `knowledgetools`) |
| Feature folder | `backend/internal/domain/discovery/internal/knowledgetools/readfile/` |
| Endpoint (milik T07) | `/mcp` |
| Method | POST, JSON-RPC `tools/call` dengan `params.name="read_file"` (dan `tools/list` memuat entri tool ini) |
| Version | MCP 2025-11-25 / 2025-06-18 / 2025-03-26, JSON-RPC 2.0 |
| Endpoint REST baru | Tidak ada. endpointsChanged = false |
| Bus dikonsumsi | `project.knowledge.read-file` (`constants.BUS_PROJECT_KNOWLEDGE_READ_FILE`, Call sinkron, provider T05) |
| Bus disediakan | Tidak ada |
| Aksi audit | `mcp.read_file` (sudah ada di `record.Actions()`, ditulis gateway; tool tidak menulis audit, D8) |
| Migrasi | Tidak ada |

---

## 2. Summary Plan

`read_file` membaca isi satu file knowledge per potongan byte. Handler mem-parse argumen secara ketat (`path`, `offset`, `limit`, tanpa `project_id`), usecase memanggil bus T05 lewat repository, lalu entity `base` menentukan bentuk hasil: potongan teks, penanda lanjutan, atau "tidak ditemukan" dengan saran path.

Keputusan kunci usecase ini:
1. Teks hasil adalah isi file apa adanya, tanpa header. Satu-satunya tambahan adalah penanda `[terpotong: ...]` di akhir bila masih ada sisa. Dengan begitu menyambung teks semua potongan (setelah membuang penanda) sama persis dengan file asli (AC2).
2. Satuan byte (D2, menyimpang dari tiket yang menulis "karakter"). Offset di penanda adalah `NextOffset` dari T05, jadi follow-up tanpa duplikat dan tanpa celah, termasuk untuk file multibyte.
3. `limit` maksimum = `MCPMaxResponseBytes - 512` (minimal 1024). Cadangan 512 byte memuat penanda dan selisih rune, sehingga `base.Truncate` T07 tidak pernah memotong teks `read_file`. Penanda T08 menjadi satu-satunya sumber offset lanjutan.
4. Nilai di luar rentang ditolak, bukan di-clamp (D5), dengan pesan yang menyebut angka maksimum.
5. `ProjectID` hanya dari `call.ProjectID`. Argumen `project_id` ditolak oleh `DecodeStrict` (AC5).
6. Rule bentuk hasil (penanda, file kosong, saran, ringkasan audit) ada di entity `base/reading.go`. Usecase hanya orkestrasi: panggil `Reader`, lalu pilih `NewReadChunk` atau `NewNotFound`. Satu usecase, tidak dipecah.

---

## 3. Plan (langkah berurutan)

Semua path relatif terhadap `backend/internal/domain/discovery/internal/knowledgetools/` kecuali disebut lain. Kerjakan dengan TDD: tes dulu, lalu implementasi.

**R0. Gerbang prasyarat.** Lihat bagian 0. Berhenti dan lapor bila tidak terpenuhi.

**R1. Entity `base/reading.go` (+ `base/reading_test.go`).** Murni, tanpa I/O. Import: stdlib, `toolkit`, `readfile` (tipe `readfile.Response`). Tulis tes dulu (bagian 6.1), lalu:

```go
package base

// ReadOutcome adalah hasil usecase read_file: ReadChunk atau NotFound.
type ReadOutcome interface {
	Result() toolkit.Result
}

const (
	reserveBytes       = 512   // cadangan untuk penanda dan selisih rune di bawah MCPMaxResponseBytes
	minReadLimit       = 1024
	defaultReadLimit   = 40000
	maxSimilar         = 5
)

// MaxReadLimit = max(1024, maxResponseBytes-512).
func MaxReadLimit(maxResponseBytes int) int
// DefaultReadLimit = min(40000, max).
func DefaultReadLimit(max int) int

type ReadChunk struct {
	Path       string
	Offset     int    // offset yang diminta caller (bukan posisi awal sebenarnya bila jatuh di tengah rune)
	Content    string
	SizeBytes  int
	TotalBytes int
	NextOffset int
	Truncated  bool
}

// NewReadChunk menyalin field dari resp; Offset dari argumen. Tidak mengubah Content.
func NewReadChunk(resp readfile.Response, offset int) ReadChunk

func (c ReadChunk) Text() string
func (c ReadChunk) Summary() map[string]any
func (c ReadChunk) Result() toolkit.Result

type NotFound struct {
	Path    string
	Similar []string // maks 5, urutan dari T05
}

func NewNotFound(path string, similar []string) NotFound // memotong ke 5 bila lebih, menyalin slice
func (n NotFound) Text() string
func (n NotFound) Summary() map[string]any
func (n NotFound) Result() toolkit.Result
```

Aturan `ReadChunk.Text()` (urutan evaluasi):
1. `TotalBytes == 0` menghasilkan `"[file kosong: <Path>]"`.
2. `Content == ""` dengan `TotalBytes > 0` (offset tepat di akhir file) menghasilkan `"[tidak ada isi pada offset=<Offset>; file berukuran <TotalBytes> byte]"`.
3. Selain itu `Content` apa adanya. Bila `Truncated`, tambahkan `"\n\n[terpotong: lanjutkan dengan offset=<NextOffset> dari total <TotalBytes> byte]"` (D10).
4. `Offset > 0` tanpa `Truncated` tidak menambah penanda apa pun.
Tidak ada trim, normalisasi newline, atau header. Teks isi file yang berakhir `\n` tetap dipertahankan sebelum penanda.

`ReadChunk.Summary()` = `{found:true, bytes:SizeBytes, total_bytes:TotalBytes, offset:Offset, next_offset:<NextOffset bila Truncated, -1 bila tidak>, truncated:Truncated}`. Tidak ada kunci yang memuat isi file.
`ReadChunk.Result()` = `toolkit.Result{Text: Text(), IsError:false, TargetType:"file", Target:Path, Summary:Summary(), Structured:nil}`.

Aturan `NotFound.Text()`:
```
File tidak ditemukan: <path>
Similar:
- <s1>
- <s2>            (maks 5, urutan input dipertahankan)
Gunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi.
```
Bila `Similar` kosong, baris `Similar:` dan baris `- ...` dihilangkan; baris pertama dan terakhir tetap. Baris dipisah `\n`, tanpa `\n` penutup.
`NotFound.Summary()` = `{found:false, similar_count:len(Similar), error_code:"not_found"}`. `Result()` = `{Text, IsError:true, TargetType:"file", Target:Path, Summary}`.

**R2. `readfile/params.go` (+ `params_test.go`).**

```go
package readfile

type rawArgs struct {
	Path   *string `json:"path"`
	Offset *int    `json:"offset"`
	Limit  *int    `json:"limit"`
}

var allowedArgs = []string{"path", "offset", "limit"}

type Params struct {
	Path   string // ternormalisasi
	Offset int
	Limit  int
}

// schema membangun InputSchema dengan angka maxLimit dari config (bagian 4).
func schema(maxLimit int) json.RawMessage

// Parse memvalidasi argumen. maxLimit = base.MaxReadLimit(cfg.MCPMaxResponseBytes).
func Parse(raw json.RawMessage, maxLimit int) (Params, *base.ArgError)
```

Urutan validasi di `Parse` (error pertama menang, urutan field sesuai schema, D6):
1. `base.DecodeStrict(raw, &r, allowedArgs)`. Menangani `project_id`/`projectId`/`project` (pesan `ditentukan oleh API key`), field tak dikenal, tipe salah (`offset harus integer`), JSON rusak.
2. `path`: `r.Path == nil` atau hasil trim kosong menghasilkan `ArgError{Field:"path", Reason:"wajib diisi", Hint:"Dapatkan path dengan list_files atau search_knowledge."}`, sehingga pesan akhir `Argumen tidak valid: path wajib diisi. Dapatkan path dengan list_files atau search_knowledge.` Bila `NormalizePath` mengembalikan error lain (folder `docs/`, terlalu panjang, `..`, karakter kontrol), teruskan apa adanya; untuk error "wajib diisi" dari `NormalizePath` isi `Hint` yang sama bila kosong. `docs/` menghasilkan `path menunjuk folder (diterima: docs/). Gunakan list_files dengan path=<tanpa slash akhir>.`
3. `offset`: `base.IntArg("offset", r.Offset, 0, 0, math.MaxInt32, "Pakai offset dari penanda [terpotong: ...] atau 0.")`. Pesan: `offset harus >= 0 (diterima: -1)`.
4. `limit`: `base.IntArg("limit", r.Limit, base.DefaultReadLimit(maxLimit), 1, maxLimit, "Maksimum <maxLimit> byte per panggilan; ambil sisanya dengan offset dari penanda.")`. Pesan memuat maksimum: `limit harus 1-101888 (diterima: 101889). Maksimum 101888 byte per panggilan; ...`. `limit: 0` ditolak.

`schema(maxLimit)`: `json.Marshal` dari `map[string]any` (kunci alfabetis supaya golden stabil):
```
type: object; additionalProperties: false; required: ["path"]
properties:
  path:   {type:string, minLength:1, maxLength:1024, description:"path file, mis. docs/guides/a.md"}
  offset: {type:integer, minimum:0, maximum:2147483647, default:0, description:"posisi byte awal; pakai nilai dari penanda terpotong"}
  limit:  {type:integer, minimum:1, maximum:<maxLimit>, default:<DefaultReadLimit(maxLimit)>, description:"jumlah byte maksimal per potongan"}
```
Teks ini harus identik dengan golden overview bagian 12.1.

**R3. `readfile/repository.go`.**

```go
package readfile

// Reader adalah port ke provider T05.
type Reader interface {
	Read(ctx context.Context, req readfileContract.Request) (readfileContract.Response, error)
}

type busReader struct{ b bus.Bus } // tipe bus mengikuti app.Deps.Bus

func newRepository(b <tipe app.Deps.Bus>) Reader

func (r busReader) Read(ctx context.Context, req readfileContract.Request) (readfileContract.Response, error) {
	return bus.CallTyped[readfileContract.Response](ctx, r.b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, req)
}
```
Import contract dengan alias, karena nama package feature `readfile` bentrok dengan `lib/contracts/project/knowledge/readfile`: `rf "grimoire/lib/contracts/project/knowledge/readfile"`. Request dikirim sebagai NILAI (bukan pointer), sesuai pola kontrak T05 (`bus.ErrBadRequestType` bila tipe salah). Tidak ada logika, tidak ada SQL. Error dikembalikan apa adanya (tanpa wrap tambahan, supaya `errors.Is(err, errs.ErrInvalid)` dan `context.DeadlineExceeded` tetap berlaku).

**R4. `readfile/usecase.go`.**

```go
type Usecase struct{ reader Reader }

func newUsecase(r Reader) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error) {
	resp, err := u.reader.Read(ctx, rf.Request{ProjectID: projectID, Path: p.Path, Offset: p.Offset, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	if resp.Found {
		return base.NewReadChunk(resp, p.Offset), nil
	}
	return base.NewNotFound(p.Path, resp.Similar), nil
}
```
Tidak ada format teks, tidak ada aturan rentang, tidak ada cek `projectID` (provider T05 memvalidasi UUID dan mengembalikan `ErrInvalid`).

**R5. `readfile/handler.go`.**

```go
func newHandler(uc *Usecase, maxLimit int) toolkit.Handler {
	return func(ctx context.Context, call toolkit.Call) (toolkit.Result, error) {
		p, argErr := Parse(call.Arguments, maxLimit)
		if argErr != nil {
			return base.ArgResult(argErr, "file", ""), nil // bus TIDAK dipanggil
		}
		out, err := uc.Execute(ctx, call.ProjectID, p)
		if err != nil {
			return base.BusResult(base.ToolReadFile, err,
				"Gunakan offset dari penanda terpotong atau 0.", "file", p.Path)
		}
		return out.Result(), nil
	}
}
```
- `BusResult` (overview 5.5, D13): `errs.ErrInvalid` (termasuk terbungkus) menjadi `Result{IsError:true, Text:"Permintaan ditolak: <pesan T05>. Gunakan offset dari penanda terpotong atau 0.", TargetType:"file", Target:path, Summary:{error_code:"invalid_request"}}` dengan err nil. Error lain (termasuk `bus.ErrNotRegistered`, `context.DeadlineExceeded`, `context.Canceled`) menjadi `fmt.Errorf("read_file: %w", err)`. Gateway menyembunyikan detail dan memetakan ke `internal`/`timeout`.
- Hasil `ArgError` memakai `Target ""` karena path belum tentu valid; `Summary` = `{error_code:"invalid_argument", field:<field>}` dari `ArgError.Summary()`.
- Handler tidak memformat teks sendiri dan tidak menyentuh bus langsung.

**R6. `readfile/init.go`.**

```go
// Init merakit tool read_file. Config dibaca sekali di sini.
func Init(d app.Deps) toolkit.Tool {
	maxLimit := base.MaxReadLimit(d.Config.MCPMaxResponseBytes)
	uc := newUsecase(newRepository(d.Bus))
	return toolkit.Tool{
		Name:        base.ToolReadFile, // "read_file"
		Title:       "Baca file knowledge",
		Description: "Baca isi satu file knowledge (read-only). path persis seperti dari list_files atau search_knowledge. File besar dikembalikan per potongan: bila ada penanda [terpotong: ... offset=N ...], panggil lagi dengan offset=N untuk lanjutan tanpa duplikat. offset dan limit dalam byte.",
		InputSchema: schema(maxLimit),
		Handler:     newHandler(uc, maxLimit),
	}
}
```
`MaxReadLimit` minimal 1024 menjamin `maximum >= default`-nya tetap konsisten: `DefaultReadLimit(1024) == 1024` (default diturunkan ke maksimum; golden config kecil memuat `default == maximum == 1024`). Untuk config default (102400) hasilnya `maximum=101888`, `default=40000`.

**R7. Verifikasi lokal.** Jalankan `cd backend && go build ./... && go vet ./... && go test ./internal/domain/discovery/internal/knowledgetools/readfile/... ./internal/domain/discovery/internal/knowledgetools/base/...`. Setelah tool lain selesai, `base_test.go` module (golden AC6, allowlist AC7) dan tes integrasi (bagian 6.4) dijalankan oleh F7/F8 overview. Usecase ini hanya memastikan kontribusinya benar: `Init` menghasilkan entri golden `read_file` di overview 12.1 persis.

Catatan wiring: `knowledgetools/base.go` (F4) dan `discovery/base.go` (F6) milik overview/foundation, TIDAK diubah di usecase ini. Tidak ada file di luar daftar bagian 5 yang boleh disentuh.

---

## 4. Migration

Tidak ada. Tidak ada tabel, kolom, atau index baru. Data dibaca lewat bus T05.

---

## 5. Changes (per file)

| File (relatif `knowledgetools/`) | Status | Isi |
|---|---|---|
| `base/reading.go` | baru | `ReadOutcome`, `ReadChunk`, `NotFound`, `MaxReadLimit`, `DefaultReadLimit`, konstruktor, `Text/Summary/Result` |
| `base/reading_test.go` | baru | bagian 6.1 |
| `readfile/init.go` | baru | `Init`, nama, judul, deskripsi (dibekukan), schema |
| `readfile/params.go` | baru | `rawArgs`, `Params`, `schema`, `Parse` |
| `readfile/repository.go` | baru | `Reader`, implementasi `bus.CallTyped` |
| `readfile/usecase.go` | baru | `Execute` |
| `readfile/handler.go` | baru | adapter `toolkit.Handler` |
| `readfile/params_test.go` | baru | bagian 6.2 |
| `readfile/usecase_test.go` | baru | bagian 6.3 |
| `readfile/handler_test.go` | baru | bagian 6.3 |
| `backend/cmd/internal/server/knowledgetools_integration_test.go` | tambah (file milik overview F8) | tiga tes `read_file` pada bagian 6.4 |

Tidak berubah: `base/{names,args,path,format,result}.go` (milik foundation; bila butuh helper baru tambahkan aditif beserta tes dan catat), `lib/constants`, `lib/contracts/**`, `infra/**`, `migrations/**`, `api/**`, `frontend/**`, `go.mod`.

Import yang diizinkan di file usecase ini (allowlist AC7 overview bagian 9): stdlib, `grimoire/internal/infra/app`, `grimoire/internal/infra/bus`, `grimoire/lib/constants`, `grimoire/lib/contracts/project/knowledge/readfile`, `grimoire/lib/contracts/discovery/mcpgateway/toolkit`, `grimoire/lib/common/errs`, package internal module. Dilarang `internal/domain/project/**` dan `.../mcpgateway/**`. Karena itu fake T05 di tes ditulis ulang di file tes (tidak mengimpor `SliceUTF8` milik T05).

---

## 6. Test Scenario

### 6.1 `base/reading_test.go` (tabel, tanpa I/O)

Helper tes `fakeT05(content string) func(offset, limit int) readfile.Response` mereplikasi `SliceUTF8` T05: offset maju ke awal rune, akhir mundur ke batas rune dengan maju minimal satu rune, `offset == len` mengembalikan kosong tanpa truncated.

- `TestT08_AC2_ChunksConcatenate`: dua file 200 KB (200*1024 byte), satu ASCII, satu campuran ASCII + `é` (2 byte) + `日本語` (3 byte) + emoji (4 byte) berselang-seling sehingga batas potongan sering jatuh di tengah rune. Loop: `offset=0`; `resp := fake(offset, 7000)`; `chunk := NewReadChunk(resp, offset)`; `text := chunk.Text()`; parse penanda dengan regex `\[terpotong: lanjutkan dengan offset=(\d+) dari total (\d+) byte\]$`; bila ada, buang suffix `"\n\n"+penanda`, tambahkan sisanya ke buffer, lalu `offset` = angka dari regex; bila tidak ada, tambahkan `text` utuh dan berhenti. Ulang dengan limit 1, 3, 4, 4096 pada file multibyte pendek (cek tidak ada loop tak hingga: batasi iterasi `len(content)+1`). Assert: buffer == isi asli byte-per-byte (tanpa duplikat maupun celah, `utf8.ValidString` benar), potongan terakhir tidak memuat penanda, `regex total == len(content)` di setiap potongan, `offset` dari penanda selalu naik (strictly increasing).
- `TestReadChunk_MarkerExactText`: `Truncated` menghasilkan akhir teks persis `"\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]"`; `Content` di depannya tidak dimodifikasi (mis. berakhir `"\n"` tetap berakhir `"\n"` sebelum `"\n\n["`).
- `TestReadChunk_NoMarkerWhenOffsetNonZeroAndNotTruncated`: `Offset 100`, `Truncated false` menghasilkan `Text() == Content`.
- `TestReadChunk_EmptyFile`: `TotalBytes 0` menghasilkan `[file kosong: docs/e.md]`, tanpa penanda; `IsError false`.
- `TestReadChunk_OffsetAtEnd`: `Content ""`, `TotalBytes 10`, `Offset 10` menghasilkan `[tidak ada isi pada offset=10; file berukuran 10 byte]`.
- `TestReadChunk_Summary`: kunci tepat `found, bytes, total_bytes, offset, next_offset, truncated`; `next_offset == -1` bila tidak terpotong; assert tidak ada nilai string di Summary yang berisi `Content` (isi sengaja `SECRET-123`), `Structured == nil`, `TargetType=="file"`, `Target==Path`.
- `TestT08_AC3_SimilarPaths`: tabel 0, 3, 7 saran. 0: teks tanpa `Similar:` dan tanpa `- `, tetap memuat `File tidak ditemukan: <path>`, `list_files`, `search_knowledge`. 3: tepat 3 baris `- `, urutan sama dengan input. 7: tepat 5 baris `- ` (5 pertama), `similar_count == 5`. Selalu `IsError true`, `error_code == "not_found"`, `found == false`, `TargetType "file"`.
- `TestNewNotFound_CopiesSlice`: memodifikasi slice input setelah konstruksi tidak mengubah `NotFound`.
- `TestMaxReadLimit`: `102400 -> 101888`, `1024 -> 1024`, `1100 -> 1024` (minimal 1024), `2048 -> 1536`. `TestDefaultReadLimit`: `101888 -> 40000`, `1024 -> 1024`, `40000 -> 40000`, `20000 -> 20000`.
- `TestReserveCoversMarkerAndRuneOvershoot` (menjaga catatan batas): untuk `MCPMaxResponseBytes` 102400 dan 1024, `ReadChunk` dengan `Content` sepanjang `MaxReadLimit + 3` byte (selisih rune maksimum), `NextOffset`/`TotalBytes` bernilai `2147483647` (penanda terpanjang), `Truncated true`: `len(Text()) <= MCPMaxResponseBytes`.

### 6.2 `readfile/params_test.go` (tabel)

Konstanta `maxLimit := base.MaxReadLimit(102400)` (= 101888) kecuali disebut.

| Kasus | Input | Ekspektasi |
|---|---|---|
| path wajib | `{}` | `Field=="path"`, pesan `Argumen tidak valid: path wajib diisi. Dapatkan path dengan list_files atau search_knowledge.` |
| path kosong | `{"path":""}` | sama dengan di atas |
| path spasi | `{"path":"   "}` | sama dengan di atas |
| slash depan | `{"path":"/docs/a.md"}` | `Path=="docs/a.md"` |
| trim | `{"path":"  docs/a.md  "}` | `Path=="docs/a.md"` |
| folder | `{"path":"docs/"}` | error, pesan memuat `menunjuk folder` dan `list_files` |
| segmen `..` | `{"path":"a/../b"}` | error memuat `..` |
| terlalu panjang | path 1025 byte | error memuat `1024` |
| default | `{"path":"a"}` | `Offset 0`, `Limit 40000` |
| offset negatif | `{"path":"a","offset":-1}` | `Field=="offset"`, memuat `(diterima: -1)` |
| offset bukan integer | `{"path":"a","offset":"1"}` dan `1.5` | `Field=="offset"`, `harus integer` |
| limit nol | `{"path":"a","limit":0}` | `Field=="limit"`, ditolak |
| limit maks | `limit == maxLimit` | diterima |
| limit lewat maks | `limit == maxLimit+1` | ditolak; pesan memuat `101888` dan `101889` |
| config kecil | `maxLimit = base.MaxReadLimit(1024)` (=1024); `{"path":"a"}` | `Limit == 1024` (default diturunkan); `limit:1025` ditolak dengan `1024` di pesan |
| `schema` config kecil | `schema(1024)` | `limit.maximum == 1024` dan `limit.default == 1024` |
| `schema` default | `schema(101888)` | `limit.maximum == 101888`, `default == 40000`, `required == ["path"]`, `additionalProperties == false`, tidak ada properti project |
| project_id (AC5) | `{"path":"a","project_id":"x"}`, `projectId`, `project` | error dengan pesan khusus memuat `ditentukan oleh API key`; `Params` kosong |
| field tak dikenal | `{"path":"a","foo":1}` | memuat `foo tidak dikenal` dan `path, offset, limit` |
| bentuk salah | `[]`, `1`, `{"path":"a"}{"x":1}` | `Field=="arguments"` |
| raw `null`/kosong | `null`, `""` | diperlakukan `{}` sehingga `path wajib diisi` |
| urutan error | `{"offset":-1,"limit":0}` | yang dilaporkan `path` (field pertama menurut schema) |

### 6.3 `readfile/usecase_test.go` dan `readfile/handler_test.go`

`usecase_test.go` (fake `Reader` yang merekam `Request`):
- `TestExecute_ForwardsRequest`: `Execute(ctx, "p-1", Params{Path:"docs/a.md", Offset:50, Limit:1000})` mengirim tepat `Request{ProjectID:"p-1", Path:"docs/a.md", Offset:50, Limit:1000}`, satu kali.
- `TestExecute_FoundBecomesChunk`: `Found:true` menghasilkan `base.ReadChunk` dengan `Offset==50` (dari Params, bukan dari resp), `Content/TotalBytes/NextOffset/Truncated` dari resp.
- `TestExecute_NotFoundBecomesNotFound`: `Found:false, Similar:[a,b]` menghasilkan `base.NotFound{Path:"docs/redme.md", Similar:[a,b]}` (Path dari Params karena resp.Path kosong).
- `TestExecute_PropagatesError`: reader mengembalikan `errs.ErrInvalid` terbungkus, `outcome == nil`, `errors.Is(err, errs.ErrInvalid)`; sama untuk `context.DeadlineExceeded`.

`handler_test.go` (registry bus asli `bus.NewRegistry()` dengan handler palsu didaftarkan pada `constants.BUS_PROJECT_KNOWLEDGE_READ_FILE`, tool dirakit lewat `Init(app.Deps{Config: cfg, Bus: reg})`):
- `TestHandler_Found_Chunk`: handler palsu mengembalikan potongan terpotong; `res.IsError false`, `res.Text` berakhir penanda, `res.Structured == nil`, `TargetType "file"`, `Target "docs/a.md"`; handler palsu menerima `Request.ProjectID == call.ProjectID` dan Limit eksplisit 40000 saat tidak diberikan.
- `TestHandler_NotFound`: `Found:false, Similar` menghasilkan `IsError true`, teks memuat `File tidak ditemukan` dan saran.
- `TestHandler_ErrInvalid_BecomesIsError`: handler palsu mengembalikan `fmt.Errorf("offset 9 di luar ukuran file (3 byte): %w", errs.ErrInvalid)`; hasil: err nil, `IsError true`, teks memuat `Permintaan ditolak:` dan diakhiri `Gunakan offset dari penanda terpotong atau 0.`, `Summary["error_code"] == "invalid_request"`.
- `TestHandler_OtherError_IsGoError`: handler palsu mengembalikan `errors.New("db down")` lalu `context.DeadlineExceeded`; hasil: err non-nil, `errors.Is` tetap berlaku, `Result` kosong. Registry kosong (tanpa handler) menghasilkan error Go dengan `errors.Is(err, bus.ErrNotRegistered)`.
- `TestHandler_SummaryHasNoContent`: isi file `SECRET-CONTENT-123`; marshal `Summary` ke JSON dan assert tidak memuat string itu; tidak ada nilai string di Summary kecuali `error_code`.
- `TestHandler_InvalidArgs_BusNotCalled` (AC5): tabel `{}`, `{"path":"docs/"}`, `{"path":"a","limit":0}`, `{"path":"a","project_id":"<B>"}`; handler palsu menghitung panggilan, harus 0; `IsError true`, `Summary["error_code"] == "invalid_argument"`.
- `TestHandler_ProjectFromCallOnly`: arguments memuat `project_id` ditolak; arguments sah dengan `call.ProjectID="A"` menghasilkan `Request.ProjectID=="A"`.
- `TestInit_Tool`: `Name=="read_file"`, `Title=="Baca file knowledge"`, deskripsi persis string pada R6, `InputSchema` JSON valid; config `MCPMaxResponseBytes=1024` menghasilkan `limit.maximum==1024`.

### 6.4 Integrasi (`backend/cmd/internal/server/knowledgetools_integration_test.go`, `//go:build integration`; fixture dan `mcpCall` dari overview 10.3)

| Tes | AC | Langkah | Ekspektasi |
|---|---|---|---|
| `TestT08_Int_AC2_ReadChunks` | 2 | upload `docs/big.txt` 200 KB (ASCII + multibyte 3/4 byte); `read_file {path:"docs/big.txt", limit:50000}`; ikuti `offset=(\d+)` dari penanda sampai tidak ada penanda | gabungan teks (penanda dibuang) == isi asli byte-per-byte; tidak ada respons dengan `_meta.truncated`; tidak ada `structuredContent`; `len(text) <= MCPMaxResponseBytes` |
| `TestT08_Int_AC3_SimilarPaths` | 3 | `read_file {path:"docs/redme.md"}` | `isError:true`; teks memuat `File tidak ditemukan: docs/redme.md`, `Similar:`, `- docs/readme.md`, `list_files` |
| `TestT08_Int_AC5_ProjectArgRejected` | 5 | key A, `read_file {path:"secret/b.md", project_id:<B>}` | `isError:true`, teks memuat `ditentukan oleh API key` dan tidak memuat `SECRET-B`; tanpa `project_id`, `read_file {path:"secret/b.md"}` menghasilkan `File tidak ditemukan` (di A) |
| `TestT08_Int_ReadOffsetBeyondEOF` | D13 | `read_file {path:"docs/a.md", offset:999999999}` | `isError:true`, teks memuat `Permintaan ditolak:` dan `Gunakan offset dari penanda terpotong atau 0.` |
| `TestT08_Int_AuditOneEntryPerCall` (bagian milik overview, `read_file` sukses) | D8 | satu `read_file {path:"docs/a.md"}` | satu entri `mcp.read_file`, `target_type=file`, `target=docs/a.md`, `Result` tanpa isi file |

Cakupan AC untuk usecase ini: AC2 (unit `TestT08_AC2_ChunksConcatenate` + integrasi), AC3 (unit + integrasi), AC5 (params + handler + integrasi), AC6 (entri golden `read_file` di `base_test.go` overview), AC7 (file usecase ini mematuhi allowlist).

---

## 7. E2E Scenarios

Positif:
1. File kecil: `read_file {path:"docs/a.md"}` menghasilkan isi utuh tanpa penanda, `isError:false`.
2. File 200 KB: `read_file {path:"docs/big.txt", limit:50000}` menghasilkan isi 50000 byte (atau sedikit berbeda di batas rune) diikuti `[terpotong: lanjutkan dengan offset=N dari total 204800 byte]`. Memanggil ulang dengan `offset=N` menyambung; ulangi sampai potongan tanpa penanda. Gabungan sama dengan file asli.
3. Default `limit` (tanpa argumen): potongan 40000 byte pertama untuk file besar.
4. Path berawalan slash `read_file {path:"/docs/a.md"}` dibaca sama dengan `docs/a.md`.
5. File kosong: `[file kosong: docs/empty.md]`, `isError:false`.
6. File multibyte dengan `limit` kecil: tiap penanda memberi offset di batas karakter; hasil gabungan valid UTF-8.
7. `offset` tepat di akhir file: `[tidak ada isi pada offset=N; file berukuran N byte]`.

Negatif:
1. `read_file {}`: `isError`, `Argumen tidak valid: path wajib diisi. Dapatkan path dengan list_files atau search_knowledge.`
2. `read_file {path:"docs/"}`: `isError`, menunjuk folder, saran `list_files`.
3. `read_file {path:"a", offset:-1}`: `isError`, `offset harus >= 0 (diterima: -1)`.
4. `read_file {path:"a", limit:0}` dan `limit:101889`: `isError`, pesan memuat angka maksimum 101888.
5. `read_file {path:"a", project_id:"<B>"}`: `isError`, `Argumen project_id tidak didukung: project ditentukan oleh API key...`. Bus tidak dipanggil, project B tidak terbaca.
6. `read_file {path:"docs/redme.md"}`: `isError` dengan `Similar` dan saran `list_files`/`search_knowledge`.
7. `read_file {path:"zzz.md"}` (tanpa kemiripan): `isError`, tanpa blok `Similar:`, tetap ada saran.
8. `offset` melewati file: `isError`, `Permintaan ditolak: ... Gunakan offset dari penanda terpotong atau 0.`, `error_code=invalid_request` di audit.
9. Provider T05 tidak terdaftar atau timeout: error Go, gateway mengembalikan `Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=<id>` atau teks timeout; detail tidak bocor.
10. `read_file {path:"a", offset:"10"}`: `isError`, `offset harus integer`.

---

## 8. Request/Response body

Semua contoh melalui `POST /mcp` (header `Authorization: Bearer <api-key>` dan `Mcp-Session-Id` mengikuti T07).

### 8.1 `tools/list` (entri `read_file`, config default `MCP_MAX_RESPONSE_BYTES=102400`)

```json
{
  "name": "read_file",
  "title": "Baca file knowledge",
  "description": "Baca isi satu file knowledge (read-only). path persis seperti dari list_files atau search_knowledge. File besar dikembalikan per potongan: bila ada penanda [terpotong: ... offset=N ...], panggil lagi dengan offset=N untuk lanjutan tanpa duplikat. offset dan limit dalam byte.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "path":   { "type": "string", "minLength": 1, "maxLength": 1024, "description": "path file, mis. docs/guides/a.md" },
      "offset": { "type": "integer", "minimum": 0, "maximum": 2147483647, "default": 0, "description": "posisi byte awal; pakai nilai dari penanda terpotong" },
      "limit":  { "type": "integer", "minimum": 1, "maximum": 101888, "default": 40000, "description": "jumlah byte maksimal per potongan" }
    },
    "required": ["path"],
    "additionalProperties": false
  },
  "annotations": { "readOnlyHint": true }
}
```
`annotations` ditambahkan gateway. Dengan `MCP_MAX_RESPONSE_BYTES=1024`: `limit.maximum` dan `limit.default` keduanya 1024.

### 8.2 Potongan pertama (terpotong)

Request:
```json
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/big.txt","limit":50000}}}
```
Bus request (repository): `readfile.Request{ProjectID:"<dari key>", Path:"docs/big.txt", Offset:0, Limit:50000}`.
Response:
```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "result": {
    "content": [{"type":"text","text":"<49998 byte isi file>\n\n[terpotong: lanjutkan dengan offset=49998 dari total 204800 byte]"}],
    "isError": false
  }
}
```
Tanpa `structuredContent` dan tanpa `_meta`. Audit (gateway): `action=mcp.read_file`, `target_type=file`, `target=docs/big.txt`, summary `{found:true,bytes:49998,total_bytes:204800,offset:0,next_offset:49998,truncated:true}`.

### 8.3 Lanjutan (potongan terakhir, tanpa penanda)

Request:
```json
{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/big.txt","offset":199998,"limit":50000}}}
```
Response:
```json
{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"<4802 byte sisa file>"}],"isError":false}}
```
Summary: `{found:true,bytes:4802,total_bytes:204800,offset:199998,next_offset:-1,truncated:false}`.

### 8.4 File kosong

```json
{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"[file kosong: docs/empty.md]"}],"isError":false}}
```

### 8.5 Tidak ditemukan (dengan saran)

Request: `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"docs/redme.md"}}}`
Response:
```json
{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"File tidak ditemukan: docs/redme.md\nSimilar:\n- docs/readme.md\n- readme.md\nGunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi."}],"isError":true}}
```
Summary: `{found:false,similar_count:2,error_code:"not_found"}`. Tanpa saran:
```json
{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"File tidak ditemukan: zzz.md\nGunakan list_files untuk melihat struktur folder atau search_knowledge untuk mencari berdasarkan isi."}],"isError":true}}
```

### 8.6 Format error

Path wajib (`invalid_argument`, field `path`):
```json
{"jsonrpc":"2.0","id":13,"result":{"content":[{"type":"text","text":"Argumen tidak valid: path wajib diisi. Dapatkan path dengan list_files atau search_knowledge."}],"isError":true}}
```
Path folder:
```json
{"jsonrpc":"2.0","id":14,"result":{"content":[{"type":"text","text":"Argumen tidak valid: path menunjuk folder (diterima: docs/). Gunakan list_files dengan path=<tanpa slash akhir>."}],"isError":true}}
```
Limit melewati maksimum:
```json
{"jsonrpc":"2.0","id":15,"result":{"content":[{"type":"text","text":"Argumen tidak valid: limit harus 1-101888 (diterima: 101889). Maksimum 101888 byte per panggilan; ambil sisanya dengan offset dari penanda."}],"isError":true}}
```
Offset negatif:
```json
{"jsonrpc":"2.0","id":16,"result":{"content":[{"type":"text","text":"Argumen tidak valid: offset harus >= 0 (diterima: -1). Pakai offset dari penanda [terpotong: ...] atau 0."}],"isError":true}}
```
Argumen project (AC5, `invalid_argument`, field `arguments` tidak dilaporkan; pesan khusus):
```json
{"jsonrpc":"2.0","id":17,"result":{"content":[{"type":"text","text":"Argumen project_id tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}],"isError":true}}
```
Ditolak provider (`invalid_request`):
```json
{"jsonrpc":"2.0","id":18,"result":{"content":[{"type":"text","text":"Permintaan ditolak: offset 999999999 di luar ukuran file (1229 byte): invalid argument. Gunakan offset dari penanda terpotong atau 0."}],"isError":true}}
```
Summary audit: `{error_code:"invalid_request"}`. (Pesan T05 dikutip apa adanya dari `err.Error()`; akhiran `: invalid argument` berasal dari pembungkus `errs.ErrInvalid`.)

Error internal (error Go dipetakan gateway T07):
```json
{"jsonrpc":"2.0","id":19,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=7f3c2a9e1b4d"}],"isError":true}}
```
Timeout (ctx deadline, dipetakan gateway):
```json
{"jsonrpc":"2.0","id":20,"result":{"content":[{"type":"text","text":"Tool read_file melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```

---

## 9. Catatan keputusan dan asumsi (untuk reviewer)

- Menyimpang dari tiket: satuan byte, bukan karakter (D2 overview). Penanda menulis "byte", bukan "karakter" seperti baris tiket.
- Asumsi baru (di luar spesifikasi yang diberikan): (a) kasus `Content == ""` dengan `TotalBytes > 0` mendapat pesan `[tidak ada isi pada offset=...]` supaya MCP tidak mengembalikan teks kosong; (b) `next_offset` di Summary bernilai `-1` bila tidak terpotong (kunci tetap ada agar bentuk Summary stabil); (c) pesan folder memakai kalimat dari `base.NormalizePath` (`path menunjuk folder (diterima: ...). Gunakan list_files dengan path=<tanpa slash akhir>.`), bukan menyalin literal `path=<...>` dari ringkasan usecase.
- Risiko yang sudah ditutup tes: loop tak hingga saat `limit` lebih kecil dari satu rune (T05 selalu maju minimal satu rune; fake tes mereplikasinya dan iterasi dibatasi), serta pemotongan gateway pada teks `read_file` (`TestReserveCoversMarkerAndRuneOvershoot`).
- Bila `base/args.go`/`path.go` foundation berbeda dari signature overview saat dikerjakan, sesuaikan di `params.go` saja dan laporkan selisihnya; jangan memodifikasi base bersama tanpa tes.
