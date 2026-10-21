# T05 project/knowledge — Usecase Plan: `buslistfiles`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (foundation). Bila bertentangan, overview menang.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (usecase ini hanya backend; tidak ada UI, tidak ada route HTTP) |
| Domain | project |
| Module | knowledge |
| Feature | `buslistfiles` (gelombang C, U11) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | bus `project.knowledge.list-files` (konstanta `BUS_PROJECT_KNOWLEDGE_LIST_FILES`) |
| Method | Call (bus, bukan HTTP) |
| Version | v1 (kontrak bus baru, sisi provider) |
| Kontrak | `backend/lib/contracts/project/knowledge/listfiles` (dibuat di foundation F langkah 5) |
| Consumer | T08 (lewat gateway T07; `ProjectID` berasal dari API key terverifikasi) |
| Auth | tidak ada check-access: `ProjectID` sudah diverifikasi gateway |
| Audit | tidak ada (gateway T07 yang mengaudit; rule 9.3) |
| AC | AC8 |

Bus yang dikonsumsi: tidak ada. Hanya menyediakan `project.knowledge.list-files`.

## 2. Summary Plan

Handler bus read-only untuk agent: mendaftar file knowledge satu project, rekursif (semua file di bawah prefix) atau satu level (file langsung + nama subfolder langsung). Alur usecase (hanya orkestrasi, rule di `base/`):

1. `handler.go`: `bus.Expect[listfiles.Request](req)`; tipe salah -> kembalikan error apa adanya (membungkus `bus.ErrBadRequestType`).
2. `uuid.Parse(ProjectID)` gagal -> `base.Invalid(base.ReasonInvalidProjectID)` (membungkus `errs.ErrInvalid`).
3. `Prefix`: `base.StripLeadingSlash` (tepat satu `/`), lalu `base.NormalizePrefix`; gagal -> `base.InvalidMsg(reason, fmt.Sprintf("prefix %q tidak valid: %s", in, reason.Message()))`. Pesan memandu agent.
4. `Limit`: `<= 0` -> `listfiles.DefaultLimit` (100); `> listfiles.MaxLimit` -> 500. `Offset < 0` -> `base.InvalidMsg(base.ReasonInvalidOffset, "offset tidak boleh negatif")`.
5. `Recursive == true`: repository menjalankan `SELECT path, size_bytes, updated_at ... ORDER BY path COLLATE "C" LIMIT/OFFSET` ditambah `COUNT(*)` untuk `Total`. `Folders` = nil.
6. `Recursive == false`: `store.ListMeta(projectID, base.LikePrefix(prefix))` lalu `base.BuildLevel(prefix, metas)`. `Items` = file langsung (`Level.Files`), `Folders` = `Path` tiap `Level.Folders` (urut nama), paging (`Offset`/`Limit`) hanya atas `Items`; `Folders` selalu dikembalikan utuh. `Total` = jumlah file langsung.
7. `Truncated = Offset + len(Items) < Total`.

Keputusan penting:
- Semua SQL memakai `project_id = $1` (rule 6.10). Test AC8 membuktikan `Items`/`Folders` tidak pernah memuat data project lain.
- Prefix tanpa file bukan error: `Items` kosong, `Total` 0, err nil.
- Tidak membaca kolom `content` (hanya metadata). Isi file tidak pernah masuk log.
- `LikePrefix` men-escape `\ % _` dan menambah `/%` sehingga `docs` tidak cocok dengan `docs2/x.md` maupun `docs.md`.
- `Total` dan halaman pada mode rekursif diambil dengan dua statement (`SELECT` halaman, `COUNT`). Selisih akibat penulisan paralel di antara keduanya dapat diterima (read-only, hanya untuk agent); dicatat di PR. Bila ingin konsisten, boleh dijadikan satu statement dengan CTE; tidak wajib.
- Usecase satu (tidak dipecah): cabang rekursif dan non-rekursif hanyalah dua jalur pembacaan dalam satu usecase dengan validasi bersama.

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai. Cek `ls backend/internal/domain/project/internal/knowledge/base/store/store.go backend/lib/contracts/project/knowledge/listfiles/listfiles.go` dan konstanta `BUS_PROJECT_KNOWLEDGE_LIST_FILES` di `backend/lib/constants/bus.go`. Bila ada yang belum, STOP (bukan tugas usecase ini; dibuat di F).

