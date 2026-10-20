# T05 project/knowledge — Usecase Plan: `uploadfiles`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (FOUNDATION). Bila bertentangan, overview menang sampai diperbarui. Usecase ini hanya bergantung pada `base/`, `base/store`, `base/access`, `base/audit` yang dibuat di langkah F overview. Jangan mengubah file foundation; bila ada kekurangan, catat di PR dan perbarui overview lebih dulu.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Usecase | `uploadfiles` (gelombang B, U5) |
| Scope | fullstack (plan ini: backend; UI upload di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Folder feature | `backend/internal/domain/project/internal/knowledge/uploadfiles` |
| Endpoint | `/api/projects/{projectID}/knowledge/upload` |
| Method | POST (`multipart/form-data`) |
| Version | tanpa prefix versi URL (prefix `/api`); OpenAPI `knowledge.yml` `info.version: 0.1.0` |
| Middleware route | `middleware.RequireAuth()`, `middleware.BodyLimit(Limits.UploadBodyLimit())` (= `MaxUploadBytes + 1 MiB`). CSRF (`X-Requested-With: grimoire`), Session, Timeout 30 s sudah global |
| Bus dikonsumsi | `project.management.check-access` (fail-closed, via `base/access`), `project.management.get-project` (fail-soft, via `base/audit`), `core.audit.record` (fail-open, setelah commit, via `base/audit`) |
| Bus disediakan | tidak ada |
| AC | T05-AC1, AC2, AC7 (+ AC6 untuk non-anggota) |
| Prasyarat | Foundation F selesai (`base/*`, migrasi `knowledge_files`, config `MAX_PROJECT_FILES`, `Limits`). T03 merged |

---

## 2. Summary Plan

`uploadfiles` menerima banyak file sekaligus (folder dari browser lewat `webkitdirectory`) untuk satu project. Alur satu usecase utuh (tidak dipecah):

1. `access.Require` (404 untuk non-anggota / bukan uuid, sebelum membaca body).
2. Baca body sebagai stream `r.MultipartReader()`. Path relatif diambil dari parameter `filename` MENTAH di header `Content-Disposition` (`mime.ParseMediaType`), BUKAN `Part.FileName()` (yang memanggil `filepath.Base` dan membuang folder).
3. Tiap part dibaca maksimum `MaxFileBytes + 3 (BOM) + 1` byte. Lebih -> ditandai `tooLarge` dan sisa part dikuras (drain). Part setelah `MaxUploadFiles` -> `rejected too_many_files` (dikuras tanpa disimpan). Total byte mentah semua part file (termasuk yang dibuang/dikuras) melebihi `MaxUploadBytes` -> hentikan, 413 `upload_too_large` untuk seluruh request, tidak ada yang ditulis, tanpa audit.
4. Per file, `base.ClassifyPart`: `NormalizePath` gagal -> rejected + reason; `IsHidden` -> skipped `hidden`; ekstensi tak didukung -> rejected `unsupported_type`; `tooLarge` -> rejected `too_large`; `PrepareContent` (BOM/UTF-8/NUL) gagal -> rejected + reason.
5. File lolos: SATU `BeginTx`: `LockProject` -> `CountFiles` -> select `(path, content_hash)` untuk `path = ANY($2)` -> `base.PlanUpload` (created / updated / unchanged / `duplicate_in_request` / `project_file_limit`) -> batch INSERT dan batch UPDATE (title, hash, size, label aktor); `unchanged` tidak ditulis. Kegagalan DB -> rollback seluruh batch -> 500, tanpa audit.
6. Respons 200 `{summary, items}` dengan item dalam urutan request.
7. Setelah commit: SATU entri audit `knowledge.upload.completed` (hitungan + maks 20 path created/updated pertama), fail-open.

Keputusan penting khusus usecase ini:

- Konkurensi: advisory lock per project (`LockProject`) mengserialkan dua upload paralel pada project yang sama, sehingga kuota `MAX_PROJECT_FILES` tidak terlampaui (dua upload masing-masing membaca `CountFiles` setelah lock).
- Validasi dan pembacaan body terjadi SEBELUM tx dibuka; tx hanya berisi SQL (tidak ada panggilan bus, tidak ada I/O jaringan di dalam tx; rule overview 3.5).
- Isi file valid ditahan di memori sampai tx (maks `MaxUploadBytes` per request; diterima, lihat overview bagian 15).
- Jumlah byte yang dicatat di audit (`bytes`) = total `len(Content)` file created+updated yang benar-benar ditulis.
- Upload tetap menulis tepat satu entri audit `knowledge.upload.completed` walaupun semua item unchanged/rejected/skipped (catatan bahwa upload terjadi), selama request sukses (200). Tidak ada `knowledge.file.updated` per file.
- Part dengan form name selain `files` atau tanpa `filename` dikuras dan diabaikan. Tanpa satu pun part file -> 400 `no_files`. `skipped`/`rejected` dari part file tetap dihitung sebagai part file (jadi upload berisi hanya `.git/config` tetap 200, bukan `no_files`).
- Nama file mengandung `/` dipertahankan apa adanya (inti AC1). `\` karakter biasa (D11). Tanpa percent-decoding (risiko `%22` didokumentasikan di overview).
- Race rare: `INSERT` gagal unique violation `knowledge_files_project_id_path_key` di dalam tx terkunci tidak mungkin terjadi untuk upload lain (serial), tetapi `createfile`/`movefile` memakai lock yang sama sehingga juga serial. Bila tetap terjadi (jaring pengaman), perlakukan sebagai error infra -> rollback -> 500 (bukan 409), karena satu batch tidak bisa menjelaskan per item.

---

## 3. Plan (langkah berurutan)

Prasyarat: `ls backend/internal/domain/project/internal/knowledge/base/store` dan `.../base/access`, `.../base/audit` ada, `go build ./...` hijau, migrasi `knowledge_files` ada. Bila tidak, STOP (foundation F belum selesai).

### Langkah 1 — `params.go` (parsing request, tanpa I/O DB)

File: `uploadfiles/params.go`.

```go
// Part adalah hasil baca satu part file multipart (sebelum klasifikasi).
type RawPart struct {
    Index   int    // urutan part file di request, 0-based
    Name    string // filename MENTAH dari Content-Disposition (bisa memuat "/")
    Data    []byte // maks MaxFileBytes+3 byte; kosong bila TooLarge atau di luar kuota jumlah
    TooLarge bool  // part melebihi MaxFileBytes+3 byte
    OverCount bool // part ke-(MaxUploadFiles+1) dst: tidak disimpan
}

// Parsed adalah hasil pembacaan seluruh body.
type Parsed struct {
    Parts      []RawPart
    TotalBytes int64 // byte mentah semua part file, termasuk yang dikuras
}
```

Fungsi (handler-level, tidak berisi rule bisnis):

- `readParts(r *http.Request, lim base.Limits) (Parsed, error)`:
  1. `mr, err := r.MultipartReader()`; `errors.Is(err, http.ErrNotMultipart)` atau `http.ErrMissingBoundary` -> `base.Invalid(base.ReasonNotMultipart)`.
  2. Loop `part, err := mr.NextPart()`; `io.EOF` -> selesai. `*http.MaxBytesError` (dibungkus pada error multipart; gunakan `errors.As`) -> `base.TooLarge(base.ReasonUploadTooLarge, lim.MaxUploadBytes)`. Error multipart lain (mis. `multipart: NextPart: ...`, `unexpected EOF`) -> `base.Invalid(base.ReasonNotMultipart)` (400).
  3. `part.FormName() != "files"` atau tidak ada parameter `filename` -> `io.Copy(io.Discard, part)` (error `MaxBytesError` ikut dipetakan 413), lanjut. Part non-file TIDAK dihitung ke `TotalBytes`/jumlah file, tetapi byte-nya tetap dibatasi `BodyLimit`.
  4. Ambil nama mentah: `_, params, perr := mime.ParseMediaType(part.Header.Get("Content-Disposition"))`; `params["filename"]`. Bila `perr != nil`, fallback: tolak part sebagai nama kosong (`Name = ""` -> kelak `path_empty`), jangan panic. DILARANG memanggil `part.FileName()`.
  5. Bila `len(parts) >= lim.MaxUploadFiles`: `OverCount = true`; kuras part dengan menghitung byte (`io.Copy(io.Discard, part)` mengembalikan n); `TotalBytes += n`.
  6. Selain itu: `n, rerr := io.Copy(&buf, io.LimitReader(part, lim.MaxFileBytes+3+1))`. Bila `n > lim.MaxFileBytes+3`: `TooLarge = true`, `Data = nil`, kuras sisa (`io.Copy(io.Discard, part)`), `total += n + drained`. Bila tidak: `Data = buf.Bytes()`, `total += n`.
  7. Setiap penambahan `TotalBytes`: bila `> lim.MaxUploadBytes` -> segera `return base.TooLarge(base.ReasonUploadTooLarge, lim.MaxUploadBytes)` (hentikan membaca; usecase tidak menulis apa pun).
  8. Penamaan: `Name` dipotong tidak perlu di sini; pemotongan 1024 rune untuk tampilan dilakukan `ClassifyPart` (`Decision.Path`).
- Pastikan `Data` disalin (bukan slice buffer bersama) agar aman antar part.

Catatan implementasi penting: karena `Part.FileName()` membuang folder, test regresi wajib memverifikasi `docs/auth/login.md` tidak menjadi `login.md`.

### Langkah 2 — `repository.go` (SQL khusus feature)

File: `uploadfiles/repository.go`. Membungkus `*store.Store`.

```go
type Repo struct{ s *store.Store }
func NewRepo(s *store.Store) *Repo
func (r *Repo) InTx(ctx context.Context, fn func(*Repo) error) error {
    return r.s.InTx(ctx, func(ts *store.Store) error { return fn(&Repo{s: ts}) })
}
func (r *Repo) LockProject(ctx context.Context, projectID string) error      // delegasi ke store
func (r *Repo) CountFiles(ctx context.Context, projectID string) (int, error) // delegasi ke store
// ExistingHashes: SELECT path, content_hash FROM knowledge_files WHERE project_id = $1 AND path = ANY($2)
func (r *Repo) ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
// InsertFiles: batch insert baris created; chunk 100 baris per statement.
func (r *Repo) InsertFiles(ctx context.Context, files []base.File) error
// UpdateFiles: batch update baris updated; chunk 100 baris per statement.
func (r *Repo) UpdateFiles(ctx context.Context, files []base.File) error
```

SQL (semua parameter, rule 6.10 `project_id`):

- `ExistingHashes`: `SELECT path, content_hash FROM knowledge_files WHERE project_id = $1 AND path = ANY($2)` dengan `$2` `pq`-style array via `pgx` (`[]string` dikonversi `pgtype`/`pq.Array` sesuai driver yang sudah dipakai `store`; ikuti pola `store.ListPaths`). Daftar path kosong -> return map kosong tanpa query. Chunk `ANY` tidak perlu (maks 500 path).
- `InsertFiles` (per chunk, hash dikirim hex `text[]`):

  ```sql
  INSERT INTO knowledge_files
    (project_id, path, title, content, content_hash, size_bytes,
     created_by, created_by_label, updated_by, updated_by_label)
  SELECT $1, u.path, u.title, u.content, decode(u.hash, 'hex'), u.size,
         $2::uuid, $3, $2::uuid, $3
  FROM unnest($4::text[], $5::text[], $6::text[], $7::text[], $8::int[])
       AS u(path, title, content, hash, size)
  ```
- `UpdateFiles` (per chunk):

  ```sql
  UPDATE knowledge_files f
  SET title = u.title, content = u.content, content_hash = decode(u.hash, 'hex'),
      size_bytes = u.size, updated_at = now(), updated_by = $2::uuid, updated_by_label = $3
  FROM unnest($4::text[], $5::text[], $6::text[], $7::text[], $8::int[])
       AS u(path, title, content, hash, size)
  WHERE f.project_id = $1 AND f.path = u.path
  ```
  Verifikasi `RowsAffected == len(chunk)`; bila kurang (baris dihapus di luar lock oleh `deletefile` yang tanpa lock) -> kembalikan error `fmt.Errorf("uploadfiles: update: rows affected %d, want %d", ...)` -> rollback -> 500. Untuk insert pun `RowsAffected` harus sama.
- Konten ber-`\x00` tidak mungkin (sudah ditolak `PrepareContent`), sehingga aman untuk `text[]`.
- Jangan me-log isi.

Test integrasi repository (tag `integration`): lihat bagian Test.

### Langkah 3 — `usecase.go` (orkestrasi)

File: `uploadfiles/usecase.go`. Usecase hanya orkestrasi; semua rule di `base`.

```go
type repository interface {
    InTx(ctx context.Context, fn func(repository) error) error
    LockProject(ctx context.Context, projectID string) error
    CountFiles(ctx context.Context, projectID string) (int, error)
    ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
    InsertFiles(ctx context.Context, files []base.File) error
    UpdateFiles(ctx context.Context, files []base.File) error
}
```

Karena `Repo.InTx` meneruskan `*Repo`, bungkus dengan adaptor kecil di `init.go`/`repository.go` supaya interface `repository` dapat dipalsukan di test (`InTx(ctx, fn func(repository) error)`). Alternatif yang diterima: interface dua lapis (`txRunner` + `tx`). Pilih satu dan konsisten dengan feature lain (`createfile`, `movefile`).

```go
type Usecase struct {
    repo   repository
    access access.Checker
    audit  audit.Recorder
    limits base.Limits
}
type Input struct {
    ProjectID string
    Parts     []RawPart
    Meta      base.RequestMeta
}
func (u *Usecase) Execute(ctx context.Context, in Input) (base.UploadResultDTO, error)
```

Catatan tentang urutan: `access.Require` HARUS dipanggil di handler SEBELUM `readParts` (agar non-anggota 404 tanpa membaca body besar). Karena `Execute` menerima `Parts` yang sudah dibaca, pecah menjadi dua panggilan di handler:

1. `actor, err := uc.Authorize(ctx, projectID)` (membungkus `access.Require`).
2. `parsed, err := readParts(r, limits)`.
3. `res, err := uc.Execute(ctx, actor, Input{...})`.

(Ini bukan pemecahan usecase artifisial: satu `Usecase` struct, dua metode karena urutan I/O stream memaksanya. Alternatif ekuivalen: `Execute` menerima `PartSource` interface yang dibaca lambat setelah `Require`. Pilih `Authorize` + `Execute`; test unit tetap memanggil keduanya.)

`Execute(ctx, actor, in)` langkah:

1. `decisions := make([]base.Decision, 0, len(parts))`, `incoming := []base.Incoming{}`.
2. Untuk tiap `RawPart` urut Index:
   - `OverCount` -> `Decision{Index, Path: truncateRunes(Name, 1024), Status: rejected, Reason: ReasonTooManyFiles}`.
   - selain itu `inc, dec, ok := base.ClassifyPart(p.Index, p.Name, p.Data, p.TooLarge, u.limits.MaxFileBytes)`; `!ok` -> append `dec`; `ok` -> append ke `incoming`.
3. Bila `len(incoming) == 0`: lewati DB sepenuhnya (tidak ada tx); `base.SortByIndex(decisions)`; langsung ke langkah audit (tetap 1 entri) dan respons.
4. Tx: `repo.InTx(ctx, func(tx repository) error {...})`:
   1. `tx.LockProject(ctx, projectID)`.
   2. `count, err := tx.CountFiles(...)`.
   3. `existing, err := tx.ExistingHashes(ctx, projectID, pathsOf(incoming))`.
   4. `planned := base.PlanUpload(incoming, existing, count, u.limits.MaxProjectFiles)`.
   5. Bangun `[]base.File`: untuk `created` -> `base.NewFile(projectID, path, content, actor)`; untuk `updated` -> `base.File{...}.WithContent(content, actor)` (cukup `base.NewFile(...)` karena UPDATE hanya memakai title/content/hash/size/updated_by*; `created_*` tidak diubah). `unchanged`/`rejected` tidak ditulis.
   6. `tx.InsertFiles(created)`, `tx.UpdateFiles(updated)` (lewati bila kosong).
   7. Simpan `planned` ke variabel luar dan `bytesWritten`.
   - Error di dalam fn -> rollback; dikembalikan dibungkus `fmt.Errorf("uploadfiles: batch: %w", err)`.
5. Gabungkan `decisions` (pra-tolak) + `planned`, `base.SortByIndex(all)`.
6. Setelah commit (di luar tx): `u.audit.Record(ctx, audit.Event{Action: record.ActionKnowledgeUploadCompleted, ProjectID, Actor: actor, TargetType: "knowledge", Target: "", Args: {"files": len(parts)}, Result: {created, updated, unchanged, skipped, rejected, bytes, paths, paths_truncated}, Meta: in.Meta})`. `paths, trunc := base.ChangedPaths(all, 20)`. `Args.files` = jumlah part file (semua, termasuk `OverCount`).
7. Return `base.NewUploadResult(all)`.

Aturan error: error bisnis hanya dari `readParts`/`Authorize` (`*errs.Error`). Error DB -> wrap generik -> 500. Tidak ada audit bila error.

Bila ada `no_files` (len(parts) == 0): `Execute` mengembalikan `base.Invalid(base.ReasonNoFiles)` sebelum langkah 1.

### Langkah 4 — `handler.go`

```go
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
    pid := chi.URLParam(r, "projectID")
    actor, err := h.uc.Authorize(r.Context(), pid)        // 401/404/500
    if err != nil { respond.WriteError(w, r, err); return }
    parsed, err := readParts(r, h.limits)                  // 400/413
    if err != nil { respond.WriteError(w, r, err); return }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    res, err := h.uc.Execute(r.Context(), actor, Input{ProjectID: pid, Parts: parsed.Parts, Meta: meta})
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, r, http.StatusOK, res)
}
```

Cek signature `respond.WriteError/WriteJSON` di `main` (pakai persis yang dipakai `management` T03/feature lain). Handler tidak berisi rule bisnis. Saat `readParts` mengembalikan 413, `respond.WriteError` memetakan `*errs.Error` kode `PAYLOAD_TOO_LARGE` -> 413 dengan `details.reason` dan `max_bytes`. Kuras sisa body (`io.Copy(io.Discard, r.Body)` terbatas) tidak diperlukan; `BodyLimit` memutus koneksi.

### Langkah 5 — `init.go`

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    limits := base.LimitsFrom(d.Config)
    uc := NewUsecase(newRepo(s), access.New(d.Bus), audit.New(d.Bus, d.Log), limits)
    h := &Handler{uc: uc, limits: limits}
    d.API.With(middleware.RequireAuth(), middleware.BodyLimit(limits.UploadBodyLimit())).
        Post("/projects/{projectID}/knowledge/upload", h.Upload)
}
```

