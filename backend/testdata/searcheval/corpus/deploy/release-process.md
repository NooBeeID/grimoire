# Release process

Kami merilis ke production setiap Selasa dan Kamis, kecuali ada code freeze.

## Persiapan

1. Pastikan semua PR untuk rilis sudah merge ke `main` dan CI hijau.
2. Tentukan versi semver: patch untuk perbaikan, minor untuk fitur, major untuk perubahan yang tidak kompatibel.
3. Perbarui `CHANGELOG.md` dengan ringkasan per perubahan.
4. Buat tag:

```bash
git tag -a v1.8.0 -m "release 1.8.0"
git push origin v1.8.0
```

## Canary

Pipeline men-deploy tag ke canary yang menerima 5% traffic. Pantau selama 30 menit:

- error rate tidak naik,
- latency p95 tidak naik lebih dari 10%,
- tidak ada alert baru.

## Approval

Rilis ke 100% traffic membutuhkan approval dari satu engineer lain dan satu orang on-call.
Catat hasil canary di thread rilis di Slack.

## Setelah rilis

Umumkan di channel #releases dengan link changelog. Bila ada masalah, ikuti dokumen rollback.
