# T05 project/knowledge — Usecase `busreadfile`

Plan usecase. Wajib mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (foundation); bila bertentangan, overview yang menang. Dokumen ini hanya merencanakan SATU usecase: handler bus `project.knowledge.read-file`.

Sumber: tiket `docs/tickets/T05-project-knowledge.md`, overview bagian 4.2 (path), 4.9 (`SliceUTF8`), 4.10 (`SimilarPaths`), 4.13 (error), 5.3 (kontrak), 5.6 (aturan provider bus), 5.7 (`store`), 10 (konsumsi), 11 (test).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | backend saja (tidak ada HTTP, tidak ada UI) |
| Domain | project |
| Module | knowledge |
| Feature (folder) | `busreadfile` |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | bus `project.knowledge.read-file` (konstanta `BUS_PROJECT_KNOWLEDGE_READ_FILE`) |
| Method | Call (request/response, sinkron) |
| Version | v1 |
| Provider / Consumer | T05 / T08 (lewat gateway T07; `ProjectID` berasal dari API key terverifikasi) |
| Kontrak | `backend/lib/contracts/project/knowledge/readfile` (`Request`, `Response`, `DefaultLimit = 65536`) |
| Audit | Tidak ada (rule 9.3; gateway T07 yang mengaudit pemanggilan MCP) |
| Otorisasi | Tidak memakai `base/access` (ProjectID sudah diverifikasi gateway) |
| AC | T05-AC8 |

Endpoint HTTP: tidak ada. Prasyarat foundation (F) sudah merge: kontrak `readfile`, konstanta bus, `base/` (path, slice, similar, errors, limits), `base/store` (`FindByPath`, `ListPaths`), migrasi `knowledge_files`, `Config.MCPMaxResponseBytes`. Verifikasi dengan `ls backend/lib/contracts/project/knowledge/readfile` dan `grep BUS_PROJECT_KNOWLEDGE_READ_FILE backend/lib/constants/bus.go`; bila belum ada, STOP dan selesaikan foundation lebih dulu (jangan membuat versi kontrak baru di sini).

---

## 2. Summary Plan

Handler bus read-only yang dipakai agent (lewat T08) untuk membaca isi satu file knowledge, dengan paginasi berbasis byte yang aman untuk UTF-8.

Alur usecase (urutan mengikat):

1. `bus.Expect[readfile.Request](req)` (di handler, sebelum apa pun). Tipe salah -> error membungkus `bus.ErrBadRequestType`, dikembalikan apa adanya.
2. `uuid.Parse(ProjectID)`; gagal -> `base.Invalid(base.ReasonInvalidProjectID)` (membungkus `errs.ErrInvalid`).
3. Path: `base.StripLeadingSlash(in)` (tepat satu `/`), lalu `base.NormalizePath`. Gagal -> `base.InvalidMsg(reason, fmt.Sprintf("path %q tidak valid: %s", in, reason.Message()))` dengan `in` = path asli dari request.
4. `store.FindByPath(ctx, projectID, path)`.
5. Tidak ketemu -> `store.ListPaths(ctx, projectID)` -> `base.SimilarPaths(path, all, 5)` -> `Response{Found:false, Similar}`, error nil (rule 6.7: tidak ada = bukan error).
6. Ketemu -> hitung limit efektif: `Limit <= 0` -> `readfile.DefaultLimit` (65536); lalu clamp ke `Limits.MaxReadBytes` (= `Config.MCPMaxResponseBytes`) bila `MaxReadBytes > 0` dan limit lebih besar.
7. `base.SliceUTF8(content, Offset, limit)`; `reason == invalid_offset` -> `base.InvalidMsg(reason, fmt.Sprintf("offset %d di luar ukuran file (%d byte)", Offset, total))`.
8. `Response{Found:true, Path, Content: part, SizeBytes: len(part), TotalBytes: len(content), NextOffset: end, Truncated}`.

