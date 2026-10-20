# T05 project/knowledge — Usecase `createfile` (U1, gelombang A)

Melengkapi `docs/superpowers/plans/project-knowledge/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `createfile`. Plan usecase lain tidak disentuh.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T05 (AC6; mendukung AC3, AC8 sebagai penyedia data) |
| Scope | fullstack pada tiket; usecase ini backend + verifikasi kontrak OpenAPI. UI editor "File baru" ada di `frontend-plan.md` |
| Domain | project |
| Module | knowledge |
| Feature | `createfile` |
| Endpoint | `/api/projects/{projectID}/knowledge/files` |
| Method | POST |
| Version | tanpa prefix versi di URL (prefix `/api`); `api/modules/project/knowledge.yml` `info.version: 0.1.0` |
| Siapa | member, owner, admin (dicek lewat `check-access` di usecase; middleware route hanya `RequireAuth()`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Body limit | `middleware.BodyLimit(Limits.JSONBodyLimit())` = `6*MaxFileBytes + 64 KiB` |
| Respons sukses | 201 `KnowledgeFile` (FileDTO) |
| Audit | `knowledge.file.created` `{bytes}` (Call setelah commit, fail-open) |
| Bus dikonsumsi | `project.management.check-access` (fail-closed, via `base/access`), `project.management.get-project` (fail-soft, via `base/audit`), `core.audit.record` (fail-open, via `base/audit`) |
| Tabel / migrasi | menulis `knowledge_files`; migrasi dibuat di F overview, tidak ada migrasi baru di sini |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| allowedPaths | `.../knowledge/createfile/`, `.../knowledge/base.go` (satu baris `createfile.Init(d)`) |

Prasyarat (dari F overview; bila belum ada, hentikan dan laporkan, jangan dibuat di luar allowedPaths): migrasi `20261024090000000_create_knowledge_files.sql` (atau versi geser, lihat overview bagian 6); config `MaxProjectFiles`; package `base` (`NormalizePath`, `IsHidden`, `IsSupported`, `PrepareContent`, `NewFile`, `File.ToDTO`, `Actor`, `RequestMeta`, `Limits`, `LimitsFrom`, `Invalid`, `TooLarge`, `ErrPathExists`, `Reason*`); `base/store` (`Store`, `New`, `InTx`, `Q`, `LockProject`, `CountFiles`, `IsUniqueViolation`); `base/access` (`Checker`, `New`); `base/audit` (`Recorder`, `Event`, `New`); `record.ActionKnowledgeFileCreated` dan konstanta `BUS_CORE_AUDIT_RECORD` (T02); path `POST /projects/{projectID}/knowledge/files` dan schema `KnowledgeCreateRequest`/`KnowledgeFile` sudah ada di `api/modules/project/knowledge.yml` (dibuat di F; usecase ini hanya memverifikasi, jangan diubah).

## Summary Plan

`createfile` membuat satu file knowledge baru dari editor web.

Aturan (urutan eksekusi mengikat; error pertama menang):
1. **Otorisasi dulu** (`access.Require`): tanpa sesi 401; `projectID` bukan uuid, project tidak ada, atau bukan anggota (level `none`) -> 404 `project tidak ditemukan`. Non-anggota mendapat 404 walau input salah (rule 7.3, AC6). Level `member|owner|admin` semuanya boleh.
2. **Path** (`base.NormalizePath`): gagal -> 400 `INVALID_ARGUMENT` `details.reason` = salah satu `path_empty|path_absolute|path_traversal|path_empty_segment|path_control_char|path_too_long|segment_too_long|invalid_encoding`. Path dipakai apa adanya (peka huruf besar/kecil, tidak diperbaiki diam-diam).
3. **Hidden** (`base.IsHidden`): segmen berawalan `.` (mis. `.git/x.md`, `docs/.env.md`) -> 400 reason `hidden`. Di create ini ditolak 400 untuk konsistensi dengan upload yang melaporkan `skipped hidden` (overview D9); file tersembunyi tidak pernah ada di project.
4. **Tipe** (`base.IsSupported`): ekstensi bukan `.md|.markdown|.txt` (case-insensitive) -> 400 reason `unsupported_type`. Urutan hidden sebelum tipe sama dengan `ClassifyPart` di upload, sehingga `.env` dilaporkan `hidden` (bukan `unsupported_type`).
5. **Isi** (`base.PrepareContent(raw, MaxFileBytes)`): BOM dibuang; bukan UTF-8 valid atau memuat NUL -> 400 reason `invalid_encoding`; byte > `MaxFileBytes` (diukur setelah BOM dibuang) -> 413 `PAYLOAD_TOO_LARGE` reason `too_large` + `details.max_bytes`. Isi kosong valid (file kosong boleh dibuat).
6. **Transaksi** (`repo.InTx`): `LockProject` (advisory xact lock per project) -> `CountFiles >= Limits.MaxProjectFiles` -> 400 reason `project_file_limit` -> `INSERT ... RETURNING` -> unique violation `knowledge_files_project_id_path_key` -> 409 `CONFLICT` reason `path_exists`.
7. Entity `base.NewFile(projectID, path, content, actor)` menghitung `Title` (heading `# ` pertama), `Hash` (sha256), `SizeBytes` (`len(content)` byte), dan `Created*/Updated*` dari aktor. Usecase tidak menghitung apa pun sendiri (rule bisnis di entity). `created_at/updated_at` diisi `now()` oleh DB.
8. Respons 201 = `File.ToDTO()` dari baris hasil `RETURNING` (waktu dari DB, UTC), termasuk `version` = hex sha256 isi (D7).
9. **Audit** `knowledge.file.created` setelah commit, fail-open: `TargetType "file"`, `Target` = path, `Args {}`, `Result {"bytes": SizeBytes}`. Isi file TIDAK PERNAH masuk audit atau log (rule 10.3, 13.6). Gagal/ditolak -> 0 entri.
10. Tidak ada panggilan bus di dalam transaksi (check-access sebelum tx, audit dan get-project setelah commit).
11. Error infra dibungkus `fmt.Errorf("createfile: <langkah>: %w", err)` -> 500 generik. Rollback otomatis bila fn tx mengembalikan error (`db.BeginTx`).

