# T18: tool MCP outline, section, dan perubahan terbaru

| | |
|---|---|
| Domain / Module | discovery / knowledge-tools dan api-spec-tools (membaca project/knowledge dan project/apispec lewat bus) |
| Ukuran | M |
| Bergantung pada | T07, T08, T09 |
| Dikerjakan | pertama dari tiga (T18, T19, T20) karena parser heading dipakai T20 |

## Maksud dan alasan

Agent hari ini hanya bisa `read_file` penuh atau per byte. File Markdown panjang menghabiskan token untuk satu jawaban kecil, dan agent tidak tahu apa yang berubah sejak terakhir ia membaca. Tiga tool baru memberi agent cara membaca bertahap: lihat kerangka dulu, baca satu bagian, dan tanya apa yang berubah.

## Tujuan

- `get_file_outline`: daftar heading sebuah file Markdown (level, teks, nomor baris, rentang byte) beserta `size_bytes` dan `updated_at`.
- `read_section`: isi satu bagian file, dari satu heading sampai sebelum heading berikutnya yang selevel atau lebih tinggi.
- `list_recent_changes`: file knowledge dan spec API yang berubah sejak waktu tertentu.
- Ketiganya read-only, tunduk pada project key, timeout, batas respons, dan rate limit yang sama dengan tool lain, dan masing-masing menulis tepat satu entri audit.

## Non-goals

- Riwayat versi atau diff isi (tidak ada tabel versi; tool hanya bilang "berubah", bukan "apa yang berubah").
- Tool tulis apa pun.
- Mengubah `read_file`, `list_files`, atau `search_knowledge` yang sudah ada.
- Outline untuk file non-Markdown (file lain mengembalikan outline kosong dengan `size_bytes`, bukan error).

## Anti-tujuan (gagal walau fitur jalan)

- Parser heading menganggap `#` di dalam fenced code block atau komentar shell sebagai heading.
- `read_section` membocorkan byte di luar bagian yang diminta.
- Argumen `project_id` (dan alias-nya) diterima; tetap ditolak seperti tool lain.
- Tool baru tidak muncul di dashboard pemakaian atau tidak punya aksi audit.

## Keputusan pemilik (dari percakapan)

- Tool yang diminta: `get_file_outline`, `list_recent_changes`, dan baca per bagian. Semua read-only.

## Keputusan pembangun (tebakan saya, boleh dikoreksi)

- **K1** Baca per bagian adalah tool terpisah `read_section`, bukan argumen tambahan `read_file`, supaya `read_file` tetap stabil dan skemanya tetap sederhana.
- **K2** Parser heading adalah fungsi murni di `backend/lib/util/mdoutline` (dipakai T20 juga). Mendukung ATX (`#` sampai `######`) dan Setext; mengabaikan fenced code (``` dan ~~~) dan blok HTML; baris heading dihitung dari 1.
- **K3** `read_section` menerima `path`, `heading` (teks persis, tanpa `#`), `occurrence` (default 1, untuk judul ganda), dan `limit` (byte, default sama dengan `read_file`). Heading tidak ketemu menghasilkan `IsError` dengan daftar heading mirip, bukan error Go.
- **K4** `list_recent_changes` menerima `since` (RFC3339 atau durasi `24h`/`7d`/`30d`, wajib), `kind` (`files|specs|all`, default `all`), `path` (prefix untuk file), `limit` (1 sampai 50, default 20). Hasil diurut `updated_at` menurun. Jendela `since` dibatasi 90 hari. Tiap baris: `kind`, `path` atau `name`, `updated_at`, `created` (true bila `created_at == updated_at`), `size_bytes`.
- **K5** `updated_by_label` tidak dikembalikan ke agent (hindari membocorkan identitas manusia ke agent).
- **K6** Mengambil isi file untuk outline lewat bus `project.knowledge.readfile` yang sudah ada; bila kontraknya tidak cukup (mis. tidak ada `updated_at`), tambahkan field di kontrak bus yang ada atau bus baru `project.knowledge.listchanges` dan `project.apispec.listchanges`. Pembangun memilih, dengan syarat aturan arsitektur (`rules.txt`) dan tes `archtest` tetap lulus.

## Kontrak tool

| Tool | Argumen | Hasil ringkas |
|---|---|---|
| `get_file_outline` | `path` | `path`, `size_bytes`, `updated_at`, `headings[{level,text,line,start,end}]` |
| `read_section` | `path`, `heading`, `occurrence?`, `limit?` | teks bagian, `heading`, `start`, `end`, `truncated` |
| `list_recent_changes` | `since`, `kind?`, `path?`, `limit?` | `items[{kind,path|name,updated_at,created,size_bytes}]`, `total` |

Semua skema memakai `additionalProperties:false` dan parse ketat seperti `search_knowledge`.

## Audit

- Aksi baru: `mcp.get_file_outline`, `mcp.read_section`, `mcp.list_recent_changes`.
- `target_type`/`target`: file dan path untuk dua tool pertama; `list_recent_changes` tanpa target.
- Ringkasan `result`: `count` (dan `total` untuk `list_recent_changes`), tanpa isi file.
- Tambah konstanta `ActionMCP*`, daftar `record.Actions()`, katalog aksi di `docs/tickets/README.md`, `docs/mcp-gateway.md`, dan golden `tools_list.golden.json` (`go test ... -update`).

## Yang diserahkan ke pembangun

Layout feature (satu folder per tool seperti `searchknowledge`), nama bus bila perlu, format teks keluaran, dan tes (unit, golden, integrasi `mcpEnv`).

## Kriteria penerimaan

- Fixture Markdown dengan heading di dalam code fence, Setext, heading ganda, dan file tanpa heading dites untuk parser.
- `read_section` mengembalikan tepat satu bagian dan tidak memotong di tengah rune UTF-8.
- Tiga tool terdaftar, ada di `tools/list`, tercatat di audit, dan muncul di agregat dashboard otomatis.
- `make lint`, `make test`, dan `make test-integration` lulus.
