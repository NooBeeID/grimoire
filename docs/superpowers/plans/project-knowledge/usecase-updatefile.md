# T05 project/knowledge — Usecase `updatefile`

Konteks bersama (migrasi `knowledge_files`, konstanta bus, `base/`, `base/store`, `base/access`, `base/audit`, aksi audit, OpenAPI, config) ada di `overview.md` dan dikerjakan di langkah foundation F. Dokumen ini hanya memuat usecase `updatefile`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T05-project-knowledge.md`. Kriteria: AC3 (konflik edit), AC6 (non-anggota 404).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Scope | fullstack (backend di dokumen ini; editor dan dialog konflik di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Feature | `updatefile` |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| allowedPaths | `backend/internal/domain/project/internal/knowledge/updatefile/`, baris `updatefile.Init(d)` di `.../knowledge/base.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}/knowledge/files?path=<path>` | PUT | tanpa prefix versi (prefix `/api`; `info.version` 0.1.0) | member, owner, super_admin (semua level yang lolos `check-access`). Non-anggota, project tidak ada, atau `projectID` bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json`, sesi login (`RequireAuth`). Body limit route: `middleware.BodyLimit(limits.JSONBodyLimit())` = `6*MaxFileBytes + 64 KiB`.

Kontrak bus yang dipakai (nama dan tipe persis plan pemilik, tidak didefinisikan ulang):

| Endpoint | Mode | Provider | Kegagalan | Dipakai untuk |
|---|---|---|---|---|
| `project.management.check-access` | Call | T03 | fail-closed (500) | otorisasi via `base/access` |
| `project.management.get-project` | Call | T03 | fail-soft (label kosong) | label project audit via `base/audit` |
| `core.audit.record` | Call setelah commit | T02 | fail-open (log error) | `knowledge.file.updated` via `base/audit` |

Tidak ada endpoint bus baru, tidak ada migrasi, tidak ada env baru, tidak ada kode error baru.

## 2. Summary Plan

`updatefile` menyimpan isi baru satu file knowledge dengan optimistic concurrency. Satu usecase, tidak dipecah.

Alur `Execute` (orkestrasi saja; rule bisnis ada di `base`: `NormalizePath`, `PrepareContent`, `ValidVersion`, `Hash`, `Version`, `File.WithContent`, `File.ToDTO`, `File.ToConflict`, `VersionConflict`):

1. Aktor dari `reqctx` lalu `access.Require(ctx, projectID)` (401/404/500). Selalu pertama, agar non-anggota mendapat 404 walau input salah.
2. `base.NormalizePath(path)`; gagal -> `base.Invalid(reason)` (400).
3. `Content` tidak dikirim (nil) -> 400 (lihat keputusan U-1). Lalu `base.PrepareContent([]byte(content), Limits.MaxFileBytes)`: BOM dibuang, UTF-8 valid, tanpa NUL; `too_large` -> `base.TooLarge` (413, `details.max_bytes`); lainnya -> `base.Invalid` (400).
4. `base.ValidVersion(version)` salah/kosong -> `base.Invalid(ReasonInvalidVersion)` (400). Urutan ini (isi dulu, version kemudian) mengikuti spesifikasi usecase.
5. `newHash := base.Hash(prepared)`; bila `base.Version(newHash) == version` -> jalur no-op terverifikasi (D12): `FindByPath`; tidak ada -> 404; hash tersimpan != `version` -> 409 `version_conflict` dengan `current`; sama -> 200 dengan DTO file tersimpan, TANPA UPDATE dan TANPA audit.
6. Selain itu: `UPDATE` atomik dengan syarat `content_hash = <version>` (satu statement, tanpa lock aplikasi, tanpa tx eksplisit). 1 baris -> sukses. 0 baris -> `FindByPath`: tidak ada -> 404 `file tidak ditemukan`; ada -> 409 `version_conflict` dengan `details.current{content, version, updated_at, updated_by_label}`.
7. Sukses: kirim audit `knowledge.file.updated` setelah commit (fail-open), `Result {bytes_before, bytes}`. Respons 200 `FileDTO` dengan `version` baru.

Keputusan khusus usecase ini:

