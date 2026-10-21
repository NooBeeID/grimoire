# Database migration

Migrasi skema dijalankan otomatis saat deploy, tetapi harus ditulis dengan hati-hati.

## Aturan penamaan

File migrasi diberi prefix timestamp, contoh `20261021090000_add_refund_reason.sql`. Urutan eksekusi mengikuti timestamp, jadi jangan menyisipkan migrasi dengan timestamp lebih lama dari yang sudah berjalan.

## Pola expand-contract

Perubahan yang merusak dipecah menjadi beberapa rilis:

1. Expand: tambah kolom atau tabel baru, kode lama tetap jalan.
2. Migrate: isi data baru, kode membaca dari yang baru.
3. Contract: hapus kolom lama di rilis berikutnya.

## Lock

- Hindari `ALTER TABLE` yang menahan lock lama pada tabel besar.
- Buat index dengan `CREATE INDEX CONCURRENTLY`.
- Atur `lock_timeout` 5 detik agar migrasi gagal cepat daripada memblokir traffic.

## Sebelum production

- Uji di staging dengan salinan skema terbaru.
- Pastikan ada backup yang berumur kurang dari 24 jam.
- Tulis perkiraan durasi di deskripsi PR.

```sql
SET lock_timeout = '5s';
ALTER TABLE refunds ADD COLUMN reason text;
```
