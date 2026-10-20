# T00 Foundation — Usecase `http-server`

Tiket: `docs/tickets/T00-foundation.md` (F4, F11, bagian dari F9 untuk OpenAPI shared). Overview: `docs/superpowers/plans/foundation/overview.md` (keputusan di sana mengikat, kecuali yang ditandai "Penyimpangan" di bagian 3 dokumen ini).

Konteks dicek: belum ada `CLAUDE.md`, belum ada `.claude/rules/`, belum ada kode `backend/`. Usecase ini bergantung pada usecase 1 (`repo-skeleton-config`: `go.mod`, `lib/common/{reqctx,errs}`, `infra/{config,logger,app}`) dan usecase 2 (`bus`: `infra/bus`, `lib/constants/bus.go`, `lib/contracts/core/auth/resolvesession`). Tidak bergantung pada `infra/db`; readyz memakai interface `Pinger`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | `http-server` (urutan 5 dari 8) |
| Scope | backend Go + kontrak OpenAPI (tanpa perubahan frontend) |
| Domain | infra |
| Module | foundation |
| Resource root | `backend/internal/infra` (`http`, `middleware`) |

| Endpoint | Method | Version | Auth | Keterangan |
|---|---|---|---|---|
| `/healthz` | GET, HEAD | - (tanpa prefix versi) | publik | Selalu 200 `{"status":"ok"}` bila proses menjawab |
| `/readyz` | GET, HEAD | - | publik | `Pinger.Ping` timeout 2s: 200 `{"status":"ok"}` atau 503 `{"status":"unavailable"}` |
| `/api/**` tidak cocok route | semua | - | - | 404 `NOT_FOUND` atau 405 `METHOD_NOT_ALLOWED` (+ header `Allow`), format error seragam |
| `/mcp/**` tanpa route | semua | - | - | 404 JSON `NOT_FOUND` (route diisi T07). Tanpa CSRF dan tanpa session |
| Root tidak cocok (di luar `/api`, `/mcp`) | semua | - | - | Default 404 JSON; `Run` (usecase 7) menimpanya dengan handler static (usecase 6) |

Bus: usecase ini hanya CONSUMER `core.auth.resolve-session` (provider T01). Tidak ada endpoint bus baru, tidak ada aksi audit, tidak ada env var baru.

---

## 2. Summary Plan

Membangun `infra/http` (package `httpx`) dan `infra/middleware` (package `middleware`): router chi dengan rantai root `Recover -> TraceID -> AccessLog -> SecurityHeaders -> BodyLimit(default)`, grup `/api` (`Timeout(default) -> CSRF -> Session(bus)`, 404/405 JSON), grup `/mcp` (tanpa CSRF dan session), `/healthz` dan `/readyz`. Menyediakan blok bangunan yang dipakai module per route: `RateLimit` (keyed, `x/time/rate`), `RequireAuth`, `RequireRole`, `BodyLimit`, `Timeout`, dan helper `CheckOrigin` untuk `/mcp` (dipakai T07). Format error seragam lewat `WriteError`/`WriteJSON` dengan pemetaan `errs` ke status dan 500 generik tanpa bocor detail. Menyediakan `Serve` dan `RunShutdown` untuk graceful shutdown berurutan (`srv.Shutdown -> jobs.Stop -> bus.Shutdown -> db.Close`) yang dirakit `cmd/internal/server.Run` di usecase 7. Menambah `api/modules/infra/foundation.yml`.

Tidak ada entity, usecase bisnis, repository, atau SQL. Pola `init/handler/params/usecase/repository` tidak berlaku (T00 adalah infra). Usecase tidak dipecah.

---

## 3. Keputusan khusus usecase ini

### D1. Penyimpangan: paket `respond` untuk memutus siklus import
`httpx.NewRouter` mengimpor `middleware`, sedangkan `middleware` butuh menulis error JSON. Jika `middleware` mengimpor `httpx` terjadi siklus. Solusi: implementasi primitif error/JSON ditaruh di sub-package leaf `backend/internal/infra/http/respond` (package `respond`, hanya mengimpor `lib/common/{errs,reqctx}` dan stdlib). `httpx` mengekspos fungsi pembungkus tipis dengan nama dan signature sesuai overview 4.5 (`WriteJSON`, `WriteError`, `StatusFor`, `RegisterCodeStatus`, `DecodeJSON`). `middleware` mengimpor `respond`; module mengimpor `httpx`. Sub-folder ini berada di dalam `infra/http`, jadi tidak menambah folder top-level di luar rules 3.1 dan tidak perlu update `rules.txt`.

### D2. Log dari `WriteError`
`WriteError` tidak menerima logger. Ia memakai `slog.Default()` (`slog.ErrorContext(r.Context(), ...)`), sehingga `trace_id` ikut lewat handler logger usecase 1. `cmd/internal/server.Run` WAJIB memanggil `slog.SetDefault(log)` (dicatat sebagai handoff ke usecase 7). Test men-set default dengan buffer dan memulihkannya di `t.Cleanup` (test ini tidak boleh `t.Parallel()`).

### D3. Trace id untuk Recover yang paling luar
`Recover` berada di luar `TraceID`, jadi ctx belum berisi trace id. `TraceID` sudah menyetel header respons `X-Request-Id` sebelum memanggil next; `Recover` membaca `w.Header().Get("X-Request-Id")` sebagai trace id (fallback `reqctx.TraceID`, lalu kosong). `respond.WriteError` memakai urutan yang sama: `reqctx.TraceID(ctx)`, lalu header respons.