- **U-1 `content` wajib ada.** Body `{ "version": "..." }` tanpa `content` bukan berarti "kosongkan file": ditolak 400 `invalid_encoding` dengan pesan khusus `content wajib diisi (boleh string kosong)` lewat `base.InvalidMsg`. Isi `""` yang dikirim eksplisit valid (file boleh kosong). Memakai `*string` di `params.go` untuk membedakan. Tidak menambah `Reason` baru (foundation tidak diubah).
- **U-2 Tidak ada pemeriksaan ekstensi/hidden.** File sudah ada berarti sudah lolos aturan itu saat dibuat. Path hidden atau ekstensi tak didukung pada update berujung 404 karena tidak mungkin ada.
- **U-3 Atomisitas dan `bytes_before`.** UPDATE memakai CTE yang mengunci baris lama (`FOR UPDATE`) dan mengembalikan `size_bytes` lama. Pada READ COMMITTED, editor kedua yang menunggu lock akan mengevaluasi ulang syarat `content_hash` terhadap versi baris yang sudah di-commit editor pertama, sehingga 0 baris (AC3). Tanpa SELECT-lalu-UPDATE terpisah, tanpa advisory lock.
- **U-4 Retry sekali.** Bila setelah 0 baris `FindByPath` menemukan hash tersimpan == `version` (baris berubah lalu kembali ke isi semula di antara dua statement), UPDATE diulang satu kali; hasil ulang 0 baris langsung 409. Mencegah 409 palsu, tidak ada loop tak terbatas.
- **U-5 Kolom yang ditulis.** `content`, `title` (dihitung ulang entity), `content_hash`, `size_bytes`, `updated_at = now()`, `updated_by`, `updated_by_label`. `created_*`, `id`, `project_id`, `path` tidak berubah. Kolom `search` generated dihitung ulang Postgres.
- **U-6 Hash dihitung setelah BOM dibuang.** Version = hex(sha256(isi tersimpan)), konsisten dengan `getfile`, `createfile`, dan upload.
- **U-7 Isi file tidak pernah masuk log atau audit** (rule 10.3, 13.6). Konten pada `details.current` hanya ada di respons 409 ke pemanggil yang sudah lolos otorisasi.
- **U-8 Label aktor** dari `reqctx.Identity` (Name, fallback Email), lewat `base.ActorFrom` (D6).
- **U-9 Timeout.** Tidak ada tx panjang; satu statement. Panggilan bus (check-access sebelum, audit sesudah) tidak pernah di dalam transaksi.

Prasyarat (dari F; bila belum ada, hentikan dan selesaikan F dulu, jangan membuatnya dari folder `updatefile/`): migrasi `knowledge_files`; paket `base` lengkap (4.1-4.13 overview); `base/store` (`New`, `Q`, `FindByPath`); `base/access`; `base/audit`; `record.ActionKnowledgeFileUpdated` (T02); entri OpenAPI `updatefile` di `knowledge.yml`.

## 3. Plan (langkah berurutan)

Semua file di `backend/internal/domain/project/internal/knowledge/updatefile/`. Urutan TDD: tulis tes dulu per langkah.