Catatan perilaku yang disengaja:
- Bila project sudah di batas kuota DAN path yang diminta sudah ada, hasilnya 400 `project_file_limit` (cek kuota mendahului INSERT sesuai urutan yang ditetapkan). Dicatat di risiko; tidak ada pre-check keberadaan terpisah agar tidak menambah query.
- Tidak ada idempotensi: POST kedua dengan path sama -> 409, tanpa menimpa. Menimpa = `updatefile` (dengan `version`) atau upload.
- Folder tidak punya entitas sendiri; folder "ada" selama ada file di bawahnya.
- `projectID` bukan uuid: `base/access` menjawab 404 tanpa menyentuh DB.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol prasyarat ada (`ls backend/internal/domain/project/internal/knowledge/base backend/internal/domain/project/internal/knowledge/base/store`). Baca `app.Deps`, `db.DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `respond.DecodeJSON/WriteJSON/WriteError`, `middleware.RequireAuth/BodyLimit/KeyByIP`, dan pola `Init` feature di module `management` (T03) lalu ikuti. Bila prasyarat hilang, STOP dan laporkan.

**B1. `createfile/params.go`**

```go
type CreateRequest struct {
    Path    string `json:"path"`
    Content string `json:"content"`
}
```

Hanya bentuk. Tanpa validasi isi. `respond.DecodeJSON` menolak field tak dikenal, body kosong/rusak (400), dan `*http.MaxBytesError` (413). `path` tidak ada -> `""` -> entity menolak `path_empty`. `content` tidak ada -> `""` (file kosong valid).

**B2. `createfile/usecase.go`** (orkestrasi saja, rule 5.x)

```go
type Repository interface {
    InTx(ctx context.Context, fn func(Repository) error) error
    LockProject(ctx context.Context, projectID string) error
    CountFiles(ctx context.Context, projectID string) (int, error)
    Insert(ctx context.Context, f base.File) (base.File, error) // RETURNING; unique violation -> base.ErrPathExists
}

type Usecase struct {
    repo   Repository
    access access.Checker
    audit  audit.Recorder
    limits base.Limits
}

