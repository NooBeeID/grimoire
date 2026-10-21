# API keys

API key dipakai oleh integrasi server-to-server, misalnya service billing milik pelanggan atau agent internal.

## Aturan dasar

- Satu key per integrasi. Jangan berbagi key antar sistem.
- Prefix key selalu `gh_` diikuti 40 karakter acak.
- Key hanya ditampilkan sekali saat dibuat. Setelah itu yang tersimpan hanya hash.

## Membuat key

```bash
curl -X POST /api/projects/$PROJECT/api-keys \
  -H 'Content-Type: application/json' \
  -d '{"name": "billing-sync"}'
```

Respons berisi `raw_key`. Simpan di secret manager, bukan di repo.

## Rotasi

Rotasi dilakukan tiap 90 hari atau segera bila ada kecurigaan bocor:

1. Buat key baru dengan nama yang sama plus sufiks tanggal.
2. Deploy konfigurasi integrasi dengan key baru.
3. Revoke key lama setelah traffic key lama nol.

## Revoke

Revoke bersifat langsung. Request dengan key yang sudah di-revoke menerima 401.
Setiap key punya batas rate limit sendiri, lihat dokumen API terkait.
