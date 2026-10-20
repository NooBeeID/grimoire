# T05 project/knowledge — Usecase Plan: `listtree`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (foundation). Bila bertentangan, overview menang.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (bagian ini backend; UI pohon di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Feature | `listtree` (gelombang A, U3) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | `/api/projects/{projectID}/knowledge/tree?prefix=` |
| Method | GET |
| Version | tanpa prefix versi URL (prefix `/api`); OpenAPI `info.version` 0.1.0; tidak ada kontrak bus baru |
| Auth | `middleware.RequireAuth()`; otorisasi project lewat `base/access` (member, owner, admin) |
| Body limit | default |
| Audit | tidak ada (read-only) |
| AC | AC1, AC6 |

Bus: mengonsumsi `project.management.check-access` (fail-closed, 500) lewat `base/access`. Tidak menyediakan endpoint bus.

## 2. Summary Plan

Mengembalikan SATU level pohon folder/file untuk satu `prefix` (lazy; klien meminta level berikutnya saat folder dibuka). Alur usecase (hanya orkestrasi):

1. `access.Require(ctx, projectID)`: non-anggota atau `projectID` bukan uuid -> 404; tanpa sesi -> 401. Dilakukan SEBELUM validasi prefix.
2. `base.NormalizePrefix(raw)`: kosong = root; satu `/` akhir dibuang; gagal -> 400 `base.Invalid(reason)`.
3. `repo.ListMeta(ctx, projectID, base.LikePrefix(prefix))`: SQL hanya `path, size_bytes, updated_at` (tanpa `content`), `project_id = $1 AND path LIKE $2 ESCAPE '\'`, `ORDER BY path COLLATE "C"`.
4. `base.BuildLevel(prefix, metas)`: rule murni pengelompokan folder/file satu level, `file_count` rekursif, sort per nama.
5. 200 `Level`. Prefix non-root tanpa file -> 200 kosong (`folders: []`, `files: []`, `total_files: 0`), bukan 404 (UI yang menangani).

Keamanan dan kebenaran: `LikePrefix` meng-escape `\ % _` dan menambah `/%` sehingga `docs` tidak cocok dengan `docs2/x.md` maupun `docs.md`; query selalu terikat `project_id`; `content` tidak pernah dibaca. Semua rule ada di `base/` (sudah dibuat di foundation); feature ini tidak menambah rule, hanya menyusun.

Catatan performa: indeks `knowledge_files_project_path_prefix (project_id, path text_pattern_ops)` melayani `LIKE 'prefix/%'`. Untuk root (`%`) seluruh file project dipindai lewat indeks `project_id` (wajar; hanya 3 kolom kecil). `ORDER BY path COLLATE "C"` selaras dengan `text_pattern_ops` (byte order) sehingga tidak butuh sort tambahan.

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai (`base/`, `base/store`, `base/access`, migrasi tabel). Cek `ls backend/internal/domain/project/internal/knowledge/base/store/store.go`; bila belum ada, STOP.

1. Buat folder `backend/internal/domain/project/internal/knowledge/listtree/`.
2. `params.go`: tidak ada body; handler membaca `r.URL.Query().Get("prefix")`. Definisikan tipe input usecase:
   ```go
   type Input struct {
       ProjectID string
       Prefix    string // mentah dari query
   }
   ```
   (letakkan di `usecase.go`; `params.go` tidak diperlukan karena tidak ada body, namun boleh dibuat berisi `Input` bila ingin mengikuti pola feature lain. Pilih satu: taruh `Input` di `params.go`.)
3. `repository.go`: interface kecil yang dibutuhkan usecase, dipenuhi `*store.Store` langsung (tanpa SQL khusus feature):
   ```go
   type Repository interface {
       ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error)
   }
   ```
   Tidak perlu `repository.go` terpisah bila interface ini diletakkan di `usecase.go` (overview 3.2 mengizinkan). Putuskan: interface di `usecase.go`, tanpa `repository.go`.