func NewUsecase(repo Repository, ac access.Checker, au audit.Recorder, limits base.Limits) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID string, req CreateRequest, meta base.RequestMeta) (base.FileDTO, error)
```

Alur `Execute`:
1. `actor, err := u.access.Require(ctx, projectID)`; error dikembalikan apa adanya.
2. `path, r := base.NormalizePath(req.Path)`; `r != ""` -> `base.Invalid(r)`.
3. `base.IsHidden(path)` -> `base.Invalid(base.ReasonHidden)`.
4. `!base.IsSupported(path)` -> `base.Invalid(base.ReasonUnsupportedType)`.
5. `content, r := base.PrepareContent([]byte(req.Content), u.limits.MaxFileBytes)`: `r == base.ReasonTooLarge` -> `base.TooLarge(r, u.limits.MaxFileBytes)` dengan pesan `ukuran file melebihi batas <N> byte`; `r != ""` lain -> `base.Invalid(r)`.
6. `f := base.NewFile(projectID, path, content, actor)`.
7. `u.repo.InTx(ctx, func(tx Repository) error { ... })`:
   - `tx.LockProject(ctx, projectID)`;
   - `n, err := tx.CountFiles(ctx, projectID)`; `n >= u.limits.MaxProjectFiles` -> return `base.Invalid(base.ReasonProjectFileLimit)`;
   - `saved, err = tx.Insert(ctx, f)`; error `base.ErrPathExists` dikembalikan apa adanya (tanpa membungkus).
   - Error infra dibungkus `fmt.Errorf("createfile: <langkah>: %w", err)`.
8. Setelah tx sukses: `u.audit.Record(ctx, audit.Event{Action: record.ActionKnowledgeFileCreated, ProjectID: projectID, Actor: actor, TargetType: "file", Target: saved.Path, Args: map[string]any{}, Result: map[string]any{"bytes": saved.SizeBytes}, Meta: meta})`. Tidak mengembalikan error.
9. `return saved.ToDTO(), nil`.

Catatan: sentinel `base.ErrPathExists` dibagi; usecase dilarang memodifikasinya.

**B3. `createfile/repository.go`**

```go
type Repo struct{ s *store.Store }

func NewRepository(s *store.Store) *Repo

func (r *Repo) InTx(ctx context.Context, fn func(Repository) error) error {
    return r.s.InTx(ctx, func(ts *store.Store) error { return fn(&Repo{s: ts}) })
}
func (r *Repo) LockProject(ctx, id) error        { return r.s.LockProject(ctx, id) }
func (r *Repo) CountFiles(ctx, id) (int, error)  { return r.s.CountFiles(ctx, id) }
func (r *Repo) Insert(ctx, f base.File) (base.File, error)
```

SQL `Insert` (parameter saja, tanpa interpolasi string):

```sql
INSERT INTO knowledge_files
  (project_id, path, title, content, content_hash, size_bytes,
   created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$7,$8)
RETURNING id::text, project_id::text, path, title, content, content_hash, size_bytes,
          created_at, created_by::text, created_by_label,
          updated_at, updated_by::text, updated_by_label
