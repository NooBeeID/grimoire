# T01 core/auth — Usecase `purgesessions` (job `core.auth.purge-sessions`)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (U15, gelombang C). Prasyarat dari foundation F1: migrasi `20261021100000000_create_auth_tables.sql` (tabel `sessions`, `login_throttles` beserta indeksnya), `base.Shared` (`Clock`, `Session.IdleTimeout`, `Throttle.Window`), `base.LoginThrottle.Expired`, `auth/base.go` dengan pola `<feature>.Init(d, shared)`. Konvensi: `docs/tickets/README.md` dan `rules.txt`. Overview menang bila ada konflik. Pola job mengikuti `docs/superpowers/plans/core-audit-log/usecase-purgeexpired.md`.

## Metadata

| Field | Nilai |
|---|---|
| Endpoint | job `core.auth.purge-sessions` (bukan HTTP, bukan bus) |
| Method | `DailyAt: "03:00"` (UTC), dijalankan `infra/jobs` dengan `pg_try_advisory_lock` per nama job |
| Version | tidak berlaku (tidak ada kontrak REST/bus; tanpa prefix versi) |
| Scope | backend saja (tanpa FE) |
| Domain | core |
| Module | auth |
| Feature | `purgesessions` (U15, gelombang C) |
| Akses | sistem (scheduler internal); tidak ada route, tidak ada identitas pemanggil |
| Bus | tidak menyediakan, tidak mengonsumsi. Tidak memanggil `core.audit.record` (pembersihan rutin tidak ada di katalog audit; hanya log jumlah) |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/purgesessions/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `purgesessions.Init`) |

## Summary Plan

Job harian yang menghapus dua jenis baris kedaluwarsa:

1. `sessions` yang `expires_at < now` (batas absolut) ATAU `last_seen_at < now - SESSION_IDLE_TIMEOUT` (batas idle).
2. `login_throttles` yang jendelanya sudah lewat (`window_started_at < now - LOGIN_FAILURE_WINDOW`) DAN tidak sedang terkunci (`locked_until IS NULL OR locked_until < now`).

Setelah selesai, job me-log `deleted_sessions`, `deleted_throttles`, `duration_ms`. Keputusan:

- Semua batas waktu dihitung di Go dari `base.Clock` (`now`, `idleCutoff = now - IdleTimeout`, `throttleCutoff = now - Throttle.Window`) lalu dikirim sebagai parameter `$n`. Tidak ada konversi `time.Duration` ke `interval` di SQL, dan test bisa membekukan waktu. `now` dihitung SEKALI per run.
- Perbandingan strict (`<`), sejalan dengan `Session.IsValid` (valid bila `now < ExpiresAt` dan `now < LastSeenAt + Idle`): sesi tepat pada batas boleh bertahan sampai run berikutnya, dan memang sudah tidak valid; tidak ada sesi valid yang terhapus.
- Aman terhadap race dengan request: sesi yang dihapus job sudah tidak valid, sedangkan `resolvesession` menyentuh `last_seen_at` hanya untuk sesi valid. Throttle yang dihapus hanya yang jendela dan lock-nya sudah lewat, setara dengan "tidak pernah gagal" bagi `RegisterFailure` (ia me-reset jendela yang lewat), sehingga penghapusan tidak mengubah perilaku login.
- Dua `DELETE` independen, masing-masing auto-commit, tanpa transaksi bersama. Keduanya idempoten dan tidak saling bergantung, jadi atomisitas tidak diperlukan. Bila yang pertama sukses lalu yang kedua gagal, usecase me-log jumlah yang sudah terhapus dan mengembalikan error (scheduler mencatat status "error"); run berikutnya melanjutkan.
- Tanpa batching: tabel kecil (skala tim, puluhan user; sesi dan throttle kedaluwarsa per hari ribuan paling banyak) dan ditopang indeks `sessions_expires_at_idx`, `sessions_last_seen_at_idx` (Postgres memakai BitmapOr untuk dua predikat OR) serta `login_throttles_window_idx`. Rule 8.8 tidak memicu kebutuhan batch. Bila volume tumbuh, batching ditambahkan kemudian tanpa mengubah kontrak.
- Idempoten: run ulang atau dua instance berurutan aman. Advisory lock scheduler hanya mencegah tumpang tindih.
- Satu usecase, satu file `Run`; tidak dipecah. Tidak ada rule bisnis baru di entity: kebijakan "throttle kedaluwarsa" sudah ada sebagai `base.LoginThrottle.Expired` (dipakai test sebagai oracle kesetaraan dengan SQL, bukan dipanggil di jalur produksi).
- Log hanya angka. Tanpa email, id, atau token (rule 14.2).
- Repository adalah satu-satunya yang menyentuh SQL, dan ia memegang kedua `DELETE` untuk fitur ini. Query bersama di `base/store` tidak dipakai (tidak ada query purge di sana).

