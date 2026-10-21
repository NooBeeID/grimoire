# Environment variables

Konfigurasi aplikasi dibaca dari environment variables. Contoh lengkap ada di `.env.example`.

## Variabel utama

| Nama | Keterangan |
|---|---|
| `DATABASE_URL` | connection string Postgres |
| `SESSION_SECRET` | kunci penandatangan session, minimal 32 karakter |
| `PORT` | port HTTP, default 8080 |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` |
| `MIDTRANS_SERVER_KEY` | kunci server payment gateway |

## Aturan

- Jangan commit file `.env`; hanya `.env.example` tanpa nilai rahasia.
- Nilai rahasia di production disimpan di secret manager dan diinjeksi saat deploy.
- Aplikasi gagal start dengan pesan jelas bila variabel wajib kosong.

## Menambah variabel baru

1. Tambahkan ke struct config beserta validasi.
2. Tambahkan ke `.env.example` dengan komentar.
3. Perbarui dokumen ini.
4. Minta tim ops menambahkan nilainya di secret manager sebelum rilis.

```bash
export DATABASE_URL=postgres://payments:payments@localhost:5432/payments?sslmode=disable
```
