# Usecase Plan: repo-skeleton-config (T00 Foundation, langkah 1)

Overview induk: `docs/superpowers/plans/foundation/` (overview T00). Tiket: `docs/tickets/T00-foundation.md`. Aturan: `rules.txt`, `docs/tickets/README.md`.
Konteks yang dicek: belum ada `CLAUDE.md`, belum ada `.claude/rules/`, belum ada kode di `backend/`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | repo-skeleton-config (satu PR koheren) |
| Scope | fullstack-tooling (hanya sisi backend Go dan tooling repo; frontend belum disentuh) |
| Domain | infra |
| Module | foundation |
| Resource root | `backend/internal/infra` |
| AC yang dicakup | T00-AC7 (semua field Config terdokumentasi) |

| Endpoint | Method | Version | Keterangan |
|---|---|---|---|
| (tidak ada) | - | - | Usecase ini tidak menambah endpoint HTTP. `/healthz`, `/readyz` milik usecase `http-server` |

Endpoint bus: tidak ada (provider maupun consumer). Aksi audit: tidak ada.

Pola `init/handler/params/usecase/repository` TIDAK berlaku (tidak ada logika bisnis). Rule 4.6 berlaku: `internal/infra/**` dilarang meng-import `internal/domain/**`. Stub `base.go` domain boleh meng-import `infra/app` (arah domain -> infra diizinkan).

## 2. Summary Plan

Membuat fondasi kompilasi dan konfigurasi untuk semua usecase T00 berikutnya: `go.mod`, seluruh struktur folder sesuai konvensi, stub tiga domain (`core`, `project`, `discovery`) dengan `Register` no-op, paket bersama `lib/common/reqctx` dan `lib/common/errs`, `infra/app.Deps` (tahap awal: `Config` dan `Log`), `infra/config` (parse dan validasi env, kumpulkan semua error sekaligus, secret disamarkan), `infra/logger` (slog JSON/text dengan `trace_id`, `user_id`, `api_key_id` dari reqctx), serta berkas repo: `docker-compose.dev.yml` (Postgres 16 saja), `.env.example`, `.gitignore`, kerangka `makefile`, dan `docs/configuration.md`.

Keputusan penting:
- Config diparse dari fungsi `getenv` yang diinjeksi sehingga test tidak menyentuh env proses.
- Semua field `Config` memakai struct tag `env:"NAME"`, dipakai test T00-AC7 lewat refleksi.
- Tidak ada dependensi eksternal selain `log/slog` stdlib dan `github.com/stretchr/testify` TIDAK dipakai (cukup stdlib `testing`) agar `go.sum` tetap kosong pada langkah ini.
- Usecase lain hanya boleh menambah (field Config, env, field Deps), tidak mengubah signature.

## 3. Plan (langkah berurutan)

### Langkah 1. go.mod dan .gitignore
1. Buat `backend/go.mod`: `module grimoire`, `go 1.23`. Tanpa `require` (belum ada dependensi).
2. Ubah `.gitignore` (sudah ada, untracked): pastikan memuat `bin/`, `.env`, `frontend/node_modules/`, `frontend/dist/`, `backend/internal/infra/static/dist/*` dengan negasi `!backend/internal/infra/static/dist/index.html`, `playwright-report/`, `test-results/`, `.DS_Store`. Jangan hapus entri yang sudah ada.

### Langkah 2. Struktur folder
Buat (folder kosong diberi `.keep`/`.gitkeep` bila belum berisi file):
```
backend/cmd/http/                      .keep
backend/cmd/cli/                       .keep
backend/cmd/internal/server/           .keep
backend/cmd/internal/buildinfo/        .keep
backend/internal/domain/{core,project,discovery}/   (berisi base.go)
backend/internal/infra/{app,config,logger}/         (berisi kode)
backend/internal/infra/{bus,db,http,middleware,static,jobs}/  .keep
backend/internal/archtest/             .keep
backend/lib/common/{reqctx,errs}/      (berisi kode)
backend/lib/constants/                 .keep
backend/lib/contracts/                 .keep
backend/lib/util/                      .gitkeep
backend/migrations/{core,project}/     .keep
api/modules/infra/                     .keep
```
`.keep` di folder yang akan diisi usecase lain dihapus oleh usecase pemiliknya.

