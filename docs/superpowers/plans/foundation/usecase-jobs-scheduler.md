# T00 Foundation — Usecase `jobs-scheduler`

Sumber: `docs/tickets/T00-foundation.md` (F7), `rules.txt` 14.4, 14.2, 6.x (trace id), `docs/superpowers/plans/foundation/overview.md` (bagian 3, 4, 7 langkah 4, 13).
Usecase ini hanya `infra/jobs`. Tidak ada job yang didaftarkan di T00 (T01 `purgesessions`, T02 `purgeexpired` mendaftar sendiri lewat `deps.Jobs.Register`).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 (F7) |
| Usecase | `jobs-scheduler` (urutan 4 dari 8) |
| Scope | fullstack (usecase ini hanya backend Go) |
| Domain | infra |
| Module | foundation |
| Resource root | `backend/internal/infra` (package `jobs` di `backend/internal/infra/jobs`) |
| Endpoint | - (tidak ada endpoint HTTP) |
| Method | - |
| Version | - (tanpa versi URL; tidak menyentuh `api/`) |
| Endpoint bus | Tidak menyediakan dan tidak mengonsumsi endpoint bus. Tidak ada konstanta baru di `lib/constants/bus.go` |
| Aksi audit | Tidak ada |
| Env var baru | Tidak ada (tabel overview bagian 9 dan `docs/configuration.md` tidak berubah) |
| Bergantung pada | `repo-skeleton-config` (reqctx, logger, go.mod), `db-migrations` (`db.DB`, `db.TestDB`) |
| Dibutuhkan oleh | `cli-and-wiring` (membuat `jobs.New`, memanggil `Start`/`Stop`), T01, T02 |

Pola `init/handler/params/usecase/repository` tidak berlaku (infra, tanpa entity/SQL domain). Satu-satunya SQL adalah `pg_try_advisory_lock` / `pg_advisory_unlock` di `lock.go`, bukan tabel domain. Rule 4.6: package `jobs` DILARANG meng-import `internal/domain/**`.

---

## 2. Summary Plan

Membangun scheduler in-process yang ringan dan aman multi-instance:

- `Register(Job{Name, Every | DailyAt, Run})` sebelum `Start`. Nama duplikat, nama/jadwal invalid, atau Register sesudah Start = panic (gagal cepat saat wiring).
- Satu goroutine per job. Goroutine menghitung waktu eksekusi berikutnya lewat fungsi murni `nextRun` (UTC), tidur sampai waktu itu, lalu mengeksekusi sinkron di goroutine yang sama. Karena sinkron, tick yang terlewat akibat job masih berjalan otomatis dilewati dan tidak menumpuk.
- Setiap eksekusi: trace id baru `job-<uuid>` di ctx (`reqctx.WithTraceID`), ambil koneksi khusus dari pool, `SELECT pg_try_advisory_lock(hashtext('grimoire.job.'||$1))`. Bila terkunci instance lain, lewati. Bila berhasil, jalankan `Run` dengan recover panic, lalu `pg_advisory_unlock` pada koneksi yang SAMA, lalu kembalikan koneksi.
- Log satu baris per eksekusi: `job`, `duration_ms`, `status` (`ok|error|panic|skipped`), `trace_id`.
- `Stop(ctx)` berhenti menjadwalkan dan menunggu job yang sedang berjalan sampai deadline ctx.

Keputusan desain yang mengikat (tidak ada di overview, jadi ditetapkan di sini):

