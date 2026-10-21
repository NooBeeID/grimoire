# T05 project/knowledge — Usecase Plan: `bussearch`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (selanjutnya "overview"). Bila bertentangan, overview menang. Plan ini hanya untuk satu usecase: handler bus `project.knowledge.search`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | backend saja (tidak ada HTTP, tidak ada frontend) |
| Domain | project |
| Module | knowledge |
| Feature | `bussearch` |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | bus `project.knowledge.search` (konstanta `BUS_PROJECT_KNOWLEDGE_SEARCH`) |
| Method | Call (request/response, `bus.CallTyped[search.Response]`) |
| Version | v1 |
| Provider / Consumer | T05 / T08 (lewat gateway T07; ProjectID dari key terverifikasi) |
| Kontrak | `backend/lib/contracts/project/knowledge/search` |
| Audit | tidak ada (rule 9.3; gateway T07 yang mengaudit) |
| Otorisasi | tidak ada check-access (ProjectID sudah diverifikasi gateway) |
| Bergantung pada | foundation F selesai: `base/` (query, snippet, path, errors), `base/store` (`Search`), kontrak + konstanta bus |

## 2. Summary Plan

`bussearch` memberi agent (T08) pencarian full-text atas knowledge satu project. Usecase hanya orkestrasi:

1. `bus.Expect[search.Request]` (tipe salah -> `bus.ErrBadRequestType`).
2. `uuid.Parse(ProjectID)`; gagal -> `base.Invalid(ReasonInvalidProjectID)` (membungkus `errs.ErrInvalid`).
3. `base.ParseQuery(Query)`; tanpa term -> `Response{Items: nil, Total: 0}`, err nil, tanpa query DB.
4. Prefix opsional: `base.StripLeadingSlash` (tepat satu `/`), lalu `base.NormalizePrefix`; gagal -> `base.InvalidMsg(reason, ...)`. `LikePrefix`.
5. `Limit = base.ClampLimit(req.Limit, search.DefaultLimit(20), search.MaxLimit(50))`.
6. `store.Search(ctx, projectID, tsq, likePrefix, limit)` — SQL yang SAMA dengan HTTP `search` (satu implementasi di `base/store`, tidak diduplikasi).
7. Map ke `search.Item{Path, Score, Snippet: base.ToMarkdownBold(hit.Headline)}`, `Total` dari store.

Rule bisnis (pembersihan kueri, clamp, markdown bold) tinggal di `base/`; SQL hanya di `base/store`. Tidak ada `repository.go` khusus: store dipakai lewat interface kecil di usecase (rule 5.6; overview 3.2 mengizinkan feature tanpa SQL khusus). Isolasi antar project dijamin `project_id = $1` pada query store (AC8). Isi file tidak pernah di-log.

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai (`base/query.go`, `base/snippet.go`, `base/path.go`, `base/errors.go`, `base/store`, kontrak `search`, konstanta bus). Cek: `ls backend/lib/contracts/project/knowledge/search` dan `grep BUS_PROJECT_KNOWLEDGE_SEARCH backend/lib/constants/bus.go`. Bila belum ada, STOP dan selesaikan F dulu (jangan membuat versi sendiri).

1. **Folder feature** `backend/internal/domain/project/internal/knowledge/bussearch/` berisi `init.go`, `handler.go`, `usecase.go`, `usecase_test.go`, `handler_test.go`. Tanpa `params.go` dan tanpa `repository.go`.
2. **`usecase.go`** (package `bussearch`):
   ```go
   type searcher interface {
       Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error)
   }
   type Usecase struct{ store searcher }
   func NewUsecase(s searcher) *Usecase
   func (u *Usecase) Execute(ctx context.Context, req search.Request) (search.Response, error)
   ```
   Urutan `Execute`:
   - `uuid.Parse(req.ProjectID)` gagal -> `base.Invalid(base.ReasonInvalidProjectID)`. (Dilakukan SEBELUM ParseQuery agar project id salah selalu error walau query kosong.)
   - `tsq, ok := base.ParseQuery(req.Query)`; `!ok` -> `return search.Response{}, nil`.
   - `prefix := base.StripLeadingSlash(req.Prefix)`; `prefix, reason := base.NormalizePrefix(prefix)`; `reason != ""` -> `base.InvalidMsg(reason, fmt.Sprintf("prefix %q tidak valid: %s", req.Prefix, reason.Message()))`.
   - `limit := base.ClampLimit(req.Limit, search.DefaultLimit, search.MaxLimit)`.
   - `hits, total, err := u.store.Search(ctx, req.ProjectID, tsq, base.LikePrefix(prefix), limit)`; err -> `fmt.Errorf("bussearch: search: %w", err)`.
   - Bangun `items := make([]search.Item, 0, len(hits))` (hit kosong -> `Items` kosong/nil, boleh); `Snippet: base.ToMarkdownBold(h.Headline)`. Kembalikan `search.Response{Items: items, Total: total}`.
   Catatan `StripLeadingSlash` hanya satu `/`: `//docs` menjadi `/docs` lalu ditolak `path_absolute` (sesuai overview 5.6).
