# T02 core/audit-log — Usecase `purgeexpired` (plan)

Tiket: `docs/tickets/T02-core-audit-log.md` (AC6, AC7). Bagian bersama (migrasi, `base/`, wiring `base.go`, arch test) ada di `overview.md`; dokumen ini hanya memuat usecase `purgeexpired`.

## Metadata

| Field | Nilai |
|---|---|
| Scope | backend saja (tidak ada FE, tidak ada HTTP) |
| Domain | core |
| Module | audit-log (folder Go `auditlog`) |
| Feature | `purgeexpired` |
| Resource root | `backend/internal/domain/core/internal/auditlog` |
| allowedPaths | `backend/internal/domain/core/internal/auditlog/purgeexpired/` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| job `core.auditlog.purge-expired` | DailyAt `03:15` (UTC) | v1 | sistem (scheduler internal) |
| (HTTP) | tidak ada | - | - |
| (bus) | tidak menyediakan, tidak mengonsumsi | - | - |

## Summary Plan

Job harian yang menghapus entri `audit_log` dengan `occurred_at < cutoff`, dengan `cutoff = base.PurgeCutoff(now, AUDIT_RETENTION_DAYS)` dihitung SEKALI per run. Penghapusan dilakukan dalam batch 5000 baris dengan jeda 100 ms antar batch (agar tabel tidak terkunci lama dan WAL tidak melonjak), berhenti bila batch terakhir parsial (`affected < batch`) atau ctx selesai. Repository memegang satu-satunya `DELETE` di module (diverifikasi arch test W2 yang mengizinkan `delete from audit_log` hanya di `purgeexpired/repository.go`). Usecase satu file, tanpa dipecah. Error dikembalikan ke scheduler (status job "error", di-log scheduler); job tidak menulis entri audit dan tidak mengonsumsi bus. Idempoten: run ulang aman karena kriteria berbasis waktu.

Catatan foundation (`infra/jobs`): scheduler memegang Postgres advisory lock per nama job (aman multi-instance), `Run` dipanggil dengan ctx yang dibatalkan saat Stop, dan `Register` panic bila nama/jadwal invalid atau duplikat. Tidak ada accessor untuk membaca job terdaftar, sehingga tes pendaftaran memakai seam `newJob` (lihat Plan langkah 3).

Prasyarat (dikerjakan foundation F1/F4, bukan bagian plan ini): tabel `audit_log` + indeks `audit_log_occurred_idx (occurred_at DESC, id DESC)`, `base.PurgeCutoff`, `Config.AuditRetentionDays`. Subquery `ORDER BY occurred_at LIMIT n` memakai indeks itu (scan mundur), jadi tanpa seq scan. `base.PurgeCutoff` WAJIB sudah ada sebelum usecase ini dikompilasi.

## Plan

Urutan wajib. Semua file di `backend/internal/domain/core/internal/auditlog/purgeexpired/`, `package purgeexpired`. Import path memakai nama final di `main` (saat ini `grimoire/internal/infra/app`, `grimoire/internal/infra/jobs`, `grimoire/internal/infra/db`); jangan membuat alias.

1. `params.go` (tes tidak bergantung pada file lain)
   ```go
   const (
       BatchSize = 5000
       Pause     = 100 * time.Millisecond
   )

   // Options mengatur loop; nilai nol dipakai default produksi kecuali Pause (lihat WithDefaults).
   type Options struct {
       Retention int           // hari, dari Config.AuditRetentionDays
       BatchSize int           // <= 0 -> BatchSize
       Pause     time.Duration // < 0 -> Pause; 0 = tanpa jeda (tes)
   }
   ```
   Keputusan: agar tes dapat `Pause: 0` tetapi produksi default 100 ms, `Pause` bertipe `*time.Duration` TIDAK dipakai; sebagai gantinya `init.go` selalu mengisi `Options{Retention: days, BatchSize: BatchSize, Pause: Pause}` secara eksplisit, dan `usecase.New` hanya menormalkan `BatchSize <= 0 -> BatchSize` dan `Pause < 0 -> 0`. Dengan begitu nol = tanpa jeda tanpa ambiguitas. `Retention <= 0` ditolak oleh `New` (panic wiring, rule 5.9: nilai ini sudah divalidasi config min 1, jadi mustahil di runtime) agar tidak menghapus semuanya secara tidak sengaja.