1. **Jadwal `Every` disejajarkan ke jam dinding (epoch-aligned)**: waktu berikutnya = kelipatan `Every` sejak Unix epoch yang `>` now. Dengan begitu semua instance memiliki tick yang sama walau start pada waktu berbeda, sehingga advisory lock benar-benar menyaring "satu instance per tick". Job `Every` TIDAK jalan sekali saat start; yang pertama di tick berikutnya.
2. **Format `DailyAt`** `"HH:MM"` UTC (regex `^([01][0-9]|2[0-3]):[0-5][0-9]$`). Detik selalu `:00`.
3. **Lock bukan dedupe tick.** Advisory lock hanya menjamin tidak ada dua eksekusi yang tumpang tindih. Bila job selesai lebih cepat daripada selisih waktu tick antar instance (skew jam/jitter milidetik), instance kedua bisa mengambil lock setelah dilepas dan menjalankan job lagi pada tick yang sama. Konsekuensi: setiap job WAJIB idempoten (purge sesi/audit memang idempoten). Dicatat di godoc `Job.Run` dan di bagian Risiko.
4. **Lock dipegang di koneksi khusus** (`db.Conn(ctx)` -> `*sqlx.Conn`) karena advisory lock level sesi terikat ke koneksi. Satu job yang sedang berjalan menahan satu koneksi pool (perhatikan `DB_MAX_OPEN_CONNS`, default 20).
5. **Bila unlock gagal**, koneksi WAJIB dibuang dari pool (bukan dikembalikan), supaya lock tidak bocor di koneksi yang dipakai ulang: `conn.Raw(func(any) error { return driver.ErrBadConn })` lalu `conn.Close()`.
6. **Locker di balik interface** supaya unit test tidak butuh DB (panic job, Stop, duplikat). Implementasi produksi `pgLocker`; test unit memakai fake.
7. **Clock injectable** (field `now func() time.Time` yang tidak diekspor) dipakai `nextRun` dan loop; test unit memakai `Every` kecil (20-50ms) dengan clock nyata, jadi clock palsu hanya dipakai bila perlu. `nextRun` sendiri murni (menerima `now`).
8. Tidak ada timeout per job di T00. `Run` wajib menghormati `ctx` (dibatalkan saat `Stop` kehabisan deadline). Job yang butuh timeout membuat sendiri dengan `context.WithTimeout`.

---

## 3. Plan (langkah berurutan)

Prasyarat: `backend/go.mod` ada, `reqctx` ada, `db.DB`/`db.TestDB` dari usecase `db-migrations` sudah merge. Tambahkan dependensi `github.com/google/uuid` ke `go.mod` bila belum ada (go mod tidy).

### Langkah 1 — `schedule.go` (fungsi murni, TDD dulu)
Tulis `schedule_test.go` lebih dulu, lalu:

```go
package jobs

// parseDailyAt memvalidasi "HH:MM" dan mengembalikan jam dan menit.
func parseDailyAt(s string) (hour, min int, err error)

// nextRun mengembalikan waktu eksekusi berikutnya yang STRICTLY setelah now, selalu UTC.
//   Every:   kelipatan Every sejak Unix epoch yang > now
//            (floor(now.UnixNano()/every)+1)*every
//   DailyAt: hari ini HH:MM:00 UTC bila > now, selain itu besok HH:MM:00 UTC
// now dikonversi dengan now.UTC() terlebih dulu. Panic bila Job tidak valid
// (sudah divalidasi Register).
func nextRun(now time.Time, j Job) time.Time
```

Catatan: pakai `time.Date(y, m, d, h, mi, 0, 0, time.UTC)` dan `AddDate(0, 0, 1)`; UTC tidak punya DST jadi aman.

### Langkah 2 — `lock.go` (lock Postgres)
```go
package jobs

// locker memisahkan akses lock dari Scheduler agar bisa difake di unit test.
type locker interface {
    // TryLock: ok=false, err=nil bila lock dipegang pihak lain.
    // unlock dipanggil tepat sekali oleh pemanggil bila ok=true.
    TryLock(ctx context.Context, name string) (unlock func(ctx context.Context) error, ok bool, err error)
}

type pgLocker struct{ db *db.DB }

const lockKeyPrefix = "grimoire.job."
```

`pgLocker.TryLock`:
1. `ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)` hanya untuk query lock.
2. `conn, err := p.db.Conn(ctx2)` (gagal -> return err).
3. `var got bool; err = conn.GetContext(ctx2, &got, "SELECT pg_try_advisory_lock(hashtext($1))", lockKeyPrefix+name)`.
   - err -> `conn.Close()`, return err. (Query yang gagal di tengah jalan bisa saja sudah mengambil lock; `Close()` ke pool tidak melepas lock, jadi buang koneksi dengan helper `discard(conn)` di bawah.)
   - `!got` -> `conn.Close()` (aman, tidak memegang lock), return `nil, false, nil`.
4. `got` -> return closure `unlock`:
   ```go
   func(uctx context.Context) error {
       var released bool
       err := conn.GetContext(uctx, &released, "SELECT pg_advisory_unlock(hashtext($1))", lockKeyPrefix+name)
       if err != nil || !released {
           discard(conn) // lock tidak terjamin lepas: buang koneksi
           if err == nil { err = errors.New("jobs: advisory unlock returned false") }
           return err
       }
       return conn.Close()
   }
   ```
