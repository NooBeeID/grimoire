# Staging dengan Docker Compose

Staging berjalan di satu VM memakai docker compose. Tujuannya meniru production dalam skala kecil.

## Layanan

| Service | Fungsi |
|---|---|
| `app` | binary utama |
| `postgres` | database |
| `worker` | pemroses job |

## Menjalankan

```bash
cd /opt/payments
docker compose -f docker-compose.staging.yml pull
docker compose -f docker-compose.staging.yml up -d
```

## Healthcheck

Tiap service punya healthcheck. `app` dianggap sehat bila `/healthz` mengembalikan 200 dalam 3 detik.
Gunakan `docker compose ps` untuk melihat status.

## Seed data

Setelah database baru dibuat, jalankan seed supaya QA punya akun dan project contoh:

```bash
docker compose exec app ./payments seed --env staging
```

Data staging bukan data production. Jangan menyalin data pelanggan ke staging.

## Pembersihan

Volume database dibersihkan tiap Minggu malam oleh cron, lalu seed dijalankan ulang.