### D4. Access log tahu status 500 saat panic dan user_id
- `AccessLog` berada di dalam `Recover`. Ia memakai `defer` yang menangkap panic (`recover`), mencatat baris log dengan `status=500`, lalu `panic(rec)` ulang agar `Recover` menulis respons. `http.ErrAbortHandler` ikut diteruskan.
- `user_id` ditetapkan middleware `Session` yang lebih dalam. `AccessLog` menaruh pointer `*logFields` di ctx (kunci privat package `middleware`); `Session` mengisi `logFields.UserID`. Dibaca `AccessLog` setelah handler selesai.
- Wrapper writer bersama `statusWriter` (status, bytes, `wroteHeader`, `Unwrap()` untuk `http.ResponseController`, `Flush()`). `Recover` membuatnya bila belum ada; `AccessLog` memakai ulang bila `w` sudah `*statusWriter`.

### D5. BodyLimit: batas terdalam menang, termasuk menaikkan
Hanya pembatasan lazy (`http.MaxBytesReader`), TANPA penolakan awal berdasar `Content-Length` (penolakan awal di tingkat root akan menggagalkan route upload yang menaikkan batas). `BodyLimit(n)` menyimpan `r.Body` asli di ctx (kunci privat) sekali, lalu setiap `BodyLimit` membungkus body asli itu. Handler yang membaca body lewat `DecodeJSON` atau `io.ReadAll` menerima `*http.MaxBytesError`, dipetakan `WriteError` ke 413 dengan `details.limit_bytes = MaxBytesError.Limit`.

### D6. Konsekuensi chi: middleware grup berjalan juga untuk 404/405
Di chi, middleware `Use` sebuah router ikut membungkus handler NotFound/MethodNotAllowed router itu. Akibatnya `POST /api/tidak-ada` tanpa header CSRF mendapat 403 `CSRF_REQUIRED` (bukan 404), dan cookie sesi tetap di-resolve untuk request 404. Ini diterima dan didokumentasikan di test (`TestAPI_MutationOn404StillCSRFChecked`).

### D7. Header `Allow` untuk 405
Handler MethodNotAllowed kustom chi v5.1.0 tidak menerima daftar method. Helper `allowedMethods(r)` menghitung sendiri: untuk tiap method di `[GET HEAD POST PUT PATCH DELETE OPTIONS]` panggil `chi.RouteContext(r.Context()).Routes.Match(chi.NewRouteContext(), m, r.URL.Path)`; method yang cocok digabung dengan `, ` ke header `Allow`. Test memverifikasi `GET, HEAD` untuk `DELETE /healthz`. Bila `Match` pada router yang di-mount ternyata tidak mengenali path penuh, fallback: hitung dari router `api` yang disimpan di closure. Developer wajib menjalankan test ini lebih dulu (langkah 8) sebelum mengandalkan pendekatan.

### D8. Pemakaian konfigurasi (semua sudah ada di tabel overview bagian 9)
`CookieSecure`, `AppEnv`, `HTTPDefaultBodyBytes`, `HTTPDefaultTimeout`, `HTTPReadHeaderTimeout`, `ShutdownTimeout`, `HTTPAddr`. Tidak ada env baru, jadi `docs/configuration.md` dan test AC7 tidak berubah.

---

## 4. Plan (langkah berurutan)

Setiap langkah diakhiri `go build ./... && go vet ./...` bersih dan test paket terkait hijau (`-race`). Tulis test lebih dulu bila memungkinkan.

### Langkah 0 — Dependensi
`cd backend && go get github.com/go-chi/chi/v5@v5.1.0 golang.org/x/time@v0.6.0 github.com/google/uuid@v1.6.0 && go mod tidy`. Mengubah `go.mod`/`go.sum` yang dibuat usecase 1 (hanya menambah require). Bila usecase lain sudah menambahkan versi yang sama, tidak ada konflik; bila versi berbeda, pakai yang lebih baru.

### Langkah 1 — `app.Deps` (tambahan)
Edit `backend/internal/infra/app/deps.go`, tambahkan field (hanya menambah): `Ctx context.Context`, `API chi.Router`, `MCP chi.Router` dengan komentar sesuai overview 4.4 (API: csrf + session auth sudah terpasang; MCP: tanpa csrf dan tanpa session cookie). Field `DB`, `Bus`, `Jobs` ditambahkan usecase 2/3/4 atau difinalkan usecase 7. Paket `app` mengimpor `chi` saja untuk field ini.

### Langkah 2 — `infra/http/respond` (leaf)
File:
- `backend/internal/infra/http/respond/status.go`
  - `var codeStatus = map[errs.Code]int{...}` sesuai tabel kode di overview 4.2 (NOT_FOUND 404, FORBIDDEN 403, CONFLICT 409, INVALID_ARGUMENT 400, UNAUTHORIZED 401, CSRF_REQUIRED 403, RATE_LIMITED 429, PAYLOAD_TOO_LARGE 413, METHOD_NOT_ALLOWED 405, TIMEOUT 504, UNAVAILABLE 503, INTERNAL 500) dengan `sync.RWMutex`.
  - `StatusFor(code errs.Code) int` (tak dikenal -> 500).
  - `RegisterCodeStatus(code errs.Code, status int)`: panic bila `code` sudah ada (termasuk kode bawaan) atau `status` di luar 400..599.