5. `discard(conn)`: `_ = conn.Raw(func(any) error { return driver.ErrBadConn })` lalu `_ = conn.Close()`. Menutup koneksi fisik otomatis melepas lock sesi di sisi Postgres.

Nama kunci: `'grimoire.job.' || name` dihitung di Go lalu dikirim sebagai parameter ke `hashtext($1)`, hasilnya identik dengan `hashtext('grimoire.job.'||name)` yang diminta. `hashtext` mengembalikan int4 yang di-cast implisit ke bigint untuk `pg_try_advisory_lock(bigint)`. Satu-argumen bigint dan dua-argumen int4 adalah ruang kunci berbeda; pakai versi satu argumen secara konsisten, dan beda dengan kunci migrasi `hashtext('grimoire.migrate')` karena string-nya beda.

### Langkah 3 — `scheduler.go`
```go
package jobs

type Job struct {
    Name    string        // unik, ^[a-z0-9][a-z0-9._-]*$ (mis. "core.purge-sessions")
    Every   time.Duration // > 0, epoch-aligned. Tepat salah satu dari Every / DailyAt
    DailyAt string        // "HH:MM" UTC
    // Run WAJIB idempoten (lock hanya mencegah tumpang tindih, bukan eksekusi ganda
    // antar instance) dan menghormati ctx. Mengembalikan error = status "error" (di-log).
    Run func(ctx context.Context) error
}

type Scheduler struct { /* mu, jobs map[string]Job + urutan, started bool, locker, log, now, stopCh, runCtx, cancel, wg, running atomic.Int32 */ }

func New(d *db.DB, log *slog.Logger) *Scheduler             // memakai pgLocker{d}
func newWithLocker(l locker, log *slog.Logger) *Scheduler   // untuk test
func (s *Scheduler) Register(j Job)                         // panic: nama kosong/format salah, duplikat, Run nil,
                                                            //   Every dan DailyAt keduanya/tidak ada, Every < 0,
                                                            //   DailyAt invalid, setelah Start
func (s *Scheduler) Start(ctx context.Context)              // panic bila dipanggil dua kali
func (s *Scheduler) Stop(ctx context.Context) error         // idempoten; sebelum Start -> nil
```

Pesan panic (diuji dengan `assert.PanicsWithValue` atau cek substring): `jobs: job "<name>" registered twice`, `jobs: Register after Start (job "<name>")`, `jobs: job "<name>": exactly one of Every or DailyAt is required`, dst.

`Start(ctx)`:
1. Lock `mu`, set `started=true`, buat `runCtx, cancel := context.WithCancel(ctx)` (ctx induk = ctx server) dan `stopCh`.
2. Per job: `wg.Add(1)`; `go s.loop(j)`.
3. Log info `jobs started` dengan `count`. Tanpa job terdaftar (kondisi T00) Start tetap sukses, tidak membuat goroutine.

`loop(j)`:
```go
defer s.wg.Done()
for {
    next := nextRun(s.now(), j)
    t := time.NewTimer(time.Until(next))   // gunakan next.Sub(s.now()) agar patuh clock injeksi
    select {
    case <-s.stopCh:      t.Stop(); return
    case <-s.runCtx.Done(): t.Stop(); return
    case <-t.C:
    }
    s.execute(j)           // sinkron: tick yang lewat selama eksekusi otomatis dilewati
}
```
Setelah `execute` kembali, iterasi berikutnya menghitung `nextRun` dari now baru, sehingga tick yang terlewat dibuang (tidak menumpuk, tidak "mengejar").

`execute(j)`:
```go
ctx := reqctx.WithTraceID(s.runCtx, "job-"+uuid.NewString())
start := s.now()
status, err := s.runLocked(ctx, j)
// log: level sesuai status
s.log.Log(ctx, level, "job finished", "job", j.Name, "status", status,
    "duration_ms", float64(time.Since(start).Microseconds())/1000, "error", err)  // error hanya bila ada
```
Level: `ok` info, `skipped` debug, `error` dan `panic` error. Pesan konsisten `job finished`. Wajib memakai `*Context` agar logger menambah `trace_id`.

