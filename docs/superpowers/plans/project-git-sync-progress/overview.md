# T17 project/git-sync progress — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T17-project-git-sync-progress.md` (revisi T14; isi tiket T14 tidak diubah, semua perubahan dicatat di T17).
Sumber aturan: `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog audit), `rules.txt`, `.claude/rules/migrations.md`, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (file `*knowledge-hub-design.md` tidak ada; ini padanannya), `DESIGN.md` + `.claude/skills/design-taste-frontend/references/app-screens.md` (frontend).
Konteks yang dicek: `CLAUDE.md` tidak ada. `.claude/rules/` hanya berisi `migrations.md`. Plan T14 ada di `docs/superpowers/plans/project-git-sync/`. Kode T14 sudah ada di `main` (`e2cbdf5`, `fefabd1`).

Dokumen ini adalah plan FOUNDATION: keputusan bersama, perubahan infra (`infra/config`, `infra/gitprovider`, `infra/middleware`), perubahan `base/` module gitsync, kontrak bersama (pacer, Sink, event SSE), migrasi, OpenAPI, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-git-sync-progress/usecase-runsync.md` dan `usecase-getconfig.md`. Keduanya WAJIB mengikuti dokumen ini; bila bertentangan, dokumen ini yang menang.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T17 (revisi T14) |
| Scope | fullstack |
| Domain / Module | project / git-sync (`gitsync`) |
| Resource root | `backend/internal/domain/project/internal/gitsync` |
| Infra yang disentuh | `infra/config` (env baru), `infra/gitprovider` (pacing), `infra/middleware/writer.go` (deteksi Flush) |
| Frontend root | `frontend/src/domain/project/git-sync/{runsync,getconfig}`, `frontend/src/shared/services/` |
| Bergantung pada | T14 (merged) |
| Usecase baru | tidak ada; dua feature yang sudah ada direvisi: `runsync`, `getconfig` |

| Endpoint | Method | Version | Module / feature | Perubahan |
|---|---|---|---|---|
| `/api/projects/{projectID}/git-sync/run` | POST | `/api` tanpa prefix versi; `api/modules/project/git-sync.yml` `info.version` 0.1.0 -> 0.2.0 (aditif) | gitsync/runsync | `Accept: text/event-stream` -> respons SSE; selain itu JSON T14 tidak berubah |
| `/api/projects/{projectID}/git-sync?kind=` | GET | sama | gitsync/getconfig | field aditif `in_progress` (bool) |
| bus | – | – | – | tidak ada kontrak baru maupun perubahan kontrak |

---

## 2. Summary Plan

Sync T14 gagal seketika dengan `RATE_LIMITED` saat jatah API GitHub/GitLab habis, dan user tidak melihat apa pun selama sync berjalan. T17:

1. **Pacing per request** di satu tempat, `infra/gitprovider` (K8). Tiap sesi sync memakai `gitprovider.Session` yang (a) menjaga jeda minimum antar awal request = `GIT_SYNC_REQUEST_INTERVAL` (default **500ms**, keputusan pemilik), (b) membaca header rate limit di setiap respons, tidur sampai reset bila `remaining=0`, (c) mengulang request yang kena 429/403-rate-limit maksimal 3 kali, (d) tidak pernah tidur lebih dari `GIT_SYNC_MAX_WAIT` per tunggu maupun `GIT_SYNC_MAX_TOTAL_WAIT` per sync. Melewati batas berarti `RateLimitedError` tanpa tidur, lalu dipetakan ke `RATE_LIMITED` + `retry_after_seconds`.
2. **Progress per file lewat SSE** di pintu yang sama (`POST .../git-sync/run`, negosiasi `Accept`). Event: `start`, `progress`, `waiting`, `saving`, `done`, `error`, ditambah heartbeat.
3. **Sync lepas dari koneksi klien**: usecase dipecah `Start` (validasi, token, lock, upsert config; error di sini tetap respons JSON berstatus HTTP) dan `Run.Execute` (pull, write, simpan hasil, audit, unlock) yang berjalan dengan ctx `WithoutCancel` + `GIT_SYNC_TIMEOUT`. SSE hanya penonton; event dikirim non-blocking.
4. **`getconfig.in_progress`** supaya dialog yang dibuka ulang tahu sync masih berjalan di server (membaca `pg_locks`, tanpa mengambil lock).
5. **Audit** `git_sync.run` menambah `wait_count` dan `wait_total_seconds`.
6. Hasil sync, urutan file, aturan tulis, dan bentuk laporan identik dengan T14.

Tafsiran keputusan pemilik "batas tunggu per 500ms per request, configurable di env": jeda minimum antar request ke provider dalam satu sync, env `GIT_SYNC_REQUEST_INTERVAL`, default `500ms`. Tafsiran "satu tunggu maksimal 500ms" tidak dipakai karena reset rate limit GitHub/GitLab dalam hitungan menit, sehingga sync akan selalu gagal begitu rate limit kena; batas satu tunggu tetap ada sebagai `GIT_SYNC_MAX_WAIT` (120s). Pacing adaptif K1 berjalan di atas jeda ini.

---

## 3. Konteks terverifikasi di kode (bukan asumsi)