1. **Verifikasi prasyarat.** `go build ./...` lulus; `ls backend/internal/domain/project/internal/knowledge/base/store` ada; `grep -n "operationId: updatefile" api/modules/project/knowledge.yml` menunjukkan `PUT /api/projects/{projectID}/knowledge/files` dengan query `path` wajib, body `KnowledgeUpdateRequest` (`content`, `version` wajib), respons 200 `KnowledgeFile`, 400/401/403(CSRF)/404/409 (`KnowledgeConflictDetails`)/413/500. Bila selisih dengan bagian 7 dokumen ini, perbaiki `knowledge.yml` di PR ini, `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus (rule 11.6).

2. **`params.go`** (bentuk request, tanpa logika bisnis):
   ```go
   // UpdateRequest adalah body PUT. Content pointer agar "tidak dikirim" (nil) dibedakan dari "" (kosong sengaja).
   type UpdateRequest struct {
       Content *string `json:"content"`
       Version string  `json:"version"`
   }
   ```

3. **`repository.go`** (satu-satunya SQL khusus feature; membungkus `*store.Store`):
   ```go
   type Repo struct{ s *store.Store }
   func NewRepo(s *store.Store) *Repo

   // FindByPath mendelegasikan ke store.FindByPath (SQL bersama).
   func (r *Repo) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)

   // UpdateIfVersion menjalankan UPDATE atomik. found=false bila 0 baris (path tidak ada ATAU hash berbeda).
   // projectID bukan uuid -> (zero, 0, false, nil) tanpa query.
   func (r *Repo) UpdateIfVersion(ctx context.Context, projectID, path string, expectedHash []byte, f base.File) (updated base.File, bytesBefore int, found bool, err error)
   ```
   SQL (parameter saja, rule 6.10 `project_id = $1`):
   ```sql
   WITH old AS (
       SELECT id, size_bytes
       FROM knowledge_files
       WHERE project_id = $1 AND path = $2 AND content_hash = $3
       FOR UPDATE
   )
   UPDATE knowledge_files f
   SET content = $4, title = $5, content_hash = $6, size_bytes = $7,
       updated_at = now(), updated_by = $8, updated_by_label = $9
   FROM old
   WHERE f.id = old.id
   RETURNING f.id, f.project_id, f.path, f.title, f.content, f.content_hash, f.size_bytes,
             f.created_at, f.created_by, f.created_by_label,
             f.updated_at, f.updated_by, f.updated_by_label,
             old.size_bytes AS bytes_before
   ```
   Parameter: `$1` projectID, `$2` path, `$3` expectedHash (`[]byte`), `$4` f.Content, `$5` f.Title, `$6` f.Hash, `$7` f.SizeBytes, `$8` f.UpdatedBy (uuid), `$9` f.UpdatedByLabel. `sql.ErrNoRows` -> `found=false`. Pindai ke row struct privat bertag `db` (sama dengan yang dipakai `store`; bila `store` mengekspor helper scan, pakai itu, jangan menduplikasi). Waktu `.UTC()`. Error DB dibungkus `fmt.Errorf("updatefile: update: %w", err)`.

   Tidak memakai `InTx`: statement tunggal sudah atomik.

4. **`usecase.go`**:
   ```go
   // Sisi consumer (rule 5.6). *Repo memenuhinya.
   type Repository interface {
       FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
       UpdateIfVersion(ctx context.Context, projectID, path string, expectedHash []byte, f base.File) (base.File, int, bool, error)
   }

   type Input struct {
       ProjectID string
       Path      string  // mentah dari query
       Content   *string // nil = tidak dikirim
       Version   string
       Meta      base.RequestMeta
   }

   type Usecase struct {
       repo   Repository
       access access.Checker
       audit  audit.Recorder
       limits base.Limits
   }
   func NewUsecase(r Repository, a access.Checker, au audit.Recorder, l base.Limits) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (base.FileDTO, error)
   ```
   Alur `Execute`:
   1. `actor, err := u.access.Require(ctx, in.ProjectID)`; error dikembalikan apa adanya.
   2. `path, r := base.NormalizePath(in.Path)`; `r != ""` -> `base.Invalid(r)`.
   3. `in.Content == nil` -> `base.InvalidMsg(base.ReasonInvalidEncoding, "content wajib diisi (boleh string kosong)")`.
   4. `content, r := base.PrepareContent([]byte(*in.Content), u.limits.MaxFileBytes)`; `r == base.ReasonTooLarge` -> `base.TooLarge(r, u.limits.MaxFileBytes)` (pesan memformat angka batas); `r != ""` lainnya -> `base.Invalid(r)`.
   5. `!base.ValidVersion(in.Version)` -> `base.Invalid(base.ReasonInvalidVersion)`.
   6. `expected, _ := hex.DecodeString(in.Version)`; `newHash := base.Hash(content)`.
   7. Jalur no-op: `bytes.Equal(newHash, expected)`:
      - `cur, ok, err := u.repo.FindByPath(...)`; err -> `fmt.Errorf("updatefile: cari file: %w", err)`; `!ok` -> `base.ErrFileNotFound`.
      - `!bytes.Equal(cur.Hash, expected)` -> `base.VersionConflict(cur.ToConflict())`.
      - sama -> `return cur.ToDTO(), nil` (tanpa audit).
   8. Jalur tulis (maks 2 percobaan, U-4):
      ```go
      next := base.File{ProjectID: in.ProjectID, Path: path}.WithContent(content, actor) // Title, Hash, SizeBytes, Updated*
      for attempt := 0; attempt < 2; attempt++ {
          upd, before, found, err := u.repo.UpdateIfVersion(ctx, in.ProjectID, path, expected, next)
          if err != nil { return base.FileDTO{}, err }
          if found {
              u.audit.Record(ctx, audit.Event{
                  Action: record.ActionKnowledgeFileUpdated, ProjectID: in.ProjectID, Actor: actor,
                  TargetType: "file", Target: path,
                  Args:   map[string]any{},
                  Result: map[string]any{"bytes_before": before, "bytes": upd.SizeBytes},
                  Meta:   in.Meta,
              })
              return upd.ToDTO(), nil
          }
          cur, ok, err := u.repo.FindByPath(ctx, in.ProjectID, path)
          if err != nil { return base.FileDTO{}, fmt.Errorf("updatefile: cari file: %w", err) }
          if !ok { return base.FileDTO{}, base.ErrFileNotFound }
          if !bytes.Equal(cur.Hash, expected) { return base.FileDTO{}, base.VersionConflict(cur.ToConflict()) }
          // hash cocok lagi: balapan langka, ulangi sekali
      }
      return base.FileDTO{}, base.VersionConflict(<current terakhir>.ToConflict())
      ```
      Simpan `cur` terakhir di variabel luar loop untuk konflik setelah percobaan kedua. Bila `File.WithContent` tidak bisa dipanggil pada struct kosong (periksa implementasi F), buat helper kecil lokal yang memanggil `base.NewFile(...)` lalu memakai field yang dibutuhkan; JANGAN menaruh perhitungan Title/Hash di usecase.
   9. Usecase tidak menyentuh `sqlx`, tidak berisi rule bisnis baru. Audit dipanggil hanya setelah UPDATE commit (statement tunggal sudah commit saat kembali).

5. **`handler.go`**:
   ```go
   type Handler struct{ uc *Usecase }
   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
   ```
   - `var req UpdateRequest`; `respond.DecodeJSON(w, r, &req)` (field tak dikenal -> 400; `*http.MaxBytesError` -> 413 oleh `respond`); gagal -> `respond.WriteError` dan return. Catatan: decode terjadi sebelum otorisasi (tidak bergantung pada project, pola T03); body valid dari non-anggota tetap berakhir 404.
   - `in := Input{ProjectID: chi.URLParam(r, "projectID"), Path: r.URL.Query().Get("path"), Content: req.Content, Version: req.Version, Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}}`.
   - `dto, err := h.uc.Execute(r.Context(), in)`; `err` -> `respond.WriteError(w, r, err)`; sukses -> `respond.WriteJSON(w, r, http.StatusOK, dto)`.
   - Log hanya `project_id`, `user_id`, `feature=updatefile`, `path`. DILARANG me-log body.

6. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       limits := base.LimitsFrom(d.Config)
       repo := NewRepo(store.New(d.DB))
       uc := NewUsecase(repo, access.New(d.Bus), audit.New(d.Bus, d.Log), limits)
       h := &Handler{uc: uc}
       d.API.With(middleware.RequireAuth(), middleware.BodyLimit(limits.JSONBodyLimit())).
           Put("/projects/{projectID}/knowledge/files", h.ServeHTTP)
   }
   ```
   Sesuaikan penulisan pola route dengan konvensi `d.API` T00/T03 yang sudah di `main` (pola penuh per T03 D4; lihat `management/*/init.go` pertama yang ada). Jangan mendaftarkan GET/POST/DELETE di sini (milik feature lain; pola sama dengan method berbeda aman di chi).

