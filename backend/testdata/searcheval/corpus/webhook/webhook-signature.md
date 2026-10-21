# Webhook signature

Setiap webhook yang kami kirim ditandatangani agar penerima bisa memastikan pengirimnya benar.

## Header

- `X-Signature`: HMAC SHA-256 dalam hex.
- `X-Timestamp`: waktu kirim dalam detik Unix.

## Cara menghitung

Pesan yang ditandatangani adalah `timestamp + "." + body` mentah, memakai secret endpoint:

```go
mac := hmac.New(sha256.New, []byte(secret))
mac.Write([]byte(ts + "." + string(body)))
expected := hex.EncodeToString(mac.Sum(nil))
```

Bandingkan dengan `hmac.Equal`, bukan `==`, supaya tahan timing attack.

## Timestamp tolerance

Tolak webhook bila selisih timestamp dengan waktu sekarang lebih dari 5 menit. Ini mencegah replay dari request yang disadap.

## Rotasi secret

Saat secret dirotasi, kami menandatangani dengan dua secret selama 24 jam; header `X-Signature` berisi dua nilai dipisah koma.

## Tips

- Hitung signature dari body mentah sebelum JSON di-parse ulang.
- Simpan secret di secret manager.