### Langkah 3. lib/common/errs (`backend/lib/common/errs/errs.go`)
Implementasi persis kontrak overview 4.2: sentinel `ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrInvalid`, `ErrUnauthorized`; tipe `Code` dan 12 konstanta (`CodeNotFound` ... `CodeInternal`); struct `Error{Code, Message, Details, Err}`; `Error()` (format `"<CODE>: <message>"`, tidak memuat `Err`), `Unwrap()`, `New`, `Wrap`, `WithDetails`. Hanya import stdlib (`errors`).

Catatan: `Error.Is(target)` tidak perlu; `errors.Is(wrapped, errs.ErrNotFound)` bekerja bila pemanggil memakai `Wrap(CodeNotFound, msg, errs.ErrNotFound)`.

### Langkah 4. lib/common/reqctx (`backend/lib/common/reqctx/reqctx.go`)
Implementasi kontrak overview 4.1: kunci privat `type key int`, `WithTraceID/TraceID`, `WithUserID/UserID`, `WithRole/Role`, `WithAPIKeyID/APIKeyID`, `Identity`, `WithIdentity` (juga set UserID dan Role), `IdentityFrom`, `Carry(from, to)`. Getter mengembalikan "" bila tidak ada; ctx nil tidak diperlukan ditangani (pemanggil wajib non-nil). Tanpa import `internal/**`.

### Langkah 5. infra/config (`backend/internal/infra/config/config.go`)
1. Struct `Config` dengan 22 field dari tabel overview bagian 9, tiap field ber-tag `env:"NAME"`. Tipe: string, bool, int, int64, `time.Duration`.
2. `func Load(getenv func(string) string) (Config, error)`:
   - Terapkan default saat env kosong (string kosong setelah `strings.TrimSpace`).
   - Parse per field; setiap kegagalan dikumpulkan ke slice `[]error` berformat `"<ENV>: <pesan>"`; validasi lintas-field (idle <= open, `MAX_UPLOAD_BYTES` >= `MAX_FILE_BYTES`, `DEV_VITE_URL` hanya saat `APP_ENV=development`) dijalankan hanya bila field terkait lolos parse sendiri.
   - Bila ada error, kembalikan `Config{}` dan error tunggal bertipe `*ValidationError` (`Problems []string`) dengan `Error()`:
     ```
     konfigurasi tidak valid:
     - DATABASE_URL: wajib diisi
     - SESSION_SECRET: minimal 32 byte (sekarang 12)
     ```
     Pesan tidak boleh memuat nilai secret; untuk nilai non-secret yang salah boleh dikutip (`nilai "verbose" tidak dikenal (debug|info|warn|error)`). Untuk `DATABASE_URL` yang gagal parse, JANGAN sertakan nilainya (bisa berisi password).
   - `LoadFromEnv() (Config, error)` = `Load(os.Getenv)`.
3. Validasi `DATABASE_URL`: tidak kosong, `url.Parse` sukses, scheme `postgres` atau `postgresql`, host tidak kosong.
4. Validasi `HTTP_ADDR`: `net.SplitHostPort`; port numerik 0-65535.
5. `DEV_VITE_URL`: bila terisi, `url.Parse`, scheme `http`/`https`, host tidak kosong.
6. Penyamaran:
   - `func (c Config) String() string` dan `func (c Config) LogValue() slog.Value` (grup atribut) memakai helper `maskURL(string) string` (ganti password userinfo dengan `***`, bila parse gagal kembalikan `***`) dan `SessionSecret` selalu `***`.
   - `func (c Config) GoString() string` mengembalikan `String()` agar `%#v` tidak bocor.
   - Gunakan value receiver agar `%v` pada Config maupun pointer aman.
