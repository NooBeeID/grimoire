# Retry policy webhook

Bila endpoint pelanggan tidak membalas 2xx, kami mengirim ulang event.

## Jadwal

Retry memakai exponential backoff dengan jitter:

| Percobaan | Jeda kira-kira |
|---|---|
| 1 | langsung |
| 2 | 30 detik |
| 3 | 2 menit |
| 4 | 10 menit |
| 5 | 1 jam |
| 6-8 | 6 jam |

Maksimum 8 percobaan. Setelah itu event masuk dead letter dan statusnya `failed`.

## Dianggap gagal

- Status selain 2xx.
- Timeout lebih dari 10 detik.
- Error koneksi atau TLS.

## Dead letter

Event di dead letter bisa dikirim ulang manual dari dashboard. Endpoint yang gagal terus selama 3 hari dinonaktifkan otomatis dan pemiliknya diberi tahu lewat email.

## Saran untuk penerima

1. Balas 200 secepatnya, proses berat dikerjakan async.
2. Buat handler idempotent, karena event yang sama bisa datang lebih dari sekali.
3. Gunakan `event.id` untuk deduplikasi.
