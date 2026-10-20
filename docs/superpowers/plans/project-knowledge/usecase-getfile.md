# T05 project/knowledge — Usecase `getfile` (GET /api/projects/{projectID}/knowledge/files?path=)

Melengkapi `docs/superpowers/plans/project-knowledge/overview.md` (F: migrasi `knowledge_files`, `base/`, `base/store`, `base/access`, `knowledge/base.go`, `knowledge.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `getfile` (U2, gelombang A). Jangan tertukar dengan `busreadfile` (handler bus `project.knowledge.read-file`, U12): folder, kontrak, dan semantik "tidak ada" berbeda (di sini 404, di sana `Found=false` + `Similar`).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack pada tiket; plan usecase ini backend saja. FE (viewer/editor) ada di `frontend-plan.md` |
| Domain | project |
| Module | knowledge |
| Feature | `getfile` (gelombang A, U2) |
| Endpoint | `/api/projects/{projectID}/knowledge/files?path=` |
| Method | GET |
| Version | tanpa prefix versi di URL (prefix `/api`); `knowledge.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`. Otorisasi project di usecase lewat `access.Checker.Require` (bus `project.management.check-access`, fail-closed) |
| Siapa | anggota (member/owner) dan super_admin (level `admin`) |
| Body limit | default (tidak ada body) |
| Bus dikonsumsi | `project.management.check-access` (via `base/access`). Tidak memanggil `get-project` dan `core.auth.get-user` |
| Bus disediakan | tidak ada |
| Audit | tidak ada (aksi baca) |
| Tabel / migrasi | membaca `knowledge_files`; tidak ada migrasi |
| AC | AC3 (`version` dari getfile dipakai updatefile), AC6 (non-anggota 404, isolasi project) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| allowedPaths | `.../knowledge/getfile/`, `.../knowledge/base.go` (satu baris `getfile.Init(d)`) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): `base.NormalizePath`, `base.Reason`, `base.Invalid`, `base.ErrFileNotFound`, `base.ErrProjectNotFound`, `base.File`/`File.ToDTO`/`FileDTO`, `base/store.Store.FindByPath` (projectID bukan uuid -> not found tanpa query), `base/access.Checker`/`BusChecker`, `knowledge.Register`, path `GET .../knowledge/files` + schema `KnowledgeFile` di `api/modules/project/knowledge.yml`, migrasi `20261024090000000_create_knowledge_files.sql`, serta handler T03 `check-access` (atau palsu di test).

## Summary Plan

`getfile` mengembalikan isi dan metadata satu file knowledge untuk anggota atau Super Admin. Alur usecase (urutan wajib overview 3.4: otorisasi sebelum validasi input):

1. `actor, err := checker.Require(ctx, projectID)`. Error dikembalikan apa adanya: 401 (tanpa identitas), 404 `project tidak ditemukan` (non-anggota, project tidak ada, `projectID` bukan uuid), 500 terbungkus (bus gagal, fail-closed).
2. `p, reason := base.NormalizePath(rawPath)`; `reason != ""` -> `base.Invalid(reason)` (400, `details.reason`). Path kosong (query `path` tidak ada) -> `path_empty`.
3. `f, found, err := repo.FindByPath(ctx, projectID, p)`; `err` -> `fmt.Errorf("getfile: find: %w", err)` (500).
4. `!found` -> `base.ErrFileNotFound` (404 `file tidak ditemukan`).
5. Jawab 200 `f.ToDTO()`. `version` = hex sha256 isi (D7), dihitung entity dari `content_hash` tersimpan, bukan dari `updated_at`.

Keputusan:
- Nama field JSON snake_case (`size_bytes`, `created_at`, `created_by`, `updated_at`, `updated_by`) mengikuti overview D10. Nama camelCase di deskripsi tugas hanya ilustrasi; overview menang.
- Non-anggota dan `projectID` bukan uuid menghasilkan respons byte-identik (kode, pesan, tanpa `details`), dan terjadi SEBELUM path divalidasi: non-anggota dengan `path=../x` tetap 404, bukan 400 (rule 7.3).
- Isolasi project: `FindByPath` selalu `WHERE project_id = $1 AND path = $2`. File project lain dengan path sama tidak terlihat (404), baik untuk anggota project A yang meminta path milik B.
- Path dipakai apa adanya (peka huruf besar/kecil, tanpa trim). `Docs/A.md` != `docs/a.md`. Tidak ada koreksi diam-diam.
- Query `path` dibaca dari `r.URL.Query().Get("path")` (net/http sudah percent-decode); tidak ada decoding tambahan.
- Tanpa `params.go` (satu query param string, tanpa body; cukup di handler). Tanpa `repository.go`: tidak ada SQL khusus feature; usecase menerima interface sempit yang dipenuhi `*store.Store` (overview 3.2).
- Respons 200 memasang `Cache-Control: no-store` karena isi dan akses bergantung pada keanggotaan saat itu.
- Isi file tidak pernah masuk log (rule 10.3). Log hanya `project_id`, `user_id`, `feature=getfile`, `path` pada error infra.
- Usecase hanya orkestrasi; rule path di `base`, SQL di `base/store`, otorisasi di `base/access`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Cek simbol F di atas. Baca signature nyata `respond.WriteJSON(w, status, v)`, `respond.WriteError(w, r, err)`, `middleware.RequireAuth()` dari kode T00, dan pola `Init` feature knowledge lain yang sudah ada (mis. `createfile`) bila ada; ikuti persis. Pastikan `d.API` adalah `chi.Router`.

**B1. `getfile/usecase.go`**

```go
package getfile

// FileFinder dipenuhi *store.Store (base/store). Interface didefinisikan di sisi consumer (rule 5.6).
type FileFinder interface {
    FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
}

type Usecase struct {
    access access.Checker
    files  FileFinder
}

func NewUsecase(a access.Checker, f FileFinder) *Usecase

// Execute: urutan 1-5 pada Summary Plan. Mengembalikan base.FileDTO.
func (u *Usecase) Execute(ctx context.Context, projectID, rawPath string) (base.FileDTO, error)
```

Tidak ada import `database/sql`/`sqlx`/`net/http`.

**B2. `getfile/handler.go`**

```go
type Handler struct{ uc *Usecase }
func NewHandler(uc *Usecase) *Handler

// Handle: projectID := chi.URLParam(r, "projectID") (diteruskan apa adanya, tanpa validasi uuid);
// rawPath := r.URL.Query().Get("path");
// dto, err := h.uc.Execute(r.Context(), projectID, rawPath)
// err != nil -> respond.WriteError(w, r, err); return
// w.Header().Set("Cache-Control", "no-store"); respond.WriteJSON(w, http.StatusOK, dto)
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request)
```

**B3. `getfile/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(access.New(d.Bus), store.New(d.DB))
    h := NewHandler(uc)
    d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/knowledge/files", h.Handle)
}
```

Pola route penuh dengan param (T03 D4). Tanpa `BodyLimit` khusus. Rute `GET .../files` tidak bentrok dengan `POST/PUT/DELETE` pada path yang sama (method berbeda) maupun `POST .../files/move` (segmen lebih dalam); chi menangani keduanya.

**B4. Wiring.** Tambah SATU baris `getfile.Init(d)` di `knowledge/base.go` (import `.../knowledge/getfile`). Tidak ada perubahan lain di luar `allowedPaths`.

**B5. Verifikasi.** `go build ./... && go vet ./...`; `go test ./internal/domain/project/internal/knowledge/getfile/...`; `go test -tags integration ./internal/domain/project/internal/knowledge/...`; `internal/archtest` lulus (tidak ada import antar-feature, tidak ada SQL di usecase/handler).

## Migration

Tidak ada. `getfile` hanya membaca `knowledge_files` (dibuat F: `backend/migrations/project/20261024090000000_create_knowledge_files.sql`). Indeks `UNIQUE (project_id, path)` (`knowledge_files_project_id_path_key`) melayani lookup equality.

## Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/getfile/usecase.go` | baru: `FileFinder`, `Usecase`, `Execute` |
| `.../knowledge/getfile/handler.go` | baru: `Handler.Handle` |
| `.../knowledge/getfile/init.go` | baru: `Init(d app.Deps)` + route GET |
| `.../knowledge/getfile/usecase_test.go` | baru: unit usecase dengan `access.Checker` dan `FileFinder` palsu |
| `.../knowledge/getfile/handler_test.go` | baru: httptest + router chi + usecase dengan palsu (kode status, bentuk JSON, header) |
| `.../knowledge/getfile/getfile_integration_test.go` | baru (tag `integration`): `db.TestDB` + `store.Store` nyata + `check-access` palsu di bus; isolasi project dan version |
| `.../knowledge/base.go` | tambah `getfile.Init(d)` |