`runLocked(ctx, j) (status string, err error)`:
1. `unlock, ok, lerr := s.locker.TryLock(ctx, j.Name)`.
   - `lerr != nil` -> status `error`, err dibungkus `fmt.Errorf("jobs: acquire lock: %w", lerr)`; Run TIDAK dipanggil.
   - `!ok` -> status `skipped`, err nil.
2. `s.running.Add(1)`; defer `s.running.Add(-1)`.
3. defer unlock: `uctx, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)`; bila unlock error -> log error `job unlock failed` (tidak menimpa status job).
4. `err = safeRun(ctx, j)`; `status = "ok"` atau `"error"`.

`safeRun`: `defer func(){ if p := recover(); p != nil { status="panic"; err=fmt.Errorf("panic: %v", p); log error "job panicked" dengan "stack", string(debug.Stack()) } }()`. Panic job tidak menghentikan loop job tersebut maupun job lain.

`Stop(ctx)`:
1. Lock `mu`; bila belum started atau sudah stopped, return nil. `close(stopCh)` sekali (guard `sync.Once`).
2. `done := make(chan struct{}); go func(){ s.wg.Wait(); close(done) }()`.
3. `select { case <-done: s.cancel(); return nil; case <-ctx.Done(): s.cancel(); return fmt.Errorf("jobs: stop timed out with %d job(s) still running: %w", s.running.Load(), ctx.Err()) }`.
   Job yang berjalan diberi kesempatan selesai normal selama deadline; hanya bila deadline habis `runCtx` dibatalkan agar `Run` yang patuh ctx berhenti.
4. Log info `jobs stopped`.

Catatan Stop saat job sedang menunggu `TryLock`: `runCtx` belum dibatalkan, query selesai dalam hitungan ms; tidak masalah.

### Langkah 4 — Finalisasi `app.Deps` (kontribusi usecase ini)
Di `backend/internal/infra/app/deps.go` tambahkan `Jobs *jobs.Scheduler` (overview bagian 4.4: field ditambahkan usecase yang membuat tipenya). Jangan mengubah field lain. Penginstansiasian `jobs.New(...)` dan `Start/Stop` ada di `cli-and-wiring`, BUKAN di sini.

### Langkah 5 — Test (lihat bagian 6), jalankan `go test -race ./internal/infra/jobs/...`, lalu `go vet`, `go test -tags integration` dengan `TEST_DATABASE_URL`.

### Langkah 6 — Dokumentasi singkat
- Godoc paket `jobs` (di `scheduler.go`): semantik jadwal epoch-aligned, idempotensi wajib, satu koneksi per job berjalan, panic recover, perilaku Stop.
- Tidak mengubah `docs/configuration.md` (tidak ada env baru). Tidak mengubah `rules.txt`/README (folder `infra/jobs` sudah terdaftar di rules 3.1).

### Kontrak bus
Tidak ada. Module memakai scheduler lewat `deps.Jobs.Register(jobs.Job{...})` di `base.go` module (bukan bus). Contoh pemakaian untuk T01/T02 (dokumentasi saja, tidak dibuat di T00):

```go
d.Jobs.Register(jobs.Job{
    Name:    "core.purge-sessions",
    Every:   time.Hour,
    Run:     purgeSessionsUsecase.Run, // memanggil repository, bukan SQL langsung (rule 4.x/5.x)
})
```

---

## 4. Migration

Tidak ada. Advisory lock tidak membutuhkan tabel. Tidak ada perubahan `backend/migrations/**`.

---

## 5. Changes

Baru:
- `backend/internal/infra/jobs/schedule.go` — `parseDailyAt`, `nextRun`.
- `backend/internal/infra/jobs/lock.go` — interface `locker`, `pgLocker`, `discard`.
- `backend/internal/infra/jobs/scheduler.go` — `Job`, `Scheduler`, `New`, `newWithLocker`, `Register`, `Start`, `Stop`, `loop`, `execute`, `runLocked`, `safeRun`.
- `backend/internal/infra/jobs/schedule_test.go`
- `backend/internal/infra/jobs/scheduler_test.go` (unit, fake locker)
- `backend/internal/infra/jobs/scheduler_integration_test.go` (build tag `integration`, memakai `db.TestDB`)

Diubah:
- `backend/internal/infra/app/deps.go` — tambah field `Jobs *jobs.Scheduler`.
- `backend/go.mod` / `go.sum` — `github.com/google/uuid` bila belum ada (testify opsional; boleh stdlib `testing` saja).

