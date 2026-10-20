# T05 project/knowledge — Usecase `movefile` (POST /api/projects/{projectID}/knowledge/files/move)

Melengkapi `docs/superpowers/plans/project-knowledge/overview.md` (F: migrasi `knowledge_files`, `base/`, `base/store`, `base/access`, `base/audit`, `knowledge/base.go`, `knowledge.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `movefile` (U9, gelombang B). Satu-satunya hal baru di luar folder feature adalah aksi audit `knowledge.file.moved` (D8 overview), yang titik ekstensinya dipakai bersama F.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack pada tiket; plan usecase ini backend saja. FE (aksi "Pindah/Ubah nama" di pohon dan viewer) ada di `frontend-plan.md` |
| Domain | project |
| Module | knowledge |
| Feature | `movefile` (gelombang B, U9) |
| Endpoint | `/api/projects/{projectID}/knowledge/files/move` |
| Method | POST (body JSON `{from, to}`) |
| Version | tanpa prefix versi di URL (prefix `/api`); `knowledge.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; CSRF global (`X-Requested-With: grimoire`). Otorisasi project di usecase lewat `access.Checker.Require` (bus `project.management.check-access`, fail-closed) |
| Siapa | anggota (member/owner) dan super_admin (level `admin`) |
| Body limit | default (1 MiB root; body hanya dua path, 64 KiB lebih dari cukup) |
| Bus dikonsumsi | `project.management.check-access` (via `base/access`); `project.management.get-project` (via `base/audit`, fail-soft); `core.audit.record` (via `base/audit`, setelah commit, fail-open) |
| Bus disediakan | tidak ada |
| Audit | BARU `knowledge.file.moved`, TargetType `file`, Target = path tujuan, Args `{from, to}`, Result `{bytes}` |
| Tabel / migrasi | `UPDATE knowledge_files`; tidak ada migrasi |
| AC | AC6 (non-anggota 404, isolasi project); pendukung AC4 (tsvector mengikuti path baru) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| allowedPaths | `.../knowledge/movefile/`, `.../knowledge/base.go` (satu baris `movefile.Init(d)`), `backend/lib/contracts/core/audit/record/actions.go`, `actions_test.go`, `frontend/src/domain/core/audit-log/listentries/types.ts`, `docs/tickets/README.md` (katalog aksi) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): `base.NormalizePath`, `base.IsHidden`, `base.IsSupported`, `base.Reason*` (`same_path`, `hidden`, `unsupported_type`), `base.Invalid`, `base.ErrFileNotFound`, `base.ErrPathExists`, `base.File`/`File.ToDTO`/`FileDTO`, `base.Actor`, `base.RequestMeta`, `base/store` (`Store`, `InTx`, `Q`, `LockProject`, `FindByPath`, `ListMeta`, `Search`, `IsUniqueViolation`), `base/access`, `base/audit`, `knowledge.Register`, path `POST .../knowledge/files/move` + schema `KnowledgeMoveRequest` di `api/modules/project/knowledge.yml`, migrasi `20261024090000000_create_knowledge_files.sql`, handler T03 `check-access` (atau palsu di test). Aksi `ActionKnowledgeFileMoved` ditambahkan F (overview langkah F6); bila belum ada di `main`, tambahkan di langkah B0 di bawah.

## Summary Plan

`movefile` memindahkan atau mengubah nama SATU file knowledge dalam project yang sama. Hanya kolom `path` (dan jejak perubah) yang berubah; `content`, `content_hash`, `title`, `size_bytes`, `created_*` tetap, sehingga `version` (hex sha256 isi, D7) TIDAK berubah. Folder tidak punya baris sendiri (diturunkan dari path), jadi folder asal otomatis hilang dari pohon bila kosong dan folder tujuan otomatis muncul.

Alur usecase (urutan wajib overview 3.4: otorisasi sebelum validasi input):

