# T16 — project / notification (siaran email ke anggota project)

| | |
|---|---|
| Domain / Module | project / notification (baru); tambahan handler bus kecil di core / auth dan project / management |
| Ukuran | M |
| Bergantung pada | T01, T02, T03, T12 |
| Sumber | permintaan pemilik (belum ada spec desain) |

## Ringkasan

Owner project (atau Super Admin) bisa menulis pesan (subjek + isi teks biasa) lalu mengirimkannya lewat email ke **semua anggota project**. Pengiriman dilakukan langsung (tanpa digest), memakai `infra/mailer` dari T12. Tiap siaran dan status kirim per penerima disimpan sebagai riwayat; owner bisa melihatnya dan mengirim ulang yang gagal.

Ini siaran **manual**. Tidak ada notifikasi otomatis dari kejadian sistem (perubahan knowledge, API spec, anggota, hasil git sync). Itu bukan bagian tiket ini (lihat non-goals).

## Tujuan

1. Owner dan Super Admin bisa mengirim siaran email (subjek + isi) ke seluruh anggota sebuah project dari halaman project.
2. Setiap penerima mendapat **email sendiri** (tanpa CC/BCC), berisi nama project, nama pengirim, subjek, isi, dan footer.
3. Riwayat siaran per project: siapa mengirim, kapan, subjek, jumlah penerima, jumlah terkirim/gagal, dan status per penerima.
4. Owner bisa mengirim ulang ke penerima yang gagal tanpa membuat siaran baru.
5. Penyalahgunaan dibatasi: rate limit per project, batas panjang isi, dan penerima hanya anggota project saat dikirim.

## Non-goals

- Notifikasi otomatis dari event sistem (knowledge, API spec, anggota/role, git sync, audit) dan digest berkala.
- Preferensi atau opt-out per anggota. Versi pertama selalu terkirim ke semua anggota yang memenuhi syarat penerima.
- Markdown/HTML dari pengirim, lampiran, gambar, template pilihan, dan penjadwalan kirim nanti.
- Siaran lintas project atau ke semua pengguna (siaran global Super Admin).
- Antrean persisten dengan retry otomatis di latar belakang, pelacakan buka/klik, dan balasan masuk.
- Notifikasi in-app, push, atau kanal selain email.
- Membatalkan siaran yang sedang berjalan.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Satu penerima bisa melihat alamat email penerima lain (CC/BCC bersama, header To berisi banyak alamat).
- Isi pesan dari pengirim disisipkan ke HTML tanpa escape (injeksi HTML/link), atau subjek/header bisa disusupi newline (header injection).
- Email terkirim ke orang yang bukan anggota aktif project saat itu, atau ke akun nonaktif.
- Kegagalan SMTP untuk sebagian penerima menggagalkan seluruh siaran atau membuat statusnya tidak terlacak.
- Isi pesan masuk ke audit log atau log aplikasi.
- Member biasa bisa mengirim, atau non-anggota bisa membaca riwayat.

## User story

- Sebagai owner, saya menulis pengumuman (mis. "deploy knowledge baru malam ini") dan mengirimkannya ke seluruh anggota project dengan sekali klik.
- Sebagai owner, saya melihat siapa saja yang menerima dan siapa yang gagal, lalu mengirim ulang yang gagal.
- Sebagai Super Admin, saya bisa menyiarkan ke project mana pun.
- Sebagai anggota, saya menerima email yang jelas dari project mana dan dari siapa.
- Sebagai operator, siaran tidak membanjiri SMTP: ada rate limit dan batas konkurensi.

## Alur

```
Halaman project → tab/section "Siaran" → "Kirim siaran" {subject, body}
  POST /api/projects/{projectID}/broadcasts

  1. RequireAuth → check-access (owner atau super_admin; member 403, non-anggota 404)
  2. mailer terkonfigurasi? tidak → 503 EMAIL_NOT_CONFIGURED (tidak ada fallback manual)
  3. validasi subject/body; rate limit per project
  4. daftar penerima (snapshot): anggota aktif project, via bus → alamat email via bus
  5. simpan broadcast + broadcast_recipients (status pending) dalam satu transaksi → commit
  6. audit (tanpa isi pesan)
  7. respons 202 {id, recipientCount, status: "sending"}
  8. di latar (goroutine dibatasi): kirim satu email per penerima, update status
     sent | failed (+ error ringkas) per baris; selesai → broadcast.status selesai

Owner: GET .../broadcasts, GET .../broadcasts/{broadcastID}
Gagal sebagian: POST .../broadcasts/{broadcastID}/resend-failed → ulangi hanya pending/failed
```

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Pemicu | **Siaran manual dari owner**, bukan otomatis dari event. |
| Penerima | **Semua anggota project**; tanpa opt-out. |
| Preferensi | **Tanpa pengaturan** di versi pertama. |
| Pengiriman | **Langsung** per kejadian (siaran), bukan digest. |
| Pengirim | **Owner project + super_admin**. |
| Riwayat | **Ya**: simpan siaran dan status kirim per penerima, bisa kirim ulang yang gagal. |
| Isi pesan | **Subjek + isi teks biasa** dengan template email sederhana. |
| Batas | **Rate limit + batas penerima/panjang**; angka konfigurabel. |

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