1. Buat folder `backend/internal/domain/project/internal/knowledge/buslistfiles/`.
2. `usecase.go`:
   ```go
   type Repository interface {
       ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error)
       ListPage(ctx context.Context, projectID, likePrefix string, limit, offset int) ([]base.Meta, error)
       CountByPrefix(ctx context.Context, projectID, likePrefix string) (int, error)
   }
   type Usecase struct{ repo Repository }
   func New(repo Repository) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in listfiles.Request) (listfiles.Response, error)
   ```
   `Execute` urutan: uuid -> prefix -> limit/offset -> cabang rekursif/non-rekursif -> `Truncated`. Error infra dibungkus `fmt.Errorf("buslistfiles: <langkah>: %w", err)`. Tidak menyentuh `sqlx`. Pemetaan `base.Meta` -> `listfiles.Item` dilakukan di `usecase.go` (fungsi privat `toItems`), `Items` tidak pernah nil (slice kosong).
   - Mode non-rekursif: setelah `BuildLevel`, `total := len(level.Files)`; bila `offset >= total` -> `Items` kosong; selainnya `Items = level.Files[offset:min(offset+limit,total)]`. `Folders` dari `level.Folders` (kosong -> slice kosong atau nil boleh, tetapi konsisten: non-nil pada mode non-rekursif).
3. `repository.go`: `type Repo struct{ s *store.Store }`, `NewRepo(s *store.Store) *Repo`.
   - `ListMeta`: delegasi `s.ListMeta`.
   - `ListPage` (SQL khusus feature, lewat `s.Q()`; `projectID` bukan uuid dijaga usecase, tetapi tetap return kosong bila gagal parse untuk keamanan):
     ```sql
     SELECT path, size_bytes, updated_at
     FROM knowledge_files
     WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'
     ORDER BY path COLLATE "C"
     LIMIT $3 OFFSET $4
     ```
   - `CountByPrefix`:
     ```sql
     SELECT count(*) FROM knowledge_files
     WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'
     ```
   - Row struct privat bertag `db`; waktu `.UTC()`.
4. `handler.go`:
   ```go
   type Handler struct{ uc *Usecase }
   func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
       in, err := bus.Expect[listfiles.Request](req)
       if err != nil { return nil, err }
       return h.uc.Execute(ctx, in)
   }
   ```
   Sesuaikan signature dengan tipe handler `bus.Registry.Register` di `main` (ikuti pola `countfiles` / handler T03; cek `backend/internal/infra/bus`). Tanpa `params.go` (feature bus, rule 6.9). Tidak ada log isi; log hanya `project_id`, `feature` pada error infra.
5. `init.go`:
   ```go
   func Init(d app.Deps) {
       repo := NewRepo(store.New(d.DB))
       h := &Handler{uc: New(repo)}
       d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, h.Handle)
   }
   ```