1. `actor, err := access.Require(ctx, projectID)`. Error dikembalikan apa adanya: 401, 404 `project tidak ditemukan` (non-anggota, project tidak ada, `projectID` bukan uuid), 500 terbungkus (fail-closed).
2. Validasi murni, tanpa I/O, berurutan (alasan pertama yang gagal menang):
   - `from, r := base.NormalizePath(in.From)`; gagal -> `base.Invalid(r)`.
   - `to, r := base.NormalizePath(in.To)`; gagal -> `base.Invalid(r)`.
   - `base.IsHidden(to)` -> `base.Invalid(ReasonHidden)` (D9).
   - `!base.IsSupported(to)` -> `base.Invalid(ReasonUnsupportedType)`.
   - `from == to` -> `base.Invalid(ReasonSamePath)` (400, bukan no-op 200).
   - `from` yang hidden atau bertipe tak didukung TIDAK ditolak khusus: file semacam itu tidak pernah ada (D9, createfile/upload menolaknya), sehingga hasilnya 404 `file tidak ditemukan` secara wajar.
3. `repo.InTx`:
   1. `LockProject(projectID)` (advisory xact lock; serial dengan `createfile`, `uploadfiles`, `movefile` lain di project yang sama, sehingga cek `to` lalu UPDATE tidak balapan).
   2. `PathExists(from)` false -> `base.ErrFileNotFound` (404).
   3. `PathExists(to)` true -> `base.ErrPathExists` (409 `path_exists`). Urutan 404 sebelum 409 mengikat: bila `from` hilang DAN `to` ada, hasilnya 404.
   4. `Rename(from, to, actor)`: `UPDATE ... SET path=$to, updated_at=now(), updated_by=$actor, updated_by_label=$label WHERE project_id=$1 AND path=$from`. 0 baris (file dihapus oleh `deletefile` yang tanpa lock di antara langkah 2 dan 4) -> `ErrFileNotFound`. Unique violation (jaring pengaman) -> `ErrPathExists`.
   5. `FindByPath(to)` -> `base.File` untuk respons.
4. Setelah commit: `audit.Record(ctx, Event{Action: ActionKnowledgeFileMoved, TargetType: "file", Target: to, Args: {from, to}, Result: {bytes: f.SizeBytes}, ...})`. Fail-open: kegagalan audit hanya di-log. Gagal/ditolak -> 0 entri audit.
5. Jawab 200 `f.ToDTO()` (path baru, `version` sama dengan sebelum pindah).