- `backend/internal/infra/http/respond/respond.go`
  - `type body struct { Error errBody \`json:"error"\`; TraceID string \`json:"trace_id"\` }`, `type errBody struct { Code errs.Code \`json:"code"\`; Message string \`json:"message"\`; Details any \`json:"details"\` }` (`details` selalu ada, `null` bila kosong).
  - `WriteJSON(w, status, v)`: set `Content-Type: application/json; charset=utf-8`, `WriteHeader(status)`, `json.NewEncoder(w).Encode(v)`. Gagal encode di-log (error), tidak mencoba menulis ulang. Status 204/304: tanpa body.
  - `WriteError(w, r, err)` dengan urutan pemetaan:
    1. `errors.As(err, &*errs.Error)`: status `StatusFor(Code)`, `Message`, `Details`. Untuk `CodeInternal` pesan SELALU diganti `"terjadi kesalahan internal"` dan details nil.
    2. Sentinel via `errors.Is`: `ErrNotFound` -> NOT_FOUND "resource tidak ditemukan"; `ErrForbidden` -> FORBIDDEN "akses ditolak"; `ErrConflict` -> CONFLICT "konflik data"; `ErrInvalid` -> INVALID_ARGUMENT "permintaan tidak valid"; `ErrUnauthorized` -> UNAUTHORIZED "perlu login".
    3. `errors.As(err, &*http.MaxBytesError)` -> 413 PAYLOAD_TOO_LARGE "ukuran body melebihi batas", details `{"limit_bytes": <Limit>}`.
    4. `errors.Is(err, context.DeadlineExceeded)` -> 504 TIMEOUT "permintaan melebihi batas waktu".
    5. lainnya (termasuk `context.Canceled`, log level warn untuk Canceled) -> 500 INTERNAL generik.
    - Untuk status >= 500: `slog.ErrorContext(r.Context(), "request failed", "error", err, "status", status)` (detail asli hanya di log). Status < 500 tidak di-log di sini (sudah di access log).
    - Trace id: `reqctx.TraceID(r.Context())`, fallback `w.Header().Get("X-Request-Id")`.
    - Pesan error tidak pernah berasal dari `err.Error()` kecuali `errs.Error.Message` non-internal.
  - `DecodeJSON(r, dst) error`: `json.NewDecoder(r.Body)` + `DisallowUnknownFields()`. Pemetaan: `io.EOF` (body kosong) -> `errs.New(CodeInvalid, "body wajib diisi")`; `*json.SyntaxError`/`io.ErrUnexpectedEOF` -> `errs.New(CodeInvalid, "JSON tidak valid")`; `*json.UnmarshalTypeError` -> CodeInvalid dengan details `{"field": e.Field}`; error unknown field (`json: unknown field "x"`) -> CodeInvalid pesan `"field tidak dikenal"` dengan details `{"field": "x"}` (parse nama dari pesan error stdlib, hati-hati format); data sisa setelah objek pertama (`dec.More()` atau `dec.Decode(&struct{}{})` bukan EOF) -> CodeInvalid `"JSON tidak valid"`; `*http.MaxBytesError` dikembalikan apa adanya (dibungkus `%w`) agar `WriteError` memetakannya.
- `backend/internal/infra/http/respond/respond_test.go`, `status_test.go`.

### Langkah 3 — `httpx` pembungkus dan helper
- `backend/internal/infra/http/errors.go`: `WriteError`, `StatusFor`, `RegisterCodeStatus` meneruskan ke `respond`.
- `backend/internal/infra/http/json.go`: `WriteJSON`, `DecodeJSON` meneruskan ke `respond`.