Sesuaikan `d.Config` (nilai/pointer) dan pola `d.API.With(...)` dengan yang dipakai T03/foundation (pola penuh `/projects/{projectID}/...` di `d.API`). Pastikan `BodyLimit` per-route menang atas root 1 MiB (dicatat overview 0).

### Langkah 6 — Registrasi

Tambah satu baris `uploadfiles.Init(d)` di `backend/internal/domain/project/internal/knowledge/base.go` (`Register`). Jangan menyentuh baris feature lain (gelombang B paralel; hindari konflik merge, tambahkan baris di akhir daftar bersama import).

### Langkah 7 — Test, lalu OpenAPI

- Tulis test unit/integrasi (bagian 6).
- OpenAPI operasi `POST /api/projects/{projectID}/knowledge/upload` sudah dibuat di foundation F langkah 1. Verifikasi cocok dengan bagian 8 plan ini (field `files` array binary, respons 200/400/401/403/404/413/500); bila selisih, perbarui `api/modules/project/knowledge.yml` + `make api-bundle` + `make api-check` di PR ini dan beri tahu di deskripsi PR (hanya bagian `/knowledge/upload`).

### Langkah 8 — Verifikasi

`go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/...` lalu `go test -tags integration ./internal/domain/project/internal/knowledge/uploadfiles/...`. `internal/archtest` hijau: `uploadfiles` tidak meng-import feature lain, tidak ada SQL di usecase/handler, tidak ada `sqlx` di usecase.

