# T13 — project / api-key (kepemilikan key per user per project)

| | |
|---|---|
| Domain / Module | project / api-key |
| Ukuran | M |
| Bergantung pada | T03, T04 |
| Sumber | laporan pemilik (isu kebocoran visibilitas key antar anggota project) |

## Ringkasan

Di T04, API key adalah milik **project**: semua anggota project melihat semua key, dan anggota mana pun bisa mencabut key anggota lain. Itu salah. API key harus milik **user di dalam project**: key buatan user X tidak boleh terlihat, terhitung, atau bisa dicabut oleh user Y, walau keduanya anggota project yang sama.

Akar masalah (terverifikasi di kode):

- `listkeys/repository.go`: `WHERE project_id = $1` tanpa filter pemilik.
- `revokekey/repository.go`: `WHERE id = $1 AND project_id = $2` tanpa filter pemilik.
- `access.Require` hanya memeriksa keanggotaan project, bukan kepemilikan key.
- Kolom `created_by` sudah ada dan terisi setiap `createkey`, tetapi tidak dipakai untuk scoping.
- Batas jumlah key aktif (`APIKeyMaxActivePerProject`) dan keunikan nama (`api_keys_project_active_name_key`) dihitung per project, sehingga satu user bisa menghabiskan kuota semua orang dan error `NAME_TAKEN` membocorkan nama key milik user lain.

`verifykey` (bus, dipakai MCP gateway T07) mencari lewat prefix dan tidak terpengaruh: key tetap memberi akses ke project-nya.

## Tujuan

1. `listkeys` hanya mengembalikan key milik pemanggil di project itu.
2. `revokekey` hanya bisa mencabut key milik pemanggil. Key milik orang lain diperlakukan seperti tidak ada (404), tanpa membocorkan eksistensinya.
3. Batas jumlah key aktif dan keunikan nama berlaku **per user per project**.
4. Super Admin tetap bisa melihat dan mencabut semua key di project mana pun.
5. Perilaku `createkey`, `verifykey`, `touchlastused` dari sisi pemanggil tidak berubah.

## Non-goals

- Memindahkan kepemilikan key antar user, atau key bersama tim.
- Mengubah format key, hashing, atau alur verifikasi MCP.
- Mengubah siapa yang boleh membuka tab API Keys (anggota project dan Super Admin, seperti T04).
- Membatasi tampilan log audit (T02). Lihat Risiko.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Key user lain muncul di daftar, jumlah, respons error, atau pesan validasi untuk user biasa.
- User biasa bisa membedakan "key tidak ada" dari "key milik orang lain" (harus sama-sama 404).
- Super Admin kehilangan kemampuan melihat/mencabut key.
- Migrasi menggagalkan data lama atau membuat dua key aktif lama dengan nama sama jadi konflik.

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Visibilitas key antar anggota | Tidak terlihat. Key milik user X tidak terlihat oleh user Y di project yang sama. |
| Super Admin | **Tetap bisa melihat dan mencabut** key semua user di project. |
| Batas jumlah dan unik nama | **Per user per project**. Dua user boleh punya key bernama sama. |

## Keputusan pembangun (mohon dicek)

