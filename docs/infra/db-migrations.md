# Migrasi Database

Paket: `backend/internal/infra/db` (runner) dan `backend/migrations` (file SQL ter-embed).

## Lokasi dan format nama

- File berada di `backend/migrations/core/` atau `backend/migrations/project/`.
- Nama file: `YYYYMMDDHHMMSSmmm_<snake_case>.sql`, divalidasi `^\d{17}_[a-z0-9_]+\.sql$`.
- Versi (17 digit pertama) harus unik secara global, lintas `core` dan `project`. Duplikat ditolak.
- Timestamp di-generate saat implement dengan milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`.
- File non-`.sql` (mis. `.keep`, README) diabaikan.
- Embed memakai `//go:embed all:core all:project` agar `.keep` ikut dan folder kosong tidak menggagalkan build.

## Perilaku runner

- Urutan eksekusi mengikuti versi naik secara global, bukan per folder.
- Seluruh FS divalidasi (nama, duplikat, urutan) sebelum menyentuh database.
- Tabel `schema_migrations(version, name, applied_at)` dibuat otomatis setelah lock didapat.
- Satu file = satu transaksi, bersama insert versi ke `schema_migrations`. Gagal berarti rollback penuh dan file berikutnya tidak dijalankan.
- `pg_advisory_lock(hashtext('grimoire.migrate'))` dipegang selama proses sehingga banyak instance aman. Instance lain menunggu (batas waktu mengikuti ctx pemanggil).
- Lock, transaksi, dan insert versi memakai satu koneksi khusus, jadi `DB_MAX_OPEN_CONNS=1` tidak deadlock.
- Forward-only: tidak ada down dan tidak ada checksum. Versi di database yang tidak ada di binary hanya menghasilkan warning.

## Aturan menulis migrasi

- Jangan menulis `BEGIN`/`COMMIT` sendiri; runner sudah membungkus transaksi.
- Jangan memakai `CREATE INDEX CONCURRENTLY`, `VACUUM`, atau statement lain yang menolak transaksi.
- Jangan mengedit migrasi yang sudah dirilis; buat file baru.
- Satu file boleh berisi banyak statement.

## Penggunaan

- CLI: `grimoire migrate up` atau `make migrate` (lihat `scripts/make/db.mk` untuk target tambahan).
- Otomatis saat start: `RUN_MIGRATIONS_ON_START=true`.
- Kode: `db.Up(ctx, d, log)` untuk FS ter-embed, `db.Migrate(ctx, d, fsys, log)` untuk FS lain (test).

## Transaksi dan test

- `d.BeginTx(ctx, func(tx *sqlx.Tx) error)`: commit bila nil, rollback bila error (error dikembalikan apa adanya), rollback lalu panic diteruskan. Tidak ada savepoint.
- `db.Querier` dipenuhi `*sqlx.DB`, `*sqlx.Tx`, dan `*db.DB`.
- `db.TestDB(t)` membuat schema unik per test dan menghapusnya saat cleanup. `TEST_DATABASE_URL` kosong: skip lokal, gagal bila `CI=true`.

## Menjalankan test

```
cd backend
go test -race ./internal/infra/db/...
TEST_DATABASE_URL=postgres://... go test -race -tags integration ./internal/infra/db/...
```
