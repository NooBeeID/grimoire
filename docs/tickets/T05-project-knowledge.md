# T05 — project / knowledge

| | |
|---|---|
| Domain / Module | project / knowledge |
| Ukuran | L |
| Bergantung pada | T03 |
| Dibutuhkan oleh | T08 |

## Ringkasan

Penyimpanan dan pengelolaan file knowledge (Markdown/teks) per project, dengan struktur folder dari path file. Developer mengunggah folder, membuat, dan mengedit file langsung di web. Termasuk pencarian full-text yang dipakai agent lewat T08.

## Tujuan

- Upload folder utuh (struktur path dipertahankan) dan file tunggal.
- Edit Markdown di web dengan preview, termasuk diagram Mermaid.
- Isi disimpan di Postgres; pencarian full-text keyword yang cepat.

## Non-goals

Riwayat versi/rollback, format selain `.md`/`.txt`, gambar/lampiran, semantic search, komentar, kolaborasi real-time, upload ZIP, sinkronisasi dari repo (belum), rename/move folder massal.

## User story

- Sebagai developer, saya drag-drop folder `docs/` ke project dan semua `.md` masuk dengan struktur yang sama.
- Sebagai developer, saya mengedit satu file di browser dan melihat preview Mermaid.
- Sebagai agent (lewat T08), saya membaca daftar file, isi file, dan mencari kata kunci.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `uploadfiles` | Upload banyak file dengan path relatif; timpa yang sama, sisanya dibiarkan; laporan per file | `POST /api/projects/{id}/knowledge/upload` (multipart) | anggota, super_admin |
| `createfile` | Buat file baru dari editor | `POST /api/projects/{id}/knowledge/files` | anggota, super_admin |
| `updatefile` | Simpan perubahan editor (dengan cek konflik) | `PUT /api/projects/{id}/knowledge/files?path=` | anggota, super_admin |
| `getfile` | Isi + metadata satu file | `GET /api/projects/{id}/knowledge/files?path=` | anggota, super_admin |
| `listtree` | Pohon folder/file (atau satu level per request) | `GET /api/projects/{id}/knowledge/tree` | anggota, super_admin |
| `deletefile` | Hapus satu file | `DELETE /api/projects/{id}/knowledge/files?path=` | anggota, super_admin |
| `deletefolder` | Hapus semua file di bawah prefix | `DELETE /api/projects/{id}/knowledge/folders?path=` | anggota, super_admin |
| `movefile` | Pindah/ubah nama satu file | `POST /api/projects/{id}/knowledge/files/move` | anggota, super_admin |
| `search` | Pencarian full-text di UI | `GET /api/projects/{id}/knowledge/search?q=` | anggota, super_admin |
| `busreader` | Handler bus untuk agent: `list-files`, `read-file`, `search` | bus `project.knowledge.*` | internal |

(`path` memakai query parameter agar karakter `/` tidak bermasalah di URL.)

## Aturan bisnis

### Path
- Dinormalisasi: pemisah `/`, tanpa awalan `/`, tanpa segmen kosong, `.`, atau `..`, tanpa karakter kontrol atau NUL, panjang ≤ 512, tiap segmen ≤ 255. Peka huruf besar/kecil. Unik per (project, path).
- Path yang tidak lolos ditolak dengan alasan jelas (tidak diam-diam diperbaiki).
- Path diawali titik (mis. `.git/`, `.DS_Store`) dilewati pada upload folder dan dilaporkan sebagai "dilewati".

### Tipe & batas
- Ekstensi diterima: `.md`, `.markdown`, `.txt`. File lain dilaporkan sebagai `unsupported`. (File `.yaml/.yml/.json` ditangani T06; lihat "Upload campuran".)
- Isi harus UTF-8 valid tanpa NUL. BOM dibuang.
- Default batas: 1 MB per file, 200 file per request, 20 MB per request (env, lihat T00).
- Batas jumlah file per project (default 5000) untuk mencegah penyalahgunaan.

### Upload
- File dengan path yang sama ditimpa; file lain di project dibiarkan.
- Hasil per file: `created`, `updated`, `unchanged` (hash isi sama, tidak menulis ulang), `skipped`, `rejected` + alasan. Sebagian gagal tidak membatalkan yang lain; respons merangkum jumlah.
- Penulisan tiap upload dibungkus transaksi per batch agar tidak setengah-setengah pada kegagalan DB.

### Edit & konflik
- `getfile` mengembalikan `version` (mis. `updated_at` atau hash). `updatefile` mengirim `version` yang dibaca; bila berbeda dengan yang tersimpan → 409 dengan isi terbaru, agar tidak menimpa kerja orang lain tanpa sadar. Upload folder menimpa tanpa cek (sesuai keputusan: overwrite).

### Pencarian
- Full-text Postgres (`tsvector` kolom generated + indeks GIN). Konfigurasi bahasa `simple` agar adil untuk teks campuran Indonesia/Inggris (builder boleh memperbaiki dengan alasan tertulis).
- Peringkat: kecocokan path/judul lebih berat dari isi. Hasil berisi path, skor, cuplikan dengan sorotan, maksimum 20 (maks 50).
- Kueri dibersihkan (tanpa sintaks mentah tsquery dari user) agar tidak error; mendukung frasa dalam tanda kutip.

