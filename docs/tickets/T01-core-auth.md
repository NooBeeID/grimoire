# T01 — core / auth

| | |
|---|---|
| Domain / Module | core / auth |
| Ukuran | L |
| Bergantung pada | T00 |
| Dibutuhkan oleh | T03 (lookup user), middleware sesi (T00) |

## Ringkasan

Akun user, login/logout, sesi, dan manajemen user oleh Super Admin. Hanya ada dua role user: `super_admin` dan `developer`. Agent (Agentic) **bukan** akun; ia diurus di T04.

## Tujuan

- Super Admin bisa membuat, mengubah, menonaktifkan user, dan mereset password mereka.
- User login dengan email + password, memakai cookie session.
- Sesi dicek ke status user di setiap request (user nonaktif langsung kehilangan akses).

## Non-goals

SSO/OAuth, email/SMTP, reset mandiri lewat email, 2FA, registrasi mandiri, role selain dua di atas.

## User story

- Sebagai Super Admin, saya membuat akun developer dengan password awal agar mereka bisa login.
- Sebagai Super Admin, saya menonaktifkan user yang pergi agar akses dan sesinya mati seketika.
- Sebagai user, saya wajib mengganti password awal saat login pertama.
- Sebagai Super Admin pertama, saya dibuat lewat CLI saat instalasi.

## Features (1 feature = 1 folder)

| Folder | Deskripsi | Endpoint / titik masuk | Siapa |
|---|---|---|---|
| `login` | Verifikasi email+password, buat sesi, set cookie | `POST /api/auth/login` | publik |
| `logout` | Hapus sesi, hapus cookie | `POST /api/auth/logout` | user |
| `getme` | Profil sesi saat ini (id, nama, email, role, `must_change_password`) | `GET /api/auth/me` | user |
| `changepassword` | Ganti password sendiri (butuh password lama); menghapus flag wajib ganti; mencabut sesi lain | `POST /api/auth/change-password` | user |
| `createuser` | Buat user dengan password awal, `must_change_password=true` | `POST /api/users` | super_admin |
| `listusers` | Daftar user + filter role/status/pencarian + paginasi | `GET /api/users` | super_admin |
| `getuser` | Detail user | `GET /api/users/{id}` | super_admin |
| `updateuser` | Ubah nama, email, role | `PATCH /api/users/{id}` | super_admin |
| `setuserstatus` | Nonaktifkan / aktifkan; menonaktifkan menghapus semua sesinya | `POST /api/users/{id}/deactivate`, `/activate` | super_admin |
| `resetpassword` | Admin menetapkan password sementara baru; flag wajib ganti aktif; sesi user dihapus | `POST /api/users/{id}/reset-password` | super_admin |
| `createadmin` | Perintah CLI membuat Super Admin (dipanggil dari `cmd/cli`) | `./grimoire create-admin` | operator |
| `resolvesession` | Handler bus: cookie → identitas | bus `core.auth.resolve-session` | internal |
| `getuserbus` | Handler bus: id → ringkasan user | bus `core.auth.get-user` | internal |
| `finddevelopers` | Handler bus: cari developer aktif berdasarkan email/nama | bus `core.auth.find-developers` | internal |
| `purgesessions` | Job: hapus sesi kedaluwarsa | job harian | sistem |

## Aturan bisnis