### Langkah 4 — Middleware dasar (urut sesuai rantai)
Semua di `backend/internal/infra/middleware/`, tanda tangan `func(http.Handler) http.Handler` (konstruktor mengembalikannya).
1. `writer.go`: `statusWriter` (D4).
2. `recover.go`: `Recover(log *slog.Logger)`. `defer`: `rec := recover()`; `rec == http.ErrAbortHandler` -> panic ulang; selain itu log error `"panic recovered"` dengan `panic`, `stack` (`debug.Stack()`), `trace_id` (D3), `method`, `path`. Bila `!sw.wroteHeader` -> `respond.WriteError(w, r, errs.New(CodeInternal, ""))`; selain itu hanya log.
3. `traceid.go`: `TraceID()`. Regex `^[A-Za-z0-9-]{1,64}$` pada header `X-Request-Id` masuk; invalid atau kosong -> `uuid.NewString()`. `w.Header().Set("X-Request-Id", id)`; `ctx = reqctx.WithTraceID(ctx, id)`.
4. `accesslog.go`: `AccessLog(log *slog.Logger)` sesuai D4. Field: `method`, `path` (`r.URL.Path`, tanpa query), `route` (`chi.RouteContext(r.Context()).RoutePattern()` setelah handler, kosong bila tidak ada), `status`, `bytes`, `duration_ms` (float), `remote_ip` (host dari `RemoteAddr`; `X-Forwarded-For` TIDAK dipercaya), `trace_id`, `user_id`. Msg `"http request"`. Level: info (< 500), error (>= 500). `r.URL.Path` sama dengan `/healthz` atau `/readyz` -> level debug. Memakai `log.LogAttrs(ctx, level, ...)`.
5. `securityheaders.go`: `SecurityHeaders(cfg config.Config)`. Header sesuai overview 8.4: `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, CSP; HSTS hanya `cfg.CookieSecure`; bila `cfg.AppEnv == "development"` `connect-src 'self' ws:`. Header diset SEBELUM `next` (supaya tetap ada pada respons 500 hasil Recover).
6. `bodylimit.go`: `BodyLimit(n int64)` (panic bila n <= 0) sesuai D5.
7. `timeout.go`: `Timeout(d time.Duration)` (panic bila d <= 0): `context.WithTimeout(r.Context(), d)`, `defer cancel()`, `next.ServeHTTP(w, r.WithContext(ctx))`. Tidak menulis respons sendiri (handler yang menerima `DeadlineExceeded` memanggil `WriteError` -> 504).
8. `csrf.go`: `CSRF()`. Method `POST|PUT|PATCH|DELETE` wajib `r.Header.Get("X-Requested-With") == "grimoire"` (persis). Gagal -> `respond.WriteError` dengan `errs.New(CodeCSRFRequired, "header X-Requested-With wajib untuk request mutasi")`. Konstanta `CSRFHeader = "X-Requested-With"`, `CSRFValue = "grimoire"`.
9. `session.go`: `const SessionCookieName = "grimoire_session"`. `Session(b *bus.Registry, cookieSecure bool)`:
   - `c, err := r.Cookie(SessionCookieName)`; err atau `c.Value == ""` -> lanjut anonim, TIDAK memanggil bus.
   - Ada nilai: `bus.CallTyped[resolvesession.Response](ctx, b, constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: c.Value})`.
   - `err != nil` (termasuk `ErrNotRegistered`, `ErrNilRegistry`) -> `slog.ErrorContext` detail, `respond.WriteError(w, r, errs.New(CodeInternal, ""))` (500 generik). Cookie TIDAK dihapus.
   - `Found == false` -> anonim + `http.SetCookie` (`Name`, `Value:""`, `Path:"/"`, `MaxAge:-1` sehingga header `Max-Age=0`, `HttpOnly`, `SameSite=Lax`, `Secure: cookieSecure`).
   - `Found == true` dengan `UserID == ""` -> perlakukan sebagai pelanggaran kontrak: 500 generik + log.
   - `Found == true` -> `reqctx.WithIdentity(ctx, reqctx.Identity{UserID, Email, Name, Role, MustChangePassword})`, isi `logFields.UserID` (D4).
10. `roleguard.go`: `RequireAuth()` (tanpa `Identity` -> `errs.New(CodeUnauthorized, "perlu login")`); `RequireRole(roles ...string)` (panic saat konstruksi bila `len(roles)==0`; tanpa identitas -> 401; role tidak termasuk -> `errs.New(CodeForbidden, "akses ditolak")`).
11. `ratelimit.go` sesuai overview 8.7:
    - `type Rule struct { Name string; Rate rate.Limit; Burst int; KeyFunc func(*http.Request) string }`. `NewLimiter` panic bila `Rate <= 0`, `Burst < 1`, atau `KeyFunc == nil`.
    - `NewLimiter(ctx, rule) *Limiter`: `map[string]*entry{lim *rate.Limiter; lastSeen time.Time}` + mutex. Field unexported `now func() time.Time`, `janitorEvery` (default 1m), `idleTTL` (default 10m) agar bisa dioverride di test dalam package. Goroutine janitor membuang entri idle dan berhenti saat `ctx.Done()`.
    - `(*Limiter) Allow(key string) (ok bool, retryAfter time.Duration)`: `res := lim.Reserve()`; `!res.OK()` atau `res.Delay() > 0` -> `res.Cancel()`, `ok=false`, `retryAfter=res.Delay()` (bila `!res.OK()`, pakai `1/Rate` detik).
    - `RateLimit(l *Limiter)`: key `""` -> lolos tanpa batas. Ditolak -> header `Retry-After: <detik, ceil, min 1>`, `errs.WithDetails(CodeRateLimited, "terlalu banyak permintaan, coba lagi nanti", map[string]int{"retry_after_seconds": n})`.
    - `KeyByIP(r)`: `net.SplitHostPort(r.RemoteAddr)` host, fallback `RemoteAddr`. `KeyByUser(r)`: `reqctx.UserID(r.Context())`, fallback `KeyByIP`.
12. `origin.go`: `CheckOrigin(r *http.Request, allowed []string) bool` dan middleware `OriginGuard(allowed []string)`.
    - Header `Origin` kosong -> true (klien non-browser/MCP client).
    - `Origin: null` -> false.
    - Parse URL origin; true bila host (termasuk port) sama dengan `r.Host` atau cocok persis dengan salah satu entri `allowed` (entri berbentuk `scheme://host[:port]`, case-insensitive). Selain itu false.
    - `OriginGuard` gagal -> `errs.New(CodeForbidden, "origin tidak diizinkan")`. T00 tidak memasangnya di `/mcp`; T07 yang memutuskan daftar `allowed`.

### Langkah 5 — Router
`backend/internal/infra/http/server.go`:
```go
type Pinger interface{ Ping(ctx context.Context) error }

func NewRouter(cfg config.Config, log *slog.Logger, b *bus.Registry, p Pinger) (root, api, mcp chi.Router)
```
Perakitan:
1. `root := chi.NewRouter()`; `root.Use(Recover(log), TraceID(), AccessLog(log), SecurityHeaders(cfg), BodyLimit(cfg.HTTPDefaultBodyBytes))`.
2. `root.Get("/healthz", healthz)`, `root.Head(...)`, `root.Get("/readyz", readyz(p))`, `root.Head(...)`.
3. `api := chi.NewRouter()`; `api.Use(Timeout(cfg.HTTPDefaultTimeout), CSRF(), Session(b, cfg.CookieSecure))`; `api.NotFound(notFound)`; `api.MethodNotAllowed(methodNotAllowed)`; `root.Mount("/api", api)`.
4. `mcp := chi.NewRouter()`; tanpa CSRF/Session; `mcp.NotFound(notFound)`, `mcp.MethodNotAllowed(...)`; `root.Mount("/mcp", mcp)`.
5. `root.NotFound(notFound)` default JSON dan `root.MethodNotAllowed(methodNotAllowed)` (JSON). `cmd/internal/server.Run` menimpa `root.NotFound` dengan handler static. Karena `/api` dan `/mcp` di-mount, static tidak pernah menerimanya.
6. Mengembalikan `root, api, mcp`. Rute `/healthz` tidak melewati `Timeout` default `/api`.

`notFound` -> `respond.WriteError(..., errs.New(CodeNotFound, "resource tidak ditemukan"))`. `methodNotAllowed` -> set `Allow` (D7), lalu `errs.New(CodeMethodNotAllowed, "method tidak diizinkan")`.

