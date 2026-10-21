# Invoice generation

Invoice dibuat otomatis setelah payment berhasil dan dikirim ke pelanggan.

## Isi invoice

- Nomor urut dengan format `INV-2026-000123`, tidak boleh ada lompatan.
- Data merchant dan pelanggan.
- Rincian item, subtotal, pajak PPN 11%, dan total.
- Tanggal terbit dan metode pembayaran.

## Alur

1. Event `payment.succeeded` diterima oleh worker.
2. Worker mengambil nomor urut berikutnya dari tabel sequence dalam satu transaksi.
3. Worker merender template menjadi PDF.
4. PDF disimpan di object storage dan link-nya dicatat di database.
5. Email dikirim ke alamat pelanggan dengan PDF terlampir.

## Pembulatan

Pajak dihitung per baris lalu dijumlahkan, dibulatkan ke rupiah terdekat. Jangan menghitung pajak dari total.

## Membuat ulang

Invoice yang sudah terbit tidak diubah. Koreksi dilakukan dengan credit note yang punya nomor sendiri.

```bash
./payments invoice resend --id inv_123
```
