# T00 / db-migrations — Plan Usecase

Induk: `docs/superpowers/plans/foundation/overview.md` (keputusan bersama di sana mengikat). Tiket: `docs/tickets/T00-foundation.md` (F3, AC5). Aturan: `rules.txt` 5.3, 5.7, 5.9, 8.3, 4.6, 13.3.

Usecase ini bergantung pada `repo-skeleton-config` (go.mod, `infra/config.Config`, `infra/logger`). Tidak bergantung pada `bus`, `jobs`, atau `http-server`. Dua usecase lain bergantung padanya: `jobs-scheduler` (memakai `*db.DB`, `db.Conn`) dan `cli-and-wiring` (memakai `Open`, `Up`).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 (F3, AC5) |
| Usecase | `db-migrations` |
| Scope | backend (Go) saja |
| Domain / Module | infra / foundation (package `db` dan `migrations`, bukan module domain) |
| Resource root | `backend/internal/infra/db`, `backend/migrations` |
| Endpoint HTTP | tidak ada |
| Method | - |
| Version | - |
| Endpoint bus (provider/consumer) | tidak ada |
| Aksi audit | tidak ada |
| Konsumen API Go | `jobs-scheduler` (`db.DB`, `Conn`), `http-server` (`Pinger` dipenuhi `*db.DB`), `cli-and-wiring` (`Open`, `Up`, `Close`), semua repository tiket lain (`BeginTx`, `Querier`), semua test integrasi (`TestDB`) |

Tidak ada pola `init/handler/params/usecase/repository`. Usecase ini murni package infra. Infra DILARANG meng-import `internal/domain/**` (rules 4.6). Package `db` boleh meng-import `internal/infra/config` dan `grimoire/migrations`.

---

## 2. Summary Plan

Menyediakan tiga hal:

1. **Koneksi** (`db.go`): `Open(ctx, cfg)` membuat pool sqlx di atas pgx stdlib dengan setelan dari `config.Config`, ping saat open, `Ping(ctx)` untuk `/readyz`, `Close()`.
2. **Transaksi** (`tx.go`): `BeginTx(ctx, func(tx *sqlx.Tx) error) error` (commit bila nil, rollback bila error, rollback lalu panic diteruskan bila panic) dan `Querier` agar repository bisa menerima DB maupun Tx.
3. **Runner migrasi** (`migrate.go`, `up.go`, `backend/migrations/embed.go`): file SQL ter-embed dari `migrations/{core,project}`, nama divalidasi `^\d{17}_[a-z0-9_]+\.sql$`, urutan versi global lintas folder, versi duplikat ditolak, `schema_migrations` dibuat otomatis, satu file = satu transaksi bersama insert versi, `pg_advisory_lock` selama seluruh proses, forward-only. `Up(ctx, d, log)` memakai FS ter-embed dan dipanggil `cmd migrate up` serta `RUN_MIGRATIONS_ON_START`.
4. **Helper test** (`testing.go`): `TestDB(t)` memberi DB dengan schema unik per test untuk semua test integrasi tiket lain.

### Keputusan desain