Tidak diubah: `lib/constants`, `lib/contracts`, `api/**`, `docs/**`, `rules.txt`, migrasi, `infra/bus`, `infra/db`.

---

## 6. Test Scenario

Penamaan mengikuti rule 13.4. Semua dijalankan dengan `-race`. Unit test memakai `fakeLocker` (menghitung panggilan, bisa dikonfigurasi `ok=false` atau error).

### Unit — `schedule_test.go` (tanpa DB, tanpa tidur)
- `TestNextRun_DailyAt`: tabel kasus dengan `DailyAt: "03:00"`:
  - now 02:59:59 -> hari ini 03:00:00 UTC
  - now tepat 03:00:00.000 -> besok 03:00 (strictly after)
  - now 03:00:00.001 -> besok 03:00
  - now 23:59:59 -> besok 03:00 (lintas hari)
  - akhir bulan/tahun (31 Des 23:00) -> 1 Jan 03:00
  - `now` dengan lokasi non-UTC (mis. UTC+7 `2026-10-20 09:59`, = 02:59 UTC) -> hasil dihitung dari UTC, `Location()==time.UTC`
  - `"00:00"` dan `"23:59"` (batas)
- `TestNextRun_Every`: `Every: 15*time.Minute`; now 10:07 -> 10:15; now tepat 10:15:00 -> 10:30; hasil selalu kelipatan Every dari epoch; dua `now` berbeda dalam tick yang sama menghasilkan nilai identik (properti epoch-aligned yang dibutuhkan lintas instance). `Every: 1s` dan `Every: 24h`.
- `TestParseDailyAt`: valid `00:00`, `09:05`, `23:59`; invalid `24:00`, `12:60`, `9:05`, `12:5`, `""`, `12:00:00`, `ab:cd`.

### Unit — `scheduler_test.go`
- `TestRegister_DuplicatePanics` (pesan memuat nama job).
- `TestRegister_InvalidPanics`: tabel — nama kosong, nama berspasi/huruf besar, `Run` nil, Every dan DailyAt keduanya terisi, keduanya kosong, `Every` negatif, `DailyAt` invalid.
- `TestRegister_AfterStartPanics`.
- `TestStart_Twice_Panics`.
- `TestStart_NoJobs_StopOK`: tanpa job, Start lalu Stop sukses, tidak ada goroutine bocor (`runtime.NumGoroutine` sebelum/sesudah dalam toleransi, atau `goleak` bila dipakai repo).
- `TestJobRuns_OnEverySchedule`: `Every: 20ms`, job menambah counter; setelah ~200ms counter >= 3.
- `TestJobPanicDoesNotStopScheduler`: dua job `Every: 20ms`; job A panic pada setiap eksekusi, job B menghitung. Setelah ~200ms: A dipanggil >= 2 kali (loop tetap hidup setelah panic), B >= 3. Buffer log memuat `status=panic`, `job=A`, dan stack. `Stop` sukses.
- `TestJobError_LoggedAndContinues`: `Run` mengembalikan error -> log `status=error` dengan pesan error, eksekusi berikutnya tetap berjalan.
- `TestExecute_NewTraceIDPerRun`: `Run` merekam `reqctx.TraceID(ctx)`; semua eksekusi berawalan `job-`, unik antar eksekusi, dan baris log `job finished` memuat `trace_id` yang sama dengan yang dilihat `Run`.
- `TestExecute_LockedIsSkipped`: `fakeLocker{ok:false}` -> `Run` tidak pernah dipanggil, log `status=skipped` (level debug), loop tetap jalan.
- `TestExecute_LockError_NotRun`: locker mengembalikan error -> `Run` tidak dipanggil, log level error `status=error`.
- `TestExecute_UnlockCalledAfterRun`: urutan tercatat `lock -> run -> unlock`; unlock tetap dipanggil saat `Run` panic dan saat `Run` error; unlock memakai ctx yang tidak batal walau `runCtx` sudah dibatalkan (cek `ctx.Err()==nil` di fake).
- `TestExecute_UnlockErrorLogged`: unlock gagal -> log error `job unlock failed`, status job tidak berubah.
- `TestStop_WaitsRunningJob`: `Run` menunggu channel; panggil `Stop` di goroutine; `Stop` belum kembali selama job berjalan; setelah channel ditutup, `Stop` kembali nil dan tidak ada eksekusi baru sesudahnya.
- `TestStop_TimeoutCancelsRunCtx`: `Run` memblok sampai `ctx.Done()`; `Stop` dengan deadline 50ms mengembalikan error yang `errors.Is(err, context.DeadlineExceeded)`, `Run` melihat ctx batal.
- `TestStop_Idempotent_AndBeforeStart`.
- `TestLoop_MissedTicksNotQueued`: `Every: 20ms`, `Run` tidur 100ms; selama 300ms jumlah eksekusi <= 3 (tidak ada burst pengejar).
- `TestStopInterruptsWaiting`: job `DailyAt` jauh di masa depan; `Stop` kembali segera (< 100ms).

