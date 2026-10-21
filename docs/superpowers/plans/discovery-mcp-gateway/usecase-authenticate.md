# T07 discovery/mcp-gateway — Usecase `authenticate`

Konteks bersama (keputusan K1-K27, `base/`, `base/auditwriter`, `Limiter.Peek`, config `MCP_*`, `Deps.Version`, aksi audit) ada di `overview.md` dan dikerjakan di foundation F. Dokumen ini hanya memuat usecase `authenticate` (auth + rate limit, digabung dari fitur `ratelimit` tiket, K1). Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T07-discovery-mcp-gateway.md`. Kriteria terkait: AC2, AC5, AC6, AC9 (urutan dengan Origin). Keputusan terkait: K4, K6, K19, K20, K21, K22, K23, K24.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T07 |
| Scope | backend (Go) saja; tanpa UI, tanpa migrasi, tanpa OpenAPI |
| Domain | discovery |
| Module | mcp-gateway (`mcpgateway`) |
| Feature | `authenticate` |
| Resource root | `backend/internal/domain/discovery/internal/mcpgateway` |
| allowedPaths | `backend/internal/domain/discovery/internal/mcpgateway/authenticate/` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/mcp` | POST | MCP 2025-11-25 / 2025-06-18 / 2025-03-26 | Middleware admisi; dipasang oleh `endpoint.Init` setelah `OriginGuard` dan `BodyLimit`. Tidak mendaftarkan route sendiri |

Endpoint bus yang dikonsumsi (tidak ada yang baru):

| Endpoint | Mode | Sifat |
|---|---|---|
| `project.apikey.verify` | Call, tanpa cache | fail-closed: err -> 503 `-32004`, log error, tanpa audit |
| `project.management.get-project` | Call | fail-soft: err atau `Found=false` -> `ProjectName=""`, log warn |
| `project.apikey.touch` | Publish | hanya setelah lolos auth dan limit key; err -> log warn |
| `core.audit.record` | Call sinkron via `auditwriter.Writer` | fail-open |

## Summary Plan

`authenticate` adalah satu usecase yang membungkus `POST /mcp` sebagai `func(http.Handler) http.Handler`. Tidak dipecah: auth dan dua limiter berbagi identitas, body penolakan, dan audit. Alur:

1. Ekstrak key (`params.go`): `Authorization: Bearer <key>` (skema case-insensitive) atau `X-API-Key`. Bearer menang bila keduanya ada. Basic tidak didukung (diabaikan, jatuh ke `X-API-Key`). IP = `middleware.KeyByIP(r)`, UA = `r.UserAgent()` dipotong 256 byte aman UTF-8.
2. Pre-check limiter IP gagal-auth dengan `Peek`. Habis -> 429 + `Retry-After` + audit `mcp.rate_limited` scope `auth_ip` (anonymous, dibatasi `AuditThrottle` subjek `ip:<ip>`), TANPA memanggil verify (walau key berikutnya benar).
3. Key kosong -> reason `missing`, tanpa verify. Selain itu `Call project.apikey.verify`:
   - err -> 503 `-32004` + `log.ErrorContext` (fail-closed), tanpa audit, tanpa konsumsi token IP.
   - `Found=false` (atau missing) -> `ipLimiter.Allow(ip)` (konsumsi token; hasilnya tidak mengubah respons), 401 body JSON-RPC identik byte-per-byte + `WWW-Authenticate: Bearer realm="grimoire-mcp"`, audit `mcp.auth.rejected` (anonymous, prefix dari `res.Prefix`, `res.Reason`, status `denied`). Audit rejected TIDAK dibatasi throttle (AC2: satu entri per penolakan); pembatas banjirnya adalah limiter IP di langkah 2.
4. Valid -> `keyLimiter.Allow(KeyID)`. Habis -> 429 + `Retry-After` + audit `mcp.rate_limited` scope `key` (actor `api_key`, dibatasi throttle subjek `key:<KeyID>`; satu entri per 60 detik, AC6). Key lain tidak terpengaruh karena bucket per KeyID.
5. Lolos: `Call get-project` fail-soft untuk label, bentuk `base.Principal{KeyID, KeyName, KeyPrefix, ProjectID, ProjectName, IP, UserAgent}`, `ctx = base.WithPrincipal(reqctx.WithAPIKeyID(ctx, KeyID), p)`, `Publish touch{KeyID}` (konteks `context.WithoutCancel`), lalu `next.ServeHTTP(w, r.WithContext(ctx))`.

