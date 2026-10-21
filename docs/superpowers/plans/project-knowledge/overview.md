# T05 project/knowledge — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T05-project-knowledge.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit, pola fail-open), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/project-management/overview.md` (T03: kontrak `check-access`, `get-project`, `contentcount`, pola module), `docs/superpowers/plans/core-audit-log/overview.md` (kontrak `core.audit.record`).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. Tidak ada knowledge doc module knowledge. Spec desain bernama `2026-10-20-grimoire-design.md` (tidak ada file `*knowledge-hub-design.md`; Grimoire = Knowledge Hub). Kode T00 dan T02 ada di `main`; T03 baru sampai tahap plan.

Dokumen ini adalah plan FOUNDATION untuk module `project/knowledge`: keputusan bersama, `base/` (entity + rule murni) dan subpackage I/O bersama, kontrak bus, konstanta endpoint, migrasi, config, urutan implementasi, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-knowledge/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini; bila bertentangan, dokumen ini yang menang sampai diperbarui. Frontend ditulis di `docs/superpowers/plans/project-knowledge/frontend-plan.md`.

---

## 0. Prasyarat dan nama infra yang dipakai

- **T03 WAJIB sudah merge** (tabel `projects` sebagai target FK, handler `project.management.check-access` dan `project.management.get-project`). Sebelum mulai: `ls backend/internal/domain/project/internal/management` dan `ls backend/migrations/project/*create_projects.sql`. Bila belum ada, STOP (T05 tidak bisa jalan tanpa FK dan otorisasi).
- Kontrak dari T03 yang dipakai (cek di `backend/lib/`; bila belum ada, buat PERSIS seperti `docs/superpowers/plans/project-management/overview.md` bagian 5.1-5.4, jangan membuat versi baru):
  - `lib/contracts/project/management/checkaccess`: `Request{ProjectID, UserID, UserRole string}` -> `Response{ProjectExists bool; Level Level}`, `Level` = `none|member|owner|admin`.
  - `lib/contracts/project/management/getproject`: `Request{ProjectID string}` -> `Response{Found bool; ID, Name string}`.
  - `lib/contracts/project/management/contentcount`: `MaxProjectIDs = 100`, `Request{ProjectIDs []string}` -> `Response{Counts map[string]int}`.
  - `lib/constants/bus.go`: `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`, `BUS_PROJECT_MANAGEMENT_GET_PROJECT`, `BUS_PROJECT_KNOWLEDGE_COUNT_FILES = "project.knowledge.count-files"`.
- Signature publik T00/T02/T03 TIDAK diubah. T05 hanya MENAMBAH (kecuali `actions.go`/`actions_test.go` T02 dan `config.go` T00 yang memang titik ekstensi resmi).
- Nama infra final di `main`:

| Simbol | Import path | Package |
|---|---|---|
| `Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` | `grimoire/internal/infra/app` | `app` |
| `Registry.Register`, `CallTyped[T]`, `Expect[T]`, `ErrNotRegistered`, `ErrBadRequestType` | `grimoire/internal/infra/bus` | `bus` |
| `WriteJSON`, `WriteError`, `DecodeJSON`, `StatusFor` | `grimoire/internal/infra/http/respond` | `respond` |
| `RequireAuth()`, `BodyLimit(n)`, `KeyByIP(r)`, `CSRFHeader` | `grimoire/internal/infra/middleware` | `middleware` |
| `DB` (embed `*sqlx.DB`), `DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `Querier`, `TestDB(t)` | `grimoire/internal/infra/db` | `db` |
| `Config` (`MaxFileBytes int64`, `MaxUploadBytes int64`, `MaxUploadFiles int`, `MCPMaxResponseBytes int`, + baru `MaxProjectFiles int`) | `grimoire/internal/infra/config` | `config` |
| `New`, `Wrap`, `WithDetails`, `Error`, `CodeNotFound`, `CodeConflict`, `CodeInvalid`, `CodeUnauthorized`, `CodePayloadTooLarge`, `ErrInvalid` | `grimoire/lib/common/errs` | `errs` |
| `IdentityFrom(ctx)` (`Identity{UserID, Email, Name, Role}`), `TraceID(ctx)` | `grimoire/lib/common/reqctx` | `reqctx` |
| `record.Request`, `ActorUser`, `StatusOK`, `ActionKnowledge*` | `grimoire/lib/contracts/core/audit/record` | `record` |

- Middleware yang SUDAH terpasang global di `d.API` (T00 `router.go`): `Timeout(HTTPDefaultTimeout=30s)`, `CSRF()` (mutasi wajib `X-Requested-With: grimoire`), `Session`, `PasswordChangeGate`. Root memasang `BodyLimit(HTTPDefaultBodyBytes=1 MiB)`; `BodyLimit` per-route yang lebih dalam MENANG (membungkus body asli), jadi upload dan JSON ber-content boleh menaikkan batas.
- Tidak ada kode error baru dan tidak ada `RegisterCodeStatus`. Semua kasus memakai kode bawaan (`NOT_FOUND`, `CONFLICT`, `INVALID_ARGUMENT`, `PAYLOAD_TOO_LARGE`, `UNAUTHORIZED`) dan dibedakan lewat `details.reason`.
- Tidak ada library baru. Hanya stdlib (`mime`, `mime/multipart`, `crypto/sha256`, `unicode/utf8`), `sqlx`, `pgx`, `chi`, `github.com/google/uuid` yang sudah di `go.mod`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (backend Go di plan ini; frontend di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge (`backend/internal/domain/project/internal/knowledge`) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Versi kontrak REST | Tanpa prefix versi di URL (prefix `/api`). `api/modules/project/knowledge.yml` memakai `info.version: 0.1.0` |
| Versi kontrak bus | v1 (4 endpoint baru di sisi provider) |
| Bergantung pada | T00 (merged), T02 (merged), T03 (WAJIB merged) |
| Dibutuhkan oleh | T08 (`list-files`, `read-file`, `search`), T03 (`count-files`, fail-soft) |

Endpoint HTTP (semua di `d.API` dengan pola penuh, param `{projectID}`, semua `middleware.RequireAuth()`; otorisasi project di usecase lewat `check-access`; semua level `member|owner|admin` boleh semua aksi T05):

| Endpoint | Method | Version | Body limit | Usecase |
|---|---|---|---|---|
| `/api/projects/{projectID}/knowledge/upload` | POST (multipart) | - | `MaxUploadBytes + 1 MiB` | `uploadfiles` |
| `/api/projects/{projectID}/knowledge/files` | POST | - | `6*MaxFileBytes + 64 KiB` | `createfile` |
| `/api/projects/{projectID}/knowledge/files?path=` | PUT | - | `6*MaxFileBytes + 64 KiB` | `updatefile` |
| `/api/projects/{projectID}/knowledge/files?path=` | GET | - | default | `getfile` |
| `/api/projects/{projectID}/knowledge/tree?prefix=` | GET | - | default | `listtree` |
| `/api/projects/{projectID}/knowledge/files?path=` | DELETE | - | default | `deletefile` |
| `/api/projects/{projectID}/knowledge/folders?path=` | DELETE | - | default | `deletefolder` |
| `/api/projects/{projectID}/knowledge/files/move` | POST | - | default (64 KiB cukup) | `movefile` |
| `/api/projects/{projectID}/knowledge/search?q=&prefix=&limit=` | GET | - | default | `search` |

Endpoint bus (provider T05):

| Endpoint | Mode | Version | Kontrak | Consumer | Usecase |
|---|---|---|---|---|---|
| `project.knowledge.list-files` | Call | v1 | `lib/contracts/project/knowledge/listfiles` | T08 | `buslistfiles` |
| `project.knowledge.read-file` | Call | v1 | `lib/contracts/project/knowledge/readfile` | T08 | `busreadfile` |
| `project.knowledge.search` | Call | v1 | `lib/contracts/project/knowledge/search` | T08 | `bussearch` |
| `project.knowledge.count-files` | Call | v1 | `lib/contracts/project/management/contentcount` (T03) | T03 | `countfiles` |

Endpoint bus yang dikonsumsi:

| Endpoint | Mode | Provider | Kegagalan | Dipakai oleh |
|---|---|---|---|---|
| `project.management.check-access` | Call | T03 | fail-closed (500) | semua 9 route HTTP (`base/access`) |
| `project.management.get-project` | Call | T03 | fail-soft (label kosong) | `base/audit` (label project untuk audit) |
| `core.audit.record` | Call setelah commit | T02 | fail-open (log error) | `uploadfiles`, `createfile`, `updatefile`, `deletefile`, `deletefolder`, `movefile` |

`core.auth.get-user` TIDAK dikonsumsi (deviasi dari baris "Dikonsumsi" tiket; D6).

---

## 2. Summary Plan

T05 menyimpan file knowledge (Markdown/teks) per project di Postgres, dengan struktur folder diturunkan dari path. Isinya:

- Satu tabel `knowledge_files` (migrasi `migrations/project/`) dengan kolom `search tsvector` generated (bobot A=path, B=judul, D=isi), indeks GIN, dan indeks `(project_id, path text_pattern_ops)` untuk prefix folder.
- Rule murni di `base/`: normalisasi dan validasi path/prefix, tipe file, persiapan isi (BOM/UTF-8/NUL/ukuran), hash/version, judul, rencana upload (created/updated/unchanged/skipped/rejected + kuota), pembersihan kueri pencarian, pohon satu level, potong UTF-8 untuk bus, path mirip, parse sorotan, kode alasan stabil, sentinel error.
- Tiga subpackage I/O bersama di bawah `base/`: `base/store` (SQL bersama: lock kuota, hitung, cari path, list meta, search), `base/access` (otorisasi via `check-access`, fail-closed), `base/audit` (audit via bus setelah commit, fail-open).
- 13 feature: 9 HTTP dan 4 handler bus (`buslistfiles`, `busreadfile`, `bussearch`, `countfiles`).
- 3 kontrak bus baru (`listfiles`, `readfile`, `search`) + provider `count-files` memakai kontrak netral T03.
- Satu aksi audit baru `knowledge.file.moved`. Satu env baru `MAX_PROJECT_FILES`.
- OpenAPI `api/modules/project/knowledge.yml`, pembaruan README (katalog bus/audit) dan tiket T05.

Hal penting soal keamanan:
- Non-anggota (dan `projectID` bukan uuid) selalu 404 di semua 9 route (rule 7.3, AC6).
- Semua query memakai `project_id = $1` (rule 6.10); handler bus tidak pernah mengembalikan data project lain (AC8), dibuktikan test isolasi.
- Path divalidasi dan ditolak dengan alasan, tidak diam-diam diperbaiki (kecuali satu `/` di depan dari agent di bus).
- Isi file tidak pernah masuk log atau audit (rule 10.3, 13.6).
- Kueri pencarian tidak pernah diteruskan mentah ke tsquery; tiap term dibungkus kutip.
- Cuplikan pencarian di HTTP dikirim sebagai segmen `{text, highlight}`, bukan HTML.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Keputusan baru (dicatat di PR; tiket/README diperbarui di PR yang sama)

| # | Keputusan | Alasan |
|---|---|---|
| D1 | Feature `busreader` di tiket dipecah menjadi `buslistfiles`, `busreadfile`, `bussearch`, ditambah `countfiles` | Rule 3.4: satu folder = satu handler bus. `count-files` diminta T03 D1 |
| D2 | Batas per request memakai env T00 yang sudah ada: `MAX_FILE_BYTES` 1 MiB, `MAX_UPLOAD_BYTES` 50 MB, `MAX_UPLOAD_FILES` 500. Default T00 menang atas angka tiket (20 MB/200 file) | T00 sudah merge dan terdokumentasi di `docs/configuration.md`; dua sumber angka akan membingungkan operator |
| D3 | Env baru `MAX_PROJECT_FILES` (int, default 5000, >= 1) | Tiket meminta batas jumlah file per project |
| D4 | Kolom `title` ditambahkan (dihitung entity dari heading `# ` pertama). `search` memakai `left(content, 200000)` | Tiket meminta "path/judul" lebih berat; batas tsvector Postgres 1 MB bisa terlampaui file 1 MB berisi kata unik |
| D5 | Konfigurasi FTS `simple` dipertahankan | Sesuai tiket; adil untuk campuran Indonesia/Inggris, tanpa stemming yang salah bahasa |
| D6 | Label aktor diambil dari `reqctx.Identity` (`Name`, fallback `Email`), bukan `core.auth.get-user` | Pola T03 D6: `resolve-session` sudah memuat identitas dan menolak user nonaktif di tiap request |
| D7 | `version` = hex(sha256(isi)) (64 karakter huruf kecil) | Deterministik, tidak bergantung presisi waktu, sama untuk isi identik; upload memakai hash yang sama untuk `unchanged` |
| D8 | Aksi audit baru `knowledge.file.moved` | `movefile` adalah mutasi; tidak ada aksi yang cocok di katalog |
| D9 | Path dengan segmen diawali `.` ditolak 400 `hidden` di `createfile` dan `movefile` (tujuan), dan `skipped` di upload | Konsistensi: file tersembunyi tidak pernah ada di project, sehingga upload ulang folder tidak menghasilkan perilaku berbeda |
| D10 | JSON REST memakai snake_case (`size_bytes`, `updated_at`, `file_count`, `total_files`) | Konvensi modul yang sudah ada (`management.yml`, `audit-log.yml`). Nama camelCase di deskripsi dekomposisi hanya ilustrasi |
| D11 | Backslash `\` adalah karakter biasa di path (tidak diubah jadi `/`, tidak ditolak). Tidak ada normalisasi Unicode (NFC) | Tiket: pemisah hanya `/`, peka huruf besar/kecil, tidak diperbaiki diam-diam. Browser sudah mengirim `/` untuk `webkitRelativePath` |
| D12 | `updatefile` dengan isi identik (hash baru == `version`) tetap memverifikasi baris: tidak ada -> 404, hash tersimpan != `version` -> 409, sama -> 200 no-op tanpa audit | Mencegah 200 palsu ketika orang lain sudah mengubah file |

### 3.2 Struktur module

```
backend/internal/domain/project/
  base.go                              # ubah: Register memanggil management.Register(d) lalu knowledge.Register(d)
  internal/knowledge/
    base.go                            # package knowledge: Register(d app.Deps) memanggil 13 <feature>.Init(d)
    knowledge_integration_test.go      # tag integration; AC1/AC2/AC6/AC7/AC8 lewat router + bus
    base/                              # package base: MURNI (tanpa I/O, tanpa context)
      path.go       path_test.go
      content.go    content_test.go
      file.go       file_test.go
      upload.go     upload_test.go
      query.go      query_test.go
      tree.go       tree_test.go
      slice.go      slice_test.go
      similar.go    similar_test.go
      snippet.go    snippet_test.go
      reasons.go
      limits.go     limits_test.go
      errors.go     errors_test.go
      meta.go                          # RequestMeta, Actor
    base/store/                        # package store: SQL bersama
      store.go      store_integration_test.go
    base/access/                       # package access: otorisasi via check-access
      access.go     access_test.go
    base/audit/                        # package audit: audit via bus, fail-open
      audit.go      audit_test.go
    uploadfiles/ createfile/ updatefile/ getfile/ listtree/ deletefile/ deletefolder/ movefile/ search/
    buslistfiles/ busreadfile/ bussearch/ countfiles/
      init.go handler.go [params.go] usecase.go [repository.go] *_test.go
