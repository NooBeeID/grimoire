# Konfigurasi

Konfigurasi hanya lewat environment. Saat start, semua error validasi dilaporkan sekaligus dan proses keluar dengan kode 1. Secret (`DATABASE_URL` password, `SESSION_SECRET`) disamarkan di log dan `String()`.

| Env | Tipe | Default | Wajib | Validasi | Keterangan |
|---|---|---|---|---|---|
| `DATABASE_URL` | string | - | ya | tidak kosong, URL postgres:// atau postgresql:// | Koneksi Postgres. Password disamarkan di log. |
| `HTTP_ADDR` | string | `:8080` | tidak | host:port valid | Alamat listen HTTP. |
| `APP_ENV` | enum | `production` | tidak | production|development | Mode aplikasi. |
| `SESSION_SECRET` | string | - | ya | >= 32 byte | Rahasia sesi. Selalu disamarkan. |
| `COOKIE_SECURE` | bool | `true` | tidak | boolean | Flag Secure pada cookie. |
| `AUDIT_RETENTION_DAYS` | int | `90` | tidak | >= 1 | Retensi audit log (hari). |
| `MAX_FILE_BYTES` | int64 | `1048576` | tidak | >= 1 | Ukuran maksimum satu file. |
| `MAX_UPLOAD_BYTES` | int64 | `52428800` | tidak | >= MAX_FILE_BYTES | Ukuran total satu upload. |
| `MAX_UPLOAD_FILES` | int | `500` | tidak | >= 1 | Jumlah file maksimum per upload. |
| `MCP_MAX_RESPONSE_BYTES` | int | `102400` | tidak | >= 1024 | Batas ukuran respons MCP. |
| `RUN_MIGRATIONS_ON_START` | bool | `false` | tidak | boolean | Jalankan migrasi saat start. |
| `DB_MAX_OPEN_CONNS` | int | `20` | tidak | >= 1 | Maksimum koneksi terbuka. |
| `DB_MAX_IDLE_CONNS` | int | `5` | tidak | >= 0 dan <= open | Maksimum koneksi idle. |
| `DB_CONN_MAX_LIFETIME` | duration | `30m` | tidak | durasi > 0 | Umur maksimum koneksi. |
| `HTTP_DEFAULT_BODY_BYTES` | int64 | `1048576` | tidak | >= 1 | Batas body default. |
| `HTTP_READ_HEADER_TIMEOUT` | duration | `10s` | tidak | durasi > 0 | ReadHeaderTimeout server. |
| `HTTP_DEFAULT_TIMEOUT` | duration | `30s` | tidak | durasi > 0 | Timeout request default. |
| `SHUTDOWN_TIMEOUT` | duration | `15s` | tidak | durasi > 0 | Timeout shutdown HTTP. |
| `BUS_SHUTDOWN_TIMEOUT` | duration | `5s` | tidak | durasi > 0 | Timeout shutdown bus. |
| `LOG_LEVEL` | enum | `info` | tidak | debug|info|warn|error | Level log. |
| `LOG_FORMAT` | enum | `json` | tidak | json|text | Format log. |
| `DEV_VITE_URL` | string | - | tidak | URL http(s); hanya saat APP_ENV=development | Proxy ke Vite dev server. |

## Env di luar Config

- `CREATE_ADMIN_PASSWORD`: password untuk CLI `create-admin` (non-interaktif).
- `TEST_DATABASE_URL`: Postgres untuk test integrasi (wajib di CI).
- `CI`: bila `true`, test integrasi gagal jika `TEST_DATABASE_URL` kosong.

## Audit log

- `AUDIT_RETENTION_DAYS` (default 90, minimal 1): entri `audit_log` yang lebih tua dihapus job `core.auditlog.purge-expired`.
- Job purge berjalan harian pukul 03:15 UTC, menghapus per batch 5000 baris.
- Tabel `audit_log` append-only: trigger database menolak `UPDATE`. `DELETE` tetap diizinkan karena dipakai job purge.
- Saran deploy: cabut hak ubah dari role aplikasi dengan `REVOKE UPDATE, TRUNCATE ON audit_log FROM <app_role>;` (jangan cabut `DELETE`).

## Contoh

Pengembangan: salin `.env.example` ke `.env`.

Produksi:

```
DATABASE_URL=postgres://user:pass@db:5432/grimoire?sslmode=require
SESSION_SECRET=<string acak >= 32 byte>
```

## Catatan

- `make dev` diisi oleh usecase cli-and-wiring.
- Migrasi: satu transaksi per file; `CREATE INDEX CONCURRENTLY` tidak didukung. Detail dilengkapi usecase db-migrations.

## Pemeliharaan

Setiap env baru wajib ditambah di tabel ini dan di tabel overview plan.