## Plan

Urutan wajib. Semua file di `backend/internal/domain/core/internal/auth/purgesessions/`, `package purgesessions`. Import memakai path nyata T00 (`grimoire/internal/infra/app`, `grimoire/internal/infra/jobs`, `grimoire/internal/infra/db`); bila F1/T00 memindahkan folder, ikuti lokasi akhirnya, nama simbol tetap.

1. `repository.go` (satu-satunya SQL fitur ini)

   ```go
   type Repository struct{ q db.Querier }

   func NewRepository(q db.Querier) *Repository

   const deleteSessionsSQL = `DELETE FROM sessions
   WHERE expires_at < $1 OR last_seen_at < $2`

   const deleteThrottlesSQL = `DELETE FROM login_throttles
   WHERE window_started_at < $2
     AND (locked_until IS NULL OR locked_until < $1)`

   // DeleteExpiredSessions: $1 = now, $2 = idleCutoff. Mengembalikan RowsAffected.
   func (r *Repository) DeleteExpiredSessions(ctx context.Context, now, idleCutoff time.Time) (int64, error)

   // DeleteExpiredThrottles: $1 = now, $2 = windowCutoff. Mengembalikan RowsAffected.
   func (r *Repository) DeleteExpiredThrottles(ctx context.Context, now, windowCutoff time.Time) (int64, error)
   ```

   Error dibungkus `fmt.Errorf("purgesessions: hapus sessions: %w", err)` / `"...: hapus login_throttles: %w"`; `RowsAffected` gagal dibungkus serupa. Argumen hanya `$n`. Tanpa transaksi (`BeginTx` tidak dipakai). Catatan `$1`/`$2` dipakai bukan berurutan di query kedua; pgx menerima selama kedua parameter terpakai. Bila driver mengeluh tentang tipe parameter, tambahkan cast eksplisit `$1::timestamptz`, `$2::timestamptz`.