### Keamanan tampilan
- Markdown dirender dengan sanitasi (tanpa HTML mentah/skrip, tautan `javascript:` ditolak). Mermaid dijalankan dengan `securityLevel: 'strict'`. Isi knowledge tidak pernah dianggap tepercaya.

## Upload campuran (folder berisi `.md` dan `.yaml`)

Backend T05 hanya menerima Markdown/teks. Tampilan "Upload folder" di UI memilah berkas berdasarkan ekstensi: `.md/.markdown/.txt` ke T05, `.yaml/.yml/.json` ke T06, sisanya ditampilkan sebagai tidak didukung. Dua panggilan dipanggil dari lapisan komposisi `app/` (bukan dari dalam module), lalu hasilnya digabung dalam satu laporan. Backend tetap otoritatif atas penolakan.

## Model data

```
knowledge_files(id uuid pk, project_id fk→projects on delete cascade,
  path text, content text, content_hash bytea, size_bytes int,
  created_at, created_by uuid, created_by_label text,
  updated_at, updated_by uuid, updated_by_label text,
  search tsvector generated,
  unique(project_id, path))
index: gin(search); (project_id, path text_pattern_ops)  -- untuk prefix folder
```

## Kontrak bus

Disediakan (`lib/contracts/project/knowledge/...`) — semuanya sudah dibatasi pada `ProjectID` yang diberikan pemanggil; module ini tidak tahu soal key:
- `project.knowledge.list-files` — `{ProjectID, Prefix, Recursive, Limit, Offset}` → `{Items[{Path, SizeBytes, UpdatedAt}], Total, Truncated}`.
- `project.knowledge.read-file` — `{ProjectID, Path, Offset, Limit}` → `{Found, Path, Content, SizeBytes, TotalBytes, NextOffset, Truncated, Similar[]}` (`Similar` berisi path mirip bila tidak ketemu).
- `project.knowledge.search` — `{ProjectID, Query, Prefix, Limit}` → `{Items[{Path, Score, Snippet}], Total}`.

Dikonsumsi: `project.management.check-access` (otorisasi HTTP), `core.auth.get-user` (label), `core.audit.record`.

Event audit: `knowledge.file.created`, `knowledge.file.updated`, `knowledge.file.deleted`, `knowledge.folder.deleted` (jumlah file), `knowledge.upload.completed` (satu entri per upload: jumlah created/updated/unchanged/rejected dan maksimal N path pertama, bukan satu entri per file).

## UI (Svelte, `domain/project/knowledge/...`)

Tab "Knowledge":
- Panel kiri: pohon folder/file (lazy), pencarian cepat; panel kanan: viewer Markdown dengan Mermaid, tombol Edit.
- Editor (CodeMirror atau setara) dengan preview berdampingan, indikator belum disimpan, penanganan 409 (tawarkan muat ulang/bandingkan).
- Tombol "Upload": pilih folder atau drag-drop (`webkitdirectory`), pilih file; layar ringkasan pra-upload (jumlah file, yang akan dilewati), progres, laporan akhir per file dengan filter (gagal/ditimpa).
- Aksi hapus file/folder dengan konfirmasi; ganti nama/pindah file.
- Pencarian proyek: kotak cari, hasil dengan cuplikan tersorot.
- Breadcrumb dan salin path (berguna untuk dirujuk ke agent).

## Kriteria penerimaan

1. Upload folder berisi 3 tingkat subfolder mempertahankan path; upload ulang folder yang sama melaporkan `unchanged`; mengubah satu file melaporkan `updated`, sisanya dibiarkan.
2. Upload berisi `../x.md`, path 600 karakter, file 2 MB, file biner `.md`: masing-masing ditolak dengan alasan yang benar; file valid lain di request yang sama tetap masuk.
3. Dua developer mengedit file yang sama; yang menyimpan kedua mendapat 409 dan tidak menimpa.
4. Pencarian "login" menemukan file yang memuat kata itu, menaruh file `login.md` di atas file yang hanya menyebutnya sekilas; kueri dengan karakter aneh (`"`, `&`, `:`) tidak menyebabkan error 500.
5. Markdown berisi `<script>` dan tautan `javascript:` ditampilkan tanpa dieksekusi; blok ```mermaid dirender.
6. Non-anggota mendapat 404 pada semua endpoint; hapus folder menghapus seluruh file di bawah prefix dan hanya itu.
7. Satu upload 100 file menghasilkan satu entri audit ringkasan.
8. Handler bus tidak pernah mengembalikan data project lain, dan `read-file` dengan `Offset/Limit` memotong dengan benar pada batas karakter UTF-8.
9. Kinerja: pencarian pada 5000 file ≤ 200 ms (p95) di mesin dev.

## Pertanyaan terbuka

Tidak ada. Batas ukuran/jumlah dan konfigurasi bahasa pencarian adalah default saya.