```

Aturan per feature:
- `init.go`: `func Init(d app.Deps)`. Merakit `store.New(d.DB)` -> repository feature -> usecase (menerima interface) -> handler, lalu mendaftarkan route atau handler bus feature itu sendiri.
- Feature bus tanpa `params.go`; `handler.go` memanggil `bus.Expect[<kontrak>.Request](req)` lebih dulu (rule 6.9). Feature bus tidak memakai `base/access` (ProjectID sudah diverifikasi gateway T07 / T03 adalah pemanggil internal) dan tidak menulis audit (rule 9.3).
- Usecase menerima interface (rule 5.6): repository feature, `access.Checker`, `audit.Recorder`, `base.Limits` (nilai). Bukan tipe konkret.
- Feature DILARANG saling import (rule 4.3). Yang dipakai bersama lewat `base`, `base/store`, `base/access`, `base/audit`. Package `base` DILARANG meng-import subpackage-nya; arah: `base/store|access|audit` -> `base`.
- Repository feature membungkus `*store.Store`. Transaksi: `func (r *Repo) InTx(ctx, fn func(*Repo) error) error` memakai `s.InTx` (3.5). Usecase tidak menyentuh `sqlx`.
- Feature yang tidak butuh SQL khusus (mis. `getfile`) boleh tanpa `repository.go` dan memakai interface yang dipenuhi `*store.Store` langsung.

### 3.3 Konvensi handler, aktor, request meta, dan log

- Route: `d.API.With(middleware.RequireAuth(), [middleware.BodyLimit(n)]).<Method>("/projects/{projectID}/knowledge/...", h)` (pola penuh, T03 D4). `chi.URLParam(r, "projectID")` diteruskan apa adanya ke usecase; `base/access` menjawab 404 untuk id bukan uuid.
- `path`/`prefix`/`q`/`limit` dibaca dari `r.URL.Query().Get(...)`. Parameter wajib yang tidak ada = string kosong, lalu entity mengembalikan `path_empty`.
- Handler membangun `base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`.
- Body JSON di-decode `respond.DecodeJSON` (field tak dikenal ditolak 400). `*http.MaxBytesError` -> 413 oleh `respond`.
- Error infra dibungkus `fmt.Errorf("<feature>: <langkah>: %w", err)` -> 500 generik. Error bisnis selalu `*errs.Error` dari `base/errors.go`.
- Log: field standar `project_id`, `user_id`, `feature`, dan bila relevan `path`. DILARANG me-log isi file, body request, `Args`/`Result` audit.

### 3.4 Otorisasi (`base/access`)

Urutan wajib di setiap usecase HTTP, sebelum validasi input lain (agar non-anggota mendapat 404 meski input salah):
1. `actor, err := checker.Require(ctx, projectID)`.
2. Error dikembalikan apa adanya (401, 404, atau 500 terbungkus).
3. Lanjut ke langkah feature dengan `actor` (ID, Role, Label).

### 3.5 Konkurensi dan transaksi

- Semua tulis yang menambah file (`uploadfiles`, `createfile`, `movefile`) berjalan di satu tx: `LockProject` (advisory xact lock per project) -> cek kuota (`CountFiles` vs `Limits.MaxProjectFiles`) -> tulis. Dua upload paralel diserialkan sehingga kuota tidak terlampaui.
- `updatefile`: tanpa lock; satu `UPDATE ... WHERE project_id=$1 AND path=$2 AND content_hash=$3` atomik (optimistic concurrency, AC3).
- `deletefile`/`deletefolder`: satu statement `DELETE ... RETURNING`, tanpa lock.
- Unique violation (`23505` pada `knowledge_files_project_id_path_key`) selalu dipetakan ke 409 `path_exists` sebagai jaring pengaman.
- Panggilan bus (check-access, get-project, audit) TIDAK PERNAH di dalam tx. check-access sebelum tx; audit (dan get-project untuk labelnya) setelah commit.
- Tidak ada `FOR UPDATE` pada baris `projects` (tabel milik T03, rule 2.4); advisory lock memakai kunci teks sendiri.

### 3.6 Audit

- Tepat satu entri per mutasi sukses, dikirim setelah commit, fail-open (pola README). Gagal/ditolak -> 0 entri.
- No-op: `updatefile` isi identik -> 0 entri. Upload tetap menulis tepat satu `knowledge.upload.completed` walaupun semua `unchanged`/`rejected` (catatan bahwa upload terjadi), tetapi tidak pernah `knowledge.file.updated` per file.
- `TargetType`: `"file"` (Target = path), `"folder"` (Target = prefix), `"knowledge"` (upload, Target kosong).
- `Args`/`Result` hanya metadata (tabel 4.13). Tidak pernah isi file.

### 3.7 Batas ukuran

- JSON ber-content (`createfile`, `updatefile`): `BodyLimit(6*MaxFileBytes + 64 KiB)` (escape JSON terburuk `\u00XX` = 6 byte per byte). Setelah decode, `PrepareContent` mengukur byte UTF-8 isi (setelah buang BOM); `> MaxFileBytes` -> 413 `too_large`.
- Upload: `BodyLimit(MaxUploadBytes + 1 MiB)` (overhead header multipart). Jumlah byte mentah semua part file (termasuk yang dibuang) `> MaxUploadBytes` -> 413 `upload_too_large` untuk seluruh request, tidak ada yang ditulis.
- `HTTP_DEFAULT_TIMEOUT` (30 s) tetap berlaku; dicatat di risiko.

---

## 4. Entity dan rule (`base/`, package `base`)

Semua fungsi murni: tanpa context, tanpa I/O. Diuji unit tanpa mock (rule 13.1). Import yang diizinkan: stdlib, `lib/common/errs`, `grimoire/internal/infra/config` (hanya di `limits.go`, tipe `Config`).

### 4.1 `base/reasons.go`

```go
type Reason string // "" = valid

const (
    ReasonPathEmpty          Reason = "path_empty"
    ReasonPathAbsolute       Reason = "path_absolute"
    ReasonPathTraversal      Reason = "path_traversal"       // segmen "." atau ".."
    ReasonPathEmptySegment   Reason = "path_empty_segment"   // "a//b", "a/" (untuk file)
    ReasonPathControlChar    Reason = "path_control_char"    // NUL, unicode.IsControl
    ReasonPathTooLong        Reason = "path_too_long"        // > 512 rune
    ReasonSegmentTooLong     Reason = "segment_too_long"     // > 255 rune
    ReasonHidden             Reason = "hidden"
    ReasonUnsupportedType    Reason = "unsupported_type"
    ReasonTooLarge           Reason = "too_large"
    ReasonInvalidEncoding    Reason = "invalid_encoding"     // isi atau path bukan UTF-8 valid, atau isi mengandung NUL
    ReasonDuplicateInRequest Reason = "duplicate_in_request"
    ReasonTooManyFiles       Reason = "too_many_files"
    ReasonProjectFileLimit   Reason = "project_file_limit"
    ReasonInvalidOffset      Reason = "invalid_offset"
    // tambahan foundation T05
    ReasonInvalidVersion     Reason = "invalid_version"      // bukan 64 hex huruf kecil
    ReasonSamePath           Reason = "same_path"            // move from == to
    ReasonPrefixRequired     Reason = "prefix_required"      // deletefolder root
    ReasonInvalidLimit       Reason = "invalid_limit"        // limit bukan bilangan bulat (HTTP)
    ReasonNoFiles            Reason = "no_files"             // upload tanpa part file
    ReasonUploadTooLarge     Reason = "upload_too_large"     // total upload > MaxUploadBytes
    ReasonPathExists         Reason = "path_exists"          // 409
    ReasonVersionConflict    Reason = "version_conflict"     // 409
    ReasonInvalidProjectID   Reason = "invalid_project_id"   // bus
    ReasonNotMultipart       Reason = "not_multipart"
)