Keputusan:
- Tidak ada cek kuota: jumlah file tidak berubah (pindah bukan menambah file). Karena itu tidak memanggil `CountFiles`.
- Cek keberadaan memakai query ringan `PathExists` (`SELECT EXISTS(...)`, tanpa kolom `content`), bukan `FindByPath`, agar tidak memuat isi hingga 1 MiB dua kali. Hanya `FindByPath(to)` terakhir memuat isi untuk DTO. Ini deviasi kecil dari redaksi tugas ("FindByPath(from)"); perilaku eksternal identik.
- `search` (kolom `GENERATED ALWAYS AS ... STORED`) dihitung ulang Postgres otomatis saat `path` berubah; tidak ada kode reindex. Bobot A path memakai `translate(path, '/._-', '    ')`, sehingga kata di nama baru langsung ditemukan dan kata di nama lama tidak lagi.
- Peka huruf besar/kecil (D11): `Docs/A.md` -> `docs/a.md` adalah pindah sah (path berbeda). Tidak ada normalisasi Unicode.
- `title` tidak diubah (diturunkan dari isi, bukan dari path).
- Memindahkan file ke path yang sama persis dengan nama folder yang sudah ada sebagai prefix (mis. `to = a/x.md` ketika ada `a/x.md/y.md`) diizinkan; sama dengan perilaku `createfile` (overview tidak melarang bentrok nama file/folder). Dicatat sebagai keterbatasan, bukan ditangani di sini.
- Non-anggota dan `projectID` bukan uuid menghasilkan respons byte-identik dan terjadi SEBELUM path divalidasi: non-anggota dengan `from=../x` tetap 404, bukan 400 (rule 7.3).
- Isolasi project: semua SQL memakai `project_id = $1`. Pindah ke path yang dipakai project lain tidak bentrok (unique per `(project_id, path)`).
- Panggilan bus (check-access sebelum tx; get-project dan audit record sesudah commit) TIDAK PERNAH di dalam tx (overview 3.5).
- Isi file tidak pernah masuk log atau audit; log hanya `project_id`, `user_id`, `feature=movefile`, dan `from`/`to` pada error infra.
- Usecase hanya orkestrasi; rule path di `base`, SQL di `repository.go`/`base/store`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat dan aksi audit (satu-satunya edit di luar folder feature).** Cek simbol F. Baca signature nyata `respond.DecodeJSON`, `respond.WriteJSON`, `respond.WriteError`, `middleware.RequireAuth()`, `middleware.KeyByIP`, dan pola `Init` feature knowledge lain yang sudah ada (mis. `createfile`) bila ada; ikuti persis. Lalu pastikan aksi audit ada (idempotent: bila F sudah menambahkannya, hanya verifikasi; jangan duplikasi):
- `backend/lib/contracts/core/audit/record/actions.go`: `ActionKnowledgeFileMoved = "knowledge.file.moved"` di grup konten (setelah `ActionKnowledgeFileDeleted`), dan tambahkan ke `Actions()`.
- `backend/lib/contracts/core/audit/record/actions_test.go`: tambahkan ke peta `catalog` dan naikkan hitungan total sebesar 1 (nilai aktual di `main` saat dikerjakan; overview mencatat 33 -> 34, bisa berbeda bila T04/T06 merge lebih dulu).
- `frontend/src/domain/core/audit-log/listentries/types.ts`: grup `content` tambah `'knowledge.file.moved'` (komentar file itu mewajibkan sinkron dengan katalog Go).
- `docs/tickets/README.md`: katalog aksi audit grup "Konten" tambah `knowledge.file.moved`.
Jalankan `go test ./lib/contracts/core/audit/record/...` dan test FE terkait katalog (bila ada) sebelum lanjut.

**B1. `movefile/params.go`**

```go
package movefile

// Request adalah body POST. Field tak dikenal ditolak respond.DecodeJSON (400).
type Request struct {
    From string `json:"from"`
    To   string `json:"to"`
}
```

**B2. `movefile/repository.go`** (SQL khusus feature; membungkus `*store.Store`, overview 3.2)

```go
package movefile

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo

// InTx menjalankan fn dengan Repo terikat tx (s.InTx). Error fn dikembalikan apa adanya.
func (r *Repo) InTx(ctx context.Context, fn func(Repository) error) error

func (r *Repo) LockProject(ctx context.Context, projectID string) error // s.LockProject
func (r *Repo) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error) // s.FindByPath

// PathExists: SELECT EXISTS(SELECT 1 FROM knowledge_files WHERE project_id=$1 AND path=$2).
// projectID bukan uuid -> false tanpa query.
func (r *Repo) PathExists(ctx context.Context, projectID, path string) (bool, error)

// Rename:
//   UPDATE knowledge_files
//      SET path = $3, updated_at = now(), updated_by = $4, updated_by_label = $5
//    WHERE project_id = $1 AND path = $2
// Mengembalikan jumlah baris terpengaruh (RowsAffected). Tidak menyentuh content/content_hash/title/size_bytes/created_*.
// store.IsUniqueViolation(err) dikembalikan sebagai base.ErrPathExists (jaring pengaman).
// Parameter ($4) = actor.ID sebagai uuid; projectID/actor.ID bukan uuid -> error biasa (tidak terjadi: access sudah lolos).
func (r *Repo) Rename(ctx context.Context, projectID, from, to string, actor base.Actor) (int64, error)
```

SQL memakai parameter saja (tanpa konkatenasi). Tidak ada `RETURNING *` agar tidak menduplikasi struct baris privat milik `base/store`; DTO dibaca ulang lewat `FindByPath(to)` dalam tx yang sama.

**B3. `movefile/usecase.go`**