| Fakta | Lokasi | Dampak ke plan |
|---|---|---|
| `Client.get` memetakan non-2xx ke error via `mapStatus`; header 2xx dibuang | `infra/gitprovider/client.go` | dipecah: `send` (respons mentah) + `get` (cek status); Session memakai `send` agar header 2xx dan error terbaca |
| GitLab tree dipaginasi (beberapa request per `ListTree`) | `infra/gitprovider/gitlab.go` | pacing wajib di level request (`get`), bukan per `ListTree` |
| `retryAfter` memakai `time.Until` (jam nyata), tanpa clamp | `client.go` | diganti `parseRateHeaders(h, now)` yang bisa dites dan di-clamp |
| `safehttp.New(Timeout: cfg.GitSyncTimeout)` | `NewFromConfig` | timeout per request = `min(GitSyncTimeout, 2m)` |
| `l.dur` menolak nilai `<= 0` | `infra/config/config.go:186` | helper baru `durBetween` agar interval boleh `0` |
| Validasi `GIT_SYNC_TIMEOUT > HTTP_DEFAULT_TIMEOUT`, default 120s | `infra/config/git.go` | default jadi 15m + validasi baru |
| `docs_test.go`, `envexample_test.go` mencocokkan env dengan dokumen | `infra/config/` | `docs/configuration.md` dan `.env.example` wajib diperbarui di commit yang sama |
| Server tanpa `WriteTimeout` | `infra/http/graceful.go` | SSE aman tanpa ubah server |
| `statusWriter.Flush` diam-diam no-op bila writer bawah tidak mendukung; punya `Unwrap` | `infra/middleware/writer.go` | tambah `FlushError() error` agar `http.NewResponseController(w).Flush()` bisa mendeteksi tidak didukung |
| `Timeout`/`ExtendTimeout` hanya mengganti ctx, tidak membungkus writer; tidak ada middleware kompresi | `infra/middleware/timeout.go` | SSE tidak tertahan middleware |
| `Execute` T14 satu fungsi: access -> kind -> URL -> prefix -> token -> TryLock -> UpsertConfig -> pull -> write -> SaveResult -> audit | `runsync/usecase.go` | dipecah `Start` + `Run.Execute` (lihat §6 U1) |
| `fetchAll` konkurensi 4, `tooLarge` ditandai tanpa fetch bila `Size > MaxFileBytes` | `runsync/usecase.go` | progress dipancarkan juga untuk kandidat tooLarge |
| `mapError` memakai `int(rl.RetryAfter.Seconds())` (dibulatkan ke bawah) | `runsync/usecase.go` | dibulatkan ke atas (`math.Ceil`) |
| `lockKeySQL = hashtext('grimoire.gitsync.' \|\| $1 \|\| '.' \|\| $2)` di runsync | `runsync/repository.go` | dipindah ke `base/store.LockKeySQL`, dipakai `TryLock` dan `IsLocked` |
| `base.Shared.Git` bertipe `*gitprovider.Client` | `gitsync/base/actor.go` | runsync membungkus dengan adapter karena `Session` mengembalikan tipe konkret |
| `audit.Event` tanpa info tunggu | `base/audit/audit.go` | tambah `WaitCount`, `WaitTotal` |
| FE `runSync` memakai `post` + `CLIENT_TIMEOUT_MS = 130_000` | `runsync/services/runsync.service.ts` | diganti stream + idle timeout 45s |
| FE `http.ts` mengekspor `CSRF_HEADER`, `toApiError`, `notifyUnauthorized` | `frontend/src/shared/services/http.ts` | helper `openStream` dibangun dari bagian ini |
| E2E T14 memakai `frontend/e2e/fixtures/fake-git-server.ts` | `frontend/e2e/` | fake server ditambah mode rate limit |

---

## 4. Keputusan desain (mengikat semua plan usecase)

| ID | Keputusan |
|---|---|
| D1 | Jeda antar request `GIT_SYNC_REQUEST_INTERVAL` default 500ms, 0..10s, berlaku untuk semua goroutine fetch dalam satu Session (jarak antar **awal** request). Jeda ini tidak dilaporkan sebagai `waiting` dan tidak dihitung ke total tunggu. |
| D2 | Pacing adaptif (K1) hanya di `infra/gitprovider.Session`. `VerifyToken` dan `Client.ListTree/FetchBlob` lama tetap tanpa pacing (dipakai `core/gitcredential`). |
| D3 | Batas: `GIT_SYNC_MAX_WAIT` 120s (satu tunggu), `GIT_SYNC_MAX_TOTAL_WAIT` 5m (total tunggu rate limit per sync), retry 429/403-rl = konstanta `maxRateLimitRetries = 3` (bukan env). Header provider di-clamp `[0, 24h]` sebelum dibandingkan. |
| D4 | `GIT_SYNC_TIMEOUT` default naik 2m -> 15m. Alasan: 500 file x 500ms ~ 250s + total tunggu 5m + fase tulis. Validasi `> HTTP_DEFAULT_TIMEOUT` dan `> GIT_SYNC_MAX_TOTAL_WAIT`. |
| D5 | Satu pintu (K4 opsi b): `Accept` memuat `text/event-stream` -> SSE; selain itu JSON T14. |
| D6 | Usecase `runsync` dipecah `Start` / `Run.Execute` / `Run.Abort`. Error `Start` (400/403/404/409/422/503) selalu respons JSON biasa, juga di jalur SSE. |
| D7 | Execute selalu dengan ctx lepas: `context.WithTimeout(context.WithoutCancel(r.Context()), cfg.GitSyncTimeout)`, di jalur JSON maupun SSE. Putusnya klien tidak membatalkan sync. |
| D8 | Sink non-blocking: kanal ber-buffer 64, event progress/waiting dibuang bila penuh; `done`/`error` dari nilai kembali Execute lewat kanal hasil kapasitas 1. Sebelum menulis `done`/`error`, handler menguras sisa kanal event (non-blocking) agar urutan benar. |
| D9 | `in_progress` di getconfig membaca `pg_locks`; dilarang `pg_try_advisory_lock` + unlock (bisa membuat run lain 409 palsu). Fail-soft: error -> `false` + log warn. |
| D10 | Tidak ada migrasi, tidak ada kontrak bus baru. |
| D11 | Di luar scope: tombol Batalkan (tiket lanjutan); sisa jatah rate limit hanya ditampilkan saat menunggu; graceful shutdown tidak menunggu sync terlepas (lock lepas bersama koneksi; fase tulis tetap satu panggilan bus seperti T14). |

---

## 5. Kontrak bersama

### 5.1 Env (infra/config)