Reason/Prefix hanya masuk audit, tidak pernah ke body/header respons (K6). Usecase hanya orkestrasi; rule (format Principal, ActorLabel, builder audit, throttle) ada di `base/`. Pemisahan: `usecase.go` mengembalikan `Decision` (tanpa `http`), `handler.go` memetakan Decision ke HTTP.

Catatan desain:
- Token IP dikonsumsi hanya saat auth gagal (termasuk `missing`); klien sah tidak pernah menghabiskannya. `Allow` yang mengembalikan false saat gagal tetap menghasilkan 401 untuk request itu; request berikutnya kena 429 dari `Peek` (K19).
- `touch` dikirim setelah lolos limiter key (K23); request 429 tidak memperbarui last_used.
- Audit pakai `auditwriter.Writer` (sudah `WithoutCancel`, fail-open, tidak me-log Args/Result).
- IP berasal dari `RemoteAddr` (X-Forwarded-For tidak dipercaya, konsisten dengan AccessLog).

## Plan

Prasyarat foundation (jangan dibuat dari folder ini; bila belum ada, selesaikan F dulu): `Limiter.Peek` (F3), config `MCPRatePerMinute`/`MCPAuthFailPerMinute` (F1), `base.Principal/WithPrincipal`, `base.JSONRPC` (`HTTPReject`, `CodeUnauthorized`, `CodeRateLimited`, `CodeUnavailable`), `base.AuthRejectedAudit`, `base.RateLimitedKeyAudit`, `base.RateLimitedIPAudit`, `base.AuditThrottle`, `base.TruncateName`, `auditwriter.Writer`. Semua file di `backend/internal/domain/discovery/internal/mcpgateway/authenticate/`. Urutan TDD: `params_test` -> `params.go` -> `usecase_test` (RED) -> `usecase.go` -> `handler_test` -> `handler.go` -> `init.go` -> `init_test`.

1. **`params.go`** (+ `params_test.go`): parsing request, tanpa I/O.
   ```go
   type Input struct {
       RawKey    string // kosong = tidak ada key
       IP        string
       UserAgent string
   }
   func ParseInput(r *http.Request) Input
   func extractKey(h http.Header) string
   ```
   Aturan `extractKey`:
   - `Authorization` dipecah `strings.Cut(v, " ")`; skema `strings.EqualFold(scheme, "Bearer")` dan token (`TrimSpace`) tidak kosong -> pakai token (menang atas `X-API-Key`).
   - Skema lain (Basic, dll), Bearer tanpa token -> diabaikan, lanjut ke `X-API-Key`.
   - `X-API-Key` -> `TrimSpace`. Kosong -> `""`.
   - Hanya header pertama yang dipakai (`Header.Get`). Nilai tidak di-log.
   `ParseInput`: `IP = middleware.KeyByIP(r)`, `UserAgent = base.TruncateName(r.UserAgent(), 256)`.

