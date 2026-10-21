# T04 — project / api-key

| | |
|---|---|
| Domain / Module | project / api-key |
| Ukuran | M |
| Bergantung pada | T03 |
| Dibutuhkan oleh | T07 |

## Ringkasan

Kredensial untuk agent. Satu key = satu project, read-only secara desain (key hanya bisa dipakai lewat MCP). Anggota project (bukan hanya owner) bisa membuat dan mencabut key untuk project itu.

## Tujuan

- Developer membuat key bernama untuk satu project dan menyalinnya sekali.
- Key bisa dicabut kapan saja dengan efek langsung.
- Gateway MCP dapat memverifikasi key dengan cepat dan aman.

## Non-goals

Akun user untuk agent, key lintas project, scope per folder/tool, kedaluwarsa otomatis (lihat pertanyaan terbuka), rotasi otomatis, penggunaan key untuk login web atau REST `/api`.

## User story

- Sebagai developer, saya membuat key "claude-code-budi" untuk project X dan mendapat potongan konfigurasi MCP siap tempel.
- Sebagai developer, saya mencabut key yang bocor dan agent langsung ditolak.
- Sebagai developer, saya melihat kapan sebuah key terakhir dipakai agar tahu key mana yang sudah tidak dipakai.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `createkey` | Buat key; plaintext dikembalikan **sekali** | `POST /api/projects/{id}/api-keys` | anggota, super_admin |
| `listkeys` | Daftar key project (tanpa secret) | `GET /api/projects/{id}/api-keys` | anggota, super_admin |
| `revokekey` | Cabut key (tidak bisa diaktifkan lagi) | `POST /api/projects/{id}/api-keys/{keyId}/revoke` | anggota, super_admin |
| `verifykey` | Handler bus: verifikasi key → identitas & project | bus `project.apikey.verify` | internal |
| `touchlastused` | Handler bus: perbarui `last_used_at` (dibatasi frekuensinya) | bus `project.apikey.touch` (Publish) | internal |

## Aturan bisnis

- Format key: `gr_<prefix>_<secret>`; `prefix` 8 karakter acak (tampil di UI/log, unik global), `secret` ≥ 32 byte acak dari CSPRNG (base64url).
- Yang disimpan hanya `SHA-256(secret)` (secret entropi tinggi, jadi hash cepat memadai) beserta prefix. Verifikasi memakai perbandingan waktu-konstan.
- Plaintext hanya ada di respons `createkey`. Tidak ada cara menampilkannya lagi.
- Nama wajib 2–60 karakter, unik per project (hanya antar key yang belum dicabut).
- Batas jumlah key aktif per project (default 20, konfigurabel).
- Key yang dicabut tetap terlihat di daftar (status "dicabut", dicabut oleh siapa dan kapan) demi jejak; tidak ada tombol hapus. Baris hilang hanya saat project dihapus.
- `verifykey` mengembalikan `Found=false` untuk key tak dikenal/dicabut/salah format; **tidak membedakan alasan** ke pemanggil luar (alasan hanya dicatat ke audit oleh T07, termasuk prefix bila ada).
- Key tidak terikat pada pembuat: bila pembuat dinonaktifkan atau dikeluarkan dari project, key tetap hidup sampai dicabut anggota project (keputusan desain).
- `last_used_at` diperbarui paling sering sekali per menit per key agar tidak membebani DB.
- Verifikasi memakai cache pendek di memori (mis. 30 detik) **tidak** diizinkan agar pencabutan langsung berlaku. Verifikasi cukup satu query berindeks pada prefix.

## Model data

```
api_keys(id uuid pk, project_id fk→projects on delete cascade,
         name text, prefix text unique, secret_hash bytea,
         created_by uuid, created_by_label text, created_at,
         last_used_at timestamptz null,
         revoked_at timestamptz null, revoked_by uuid null, revoked_by_label text null)
unique(project_id, name) where revoked_at is null
```

## Kontrak bus

Disediakan (`lib/contracts/project/apikey/...`):
- `project.apikey.count-keys` Call — kontrak `lib/contracts/project/management/contentcount` (`{ProjectIDs}` → `{Counts map[string]int}`): jumlah key aktif (belum dicabut) per project untuk T03. Hanya menghitung `ProjectIDs` yang diberikan; maksimal `contentcount.MaxProjectIDs`.
- Semua tabel dengan `project_id` WAJIB `REFERENCES projects(id) ON DELETE CASCADE`; route memakai param `{projectID}`.
- `project.apikey.verify` Call — `{RawKey}` → `{Found, KeyID, KeyName, Prefix, ProjectID}`.
- `project.apikey.touch` Publish — `{KeyID}` → –.

Dikonsumsi: `project.management.check-access` (otorisasi endpoint HTTP), `core.auth.get-user` (label pembuat), `core.audit.record`.

Event audit: `apikey.created` (nama, prefix), `apikey.revoked`.

## UI (Svelte, `domain/project/apikey/...`)

Tab "API Keys" di workspace project:
- Tabel: nama, prefix, dibuat oleh/kapan, terakhir dipakai, status.
- Dialog buat key: input nama → layar hasil berisi key lengkap dengan tombol salin dan peringatan "tidak akan ditampilkan lagi".
- **Potongan konfigurasi siap pakai** di layar hasil, minimal untuk Claude Code (`claude mcp add --transport http <nama> <URL>/mcp --header "Authorization: Bearer <key>"`) dan JSON generik untuk klien MCP lain. URL server diambil dari konfigurasi (`PUBLIC_URL`) atau dari origin browser.
- Aksi cabut dengan konfirmasi; key dicabut ditampilkan redup.
- Tautan ke Audit Log terfilter untuk key itu hanya tampil bagi Super Admin.

## Kriteria penerimaan

1. Member (bukan owner) berhasil membuat key; user yang bukan anggota mendapat 404; Super Admin berhasil.
2. Respons `createkey` memuat key lengkap; `listkeys` dan basis data tidak pernah memuatnya (tes memeriksa tabel).
3. Key baru langsung valid lewat `verify`; setelah dicabut, `verify` langsung `Found=false` pada panggilan berikutnya.
4. Nama kembar pada key aktif ditolak; nama dari key yang sudah dicabut boleh dipakai lagi.
5. Batas key aktif per project ditegakkan.
6. Keluarnya pembuat dari project tidak mematikan key.
7. `touch` berkali-kali dalam semenit hanya menulis sekali.
8. Key dengan prefix valid tetapi secret salah → `Found=false`, dan durasi respons setara dengan key tak dikenal (tidak ada timing leak yang berarti).
9. Entri audit tercipta untuk create dan revoke.

## Pertanyaan terbuka

1. **Kedaluwarsa opsional** pada key (mis. 30/90 hari/tanpa batas)? Default saya: tidak ada di versi pertama; menambahkannya nanti berupa satu kolom `expires_at` dan satu cek di `verify`.