| Env | Tipe | Default | Validasi | Field `config.Config` |
|---|---|---|---|---|
| `GIT_SYNC_REQUEST_INTERVAL` | duration | `500ms` | `0 <= v <= 10s` (`0` = tanpa jeda) | `GitSyncRequestInterval time.Duration` |
| `GIT_SYNC_MAX_WAIT` | duration | `120s` | `1s <= v <= GIT_SYNC_MAX_TOTAL_WAIT` | `GitSyncMaxWait time.Duration` |
| `GIT_SYNC_MAX_TOTAL_WAIT` | duration | `5m` | `> 0` dan `< GIT_SYNC_TIMEOUT` | `GitSyncMaxTotalWait time.Duration` |
| `GIT_SYNC_TIMEOUT` (ubah) | duration | `15m` (dulu `2m`) | `> HTTP_DEFAULT_TIMEOUT` dan `> GIT_SYNC_MAX_TOTAL_WAIT` | sudah ada |

Pesan gagal (memakai `l.fail` yang ada, bahasa Indonesia, tanpa nilai rahasia):

- `GIT_SYNC_REQUEST_INTERVAL`: `harus di antara 0s dan 10s (sekarang %s)`
- `GIT_SYNC_MAX_WAIT`: `harus di antara 1s dan GIT_SYNC_MAX_TOTAL_WAIT (%s)`
- `GIT_SYNC_TIMEOUT`: `harus lebih besar dari GIT_SYNC_MAX_TOTAL_WAIT (%s)` (pesan lama terhadap `HTTP_DEFAULT_TIMEOUT` tetap)

Helper baru di `config.go`: `func (l *loader) durBetween(env string, def, min, max time.Duration) time.Duration` (parse, gagal bila di luar `[min, max]`, kembalikan `def` saat gagal). Log dump config (blok `slog` sekitar baris 500) menambah tiga field baru.

### 5.2 Pacer (infra/gitprovider/pacer.go, baru)

```go
type WaitReason string
const (
    WaitRateLimitExhausted WaitReason = "rate_limit_exhausted" // proaktif: remaining=0
    WaitRateLimited        WaitReason = "rate_limited"         // provider membalas 429/403-rl
)
type PaceOptions struct {
    Interval, MaxWait, MaxTotalWait time.Duration
    MaxRetries int // default maxRateLimitRetries = 3
}
type Wait  struct{ Duration time.Duration; Reason WaitReason }
type Stats struct{ Waits int; TotalWait time.Duration }

func (c *Client) Session(onWait func(Wait)) *Session // onWait boleh nil
func (s *Session) ListTree(ctx context.Context, repo Repo, folder string, token secret.String) ([]TreeEntry, error)
func (s *Session) FetchBlob(ctx context.Context, repo Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error)
func (s *Session) Stats() Stats
```

- `Client` menyimpan `pace PaceOptions` (diisi `NewFromConfig` dari config; `New` menerima lewat `Options.Pace`). Field tidak diekspor `now func() time.Time` dan `sleep func(ctx, d) error` diinjeksi untuk tes.
- Refaktor internal: `githubTree`, `gitlabTree`, dan isi `FetchBlob` menerima fungsi `getFn` (signature `get` sekarang). `Client` memberi `c.get`, `Session` memberi `s.get`. Tidak ada duplikasi logika provider.
- `c.send(ctx, p, u, token, accept) (*http.Response, error)` mengembalikan respons apa pun statusnya. `c.get` = `send` + `mapStatus` (perilaku lama).
- Algoritme `s.get` per request:
  1. Di bawah mutex: `start = max(now, nextSlot, blockedUntil)`; `nextSlot = start + Interval`; `rl = max(0, blockedUntil - now)`. Bila `rl > MaxWait` atau `totalWait + rl > MaxTotalWait` -> kembalikan `&RateLimitedError{RetryAfter: rl}` tanpa tidur. Bila `rl > 0`: `totalWait += rl`, `waits++`, catat `reason` terakhir.
  2. Di luar mutex: bila `rl > 0` panggil `onWait(Wait{rl, reason})`; lalu `sleep(ctx, start - now)`; ctx batal -> `ctx.Err()`.
  3. `send`; baca `parseRateHeaders(resp.Header, now())` untuk 2xx maupun non-2xx. Bila `remaining == 0` dan `reset` diketahui: `blockedUntil = max(blockedUntil, reset)` dengan reason `rate_limit_exhausted`.
  4. Non-2xx: `mapStatus`. Bila `RateLimitedError` dan `retry < MaxRetries`: `blockedUntil = now + RetryAfter` (bila 0: backoff 1s, 2s, 4s), reason `rate_limited`, ulangi dari langkah 1. Retry habis -> kembalikan `RateLimitedError`.
- `parseRateHeaders(h http.Header, now time.Time) (remaining int, hasRemaining bool, reset time.Time, retryAfter time.Duration)`: remaining dari `X-RateLimit-Remaining` lalu `RateLimit-Remaining`; reset dari `X-RateLimit-Reset` lalu `RateLimit-Reset` (nilai > 1e9 = epoch detik, selain itu delta detik); `Retry-After` detik. Semua durasi di-clamp `[0, 24h]`. `retryAfter()` lama dihapus dan `mapStatus` memakai helper ini.
- Token tidak pernah masuk error/log (aturan T14 tetap).
- `NewFromConfig`: `safehttp.Options.Timeout = min(cfg.GitSyncTimeout, 2*time.Minute)`.

### 5.3 Sink dan event SSE (runsync)

`runsync/params.go`:

```go
// Sink menerima progress sync. Implementasi WAJIB aman dipakai bersamaan dan tidak boleh memblok.
type Sink interface {
    Start(total int)
    Progress(done, total int, path string)
    Waiting(retryAfterSeconds int, reason string)
    Saving()
}
type NopSink struct{} // jalur JSON
```