2. **`usecase.go`** (+ `usecase_test.go`): orkestrasi murni, tanpa `net/http`. Interface di sisi consumer (rules 5.6):
   ```go
   type Bus interface {
       Call(ctx context.Context, endpoint string, req any) (any, error)
       Publish(ctx context.Context, endpoint string, req any) error
   }
   type Limiter interface {
       Allow(key string) (bool, time.Duration)
       Peek(key string) (bool, time.Duration)
   }
   type Throttle interface{ Allow(subject string) bool }

   type Kind int
   const (
       Admitted Kind = iota + 1
       Unauthorized   // 401
       RateLimited    // 429
       Unavailable    // 503
   )
   type Decision struct {
       Kind       Kind
       Principal  base.Principal // hanya bila Admitted
       RetryAfter int            // detik, >= 1, hanya bila RateLimited
   }

   type Usecase struct {
       bus      Bus
       ipLim    Limiter
       keyLim   Limiter
       throttle Throttle
       audit    auditwriter.Writer
       log      *slog.Logger
   }
   func NewUsecase(bus Bus, ipLim, keyLim Limiter, th Throttle, audit auditwriter.Writer, log *slog.Logger) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) Decision
   ```
   `Bus` diimplementasi `*bus.Registry`; panggilan lewat `bus.CallTyped` tidak bisa dipakai pada interface, jadi pakai adapter kecil privat `callTyped[T any](ctx, b Bus, endpoint, req) (T, error)` di file ini yang melakukan `b.Call` lalu type-assert nilai (bukan pointer) ke T; tipe salah -> error. (Alternatif yang boleh: field `*bus.Registry` konkret bila tes memakai `bus.NewRegistry`; pilih salah satu dan konsisten, tes plan ini memakai `bus.NewRegistry` nyata sehingga field konkret `*bus.Registry` juga lolos.)

   Alur `Execute`:
   1. `if ok, wait := u.ipLim.Peek(in.IP); !ok { return u.limited(ctx, wait, in, nil) }` -> audit IP (lihat `limited`). Verify TIDAK dipanggil.
   2. `reason := ""`; bila `in.RawKey == ""`: `reason = "missing"`, `prefix = ""`. Selain itu:
      ```go
      res, err := callTyped[verify.Response](ctx, u.bus, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: in.RawKey})
      if err != nil {
          u.log.ErrorContext(ctx, "mcp verify failed", "err", err) // tanpa RawKey, tanpa IP key
          return Decision{Kind: Unavailable}
      }
      ```
      `!res.Found` -> `reason = res.Reason`, `prefix = res.Prefix`.
   3. Bila `reason != ""` (gagal auth): `u.ipLim.Allow(in.IP)` (hasil diabaikan), `u.audit.Write(ctx, base.AuthRejectedAudit(in.IP, in.UserAgent, prefix, reason))`, `return Decision{Kind: Unauthorized}`. Urutan: Allow dulu, audit kemudian.
   4. Valid: `if ok, wait := u.keyLim.Allow(res.KeyID); !ok { return u.limited(ctx, wait, in, &res) }`.
   5. Label project fail-soft:
      ```go
      name := ""
      gp, err := callTyped[getproject.Response](ctx, u.bus, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: res.ProjectID})
      if err != nil || !gp.Found { u.log.WarnContext(ctx, "mcp get-project failed", "project_id", res.ProjectID, "err", err) } else { name = gp.Name }
      ```
   6. `p := base.Principal{KeyID: res.KeyID, KeyName: res.KeyName, KeyPrefix: res.Prefix, ProjectID: res.ProjectID, ProjectName: name, IP: in.IP, UserAgent: in.UserAgent}`.
   7. `if err := u.bus.Publish(context.WithoutCancel(ctx), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: res.KeyID}); err != nil { u.log.WarnContext(ctx, "mcp touch publish failed", "key_id", res.KeyID, "err", err) }`.
   8. `return Decision{Kind: Admitted, Principal: p}`.

   Helper `limited(ctx, wait, in, res *verify.Response) Decision`:
   - `secs := int(math.Ceil(wait.Seconds())); if secs < 1 { secs = 1 }`.
   - Subjek & audit: `res == nil` -> subjek `"ip:"+in.IP`, entri `base.RateLimitedIPAudit(in.IP, in.UserAgent, secs)`. `res != nil` -> subjek `"key:"+res.KeyID`, entri `base.RateLimitedKeyAudit(principal sementara tanpa ProjectName, secs)`; untuk jalur key label project sebaiknya diisi: panggil get-project fail-soft juga di jalur ini (helper `projectName(ctx, projectID)` dipakai bersama langkah 5), supaya entri audit punya `ProjectLabel`.
   - `if u.throttle.Allow(subject) { u.audit.Write(...) } else { u.log.WarnContext(ctx, "mcp rate limited (audit throttled)", "scope", scope) }`.
   - `return Decision{Kind: RateLimited, RetryAfter: secs}`.

   Larangan di usecase: tidak ada `time.Now`, tidak ada cache, tidak menulis `net/http`, tidak me-log raw key/Reason, tidak memanggil verify bila IP terblokir atau key kosong, tidak membedakan alasan dalam `Decision` yang dipakai handler (Reason tidak ada di `Decision`).