// Message mengembalikan pesan Indonesia yang memandu (dipakai HTTP dan bus, rule 9.5).
func (r Reason) Message() string
```

Pesan (`Message()`):

| Reason | Pesan |
|---|---|
| `path_empty` | `path wajib diisi, mis. docs/auth/login.md` |
| `path_absolute` | `path harus relatif (tanpa awalan /), mis. docs/auth/login.md` |
| `path_traversal` | `path tidak boleh memuat segmen . atau ..` |
| `path_empty_segment` | `path tidak boleh memuat segmen kosong (// atau / di akhir)` |
| `path_control_char` | `path tidak boleh memuat karakter kontrol` |
| `path_too_long` | `path maksimal 512 karakter` |
| `segment_too_long` | `setiap bagian path maksimal 255 karakter` |
| `hidden` | `path yang diawali titik (mis. .git/) tidak didukung` |
| `unsupported_type` | `hanya file .md, .markdown, dan .txt yang didukung` |
| `too_large` | `ukuran file melebihi batas <MaxFileBytes> byte` (usecase memformat angka; `Message()` tanpa angka) |
| `invalid_encoding` | `isi harus teks UTF-8 tanpa karakter NUL` |
| `duplicate_in_request` | `path yang sama muncul lebih dari sekali dalam satu upload` |
| `too_many_files` | `jumlah file per upload melebihi batas` |
| `project_file_limit` | `project sudah mencapai batas jumlah file` |
| `invalid_offset` | `offset di luar ukuran file` |
| `invalid_version` | `version wajib diisi dengan nilai dari getfile` |
| `same_path` | `path tujuan sama dengan path asal` |
| `prefix_required` | `path folder wajib diisi; root tidak bisa dihapus sekaligus` |
| `invalid_limit` | `limit harus bilangan bulat` |
| `no_files` | `upload tidak berisi file` |
| `upload_too_large` | `ukuran total upload melebihi batas` |
| `path_exists` | `file dengan path ini sudah ada` |
| `version_conflict` | `file sudah diubah orang lain; muat ulang sebelum menyimpan` |
| `invalid_project_id` | `ProjectID tidak valid` |
| `not_multipart` | `body harus multipart/form-data dengan field files` |

### 4.2 `base/path.go`

```go
const (
    MaxPathRunes    = 512
    MaxSegmentRunes = 255
)

// NormalizePath memvalidasi path file. Tidak memperbaiki apa pun selain tidak ada (path dipakai apa adanya).
// Urutan cek (alasan pertama yang gagal dikembalikan):
//   raw == ""                                   -> path_empty
//   !utf8.ValidString(raw)                      -> invalid_encoding
//   ada rune == 0 atau unicode.IsControl(r)     -> path_control_char
//   strings.HasPrefix(raw, "/")                 -> path_absolute
//   utf8.RuneCountInString(raw) > 512           -> path_too_long
//   split "/": segmen "" -> path_empty_segment; "." atau ".." -> path_traversal; > 255 rune -> segment_too_long
// Hasil = raw (tidak di-trim, tidak di-lowercase).
func NormalizePath(raw string) (string, Reason)

// NormalizePrefix untuk folder: "" -> ("", "") (root). Satu "/" di akhir dibuang ("docs/" -> "docs").
// Sisanya divalidasi sama seperti NormalizePath (segmen kosong di tengah tetap ditolak).
func NormalizePrefix(raw string) (string, Reason)

// StripLeadingSlash membuang tepat SATU "/" di depan (dipakai handler bus untuk agent).
func StripLeadingSlash(s string) string

func IsHidden(p string) bool    // ada segmen yang diawali "."
func Ext(p string) string       // ekstensi segmen terakhir, huruf kecil (".MD" -> ".md"); tanpa titik -> ""
func IsSupported(p string) bool // Ext in {".md", ".markdown", ".txt"}
func Base(p string) string      // segmen terakhir
func Dir(p string) string       // semua kecuali segmen terakhir; "" untuk root

// LikePrefix untuk LIKE ... ESCAPE '\': escape "\" -> "\\", "%" -> "\%", "_" -> "\_", lalu + "/%".
// prefix "" -> "%" (seluruh project). "docs" -> "docs/%" sehingga "docs2/x.md" dan "docs.md" tidak cocok (AC6).
func LikePrefix(prefix string) string
```

### 4.3 `base/content.go`

```go
// PrepareContent: buang UTF-8 BOM (EF BB BF) di depan; !utf8.Valid -> invalid_encoding;
// mengandung 0x00 -> invalid_encoding; len > maxBytes -> too_large (diukur SETELAH BOM dibuang).
// Isi kosong valid.
func PrepareContent(raw []byte, maxBytes int64) (string, Reason)

func Hash(content string) []byte    // sha256, 32 byte
func Version(hash []byte) string    // hex huruf kecil, 64 karakter
func ValidVersion(v string) bool    // tepat 64 karakter [0-9a-f]

// Title: baris pertama di 4 KiB pertama yang berbentuk ATX "# judul" (tepat satu "#", lalu spasi/tab).
// Judul di-trim, "#" penutup opsional dibuang, dipotong ke 255 rune. Tidak ada -> "".
// Baris di dalam blok kode ``` diabaikan. Front matter tidak ditangani khusus.
func Title(content string) string
```

### 4.4 `base/file.go`

```go
type File struct {
    ID, ProjectID, Path string
    Title, Content      string
    Hash                []byte
    SizeBytes           int
    CreatedAt           time.Time
    CreatedBy           string
    CreatedByLabel      string
    UpdatedAt           time.Time
    UpdatedBy           string
    UpdatedByLabel      string
}

// NewFile menyiapkan file baru dari path tervalidasi + isi siap pakai: Title, Hash, SizeBytes = len(content),
// Created*/Updated* dari actor. Waktu diisi DB (now()).
func NewFile(projectID, path, content string, actor Actor) File
// WithContent mengganti isi (Title, Hash, SizeBytes, Updated* dari actor). Dipakai updatefile dan upload.
func (f File) WithContent(content string, actor Actor) File
func (f File) Version() string // Version(f.Hash)

type Meta struct {
    Path      string
    SizeBytes int
    UpdatedAt time.Time
}

type UserRef struct {
    ID    string `json:"id"`
    Label string `json:"label"`
}

type FileDTO struct {
    Path      string    `json:"path"`
    Title     string    `json:"title"`
    Content   string    `json:"content"`
    SizeBytes int       `json:"size_bytes"`
    Version   string    `json:"version"`
    CreatedAt time.Time `json:"created_at"`
    CreatedBy UserRef   `json:"created_by"`
    UpdatedAt time.Time `json:"updated_at"`
    UpdatedBy UserRef   `json:"updated_by"`
}

// ConflictCurrent adalah isi details.current pada 409 version_conflict.
type ConflictCurrent struct {
    Content        string    `json:"content"`
    Version        string    `json:"version"`
    UpdatedAt      time.Time `json:"updated_at"`
    UpdatedByLabel string    `json:"updated_by_label"`
}

func (f File) ToDTO() FileDTO
func (f File) ToConflict() ConflictCurrent
```

Waktu selalu UTC (store memanggil `.UTC()`).

### 4.5 `base/meta.go`

```go
type Actor struct{ ID, Role, Label string }
// ActorFrom: ok=false atau UserID kosong -> false. Label = Name (trim) bila tidak kosong, selain itu Email.
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool)

type RequestMeta struct{ IP, UserAgent string }
```

(`reqctx` boleh di-import `base` karena hanya paket tipe, pola T03 4.6.)

### 4.6 `base/upload.go`

```go
type Status string
const (
    StatusCreated   Status = "created"
    StatusUpdated   Status = "updated"
    StatusUnchanged Status = "unchanged"
    StatusSkipped   Status = "skipped"
    StatusRejected  Status = "rejected"
)

type Incoming struct {
    Index   int    // urutan di request (0-based)
    Path    string // sudah tervalidasi
    Content string
    Hash    []byte
}

type Decision struct {
    Index  int
    Path   string // path tervalidasi, atau nama mentah (dipotong 1024 rune) bila path ditolak
    Status Status
    Reason Reason // "" untuk created/updated/unchanged
}

// ClassifyPart menerapkan rule per file, berurutan:
//   NormalizePath gagal -> rejected + reason
//   IsHidden            -> skipped hidden
//   !IsSupported        -> rejected unsupported_type
//   tooLarge            -> rejected too_large
//   PrepareContent gagal-> rejected + reason
//   lolos -> ok=true, Incoming terisi (Hash dihitung)
func ClassifyPart(index int, rawName string, raw []byte, tooLarge bool, maxBytes int64) (Incoming, Decision, bool)

// PlanUpload untuk file yang lolos ClassifyPart, diproses urut Index:
//   path sudah muncul sebelumnya di request -> rejected duplicate_in_request (kemunculan pertama yang menang)
//   path ada di existing, hash sama         -> unchanged
//   path ada di existing, hash beda         -> updated
//   path baru, currentCount + created >= projectLimit -> rejected project_file_limit
//   path baru                               -> created
func PlanUpload(in []Incoming, existing map[string][]byte, currentCount, projectLimit int) []Decision

type Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int } // json: created, updated, ...
func Summarize(ds []Decision) Summary
// ChangedPaths: path created+updated urut Index, maks n; truncated = ada yang terpotong.
func ChangedPaths(ds []Decision, n int) (paths []string, truncated bool)
// SortByIndex mengurutkan keputusan gabungan (pra-tolak + rencana) sesuai urutan request.
func SortByIndex(ds []Decision)

type UploadItemDTO struct {
    Path   string `json:"path"`
    Status Status `json:"status"`
    Reason Reason `json:"reason,omitempty"`
}
type UploadResultDTO struct {
    Summary Summary         `json:"summary"`
    Items   []UploadItemDTO `json:"items"`
}
func NewUploadResult(ds []Decision) UploadResultDTO
```

### 4.7 `base/query.go`

```go
const (
    MaxQueryRunes = 200
    MaxQueryTerms = 16
)

// ParseQuery membersihkan kueri user menjadi masukan aman untuk websearch_to_tsquery('simple', $q):
//   1. buang karakter kontrol, trim, potong ke 200 rune.
//   2. scan: teks di antara tanda kutip " menjadi frasa (kutip tanpa penutup = frasa sampai akhir);
//      di luar kutip, kata = run huruf/angka/_ (unicode.IsLetter, IsDigit, '_'); karakter lain pemisah.
//      Isi frasa juga dipecah menjadi kata lalu digabung satu spasi; frasa tanpa kata dibuang.
//   3. maks 16 term (sisanya dibuang).
//   4. output: tiap term dibungkus kutip ganda, digabung spasi: `"login" "reset password"`.
//      Karena semua term berkutip, `or`, `-`, `&`, `:`, `!` tidak pernah menjadi operator websearch.
// Tanpa term -> ("", false); caller mengembalikan hasil kosong tanpa query DB.
func ParseQuery(q string) (string, bool)
```

### 4.8 `base/tree.go`

```go
type FolderNode struct {
    Name      string `json:"name"`
    Path      string `json:"path"`
    FileCount int    `json:"file_count"` // rekursif
}
type FileNode struct {
    Name      string    `json:"name"`
    Path      string    `json:"path"`
    SizeBytes int       `json:"size_bytes"`
    UpdatedAt time.Time `json:"updated_at"`
}
type Level struct {
    Prefix     string       `json:"prefix"`
    Folders    []FolderNode `json:"folders"` // tidak pernah nil (JSON [])
    Files      []FileNode   `json:"files"`
    TotalFiles int          `json:"total_files"` // file rekursif di bawah prefix
}