```go
package movefile

// Repository didefinisikan di sisi consumer (rule 5.6); dipenuhi *Repo.
type Repository interface {
    InTx(ctx context.Context, fn func(Repository) error) error
    LockProject(ctx context.Context, projectID string) error
    PathExists(ctx context.Context, projectID, path string) (bool, error)
    Rename(ctx context.Context, projectID, from, to string, actor base.Actor) (int64, error)
    FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
}

type Usecase struct {
    access access.Checker
    repo   Repository
    audit  audit.Recorder
}

func NewUsecase(a access.Checker, r Repository, au audit.Recorder) *Usecase

// Execute: langkah 1-5 pada Summary Plan. meta untuk audit (IP, UserAgent).
func (u *Usecase) Execute(ctx context.Context, projectID string, in Request, meta base.RequestMeta) (base.FileDTO, error)
```

Detail:
- Error infra dibungkus `fmt.Errorf("movefile: <langkah>: %w", err)` (langkah: `lock`, `exists from`, `exists to`, `rename`, `reload`); error bisnis (`ErrFileNotFound`, `ErrPathExists`, `base.Invalid`) dikembalikan tanpa bungkus agar `errors.Is` dan `respond.StatusFor` bekerja.
- Reload `FindByPath(to)` `found=false` (mustahil dalam tx berlock) -> `fmt.Errorf("movefile: reload: file hilang setelah rename")` (500).
- Audit dipanggil hanya setelah `InTx` mengembalikan nil. `bytes` = `f.SizeBytes`.
- Tidak ada import `database/sql`/`sqlx`/`net/http`.

**B4. `movefile/handler.go`**

```go
type Handler struct{ uc *Usecase }
func NewHandler(uc *Usecase) *Handler

// Handle: projectID := chi.URLParam(r, "projectID") (diteruskan apa adanya);
// var in Request; respond.DecodeJSON(r, &in) gagal -> respond.WriteError (400 / 413 oleh respond);
// meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)};
// dto, err := h.uc.Execute(r.Context(), projectID, in, meta)
// err != nil -> respond.WriteError(w, r, err); return
// w.Header().Set("Cache-Control", "no-store"); respond.WriteJSON(w, http.StatusOK, dto)
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request)
```

Catatan urutan: `DecodeJSON` berjalan sebelum otorisasi (body harus dibaca dulu). Body rusak dari non-anggota bisa 400 sebelum 404; ini sama dengan pola feature lain dan tidak membocorkan keberadaan project (400 tidak bergantung pada project). Bila pola feature lain di `main` menunda decode sampai setelah `Require`, ikuti pola itu.

**B5. `movefile/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(access.New(d.Bus), NewRepo(s), audit.New(d.Bus, d.Log))
    h := NewHandler(uc)
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/knowledge/files/move", h.Handle)
}
```

Pola route penuh dengan param (T03 D4). Tanpa `BodyLimit` khusus. Rute `POST .../files/move` tidak bentrok dengan `POST .../files` (`createfile`): segmen berbeda; chi menangani keduanya.

**B6. Wiring.** Tambah SATU baris `movefile.Init(d)` di `knowledge/base.go` (import `.../knowledge/movefile`). Tidak ada perubahan lain di luar `allowedPaths`.

**B7. Verifikasi.** `go build ./... && go vet ./...`; `go test ./internal/domain/project/internal/knowledge/movefile/... ./lib/contracts/core/audit/record/...`; `go test -tags integration ./internal/domain/project/internal/knowledge/...`; `internal/archtest` lulus (tidak ada import antar-feature, tidak ada SQL di usecase/handler); `make api-check` lulus (path `files/move` sudah ada di `knowledge.yml` dari F).

## Migration

Tidak ada. `movefile` hanya `UPDATE` pada `knowledge_files` (dibuat F: `backend/migrations/project/20261024090000000_create_knowledge_files.sql`). Kolom `search` (generated, stored) dihitung ulang otomatis oleh Postgres; indeks GIN `knowledge_files_search_gin` dan `knowledge_files_project_path_prefix` ikut ter-update. `UNIQUE (project_id, path)` (`knowledge_files_project_id_path_key`) melayani cek `PathExists` dan jaring pengaman 409.

## Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/movefile/params.go` | baru: `Request{From, To}` |
| `.../knowledge/movefile/repository.go` | baru: `Repo`, `InTx`, `PathExists`, `Rename` (+ delegasi `LockProject`, `FindByPath`) |
| `.../knowledge/movefile/usecase.go` | baru: `Repository`, `Usecase`, `Execute` |
| `.../knowledge/movefile/handler.go` | baru: `Handler.Handle` |
| `.../knowledge/movefile/init.go` | baru: `Init(d app.Deps)` + route POST |
| `.../knowledge/movefile/usecase_test.go` | baru: unit usecase dengan `access.Checker`, `Repository`, `audit.Recorder` palsu |
| `.../knowledge/movefile/handler_test.go` | baru: httptest + router chi + usecase palsu |
| `.../knowledge/movefile/repository_integration_test.go` | baru (tag `integration`): SQL `PathExists`, `Rename`, tsvector |
| `.../knowledge/movefile/movefile_integration_test.go` | baru (tag `integration`): end-to-end via router + bus (check-access dan record palsu), tree, search, audit, konkurensi |
| `.../knowledge/base.go` | tambah `movefile.Init(d)` |
| `backend/lib/contracts/core/audit/record/actions.go`, `actions_test.go` | `ActionKnowledgeFileMoved` di konstanta, `Actions()`, katalog test, hitungan (bila belum oleh F) |
| `frontend/src/domain/core/audit-log/listentries/types.ts` | `'knowledge.file.moved'` di grup `content` (bila belum oleh F) |
| `docs/tickets/README.md` | katalog aksi audit "Konten": `knowledge.file.moved` (bila belum oleh F) |

Tidak mengubah `base/`, `base/store`, `base/access`, `base/audit`, OpenAPI, konfigurasi, atau migrasi (milik F). Bila ditemukan celah di `base*` (mis. `Event` audit tak menyimpan `Args`), perbaiki di F, bukan di sini.

## Test Scenario

### Unit usecase (`usecase_test.go`, palsu manual, table-driven)

Palsu `Repository` mencatat urutan panggilan (`lock`, `exists:<path>`, `rename`, `find`) dan `InTx` menjalankan fn langsung; palsu `audit.Recorder` menampung `Event`.

| Kasus | Setup | Ekspektasi |
|---|---|---|
| Rename dalam satu folder | `docs/a.md` -> `docs/b.md`; exists(from)=true, exists(to)=false; find mengembalikan File path baru | DTO `Path=docs/b.md`; urutan `lock, exists:from, exists:to, rename, find`; audit 1 event |
| Pindah ke folder lain | `notes/todo.md` -> `archive/todo.md` | sukses; `Rename` dipanggil dengan from/to persis dan `actor` (ID, Label) |
| Version tidak berubah | File hasil `find` membawa `Hash` yang sama dengan sebelum | `DTO.Version` == `Version(Hash)` yang sama (64 hex); `size_bytes`, `title`, `content`, `created_by` tetap |
| `updated_by` | actor = Sari | `Rename` menerima `actor.ID/Label` Sari |
| Target ada | exists(to)=true | `errors.Is(err, base.ErrPathExists)`, code `CONFLICT`, `details.reason=path_exists`; `Rename` TIDAK dipanggil; audit 0 |
| Sumber tidak ada | exists(from)=false | `errors.Is(err, base.ErrFileNotFound)`, code `NOT_FOUND`; exists(to) TIDAK dipanggil |
| Sumber tidak ada DAN target ada | kedua kondisi | tetap 404 (urutan 404 dulu) |
| Sumber hilang saat rename | exists(from)=true lalu `Rename` -> 0 baris | `ErrFileNotFound`; audit 0 |
| Unique violation saat rename | `Rename` mengembalikan `ErrPathExists` | 409; audit 0 |
| Non-anggota | checker -> `base.ErrProjectNotFound` | error sama; repo tidak disentuh (nol panggilan); audit 0 |
| Non-anggota + path jelek | checker 404, `from=../x` | tetap 404, bukan 400 |
| Tanpa identitas | checker -> `base.ErrUnauthenticated` | code `UNAUTHORIZED` |
| Bus check-access gagal | checker -> error biasa | dikembalikan (500), repo tidak disentuh |
| `from` invalid | `""` | 400 `path_empty` |
| | `/a.md` | `path_absolute` |
| | `../x.md`, `a/./b.md` | `path_traversal` |
| | `a//b.md` | `path_empty_segment` |
| | `a\x00b.md` | `path_control_char` |
| | 513 rune | `path_too_long` |
| `to` invalid | kosong, absolut, traversal, 513 rune | alasan yang sama seperti di atas; repo tidak disentuh |
| `to` hidden | `.git/config.md`, `docs/.draft/x.md`, `.env.md` | 400 `hidden`; repo tidak disentuh |
| `to` tipe tidak didukung | `docs/a.yaml`, `README`, `x.md.bak` | 400 `unsupported_type` |
| `to` ekstensi besar | `docs/A.MD` | sukses (IsSupported tidak peka huruf) |
| Sama | `from == to` | 400 `same_path`; repo tidak disentuh |
| Urutan validasi | `from` invalid + `to` hidden | alasan `from` yang muncul |
| Peka huruf | `Docs/A.md` -> `docs/a.md` | lolos validasi (bukan same_path); path diteruskan apa adanya |
| Audit isi | sukses | Event: `Action=knowledge.file.moved`, `TargetType=file`, `Target=<to>`, `Args={from,to}`, `Result={bytes: size}`, `Meta` dari handler; tidak memuat isi file |
| Audit setelah commit | recorder mencatat apakah tx sudah selesai | `Record` dipanggil setelah `InTx` kembali, bukan di dalam fn |
| Error DB | `PathExists`/`Rename` error | dibungkus `movefile: ...`, bukan `*errs.Error` (-> 500); audit 0 |

