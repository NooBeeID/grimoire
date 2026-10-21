# Pagination

Semua endpoint list memakai cursor pagination. Offset tidak didukung karena tidak stabil saat data berubah.

## Parameter

- `limit`: jumlah item, default 20, maksimum 100.
- `cursor`: nilai `next_cursor` dari respons sebelumnya.

## Respons

```json
{
  "items": [{"id": "pay_1"}, {"id": "pay_2"}],
  "next_cursor": "eyJpZCI6InBheV8yIn0",
  "has_more": true
}
```

Bila `next_cursor` kosong, data sudah habis.

## Urutan stabil

Urutan default adalah `created_at` menurun dengan `id` sebagai tie-breaker. Dengan begitu item tidak terlewat atau muncul dua kali ketika ada insert baru di tengah paging.

## Catatan implementasi

- Cursor adalah base64 dari JSON kecil; klien tidak boleh mengurainya.
- Cursor berlaku 24 jam.
- Mengubah filter di tengah paging membuat cursor tidak valid dan mengembalikan `INVALID_ARGUMENT`.
