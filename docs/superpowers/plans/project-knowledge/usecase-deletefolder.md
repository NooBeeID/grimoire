# T05 project/knowledge — Usecase Plan: `deletefolder`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (foundation). Bila bertentangan, overview menang.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (bagian ini backend; dialog konfirmasi hapus folder di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Feature | `deletefolder` (gelombang B, U8) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | `/api/projects/{projectID}/knowledge/folders?path=` |
| Method | DELETE |
| Version | tanpa prefix versi URL (prefix `/api`); OpenAPI `info.version` 0.1.0; tidak ada kontrak bus baru |
| Auth | `middleware.RequireAuth()`; otorisasi project lewat `base/access` (member, owner, admin). CSRF global (`X-Requested-With: grimoire`) |
| Body limit | default (tanpa body) |
| Audit | `knowledge.folder.deleted`, `TargetType` `folder`, `Target` = prefix ternormalisasi, `Result {"count": n}` |
| AC | AC6 |

Bus: mengonsumsi `project.management.check-access` (fail-closed, 500) via `base/access`; `project.management.get-project` (fail-soft, label audit) dan `core.audit.record` (fail-open, setelah commit) via `base/audit`. Tidak menyediakan endpoint bus.

## 2. Summary Plan

Menghapus semua file knowledge di bawah satu folder (prefix) secara rekursif di semua kedalaman. Alur usecase (hanya orkestrasi):

1. `access.Require(ctx, projectID)`: non-anggota atau `projectID` bukan uuid -> 404; tanpa sesi -> 401. Dilakukan SEBELUM validasi `path`, sehingga non-anggota tetap 404 walau input salah.
2. `base.NormalizePrefix(raw)`: gagal -> 400 `base.Invalid(reason)`. Hasil kosong (root: `path` tidak diberikan, kosong, atau hanya `/` yang dibuang) -> 400 `base.Invalid(base.ReasonPrefixRequired)`. Ini mencegah penghapusan seluruh knowledge project dengan satu request.
3. `repo.DeleteByPrefix(ctx, projectID, base.LikePrefix(prefix))` -> jumlah baris terhapus. SQL: `DELETE FROM knowledge_files WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'` (`LikePrefix` = prefix ter-escape + `/%`).
4. `n == 0` -> 404 `base.ErrFolderNotFound` (tanpa audit).
5. `n > 0` -> audit `knowledge.folder.deleted {count: n}` setelah commit (satu statement DELETE = atomik; tidak ada tx eksplisit), fail-open -> 200 `{"deleted": n}`.