Tidak mengubah `base/`, `base/store`, `base/access`, OpenAPI, README, atau konfigurasi (semua milik F). Bila ditemukan celah di `base*` (mis. `FindByPath` tidak mengisi `CreatedByLabel`), perbaiki di F, bukan di sini.

## Test Scenario

### Unit usecase (`usecase_test.go`, palsu manual, table-driven)

| Kasus | Setup | Ekspektasi |
|---|---|---|
| Sukses | checker -> actor; finder mengembalikan `File` isi `# Login\n` dengan `Hash=Hash(isi)` | `FileDTO.Path`, `Title="Login"`, `Content`, `SizeBytes`, `Version == hex(sha256(isi))` (64 hex huruf kecil), `CreatedBy/UpdatedBy{ID,Label}` terisi |
| Version benar | dua `File` dengan isi sama | `Version` identik; isi beda -> `Version` beda |
| Path tidak ada | finder `found=false` | `errors.Is(err, base.ErrFileNotFound)`, code `NOT_FOUND`; finder dipanggil dengan projectID dan path yang sama |
| Non-anggota | checker mengembalikan `base.ErrProjectNotFound` | error sama; finder TIDAK dipanggil |
| Non-anggota + path jelek | checker 404, `path="../x"` | tetap 404 (bukan 400); finder tidak dipanggil |
| Tanpa identitas | checker mengembalikan `base.ErrUnauthenticated` | code `UNAUTHORIZED` |
| Bus gagal | checker mengembalikan error biasa | error dikembalikan (500 oleh `respond`), finder tidak dipanggil |
| Path invalid | `""` | `INVALID_ARGUMENT`, `details.reason=path_empty`; finder tidak dipanggil |
| | `/a.md` | `path_absolute` |
| | `../x.md`, `a/./b.md` | `path_traversal` |
| | `a//b.md` | `path_empty_segment` |
| | `a\x00b.md` | `path_control_char` |
| | 513 rune | `path_too_long` |
| Error DB | finder mengembalikan error | dibungkus `getfile: find:`, bukan `*errs.Error` (-> 500) |
| Peka huruf | finder dipanggil dengan `Docs/A.md` | path diteruskan apa adanya (tidak di-lowercase) |