Keamanan dan kebenaran:
- Semua SQL lewat `base/store` dengan `project_id = $1`; file project lain tidak pernah ditemukan dan tidak pernah muncul di `Similar` (`ListPaths` juga per project). Dibuktikan test isolasi (AC8).
- Isi file tidak di-log. Log hanya `project_id`, `feature=busreadfile`, `path`, dan jumlah byte.
- Tanpa audit, tanpa `check-access`, tanpa tx, tanpa panggilan bus keluar.
- Tidak ada keputusan baru; semua mengikuti overview 5.6. Rule bisnis (potong UTF-8, path mirip, normalisasi path) sudah ada di entity `base/` dan DIPAKAI, tidak diduplikasi di usecase. Usecase hanya orkestrasi: parse -> store -> entity -> response (rule 5.x).
- Tidak dipecah artifisial: satu folder, satu usecase, satu handler.

---

## 3. Plan (langkah berurutan)

Semua path relatif terhadap `backend/internal/domain/project/internal/knowledge/`.

### Langkah 0 — cek prasyarat
- Jalankan dua perintah verifikasi di bagian 1. Baca satu `Init` feature bus yang sudah ada (mis. `countfiles/init.go` bila sudah dibuat, atau handler bus T02/T03 di `main`) untuk menyalin pola persis `d.Bus.Register(...)` dan tipe fungsi handler yang dipakai `infra/bus` (jangan mengarang signature).

### Langkah 1 — `busreadfile/usecase.go`
Interface kecil yang dipenuhi `*store.Store` langsung (feature tanpa SQL khusus, jadi TIDAK ada `repository.go`; overview 3.2):

```go
package busreadfile

type files interface {
    FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
    ListPaths(ctx context.Context, projectID string) ([]string, error)
}

const similarMax = 5

type Usecase struct {
    files  files
    limits base.Limits
}

func NewUsecase(f files, l base.Limits) *Usecase

func (u *Usecase) Execute(ctx context.Context, req readfile.Request) (readfile.Response, error)
```

Isi `Execute` persis sesuai alur Summary langkah 2-8. Catatan implementasi:
- `uuid.Parse` hanya untuk validasi; `ProjectID` string asli diteruskan ke store (store sendiri menolak non-uuid tanpa query).
- Fungsi pembantu privat `effectiveLimit(requested int, maxRead int) int`: `if requested <= 0 { requested = readfile.DefaultLimit }; if maxRead > 0 && requested > maxRead { requested = maxRead }; return requested`.
- Error store dibungkus `fmt.Errorf("busreadfile: find file: %w", err)` dan `fmt.Errorf("busreadfile: list paths: %w", err)`.
- `Similar` tidak pernah berisi path target (dijamin `SimilarPaths`) dan maksimal 5.
- Offset negatif: ditolak oleh `SliceUTF8` (`invalid_offset`) hanya saat file ketemu; saat file tidak ada, offset diabaikan (Found=false, bukan error).

### Langkah 2 — `busreadfile/handler.go`
```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler

// Handle sesuai signature handler bus di main.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    r, err := bus.Expect[readfile.Request](req)
    if err != nil { return nil, err }
    return h.uc.Execute(ctx, r)
}
```
Hasil dikembalikan sebagai nilai `readfile.Response` (bukan pointer) agar `bus.CallTyped[readfile.Response]` konsisten dengan provider lain; ikuti konvensi yang sudah dipakai handler T02/T03 di `main`. Tanpa `params.go` (rule 6.9: feature bus hanya `Expect`).

### Langkah 3 — `busreadfile/init.go`
```go
func Init(d app.Deps) {
    uc := NewUsecase(store.New(d.DB), base.LimitsFrom(d.Config))
    d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, NewHandler(uc).Handle)
}
```
Registrasi ganda = panic (diharapkan; satu-satunya pendaftar endpoint ini). Gunakan bentuk argumen `Register` sesuai kode `main` (Langkah 0).

### Langkah 4 — daftarkan di `knowledge/base.go`
Tambah satu baris `busreadfile.Init(d)` (dan import) di `Register(d app.Deps)`. Hanya baris ini yang diubah di file itu; baris feature lain milik plan usecase masing-masing. Bila konflik merge, pertahankan keduanya.