Keamanan dan kebenaran:
- `LikePrefix("docs")` = `docs/%`, jadi `docs2/x.md`, `docs.md`, `docs-old/y.md` TIDAK terhapus; prefix ber-`_`, `%`, `\` di-escape sehingga literal (`a_b` tidak cocok `axb/`).
- Query selalu terikat `project_id = $1`; project lain tidak tersentuh.
- Folder bukan entitas tersendiri (diturunkan dari path), jadi "folder kosong" = tidak ada baris = 404. Satu file persis bernama prefix (mis. file `docs` tanpa ekstensi) tidak bisa ada (hanya `.md/.markdown/.txt`) dan lagi pula tidak cocok `docs/%`; tidak ada kasus khusus.
- Tanpa transaksi dan tanpa advisory lock: satu statement atomik (overview 3.5). Konkurensi dengan upload: baris yang ditambahkan setelah snapshot DELETE tidak ikut terhapus; diterima.
- Panggilan bus (check-access sebelum, get-project dan audit sesudah) tidak pernah di dalam tx.
- Isi file tidak pernah dibaca, di-log, atau masuk audit. `Args` kosong; `Result` hanya `count`.

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai (`base/`, `base/store`, `base/access`, `base/audit`, migrasi `knowledge_files`). Cek `ls backend/internal/domain/project/internal/knowledge/base/store/store.go` dan `base/audit/audit.go`; bila belum ada, STOP.

1. Buat folder `backend/internal/domain/project/internal/knowledge/deletefolder/`.
2. `params.go`:
   ```go
   type Input struct {
       ProjectID string
       Path      string // mentah dari query ?path=
       Meta      base.RequestMeta
   }
   type Output struct {
       Deleted int `json:"deleted"`
   }
   ```
3. `repository.go` (SQL khusus feature; tidak ada di `base/store`):
   ```go
   type Repository interface {
       DeleteByPrefix(ctx context.Context, projectID, likePrefix string) (int, error)
   }
   type Repo struct{ s *store.Store }
   func NewRepo(s *store.Store) *Repo
   ```
   Implementasi: `projectID` bukan uuid -> `0, nil` tanpa query. Selain itu satu statement (`$1::uuid`, `$2` text, parameter saja):
   ```sql
   WITH d AS (
     DELETE FROM knowledge_files
     WHERE project_id = $1 AND path LIKE $2 ESCAPE '\'
     RETURNING 1
   )
   SELECT count(*) FROM d
   ```
   dijalankan lewat `s.Q().GetContext`. Alternatif setara: `ExecContext` + `RowsAffected()`; pilih salah satu dan konsisten (CTE dipilih agar tidak bergantung pada perilaku driver).
4. `usecase.go`:
   ```go
   type Usecase struct {
       access access.Checker
       repo   Repository
       audit  audit.Recorder
   }
   func NewUsecase(a access.Checker, r Repository, au audit.Recorder) *Usecase

   func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error) {
       actor, err := u.access.Require(ctx, in.ProjectID)
       if err != nil { return Output{}, err }
       prefix, reason := base.NormalizePrefix(in.Path)
       if reason != "" { return Output{}, base.Invalid(reason) }
       if prefix == "" { return Output{}, base.Invalid(base.ReasonPrefixRequired) }
       n, err := u.repo.DeleteByPrefix(ctx, in.ProjectID, base.LikePrefix(prefix))
       if err != nil { return Output{}, fmt.Errorf("deletefolder: delete by prefix: %w", err) }
       if n == 0 { return Output{}, base.ErrFolderNotFound }
       u.audit.Record(ctx, audit.Event{
           Action: record.ActionKnowledgeFolderDeleted, ProjectID: in.ProjectID, Actor: actor,
           TargetType: "folder", Target: prefix,
           Args: map[string]any{}, Result: map[string]any{"count": n}, Meta: in.Meta,
       })
       return Output{Deleted: n}, nil
   }
   ```
   Tidak ada rule bisnis baru di usecase; semua rule (normalisasi, LIKE escape, alasan, error) di `base/`. Jangan memecah usecase ini lagi.
5. `handler.go`:
   ```go
   type Handler struct{ uc *Usecase }
   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       out, err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           Path:      r.URL.Query().Get("path"),
           Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
       })
       if err != nil { respond.WriteError(w, r, err); return }
       respond.WriteJSON(w, http.StatusOK, out)
   }
   ```
   Sesuaikan signature `respond.WriteError/WriteJSON` dan helper `truncate` dengan kode `main`/feature lain (jangan membuat helper duplikat bila sudah ada di `base`; bila belum ada, definisikan lokal privat).
6. `init.go`:
   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(access.New(d.Bus), NewRepo(store.New(d.DB)), audit.New(d.Bus, d.Log))
       h := &Handler{uc: uc}
       d.API.With(middleware.RequireAuth()).
           Delete("/projects/{projectID}/knowledge/folders", h.ServeHTTP)
   }
   ```
   Sesuaikan pola pendaftaran dengan feature T03/`listtree` di `main`.
7. Tambah satu baris `deletefolder.Init(d)` di `.../knowledge/base.go` (`Register`). Jangan menyentuh baris Init feature lain.
8. Pastikan konstanta `record.ActionKnowledgeFolderDeleted` (`knowledge.folder.deleted`) ada di `lib/contracts/core/audit/record/actions.go` (sudah dari T02). Tidak menambah aksi baru.
9. Tulis test (bagian 6). Jalankan `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/deletefolder/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/...`; `internal/archtest` harus lulus (feature tidak meng-import feature lain, SQL hanya di repository).
10. Catat EXPLAIN DELETE (bagian 6, integrasi no. 10) di deskripsi PR.

