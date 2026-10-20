# T08 — discovery / knowledge-tools

| | |
|---|---|
| Domain / Module | discovery / knowledge-tools |
| Ukuran | S |
| Bergantung pada | T05, T07 |

## Ringkasan

Tool MCP untuk menjelajah dan membaca knowledge project: `list_files`, `read_file`, `search_knowledge`. Module ini tipis: validasi argumen, panggil bus T05, bentuk keluaran yang ramah LLM.

## Tujuan

- Agent bisa menemukan dan membaca knowledge dengan sedikit panggilan dan sedikit token.
- Deskripsi dan pesan error ditulis supaya LLM tahu langkah berikutnya.

## Non-goals

Menulis/mengubah knowledge, membaca API spec (T09), pencarian semantik, akses lintas project.

## Features (1 feature = 1 folder)

| Folder | Tool MCP | Bus yang dipanggil |
|---|---|---|
| `listfiles` | `list_files` | `project.knowledge.list-files` |
| `readfile` | `read_file` | `project.knowledge.read-file` |
| `searchknowledge` | `search_knowledge` | `project.knowledge.search` |
| `register` | mendaftarkan ketiga tool ke gateway (T07) | – |

## Spesifikasi tool

Semua argumen string wajib dibersihkan (trim, panjang maks). Project selalu dari context gateway; **tidak ada parameter project**.

### `list_files`
- Deskripsi (untuk LLM): daftar file knowledge; mulai dari akar untuk memahami struktur sebelum membaca.
- Input: `path` (prefix folder, opsional, default akar), `recursive` (bool, default `false` — hanya satu tingkat dan folder ditampilkan sebagai entri), `limit` (default 200, maks 500), `offset`.
- Output: teks terstruktur berisi daftar `path  ukuran  diperbarui`, ditambah `structuredContent` `{items, total, next_offset}`. Jika terpotong: pesan "ada N lagi, panggil dengan offset=...".

### `read_file`
- Input: `path` (wajib), `offset` (karakter, default 0), `limit` (karakter, default 40000, maks sesuai batas gateway).
- Output: isi file; bila terpotong, baris akhir `[terpotong: lanjutkan dengan offset=N dari total M karakter]`.
- Tidak ketemu → `isError` dengan pesan "File tidak ditemukan" + sampai 5 `Similar` path terdekat dan saran memakai `list_files`/`search_knowledge`.

### `search_knowledge`
- Input: `query` (wajib, 1–200 karakter), `path` (prefix, opsional), `limit` (default 10, maks 20).
- Output: daftar hasil `path`, skor, cuplikan; tanpa hasil → pesan jelas dengan saran mengganti kata kunci.

## Aturan bisnis

- Semua tool `readOnlyHint: true`.
- Kegagalan validasi argumen → `isError` dengan penjelasan bidang mana yang salah.
- Tidak ada logika akses di sini; otorisasi sudah ditetapkan oleh key di gateway, dan T05 membatasi data pada `ProjectID`.
- Audit dilakukan oleh gateway (T07); module ini tidak menulis audit sendiri, tetapi mengembalikan metadata yang dibutuhkan ringkasan (ukuran byte, jumlah hasil) lewat hasil tool.
- Kode tidak mengimpor package T05 maupun T07; hanya `lib/constants`, `lib/contracts/...`, dan `infra/bus`.

## Kontrak bus

Dikonsumsi: tiga endpoint `project.knowledge.*` (T05). Tidak menyediakan endpoint bus.

## UI

Tidak ada.

## Kriteria penerimaan

1. Project dengan folder berstruktur: `list_files` tanpa argumen menampilkan tingkat teratas; `recursive=true` menampilkan semuanya sampai `limit`.
2. `read_file` pada file 200 KB dengan `limit` kecil mengembalikan potongan dan petunjuk `offset` yang benar; memanggil ulang dengan offset itu menyambung tanpa duplikat/hilang.
3. `read_file` path salah ketik mengembalikan saran path mirip.
4. `search_knowledge` mengembalikan cuplikan dengan kata kunci; kueri berisi karakter khusus tidak menyebabkan error.
5. Argumen tambahan seperti `project_id` diabaikan/ditolak dan tidak pernah mengubah project.
6. Tes kontrak: bentuk `tools/list` (nama, deskripsi, schema) stabil dan tervalidasi sebagai JSON Schema.
7. Tidak ada impor terlarang (lolos tes arsitektur T00).

## Pertanyaan terbuka

Tidak ada.