7. **Wiring**: tambah satu baris `updatefile.Init(d)` di `knowledge/base.go` (hanya baris itu; bila `base.go` belum ada karena F belum selesai, hentikan). Tidak mengubah feature lain.

8. **Tes** (bagian 5), jalankan `go test ./internal/domain/project/internal/knowledge/updatefile/...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/...`; `go vet`, `internal/archtest` lulus (feature tidak meng-import feature lain).

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files` dari migrasi F `backend/migrations/project/<VERSION>_create_knowledge_files.sql`. Tidak ada indeks baru: `UNIQUE (project_id, path)` melayani syarat `project_id = $1 AND path = $2`, filter `content_hash` dievaluasi pada satu baris hasil indeks.

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `.../knowledge/updatefile/params.go` | baru: `UpdateRequest` |
| `.../knowledge/updatefile/repository.go` | baru: `Repo`, `FindByPath` (delegasi), `UpdateIfVersion` (SQL bagian 3.3) |
| `.../knowledge/updatefile/usecase.go` | baru: `Repository`, `Input`, `Usecase`, `Execute` |
| `.../knowledge/updatefile/handler.go` | baru: handler PUT |
| `.../knowledge/updatefile/init.go` | baru: rakit dependensi + daftar route PUT dengan `BodyLimit(JSONBodyLimit())` |
| `.../knowledge/updatefile/usecase_test.go` | baru: tes unit dengan repo/access/audit palsu |
| `.../knowledge/updatefile/repository_integration_test.go` | baru (tag `integration`): SQL nyata |
| `.../knowledge/updatefile/handler_test.go` | baru: httptest + router chi |
| `.../knowledge/base.go` | tambah satu baris `updatefile.Init(d)` |
| `api/modules/project/knowledge.yml` | hanya bila selisih dengan bagian 7 (biasanya sudah dibuat di F); regenerate `api/openapi.yml` |

Tidak ada perubahan pada `base/`, `base/store`, `base/access`, `base/audit`, kontrak bus, konstanta, config, katalog README (katalog aksi `knowledge.file.updated` sudah ada; `Result` mengikuti tabel 4.13 overview).

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, table-driven, fake `Repository`, `access.Checker`, `audit.Recorder` yang menghitung dan menyimpan event)

Fake repo memodelkan satu baris di memori dengan semantik `UpdateIfVersion` yang sama (cocok hash -> tulis, tidak -> `found=false`).

1. Sukses: file `v1`, update ke `v2` dengan `version = Version(Hash(v1))` -> DTO `content=v2`, `version = Version(Hash(v2))`, `size_bytes=len(v2)`, `updated_by.label` = label aktor, `created_by` tidak berubah; tepat 1 event audit `Action=knowledge.file.updated`, `TargetType=file`, `Target=path`, `Result={bytes_before: len(v1), bytes: len(v2)}`, `Args` kosong; event TIDAK memuat string isi (periksa semua nilai `Args`/`Result`).
2. Title dihitung ulang: update isi dari `# A` ke `# B` -> `title=B`.
3. No-op: isi baru identik dengan tersimpan, `version` = hash tersimpan -> 200, DTO file tersimpan, repo tidak menerima `UpdateIfVersion`, 0 audit, `updated_at` tidak berubah.
4. No-op palsu (D12): isi baru identik dengan isi yang diketahui klien tetapi `version` == hash isi baru sedangkan tersimpan sudah berbeda (orang lain mengubah) -> 409 `version_conflict`, `current` = isi tersimpan, 0 audit.
5. No-op, file tidak ada -> 404 `file tidak ditemukan`.
6. AC3: dua pemanggil dengan `version` sama; panggilan A sukses; panggilan B (isi lain) -> 409, `details.reason=version_conflict`, `details.current.content` = isi A, `current.version` = hash A, `current.updated_by_label` = label A; isi di repo tetap A; audit hanya 1 (milik A).
7. Path tidak ada, `version` valid hash lain -> `UpdateIfVersion` found=false, `FindByPath` tidak ada -> 404, 0 audit.
8. `version` salah bentuk (tabel): `""`, 63 hex, 65 hex, huruf besar, `zz`*32 -> 400 `invalid_version`; repo tidak dipanggil.
9. Isi terlalu besar: `MaxFileBytes+1` byte -> 413, `details.reason=too_large`, `details.max_bytes=MaxFileBytes`; tepat `MaxFileBytes` OK; BOM + tepat batas OK (diukur setelah BOM dibuang).
10. Isi tidak valid: `\xff\xfe`, mengandung `\x00` -> 400 `invalid_encoding`.
11. `content` nil -> 400 `invalid_encoding` pesan `content wajib diisi (boleh string kosong)`; `content` `""` -> sukses (file jadi kosong, `size_bytes=0`, `title=""`).
12. BOM dibuang: isi `"﻿# T"` -> tersimpan `"# T"`, hash dihitung dari isi tanpa BOM.
13. Path tidak valid (tabel): `""` path_empty, `/a.md` path_absolute, `../x.md` path_traversal, `a//b.md` path_empty_segment, NUL path_control_char, 513 rune path_too_long -> 400 dengan `details.reason` sesuai; urutan: access dulu (access ditolak + path salah -> 404, bukan 400).
14. Urutan validasi: access error menang atas semua; path salah menang atas content salah; isi salah (413) menang atas version salah (spesifikasi).
15. Non-anggota: `access.Require` mengembalikan `ErrProjectNotFound` -> 404, repo tidak dipanggil; tanpa identitas -> 401; error bus -> 500 (error terbungkus, bukan `*errs.Error` bisnis).
16. Error DB pada `UpdateIfVersion` -> error terbungkus, 0 audit.
17. Retry (U-4): fake repo yang mengembalikan `found=false` sekali, lalu `FindByPath` hash == `version`, lalu sukses pada percobaan kedua -> 200 + 1 audit; bila gagal dua kali -> 409; tidak lebih dari 2 pemanggilan `UpdateIfVersion`.
18. Audit fail-open: recorder yang gagal internal tidak mengubah respons 200 (recorder tidak mengembalikan error; pastikan usecase tidak bergantung pada hasilnya).

### Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

1. `UpdateIfVersion` hash cocok -> baris berubah: `content`, `title`, `content_hash`, `size_bytes`, `updated_at` (> sebelumnya), `updated_by`, `updated_by_label`; `created_at`, `created_by`, `created_by_label`, `id` tidak berubah; `bytes_before` = ukuran lama.
2. Hash tidak cocok -> `found=false`, baris tidak berubah.
3. Path tidak ada -> `found=false`. Project lain dengan `path` sama tidak tersentuh (isolasi, `project_id = $1`).
4. `projectID` bukan uuid -> `found=false`, tanpa error dan tanpa query.
5. Konkurensi (AC3 di level SQL): dua goroutine memanggil `UpdateIfVersion` dengan `expectedHash` sama dan isi berbeda secara bersamaan (diulang 50 kali, sinkron dengan `sync.WaitGroup` dan barrier) -> tepat satu `found=true`, satu `found=false`; isi tersimpan = milik pemenang.
6. Kolom `search` ikut berubah: kata unik isi lama tidak lagi cocok `websearch_to_tsquery('simple', ...)`, kata unik isi baru cocok.
7. Isi 1 MB berisi kata unik bisa di-update (tsvector tidak melampaui batas, D4).
8. Isi kosong `""` valid (`size_bytes=0`).