2. `usecase.go`

   ```go
   type repository interface {
       DeleteExpiredSessions(ctx context.Context, now, idleCutoff time.Time) (int64, error)
       DeleteExpiredThrottles(ctx context.Context, now, windowCutoff time.Time) (int64, error)
   }

   type Usecase struct {
       repo        repository
       clock       base.Clock
       idleTimeout time.Duration // Shared.Session.IdleTimeout
       window      time.Duration // Shared.Throttle.Window
       log         *slog.Logger
   }

   func New(repo repository, clock base.Clock, idle, window time.Duration, log *slog.Logger) *Usecase
   func (u *Usecase) Run(ctx context.Context) error
   ```

   `New` panic (kesalahan wiring) bila `idle <= 0` atau `window <= 0` agar job tidak pernah menghapus semua baris karena konfigurasi nol (config sudah memvalidasi > 0, jadi mustahil di runtime).

   `Run`:
   1. `start := u.clock()`; `idleCutoff := start.Add(-u.idleTimeout)`; `windowCutoff := start.Add(-u.window)`. Dihitung sekali.
   2. `if err := ctx.Err(); err != nil { return err }` sebelum query pertama.
   3. `ds, err := repo.DeleteExpiredSessions(ctx, start, idleCutoff)`. Gagal: log `Error`-free (scheduler sudah me-log error job), kembalikan error terbungkus; `deleted_throttles` tidak dijalankan.
   4. `dt, err := repo.DeleteExpiredThrottles(ctx, start, windowCutoff)`. Gagal: log info parsial (`deleted_sessions=ds`), kembalikan error terbungkus `fmt.Errorf("purgesessions: ...: %w", err)`.
   5. Sukses: `log.InfoContext(ctx, "auth purge sessions selesai", "deleted_sessions", ds, "deleted_throttles", dt, "duration_ms", time.Since(wall).Milliseconds())`. `duration_ms` memakai jam nyata (`wall := time.Now()` di awal), bukan `u.clock` yang bisa dibekukan test.
   6. Ctx dibatalkan di tengah: error ctx dikembalikan apa adanya (terbungkus, `errors.Is(err, context.Canceled)` benar).

   Tidak memanggil bus/audit. Tidak ada rule bisnis di usecase selain orkestrasi dua delete dan perhitungan cutoff.

3. `init.go` (menggantikan `handler.go` dan `params.go`; fitur ini tidak punya HTTP/bus, sesuai overview 3.3)

   ```go
   const (
       JobName = "core.auth.purge-sessions"
       DailyAt = "03:00"
   )

   // newJob dipisah agar test membaca Name/DailyAt tanpa scheduler.
   func newJob(run func(context.Context) error) jobs.Job {
       return jobs.Job{Name: JobName, DailyAt: DailyAt, Run: run}
   }

   func Init(d app.Deps, s base.Shared) {
       repo := NewRepository(d.DB)
       uc := New(repo, s.Clock, s.Session.IdleTimeout, s.Throttle.Window, d.Log)
       d.Jobs.Register(newJob(uc.Run))
   }
   ```

   Tepat satu dari `Every`/`DailyAt` terisi (di sini `DailyAt`). Nama duplikat atau Register setelah Start = panic scheduler (wiring error, disengaja). `Run` job menerima ctx yang dibatalkan saat Stop.

4. Wiring: tambah satu baris `purgesessions.Init(d, shared)` di `backend/internal/domain/core/internal/auth/base.go` (beserta import). Satu baris, untuk meminimalkan konflik merge dengan usecase lain.

5. Test (lihat Test Scenario): `usecase_test.go` (unit, repo palsu), `repository_integration_test.go` dan `init_integration_test.go` (tag/skip mengikuti pola `db.TestDB` T00).

6. Verifikasi lokal sebelum selesai:
   - `cd backend && go build ./... && go vet ./...`
   - `go test ./internal/domain/core/internal/auth/purgesessions/...`
   - dengan Postgres: `go test -tags integration ./internal/domain/core/internal/auth/purgesessions/...` (atau sesuai mekanisme `db.TestDB` yang dipakai T00)
   - `go test ./internal/archtest/...` (archtest `TestT00_AC4_RepoIsClean`: tidak ada import silang feature, SQL hanya di `repository.go`).
   - Grep: `grep -rniE "audit|bus\." .../purgesessions/` tidak boleh memuat pemakaian bus.

## Migration

Tidak ada. Memakai tabel dan indeks dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (foundation F1): `sessions_expires_at_idx`, `sessions_last_seen_at_idx`, `login_throttles_window_idx`. Role DB aplikasi butuh hak DELETE pada kedua tabel (sudah tercakup hak tabel biasa).