### Integrasi — `scheduler_integration_test.go` (tag `integration`, `TEST_DATABASE_URL`)
Skip lokal bila env kosong, gagal bila `CI=true` (perilaku `db.TestDB`). Nama job unik per test (`it-<random>`) agar tidak bentrok antar test/package pada DB yang sama.
- `TestTwoSchedulers_OnlyOneRunsPerTick`: dua `Scheduler` (`jobs.New` dengan dua `*db.DB` terpisah atau satu pool) pada DB sama, job `Every: 500ms` bernama sama, `Run` mencatat `time.Now()` ke slice terkunci lalu tidur 200ms. Jalankan ~2,3 detik. Verifikasi: setiap bucket `floor(ts/500ms)` berisi tepat satu eksekusi (bukan dua), total eksekusi >= 3, dan log gabungan memuat setidaknya satu `status=skipped`. Job tidur 200ms memastikan tumpang tindih, sehingga hasil deterministik walau start kedua scheduler beda milidetik.
- `TestLockHeldWhileRunning_ReleasedAfter`: job menunggu channel; selama berjalan, sesi lain (`SELECT pg_try_advisory_lock(hashtext('grimoire.job.'||$1))`) mengembalikan false; setelah job selesai dan `Stop`, sesi lain mendapat true (lalu di-unlock). Membuktikan lock dilepas di koneksi yang sama dan tidak bocor.
- `TestLockReleasedAfterPanic`: job panic -> lock tetap terlepas (sesi lain dapat true).
- `TestConnectionReturnedToPool`: setelah 5 eksekusi, `d.Stats().InUse == 0` dan `OpenConnections` tidak naik tanpa batas.
- `TestPgLocker_UnlockFailureDiscardsConn` (opsional, bila mudah): unlock memakai ctx yang sudah dibatalkan -> error, lalu lock tidak lagi dipegang oleh koneksi mana pun di pool (sesi lain mendapat true), karena koneksi dibuang.
- `TestScheduler_DBDown_SkipsAndRecovers` (opsional): lock error tidak mematikan scheduler (dicakup unit `TestExecute_LockError_NotRun`; integrasi bersifat bonus).

---

## 7. E2E Scenarios

Usecase ini tidak punya permukaan HTTP/UI; E2E dilakukan lewat binary (setelah `cli-and-wiring`) dan lewat observasi log/DB.

Positif:
1. Start server dengan hanya `DATABASE_URL` + `SESSION_SECRET` (T00, tanpa job terdaftar): server naik normal, log `jobs started` dengan `count=0`, tidak ada goroutine job, SIGTERM -> `jobs stopped`, exit 0.
2. (Verifikasi lintas tiket, bukan bagian PR ini) Dua instance `bin/grimoire` pada DB sama dengan job `Every` terdaftar: pada tiap tick log satu instance `status=ok` dan instance lain `status=skipped` (untuk job yang berjalan cukup lama untuk tumpang tindih).
3. SIGTERM saat job sedang berjalan: proses menunggu job selesai (dalam `SHUTDOWN_TIMEOUT`), log `job finished` muncul sebelum `jobs stopped`, lalu exit 0.
4. Job `DailyAt` ("03:00"): log startup debug memuat `next_run` UTC yang benar (lihat godoc/log `job scheduled`, level debug, dipicu saat loop menghitung `nextRun`).