3. **`handler.go`** (+ `handler_test.go`): adaptor HTTP, tanpa rule bisnis.
   ```go
   type handler struct {
       uc   *Usecase
       body401, body503 []byte // dihitung sekali di newHandler
       log  *slog.Logger
   }
   func newHandler(uc *Usecase, log *slog.Logger) *handler
   func (h *handler) middleware(next http.Handler) http.Handler
   ```
   - `body401` = `json.Marshal(base.HTTPReject(base.CodeUnauthorized, "Unauthorized", nil))` dihitung SEKALI sehingga identik byte-per-byte untuk semua alasan (K6). Hasil: `{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}`.
   - `middleware`: `d := h.uc.Execute(r.Context(), ParseInput(r))`, lalu switch:
     - `Admitted`: `ctx := reqctx.WithAPIKeyID(r.Context(), d.Principal.KeyID)`; `ctx = base.WithPrincipal(ctx, d.Principal)`; `next.ServeHTTP(w, r.WithContext(ctx))`.
     - `Unauthorized`: 401, `WWW-Authenticate: Bearer realm="grimoire-mcp"`, body `body401`.
     - `RateLimited`: 429, `Retry-After: <RetryAfter>`, body `HTTPReject(CodeRateLimited, "Rate limit exceeded", map[string]int{"retry_after_seconds": secs})`.
     - `Unavailable`: 503, body `body503` (`-32004`, `Service unavailable`).
     - default: 503 (defensif) + log error.
   - Semua respons penolakan: `Content-Type: application/json`, `Cache-Control: no-store`. Tidak membaca body request (lazy BodyLimit tetap lazy, K24). Tidak menyertakan `Reason`/`Prefix` di header atau body.
   - Error `w.Write` diabaikan dengan log level debug.

4. **`init.go`** (+ `init_test.go`): satu-satunya pembuat dependensi konkret.
   ```go
   func Init(d app.Deps, audit auditwriter.Writer) func(http.Handler) http.Handler {
       cfg := d.Config
       keyLim := middleware.NewLimiter(d.Ctx, middleware.Rule{
           Name: "mcp-key", Rate: rate.Limit(float64(cfg.MCPRatePerMinute) / 60), Burst: cfg.MCPRatePerMinute,
           KeyFunc: func(r *http.Request) string { return reqctx.APIKeyID(r.Context()) },
       })
       ipLim := middleware.NewLimiter(d.Ctx, middleware.Rule{
           Name: "mcp-auth-fail-ip", Rate: rate.Limit(float64(cfg.MCPAuthFailPerMinute) / 60), Burst: cfg.MCPAuthFailPerMinute,
           KeyFunc: middleware.KeyByIP,
       })
       th := base.NewAuditThrottle(time.Minute, time.Now)
       uc := NewUsecase(d.Bus, ipLim, keyLim, th, audit, d.Log)
       return newHandler(uc, d.Log).middleware
   }
   ```
   - Janitor limiter berhenti lewat `d.Ctx` (diteruskan ke `NewLimiter`; tidak ada goroutine lain).
   - `KeyFunc` hanya memenuhi validasi `Rule` (wajib non-nil); penggunaan sebenarnya lewat `Allow`/`Peek` langsung. Nilai config sudah divalidasi `>= 1` di F1 sehingga `Rate`/`Burst` valid dan `NewLimiter` tidak panic.
   - Gunakan `*bus.Registry` konkret (`d.Bus`) bila memilih opsi field konkret di langkah 2.
   - `endpoint.Init` yang memasang hasil ini di `d.MCP.With(OriginGuard, BodyLimit, admit).Post(...)`; `authenticate` TIDAK menyentuh `d.MCP`.

5. **Ganti stub** (bila F7 memakai stub): isi `Init` asli di folder ini; tidak mengubah `mcpgateway/base.go` karena signature sudah sesuai overview.

6. **Verifikasi**: `go build ./...`, `go vet ./...`, `go test ./internal/domain/discovery/... -race`, `go test ./internal/archtest/...` (tidak ada import ke domain/module lain; hanya `infra/*`, `lib/*`, `base`, `auditwriter`).

## Migration

Tidak ada. Tidak ada tabel; limiter dan throttle di memori satu instance.

## Changes