```

- Eksekusi lewat `r.s.Q().QueryRowxContext(...)` lalu scan ke row struct privat bertag `db`, dikonversi ke `base.File` (waktu `.UTC()`).
- `store.IsUniqueViolation(err)` (code `23505`) -> return `base.ErrPathExists`.
- Karena unique violation di dalam tx Postgres membatalkan tx, error dikembalikan dari fn sehingga rollback; tidak ada query lanjutan setelah error.
- `projectID` dan `created_by` adalah uuid valid (sudah lolos `access`); tidak perlu parse ulang di repository.

**B4. `createfile/handler.go`**

```go
type Handler struct {
    uc  *Usecase
    log *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req CreateRequest
    if err := respond.DecodeJSON(r, &req); err != nil { respond.WriteError(w, r, err); return }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), req, meta)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusCreated, dto)
}
```

- Urutan penting: `DecodeJSON` boleh mendahului otorisasi karena hanya membaca bentuk (batas body sudah menutup abuse); non-anggota tetap 404 karena JSON valid diteruskan ke `Execute` yang memanggil `access` lebih dulu. Body JSON yang rusak dari non-anggota -> 400 (bentuk, bukan data project); dapat diterima.
- `truncate` adalah helper privat kecil (tidak mengimpor feature lain, rule 4.3). Bila `management` punya helper serupa, JANGAN diimpor; duplikasi 3 baris di feature ini.
- Log hanya pada 500 lewat `respond.WriteError` (bukan body, bukan `content`).

**B5. `createfile/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    limits := base.LimitsFrom(d.Config)
    uc := NewUsecase(NewRepository(s), access.New(d.Bus), audit.New(d.Bus, d.Log), limits)
    h := &Handler{uc: uc, log: d.Log}
    d.API.With(middleware.RequireAuth(), middleware.BodyLimit(limits.JSONBodyLimit())).
        Post("/projects/{projectID}/knowledge/files", h.ServeHTTP)
}
```

Nama method route mengikuti pola `management` (pola penuh dengan `{projectID}`, T03 D4). Pada prefix `/api` router T00. `PUT/GET/DELETE` pada path yang sama didaftarkan oleh feature masing-masing (`updatefile`, `getfile`, `deletefile`); chi menggabungkan method pada pola yang sama, jadi TIDAK ada konflik selama method berbeda.

**B6. `knowledge/base.go`**: tambah satu baris `createfile.Init(d)` di `Register` beserta import. Jangan menyentuh baris Init feature lain.

**B7. Verifikasi OpenAPI.** Pastikan `api/modules/project/knowledge.yml` memuat operasi `POST /api/projects/{projectID}/knowledge/files` dengan request `KnowledgeCreateRequest`, respons 201 `KnowledgeFile`, 400/401/404/409/413/500, header `X-Requested-With`, dan `details.reason` didokumentasikan. Jalankan `make api-check`. Bila tidak sesuai, laporkan (file itu milik F), jangan diubah dari usecase ini.

**B8. Jalankan** `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/createfile/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/createfile/...`; `internal/archtest` lulus (feature tidak mengimpor feature lain; hanya `base`, `base/store`, `base/access`, `base/audit`).

## Migration

Tidak ada. Tabel `knowledge_files` (kolom, `UNIQUE (project_id, path)` bernama `knowledge_files_project_id_path_key`, CHECK, indeks GIN dan prefix) dibuat oleh `backend/migrations/project/20261024090000000_create_knowledge_files.sql` pada langkah F overview. Usecase ini hanya `INSERT`.

## Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/createfile/params.go` | baru: `CreateRequest{Path, Content}` |
| `.../createfile/usecase.go` | baru: `Repository`, `Usecase`, `NewUsecase`, `Execute` |
| `.../createfile/repository.go` | baru: `Repo` membungkus `*store.Store`; `InTx`, `LockProject`, `CountFiles`, `Insert` |
| `.../createfile/handler.go` | baru: decode JSON, meta, panggil usecase, 201 |
| `.../createfile/init.go` | baru: `Init(d app.Deps)` merakit dan mendaftarkan route |
| `.../createfile/usecase_test.go` | baru: unit usecase (repo/access/audit palsu) |
| `.../createfile/repository_integration_test.go` | baru (tag `integration`): SQL `Insert` pada Postgres nyata |
| `.../createfile/handler_test.go` | baru: httptest + router chi (usecase dengan fake) |
| `backend/internal/domain/project/internal/knowledge/base.go` | tambah satu baris `createfile.Init(d)` + import |

Tidak ada perubahan pada `base/`, `base/store`, `base/access`, `base/audit`, kontrak bus, konstanta, config, migrasi, OpenAPI, README. Tidak ada kontrak bus baru dan tidak ada endpoint bus yang disediakan usecase ini.

Kontrak bus yang dikonsumsi (tanpa perubahan):
- `project.management.check-access` (`checkaccess.Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists, Level}`), fail-closed, dipanggil sebelum tx.
- `project.management.get-project` (`getproject.Request{ProjectID}` -> `Response{Found, ID, Name}`), fail-soft, hanya untuk `ProjectLabel` audit setelah commit.
- `core.audit.record` (`record.Request` dengan `Action = knowledge.file.created`), fail-open, setelah commit.

## Test Scenario

### Unit usecase (`usecase_test.go`, repository/access/audit palsu; tidak ada DB)

Fake repo mencatat urutan pemanggilan dan jumlah `Insert`; fake audit menampung `Event`; fake access mengembalikan actor atau error.