- K1. Module baru `project/notification` (bukan fitur di `management`) agar tabel dan logika email-nya terpisah; berinteraksi hanya lewat bus.
- K2. **Penerima** = anggota project (semua role) yang akunnya aktif, termasuk akun undangan T12 yang belum kedaluwarsa. Pengirim **ikut menerima** salinan (default; mudah dicek dan memberi bukti email jalan). Daftar penerima di-snapshot saat kirim; anggota yang masuk sesudahnya tidak ikut, dan kirim ulang tetap memakai snapshot itu.
- K3. Satu email per penerima, `To` hanya alamat itu. Pengiriman di latar dengan konkurensi kecil (default 4) dan timeout per email (default 10 s); tanpa retry otomatis. Request tidak menunggu SMTP, merespons `202`.
- K4. Siaran yang tertinggal `pending` karena proses mati: job periodik singkat menandai baris `pending` yang lebih tua dari 15 menit sebagai `failed` (error `interrupted`) sehingga bisa dikirim ulang. Tidak mengirim otomatis.
- K5. Mailer tidak terkonfigurasi (`Mailer` no-op T12) → `503 EMAIL_NOT_CONFIGURED`; siaran tidak dibuat. Driver `log` dianggap terkonfigurasi (dev).
- K6. Batas default: subjek 1–150 karakter tanpa newline/kontrol; isi 1–5000 karakter; maks 5 siaran/jam per project; maks 200 penerima per siaran (lebih dari itu → `422 TOO_MANY_RECIPIENTS`). Semua konfigurabel lewat env.
- K7. Subjek email diawali `[<nama project>] ` agar mudah disaring. Isi teks biasa dikirim sebagai `text/plain` dan versi HTML sederhana dengan semua isi di-escape serta baris baru jadi `<br>`; URL tidak dijadikan tautan otomatis.
- K8. Email bahasa Indonesia untuk kerangka (footer: "Dikirim oleh <nama> dari project <nama> di <APP_BASE_URL>"); isi mengikuti tulisan pengirim. `Reply-To` tidak diisi (hindari bocor alamat pengirim); bila perlu dikonfirmasi pemilik.
- K9. Nama pengirim ditampilkan di body, bukan di header `From`; `From` tetap `MAIL_FROM` dari T12.
- K10. Riwayat hanya dibaca owner dan super_admin; member biasa tidak melihat. Siaran ikut terhapus bila project dihapus (`ON DELETE CASCADE` pada `project_id`).
- K11. Alamat email penerima disimpan pada baris penerima (snapshot untuk riwayat dan kirim ulang), jadi tidak perlu lookup lagi saat kirim ulang. Isi pesan disimpan di tabel (bukan di audit/log).
- K12. Kirim ulang hanya untuk baris `failed`/`pending`, dibatasi 3 kali/jam per siaran, dan menolak bila siaran masih `sending`.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`. `project/notification` tidak meng-import `core/auth` atau `project/management`.
- Tabel baru milik module ada di `migrations/project/`. Tidak ada FK lintas module selain pola T03 (`project_id` dengan `ON DELETE CASCADE`).
- Mailer lewat `app.Deps.Mailer` (T12); tidak meng-import SMTP langsung.
- Isi pesan dan alamat penerima tidak pernah ditulis ke audit, log, atau event bus `Publish`.
- Kontrak API di `api/modules/project/notification.yml` dan `$ref` di `api/root.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Teks UI dalam bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Konfigurasi baru didokumentasikan di `docs/configuration.md` dan `.env.example`.
- Nama file/folder baru dan bentuk persis payload diserahkan ke pembangun.

## Features

