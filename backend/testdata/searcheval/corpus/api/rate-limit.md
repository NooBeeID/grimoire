# Rate limit API

Setiap API key dibatasi 120 request per menit dengan burst sampai 30 request tambahan.

## Perilaku

- Limit dihitung per API key, bukan per IP.
- Request yang melewati limit menerima status 429.
- Header `Retry-After` berisi jumlah detik sampai request berikutnya boleh dikirim.

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 12
Content-Type: application/json

{"error": {"code": "RATE_LIMITED", "message": "terlalu banyak request"}}
```

## Saran untuk klien

1. Hormati `Retry-After`, jangan retry langsung.
2. Gunakan backoff dengan jitter bila 429 berulang.
3. Gabungkan request kecil menjadi satu bila endpoint mendukung batch.

## Menaikkan limit

Pelanggan yang butuh limit lebih tinggi mengajukan permintaan ke tim platform dengan alasan dan perkiraan traffic.
Limit khusus dikonfigurasi per API key, lihat dokumen API key untuk cara membuat key baru.

## Monitoring

Dashboard Grafana "API Limits" menampilkan jumlah 429 per key per menit. Alert menyala bila 429 lebih dari 5% traffic selama 10 menit.