## Changes

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/auth/purgesessions/repository.go` | buat | `Repository`, `DeleteExpiredSessions`, `DeleteExpiredThrottles` |
| `.../purgesessions/usecase.go` | buat | interface `repository`, `Usecase`, `New`, `Run` |
| `.../purgesessions/init.go` | buat | `JobName`, `DailyAt`, `newJob`, `Init(d, s)` |
| `.../purgesessions/usecase_test.go` | buat | unit dengan repo palsu |
| `.../purgesessions/repository_integration_test.go` | buat | SQL terhadap Postgres nyata, oracle `Expired` |
| `.../purgesessions/init_integration_test.go` | buat | job terdaftar dengan nama dan jadwal benar, `Run` end-to-end |
| `backend/internal/domain/core/internal/auth/base.go` | ubah | tambah `purgesessions.Init(d, shared)` (satu baris + import) |

Tidak ada: env baru, dependensi baru, kontrak bus baru, route, entri OpenAPI, perubahan docs tiket (README tiket tidak mencatat job; overview sudah mencantumkannya), perubahan `base/` (memakai `Clock`, `SessionConfig`, `ThrottlePolicy`, `LoginThrottle.Expired` yang sudah ada dari F1).

## Test Scenario

Penamaan: tidak ada AC tiket yang dimiliki fitur ini, jadi nama `Test<Unit>_<Kasus>` (rule 13.4). Test integrasi memakai `db.TestDB(t)`, menerapkan `migrations.FS` ke schema test, dan tidak memakai mock (rule 13.1). Pengisian data dengan `INSERT` langsung di helper test (file `_test.go`). Dua helper: `seedUser(t)` (sessions punya FK ke users) dan `seedSession(t, userID, createdAt, lastSeenAt, expiresAt)` dengan `token_hash` acak unik.

### Unit `usecase_test.go` (repo palsu mencatat argumen; clock tetap; logger ke buffer)

- `TestRun_ComputesCutoffsOnce`: clock palsu yang maju tiap dipanggil; hanya dipanggil sekali; repo menerima `now == t0`, `idleCutoff == t0 - idle`, `windowCutoff == t0 - window`; kedua repo menerima `now` yang sama.
- `TestRun_CutoffValues`: now `2026-10-20T03:00:00Z`, idle 12h, window 15m -> idleCutoff `2026-10-19T15:00:00Z`, windowCutoff `2026-10-20T02:45:00Z`.
- `TestRun_LogsCounts`: repo mengembalikan `(7, nil)` dan `(3, nil)` -> log info `"auth purge sessions selesai"` memuat `deleted_sessions=7`, `deleted_throttles=3`, `duration_ms`; tidak ada log level error; return nil.
- `TestRun_ZeroDeletedIsSuccess`: `(0,0)` -> nil, log `deleted_sessions=0 deleted_throttles=0`.
- `TestRun_SessionsErrorStopsAndReturns`: repo sessions error -> `errors.Is(err, sentinel)`, `DeleteExpiredThrottles` TIDAK dipanggil.
- `TestRun_ThrottlesErrorReturnsAfterSessions`: sessions `(5,nil)`, throttles error -> error dikembalikan terbungkus (`errors.Is`), log memuat `deleted_sessions=5`.
- `TestRun_ContextCanceledBeforeStart`: ctx batal -> repo tidak dipanggil, `errors.Is(err, context.Canceled)`.
- `TestNew_RejectsNonPositiveDurations`: `idle=0` atau `window=0` (dan negatif) -> panic.
- `TestRun_NoSecretsInLog`: buffer log hanya berisi kunci angka yang diharapkan (tanpa `email`, `token`, `user_id`).

### Integrasi `repository_integration_test.go`

Semua dengan `now` tetap `N = 2026-10-20T03:00:00Z`, idle 12h, `Throttle.Window` 15m, lewat `Usecase.Run` dengan repo nyata kecuali dinyatakan lain.

Sessions:
- `TestDeleteExpiredSessions_Absolute`: sesi `expires_at = N-1s` (last_seen segar) terhapus; `expires_at = N+1s` tetap.
- `TestDeleteExpiredSessions_Idle`: sesi `last_seen_at = N-12h-1s` (expires jauh di depan) terhapus; `last_seen_at = N-11h` tetap.
- `TestDeleteExpiredSessions_Boundary`: `expires_at == N` dan `last_seen_at == N-12h` tepat TIDAK terhapus (strict); sesuai `Session.IsValid` false tetapi baris dipertahankan sampai run berikutnya.
- `TestDeleteExpiredSessions_BothConditions`: sesi yang kedua kriteria terpenuhi terhitung SATU baris (bukan dua).
- `TestDeleteExpiredSessions_ValidKept`: sesi aktif (expires depan, last_seen segar) milik user lain tetap; jumlah terhapus tepat sama dengan jumlah kedaluwarsa yang di-seed (mis. 3 dari 6).
- `TestDeleteExpiredSessions_AgreesWithIsValid`: tabel kombinasi (`expires_at`, `last_seen_at`) di sekitar batas; untuk setiap baris, bila `base.Session.IsValid(N, idle)` true maka baris tidak terhapus (properti: tidak pernah menghapus sesi valid).

Throttles:
- `TestDeleteExpiredThrottles_WindowPassedNoLock`: `window_started_at = N-16m`, `locked_until NULL` terhapus; `N-14m` tetap.
- `TestDeleteExpiredThrottles_LockStillActive`: `window_started_at = N-1h`, `locked_until = N+5m` TIDAK terhapus (lock aktif).
- `TestDeleteExpiredThrottles_LockExpired`: `window_started_at = N-1h`, `locked_until = N-1s` terhapus.
- `TestDeleteExpiredThrottles_LockBoundary`: `locked_until == N` tepat tidak terhapus (strict `<`); `window_started_at == N-15m` tepat tidak terhapus.
- `TestDeleteExpiredThrottles_WindowOpenWithExpiredLock`: `window_started_at = N-5m`, `locked_until = N-1s` tetap (jendela masih berjalan; hitungan gagal masih relevan).
- `TestDeleteExpiredThrottles_EmailCaseInsensitive`: tidak berpengaruh pada penghapusan (kunci citext); satu baris `Foo@X.id` diperlakukan sebagai satu baris.
- `TestDeleteExpiredThrottles_MatchesExpiredOracle`: tabel kombinasi (window_started_at, locked_until) di sekitar batas; hasil SQL (baris hilang atau tetap) harus sama dengan `base.LoginThrottle.Expired(N, policy)` untuk setiap kombinasi. Menjaga SQL dan rule entity tidak melenceng.

Lintas:
- `TestRun_RemovesBothTablesAndReturnsCounts`: seed campuran sesi dan throttle, `Run` -> sisa baris sesuai ekspektasi, log memuat jumlah yang benar.
- `TestRun_Idempotent`: `Run` dua kali berturut-turut; kedua kali nil, run kedua `deleted_*=0`, isi tabel tidak berubah.
- `TestRun_EmptyTables`: tabel kosong -> nil, `0/0`.
- `TestRun_DoesNotTouchUsers`: jumlah baris `users` tidak berubah setelah purge (FK `sessions.user_id` tidak men-cascade ke atas).
- `TestRun_DBError`: ctx dengan deadline lampau -> error tanpa panic.

### Integrasi `init_integration_test.go`

- `TestNewJob_NameAndSchedule`: `newJob(nil)` memiliki `Name == "core.auth.purge-sessions"`, `DailyAt == "03:00"`, `Every == 0`.
- `TestInit_RegistersJob`: `jobs.New(db, log)` + `Init(deps, shared)`; `Init` kedua kalinya pada scheduler yang sama panic (nama duplikat), membuktikan job benar-benar terdaftar. Bila tidak ada accessor, memakai perilaku panic itu sebagai bukti pendaftaran.
- `TestJob_RunsUnderScheduler` (opsional, bila scheduler T00 mengizinkan memicu satu tick): tidak diwajibkan; cakupan `Run` sudah ada di atas.
- `TestT01_AdvisoryLockName`: nama job lolos regex scheduler `^[a-z0-9][a-z0-9._-]*$` (dibuktikan oleh `Register` tidak panic).

## E2E Scenarios

Fitur tidak punya UI atau endpoint, sehingga e2e Playwright tidak berlaku. Skenario berikut diverifikasi pada level binary/integrasi (dapat dijalankan manual atau di `auth_integration_test.go` bila mudah):

Positif:
1. Dengan server berjalan dan jam diatur (clock injeksi di test integrasi `server`-level), sesi user yang `expires_at` lewat dihapus pada run job; request dengan cookie lama sudah 401 sebelum job (karena `resolvesession` menolak), dan tetap 401 sesudahnya.
2. Sesi idle > 12 jam (`last_seen_at` dimundurkan) hilang dari tabel setelah job; sesi user lain yang aktif tetap bisa dipakai (`GET /api/auth/me` 200).
3. Setelah 5 login gagal yang menghasilkan baris `login_throttles`, jendela dan lock dimundurkan melewati batas, lalu job menghapus baris; login benar sesudahnya 200 dan tidak terkunci.
4. Log server memuat `auth purge sessions selesai` dengan `deleted_sessions` dan `deleted_throttles` yang sesuai (cek lewat logger buffer di test).

Negatif:
1. Throttle dengan lock yang masih aktif tidak dihapus: login benar untuk email itu tetap 429 `ACCOUNT_LOCKED` setelah job dijalankan.
2. Sesi valid (aktif) tidak terhapus: `GET /api/auth/me` tetap 200 setelah job.
3. Dua instance/dua run bersamaan tidak saling merusak: run kedua di bawah advisory lock dilewati scheduler (`status=skipped`); bila dijalankan manual dua kali berurutan, hasil kedua `0/0`.
4. DB tidak tersedia saat job: `Run` mengembalikan error, scheduler mencatat status `error`, proses server tidak crash, dan job berjalan lagi pada jadwal berikutnya.
5. Tabel belum dimigrasi (`42P01`): `Run` mengembalikan error terbungkus; tidak panic.

## Request/Response body

Tidak ada endpoint HTTP maupun bus, jadi tidak ada body request/response. Kontrak fitur ini adalah job dan log.

Definisi job:

```go
jobs.Job{
    Name:    "core.auth.purge-sessions",
    DailyAt: "03:00", // UTC
    Run:     uc.Run, // func(ctx context.Context) error
}
```

Log sukses (JSON slog, contoh):

```json
{
  "time": "2026-10-21T03:00:00.412Z",
  "level": "INFO",
  "msg": "auth purge sessions selesai",
  "trace_id": "job-5b0e6c52-0e5a-4c41-9d52-6e3f3c1a7a10",
  "deleted_sessions": 42,
  "deleted_throttles": 5,
  "duration_ms": 18
}
```

Kegagalan: `Run` mengembalikan error (tidak ada body klien; tidak ada format error HTTP). Contoh pesan yang diteruskan ke scheduler, yang me-log `job`, `status="error"`, `duration_ms`:

```
purgesessions: hapus login_throttles: <penyebab dari driver>
```

SQL yang dieksekusi (parameter posisional saja):

```sql
DELETE FROM sessions
WHERE expires_at < $1 OR last_seen_at < $2;          -- $1=now, $2=now-SESSION_IDLE_TIMEOUT

DELETE FROM login_throttles
WHERE window_started_at < $2
  AND (locked_until IS NULL OR locked_until < $1);   -- $1=now, $2=now-LOGIN_FAILURE_WINDOW
```

Tidak ada audit yang dikirim (`core.audit.record` tidak dipanggil) dan tidak ada event bus.