`backend/internal/infra/http/health.go`: `healthz` menulis 200 `{"status":"ok"}`. `readyz(p Pinger)`: `ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)`; `p == nil` -> 503; `p.Ping(ctx)` error -> `slog.WarnContext` detail, 503 `{"status":"unavailable"}`; sukses 200 `{"status":"ok"}`. Body tidak memakai format error (sesuai overview 15).

### Langkah 6 — Server dan graceful shutdown
`backend/internal/infra/http/server.go` (lanjutan) dan `backend/internal/infra/http/graceful.go`:
```go
type Server struct{ *http.Server }

func NewServer(cfg config.Config, log *slog.Logger, h http.Handler) *Server
// Addr=cfg.HTTPAddr, ReadHeaderTimeout=cfg.HTTPReadHeaderTimeout, IdleTimeout=120s,
// WriteTimeout TIDAK diset, ErrorLog=slog.NewLogLogger(log.Handler(), slog.LevelError).

func Serve(ctx context.Context, s *Server, ln net.Listener, shutdownTimeout time.Duration, log *slog.Logger) error

type ShutdownStep struct {
    Name    string
    Timeout time.Duration
    Fn      func(ctx context.Context) error
}
func RunShutdown(log *slog.Logger, steps ...ShutdownStep) error
```
- `Serve`: goroutine `s.Serve(ln)` (bila `ln == nil`, pakai `net.Listen("tcp", s.Addr)`); tunggu `ctx.Done()` atau error serve. `ctx.Done()` -> `sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)`; `s.Shutdown(sctx)`; `http.ErrServerClosed` dari Serve dianggap nil; error dari Shutdown (timeout) dikembalikan terbungkus dan, bila timeout, panggil `s.Close()` agar koneksi tertinggal diputus. Error `Serve` selain `ErrServerClosed` dikembalikan.
- `RunShutdown`: jalankan step berurutan, tiap step dengan `context.WithTimeout(context.Background(), step.Timeout)` (ctx sinyal sudah dibatalkan, jangan dipakai ulang). Error tiap step di-log (`"shutdown step failed"` dengan `step`) dan DILANJUTKAN ke step berikutnya; return `errors.Join` semua error (nil bila bersih). Log info durasi tiap step.
- Handoff ke usecase 7: `Run` menyusun `Serve(...)` lalu `RunShutdown(log, {jobs.Stop}, {bus.Shutdown, Timeout: cfg.BusShutdownTimeout}, {db.Close})`. Urutan akhir: `srv.Shutdown -> jobs.Stop -> bus.Shutdown -> db.Close`.

### Langkah 7 — Kontrak OpenAPI
Buat `api/modules/infra/foundation.yml` (OpenAPI 3.1.0, `info.title: Grimoire Foundation`, `info.version: 0.1.0`):
- `paths`: `/healthz` (GET, 200 `Health`), `/readyz` (GET, 200 `Health` dan 503 `Health` dengan contoh `unavailable`). Kedua path memakai `servers` default (root, bukan `/api`); beri `tags: [infra]`.
- `components.schemas`: `Error` (persis overview 15, `details` tanpa `type`), `Health` (`status` enum `ok|unavailable`).
- `components.responses`: `BadRequest` (400), `Unauthorized` (401), `Forbidden` (403), `NotFound` (404), `Conflict` (409), `PayloadTooLarge` (413), `RateLimited` (429, header `Retry-After` integer), `Internal` (500). Semuanya `application/json` mengacu `#/components/schemas/Error` dengan satu `example` bergaya contoh bagian 8 dokumen ini.
- Tanpa `$ref` ke file lain (bundle dikerjakan usecase 8, `api/root.yml` bukan tanggung jawab usecase ini). Verifikasi manual: `npx @redocly/cli@latest lint api/modules/infra/foundation.yml` harus lulus (hanya sebagai pengecekan lokal, belum bagian CI).

### Langkah 8 — Test (lihat bagian 7), jalankan `go test -race ./internal/infra/...`
Urutan penulisan: respond -> middleware satuan -> `server_test.go` (rantai penuh) -> `graceful_test.go`. Jalankan `TestMethodNotAllowed_AllowHeader` paling awal (risiko D7).

### Langkah 9 — Verifikasi akhir
`gofmt -l`, `go vet ./...`, `go test -race ./internal/infra/http/... ./internal/infra/middleware/...`. Pastikan `internal/infra/**` tidak mengimpor `internal/domain/**` (grep import). Pastikan `go build ./...` tetap hijau.

---

## 5. Migration

Tidak ada. Usecase ini tidak menyentuh SQL, tabel, atau `schema_migrations`.

---

## 6. Changes (per file/folder)

Baru:
- `backend/internal/infra/http/respond/{status.go,respond.go,status_test.go,respond_test.go}`
- `backend/internal/infra/http/{errors.go,json.go,server.go,health.go,graceful.go}`
- `backend/internal/infra/http/{errors_test.go,health_test.go,server_test.go,graceful_test.go}`
- `backend/internal/infra/middleware/{writer.go,recover.go,traceid.go,accesslog.go,securityheaders.go,bodylimit.go,timeout.go,csrf.go,session.go,roleguard.go,ratelimit.go,origin.go}`
- `backend/internal/infra/middleware/{recover_test.go,traceid_test.go,accesslog_test.go,securityheaders_test.go,bodylimit_test.go,timeout_test.go,csrf_test.go,session_test.go,roleguard_test.go,ratelimit_test.go,origin_test.go}` (package `middleware_test` bila perlu mengimpor `httpx`; selain itu package internal)
- `api/modules/infra/foundation.yml`