### project / notification

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `sendbroadcast` | Validasi, rate limit, snapshot penerima, simpan, audit, lalu mulai pengiriman latar | `POST /api/projects/{projectID}/broadcasts` | owner, super_admin |
| `listbroadcasts` | Riwayat siaran project (subjek, pengirim, waktu, jumlah penerima/terkirim/gagal, status) | `GET /api/projects/{projectID}/broadcasts` | owner, super_admin |
| `getbroadcast` | Detail satu siaran + status per penerima | `GET /api/projects/{projectID}/broadcasts/{broadcastID}` | owner, super_admin |
| `resendfailed` | Kirim ulang ke penerima `failed`/`pending` | `POST /api/projects/{projectID}/broadcasts/{broadcastID}/resend-failed` | owner, super_admin |
| `recoverstale` | Job: tandai `pending` yang macet > 15 menit sebagai `failed` | job periodik | sistem |

### Tambahan handler bus (milik module lain, aditif)

| Endpoint | Provider | Kegunaan |
|---|---|---|
| `project.management.list-member-ids` (baru, baca-saja) | project/management | user id + role semua anggota sebuah project |
| `core.auth.get-contacts` (baru, baca-saja) | core/auth | batch `UserIDs` → `{UserID, Email, Name, Active}` hanya untuk akun aktif |

Nama final dan bentuk kontrak diatur pembangun; keduanya batch (tanpa N+1) dan selalu dibatasi input yang diberikan. `check-access` (T03) dipakai apa adanya untuk otorisasi.

## Aturan bisnis

- Pengirim wajib owner project atau super_admin dengan akun aktif; member 403, non-anggota 404 (pola T03).
- Subjek: dipangkas, 1–150 karakter, tanpa `\r`/`\n`/karakter kontrol. Isi: dipangkas, 1–5000 karakter; newline boleh.
- Penerima diambil dari anggota saat kirim; akun nonaktif atau yang tidak punya email valid dilewati dan dihitung di `skippedCount`.
- Bila setelah penyaringan tidak ada penerima → `422 NO_RECIPIENTS`.
- Status siaran: `sending` → `completed` (semua baris `sent`) atau `completed_with_errors` (ada `failed`). Status penerima: `pending`, `sent`, `failed`.
- Kegagalan satu penerima tidak menghentikan penerima lain; error per penerima disimpan ringkas (tanpa isi pesan/kredensial).
- Rate limit terlewati → `429`.

## Model data

Tambahan di `migrations/project/` (nama boleh disesuaikan pembangun):

```
project_broadcasts(
  id              uuid pk,
  project_id      uuid not null  fk→projects on delete cascade,
  sent_by         uuid not null,                       -- tanpa FK (pola T01 created_by)
  sent_by_label   text not null,
  subject         text not null,
  body            text not null,
  status          text not null,                        -- sending | completed | completed_with_errors
  recipient_count int not null,
  skipped_count   int not null default 0,
  created_at      timestamptz not null default now(),
  completed_at    timestamptz null
)
index on (project_id, created_at desc)

project_broadcast_recipients(
  broadcast_id uuid not null fk→project_broadcasts on delete cascade,
  user_id      uuid not null,
  email        text not null,
  status       text not null,                           -- pending | sent | failed
  error        text null,
  sent_at      timestamptz null,
  primary key (broadcast_id, user_id)
)
```

## Kontrak bus

Disediakan tiket ini: `project.management.list-member-ids` dan `core.auth.get-contacts` (lihat Features). Dikonsumsi: `project.management.check-access` (T03), `core.audit.record` (fail-open). Konstanta di `lib/constants/bus.go`, kontrak di `lib/contracts/...`, katalog di `docs/tickets/README.md` diperbarui. Kontrak tidak boleh membawa isi pesan.

## Konfigurasi

Env baru (nama final diatur pembangun, konsisten dengan `config.go`):

| Env | Keterangan | Default |
|---|---|---|
| `BROADCAST_MAX_RECIPIENTS` | batas penerima per siaran | 200 |
| `BROADCAST_MAX_BODY_CHARS` | batas panjang isi | 5000 |
| `BROADCAST_RATE_PER_HOUR` | siaran per jam per project | 5 |
| `BROADCAST_SEND_CONCURRENCY` | pengiriman paralel | 4 |
| `BROADCAST_SEND_TIMEOUT` | timeout per email | 10s |

Bergantung pada `MAIL_DRIVER`, `MAIL_FROM`, `APP_BASE_URL` dari T12.

## UI (Svelte)