// BuildLevel: metas = semua file di bawah prefix (rekursif, hasil ListMeta). Untuk tiap meta, sisa path
// setelah "prefix/" (atau seluruh path bila root): tanpa "/" -> file langsung; ada "/" -> folder = segmen pertama.
// Folder dan file diurutkan berdasarkan Name (byte order, stabil), folder lebih dulu di JSON (dua array terpisah).
// Meta yang tidak berawalan prefix diabaikan (jaring pengaman).
func BuildLevel(prefix string, metas []Meta) Level
```

### 4.9 `base/slice.go`

```go
const DefaultReadLimit = 65536

// SliceUTF8 memotong content berdasarkan BYTE. offset < 0 atau offset > len(content) -> invalid_offset.
// limit <= 0 diperlakukan DefaultReadLimit (caller sudah meng-clamp). start = offset maju ke awal rune
// berikutnya bila di tengah rune; end = min(start+limit, len) mundur ke batas rune.
// Bila end == start padahal start < len (limit < panjang satu rune), end maju ke akhir rune itu (selalu ada kemajuan).
// truncated = end < len(content). offset == len -> part "", start=end=len, truncated=false.
func SliceUTF8(content string, offset, limit int) (part string, start, end int, truncated bool, reason Reason)
```

### 4.10 `base/similar.go`

```go
// SimilarPaths: maks n path dari all yang mirip target, tanpa target sendiri:
//   1. path dengan Base sama (case-insensitive) lebih dulu, urut path;
//   2. lalu Levenshtein(lower(target), lower(p)) <= max(3, runeLen(target)/3), urut jarak lalu path.
// Levenshtein berbasis rune. Deterministik.
func SimilarPaths(target string, all []string, n int) []string
```

### 4.11 `base/snippet.go`

```go
const (
    HighlightStart = "\x01"
    HighlightStop  = "\x02"
)

type Segment struct {
    Text      string `json:"text"`
    Highlight bool   `json:"highlight"`
}

// ParseHeadline memecah hasil ts_headline bermarker \x01..\x02 menjadi segmen; marker tanpa pasangan diabaikan;
// segmen kosong dibuang; segmen bersebelahan dengan status sama digabung.
func ParseHeadline(s string) []Segment
// ToMarkdownBold mengganti \x01term\x02 menjadi **term** (untuk bus/agent); marker yatim dibuang.
func ToMarkdownBold(s string) string

type SearchHit struct {
    Path, Title string
    Score       float64
    Headline    string // mentah dengan marker
}

type SearchItemDTO struct {
    Path    string    `json:"path"`
    Title   string    `json:"title"`
    Score   float64   `json:"score"`
    Snippet []Segment `json:"snippet"`
}
type SearchResultDTO struct {
    Items []SearchItemDTO `json:"items"` // tidak pernah nil
    Total int           `json:"total"`
}

const (
    SearchDefaultLimit = 20
    SearchMaxLimit     = 50
)
// ClampLimit: n <= 0 -> def; n > max -> max.
func ClampLimit(n, def, max int) int
```

### 4.12 `base/limits.go`

```go
type Limits struct {
    MaxFileBytes    int64
    MaxUploadBytes  int64
    MaxUploadFiles  int
    MaxProjectFiles int
    MaxReadBytes    int // = Config.MCPMaxResponseBytes (clamp read-file)
}
func LimitsFrom(c config.Config) Limits
func (l Limits) JSONBodyLimit() int64   // 6*MaxFileBytes + 64<<10
func (l Limits) UploadBodyLimit() int64 // MaxUploadBytes + 1<<20
```

### 4.13 `base/errors.go`

```go
var (
    ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
    ErrFileNotFound    = errs.New(errs.CodeNotFound, "file tidak ditemukan")
    ErrFolderNotFound  = errs.New(errs.CodeNotFound, "folder tidak ditemukan atau kosong")
    ErrUnauthenticated = errs.New(errs.CodeUnauthorized, "perlu login")
    ErrPathExists      = errs.WithDetails(errs.CodeConflict, ReasonPathExists.Message(),
        map[string]any{"reason": ReasonPathExists})
)

// Invalid -> 400 INVALID_ARGUMENT, details {"reason": r}, message r.Message(). Membungkus errs.ErrInvalid
// (errors.Is(err, errs.ErrInvalid) == true, dipakai bus consumer T08).
func Invalid(r Reason) *errs.Error
// InvalidMsg sama dengan Invalid tetapi pesan khusus (dipakai bus untuk menyertakan path yang ditolak).
func InvalidMsg(r Reason, msg string) *errs.Error
// TooLarge -> 413 PAYLOAD_TOO_LARGE, details {"reason": r, "max_bytes": max}.
func TooLarge(r Reason, max int64) *errs.Error
// VersionConflict -> 409 CONFLICT, details {"reason": "version_conflict", "current": cur}.
func VersionConflict(cur ConflictCurrent) *errs.Error
```

Sentinel pointer dibagi; usecase DILARANG memodifikasinya. Test memakai `errors.Is` atau `Code` + `details.reason`.

Isi audit (`Args`/`Result`) per aksi:

| Aksi | TargetType / Target | Args | Result |
|---|---|---|---|
| `knowledge.upload.completed` | `knowledge` / `""` | `{"files": <jumlah part>}` | `{"created","updated","unchanged","skipped","rejected","bytes": <total byte ditulis>,"paths": [<=20 created/updated],"paths_truncated": bool}` |
| `knowledge.file.created` | `file` / path | `{}` | `{"bytes": n}` |
| `knowledge.file.updated` | `file` / path | `{}` | `{"bytes_before": a, "bytes": b}` |
| `knowledge.file.deleted` | `file` / path | `{}` | `{"bytes": n}` |
| `knowledge.folder.deleted` | `folder` / prefix | `{}` | `{"count": n}` |
| `knowledge.file.moved` | `file` / path tujuan | `{"from": a, "to": b}` | `{"bytes": n}` |

---

## 5. Kontrak bus dan konstanta endpoint

### 5.1 `backend/lib/constants/bus.go` (ditambah)

```go
// Provider: T05 project/knowledge. Consumer: T08 (lewat gateway T07, ProjectID dari key terverifikasi).
// Kontrak: lib/contracts/project/knowledge/listfiles. Tanpa audit di provider (gateway yang mengaudit).
const BUS_PROJECT_KNOWLEDGE_LIST_FILES = "project.knowledge.list-files"

// Provider: T05. Consumer: T08. Kontrak: lib/contracts/project/knowledge/readfile.
// Offset/Limit dalam BYTE, dipotong pada batas karakter UTF-8. Tidak ketemu -> Found=false + Similar.
const BUS_PROJECT_KNOWLEDGE_READ_FILE = "project.knowledge.read-file"

// Provider: T05. Consumer: T08. Kontrak: lib/contracts/project/knowledge/search. Snippet bersorot **term**.
const BUS_PROJECT_KNOWLEDGE_SEARCH = "project.knowledge.search"
```

`BUS_PROJECT_KNOWLEDGE_COUNT_FILES` sudah dibuat T03 (5.1 plan T03); perbarui komentarnya: "Provider: T05 (handler `countfiles`)". JANGAN diduplikasi.

### 5.2 `lib/contracts/project/knowledge/listfiles/listfiles.go` (baru)

```go
// Package listfiles adalah kontrak bus project.knowledge.list-files (provider T05, consumer T08).
//
// Semua hasil dibatasi pada ProjectID (rule 6.10). Path tanpa awalan "/".
// Error: tipe request salah -> bus.ErrBadRequestType; ProjectID bukan uuid atau Prefix tidak valid ->
// error membungkus errs.ErrInvalid dengan pesan yang memandu. Prefix tanpa file -> Items kosong, err nil.
package listfiles

import "time"

const (
    DefaultLimit = 100
    MaxLimit     = 500
)

type Request struct {
    ProjectID string
    Prefix    string // "" = root; satu "/" di depan/belakang ditoleransi
    Recursive bool   // true: semua file di bawah Prefix; false: hanya file langsung + Folders
    Limit     int    // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
    Offset    int    // >= 0 (negatif -> errs.ErrInvalid)
}

type Item struct {
    Path      string
    SizeBytes int
    UpdatedAt time.Time
}

type Response struct {
    Items     []Item
    Folders   []string // path subfolder langsung bila !Recursive (urut), nil bila Recursive
    Total     int      // jumlah file yang cocok (bukan hanya halaman ini)
    Truncated bool     // Offset + len(Items) < Total
}
```

Urutan `Items`: `path` ascending (byte order, `ORDER BY path COLLATE "C"`).

### 5.3 `lib/contracts/project/knowledge/readfile/readfile.go` (baru)

```go
// Package readfile adalah kontrak bus project.knowledge.read-file (provider T05, consumer T08).
//
// Offset dan Limit dalam BYTE. Potongan selalu pada batas karakter UTF-8: Offset di tengah karakter maju ke
// karakter berikutnya; akhir potongan mundur ke batas karakter. Membaca lanjutan: Offset = NextOffset.
// Tidak ketemu -> Found=false, Similar berisi maks 5 path mirip, err nil.
// Error: tipe salah -> bus.ErrBadRequestType; ProjectID/Path tidak valid atau Offset di luar file ->
// errs.ErrInvalid dengan pesan yang memandu.
package readfile

const DefaultLimit = 65536 // Limit 0 -> DefaultLimit; maks Config.MCPMaxResponseBytes

type Request struct {
    ProjectID string
    Path      string // satu "/" di depan ditoleransi
    Offset    int
    Limit     int
}

type Response struct {
    Found      bool
    Path       string
    Content    string // potongan
    SizeBytes  int    // len(Content) dalam byte
    TotalBytes int    // ukuran file utuh
    NextOffset int    // offset byte setelah potongan
    Truncated  bool   // NextOffset < TotalBytes
    Similar    []string
}
```

### 5.4 `lib/contracts/project/knowledge/search/search.go` (baru)

```go
// Package search adalah kontrak bus project.knowledge.search (provider T05, consumer T08).
//
// Query dibersihkan provider (tanpa sintaks tsquery mentah); frasa dalam tanda kutip didukung.
// Query tanpa kata -> Items kosong, Total 0, err nil. Snippet memakai **term** sebagai sorotan.
// Error: tipe salah -> bus.ErrBadRequestType; ProjectID/Prefix tidak valid -> errs.ErrInvalid.
package search

const (
    DefaultLimit = 20
    MaxLimit     = 50
)

type Request struct {
    ProjectID string
    Query     string
    Prefix    string // opsional; satu "/" di depan ditoleransi
    Limit     int    // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
}

type Item struct {
    Path    string
    Score   float64
    Snippet string
}