Diubah:
- `backend/internal/infra/app/deps.go`: tambah `Ctx`, `API`, `MCP` (langkah 1).
- `backend/go.mod`, `backend/go.sum`: tambah `chi/v5`, `x/time`, `uuid` (langkah 0).

Tidak diubah: `rules.txt` (tidak ada folder top-level baru; `respond` adalah sub-folder `infra/http`), `docs/configuration.md` (tidak ada env baru), `docs/tickets/README.md` (katalog bus sudah memuat `core.auth.resolve-session`; catatan "kontrak didefinisikan T00" adalah milik usecase 2/8).

Handoff ke usecase lain (catat di PR, bukan dikerjakan di sini):
- Usecase 7: `slog.SetDefault(log)`, `root.NotFound(static)`, `Serve` + `RunShutdown`, mengisi `Deps.Ctx/API/MCP` dari hasil `NewRouter`.
- Usecase 6: handler static mengembalikan 405 untuk method selain GET/HEAD.
- T01: provider `core.auth.resolve-session`; cookie `grimoire_session` dibuat T01 dan harus memakai nama `middleware.SessionCookieName`.
- T07: memasang `OriginGuard`/`CheckOrigin` dan auth API key pada `Deps.MCP`.

---

## 7. Test Scenario

Penamaan `Test<Unit>_<Kondisi>`; AC T00 diberi awalan `TestT00_AC<n>_`. Semua dijalankan dengan `-race`. Tidak butuh Postgres (readyz memakai `Pinger` palsu; bus memakai `bus.NewRegistry` asli dengan handler palsu yang menghitung panggilan). Helper test `newStack(t, opts...)` di `server_test.go` membangun `config.Load` dari map env minimum (`DATABASE_URL`, `SESSION_SECRET` 32+ byte, `COOKIE_SECURE`, `AppEnv`), memanggil `NewRouter`, dan mengembalikan `root, api, mcp, registry, buffer log`.

### respond (unit)
- `TestStatusFor_AllCodes` (tabel semua kode, kode tak dikenal -> 500).
- `TestRegisterCodeStatus_NewCode`, `TestRegisterCodeStatus_DuplicatePanics`, `TestRegisterCodeStatus_InvalidStatusPanics`.
- `TestWriteError_Mapping` (tabel): tiap `errs.Code`; sentinel `ErrNotFound/Forbidden/Conflict/Invalid/Unauthorized` (juga dibungkus `fmt.Errorf("%w")`); `*http.MaxBytesError` (cek `details.limit_bytes`); `context.DeadlineExceeded`; `errors.New("pq: connection refused host=10.0.0.5")` -> 500 INTERNAL, pesan tepat `"terjadi kesalahan internal"` dan body TIDAK mengandung "pq" atau "10.0.0.5", tetapi buffer log mengandung keduanya plus `trace_id`.
- `TestWriteError_InternalCodeMessageForcedGeneric`: `errs.New(CodeInternal, "rahasia")` -> pesan generik.
- `TestWriteError_DetailsNullWhenEmpty` (JSON memuat `"details":null`).
- `TestErrorBody_ContainsTraceID`: dari ctx dan fallback header `X-Request-Id`.
- `TestDecodeJSON`: body valid; kosong; sintaks salah; field tak dikenal (details.field); tipe salah; data sisa; melebihi `MaxBytesReader` (error dapat dipetakan `WriteError` ke 413).

### Middleware
- Recover: `TestRecover_PanicReturns500JSON` (body generik tanpa stack; stack ada di log; `trace_id` terisi dari header `X-Request-Id`); `TestRecover_AfterHeaderSentOnlyLogs`; `TestRecover_ErrAbortHandlerRepanics`.
- TraceID: `TestTraceID_GeneratedAndEchoed`, `TestTraceID_ValidIncomingReused`, `TestTraceID_InvalidIncomingReplaced` (spasi, 65 karakter, karakter `<`), `TestTraceID_InContext`.
- AccessLog: `TestAccessLog_NoQueryString`, `TestAccessLog_FieldsPresent` (method, path, route, status, bytes, duration_ms, remote_ip, trace_id, user_id), `TestAccessLog_LevelByStatus` (200 info, 500 error), `TestAccessLog_HealthAtDebug`, `TestAccessLog_IgnoresXForwardedFor`, `TestAccessLog_PanicLoggedAs500`.
- SecurityHeaders: `TestSecurityHeaders` (semua header, CSP persis), `TestSecurityHeaders_HSTSOnlyWhenCookieSecure`, `TestSecurityHeaders_DevAllowsWS` (hanya `AppEnv=development`, tanpa `unsafe-eval`).
- CSRF (T00-AC6): `TestT00_AC6_PostWithoutCSRFHeaderForbidden` (POST, PUT, PATCH, DELETE -> 403 `CSRF_REQUIRED`, pesan persis), `TestT00_AC6_GetPassesWithoutHeader` (GET, HEAD, OPTIONS), `TestT00_AC6_MCPExempt` (`POST /mcp/x` tanpa header -> bukan 403 CSRF; 404 JSON), `TestCSRF_WrongValueForbidden` (`grimoire2`, `Grimoire`, kosong), `TestCSRF_CorrectHeaderPasses`.
- Session (bus asli + handler palsu): `TestSession_NoCookieNoBusCall` (counter = 0, request anonim lolos), `TestSession_EmptyCookieNoBusCall`, `TestSession_FoundSetsIdentity` (token diteruskan persis; `reqctx.IdentityFrom` terisi; `user_id` muncul di access log), `TestSession_NotFoundClearsCookie` (`Set-Cookie: grimoire_session=; Max-Age=0; Path=/; HttpOnly; SameSite=Lax`, `Secure` sesuai config, request tetap diproses anonim), `TestSession_BusErrorReturns500` (pesan generik, error asli hanya di log, cookie tidak dihapus), `TestSession_UnregisteredReturns500` (registry kosong), `TestSession_FoundWithEmptyUserIDReturns500`.
- Role guard: `TestRequireAuth_401`, `TestRequireAuth_Allows`, `TestRequireRole_401WhenAnonymous`, `TestRequireRole_403WhenWrongRole`, `TestRequireRole_Allows` (beberapa role), `TestRequireRole_NoArgsPanics`.
- RateLimit: `TestRateLimit_429WithRetryAfter` (burst habis -> 429, header `Retry-After` integer >= 1, details.retry_after_seconds sama), `TestRateLimit_KeysIndependent` (dua IP), `TestRateLimit_EmptyKeyUnlimited`, `TestRateLimit_KeyByUserFallsBackToIP`, `TestRateLimit_RecoversAfterRefill` (clock palsu), `TestRateLimit_JanitorEvictsIdle` (clock/interval kecil; jumlah entri turun), `TestRateLimit_JanitorStopsOnCtxCancel` (tidak ada goroutine bocor, cek dengan `runtime.NumGoroutine` toleran atau sinyal channel), `TestNewLimiter_InvalidRulePanics`.
- BodyLimit: `TestBodyLimit_413PayloadTooLarge` (body > limit dibaca lewat `DecodeJSON` -> 413 `PAYLOAD_TOO_LARGE`, `details.limit_bytes`), `TestBodyLimit_UnderLimitPasses`, `TestBodyLimit_RouteOverrideRaises` (root 1 KiB, route 10 KiB menerima 5 KiB), `TestBodyLimit_RouteOverrideLowers`, `TestBodyLimit_InvalidSizePanics`.
- Timeout: `TestTimeout_504` (handler menunggu `ctx.Done()` lalu `WriteError(ctx.Err())` -> 504 `TIMEOUT`), `TestTimeout_RouteOverride`, `TestTimeout_InvalidPanics`.
- Origin: `TestCheckOrigin` (kosong -> true; sama dengan Host -> true; cocok entri allowed (case-insensitive) -> true; asing -> false; `null` -> false; URL rusak -> false), `TestOriginGuard_403`.

