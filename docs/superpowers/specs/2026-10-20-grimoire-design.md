# NooBee Grimoire — Design

Tanggal: 2026-10-20
Status: draft untuk review

## Intent dan alasan

Tim internal kecil butuh satu tempat terpusat untuk menyimpan knowledge (dokumentasi Markdown) dan API spec (OpenAPI YAML) per project, lalu membuat knowledge itu bisa dibaca agent Claude lewat MCP. Manusia (Super Admin, Developer) mengelola isinya lewat web UI. Agent hanya membaca, memakai API key yang terikat ke satu project.

Nilainya: Claude (Claude Code/Desktop) bisa menjawab dan menulis kode dengan konteks project yang benar tanpa knowledge disalin manual ke tiap sesi.

## Goals

- Web UI untuk login, manajemen user (oleh Super Admin), project, knowledge, dan API spec.
- MCP server (HTTP transport) untuk agent membaca knowledge dan API spec.
- Satu binary Go yang menyajikan API, MCP, dan static build Svelte. Satu Postgres sebagai satu-satunya penyimpanan.
- Dapat dijalankan self-hosted oleh tim kecil dengan beberapa komponen saja.

## Non-goals

- Semantic search / RAG (hanya full-text keyword).
- SSO / OAuth login (email + password saja).
- Riwayat versi dan rollback (upload ulang menimpa).
- Multi-tenant, billing, atau penggunaan publik/SaaS.
- File selain Markdown (teks) dan OpenAPI YAML/JSON. Tidak ada PDF, Office, atau gambar. Diagram memakai Mermaid di dalam Markdown.
- Object storage (S3/MinIO/RustFS). Ditunda; lihat "Dikerjakan builder".

## Anti-goals

- Sistem yang rumit dioperasikan: jika butuh lebih dari binary Go + Postgres untuk jalan, itu gagal.
- Agent bisa membaca project yang bukan miliknya, atau menulis/mengubah apa pun.

## Constraints

- Backend Go. Frontend Svelte, di-build menjadi static HTML/JS/CSS, lalu disajikan oleh binary Go (embed ke binary).
- Penyimpanan: Postgres saja, isi file disimpan sebagai teks.
- Skala: tim internal, puluhan user, ratusan sampai ribuan file teks.

## Role dan akses

| Role | Kemampuan |
|---|---|
| Super Admin | Membuat/menonaktifkan user, melihat semua project dan semua knowledge. |
| Developer | Membuat project (otomatis jadi owner), mengundang developer lain ke project, upload/edit/hapus knowledge dan API spec, membuat/mencabut API key. Hanya melihat project tempat dia menjadi anggota. |
| Agentic | Bukan akun user. Hanya API key milik satu project, read-only, dipakai lewat MCP. |

Keputusan pengguna:
- Satu API key = satu project.
- Developer mana pun yang menjadi anggota project boleh membuat API key untuk project itu (tidak harus owner).
- Pembuat project otomatis owner dan bisa mengundang developer lain.
- Super Admin pertama dibuat lewat perintah CLI (mis. `./grimoire create-admin`).

Hak akses (keputusan pengguna):
- Owner: menghapus project dan mengelola anggota (undang, keluarkan).
- Member (developer): mengelola isi (knowledge, API spec) dan membuat/mencabut API key.
- Super Admin: dapat melakukan semua hal di semua project, termasuk edit, hapus, dan atur anggota (berguna jika owner pergi).
- Penghapusan project hanya oleh owner atau Super Admin. Project tanpa owner boleh ada; Super Admin dapat menunjuk owner baru.

Autentikasi user (keputusan pengguna):
- Super Admin membuat user dan menetapkan password awal. User wajib mengganti password saat login pertama.
- Reset password dilakukan Super Admin. Tidak ada email/SMTP.
- User login ke web dengan email + password.

## Model data (konseptual)

- `users`: email, password hash, role (super_admin | developer), status aktif.
- `projects`: nama, deskripsi, pembuat.
- `project_members`: user, project, peran di project (owner | member).
- `files`: project, path (mis. `docs/auth/login.md`), isi teks, tipe (knowledge | api_spec), updated_at, updated_by. Unik per (project, path).
- Satu project boleh punya banyak API spec (mis. dipisah per module/domain). Tiap spec adalah satu file bertipe api_spec dan punya nama (default dari nama file, bisa diubah).
- `audit_log`: lihat bagian "Audit log".
- `api_keys`: project, nama label, hash key (key asli hanya ditampilkan sekali saat dibuat), prefix untuk identifikasi, dibuat oleh, last_used_at, revoked_at.

Folder tidak perlu entitas sendiri; struktur folder diturunkan dari path file.

## Alur utama

1. Super Admin membuat user lewat UI. Developer login.
2. Developer membuat project, lalu menambah knowledge dengan tiga cara:
   - memilih atau drag-drop satu folder dari browser (struktur path dipertahankan);
   - membuat/mengedit file Markdown langsung di web (editor dengan preview Markdown dan Mermaid);
   - upload file satu per satu.