- K1. Pemilik key = `api_keys.created_by`. Tidak ada kolom baru.
- K2. Filter pemilik diterapkan di **SQL** (repository), bukan menyaring di memori. Usecase menentukan scope dari `Actor`: Super Admin = tanpa filter, selain itu = `created_by = actor.ID`. Logika "siapa boleh lihat apa" ada di satu fungsi `base` (mis. `Actor.OwnerScope()`), bukan tersebar.
- K3. Key milik orang lain pada `revokekey` → `ErrKeyNotFound` (404), sama dengan `keyID` yang tidak ada. Mencabut key yang sudah dicabut milik sendiri tetap `ErrAlreadyRevoked`.
- K4. Key legacy dengan `created_by IS NULL` hanya terlihat dan bisa dicabut Super Admin. Dalam praktik tidak ada, karena `NewKey` selalu mengisi `created_by`.
- K5. Advisory lock di `createkey` ikut dipersempit ke `(project, user)` agar dua user tidak saling menunggu. Penghitungan kuota memakai `created_by = actor.ID`. Key yang dibuat Super Admin dihitung ke kuota Super Admin sendiri.
- K6. Migrasi: `DROP INDEX api_keys_project_active_name_key` lalu buat ulang dengan **nama yang sama** pada `(project_id, created_by, name) WHERE revoked_at IS NULL`, sehingga `store.ConstraintActiveName` dan pemetaan `ErrNameTaken` tidak berubah. Migrasi baru, bukan edit migrasi T04. Memperlonggar unique tidak bisa gagal pada data lama. `created_by` NULL dianggap berbeda satu sama lain oleh Postgres, dan itu dapat diterima (K4).
- K7. `countkeys` (kontrak `contentcount` milik T03) **tidak diubah** di tiket ini: kontrak tidak membawa identitas user dan tetap menghitung seluruh key aktif project. Konsekuensinya angka di kartu project bisa lebih besar dari jumlah key yang dilihat user biasa. Lihat Pertanyaan terbuka.
- K8. UI tidak perlu perubahan fungsional untuk user biasa (daftar otomatis hanya berisi miliknya). Untuk Super Admin, kolom "dibuat oleh" (`createdByLabel`, sudah ada di DTO) ditampilkan di tabel agar jelas key itu milik siapa.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`.
- Tidak ada perubahan bentuk respons REST; hanya isi yang tersaring. Pembaruan deskripsi di `api/modules/project/api-key.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Migrasi baru di `backend/migrations/project/`.
- Teks UI bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json` bila ada teks baru.
- Rahasia key tidak pernah dibaca pada jalur daftar (tetap `listColumns` tanpa `secret_hash`).

## Features

| Folder | Perubahan | Titik masuk | Siapa |
|---|---|---|---|
| `base` | `Actor.OwnerScope()` (Super Admin → kosong, selain itu → `Actor.ID`) | – | – |
| `listkeys` | Repository menerima `ownerID`; `WHERE project_id=$1 AND ($2='' OR created_by::text=$2)` | `GET /api/projects/{projectID}/api-keys` | anggota (miliknya), super_admin (semua) |
| `revokekey` | `LockInProject` menerima `ownerID`; baris tidak cocok → `ErrKeyNotFound` | `POST /api/projects/{projectID}/api-keys/{keyID}/revoke` | anggota (miliknya), super_admin (semua) |
| `createkey` | Advisory lock dan `CountActive` per `(project, actor)`; unik nama per user | `POST /api/projects/{projectID}/api-keys` | anggota, super_admin |
| `verifykey`, `touchlastused`, `countkeys` | Tidak berubah | bus | internal |

## Aturan bisnis

- User biasa: melihat dan mencabut hanya key dengan `created_by = id sendiri`.
- Super Admin: melihat dan mencabut semua key di project mana pun. Audit `api_key.revoked` mencatat aktor sebenarnya (Super Admin), bukan pemilik.
- Batas `APIKeyMaxActivePerProject` kini dibaca sebagai batas per user per project. Nama config **tidak diganti** di tiket ini agar `.env`/dokumen operator tidak patah; deskripsinya diperbarui di `docs/configuration.md` dan `.env.example`.
- Nama key: 2–60 karakter, unik di antara key aktif milik user yang sama di project itu.
- Non-anggota tetap 404 (pemeriksaan akses T04 tidak berubah).

## Model data

Satu migrasi, tanpa tabel atau kolom baru:

```
DROP INDEX IF EXISTS api_keys_project_active_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_project_active_name_key
  ON api_keys (project_id, created_by, name) WHERE revoked_at IS NULL;