2. `repository.go` (satu-satunya DELETE module)
   ```go
   type Repository struct{ db db.Querier } // atau *db.DB; pilih yang dipakai foundation untuk ExecContext

   func NewRepository(q db.Querier) *Repository

   const deleteBatchSQL = `DELETE FROM audit_log
   WHERE id IN (
       SELECT id FROM audit_log
       WHERE occurred_at < $1
       ORDER BY occurred_at
       LIMIT $2
   )`

   // DeleteBatch menghapus sampai limit entri dengan occurred_at < cutoff; mengembalikan RowsAffected.
   func (r *Repository) DeleteBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
       res, err := r.db.ExecContext(ctx, deleteBatchSQL, cutoff, limit)
       if err != nil { return 0, fmt.Errorf("purge audit_log batch: %w", err) }
       n, err := res.RowsAffected()
       if err != nil { return 0, fmt.Errorf("purge audit_log rows affected: %w", err) }
       return n, nil
   }
   ```
   Tanpa transaksi multi-statement: tiap batch auto-commit (itu tujuannya, kunci singkat). Argumen `$n` saja. Kata `delete from audit_log` ditulis persis seperti di atas (case-insensitive, satu spasi) agar cocok dengan regex arch test W2; JANGAN menaruh string SQL itu di file lain. Komentar di file ini tidak boleh memuat frasa `update audit_log`.

3. `usecase.go`
   ```go
   type repository interface {
       DeleteBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error)
   }

   type Usecase struct {
       repo repository
       opt  Options
       now  func() time.Time   // default time.Now; diganti tes
       log  *slog.Logger
   }

   func New(repo repository, opt Options, now func() time.Time, log *slog.Logger) *Usecase

   // Run adalah isi job. Cutoff dihitung sekali; loop sampai batch parsial atau ctx selesai.
   func (u *Usecase) Run(ctx context.Context) error {
       start := u.now()
       cutoff := base.PurgeCutoff(start, u.opt.Retention)
       var total int64; var batches int
       for {
           if err := ctx.Err(); err != nil { log (info total sejauh ini); return err }
           n, err := u.repo.DeleteBatch(ctx, cutoff, u.opt.BatchSize)
           if err != nil { log error dgn total_deleted/batches; return err }   // dikembalikan apa adanya (sudah dibungkus repo)
           total += n; batches++
           if n < int64(u.opt.BatchSize) { break }
           if u.opt.Pause > 0 { select { case <-ctx.Done(): log; return ctx.Err(); case <-time.After(u.opt.Pause): } }
       }
       log info "audit purge done" total_deleted, batches, duration_ms (time.Since / now-based)
       return nil
   }
   ```
   Detail:
   - `duration_ms` dihitung dari `time.Since(startWall)` memakai jam nyata (bukan `u.now`, yang bisa dibekukan tes); `u.now` hanya untuk cutoff.
   - Cutoff tidak dihitung ulang di dalam loop (run panjang tidak "mengejar" waktu; deterministik).
   - Ctx dibatalkan: kembalikan `ctx.Err()` (scheduler mencatat job berhenti saat shutdown); log info (bukan error) dengan total sejauh ini. Batch yang sedang berjalan saat ctx dibatalkan diserahkan ke driver (`ExecContext` mengembalikan error ctx); itu tetap dikembalikan sebagai error terbungkus yang `errors.Is(err, context.Canceled)`.
   - Log hanya angka, tanpa data baris (rule 14.2/7.6). Log level info saat selesai sukses; level error TIDAK dipakai di sini karena scheduler sudah me-log error job.
   - Tidak memanggil bus dan tidak menulis audit (melihat/menghapus karena retensi tidak ada di katalog aksi).
   - Batas keamanan: tidak ada batas jumlah batch (retensi pertama kali pada tabel besar harus tuntas); jeda 100 ms + ctx cukup.

4. `handler.go` (adaptor scheduler, tanpa HTTP)
   ```go
   type runner interface{ Run(ctx context.Context) error }

   // Run adalah adaptor Run(ctx) error untuk jobs.Job.
   func Handler(uc runner) func(ctx context.Context) error { return uc.Run }
   ```
   Tipis: tidak ada logika, tidak ada decode. Alasan file tetap ada: konvensi folder feature (rule 5.4/5.5) dan tempat tunggal bila nanti perlu recover/label.

