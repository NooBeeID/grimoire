# T12 — infra / mailer + core / auth + project / management (undangan anggota lewat email)

| | |
|---|---|
| Domain / Module | infra / mailer (baru), core / auth (undangan), project / management (invite anggota) |
| Ukuran | L |
| Bergantung pada | T01, T02, T03 |
| Sumber | permintaan pemilik (belum ada spec desain) |

## Ringkasan

Hari ini owner hanya bisa menambah anggota yang **sudah punya akun** (T03 `addmember`, autocomplete lewat `searchmembercandidates`). Orang yang belum punya akun harus dibuatkan Super Admin lebih dulu (T01 `createuser`). Tiket ini menambah jalur undangan: bila developer tidak ditemukan, owner bisa mengundang lewat email. Sistem membuat akun dengan token undangan sekali pakai, mengirim email berisi tautan, dan penerima langsung menetapkan password sendiri lalu masuk.

Tiket ini juga memperkenalkan pengiriman email (SMTP) ke aplikasi. Ini membalik non-goal "email/SMTP" di T01 dan "undangan lewat email" di T03 (lihat "Catatan revisi").

## Tujuan

1. Owner (dan Super Admin) bisa mengundang orang lewat email ke sebuah project dari dialog tambah anggota, bila pencarian developer tidak menemukan orangnya.
2. Penerima menerima email, membuka tautan, menetapkan password, dan langsung login sebagai developer anggota project itu, tanpa campur tangan Super Admin.
3. Owner bisa melihat undangan yang masih menunggu, mengirim ulang, dan membatalkannya.
4. Infrastruktur kirim email bisa dipakai module lain nanti (mis. notifikasi), tanpa module itu tahu soal SMTP.

## Non-goals

- Reset password mandiri lewat email ("lupa password"). Infra email-nya terpakai, fiturnya bukan bagian tiket ini.
- Verifikasi email untuk akun yang dibuat Super Admin lewat `createuser`.
- Undangan sebagai role `super_admin`. Undangan hanya menghasilkan `developer`.
- Template email berbahasa banyak, email HTML bergaya marketing, pelacakan buka/klik, antrean email persisten (outbox), retry di latar belakang.
- Undangan massal (banyak email sekaligus) dan undangan lewat tautan publik tanpa email.
- Provider email berbasis API (SES, SendGrid, dst). Hanya SMTP.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Token undangan tersimpan polos di DB, ada di log, audit, atau respons API (selain di dalam email itu sendiri).
- Endpoint undangan/penerimaan membocorkan apakah sebuah email sudah terdaftar kepada pihak yang tidak berwenang.
- Akun undangan bisa login dengan password selain yang ditetapkan penerima sendiri lewat token.
- Kegagalan SMTP membuat request owner gagal setengah jalan (akun terbuat tanpa jejak, atau anggota tercatat tanpa undangan bisa dikirim ulang).

## User story

- Sebagai owner, saya mengetik email rekan yang belum punya akun lalu menekan "Undang lewat email" agar rekan itu bisa bergabung tanpa menunggu Super Admin.
- Sebagai penerima undangan, saya membuka email, memilih password, dan langsung masuk ke project.
- Sebagai owner, saya melihat siapa yang belum menerima undangan, bisa kirim ulang bila email tidak sampai, atau batalkan bila salah alamat.
- Sebagai operator, saya mengatur SMTP lewat env dan bisa mengembangkan lokal tanpa SMTP sungguhan.

## Alur

```
Owner buka dialog "Tambah anggota"
  └─ cari developer (searchmembercandidates)
       ├─ ketemu  → addmember (alur T03, tidak berubah)
       └─ tidak ketemu + input berbentuk email → tombol "Undang lewat email"
            └─ POST /api/projects/{projectID}/invitations {email, role}
                 1. cek akses (owner/admin), cek email belum anggota
                 2. bus core.auth.invite-user  → buat/ambil akun "pending" + token baru
                 3. tambah project_members (user_id akun pending, role)
                 4. commit → kirim email (di luar transaksi)
                 5. audit
Penerima buka  {APP_BASE_URL}/accept-invitation?token=…
  └─ POST /api/auth/accept-invitation {token, name, password}
       → akun aktif, must_change_password=false, token dipakai, sesi dibuat (login otomatis)
```

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