### Router dan server (`httpx`)
- `TestAPI_NotFoundJSON` (GET `/api/nope` -> 404, `NOT_FOUND`, `trace_id` terisi), `TestAPI_MethodNotAllowedJSON` (route GET didaftar; `PUT` dengan header CSRF -> 405 + `Allow`), `TestMethodNotAllowed_AllowHeader` (`DELETE /healthz` -> 405, `Allow: GET, HEAD`), `TestAPI_MutationOn404StillCSRFChecked` (D6), `TestMCP_NotFoundJSON`, `TestRoot_DefaultNotFoundJSON`.
- `TestHealthz_200`, `TestHealthz_HEAD`, `TestReadyz_200`, `TestReadyz_503WhenDBDown` (pinger mengembalikan error; body `{"status":"unavailable"}`, tanpa detail error di body; log memuat error), `TestReadyz_PingTimeout` (pinger menunggu `ctx.Done()`, respons 503 dalam ~2s; gunakan pinger yang memverifikasi deadline di ctx <= 2s, bukan tidur 2 detik), `TestReadyz_NilPinger503`.
- `TestMiddlewareOrder`: route `api.Get("/boom")` yang panic -> respons 500 JSON dengan header `X-Request-Id`, semua security headers, `trace_id` body sama dengan header, dan baris access log `status=500` memuat `trace_id` yang sama; urutan eksekusi diverifikasi lewat middleware pengamat yang mencatat urutan (Recover paling luar: panic dari middleware terdalam tetap tertangkap).
- `TestAPI_DefaultTimeoutApplied` (handler memeriksa `ctx.Deadline()` ~ `HTTPDefaultTimeout`), `TestHealthz_NotSubjectToAPITimeout`.
- `TestAPI_SessionAndCSRFOrder`: POST tanpa header CSRF dan dengan cookie -> 403 dan bus TIDAK dipanggil (CSRF sebelum session).
- `TestNewServer_Settings` (`ReadHeaderTimeout`, `IdleTimeout=120s`, `WriteTimeout==0`, `Addr`).

### Graceful shutdown
- `TestServe_GracefulWaitsInflight`: listener `127.0.0.1:0`, handler lambat (200 ms, sinyal mulai lewat channel); cancel ctx di tengah request -> klien menerima 200, `Serve` return nil.
- `TestServe_ShutdownTimeoutForcesClose`: handler diblok; `shutdownTimeout` kecil -> `Serve` return error `context.DeadlineExceeded` terbungkus dan tidak menggantung.
- `TestServe_ListenerErrorReturned`.
- `TestRunShutdown_OrderAndContinueOnError`: tiga step palsu mencatat urutan (`http`, `jobs`, `bus`, `db` pada pemanggilan komposisi), step kedua gagal -> step ketiga tetap jalan, hasil `errors.Join` memuat error step kedua; tiap step menerima ctx dengan deadline sesuai `Timeout`.
- `TestRunShutdown_StepContextNotCanceled` (ctx step tidak terbatal walau ctx sinyal sudah batal).

Tambahan: `go vet` dan `golangci-lint` bersih; `go test -race -count=3 ./internal/infra/middleware/...` untuk menangkap flake pada limiter/janitor.

---

## 8. E2E Scenarios

Dijalankan terhadap binary hasil usecase 7 (`bin/grimoire`) atau server `go run ./cmd/http` dengan Postgres lokal; sebelum usecase 7 selesai, ekuivalennya adalah test `httptest` pada `NewRouter` yang sama (tercakup di bagian 7). Semua `curl` memakai `-i`.