| # | Keputusan | Alasan |
|---|---|---|
| D1 | Runner menjalankan SQL lewat `pgx.Conn` mentah (`sql.Conn.Raw` -> `*stdlib.Conn`.`Conn()`), memakai `pgx.QueryExecModeSimpleProtocol` untuk `Exec` isi file | Satu file bisa berisi banyak statement (`CREATE TABLE ...; CREATE INDEX ...;`). Extended protocol menolak multi-statement. Simple protocol tanpa argumen menerimanya. Eksplisit lebih aman daripada mengandalkan perilaku default driver |
| D2 | Lock, transaksi migrasi, dan insert versi semuanya di SATU koneksi yang sama (satu `sql.Conn` khusus) | Dengan `DB_MAX_OPEN_CONNS=1` (nilai sah) transaksi dari pool akan deadlock menunggu koneksi yang dipegang lock. Satu koneksi juga menjamin lock dan unlock di session yang sama |
| D3 | Validasi seluruh FS (nama, duplikat, urutan) dilakukan SEBELUM menyentuh DB | "Sebelum apa pun dieksekusi" (overview bagian 6). Error validasi tidak butuh DB dan bisa dites unit |
| D4 | `schema_migrations` dibuat DI DALAM lock (setelah lock didapat) | `CREATE TABLE IF NOT EXISTS` bisa balapan di dua runner (unique violation di `pg_type`). Lock menyerialkan |
| D5 | Unlock gagal -> koneksi dibuang dari pool (`driver.ErrBadConn` lewat `Raw`) | Lock advisory milik session. Mengembalikan koneksi ke pool dengan lock tersisa akan memblokir runner berikutnya selamanya |
| D6 | `//go:embed all:core all:project` | Tanpa `all:`, file berawalan `.` (`.keep`) tidak di-embed, direktori kosong membuat build gagal ("contains no embeddable files"). Overview menulis `//go:embed core project`; ini koreksi yang wajib |
| D7 | `schema_migrations.name` = nama file dasar (`<versi>_<nama>.sql`); `MigrateResult.Applied` = path relatif FS (`core/<versi>_<nama>.sql`) | Name unik secara semantik, Applied informatif untuk CLI dan log |
| D8 | File non-`.sql` diabaikan (`.keep`, README). Hanya file ber-ekstensi `.sql` yang divalidasi regex | Folder kosong tetap sah. Nama `.SQL` atau `x.sql.bak` bukan `.sql` dan diabaikan; yang berakhiran tepat `.sql` tetapi salah nama ditolak |
| D9 | Isi file DILARANG berisi `BEGIN`/`COMMIT` sendiri dan `CREATE INDEX CONCURRENTLY`, `VACUUM`, dan statement lain yang menolak transaksi | Runner membungkus transaksi. Didokumentasikan di `docs/configuration.md` bagian Migrasi. Tidak dideteksi otomatis (kompleksitas tak sebanding); gagal di Postgres dengan pesan jelas lalu rollback |
| D10 | Tidak ada checksum, tidak ada down, versi di DB yang tak ada di FS hanya warning | Sesuai overview. Rules 8.3: migrasi yang sudah dirilis tidak diedit (konvensi, bukan dicek runner) |
| D11 | `Up` = fungsi paket di `db`, memakai `migrations.FS`; `Migrate` menerima `fs.FS` agar bisa dites | Overview bagian 6 mendefinisikan `Migrate(ctx, d, fsys, log)`; `Up` hanya pembungkus tipis |
| D12 | Tidak ada timeout internal pada lock | Waktu tunggu diatur ctx pemanggil (CLI: sinyal; server: ctx start). Menunggu instance lain selesai migrasi adalah perilaku yang benar |

---

## 3. Plan (langkah berurutan)

Mengikuti TDD: tulis test unit validasi dulu (langkah 3), baru implementasi.

### Langkah 1 — Dependensi dan embed

1. Tambah ke `backend/go.mod`: `github.com/jackc/pgx/v5` dan `github.com/jmoiron/sqlx` (versi stabil terbaru yang dikenal, dipin lewat `go get`; jalankan `go mod tidy`).
2. Buat `backend/migrations/embed.go`:
   ```go
   // Package migrations meng-embed file migrasi SQL. Tidak berisi logika.
   package migrations

   import "embed"

   // all: agar .keep ikut di-embed dan folder kosong tidak menggagalkan build.
   //
   //go:embed all:core all:project
   var FS embed.FS
   ```
3. Pastikan `backend/migrations/core/.keep` dan `backend/migrations/project/.keep` ada (dibuat `repo-skeleton-config`; bila belum, buat di sini).

### Langkah 2 — Koneksi (`backend/internal/infra/db/db.go`)

```go
package db

type DB struct{ *sqlx.DB }

// Open membuat pool dari cfg dan memastikan DB terjangkau (ping timeout 5 detik).
func Open(ctx context.Context, cfg config.Config) (*DB, error)

// Ping dipakai /readyz (memenuhi httpx.Pinger).
func (d *DB) Ping(ctx context.Context) error

// Close: warisan dari *sqlx.DB (tidak didefinisikan ulang).
```

Implementasi:
1. Fungsi internal `open(ctx, cfg, runtimeParams map[string]string) (*DB, error)`; `Open` memanggilnya dengan `nil`. `TestDB` memakai `runtimeParams{"search_path": schema}`.
2. `pgx.ParseConfig(cfg.DatabaseURL)`; error dibungkus `fmt.Errorf("db: parse DATABASE_URL: %w", err)`. Test memastikan pesan tidak memuat password (pgx menyamarkan password di `ParseConfigError`; jangan menambah `cfg.DatabaseURL` ke pesan).
3. Salin `runtimeParams` ke `connCfg.RuntimeParams`.
4. `sqlDB := stdlib.OpenDB(*connCfg)`; `x := sqlx.NewDb(sqlDB, "pgx")`.
5. `SetMaxOpenConns(cfg.DBMaxOpenConns)`, `SetMaxIdleConns(cfg.DBMaxIdleConns)`, `SetConnMaxLifetime(cfg.DBConnMaxLifetime)`.
6. `pctx, cancel := context.WithTimeout(ctx, 5*time.Second)`; `x.PingContext(pctx)`; gagal -> `x.Close()` lalu `fmt.Errorf("db: ping: %w", err)`.
7. `Ping(ctx)` = `d.PingContext(ctx)` dibungkus `fmt.Errorf("db: ping: %w", err)`. Timeout 2 detik diatur pemanggil (`/readyz`).
8. `Conn` tidak didefinisikan ulang: `jobs-scheduler` memakai `d.DB.Conn(ctx)` (`*sql.Conn`) bawaan sqlx.

