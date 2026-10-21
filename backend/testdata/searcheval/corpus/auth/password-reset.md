# Password reset

Panduan alur forgot password untuk user dashboard dan catatan untuk tim support.

## Alur untuk user

1. User klik "Forgot password" di halaman login.
2. User mengisi alamat email akun.
3. Sistem selalu menjawab "Jika email terdaftar, kami kirim instruksi" agar tidak membocorkan akun.
4. Email berisi link reset dengan reset token sekali pakai.
5. User mengisi password baru dua kali.

## Reset token

- Reset token acak 32 byte, disimpan sebagai hash.
- Reset token kedaluwarsa 30 menit setelah dibuat.
- Meminta reset baru membatalkan token sebelumnya.
- Setelah password berhasil diganti, semua session aktif user dihapus.

## Untuk support

Support tidak pernah meminta atau mengetahui password user. Kalau user tidak menerima email:

- cek folder spam,
- pastikan email akun benar,
- minta ulang setelah 5 menit.

```sql
SELECT created_at, used_at FROM password_resets WHERE user_id = $1 ORDER BY created_at DESC LIMIT 3;
```