### Handler (`handler_test.go`, router chi nyata + usecase palsu)

- `POST /projects/{id}/knowledge/files/move` body `{"from":"a.md","to":"b.md"}` sukses -> 200, `Content-Type: application/json`, `Cache-Control: no-store`, body snake_case lengkap, waktu RFC 3339 UTC.
- Body bukan JSON / field tak dikenal (`{"from":"a","to":"b","x":1}`) -> 400 (dari `respond.DecodeJSON`).
- Field `from`/`to` hilang -> usecase menerima `""` -> 400 `path_empty`.
- Path unicode dan spasi (`catatan rapat/é.md` -> `arsip/é.md`) sampai ke usecase utuh.
- Pemetaan error usecase: 400, 401, 404, 409, 500 (pesan 500 generik tanpa detail internal).
- `ProjectID` diteruskan apa adanya dari `chi.URLParam`.

### Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

- `PathExists`: true/false, peka huruf, `projectID` bukan uuid -> false tanpa error, file project lain dengan path sama tidak terhitung.
- `Rename`: baris berubah `path`, `updated_at` maju, `updated_by`/`updated_by_label` = actor; `content`, `content_hash`, `title`, `size_bytes`, `created_at`, `created_by`, `created_by_label` identik; `RowsAffected=1`. Sumber tidak ada -> 0. Target ada -> `base.ErrPathExists`. Project lain dengan path sama tidak tersentuh.
- tsvector: setelah rename `docs/lama-sekali.md` -> `docs/baru-banget.md`, `store.Search` kata `baru` menemukan file, kata `lama` tidak.

### Integrasi lintas komponen (`movefile_integration_test.go`, tag `integration`)

Router `/api` asli T00 + `movefile.Init` + `createfile`/`getfile`-nya diganti seed SQL bila feature lain belum ada; `check-access` palsu di bus (anggota project A), `get-project` palsu, `record` palsu yang menyimpan entri.