4. `usecase.go`:
   ```go
   type Usecase struct {
       access access.Checker
       repo   Repository
   }
   func NewUsecase(a access.Checker, r Repository) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (base.Level, error) {
       if _, err := u.access.Require(ctx, in.ProjectID); err != nil { return base.Level{}, err }
       prefix, reason := base.NormalizePrefix(in.Prefix)
       if reason != "" { return base.Level{}, base.Invalid(reason) }
       metas, err := u.repo.ListMeta(ctx, in.ProjectID, base.LikePrefix(prefix))
       if err != nil { return base.Level{}, fmt.Errorf("listtree: list meta: %w", err) }
       return base.BuildLevel(prefix, metas), nil
   }
   ```
   Tidak ada audit, tidak ada transaksi, tidak ada panggilan bus selain lewat `access`.
5. `handler.go`:
   ```go
   type Handler struct{ uc *Usecase }
   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       lvl, err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           Prefix:    r.URL.Query().Get("prefix"),
       })
       if err != nil { respond.WriteError(w, r, err); return }
       respond.WriteJSON(w, http.StatusOK, lvl)
   }
   ```
   Sesuaikan nama/signature `respond.WriteError`/`WriteJSON` dengan kode `main` (cek `backend/internal/infra/http/respond`). `Level` sudah bertag JSON snake_case dan slice tidak pernah nil.
6. `init.go`:
   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(access.New(d.Bus), store.New(d.DB))
       h := &Handler{uc: uc}
       d.API.With(middleware.RequireAuth()).
           Get("/projects/{projectID}/knowledge/tree", h.ServeHTTP)
   }
   ```
   Sesuaikan pola pendaftaran route dengan `management` (T03 D4: pola penuh di bawah `d.API`, sama seperti feature T03 di `main`).
7. Tambah satu baris `listtree.Init(d)` di `backend/internal/domain/project/internal/knowledge/base.go` (`Register`). Jangan menyentuh baris Init feature lain.
8. Pastikan `BuildLevel` dan `ListMeta` dari foundation sesuai bagian 4.8 dan 5.7 overview; bila ada celah (mis. `ListMeta` belum `COLLATE "C"`), perbaiki di `base/store` dengan test, bukan di feature.
9. Tulis test (bagian 6). Jalankan `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/listtree/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/...`.
10. Catat output EXPLAIN (bagian 6) di deskripsi PR.

Tidak ada kontrak bus baru. Aturan arsitektur: feature tidak meng-import feature lain; hanya `base`, `base/store`, `base/access`; usecase menerima interface; SQL hanya di `base/store`.

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dan indeks `knowledge_files_project_path_prefix` dari `backend/migrations/project/20261024090000000_create_knowledge_files.sql` (foundation).

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `.../knowledge/listtree/init.go` | baru: rakit `access.New(d.Bus)`, `store.New(d.DB)`, usecase, handler; daftarkan GET route |
| `.../knowledge/listtree/handler.go` | baru: baca `projectID` dan `prefix`, panggil usecase, tulis 200 / error |
| `.../knowledge/listtree/params.go` | baru: `Input{ProjectID, Prefix}` |
| `.../knowledge/listtree/usecase.go` | baru: `Usecase`, interface `Repository`, `Execute` |
| `.../knowledge/listtree/usecase_test.go` | baru: unit dengan repository dan checker palsu |
| `.../knowledge/listtree/handler_test.go` | baru: httptest + router chi |
| `.../knowledge/listtree/listtree_integration_test.go` | baru (tag `integration`): DB nyata + EXPLAIN |
| `.../knowledge/base.go` | tambah baris `listtree.Init(d)` |
| `api/modules/project/knowledge.yml` | path item `/api/projects/{projectID}/knowledge/tree` (sudah dibuat di foundation; verifikasi sesuai bagian 8) |

Tidak mengubah `base/`, migrasi, config, README, atau feature lain (README/tiket diperbarui di langkah F/W).

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, repository dan checker palsu)

- Checker mengembalikan `ErrProjectNotFound` -> `Execute` mengembalikan error itu; repository TIDAK dipanggil (pastikan counter nol), walau prefix tidak valid (`../x`), jadi non-anggota tetap 404.
- Checker error infra -> error terbungkus; repository tidak dipanggil.
- Prefix `../x`, `/docs`, `docs//a`, prefix berisi NUL -> `*errs.Error` 400 `INVALID_ARGUMENT` dengan `details.reason` yang tepat (`path_traversal`, `path_absolute`, `path_empty_segment`, `path_control_char`); repository tidak dipanggil.
- Prefix kosong -> repository menerima `likePrefix == "%"`; prefix `docs` dan `docs/` -> keduanya `docs/%`.
- Repository error -> error terbungkus `listtree: list meta: ...` (500 generik).
- Repository mengembalikan nil -> hasil `Folders` dan `Files` non-nil (JSON `[]`), `TotalFiles` 0.
- Hasil BuildLevel diteruskan apa adanya (satu kasus end-to-end usecase dengan 3 meta).