- `domain/project/notification/...`: tab/section "Siaran" di halaman project (hanya owner/super_admin).
- Form "Kirim siaran": subjek, isi (textarea dengan penghitung karakter), keterangan "akan dikirim ke N anggota (termasuk Anda)", dialog konfirmasi sebelum kirim. Bila email belum dikonfigurasi, form dinonaktifkan dengan penjelasan (memakai `emailEnabled` dari T12).
- Daftar riwayat: subjek, pengirim, waktu, ringkasan terkirim/gagal, status; klik membuka detail.
- Detail: isi pesan, daftar penerima + status + error ringkas, tombol "Kirim ulang yang gagal" (aktif bila ada `failed`/`pending` dan siaran tidak `sending`), polling ringan saat `sending`.
- Semua teks lewat i18n `id` dan `en`.

## Titik sentuh

Backend: module baru `project/internal/notification`, `project/base.go` (wiring), `project/internal/management` (handler `list-member-ids`), `core/internal/auth` (handler `get-contacts`), `infra/config`, `infra/jobs`, `lib/constants/bus.go`, `lib/contracts`, `lib/contracts/core/audit/record/actions.go`, `migrations/project`.
Frontend: halaman project (T03) + domain baru `project/notification`, `shared/i18n`, rute.
Lainnya: `api/modules/project/notification.yml`, `api/root.yml`, `api/openapi.yml`, `docs/configuration.md`, `.env.example`.

## Aksi audit baru

Tambah di `lib/contracts/core/audit/record/actions.go` (dan `Actions()`), perbarui `actions_test.go`, dan katalog di README: `project.broadcast.sent` (target: broadcast id; hasil: jumlah penerima/lewati), `project.broadcast.resent`. Tanpa subjek, isi, atau alamat email.

## Risiko

- Penyalahgunaan sebagai kanal spam internal oleh owner. Mitigasi: rate limit, batas penerima, penerima terbatas anggota, riwayat + audit.
- Proses berhenti di tengah pengiriman (tanpa outbox). Mitigasi: status per baris, job `recoverstale`, kirim ulang manual.
- Deliverability (SPF/DKIM) di luar kendali aplikasi; email bisa masuk spam. Mitigasi: status kirim hanya berarti "diterima SMTP"; dokumentasikan.
- Isi pesan tersimpan di DB sebagai teks biasa. Mitigasi: hanya terbaca owner/super_admin; larang menaruh rahasia di peringatan UI.
- Konkurensi pengiriman membebani SMTP. Mitigasi: konkurensi dan timeout konfigurabel.

## Kriteria penerimaan

1. Owner mengirim siaran valid ke project beranggota 3 aktif: respons `202`, satu email terpisah per anggota (termasuk pengirim) dengan `To` hanya alamat penerima; riwayat berstatus `completed` dengan 3 baris `sent`.
2. Isi berisi `<script>` atau `<a href>`: HTML email menampilkannya sebagai teks (ter-escape); subjek berisi newline ditolak `422`.
3. Anggota nonaktif dilewati dan dihitung di `skippedCount`; tanpa penerima sama sekali → `422 NO_RECIPIENTS`; lebih dari batas → `422 TOO_MANY_RECIPIENTS`.
4. Member biasa → 403 di semua endpoint siaran; non-anggota → 404; super_admin berhasil di project mana pun.
5. Siaran keenam dalam satu jam di project yang sama → `429`; kirim ulang melebihi batas → `429`.
6. Mailer tidak terkonfigurasi → `503 EMAIL_NOT_CONFIGURED` dan tidak ada baris siaran terbuat.
7. SMTP menolak sebagian penerima: penerima lain tetap terkirim, siaran `completed_with_errors`, baris gagal berisi error ringkas; `resend-failed` hanya mengirim ke yang gagal dan memperbarui status; ditolak selagi `sending`.
8. Baris `pending` yang macet > 15 menit ditandai `failed` oleh job dan bisa dikirim ulang.
9. Audit mencatat `project.broadcast.sent`/`resent` tanpa subjek, isi, atau email; isi pesan tidak ada di log.
10. Menghapus project menghapus siaran dan penerimanya.
11. Dev lokal: dengan Mailpit (T12) email siaran terlihat di UI Mailpit.
12. Arch test, `make api-check`, lint, tes unit/integrasi/e2e hijau.

## Pertanyaan terbuka

Tidak ada. Semua keputusan K-pembangun masih menunggu persetujuan pemilik, terutama K2 (pengirim ikut menerima salinan; akun undangan T12 ikut menjadi penerima), K4 (tanpa kirim otomatis setelah crash), K6 (angka batas), dan K8 (tanpa `Reply-To`).