3. Konflik saat upload: file dengan path yang sama ditimpa, file lain di project dibiarkan.
4. Developer meng-upload API spec (YAML/JSON OpenAPI). Spec divalidasi sebagai OpenAPI; yang tidak valid ditolak dengan pesan jelas. UI menampilkannya sebagai dokumentasi API yang dirender (daftar endpoint, detail request/response).
5. Developer membuat API key untuk project, memberi nama (mis. "claude-code-budi"), menyalin key sekali, dan bisa mencabutnya kapan saja.
6. Agent connect ke MCP server dengan key tersebut.

## Permukaan MCP (read-only)

HTTP transport, autentikasi dengan API key (header). Key hanya melihat project-nya sendiri. Tool awal:

- `list_files` — daftar path (opsional filter folder) di project.
- `read_file` — isi satu file.
- `search_knowledge` — pencarian full-text keyword di knowledge project, mengembalikan path dan cuplikan.
- `list_api_specs` — daftar API spec di project (nama, judul, versi).
- `list_api_endpoints` — daftar endpoint (method, path, ringkasan) dari satu spec, atau dari semua spec dikelompokkan per spec.
- `get_api_endpoint` — detail satu endpoint pada spec tertentu (parameter, body, response, dengan skema yang relevan saja agar hemat token).

`list_files`/`read_file` hanya mencakup knowledge; spec diakses lewat tool API spec. Tipe file ditentukan saat upload: file `.yaml`/`.yml`/`.json` yang valid sebagai OpenAPI menjadi api_spec; `.md`/`.txt` menjadi knowledge; file lain ditolak dengan pesan jelas. Saat upload folder campuran, tiap file diklasifikasi sendiri.

Nama dan bentuk persisnya boleh disesuaikan builder.

## Audit log

Kebutuhan utama: bisa mengaudit apa saja yang dilakukan agent. Aksi manusia dicatat di tabel yang sama.

Keputusan pengguna:
- Setiap panggilan tool MCP dicatat: API key (id + nama), project, nama tool, argumen (path, query, nama spec, dsb.), waktu, IP, status (ok/error), dan ringkasan hasil (mis. ukuran byte, jumlah hasil pencarian, pesan error). Isi file yang dibaca tidak disalin ke log.
- Panggilan yang ditolak juga dicatat (key salah/dicabut, project tidak cocok), sejauh bisa dikaitkan ke prefix key.
- Aksi manusia dicatat di audit log yang sama: login (berhasil/gagal), buat/edit/hapus/upload file, buat/cabut API key, ubah anggota, buat/nonaktifkan user, reset password, hapus project.
- Hanya Super Admin yang dapat melihat audit log (halaman terfilter per aktor, project, tool, rentang waktu).
- Retensi 90 hari; entri lebih lama dihapus otomatis oleh job terjadwal di dalam binary.
- Log bersifat append-only dari sisi aplikasi (tidak ada UI atau API untuk mengubah/menghapus entri).

Dikerjakan builder: skema tabel, indeks, mekanisme job retensi, dan cara mencatat tanpa memperlambat respons MCP (mis. insert sinkron ringan atau buffer). Entri audit tetap tersimpan walaupun project/key yang bersangkutan dihapus, sampai kedaluwarsa (simpan nama dan id sebagai salinan, bukan hanya foreign key).

## Dikerjakan builder (keputusan ditinggal)

- Library/framework Go (router, migrasi, driver DB) dan library MCP Go; pilih yang maintained.
- Mekanisme sesi login web (cookie session) dan hashing password (argon2id atau bcrypt).
- Library Svelte (SvelteKit static adapter atau Vite SPA), editor Markdown, renderer OpenAPI, renderer Mermaid.
- Full-text search: pakai fitur FTS bawaan Postgres. Bahasa campuran Indonesia/Inggris, pilih konfigurasi yang masuk akal.
- Batas ukuran file dan jumlah file per upload; validasi path (tanpa `..`, normalisasi).
- Layer storage dibungkus interface kecil agar object storage bisa ditambah kemudian tanpa mengubah sisanya.

## Catatan keputusan

Pengguna sempat memilih Postgres + RustFS, lalu setelah menimbang bahwa semua file hanya teks kecil, beralih ke "semua di Postgres". Alasan: komponen lebih sedikit, backup satu `pg_dump`, tidak ada risiko data tidak sinkron antara DB dan object storage.

## Keputusan asisten (belum disetujui eksplisit, periksa)

- Transport MCP: HTTP, agar tim bisa mengakses satu server dari mana saja.
- Login web: cookie session; API key hanya untuk MCP dan tidak bisa dipakai login web.
- Developer yang dihapus dari project otomatis kehilangan akses, tetapi key yang dia buat tetap hidup sampai dicabut oleh anggota project.
- User yang dinonaktifkan: sesinya tidak berlaku lagi (status dicek di setiap request). Key yang dia buat tetap hidup sampai dicabut, sama seperti kasus di atas.
- Respons MCP dibatasi ukurannya (default sekitar 100 KB, dengan penanda terpotong dan opsi offset); angka pastinya diputuskan builder.
- OpenAPI yang didukung: 3.0 dan 3.1, dengan resolusi `$ref` (termasuk sirkular). Swagger 2.0 tidak didukung.
- TLS ditangani reverse proxy di depan binary. Login diberi rate limit; key yang salah juga dibatasi. CORS tidak diperlukan karena UI dan API satu origin.
- Hapus bersifat permanen (hard delete). Menghapus project menghapus semua file, anggota, dan key-nya.
