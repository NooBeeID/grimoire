# T12 — infra / mailer + core / auth + project / management (undangan lewat email)

| | |
|---|---|
| Domain / Module | infra / mailer (baru), core / auth (undangan), project / management (invite anggota) |
| Ukuran | L |
| Bergantung pada | T01, T02, T03 |
| Sumber | permintaan pemilik (belum ada spec desain) |

## Ringkasan

Hari ini owner hanya bisa menambah anggota yang **sudah punya akun** (T03 `addmember`), dan akun baru hanya dibuat Super Admin lewat `createuser` (T01) dengan password yang ia ketik sendiri. Tiket ini menambah jalur undangan: bila developer tidak ditemukan, owner (atau Super Admin) mengundang lewat email. Sistem membuat akun developer dengan **password sementara**, mengirimkannya lewat email, dan penerima login lalu langsung diminta mengganti password (mekanisme `must_change_password` T01).

Tiket ini juga memperkenalkan pengiriman email (SMTP). Itu membalik non-goal "email/SMTP" di T01 dan "undangan lewat email" di T03 (lihat "Catatan revisi").

## Tujuan

1. Owner dan Super Admin bisa mengundang orang lewat email dari dialog tambah anggota project, bila pencarian developer tidak menemukan orangnya.
2. Super Admin bisa mengundang developer dari halaman Users tanpa terikat project.
3. Penerima menerima email berisi alamat login dan password sementara, login, lalu wajib mengganti password. Tanpa campur tangan Super Admin.
4. Owner bisa melihat undangan yang belum diterima, mengirim ulang, dan membatalkannya.
5. Bila email tidak bisa dikirim (SMTP belum diatur atau gagal), pengundang menerima kredensial sementara di layar satu kali untuk disampaikan manual.
6. Infrastruktur kirim email bisa dipakai module lain nanti, tanpa module itu tahu soal SMTP.

## Non-goals

- Reset password mandiri lewat email ("lupa password"). Infra email-nya terpakai, fiturnya bukan bagian tiket ini.
- Verifikasi email untuk akun yang dibuat Super Admin lewat `createuser`.
- Undangan sebagai role `super_admin`. Undangan hanya menghasilkan `developer`.
- Template email multi-bahasa atau bergaya marketing, pelacakan buka/klik, antrean email persisten (outbox), retry di latar belakang.
- Undangan massal dan tautan undangan publik tanpa email.
- Provider berbasis API (SES, SendGrid, dst). Hanya SMTP.
- Halaman penerimaan undangan khusus. Penerima memakai halaman login dan alur ganti password T01.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Password sementara tersimpan polos di DB, ada di log, audit, atau muncul di respons API selain pada kasus fallback yang ditentukan di bawah.
- Password sementara bisa dipakai tanpa batas waktu.
- Endpoint undangan membocorkan apakah sebuah email sudah terdaftar kepada pihak yang tidak berwenang.
- Kegagalan SMTP membuat request gagal setengah jalan (akun terbuat tanpa jejak, atau anggota tercatat tanpa bisa dikirim ulang).

## User story

- Sebagai owner, saya mengetik email rekan yang belum punya akun lalu menekan "Undang lewat email" agar rekan itu bisa bergabung tanpa menunggu Super Admin.
- Sebagai penerima, saya membuka email, login dengan password sementara, lalu memilih password saya sendiri.
- Sebagai owner, saya melihat siapa yang belum menerima undangan, bisa kirim ulang bila email tidak sampai, atau batalkan bila salah alamat.
- Sebagai Super Admin, saya mengundang developer baru dari halaman Users tanpa harus membuat password untuk mereka.
- Sebagai pengundang, bila email gagal terkirim, saya masih bisa menyalin kredensial dan mengirimnya lewat kanal lain.
- Sebagai operator, saya mengatur SMTP lewat env dan bisa mengembangkan lokal tanpa SMTP sungguhan.

## Alur