- **Rename dalam satu folder**: seed `docs/a.md`; move ke `docs/b.md` -> 200, `path=docs/b.md`; `store.FindByPath(docs/a.md)` tidak ada; `version` sama dengan sebelum; `updated_by` = pemindah, `created_by` = pembuat awal; jumlah baris project tetap.
- **Pindah ke folder lain, folder asal hilang dari pohon**: seed `notes/todo.md` + `docs/a.md`; move `notes/todo.md` -> `archive/todo.md`; `ListMeta(root)` + `base.BuildLevel("", ...)` memuat folder `archive` dan `docs`, TIDAK memuat `notes`; `BuildLevel("notes", ...)` kosong. Seed kedua (`notes/a.md`, `notes/b.md`, pindahkan satu): folder `notes` tetap ada dengan `file_count=1`.
- **Target ada -> 409**: seed `a.md` dan `b.md`; move `a.md` -> `b.md` -> 409 `path_exists`; kedua file tetap dengan isi aslinya; audit 0.
- **Sumber tidak ada -> 404**: 404 `file tidak ditemukan`; audit 0.
- **Sumber tidak ada + target ada -> 404** (bukan 409).
- **tsvector mengikuti path baru**: seed `drafts/x.md` berisi kata `zzz`; move ke `panduan/onboarding.md`; `GET .../search?q=onboarding` menemukan `panduan/onboarding.md` (lewat route search bila sudah ada; bila belum, via `store.Search` langsung); search `drafts` tidak menemukannya; search `zzz` (isi) tetap menemukannya di path baru.
- **Non-anggota -> 404**: checker `Level=none` -> 404 `project tidak ditemukan`; baris tidak berubah; audit 0. `projectID=bukan-uuid` -> respons 404 identik.
- **Isolasi project**: A dan B sama-sama punya `same.md`; anggota A pindah `same.md` -> `renamed.md` pada project A; baris B tidak berubah. Pindah ke path yang sudah ada di project B (tapi tidak di A) berhasil.
- **Validasi 400**: `same_path`, `hidden`, `unsupported_type`, `path_traversal` pada `to`; `path_empty` pada `from`; tidak ada perubahan data, audit 0.
- **Audit**: sukses -> tepat 1 entri `knowledge.file.moved`, `ActorLabel`, `ProjectLabel` dari `get-project` palsu, `TargetType=file`, `Target=<to>`, `Args={from,to}`, `Result.bytes`; gagal (400/404/409) -> 0 entri; `record` palsu mengembalikan error -> respons tetap 200 (fail-open) dan error di-log. Cari string isi unik file di semua `Args`/`Result`: tidak ada (rule 13.6).
- **Konkurensi**: dua move paralel dari sumber berbeda (`a.md`, `b.md`) ke target sama `c.md` -> tepat satu 200 dan satu 409 (`LockProject` menyerialkan); satu file tetap di `c.md`. Move paralel dengan `deletefile` (atau DELETE SQL) pada sumber -> hasil 200 atau 404, tidak pernah 500 dan tidak ada baris yatim.
- **Kuota tidak terpengaruh**: dengan `MAX_PROJECT_FILES=3` dan 3 file ada, pindah tetap 200 (tidak memakai kuota).
- **Folder `projects` dihapus**: cascade menghapus baris (sudah dicakup test W); tidak diulang di sini.
- Untuk `TestRoutesRegistered` di W: route `POST /api/projects/{projectID}/knowledge/files/move` terdaftar (tidak mengubah test itu di sini).

## E2E Scenarios

Playwright terhadap binary (butuh T01, T02, T03 merge dan UI T05).

Positif:
1. Developer anggota membuka tab Knowledge, file `notes/todo.md` ada. Pilih "Pindah" lalu isi path baru `archive/todo.md` -> pohon menampilkan `archive/todo.md`, folder `notes` hilang; membuka file baru menampilkan isi yang sama.
2. Ubah nama dalam folder yang sama (`docs/a.md` -> `docs/b.md`) -> pohon memperbarui nama; viewer yang terbuka beralih ke path baru atau menutup dengan pesan; versi/isi tidak berubah (tidak ada konflik saat Edit sebelum dan sesudah pindah dengan `version` yang sama bila editor dimuat ulang).
3. Cari kata dari nama baru (`onboarding`) -> file muncul di hasil; cari kata dari nama lama -> tidak muncul.
4. Super Admin (bukan anggota) memindahkan file di project mana pun -> berhasil.
5. Setelah pindah, halaman Audit Log (Super Admin) menampilkan entri `knowledge.file.moved` dengan `from` dan `to`.

