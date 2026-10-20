# T00 Foundation — Overview Plan

Tiket: `docs/tickets/T00-foundation.md`
Sumber aturan: `rules.txt` (arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk).
Konteks yang dicek: belum ada `CLAUDE.md`, belum ada `.claude/rules/`, belum ada kode di `backend/` maupun `frontend/`.

Dokumen ini adalah plan FOUNDATION: keputusan bersama, paket bersama, kontrak, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/foundation/<slug>.md` dan WAJIB mengikuti keputusan di sini.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Scope | fullstack (backend Go, frontend Svelte, tooling repo/CI) |
| Domain | infra (bukan domain bisnis) |
| Module | foundation (bukan module domain) |
| Resource root | `backend/internal/infra` |
| Versi kontrak | Tanpa prefix versi di URL. `api/modules/infra/foundation.yml` memakai `info.version: 0.1.0` |

Endpoint HTTP yang dimiliki T00:

| Endpoint | Method | Version | Auth | Keterangan |
|---|---|---|---|---|
| `/healthz` | GET | - | publik | Proses hidup. Selalu 200 bila server menjawab |
| `/readyz` | GET | - | publik | Ping DB (timeout 2s). 200 atau 503 |
| `/*` (selain `/api/**`, `/mcp/**`, `/healthz`, `/readyz`) | GET, HEAD | - | publik | Static SPA ter-embed, atau proxy Vite di mode dev |
| `/api/**` yang tidak cocok route | semua | - | - | 404 `NOT_FOUND` atau 405 `METHOD_NOT_ALLOWED` dalam format error JSON |

Endpoint bus yang dimiliki T00 sebagai provider: **tidak ada**. T00 hanya mendefinisikan konstanta dan kontrak `core.auth.resolve-session` yang dikonsumsi middleware auth. Provider-nya T01.

---

## 2. Summary Plan

T00 membangun kerangka satu binary `grimoire` (Go) yang menyajikan `/api`, `/mcp` (diisi T07), `/healthz`, `/readyz`, dan SPA Svelte yang di-embed, dengan Postgres sebagai satu-satunya dependensi. Isinya: struktur repo, konfigurasi env yang divalidasi sekaligus, logger slog dengan `trace_id`, bus in-process (`Call`, `Publish`, `CallTyped`, `Shutdown`), koneksi DB sqlx/pgx + `BeginTx` + runner migrasi buatan sendiri, scheduler job dengan advisory lock, server chi dengan rantai middleware lengkap (recover, trace id, access log, security headers, CSRF, session auth via bus, rate limit, role guard, batas body, timeout), handler static dengan fallback SPA, CLI (`serve`, `migrate up`, `create-admin`, `version`), scaffold frontend, test arsitektur, tooling OpenAPI, dan CI.

T00 tidak punya entity bisnis, usecase, repository domain, tabel domain, aksi audit, atau endpoint REST bisnis. Pola folder `init/handler/params/usecase/repository` TIDAK berlaku. Setiap usecase dipetakan ke package infra (plus `cmd/`, `lib/`, `migrations/`, `frontend/`, tooling). Rule 4.6 berlaku penuh: `internal/infra/**` DILARANG meng-import `internal/domain/**`.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Library

| Kebutuhan | Pilihan | Alasan singkat |
|---|---|---|
| Go | >= 1.23, module path `grimoire` | Generik, `context.WithoutCancel` (1.21+), `log/slog` |
| Router | `github.com/go-chi/chi/v5` | Middleware per route/grup, kompatibel `net/http` |
| Logger | `log/slog` (stdlib) | JSON/text, handler kustom untuk `trace_id` |
| DB | `github.com/jmoiron/sqlx` + `github.com/jackc/pgx/v5/stdlib` | Sesuai rules 5.3 (sqlx) |
| Rate limit | `golang.org/x/time/rate` | Token bucket, keyed in-memory |
| UUID | `github.com/google/uuid` | trace id |
| Prompt password | `golang.org/x/term` | Input tanpa echo |
| Migrasi | runner sendiri (sekitar 150 baris) | Format versi 17 digit, tabel `schema_migrations`, embed. goose/migrate memakai tabel dan konvensi lain |
| Scheduler | buatan sendiri + `pg_try_advisory_lock` | Kecil, tanpa dependensi |
| Cek arsitektur Go | `golang.org/x/tools/go/packages` | Daftar import akurat per package |
| Cek arsitektur FE | `dependency-cruiser` | Aturan import berbasis regex path |
| OpenAPI bundle | `@redocly/cli` (devDependency di `frontend/package.json` atau via `npx`, versi dipin) | `redocly bundle` |
| Frontend | Vite + Svelte 5 + TypeScript + Tailwind v4 + Zod, Vitest, Playwright | Sesuai F10. Router minimal buatan sendiri (history API), i18n store buatan sendiri |

### 3.2 Binary dan entrypoint

- Artefak rilis: satu file `bin/grimoire`, dibangun dari `backend/cmd/cli`. Subcommand: `serve` (default bila tanpa argumen), `migrate up`, `create-admin`, `version`.
- `backend/cmd/http/main.go` tetap ada (F8/README menyebut `cmd/{http,cli}`) sebagai entrypoint server murni untuk `go run ./cmd/http` di mode dev. Isinya hanya memanggil `server.Run`.
- Logika perakitan server bersama ditaruh di `backend/cmd/internal/server` (package `server`, fungsi `Run(ctx context.Context, cfg config.Config, log *slog.Logger) error`). Alasan: perakitan butuh meng-import `internal/domain/*/base.go`, jadi tidak boleh di `infra/` (rule 4.6). Letak di bawah `cmd/` menjaga rule 4.7 (cmd hanya merakit) dan `internal/` Go membatasi pemakainya ke `cmd/http` dan `cmd/cli`.
- `version`, `commit`, `buildDate` diisi `-ldflags "-X grimoire/cmd/internal/buildinfo.Version=..."` (package `backend/cmd/internal/buildinfo`).

### 3.3 Penambahan folder di luar daftar rules 3.1

Tiga folder baru di luar daftar `infra/{bus,db,http,middleware,config,static,logger,jobs}`. Ketiganya dicatat di `rules.txt` 3.1 dan README tiket dalam PR yang sama (rule 15.3):
- `internal/infra/app`: hanya struct `Deps` yang diteruskan ke wiring domain.
- `internal/archtest`: test arsitektur (tanpa kode produksi).
- `cmd/internal/{server,buildinfo}`: perakitan bersama dua entrypoint.

### 3.4 Nama package

| Folder | Nama package | Catatan |
|---|---|---|
| `internal/infra/http` | `httpx` | Hindari bentrok dengan `net/http` |
| `internal/infra/middleware` | `middleware` | |
| `internal/infra/bus` | `bus` | |
| `internal/infra/db` | `db` | |
| `internal/infra/jobs` | `jobs` | |
| `internal/infra/config` | `config` | |
| `internal/infra/logger` | `logger` | |
| `internal/infra/static` | `static` | |
| `internal/infra/app` | `app` | |
| `lib/common/reqctx` | `reqctx` | |
| `lib/common/errs` | `errs` | |
| `lib/constants` | `constants` | |
| `lib/contracts/core/auth/resolvesession` | `resolvesession` | |

---

## 4. Paket bersama (setara "base/" untuk T00)

T00 tidak punya folder `base/` berisi entity. Peran "dipakai bersama" diisi paket berikut. Semua dibuat di usecase `repo-skeleton-config` (kecuali yang ditandai), dan usecase lain hanya boleh menambah, tidak mengubah signature.

### 4.1 `backend/lib/common/reqctx`

Tanpa import `internal/**`. Kunci context privat (`type key int`).

```go
func WithTraceID(ctx context.Context, id string) context.Context
func TraceID(ctx context.Context) string            // "" bila tidak ada
func WithUserID(ctx context.Context, id string) context.Context
func UserID(ctx context.Context) string
func WithRole(ctx context.Context, role string) context.Context
func Role(ctx context.Context) string
func WithAPIKeyID(ctx context.Context, id string) context.Context
func APIKeyID(ctx context.Context) string

// Identitas sesi lengkap yang ditaruh middleware auth.
type Identity struct {
    UserID, Email, Name, Role string
    MustChangePassword bool
}
func WithIdentity(ctx context.Context, id Identity) context.Context // juga set UserID dan Role
func IdentityFrom(ctx context.Context) (Identity, bool)

// Salin nilai propagasi (trace_id, user_id, role, api_key_id, identity) ke ctx lain.
func Carry(from, to context.Context) context.Context
```

### 4.2 `backend/lib/common/errs`

```go
var (
    ErrNotFound     = errors.New("not found")
    ErrForbidden    = errors.New("forbidden")
    ErrConflict     = errors.New("conflict")
    ErrInvalid      = errors.New("invalid argument")
    ErrUnauthorized = errors.New("unauthorized")
)

type Code string
const (
    CodeNotFound         Code = "NOT_FOUND"          // 404
    CodeForbidden        Code = "FORBIDDEN"          // 403
    CodeConflict         Code = "CONFLICT"           // 409
    CodeInvalid          Code = "INVALID_ARGUMENT"   // 400
    CodeUnauthorized     Code = "UNAUTHORIZED"       // 401
    CodeCSRFRequired     Code = "CSRF_REQUIRED"      // 403
    CodeRateLimited      Code = "RATE_LIMITED"       // 429
    CodePayloadTooLarge  Code = "PAYLOAD_TOO_LARGE"  // 413
    CodeMethodNotAllowed Code = "METHOD_NOT_ALLOWED" // 405
    CodeTimeout          Code = "TIMEOUT"            // 504
    CodeUnavailable      Code = "UNAVAILABLE"        // 503
    CodeInternal         Code = "INTERNAL"           // 500
)

type Error struct {
    Code    Code
    Message string // aman ditampilkan ke klien
    Details any    // opsional, aman ditampilkan (mis. daftar field invalid)
    Err     error  // penyebab internal, TIDAK pernah dikirim ke klien
}
func (e *Error) Error() string
func (e *Error) Unwrap() error
func New(code Code, msg string) *Error
func Wrap(code Code, msg string, err error) *Error
func WithDetails(code Code, msg string, details any) *Error
```

Module lain (T01 dst.) boleh menambah `Code` baru (mis. `PASSWORD_CHANGE_REQUIRED`) di paket ini atau di paketnya sendiri sebagai `errs.Code`. Pemetaan status untuk kode tak dikenal dari module: lihat `httpx.StatusFor` (4.5).

### 4.3 `backend/lib/constants/bus.go`

```go
package constants

// Provider: T01 core/auth. Consumer: infra/middleware (session auth).
const BUS_CORE_AUTH_RESOLVE_SESSION = "core.auth.resolve-session"
```

Konstanta lain ditambahkan tiket pemiliknya. `lib/constants/` boleh berisi file lain untuk konstanta non-bus bila perlu, tetapi T00 hanya membuat `bus.go`.

### 4.4 `backend/internal/infra/app/deps.go`

```go
type Deps struct {
    Ctx    context.Context // ctx server, dibatalkan saat shutdown (janitor rate limiter, goroutine milik module)
    Config config.Config
    Log    *slog.Logger
    DB     *db.DB
    Bus    *bus.Registry
    Jobs   *jobs.Scheduler
    API    chi.Router // sub-router yang di-mount di /api; csrf + session auth sudah terpasang
    MCP    chi.Router // sub-router yang di-mount di /mcp; TANPA csrf dan TANPA session cookie
}
```

Alur wiring: `cmd/internal/server.Run` -> `core.Register(deps)`, `project.Register(deps)`, `discovery.Register(deps)` -> module `base.go` -> feature `init.go`. Urutan pemanggilan Register tidak boleh berpengaruh (bus mengizinkan urutan bebas).

### 4.5 `backend/internal/infra/http` (package `httpx`), API publik yang dipakai module

```go
func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, r *http.Request, err error)
func StatusFor(code errs.Code) int // kode tak dikenal -> 500
func RegisterCodeStatus(code errs.Code, status int) // dipanggil wiring module saat start untuk kode baru; duplikat = panic
func DecodeJSON(r *http.Request, dst any) error // DisallowUnknownFields, body kosong -> ErrInvalid, MaxBytesError -> PAYLOAD_TOO_LARGE
```

Pemetaan `WriteError`:
1. `*errs.Error` -> status dari `StatusFor(Code)`, `message` dan `details` dari error.
2. Sentinel `errs.Err*` (via `errors.Is`) -> kode terkait dengan pesan generik.
3. `*http.MaxBytesError` -> 413 `PAYLOAD_TOO_LARGE`.
4. `context.DeadlineExceeded` -> 504 `TIMEOUT`.
5. Selain itu -> 500 `INTERNAL`, pesan `"terjadi kesalahan internal"`, detail asli hanya di log (level error, dengan `trace_id`).

Body selalu `{ "error": { "code", "message", "details" }, "trace_id" }`. `details` bernilai `null` bila kosong.

---

## 5. Kontrak bus dan konstanta endpoint

### 5.1 API `infra/bus`

```go
type Handler func(ctx context.Context, req any) (any, error)

var (
    ErrNotRegistered = errors.New("bus: endpoint not registered")
    ErrNilRegistry   = errors.New("bus: nil registry (wiring error)")
    ErrShuttingDown  = errors.New("bus: shutting down")
    ErrBadRequestType  = errors.New("bus: unexpected request type")
    ErrBadResponseType = errors.New("bus: unexpected response type")
)

func NewRegistry(log *slog.Logger) *Registry
func (r *Registry) Register(endpoint string, h Handler)               // duplikat atau h nil = panic
func (r *Registry) Call(ctx context.Context, endpoint string, req any) (any, error)
func (r *Registry) Publish(ctx context.Context, endpoint string, req any) error
func (r *Registry) Shutdown(ctx context.Context) error                // ctx deadline = BUS_SHUTDOWN_TIMEOUT

func CallTyped[T any](ctx context.Context, b *Registry, endpoint string, req any) (T, error)
func Expect[T any](req any) (T, error) // untuk handler provider (rule 6.9)
```

Perilaku:
- `Register`: thread-safe (`sync.RWMutex`). Panic dengan pesan `bus: endpoint "<e>" registered twice`.
- `Call`: sinkron, memakai ctx pemanggil apa adanya. Tidak terdaftar -> error membungkus `ErrNotRegistered` dengan nama endpoint. Panic di handler `Call` TIDAK di-recover bus (dibiarkan naik ke middleware recover di jalur HTTP).
- `Publish`: bila sedang shutdown -> `ErrShuttingDown`. Tidak terdaftar -> `ErrNotRegistered` segera. Selain itu `wg.Add(1)`, goroutine dengan `ctx2 := context.WithoutCancel(ctx)` (nilai reqctx otomatis ikut karena WithoutCancel mempertahankan values). Error handler di-log `level=error msg="bus publish failed" endpoint=... trace_id=...`. Panic di-recover, di-log dengan stack (`debug.Stack()`), proses tetap hidup. Return `nil`.
- `Shutdown`: set flag tolak Publish baru, tunggu `wg` atau `ctx.Done()`. Timeout -> return error membungkus `context.DeadlineExceeded` dan log jumlah goroutine yang belum selesai.
- `CallTyped`: `b == nil` -> `ErrNilRegistry`. Respons bukan `T` -> error membungkus `ErrBadResponseType` dengan `want=%T got=%T`. Error dari handler diteruskan apa adanya.
- `Expect`: `req` bukan `T` -> error membungkus `ErrBadRequestType`.

### 5.2 Kontrak yang didefinisikan T00 (provider T01)

`backend/lib/contracts/core/auth/resolvesession/resolvesession.go`:

```go
package resolvesession

type Request struct {
    Token string // nilai mentah cookie grimoire_session
}

type Response struct {
    Found              bool
    UserID             string
    Email              string
    Name               string
    Role               string // "super_admin" | "developer"
    MustChangePassword bool
}

const (
    RoleSuperAdmin = "super_admin"
    RoleDeveloper  = "developer"
)
```

Hanya import stdlib (rule 4.5). Sesi tidak valid, kedaluwarsa, atau user nonaktif dikembalikan sebagai `Found=false, err=nil` (rule 6.7). Error hanya untuk kegagalan infra.

### 5.3 Konsumsi di middleware session auth

```
cookie grimoire_session kosong/tidak ada  -> anonim, TIDAK memanggil bus
ada cookie -> bus.CallTyped[resolvesession.Response](ctx, b, constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: v})
   err != nil (termasuk ErrNotRegistered) -> 500 INTERNAL (log detail)
   Found == false -> anonim + Set-Cookie grimoire_session="" Max-Age=0 Path=/ HttpOnly SameSite=Lax (Secure bila COOKIE_SECURE)
   Found == true  -> reqctx.WithIdentity(...)
```

Konstanta `middleware.SessionCookieName = "grimoire_session"` di `infra/middleware`. Nilai token dibuat T01. `MustChangePassword` hanya diteruskan di `Identity`; penegakannya milik T01.

Konsekuensi selama T01 belum ada: request dengan cookie `grimoire_session` -> 500. Request tanpa cookie -> anonim. Ini disengaja (gagal tertutup) dan diuji dengan bus palsu.

### 5.4 Endpoint yang dicatat sebagai TODO (bukan milik T00)

- `core.auth.create-admin` (Call, provider T01, consumer `cmd/cli`): ditambahkan T01 beserta konstanta, kontrak, dan baris katalog. T00 hanya menulis catatan TODO di README katalog.
- Tidak ada aksi audit baru.

---

## 6. Migration

**Tidak ada file migrasi** di T00. Folder `backend/migrations/core/` dan `backend/migrations/project/` dibuat kosong (berisi `.keep`).

Runner membuat tabel jejaknya sendiri saat `Migrate` dipanggil (bukan file migrasi):

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    bigint      PRIMARY KEY,
    name       text        NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

Aturan runner (`backend/internal/infra/db/migrate.go`):
- Sumber: `backend/migrations/embed.go` (package `migrations`) dengan `//go:embed core project` dan `var FS embed.FS`. Runner hanya membaca `*.sql` dan mengabaikan `.keep`, jadi folder kosong tidak menggagalkan build. Runner menerima `fs.FS` sebagai parameter agar bisa dites dengan `fstest.MapFS`.
- Nama file wajib cocok `^\d{17}_[a-z0-9_]+\.sql$`. Tidak cocok -> error sebelum apa pun dieksekusi, menyebut path file.
- Versi = 17 digit pertama (`YYYYMMDDHHMMSSmmm`), unik global lintas folder. Duplikat -> error sebelum eksekusi, menyebut kedua path.
- Urutan eksekusi: versi naik, lintas folder.
- Seluruh `Migrate` memegang `pg_advisory_lock(hashtext('grimoire.migrate'))` pada satu `*sql.Conn` khusus, dilepas dengan `pg_advisory_unlock` di koneksi yang sama (defer).
- Per file: satu transaksi berisi isi file + `INSERT INTO schema_migrations(version, name) VALUES ($1,$2)`. Gagal -> rollback penuh, `Migrate` berhenti, error menyebut file. Rerun setelah diperbaiki berjalan normal.
- Versi yang sudah ada di `schema_migrations` dilewati. Forward-only, tanpa down, tanpa checksum.
- Versi di DB yang tidak ada di FS: log warning, tidak error (binary lama di DB baru).
- Konsekuensi untuk tiket lain: SQL yang tidak boleh di dalam transaksi (`CREATE INDEX CONCURRENTLY`) tidak didukung. Didokumentasikan di `docs/configuration.md` bagian Migrasi.

Signature:

```go
type MigrateResult struct{ Applied []string } // nama file yang dijalankan
func Migrate(ctx context.Context, d *DB, fsys fs.FS, log *slog.Logger) (MigrateResult, error)
```

---

## 7. Plan (langkah berurutan)

Urutan dependensi:

```
1 repo-skeleton-config
 ├── 2 bus ─────────────┐
 ├── 3 db-migrations ───┤
 │    └── 4 jobs-scheduler ───┐
 │   5 http-server (butuh 2, 1) ──┐
 │        └── 6 static-frontend-scaffold
 │                                 └── 7 cli-and-wiring (butuh 2..6)
 └── 8 architecture-guard (mulai sejak 1, final setelah 7)
```

Paralel yang aman: 2 dan 3; 4 dan 5; 8 berjalan di samping semuanya.

### Langkah 1 — `repo-skeleton-config`
1. Buat `backend/go.mod` (`module grimoire`, `go 1.23`), `.gitignore` (tambah `bin/`, `frontend/node_modules/`, `frontend/dist/`, `backend/internal/infra/static/dist/*` kecuali `index.html`, `.env`).
2. Buat folder: `backend/cmd/{http,cli,internal/server,internal/buildinfo}`, `backend/internal/{domain/{core,project,discovery},infra/{app,bus,db,http,middleware,config,static,logger,jobs},archtest}`, `backend/lib/{common/{reqctx,errs},constants,contracts,util}`, `backend/migrations/{core,project}` (dengan `.keep`), `api/modules/infra`.
3. Stub `backend/internal/domain/{core,project,discovery}/base.go`: `func Register(d app.Deps) {}` (no-op, berkomentar "diisi T01/T03/T07"). Domain boleh meng-import infra; arah sebaliknya dilarang.
4. `lib/common/reqctx`, `lib/common/errs` (bagian 4.1, 4.2).
5. `infra/config`: `Load(getenv func(string) string) (Config, error)`, validasi semua field dan gabungkan error dengan `errors.Join`, `Config.String()`/`LogValue()` menyamarkan `DatabaseURL` dan `SessionSecret`. Field dan default di bagian 9.
6. `infra/logger`: `New(cfg) *slog.Logger` dengan handler pembungkus yang menambah `trace_id`, `user_id`, `api_key_id` dari `reqctx` bila ada. Wajib memakai `logger.InfoContext(ctx, ...)` agar ctx terbaca.
7. `infra/app/deps.go` dibuat dengan field `Config` dan `Log` saja. Field `Ctx`, `DB`, `Bus`, `Jobs`, `API`, `MCP` ditambahkan oleh usecase yang membuat tipenya, lalu difinalkan di langkah 7.
8. `docker-compose.dev.yml` (service `db`: `postgres:16`, user/pass/db `grimoire`, port `5432`, volume bernama, healthcheck `pg_isready`). `.env.example`. `docs/configuration.md`. Kerangka `makefile` (target diisi bertahap).

### Langkah 2 — `bus`
1. `infra/bus/bus.go`, `typed.go` (bagian 5.1).
2. `lib/constants/bus.go`, `lib/contracts/core/auth/resolvesession/resolvesession.go` (bagian 4.3, 5.2).
3. `infra/bus/bus_test.go` (T00-AC3).

### Langkah 3 — `db-migrations`
1. `infra/db/db.go`: `Open(ctx, cfg) (*DB, error)` dengan pool (`DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`), ping saat open (timeout 5s). `type DB struct{ *sqlx.DB }`. `Ping(ctx) error`. `Close() error`.
2. `infra/db/tx.go`: `BeginTx(ctx, fn func(tx *sqlx.Tx) error) error` (commit bila nil; rollback bila error; rollback lalu `panic(p)` ulang bila panic). Juga `type Querier interface` (gabungan `sqlx.ExtContext` + `GetContext/SelectContext`) agar repository bisa menerima DB atau Tx.
3. `infra/db/migrate.go` dan `backend/migrations/embed.go` (bagian 6).
4. `infra/db/testdb.go` (build tag tidak perlu; nama file `testing.go` dengan fungsi `TestDB(t testing.TB) *DB` yang skip bila `TEST_DATABASE_URL` kosong, membuat schema unik `t_<random>` dan `search_path`-nya, drop saat cleanup). Dipakai semua test integrasi tiket lain.
5. Test integrasi `migrate_integration_test.go`, `tx_integration_test.go`, unit `migrate_test.go` (validasi nama/duplikat dengan `fstest.MapFS`, tanpa DB).

### Langkah 4 — `jobs-scheduler`
1. `infra/jobs/scheduler.go`: `type Job struct{ Name string; Every time.Duration; DailyAt string; Run func(ctx context.Context) error }` (tepat satu dari `Every`/`DailyAt` wajib diisi, selain itu panic saat `Register`). `New(d *db.DB, log *slog.Logger) *Scheduler`, `Register(Job)` (nama duplikat = panic, setelah `Start` = panic), `Start(ctx)`, `Stop(ctx) error`.
2. `infra/jobs/schedule.go`: `nextRun(now time.Time, j Job) time.Time` murni (UTC).
3. Eksekusi per tick: trace id baru `job-<uuid>` di ctx, `conn := db.Conn(ctx)`, `SELECT pg_try_advisory_lock(hashtext($1))` dengan `'grimoire.job.'||name`; false -> log debug lalu lewati; true -> `Run`, recover panic, `pg_advisory_unlock` di conn yang sama, `conn.Close()`. Log `job`, `duration_ms`, `status` (ok|error|panic|skipped).
4. Tick yang terlewat karena job sebelumnya masih berjalan dilewati (tidak menumpuk).

### Langkah 5 — `http-server`
1. `infra/http`: `errors.go` (bagian 4.5), `json.go`, `server.go` (`NewRouter(cfg config.Config, log *slog.Logger, b *bus.Registry, p Pinger) (root, api, mcp chi.Router)` dengan `type Pinger interface{ Ping(ctx context.Context) error }`, `Server{ *http.Server }` dengan `ReadHeaderTimeout`), `health.go` (`/healthz`, `/readyz`).
2. `infra/middleware`: `recover.go`, `traceid.go`, `accesslog.go`, `securityheaders.go`, `csrf.go`, `session.go`, `ratelimit.go`, `roleguard.go`, `bodylimit.go`, `timeout.go`, `origin.go`.
3. Rantai: root `Recover -> TraceID -> AccessLog -> SecurityHeaders -> BodyLimit(default)`. Grup `/api`: `CSRF -> Session(bus)` + `NotFound`/`MethodNotAllowed` JSON. Grup `/mcp`: tanpa CSRF dan tanpa Session (T07 memasang auth key sendiri). `/healthz`, `/readyz` di root. Per route (dipasang module): `Timeout(d)`, `BodyLimit(n)`, `RateLimit(limiter)`, `RequireAuth()`, `RequireRole(...)`.
4. Detail tiap middleware: bagian 8.
5. `api/modules/infra/foundation.yml` (healthz, readyz, `components/schemas/Error`, `components/responses/{BadRequest,Unauthorized,Forbidden,NotFound,Conflict,PayloadTooLarge,RateLimited,Internal}`).

### Langkah 6 — `static-frontend-scaffold`
1. `infra/static/static.go`: `//go:embed all:dist` + `var distFS embed.FS`; `New(cfg config.Config, fsys fs.FS, log) (http.Handler, error)`; `Embedded() fs.FS` (sub `dist`). Placeholder `infra/static/dist/index.html` di-commit.
2. Aturan handler di bagian 8.9. Mode dev: `httputil.ReverseProxy` ke `DEV_VITE_URL` (websocket HMR didukung bawaan ReverseProxy Go 1.12+ untuk `Upgrade`).
3. `frontend/`: scaffold Vite + Svelte 5 + TS + Tailwind v4 + Zod, struktur bagian 10.
4. `vite.config.ts`: `build.outDir = 'dist'`, `assetsDir = 'assets'`, dev `server.port = 5173`, `server.strictPort = true`. Di dev, browser membuka `http://localhost:8080` (Go mem-proxy Vite) agar cookie dan `/api` satu origin.

### Langkah 7 — `cli-and-wiring`
1. `cmd/internal/server/run.go`: `config -> logger -> db.Open -> (Migrate bila RUN_MIGRATIONS_ON_START) -> bus.NewRegistry -> jobs.New -> httpx.NewRouter -> app.Deps -> core/project/discovery.Register -> static.New dipasang sebagai NotFound root -> jobs.Start -> srv.ListenAndServe -> tunggu SIGINT/SIGTERM -> srv.Shutdown(SHUTDOWN_TIMEOUT) -> jobs.Stop -> bus.Shutdown(BUS_SHUTDOWN_TIMEOUT) -> db.Close`. Error di tiap langkah shutdown di-log dan dilanjutkan; exit code 1 bila ada yang gagal.
2. `cmd/http/main.go` dan `cmd/cli/main.go` (subcommand bagian 8.10).
3. `makefile` lengkap (bagian 11).
4. Finalisasi `infra/app.Deps` (bagian 4.4).

### Langkah 8 — `architecture-guard`
1. `internal/archtest/rules.go` (fungsi murni `Check`), `rules_test.go` (fixture), `repo_test.go` (memindai repo nyata).
2. `frontend/.dependency-cruiser.cjs`.
3. Tooling OpenAPI: `make api-bundle`, `make api-check`, `api/openapi.yml` hasil bundle (di-commit, header komentar "GENERATED").
4. `.github/workflows/ci.yml`.
5. Update `rules.txt` 3.1 dan `docs/tickets/README.md` (bagian 3.3, 12).

---

## 8. Spesifikasi perilaku infra (acuan plan per usecase)

### 8.1 Recover
Panic -> log error dengan stack + `trace_id`, respons 500 `INTERNAL` format seragam. Bila header sudah terkirim, hanya log. `http.ErrAbortHandler` diteruskan (panic ulang).

### 8.2 TraceID
`X-Request-Id` masuk dipakai bila cocok `^[A-Za-z0-9-]{1,64}$`, selain itu `uuid.NewString()`. Disimpan via `reqctx.WithTraceID`, dipantulkan di header respons `X-Request-Id`, dan muncul di `trace_id` body error.

### 8.3 AccessLog
Satu baris per request setelah selesai: `method`, `path` (tanpa query string), `route` (pola chi bila ada), `status`, `bytes`, `duration_ms`, `remote_ip` (dari `RemoteAddr`; `X-Forwarded-For` TIDAK dipercaya di T00), `trace_id`, `user_id`. Level: info (<500), error (>=500). `/healthz` dan `/readyz` dicatat di level debug.

### 8.4 SecurityHeaders
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: same-origin
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'
Strict-Transport-Security: max-age=31536000; includeSubDomains   (hanya bila COOKIE_SECURE=true)
```
Mode dev (proxy Vite): hanya bila `APP_ENV=development`, `connect-src` menjadi `'self' ws:` agar HMR jalan. `'unsafe-eval'` tidak ditambahkan.

### 8.5 CSRF (grup `/api`)
Method `POST`, `PUT`, `PATCH`, `DELETE` wajib header `X-Requested-With: grimoire` (perbandingan persis, case-sensitive pada nilai). Tidak ada atau salah -> 403 `CSRF_REQUIRED`, pesan `"header X-Requested-With wajib untuk request mutasi"`. `GET`, `HEAD`, `OPTIONS` lolos. `/mcp` tidak melewati middleware ini (dipasang hanya di grup `/api`). Cookie tetap `SameSite=Lax` (diset T01).

### 8.6 Session auth (grup `/api`)
Bagian 5.3. `RequireAuth()`: tidak ada `Identity` -> 401 `UNAUTHORIZED`. `RequireRole(roles ...string)`: tanpa identitas -> 401; role tidak termasuk -> 403 `FORBIDDEN`. `RequireRole` tanpa argumen = panic saat wiring.

### 8.7 RateLimit
```go
type Rule struct {
    Name    string                       // untuk log/metrik, mis. "auth.login"
    Rate    rate.Limit                   // token per detik
    Burst   int
    KeyFunc func(r *http.Request) string // "" = tidak dibatasi
}
func NewLimiter(ctx context.Context, rule Rule) *Limiter // map[key]*entry dengan lastSeen; janitor goroutine tiap 1 menit membuang entri idle > 10 menit, berhenti saat ctx (ctx server) selesai
func RateLimit(l *Limiter) func(http.Handler) http.Handler
func KeyByIP(r *http.Request) string     // host dari RemoteAddr
func KeyByUser(r *http.Request) string   // reqctx.UserID, fallback IP
```
Ditolak -> 429 `RATE_LIMITED`, header `Retry-After` (detik, dibulatkan ke atas, minimal 1). Aturan per route milik module. Module memakai ctx server yang tersedia di `app.Deps` lewat field tambahan `Ctx context.Context` (dibatalkan saat shutdown).

### 8.8 BodyLimit dan Timeout
- `BodyLimit(n int64)`: membungkus `r.Body` dengan `http.MaxBytesReader`. Default `HTTP_DEFAULT_BODY_BYTES` dipasang di root. Override per route dengan memanggil `BodyLimit(n)` lagi, dan yang terdalam menang. Caranya: middleware root menyimpan body asli di ctx, dan setiap `BodyLimit` membungkus body asli itu (bukan reader yang sudah dibatasi), sehingga override juga bisa menaikkan batas. Melebihi -> 413 `PAYLOAD_TOO_LARGE` lewat `WriteError`.
- `Timeout(d)`: `context.WithTimeout` pada request ctx (bukan `http.TimeoutHandler`). Default `HTTP_DEFAULT_TIMEOUT` dipasang di grup `/api`; override per route. Handler yang mengembalikan `context.DeadlineExceeded` -> 504 `TIMEOUT`.
- `http.Server`: `ReadHeaderTimeout = HTTP_READ_HEADER_TIMEOUT`, `IdleTimeout = 120s`. `WriteTimeout` tidak diset global (upload besar dan MCP diatur per route).

### 8.9 Static
- Path diawali `/api` atau `/mcp` tidak pernah dilayani di sini (dicek eksplisit, 404 JSON).
- File ada di FS -> sajikan (`http.FileServerFS` atau `http.ServeFileFS`), `Content-Type` dari ekstensi.
- `/assets/*` -> `Cache-Control: public, max-age=31536000, immutable`.
- `index.html` dan fallback -> `Cache-Control: no-cache`.
- Path tanpa ekstensi yang tidak ada -> `index.html` (200).
- Path berekstensi yang tidak ada -> 404 teks polos.
- Method selain GET/HEAD -> 405.
- `APP_ENV=development` dan `DEV_VITE_URL` terisi -> semua path non-`/api`/`/mcp` di-proxy ke Vite. Validasi config: `DEV_VITE_URL` terisi dengan `APP_ENV=production` -> error konfigurasi.

### 8.10 CLI
```
grimoire [serve]                      jalankan server (default)
grimoire migrate up                   jalankan migrasi lalu keluar; cetak nama file yang diterapkan
grimoire create-admin --email E --name N
grimoire version                      cetak "grimoire <version> (commit <commit>, built <buildDate>)"
grimoire help | -h | --help
```
- `create-admin`: `--email` dan `--name` wajib (kosong -> exit 2 dengan pesan usage). Sumber password berurutan: env `CREATE_ADMIN_PASSWORD`; bila stdin bukan TTY baca satu baris dari stdin; bila TTY prompt dua kali tanpa echo (`term.ReadPassword`) dan harus sama. Password kosong -> exit 2. Password tidak pernah dicetak atau di-log. Setelah argumen dan password valid, T00 mencetak ke stderr `create-admin belum tersedia (butuh T01)` dan exit 1, tanpa membuka DB. Fungsi `runCreateAdmin(ctx, args createAdminArgs) error` diberi komentar `TODO(T01)`. T01 mengganti isinya dengan merakit config, db, bus, dan `core.Register`, lalu memanggil `bus.Call(constants.BUS_CORE_AUTH_CREATE_ADMIN, ...)`.
- Subcommand tidak dikenal -> exit 2, usage ke stderr.
- Config hanya di-load untuk `serve`, `migrate up`, `create-admin`. `version` dan `help` tidak butuh env.

---

## 9. Konfigurasi (`infra/config`)

| Env | Field `Config` | Tipe | Default | Wajib | Validasi |
|---|---|---|---|---|---|
| `DATABASE_URL` | `DatabaseURL` | string | - | ya | tidak kosong, parse URL `postgres://`/`postgresql://` |
| `HTTP_ADDR` | `HTTPAddr` | string | `:8080` | tidak | `net.SplitHostPort` valid |
| `APP_ENV` | `AppEnv` | enum | `production` | tidak | `production`\|`development` |
| `SESSION_SECRET` | `SessionSecret` | string | - | ya | >= 32 byte |
| `COOKIE_SECURE` | `CookieSecure` | bool | `true` | tidak | `strconv.ParseBool` |
| `AUDIT_RETENTION_DAYS` | `AuditRetentionDays` | int | `90` | tidak | >= 1 |
| `MAX_FILE_BYTES` | `MaxFileBytes` | int64 | `1048576` | tidak | >= 1 |
| `MAX_UPLOAD_BYTES` | `MaxUploadBytes` | int64 | `52428800` | tidak | >= `MAX_FILE_BYTES` |
| `MAX_UPLOAD_FILES` | `MaxUploadFiles` | int | `500` | tidak | >= 1 |
| `MCP_MAX_RESPONSE_BYTES` | `MCPMaxResponseBytes` | int | `102400` | tidak | >= 1024 |
| `RUN_MIGRATIONS_ON_START` | `RunMigrationsOnStart` | bool | `false` | tidak | bool |
| `DB_MAX_OPEN_CONNS` | `DBMaxOpenConns` | int | `20` | tidak | >= 1 |
| `DB_MAX_IDLE_CONNS` | `DBMaxIdleConns` | int | `5` | tidak | >= 0 dan <= open |
| `DB_CONN_MAX_LIFETIME` | `DBConnMaxLifetime` | duration | `30m` | tidak | `time.ParseDuration`, > 0 |
| `HTTP_DEFAULT_BODY_BYTES` | `HTTPDefaultBodyBytes` | int64 | `1048576` | tidak | >= 1 |
| `HTTP_READ_HEADER_TIMEOUT` | `HTTPReadHeaderTimeout` | duration | `10s` | tidak | > 0 |
| `HTTP_DEFAULT_TIMEOUT` | `HTTPDefaultTimeout` | duration | `30s` | tidak | > 0 |
| `SHUTDOWN_TIMEOUT` | `ShutdownTimeout` | duration | `15s` | tidak | > 0 |
| `BUS_SHUTDOWN_TIMEOUT` | `BusShutdownTimeout` | duration | `5s` | tidak | > 0 |
| `LOG_LEVEL` | `LogLevel` | enum | `info` | tidak | `debug`\|`info`\|`warn`\|`error` |
| `LOG_FORMAT` | `LogFormat` | enum | `json` | tidak | `json`\|`text` |
| `DEV_VITE_URL` | `DevViteURL` | string | kosong | tidak | URL http(s) bila terisi; terisi hanya boleh saat `APP_ENV=development` |

- Env di luar Config yang tetap didokumentasikan di `docs/configuration.md` bagian terpisah: `CREATE_ADMIN_PASSWORD` (CLI), `TEST_DATABASE_URL` (test).
- Setiap field `Config` diberi struct tag `env:"NAME"`. Test T00-AC7 memakai refleksi atas tag ini dan memastikan setiap nama muncul di tabel `docs/configuration.md`.
- Pesan gagal start, contoh:
  ```
  konfigurasi tidak valid:
  - DATABASE_URL: wajib diisi
  - SESSION_SECRET: minimal 32 byte (sekarang 12)
  - LOG_LEVEL: nilai "verbose" tidak dikenal (debug|info|warn|error)
  ```
  Exit code 1.
- `Config` mengimplementasikan `slog.LogValuer` dan `fmt.Stringer`; `DatabaseURL` ditampilkan tanpa password (`postgres://grimoire:***@host/db`), `SessionSecret` sebagai `***`. Server mencetak config tersamarkan sekali saat start.

---

## 10. Frontend scaffold (ringkas, rinci di plan `static-frontend-scaffold`)

```
frontend/
  package.json, vite.config.ts, tsconfig.json, svelte.config.js, vitest.config.ts,
  playwright.config.ts, .dependency-cruiser.cjs, index.html
  src/
    main.ts
    app/
      App.svelte
      routes/        index.ts (tabel route: path, view, meta.roles?), router.ts, guard.ts
      layouts/       BlankLayout.svelte, AppLayout.svelte (kosong)
      views/         HomeView.svelte (halaman kosong + judul i18n), NotFoundView.svelte
    domain/{core,project,discovery}/.gitkeep
    shared/
      components/    Button, Input, Dialog, Toast (+ toast store), Table, Pagination (.svelte)
      services/      http.ts, http.test.ts
      stores/        session.ts (identity | null; diisi T01), i18n.ts (locale store, t())
      validation/    error.ts (skema Zod error seragam)
      utils/
      i18n/          id.json, en.json
  e2e/smoke.spec.ts
```

`shared/services/http.ts`:
- `request<T>(method, url, { body?, query?, schema?: ZodType<T>, signal? }): Promise<T>`; helper `get/post/put/patch/del`.
- `credentials: 'same-origin'`; `X-Requested-With: grimoire` hanya untuk POST/PUT/PATCH/DELETE; body objek -> JSON, `FormData` -> biarkan browser set boundary.
- Respons non-2xx -> `throw new ApiError({ status, code, message, details, traceId })` hasil parse skema error; bila body bukan format seragam -> `code: 'INTERNAL'` atau `'NETWORK'` untuk kegagalan fetch.
- 401 -> panggil `onUnauthorized` (default: `router.navigate('/login?next=<path>')`) lalu tetap throw. Pengecualian: request dengan opsi `{ skipAuthRedirect: true }` (dipakai T01 untuk login/me).
- `schema` terisi -> `schema.parse(json)`; gagal -> `ApiError` code `INVALID_RESPONSE`.
- Respons 204 -> `undefined`.

Router: history API, tabel route, `meta.roles?: ('super_admin'|'developer')[]`, `meta.auth?: boolean`. Guard: belum login dan route butuh auth -> `/login?next=`; role tidak cocok -> `NotFoundView` (konsisten dengan rule 7.3). Guard hanya UX; otorisasi tetap di backend (rule 12.7).

i18n: `locale` store (`id` default, simpan pilihan di `localStorage` dengan try/catch), `t(key, params?)`, kunci hilang -> kembalikan kunci dan `console.warn` di dev. Semua teks komponen dasar lewat `t`.

---

## 11. Makefile dan CI

| Target | Isi |
|---|---|
| `dev` | `docker compose -f docker-compose.dev.yml up -d db`, lalu paralel (`make -j2 dev-backend dev-frontend`): `cd backend && APP_ENV=development RUN_MIGRATIONS_ON_START=true COOKIE_SECURE=false DEV_VITE_URL=http://localhost:5173 go run ./cmd/http` dan `cd frontend && npm run dev`. Env lain dari `.env` (di-include bila ada) |
| `build` | `cd frontend && npm ci && npm run build`, salin `frontend/dist/.` ke `backend/internal/infra/static/dist/`, `cd backend && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X grimoire/cmd/internal/buildinfo.Version=$(VERSION) -X ...Commit=$(COMMIT) -X ...BuildDate=$(DATE)" -o ../bin/grimoire ./cmd/cli` |
| `test` | `cd backend && go test ./...` + `cd frontend && npm test -- --run` |
| `test-integration` | `docker compose ... up -d db`, `TEST_DATABASE_URL=... go test -tags integration ./...` (test integrasi diberi build tag `integration` dan juga skip bila env kosong) |
| `test-e2e` | `make build`, jalankan `bin/grimoire` di background, `npx playwright test` |
| `lint` | `go vet ./...`, `golangci-lint run`, `npm run check` (svelte-check + tsc), `npx depcruise src` |
| `migrate` | `cd backend && go run ./cmd/cli migrate up` |
| `api-bundle` | `npx @redocly/cli@<versi-pin> bundle api/root.yml -o api/openapi.yml` (`api/root.yml` berisi `info` + `$ref` ke `api/modules/**`) |
| `api-check` | bundle ke file sementara, `diff -u api/openapi.yml <tmp>`; beda -> exit 1 dengan pesan "jalankan make api-bundle" |

CI `.github/workflows/ci.yml` (on push/PR): job `backend` (setup-go, `go vet`, golangci-lint, `go test ./...`, test integrasi dengan service `postgres:16` dan `TEST_DATABASE_URL`, di CI env `CI=true` membuat test integrasi GAGAL bila `TEST_DATABASE_URL` kosong), job `frontend` (setup-node, `npm ci`, `npm run check`, depcruise, `npm test`), job `api-check`, job `build` (butuh dua job pertama, `make build`, unggah artefak `bin/grimoire`), job `e2e` (Playwright smoke T00-AC1 terhadap binary hasil build).

---

## 12. Changes (per file/folder)

Root:
- `.gitignore` (ubah): bin, node_modules, frontend/dist, static/dist kecuali placeholder, `.env`, `playwright-report`, `test-results`.
- `makefile` (baru).
- `docker-compose.dev.yml` (baru).
- `.env.example` (baru).
- `.github/workflows/ci.yml` (baru).
- `rules.txt` (ubah): 3.1 tambahkan `infra/app`, `internal/archtest`, `cmd/internal/{server,buildinfo}`; 3.3 sebut `api/root.yml`.

Dokumen:
- `docs/configuration.md` (baru): tabel bagian 9, env CLI/test, contoh `.env` dev dan produksi, catatan migrasi (transaksi per file, tanpa CONCURRENTLY), catatan `make dev`.
- `docs/tickets/README.md` (ubah): di katalog bus, catatan "konstanta dan kontrak `core.auth.resolve-session` didefinisikan T00, handler diimplementasi T01"; baris TODO `core.auth.create-admin` (Call, provider T01, consumer `cmd/cli`) bertanda "ditambah T01"; konvensi backend menambah folder bagian 3.3.

Backend:
- `backend/go.mod`, `backend/go.sum` (baru).
- `backend/cmd/http/main.go`, `backend/cmd/cli/main.go`, `backend/cmd/cli/main_test.go`, `backend/cmd/cli/password.go`, `backend/cmd/internal/server/run.go`, `backend/cmd/internal/server/run_integration_test.go`, `backend/cmd/internal/buildinfo/buildinfo.go` (baru).
- `backend/internal/domain/{core,project,discovery}/base.go` (baru, stub).
- `backend/internal/infra/app/deps.go`.
- `backend/internal/infra/bus/{bus.go,typed.go,bus_test.go}`.
- `backend/internal/infra/config/{config.go,config_test.go}`.
- `backend/internal/infra/logger/{logger.go,logger_test.go}`.
- `backend/internal/infra/db/{db.go,tx.go,migrate.go,testing.go,migrate_test.go,migrate_integration_test.go,tx_integration_test.go}`.
- `backend/internal/infra/jobs/{scheduler.go,schedule.go,scheduler_test.go,schedule_test.go,scheduler_integration_test.go}`.
- `backend/internal/infra/http/{server.go,errors.go,json.go,health.go,errors_test.go,health_test.go,server_test.go}`.
- `backend/internal/infra/middleware/{recover.go,traceid.go,accesslog.go,securityheaders.go,csrf.go,session.go,ratelimit.go,roleguard.go,bodylimit.go,timeout.go,origin.go}` + `*_test.go` masing-masing.
- `backend/internal/infra/static/{static.go,static_test.go,dist/index.html}`.
- `backend/internal/archtest/{rules.go,rules_test.go,repo_test.go}`.
- `backend/lib/common/reqctx/{reqctx.go,reqctx_test.go}`, `backend/lib/common/errs/{errs.go,errs_test.go}`.
- `backend/lib/constants/bus.go`.
- `backend/lib/contracts/core/auth/resolvesession/resolvesession.go`.
- `backend/lib/util/.gitkeep`.
- `backend/migrations/embed.go`, `backend/migrations/{core,project}/.keep`.
- `backend/.golangci.yml`.

API:
- `api/root.yml`, `api/modules/infra/foundation.yml`, `api/openapi.yml` (generated).

Frontend: seluruh isi bagian 10.

---

## 13. Test Scenario

Penamaan mengikuti rule 13.4. Test integrasi memakai `TEST_DATABASE_URL` (Postgres nyata, rule 13.3), skip dengan pesan `TEST_DATABASE_URL tidak di-set; lewati test integrasi` secara lokal, gagal bila `CI=true`.

### repo-skeleton-config
- `TestLoad_Defaults`: hanya `DATABASE_URL` + `SESSION_SECRET` -> semua default sesuai tabel 9.
- `TestLoad_ReportsAllErrors`: tiga env invalid sekaligus -> satu error berisi ketiganya.
- `TestLoad_InvalidValues`: tabel kasus (secret 31 byte, `APP_ENV=staging`, durasi `abc`, idle > open, `MAX_UPLOAD_BYTES` < `MAX_FILE_BYTES`, `DEV_VITE_URL` di production).
- `TestConfig_StringMasksSecrets`: `String()` dan output slog tidak memuat password DB maupun secret.
- `TestT00_AC7_AllConfigFieldsDocumented`: refleksi tag `env` -> setiap nama ada di `docs/configuration.md` beserta default.
- `TestLogger_IncludesTraceAndUser`: log dengan ctx berisi reqctx -> JSON memuat `trace_id`, `user_id`.
- `reqctx` round-trip dan `Carry`; `errs` `errors.Is`/`errors.As` melalui `Wrap`.

### bus (T00-AC3)
- `TestT00_AC3_RegisterDuplicatePanics`.
- `TestT00_AC3_RegisterOrderIndependent`: dua endpoint didaftar dalam urutan terbalik, hasil sama.
- `TestT00_AC3_CallUnregistered` -> `errors.Is(err, ErrNotRegistered)`.
- `TestCall_PassesThroughResponseAndError`.
- `TestT00_AC3_PublishCarriesContextValues`: trace_id, user_id, api_key_id terbaca di handler.
- `TestT00_AC3_PublishSurvivesParentCancel`: parent dibatalkan segera setelah Publish; handler (menunggu channel) masih melihat `ctx.Err()==nil` dan selesai.
- `TestT00_AC3_PublishPanicRecovered`: handler panic, proses hidup, log (buffer) memuat endpoint dan pesan panic; Publish berikutnya tetap jalan.
- `TestPublish_ErrorLogged`.
- `TestShutdown_WaitsForInflight` dan `TestShutdown_TimesOut` (handler diblok lebih lama dari deadline -> error deadline).
- `TestPublish_AfterShutdownRejected` -> `ErrShuttingDown`.
- `TestCallTyped_NilRegistry`, `TestCallTyped_WrongResponseType`, `TestExpect_WrongRequestType`.
- Jalankan dengan `-race`.

### db-migrations (T00-AC5)
- Unit (`fstest.MapFS`): nama invalid ditolak, versi duplikat lintas folder ditolak, `.keep` diabaikan, urutan lintas folder benar.
- `TestT00_AC5_MigrateTwiceNoEffect`: run 1 menerapkan 2 file, run 2 `Applied` kosong, isi tabel tidak berubah.
- `TestT00_AC5_FailedFileRollsBack`: file kedua berisi `CREATE TABLE b(...); SELECT 1/0;` -> error, tabel `b` tidak ada, versi tidak tercatat, file pertama tetap tercatat; perbaiki FS lalu rerun berhasil.
- `TestT00_AC5_ConcurrentRunners`: dua `Migrate` paralel -> keduanya sukses, setiap versi tercatat sekali.
- `TestBeginTx_Commit`, `TestBeginTx_RollbackOnError`, `TestBeginTx_RollbackAndRepanic`.
- `TestOpen_PoolSettings` dan `TestPing`.

### jobs-scheduler
- `TestNextRun_DailyAt` (sebelum, tepat, sesudah jam; lintas hari; selalu UTC).
- `TestNextRun_Every`.
- `TestRegister_DuplicatePanics`, `TestRegister_InvalidSchedulePanics`, `TestRegister_AfterStartPanics`.
- `TestJobPanicDoesNotStopScheduler` (scheduler dengan clock yang bisa diinjeksi atau `Every` kecil).
- `TestStop_WaitsRunningJob`.
- Integrasi `TestTwoSchedulers_OnlyOneRunsPerTick`: dua scheduler pada DB sama, job yang tidur 200ms menaikkan counter atomik; per tick counter naik satu.
- Integrasi `TestLockReleasedOnSameConn`: setelah job selesai, `pg_locks` tidak berisi advisory lock job.

### http-server (T00-AC6 dan lainnya)
- `TestT00_AC6_PostWithoutCSRFHeaderForbidden` (POST/PUT/PATCH/DELETE -> 403 `CSRF_REQUIRED`).
- `TestT00_AC6_GetPassesWithoutHeader`.
- `TestT00_AC6_MCPExempt` (POST `/mcp/x` tanpa header -> tidak 403 CSRF).
- `TestCSRF_WrongValueForbidden`.
- `TestTraceID_GeneratedAndEchoed`, `TestTraceID_ValidIncomingReused`, `TestTraceID_InvalidIncomingReplaced`, `TestErrorBody_ContainsTraceID`.
- `TestRecover_PanicReturns500JSON` (tanpa stack di body, stack di log).
- `TestWriteError_Mapping` (tabel: tiap `errs.Code`, sentinel, MaxBytesError, DeadlineExceeded, error tak dikenal -> 500 dengan pesan generik).
- `TestSession_NoCookieNoBusCall` (bus palsu menghitung panggilan = 0).
- `TestSession_FoundSetsIdentity`, `TestSession_NotFoundClearsCookie` (Set-Cookie Max-Age=0), `TestSession_BusErrorReturns500`, `TestSession_UnregisteredReturns500`.
- `TestRequireAuth_401`, `TestRequireRole_403`, `TestRequireRole_Allows`.
- `TestRateLimit_429WithRetryAfter`, `TestRateLimit_KeysIndependent`, `TestRateLimit_JanitorEvictsIdle`.
- `TestBodyLimit_413PayloadTooLarge`, `TestBodyLimit_RouteOverrideRaises`.
- `TestTimeout_504`.
- `TestAPI_NotFoundJSON`, `TestAPI_MethodNotAllowedJSON`.
- `TestHealthz_200`, `TestReadyz_200`, `TestReadyz_503WhenDBDown` (pinger palsu).
- `TestSecurityHeaders` (HSTS hanya bila COOKIE_SECURE=true).
- `TestMiddlewareOrder`: panic di dalam route tetap mendapat trace id dan security headers, dan access log mencatat status 500.
- `TestCheckOrigin` (origin kosong, cocok, tidak cocok).
- `TestAccessLog_NoQueryString`.

### static-frontend-scaffold
- Go `TestStatic_RootServesIndexNoCache`, `TestStatic_SPAFallback`, `TestStatic_AssetsImmutable`, `TestStatic_MissingAssetWithExtension404`, `TestStatic_NeverHandlesAPIOrMCP`, `TestStatic_DevProxy` (httptest server sebagai Vite palsu), `TestStatic_PostReturns405`.
- `TestT00_AC2_EmbeddedFSServes`: memakai `static.Embedded()` (placeholder yang di-commit atau hasil build) -> `/` dan `/some/route` 200 HTML, `/healthz` 200 melalui router lengkap.
- Vitest `http.test.ts`: header CSRF hanya pada mutasi; `credentials: 'same-origin'`; normalisasi error seragam ke `ApiError` dengan `traceId`; body non-JSON -> `INTERNAL`; fetch gagal -> `NETWORK`; 401 memicu redirect kecuali `skipAuthRedirect`; validasi Zod gagal -> `INVALID_RESPONSE`; 204 -> undefined.
- Vitest `guard.test.ts`: anonim ke route auth -> redirect login dengan `next`; role tidak cocok -> not found; role cocok -> lolos.
- Vitest `i18n.test.ts`: ganti locale mengubah `t`; kunci hilang mengembalikan kunci.

### cli-and-wiring
- `TestCLI_UnknownSubcommandExit2`, `TestCLI_DefaultIsServe` (parser saja).
- `TestCLI_Version` mencetak nilai buildinfo yang diset di test.
- `TestCreateAdmin_RequiresFlags`.
- `TestCreateAdmin_PasswordFromEnv`, `TestCreateAdmin_PasswordFromStdinNonTTY` (reader palsu), `TestCreateAdmin_EmptyPasswordRejected`.
- `TestCreateAdmin_NotAvailableUntilT01` -> exit 1, stderr berisi `create-admin belum tersedia (butuh T01)`, password tidak muncul di output.
- Integrasi `TestT00_AC2_ServeWithOnlyRequiredEnv`: `Run` dengan hanya `DATABASE_URL` (TEST_DATABASE_URL) + `SESSION_SECRET`, `HTTP_ADDR=127.0.0.1:0` -> `/` HTML, `/healthz` 200, `/readyz` 200.
- Integrasi `TestServe_GracefulShutdownWaitsInflight`: route test lambat (didaftarkan lewat hook test), kirim sinyal di tengah request -> request selesai 200, lalu Run kembali nil.
- Integrasi `TestServe_RunMigrationsOnStart`.
- Smoke skrip CI: `bin/grimoire version`, lalu `bin/grimoire` dengan dua env di direktori kosong (`cd $(mktemp -d)`) -> curl `/` dan `/healthz`.

### architecture-guard (T00-AC4)
- `TestT00_AC4_Rule41_DomainToDomain` (fixture `grimoire/internal/domain/project/...` -> `grimoire/internal/domain/core/...`).
- `TestT00_AC4_Rule42_ModuleToModuleSameDomain`.
- `TestT00_AC4_Rule43_FeatureToFeature` (boleh ke `base/`).
- `TestT00_AC4_Rule44_ConsumerImportsProvider` (tercakup oleh 4.1/4.2 karena provider selalu di module lain; test memastikan pesan menyebut 4.4).
- `TestT00_AC4_Rule45_ContractsImportInternalOrNonCommonLib`.
- `TestT00_AC4_Rule46_InfraImportsDomain`.
- `TestT00_AC4_Rule_LibImportsInternal`.
- `TestT00_AC4_AllowedImportsPass` (domain -> infra, domain -> lib, cmd -> semua, module -> base sendiri).
- `TestT00_AC4_RepoIsClean`: muat `grimoire/...` via `go/packages` (termasuk file test), `Check` -> nol pelanggaran.
- Format pesan: `rules.txt 4.2: grimoire/internal/domain/project/internal/knowledge/upload imports grimoire/internal/domain/project/internal/apikey/base (module -> module lain)`.
- Depcruise: fixture `frontend/test/depcruise-fixtures/` dengan file pelanggar, dijalankan dengan config yang sama di test npm (`npm run test:arch`) dan harus gagal; `src/` harus lulus.
- `api-check`: test shell di CI mengubah `api/openapi.yml` sementara lalu memastikan `make api-check` exit != 0 (langkah CI `api-check-selftest`).

---

## 14. E2E Scenarios

Positif:
1. **T00-AC1** `make dev`: Postgres sehat, backend di `:8080`, Vite di `:5173`; Playwright membuka `http://localhost:8080/` -> halaman kosong dengan judul dari i18n, tanpa error console. Mengubah file Svelte memicu HMR lewat proxy.
2. **T00-AC2** `make build` -> `bin/grimoire` disalin ke folder kosong, dijalankan dengan `DATABASE_URL` + `SESSION_SECRET` -> `GET /` 200 HTML, `GET /projects/abc` 200 HTML (fallback), `GET /assets/<hash>.js` 200 dengan `immutable`, `GET /healthz` 200.
3. `bin/grimoire migrate up` pada DB kosong -> exit 0, tabel `schema_migrations` ada; dijalankan lagi -> exit 0, "tidak ada migrasi baru".
4. `bin/grimoire version` -> versi, commit, tanggal build.
5. `GET /readyz` dengan DB hidup -> 200.
6. SIGTERM saat request sedang berjalan -> request selesai, proses keluar 0 dalam `SHUTDOWN_TIMEOUT`.

Negatif:
1. Start tanpa `DATABASE_URL` dan dengan `SESSION_SECRET` pendek -> exit 1, kedua error tercetak sekaligus, tidak ada secret di output.
2. **T00-AC6** `POST /api/anything` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
3. `GET /api/does-not-exist` -> 404 JSON `NOT_FOUND` dengan `trace_id`.
4. `DELETE /healthz` -> 405.
5. `GET /api/x` dengan cookie `grimoire_session=abc` sebelum T01 -> 500 `INTERNAL` dengan pesan generik.
6. DB dimatikan saat server jalan -> `/readyz` 503, `/healthz` tetap 200.
7. `GET /assets/missing.js` -> 404.
8. `bin/grimoire create-admin --email a@b.c --name Admin` (password via env) -> exit 1, `create-admin belum tersedia (butuh T01)`.
9. `bin/grimoire create-admin` tanpa flag -> exit 2 dengan usage.
10. `bin/grimoire frobnicate` -> exit 2 dengan usage.

---

## 15. Request/Response body

### `GET /healthz` -> 200
```json
{ "status": "ok" }
```

### `GET /readyz` -> 200
```json
{ "status": "ok" }
```

### `GET /readyz` -> 503
```json
{ "status": "unavailable" }
```
(Detail error DB hanya di log dengan `trace_id`.)

### Format error seragam (semua error `/api`)
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

### 403 CSRF (`POST /api/projects` tanpa header)
```json
{
  "error": {
    "code": "CSRF_REQUIRED",
    "message": "header X-Requested-With wajib untuk request mutasi",
    "details": null
  },
  "trace_id": "0b1c..."
}
```

### 401 (`RequireAuth` tanpa sesi)
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

### 403 (`RequireRole("super_admin")` oleh developer)
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

### 404 dan 405 di `/api`
```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "..." }
```
```json
{ "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null }, "trace_id": "..." }
```
(405 menyertakan header `Allow`.)

### 413
```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran body melebihi batas", "details": { "limit_bytes": 1048576 } }, "trace_id": "..." }
```

### 429 (header `Retry-After: 3`)
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } }, "trace_id": "..." }
```

### 504
```json
{ "error": { "code": "TIMEOUT", "message": "permintaan melebihi batas waktu", "details": null }, "trace_id": "..." }
```

### 500 (panic, error tak dikenal, bus gagal)
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

### Kontrak bus `core.auth.resolve-session` (Go, bukan JSON di wire)
```go
resolvesession.Request{Token: "k3J9...raw-cookie-value"}
resolvesession.Response{Found: true, UserID: "0b7c...", Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: false}
resolvesession.Response{Found: false}
```

### Skema OpenAPI bersama (`api/modules/infra/foundation.yml`, potongan)
```yaml
components:
  schemas:
    Error:
      type: object
      required: [error, trace_id]
      properties:
        error:
          type: object
          required: [code, message, details]
          properties:
            code: { type: string, example: NOT_FOUND }
            message: { type: string }
            details:
              description: Data tambahan yang aman ditampilkan, atau null
        trace_id: { type: string }
```
(OpenAPI 3.1. `details` sengaja tanpa `type` agar menerima objek, array, atau `null`.)

### Log baris access (JSON, contoh)
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"http request","method":"POST","path":"/api/projects","route":"/api/projects","status":403,"bytes":118,"duration_ms":0.4,"remote_ip":"10.0.0.5","trace_id":"0b1c...","user_id":""}
```

---

## 16. Daftar usecase (plan rinci ditulis terpisah)

| # | Slug | Package/area utama | Bergantung | AC |
|---|---|---|---|---|
| 1 | `repo-skeleton-config` | go.mod, folder, stub domain, `lib/common/{reqctx,errs}`, `infra/{app,config,logger}`, compose, `.env.example`, makefile kerangka, `docs/configuration.md` | - | AC7 |
| 2 | `bus` | `infra/bus`, `lib/constants/bus.go`, `lib/contracts/core/auth/resolvesession` | 1 | AC3 |
| 3 | `db-migrations` | `infra/db`, `backend/migrations/embed.go` | 1 | AC5 |
| 4 | `jobs-scheduler` | `infra/jobs` | 1, 3 | - |
| 5 | `http-server` | `infra/http`, `infra/middleware`, `api/modules/infra/foundation.yml` | 1, 2 (3 untuk readyz lewat interface `Pinger`) | AC6 |
| 6 | `static-frontend-scaffold` | `infra/static`, `frontend/**` | 5 | AC1, AC2 (unit) |
| 7 | `cli-and-wiring` | `cmd/http`, `cmd/cli`, `cmd/internal/{server,buildinfo}`, makefile final | 2-6 | AC2 (binary) |
| 8 | `architecture-guard` | `internal/archtest`, `.dependency-cruiser.cjs`, `api/root.yml` + bundle/check, CI, update rules/README | 1 (final setelah 7) | AC4 |

Catatan untuk penulis plan usecase:
- Jangan memecah usecase lebih kecil secara artifisial. Satu usecase = satu PR yang koheren.
- `http-server` bergantung ke `db` hanya lewat interface `type Pinger interface{ Ping(ctx context.Context) error }` agar bisa dikerjakan paralel dengan `db-migrations`.
- Setiap usecase yang menambah env var WAJIB memperbarui tabel bagian 9 dan `docs/configuration.md` (test AC7 akan gagal bila tidak).
- Infra tidak boleh meng-import domain; archtest akan menegakkannya sejak langkah 8 aktif.
