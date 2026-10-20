# T05 project/knowledge - Usecase `deletefile` (Plan)

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (selanjutnya "overview"). Bila bertentangan, overview menang. Ini usecase U7, gelombang B.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Usecase | `deletefile` (U7) |
| Scope | fullstack (plan ini: backend; UI tombol hapus + konfirmasi ada di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Folder feature | `backend/internal/domain/project/internal/knowledge/deletefile` |
| Endpoint | `/api/projects/{projectID}/knowledge/files?path=<path>` |
| Method | `DELETE` |
| Version | Tanpa prefix versi (prefix `/api`); OpenAPI `api/modules/project/knowledge.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; CSRF global (`X-Requested-With: grimoire`) |
| Otorisasi | `member`, `owner`, `admin` (via bus `project.management.check-access`); non-anggota 404 |
| Body limit | default (tanpa body) |
| Bus dikonsumsi | `project.management.check-access` (fail-closed), `project.management.get-project` (fail-soft, lewat `base/audit`), `core.audit.record` (fail-open, setelah commit) |
| Bus disediakan | tidak ada |
| Audit | `knowledge.file.deleted`, `TargetType=file`, `Target=path`, `Args={}`, `Result={"bytes": n}` |
| Rule | Hard delete (rule 8.2): tidak ada soft delete, tidak ada tombstone |
| Bergantung pada | foundation F (`base/`, `base/access`, `base/audit`, migrasi `knowledge_files`, aksi audit) |
| AC terkait | AC6 (isolasi/non-anggota), AC8 (tidak menyentuh project lain) |

---

## 2. Summary Plan

`deletefile` menghapus tepat satu file knowledge dari satu project berdasarkan path persis.

Alur usecase (orkestrasi saja, rule 5.x):
1. `access.Require(ctx, projectID)` -> actor. Non-anggota / project tidak ada / `projectID` bukan uuid -> 404; tanpa identitas -> 401; gagal bus -> 500 (fail-closed). Dilakukan SEBELUM validasi input lain.
2. `base.NormalizePath(path)`; ditolak -> 400 `INVALID_ARGUMENT` dengan `details.reason` (`path_empty`, `path_absolute`, `path_traversal`, ...).
3. `repo.DeleteByPath(ctx, projectID, path)` -> satu statement `DELETE FROM knowledge_files WHERE project_id=$1 AND path=$2 RETURNING size_bytes`. 0 baris -> `base.ErrFileNotFound` (404).
4. Setelah statement sukses (autocommit, satu statement = atomik; tidak perlu tx eksplisit) panggil `audit.Record` `knowledge.file.deleted {bytes}`. Fail-open: kegagalan audit hanya di-log.
5. Handler membalas `204 No Content` tanpa body.

Poin penting:
- Semua SQL memakai `project_id = $1` (rule 6.10): file bernama sama di project lain tidak pernah tersentuh.
- Path dipakai apa adanya (peka huruf besar/kecil, tidak di-trim). Tidak ada penghapusan berdasarkan prefix di usecase ini (itu `deletefolder`).
- Tidak ada pengecekan `version`/optimistic concurrency: hapus idempoten secara semantik; panggilan kedua mendapat 404.
- Tanpa tx eksplisit dan tanpa advisory lock (overview 3.5: `deletefile` satu statement `DELETE ... RETURNING`).
- Audit hanya metadata (path, byte). Isi file tidak pernah dibaca, di-log, atau diaudit (rule 10.3, 13.6): query tidak memilih `content`.
- Penolakan 400/404 tidak menulis audit.

Usecase tidak dipecah: satu usecase, satu repository method, satu handler.

---

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai (`base/path.go`, `base/errors.go`, `base/meta.go`, `base/reasons.go`, `base/store` tidak dipakai di sini, `base/access`, `base/audit`, migrasi `knowledge_files`, konstanta `ActionKnowledgeFileDeleted` sudah ada di `record`). Cek: `ls backend/internal/domain/project/internal/knowledge/base/access`.

Langkah:

1. **Folder feature**: buat `backend/internal/domain/project/internal/knowledge/deletefile/` dengan file `init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go` dan test `usecase_test.go`, `handler_test.go`, `repository_integration_test.go`.

2. **`params.go`** (parsing + DTO input, tanpa rule bisnis):
   ```go
   package deletefile

   type params struct {
       ProjectID string
       Path      string // r.URL.Query().Get("path"), mentah
   }
   ```
   Tidak ada body. Tidak ada response DTO (204).

3. **`repository.go`** (satu-satunya yang menyentuh SQL):
   ```go
   package deletefile

   type Repository interface {
       // DeleteByPath menghapus satu baris. found=false bila 0 baris. sizeBytes = size_bytes baris yang dihapus.
       DeleteByPath(ctx context.Context, projectID, path string) (sizeBytes int, found bool, err error)
   }

   type repo struct{ q db.Querier }

   func newRepo(d *db.DB) *repo { return &repo{q: d} }
   ```
   SQL (parameter saja, tanpa interpolasi):
   ```sql
   DELETE FROM knowledge_files
   WHERE project_id = $1 AND path = $2
   RETURNING size_bytes
   ```
   Implementasi: `uuid.Parse(projectID)` gagal -> `(0, false, nil)` tanpa query (jaring pengaman; usecase sudah menjamin 404 lewat access). Pakai `QueryRowxContext(...).Scan(&size)`; `sql.ErrNoRows` -> `found=false`; error lain dibungkus `fmt.Errorf("deletefile: delete: %w", err)`. Repository tidak membuat keputusan 404 (itu usecase).

4. **`usecase.go`** (orkestrasi; menerima interface, rule 5.6):
   ```go
   type Usecase struct {
       access access.Checker
       repo   Repository
       audit  audit.Recorder
   }

   func NewUsecase(a access.Checker, r Repository, au audit.Recorder) *Usecase

   func (u *Usecase) Execute(ctx context.Context, p params, meta base.RequestMeta) error {
       actor, err := u.access.Require(ctx, p.ProjectID)
       if err != nil { return err }

       path, reason := base.NormalizePath(p.Path)
       if reason != "" { return base.Invalid(reason) }

       size, found, err := u.repo.DeleteByPath(ctx, p.ProjectID, path)
       if err != nil { return fmt.Errorf("deletefile: %w", err) }
       if !found { return base.ErrFileNotFound }

       u.audit.Record(ctx, audit.Event{
           Action:     record.ActionKnowledgeFileDeleted,
           ProjectID:  p.ProjectID,
           Actor:      actor,
           TargetType: "file",
           Target:     path,
           Args:       map[string]any{},
           Result:     map[string]any{"bytes": size},
           Meta:       meta,
       })
       return nil
   }
   ```
   Tidak ada logika bisnis di sini selain urutan; validasi path ada di entity (`base.NormalizePath`). `Record` tidak mengembalikan error (fail-open dijamin `base/audit`).

5. **`handler.go`**:
   ```go
   type handler struct{ uc *Usecase }

   func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
       p := params{
           ProjectID: chi.URLParam(r, "projectID"),
           Path:      r.URL.Query().Get("path"),
       }
       meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
       if err := h.uc.Execute(r.Context(), p, meta); err != nil {
           respond.WriteError(w, r, err)
           return
       }
       w.WriteHeader(http.StatusNoContent)
   }
   ```
   `truncate` adalah helper lokal kecil (potong rune ke 512), sama seperti feature lain; jangan import dari feature lain (rule 4.3). Bila helper sudah ada di `base`, pakai itu.

6. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(access.New(d.Bus), newRepo(d.DB), audit.New(d.Bus, d.Log))
       h := &handler{uc: uc}
       d.API.With(middleware.RequireAuth()).
           Delete("/projects/{projectID}/knowledge/files", h.serve)
   }
   ```
   Tanpa `BodyLimit` khusus. Catatan: route `DELETE .../files` berbagi pola path dengan `POST/GET/PUT .../files` milik feature lain; chi mengizinkan method berbeda pada pola yang sama (tiap feature mendaftarkan method-nya sendiri). Pastikan tidak ada `Handle` tanpa method yang bentrok.