6. `backend/internal/domain/project/internal/knowledge/base.go`: tambah satu baris `buslistfiles.Init(d)` di `Register`. (Registrasi ganda endpoint = panic; pastikan hanya satu.)
7. Test (bagian 6): `usecase_test.go` (repo palsu), `handler_test.go` (registry bus nyata), `repository_integration_test.go` (tag `integration`).
8. Verifikasi: `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/buslistfiles/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/buslistfiles/...`; `internal/archtest` lulus (feature tidak meng-import feature lain).
9. Catatan dokumen: README katalog bus sudah diperbarui di F langkah 10. Bila belum memuat `Folders`, tambahkan di PR ini hanya pada baris `list-files` (bukan file plan lain).

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari `backend/migrations/project/<VERSION>_create_knowledge_files.sql` (foundation). Indeks `knowledge_files_project_path_prefix` melayani `LIKE 'prefix/%'` dan `ORDER BY path COLLATE "C"`.

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/buslistfiles/init.go` | baru: rakit repo -> usecase -> handler, daftar bus `list-files` |
| `.../buslistfiles/handler.go` | baru: `bus.Expect`, panggil usecase |
| `.../buslistfiles/usecase.go` | baru: interface `Repository`, `Execute` (validasi + orkestrasi) |
| `.../buslistfiles/repository.go` | baru: `ListPage`, `CountByPrefix`, delegasi `ListMeta` |
| `.../buslistfiles/usecase_test.go`, `handler_test.go`, `repository_integration_test.go` | baru |
| `.../knowledge/base.go` | tambah `buslistfiles.Init(d)` |
| `.../knowledge/knowledge_integration_test.go` | (di W, bukan di plan ini) memuat skenario AC8 lintas usecase |

Tidak mengubah: kontrak `listfiles`, konstanta bus, `base/`, `base/store`, migrasi, OpenAPI (bus tidak masuk OpenAPI), frontend.

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, repo palsu, table-driven)

- ProjectID `"bukan-uuid"` -> `errors.Is(err, errs.ErrInvalid)`, `details.reason = invalid_project_id`, repo tidak dipanggil.
- Prefix `/docs` (satu slash depan) -> repo menerima `LikePrefix("docs")` = `docs/%`.
- Prefix `/` -> root (prefix `""`, like `%`). Prefix `docs/` -> `docs`.
- Prefix `//docs` -> invalid (`path_absolute`); `../x` -> `path_traversal`; `a//b` -> `path_empty_segment`; pesan memuat prefix asli dan alasan.
- Limit 0 -> 100 diteruskan ke repo; limit 9999 -> 500; limit 7 -> 7.
- Offset -1 -> invalid (`invalid_offset`), repo tidak dipanggil. Offset 0 valid.
- Recursive: repo `ListPage` dipanggil dengan limit/offset; `CountByPrefix` -> `Total`; `Folders` nil.
- Recursive paging/Truncated: total 5, limit 2, offset 0 -> 2 item, Truncated true; offset 4 -> 1 item, Truncated false; offset 5 -> 0 item, Truncated false; total 0 -> `Items` kosong non-nil, Truncated false.
- Non-recursive: metas `docs/a.md`, `docs/b.md`, `docs/auth/login.md`, `docs/auth/flow/x.md` prefix `docs` -> Items `[docs/a.md, docs/b.md]`, Folders `[docs/auth]`, Total 2; `ListPage` tidak dipanggil.
- Non-recursive paging: limit 1 -> Items `[docs/a.md]`, Truncated true, Folders tetap utuh; offset 1 -> `[docs/b.md]`, Truncated false; offset 10 -> kosong, Truncated false.
- Error repo -> error terbungkus (bukan `errs.Invalid`), tidak bocor detail SQL ke pesan bisnis.

### Handler (`handler_test.go`, registry bus nyata + repo palsu)

- Request bertipe salah (`struct{}` / string) -> `errors.Is(err, bus.ErrBadRequestType)`.
- Request valid melewati `bus.CallTyped[listfiles.Response]` dan hasil sesuai usecase.
- Endpoint terdaftar: `Init` memakai `constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES` (panggil tanpa `ErrNotRegistered`).

### Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

- **AC8 isolasi**: dua project A dan B; keduanya memiliki `same.md` dan `docs/only-a.md` (hanya A) / `docs/only-b.md` (hanya B). `list-files` untuk A (rekursif dan non-rekursif, root dan `docs`) tidak pernah memuat `only-b.md`; `Total` sama dengan jumlah milik A saja; sebaliknya untuk B.
- Rekursif vs non-rekursif pada struktur 3 tingkat (`docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`, `top.md`): root rekursif -> 4 item urut `path`; root non-rekursif -> Items `[top.md]`, Folders `[docs]`; prefix `docs` rekursif -> 3 item; non-rekursif -> `[docs/a.md]` + `[docs/auth]`.
- Urutan byte order: `B.md`, `a.md`, `docs/x.md` urut `COLLATE "C"` (huruf besar lebih dulu).
- Paging 7 file, limit 3: halaman offset 0/3/6 menyusun ulang daftar lengkap tanpa duplikat; `Truncated` benar di tiap halaman.
- Prefix dengan karakter khusus: `a_b/x.md` vs `axb/x.md` (prefix `a_b` hanya cocok yang pertama); `a%c/x.md` tidak cocok `abc/x.md`; `docs` tidak mencakup `docs2/x.md` dan `docs.md`.
- Prefix tanpa file -> `Items` kosong, `Total` 0, err nil.
- `ProjectID` uuid valid tetapi project tidak punya file / tidak ada -> kosong, err nil.
- EXPLAIN (rule 8.8): `ListPage` untuk prefix non-root memakai `knowledge_files_project_path_prefix` (isi >= 5000 baris + `ANALYZE`, tanpa memaksa planner); dicatat di PR.
- Tidak ada baris audit/record dibuat (handler `record` palsu tidak dipanggil).