```
Dialog "Tambah anggota" (project)                 Halaman Users (super_admin)
  └─ cari developer                                 └─ "Undang developer"
       ├─ ketemu → addmember (T03, tidak berubah)        POST /api/users/invite {email, name?}
       └─ tidak ketemu + input berbentuk email
            └─ POST /api/projects/{projectID}/invitations {email, role}

Keduanya:
  1. cek akses, cek email belum jadi akun yang sudah menerima undangan
  2. bus core.auth.invite-user → buat akun developer (must_change_password=true)
     dengan password sementara acak + catat undangan (kedaluwarsa)
  3. (alur project) tambah project_members
  4. commit → kirim email di luar transaksi
  5. audit
  6. respons: emailSent=true | emailSent=false + kredensial sementara (sekali) bila tidak terkirim

Penerima: buka {APP_BASE_URL}/login → login email + password sementara
  → T01: must_change_password → paksa ganti password
  → ganti password berhasil = undangan diterima
```

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Bentuk kredensial awal | **Password sementara** yang dikirim lewat email; penerima login lalu wajib ganti. |
| Tanpa SMTP / email gagal | **Fallback**: pengundang diberi kredensial sementara (email, password sementara, URL login) satu kali untuk disalin dan dikirim manual. |
| Undangan dari halaman Users | **Ya**, Super Admin bisa mengundang developer tanpa project. |

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

- K1. Password sementara 16+ karakter acak (crypto/rand, alfabet tanpa karakter yang mudah tertukar), di-hash `argon2id` seperti password biasa. Akun dibuat `is_active=true`, `must_change_password=true`.
- K2. **Tidak ada status `pending` yang memblokir login.** Akun undangan bisa login segera dengan password sementara, sehingga keanggotaan project langsung berlaku. "Menunggu" berarti: ada baris undangan dengan `accepted_at IS NULL`.
- K3. Password sementara **kedaluwarsa dalam 7 hari** (konfigurabel). Selama undangan belum diterima dan lewat `expires_at`, `login` menolak akun itu dengan respons yang sama seperti kredensial salah. Kirim ulang menerbitkan password baru dan memperpanjang masa berlaku.
- K4. Undangan dianggap **diterima** saat `changepassword` (T01) berhasil pada akun itu. `changepassword` menandai `accepted_at` dalam transaksi yang sama dengan penggantian password.
- K5. Mengundang email yang sudah punya akun dan sudah menerima undangan (atau dibuat lewat `createuser`) ditolak dengan `ALREADY_REGISTERED`. Pesan ini hanya diperlihatkan kepada owner/admin yang berwenang (mereka memang boleh mencari developer lewat T03). UI mengarahkan memakai pencarian biasa.
- K6. Mengundang email yang akunnya masih menunggu (belum diterima): tidak membuat akun baru. Menambah keanggotaan project ini (bila lewat project), menerbitkan password sementara baru, dan mengirim ulang email.
- K7. Email dikirim **setelah commit**, satu percobaan dengan timeout. Gagal kirim tidak membatalkan undangan: respons `201` dengan `emailSent: false` dan `credentials` (lihat K8). Error SMTP di-log dan diaudit tanpa memuat password.
- K8. **Kredensial di respons hanya pada fallback.** Field `credentials {email, temporaryPassword, loginUrl}` muncul di respons invite/resend **hanya** bila `emailSent=false`, dan tidak pernah bisa dibaca lagi (tidak ada endpoint baca). UI menampilkannya di dialog sekali tampil dengan tombol salin dan peringatan. Header `Cache-Control: no-store` pada respons itu.
- K9. Bila `MAIL_DRIVER` tidak dikonfigurasi (tidak ada SMTP dan bukan `log`), mailer adalah no-op yang mengembalikan "tidak terkonfigurasi"; undangan tetap jalan dan selalu berakhir di jalur fallback K8. Aplikasi tetap bisa start tanpa SMTP. Untuk UI, `getme`/endpoint config yang sudah ada menyertakan `emailEnabled` agar dialog menjelaskan "email belum diatur, kredensial akan ditampilkan untuk disalin".
- K10. `infra/mailer` dengan antarmuka kecil `Send(ctx, Message) error` dan tiga driver: `smtp`, `log` (mencetak email ke log, hanya untuk dev/tes), dan tidak terkonfigurasi (K9). Module mengaksesnya lewat `app.Deps`, tidak meng-import SMTP langsung. Driver `log` mencetak email apa adanya, jadi tidak boleh aktif di produksi; konfigurasi menolak `log` bila mode produksi aktif.
- K11. Teks email bahasa Indonesia, plain text + HTML sederhana. Memuat nama pengundang, nama project (bila ada), alamat login, email, password sementara, tanggal kedaluwarsa, dan peringatan agar password segera diganti. `APP_BASE_URL` wajib bila driver `smtp`; validasi config gagal saat start bila kosong.
- K12. Rate limit: maksimal 20 undangan/jam per pengundang dan 3 kirim ulang/jam per undangan (default, konfigurabel). Login memakai throttle yang sudah ada.
- K13. Membatalkan undangan dari project: hanya bila belum diterima. Menghapus keanggotaan di project itu. Akunnya tidak dihapus seketika; job `purgeinvitations` menghapus akun yang belum diterima, sudah kedaluwarsa > 30 hari, dan tidak punya keanggotaan project. Super Admin yang ingin membatalkan undangan dari halaman Users memakai "nonaktifkan" (T01).
- K14. Undangan dari halaman Users: nama opsional (default dari bagian lokal email, bisa diubah penerima nanti lewat Super Admin `updateuser`). Role selalu `developer`. Berbeda dengan `createuser`: tidak ada input password; sistem membuatnya dan mengirim.
- K15. Dev lokal memakai Mailpit di `docker-compose.yml` (SMTP 1025, UI 8025) dan `.env.example` menunjuk ke sana.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`. `core/auth` tidak boleh meng-import `project/management` dan sebaliknya.
- Tabel baru milik `core/auth` ada di `migrations/core/`. Tidak ada FK lintas module selain pola T03.
- Password sementara mengikuti aturan hash dan kebijakan T01; tidak pernah masuk log, audit, atau bus event.
- Kontrak API di `api/modules/core/auth.yml` dan `api/modules/project/management.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Teks UI dan email dalam bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Dokumentasi konfigurasi diperbarui di `docs/configuration.md`, `docs/infra/running.md`, dan `.env.example`.
- Nama file/folder baru, bentuk persis payload, dan pemilihan library SMTP diserahkan ke pembangun (utamakan stdlib `net/smtp` atau library kecil yang sudah ada di repo).