- K1. **Token berupa tautan, bukan password sementara.** Permintaan awal menyebut "akun dengan default token, lalu user langsung update password". Saya terjemahkan menjadi: akun dibuat dengan `password_hash` yang tidak bisa dipakai login, token undangan acak 256-bit dikirim sebagai tautan, dan penerima menetapkan password sendiri di halaman penerimaan. Alasan: password default yang dikirim lewat email tersimpan di kotak masuk selamanya dan bisa dipakai login sebelum diganti. Bila pemilik tetap ingin "password default = token", ubah K1 dan alur penerimaan menjadi login + `must_change_password` (T01).
- K2. **Akun dibuat saat undangan dikirim**, bukan saat diterima, supaya keanggotaan project bisa tercatat dan tampil sebagai "menunggu". Akun ini berstatus `pending`: tidak bisa login, tidak dihitung kandidat `searchmembercandidates`/`find-developers`, dan `check-access` menganggapnya `none`.
- K3. Masa berlaku token **7 hari** (konfigurabel). Token kedaluwarsa tidak bisa dipakai; owner/admin bisa kirim ulang (token lama dicabut, token baru dibuat).
- K4. Hanya **satu undangan aktif per akun pending**. Mengundang email yang sudah pending (di project mana pun) tidak membuat akun baru: ia menambah keanggotaan project ini, menerbitkan token baru, dan mengirim ulang email.
- K5. Email yang sudah punya akun `active` ditolak di endpoint undangan dengan `ALREADY_REGISTERED` dan UI mengarahkan owner memakai pencarian biasa. Ini hanya terlihat oleh owner/admin yang berwenang atas project itu (mereka memang boleh mencari developer lewat T03); endpoint penerimaan tidak pernah membedakan kasus.
- K6. Email dikirim **setelah commit**, dalam satu percobaan dengan timeout. Gagal kirim tidak membatalkan undangan: respons `201` dengan `emailSent: false`, entri tetap "menunggu", dan owner memakai "Kirim ulang". Error SMTP di-log (tanpa token) dan diaudit.
- K7. Pengiriman memakai paket `infra/mailer` dengan antarmuka kecil (`Send(ctx, Message) error`) dan dua driver: `smtp` dan `log` (mencetak email ke log, untuk dev/tes). Module mengakses lewat `app.Deps`, bukan meng-import SMTP langsung.
- K8. Bila SMTP **tidak dikonfigurasi** dan driver bukan `log`, fitur undangan mati: endpoint mengembalikan `503 EMAIL_NOT_CONFIGURED`, `GET /api/config`/`getme` (atau endpoint sejenis yang sudah ada) memberi flag `invitationsEnabled=false`, dan UI menyembunyikan tombol undang. Aplikasi tetap bisa start tanpa SMTP.
- K9. `APP_BASE_URL` wajib diisi untuk membangun tautan; bila kosong dan driver `smtp`, validasi config gagal saat start.
- K10. Teks email bahasa Indonesia, plain text + HTML sederhana (satu tombol/tautan). Memuat nama pengundang, nama project, role, tanggal kedaluwarsa. Tidak memuat password.
- K11. Rate limit undangan: maksimal 20 undangan/jam per pengundang dan 3 kirim ulang/jam per undangan (nilai default, konfigurabel). Endpoint penerimaan memakai throttle per IP seperti login.
- K12. Pembatalan undangan: hanya bila akun masih `pending`. Membatalkan menghapus keanggotaan di project itu; akun pending dihapus bila tidak lagi menjadi anggota project mana pun.
- K13. Dev lokal memakai Mailpit di `docker-compose.yml` (SMTP 1025, UI 8025) dan `.env.example` menunjuk ke sana.

## Keputusan pemilik