1. Sukses: path `docs/auth/login.md`, isi `# Login\n\nAlur`. DTO memuat `path`, `title = "Login"`, `size_bytes = len(isi)`, `version` = 64 hex sha256 isi, `created_by.label = updated_by.label = actor.Label`. Urutan repo: `LockProject` -> `CountFiles` -> `Insert`. Tepat 1 audit: `Action = knowledge.file.created`, `TargetType = "file"`, `Target = path`, `Args = {}`, `Result = {"bytes": n}`, `Meta` diteruskan.
2. Audit tidak memuat isi: seluruh `Args`/`Result` tidak mengandung string unik dari isi (rule 13.6).
3. Access gagal: 404 (`base.ErrProjectNotFound`) -> tidak ada pemanggilan repo, 0 audit; 401 dan error 500 dari access juga meneruskan error tanpa menyentuh repo.
4. Non-anggota dengan path TIDAK valid -> tetap 404 (bukan 400): access dievaluasi lebih dulu.
5. Path tidak valid (table-driven, `errors.Is(err, errs.ErrInvalid)` dan `details.reason`): `""` `path_empty`; `/a.md` `path_absolute`; `../x.md`, `a/./b.md` `path_traversal`; `a//b.md`, `a/` `path_empty_segment`; `a\x00b.md` `path_control_char`; 513 rune `path_too_long`; segmen 256 rune `segment_too_long`; `\xff.md` `invalid_encoding`. Setiap kasus: repo tidak dipanggil, 0 audit.
6. Hidden: `.git/config.md`, `docs/.hidden/x.md`, `.env.md`, `.DS_Store` -> 400 reason `hidden` (bukan `unsupported_type`); `docs/a.b.md` lolos.
7. Tipe tidak didukung: `a.yaml`, `README`, `x.md.bak`, `img.png` -> 400 `unsupported_type`; `A.MD`, `a.markdown`, `a.txt` lolos.
8. Isi: tepat `MaxFileBytes` ok; `MaxFileBytes+1` -> 413 `too_large` dengan `details.max_bytes`; BOM + tepat batas ok dan BOM dibuang (`size_bytes` tanpa 3 byte BOM); NUL -> 400 `invalid_encoding`; byte `\xff\xfe` -> 400 `invalid_encoding`; isi kosong -> sukses, `size_bytes 0`, `title ""`.
9. Kuota: `CountFiles` mengembalikan `MaxProjectFiles` -> 400 `project_file_limit`, `Insert` tidak dipanggil, 0 audit; `MaxProjectFiles-1` -> sukses.
10. Duplikat: `Insert` mengembalikan `base.ErrPathExists` -> 409 `path_exists`, 0 audit.
11. Error infra dari `LockProject`/`CountFiles`/`Insert` -> error terbungkus (`errors.Is` ke error asli), bukan `*errs.Error`, 0 audit.
12. Fail-open audit: fake audit yang gagal (log error) tidak mengubah hasil; usecase tetap mengembalikan DTO. (Perilaku `Record` tanpa error sudah dijamin interface; test memastikan usecase tidak bergantung pada hasilnya.)
13. Tidak ada panggilan audit/bus di dalam `InTx`: fake mencatat flag "dalam tx" dan test memastikan `Record` dipanggil setelah `InTx` selesai.
14. Path peka huruf besar/kecil dipertahankan: `Docs/A.md` tersimpan `Docs/A.md` (bukan lowercase).

### Unit handler (`handler_test.go`, httptest + chi, fake usecase atau usecase dengan fake)