7. Hanya stdlib.

### Langkah 6. infra/logger (`backend/internal/infra/logger/logger.go`)
1. `func New(cfg config.Config) *slog.Logger` -> `NewWithWriter(cfg, os.Stdout)`.
2. `func NewWithWriter(cfg config.Config, w io.Writer) *slog.Logger`: handler dasar `slog.NewJSONHandler` atau `slog.NewTextHandler` sesuai `LogFormat`, level sesuai `LogLevel`.
3. Tipe `ctxHandler` membungkus `slog.Handler`; `Handle(ctx, r)` menambah atribut `trace_id`, `user_id`, `api_key_id` hanya bila nilai tidak kosong. `WithAttrs`/`WithGroup` mengembalikan `ctxHandler` baru yang tetap membungkus. `Enabled` didelegasikan.
4. Sebagai pengaman secret: handler menambahkan `ReplaceAttr` yang mengganti nilai atribut ber-key `password`, `secret`, `session_secret`, `token`, `database_url` (case-insensitive) dengan `***`.
5. Dokumentasi di komentar paket: wajib memakai `InfoContext(ctx, ...)` agar ctx terbaca.

### Langkah 7. infra/app (`backend/internal/infra/app/deps.go`)
```go
package app

type Deps struct {
    Config config.Config
    Log    *slog.Logger
}
```
Komentar: field `Ctx`, `DB`, `Bus`, `Jobs`, `API`, `MCP` ditambahkan oleh usecase yang membuat tipenya (`bus`, `db-migrations`, `jobs-scheduler`, `http-server`) dan difinalkan di `cli-and-wiring`. Import hanya `infra/config` dan stdlib.

### Langkah 8. Stub domain
Untuk masing-masing `core`, `project`, `discovery`: `backend/internal/domain/<d>/base.go`
```go
package core // project / discovery

import "grimoire/internal/infra/app"

// Register merakit module domain ini. No-op pada T00; diisi T01 (core), T03 (project), T07 (discovery).
func Register(d app.Deps) {}
```
Hapus tidak perlu `.keep` di folder ini.

### Langkah 9. Berkas repo
1. `docker-compose.dev.yml`: service `db` image `postgres:16`, env `POSTGRES_USER/PASSWORD/DB=grimoire`, port `5432:5432`, volume bernama `grimoire_pgdata`, healthcheck `pg_isready -U grimoire -d grimoire` (interval 5s, timeout 3s, retries 10). Tidak ada service lain.
2. `.env.example`: semua env bagian 9 beserta default sebagai komentar, `DATABASE_URL=postgres://grimoire:grimoire@localhost:5432/grimoire?sslmode=disable`, `SESSION_SECRET=` berisi placeholder `ganti-dengan-string-acak-minimal-32-byte`, `COOKIE_SECURE=false`, `APP_ENV=development`. Tambahan `TEST_DATABASE_URL` dan `CREATE_ADMIN_PASSWORD` (dikomentari).
3. `makefile` (kerangka): `.PHONY`, `-include .env` dan `export`, variabel `VERSION/COMMIT/DATE`, target `help`, `db-up` (`docker compose -f docker-compose.dev.yml up -d db`), `db-down`, `test` (`cd backend && go test ./...`), `vet` (`cd backend && go vet ./...`). Target `dev`, `build`, `test-integration`, `test-e2e`, `lint`, `migrate`, `api-bundle`, `api-check` ditulis sebagai stub yang mencetak `"diisi oleh usecase <slug>"` dan exit 1 agar tidak diam-diam sukses. Wajib tab untuk recipe.
4. `docs/configuration.md`: lihat Changes.