type Response struct {
    Items []Item
    Total int // jumlah dokumen cocok (sebelum limit)
}
```

### 5.5 Provider `count-files` (kontrak T03 `contentcount`)

- `bus.Expect[contentcount.Request]`. `len(ProjectIDs) > contentcount.MaxProjectIDs` -> error membungkus `errs.ErrInvalid`.
- id bukan uuid diabaikan (tetap muncul di `Counts` dengan 0). Input kosong -> `Counts` map kosong tanpa query.
- Satu query: `SELECT project_id::text, count(*) FROM knowledge_files WHERE project_id = ANY($1::uuid[]) GROUP BY project_id`.
- Semua id yang diminta ada di `Counts` (0 bila tanpa file).

### 5.6 Aturan bersama semua provider bus T05

1. `bus.Expect[<kontrak>.Request](req)` pertama kali; error dikembalikan apa adanya (membungkus `ErrBadRequestType`).
2. `uuid.Parse(ProjectID)` gagal -> `base.Invalid(base.ReasonInvalidProjectID)`.
3. `Path`/`Prefix`: `base.StripLeadingSlash` (tepat satu), lalu `NormalizePath`/`NormalizePrefix`; gagal -> `base.InvalidMsg(reason, fmt.Sprintf("path %q tidak valid: %s", in, reason.Message()))`.
4. Semua query di `base/store` memakai `project_id = $1`.
5. Tidak ketemu -> `Found=false`, err nil (rule 6.7).
6. Tanpa audit, tanpa check-access.
7. Tidak me-log isi file.

### 5.7 Subpackage I/O bersama

`base/store/store.go` (package `store`):

```go
type Store struct {
    db *db.DB      // nil di dalam tx
    q  db.Querier
}
func New(d *db.DB) *Store
// InTx menjalankan fn dengan Store yang terikat tx (d.BeginTx). Error fn dikembalikan apa adanya.
func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error
func (s *Store) Q() db.Querier // untuk repository feature (SQL khusus feature)

// projectID bukan uuid -> nilai nol / not found tanpa query. Berlaku untuk semua fungsi di bawah.
func (s *Store) LockProject(ctx context.Context, projectID string) error
//   SELECT pg_advisory_xact_lock(hashtextextended('knowledge:' || $1, 0)); WAJIB di dalam tx.
func (s *Store) CountFiles(ctx context.Context, projectID string) (int, error)
func (s *Store) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
func (s *Store) ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error)
//   SELECT path, size_bytes, updated_at FROM knowledge_files
//   WHERE project_id = $1 AND path LIKE $2 ESCAPE '\' ORDER BY path COLLATE "C"
//   (tanpa kolom content; root memakai likePrefix "%")
func (s *Store) ListPaths(ctx context.Context, projectID string) ([]string, error)
func (s *Store) Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error)
// IsUniqueViolation(err) bool: pgconn.PgError Code 23505.
func IsUniqueViolation(err error) bool
```

SQL `Search` (satu statement, parameter saja):

```sql
WITH q AS (SELECT websearch_to_tsquery('simple', $2) AS q),
hits AS (
  SELECT f.id, f.path, f.title,
         ts_rank_cd(f.search, q.q, 1) AS score,
         count(*) OVER () AS total
  FROM knowledge_files f, q
  WHERE f.project_id = $1 AND f.search @@ q.q AND f.path LIKE $3 ESCAPE '\'
  ORDER BY score DESC, f.path COLLATE "C"
  LIMIT $4
)
SELECT h.path, h.title, h.score, h.total,
       ts_headline('simple',
         replace(replace(left(f.content, 200000), E'\x01', ''), E'\x02', ''),
         q.q, $5) AS headline
FROM hits h JOIN knowledge_files f ON f.id = h.id, q
ORDER BY h.score DESC, h.path COLLATE "C";
```

`$5` = konstanta Go `"StartSel=\x01, StopSel=\x02, MaxFragments=2, MaxWords=30, MinWords=10, FragmentDelimiter=\" … \""`. `total` = 0 bila tidak ada baris. ts_headline hanya dijalankan untuk N teratas.

Row struct privat bertag `db`; entity `base` tanpa tag `db`. `content_hash` dipindai sebagai `[]byte`. Waktu di-`.UTC()`.

`base/access/access.go` (package `access`):

```go
type Checker interface {
    Require(ctx context.Context, projectID string) (base.Actor, error)
}
type BusChecker struct{ b *bus.Registry }
func New(b *bus.Registry) *BusChecker
// Require:
//   id, ok := reqctx.IdentityFrom(ctx); actor, ok := base.ActorFrom(id, ok); !ok -> base.ErrUnauthenticated (401)
//   res, err := bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
//       checkaccess.Request{ProjectID: projectID, UserID: actor.ID, UserRole: actor.Role})
//   err -> fmt.Errorf("access: check-access: %w", err) (500, fail-closed; termasuk ErrNotRegistered)
//   !res.ProjectExists || res.Level == checkaccess.LevelNone -> base.ErrProjectNotFound (404)
//   level member/owner/admin -> actor, nil
```

`base/audit/audit.go` (package `audit`):

```go
type Recorder interface{ Record(ctx context.Context, ev Event) }
type Event struct {
    Action     string // record.ActionKnowledge*
    ProjectID  string
    Actor      base.Actor
    TargetType string // "file" | "folder" | "knowledge"
    Target     string
    Args       map[string]any
    Result     map[string]any
    Meta       base.RequestMeta
}
type BusRecorder struct{ b *bus.Registry; log *slog.Logger }
func New(b *bus.Registry, log *slog.Logger) *BusRecorder
// Record (dipanggil SETELAH commit, tidak pernah mengembalikan error):
//   label: bus.CallTyped[getproject.Response](... GET_PROJECT, getproject.Request{ProjectID}); error atau
//          !Found -> label "" (log warn "project label gagal", fail-soft)
//   req := record.Request{ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
//          ProjectID, ProjectLabel: label, Action, TargetType, Target, Args, Result,
//          Status: record.StatusOK, IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx)}
//   _, err := bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
//   err -> log.ErrorContext(ctx, "audit write failed", "action", ev.Action, "project_id", ev.ProjectID, "err", err)
```

### 5.8 Katalog README (diperbarui di langkah F)

Baris yang ada diperbarui (kolom Mode/Provider/Consumer tetap), tambah catatan di bawah tabel:

```
| `project.knowledge.list-files` | Call | T05 | T08 |
| `project.knowledge.read-file` | Call | T05 | T08 |
| `project.knowledge.search` | Call | T05 | T08 |
| `project.knowledge.count-files` | Call | T05 | T03 |   (baris dari T03; pastikan ada)
```

Catatan: "Kontrak T05 di `lib/contracts/project/knowledge/{listfiles,readfile,search}`; `count-files` memakai `lib/contracts/project/management/contentcount`. `list-files` mengembalikan juga `Folders` (subfolder langsung bila `!Recursive`). `read-file`: `Offset/Limit` dalam byte, dipotong pada batas UTF-8. Provider tidak menulis audit (gateway T07)." Baris `core.auth.get-user`: hapus T05 dari kolom Consumer (D6). Katalog aksi audit "Konten": tambah `knowledge.file.moved`.

---

## 6. Migration

Satu file: `backend/migrations/project/<VERSION>_create_knowledge_files.sql`

`<VERSION>` bukan nilai tetap: di-generate saat implementasi dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`) (format `YYYYMMDDHHMMSSmmm`, 17 digit sesuai validasi runner). Karena dibuat saat implement, otomatis lebih besar dari migrasi T03 (`create_projects`). Sebelum commit, pastikan tidak ada versi kembar di `backend/migrations/**`; bila kembar, generate ulang.

```sql
-- T05 project/knowledge. Forward-only. gen_random_uuid() bawaan Postgres >= 13.
CREATE TABLE knowledge_files (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path             text        NOT NULL,
    title            text        NOT NULL DEFAULT '',
    content          text        NOT NULL,
    content_hash     bytea       NOT NULL,
    size_bytes       int         NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,   -- tanpa FK ke users (rule 8.5)
    created_by_label text        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    updated_by_label text        NOT NULL,
    search           tsvector    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', translate(path, '/._-', '    ')), 'A') ||
        setweight(to_tsvector('simple', title), 'B') ||
        setweight(to_tsvector('simple', left(content, 200000)), 'D')
    ) STORED,
    CONSTRAINT knowledge_files_path_len CHECK (char_length(path) BETWEEN 1 AND 512),
    CONSTRAINT knowledge_files_hash_len CHECK (octet_length(content_hash) = 32),
    CONSTRAINT knowledge_files_size_nonneg CHECK (size_bytes >= 0),
    CONSTRAINT knowledge_files_project_id_path_key UNIQUE (project_id, path)
);

CREATE INDEX knowledge_files_search_gin ON knowledge_files USING gin (search);
CREATE INDEX knowledge_files_project_path_prefix ON knowledge_files (project_id, path text_pattern_ops);
```

Catatan:
- Tanpa `CONCURRENTLY` (runner T00 membungkus tiap file dalam transaksi).
- FK satu-satunya ke `projects(id) ON DELETE CASCADE` (rule 8.5; test pg_constraint T03 AC6 mengecek cascade).
- `UNIQUE (project_id, path)` melayani `FindByPath` dan equality; `text_pattern_ops` melayani `LIKE 'prefix/%'` (indeks btree biasa dengan collation non-C tidak dipakai LIKE). Ukuran entri btree maks ~2 KB (512 rune x 4 byte) < batas 2704 byte.
- `search` mengindeks path dengan `/ . _ -` diganti spasi agar `login.md` dan `docs/auth/login.md` cocok dengan kueri `login`.
- GIN pada tsvector besar: untuk 5000 file x 1 MB isi terburuk, storage dominan dari `content`; dapat diterima pada skala desain.

---

## 7. Config

- `backend/internal/infra/config/config.go`: field `MaxProjectFiles int \`env:"MAX_PROJECT_FILES"\``; di loader: `c.MaxProjectFiles, _ = l.intv("MAX_PROJECT_FILES", 5000, 1)` (di sebelah `MAX_UPLOAD_FILES`).
- `config_test.go`: default 5000; `MAX_PROJECT_FILES=0` -> error "minimal 1"; `abc` -> "bukan bilangan bulat"; nilai valid terbaca.
- `docs/configuration.md`: baris `| MAX_PROJECT_FILES | int | 5000 | tidak | >= 1 | Jumlah file knowledge maksimum per project. |` di bawah `MAX_UPLOAD_FILES`.
- Bila ada `.env.example`/`docker-compose.dev.yml` yang mendaftar env, tambahkan juga (opsional, default cukup).

---

## 8. Plan (langkah berurutan)

```
F  foundation (OpenAPI, migrasi, config, konstanta, kontrak, aksi audit, base/ + subpackage + test, dokumen)
 ├── gelombang A (paralel): U1 createfile, U2 getfile, U3 listtree, U4 countfiles
 ├── gelombang B (paralel): U5 uploadfiles, U6 updatefile, U7 deletefile, U8 deletefolder, U9 movefile
 └── gelombang C (paralel): U10 search, U11 buslistfiles, U12 busreadfile, U13 bussearch
W  wiring final: knowledge/base.go (13 Init), project/base.go, test integrasi, TestRoutesRegistered, EXPLAIN, benchmark
```

Feature hanya bergantung pada `base*`, jadi secara kode semua gelombang bisa paralel setelah F. Gelombang A dulu karena `createfile`/`getfile` memberi data uji dan `countfiles` membuka hitungan di T03.

### F — foundation module