Belum ada. K1 dan K2 yang paling menentukan bentuk akhir; mohon ditinjau sebelum implementasi.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`. `core/auth` tidak boleh meng-import `project/management` dan sebaliknya.
- Tabel baru milik `core/auth` ada di `migrations/core/`. Tidak ada FK lintas module selain pola T03 (`project_id`/`user_id`, `ON DELETE CASCADE`).
- Token: 256-bit acak (crypto/rand), disimpan sebagai hash (SHA-256), dibandingkan dengan constant-time. Satu kali pakai.
- Kontrak API di `api/modules/core/auth.yml` dan `api/modules/project/management.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Teks UI dan email dalam bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Dokumentasi konfigurasi diperbarui di `docs/configuration.md` dan `.env.example`.
- Nama file/folder baru, bentuk persis payload, dan pemilihan library SMTP diserahkan ke pembangun (utamakan stdlib `net/smtp` atau library kecil yang sudah dipakai di repo).

## Features

### infra / mailer

| Paket | Deskripsi |
|---|---|
| `infra/mailer` | Antarmuka `Mailer`, driver `smtp` (STARTTLS / TLS implisit / tanpa TLS untuk dev) dan `log`; template email undangan; ditambahkan ke `app.Deps` |

### core / auth

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `inviteuser` | Buat akun `pending` + undangan (atau terbitkan ulang token bila sudah pending); kirim email | bus `core.auth.invite-user` | internal (dipanggil T03) |
| `resendinvitation` | Cabut token lama, terbitkan baru, kirim email | bus `core.auth.resend-invitation` | internal (dipanggil T03) |
| `revokeinvitation` | Cabut token; hapus akun pending bila tidak ada lagi keanggotaan | bus `core.auth.revoke-invitation` | internal (dipanggil T03) |
| `getinvitation` | Periksa token (valid/kedaluwarsa/terpakai) dan tampilkan email + nama project untuk halaman penerimaan | `GET /api/auth/invitations/{token}` | publik (throttle) |
| `acceptinvitation` | Tetapkan nama + password, aktifkan akun, tandai token terpakai, buat sesi + set cookie | `POST /api/auth/accept-invitation` | publik (throttle) |
| `purgeinvitations` | Job: hapus undangan kedaluwarsa > 30 hari dan akun pending tanpa keanggotaan | job harian | sistem |

### project / management

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `inviteuser` | Undang lewat email ke project: validasi akses, panggil `core.auth.invite-user`, catat keanggotaan, audit | `POST /api/projects/{projectID}/invitations` | owner, super_admin |
| `listinvitations` | Daftar undangan menunggu di project (email, role, pengundang, dikirim, kedaluwarsa, status kirim) | `GET /api/projects/{projectID}/invitations` | owner, super_admin |
| `resendinvitation` | Kirim ulang | `POST /api/projects/{projectID}/invitations/{userID}/resend` | owner, super_admin |
| `revokeinvitation` | Batalkan undangan | `DELETE /api/projects/{projectID}/invitations/{userID}` | owner, super_admin |

`listmembers` (T03) tetap menampilkan hanya anggota aktif; undangan menunggu hanya muncul di `listinvitations`. Pembangun boleh menggabungkan tampilannya di UI (tab/section "Menunggu").

## Aturan bisnis

- Email dinormalisasi lowercase dan dipangkas; format divalidasi dan panjang maks 254. Email unik dengan `users.email` (citext) seperti T01.
- Role undangan `member` (default) atau `owner`; `owner` hanya boleh dipilih oleh owner/admin (sama dengan `addmember`).
- Pengundang harus berwenang atas project (owner atau super_admin) dan akunnya aktif.
- Akun `pending` diberi penanda eksplisit, terpisah dari `is_active`, supaya tidak ambigu dengan "dinonaktifkan Super Admin". Akun pending tidak bisa login dan tidak bisa dipilih sebagai kandidat. Mengaktifkan akun pending lewat T01 tidak membuatnya bisa login sebelum undangan diterima.
- Menerima undangan: nama 1–100 karakter, password mengikuti aturan T01 (min 10 karakter, tidak sama dengan email), `must_change_password=false`, flag pending dihapus, semua keanggotaan project tetap, token ditandai terpakai dalam transaksi yang sama. Token yang sudah dipakai atau kedaluwarsa → `400 INVITATION_INVALID` (pesan sama untuk salah/terpakai/kedaluwarsa supaya tidak jadi oracle).
- Penerimaan bersifat atomik dan tahan balapan: dua permintaan serentak dengan token yang sama, hanya satu berhasil.
- Saat penerimaan, bila undangan project sudah dibatalkan, token tidak berlaku.
- Mengirim ulang mencabut token lama seketika.
- Audit tidak pernah memuat token, hash token, atau password.
- Email hanya dikirim ke alamat yang diundang; tidak ada CC/BCC.