-- opsional untuk query daftar per pemilik:
CREATE INDEX IF NOT EXISTS api_keys_owner_idx ON api_keys (project_id, created_by, created_at DESC);
```

## Kontrak bus

Tidak ada perubahan. `project.apikey.verify`, `project.apikey.touch`, `project.apikey.count-keys` tetap sama (lihat K7 untuk `count-keys`).

## UI (Svelte, `domain/project/api-key/...`)

- `ListKeysView`/`KeysTable`: tidak ada perubahan untuk user biasa.
- Super Admin: tampilkan kolom "Dibuat oleh" (dari `createdByLabel`).
- Teks kosong dan penjelasan tab diperjelas: "Key di sini hanya milik Anda."
- Dialog buat key: pesan `NAME_TAKEN` dan `KEY_LIMIT_REACHED` tetap, kini merujuk key milik sendiri.

## Titik sentuh

Backend: `project/internal/apikey/{base,listkeys,revokekey,createkey}`, `migrations/project`, `docs/configuration.md`, `.env.example`.
Frontend: `domain/project/api-key/listkeys/components/KeysTable.svelte`, teks i18n.
Lainnya: `api/modules/project/api-key.yml`, `api/openapi.yml`, `docs/tickets/T04-project-api-key.md` (catatan revisi).

## Aksi audit baru

Tidak ada. Aksi `api_key.created` dan `api_key.revoked` tetap.

## Catatan revisi

- T04: aturan "semua anggota project melihat dan mencabut semua key" dan batas/unik nama per project digantikan tiket ini. Beri catatan revisi di T04 (tidak ditulis ulang).

## Risiko

- **Log audit**: event `api_key.created`/`api_key.revoked` memuat nama dan prefix key. Bila halaman audit (T02) bisa dibuka non-Super Admin, nama key user lain tetap bocor lewat sana. Periksa saat mengerjakan; di T02 audit-log setahu kami khusus Super Admin, tetapi harus diverifikasi, bukan diasumsikan.
- Angka `countkeys` di kartu project tidak sama dengan yang dilihat user biasa (K7).
- Perubahan batas jadi per user menaikkan total key aktif per project secara teoretis (jumlah anggota × batas). Dapat diterima untuk skala saat ini.
- Menyentuh tiga jalur tulis/baca key; butuh tes regresi untuk jalur MCP (`verifykey`).

## Kriteria penerimaan

1. Dua anggota A dan B di project yang sama, masing-masing membuat key. `GET .../api-keys` oleh A hanya berisi key A; oleh B hanya key B.
2. B memanggil revoke dengan `keyID` milik A: `404`, key A tetap aktif, tidak ada audit `api_key.revoked`. Respons identik dengan `keyID` yang tidak ada.
3. A mencabut key A: sukses; mencabut lagi → `ErrAlreadyRevoked` (409) seperti T04.
4. A dan B boleh membuat key aktif dengan nama sama di project yang sama. A membuat dua key aktif bernama sama → `NAME_TAKEN`. Nama key yang sudah dicabut bisa dipakai ulang oleh pemiliknya.
5. Batas: A mencapai limit → `KEY_LIMIT_REACHED` untuk A, sedangkan B masih bisa membuat key. Pembuatan serentak A (limit 1) tepat satu sukses; A dan B serentak keduanya sukses.
6. Super Admin melihat key A dan B di project itu, bisa mencabut keduanya; audit mencatat Super Admin sebagai aktor. Kunci yang Super Admin buat dihitung ke kuotanya sendiri.
7. Non-anggota project: `404` di ketiga endpoint (tidak berubah).
8. `verifykey` untuk key A dan B tetap berhasil sama seperti sebelumnya; tes jalur MCP T07 tetap hijau.
9. Migrasi berjalan di DB berisi data T04 tanpa error; `ConstraintActiveName` tetap memetakan ke `ErrNameTaken`.
10. Respons daftar tidak pernah memuat `secret_hash`/plaintext (tes T04 tetap lolos).
11. Tes unit (usecase dengan fake repo: scope owner, scope Super Admin), integrasi (isolasi dua user, limit, unik nama, revoke lintas user), handler, e2e `project-api-key.spec.ts` (dua user, tab hanya menampilkan milik sendiri), `make api-check`, lint, dan arch test hijau.

## Pertanyaan terbuka

1. **`countkeys` (K7)**: biarkan jumlah total project (default tiket ini), atau ubah kontrak `contentcount` agar membawa user dan menghitung key milik user itu saja? Opsi kedua menyentuh kontrak T03 dan konsumennya.
2. Apakah halaman audit log memang hanya untuk Super Admin? Bila tidak, nama key di audit perlu disaring juga (di luar tiket ini).