Negatif:
1. Pindah ke path yang sudah ada -> dialog error "file dengan path ini sudah ada"; kedua file utuh.
2. Dua tab: tab kedua memindahkan file yang sudah dipindah tab pertama -> pesan "file tidak ditemukan"; pohon tidak rusak.
3. Path tujuan `../x.md`, `.env.md`, `docs/a.yaml`, atau sama dengan asal -> pesan error dengan alasan (traversal, tersembunyi, tipe tidak didukung, sama); tidak ada perubahan.
4. Developer non-anggota memanggil endpoint lewat `page.request.post` pada project lain -> 404 `project tidak ditemukan`; respons identik dengan `projectID` bukan uuid.
5. Request tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401 `UNAUTHORIZED`.
6. Body `{}` -> 400 `path_empty`; JSON dengan field asing -> 400.

## Request/Response body

Request: header sesi (cookie) dan `X-Requested-With: grimoire` wajib, `Content-Type: application/json`.

```json
{ "from": "notes/todo.md", "to": "archive/todo.md" }
```

### 200

`FileDTO` dengan `path` baru; `version`, `content`, `title`, `size_bytes`, `created_*` sama dengan sebelum pindah; `updated_at`/`updated_by` = waktu dan pelaku pindah.

```json
{
  "path": "archive/todo.md",
  "title": "Todo",
  "content": "# Todo\n\n- [ ] siapkan rilis",
  "size_bytes": 30,
  "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea",
  "created_at": "2026-10-24T09:00:00Z",
  "created_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" },
  "updated_at": "2026-10-24T10:15:00Z",
  "updated_by": { "id": "0192f0c4-0000-7000-8000-00000000000b", "label": "Sari" }
}
```

Header: `Cache-Control: no-store`.

### Audit (`record.Request` yang dikirim setelah commit)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000b", ActorLabel: "Sari",
    ProjectID: pid, ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeFileMoved, TargetType: "file", Target: "archive/todo.md",
    Args:   map[string]any{"from": "notes/todo.md", "to": "archive/todo.md"},
    Result: map[string]any{"bytes": 30},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...",
}
```

### Error (format seragam, rule 11.1)

400 path tidak valid (juga untuk `from`):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

400 `from` atau `to` kosong/tidak dikirim:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path wajib diisi, mis. docs/auth/login.md", "details": { "reason": "path_empty" } }, "trace_id": "..." }
```

400 tujuan tersembunyi:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path yang diawali titik (mis. .git/) tidak didukung", "details": { "reason": "hidden" } }, "trace_id": "..." }
```

400 tipe tujuan tidak didukung:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "hanya file .md, .markdown, dan .txt yang didukung", "details": { "reason": "unsupported_type" } }, "trace_id": "..." }
```

400 asal sama dengan tujuan:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tujuan sama dengan path asal", "details": { "reason": "same_path" } }, "trace_id": "..." }
```

400 body tidak valid (JSON rusak atau field tak dikenal), kode dan pesan dari `respond.DecodeJSON`:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "body request tidak valid" }, "trace_id": "..." }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

403 tanpa header CSRF:

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib" }, "trace_id": "..." }
```

404 file asal tidak ada (atau milik project lain):

```json
{ "error": { "code": "NOT_FOUND", "message": "file tidak ditemukan" }, "trace_id": "..." }
```

404 non-anggota, project tidak ada, atau `projectID` bukan uuid (identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

409 path tujuan sudah dipakai:

```json
{ "error": { "code": "CONFLICT", "message": "file dengan path ini sudah ada", "details": { "reason": "path_exists" } }, "trace_id": "..." }
```

500 (bus `check-access` gagal atau DB error; pesan generik, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "..." }
```

Catatan: teks pesan untuk 400 body tidak valid, 403 CSRF, dan 500 mengikuti implementasi nyata `respond`/`middleware` T00; test memeriksa `code` dan status, bukan teks pesan untuk tiga kasus itu.