## Features

### infra / mailer

| Paket | Deskripsi |
|---|---|
| `infra/mailer` | Antarmuka `Mailer`, driver `smtp` (STARTTLS / TLS implisit / tanpa TLS untuk dev), `log`, dan no-op tidak terkonfigurasi; template email undangan; masuk `app.Deps` |

### core / auth

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `inviteuser` | Buat akun developer + undangan (atau terbitkan ulang bila masih menunggu), kirim email, kembalikan hasil fallback | `POST /api/users/invite` dan bus `core.auth.invite-user` | super_admin; internal (dipanggil T03) |
| `resendinvitation` | Password sementara baru + perpanjang masa berlaku + kirim email | `POST /api/users/{userID}/resend-invitation` dan bus `core.auth.resend-invitation` | super_admin; internal (dipanggil T03) |
| `purgeinvitations` | Job: hapus akun belum diterima yang kedaluwarsa > 30 hari dan tanpa keanggotaan | job harian | sistem |

Perubahan pada feature T01: `login` menolak akun yang undangannya kedaluwarsa (K3); `changepassword` menandai undangan diterima (K4); `listusers`/`getuser` menampilkan penanda "undangan menunggu" dan masa berlaku; `getuserbus` dan `finddevelopers` memberi penanda yang sama.

### project / management

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `inviteuser` | Undang lewat email ke project: validasi akses, panggil `core.auth.invite-user`, catat keanggotaan, audit | `POST /api/projects/{projectID}/invitations` | owner, super_admin |
| `listinvitations` | Daftar undangan belum diterima di project (email, role, pengundang, dikirim, kedaluwarsa, status kirim) | `GET /api/projects/{projectID}/invitations` | owner, super_admin |
| `resendinvitation` | Kirim ulang | `POST /api/projects/{projectID}/invitations/{userID}/resend` | owner, super_admin |
| `revokeinvitation` | Batalkan undangan di project | `DELETE /api/projects/{projectID}/invitations/{userID}` | owner, super_admin |

`listmembers` (T03) tidak berubah; anggota hasil undangan tampil di sana seperti anggota lain dengan penanda "belum menerima undangan". `listinvitations` hanya menyaring yang belum diterima. Pembangun boleh menggabungkan tampilannya di UI.

## Aturan bisnis

- Email dinormalisasi lowercase dan dipangkas; format divalidasi, panjang maks 254; unik dengan `users.email` (citext) seperti T01.
- Role undangan project `member` (default) atau `owner`; `owner` hanya boleh dipilih oleh owner/admin (sama dengan `addmember`).
- Pengundang harus berwenang (owner project atau super_admin) dan akunnya aktif. Member biasa tidak bisa mengundang.
- Akun undangan memakai semua aturan T01 (nama 1–100 karakter, email unik, dan seterusnya).
- Ganti password pertama mengikuti aturan T01 (min 10 karakter, tidak sama dengan email, tidak sama dengan password lama) dan mencabut sesi lain.
- Mengirim ulang mencabut sesi akun itu dan mengganti hash password sementara seketika.
- Audit tidak pernah memuat password sementara atau hash-nya.
- Email hanya dikirim ke alamat yang diundang; tanpa CC/BCC.

