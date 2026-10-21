# T03 — project / management

| | |
|---|---|
| Domain / Module | project / management |
| Ukuran | M |
| Bergantung pada | T01, T02 |
| Dibutuhkan oleh | T04, T05, T06, T07 |

## Ringkasan

Project sebagai unit utama, plus keanggotaan developer di dalamnya. Module ini juga sumber kebenaran "siapa boleh apa di project ini" yang dipakai module project lainnya lewat bus.

## Tujuan

- Developer membuat project (otomatis owner), mengundang developer lain, dan hanya melihat project tempat ia menjadi anggota.
- Super Admin melihat dan mengelola semua project.
- Satu tempat memutuskan level akses, sehingga module lain tidak mengulang logikanya.

## Non-goals

Template project, arsip/soft delete, transfer antar organisasi, grup/tim, undangan lewat email, izin per folder.

## Hak akses (keputusan di desain)

| Aksi | Owner | Member | Super Admin |
|---|---|---|---|
| Lihat project, knowledge, spec, daftar key | ✔ | ✔ | ✔ |
| Kelola isi (knowledge, spec) dan buat/cabut key | ✔ | ✔ | ✔ |
| Ubah nama/deskripsi project | ✔ | ✔ | ✔ |
| Kelola anggota (tambah, keluarkan, ubah peran) | ✔ | ✘ | ✔ |
| Hapus project | ✔ | ✘ | ✔ |
| Keluar dari project sendiri | ✔ (kecuali owner terakhir) | ✔ | – |

Satu project boleh punya lebih dari satu owner (keputusan saya).

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `createproject` | Buat project; developer pembuat jadi owner, Super Admin tidak dijadikan anggota (project lahir tanpa owner) | `POST /api/projects` | developer, super_admin |
| `listprojects` | Developer: project yang ia ikuti. Super Admin: semua (+ filter/pencarian) | `GET /api/projects` | keduanya |
| `getproject` | Detail + peran saya + ringkasan jumlah (file, spec, key) | `GET /api/projects/{id}` | anggota, super_admin |
| `updateproject` | Ubah nama/deskripsi | `PATCH /api/projects/{id}` | anggota, super_admin |
| `deleteproject` | Hapus permanen beserta isinya | `DELETE /api/projects/{id}` | owner, super_admin |
| `listmembers` | Daftar anggota dengan peran | `GET /api/projects/{id}/members` | anggota, super_admin |
| `addmember` | Tambah developer aktif; `role` opsional (default `member`, `owner` untuk menunjuk owner) | `POST /api/projects/{id}/members` | owner, super_admin |
| `removemember` | Keluarkan anggota / keluar sendiri | `DELETE /api/projects/{id}/members/{userId}` | owner, super_admin, diri sendiri |
| `changememberrole` | Owner ↔ member | `PATCH /api/projects/{id}/members/{userId}` | owner, super_admin |
| `searchmembercandidates` | Cari developer aktif yang belum menjadi anggota (autocomplete tambah anggota) | `GET /api/projects/{projectID}/member-candidates` | owner, super_admin |
| `checkaccess` | Handler bus: tentukan level akses user pada project | bus `project.management.check-access` | internal |
| `getprojectbus` | Handler bus: id → ringkasan project | bus `project.management.get-project` | internal |

## Aturan bisnis

- Nama project wajib 2–100 karakter; tidak harus unik (project dikenali lewat id). Deskripsi opsional ≤ 1000 karakter.
- Hanya developer **aktif** yang bisa ditambahkan. Super Admin tidak perlu menjadi anggota untuk mengakses project; Super Admin juga tidak ditambahkan sebagai anggota lewat fitur ini.
- Pemilik terakhir tidak bisa keluar atau diturunkan oleh dirinya sendiri; ia harus lebih dulu mengangkat owner lain. Super Admin boleh melakukannya (project boleh tanpa owner; Super Admin dapat menunjuk owner baru lewat `changememberrole`/`addmember`).
- Developer yang dinonaktifkan: keanggotaannya tetap tercatat tetapi `checkaccess` menganggapnya tanpa akses (memeriksa `core.auth.get-user`).
- **Hapus project** meminta konfirmasi dengan mengetik nama project (UI) dan menyertakan nama itu pada request (API) agar tidak terhapus tak sengaja. Penghapusan menghapus: anggota, knowledge, API spec, API key (lewat `ON DELETE CASCADE` pada tabel milik T04–T06 yang mereferensikan `projects.id`). Entri audit tetap ada (snapshot).
- Daftar project untuk developer tidak boleh membocorkan keberadaan project lain (respons 404, bukan 403, untuk project yang bukan miliknya).