Aturan arsitektur: feature hanya meng-import `base`, `base/store`, `base/access`, `base/audit`; usecase menerima interface; SQL hanya di `repository.go`; interaksi lintas module hanya via bus (check-access, get-project, audit record).

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari `backend/migrations/project/<VERSION>_create_knowledge_files.sql` (foundation). Indeks `knowledge_files_project_path_prefix (project_id, path text_pattern_ops)` melayani `LIKE 'prefix/%'` pada DELETE; hapus baris juga otomatis memperbarui GIN `search`.

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `.../knowledge/deletefolder/init.go` | baru: rakit access, repo, audit, usecase, handler; daftarkan route DELETE |
| `.../knowledge/deletefolder/handler.go` | baru: baca `projectID`, `path`, meta request; 200 `{deleted}` atau error |
| `.../knowledge/deletefolder/params.go` | baru: `Input`, `Output` |
| `.../knowledge/deletefolder/usecase.go` | baru: `Usecase.Execute` (access -> normalisasi -> delete -> 404/audit) |
| `.../knowledge/deletefolder/repository.go` | baru: interface `Repository`, `Repo.DeleteByPrefix` |
| `.../knowledge/deletefolder/usecase_test.go` | baru: unit dengan repository, checker, recorder palsu |
| `.../knowledge/deletefolder/handler_test.go` | baru: httptest + router chi |
| `.../knowledge/deletefolder/repository_integration_test.go` | baru (tag `integration`): DB nyata |
| `.../knowledge/base.go` | tambah baris `deletefolder.Init(d)` |
| `api/modules/project/knowledge.yml` | path item `delete` pada `/api/projects/{projectID}/knowledge/folders` (dibuat di foundation; verifikasi sesuai bagian 8, termasuk schema `KnowledgeDeleteFolderResult`) |

Tidak mengubah `base/`, migrasi, config, README, atau feature lain (README/tiket diperbarui di langkah F/W).

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, repository/checker/recorder palsu)

- Checker `ErrProjectNotFound` -> error itu; repository dan recorder TIDAK dipanggil, walau `Path` tidak valid (`../x`) atau kosong (non-anggota tetap 404).
- Checker error infra -> error terbungkus; repo/recorder tidak dipanggil.
- `Path` kosong, `"/"`, dan hanya spasi? Kosong dan `"/"` -> 400 `INVALID_ARGUMENT` dengan `details.reason = prefix_required` (atau `path_absolute` untuk `"/"` bila `NormalizePrefix` menolaknya lebih dulu; asersi: 400 dengan salah satu dari dua alasan itu). Repository TIDAK dipanggil.
- `../x`, `/docs`, `docs//a`, `a/./b`, NUL -> 400 dengan reason `path_traversal`, `path_absolute`, `path_empty_segment`, `path_traversal`, `path_control_char`; repository tidak dipanggil.
- `docs` dan `docs/` -> repository menerima `likePrefix == "docs/%"` (keduanya); audit `Target == "docs"`.
- `a_b%c\d` -> repository menerima `a\_b\%c\\d/%`.
- Repository mengembalikan 0 -> `base.ErrFolderNotFound` (404); recorder TIDAK dipanggil.
- Repository error -> error terbungkus `deletefolder: delete by prefix: ...`; recorder tidak dipanggil.
- Repository mengembalikan 3 -> `Output{Deleted: 3}`; recorder dipanggil TEPAT sekali dengan `Action = knowledge.folder.deleted`, `TargetType = folder`, `Target = "docs"`, `Result["count"] == 3`, `Args` kosong, `Actor` dari checker, `Meta` diteruskan; `Result` tidak memuat string lain selain `count`.
- Recorder palsu yang gagal (fail-open ada di `base/audit`; di sini recorder tidak mengembalikan error) -> usecase tetap sukses.

### Unit repository/`LikePrefix` (di foundation `base/path_test.go`; verifikasi mencakup)