## 7. E2E Scenarios

Usecase ini tidak punya UI atau route HTTP, jadi tidak ada Playwright khusus. Skenario berikut dijalankan sebagai integrasi lewat bus/router (di W, `knowledge_integration_test.go`), dan akan ikut diuji lewat gateway MCP saat T08 selesai.

Positif:
1. Upload 3 file 3 tingkat via `uploadfiles` -> `list-files` root non-rekursif memuat folder `docs`; `docs` rekursif memuat 3 path berurut.
2. Prefix ditulis agent dengan awalan `/docs` -> hasil sama dengan `docs`.
3. Project dengan 250 file, limit 100: tiga panggilan (offset 0/100/200) menghasilkan 250 path unik; `Truncated` true, true, false.
4. Setelah `deletefolder docs`, `list-files` tidak lagi memuat `docs/**`.
5. Setelah `movefile`, `list-files` menampilkan path baru dan tidak path lama.

Negatif:
1. Dua project dengan nama file sama: panggilan untuk project A tidak pernah menampilkan file B (AC8).
2. `ProjectID` bukan uuid -> error `ErrInvalid` dengan pesan memandu, tanpa query DB.
3. Prefix `../secret` atau `a//b` -> error `ErrInvalid` berisi prefix dan alasan.
4. Offset negatif -> error `ErrInvalid`.
5. Request bertipe salah -> `ErrBadRequestType`.
6. Prefix tidak ada -> kosong, bukan error.
7. Tidak ada entri audit yang tercatat dari pemanggilan `list-files`.
8. Pemanggil dengan project yang tidak ada di database -> kosong, tanpa kebocoran data project lain.

## 8. Request/Response body

Bus Go (bukan JSON di wire). Tipe dari `lib/contracts/project/knowledge/listfiles`.

Non-rekursif (file langsung + subfolder):

```go
res, err := bus.CallTyped[listfiles.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
    listfiles.Request{ProjectID: pid, Prefix: "/docs", Recursive: false, Limit: 100})
// listfiles.Response{
//   Items:     []listfiles.Item{{Path: "docs/a.md", SizeBytes: 120, UpdatedAt: t}},
//   Folders:   []string{"docs/auth"},
//   Total:     1,
//   Truncated: false,
// }
```

Rekursif dengan paging:

```go
res, err := bus.CallTyped[listfiles.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
    listfiles.Request{ProjectID: pid, Prefix: "", Recursive: true, Limit: 2, Offset: 0})
// listfiles.Response{
//   Items: []listfiles.Item{
//     {Path: "docs/a.md", SizeBytes: 120, UpdatedAt: t1},
//     {Path: "docs/auth/login.md", SizeBytes: 842, UpdatedAt: t2},
//   },
//   Folders:   nil,
//   Total:     5,
//   Truncated: true, // 0 + 2 < 5
// }
```

Prefix tanpa file: `listfiles.Response{Items: []listfiles.Item{}, Folders: []string{}, Total: 0, Truncated: false}`, err nil.

Error (semua membungkus `errs.ErrInvalid`, kecuali tipe salah; format `*errs.Error` seragam rule 11.1, yang oleh gateway T07 dipetakan ke tool error agent):

```go
// ProjectID salah
// err: errs.Error{Code: INVALID_ARGUMENT, Message: "ProjectID tidak valid", Details: {"reason": "invalid_project_id"}}

// Prefix salah
// err: errs.Error{Code: INVALID_ARGUMENT,
//   Message: `prefix "../x" tidak valid: path tidak boleh memuat segmen . atau ..`,
//   Details: {"reason": "path_traversal"}}

// Offset negatif
// err: errs.Error{Code: INVALID_ARGUMENT, Message: "offset tidak boleh negatif", Details: {"reason": "invalid_offset"}}

// Tipe request salah
// errors.Is(err, bus.ErrBadRequestType) == true
```

Bila error ini diteruskan ke HTTP oleh pemanggil, bentuk JSON-nya:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "prefix \"../x\" tidak valid: path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```