## Model data

Tambahan di `migrations/core/` (nama tabel dan kolom boleh disesuaikan pembangun):

```
user_invitations(
  user_id        uuid pk  fk→users on delete cascade,   -- satu undangan aktif per akun pending (K4)
  token_hash     bytea not null unique,
  invited_by     uuid not null,                          -- tanpa FK (pola T01 created_by)
  invited_by_label text null,
  sent_at        timestamptz null,                       -- null = belum pernah berhasil terkirim
  expires_at     timestamptz not null,
  accepted_at    timestamptz null,
  created_at     timestamptz not null default now(),
  resend_count   int not null default 0
)
index on (expires_at)
```

Sumber status pending: `user_invitations.accepted_at IS NULL` bersama penanda di `users` (mis. kolom `invitation_pending boolean`), dipilih pembangun asal `login` menolak akun pending dengan respons yang sama seperti kredensial salah.

`project_members` (T03) tidak berubah bentuk; undangan menunggu dikenali lewat gabungan dengan `core.auth.get-user` (`status=pending`). Pembangun mungkin menambah `invited_at` bila perlu daftar cepat; migrasi di `migrations/project/`.

## Kontrak bus

Disediakan T01 (`lib/contracts/core/auth/...`):
- `core.auth.invite-user` Call — `{Email, InvitedBy, InvitedByLabel, ProjectName, Role}` → `{UserID, Created, EmailSent}`.
- `core.auth.resend-invitation` Call — `{UserID, InvitedBy, ProjectName}` → `{EmailSent}`.
- `core.auth.revoke-invitation` Call — `{UserID}` → `{Revoked, AccountRemoved}`.

Diubah (aditif): `core.auth.get-user` mengembalikan penanda `Pending`; `core.auth.find-developers` mengecualikan akun pending; `check-access` (T03) memperlakukan pending sebagai `none`.

Dikonsumsi T03: tiga endpoint di atas, `core.audit.record` (fail-open).

Konstanta di `lib/constants/bus.go`; katalog di `docs/tickets/README.md` diperbarui.

## Konfigurasi

Env baru (nama final diatur pembangun, konsisten dengan `config.go`):

| Env | Keterangan |
|---|---|
| `MAIL_DRIVER` | `smtp` \| `log` (default `log` di dev) |
| `SMTP_HOST`, `SMTP_PORT` | alamat server |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | kredensial, opsional |
| `SMTP_TLS` | `starttls` \| `tls` \| `none` |
| `MAIL_FROM` | alamat dan nama pengirim |
| `APP_BASE_URL` | dasar tautan di email (mis. `https://grimoire.example.com`) |
| `INVITATION_TTL` | default `168h` |

Password SMTP tidak boleh dicetak di log atau di output `config` yang ditampilkan.

## UI (Svelte)

- `domain/project/management/...`: pada dialog Tambah Anggota, bila pencarian kosong dan teks berbentuk email, tampil aksi "Undang `<email>` lewat email" dengan pilihan peran. Tombol hanya tampil bila `invitationsEnabled`.
- Section/tab "Menunggu" di Members: tabel email, peran, diundang oleh, dikirim, kedaluwarsa, penanda "email gagal terkirim", aksi Kirim ulang dan Batalkan (dengan konfirmasi).
- `domain/core/auth/acceptinvitation`: halaman publik `/accept-invitation?token=…`: memvalidasi token saat dibuka (nama project + email ditampilkan), form nama + password + konfirmasi, tampil kesalahan jelas untuk token tidak valid/kedaluwarsa, setelah sukses redirect ke project. Rute ini dikecualikan dari guard login dan memakai `Referrer-Policy: no-referrer`.
- Semua teks lewat i18n `id` dan `en`.

## Titik sentuh