### Handler (`handler_test.go`, router chi nyata + usecase palsu)

- `GET /projects/{id}/knowledge/files?path=docs%2Fa.md` sukses -> 200, `Content-Type: application/json`, `Cache-Control: no-store`, body memuat semua field snake_case; `path` dengan spasi dan unicode (`catatan%20rapat/é.md`) ter-decode benar dan sampai ke usecase.
- `path` tidak ada di query -> usecase menerima `""` -> 400 `path_empty`.
- Error usecase dipetakan: 400, 401, 404, 500 (pesan 500 generik, tanpa membocorkan error internal).
- Waktu pada JSON berformat RFC 3339 UTC (`...Z`).

### Integrasi (`getfile_integration_test.go`, tag `integration`)

- Seed dua project A dan B via SQL test (baris `projects` minimal + `knowledge_files`). `check-access` palsu: anggota A saja.
- Sukses: file A `docs/auth/login.md` -> 200; `version` sama dengan sha256 isi yang di-seed; `created_by.label` dan `updated_by.label` sesuai kolom `*_label`.
- Path tidak ada di project A -> 404 `file tidak ditemukan`.
- File project lain tidak terlihat: B punya `only-b.md`, anggota A meminta `only-b.md` pada projectID A -> 404; A dan B sama-sama punya `same.md` dengan isi beda -> minta lewat A menghasilkan isi A, bukan B.
- Non-anggota (checker `ProjectExists=true, Level=none`) -> 404 `project tidak ditemukan`; project tidak ada -> respons identik.
- `projectID = "bukan-uuid"` (checker palsu mengembalikan `ProjectExists=false`; `store` tidak boleh error uuid) -> 404 `project tidak ditemukan`.
- Untuk melengkapi `TestRoutesRegistered` di W: route `GET /api/projects/{projectID}/knowledge/files` terdaftar (tanpa mengubah test itu di sini).
- Konsistensi dengan `updatefile` (AC3) diuji di integrasi W: dua `getfile` mengembalikan `version` sama sebelum ada update.

