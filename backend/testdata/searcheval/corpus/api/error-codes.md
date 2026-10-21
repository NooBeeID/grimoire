# Error codes

Semua error API memakai format JSON yang sama supaya klien mudah menanganinya.

## Format

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "amount harus lebih besar dari 0",
    "request_id": "req_8f2c"
  }
}
```

Selalu sertakan `request_id` saat melapor ke support.

## Daftar kode

| Code | HTTP | Arti |
|---|---|---|
| `INVALID_ARGUMENT` | 400 | input tidak valid |
| `UNAUTHENTICATED` | 401 | key tidak ada atau salah |
| `PERMISSION_DENIED` | 403 | tidak punya akses |
| `NOT_FOUND` | 404 | resource tidak ditemukan |
| `CONFLICT` | 409 | bentrok, misalnya idempotency key dipakai dengan body berbeda |
| `RATE_LIMITED` | 429 | melewati limit |
| `INTERNAL` | 500 | kesalahan server |

## Aturan penulisan pesan

- Pesan ditulis untuk developer, singkat, dan menyebut field yang bermasalah.
- Jangan memasukkan data sensitif ke pesan.
- Kode error tidak pernah dihapus; kode baru ditambah di akhir.