### Langkah 5 — test (bagian 6) lalu jalankan
- `go test ./internal/domain/project/internal/knowledge/busreadfile/...`
- `go test -tags integration ./internal/domain/project/internal/knowledge/...` (skenario AC8 lintas usecase di `knowledge_integration_test.go` milik tahap W; di plan ini hanya didefinisikan skenario untuk `busreadfile`, lihat 6.3).
- `go build ./... && go vet ./... && go test ./internal/archtest/...` (pastikan tidak ada import terlarang: feature ini hanya meng-import `base`, `base/store`, `infra/app`, `infra/bus`, `lib/constants`, `lib/contracts/project/knowledge/readfile`, `uuid`).

---

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari migrasi foundation `backend/migrations/project/20261024090000000_create_knowledge_files.sql` (atau versi yang digeser bila bentrok, lihat overview bagian 6).

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `knowledge/busreadfile/init.go` | baru: `Init(d)` merakit store + usecase + handler, `Register` endpoint read-file |
| `knowledge/busreadfile/handler.go` | baru: `bus.Expect[readfile.Request]` lalu panggil usecase |
| `knowledge/busreadfile/usecase.go` | baru: interface `files`, `Usecase`, `Execute`, `effectiveLimit` |
| `knowledge/busreadfile/usecase_test.go` | baru: test unit dengan repository palsu |
| `knowledge/busreadfile/handler_test.go` | baru: test handler via registry bus nyata |
| `knowledge/busreadfile/busreadfile_integration_test.go` | baru (tag `integration`): DB nyata untuk read-file saja |
| `knowledge/base.go` | tambah satu baris `busreadfile.Init(d)` |

Tidak diubah (sudah disediakan foundation): `lib/constants/bus.go`, `lib/contracts/project/knowledge/readfile/readfile.go`, `knowledge/base/*`, `knowledge/base/store/store.go`, migrasi, config, README/katalog bus. Tidak ada perubahan OpenAPI (bukan endpoint HTTP). Tidak ada perubahan frontend.

---

## 6. Test Scenario

Rule 13: unit tanpa mock untuk entity (sudah dicakup test `base/`); usecase diuji dengan repository palsu; handler diuji lewat registry bus nyata.

### 6.1 Unit usecase (`usecase_test.go`, repository palsu in-memory, `Limits{MaxReadBytes: 1<<20}` kecuali disebut)

Palsu: `fakeFiles` menyimpan `map[projectID]map[path]content`; `FindByPath` dan `ListPaths` hanya melihat project yang diminta; mencatat jumlah pemanggilan.

Validasi dan error:
1. `ProjectID = "bukan-uuid"` -> `errors.Is(err, errs.ErrInvalid)`, `details.reason == invalid_project_id`; repository tidak dipanggil sama sekali.
2. `ProjectID = ""` -> sama seperti di atas.
3. Path tidak valid (table-driven): `""` path_empty; `"/"` path_empty (setelah strip); `"//a.md"` path_absolute (hanya satu `/` dibuang); `"../x.md"`, `"a/./b.md"` path_traversal; `"a//b.md"` path_empty_segment; `"a\x00b.md"` path_control_char; 513 rune path_too_long; segmen 256 rune segment_too_long; `"\xff.md"` invalid_encoding. Tiap kasus: `errors.Is(err, errs.ErrInvalid)`, `details.reason` benar, pesan memuat path asli dengan `%q` dan `reason.Message()`; repository tidak dipanggil.
4. Path memuat `/` di depan: `"/docs/a.md"` diperlakukan sama dengan `"docs/a.md"` dan `Response.Path == "docs/a.md"` (tanpa `/`).
5. Tipe request salah (di handler, 6.2) bukan di sini.

Tidak ditemukan dan Similar:
6. File tidak ada, project kosong -> `Found=false`, `Similar` kosong, err nil, `Content == ""`, `TotalBytes == 0`.
7. Typo `docs/auth/logn.md` dengan `docs/auth/login.md` ada -> `Found=false`, `Similar` memuat `docs/auth/login.md`.
8. Path dengan basename sama di folder lain (`other/login.md`) muncul di `Similar` (dan lebih dulu daripada yang hanya mirip jarak).
9. Banyak path mirip (> 5) -> `len(Similar) == 5`; target sendiri tidak pernah ada di `Similar`.
10. `Offset` negatif atau besar pada file yang tidak ada -> tetap `Found=false`, err nil (bukan `invalid_offset`).

