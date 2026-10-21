# Setup lokal

Panduan menyiapkan mesin development untuk Payments Platform.

## Prasyarat

- Go 1.25
- Node 22
- Docker dan docker compose

## Langkah

1. Clone repo dan salin konfigurasi:

```bash
git clone git@example.com:payments/platform.git
cd platform
cp .env.example .env
```

2. Jalankan Postgres lewat docker compose:

```bash
docker compose up -d postgres
```

3. Jalankan aplikasi dengan `make dev`. Migrasi berjalan otomatis.
4. Isi data contoh dengan `make seed`.
5. Buka `http://localhost:8080` dan login dengan akun seed.

## Menjalankan test

- `make test` untuk unit test.
- `make test-integration` untuk test yang butuh Postgres.

## Masalah umum

- Port 5432 bentrok: hentikan Postgres lokal atau ubah port di compose.
- Migrasi gagal: hapus volume dengan `docker compose down -v` lalu ulangi.