- `LikePrefix("docs") == "docs/%"`, `LikePrefix("a_b%c\\d") == "a\\_b\\%c\\\\d/%"`.

### Unit handler (`handler_test.go`)

- `DELETE .../folders?path=docs` -> 200 `{"deleted":3}`; usecase menerima `Path == "docs"`.
- `?path=docs%2Fauth` -> `Path == "docs/auth"`.
- Tanpa `path` -> 400 `prefix_required`.
- `projectID` bukan uuid -> 404 `project tidak ditemukan`.
- Tanpa sesi -> 401 (`RequireAuth`); tanpa header CSRF -> 403 `CSRF_REQUIRED` (router T00 asli di test integrasi/W).
- Format error seragam dengan `trace_id`.
- `IP` dan `UserAgent` (dipotong 512) sampai ke `Input.Meta`.

### Integrasi (`repository_integration_test.go` dan W `knowledge_integration_test.go`, tag `integration`, `db.TestDB`)

Fixture: dua project, file disisipkan via SQL test atau `createfile`; handler `check-access` T03 asli atau palsu; `record` palsu yang mencatat entri.

1. **AC6 inti**: file `docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`, `docs/auth/flow/deep/er/y.md`, ditambah `docs2/x.md`, `docs.md`, `docs-old/y.md`, `readme.md`. `DELETE ?path=docs` -> 200 `{deleted: 4}`; sisa tepat 4 file (`docs2/x.md`, `docs.md`, `docs-old/y.md`, `readme.md`); `docs/**` pada semua kedalaman hilang.
2. **Project lain utuh**: project B memiliki `docs/a.md` (path sama) dan `docs/b.md`; setelah hapus `docs` di project A, jumlah file project B tidak berubah.
3. **Escaping `_`**: `a_b/x.md`, `axb/y.md`; hapus `a_b` -> hanya `a_b/x.md` hilang (`axb/y.md` tetap).
4. **Escaping `%`**: `100%/z.md`, `100x/w.md`, `100/v.md`; hapus `100%` -> hanya `100%/z.md` hilang.
5. **Escaping `\`**: `back\slash/q.md`, `back/slash/q.md`; hapus `back\slash` -> hanya milik backslash hilang.
6. **Subfolder saja**: hapus `docs/auth` -> `docs/a.md` tetap, `docs/auth/**` hilang; hapus `docs/auth/flow/deep` -> hanya isinya.
7. **Case-sensitive**: `Docs/x.md` dan `docs/x.md`; hapus `docs` -> `Docs/x.md` tetap.
8. **Root ditolak**: `?path=` kosong atau tanpa param -> 400 `prefix_required`; jumlah file project tidak berubah; 0 entri audit.
9. **Tidak ada folder / prefix tanpa file**: `?path=tidak/ada` -> 404 `folder tidak ditemukan atau kosong`; 0 entri audit. Hapus `docs` dua kali berturut-turut: kedua kali 404 (idempoten dalam arti aman).
10. **Non-anggota 404**: user non-anggota (bukan super_admin) -> 404 `project tidak ditemukan`, tidak ada file terhapus, 0 audit; `projectID` bukan uuid -> 404; super_admin non-anggota -> 200 (AC6).
11. **Audit**: sukses -> tepat 1 entri `knowledge.folder.deleted`, `Target = "docs"`, `TargetType = folder`, `Result.count = 4`, `ProjectLabel` dari get-project; gagal (404/400) -> 0 entri; string isi unik di salah satu file tidak muncul di `Args`/`Result` entri (rule 13.6). Recorder yang gagal -> respons tetap 200 dan file tetap terhapus (fail-open).
12. **Normalisasi slash akhir**: `?path=docs/` setara `docs`; `?path=docs//` -> 400 `path_empty_segment`; `?path=../x` -> 400 `path_traversal`; `?path=/docs` -> 400 `path_absolute`.
13. **Index/FTS ikut bersih**: setelah hapus, `search` kata dari file terhapus mengembalikan 0 hasil dan `tree` tidak lagi menampilkan folder itu.
14. **EXPLAIN (rule 8.8)**: isi 5000 baris di beberapa project/folder, `ANALYZE knowledge_files`, lalu `EXPLAIN` statement DELETE (di dalam transaksi yang di-rollback) untuk prefix `docs/` non-root: plan memakai `knowledge_files_project_path_prefix` (Index Scan / Bitmap Index Scan), bukan Seq Scan. Simpan keluaran di deskripsi PR.
15. **Cascade tidak terkait**: tidak ada; hapus folder tidak menyentuh tabel lain (asersi bahwa `projects` dan anggota tidak berubah).

Registrasi: `TestRoutesRegistered` di W memastikan route DELETE `/api/projects/{projectID}/knowledge/folders` terdaftar.

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Setelah upload folder `docs/` (3 tingkat) dan `archive/`, buka tab Knowledge, klik aksi hapus pada folder `archive`, dialog konfirmasi menampilkan nama folder dan jumlah file (dari `file_count`), konfirmasi -> `archive` hilang dari pohon, `docs/` tetap lengkap.
2. Hapus folder `docs/auth` -> `docs/a.md` tetap tampil; `docs/auth/login.md` tidak bisa dibuka lagi (404 di viewer); pencarian `login` tidak lagi menemukannya.
3. Folder sejenis (`docs2`, file `docs.md`) tetap ada setelah hapus `docs`.
4. Super Admin (bukan anggota) menghapus folder -> berhasil.
5. Entri audit `knowledge.folder.deleted` muncul di halaman audit log (target = path folder, `count` benar).

Negatif:
1. Batal pada dialog konfirmasi -> tidak ada request DELETE, pohon tidak berubah.
2. Developer non-anggota memanggil `page.request.delete(.../folders?path=docs)` dengan header CSRF -> 404; file project tetap ada.
3. Request `DELETE .../folders` tanpa `path` (via `page.request`) -> 400 `prefix_required`; seluruh isi project utuh (tidak ada tombol "hapus root" di UI).
4. Folder sudah dihapus di tab lain, lalu dihapus lagi di tab pertama -> 404 dan UI menampilkan pesan "folder sudah tidak ada" lalu menyegarkan pohon, tanpa crash.
5. `?path=../x` -> 400 `path_traversal`.
6. Request DELETE tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401.

## 8. Request/Response body

Permintaan: `DELETE /api/projects/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f/knowledge/folders?path=docs`
Header: `X-Requested-With: grimoire`, cookie sesi. Tanpa body.

200:

```json
{ "deleted": 3 }
```

400 root (tanpa `path`, `path=` kosong):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "path folder wajib diisi; root tidak bisa dihapus sekaligus",
    "details": { "reason": "prefix_required" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

400 path tidak valid (`?path=../x`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "path tidak boleh memuat segmen . atau ..",
    "details": { "reason": "path_traversal" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

Alasan 400 lain: `path_absolute`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`, `prefix_required`.

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

403 tanpa header CSRF:

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib diisi" }, "trace_id": "..." }
```

(teks pesan mengikuti middleware CSRF T00; asersi test pada `code`.)

404 non-anggota atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

404 folder tidak ada / kosong:

```json
{ "error": { "code": "NOT_FOUND", "message": "folder tidak ditemukan atau kosong" }, "trace_id": "..." }
```

500 (check-access atau DB gagal): error generik tanpa detail internal, mis. `{"error":{"code":"INTERNAL","message":"terjadi kesalahan"},"trace_id":"..."}` (format `respond` T00).

Bus (dikonsumsi, bukan wire JSON):

```go
// base/access (sebelum aksi)
bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
    checkaccess.Request{ProjectID: pid, UserID: actor.ID, UserRole: actor.Role})
// base/audit (setelah DELETE sukses; get-project fail-soft, record fail-open)
bus.CallTyped[getproject.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: pid})
bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
```

Audit (`record.Request` yang dikirim):

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000a", ActorLabel: "Budi",
    ProjectID: pid, ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeFolderDeleted, TargetType: "folder", Target: "docs",
    Args:   map[string]any{},
    Result: map[string]any{"count": 3},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...",
}
```