File ketemu:
11. File kecil, `Offset=0, Limit=0` -> `Found`, `Content` utuh, `SizeBytes == TotalBytes == len(content)`, `NextOffset == len(content)`, `Truncated=false`.
12. Limit default: file 100000 byte ASCII, `Limit=0` -> `SizeBytes == 65536`, `NextOffset == 65536`, `Truncated=true`.
13. Clamp: `Limits.MaxReadBytes = 1000`, file 5000 byte ASCII, `Limit=0` -> `SizeBytes == 1000` (default 65536 diklamp); `Limit=999999` -> 1000; `Limit=10` -> 10.
14. `MaxReadBytes = 0` (tidak diatur) -> tidak ada clamp tambahan di atas default/limit user (jaring pengaman, tidak panik).
15. `Offset == len(content)` -> `Content == ""`, `SizeBytes == 0`, `NextOffset == len`, `Truncated=false`, `Found=true`.
16. `Offset = len+1` dan `Offset = -1` -> `errors.Is(err, errs.ErrInvalid)`, `details.reason == invalid_offset`, pesan menyebut offset dan ukuran file.
17. File kosong (0 byte), `Offset=0` -> `Found=true`, `Content ""`, `Truncated=false`; `Offset=1` -> `invalid_offset`.

UTF-8 (AC8; inti):
18. Isi aksen Indonesia/Latin (`"café résumé naïve"`), `Limit` memotong di tengah `é` -> `Content` tetap UTF-8 valid (`utf8.ValidString`), akhir mundur ke batas rune, `NextOffset == len(Content)` dari offset awal.
19. Emoji 4 byte (`"a😀b"`), `Limit=2` -> `Content == "a"`, `NextOffset == 1`; `Limit=1` pada awal emoji (Offset=1) -> tetap maju satu rune penuh (`Content == "😀"`, `NextOffset == 5`), tidak pernah `SizeBytes == 0` saat `Offset < total`.
20. `Offset` di tengah rune (Offset=2 pada `"é"` di posisi 1..3) -> maju ke awal rune berikutnya; `Content` valid UTF-8; tidak ada byte setengah rune.
21. Chaining: isi campuran ASCII + aksen + emoji + CJK (>= 5 KB), loop `Offset = NextOffset` dengan `Limit` kecil (1, 2, 3, 7, 100) sampai `Truncated=false` -> gabungan semua `Content` PERSIS sama dengan isi asli (`==`), tiap potongan valid UTF-8, loop selalu maju (jaga dengan batas iterasi agar test tidak menggantung bila ada bug).
22. Fuzz/properti kecil: 200 string acak multibyte dengan `Limit` acak 1..20: chaining menyusun ulang isi asli persis.
23. `TotalBytes` selalu = ukuran byte file utuh (bukan jumlah rune) pada semua potongan.

Isolasi (AC8):
24. Dua project A dan B, keduanya punya `same.md` dengan isi berbeda: baca dengan ProjectID A mengembalikan isi A, B mengembalikan isi B.
25. File hanya ada di project B; baca dari project A -> `Found=false` (BUKAN isi B). `Similar` untuk A tidak memuat path milik B walaupun mirip (mis. A punya `docs/x.md`, B punya `docs/login.md`, minta `docs/logn.md` dari A -> `Similar` tidak memuat `docs/login.md`).

Error infrastruktur:
26. `FindByPath` mengembalikan error -> `Execute` mengembalikan error terbungkus (`errors.Is` ke error asal), BUKAN `errs.ErrInvalid`, `ListPaths` tidak dipanggil.
27. `ListPaths` mengembalikan error (saat tidak ketemu) -> error terbungkus.
28. Context sudah dibatalkan -> error diteruskan dari repository palsu (tidak ditelan).

### 6.2 Test handler (`handler_test.go`, registry bus nyata + repository palsu atau DB palsu via `Init` terpisah)