### Handler (`handler_test.go`, httptest + chi, usecase dengan fake atau stack nyata ringan)

1. 200 pada update valid; header `Content-Type: application/json`; body = `FileDTO` snake_case (`size_bytes`, `created_at`, `created_by{id,label}`, `updated_at`, `updated_by{id,label}`, `version`).
2. `path` dibaca dari query (`docs%2Fauth%2Flogin.md` terdecode `docs/auth/login.md`); `path` tidak ada -> 400 `path_empty`.
3. Body bukan JSON / field tak dikenal (`{"content":"x","version":"..","extra":1}`) -> 400.
4. Body > `JSONBodyLimit()` -> 413 (`PAYLOAD_TOO_LARGE`).
5. JSON ber-escape berat: isi 1 MiB berisi karakter kontrol (`\u0001` x N, ditolak `invalid_encoding`) dan isi 1 MiB biasa ber-escape `\n`/`\"` tidak ditolak oleh `BodyLimit` (batas 6x cukup).
6. 409 menyajikan `error.code=CONFLICT`, `error.details.reason=version_conflict`, `error.details.current.{content,version,updated_at,updated_by_label}` dan `trace_id`.
7. Tanpa sesi -> 401; tanpa `X-Requested-With: grimoire` -> 403 `CSRF_REQUIRED` (middleware T00).
8. `TestRoutesRegistered` (di W): `PUT /api/projects/{projectID}/knowledge/files` terdaftar.