### Langkah 10. Test (lihat bagian 6) lalu jalankan
`cd backend && go build ./... && go vet ./... && go test -race ./...`

## 4. Migration

Tidak ada. Folder `backend/migrations/{core,project}/` hanya dibuat berisi `.keep`. `embed.go` dibuat usecase `db-migrations`.

## 5. Changes (per file/folder)

Root:
- `.gitignore` (ubah): bagian Langkah 1.
- `docker-compose.dev.yml` (baru).
- `.env.example` (baru).
- `makefile` (baru, kerangka).

Dokumen:
- `docs/configuration.md` (baru). Isi:
  1. Pengantar: konfigurasi hanya lewat env; gagal cepat saat start dengan semua error sekaligus; secret disamarkan di log.
  2. Tabel utama kolom `Env | Tipe | Default | Wajib | Validasi | Keterangan` berisi SEMUA 22 env bagian 9. Nama env ditulis dalam backtick di kolom pertama dan default ditulis dalam backtick (test T00-AC7 mencocokkan baris yang diawali `| \`NAME\``). Default dokumentasi harus identik dengan nilai di kode (bentuk string).
  3. Bagian "Env di luar Config": `CREATE_ADMIN_PASSWORD` (CLI create-admin), `TEST_DATABASE_URL` (test integrasi; wajib di CI), `CI`.
  4. Contoh `.env` pengembangan dan produksi.
  5. Catatan: `make dev` (diisi `cli-and-wiring`), migrasi (transaksi per file, tidak mendukung `CREATE INDEX CONCURRENTLY`; ditulis dengan placeholder yang dilengkapi `db-migrations`).
  6. Aturan pemeliharaan: setiap env baru wajib ditambah di tabel ini dan di tabel overview.
- `docs/tickets/README.md`: TIDAK diubah di usecase ini (diubah `architecture-guard`).

Backend:
- `backend/go.mod` (baru).
- `backend/internal/domain/core/base.go`, `.../project/base.go`, `.../discovery/base.go` (baru, stub).
- `backend/internal/infra/app/deps.go` (baru).
- `backend/internal/infra/config/config.go`, `config_test.go`, `docs_test.go`, `envexample_test.go` (baru; `TestEnvExample_Loads` membaca `.env.example` lewat path dari `runtime.Caller`).
- `backend/internal/infra/logger/logger.go`, `logger_test.go` (baru).
- `backend/lib/common/errs/errs.go`, `errs_test.go` (baru).
- `backend/lib/common/reqctx/reqctx.go`, `reqctx_test.go` (baru).
- Folder kosong ber-`.keep` sesuai Langkah 2 (baru).

Tidak disentuh: `rules.txt`, plan usecase lain, `frontend/`.

## 6. Test Scenario

Semua unit test, tanpa DB. Jalankan dengan `-race`. Penamaan rule 13.4.