| Event | Data JSON | Kapan |
|---|---|---|
| `start` | `{"total": 42}` | setelah `SelectFiles` + `CheckBatchLimits` (`total = len(cands)`) |
| `progress` | `{"done": 7, "total": 42, "path": "docs/auth/login-flow.md"}` | sekali per kandidat, setelah blob diambil atau ditandai tooLarge; `done` atomic; `path = cand.RepoPath` |
| `waiting` | `{"retry_after_seconds": 38, "reason": "rate_limit_exhausted"}` | dari `onWait`; detik dibulatkan ke atas |
| `saving` | `{}` | sebelum panggilan bus upsert (tidak dikirim bila tidak ada kandidat) |
| `done` | `RunResponse` T14 `{"report": ..., "config": ...}` | event terakhir bila sukses/partial |
| `error` | `{"code": "...", "message": "...", "details": ...}` | event terakhir bila gagal setelah `Start` |

Format: `event: <nama>\ndata: <json satu baris>\n\n`. Heartbeat `: ping\n\n` tiap 15 detik (konstanta, bisa diinjeksi di tes). Header: `Content-Type: text/event-stream; charset=utf-8`, `Cache-Control: no-store`, `X-Accel-Buffering: no`, `Connection: keep-alive`; tanpa `Content-Length`. Event tidak pernah memuat PAT, URL berkredensial/berquery, atau isi file. Error internal tetap `terjadi kesalahan internal`.

### 5.4 Store (base/store)

```go
// LockKeySQL: kunci advisory (project, kind); $1 = project_id, $2 = kind.
const LockKeySQL = "hashtext('grimoire.gitsync.' || $1::text || '.' || $2::text)"

// IsLocked: true bila advisory lock (project, kind) sedang dipegang sesi mana pun. Tidak mengambil lock.
func (s *Store) IsLocked(ctx context.Context, projectID string, kind base.Kind) (bool, error)
```

Query `IsLocked` (`projectID` bukan uuid -> `false, nil` tanpa query):

```sql
WITH k AS (SELECT (hashtext('grimoire.gitsync.' || $1::text || '.' || $2::text))::bigint AS v)
SELECT EXISTS (
  SELECT 1 FROM pg_locks, k
  WHERE locktype = 'advisory' AND granted AND objsubid = 1
    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
    AND classid = ((k.v >> 32) & 4294967295)::oid
    AND objid   = (k.v & 4294967295)::oid)
```

(Teks hashtext dibangun dari `LockKeySQL`, bukan disalin.) `hashtext` bertipe int4; cast ke bigint memberi sign-extension yang sama dengan `pg_try_advisory_lock(bigint)`, sehingga hash negatif juga cocok (classid = 4294967295).

### 5.5 Audit (base/audit)

`audit.Event` + `WaitCount int`, `WaitTotal time.Duration`. `Result` `git_sync.run` + `"wait_count": ev.WaitCount`, `"wait_total_seconds": int(math.Ceil(ev.WaitTotal.Seconds()))`. Aksi audit tidak bertambah.

---

## 6. Plan (langkah berurutan)

Kerjakan berurutan; F1 sampai F5 adalah foundation dan harus hijau (`go build ./...`, `go test ./internal/infra/... ./internal/domain/project/internal/gitsync/...`) sebelum plan usecase dikerjakan.

### F1. Config env (infra/config)
1. `config.go`: field `GitSyncRequestInterval`, `GitSyncMaxWait`, `GitSyncMaxTotalWait`; helper `durBetween`; tiga baris log dump.
2. `git.go` `loadGit`: urutan baca `MAX_TOTAL_WAIT` (`l.dur`, default 5m) -> `MAX_WAIT` (`durBetween(1s, MaxTotalWait)`) -> `REQUEST_INTERVAL` (`durBetween(0, 10s)`) -> `TIMEOUT` (default 15m; validasi lama + `> MaxTotalWait`).
3. `config_test.go`: default, batas bawah/atas tiap env, interval `0` diterima, `MAX_WAIT > MAX_TOTAL_WAIT` gagal, `TIMEOUT <= MAX_TOTAL_WAIT` gagal, pesan gagal tepat.
4. `docs/configuration.md` (tiga baris baru, default `GIT_SYNC_TIMEOUT` = `15m`, catatan pacing) dan `.env.example` (`# GIT_SYNC_REQUEST_INTERVAL=500ms`, `# GIT_SYNC_MAX_WAIT=120s`, `# GIT_SYNC_MAX_TOTAL_WAIT=5m`, `# GIT_SYNC_TIMEOUT=15m`). `docs_test.go`/`envexample_test.go` harus hijau.

### F2. Pacer (infra/gitprovider)
1. `pacer.go` baru sesuai §5.2 (`PaceOptions`, `Wait`, `WaitReason`, `Stats`, `Session`, `parseRateHeaders`, konstanta `maxRateLimitRetries = 3`, `maxHeaderWait = 24 * time.Hour`).
2. `client.go`: `Options.Pace`, field `pace/now/sleep`, `send` + `get`, `getFn` di jalur tree/blob, `Session(onWait)`, timeout safehttp `min(GitSyncTimeout, 2m)`, hapus `retryAfter`.
3. `github.go`/`gitlab.go`: tree memakai `getFn` yang diteruskan.
4. `pacer_test.go` (httptest + `now`/`sleep` palsu, tanpa `time.Sleep` nyata): lihat §9.1.

### F3. Deteksi Flush (infra/middleware)
1. `writer.go`: tambah `func (w *statusWriter) FlushError() error`. Urutan: (a) cek writer bawah mendukung flush (`http.Flusher`, `interface{ FlushError() error }`, atau rantai `Unwrap()`); bila tidak, kembalikan `http.ErrNotSupported` TANPA menulis header; (b) bila didukung dan header belum ditulis, `w.WriteHeader(http.StatusOK)`; (c) kembalikan `http.NewResponseController(w.ResponseWriter).Flush()`. `Flush()` lama dipertahankan. Karena `ResponseController` memprioritaskan `FlushError`, handler bisa mendeteksi flush tidak didukung lalu membalas 500 sebelum header SSE ditulis.
2. `writer_test.go`: writer bawah tanpa Flusher -> `ResponseController.Flush()` = `http.ErrNotSupported` dan header belum ditulis; dengan `httptest.ResponseRecorder` -> nil dan `Flushed == true`.