| File/folder | Aksi | Isi |
|---|---|---|
| `.../mcpgateway/authenticate/init.go` | baru (atau ganti stub) | `Init(d, audit)` membuat dua limiter, throttle, usecase, handler |
| `.../authenticate/params.go` | baru | `Input`, `ParseInput`, `extractKey` |
| `.../authenticate/usecase.go` | baru | `Usecase.Execute`, `Decision`, interface `Limiter`/`Throttle`, helper `limited` |
| `.../authenticate/handler.go` | baru | middleware, pemetaan Decision ke HTTP, body 401 tetap |
| `.../authenticate/params_test.go`, `usecase_test.go`, `handler_test.go`, `init_test.go` | baru | lihat Test Scenario |
| `repository.go` | TIDAK ada | tanpa SQL |
| `.../mcpgateway/base/**`, `base.go`, `endpoint/`, `callaudit/` | tidak diubah | milik foundation / usecase lain |
| `lib/constants/bus.go`, `lib/contracts/**`, `go.mod`, `api/**`, `migrations/**` | tidak berubah | - |

## Test Scenario

Penamaan `TestT07_ACn_...` untuk kriteria penerimaan. Tanpa mock framework; fake tulisan tangan. Tes `usecase_test` memakai `bus.NewRegistry` nyata + handler palsu (`verify`, `get-project`, `touch` yang menandai lewat channel; Publish async, tunggu channel atau `Registry.Shutdown`), `auditwriter.Writer` palsu yang menyimpan `[]record.Request`, limiter palsu (`Allow`/`Peek` dengan counter dan klok terkendali) dan throttle palsu atau `base.NewAuditThrottle` dengan klok palsu.

### `params_test.go`
- Bearer saja, `bearer` huruf kecil, `BEARER` -> token terambil.
- `X-API-Key` saja -> terambil, spasi tepi dibuang.
- Keduanya ada dengan nilai berbeda -> Bearer menang.
- `Authorization: Basic xxx` saja -> `""`; Basic + `X-API-Key` -> pakai `X-API-Key`.
- `Authorization: Bearer ` (kosong) + `X-API-Key: k` -> `k`.
- Tanpa header -> `""`. IP dari `RemoteAddr` `10.0.0.7:5555` -> `10.0.0.7`. UA panjang > 256 byte dipotong dan tetap UTF-8 valid.

### `usecase_test.go`
- `TestT07_AC2_RejectedAudit`: tiga kasus (tanpa key; key acak -> verify `Found=false, Reason=unknown, Prefix=abcd1234`; key dicabut -> `Reason=revoked`) menghasilkan `Decision{Unauthorized}` dan MASING-MASING tepat satu `mcp.auth.rejected` dengan `ActorType=anonymous`, `ActorID=""`, `ActorLabel=prefix`, `Status=denied`, `Args.reason` sesuai (`missing`/`unknown`/`revoked`), `Args.key_prefix`, `IP`, `UserAgent`. Kasus tanpa key: verify TIDAK dipanggil.
- `Reason` tidak ada di `Decision` (assert via refleksi tipe atau cukup tidak ada field; handler test memeriksa body).
- `TestT07_AC5_RevokeTakesEffectNextRequest`: fake verify mengembalikan `Found=true` lalu `Found=false/revoked`; panggilan kedua 401; verify dipanggil pada setiap `Execute` (tidak ada cache; hitung panggilan = 2).
- `TestT07_AC6_KeyRateLimit`: limiter key asli (`middleware.NewLimiter`, burst 2) dan throttle klok palsu: panggilan 1-2 `Admitted`; 3-6 `RateLimited` dengan `RetryAfter >= 1`; audit `mcp.rate_limited` tepat 1 entri (`ActorType=api_key`, `ActorID=KeyID`, `Args.scope=key`, `Args.retry_after_seconds`, `Status=denied`, `ProjectLabel` terisi); setelah klok throttle maju 61 detik dan masih habis -> entri ke-2. Key B (KeyID lain) tetap `Admitted` saat key A habis.
- `TestT07_IPPrecheckBlocksEvenWithValidKey`: limiter IP burst 1; satu request gagal (konsumsi token); request kedua dengan key VALID -> `RateLimited`, verify TIDAK dipanggil (counter tetap 1 dari request pertama), audit `mcp.rate_limited` scope `auth_ip` (`ActorType=anonymous`, `Target=ip`), touch tidak dikirim. Banyak request ter-blok dalam 60 detik -> 1 entri audit saja; sisanya log warn.
- `TestT07_VerifyErrorIs503`: verify mengembalikan error -> `Unavailable`; tidak ada audit; log berisi `mcp verify failed` level error dan TIDAK memuat raw key; token limiter IP tidak terkonsumsi (`Peek` dan `Allow` hitungan: Allow tidak dipanggil); touch tidak dikirim; get-project tidak dipanggil.
- `TestT07_GetProjectFailureStillAdmits`: get-project error -> `Admitted`, `Principal.ProjectName==""`, log warn; get-project `Found=false` -> sama; touch tetap dikirim.
- `TestT07_TouchOnlyOnSuccess`: touch dikirim tepat sekali dengan `KeyID` saat `Admitted`; TIDAK dikirim saat Unauthorized, RateLimited (key), RateLimited (IP), Unavailable. Publish error (bus tanpa handler touch) -> tetap `Admitted` + log warn.
- Principal lengkap: `KeyID/KeyName/KeyPrefix/ProjectID/ProjectName/IP/UserAgent` sesuai fake.
- `missing` mengonsumsi token IP (limiter IP burst N: N kali tanpa key lalu request berikutnya `RateLimited`).
- Audit gagal (writer palsu diam / bus audit error lewat `auditwriter` asli) tidak mengubah Decision (fail-open).