1. 201 dengan JSON DTO snake_case (`size_bytes`, `created_at`, `created_by.id/label`).
2. Body bukan JSON / field tak dikenal (`{"path":"a.md","x":1}`) / body kosong -> 400.
3. Body melebihi `BodyLimit` -> 413 (lewat `respond`).
4. `projectID` diteruskan apa adanya dari `chi.URLParam`.
5. `*errs.Error` dipetakan benar: `INVALID_ARGUMENT` 400 dengan `details.reason`, `PAYLOAD_TOO_LARGE` 413 dengan `details.max_bytes`, `CONFLICT` 409, `NOT_FOUND` 404; error biasa -> 500 generik tanpa bocor pesan internal; `trace_id` ada.
6. Tanpa sesi -> 401 (middleware `RequireAuth`); tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED` (router T00 asli).
7. `TestRoutesRegistered` fragmen: `POST /api/projects/{projectID}/knowledge/files` terdaftar.

### Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

1. `Insert` menyimpan baris: kolom `content_hash` 32 byte, `size_bytes` = byte UTF-8 (isi `é😀` -> 2+4), `created_by = updated_by`, label tersimpan, `created_at = updated_at`, waktu UTC; hasil `RETURNING` sama dengan baris di DB.
2. `Insert` kedua dengan `(project, path)` sama -> `base.ErrPathExists` (unique violation `knowledge_files_project_id_path_key`); path sama di project LAIN sukses (isolasi).
3. Path berbeda hanya huruf besar/kecil (`A.md` vs `a.md`) keduanya sukses (peka huruf besar/kecil).
4. Isi 1 MB berisi kata unik tidak gagal pada kolom generated `search` (batas tsvector, D4).
5. Kolom generated `search` terisi: `Search` kata dari path (`login` pada `docs/auth/login.md`) menemukan file yang baru dibuat (mendukung AC4).
6. Kuota konkuren (service level, `MaxProjectFiles=3`): 5 goroutine `createfile` path berbeda pada project yang sama -> tepat 3 sukses (201) dan 2 gagal 400 `project_file_limit`; `count(*) = 3` (advisory lock menyerialkan; tanpa lock test ini flaky/gagal).
7. Dua goroutine membuat path IDENTIK -> tepat satu 201 dan satu 409; baris hanya satu.
8. Rollback: bila fn tx mengembalikan error setelah `Insert` (disimulasikan), tidak ada baris tersisa.
9. Tx kedua menunggu `LockProject` tx pertama (sudah diuji di `store_integration_test.go`; di sini cukup test 6).

### Integrasi lintas komponen (ditambahkan sebagai subtest di `knowledge_integration_test.go` pada langkah W; tidak dibuat di usecase ini, didaftar agar W mencakupnya)

1. POST create lewat router `/api` asli + `check-access`/`get-project` T03 (atau palsu) + `record` palsu penghitung: member, owner, dan admin (super_admin non-anggota) sukses 201; tepat 1 entri audit `knowledge.file.created` dengan `Result.bytes`; gagal (400/409/413) -> 0 entri.
2. Non-anggota -> 404 (AC6); `projectID` `bukan-uuid` -> 404.
3. Isolasi: file dibuat di project A tidak terlihat oleh `list-files`/`read-file`/`search` bus project B (AC8).
4. Kemudian `GET files?path=` mengembalikan `version` yang sama dengan DTO create; `PUT` dengan `version` itu sukses (kontrak versi konsisten dengan `getfile`/`updatefile`).
5. Cascade: hapus baris `projects` -> file yang dibuat hilang.

## E2E Scenarios

Playwright terhadap binary (butuh T01, T02, T03 merge dan UI editor dari `frontend-plan.md`).

Positif:
1. Developer anggota membuka project -> tab Knowledge -> "File baru" -> isi path `notes/todo.md` dan isi `# Todo` -> simpan -> file muncul di pohon di folder `notes`, editor menampilkan judul "Todo"; reload halaman: file masih ada.
2. Membuat `docs/auth/login.md` dengan diagram Mermaid -> viewer merender diagram.
3. Super Admin (bukan anggota) membuat file di project mana pun -> berhasil (level `admin`).
4. File baru langsung dapat ditemukan lewat pencarian (`todo` -> `notes/todo.md` peringkat teratas karena path).
5. Entri audit `knowledge.file.created` muncul di halaman audit (T02) untuk Super Admin dengan target path dan `bytes`.

Negatif:
1. Path `../x.md` -> pesan error dengan alasan "path tidak boleh memuat segmen . atau .."; tidak ada file dibuat.
2. Path `.env` atau `.git/config.md` -> pesan "path yang diawali titik ... tidak didukung" (400 `hidden`).
3. Path `config.yaml` -> pesan "hanya file .md, .markdown, dan .txt yang didukung".
4. Membuat path yang sudah ada -> pesan "file dengan path ini sudah ada" (409); isi file lama tidak berubah.
5. Isi > 1 MB (tempel teks besar) -> pesan ukuran melebihi batas (413); tidak ada file dibuat.
6. Isi berisi karakter NUL (via `page.request` langsung) -> 400 `invalid_encoding`.
7. Project penuh (env test `MAX_PROJECT_FILES` kecil) -> pesan "project sudah mencapai batas jumlah file".
8. Developer non-anggota membuka URL knowledge project lain, atau memanggil POST langsung via `page.request` -> 404; tidak ada file dibuat.
9. Request POST tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.
10. Dua tab membuat path sama bersamaan -> satu sukses, satu 409.