1. **OpenAPI dulu (rule 11.6)**: buat `api/modules/project/knowledge.yml` (bagian 14) dengan 6 path item dan schema; tambah `$ref` di `api/root.yml` (paths dan components); `make api-bundle`; `make api-check` lulus; commit `api/openapi.yml`.
2. **Migrasi** (generate `<VERSION>` dulu dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`)): `backend/migrations/project/<VERSION>_create_knowledge_files.sql` (bagian 6). Verifikasi `make migrate-up` lokal atau test `db.TestDB`.
3. **Config**: bagian 7.
4. **Konstanta**: 3 konstanta di `backend/lib/constants/bus.go` (5.1) + perbarui komentar `COUNT_FILES`. Bila kontrak/konstanta T03 belum ada, tambahkan persis plan T03 5.1-5.4.
5. **Kontrak** (stdlib saja, rule 4.5): `backend/lib/contracts/project/knowledge/listfiles/listfiles.go`, `readfile/readfile.go`, `search/search.go` (5.2-5.4) + `*_test.go` kecil yang memastikan konstanta limit (DefaultLimit/MaxLimit) sesuai.
6. **Aksi audit**: `backend/lib/contracts/core/audit/record/actions.go` tambah `ActionKnowledgeFileMoved = "knowledge.file.moved"` di grup konten dan di `Actions()`; `actions_test.go`: tambah ke `catalog`, naikkan hitungan (33 -> 34; bila T04/T06 merge lebih dulu, sesuaikan ke jumlah aktual). `frontend/src/domain/core/audit-log/listentries/types.ts` grup `content`: tambah `'knowledge.file.moved'` (komentar di file itu mewajibkan sinkron).
7. **`base/`**: semua file di 4.1-4.13 + test unit (bagian 11). `go test ./internal/domain/project/internal/knowledge/base/...` lulus.
8. **`base/store`**, **`base/access`**, **`base/audit`** (5.7) + test (`store` integrasi tag `integration`; `access`/`audit` dengan registry bus nyata + handler palsu).
9. **Module skeleton**: `knowledge/base.go` dengan `func Register(d app.Deps) {}` (diisi per usecase/W). `project/base.go`: `func Register(d app.Deps) { management.Register(d); knowledge.Register(d) }`.
10. **Dokumen**: README (5.8, katalog aksi, status T05 "Plan siap"), tiket T05 (tabel Features: ganti `busreader` dengan 4 feature; baris "Dikonsumsi" tanpa `core.auth.get-user` + alasan D6; "Disediakan" tambah `count-files`; catatan D2 batas env), `docs/configuration.md`.
11. `go build ./... && go vet ./... && go test ./...` dan `go test -tags integration ./internal/domain/project/internal/knowledge/...` lulus; `internal/archtest` lulus.

### U1-U13

Tiap usecase mengikuti plan `usecase-<slug>.md`: buat folder feature, `init.go` mendaftarkan route/handler, tambah satu baris `<feature>.Init(d)` di `knowledge/base.go`, test unit usecase (repository/bus palsu), test integrasi repository (bila ada SQL khusus), test handler (httptest + router chi).

### W — wiring final

1. `knowledge/base.go` memanggil 13 `Init` (urutan bebas; registrasi ganda = panic).
2. `knowledge_integration_test.go` (tag `integration`): router `/api` asli T00 + bus dengan handler T03 asli (atau palsu `check-access`/`get-project` bila test T03 terlalu berat) + `record` palsu yang menghitung entri. Skenario bagian 11 "Integrasi".
3. `TestRoutesRegistered`: `chi.Walk` memastikan 9 route dengan method yang benar; `bus` memastikan 4 endpoint terdaftar.
4. EXPLAIN (rule 8.8) di test integrasi: `ListMeta` memakai `knowledge_files_project_path_prefix`; `Search` memakai `knowledge_files_search_gin` (dengan `SET enable_seqscan = off` untuk tabel kecil tidak cukup sebagai bukti; isi 5000 baris lalu `ANALYZE`, EXPLAIN tanpa memaksa).
5. Benchmark/integrasi AC9: 5000 file, 50 kueri campuran, p95 <= 200 ms.
6. Hasil EXPLAIN dan angka p95 dicatat di deskripsi PR.

---

## 9. Changes (per file/folder)

| File/folder | Perubahan |
|---|---|
| `api/modules/project/knowledge.yml` | baru: 6 path item, 9 operasi, schema `KnowledgeFile`, `KnowledgeTree`, `KnowledgeUploadResult`, `KnowledgeSearchResult`, `KnowledgeConflictDetails` |
| `api/root.yml` | tambah 6 `$ref` path + schema |
| `api/openapi.yml` | regenerate (`make api-bundle`) |
| `backend/migrations/project/<VERSION>_create_knowledge_files.sql` | baru (bagian 6) |
| `backend/internal/infra/config/config.go`, `config_test.go` | `MaxProjectFiles` (bagian 7) |
| `docs/configuration.md` | baris `MAX_PROJECT_FILES` |
| `backend/lib/constants/bus.go` | 3 konstanta + komentar COUNT_FILES |
| `backend/lib/contracts/project/knowledge/{listfiles,readfile,search}/*.go` | baru (5.2-5.4) |
| `backend/lib/contracts/core/audit/record/actions.go`, `actions_test.go` | `ActionKnowledgeFileMoved` |
| `frontend/src/domain/core/audit-log/listentries/types.ts` | tambah `knowledge.file.moved` (sinkron katalog) |
| `backend/internal/domain/project/base.go` | panggil `knowledge.Register(d)` |
| `backend/internal/domain/project/internal/knowledge/base.go` | baru: Register (13 Init) |
| `.../knowledge/base/*.go` | baru (bagian 4) + test |
| `.../knowledge/base/store/store.go` | baru (5.7) + integration test |
| `.../knowledge/base/access/access.go` | baru (5.7) + test |
| `.../knowledge/base/audit/audit.go` | baru (5.7) + test |
| `.../knowledge/{uploadfiles,createfile,updatefile,getfile,listtree,deletefile,deletefolder,movefile,search}/` | baru (plan usecase) |
| `.../knowledge/{buslistfiles,busreadfile,bussearch,countfiles}/` | baru (plan usecase) |
| `.../knowledge/knowledge_integration_test.go` | baru (W) |
| `docs/tickets/README.md` | katalog bus (5.8), katalog aksi, status T05 |
| `docs/tickets/T05-project-knowledge.md` | Features (D1), Dikonsumsi (D6), batas (D2), `title` (D4), aksi `knowledge.file.moved` (D8) |

---

## 10. Konsumsi per usecase (ringkas, acuan plan usecase)

| Usecase | base | store | access | audit | SQL khusus feature (repository.go) |
|---|---|---|---|---|---|
| `uploadfiles` | ClassifyPart, PlanUpload, Summarize, ChangedPaths, NewUploadResult, Limits | InTx, LockProject, CountFiles | ya | upload.completed | select hash `path = ANY($2)`; batch INSERT/UPDATE via `unnest` (chunk 100 baris; hash dikirim hex `text[]` lalu `decode(h,'hex')`) |
| `createfile` | NormalizePath, IsHidden, IsSupported, PrepareContent, NewFile | InTx, LockProject, CountFiles, IsUniqueViolation | ya | file.created | INSERT ... RETURNING |
| `updatefile` | NormalizePath, PrepareContent, ValidVersion, Hash, WithContent, ToConflict | FindByPath | ya | file.updated | UPDATE ... WHERE content_hash=$old RETURNING (juga mengembalikan size lama via CTE) |
| `getfile` | NormalizePath, ToDTO | FindByPath | ya | - | - |
| `listtree` | NormalizePrefix, LikePrefix, BuildLevel | ListMeta | ya | - | - |
| `deletefile` | NormalizePath | - | ya | file.deleted | DELETE ... RETURNING size_bytes |
| `deletefolder` | NormalizePrefix, LikePrefix | - | ya | folder.deleted | DELETE ... LIKE ... ESCAPE, count |
| `movefile` | NormalizePath, IsHidden, IsSupported | InTx, LockProject, FindByPath, IsUniqueViolation | ya | file.moved | UPDATE path ... RETURNING |
| `search` | ParseQuery, NormalizePrefix, LikePrefix, ClampLimit, ParseHeadline | Search | ya | - | - |
| `buslistfiles` | StripLeadingSlash, NormalizePrefix, LikePrefix, BuildLevel | ListMeta | - | - | recursive: SELECT paged + COUNT |
| `busreadfile` | StripLeadingSlash, NormalizePath, SliceUTF8, SimilarPaths | FindByPath, ListPaths | - | - | - |
| `bussearch` | StripLeadingSlash, ParseQuery, NormalizePrefix, LikePrefix, ClampLimit, ToMarkdownBold | Search | - | - | - |
| `countfiles` | - | - | - | - | SELECT ... GROUP BY |

Catatan khusus `uploadfiles` (mengikat plan usecasenya):
- `r.MultipartReader()` (bukan `ParseMultipartForm`); bukan multipart -> 400 `not_multipart`.
- Hanya part dengan form name `files` dan parameter `filename` yang diproses; part lain dikuras dan diabaikan. Tanpa part file -> 400 `no_files`.
- Nama relatif dari parameter `filename` MENTAH: `mime.ParseMediaType(part.Header.Get("Content-Disposition"))` -> `params["filename"]`. DILARANG `Part.FileName()` (memanggil `filepath.Base`, membuang folder). Tanpa percent-decoding.
- Tiap part dibaca `io.LimitReader(part, MaxFileBytes+3+1)` (BOM 3 byte); lebih -> `tooLarge=true` dan sisa dikuras. Part ke-`MaxUploadFiles+1` dst. -> `rejected too_many_files`, dikuras.
- Total byte mentah semua part file > `MaxUploadBytes` -> 413 `upload_too_large`, tidak ada tulis, tanpa audit.
- Gagal DB di tengah batch -> rollback seluruh batch -> 500, tanpa audit.

---

## 11. Test Scenario

### Unit `base/` (tanpa mock, rule 13.1)

- `path_test.go` (table-driven): `docs/auth/login.md` ok; `""` path_empty; `/a.md` path_absolute; `../x.md`, `a/./b.md`, `a/../b.md` path_traversal; `a//b.md`, `a/` path_empty_segment; `a\x00b.md`, `a\nb.md` path_control_char; 600 rune path_too_long; 513 rune path_too_long, 512 rune ok; segmen 256 rune segment_too_long, 255 ok; byte `\xff` invalid_encoding; `Docs/A.MD` dipertahankan apa adanya; `a\b.md` ok (D11). `IsHidden`: `.git/config`, `a/.DS_Store`, `docs/.hidden/x.md` true; `docs/a.b.md` false. `IsSupported`: `.md`, `.MD`, `.markdown`, `.txt` true; `.yaml`, `README`, `x.md.bak` false. `NormalizePrefix`: `""` root, `docs/` -> `docs`, `docs//` path_empty_segment, `/docs` path_absolute. `LikePrefix`: `""` -> `%`; `docs` -> `docs/%`; `a_b%c\d` -> `a\_b\%c\\d/%`. `StripLeadingSlash`: `/a` -> `a`, `//a` -> `/a`.
- `content_test.go`: BOM dibuang; tepat `MaxFileBytes` ok, +1 too_large; BOM + tepat batas ok; NUL invalid_encoding; `\xff\xfe` invalid_encoding; biner PNG header invalid_encoding; kosong ok. `Hash` 32 byte, `Version` 64 hex; `ValidVersion` menolak huruf besar/63 karakter/non-hex. `Title`: `# Login\n` -> `Login`; `## Sub` saja -> ``; `#NoSpace` -> ``; `# Judul #` -> `Judul`; heading di dalam ``` diabaikan; heading setelah 4 KiB diabaikan; judul 300 rune dipotong 255.
- `file_test.go`: `NewFile` mengisi Title/Hash/Size/label; `WithContent` mengubah Updated* saja; `ToDTO` version = hex hash; `ToConflict`.
- `upload_test.go`: `ClassifyPart` urutan alasan (`.git/x.md` skipped bukan rejected; `.DS_Store` skipped; `x.png` rejected unsupported_type; `../x.md` path_traversal; 2 MB too_large; biner `.md` invalid_encoding). `PlanUpload`: baru -> created; hash sama -> unchanged; hash beda -> updated; duplikat -> kemunculan kedua rejected duplicate_in_request; kuota: currentCount 4998, limit 5000, 3 baru -> 2 created + 1 project_file_limit; updated tidak memakan kuota. `Summarize`, `ChangedPaths(…, 20)` truncated pada 25 path, `SortByIndex`.
- `query_test.go`: `login` -> `"login"`; `reset password` -> `"reset" "password"`; `"reset password"` -> `"reset password"`; `"unclosed phrase` -> `"unclosed phrase"`; `a & b : c or -d !e` -> `"a" "b" "c" "or" "d" "e"`; `"""` / `&&&` / spasi -> ok=false; 17 term -> 16; 300 rune dipotong 200; karakter kontrol dibuang; `kata_satu` tetap satu term; huruf Indonesia/emoji (emoji bukan letter -> pemisah).
- `tree_test.go`: 3 tingkat (`docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`) prefix `docs` -> folder `auth` (file_count 2) + file `a.md`; root; urutan nama; meta di luar prefix diabaikan; prefix `docs` tidak menyertakan `docs2/`.
- `slice_test.go`: ASCII; `é` (2 byte), `😀` (4 byte) offset di tengah rune maju; end di tengah rune mundur; limit 1 pada rune 4 byte tetap maju satu rune; chaining NextOffset menyusun ulang isi persis (fuzz kecil dengan string acak multibyte); offset -1 dan len+1 invalid_offset; offset == len -> kosong, truncated false.
- `similar_test.go`: typo `docs/auth/logn.md` menemukan `docs/auth/login.md`; basename sama di folder lain lebih dulu; maks 5; target sendiri tidak ikut; urutan stabil.
- `snippet_test.go`: `a \x01login\x02 b` -> 3 segmen; marker yatim; `ToMarkdownBold`.
- `errors_test.go`: `Invalid(r)` -> 400 + details.reason + `errors.Is(err, errs.ErrInvalid)`; `TooLarge` -> 413; `VersionConflict` -> 409 + details.current; `respond.StatusFor` sesuai.
- `limits_test.go`: `LimitsFrom` dan dua body limit.

### Subpackage I/O

- `store_integration_test.go` (tag integration, `db.TestDB`): insert fixture via SQL test; `FindByPath` hanya project sendiri; `CountFiles`; `ListMeta` dengan prefix ber-`_`/`%` (`a_b/x.md` vs `axb/x.md`); `ListPaths`; `Search` urutan skor (AC4), total, prefix, frasa, kueri tanpa hasil total 0; konten berisi `\x01` tidak merusak headline; `LockProject` memblokir tx kedua (dua goroutine, tx kedua menunggu sampai tx pertama commit); id bukan uuid -> not found tanpa error; file 1 MB berisi kata unik bisa di-insert (tsvector < 1 MB, D4); isolasi dua project.
- `access_test.go`: registry bus nyata dengan handler `check-access` palsu: tanpa identitas -> 401; error bus / `ErrNotRegistered` -> error (500); `ProjectExists=false` -> 404; `LevelNone` -> 404; member/owner/admin -> actor dengan Label = Name, fallback Email.
- `audit_test.go`: handler `record` palsu menerima `record.Request` dengan ActorType user, label, ProjectLabel dari `get-project` palsu; `get-project` error -> label kosong, tetap dikirim; `record` error -> di-log, `Record` tidak panik/tidak mengembalikan error; `Args`/`Result` diteruskan tanpa isi file.

### Integrasi lintas usecase (`knowledge_integration_test.go`, tag `integration`)

- **T05-AC1**: upload multipart 3 tingkat (`docs/a.md`, `docs/auth/login.md`, `docs/auth/flow/x.md`) -> 3 created; tree prefix `docs` dan `docs/auth` sesuai; upload ulang -> 3 unchanged; ubah satu -> 1 updated + 2 unchanged; file lain di project tetap.
- **T05-AC2**: satu upload berisi `../x.md`, path 600 karakter, file 2 MB, biner `.md`, `.git/config`, `.DS_Store`, dan 2 file valid -> 4 rejected dengan reason benar, 2 skipped, 2 created; `getfile` 2 file valid ok.
- **T05-AC3**: dua `getfile` (version sama), `updatefile` A -> 200, `updatefile` B dengan version lama -> 409 `version_conflict` dengan `details.current.content` = isi A; isi tersimpan = A.
- **T05-AC4**: `login.md` (pendek, path memuat login) vs `guide.md` panjang yang menyebut `login` sekali -> `login.md` peringkat 1; kueri `"`, `&`, `:`, `or`, `-`, `!`, `"unclosed` -> 200.
- **T05-AC6**: user non-anggota mendapat 404 di 9 route (termasuk `projectID` bukan uuid); hapus folder `docs` menghapus `docs/**` semua kedalaman, `docs2/x.md` dan `docs.md` tetap, project lain tetap.
- **T05-AC7**: upload 100 file -> tepat 1 entri audit `knowledge.upload.completed` dengan `paths` 20 dan `paths_truncated=true`.
- **T05-AC8**: dua project dengan file `same.md` dan kata unik masing-masing: `list-files`, `read-file`, `search` untuk project A tidak pernah memuat data B; `read-file` memotong multibyte benar dan chaining menyusun ulang isi.
- **T05-AC9**: 5000 file (isi campuran 1-50 KB), 50 kueri, p95 <= 200 ms; EXPLAIN memakai `knowledge_files_search_gin` dan `knowledge_files_project_path_prefix`.
- Kuota: `MAX_PROJECT_FILES=3` di config test; 2 upload paralel masing-masing 2 file baru -> total 3 file, satu `project_file_limit`.
- `count-files` terdaftar: `bus.CallTyped[contentcount.Response]` untuk 2 project + 1 id bukan uuid -> map benar; bila handler T03 asli dipakai, `GET /api/projects/{id}` menampilkan `counts.files`.
- Audit: tiap mutasi sukses tepat 1 entri; gagal 0 entri; tidak ada entri yang memuat isi file (cari string isi unik di semua `Args`/`Result`, rule 13.6).
- `move`: tsvector mengikuti path baru (search kata dari nama baru menemukan file).
- Cascade: hapus baris `projects` -> semua `knowledge_files` project itu hilang.

---

## 12. E2E Scenarios (Playwright terhadap binary; butuh T01, T02, T03 merge)

Positif:
1. Login developer -> buat project -> tab Knowledge -> upload folder `docs/` (3 tingkat, `webkitdirectory`) -> laporan 3 created -> pohon menampilkan `docs/auth/flow/x.md`.
2. Upload ulang folder yang sama -> laporan semua `unchanged`.
3. Buka `docs/auth/login.md` -> viewer Markdown + diagram Mermaid dirender -> Edit -> simpan -> isi baru tampil.
4. Buat file baru `notes/todo.md` dari editor -> muncul di pohon.
5. Pindah `notes/todo.md` ke `archive/todo.md` -> folder `notes` hilang dari pohon.
6. Cari `login` -> `login.md` di urutan pertama dengan sorotan.
7. Hapus folder `archive` dengan konfirmasi -> hilang; `docs/` tetap.
8. Super Admin (bukan anggota) membuka knowledge project -> bisa melihat dan mengedit.

Negatif:
1. Upload folder berisi `.git/`, gambar `.png`, file 2 MB -> laporan `skipped`/`rejected` dengan alasan; file valid tetap masuk.
2. Dua tab mengedit file yang sama; tab kedua menyimpan -> dialog konflik 409 (muat ulang/bandingkan), isi tab pertama tidak tertimpa.
3. Markdown berisi `<script>alert(1)</script>` dan `[x](javascript:alert(1))` -> tidak dieksekusi, tautan tidak aktif (AC5).
4. Developer non-anggota membuka URL knowledge project lain -> halaman 404.
5. Buat file `../x.md` atau `.env` -> pesan error dengan alasan.
6. Kueri pencarian `" & : or -` -> tidak error, hasil kosong/valid.
7. Request mutasi tanpa header CSRF (lewat `page.request`) -> 403 `CSRF_REQUIRED`.

---

## 13. Request/Response body

Format error seragam (rule 11.1):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```

### POST /api/projects/{projectID}/knowledge/upload

Request: `Content-Type: multipart/form-data; boundary=...`, header `X-Requested-With: grimoire`.

```
--b
Content-Disposition: form-data; name="files"; filename="docs/auth/login.md"
Content-Type: text/markdown

# Login
...
--b
Content-Disposition: form-data; name="files"; filename="../x.md"
...
--b--
```

200:

```json
{
  "summary": { "created": 2, "updated": 1, "unchanged": 3, "skipped": 1, "rejected": 2 },
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

413 (total > `MAX_UPLOAD_BYTES`):

```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran total upload melebihi batas", "details": { "reason": "upload_too_large", "max_bytes": 52428800 } }, "trace_id": "..." }
```

400: `{"error":{"code":"INVALID_ARGUMENT","message":"upload tidak berisi file","details":{"reason":"no_files"}},"trace_id":"..."}`

### POST /api/projects/{projectID}/knowledge/files

```json
{ "path": "docs/auth/login.md", "content": "# Login\n\nAlur login..." }
```

201:

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

409: `{"error":{"code":"CONFLICT","message":"file dengan path ini sudah ada","details":{"reason":"path_exists"}},"trace_id":"..."}`
400 kuota: `{"error":{"code":"INVALID_ARGUMENT","message":"project sudah mencapai batas jumlah file","details":{"reason":"project_file_limit"}},"trace_id":"..."}`
413: `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"ukuran file melebihi batas 1048576 byte","details":{"reason":"too_large","max_bytes":1048576}},"trace_id":"..."}`

### PUT /api/projects/{projectID}/knowledge/files?path=docs%2Fauth%2Flogin.md

```json
{ "content": "# Login\n\nAlur login v2", "version": "3f79bb7b435b05321651daefd374cdc681dc06faa65e374e38337b88ca046dea" }
```

200: FileDTO dengan `version` baru. 409:

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "file sudah diubah orang lain; muat ulang sebelum menyimpan",
    "details": {
      "reason": "version_conflict",
      "current": {
        "content": "# Login\n\nAlur login versi Sari",
        "version": "a1b2...64hex",
        "updated_at": "2026-10-24T09:05:00Z",
        "updated_by_label": "Sari"
      }
    }
  },
  "trace_id": "..."
}
```

400 version: `{"error":{"code":"INVALID_ARGUMENT","message":"version wajib diisi dengan nilai dari getfile","details":{"reason":"invalid_version"}},"trace_id":"..."}`

### GET /api/projects/{projectID}/knowledge/files?path=docs%2Fauth%2Flogin.md

200: FileDTO (seperti 201 di atas). 404: `{"error":{"code":"NOT_FOUND","message":"file tidak ditemukan"},"trace_id":"..."}`

### GET /api/projects/{projectID}/knowledge/tree?prefix=docs

```json
{
  "prefix": "docs",
  "folders": [ { "name": "auth", "path": "docs/auth", "file_count": 2 } ],
  "files": [ { "name": "a.md", "path": "docs/a.md", "size_bytes": 120, "updated_at": "2026-10-24T09:00:00Z" } ],
  "total_files": 3
}
```

### DELETE /api/projects/{projectID}/knowledge/files?path=docs%2Fa.md

204 tanpa body. 404 `file tidak ditemukan`.

### DELETE /api/projects/{projectID}/knowledge/folders?path=docs

200: `{ "deleted": 3 }`. 400 root: `{"error":{"code":"INVALID_ARGUMENT","message":"path folder wajib diisi; root tidak bisa dihapus sekaligus","details":{"reason":"prefix_required"}},"trace_id":"..."}`. 404: `folder tidak ditemukan atau kosong`.

### POST /api/projects/{projectID}/knowledge/files/move

```json
{ "from": "notes/todo.md", "to": "archive/todo.md" }
```

200: FileDTO dengan `path` baru, `version` sama. 409 `path_exists`; 404 `file tidak ditemukan`; 400 `same_path`/`hidden`/`unsupported_type`.

### GET /api/projects/{projectID}/knowledge/search?q=login&prefix=docs&limit=20

```json
{
  "items": [
    {
      "path": "docs/auth/login.md",
      "title": "Login",
      "score": 0.83,
      "snippet": [ { "text": "Alur ", "highlight": false }, { "text": "login", "highlight": true }, { "text": " memakai email dan password", "highlight": false } ]
    }
  ],
  "total": 1
}
```

Kueri tanpa kata: `{ "items": [], "total": 0 }`. `limit=abc`: 400 `invalid_limit`.

Non-anggota di semua route: `404 {"error":{"code":"NOT_FOUND","message":"project tidak ditemukan"},"trace_id":"..."}`. Tanpa sesi: 401 `UNAUTHORIZED`. Mutasi tanpa CSRF: 403 `CSRF_REQUIRED`.

### Bus (Go, bukan JSON di wire)

```go
res, err := bus.CallTyped[readfile.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
    readfile.Request{ProjectID: pid, Path: "/docs/auth/login.md", Offset: 0, Limit: 0})
// ketemu:     readfile.Response{Found: true, Path: "docs/auth/login.md", Content: "...", SizeBytes: 65533,
//             TotalBytes: 120000, NextOffset: 65533, Truncated: true}
// tidak ada:  readfile.Response{Found: false, Similar: []string{"docs/auth/login.md"}}, err nil
// path salah: err membungkus errs.ErrInvalid: `path "../x.md" tidak valid: path tidak boleh memuat segmen . atau ..`

bus.CallTyped[listfiles.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
    listfiles.Request{ProjectID: pid, Prefix: "docs", Recursive: false, Limit: 100})
// listfiles.Response{Items: [{Path: "docs/a.md", SizeBytes: 120, UpdatedAt: t}], Folders: ["docs/auth"],
//                    Total: 1, Truncated: false}

bus.CallTyped[search.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
    search.Request{ProjectID: pid, Query: "login", Limit: 5})
// search.Response{Items: [{Path: "docs/auth/login.md", Score: 0.83, Snippet: "Alur **login** memakai email"}], Total: 1}

bus.CallTyped[contentcount.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: []string{pidA, pidB, "bukan-uuid"}})
// contentcount.Response{Counts: map[string]int{pidA: 12, pidB: 0, "bukan-uuid": 0}}
```

### Audit (`record.Request` yang dikirim, contoh upload)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f0c4-...-0a", ActorLabel: "Budi",
    ProjectID: pid, ProjectLabel: "Payment Service",
    Action: record.ActionKnowledgeUploadCompleted, TargetType: "knowledge", Target: "",
    Args:   map[string]any{"files": 100},
    Result: map[string]any{"created": 98, "updated": 0, "unchanged": 0, "skipped": 1, "rejected": 1,
        "bytes": 812345, "paths": []string{"docs/a.md", "...20 path"}, "paths_truncated": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...",
}
```

---

## 14. Skema OpenAPI (`api/modules/project/knowledge.yml`, ringkas)

- `info.version: 0.1.0`. Komponen bersama (`Error`, responses `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `PayloadTooLarge`, `Internal`) dirujuk dari `../infra/foundation.yml`.
- Path item: `/api/projects/{projectID}/knowledge/upload` (post, `multipart/form-data` dengan `files: array of binary`, keterangan bahwa `filename` membawa path relatif), `/knowledge/files` (post, get, put, delete; `path` query wajib untuk get/put/delete), `/knowledge/files/move` (post), `/knowledge/tree` (get, `prefix` opsional), `/knowledge/folders` (delete, `path` wajib), `/knowledge/search` (get, `q`, `prefix`, `limit` 1..50 default 20).
- Schema: `KnowledgeFile` (FileDTO), `KnowledgeUserRef`, `KnowledgeTree`, `KnowledgeFolderNode`, `KnowledgeFileNode`, `KnowledgeUploadResult` (+ `KnowledgeUploadItem` enum status, `reason` string), `KnowledgeSearchResult` (+ `KnowledgeSnippetSegment`), `KnowledgeConflictDetails`, `KnowledgeCreateRequest`, `KnowledgeUpdateRequest`, `KnowledgeMoveRequest`, `KnowledgeDeleteFolderResult`.
- `details.reason` didokumentasikan sebagai enum string dari `reasons.go` di deskripsi tiap respons 400/409/413.
- Mutasi mencantumkan header `X-Requested-With: grimoire`.

---

## 15. Risiko dan catatan

- **T03 belum merge** saat plan ini ditulis. Bila kontrak T03 berubah saat merge, yang terdampak hanya `base/access`, `base/audit`, `countfiles`; sesuaikan ke kode di `main`.
- **Versi migrasi** paralel dengan T04/T06; cek unik sebelum commit (bagian 6).
- **Hitungan `Actions()`** di `actions_test.go` berubah juga oleh T04/T06 bila mereka menambah aksi; resolusi saat rebase.
- **Upload besar vs timeout**: `HTTP_DEFAULT_TIMEOUT` 30 s memotong context (query DB) bukan pembacaan body; upload 50 MB lewat koneksi lambat bisa gagal 504 saat tulis DB. Diterima untuk versi pertama (tim internal, LAN); catat di PR. Tidak menambah middleware timeout khusus.
- **Memori upload**: isi valid ditahan di memori sampai tx (maks `MAX_UPLOAD_BYTES` per request). Diterima pada skala desain.
- **Nama file dengan `"`**: browser meng-encode `"` sebagai `%22` di `filename`; tanpa percent-decoding path tersimpan memuat `%22`. Kasus langka, didokumentasikan.
- **Unicode NFD (macOS)**: tanpa normalisasi NFC, `é` NFD dan NFC menjadi path berbeda (D11). Didokumentasikan; bisa ditambah di tiket lanjutan.
- **FTS `simple`** tanpa stemming: `login` tidak cocok `logins`. Sesuai tiket (D5).
- **Benchmark AC9** bergantung mesin dev; test integrasi memakai ambang 200 ms dengan `-tags integration`, dan skip dengan pesan bila `testing.Short()`.

---

## 16. Daftar usecase (plan rinci ditulis terpisah di `docs/superpowers/plans/project-knowledge/usecase-<slug>.md`)

| # | Slug | Gelombang | Titik masuk | Ringkasan | AC |
|---|---|---|---|---|---|
| U1 | `createfile` | A | `POST /api/projects/{projectID}/knowledge/files` | access -> NormalizePath + hidden + IsSupported (400) -> PrepareContent (413/400) -> tx LockProject + kuota + INSERT (409 path_exists) -> 201 FileDTO. Audit `knowledge.file.created {bytes}` | AC6 |
| U2 | `getfile` | A | `GET /api/projects/{projectID}/knowledge/files?path=` | access -> NormalizePath -> FindByPath (404) -> 200 FileDTO + version. Tanpa audit | AC3, AC6 |
| U3 | `listtree` | A | `GET /api/projects/{projectID}/knowledge/tree?prefix=` | access -> NormalizePrefix -> ListMeta(LikePrefix) -> BuildLevel -> 200 satu level. EXPLAIN indeks prefix. Tanpa audit | AC1, AC6 |
| U4 | `countfiles` | A | bus `project.knowledge.count-files` (Call, kontrak T03 `contentcount`) | Expect; id bukan uuid -> 0; satu query GROUP BY; semua id ada di map. Tanpa audit | - (T03) |
| U5 | `uploadfiles` | B | `POST /api/projects/{projectID}/knowledge/upload` (multipart) | access -> stream part (filename mentah) -> ClassifyPart -> tx LockProject + CountFiles + select hash + PlanUpload + batch tulis -> 200 laporan urut request. 413 total. Audit `knowledge.upload.completed` sekali | AC1, AC2, AC7 |
| U6 | `updatefile` | B | `PUT /api/projects/{projectID}/knowledge/files?path=` | access -> NormalizePath -> PrepareContent -> ValidVersion -> no-op terverifikasi (D12) -> UPDATE atomik WHERE hash -> 0 baris: 404/409 + current. Audit `knowledge.file.updated {bytes_before, bytes}` | AC3 |
| U7 | `deletefile` | B | `DELETE /api/projects/{projectID}/knowledge/files?path=` | access -> NormalizePath -> DELETE RETURNING (404) -> 204. Audit `knowledge.file.deleted {bytes}` | AC6 |
| U8 | `deletefolder` | B | `DELETE /api/projects/{projectID}/knowledge/folders?path=` | access -> NormalizePrefix (root 400 prefix_required) -> DELETE LIKE ESCAPE -> 0: 404 -> 200 {deleted}. Audit `knowledge.folder.deleted {count}` | AC6 |
| U9 | `movefile` | B | `POST /api/projects/{projectID}/knowledge/files/move` | access -> validasi from/to (same_path, hidden, unsupported) -> tx LockProject + FindByPath from (404) + to (409) + UPDATE path -> 200 FileDTO version tetap. Audit BARU `knowledge.file.moved {from, to}` | AC6 |
| U10 | `search` | C | `GET /api/projects/{projectID}/knowledge/search?q=&prefix=&limit=` | access -> ParseQuery (kosong -> 200 []) -> NormalizePrefix -> ClampLimit -> store.Search -> ParseHeadline -> 200 segmen. Benchmark 5000 file + EXPLAIN GIN. Tanpa audit | AC4, AC6, AC9 |
| U11 | `buslistfiles` | C | bus `project.knowledge.list-files` (Call) | Expect -> uuid -> StripLeadingSlash + NormalizePrefix -> Limit/Offset -> recursive SELECT paged + COUNT, atau ListMeta + BuildLevel (Items langsung + Folders) -> Truncated. Tanpa audit | AC8 |
| U12 | `busreadfile` | C | bus `project.knowledge.read-file` (Call) | Expect -> uuid -> StripLeadingSlash + NormalizePath -> FindByPath; tidak ada -> ListPaths + SimilarPaths(5); ada -> clamp Limit (65536, maks MCPMaxResponseBytes) -> SliceUTF8. Tanpa audit | AC8 |
| U13 | `bussearch` | C | bus `project.knowledge.search` (Call) | Expect -> uuid -> ParseQuery (kosong -> kosong) -> prefix -> ClampLimit -> store.Search (query sama dengan HTTP) -> ToMarkdownBold. Tanpa audit | AC8 |

Frontend: `docs/superpowers/plans/project-knowledge/frontend-plan.md` (ditulis terpisah): `frontend/src/domain/project/knowledge/{tree,viewer,editor,upload,search,...}/` dengan `services` (Zod sesuai bagian 13), `stores`, `components`, `views`, `index.ts`; Markdown tersanitasi + Mermaid `securityLevel: 'strict'` (AC5); editor dengan penanganan 409 memakai `details.current`; upload folder `webkitdirectory` dengan `FormData.append('files', file, file.webkitRelativePath)`; upload campuran `.md` + `.yaml` dikomposisi di `app/` (rule 12.4); tab Knowledge dipasang di workspace project T03.