- Email unik tanpa membedakan huruf besar/kecil; disimpan lowercase.
- Password: hash `argon2id` (parameter dapat diatur). Panjang minimal 10 karakter; ditolak jika sama dengan email. Tidak ada aturan komposisi rumit.
- `must_change_password=true`: selama flag aktif, hanya `getme`, `changepassword`, dan `logout` yang boleh dipakai; endpoint lain mengembalikan 403 dengan kode `PASSWORD_CHANGE_REQUIRED`.
- Sesi: token acak 256-bit; yang disimpan di DB hanya hash-nya. Cookie `HttpOnly`, `Secure` (konfigurabel untuk dev), `SameSite=Lax`. Masa berlaku: idle 12 jam, absolut 7 hari (nilai default, konfigurabel).
- Perlindungan lockout: ≥5 login gagal untuk satu email dalam 15 menit → email itu ditunda (backoff) selama 15 menit; ditambah rate limit per IP. Respons gagal tidak membedakan "email tidak ada" dan "password salah" dan waktu responsnya disamakan.
- Super Admin tidak boleh: menonaktifkan dirinya sendiri, menurunkan role dirinya sendiri, atau menonaktifkan/menurunkan Super Admin aktif terakhir.
- Menonaktifkan user tidak menghapus datanya; keanggotaan project tetap tercatat tetapi tidak berlaku selama nonaktif (T03 memeriksa lewat `get-user`).
- Mengubah role dari `developer` ke `super_admin` atau sebaliknya diperbolehkan; sesi aktif memakai role terbaru karena dicek di setiap request.
- Password tidak pernah masuk log atau audit.

## Model data

```
users(id uuid pk, email citext unique, name text, password_hash text,
      role text check in ('super_admin','developer'),
      is_active bool, must_change_password bool,
      failed_login_count int, locked_until timestamptz null,
      created_at, created_by uuid null, updated_at, last_login_at null)
sessions(id uuid pk, user_id fk→users on delete cascade, token_hash bytea unique,
         created_at, last_seen_at, expires_at, ip inet, user_agent text)
```

Migrasi di `migrations/core/`.

## Kontrak bus

Disediakan (`lib/contracts/core/auth/...`):
- `core.auth.resolve-session` Call — `{Token}` → `{Found, UserID, Email, Name, Role, MustChangePassword}`.
- `core.auth.get-user` Call — `{UserID}` → `{Found, ID, Name, Email, Role, IsActive}`.
- `core.auth.find-developers` Call — `{Query, Limit}` → daftar `{ID, Name, Email}` developer aktif.

Dikonsumsi: `core.audit.record` (T02) — gunakan `Publish` untuk event di bawah, kecuali `auth.login.failed` yang boleh `Publish` juga.

Event audit yang dikirim: `auth.login.success`, `auth.login.failed` (dengan email yang dicoba, tanpa password), `auth.logout`, `auth.password.changed`, `auth.password.reset`, `user.created`, `user.updated` (field yang berubah), `user.deactivated`, `user.activated`.

## UI (Svelte, `domain/core/auth/...`)

- Halaman login (error generik, pesan terkunci/ditunda).
- Halaman wajib ganti password (muncul otomatis bila `must_change_password`).
- Profil: ganti password.
- Admin → Users: tabel (nama, email, role, status, login terakhir), filter/pencarian, dialog buat user (password awal ditampilkan sekali dengan tombol salin), edit, nonaktifkan/aktifkan, reset password (menampilkan password sementara sekali).
- Navigasi menyesuaikan role: menu Users hanya untuk Super Admin.

## Kriteria penerimaan

1. `./grimoire create-admin` membuat Super Admin; dijalankan lagi dengan email yang sama gagal dengan pesan jelas.
2. Login benar menghasilkan cookie `HttpOnly`; login salah (email tak ada vs password salah) mengembalikan respons dan waktu yang setara.
3. Setelah 5 kegagalan berturut-turut, login benar ditolak sampai jeda lewat.
4. User dengan `must_change_password` mengakses `/api/users` → 403 `PASSWORD_CHANGE_REQUIRED`; setelah ganti password, akses normal.
5. Menonaktifkan user membuat request berikutnya dengan cookie lamanya 401 seketika.
6. Super Admin tidak bisa menonaktifkan dirinya atau Super Admin aktif terakhir.
7. Developer yang memanggil `/api/users*` mendapat 403.
8. Password tidak muncul di log aplikasi maupun entri audit (tes yang memeriksa).
9. Setiap aksi pada daftar event audit menghasilkan satu entri.

## Pertanyaan terbuka

Tidak ada. Panjang minimal password dan durasi sesi adalah default saya; ubah di config bila perlu.