7. **Wiring**: tambah satu baris `deletefile.Init(d)` di `backend/internal/domain/project/internal/knowledge/base.go` (`Register`). Registrasi ganda = panic, jadi hanya satu baris.

8. **OpenAPI** (foundation sudah membuat `knowledge.yml`; verifikasi bahwa operasi DELETE `/api/projects/{projectID}/knowledge/files` ada): query `path` wajib, respons `204`, `400`, `401`, `404`, `500`; header `X-Requested-With`. Bila belum ada, tambahkan lalu `make api-bundle && make api-check`.

9. **Jalankan**: `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/deletefile/... && go test -tags integration ./internal/domain/project/internal/knowledge/...` dan `internal/archtest` lulus (feature tidak meng-import feature lain).

Kontrak bus: tidak ada kontrak baru. Yang dipakai (sudah didefinisikan di overview/T02/T03):
- `bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, checkaccess.Request{ProjectID, UserID, UserRole})` melalui `access.BusChecker`.
- `bus.CallTyped[getproject.Response](... BUS_PROJECT_MANAGEMENT_GET_PROJECT ...)` dan `bus.CallTyped[record.Response](... BUS_CORE_AUDIT_RECORD, record.Request{...})` melalui `audit.BusRecorder`, setelah statement DELETE sukses, tidak pernah di dalam tx.