Negatif:
1. Wiring module mendaftarkan dua job bernama sama: proses panic saat start dengan pesan `jobs: job "<name>" registered twice` (gagal cepat, sebelum menerima trafik).
2. `Run` panic: log error dengan stack dan `trace_id` `job-...`, server tetap melayani `/healthz` 200, job dijadwalkan lagi pada tick berikutnya.
3. `Run` mengembalikan error: log `status=error`, tidak ada efek ke request HTTP, tick berikutnya tetap jalan.
4. DB mati saat tick: log error `jobs: acquire lock`, `Run` tidak dipanggil, `/readyz` 503 (dari usecase lain), setelah DB pulih tick berikutnya berjalan normal.
5. Shutdown dengan job macet melebihi deadline: `Stop` mengembalikan error timeout, `runCtx` dibatalkan, server di-log gagal bersih dan exit code 1 (aturan shutdown di `cli-and-wiring`).

---

## 8. Request/Response body

Tidak ada endpoint HTTP, jadi tidak ada request/response JSON baru. Format error seragam `{ "error": {...}, "trace_id": "..." }` tidak diproduksi oleh usecase ini; kegagalan job hanya muncul di log, tidak pernah ke klien.

### API Go (kontrak untuk module pemakai)
```go
s := jobs.New(database, logger)

s.Register(jobs.Job{Name: "core.purge-sessions", Every: time.Hour, Run: purge})
s.Register(jobs.Job{Name: "core.purge-expired-audit", DailyAt: "03:00", Run: purgeAudit})

s.Start(serverCtx)
// ... saat shutdown
err := s.Stop(ctxWithDeadline) // nil bila semua job selesai sebelum deadline
```

### Log (JSON)
Eksekusi sukses:
```json
{"time":"2026-10-20T03:00:00.004Z","level":"INFO","msg":"job finished","job":"core.purge-expired-audit","status":"ok","duration_ms":3.9,"trace_id":"job-6f0c2f1a-6a53-4d57-8a63-19a3f6ed7e10"}
```
Dilewati karena terkunci instance lain (level debug):
```json
{"time":"2026-10-20T03:00:00.002Z","level":"DEBUG","msg":"job finished","job":"core.purge-expired-audit","status":"skipped","duration_ms":1.2,"trace_id":"job-1b8e0d2c-9d3e-4a0e-8c1f-0a5e2f6f7b44"}
```
Error dari `Run`:
```json
{"time":"2026-10-20T03:00:00.010Z","level":"ERROR","msg":"job finished","job":"core.purge-sessions","status":"error","duration_ms":8.1,"error":"delete sessions: connection reset","trace_id":"job-2c4d..."}
```
Panic (dua baris: detail panic + ringkasan):
```json
{"time":"2026-10-20T03:00:00.011Z","level":"ERROR","msg":"job panicked","job":"core.purge-sessions","panic":"runtime error: index out of range [3] with length 3","stack":"goroutine 41 [running]:\n...","trace_id":"job-2c4d..."}
{"time":"2026-10-20T03:00:00.011Z","level":"ERROR","msg":"job finished","job":"core.purge-sessions","status":"panic","duration_ms":0.3,"error":"panic: runtime error: index out of range [3] with length 3","trace_id":"job-2c4d..."}
```
Gagal ambil lock:
```json
{"time":"2026-10-20T03:00:00.005Z","level":"ERROR","msg":"job finished","job":"core.purge-sessions","status":"error","duration_ms":5000.1,"error":"jobs: acquire lock: context deadline exceeded","trace_id":"job-9a7b..."}
```
Unlock gagal:
```json
{"time":"2026-10-20T03:00:00.020Z","level":"ERROR","msg":"job unlock failed","job":"core.purge-sessions","error":"jobs: advisory unlock returned false","trace_id":"job-2c4d..."}
```
Lifecycle:
```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"jobs started","count":0}
{"time":"2026-10-20T08:10:00Z","level":"INFO","msg":"jobs stopped"}
```

---

## 9. Risiko dan catatan untuk developer

- **Eksekusi ganda antar instance** untuk job yang sangat cepat (lihat keputusan 3). Mitigasi sekarang: job idempoten. Bila kelak ada job non-idempoten, tambahkan penanda "terakhir jalan" di tabel (butuh migrasi, di luar T00).
- **Keterikatan koneksi**: advisory lock level sesi + `database/sql` pool. Jangan pernah memakai `s.db.ExecContext` untuk lock/unlock (bisa mendarat di koneksi berbeda). Selalu lewat `*sqlx.Conn` yang sama.
- **Skew jam** antar instance hanya memengaruhi seberapa dekat tick; lock tetap benar.
- **Tidak ada penundaan start (jitter)**; tidak perlu karena lock.
- Tidak ada endpoint admin untuk memicu job manual; di luar scope T00.