### config (`config_test.go`)
Helper `env(map[string]string) func(string) string`; env minimum: `DATABASE_URL=postgres://u:p@h:5432/d`, `SESSION_SECRET` 32 karakter.
- `TestLoad_Defaults`: hanya dua env wajib -> setiap field sama dengan default tabel (`:8080`, `production`, `true`, 90, 1048576, 52428800, 500, 102400, false, 20, 5, 30m, 1048576, 10s, 30s, 15s, 5s, `info`, `json`, "").
- `TestLoad_OverridesApplied`: semua env diisi nilai non-default -> struct sesuai.
- `TestLoad_MissingRequired`: tanpa `DATABASE_URL` dan `SESSION_SECRET` -> error `*ValidationError` memuat kedua baris.
- `TestLoad_ReportsAllErrors`: `DATABASE_URL` kosong, `SESSION_SECRET` 12 byte, `LOG_LEVEL=verbose` -> satu error, tepat 3 problem, urutan stabil (urutan field di struct).
- `TestLoad_InvalidValues` (table-driven): secret 31 byte; `APP_ENV=staging`; `DB_CONN_MAX_LIFETIME=abc`; `DB_CONN_MAX_LIFETIME=0s`; `DB_MAX_IDLE_CONNS=30` dengan open 20; `MAX_UPLOAD_BYTES` < `MAX_FILE_BYTES`; `DEV_VITE_URL=http://localhost:5173` dengan `APP_ENV=production` (default); `DEV_VITE_URL=ftp://x`; `HTTP_ADDR=nonsense`; `DATABASE_URL=mysql://x/y`; `COOKIE_SECURE=maybe`; `AUDIT_RETENTION_DAYS=0`; `MCP_MAX_RESPONSE_BYTES=100`; `LOG_FORMAT=xml`. Tiap kasus: error menyebut nama env yang salah.
- `TestLoad_DevViteAllowedInDevelopment`: `APP_ENV=development` + `DEV_VITE_URL` valid -> sukses.
- `TestLoad_ErrorDoesNotLeakSecrets`: `DATABASE_URL=mysql://u:topsecretpw@h/d` dan secret pendek `shortsecret` -> `err.Error()` tidak memuat `topsecretpw` maupun `shortsecret`.
- `TestConfig_StringMasksSecrets`: `fmt.Sprintf("%v %+v %#v %s", c, c, c, c)` dan `slog` ke buffer JSON tidak memuat password DB maupun `SessionSecret`; memuat `***`; host DB tetap terlihat.
- `TestMaskURL`: tanpa userinfo (tidak berubah), user saja, user+password, string tak terparse -> `***`.

### docs (`docs_test.go`)
- `TestT00_AC7_AllConfigFieldsDocumented`: baca `docs/configuration.md` (path relatif dari file test: `../../../../docs/configuration.md`, dihitung dengan `runtime.Caller`), refleksi atas `Config` ambil tag `env`; untuk tiap nama pastikan ada baris tabel yang diawali `` | `NAME` `` dan baris itu memuat default (dalam backtick) yang sama dengan hasil `Load` minimal untuk field tersebut (default kosong ditulis `-`). Gagal dengan daftar field yang hilang.
- `TestT00_AC7_NoUnknownEnvInDocsTable`: setiap baris tabel utama merujuk tag yang ada di Config (mencegah dokumentasi usang).
- `TestConfig_AllFieldsHaveEnvTag`: tiap field exported punya tag `env` unik.

### logger (`logger_test.go`)
- `TestLogger_IncludesTraceAndUser`: ctx dengan `WithTraceID`, `WithUserID`, `WithAPIKeyID`; `InfoContext` ke buffer JSON -> `trace_id`, `user_id`, `api_key_id` terisi.
- `TestLogger_OmitsEmptyContextFields`: ctx kosong -> tidak ada key `trace_id`/`user_id`.
- `TestLogger_WithAttrsKeepsContextHandler`: `log.With("a","b").InfoContext(ctx, ...)` tetap memuat `trace_id`; juga setelah `WithGroup`.
- `TestLogger_DoesNotPrintSecrets`: log atribut `password`, `session_secret`, `token`, `database_url` -> nilai tidak muncul, `***` muncul; log `cfg` (LogValuer) tidak memuat secret.
- `TestLogger_LevelAndFormat`: `LogLevel=warn` menyembunyikan info; `LogFormat=text` menghasilkan baris `key=value` bukan JSON.

### reqctx dan errs
- `TestReqctx_RoundTrip`: tiap With/getter; ctx kosong mengembalikan "".
- `TestReqctx_IdentitySetsUserAndRole`, `TestReqctx_IdentityFromMissing`.
- `TestReqctx_Carry`: salin semua nilai ke ctx baru yang tidak punya nilai tersebut; ctx tujuan tetap tidak ikut cancel ctx sumber.
- `TestErrs_WrapUnwrap`: `errors.Is(Wrap(CodeNotFound,"x",ErrNotFound), ErrNotFound)`, `errors.As` ke `*Error` mendapatkan Code.
- `TestErrs_ErrorStringHidesCause`: `Error()` tidak memuat pesan `Err` internal.
- `TestErrs_WithDetails`.