### `handler_test.go` (httptest)
- `TestT07_AC2_IdenticalBody401`: tanpa key, key acak, key dicabut -> status 401, header `WWW-Authenticate` persis `Bearer realm="grimoire-mcp"`, body byte-per-byte sama (bandingkan `bytes.Equal` ketiganya) dan sama dengan `{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}`; body tidak memuat `unknown`, `revoked`, `missing`, `malformed`, `mismatch`, prefix key; header respons tidak memuat Reason.
- 429 key: status 429, `Retry-After` angka >= 1, body `-32003` dengan `data.retry_after_seconds` sama dengan header.
- 429 IP: format sama.
- 503: body `-32004`, tanpa `WWW-Authenticate`.
- Admitted: handler `next` melihat `base.PrincipalFrom(ctx)` benar dan `reqctx.APIKeyID(ctx)==KeyID`; `next` TIDAK dipanggil pada penolakan.
- Body request tidak dibaca saat penolakan (reader palsu yang gagal bila dibaca).
- `Content-Type: application/json` dan `Cache-Control: no-store` pada semua penolakan.

### `init_test.go`
- `Init` dengan `Config` (rate 2, auth-fail 2) menghasilkan middleware yang bekerja end-to-end: dua request valid lolos, ketiga 429.
- Ctx dibatalkan -> janitor limiter berhenti (tes tidak bocor goroutine; `goleak` bila sudah ada di repo, kalau tidak cukup memastikan tidak panic dan `-race` bersih).
- `Init` tidak panic untuk nilai config minimum (1) dan default (120/20).

## E2E Scenarios

Frontend tidak berubah di usecase ini; skenario e2e API-only berada di `frontend/e2e/mcp-gateway.spec.ts` (ditulis di I6 foundation; usecase ini menjadi dasar skenario berikut). Tes integrasi Postgres di `backend/cmd/internal/server/mcpgateway_integration_test.go` (I2) memakai key T04 asli.

Positif:
1. Key valid dengan `Authorization: Bearer` -> `initialize` 200; baris audit-log aktor `api_key` muncul; `last_used_at` key terisi (touch).
2. Key valid dengan `X-API-Key` -> hasil sama.
3. Bearer dan `X-API-Key` keduanya ada, Bearer valid dan `X-API-Key` salah -> 200 (Bearer menang). Sebaliknya Bearer salah + `X-API-Key` valid -> 401.
4. Key milik project lain tidak mempengaruhi: dua key, habiskan limit key A (config e2e `MCP_RATE_PER_MINUTE` kecil bila environment e2e mengizinkan; kalau tidak, cukup lewat tes Go AC6) -> key B tetap 200.