### Unit rule (`base/tree_test.go`, sudah di foundation; verifikasi mencakup)

- 3 tingkat (`docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`): prefix `docs` -> folder `auth` (file_count 2), file `a.md`, total 3; prefix `docs/auth` -> folder `flow` (1), file `login.md`; prefix `docs/auth/flow` -> file `x.md`, tanpa folder; root -> folder `docs` (3).
- Prefix `docs` tidak menyertakan `docs2/x.md` dan `docs.md` (meta di luar prefix diabaikan).
- Sorting: folder dan file masing-masing urut nama byte order (`B.md` sebelum `a.md`; `Z/` sebelum `a/`), stabil.
- Nama berisi `_` dan `%` diperlakukan literal (`a_b`, `100%`).

### Unit handler (`handler_test.go`)

- `GET .../tree` tanpa `prefix` -> 200, `prefix: ""`.
- `GET .../tree?prefix=docs%2Fauth` -> usecase menerima `docs/auth`.
- `projectID` bukan uuid -> 404 (checker asli dengan check-access palsu yang menjawab not exists, atau checker palsu).
- Tanpa sesi -> 401 (middleware `RequireAuth`).
- Error 400 memakai format error seragam dengan `trace_id`.

### Integrasi (`listtree_integration_test.go`, tag `integration`, `db.TestDB`)

Fixture: dua project; sisipkan file lewat SQL test.

1. **3 tingkat**: `docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`, `readme.md` -> root, `docs`, `docs/auth`, `docs/auth/flow` menghasilkan level yang benar (T05-AC1).
2. **Prefix `docs` tidak mencakup `docs2/`**: tambah `docs2/x.md`, `docs.md`, `docs-old/y.md`; prefix `docs` hanya memuat `docs/**`; root memuat `docs`, `docs2`, `docs-old` sebagai folder terpisah dan `docs.md` sebagai file.
3. **LIKE escaping**: file `a_b/x.md`, `axb/y.md`, `100%/z.md`, `100x/w.md`, `back\slash/q.md`; prefix `a_b` hanya `a_b/x.md`; prefix `100%` hanya `100%/z.md`; prefix `back\slash` hanya miliknya.
4. **Sorting**: urutan insert acak -> keluaran terurut nama (byte order).
5. **Non-anggota 404**: user bukan anggota (dan bukan super_admin) -> 404 `project tidak ditemukan`; super_admin -> 200 (T05-AC6). Memakai handler check-access T03 asli, atau palsu bila terlalu berat.
6. **Isolasi project**: file di project B tidak muncul di tree project A walau path sama.
7. **Prefix tanpa file**: `prefix=tidak/ada` -> 200 kosong; root project tanpa file -> 200 kosong.
8. **Tanpa content**: asersi statis bahwa query `ListMeta` tidak memuat `content` (code review) dan tree tetap cepat untuk file 1 MB (sisipkan 20 file besar; hanya meta yang dipindai).
9. **EXPLAIN** (rule 8.8): isi 5000 baris di beberapa project dan folder, `ANALYZE knowledge_files`, lalu `EXPLAIN` query `ListMeta` untuk prefix `docs/` non-root: plan memakai `knowledge_files_project_path_prefix` (Index Scan / Bitmap Index Scan), bukan Seq Scan. Simpan keluaran di deskripsi PR.
10. `prefix=docs/` (slash akhir) setara `prefix=docs`; `prefix=docs//` -> 400 `path_empty_segment`; `prefix=../x` -> 400 `path_traversal`; `prefix=/docs` -> 400 `path_absolute`.