### Build
- `go build ./...` dan `go vet ./...` bersih; `gofmt -l` kosong.
- Pemeriksaan manual rule 4.6: `grep -r "internal/domain" backend/internal/infra` kosong.

## 7. E2E Scenarios

Usecase ini belum menghasilkan binary, jadi E2E berupa verifikasi tingkat repo.

Positif:
1. `docker compose -f docker-compose.dev.yml up -d db` -> container `db` healthy; `pg_isready` sukses; `docker compose ... config` hanya memuat satu service.
2. `cd backend && go build ./... && go test -race ./...` lulus pada checkout bersih.
3. `make help` dan `make test` berjalan; `make dev` keluar dengan exit 1 dan pesan `diisi oleh usecase ...`.
4. Menyalin `.env.example` menjadi `.env` lalu `Load` dengan env tersebut (diuji lewat test kecil `TestEnvExample_Loads` yang mem-parse `.env.example` sederhana KEY=VALUE, mengabaikan komentar) -> sukses tanpa error.
5. `git status` tidak menampilkan `.env`, `bin/`, `node_modules/` setelah dibuat (ignore berlaku).

Negatif:
1. `Load` tanpa env apa pun -> error berisi `DATABASE_URL: wajib diisi` dan `SESSION_SECRET: wajib diisi` sekaligus.
2. `SESSION_SECRET` 31 byte -> ditolak; pesan menyebut panjang (31), bukan nilainya.
3. `APP_ENV=production` dengan `DEV_VITE_URL` terisi -> ditolak.
4. Menambah field baru ber-tag `env` ke Config tanpa mengubah `docs/configuration.md` -> `TestT00_AC7_AllConfigFieldsDocumented` gagal dan menyebut field.
5. Log yang menyertakan `cfg` tidak pernah memuat password DB atau secret.
6. `grep -r "internal/domain" backend/internal/infra` menemukan hasil -> dianggap pelanggaran rule 4.6 (ditegakkan otomatis oleh `architecture-guard`).

## 8. Request/Response body

Tidak ada endpoint HTTP pada usecase ini. Format yang ditetapkan dan dipakai usecase berikutnya:

### Error konfigurasi saat start (stderr, exit code 1)
```
konfigurasi tidak valid:
- DATABASE_URL: wajib diisi
- SESSION_SECRET: minimal 32 byte (sekarang 12)
- LOG_LEVEL: nilai "verbose" tidak dikenal (debug|info|warn|error)
```

### Config tersamarkan (log saat start, JSON)
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"config loaded","config":{"database_url":"postgres://grimoire:***@localhost:5432/grimoire?sslmode=disable","http_addr":":8080","app_env":"production","session_secret":"***","cookie_secure":true,"log_level":"info","log_format":"json"}}
```
(`LogValue` memuat semua field; contoh dipersingkat.)

### Baris log dengan konteks
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"http request","trace_id":"0b1c2d3e-4f5a-6789-abcd-ef0123456789","user_id":"0b7c4e1a-2d3f-4a5b-8c9d-0e1f2a3b4c5d"}
```

### Format error API seragam (dikonsumsi usecase `http-server`; kode berasal dari `errs.Code` yang dibuat di sini)
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "name": "wajib diisi" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```
```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

## 9. Definition of Done
- Semua test bagian 6 hijau dengan `-race`; `go vet` dan `gofmt` bersih.
- `docs/configuration.md` memuat 22 env; T00-AC7 hijau.
- Tidak ada import `internal/domain` dari `internal/infra`; `lib/**` tidak meng-import `internal/**`.
- Tidak ada perubahan di luar daftar Changes.