Backend: `infra/config`, `infra/app` (`Deps`), paket baru `infra/mailer`, `core/internal/auth` (feature baru + login/getuser/finddevelopers), `project/internal/management` (feature baru + checkaccess), `lib/constants/bus.go`, `lib/contracts`, `eventregistry` bila perlu, `migrations/core`, job di `infra/jobs`.
Frontend: dialog anggota dan view Members (T03), rute publik baru, `shared/i18n`.
Lainnya: `docker-compose.yml` (Mailpit), `.env.example`, `docs/configuration.md`, `docs/infra/running.md`, `api/modules/**`, `api/openapi.yml`.

## Aksi audit baru

Tambah di `lib/contracts/core/audit/record/actions.go` (dan `Actions()`), perbarui `actions_test.go`:
`user.invited`, `user.invitation.resent`, `user.invitation.revoked`, `user.invitation.accepted`, `user.invitation.email_failed`, `project.member.invited`.

## Catatan revisi

- T01: non-goal "email/SMTP" tidak lagi berlaku untuk undangan. Reset password mandiri tetap non-goal.
- T03: non-goal "undangan lewat email" digantikan tiket ini. `addmember` untuk developer yang sudah punya akun tidak berubah.
- Beri catatan revisi di kedua tiket itu (tidak ditulis ulang).

## Risiko

- Deliverability email (SPF/DKIM) di luar kendali aplikasi; undangan bisa masuk spam. Mitigasi: tombol Kirim ulang dan status "gagal terkirim". Tautan sengaja tidak ditampilkan ke owner, karena itu melemahkan jaminan token.
- Akun pending yang tidak pernah diterima menumpuk. Mitigasi: job `purgeinvitations`.
- Perubahan `login`/`getuser`/`finddevelopers` menyentuh jalur kritis auth; butuh tes regresi.

## Kriteria penerimaan

1. Owner mengundang `baru@contoh.id` yang belum punya akun: respons `201`, akun `pending` terbuat, keanggotaan tercatat, satu email terkirim berisi tautan dengan token; token tidak ada di DB (hanya hash), di log, maupun di respons API.
2. Penerima membuka tautan, menetapkan nama dan password, otomatis login; `getme` mengembalikan developer dengan `must_change_password=false`; project muncul di daftarnya dengan role yang diundang.
3. Token yang sudah dipakai, kedaluwarsa, dicabut, atau salah semuanya mengembalikan `INVITATION_INVALID` yang sama; dua penerimaan serentak dengan token yang sama hanya satu yang berhasil.
4. Akun `pending` tidak bisa login (respons sama dengan kredensial salah), tidak muncul di `searchmembercandidates`, dan `check-access`-nya `none`.
5. Mengundang email yang sudah akun aktif → `ALREADY_REGISTERED`; mengundang email yang sudah anggota project → ditolak; mengundang email pending yang sama dari project lain → tidak membuat akun ganda dan menerbitkan token baru.
6. Member (bukan owner) mendapat 403 di semua endpoint undangan; non-anggota mendapat 404.
7. Kirim ulang membuat token lama tidak berlaku seketika; melebihi rate limit → `429`.
8. Membatalkan undangan menghapus keanggotaan; akun pending tanpa keanggotaan lain ikut dihapus; tautan lama tidak berlaku.
9. SMTP mati/ditolak: undangan tetap tercatat, respons `emailSent=false`, UI menandai gagal terkirim, Kirim ulang berhasil setelah SMTP pulih; error tercatat di log dan audit tanpa token.
10. Tanpa konfigurasi email: aplikasi start normal, `invitationsEnabled=false`, tombol undang tersembunyi, endpoint mengembalikan `503 EMAIL_NOT_CONFIGURED`.
11. Dev lokal: `docker compose up` menyalakan Mailpit dan email undangan terlihat di UI Mailpit.
12. Audit mencatat semua aksi di "Aksi audit baru" tanpa data rahasia; arch test, `make api-check`, lint, tes unit/integrasi/e2e hijau.

## Pertanyaan terbuka

1. **K1**: tautan token (rekomendasi) atau password sementara di email seperti permintaan awal?
2. **K8**: boleh fitur undangan mati total bila SMTP belum dikonfigurasi, atau Super Admin perlu jalur cadangan (mis. salin tautan)?
3. Apakah Super Admin juga perlu bisa mengundang dari halaman Users (T01) tanpa terikat project?