3. **`handler.go`**: `func Handle(uc *Usecase) bus.Handler` (signature mengikuti `bus.Registry.Register` di `main`; cek `backend/internal/infra/bus`). Isi: `r, err := bus.Expect[search.Request](req)`; err -> return err apa adanya; `uc.Execute(ctx, r)`; kembalikan `search.Response` (nilai, bukan pointer, agar `CallTyped[search.Response]` cocok). Tanpa log isi query berlebihan: boleh log debug `project_id`, `feature=bussearch`, jumlah hasil; DILARANG me-log snippet/isi.
4. **`init.go`**: `func Init(d app.Deps)`: `st := store.New(d.DB)`; `uc := NewUsecase(st)`; `d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_SEARCH, Handle(uc))`. Registrasi ganda panic, jadi hanya satu kali.
5. **`knowledge/base.go`**: tambah satu baris `bussearch.Init(d)` di `Register` (jangan menyentuh baris `Init` feature lain; tiap usecase menambah barisnya sendiri).
6. **Tidak mengubah**: kontrak, konstanta, `base/*`, `base/store` (sudah dari F). Bila `store.Search` ternyata belum menerima `limit`/`likePrefix` seperti overview 5.7, perbaiki di F, bukan di feature ini.
7. **Test** (bagian 6) lalu `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/bussearch/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/...`.

Kontrak bus (tidak diubah, rujukan):

```go
// lib/contracts/project/knowledge/search
const (DefaultLimit = 20; MaxLimit = 50)
type Request  struct{ ProjectID, Query, Prefix string; Limit int }
type Item     struct{ Path string; Score float64; Snippet string }
type Response struct{ Items []Item; Total int }
```

Perilaku error:

| Kondisi | Hasil |
|---|---|
| Request bukan `search.Request` | error membungkus `bus.ErrBadRequestType` |
| ProjectID bukan uuid | `*errs.Error` 400, `details.reason=invalid_project_id`, `errors.Is(err, errs.ErrInvalid)` |
| Prefix tidak valid (`../x`, `a//b`, `//a`) | `*errs.Error` 400 dengan pesan memandu, `errors.Is(err, errs.ErrInvalid)` |
| Query tanpa kata / hanya simbol | `Response{Items: nil, Total: 0}`, err nil |
| Query berkarakter aneh (`"`, `&`, `:`, `or`, `-`, `!`) | tidak error (selalu dibungkus kutip oleh ParseQuery) |
| Tidak ada hasil | `Items` kosong, `Total: 0`, err nil |
| Gagal DB | error terbungkus (infra, bukan `errs.Error`) |

## 4. Migration