### Langkah 3 — Test unit validasi (merah dulu)

Tulis `migrate_test.go` (lihat bagian 6) untuk `plan()` sebelum implementasi.

### Langkah 4 — Runner (`backend/internal/infra/db/migrate.go`)

Tipe dan sentinel:

```go
var (
    ErrInvalidMigrationName = errors.New("db: invalid migration file name")
    ErrDuplicateVersion     = errors.New("db: duplicate migration version")
)

type MigrateResult struct{ Applied []string } // path relatif FS, urut eksekusi

type migration struct {
    Version int64  // 17 digit pertama
    Name    string // nama file dasar
    Path    string // path di FS, mis. core/20261020080000000_create_users.sql
}

var migrationNameRE = regexp.MustCompile(`^(\d{17})_[a-z0-9_]+\.sql$`)

// plan: baca + validasi + urutkan. Murni (tanpa DB), mudah dites.
func plan(fsys fs.FS) ([]migration, error)

func Migrate(ctx context.Context, d *DB, fsys fs.FS, log *slog.Logger) (MigrateResult, error)
```

`plan(fsys)`:
1. `fs.WalkDir(fsys, ".", ...)`. Lewati direktori. Lewati file yang tidak berakhiran `.sql` (D8). Galat walk dikembalikan dibungkus.
2. Untuk file `.sql`: cocokkan `migrationNameRE` pada nama dasar. Tidak cocok -> `fmt.Errorf("%w: %s (harus ^\\d{17}_[a-z0-9_]+\\.sql$)", ErrInvalidMigrationName, path)`. Hentikan di pelanggaran pertama berdasarkan urutan walk (leksikal, deterministik).
3. `strconv.ParseInt(version, 10, 64)` (17 digit < 9,2e18, aman).
4. Versi sama dengan entri sebelumnya -> `fmt.Errorf("%w: %d di %s dan %s", ErrDuplicateVersion, v, pathA, pathB)`.
5. Urutkan naik berdasarkan `Version` (lintas folder).

`Migrate(ctx, d, fsys, log)`:
1. `log == nil` -> `slog.Default()`. `list, err := plan(fsys)`; error dikembalikan sebelum menyentuh DB (D3).
2. `conn, err := d.DB.Conn(ctx)` (`*sql.Conn` khusus, D2). `defer conn.Close()`.
3. Seluruh pekerjaan di dalam `conn.Raw(func(driverConn any) error { pc := driverConn.(*stdlib.Conn).Conn(); ... })` (D1). Hasil dikumpulkan lewat variabel tertutup.
4. Lock: `pc.Exec(ctx, "SELECT pg_advisory_lock(hashtext('grimoire.migrate'))")`. Pakai `QueryExecModeSimpleProtocol`. Tunggu terbatas ctx.
5. Unlock (defer, ctx bebas cancel: `context.WithoutCancel(ctx)` + timeout 5 detik): `SELECT pg_advisory_unlock(hashtext('grimoire.migrate'))`. Gagal atau hasil `false` -> log error dan tandai koneksi buruk: setelah `Raw` selesai, panggil `conn.Raw(func(any) error { return driver.ErrBadConn })` agar dibuang dari pool (D5).
6. Buat jejak: `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())` (D4).
7. Baca versi terpasang: `SELECT version FROM schema_migrations` -> `map[int64]struct{}`. Versi di DB yang tidak ada di `list` -> `log.WarnContext(ctx, "migration version in database but not in binary", "version", v)` (tidak error).
8. Untuk tiap `m` dalam `list` yang belum terpasang, berurutan:
   1. `sqlBytes, err := fs.ReadFile(fsys, m.Path)`.
   2. `tx, err := pc.Begin(ctx)`.
   3. `tx.Exec(ctx, string(sqlBytes), pgx.QueryExecModeSimpleProtocol)`; error -> `tx.Rollback(ctxTanpaCancel)`, return `fmt.Errorf("migration %s: %w", m.Path, err)`.
   4. `tx.Exec(ctx, "INSERT INTO schema_migrations(version, name) VALUES ($1, $2)", m.Version, m.Name)` (extended protocol, parameterized; rules 5.3). Error -> rollback + return dengan nama file.
   5. `tx.Commit(ctx)`; error -> return dengan nama file.
   6. Tambah `m.Path` ke `Applied`. Log info `"migration applied"` dengan `version`, `file`, `duration_ms`.
   7. Berhenti pada error pertama (file berikutnya tidak dijalankan). `Applied` berisi file yang sudah commit sebelum gagal; dikembalikan bersama error agar CLI bisa mencetaknya.