Negatif:
5. Tanpa header -> 401, `www-authenticate` diawali `Bearer`, `error.code == -32001`; audit-log filter `mcp.auth.rejected` menampilkan baris aktor anonymous.
6. Key acak berformat benar -> 401, body identik dengan skenario 5.
7. Key dicabut lewat `revokeKeyApi` -> request berikutnya 401 (AC5); baris `mcp.auth.rejected` berlabel prefix key.
8. `Authorization: Basic ...` saja -> 401.
9. Brute force: > `MCP_AUTH_FAIL_PER_MINUTE` request gagal dari IP yang sama -> 429 + `Retry-After`; satu baris `mcp.rate_limited` (scope `auth_ip`) di audit log per jendela 60 detik, walau request ditolak berkali-kali.
10. Origin asing + key valid -> 403 dari `OriginGuard` sebelum auth (tidak ada entri `mcp.auth.rejected` untuk request itu).
11. Verify tidak tersedia (hanya tes Go; tidak bisa disimulasikan di e2e): 503 `-32004`.

## Request/Response body

Semua contoh untuk `POST /mcp`; body request tidak dibaca oleh `authenticate`. Contoh key: `gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA`.

### Diterima (Admitted)

```http
POST /mcp
Authorization: Bearer gr_a1b2c3d4_Q2xhdWRlLWxhcHRvcC1zZWNyZXQtdmFsdWUtNDN4eHh4eHh4eA
Content-Type: application/json
```
Request diteruskan ke `endpoint` dengan `Principal{KeyID:"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", KeyName:"claude-laptop", KeyPrefix:"a1b2c3d4", ProjectID:"0192f0c4-0000-7000-8000-00000000000a", ProjectName:"Payments", IP:"10.0.0.7", UserAgent:"claude-code/2.1.0"}`. Respons akhir berasal dari `endpoint` (lihat overview bagian 14).

Header alternatif: `X-API-Key: gr_a1b2c3d4_Q2xh...`.

### 401 (identik untuk missing/malformed/unknown/mismatch/revoked)

Header: `WWW-Authenticate: Bearer realm="grimoire-mcp"`, `Content-Type: application/json`, `Cache-Control: no-store`
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}
```

### 429 (key melebihi limit atau IP gagal-auth habis)

Header: `Retry-After: 1`
```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32003,"message":"Rate limit exceeded","data":{"retry_after_seconds":1}}}
```

### 503 (verify error, fail-closed)

```json
{"jsonrpc":"2.0","id":null,"error":{"code":-32004,"message":"Service unavailable"}}
```

### Entri audit yang dikirim ke `core.audit.record`

`mcp.auth.rejected`:
```json
{"ActorType":"anonymous","ActorID":"","ActorLabel":"a1b2c3d4","Action":"mcp.auth.rejected","TargetType":"api_key","Target":"a1b2c3d4","Args":{"reason":"revoked","key_prefix":"a1b2c3d4"},"Result":{},"Status":"denied","IP":"10.0.0.7","UserAgent":"claude-code/2.1.0"}
```
Tanpa key (`missing`):
```json
{"ActorType":"anonymous","ActorID":"","ActorLabel":"","Action":"mcp.auth.rejected","TargetType":"","Target":"","Args":{"reason":"missing","key_prefix":""},"Result":{},"Status":"denied","IP":"10.0.0.7","UserAgent":"curl/8.4.0"}
```
`mcp.rate_limited` scope key (maks 1 per 60 detik per key):
```json
{"ActorType":"api_key","ActorID":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","ActorLabel":"claude-laptop (a1b2c3d4)","ProjectID":"0192f0c4-0000-7000-8000-00000000000a","ProjectLabel":"Payments","Action":"mcp.rate_limited","TargetType":"api_key","Target":"0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c","Args":{"scope":"key","retry_after_seconds":1},"Result":{},"Status":"denied","IP":"10.0.0.7","UserAgent":"claude-code/2.1.0"}
```
`mcp.rate_limited` scope auth_ip (maks 1 per 60 detik per IP):
```json
{"ActorType":"anonymous","ActorID":"","ActorLabel":"","Action":"mcp.rate_limited","TargetType":"ip","Target":"10.0.0.7","Args":{"scope":"auth_ip","retry_after_seconds":3},"Result":{},"Status":"denied","IP":"10.0.0.7","UserAgent":"curl/8.4.0"}
```

### Format error 403 Origin (bukan bagian usecase ini, dari `OriginGuard`)

```json
{"error":{"code":"FORBIDDEN","message":"origin tidak diizinkan"},"trace_id":"01HZX3K9Q7"}
```