Tidak ada. Tabel `knowledge_files` (kolom `search` generated + indeks GIN + `text_pattern_ops`) dibuat oleh foundation: `backend/migrations/project/<VERSION>_create_knowledge_files.sql`.

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/bussearch/init.go` | baru: rakit store -> usecase, `Bus.Register(BUS_PROJECT_KNOWLEDGE_SEARCH, ...)` |
| `.../bussearch/handler.go` | baru: `bus.Expect` lalu panggil usecase |
| `.../bussearch/usecase.go` | baru: orkestrasi (7 langkah di atas) |
| `.../bussearch/usecase_test.go` | baru: unit dengan store palsu |
| `.../bussearch/handler_test.go` | baru: tipe salah lewat registry bus nyata |
| `.../bussearch/bussearch_integration_test.go` | baru (tag `integration`): DB nyata, AC8 dan lainnya |
| `backend/internal/domain/project/internal/knowledge/base.go` | tambah `bussearch.Init(d)` |

Tidak ada perubahan di `api/` (bukan HTTP), `docs/tickets/README.md` (katalog bus sudah diperbarui di F), dan migrasi.

## 6. Test Scenario

### Unit `usecase_test.go` (store palsu, tabel-driven)

- Query `login`: store dipanggil dengan `tsq=``"login"``, `likePrefix="%"`, `limit=20`; hasil dipetakan, `Snippet` = `Alur **login** memakai...` dari headline `Alur \x01login\x02 memakai...`; `Total` diteruskan dari store (mis. 7 walau Items 2).
- Query kosong, `"   "`, `"&&&"`, `"\"\"\""`: `Response{Items: nil, Total: 0}`, err nil, store TIDAK dipanggil (counter 0).
- ProjectID `""` dan `"bukan-uuid"`: `errors.Is(err, errs.ErrInvalid)`, `details.reason=invalid_project_id`, store tidak dipanggil; berlaku juga saat Query kosong.
- Prefix: `docs` -> `likePrefix="docs/%"`; `/docs` -> sama (satu slash dibuang); `docs/` -> sama; `""` -> `%`; `a_b` -> `a\_b/%`; `//docs`, `../x`, `a//b` -> ErrInvalid, store tidak dipanggil.
- Limit clamp: 0 -> 20; -5 -> 20; 1 -> 1; 50 -> 50; 51 dan 1000 -> 50 (assert argumen `limit` yang diterima store).
- Karakter aneh: `"unclosed phrase`, `a & b : c or -d !e` -> tidak error, `tsq` seperti kontrak ParseQuery.
- Store mengembalikan error -> `Execute` error terbungkus (`errors.Is` ke error asal), bukan `*errs.Error`.
- Store mengembalikan 0 hit -> `Items` kosong, `Total 0`, err nil.

### `handler_test.go` (registry bus nyata)

- `bus.CallTyped[search.Response]` dengan request benar -> handler jalan (store palsu lewat `NewUsecase`).
- Wrong type: `registry.Call(ctx, BUS_PROJECT_KNOWLEDGE_SEARCH, "string")` dan `listfiles.Request{}` -> `errors.Is(err, bus.ErrBadRequestType)`, usecase tidak dijalankan.
- Double `Init` pada registry yang sama panic (jaminan registrasi satu kali), opsional.

### Integrasi `bussearch_integration_test.go` (tag `integration`, `db.TestDB`)

Fixture: dua project A dan B (baris `projects` minimal, atau via fixture T03) dan file diisi lewat SQL test. Handler didaftarkan lewat `Init(deps)`; panggilan lewat `bus.CallTyped`.

- **AC8 isolasi**: kata unik `zebraunik` hanya ada di file project B; cari di project A -> `Total 0`, `Items` kosong. Kata unik `kudaunik` hanya di A; cari di B -> 0. File `same.md` ada di keduanya; hasil A hanya memuat konten A (cek Snippet).
- **Karakter aneh tidak error**: tiap query berikut menghasilkan err nil: `"`, `&`, `:`, `or`, `-`, `!`, `"unclosed`, `a & b | c`, `'; DROP TABLE knowledge_files; --`, `()<>`, dan emoji. Setelahnya tabel masih ada (`count(*)` benar).
- **Prefix filter**: file `docs/auth/login.md`, `docs2/login.md`, `docs.md`, `a_b/login.md`, `axb/login.md` semua memuat `login`. Prefix `docs` -> hanya `docs/auth/login.md`; prefix `/docs` sama; prefix `a_b` -> hanya `a_b/login.md` (bukan `axb`); prefix kosong -> semua; prefix `../x` -> ErrInvalid.
- **Limit clamp**: 60 file memuat kata sama; Limit 0 -> 20 item; Limit 5 -> 5; Limit 500 -> 50; `Total` tetap 60 pada semua kasus.
- **Peringkat dan snippet**: `login.md` (path memuat login) peringkat di atas `guide.md` panjang yang menyebut `login` sekali; `Snippet` berisi `**login**` dan tidak mengandung `\x01`/`\x02`; isi file berisi karakter `\x01` tidak merusak snippet.
- **Konsistensi dengan HTTP**: untuk query + prefix + limit yang sama, urutan path dan `Total` identik dengan `store.Search` langsung (membuktikan satu query dipakai bersama).
- **Wrong type** end-to-end lewat bus nyata -> `ErrBadRequestType`.
- **Tanpa audit**: handler `core.audit.record` palsu terdaftar menghitung panggilan; setelah beberapa pencarian jumlahnya 0.
- **Tanpa check-access**: tidak ada handler `project.management.check-access` terdaftar di test ini dan pencarian tetap berhasil (membuktikan tidak ada dependensi).
- Log tidak memuat isi file (tangkap `slog` handler test, cari kata unik isi: tidak ada).