### Integrasi lintas usecase (ditambahkan ke `knowledge_integration_test.go` pada langkah W; untuk PR feature ini cukup tes berikut dalam `updatefile/` dengan tag `integration`)

Router `/api` asli T00 + bus dengan `check-access` dan `get-project` palsu + `core.audit.record` palsu yang menghitung entri.

1. **AC3**: `createfile` `docs/a.md`; dua `getfile` (version sama V0); `PUT` editor A (isi A, V0) -> 200; `PUT` editor B (isi B, V0) -> 409 dengan `details.current.content = A`; `getfile` -> isi A; audit `knowledge.file.updated` tepat 1.
2. Dua `PUT` bersamaan (goroutine) dengan V0 -> tepat satu 200 dan satu 409; audit 1.
3. Path tidak ditemukan -> 404; version malformed -> 400 `invalid_version`; isi `MaxFileBytes+1` -> 413; non-anggota -> 404 (juga dengan `version` malformed dan `projectID` bukan uuid; tidak membocorkan keberadaan file).
4. No-op: `PUT` isi identik dengan V0 -> 200, `updated_at` sama, audit 0.
5. Audit: entri berisi `bytes_before` dan `bytes`, `Target=path`, `ActorLabel`, `ProjectLabel` dari `get-project` palsu; tidak ada string isi file di `Args`/`Result` (cari string isi unik, rule 13.6); `core.audit.record` gagal -> `PUT` tetap 200 dan error di-log.
6. Pencarian setelah update (bila `search` sudah ada; kalau belum, cek via SQL `knowledge_files.search`): kata baru ditemukan, kata lama tidak.
7. `getfile` setelah update mengembalikan `version = hex(sha256(isi baru))`; `PUT` berikutnya dengan version itu sukses (rantai edit).

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01, T02, T03, plus UI knowledge dari `frontend-plan.md`)

Positif:
1. Developer anggota login -> buka project -> tab Knowledge -> pilih `docs/auth/login.md` -> Edit -> ubah teks -> Simpan -> indikator "belum disimpan" hilang, viewer menampilkan isi baru, reload halaman tetap menampilkan isi baru.
2. Simpan dua kali berturut-turut tanpa reload (edit, simpan, edit lagi, simpan): keduanya berhasil (klien memakai `version` baru dari respons 200 untuk simpan kedua).
3. Simpan tanpa perubahan (tombol Simpan ditekan pada isi identik) -> sukses diam-diam, tanpa entri baru di Audit Log.
4. Super Admin (bukan anggota) mengedit file di project mana pun -> berhasil; Audit Log menampilkan `knowledge.file.updated` dengan label aktor dan path.
5. Setelah simpan, pencarian kata yang baru ditambahkan menemukan file itu.

Negatif:
1. Dua tab (dua sesi developer) membuka file yang sama; tab 1 menyimpan; tab 2 menyimpan -> dialog konflik 409 ("file sudah diubah orang lain") dengan opsi muat ulang/bandingkan memakai `details.current`; isi tab 1 tidak tertimpa; pilih "muat ulang" -> editor memuat isi tab 1 dan `version` baru.
2. File dihapus orang lain saat sedang diedit, lalu simpan -> pesan "file tidak ditemukan" (404), editor tidak menimpa.
3. Isi lebih dari batas (tempel teks > 1 MiB) lalu simpan -> pesan 413 dengan batas, isi di editor tidak hilang.
4. Isi mengandung karakter NUL (tempel biner) -> pesan alasan `invalid_encoding`.
5. Developer non-anggota membuka URL knowledge project lain dan mencoba menyimpan (lewat `page.request.put`) -> 404.
6. Request `PUT` tanpa header CSRF (lewat `page.request`) -> 403 `CSRF_REQUIRED`.
7. Sesi kedaluwarsa saat menyimpan -> 401, UI mengarahkan ke login tanpa kehilangan draft yang belum disimpan (perilaku UI, sesuai `frontend-plan.md`).

