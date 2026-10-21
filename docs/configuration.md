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
| `MAX_PROJECT_FILES` | int | `5000` | tidak | >= 1 | Jumlah file knowledge maksimum per project. |
| `MCP_MAX_RESPONSE_BYTES` | int | `102400` | tidak | >= 1024 | Batas ukuran respons MCP. |
| `RUN_MIGRATION` | bool | `false` | tidak | boolean | Jalankan migrasi saat start. |
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
| `SESSION_IDLE_TIMEOUT` | duration | `12h` | tidak | durasi > 0 dan <= SESSION_ABSOLUTE_TIMEOUT | Sesi habis bila tidak dipakai selama ini. |
| `SESSION_ABSOLUTE_TIMEOUT` | duration | `168h` | tidak | durasi > 0 | Umur maksimum sesi sejak login. |
| `PASSWORD_MIN_LENGTH` | int | `10` | tidak | 8..128 | Panjang minimum password (rune). |
| `ARGON2_MEMORY_KIB` | int | `65536` | tidak | 8192..4194304 | Memori argon2id (KiB). |
| `ARGON2_ITERATIONS` | int | `3` | tidak | 1..20 | Iterasi argon2id. |
| `ARGON2_PARALLELISM` | int | `2` | tidak | 1..16 | Paralelisme argon2id. |
| `LOGIN_MAX_FAILURES` | int | `5` | tidak | 1..100 | Gagal login per email sebelum akun ditunda. |
| `LOGIN_FAILURE_WINDOW` | duration | `15m` | tidak | durasi > 0 | Jendela penghitungan gagal login. |
| `LOGIN_LOCK_DURATION` | duration | `15m` | tidak | durasi > 0 | Lama penundaan login setelah batas gagal. |
| `LOGIN_RATE_PER_MINUTE` | int | `20` | tidak | 1..1000 | Batas login per menit per IP. |
| `API_KEY_MAX_ACTIVE_PER_PROJECT` | int | `20` | tidak | >= 1 | Batas API key aktif per project. |
| `PUBLIC_URL` | string | - | tidak (wajib bila `MAIL_DRIVER=smtp`) | URL absolut http(s) dengan host, tanpa query/fragment/userinfo; trailing slash dibuang | URL publik server; dipakai membentuk `mcp_url` (`<PUBLIC_URL>/mcp`) di respons buat key. |
| `MAIL_DRIVER` | enum | - | tidak | kosong, smtp, log; `log` ditolak saat `APP_ENV=production` | Driver email. Kosong = tanpa email (undangan mengembalikan kredensial di respons). `log` mencetak email ke log (hanya dev). |
| `SMTP_HOST` | string | - | bila `smtp` | tidak kosong bila `MAIL_DRIVER=smtp` | Host server SMTP. |
| `SMTP_PORT` | int | `587` | tidak | 1..65535 | Port SMTP (587 untuk STARTTLS, 465 untuk TLS implisit). |
| `SMTP_USERNAME` | string | - | tidak | - | Username AUTH PLAIN; kosong = tanpa autentikasi. |
| `SMTP_PASSWORD` | string | - | tidak | - | Password SMTP. Selalu disamarkan. |
| `SMTP_TLS` | enum | `starttls` | tidak | starttls, tls, none | Mode TLS. `none` hanya untuk dev (Mailpit); di produksi dicatat sebagai peringatan. |
| `MAIL_FROM` | string | - | bila `smtp` | alamat email valid; boleh `Nama <a@b.id>` | Pengirim email. |
| `MAIL_TIMEOUT` | duration | `10s` | tidak | durasi > 0 dan <= 60s | Batas waktu satu pengiriman email. |
| `INVITATION_TTL` | duration | `168h` | tidak | 1h..720h | Masa berlaku password sementara undangan. |
| `INVITATION_MAX_PER_HOUR` | int | `20` | tidak | 1..1000 | Batas undangan baru per pengundang per jam. |
| `INVITATION_RESEND_MAX_PER_HOUR` | int | `3` | tidak | 1..100 | Batas kirim ulang undangan per akun per jam. |
| `INVITATION_PURGE_AFTER` | duration | `720h` | tidak | 24h..8760h | Akun undangan kedaluwarsa tanpa keanggotaan dihapus setelah selisih ini. |
| `APISPEC_MAX_FILE_BYTES` | int64 | `5242880` | tidak | 1..MAX_UPLOAD_BYTES | Ukuran maksimum satu file API spec (OpenAPI). |
| `APISPEC_MAX_SPECS` | int | `100` | tidak | >= 1 | Jumlah API spec maksimum per project. |
| `MCP_RATE_PER_MINUTE` | int | `120` | tidak | 1..10000 | Batas panggilan MCP per menit per API key. |
| `MCP_AUTH_FAIL_PER_MINUTE` | int | `20` | tidak | 1..1000 | Batas autentikasi MCP yang gagal per menit per IP. |
| `MCP_MAX_REQUEST_BYTES` | int | `65536` | tidak | >= 1024 | Batas ukuran body request `POST /mcp` (byte). |
| `MCP_TOOL_TIMEOUT` | duration | `10s` | tidak | durasi > 0 dan <= 60s | Batas waktu eksekusi satu tool MCP. |
| `MCP_ALLOWED_ORIGINS` | list | - | tidak | dipisah koma; tiap entri `scheme://host[:port]` (http/https) | Origin browser yang diizinkan memanggil `/mcp`; klien tanpa header Origin selalu lolos. |

## Email undangan (T12)

- Email hanya dipakai untuk undangan. Tanpa `MAIL_DRIVER` aplikasi tetap jalan; undangan membuat akun dan mengembalikan kredensial satu kali di respons (`Cache-Control: no-store`).
- Contoh penyedia umum: STARTTLS `SMTP_PORT=587 SMTP_TLS=starttls`; TLS implisit `SMTP_PORT=465 SMTP_TLS=tls`.
- `PUBLIC_URL` wajib bila `MAIL_DRIVER=smtp`; URL login di email = `<PUBLIC_URL>/login`.
- `MAIL_DRIVER=log` ditolak di produksi karena mencetak password sementara ke log.

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
- Migrasi auth (T01) menjalankan `CREATE EXTENSION IF NOT EXISTS citext`: role DB aplikasi butuh hak membuat ekstensi, atau DBA menjalankannya lebih dulu.

## Pemeliharaan

Setiap env baru wajib ditambah di tabel ini dan di tabel overview plan.