## 7. E2E Scenarios

Handler ini hanya jalur bus (tidak ada route HTTP/UI sendiri); E2E nyata baru ada setelah T07/T08 (MCP). Skenario yang dicatat untuk saat itu, plus yang sudah bisa dijalankan sekarang lewat test integrasi:

Positif:
1. Agent memanggil tool search dengan `query=login` pada project miliknya -> hasil berurut skor, snippet ber-`**term**`, `total` benar.
2. Agent memakai `prefix=/docs/auth` (dengan slash depan) -> hasil dibatasi folder tersebut.
3. Agent memanggil dengan `limit=500` -> maksimal 50 item dikembalikan.
4. Frasa `"reset password"` mencocokkan dokumen yang memuat kedua kata berurutan.
5. Kueri tanpa kata (`"???"`) -> hasil kosong, bukan error.

Negatif:
1. Key project A mencari kata yang hanya ada di project B -> 0 hasil (AC8).
2. Kueri `" & : or -` atau `"unclosed` -> tidak error.
3. Prefix `../secret` -> error 400-setara berpesan memandu (`prefix "../secret" tidak valid: path tidak boleh memuat segmen . atau ..`).
4. Tipe request salah dari consumer (bug internal) -> `ErrBadRequestType`, tidak panic.
5. Tidak ada entri audit yang ditulis oleh provider (gateway T07 yang mengaudit pemanggilan).

## 8. Request/Response body

Bus bukan JSON di wire; berikut contoh pemanggilan Go dan nilai yang dikembalikan.

Berhasil:

```go
res, err := bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f", Query: "login", Prefix: "/docs", Limit: 5})
// err == nil
// res == search.Response{
//   Items: []search.Item{
//     {Path: "docs/auth/login.md", Score: 0.83, Snippet: "Alur **login** memakai email dan password"},
//     {Path: "docs/guide.md",      Score: 0.04, Snippet: "… setelah **login** pengguna diarahkan … "},
//   },
//   Total: 2,
// }
```

Query tanpa kata (tanpa akses DB):

```go
res, err := bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: pid, Query: `" & : or -`})
// res == search.Response{Items: nil, Total: 0}, err == nil
```

Isolasi (kata hanya ada di project lain):

```go
res, err := bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: pidA, Query: "zebraunik"})
// res == search.Response{Items: []search.Item{}, Total: 0}, err == nil
```

Error (semua membungkus `errs.ErrInvalid` kecuali tipe salah):

```go
// ProjectID salah
_, err := bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: "bukan-uuid", Query: "login"})
// errors.Is(err, errs.ErrInvalid) == true
// errs.Error{Code: INVALID_ARGUMENT, Message: "ProjectID tidak valid", Details: {"reason": "invalid_project_id"}}

// Prefix salah
_, err = bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: pid, Query: "login", Prefix: "../x"})
// errs.Error{Code: INVALID_ARGUMENT,
//   Message: `prefix "../x" tidak valid: path tidak boleh memuat segmen . atau ..`,
//   Details: {"reason": "path_traversal"}}

// Tipe request salah
_, err = b.Call(ctx, constants.BUS_PROJECT_KNOWLEDGE_SEARCH, "bukan request")
// errors.Is(err, bus.ErrBadRequestType) == true
```

Bila consumer (T08/gateway) menerjemahkan error ini ke HTTP/MCP, bentuk error seragam rule 11.1 adalah:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "prefix \"../x\" tidak valid: path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```
