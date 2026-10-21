# Integrasi Midtrans

Kami memakai Midtrans sebagai payment gateway untuk transfer bank dan e-wallet.

## Virtual account

Untuk transfer bank, sistem meminta Midtrans membuat virtual account per transaksi.

- Nomor VA berlaku 24 jam; setelah itu transaksi berstatus `expired`.
- Nominal harus sama persis dengan yang ditagihkan, kalau tidak pembayaran ditolak bank.
- Satu VA hanya untuk satu transaksi.

## Notifikasi pembayaran

Midtrans mengirim notifikasi HTTP ke endpoint `/internal/midtrans/notify` saat status berubah.

1. Verifikasi `signature_key` sesuai dokumentasi Midtrans.
2. Ambil status terbaru lewat API status, jangan percaya isi notifikasi saja.
3. Perbarui status payment dan publikasikan event `payment.succeeded` bila sukses.

## Konfigurasi

- `MIDTRANS_SERVER_KEY` dan `MIDTRANS_ENV` (`sandbox` atau `production`).
- Sandbox dipakai di staging dan CI.

## Troubleshooting

Bila pembayaran sudah dilakukan tetapi status belum berubah, cek log notifikasi lalu panggil API status secara manual.