## Model data

Tambahan di `migrations/core/` (nama boleh disesuaikan pembangun):

```
user_invitations(
  user_id          uuid pk  fk→users on delete cascade,   -- satu undangan per akun
  invited_by       uuid not null,                          -- tanpa FK (pola T01 created_by)
  invited_by_label text null,
  sent_at          timestamptz null,                       -- null = belum pernah berhasil terkirim
  expires_at       timestamptz not null,
  accepted_at      timestamptz null,
  resend_count     int not null default 0,
  created_at       timestamptz not null default now()
)
index on (expires_at) where accepted_at is null
```

Tidak ada kolom token dan tidak ada status `pending` baru di `users`: bentuk "menunggu" cukup dari `accepted_at IS NULL`. Akun yang dibuat lewat `createuser` tidak punya baris di tabel ini dan tidak terpengaruh. `project_members` (T03) tidak berubah bentuk; migrasi `migrations/project/` hanya bila pembangun perlu kolom bantu.

## Kontrak bus

Disediakan T01 (`lib/contracts/core/auth/...`):
- `core.auth.invite-user` Call — `{Email, Name, InvitedBy, InvitedByLabel, ProjectName}` → `{UserID, Created, EmailSent, Credentials?}`.
- `core.auth.resend-invitation` Call — `{UserID, InvitedBy, ProjectName}` → `{EmailSent, Credentials?}`.

Diubah (aditif): `core.auth.get-user` dan `core.auth.find-developers` membawa penanda undangan menunggu. `check-access` (T03) tidak berubah; akun undangan yang belum kedaluwarsa dianggap aktif biasa.

Dikonsumsi T03: dua endpoint di atas dan `core.audit.record` (fail-open). `Credentials` hanya diteruskan kembali ke respons HTTP, tidak ditulis ke audit atau log.

Konstanta di `lib/constants/bus.go`; katalog di `docs/tickets/README.md` diperbarui.

## Konfigurasi

Env baru (nama final diatur pembangun, konsisten dengan `config.go`):

| Env | Keterangan |
|---|---|
| `MAIL_DRIVER` | `smtp` \| `log` (hanya non-produksi) \| kosong (tidak terkonfigurasi) |
| `SMTP_HOST`, `SMTP_PORT` | alamat server |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | kredensial, opsional |
| `SMTP_TLS` | `starttls` \| `tls` \| `none` |
| `MAIL_FROM` | alamat dan nama pengirim |
| `APP_BASE_URL` | dasar URL login di email |
| `INVITATION_TTL` | default `168h` |

Password SMTP tidak boleh tercetak di log atau output config.

## UI (Svelte)

- `domain/project/management/...`: pada dialog Tambah Anggota, bila pencarian kosong dan teks berbentuk email, tampil aksi "Undang `<email>` lewat email" dengan pilihan peran. Bila `emailEnabled=false`, aksi tetap ada dengan keterangan bahwa kredensial akan ditampilkan untuk disalin.
- Section "Belum menerima undangan" di Members: email, peran, diundang oleh, dikirim, kedaluwarsa, penanda "email gagal terkirim", aksi Kirim ulang dan Batalkan (dengan konfirmasi).
- Dialog "Kredensial sementara" (sekali tampil): email, password sementara, URL login, tombol salin, peringatan "tidak akan ditampilkan lagi". Dipakai di alur project dan alur Users.
- `domain/core/auth/...` (halaman Users): tombol "Undang developer" (email, nama opsional), penanda "undangan menunggu" di daftar, aksi "Kirim ulang undangan".
- Login dan ganti password paksa memakai alur T01 yang ada; tambahkan teks bantu "gunakan password sementara dari email undangan".
- Semua teks lewat i18n `id` dan `en`.

## Titik sentuh

Backend: `infra/config`, `infra/app` (`Deps`), paket baru `infra/mailer`, `core/internal/auth` (feature baru + `login`, `changepassword`, `listusers`, `getuser`, `getuserbus`, `finddevelopers`), `project/internal/management` (feature baru), `lib/constants/bus.go`, `lib/contracts`, `migrations/core`, job di `infra/jobs`.
Frontend: dialog anggota dan view Members (T03), halaman Users (T01), teks login/ganti password, `shared/i18n`.
Lainnya: `docker-compose.yml` (Mailpit), `.env.example`, `docs/configuration.md`, `docs/infra/running.md`, `api/modules/**`, `api/openapi.yml`.