9. Tidak ada yang diterapkan -> log info `"no new migrations"`. Return `MigrateResult{Applied: nil}`.
10. Panic di tengah (mis. type assertion driver gagal) tidak ditangkap; defer unlock tetap jalan.

Catatan: `driverConn.(*stdlib.Conn)` gagal bila `d` bukan dibuat lewat `Open` (mis. driver lain). Tangani dengan error eksplisit `"db: migrate requires pgx connection"`, bukan panic.

### Langkah 5 — `Up` (`backend/internal/infra/db/up.go`)

```go
// Up menjalankan semua migrasi ter-embed. Dipakai `grimoire migrate up` dan RUN_MIGRATIONS_ON_START.
func Up(ctx context.Context, d *DB, log *slog.Logger) (MigrateResult, error) {
    return Migrate(ctx, d, migrations.FS, log)
}
```

Tidak ada logika lain. Pencetakan "tidak ada migrasi baru" dan exit code milik `cli-and-wiring` (cukup memeriksa `len(res.Applied) == 0`).

### Langkah 6 — Transaksi (`backend/internal/infra/db/tx.go`)

```go
// Querier dipenuhi *sqlx.DB, *sqlx.Tx, dan *db.DB. Repository menerima Querier bila dipakai di dalam/luar transaksi.
type Querier interface {
    sqlx.ExtContext
    GetContext(ctx context.Context, dest any, query string, args ...any) error
    SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

var (
    _ Querier = (*sqlx.DB)(nil)
    _ Querier = (*sqlx.Tx)(nil)
    _ Querier = (*DB)(nil)
)

func (d *DB) BeginTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error
```

Perilaku `BeginTx`:
1. `tx, err := d.BeginTxx(ctx, nil)`; gagal -> `fmt.Errorf("db: begin tx: %w", err)`.
2. Pola `defer` dengan flag `done`:
   ```go
   done := false
   defer func() {
       if done { return }
       // fn panic (atau Goexit): rollback lalu biarkan panic naik.
       _ = tx.Rollback()
   }()
   err = fn(tx)
   if err != nil {
       done = true
       if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
           return errors.Join(err, fmt.Errorf("db: rollback: %w", rbErr))
       }
       return err                // apa adanya; errors.Is/As tetap bekerja
   }
   done = true
   if err := tx.Commit(); err != nil { return fmt.Errorf("db: commit: %w", err) }
   return nil
   ```
   Panic dari `fn` tidak di-recover: `defer` merollback, lalu runtime meneruskan panic apa adanya (nilai dan stack asli terjaga, lebih baik daripada recover lalu `panic(p)` ulang). Perilaku tetap sesuai spesifikasi "rollback lalu panic diteruskan".
3. Error dari `fn` dikembalikan tanpa dibungkus agar sentinel bisnis (`errs.ErrNotFound`, dst.) tetap lolos `errors.Is` (rules 5.8).
4. Dokumentasikan di komentar: `fn` tidak boleh memanggil `Commit`/`Rollback` sendiri; `fn` yang memanggil `BeginTx` bersarang membuka transaksi independen (tidak ada savepoint).

### Langkah 7 — Helper test (`backend/internal/infra/db/testing.go`)

```go
// TestDB mengembalikan DB terisolasi (schema unik) untuk test integrasi.
// TEST_DATABASE_URL kosong: t.Skip lokal; t.Fatal bila CI=true.
func TestDB(t testing.TB) *DB
```

1. Baca `TEST_DATABASE_URL`. Kosong: bila `os.Getenv("CI") == "true"` -> `t.Fatal("TEST_DATABASE_URL wajib di CI")`, selain itu `t.Skip("TEST_DATABASE_URL tidak di-set; lewati test integrasi")`.
2. Buat `config.Config` minimal manual (`DatabaseURL`, `DBMaxOpenConns: 10`, `DBMaxIdleConns: 2`, `DBConnMaxLifetime: time.Minute`), TIDAK lewat `config.Load` (tidak butuh `SESSION_SECRET`).
3. Koneksi admin (`open(ctx, cfg, nil)`), `CREATE SCHEMA t_<8 hex acak>`.
4. Kembalikan `open(ctx, cfg, {"search_path": schema})` sehingga semua tabel (termasuk `schema_migrations`) jatuh ke schema itu.
5. `t.Cleanup`: tutup DB test, `DROP SCHEMA <schema> CASCADE` lewat koneksi admin, tutup admin. Gagal drop -> `t.Logf`, bukan gagal.
6. Catatan: `pg_advisory_lock` bersifat database-wide, bukan per schema. Test migrasi di schema berbeda yang jalan paralel akan saling menunggu (aman, hanya serial). Jangan memakai `t.Parallel()` pada test migrasi yang menguji lock.
7. File `testing.go` mengimpor paket `testing` di kode non-test. Dapat diterima (flag testing tidak terdaftar tanpa `testing.Init`) dan diminta overview. Di `lint`, beri `//nolint` hanya bila `golangci-lint` memprotes.