---

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari `backend/migrations/project/20261024090000000_create_knowledge_files.sql` (foundation F langkah 2). Tidak butuh kolom/indeks tambahan: `UNIQUE (project_id, path)` melayani `path = ANY($2)`.

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/uploadfiles/init.go` | baru: `Init(d app.Deps)`, rakit store -> repo -> usecase -> handler, daftarkan `POST /projects/{projectID}/knowledge/upload` dengan `RequireAuth` + `BodyLimit(UploadBodyLimit())` |
| `.../uploadfiles/handler.go` | baru: `Handler.Upload` (Authorize -> readParts -> Execute -> WriteJSON 200) |
| `.../uploadfiles/params.go` | baru: `RawPart`, `Parsed`, `readParts` (MultipartReader, filename mentah, limit per part, drain, total byte) |
| `.../uploadfiles/usecase.go` | baru: `Usecase`, `Authorize`, `Execute`, interface `repository` |
| `.../uploadfiles/repository.go` | baru: `Repo` membungkus `*store.Store`: `InTx`, `ExistingHashes`, `InsertFiles`, `UpdateFiles` (+ delegasi lock/count) |
| `.../uploadfiles/params_test.go` | baru: unit `readParts` (httptest multipart nyata) |
| `.../uploadfiles/usecase_test.go` | baru: unit usecase dengan repo/access/audit palsu |
| `.../uploadfiles/repository_integration_test.go` | baru (tag `integration`): SQL batch + hash |
| `.../uploadfiles/handler_test.go` | baru: httptest + router chi |
| `.../knowledge/base.go` | ubah: tambah satu baris `uploadfiles.Init(d)` |
| `.../knowledge/knowledge_integration_test.go` | tambah skenario AC1/AC2/AC7/kuota/konkurensi (dimiliki langkah W; plan ini mendefinisikan skenarionya, implementasi boleh ditunda ke W bila file belum ada) |
| `api/modules/project/knowledge.yml`, `api/openapi.yml` | hanya bila ada selisih dengan bagian 8 |

Tidak diubah: `base/*`, `base/store`, `base/access`, `base/audit`, feature lain, `backend/lib/*`, migrasi.

---

## 6. Test Scenario

Aturan: unit `base` sudah dicakup foundation; di sini hanya perilaku usecase/handler/repo. Tanpa mock untuk rule murni; usecase memakai repo/access/audit palsu.

### 6.1 Unit `params_test.go` (`readParts`, multipart nyata lewat `mime/multipart.Writer` + `httptest`)

Catatan: `multipart.Writer.CreateFormFile` memakai `filepath.Base`? Tidak, tetapi meng-escape; untuk filename mengandung `/` bangun header manual lewat `CreatePart(textproto.MIMEHeader{"Content-Disposition": `form-data; name="files"; filename="docs/auth/login.md"`})` supaya terjamin.

- `docs/auth/login.md` -> `RawPart.Name == "docs/auth/login.md"` (regresi `Part.FileName()`).
- `a/b/c/d.md`, nama dengan spasi dan Unicode (`catatan/é.md`) dipertahankan; `a\b.md` utuh; `../x.md` utuh (ditolak nanti oleh `ClassifyPart`, bukan di sini).
- Part form name bukan `files` (mis. `meta`) atau tanpa `filename` -> diabaikan, tidak muncul di `Parts`, tidak menambah `TotalBytes`.
- Part tepat `MaxFileBytes+3` byte -> `TooLarge=false`; `+1` byte -> `TooLarge=true`, `Data=nil`, sisa dikuras (verifikasi part berikutnya tetap terbaca benar).
- Part ke-`MaxUploadFiles+1` -> `OverCount=true`; byte dikuras dan dihitung ke `TotalBytes`.
- `TotalBytes` melewati `MaxUploadBytes` -> error `PAYLOAD_TOO_LARGE`, `details.reason=upload_too_large`, `details.max_bytes` benar; berhenti membaca (limit kecil di test: mis. `MaxUploadBytes=1000`).
- Body `application/json` -> 400 `not_multipart`; multipart tanpa boundary -> 400 `not_multipart`; multipart terpotong (body dipotong di tengah) -> 400 `not_multipart`.
- Header `Content-Disposition` rusak (tidak bisa di-parse) -> part tercatat dengan `Name=""` (nanti `path_empty`), tidak panic.
- `http.MaxBytesReader` yang memicu limit di tengah part -> `upload_too_large` 413 (via `errors.As(*http.MaxBytesError)`).

### 6.2 Unit `usecase_test.go` (repo/access/audit palsu)

- Tanpa part file -> `no_files` 400, repo tidak disentuh, audit 0.
- Semua part tertolak/terlewat (hanya `.git/config` dan `x.png`) -> tidak ada `InTx` (repo palsu mencatat 0 panggilan), respons 200 dengan skipped 1 + rejected 1, audit tepat 1 (`created=0`).
- Campuran valid: urutan item respons = urutan request walau `OverCount`, rejected, skipped, created saling selang-seling.
- `OverCount` -> `rejected too_many_files`; file valid sebelumnya tetap diproses.
- Alur DB: repo palsu mencatat urutan panggilan `LockProject -> CountFiles -> ExistingHashes -> InsertFiles/UpdateFiles` di dalam `InTx`; audit dipanggil SETELAH `InTx` selesai (palsu mencatat urutan global: `tx-commit` sebelum `audit`).
- `ExistingHashes` dipanggil dengan hanya path file lolos (tanpa path ditolak/skipped/duplikat dobel diizinkan satu kali).
- `unchanged` tidak masuk `InsertFiles`/`UpdateFiles`; `updated` hanya yang hash beda; `File.Title` dihitung dari `# heading`; `UpdatedByLabel` = label aktor.
- `duplicate_in_request`: path sama dua kali -> kemunculan pertama created, kedua rejected; hanya satu insert.
- Kuota: `count=4998`, `MaxProjectFiles=5000`, 3 file baru -> 2 created + 1 `project_file_limit`; updated tidak memakan kuota.
- Error repo di `InsertFiles` -> `Execute` mengembalikan error non-`*errs.Error` (500), audit TIDAK dipanggil.
- Error `access.Require` (404/401/500) dikembalikan apa adanya oleh `Authorize`; repo/audit tidak disentuh.
- Audit event: `Action == record.ActionKnowledgeUploadCompleted`, `TargetType=="knowledge"`, `Target==""`, `Args.files` = jumlah part file, `Result` memuat 5 hitungan + `bytes` (jumlah `len(content)` created+updated) + `paths` (maks 20) + `paths_truncated`. Verifikasi isi file TIDAK ada di `Args`/`Result` (cari string unik).
- 25 file created -> `paths` 20, `paths_truncated=true`.
- Audit palsu yang "gagal" (fail-open) tidak mengubah respons 200 (Recorder tidak mengembalikan error; test memastikan usecase tidak peduli).

### 6.3 Integrasi repository (`repository_integration_test.go`, `db.TestDB`)

- `InsertFiles` 250 baris (3 chunk) -> semua tersimpan; `content_hash` 32 byte = sha256 isi; `size_bytes`, `title`, `created_by/label`, `updated_by/label` benar; kolom `search` terisi (query `search @@ to_tsquery('simple','kata')` menemukan).
- `UpdateFiles` mengubah title/content/hash/size/`updated_at`/`updated_by*` tetapi TIDAK `created_*`; `updated_at` maju.
- `ExistingHashes` hanya mengembalikan baris project sendiri (dua project, path sama); path tak ada tidak muncul; list kosong tanpa query.
- `UpdateFiles` untuk path yang tidak ada -> error rows affected (rollback).
- Isolasi: update project A tidak menyentuh path sama di project B.
- Rollback atomik: `InsertFiles` sukses lalu `UpdateFiles` error dalam satu `InTx` -> tidak ada baris insert yang tersisa.
- Isi memuat karakter Unicode, `\r\n`, dan baris kosong tersimpan persis (byte-ke-byte).
- `LockProject` serial: dua goroutine `InTx` -> tx kedua menunggu sampai tx pertama commit (urutan waktu).

### 6.4 Handler (`handler_test.go`, router chi + usecase palsu atau nyata dengan repo palsu)

- Route terdaftar: `POST /projects/{projectID}/knowledge/upload`; `GET` pada path sama -> 405.
- Tanpa sesi -> 401 (RequireAuth), tanpa header CSRF -> 403 `CSRF_REQUIRED` (bila middleware global dipasang di test router; bila tidak, ditutup di test integrasi).
- Non-anggota -> 404 dan body tidak dibaca (usecase palsu mencatat `readParts` tidak dipanggil: kirim body besar, pastikan cepat dan 404).
- Respons 200 JSON sesuai bagian 8 (`summary`, `items`, `reason` omitempty).
- `Content-Type: application/json` -> 400 `not_multipart`.

### 6.5 Integrasi lintas komponen (router `/api` asli + bus; di `knowledge_integration_test.go`, tag `integration`)

- **AC1**: upload multipart 3 tingkat (`docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`) -> `summary.created=3`, path persis seperti dikirim (tanpa `filepath.Base`); `GET tree?prefix=docs` menampilkan folder `auth` (file_count 2) + file `a.md`; upload ulang identik -> `unchanged=3`, `updated_at` tidak berubah, 0 baris ditulis (cek `updated_at` sama); ubah isi satu file -> `updated=1, unchanged=2`; file lain di project (dibuat via `createfile`) tetap.
- **AC2**: satu upload berisi `../x.md`, path 600 karakter (`path_too_long`), file 2 MB (`too_large`), biner `.md` (header PNG + NUL, `invalid_encoding`), `.git/config`, `.DS_Store` (skipped `hidden`), `x.png` (`unsupported_type`), dan 2 file valid -> reason tiap item benar, 2 created, `getfile` kedua file valid OK, tidak ada baris untuk yang ditolak.
- **AC7**: upload 100 file valid -> `record` palsu menerima TEPAT 1 entri `knowledge.upload.completed`, `Result.paths` panjang 20, `paths_truncated=true`, `created=100`; 0 entri `knowledge.file.*`.
- Nama file mengandung `/` dipertahankan; unicode (`catatan/é.md`) dan spasi dipertahankan; BOM dibuang (hash sama dengan isi tanpa BOM, upload ulang tanpa BOM -> unchanged).
- `.git/` dan `.DS_Store` -> `skipped hidden`; segmen `docs/.hidden/x.md` -> skipped.
- Kuota: config `MAX_PROJECT_FILES=3`; upload 5 file baru -> 3 created + 2 `project_file_limit`; upload yang meng-update file existing saat kuota penuh tetap `updated`.
- Konkurensi: `MAX_PROJECT_FILES=3`; dua upload paralel (goroutine, `sync.WaitGroup`) masing-masing 2 file baru berbeda -> total di DB = 3 file; tepat satu item `project_file_limit` di gabungan kedua respons; ulangi 20 kali (stabil). Dua upload paralel dengan path SAMA -> satu created, satu `unchanged` (atau `updated` bila isi beda), tanpa 500 dan tanpa duplikat baris.
- 413 total: `MAX_UPLOAD_BYTES` kecil di config test (mis. 4 KiB), upload 5 KiB total -> 413 `upload_too_large` dan 0 baris tertulis, 0 entri audit.
- `MAX_UPLOAD_FILES=2`, upload 3 file valid -> 2 created + item ketiga `rejected too_many_files`.
- Kegagalan DB di tengah batch: sisipkan trigger/constraint pada tabel test (mis. `CHECK` yang menolak path tertentu via `ALTER TABLE ... ADD CONSTRAINT ... NOT VALID`, atau drop kolom di tx test) agar baris ke-N gagal -> 500, 0 baris tertulis (rollback batch), 0 entri audit.
- Audit gagal (fail-open): handler `core.audit.record` palsu mengembalikan error -> upload tetap 200, error di-log (tangkap `slog` handler test).
- Audit: tidak ada string isi file di `Args`/`Result` (isi unik `RAHASIA-123`, rule 13.6).
- Non-anggota -> 404 (AC6); `projectID` bukan uuid -> 404; user nonaktif/tanpa sesi -> 401.
- Tsvector: file yang di-upload langsung dapat dicari (`search` kata unik di isi dan di nama file).
- Cascade: hapus project -> file hasil upload hilang.

---

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01, T02, T03 merge)

Positif:
1. Login developer, buat project, tab Knowledge, pilih folder `docs/` (3 tingkat, `webkitdirectory`) -> laporan "3 dibuat"; pohon menampilkan `docs/auth/flow/x.md`.
2. Upload ulang folder yang sama -> laporan semua `unchanged`, tidak ada perubahan tanggal.
3. Ubah satu file lokal lalu upload ulang folder -> laporan 1 `updated` + sisanya `unchanged`; buka file, isi baru tampil.
4. Super Admin (bukan anggota) membuka knowledge project -> bisa upload.
5. Upload multi-file (bukan folder) lewat dialog file `.md` + `.txt` -> nama sederhana tanpa folder tersimpan di root.
6. Upload 100 file -> laporan menampilkan ringkasan dan daftar item; riwayat audit (halaman audit log, admin) menampilkan satu entri `knowledge.upload.completed`.

Negatif:
1. Upload folder berisi `.git/config`, gambar `.png`, file 2 MB, biner `.md` -> laporan `skipped`/`rejected` dengan alasan terbaca; file valid tetap masuk dan tampil di pohon.
2. Upload melebihi batas total (`MAX_UPLOAD_BYTES` dikecilkan pada env E2E) -> pesan "ukuran total upload melebihi batas", pohon tidak berubah.
3. Developer non-anggota memanggil endpoint upload project lain (`page.request.post` dengan CSRF header) -> 404.
4. Upload tanpa header CSRF (`page.request`) -> 403 `CSRF_REQUIRED`.
5. Upload tanpa sesi -> 401 dan UI mengarahkan ke login.
6. Project penuh kuota (`MAX_PROJECT_FILES` kecil) -> item baru `project_file_limit`, item update tetap berhasil.
7. Kirim body bukan multipart (via `page.request.post` JSON) -> 400 `not_multipart`.

---

## 8. Request/Response body

Format error seragam (rule 11.1):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "upload tidak berisi file", "details": { "reason": "no_files" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

### Request

```
POST /api/projects/0192f0c4-0000-7000-8000-0000000000aa/knowledge/upload
Content-Type: multipart/form-data; boundary=----b
X-Requested-With: grimoire
Cookie: <sesi>

------b
Content-Disposition: form-data; name="files"; filename="docs/auth/login.md"
Content-Type: text/markdown

# Login
Alur login memakai email dan password.
------b
Content-Disposition: form-data; name="files"; filename="../x.md"
Content-Type: text/markdown

x
------b
Content-Disposition: form-data; name="files"; filename="docs/big.md"
Content-Type: text/markdown

<2 MB>
------b
Content-Disposition: form-data; name="files"; filename=".git/config"
Content-Type: application/octet-stream

[core]
------b
Content-Disposition: form-data; name="files"; filename="docs/readme.md"
Content-Type: text/markdown

# Readme v2
------b
Content-Disposition: form-data; name="files"; filename="docs/a.md"
Content-Type: text/markdown

# A (tidak berubah)
------b--
```

Aturan field: nama field `files` (berulang); `filename` membawa path relatif lengkap (dari `webkitRelativePath`); bagian lain diabaikan.

### 200 OK

```json
{
  "summary": { "created": 1, "updated": 1, "unchanged": 1, "skipped": 1, "rejected": 2 },
  "items": [
    { "path": "docs/auth/login.md", "status": "created" },
    { "path": "../x.md", "status": "rejected", "reason": "path_traversal" },
    { "path": "docs/big.md", "status": "rejected", "reason": "too_large" },
    { "path": ".git/config", "status": "skipped", "reason": "hidden" },
    { "path": "docs/readme.md", "status": "updated" },
    { "path": "docs/a.md", "status": "unchanged" }
  ]
}
```

`reason` hanya ada untuk `skipped`/`rejected`. Nilai `reason` item: `path_empty`, `path_absolute`, `path_traversal`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`, `hidden` (skipped), `unsupported_type`, `too_large`, `duplicate_in_request`, `too_many_files`, `project_file_limit`.

Contoh item tertolak lain: `{ "path": "<600 karakter dipotong 1024 rune>", "status": "rejected", "reason": "path_too_long" }`, `{ "path": "x.png", "status": "rejected", "reason": "unsupported_type" }`, `{ "path": "bin/data.md", "status": "rejected", "reason": "invalid_encoding" }`, `{ "path": "docs/a.md", "status": "rejected", "reason": "duplicate_in_request" }`.

### 400 bukan multipart

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "body harus multipart/form-data dengan field files", "details": { "reason": "not_multipart" } }, "trace_id": "..." }
```

### 400 tanpa part file

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "upload tidak berisi file", "details": { "reason": "no_files" } }, "trace_id": "..." }
```

### 413 total melebihi batas

```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran total upload melebihi batas", "details": { "reason": "upload_too_large", "max_bytes": 52428800 } }, "trace_id": "..." }
```

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

### 403 tanpa header CSRF

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib diisi" }, "trace_id": "..." }
```

(Teks pesan dari middleware T00; yang dijamin hanya `code` dan status.)

### 404 non-anggota / projectID bukan uuid

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

### 500 kegagalan DB (batch di-rollback)

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server" }, "trace_id": "..." }
```

(Kode/pesan generik dari `respond`; tidak membocorkan detail SQL.)

### Audit yang dikirim (setelah commit, satu per request sukses)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000a", ActorLabel: "Budi",
    ProjectID: "0192f0c4-0000-7000-8000-0000000000aa", ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeUploadCompleted, TargetType: "knowledge", Target: "",
    Args:   map[string]any{"files": 6},
    Result: map[string]any{"created": 1, "updated": 1, "unchanged": 1, "skipped": 1, "rejected": 2,
        "bytes": 58, "paths": []string{"docs/auth/login.md", "docs/readme.md"}, "paths_truncated": false},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "0192f0c4-...",
}
```

`bytes` = total byte isi yang ditulis (created+updated). `paths` maks 20 (created/updated urut request). Isi file tidak pernah ada di audit.