## Aksi audit baru

Tambah di `lib/contracts/core/audit/record/actions.go` (dan `Actions()`), perbarui `actions_test.go`:
`user.invited`, `user.invitation.resent`, `user.invitation.accepted`, `user.invitation.email_failed`, `project.member.invited`, `project.invitation.revoked`.

## Catatan revisi

- T01: non-goal "email/SMTP" tidak lagi berlaku untuk undangan; reset password mandiri tetap non-goal. `login`, `changepassword`, `listusers`, `getuser` dan handler bus user mendapat perubahan aditif.
- T03: non-goal "undangan lewat email" digantikan tiket ini; `addmember` untuk developer yang sudah punya akun tidak berubah.
- Beri catatan revisi di kedua tiket itu (tidak ditulis ulang).

## Risiko

- Password sementara yang dikirim lewat email tersimpan di kotak masuk penerima dan penyedia email. Mitigasi: kedaluwarsa 7 hari, wajib ganti saat login pertama, dan password lama langsung tak berlaku setelah diganti.
- Fallback menampilkan kredensial di layar pengundang. Mitigasi: hanya sekali, `no-store`, tidak di audit/log, hanya untuk pengundang yang berwenang.
- Deliverability email (SPF/DKIM) di luar kendali aplikasi; undangan bisa masuk spam. Mitigasi: kirim ulang dan fallback.
- Akun undangan yang tidak pernah diterima menumpuk. Mitigasi: job `purgeinvitations`.
- Mengubah `login` dan `changepassword` menyentuh jalur kritis auth; butuh tes regresi.

## Kriteria penerimaan

1. Owner mengundang `baru@contoh.id` yang belum punya akun: respons `201`, akun developer terbuat dengan `must_change_password=true`, keanggotaan tercatat, satu email terkirim berisi alamat login dan password sementara; respons API tidak memuat password, dan password tidak ada di DB (hanya hash), log, maupun audit.
2. Penerima login dengan password sementara, dipaksa ganti password, setelah itu `getme` normal dan project muncul di daftarnya dengan role yang diundang; undangan tercatat diterima.
3. Akun undangan yang belum diterima dan lewat `expires_at` tidak bisa login (respons sama seperti kredensial salah); kirim ulang memulihkannya dengan password baru dan password lama tidak berlaku.
4. Mengundang email yang sudah akun yang menerima undangan, atau yang dibuat lewat `createuser` → `ALREADY_REGISTERED`; mengundang email yang sudah anggota project → ditolak; mengundang email yang masih menunggu → tidak membuat akun ganda, memberi password baru, dan mengirim ulang.
5. Member (bukan owner) mendapat 403 di semua endpoint undangan project; non-anggota mendapat 404.
6. Super Admin mengundang dari halaman Users: akun developer terbuat tanpa project dan email terkirim; developer biasa mendapat 403.
7. Kirim ulang melebihi rate limit → `429`.
8. Membatalkan undangan dari project menghapus keanggotaan; job purge menghapus akun belum diterima yang kedaluwarsa > 30 hari tanpa keanggotaan.
9. SMTP mati/ditolak atau tidak dikonfigurasi: undangan tetap tercatat, respons `emailSent=false` dengan `credentials` sekali tampil dan `Cache-Control: no-store`; kredensial tidak bisa diambil lagi lewat endpoint mana pun; error tercatat di log dan audit tanpa password.
10. Dengan email sukses, respons tidak memuat `credentials` sama sekali.
11. Tanpa konfigurasi email aplikasi start normal; `MAIL_DRIVER=log` ditolak saat mode produksi.
12. Dev lokal: `docker compose up` menyalakan Mailpit dan email undangan terlihat di UI Mailpit.
13. Audit mencatat semua aksi di "Aksi audit baru" tanpa data rahasia; arch test, `make api-check`, lint, tes unit/integrasi/e2e hijau.

## Pertanyaan terbuka

Tidak ada. Semua keputusan K-pembangun di atas masih menunggu persetujuan pemilik, terutama K3 (masa berlaku 7 hari), K5/K6 (penanganan email yang sudah ada), dan K13 (pembatalan tidak menghapus akun seketika).