Positif:
1. `GET /healthz` -> 200 `{"status":"ok"}`, header `X-Request-Id` (uuid), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, CSP.
2. `GET /healthz` dengan `X-Request-Id: abc-123` -> header respons `X-Request-Id: abc-123`.
3. `GET /readyz` dengan DB hidup -> 200 `{"status":"ok"}`.
4. `GET /api/does-not-exist` -> 404 JSON `NOT_FOUND` dengan `trace_id` sama dengan header `X-Request-Id`.
5. `GET /api/x` tanpa cookie -> bukan 500 (anonim; 404 karena route belum ada) dan tanpa panggilan bus.
6. `POST /mcp/anything` tanpa `X-Requested-With` -> tidak 403 CSRF (404 JSON sampai T07).
7. SIGTERM saat request lambat sedang berjalan -> request selesai 200, proses keluar 0 dalam `SHUTDOWN_TIMEOUT`, urutan log shutdown: http, jobs, bus, db.
8. Request ber-cookie dengan bus palsu `Found=true` (test integrasi) -> `GET` route `RequireAuth` 200; route `RequireRole("super_admin")` oleh developer -> 403.

Negatif:
1. **T00-AC6** `POST /api/anything` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; dengan `X-Requested-With: wrong` -> 403; dengan `grimoire` -> sampai ke router (404 bila route belum ada).
2. `DELETE /healthz` -> 405 JSON `METHOD_NOT_ALLOWED` dengan `Allow: GET, HEAD`.
3. `GET /api/x` dengan `Cookie: grimoire_session=abc` sebelum T01 (bus belum punya provider) -> 500 `INTERNAL` pesan generik, tanpa kebocoran nama endpoint di body, cookie tidak dihapus.
4. DB dimatikan saat server berjalan -> `/readyz` 503 `{"status":"unavailable"}`, `/healthz` tetap 200.
5. Body melebihi batas pada route uji -> 413 `PAYLOAD_TOO_LARGE` dengan `details.limit_bytes`.
6. Burst melebihi aturan rate limit pada route uji -> 429 `RATE_LIMITED` dengan `Retry-After`.
7. Handler panic pada route uji -> 500 JSON generik, proses tetap hidup (`/healthz` masih 200), stack hanya di log.
8. `Origin: https://evil.example` pada route uji yang memakai `OriginGuard` -> 403 `FORBIDDEN`.
9. Query string rahasia, mis. `GET /healthz?token=secret`, tidak muncul di log access.

---

## 9. Request/Response body

Semua error `/api` berformat sama. Contoh lengkap:

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

### Format error seragam (contoh 400 dari `DecodeJSON` unknown field)
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "field tidak dikenal",
    "details": { "field": "nama" }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### 401 (`RequireAuth` tanpa sesi)
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 403 CSRF (`POST /api/projects` tanpa header)
```json
{
  "error": {
    "code": "CSRF_REQUIRED",
    "message": "header X-Requested-With wajib untuk request mutasi",
    "details": null
  },
  "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a"
}
```

### 403 (`RequireRole("super_admin")` oleh developer; atau `OriginGuard` gagal dengan pesan "origin tidak diizinkan")
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### 404 dan 405 (`/api`, `/mcp`, root default)
```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```
```
HTTP/1.1 405 Method Not Allowed
Allow: GET, HEAD
Content-Type: application/json; charset=utf-8
```
```json
{ "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### 413
```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran body melebihi batas", "details": { "limit_bytes": 1048576 } }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### 429 (header `Retry-After: 3`)
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### 504
```json
{ "error": { "code": "TIMEOUT", "message": "permintaan melebihi batas waktu", "details": null }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### 500 (panic, error tak dikenal, bus gagal, kontrak resolve-session dilanggar)
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a" }
```

### Header penghapus cookie sesi (`Found=false`)
```
Set-Cookie: grimoire_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax; Secure
```
(`Secure` hanya bila `COOKIE_SECURE=true`.)

### Kontrak bus yang dikonsumsi (Go, bukan JSON di wire)
```go
bus.CallTyped[resolvesession.Response](ctx, b, constants.BUS_CORE_AUTH_RESOLVE_SESSION,
    resolvesession.Request{Token: "k3J9...raw-cookie-value"})

resolvesession.Response{Found: true, UserID: "0b7c...", Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: false}
resolvesession.Response{Found: false}
```

### Baris access log (JSON)
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"http request","method":"POST","path":"/api/projects","route":"/api/projects","status":403,"bytes":118,"duration_ms":0.4,"remote_ip":"10.0.0.5","trace_id":"0b1c5d2e-77aa-4a0e-93c1-1f2e3d4c5b6a","user_id":""}
```

### Potongan `api/modules/infra/foundation.yml`
```yaml
openapi: 3.1.0
info: { title: Grimoire Foundation, version: 0.1.0 }
paths:
  /healthz:
    get:
      tags: [infra]
      summary: Proses hidup
      responses:
        "200": { description: OK, content: { application/json: { schema: { $ref: "#/components/schemas/Health" } } } }
  /readyz:
    get:
      tags: [infra]
      summary: Dependensi (DB) terjangkau
      responses:
        "200": { description: Siap, content: { application/json: { schema: { $ref: "#/components/schemas/Health" } } } }
        "503": { description: Tidak siap, content: { application/json: { schema: { $ref: "#/components/schemas/Health" }, example: { status: unavailable } } } }
components:
  schemas:
    Health:
      type: object
      required: [status]
      properties: { status: { type: string, enum: [ok, unavailable] } }
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
  responses:
    RateLimited:
      description: Terlalu banyak permintaan
      headers: { Retry-After: { schema: { type: integer, minimum: 1 } } }
      content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } }
    # BadRequest, Unauthorized, Forbidden, NotFound, Conflict, PayloadTooLarge, Internal: pola sama
```