## Model data

```
projects(id uuid pk, name text, description text, created_by uuid, created_at, updated_at)
project_members(project_id fk→projects on delete cascade, user_id uuid,
                role text check in ('owner','member'), added_by uuid, added_at,
                pk(project_id, user_id))
```

`user_id` tidak diberi FK ke `users` (module berbeda); konsistensi dijaga lewat bus.
Migrasi di `migrations/project/`. Tabel module lain (T04–T06) mereferensikan `projects(id)` dengan `ON DELETE CASCADE`. Itu satu-satunya kopling tingkat DB yang diizinkan.

## Kontrak bus

Disediakan (`lib/contracts/project/management/...`):
- `project.management.check-access` Call — `{ProjectID, UserID, UserRole}` → `{ProjectExists, Level}` dengan `Level ∈ none | member | owner | admin`. `admin` untuk `UserRole == super_admin`.
- `project.management.get-project` Call — `{ProjectID}` → `{Found, ID, Name}`.

Dikonsumsi: `core.auth.get-user`, `core.auth.find-developers` (T01), `core.audit.record` (T02, Call setelah commit, fail-open), dan hitungan isi `project.knowledge.count-files` (T05), `project.apispec.count-specs` (T06), `project.apikey.count-keys` (T04) lewat `lib/contracts/project/management/contentcount` (fail-soft; `null` sampai provider ada).

Event audit: `project.created`, `project.updated`, `project.deleted` (nama, jumlah file/spec/key saat dihapus), `project.member.added`, `project.member.removed`, `project.member.role_changed`.

## UI (Svelte, `domain/project/management/...`)

- Beranda: daftar project sebagai kartu/tabel (nama, peran saya, jumlah file/spec, update terakhir), pencarian, tombol "Project baru".
- Layout workspace project dengan tab: **Knowledge** (T05), **API Specs** (T06), **API Keys** (T04), **Members**, **Settings**. Tab diisi oleh view module lain; layout hanya menyediakan slot (komposisi di `app/`).
- Members: tabel + dialog tambah (autocomplete developer), ubah peran, keluarkan; tombol bagi member hanya "Keluar".
- Settings: ubah nama/deskripsi; zona bahaya "Hapus project" dengan konfirmasi mengetik nama.
- Super Admin melihat semua project dengan penanda "bukan anggota".

## Kriteria penerimaan

1. Developer A membuat project → A tercatat owner; developer B tidak melihatnya di daftar dan `GET` langsung mengembalikan 404.
2. A menambahkan B → B melihat project, bisa ubah nama, tidak bisa tambah/keluarkan anggota (403), tidak bisa hapus project.
3. Owner terakhir tidak bisa keluar/diturunkan sendiri; Super Admin bisa.
4. Menambahkan user nonaktif atau Super Admin sebagai anggota ditolak dengan pesan jelas.
5. Menonaktifkan B membuat `check-access` untuk B bernilai `none`.
6. Menghapus project tanpa nama konfirmasi yang cocok ditolak; yang benar menghapus semua data turunan (diverifikasi lewat tes integrasi bersama tabel T04–T06) dan menulis entri audit.
7. Super Admin melihat semua project dan bisa mengelola anggotanya.
8. `check-access` untuk project yang tidak ada mengembalikan `ProjectExists=false`, bukan error.

## Pertanyaan terbuka

Tidak ada. Catatan: "project boleh punya banyak owner" dan "project tanpa owner diperbolehkan" adalah keputusan saya.

## Catatan revisi (T12)

Non-goal "undangan lewat email" di atas digantikan T12 (`/api/projects/{projectID}/invitations`). `addmember` untuk developer yang sudah punya akun tidak berubah. Respons anggota dan kandidat mendapat field aditif `invitation_pending`/`invitation_expires_at`. Isi tiket ini tidak ditulis ulang.