### Langkah 8 — Test

Tulis test (bagian 6). Jalankan `go test ./internal/infra/db/...` (unit), lalu `go test -tags integration -race ./internal/infra/db/...` dengan `TEST_DATABASE_URL`.

### Langkah 9 — Dokumentasi

Tambah bagian "Migrasi" ke `docs/configuration.md` (file dibuat `repo-skeleton-config`; PR ini hanya menambah bagian): format nama, folder, urutan global, satu file = satu transaksi, larangan `BEGIN/COMMIT` dan `CREATE INDEX CONCURRENTLY`, forward-only (jangan edit migrasi yang sudah dirilis, buat baru), perilaku multi-instance (lock), versi di DB tak dikenal = warning. Tidak ada env var baru, jadi tabel env dan test T00-AC7 tidak berubah.

---

## 4. Migration

**Tidak ada file migrasi** (`YYYYMMDDHHMMSSmmm_*.sql`) di usecase ini. Folder `backend/migrations/{core,project}/` tetap hanya berisi `.keep`.

Tabel jejak dibuat runner saat `Migrate` berjalan (bukan file migrasi):

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    bigint      PRIMARY KEY,
    name       text        NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

Kontrak untuk tiket lain yang menulis migrasi:
- Nama `YYYYMMDDHHMMSSmmm_<snake_case>.sql`, versi unik global (dua folder tidak boleh punya versi sama).
- Satu file berjalan dalam satu transaksi; tanpa `BEGIN/COMMIT`, tanpa `CREATE INDEX CONCURRENTLY`.
- Ekstensi `citext` dan `pgcrypto`/`gen_random_uuid()` dibuat oleh migrasi pertama T01 (bukan T00).

---

## 5. Changes (per file/folder)

Baru:
- `backend/migrations/embed.go` (package `migrations`, `//go:embed all:core all:project`).
- `backend/internal/infra/db/db.go`: `DB`, `Open`, `open`, `Ping`.
- `backend/internal/infra/db/tx.go`: `BeginTx`, `Querier`.
- `backend/internal/infra/db/migrate.go`: `plan`, `Migrate`, `MigrateResult`, sentinel error.
- `backend/internal/infra/db/up.go`: `Up`.
- `backend/internal/infra/db/testing.go`: `TestDB`.
- `backend/internal/infra/db/migrate_test.go` (unit, tanpa DB).
- `backend/internal/infra/db/db_test.go` (unit: error parse URL tidak membocorkan password).
- `backend/internal/infra/db/migrate_integration_test.go`, `tx_integration_test.go`, `db_integration_test.go` (build tag `integration`).

Diubah:
- `backend/go.mod`, `backend/go.sum`: tambah pgx/v5, sqlx.
- `docs/configuration.md`: tambah bagian "Migrasi" (langkah 9). Tabel env tidak berubah.

Tidak diubah: `overview.md`, plan usecase lain, `rules.txt`, README tiket, katalog bus (tidak ada endpoint bus baru).

Dependensi arah import (diverifikasi `architecture-guard` nanti): `db` -> `config`, `migrations`; `db` TIDAK -> `domain`, `bus`, `http`.

---

## 6. Test Scenario

Penamaan mengikuti rules 13.4. Integrasi memakai Postgres nyata (rules 13.3), diberi `//go:build integration`, skip bila `TEST_DATABASE_URL` kosong, gagal bila `CI=true`. Jalankan dengan `-race`.

### Unit (tanpa DB, `fstest.MapFS`)

`TestPlan_*` pada `plan(fsys)`:
- `TestPlan_ValidOrderAcrossFolders`: `project/20261020080000002_b.sql`, `core/20261020080000001_a.sql`, `core/20261020080000003_c.sql` -> urutan a, b, c (versi global, bukan per folder).
- `TestPlan_IgnoresNonSQL`: `core/.keep`, `project/.keep`, `core/README.md` diabaikan; FS hanya berisi itu -> daftar kosong, tanpa error.
- `TestPlan_InvalidNamesRejected` (tabel, tiap kasus `errors.Is(err, ErrInvalidMigrationName)` dan pesan memuat path):
  - versi 16 digit (`2026102008000000_x.sql`)
  - versi 18 digit
  - tanpa nama (`20261020080000000.sql`, `20261020080000000_.sql`)
  - huruf besar (`20261020080000000_Create.sql`)
  - tanda hubung (`20261020080000000_create-users.sql`)
  - spasi
  - awalan huruf (`v20261020080000000_x.sql`)