1. `bus.CallTyped[readfile.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, readfile.Request{...})` pada handler yang terdaftar -> respons sesuai usecase.
2. Request bertipe salah (`CallTyped` dengan `struct{}` atau string) -> `errors.Is(err, bus.ErrBadRequestType)`; usecase tidak dipanggil.
3. Pemanggil tanpa handler terdaftar -> `bus.ErrNotRegistered` (perilaku infra, memastikan konstanta endpoint dipakai benar: gunakan konstanta, bukan literal string).
4. Handler mengembalikan tipe `readfile.Response` yang bisa di-decode `CallTyped` (tidak ada salah tipe pointer/nilai).

### 6.3 Integrasi `busreadfile_integration_test.go` (tag `integration`, `db.TestDB`)

Fixture dimasukkan lewat SQL langsung (dua project di tabel `projects` + `knowledge_files` dengan `content_hash` 32 byte dan `size_bytes` benar).
1. Baca file ASCII utuh dan terpotong dari Postgres nyata; `TotalBytes` = `size_bytes` kolom.
2. Isi multibyte (aksen Indonesia, emoji, CJK) tersimpan di Postgres lalu dibaca: chaining `NextOffset` menyusun ulang isi persis (AC8). Verifikasi bahwa tidak ada konversi collation/encoding yang mengubah byte.
3. File 1 MiB penuh dibaca dengan `Limit=0`: potongan pertama 65536 byte (atau clamp `MCPMaxResponseBytes` bila lebih kecil), `Truncated=true`, chaining lengkap selesai dalam jumlah iterasi yang wajar.
4. Isolasi dua project: file `same.md` di keduanya; kata/jalur unik masing-masing; `Found=false` untuk path milik project lain; `Similar` tidak membocorkan path project lain.
5. `ProjectID` uuid valid tetapi project tidak punya file -> `Found=false`, `Similar` kosong.
6. Path peka huruf besar/kecil: `Docs/A.md` tidak ketemu bila tersimpan `docs/a.md`, namun `Similar` menyarankan `docs/a.md` (similar case-insensitive).
7. Path dengan `_`/`%` literal (`a_b/x.md`) dibaca persis (equality, bukan LIKE) dan `axb/x.md` tidak ikut.
8. Tidak ada entri audit yang ditulis (handler `record` palsu terpasang: hitungan 0) dan tidak ada panggilan bus keluar (registry hanya memuat handler read-file; bila ada panggilan keluar, test gagal dengan `ErrNotRegistered`).
9. (Dicatat untuk tahap W) skenario AC8 lintas `list-files`/`read-file`/`search` di `knowledge_integration_test.go` memakai handler ini; tidak ditulis ulang di sini.

### 6.4 Kriteria selesai
- Semua test 6.1 dan 6.2 lulus dengan `go test`; 6.3 lulus dengan `-tags integration`.
- `go vet`, `internal/archtest` lulus; tidak ada tag `db`/SQL di `busreadfile/` (repository satu-satunya SQL = `base/store`).
- Tidak ada log isi file (grep `slog`/`Log` di feature ini hanya menyertakan field metadata).

---

## 7. E2E Scenarios

Handler ini tidak punya endpoint HTTP maupun UI, jadi tidak ada skenario Playwright untuk feature ini sendiri. Konsumen akhir adalah T08 (MCP `read_file` lewat gateway T07); E2E MCP end-to-end dimiliki T08. Skenario berikut dijalankan sebagai test integrasi lewat bus (bukan browser) dan menjadi kontrak yang dipakai T08.

Positif:
1. Agent membaca `docs/auth/login.md` (ada awalan `/` dari model) -> `Found=true`, isi lengkap, `Truncated=false`.
2. Agent membaca file besar (120000 byte) tanpa Limit -> potongan 65536 byte, `Truncated=true`; membaca lanjutan dengan `Offset=NextOffset` sampai selesai; gabungan = isi asli persis, termasuk aksen/emoji.
3. Agent salah ketik path (`docs/auth/logn.md`) -> `Found=false` dengan `Similar: ["docs/auth/login.md"]`; agent mengulang dengan path benar -> `Found=true`.
4. Setelah `updatefile` (HTTP, T05) mengubah isi, pembacaan berikutnya mengembalikan isi terbaru (tidak ada cache).
5. Setelah `movefile`, path lama -> `Found=false` (Similar menyarankan path baru bila mirip), path baru -> `Found=true`.