## E2E Scenarios

Playwright terhadap binary (butuh T01, T02, T03 merge dan UI T05).

Positif:
1. Developer anggota login -> buka project -> tab Knowledge -> klik `docs/auth/login.md` di pohon -> viewer menampilkan Markdown terender; request jaringan `GET .../knowledge/files?path=docs%2Fauth%2Flogin.md` mengembalikan 200 dengan `version` 64 hex.
2. Buka file lalu masuk mode Edit: editor memuat `content` yang sama dan menyimpan `version` untuk dikirim saat simpan (AC3).
3. Super Admin (bukan anggota) membuka file project mana pun -> 200 dan viewer tampil.
4. File dengan nama berisi spasi/unicode (`catatan rapat/é.md`) terbuka benar (encoding query).

Negatif:
1. Buka URL file yang sudah dihapus (tab lain menghapusnya) -> viewer menampilkan pesan "file tidak ditemukan" (404), bukan crash.
2. Developer non-anggota membuka URL knowledge project lain, termasuk dengan `path` valid -> halaman 404 `project tidak ditemukan`; respons identik dengan `projectID` acak/bukan uuid.
3. Pengguna anggota project A memanipulasi `projectID` ke project B yang berisi path yang sama -> 404, isi B tidak tampil.
4. Panggilan langsung `page.request.get` dengan `path=../x.md` sebagai anggota -> 400 `path_traversal`; tanpa `path` -> 400 `path_empty`.
5. Tanpa sesi (logout) -> 401 `UNAUTHORIZED`, UI mengarahkan ke login.

## Request/Response body

Tidak ada body request. Query: `path` (wajib), contoh `?path=docs%2Fauth%2Flogin.md`. Header sesi (cookie) wajib; GET tidak butuh `X-Requested-With`.

### 200

```json
{
  "path": "docs/auth/login.md",
  "title": "Login",
  "content": "# Login\n\nAlur login...",
  "size_bytes": 22,
  "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea",
  "created_at": "2026-10-24T09:00:00Z",
  "created_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" },
  "updated_at": "2026-10-24T09:05:00Z",
  "updated_by": { "id": "0192f0c4-0000-7000-8000-00000000000b", "label": "Sari" }
}
```

Header: `Cache-Control: no-store`. `title` string kosong bila tidak ada heading `# `. `size_bytes` dalam byte UTF-8.

### Error (format seragam)

400 path tidak valid:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

400 `path` kosong atau tidak dikirim:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path wajib diisi, mis. docs/auth/login.md", "details": { "reason": "path_empty" } }, "trace_id": "..." }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

404 file tidak ada (atau milik project lain):

```json
{ "error": { "code": "NOT_FOUND", "message": "file tidak ditemukan" }, "trace_id": "..." }
```

404 non-anggota, project tidak ada, atau `projectID` bukan uuid (identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

500 (bus `check-access` gagal atau DB error; pesan generik, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "..." }
```