## Request/Response body

Format error seragam (rule 11.1):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

### Request

`POST /api/projects/0192f0c4-0000-7000-8000-0000000000aa/knowledge/files`

Header: `Content-Type: application/json`, `X-Requested-With: grimoire`, cookie sesi.

```json
{ "path": "docs/auth/login.md", "content": "# Login\n\nAlur login..." }
```

### 201 Created

```json
{
  "path": "docs/auth/login.md",
  "title": "Login",
  "content": "# Login\n\nAlur login...",
  "size_bytes": 22,
  "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea",
  "created_at": "2026-10-24T09:00:00Z",
  "created_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" },
  "updated_at": "2026-10-24T09:00:00Z",
  "updated_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" }
}
```

(`version` pada contoh ilustratif; nilai nyata = hex sha256 isi setelah BOM dibuang. `size_bytes` dalam byte UTF-8.)

### Error

400 path tidak valid (`reason` salah satu: `path_empty`, `path_absolute`, `path_traversal`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path wajib diisi, mis. docs/auth/login.md", "details": { "reason": "path_empty" } }, "trace_id": "..." }
```

400 hidden:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path yang diawali titik (mis. .git/) tidak didukung", "details": { "reason": "hidden" } }, "trace_id": "..." }
```

400 tipe tidak didukung:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "hanya file .md, .markdown, dan .txt yang didukung", "details": { "reason": "unsupported_type" } }, "trace_id": "..." }
```

400 isi bukan teks UTF-8 / berisi NUL:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "isi harus teks UTF-8 tanpa karakter NUL", "details": { "reason": "invalid_encoding" } }, "trace_id": "..." }
```

400 kuota project:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "project sudah mencapai batas jumlah file", "details": { "reason": "project_file_limit" } }, "trace_id": "..." }
```

400 body bukan JSON / field tak dikenal:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "body tidak valid" }, "trace_id": "..." }
```

(teks pesan mengikuti `respond.DecodeJSON`.)

401 tanpa sesi: `{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }`

403 tanpa header CSRF: `{ "error": { "code": "CSRF_REQUIRED", "message": "..." }, "trace_id": "..." }`

404 non-anggota, project tidak ada, atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

409 path sudah ada:

```json
{ "error": { "code": "CONFLICT", "message": "file dengan path ini sudah ada", "details": { "reason": "path_exists" } }, "trace_id": "..." }
```

413 isi terlalu besar:

```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran file melebihi batas 1048576 byte", "details": { "reason": "too_large", "max_bytes": 1048576 } }, "trace_id": "..." }
```

500: `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "..." }` (generik; detail hanya di log).

### Audit (`record.Request` yang dikirim setelah commit)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000a", ActorLabel: "Budi",
    ProjectID: "0192f0c4-0000-7000-8000-0000000000aa", ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeFileCreated, TargetType: "file", Target: "docs/auth/login.md",
    Args: map[string]any{}, Result: map[string]any{"bytes": 22},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "0192f0c4-7b00-...",
}
```

## Risiko dan catatan

- Bila project sudah penuh dan path yang diminta sudah ada, hasilnya `project_file_limit` (400), bukan `path_exists` (409), karena urutan kuota-lalu-INSERT yang ditetapkan. Dampak kecil; bisa dibalik di tiket lanjutan dengan pre-check keberadaan di dalam tx yang sama (lock sudah dipegang).
- Kode status pesan `body tidak valid` bergantung teks `respond.DecodeJSON` di `main`; test handler mengecek `code` dan status, bukan teks pesan.
- Unicode NFD vs NFC tidak dinormalisasi (D11): `é` NFC dan NFD menjadi dua path berbeda.
- Pesan `too_large` memuat angka batas dari config (usecase memformat), bukan dari `Reason.Message()`.