Registrasi: `TestRoutesRegistered` di W memastikan route GET `/api/projects/{projectID}/knowledge/tree` terdaftar.

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Setelah upload folder `docs/` (3 tingkat), tab Knowledge memuat root: folder `docs` tampil; klik `docs` memanggil `tree?prefix=docs` (lazy, satu request) dan menampilkan `auth/` dan `a.md`; klik `auth` memuat `docs/auth`, lalu `flow`.
2. Folder yang sudah dibuka tidak memanggil ulang request saat dilipat dan dibuka lagi (cache klien), dan di-refresh setelah mutasi (upload, buat, pindah, hapus).
3. `file_count` pada folder sesuai jumlah file rekursif; urutan folder dan file alfabetis.
4. Super Admin non-anggota membuka knowledge project -> pohon termuat.
5. Folder dengan nama berisi `_` atau `%` dibuka -> hanya isinya sendiri yang tampil.

Negatif:
1. Developer non-anggota membuka URL knowledge project lain -> halaman 404 (API 404).
2. Prefix tanpa file (folder baru saja dihapus di tab lain, lalu dibuka) -> 200 kosong; UI menampilkan keadaan "folder kosong" tanpa error.
3. Request langsung `page.request.get(.../tree?prefix=../x)` -> 400 dengan `details.reason = path_traversal`.
4. Tanpa sesi -> 401 dan UI mengarahkan ke login.

## 8. Request/Response body

Permintaan: `GET /api/projects/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f/knowledge/tree?prefix=docs` (GET, tanpa body, tanpa header CSRF).

200 (satu level, `docs` dengan isi `docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`):

```json
{
  "prefix": "docs",
  "folders": [
    { "name": "auth", "path": "docs/auth", "file_count": 2 }
  ],
  "files": [
    { "name": "a.md", "path": "docs/a.md", "size_bytes": 120, "updated_at": "2026-10-24T09:00:00Z" }
  ],
  "total_files": 3
}
```

Catatan nama field: overview D10 memilih snake_case (`file_count`, `size_bytes`, `updated_at`, `total_files`); `fileCount`/`sizeBytes` di deskripsi tugas hanya ilustrasi.

200 root (`prefix` kosong):

```json
{
  "prefix": "",
  "folders": [ { "name": "docs", "path": "docs", "file_count": 3 } ],
  "files": [ { "name": "readme.md", "path": "readme.md", "size_bytes": 45, "updated_at": "2026-10-24T09:00:00Z" } ],
  "total_files": 4
}
```

200 prefix tanpa file (non-root) atau project kosong:

```json
{ "prefix": "tidak/ada", "folders": [], "files": [], "total_files": 0 }
```

400 prefix tidak valid (`?prefix=../x`):

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

Alasan 400 lain: `path_absolute`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`.

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

404 non-anggota atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

500 (check-access gagal atau DB gagal): error generik tanpa detail internal, mis. `{"error":{"code":"INTERNAL","message":"terjadi kesalahan"},"trace_id":"..."}` (sesuai format `respond` T00).

Bus: tidak ada endpoint bus baru. Konsumsi: `bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, checkaccess.Request{ProjectID, UserID, UserRole})` melalui `base/access`.

Audit: tidak ada entri untuk usecase ini.
