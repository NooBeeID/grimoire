# Login flow

Dokumen ini menjelaskan alur login dashboard Payments Platform. Kami memakai OAuth 2.0 dengan authorization code, bukan implicit flow.

## Langkah login

1. User membuka halaman login dan memilih penyedia identitas.
2. Backend mengarahkan browser ke endpoint authorization dengan `state` acak.
3. Setelah user setuju, penyedia mengembalikan `code` ke `/auth/callback`.
4. Backend menukar code dengan access token dan refresh token.
5. Backend membuat session cookie `HttpOnly` dan `SameSite=Lax`.

## Session cookie

- Masa berlaku session 12 jam, diperpanjang setiap ada aktivitas.
- Cookie hanya dikirim lewat HTTPS di production.
- Isi cookie hanya session id; data session disimpan di database.

## Refresh token

Refresh token disimpan terenkripsi dan dipakai untuk meminta access token baru saat expired.
Kalau refresh gagal dua kali berturut-turut, session dihapus dan user diminta login ulang.

```go
tok, err := oauth.Exchange(ctx, code)
if err != nil {
    return ErrLoginFailed
}
```

## Logout

Logout menghapus session di database lalu menghapus cookie. Refresh token ikut di-revoke ke penyedia.
Jangan lupa menguji logout dari dua tab sekaligus.
