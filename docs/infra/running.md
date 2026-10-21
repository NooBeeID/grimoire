# Menjalankan Grimoire

Satu binary: `bin/grimoire`. Perakitan ada di `backend/cmd/internal/server`; `cmd/http` dan `cmd/cli` hanya memanggilnya. Konfigurasi lengkap: `docs/configuration.md`.

## Persiapan

```
cp .env.example .env   # isi DATABASE_URL dan SESSION_SECRET (minimal 32 byte)
```

Hanya `DATABASE_URL` dan `SESSION_SECRET` yang wajib untuk `serve` dan `migrate up`.

## Development

`make dev` menjalankan Postgres (docker compose, menunggu sehat), backend di `:8080` dengan migrasi otomatis, dan Vite di `:5173`. Ctrl-C menghentikan keduanya.

## Email undangan (Mailpit)

Undangan anggota dikirim lewat SMTP. Untuk dev lokal, `docker-compose.dev.yml` dan `docker-compose.yml` menjalankan Mailpit:

- SMTP di `localhost:1025`, UI web di `http://localhost:8025` (semua email yang dikirim aplikasi tampil di sana).
- `.env.example` sudah mengarah ke Mailpit (`MAIL_DRIVER=smtp`, `SMTP_HOST=localhost`, `SMTP_PORT=1025`, `SMTP_TLS=none`).
- Tanpa SMTP: kosongkan `MAIL_DRIVER`. Aplikasi tetap jalan; undangan membuat akun dan mengembalikan kredensial sementara satu kali di respons (`Cache-Control: no-store`), lalu pengundang menyampaikannya sendiri.
- `MAIL_DRIVER=log` mencetak email (termasuk password sementara) ke log; hanya untuk development dan ditolak saat `APP_ENV=production`.

## Build

`make build` membangun frontend, menyalinnya ke `backend/internal/infra/static/dist`, lalu mengompilasi `bin/grimoire` dengan versi, commit, dan tanggal build lewat ldflags. Placeholder `dist/index.html` dipulihkan setelahnya.

## Perintah

| Perintah | Efek |
|---|---|
| `grimoire` / `grimoire serve` | jalankan server |
| `grimoire migrate up` | jalankan migrasi lalu keluar; mencetak `diterapkan: <file>` atau `tidak ada migrasi baru` |
| `grimoire create-admin --email E --name N` | buat super admin pertama (belum tersedia sampai T01) |
| `grimoire version` | cetak versi; tidak membutuhkan env |
| `grimoire help`, `-h`, `--help` | cetak bantuan |

Password `create-admin` diambil berurutan dari: env `CREATE_ADMIN_PASSWORD`; satu baris stdin bila bukan TTY; prompt dua kali bila TTY. Password tidak pernah dicetak.

Exit code: `0` sukses, `1` kegagalan runtime (config tidak valid, DB mati, bind gagal, shutdown melewati batas), `2` kesalahan penggunaan (perintah/flag/argumen salah, password kosong atau tidak sama).

## Urutan start

1. Load config dan logger (config tidak valid: semua masalah dicetak sekaligus, exit 1, tanpa nilai secret).
2. Buka DB (ping gagal berarti berhenti).
3. Migrasi bila `RUN_MIGRATION=true`.
4. Bus dan jobs, router (`/api`, `/mcp`, `/healthz`, `/readyz`), `Register` domain.
5. Static SPA dipasang sebagai NotFound root; `/api/**` yang tidak cocok tetap 404 JSON.
6. Jobs start, listener dibuka, server melayani.

## Shutdown

Pada SIGINT/SIGTERM, berurutan:

1. HTTP drain (menunggu request berjalan, batas `SHUTDOWN_TIMEOUT`; lewat batas memutus koneksi dan exit 1).
2. Ctx server dibatalkan (goroutine module dan janitor berhenti).
3. Jobs berhenti.
4. Bus menunggu goroutine `Publish` (batas `BUS_SHUTDOWN_TIMEOUT`).
5. DB ditutup.

Sinyal kedua saat shutdown berjalan mematikan proses langsung.

## Test

- `make test`: unit test backend dan frontend.
- `make test-integration`: butuh Postgres (`TEST_DATABASE_URL`); termasuk smoke test binary.
- `make test-e2e`: Playwright terhadap `bin/grimoire`.