- `TestPlan_DuplicateVersionRejected`: `core/<V>_a.sql` dan `project/<V>_b.sql` -> `errors.Is(err, ErrDuplicateVersion)`, pesan memuat kedua path.
- `TestPlan_DuplicateVersionSameFolder`.
- `TestPlan_ErrorBeforeAnyExecution`: `Migrate` dengan `*DB` nil dan FS invalid mengembalikan `ErrInvalidMigrationName` tanpa panic (membuktikan D3: tidak menyentuh DB).
- `TestPlan_EmbeddedFSValid`: `plan(migrations.FS)` tidak error (menjaga repo nyata; dengan `.keep` saja -> kosong).
- `TestPlan_DeterministicOrder`: dua kali jalan hasil identik.

`TestOpen_ParseErrorDoesNotLeakPassword`: URL `postgres://u:s3cretpw@%zz/db` -> error tidak memuat `s3cretpw`.

### Integrasi — migrasi (T00-AC5)

Helper: `mapFS(files map[string]string) fstest.MapFS`, `appliedVersions(t, d) []int64`, `tableExists(t, d, name) bool` (`SELECT to_regclass($1) IS NOT NULL`).

- `TestT00_AC5_MigrateTwiceNoEffect`:
  - FS: file 1 `CREATE TABLE a(id int); INSERT INTO a VALUES (1);` (di `core/`), file 2 `CREATE TABLE b(id int);` (di `project/`).
  - Run 1: `Applied` = [`core/..1`, `project/..2`] (urut). Run 2: `Applied` kosong, error nil.
  - `SELECT count(*) FROM a` tetap 1, `schema_migrations` tetap 2 baris, `applied_at` baris tidak berubah (dibandingkan sebelum/sesudah run 2).
- `TestT00_AC5_FailedFileRollsBack`:
  - File 1 `CREATE TABLE a(id int);`. File 2 `CREATE TABLE b(id int); SELECT 1/0;`. File 3 `CREATE TABLE c(id int);`.
  - `Migrate` error; `errors` memuat path file 2 dan "division by zero". Tabel `b` TIDAK ada (rollback penuh meski `CREATE TABLE` berhasil sebelum error, membuktikan multi-statement dalam satu transaksi). File 3 tidak dijalankan (`c` tidak ada). `schema_migrations` hanya berisi versi file 1. `res.Applied` = [file 1].
  - Perbaiki FS (versi sama, isi file 2 diganti `CREATE TABLE b(id int);`), rerun: `Applied` = [file 2, file 3], `a`, `b`, `c` ada, 3 versi tercatat.
  - Lock tidak bocor: `SELECT count(*) FROM pg_locks WHERE locktype='advisory'` untuk key `hashtext('grimoire.migrate')` = 0 setelah error.
- `TestT00_AC5_InvalidAndDuplicateRejectedWithoutSideEffects`: FS berisi satu file valid dan satu nama invalid -> error, `schema_migrations` tidak ada dan tabel file valid tidak dibuat (validasi sebelum eksekusi). Sama untuk versi duplikat.
- `TestT00_AC5_ConcurrentRunners`:
  - FS 3 file, file pertama berisi `SELECT pg_sleep(0.3); CREATE TABLE a(id int);` agar overlap pasti terjadi.
  - Dua goroutine memanggil `Migrate` pada dua `*DB` yang menunjuk schema yang sama (atau satu `*DB` dengan pool >= 3) lewat `sync.WaitGroup`.
  - Keduanya error nil. `len(res1.Applied)+len(res2.Applied) == 3` (tiap file diterapkan tepat sekali, salah satu runner kosong). `schema_migrations` 3 baris tanpa duplikat. Tidak ada error `already exists`.