---

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari migrasi foundation `backend/migrations/project/20261024090000000_create_knowledge_files.sql`. Indeks `UNIQUE (project_id, path)` (`knowledge_files_project_id_path_key`) melayani `WHERE project_id=$1 AND path=$2`.

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/deletefile/init.go` | baru: rakit repo, access, audit, usecase, handler; daftarkan `DELETE /projects/{projectID}/knowledge/files` |
| `.../deletefile/handler.go` | baru: baca `projectID` dan `path`, bangun `RequestMeta`, panggil usecase, `204` atau `respond.WriteError` |
| `.../deletefile/params.go` | baru: struct `params` |
| `.../deletefile/usecase.go` | baru: `Usecase.Execute` (access -> NormalizePath -> DeleteByPath -> audit) |
| `.../deletefile/repository.go` | baru: interface `Repository` + implementasi `DELETE ... RETURNING size_bytes` |
| `.../deletefile/usecase_test.go` | baru: unit usecase dengan repository/access/audit palsu |
| `.../deletefile/handler_test.go` | baru: httptest + router chi + `RequireAuth` palsu |
| `.../deletefile/repository_integration_test.go` | baru (tag `integration`): SQL nyata via `db.TestDB` |
| `backend/internal/domain/project/internal/knowledge/base.go` | tambah `deletefile.Init(d)` |
| `backend/internal/domain/project/internal/knowledge/knowledge_integration_test.go` | tambah skenario `deletefile` (lihat bagian 6, integrasi); file milik tahap W, bila belum ada cukup test integrasi di folder feature |
| `api/modules/project/knowledge.yml`, `api/openapi.yml` | verifikasi/tambah operasi DELETE files (hanya bila belum ada) |
| `docs/tickets/T05-project-knowledge.md` | tidak diubah oleh usecase ini |

Tidak diubah: `base/*`, feature lain, `lib/contracts`, `lib/constants`, config, migrasi.

---

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, repository/access/audit palsu, table-driven)

| # | Skenario | Setup | Harapan |
|---|---|---|---|
| U1 | Sukses | access ok, repo `(1234, true, nil)`, path `docs/a.md` | err nil; repo dipanggil dengan `(projectID, "docs/a.md")`; audit direkam tepat 1x: Action `knowledge.file.deleted`, TargetType `file`, Target `docs/a.md`, Result `{"bytes":1234}`, Args kosong, Actor dari access, Meta diteruskan |
| U2 | File tidak ada | repo `(0, false, nil)` | err `errors.Is(err, base.ErrFileNotFound)` (Code NOT_FOUND); audit 0x |
| U3 | Non-anggota | access mengembalikan `base.ErrProjectNotFound` | err sama; repo 0 panggilan; audit 0x |
| U4 | Urutan: access sebelum validasi | access 404 dan path `""` | hasil 404 (bukan 400) |
| U5 | Tanpa identitas | access `base.ErrUnauthenticated` | 401; repo/audit tidak dipanggil |
| U6 | access gagal infra | access error biasa | error dikembalikan (500 generik); repo tidak dipanggil |
| U7 | Path tidak valid | tabel: `""`->`path_empty`, `/a.md`->`path_absolute`, `../x.md`->`path_traversal`, `a//b.md`->`path_empty_segment`, `a\x00.md`->`path_control_char`; 513 rune->`path_too_long` | `*errs.Error` CodeInvalid dengan `details.reason` sesuai; `errors.Is(err, errs.ErrInvalid)`; repo 0 panggilan; audit 0x |
| U8 | Error repository | repo error DB | error terbungkus (`deletefile:`), tanpa audit |
| U9 | Audit fail-open | audit palsu yang "gagal" (Record tidak mengembalikan error) | Execute tetap nil |
| U10 | Path dipakai apa adanya | path `Docs/A.MD` | repo menerima `Docs/A.MD` persis (tanpa lowercase/trim) |
| U11 | Isi file tidak bocor | semua panggilan audit | `Args`/`Result` hanya berisi kunci `bytes` |

### Handler (`handler_test.go`, httptest + chi)

- `DELETE /projects/{id}/knowledge/files?path=docs%2Fa.md` sukses -> `204`, body kosong; path ter-decode benar (`docs/a.md`) sampai ke usecase.
- Usecase mengembalikan `ErrFileNotFound` -> `404` format error standar dengan `trace_id`.
- `Invalid(path_traversal)` -> `400` dengan `details.reason`.
- Parameter `path` tidak ada -> usecase menerima `""` -> `400 path_empty`.
- Error tak dikenal -> `500` tanpa bocor detail.

### Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

- Insert fixture lewat SQL test (dua project A dan B, keduanya punya `same.md`). `DeleteByPath(A, "same.md")` -> `(size_A, true, nil)`; baris A hilang; baris B `same.md` tetap (hitung via `SELECT count(*)`).
- Path tidak ada -> `(0, false, nil)`.
- Peka huruf: hapus `Docs/A.md` tidak menghapus `docs/a.md`.
- `projectID` bukan uuid -> `(0, false, nil)` tanpa error.
- Hapus sebuah file tidak menghapus file lain di folder yang sama (`docs/a.md` vs `docs/b.md`, juga `docs/a.md` vs `docs/a.md.txt`).
- Path ber-karakter `%` dan `_` diperlakukan literal (equality, bukan LIKE): hapus `a_b.md` tidak menghapus `axb.md`.
- Hapus dua kali: kedua kali `found=false`.
- Hard delete: setelah hapus, `SELECT count(*) FROM knowledge_files WHERE project_id=$1 AND path=$2` = 0 (tidak ada baris tombstone/kolom `deleted_at`).

### Integrasi lintas komponen (router `/api` asli + bus dengan `check-access`/`get-project` palsu dan `record` palsu penghitung entri)

1. **Sukses**: anggota membuat file (fixture), `DELETE ...?path=docs/a.md` dengan header `X-Requested-With: grimoire` -> 204; `GET getfile` setelahnya 404; tepat 1 entri audit `knowledge.file.deleted` dengan `Target=docs/a.md`, `Result.bytes` = ukuran, `ProjectLabel` dari `get-project`.
2. **Not found**: path tidak ada -> 404 `NOT_FOUND` "file tidak ditemukan"; 0 entri audit.
3. **Isolasi project**: project A dan B sama-sama punya `same.md`; hapus di A -> 204; `GET` di B masih 200 dengan isi B; hitungan file B tidak berubah.
4. **Non-anggota**: user bukan anggota -> 404 "project tidak ditemukan" (bukan 403, rule 7.3); file tetap ada (dicek lewat anggota/SQL); 0 entri audit. Termasuk `projectID` bukan uuid -> 404.
5. **Tanpa sesi** -> 401; **tanpa CSRF header** -> 403 `CSRF_REQUIRED`; file tetap ada.
6. **Super admin non-anggota** (check-access level `admin`) -> 204.
7. **Audit gagal fail-open**: `record` palsu mengembalikan error -> respons tetap 204, file terhapus, error di-log (`audit write failed`).
8. **Isi tidak bocor**: file berisi string unik `RAHASIA-XYZ`; setelah hapus, tidak ada `Args`/`Result` entri audit yang memuat string itu (rule 13.6).
9. **Cascade tidak terpengaruh**: hapus satu file tidak menghapus file lain di project yang sama (tree menampilkan sisanya).

---

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01, T02, T03 merge)

Positif:
1. Developer anggota project mengunggah folder `docs/` (3 file), membuka tab Knowledge, memilih `docs/auth/login.md`, klik Hapus, konfirmasi dialog -> file hilang dari pohon, folder lain tetap, tampil toast sukses; membuka URL file itu langsung menampilkan "file tidak ditemukan".
2. Setelah hapus, entri `knowledge.file.deleted` muncul di halaman audit log (Super Admin) dengan target path dan ukuran byte.
3. Hapus file lalu buat ulang file dengan path yang sama -> berhasil (201), karena hard delete tidak meninggalkan tombstone.
4. Super Admin (bukan anggota) membuka knowledge project dan menghapus file -> berhasil.

Negatif:
1. Batalkan dialog konfirmasi -> tidak ada request DELETE, file tetap ada.
2. Dua tab membuka file yang sama; tab pertama menghapus; tab kedua menghapus lagi -> tampil pesan "file tidak ditemukan" (404) dan pohon dimuat ulang tanpa file itu; tidak ada crash.
3. Developer non-anggota memanggil `DELETE` lewat `page.request` (dengan CSRF header) untuk project lain -> 404; file tetap ada bagi anggota.
4. `DELETE` tanpa header `X-Requested-With` lewat `page.request` -> 403 `CSRF_REQUIRED`.
5. `DELETE ...?path=../x.md` dan `?path=` (kosong) lewat `page.request` -> 400 dengan `details.reason` `path_traversal` / `path_empty`.
6. Dua project punya `same.md`; hapus di project A -> `same.md` di project B tetap tampil dan terbaca.
7. Tanpa login -> 401 dan UI mengarahkan ke login.

---

## 8. Request/Response body

Request (tanpa body):

```
DELETE /api/projects/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f/knowledge/files?path=docs%2Fauth%2Flogin.md
X-Requested-With: grimoire
Cookie: <sesi>
```

### 204 No Content

Tanpa body.

### 400 path tidak valid

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

Kasus `path` kosong:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "path wajib diisi, mis. docs/auth/login.md",
    "details": { "reason": "path_empty" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

Nilai `details.reason` lain yang mungkin: `path_absolute`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`.

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

### 403 tanpa header CSRF

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib untuk request mutasi" }, "trace_id": "..." }
```

(Pesan persis dari middleware T00; yang dijamin hanya `code`.)

### 404 file tidak ditemukan

```json
{ "error": { "code": "NOT_FOUND", "message": "file tidak ditemukan" }, "trace_id": "..." }
```

### 404 non-anggota / project tidak ada / projectID bukan uuid

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

### 500

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "..." }
```

### Audit yang dikirim (bus `core.audit.record`, bukan wire JSON)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000a", ActorLabel: "Budi",
    ProjectID: "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f", ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeFileDeleted, // "knowledge.file.deleted"
    TargetType: "file", Target: "docs/auth/login.md",
    Args:   map[string]any{},
    Result: map[string]any{"bytes": 1234},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "...",
}
```

---

## 9. Risiko dan catatan

- Hard delete tidak bisa dibatalkan (sesuai rule 8.2 dan tiket). UI wajib meminta konfirmasi; backend tidak punya konfirmasi.
- `DELETE` + `RETURNING` autocommit: bila proses mati tepat setelah commit dan sebelum audit, entri audit hilang (fail-open yang disepakati README).
- Query string `path` yang di-encode ganda (`%252F`) diperlakukan sebagai literal `%2F` dan akan 404; tidak ada normalisasi diam-diam (D11).
- Nama kode error 500 (`INTERNAL`) dan pesan CSRF mengikuti T00; sesuaikan contoh dengan `respond`/`middleware` di `main` bila beda.
