# Katalog event webhook

Daftar event yang bisa dilanggan oleh endpoint pelanggan.

## Payment

- `payment.created`: payment dibuat.
- `payment.succeeded`: pembayaran berhasil.
- `payment.failed`: pembayaran gagal atau kedaluwarsa.

## Refund

- `refund.created`: refund diajukan.
- `refund.succeeded`: dana sudah dikembalikan oleh gateway.

## Invoice

- `invoice.issued`: invoice terbit.

## Contoh payload

```json
{
  "id": "evt_01HZX",
  "type": "payment.succeeded",
  "created_at": "2026-10-21T08:30:00Z",
  "data": {
    "payment_id": "pay_123",
    "amount": 150000,
    "currency": "IDR"
  }
}
```

## Catatan

- Field baru boleh ditambahkan kapan saja; penerima harus mengabaikan field yang tidak dikenal.
- Urutan pengiriman tidak dijamin. Gunakan `created_at` bila urutan penting.
- Pelanggan lama kadang menyebut ini sebagai callback, tetapi istilah resmi kami adalah webhook.