- `TestMigrate_WaitsWhileLockHeld`: sebuah `sql.Conn` lain menahan `pg_advisory_lock(hashtext('grimoire.migrate'))`; `Migrate` dengan ctx timeout 300ms mengembalikan error yang membungkus `context.DeadlineExceeded`, tidak ada tabel dibuat. Lepas lock, `Migrate` berhasil. Membuktikan lock benar-benar dipakai.
- `TestMigrate_SingleConnPool`: `DBMaxOpenConns=1` -> `Migrate` selesai tanpa deadlock (D2).
- `TestMigrate_CreatesSchemaMigrationsOnEmptyFS`: FS kosong -> tabel `schema_migrations` ada, `Applied` kosong.
- `TestMigrate_UnknownDBVersionWarns`: sisipkan versi di `schema_migrations` yang tidak ada di FS -> tidak error, log (buffer slog) memuat `migration version in database but not in binary`.
- `TestMigrate_SkipsAlreadyAppliedAndRunsNewOnly`: run 1 dengan 1 file, run 2 FS bertambah 1 file -> hanya file baru di `Applied`.
- `TestMigrate_ConnectionReturnedClean`: setelah `Migrate`, query lock di koneksi lain bisa mengambil `pg_try_advisory_lock(hashtext('grimoire.migrate'))` = true (lock dilepas).
- `TestUp_EmbeddedFS`: `Up(ctx, d, log)` dengan FS ter-embed (hanya `.keep`) -> sukses, `Applied` kosong.
- `TestMigrate_MultiStatementAndInsertAtomic`: file dengan `CREATE TABLE`, `CREATE INDEX`, `INSERT` dalam satu file sukses; versi tercatat di transaksi yang sama (verifikasi lewat kegagalan test: file yang gagal tidak meninggalkan versi).
- `TestMigrate_ContextCancelledMidRun`: ctx dibatalkan saat file ber-`pg_sleep(2)` berjalan -> error, tabel dari file itu tidak ada, versi tidak tercatat, lock dilepas (koneksi dibuang atau unlock jalan; pg_locks kosong).

### Integrasi — transaksi

- `TestBeginTx_Commit`: insert di dalam `fn` return nil -> baris terlihat dari koneksi lain.
- `TestBeginTx_RollbackOnError`: insert lalu `fn` return `errs.ErrConflict` (sentinel contoh) -> baris tidak ada, `errors.Is(err, sentinel)` true (tidak dibungkus).
- `TestBeginTx_RollbackAndRepanic`: `fn` insert lalu `panic("boom")`; test memakai `recover()` dan memeriksa nilainya `"boom"`, baris tidak ada, koneksi kembali ke pool (query berikutnya sukses, `db.Stats().InUse == 0`).
- `TestBeginTx_CommitError`: `fn` memanggil `tx.Commit()` sendiri lalu return nil -> `BeginTx` mengembalikan error yang membungkus `sql.ErrTxDone`.
- `TestBeginTx_RollbackErrorJoined`: `fn` return error setelah `tx.Rollback()` sendiri -> error asli tetap `errors.Is`, `sql.ErrTxDone` tidak ditambahkan.
- `TestBeginTx_ContextCancelled`: ctx batal sebelum begin -> error `context.Canceled`.
- `TestBeginTx_QuerierAcceptsTx`: fungsi contoh menerima `Querier` dipanggil dengan `d` dan dengan `tx` (compile-time dan runtime).

### Integrasi — koneksi

- `TestOpen_PoolSettings`: cfg `DBMaxOpenConns=7` -> `d.Stats().MaxOpenConnections == 7`.
- `TestOpen_UnreachableFailsFast`: URL ke port tertutup -> error dalam <= 6 detik, error memuat `db: ping`.
- `TestPing_OK` dan `TestPing_AfterClose` (error setelah `Close`).
- `TestTestDB_IsolatesSchemas`: dua `TestDB` tidak melihat tabel satu sama lain; setelah cleanup, schema terhapus.

---

## 7. E2E Scenarios

Usecase ini tidak punya UI atau endpoint sendiri. Skenario e2e dijalankan terhadap binary (`bin/grimoire`, dirakit `cli-and-wiring`) dan harus lulus setelah usecase 7 selesai; di PR ini diverifikasi lewat test integrasi di atas.

### Positif
1. **DB kosong, `migrate up`**: `bin/grimoire migrate up` -> exit 0; tabel `schema_migrations` ada (kosong); output "tidak ada migrasi baru" (karena belum ada file migrasi).
2. **Dua kali berturut-turut** (T00-AC5): perintah yang sama dijalankan lagi -> exit 0, tidak ada perubahan.
3. **Dengan file migrasi sungguhan** (setelah T01 menambah file): run pertama mencetak nama file yang diterapkan berurutan lintas `core/` dan `project/`; run kedua "tidak ada migrasi baru".
4. **`RUN_MIGRATIONS_ON_START=true`**: `bin/grimoire` start pada DB kosong -> migrasi berjalan sebelum server listen, `/readyz` 200.
5. **Dua instance start bersamaan** dengan `RUN_MIGRATIONS_ON_START=true`: keduanya naik tanpa error, migrasi diterapkan sekali (instance kedua menunggu lock).
6. **`/readyz`**: DB hidup -> `Ping` sukses -> 200 `{"status":"ok"}`.