5. `init.go`
   ```go
   const (
       JobName = "core.auditlog.purge-expired"
       DailyAt = "03:15"
   )

   // newJob merakit definisi job (dipisah agar tes membaca Name/DailyAt tanpa scheduler).
   func newJob(run func(context.Context) error) jobs.Job {
       return jobs.Job{Name: JobName, DailyAt: DailyAt, Run: run}
   }

   func Init(d app.Deps) {
       repo := NewRepository(d.DB)
       uc := New(repo, Options{Retention: d.Config.AuditRetentionDays, BatchSize: BatchSize, Pause: Pause}, time.Now, d.Log)
       d.Jobs.Register(newJob(Handler(uc)))
   }
   ```
   Hanya `Every` ATAU `DailyAt` (di sini `DailyAt`; `Every` kosong). Registrasi ganda = panic dari scheduler (disengaja, wiring error). Signature `Init(d app.Deps)` sesuai overview; dipanggil dari `auditlog/base.go` (W1, bukan bagian plan ini). `d.Log` dipakai apa adanya; bila foundation menyediakan logger ber-atribut module, pakai itu tanpa mengubah kontrak.

6. Tes (lihat Test Scenario): `usecase_test.go` (unit, repo palsu), `repository_integration_test.go` dan `init_integration_test.go` (tag `integration`).

7. Verifikasi lokal sebelum selesai: `go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auditlog/purgeexpired/...`, lalu dengan DB: `go test -tags integration ./internal/domain/core/internal/auditlog/purgeexpired/...`. Grep: `grep -rniE "update\s+audit_log|truncate" .../purgeexpired/` harus kosong.

## Migration

Tidak ada. Memakai tabel `audit_log` dari `backend/migrations/core/20261021090000000_create_audit_log.sql` (foundation F1). Trigger append-only hanya memblokir UPDATE, sehingga DELETE job ini tidak terhalang. Catatan deploy (sudah di `docs/configuration.md` F8): role DB aplikasi tetap butuh hak DELETE pada `audit_log`.

