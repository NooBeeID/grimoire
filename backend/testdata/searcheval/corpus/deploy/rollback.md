# Rollback

Dokumen ini menjelaskan kapan dan bagaimana melakukan rollback layanan di production.

## Kapan boleh rollback

Lakukan rollback bila setelah rilis:

- error rate naik lebih dari 2x baseline selama 10 menit,
- fitur kritis seperti pembuatan payment gagal,
- ada kerusakan data yang bisa dihentikan dengan versi sebelumnya.

Jangan menunggu investigasi selesai. Rollback dulu, cari penyebab kemudian.

## Langkah

1. Umumkan di #incidents bahwa rollback dimulai.
2. Jalankan pipeline `deploy-production` dengan parameter `version` berisi versi sebelumnya.
3. Atau revert commit di `main` lalu rilis patch:

```bash
git revert --no-edit <sha>
git push origin main
```

## Migrasi database

Rollback aplikasi tidak membatalkan migrasi. Karena kami memakai pola expand-contract, versi sebelumnya tetap kompatibel dengan skema baru. Jangan jalankan migrasi turun di production tanpa persetujuan lead.

## Verifikasi pasca-rollback

- Error rate kembali ke baseline.
- Smoke test checkout lulus.
- Versi yang berjalan sesuai di halaman `/healthz`.

Tulis catatan singkat di thread rilis tentang alasan rollback.
