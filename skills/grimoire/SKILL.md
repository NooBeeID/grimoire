---
name: grimoire
description: Cara memakai MCP server Grimoire (knowledge base per project, tool read-only) untuk membaca dokumen dan spesifikasi API project. Gunakan setiap kali user menyebut Grimoire, knowledge base, dokumen project, spesifikasi API/OpenAPI/Swagger, endpoint, atau meminta "lihat project <nama>", "cek docs", "cari di knowledge", "endpoint apa saja", atau saat mengerjakan fitur yang bergantung pada dokumen/API project, walau user tidak menyebut nama tool-nya.
---

# Grimoire

Grimoire adalah knowledge base per project yang diakses lewat MCP (`list_files`, `read_file`, `search_knowledge`, `list_api_specs`, `list_api_endpoints`, `get_api_endpoint`). Semua tool read-only. Sumber kebenaran untuk dokumen dan kontrak API project ada di sini, jadi baca dari sini dulu sebelum menebak dari ingatan atau kode.

## 1. Pilih server yang benar

Setiap API key terikat ke **satu project**, dan tidak ada argumen `project_id` (argumen itu ditolak). Project ditentukan oleh server MCP mana yang dipanggil. Tool muncul sebagai `mcp__<nama-server>__<tool>`.

- Cari server Grimoire yang tersedia (nama tool berawalan `mcp__...` dengan tool `list_files`/`search_knowledge`).
- Bila ada lebih dari satu server, cocokkan nama server dengan project yang disebut user (mis. "lihat project aeris" -> `grimoire-aeris`). Teks `instructions` server juga memuat nama project ("Server pengetahuan untuk <project>").
- Bila tidak jelas yang mana, tanyakan ke user. Jangan menyapu semua server: itu boros dan bisa mencampur isi antar project.
- Bila tidak ada server Grimoire sama sekali, beri tahu user dan tunjukkan cara menambahkannya:
  `claude mcp add --transport http grimoire-<project> https://<host>/mcp --header "Authorization: Bearer <api-key>"`.
  Jangan menulis API key ke file yang di-commit.

## 2. Menelusuri dokumen

Urutan umum, dari murah ke mahal:

1. `search_knowledge` bila tahu kata kuncinya. Ini full-text, bukan semantik: pakai kata yang kemungkinan tertulis di dokumen, bungkus frasa dengan kutip (`"rate limit"`), dan persempit dengan `path` (prefix). Coba sinonim bila hasil kosong.
2. `list_files` bila perlu melihat struktur. Mulai tanpa `recursive` untuk satu tingkat; pakai `recursive: true` dengan `path` tertentu saja. Hasil terpotong memberi petunjuk `offset=` untuk lanjut.
3. `read_file` untuk membaca isi. `offset` dan `limit` dalam **byte**. Bila terpotong, hasil menyebut `offset=N`; lanjutkan dari sana, jangan baca ulang dari awal.

Path persis seperti yang tampil di `list_files`/`search_knowledge`. Bila file tidak ketemu, pesan error menyarankan path mirip; pakai itu.

## 3. Menelusuri API spec

Telusuri bertahap supaya tidak memuat spec besar sekaligus:

1. `list_api_specs` untuk melihat spec yang ada. Kolom nama dipakai sebagai argumen `spec`.
2. `list_api_endpoints` dengan `spec`, lalu persempit dengan `tag` (persis) atau `query` (cocok sebagian). Satu baris per operasi.
3. `get_api_endpoint` untuk detail satu operasi: beri `method` + `path` persis seperti di spec (termasuk `{param}`), atau `operation_id`. `spec` wajib bila project punya lebih dari satu spec.

Penanda di JSON detail: `{"$circular": ref}` berarti skema merujuk dirinya sendiri; `{"$ref": ref}` berarti tidak dibuka karena batas `schema_depth`. Bila butuh lebih dalam, naikkan `schema_depth` (1-8, default 4). `truncated: true` dengan `hint` berarti kedalaman diturunkan otomatis agar muat.

Spec Swagger 2 mungkin belum didukung tergantung versi server; bila upload/pembacaan gagal, laporkan apa adanya.

## 4. Kebiasaan yang membantu

- Kutip sumbernya: sebut path file atau `METHOD /path` yang dibaca, supaya user bisa memverifikasi.
- Bila dokumen dan kode bertentangan, laporkan keduanya; jangan diam-diam memilih salah satu.
- Bila tidak menemukan apa pun, katakan bahwa Grimoire tidak punya informasi itu (sebut apa yang sudah dicari). Jangan mengarang isi dokumen.
- Argumen ketat: field tak dikenal atau nilai di luar rentang menghasilkan `isError`, bukan dipotong. Baca pesan errornya, perbaiki argumen, ulangi.
- Rate limit default 120 request/menit per key. Hindari loop pembacaan file satu per satu; cari dulu, baca hanya yang relevan.
- Tool tidak bisa menulis. Untuk mengubah dokumen atau spec, arahkan user ke UI Grimoire.