## Changes

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/auditlog/purgeexpired/params.go` | buat | `BatchSize`, `Pause`, `Options` |
| `.../purgeexpired/repository.go` | buat | `Repository`, `DeleteBatch` (satu-satunya DELETE) |
| `.../purgeexpired/usecase.go` | buat | `Usecase`, `New`, `Run` (loop batch, log ringkasan) |
| `.../purgeexpired/handler.go` | buat | adaptor `Run(ctx) error` untuk scheduler |
| `.../purgeexpired/init.go` | buat | `Init(d app.Deps)`, `newJob`, konstanta `JobName`/`DailyAt` |
| `.../purgeexpired/usecase_test.go` | buat | unit dengan repo palsu |
| `.../purgeexpired/repository_integration_test.go` | buat | AC6 dengan Postgres nyata |
| `.../purgeexpired/init_integration_test.go` | buat | job terdaftar dengan nama dan jadwal benar |

Tidak menyentuh file di luar folder `purgeexpired/` (wiring `auditlog/base.go`, `base/retention.go`, arch test, dokumen dikerjakan foundation/wiring). Tidak ada env baru, tidak ada dependensi baru, tidak ada kontrak bus baru.

## Test Scenario

Penamaan: `TestT02_AC6_*` untuk bukti kriteria (rule 13.4), lainnya `Test<Unit>_<Kasus>`. Unit tanpa DB; integrasi tag `integration` dengan `db.TestDB(t)` (rule 13.3), tabel dibersihkan di awal tiap tes (`DELETE FROM audit_log` di helper tes diperbolehkan hanya karena file `_test.go`; arch test W2 mengecualikan hanya `arch_test.go`, jadi helper pembersih memakai SQL `TRUNCATE`-free: gunakan transaksi/skema tes dari `db.TestDB` atau hapus lewat `DELETE FROM audit_log` ditulis bukan sebagai literal `delete from audit_log` utuh, mis. `"DELETE FROM " + "audit_log"`. Bila `db.TestDB` sudah mengisolasi per tes, pembersihan tidak perlu).

### Unit `usecase_test.go` (repo palsu mencatat panggilan `(cutoff, limit)` dan mengembalikan urutan hasil terskrip; clock tetap; logger menulis ke buffer)

- `TestRun_StopsOnPartialBatch`: skrip `[3,3,2]` dengan BatchSize 3 -> 3 panggilan, return nil, tidak ada panggilan ke-4; log berisi `total_deleted=8`, `batches=3`.
- `TestRun_StopsOnEmptyFirstBatch`: skrip `[0]` -> 1 panggilan, nil, `total_deleted=0`, `batches=1`.
- `TestRun_FullBatchThenZero`: skrip `[3,0]` -> 2 panggilan (batch penuh memicu satu panggilan lagi), nil.
- `TestRun_CutoffComputedOnce`: clock palsu yang maju tiap dipanggil; semua panggilan repo menerima `cutoff` identik, sama dengan `base.PurgeCutoff(t0, days)`; clock hanya dipanggil sekali.
- `TestRun_CutoffValue`: now = `2026-10-20T03:15:00Z`, days 90 -> cutoff `2026-07-22T03:15:00Z`; days 30 -> `2026-09-20T03:15:00Z`.
- `TestRun_PassesBatchSize`: limit yang diterima repo = `Options.BatchSize`; `BatchSize` 0 -> 5000 (default).
- `TestRun_RepoErrorReturned`: repo mengembalikan error pada batch ke-2 -> `Run` mengembalikan error yang `errors.Is(err, sentinel)`; tidak ada panggilan sesudahnya; log memuat `total_deleted` batch sebelumnya.
- `TestRun_ContextCanceledBeforeStart`: ctx sudah batal -> repo tidak dipanggil, `errors.Is(err, context.Canceled)`.
- `TestRun_ContextCanceledDuringPause`: skrip tak berujung `[3,3,3,...]`, `Pause` 1 jam, ctx dibatalkan setelah panggilan pertama (repo palsu memanggil `cancel()` di dalamnya) -> `Run` kembali cepat (< 1 s, beri batas waktu tes) dengan `context.Canceled`; hanya 1 panggilan repo.
- `TestRun_ContextCanceledBetweenBatches_NoPause`: `Pause` 0, ctx dibatalkan oleh repo palsu di batch ke-2 -> berhenti sebelum batch ke-3.
- `TestRun_LogsSummaryOnSuccess`: log info `"audit purge done"` berisi kunci `total_deleted`, `batches`, `duration_ms`; tidak ada log level error pada sukses.
- `TestNew_RejectsNonPositiveRetention`: `Retention` 0 -> panic (wiring).
- `TestHandler_DelegatesToRun`: `Handler(fake)` memanggil `Run` dengan ctx yang sama dan meneruskan errornya.

### Integrasi `repository_integration_test.go`

Helper `seed(t, db, occurredAt, n)` mengisi `n` baris minimal valid (actor `system`, action `x.y`, status `ok`) dengan `id` uuid v7 unik.

- `TestT02_AC6_PurgeRemovesOldKeepsNew`: seed 10 entri umur 91 hari, 3 entri umur 89 hari, 2 entri baru (`now`); `Usecase.Run` dengan `Options{Retention: 90, BatchSize: 3, Pause: 0}` dan clock = `time.Now` -> sisa tepat 5 baris (3 umur 89 hari + 2 baru), tidak ada baris dengan `occurred_at < now-90d`. Batch yang dipakai: 10 baris lama / 3 = 4 batch (3,3,3,1) sehingga `batches` = 4 (diverifikasi lewat log buffer atau repo pembungkus penghitung) -> membuktikan beberapa batch.
- `TestT02_AC6_ExactMultipleOfBatch`: seed 6 lama dengan BatchSize 3 -> 3 panggilan (3,3,0), hasil 0 lama.
- `TestT02_AC6_BoundaryExclusive`: entri tepat pada `cutoff` dan 1 detik sesudahnya tetap ada (kriteria `<` ketat); entri 1 detik sebelum cutoff hilang (pakai clock tetap agar deterministik).
- `TestDeleteBatch_RespectsLimitAndOrder`: seed 5 lama dengan `occurred_at` berbeda; `DeleteBatch(cutoff, 2)` mengembalikan 2 dan yang terhapus adalah dua `occurred_at` tertua.
- `TestDeleteBatch_NothingToDelete`: tabel kosong atau semua baru -> 0, nil.
- `TestDeleteBatch_NotBlockedByAppendOnlyTrigger`: sebagai bukti bahwa trigger UPDATE tidak menghalangi DELETE (satu baris terhapus tanpa error).
- `TestRun_DBError`: gunakan koneksi yang ditutup / ctx dengan deadline lampau -> `Run` mengembalikan error tanpa panic.
- (Opsional bila `db.TestDB` mendukung) `TestPurge_QueryUsesIndex`: `EXPLAIN` subquery pada 20 ribu baris tidak memuat `Seq Scan on audit_log` (jaga performa batch); dilewati bila terlalu rapuh.

### Integrasi `init_integration_test.go`

- `TestInit_RegistersJob`: `newJob(run)` mengembalikan `Name == "core.auditlog.purge-expired"`, `DailyAt == "03:15"`, `Every == 0`, `Run != nil`.
- `TestInit_SchedulerAcceptsJob`: `s := jobs.New(db, log)`; `Init(app.Deps{DB: db, Jobs: s, Log: log, Config: config.Config{AuditRetentionDays: 90}})` tidak panic (nama dan `DailyAt` lolos validasi `Register`); pemanggilan `Init` kedua dengan scheduler yang sama panic (`registered twice`), yang membuktikan job benar-benar terdaftar dengan nama itu.
- `TestInit_UsesConfiguredRetention`: `AuditRetentionDays: 7` -> seed entri 8 hari dan 6 hari lalu, jalankan `Run` dari `newJob` hasil `Init` yang ditangkap lewat seam (uji `Init` internal dengan fungsi `build(d) *Usecase` bila perlu) -> yang 8 hari hilang, yang 6 hari tetap. Bila seam ini dianggap berlebihan, cukup andalkan `TestT02_AC6_*` dan tes konfigurasi foundation.

Bukti AC7 (hanya `purgeexpired` yang DELETE): ditegakkan arch test W2 milik wiring; plan ini hanya memastikan literal SQL berada di `purgeexpired/repository.go` dan tidak ada UPDATE/TRUNCATE.

## E2E Scenarios

Tidak ada UI maupun HTTP; e2e dilakukan di level proses dengan scheduler nyata atau pemanggilan langsung.

Positif:
1. Boot server dengan `AUDIT_RETENTION_DAYS=90`, seed entri 91 hari, 89 hari, dan baru; picu job (tes memanggil `Run` yang sama dengan yang didaftarkan, atau jalankan scheduler dengan jam yang digeser) -> hanya entri 91 hari hilang (AC6). Log aplikasi memuat `audit purge done` dengan `total_deleted`, `batches`, `duration_ms`.
2. Tabel besar (mis. 25.000 lama): 5 batch penuh + 1 batch kosong; setiap batch commit sendiri, query baca `GET /api/audit-log` tetap responsif selama purge (tidak ada kunci tabel panjang).
3. Dua instance berjalan bersamaan: advisory lock scheduler membuat hanya satu yang menjalankan job (perilaku foundation; purge idempoten sehingga aman).
4. `AUDIT_RETENTION_DAYS=7` mengubah cutoff tanpa perubahan kode.
5. Entri audit baru yang ditulis saat purge berjalan (occurred_at = sekarang) tidak pernah terhapus.

Negatif:
1. DB tidak terjangkau saat job jalan -> `Run` mengembalikan error, scheduler mencatat status "error", proses tidak crash; jadwal berikutnya (hari berikut) mencoba lagi.
2. Shutdown (SIGTERM) saat purge berjalan -> ctx dibatalkan, loop berhenti di batas batch/jeda, `Run` mengembalikan `context.Canceled`, tidak ada batch setengah jadi; sisa dihapus pada run berikutnya.
3. Salah satu batch gagal (mis. deadlock/timeout) -> error dikembalikan, batch sebelumnya tetap terhapus (commit terpisah), tidak ada rollback massal.
4. `Retention <= 0` tidak mungkin lolos config (min 1) dan `usecase.New` panic bila terjadi di wiring, sehingga tidak ada penghapusan total tak sengaja.
5. Tidak ada endpoint HTTP: `DELETE /api/audit-log` tetap 405 (AC7), tidak terpengaruh feature ini.

## Request/Response body

Tidak ada request/response HTTP maupun bus. Kontrak yang relevan:

Definisi job (dibaca scheduler):

```go
jobs.Job{
    Name:    "core.auditlog.purge-expired",
    DailyAt: "03:15", // UTC
    Run:     handler, // func(ctx context.Context) error
}
```

SQL repository (parameter: `$1` = cutoff `timestamptz`, `$2` = limit `int`):

```sql
DELETE FROM audit_log
WHERE id IN (
    SELECT id FROM audit_log
    WHERE occurred_at < $1
    ORDER BY occurred_at
    LIMIT $2
)
```

Log sukses (JSON slog, level info; `trace_id` tidak ada karena bukan request):

```json
{"time":"2026-10-21T03:15:02.412Z","level":"INFO","msg":"audit purge done","total_deleted":10,"batches":4,"duration_ms":312.5}
```

Log dihentikan ctx (info) dan error repo (error ditangani scheduler; usecase hanya menambah konteks):

```json
{"level":"INFO","msg":"audit purge interrupted","total_deleted":6000,"batches":2,"duration_ms":120.1,"err":"context canceled"}
{"level":"ERROR","msg":"audit purge failed","total_deleted":5000,"batches":1,"duration_ms":40.7,"err":"purge audit_log batch: connection refused"}
```

Format error: tidak ada respons klien. Error yang dikembalikan `Run` (nilai `err.Error()`): `purge audit_log batch: <driver error>`, `purge audit_log rows affected: <err>`, atau `context canceled` / `context deadline exceeded`; dicatat scheduler sebagai hasil job "error".