### F4. base/store + base/audit (module gitsync)
1. `base/store/store.go`: `LockKeySQL`, `IsLocked` (§5.4). Paket store tetap satu-satunya pemilik teks kunci lock.
2. `runsync/repository.go`: hapus `lockKeySQL`, pakai `store.LockKeySQL` (perilaku identik).
3. `base/audit/audit.go`: field + result baru (§5.5). `audit_test.go`: result memuat `wait_count` dan `wait_total_seconds` (pembulatan ke atas, 0 bila tidak menunggu).
4. `base/store/store_test.go` (unit): `IsLocked` dengan projectID bukan uuid -> false tanpa query. Integration (tag `integration`) ada di plan getconfig.

### F5. Wiring module (gitsync/base.go, base/actor.go)
1. `base.Shared.Git` tetap `*gitprovider.Client` (pacing ikut dari config lewat `NewFromConfig`).
2. Runsync membungkus Client dengan adapter `gitAdapter` (`Policy()`, `Session(onWait) GitSession`) di `runsync/init.go`, karena `Client.Session` mengembalikan `*gitprovider.Session`.
3. Route runsync: `ExtendTimeout(d.Config.GitSyncTimeout + 30*time.Second)`; perbarui doc comment `gitsync/base.go`.

### U1. usecase-runsync (plan terpisah)
Backend `runsync/{params,usecase,handler,init,repository}.go` + tes; frontend `runsync/*` + `shared/services/{http,sse}.ts` + i18n; OpenAPI run. Rincian di `usecase-runsync.md`. Ringkasan wajib:
- `GitClient{ Policy(); Session(onWait func(gitprovider.Wait)) GitSession }`, `GitSession{ ListTree; FetchBlob; Stats() gitprovider.Stats }`.
- `func (u *Usecase) Start(ctx, projectID string, in RunRequest, meta base.RequestMeta) (*Run, error)`; `func (r *Run) Execute(ctx context.Context, sink Sink) (RunResponse, error)` (unlock lewat defer, dipanggil maksimal sekali); `func (r *Run) Abort(ctx context.Context)` (idempoten, `sync.Once` bersama unlock).
- `mapError`: `RateLimitedError` -> `RATE_LIMITED` + `details.retry_after_seconds = ceil(RetryAfter)`; pesan tetap `provider Git membatasi permintaan; coba lagi nanti`.
- Handler: negosiasi Accept, jalur JSON (Start -> Execute ctx lepas -> JSON), jalur SSE (Start error -> JSON; sukses -> cek Flush (gagal: `Abort` + 500 JSON) -> header SSE -> goroutine Execute -> loop select event/hasil/heartbeat/`r.Context().Done()`).

### U2. usecase-getconfig (plan terpisah)
`getconfig/{usecase,repository,handler}.go` + tes; FE `getconfig/{types,stores}` + banner di dialog; OpenAPI `in_progress`. Rincian di `usecase-getconfig.md`.