Negatif:
1. `ProjectID` bukan uuid -> error `errs.ErrInvalid` `invalid_project_id`.
2. Path `../etc/passwd` atau `.git/config` bermuatan traversal -> error `errs.ErrInvalid` dengan alasan yang memandu agent memperbaiki path.
3. `Offset` melebihi ukuran file -> error `errs.ErrInvalid` `invalid_offset` (agent tahu harus berhenti/mengoreksi).
4. File project lain (path milik project B diminta dengan key project A) -> `Found=false`, tanpa kebocoran isi maupun path di `Similar`.
5. Tipe request salah -> `bus.ErrBadRequestType`.
6. Database tidak tersedia -> error terbungkus (konsumen T08 memetakannya ke error server generik; tidak ada isi file di pesan).

---

## 8. Request/Response body

Bus bersifat in-process Go (bukan JSON di wire). Contoh di bawah memakai notasi Go; bila T08 menyerialisasi hasilnya ke JSON, bentuk error mengikuti format error seragam (rule 11.1).

### Request

```go
readfile.Request{
    ProjectID: "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f",
    Path:      "/docs/auth/login.md", // satu "/" di depan ditoleransi
    Offset:    0,                      // byte
    Limit:     0,                      // 0 -> 65536, maks Config.MCPMaxResponseBytes
}
```

### Response: ketemu, terpotong

```go
readfile.Response{
    Found:      true,
    Path:       "docs/auth/login.md",
    Content:    "# Login\n\nAlur login ... (65533 byte)",
    SizeBytes:  65533, // len(Content) dalam byte; < 65536 karena akhir mundur ke batas rune
    TotalBytes: 120000,
    NextOffset: 65533,
    Truncated:  true,
    Similar:    nil,
}
```

Membaca lanjutan:

```go
readfile.Request{ProjectID: pid, Path: "docs/auth/login.md", Offset: 65533, Limit: 65536}
// -> Response{Found: true, Path: "docs/auth/login.md", Content: "...", SizeBytes: 54467,
//             TotalBytes: 120000, NextOffset: 120000, Truncated: false}
```

### Response: ketemu, utuh

```go
readfile.Response{Found: true, Path: "docs/a.md", Content: "# A\n", SizeBytes: 4, TotalBytes: 4, NextOffset: 4, Truncated: false}
```

### Response: offset tepat di akhir file

```go
readfile.Response{Found: true, Path: "docs/a.md", Content: "", SizeBytes: 0, TotalBytes: 4, NextOffset: 4, Truncated: false}
```

### Response: offset di tengah karakter (maju ke karakter berikutnya)

Isi `"café!"` (`é` = byte 3..4, total 6 byte), `Offset: 4` (tengah `é`):

```go
readfile.Response{Found: true, Path: "c.md", Content: "!", SizeBytes: 1, TotalBytes: 6, NextOffset: 6, Truncated: false}
```

### Response: tidak ketemu (bukan error)

```go
readfile.Response{
    Found:   false,
    Similar: []string{"docs/auth/login.md"},
}
// err == nil
```

### Error: path tidak valid (membungkus `errs.ErrInvalid`, `errors.Is(err, errs.ErrInvalid) == true`)

```go
// Request{ProjectID: pid, Path: "../x.md"}
// err.Error(): path "../x.md" tidak valid: path tidak boleh memuat segmen . atau ..
```

Bila diserialisasi sebagai respons error (400):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "path \"../x.md\" tidak valid: path tidak boleh memuat segmen . atau ..",
    "details": { "reason": "path_traversal" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

### Error: offset di luar file

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "offset 999 di luar ukuran file (120 byte)",
    "details": { "reason": "invalid_offset" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

### Error: ProjectID bukan uuid

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "ProjectID tidak valid",
    "details": { "reason": "invalid_project_id" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

### Error: tipe request salah

```go
_, err := bus.CallTyped[readfile.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, "bukan-request")
// errors.Is(err, bus.ErrBadRequestType) == true
```

### Error: kegagalan infrastruktur

```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server" },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

(Kode dan pesan generik mengikuti pemetaan `respond.WriteError` T00; isi error internal tidak pernah menyertakan isi file.)