### Negatif
1. **File invalid di build** (nama `20261020_x.sql`): `migrate up` exit 1, pesan memuat path dan pola yang benar, tidak ada perubahan DB.
2. **Versi duplikat** antara `core/` dan `project/`: exit 1, pesan memuat kedua path, DB tidak berubah.
3. **SQL file kedua error**: exit 1 menyebut file, file kedua tidak meninggalkan tabel setengah jadi, file pertama tetap tercatat; setelah file diperbaiki dan binary dibangun ulang, `migrate up` sukses.
4. **DB tidak terjangkau saat start**: `Open` gagal dalam <= 5 detik, exit 1, pesan tidak memuat password.
5. **DB dimatikan saat server jalan**: `Ping` error -> `/readyz` 503 `{"status":"unavailable"}`, `/healthz` tetap 200 (diverifikasi di `http-server`/`cli-and-wiring`).
6. **Ctrl-C saat migrasi berjalan**: ctx batal -> transaksi file berjalan di-rollback, lock dilepas, exit non-nol; run berikutnya melanjutkan dari file itu.

---

## 8. Request/Response body

Tidak ada endpoint HTTP di usecase ini. Berikut kontrak yang terlihat dari luar.

### API Go (kontrak untuk usecase lain)

```go
func Open(ctx context.Context, cfg config.Config) (*DB, error)
func (d *DB) Ping(ctx context.Context) error
func (d *DB) BeginTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error
func Migrate(ctx context.Context, d *DB, fsys fs.FS, log *slog.Logger) (MigrateResult, error)
func Up(ctx context.Context, d *DB, log *slog.Logger) (MigrateResult, error)
func TestDB(t testing.TB) *DB

type MigrateResult struct{ Applied []string }
```

### Pesan error runner (stderr CLI / log; bukan JSON)

```
db: invalid migration file name: core/2026102008_create_users.sql (harus ^\d{17}_[a-z0-9_]+\.sql$)
db: duplicate migration version: 20261020080000000 di core/20261020080000000_a.sql dan project/20261020080000000_b.sql
migration project/20261020080000002_add_b.sql: ERROR: division by zero (SQLSTATE 22012)
db: ping: failed to connect to `host=127.0.0.1 user=grimoire database=grimoire`: dial error (connection refused)
```

### Baris log (JSON, slog)

```json
{"time":"2026-10-20T08:00:00Z","level":"INFO","msg":"migration applied","version":20261020080000000,"file":"core/20261020080000000_create_users.sql","duration_ms":12.4}
{"time":"2026-10-20T08:00:01Z","level":"INFO","msg":"no new migrations"}
{"time":"2026-10-20T08:00:02Z","level":"WARN","msg":"migration version in database but not in binary","version":20261101000000000}
```

### Keluaran CLI (dirakit `cli-and-wiring`, berdasarkan `MigrateResult`)

```
$ grimoire migrate up
core/20261020080000000_create_users.sql
project/20261020080000001_create_projects.sql
2 migrasi diterapkan

$ grimoire migrate up
tidak ada migrasi baru
```

### Efek pada `/readyz` (diimplementasikan `http-server`, memakai `Ping`)

`200`:
```json
{ "status": "ok" }
```
`503` (detail error hanya di log dengan `trace_id`):
```json
{ "status": "unavailable" }
```

### Format error seragam API (untuk referensi; usecase ini tidak menghasilkannya)

Error DB di jalur request dibungkus repository/usecase tiket lain lalu dipetakan `httpx.WriteError`:
```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### Tabel `schema_migrations` (contoh isi setelah dua migrasi)

| version | name | applied_at |
|---|---|---|
| 20261020080000000 | `20261020080000000_create_users.sql` | 2026-10-20 08:00:00+00 |
| 20261020080000001 | `20261020080000001_create_projects.sql` | 2026-10-20 08:00:00+00 |

---

## 9. Definition of Done

- `go vet ./...` dan `golangci-lint run` bersih untuk package `db` dan `migrations`.
- `go test -race ./internal/infra/db/...` (unit) hijau tanpa DB.
- `go test -race -tags integration ./internal/infra/db/...` hijau dengan `TEST_DATABASE_URL`; semua test `TestT00_AC5_*` ada dan hijau.
- Tidak ada import `internal/domain/**` dari `internal/infra/db` dan `migrations`.
- `go build ./...` sukses dengan folder `migrations/{core,project}` hanya berisi `.keep` (membuktikan koreksi D6).
- `docs/configuration.md` memuat bagian Migrasi.