## 8. Request/Response body

Format error seragam (rule 11.1):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "version wajib diisi dengan nilai dari getfile", "details": { "reason": "invalid_version" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

### PUT /api/projects/{projectID}/knowledge/files?path=docs%2Fauth%2Flogin.md

Header: `Content-Type: application/json`, `X-Requested-With: grimoire`, cookie sesi.

Request:

```json
{
  "content": "# Login\n\nAlur login v2",
  "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea"
}
```

`version` = nilai `version` terakhir yang dibaca dari `getfile`/`createfile`/`updatefile`/`movefile` (hex sha256 isi, 64 karakter huruf kecil).

200 (`FileDTO` dengan `version` baru; `created_*` tidak berubah):

```json
{
  "path": "docs/auth/login.md",
  "title": "Login",
  "content": "# Login\n\nAlur login v2",
  "size_bytes": 24,
  "version": "9c1185a5c5e9fc54612808977ee8f548b2258d31ad2d3a2c3d8b1c5f1e6b7a90",
  "created_at": "2026-10-24T09:00:00Z",
  "created_by": { "id": "0192f0c4-0000-7000-8000-00000000000a", "label": "Budi" },
  "updated_at": "2026-10-24T09:05:00Z",
  "updated_by": { "id": "0192f0c4-0000-7000-8000-00000000000b", "label": "Sari" }
}
```

(Nilai `version` pada contoh ilustrasi; nyata = sha256 isi.) Respons 200 yang sama (isi, `version`, `updated_at` tersimpan, tanpa perubahan) dikembalikan pada kasus no-op.

409 (versi tidak cocok):

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "file sudah diubah orang lain; muat ulang sebelum menyimpan",
    "details": {
      "reason": "version_conflict",
      "current": {
        "content": "# Login\n\nAlur login versi Sari",
        "version": "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
        "updated_at": "2026-10-24T09:05:00Z",
        "updated_by_label": "Sari"
      }
    }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

404 (file tidak ada):

```json
{ "error": { "code": "NOT_FOUND", "message": "file tidak ditemukan" }, "trace_id": "..." }
```

404 (non-anggota, project tidak ada, atau `projectID` bukan uuid):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

400 (version tidak valid):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "version wajib diisi dengan nilai dari getfile", "details": { "reason": "invalid_version" } }, "trace_id": "..." }
```

400 (path tidak valid, contoh):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "..." }
```

400 (isi bukan teks UTF-8 / mengandung NUL):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "isi harus teks UTF-8 tanpa karakter NUL", "details": { "reason": "invalid_encoding" } }, "trace_id": "..." }
```

400 (`content` tidak dikirim):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "content wajib diisi (boleh string kosong)", "details": { "reason": "invalid_encoding" } }, "trace_id": "..." }
```

413 (isi melebihi `MAX_FILE_BYTES`):

```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran file melebihi batas 1048576 byte", "details": { "reason": "too_large", "max_bytes": 1048576 } }, "trace_id": "..." }
```

413 (body JSON melebihi `6*MaxFileBytes + 64 KiB`, dari `BodyLimit`/`respond`): `PAYLOAD_TOO_LARGE`.

401 tanpa sesi: `{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }`. 403 tanpa CSRF: `CSRF_REQUIRED`. 500: pesan generik, detail hanya di log.

### Audit (`record.Request` yang dikirim setelah update sukses)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-0000-7000-8000-00000000000b", ActorLabel: "Sari",
    ProjectID: pid, ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeFileUpdated, TargetType: "file", Target: "docs/auth/login.md",
    Args:   map[string]any{},
    Result: map[string]any{"bytes_before": 22, "bytes": 24},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "0192f0c4-7b00-...",
}
```

Tidak ada entri audit pada: no-op 200, 400, 404, 409, 413, 500.
