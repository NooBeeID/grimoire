# Idempotency keys

Idempotency key mencegah duplicate charge ketika klien mengulang request karena timeout.

## Cara pakai

Kirim header `Idempotency-Key` berisi UUID unik pada setiap request pembuatan payment:

```http
POST /v1/payments
Idempotency-Key: 6f1c2a0e-8d4b-4c1f-9a77-2b5e0f3c9d11
Content-Type: application/json

{"amount": 150000, "currency": "IDR"}
```

## Perilaku server

- Request pertama diproses dan responsnya disimpan.
- Request ulang dengan key dan body yang sama mengembalikan respons tersimpan, tanpa memproses ulang.
- Key yang sama dengan body berbeda mengembalikan 409 `CONFLICT`.
- Key disimpan dengan TTL 24 jam.

## Praktik terbaik

1. Buat key sebelum request pertama dan simpan bersama data order.
2. Pakai ulang key yang sama saat retry, jangan buat baru.
3. Jangan memakai key yang bisa ditebak, misalnya nomor order saja.

## Implementasi

Tabel `idempotency_records` punya unique index `(api_key_id, key)`. Insert dilakukan sebelum pemrosesan dan dikunci dengan `SELECT ... FOR UPDATE`.