### F6. Dokumen dan bundle (penutup, setelah U1+U2)
1. `api/modules/project/git-sync.yml`: `info.version: 0.2.0`; run 200 menambah `text/event-stream` (skema `GitSyncEvent*`, §11.7); deskripsi `GIT_SYNC_TIMEOUT` default 15 menit; `GitSyncConfigResponse.in_progress` (required, boolean). `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau.
2. `docs/tickets/README.md`: katalog audit (`git_sync.run` result + `wait_count`, `wait_total_seconds`), catatan kontrak T17 (tanpa bus baru; env baru), status T17.
3. `docs/tickets/T17-project-git-sync-progress.md`: bagian "Keputusan plan" yang menjawab pertanyaan terbuka (D1, D3, D4, D5, D11) dan mencatat perubahan terhadap T14 (default `GIT_SYNC_TIMEOUT`, pecah usecase, `in_progress`).

---

## 7. Migration

Tidak ada. `git_sync_configs.last_result` (jsonb) dan advisory lock sudah ada sejak T14. `in_progress` dibaca dari `pg_locks`, bukan kolom baru.

---

## 8. Changes (per file/folder)

| File | Jenis | Isi | Pemilik |
|---|---|---|---|
| `backend/internal/infra/config/config.go` | ubah | 3 field, `durBetween`, log dump | F1 |
| `backend/internal/infra/config/git.go` | ubah | baca + validasi env baru, default TIMEOUT 15m | F1 |
| `backend/internal/infra/config/config_test.go` | ubah | tes env baru | F1 |
| `docs/configuration.md`, `.env.example` | ubah | env baru + default baru | F1 |
| `backend/internal/infra/gitprovider/pacer.go` | baru | Session, PaceOptions, Wait, Stats, parseRateHeaders | F2 |
| `backend/internal/infra/gitprovider/pacer_test.go` | baru | tes pacing | F2 |
| `backend/internal/infra/gitprovider/client.go` | ubah | `send`/`get`, `getFn`, `Session`, `Options.Pace`, timeout per request | F2 |
| `backend/internal/infra/gitprovider/{github,gitlab}.go` | ubah | tree memakai `getFn` | F2 |
| `backend/internal/infra/gitprovider/gitprovider_test.go` | ubah | regresi: Client lama tanpa pacing, mapStatus via helper | F2 |
| `backend/internal/infra/middleware/writer.go` (+ `_test.go`) | ubah | `FlushError` | F3 |
| `.../gitsync/base/store/store.go` (+ `_test.go`) | ubah | `LockKeySQL`, `IsLocked` | F4 |
| `.../gitsync/base/audit/audit.go` (+ `_test.go`) | ubah | `WaitCount`, `WaitTotal` | F4 |
| `.../gitsync/runsync/repository.go` | ubah | pakai `store.LockKeySQL` | F4 |
| `.../gitsync/base.go` | ubah | doc comment route | F5 |
| `.../gitsync/runsync/{params,usecase,handler,init}.go` + tes | ubah | Sink, Start/Execute/Abort, SSE | U1 |
| `.../gitsync/getconfig/{usecase,repository}.go` + tes | ubah | `InProgress`, `in_progress` | U2 |
| `frontend/src/shared/services/http.ts` (+ test) | ubah | `openStream` | U1 |
| `frontend/src/shared/services/sse.ts` (+ test) | baru | parser SSE tanpa dependensi | U1 |
| `frontend/src/domain/project/git-sync/runsync/**` | ubah/baru | service stream, store tahap, `SyncProgress.svelte`, dialog | U1 |
| `frontend/src/domain/project/git-sync/getconfig/**` | ubah | zod `in_progress` default false, store | U2 |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | kunci `gitSync.progress.*`, `gitSync.inProgress.*` | U1, U2 |
| `frontend/e2e/fixtures/fake-git-server.ts`, `e2e/git-sync.spec.ts` | ubah | mode rate limit + skenario progress | U1 |
| `api/modules/project/git-sync.yml`, `api/openapi.yml` | ubah | SSE + `in_progress` | F6 |
| `docs/tickets/README.md`, `docs/tickets/T17-*.md` | ubah | katalog audit, keputusan | F6 |

Tidak disentuh: `lib/constants/bus.go`, `lib/contracts/**`, `migrations/**`, `core/gitcredential/**`, module knowledge/apispec.

---

## 9. Test Scenario (foundation; tes usecase ada di plan masing-masing)

### 9.1 `infra/gitprovider/pacer_test.go` (httptest, `now`/`sleep` palsu)
1. Jatah longgar (`remaining=4999`), interval 0: tidak ada panggilan `sleep` dengan durasi > 0, `onWait` tidak dipanggil, `Stats{0,0}`.
2. Interval 500ms, 4 goroutine x 5 request: awal request tercatat berjarak >= 500ms (berdasarkan jam palsu); `onWait` tidak dipanggil.
3. Respons pertama `X-RateLimit-Remaining: 0`, `X-RateLimit-Reset: now+30` (epoch): request berikutnya tidur 30s, `onWait` sekali dengan `{30s, rate_limit_exhausted}`, `Stats{1, 30s}`.
4. 429 `Retry-After: 2` lalu 200: hasil sukses, `onWait {2s, rate_limited}`, server menerima 2 request.
5. 403 + `X-RateLimit-Remaining: 0` tanpa reset: backoff 1s lalu sukses.
6. 429 empat kali berturut: `RateLimitedError` setelah 3 retry (4 request total).
7. Reset 300s dengan `MaxWait` 120s: `RateLimitedError{RetryAfter: 300s}` seketika, `sleep` tidak dipanggil, `onWait` tidak dipanggil.
8. Tiga tunggu 100s dengan `MaxTotalWait` 250s: tunggu ketiga gagal `RateLimitedError` tanpa tidur; `Stats{2, 200s}`.
9. ctx dibatalkan saat `sleep`: kembali `context.Canceled`, request tidak dikirim.
10. `RateLimit-Reset: 30` (delta) dan `X-RateLimit-Reset: <epoch>` menghasilkan reset yang sama.
11. Header negatif (`Retry-After: -5`) -> 0 (memakai backoff); sangat besar (`Retry-After: 999999999`) di-clamp 24h lalu gagal `MaxWait`.
12. 403 tanpa header rate limit -> `ErrForbidden`, tanpa retry.
13. GitLab tree 3 halaman: tiap halaman melewati pacing (3 slot interval).
14. Token canary: token unik tidak muncul di `err.Error()` pada semua jalur gagal.
15. `Client.ListTree/FetchBlob/VerifyToken` lama tidak terkena interval (regresi).
16. `go test -race` hijau untuk tes konkuren.

### 9.2 `infra/config`
Default (`500ms`, `120s`, `5m`, `15m`); `GIT_SYNC_REQUEST_INTERVAL=0` diterima; `11s` dan `-1s` ditolak; `GIT_SYNC_MAX_WAIT=500ms` ditolak; `MAX_WAIT=6m` dengan `MAX_TOTAL_WAIT=5m` ditolak; `GIT_SYNC_TIMEOUT=4m` dengan `MAX_TOTAL_WAIT=5m` ditolak; pesan persis §5.1; docs/env-example sinkron.

### 9.3 `infra/middleware/writer_test.go`
Lihat F3.2. Tambahan: rantai `accesslog -> Timeout -> ExtendTimeout` dengan `httptest.Server` nyata, handler menulis 2 event dengan Flush di antaranya; klien membaca event pertama sebelum handler menulis event kedua.

### 9.4 `base/audit`, `base/store`
Audit result baru; `IsLocked` uuid tidak valid -> false tanpa query. Integration `IsLocked` (hash positif dan negatif, kind/project lain) di plan getconfig; integration `TryLock` memakai `store.LockKeySQL` di plan runsync.

---

## 10. E2E Scenarios (Playwright, `frontend/e2e/git-sync.spec.ts` + fake git server)

Fake server ditambah opsi per tes: `rateLimit: { afterRequests: n, resetSeconds: s }` (mengirim `remaining=0` + `RateLimit-Reset`), `status429Once`, dan `delayMs` per blob. E2E memakai env server `GIT_SYNC_REQUEST_INTERVAL=0s`, `GIT_SYNC_MAX_WAIT=3s`, `GIT_SYNC_MAX_TOTAL_WAIT=5s` agar cepat.

Positif:
1. Sync knowledge 5 file: dialog menampilkan "Mengambil daftar file…", lalu progress `n dari 5` dengan nama file mono, lalu "Menyimpan…", lalu laporan per file identik T14.
2. Rate limit setelah request ke-2 dengan reset 2s: muncul "Menunggu batas request provider, lanjut dalam 2 detik", hitung mundur turun, bar tidak berwarna error, lalu sync selesai sukses.
3. 429 sekali: sync tetap sukses, laporan sama dengan tanpa 429.
4. Tutup dialog saat berjalan (blob lambat): peringatan muncul; buka lagi -> banner "Sync sedang berjalan di server", tombol Sync nonaktif; setelah selesai, "Muat ulang status" menampilkan hasil terakhir dari `last_result`.
5. Klien lama (request tanpa `Accept: text/event-stream` lewat `request.post` Playwright) tetap menerima JSON T14.

Negatif:
6. Reset 60s (> `MAX_WAIT` 3s): event `error` `RATE_LIMITED` dengan `retry_after_seconds: 60`; UI memakai pesan yang ada + tombol "Coba lagi"; tidak ada file baru di tab Knowledge.
7. Dua tab menekan Sync bersamaan: tab kedua menerima 409 `SYNC_IN_PROGRESS` sebagai respons JSON (bukan stream).
8. Token belum disimpan: 422 `GIT_CREDENTIAL_MISSING` sebagai JSON sebelum stream, UI menampilkan `CredentialNotice` seperti T14.
9. Isi stream yang direkam tidak memuat token canary maupun isi file.

---

## 11. Request/Response body

### 11.1 Request (tidak berubah)

```http
POST /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync/run
Content-Type: application/json
Accept: text/event-stream
X-Requested-With: grimoire

{"kind":"knowledge","url":"https://github.com/noobee/grimoire-docs/tree/main/docs","branch":"","folder":"","target_prefix":"handbook"}
```

### 11.2 Respons SSE sukses (200)

```
HTTP/1.1 200 OK
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-store
X-Accel-Buffering: no

event: start
data: {"total":3}

event: progress
data: {"done":1,"total":3,"path":"docs/auth/login-flow.md"}

event: waiting
data: {"retry_after_seconds":38,"reason":"rate_limit_exhausted"}

: ping

event: progress
data: {"done":2,"total":3,"path":"docs/auth/session-cookie.md"}

event: progress
data: {"done":3,"total":3,"path":"docs/onboarding/setup-local.md"}

event: saving
data: {}

event: done
data: {"report":{"status":"success","summary":{"created":2,"updated":1,"unchanged":0,"skipped":0,"rejected":0},"items":[{"repo_path":"docs/auth/login-flow.md","target":"handbook/auth/login-flow.md","status":"created","reason_code":"","reason":"","issues":[]},{"repo_path":"docs/auth/session-cookie.md","target":"handbook/auth/session-cookie.md","status":"updated","reason_code":"","reason":"","issues":[]},{"repo_path":"docs/onboarding/setup-local.md","target":"handbook/onboarding/setup-local.md","status":"created","reason_code":"","reason":"","issues":[]}],"truncated":false,"error_code":"","error_message":""},"config":{"provider":"github","host":"github.com","repo_url":"https://github.com/noobee/grimoire-docs","repo_path":"noobee/grimoire-docs","branch":"main","folder":"docs","target_prefix":"handbook","last_status":"success","last_synced_at":"2026-10-22T08:14:03Z","last_synced_by":{"id":"5d2c9a7e-1b44-4f0b-8c1e-2a9d7f3e6b21","label":"Reyhan"},"last_result":null}}

```

### 11.3 Respons SSE gagal setelah Start (200, event terakhir `error`)

```
event: start
data: {"total":120}

event: progress
data: {"done":61,"total":120,"path":"docs/api/errors.md"}

event: error
data: {"code":"RATE_LIMITED","message":"provider Git membatasi permintaan; coba lagi nanti","details":{"retry_after_seconds":1712}}

```

Kode lain yang mungkin di event `error`: `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_NOT_FOUND`, `GIT_TREE_TOO_LARGE`, `SYNC_LIMIT_EXCEEDED`, `GIT_UPSTREAM_ERROR`, `TIMEOUT`, `INVALID_ARGUMENT`, `INTERNAL` (pesan generik). `details` = `null` bila tidak ada.

### 11.4 Error sebelum stream (JSON, jalur SSE maupun JSON)

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":{"code":"SYNC_IN_PROGRESS","message":"sync lain untuk project dan jenis ini sedang berjalan","details":{"kind":"knowledge"}},"trace_id":"7f3c2a9e4b1d4c08"}
```

```http
HTTP/1.1 422 Unprocessable Entity

{"error":{"code":"GIT_CREDENTIAL_MISSING","message":"...","details":{"provider":"github","host":"github.com","settings_path":"/profile#git-credentials"}},"trace_id":"7f3c2a9e4b1d4c08"}
```

(Pesan/details mengikuti konstanta T14 di `gitsync/base/errors.go`.)

### 11.5 Jalur JSON (tanpa `Accept: text/event-stream`)

Sama dengan T14: 200 `{"report":...,"config":...}`. Gagal batas tunggu: 429 + header `Retry-After: 1712`:

```json
{"error":{"code":"RATE_LIMITED","message":"provider Git membatasi permintaan; coba lagi nanti","details":{"retry_after_seconds":1712}},"trace_id":"7f3c2a9e4b1d4c08"}
```

### 11.6 getconfig

```http
GET /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync?kind=knowledge
```

```json
{"kind":"knowledge","config":{"provider":"github","host":"github.com","repo_url":"https://github.com/noobee/grimoire-docs","repo_path":"noobee/grimoire-docs","branch":"main","folder":"docs","target_prefix":"handbook","last_status":"success","last_synced_at":"2026-10-22T08:14:03Z","last_synced_by":{"id":"5d2c9a7e-1b44-4f0b-8c1e-2a9d7f3e6b21","label":"Reyhan"},"last_result":null},"has_credential":true,"credential_unreadable":false,"feature_enabled":true,"in_progress":true}
```

Sync pertama sedang berjalan (config belum tersimpan atau baru dibuat): `{"kind":"api_spec","config":null,"has_credential":false,"credential_unreadable":false,"feature_enabled":true,"in_progress":true}`.

### 11.7 Skema OpenAPI event (untuk F6)

`GitSyncEventStart {total:int}`, `GitSyncEventProgress {done:int,total:int,path:string}`, `GitSyncEventWaiting {retry_after_seconds:int, reason: enum[rate_limit_exhausted, rate_limited]}`, `GitSyncEventSaving {}`, `GitSyncEventDone = RunGitSyncResponse`, `GitSyncEventError = Error.error`. Respons `text/event-stream` diberi `schema: {type: string}` + `description` yang menjelaskan format dan merujuk skema event, plus `example` seperti §11.2.

---

## 12. Frontend bersama (dirinci di plan usecase)

- `shared/services/http.ts`: `openStream(method, url, { body, signal, headers }): Promise<Response>` memakai `credentials`, `CSRF_HEADER`, `Content-Type` yang sama dengan `request`; `Accept: text/event-stream`. Non-2xx: baca teks -> `toApiError` (401 -> `notifyUnauthorized`), sehingga `toSyncError` lama tetap berlaku.
- `shared/services/sse.ts`: `createSseParser(onEvent: (name: string, data: string) => void)` dengan `push(chunk: string)`; tahan chunk terpotong, `\r\n`, baris komentar (`:`) diabaikan, `data:` multi-baris digabung `\n`.
- Idle timeout 45s (reset tiap byte, termasuk heartbeat) menggantikan `CLIENT_TIMEOUT_MS`. Abort stream tidak membatalkan server.
- Zod: event tak dikenal diabaikan; `getconfig` `in_progress: z.boolean().default(false)`.

### Visual (wajib; dirinci per layar di plan usecase)

| Layar | Pertanyaan | Pola DESIGN.md |
|---|---|---|
| Dialog Sync, tahap berjalan | "Sudah sampai mana sync ini, dan apakah sedang menunggu?" | `Dialog` (`shadow-pop`, radius 16) yang ada; blok progress menggantikan form (form disembunyikan saat berjalan) |
| Dialog Sync, dibuka ulang saat sync di server | "Apakah sync masih jalan, perlu saya tunggu?" | banner netral di atas form, tombol Sync nonaktif, aksi sekunder "Muat ulang status" |

- Elemen khas: progress bar tinggi 8px, radius penuh, kedua ujung berlabel (`7 dari 42` kiri, `17%` kanan, `tabular-nums`), isian `bg-accent` (aksen kuning untuk isian, sesuai "Wajah produk" #3), track `bg-raised`. Nama file terakhir di bawah bar dalam IBM Plex Mono 12px `text-muted`, dipotong di tengah (`docs/…/login-flow.md`). Tahap listing: bar indeterminate statis bergaris tanpa animasi masuk (hormati `prefers-reduced-motion`), teks "Mengambil daftar file…".
- Menunggu: bar membeku dan isian berubah ke `bg-border` (bukan merah); baris status `text-warning` dengan ikon jam + teks "Menunggu batas request provider, lanjut dalam 38 detik" (status tidak hanya warna). Hitung mundur di `aria-live="polite"` maksimal tiap 5 detik untuk pembaca layar.
- Menyimpan: teks "Menyimpan…", bar penuh.
- Hasil: `ReportSummary` + `SyncReportTable` T14 tanpa perubahan. Error: `SyncErrorBlock` + tombol sekunder "Coba lagi" (aktif setelah `retry_after_seconds` habis, memakai `cooldown` yang ada).
- Peringatan tutup: dialog konfirmasi kecil "Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi." dengan satu tombol utama "Tutup" (near-black) dan "Tetap di sini".
- Token: `bg-accent`, `bg-raised`, `bg-border`, `text-muted`, `text-warning`, `bg-pill-neutral-*` (banner in_progress), `rounded-base` (kontrol), radius penuh hanya untuk bar. Transisi lebar bar 120ms ease-out.
- Data contoh (fixture vitest/storybook): project "Grimoire Docs", repo `noobee/grimoire-docs`, total 42, path `docs/auth/login-flow.md`, `docs/onboarding/setup-local.md`, tunggu 38 detik, `retry_after_seconds` 1712. Dilarang Acme/lorem.
- Teks UI bahasa Indonesia; kunci baru di `frontend/src/shared/i18n/{id,en}.json`.

---

## 13. Urutan implementasi

1. F1 config -> F2 pacer -> F3 writer -> F4 store/audit -> F5 wiring (berurutan; F2 dan F3 boleh paralel setelah F1).
2. U1 runsync dan U2 getconfig paralel setelah F5 (U2 hanya butuh F4). Di dalam U1: backend usecase -> handler -> FE `shared/services` -> FE runsync -> e2e.
3. F6 OpenAPI + bundle + dokumen tiket.
4. Verifikasi: `go test ./...`, `go test -tags integration ./internal/domain/project/internal/gitsync/...`, `go test -race ./internal/infra/gitprovider/...`, `npm run test`, `npm run depcruise`, `npm run test:arch`, `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src`, `make api-check`, e2e `git-sync.spec.ts`.

---

## 14. Daftar usecase

| Slug | Feature | Plan | Ringkasan |
|---|---|---|---|
| `runsync` | `gitsync/runsync` (revisi) | `docs/superpowers/plans/project-git-sync-progress/usecase-runsync.md` | Session ber-pacing, Start/Execute/Abort, Sink, SSE, ctx lepas, audit tunggu, FE stream + progress |
| `getconfig` | `gitsync/getconfig` (revisi) | `docs/superpowers/plans/project-git-sync-progress/usecase-getconfig.md` | `in_progress` dari `pg_locks` (fail-soft), FE banner + muat ulang |

---

## 15. Risiko dan mitigasi

| Risiko | Mitigasi |
|---|---|
| Proxy menahan SSE | `X-Accel-Buffering: no`, heartbeat 15s, tes lewat router nyata (§9.3) |
| Goroutine sync tak terkendali setelah klien pergi | ctx lepas tetap dibatasi `GIT_SYNC_TIMEOUT`; lock dilepas lewat defer |
| Header provider ekstrem | clamp 24h + `MAX_WAIT`/`MAX_TOTAL_WAIT` |
| Event menyusul `done` | kuras kanal event sebelum menulis `done`/`error` (D8) |
| `in_progress` salah karena hash negatif | cast bigint + mask 32 bit, diuji dua tanda (U2) |
| Sync lebih lama karena interval 500ms | dapat diatur env; `0` mematikan jeda |
