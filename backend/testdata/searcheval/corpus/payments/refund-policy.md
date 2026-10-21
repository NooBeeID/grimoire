# Refund policy

Kebijakan refund untuk transaksi yang diproses lewat Payments Platform.

## Batas waktu

Refund hanya bisa diajukan dalam 14 hari sejak pembayaran berhasil. Setelah itu pelanggan perlu menghubungi tim finance untuk pengecualian.

## Jenis refund

- Full refund: mengembalikan seluruh nominal.
- Partial refund: nominal sebagian, tidak boleh melebihi sisa nominal yang belum di-refund.

## Approval

- Refund sampai Rp1.000.000 diproses otomatis.
- Refund di atas itu menunggu approval finance.
- Semua refund dicatat di audit log dengan alasan.

## Proses

1. Merchant atau support membuat refund lewat dashboard atau API.
2. Sistem memvalidasi batas waktu dan nominal.
3. Sistem mengirim instruksi ke payment gateway.
4. Status berubah menjadi `succeeded` setelah gateway mengonfirmasi, biasanya 1 sampai 5 hari kerja.

## Event

Setiap perubahan status memicu event `refund.created` dan `refund.succeeded` untuk webhook.
