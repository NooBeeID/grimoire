# T11 — discovery / knowledge-tools (pengukuran kualitas pencarian)

| | |
|---|---|
| Domain / Module | discovery / knowledge-tools (lintas ke core / audit-log hanya sebagai pembaca data) |
| Ukuran | M |
| Bergantung pada | T02, T05, T08 |

## Ringkasan

`search_knowledge` hari ini full-text Postgres (`'simple'`, tanpa stemming dan sinonim). Kita berniat memperbaikinya (trigram, stemming, cuplikan dengan offset), tetapi belum punya cara mengukur apakah perbaikan itu benar-benar membantu agent. Audit log sudah mencatat query (`args`) dan `count`/`total`, tetapi **tidak mencatat path hasil dan urutannya**. Akibatnya tidak bisa dihitung apakah agent membuka hasil teratas, hasil kedua, atau menyerah dan mencari ulang.

Tiket ini menambahkan dua alat ukur, tanpa mengubah cara kerja pencarian:

1. **Sinyal produksi:** catat path hasil teratas ke ringkasan audit, lalu hitung metrik dari `audit_log`.
2. **Tolok ukur offline:** golden set (query beserta path yang diharapkan) yang dijalankan sebagai tes dengan metrik recall@k dan MRR, supaya perubahan algoritma bisa dibandingkan sebelum dan sesudah.

## Tujuan

- Setiap panggilan `search_knowledge` mencatat daftar path hasil teratas (bukan isi, bukan cuplikan) di `audit_log.result`.
- Metrik berikut bisa dihitung per project dan rentang waktu dari data audit:
  - **zero-result rate**: persentase pencarian dengan `total = 0`.
  - **follow-up rate**: persentase pencarian yang diikuti `read_file` oleh aktor dan project yang sama, pada path yang ada di hasil, dalam 2 menit.
  - **rank dibuka**: distribusi urutan hasil yang dibuka (1, 2, 3, ..., tidak di hasil).
  - **reformulation rate**: persentase pencarian yang diikuti `search_knowledge` lain oleh aktor dan project yang sama dalam 2 menit.
- Ada golden set + runner yang menghasilkan recall@1, recall@5, dan MRR, dan bisa dijalankan lokal dan di CI.
- Tersedia baseline angka untuk implementasi pencarian saat ini, tercatat di dokumen.

## Non-goals

- Mengubah algoritma pencarian (trigram, stemming, embedding, cuplikan dengan offset). Itu tiket terpisah yang memakai alat ukur ini.
- UI dashboard kualitas pencarian.
- Tool MCP baru untuk umpan balik eksplisit (mis. `rate_search`). Seluruh tool tetap read-only.
- Menyimpan isi file, cuplikan, atau skor mentah di audit.
- Mengubah retensi, skema, atau sifat append-only `audit_log`.
- Mengelompokkan "sesi" lewat session id MCP. Gateway tetap stateless; pengelompokan memakai aktor + project + jendela waktu.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Isi file atau cuplikan bocor ke audit.
- Ringkasan audit membengkak sampai menambah latensi atau ukuran tabel secara berarti.
- Kegagalan menulis path hasil mengubah respons ke agent (audit tetap fail-open, sesuai T02).
- Perilaku dan format teks hasil `search_knowledge` untuk agent berubah.
- Golden set ditulis mengikuti keluaran pencarian saat ini sehingga baseline terlihat sempurna.

## Aturan bisnis

- **Yang dicatat:** `Summary` hasil `search_knowledge` mendapat kunci `result_paths`: array path hasil, urutan sama dengan keluaran, dibatasi 10 entri teratas. Tiap path dipotong ke batas yang sama dengan `target` audit. `count` dan `total` tetap ada.
- Path adalah metadata (sudah tercatat sebagai `target` untuk `read_file`), bukan isi file. Komentar `SearchHits.Summary` di `knowledgetools/base/searching.go` ("tanpa ... path hasil") diperbarui dan alasannya ditulis, karena ini membalik keputusan T08.
- Query sudah tercatat di `args.query` (dibersihkan T02). Tidak ada perubahan di sana.
- **Definisi follow-up:** entri `mcp.read_file` dengan `actor_id` dan `project_id` sama, `occurred_at` dalam 120 detik setelah pencarian, dan `target` termasuk di `result_paths`. Bila ada beberapa, yang pertama dipakai. `rank` = posisi 1-based di `result_paths`.
- **Definisi reformulasi:** entri `mcp.search_knowledge` berikutnya dengan `actor_id` dan `project_id` sama dalam 120 detik, dengan `args.query` berbeda.
- Pembacaan data audit hanya lewat query baca; tidak ada UPDATE atau DELETE pada `audit_log`.
- Entri audit lama (sebelum tiket ini) tidak punya `result_paths`. Metrik follow-up dan rank mengabaikan entri tanpa kunci itu, dan metrik lain tetap menghitungnya.

## Features (1 feature = 1 folder)

| Folder | Deskripsi | Titik masuk |
|---|---|---|
| `searchknowledge` (ubah) | `Summary` menambah `result_paths` | tool MCP `search_knowledge` |
| `searchquality` (baru) | Laporan metrik dari `audit_log`: zero-result, follow-up, rank, reformulasi | perintah CLI `grimoire search-quality report` |
| `searcheval` (baru) | Runner golden set: recall@1, recall@5, MRR | tes Go (`make test-search-eval`) |

Fixture dan golden set: `backend/testdata/searcheval/` (korpus markdown kecil + `golden.yaml`).

## Kontrak bus

Tidak ada endpoint bus baru.
`searchquality` membaca `audit_log` lewat bus T02 bila sudah ada endpoint baca yang cukup, atau lewat query repository sendiri yang hanya `SELECT` (periksa dulu; lihat keputusan K2).

## UI

Tidak ada.

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

- K1. Batas `result_paths` = 10 (tool memperbolehkan `limit` sampai 20). Agent hampir selalu hanya membuka beberapa hasil teratas, dan ini menjaga ukuran `result` audit kecil. Ubah bila ingin rank > 10 terlihat.
- K2. Laporan metrik berupa perintah CLI yang menjalankan query SQL baca-saja (keluaran tabel teks, opsi `--project`, `--since`, `--json`), bukan endpoint HTTP. Alasannya: tidak ada UI, dan lintas-module read ke `audit_log` perlu keputusan arsitektur yang tidak diperlukan oleh kebutuhan ini. Bila aturan arsitektur melarang CLI membaca tabel milik core langsung, pindah ke bus `core.audit.*`.
- K3. Jendela 120 detik, ditetapkan sebagai konstanta dan bisa diubah lewat flag `--window`.
- K4. Golden set diisi 30–50 pasang. Setiap pasang: `query`, `expect` (satu atau lebih path yang dianggap benar), dan `kind` (`exact-name`, `keyword`, `synonym`, `typo`, `phrase`). Pasangan `synonym` dan `typo` memang diharapkan gagal pada baseline; itu tujuan golden set, bukan bug.
- K5. Runner tidak memutuskan lulus atau gagal berdasarkan ambang absolut. Ia mencetak angka dan membandingkan dengan `baseline.json` yang di-commit; tes gagal hanya bila angka **turun** dari baseline lebih dari toleransi (default 0.02). Memperbarui baseline adalah commit eksplisit.

## Constraints

- Perubahan `Summary` tidak boleh mengubah `Result.Text` maupun `Result.Structured`.
- Ukuran `result` audit untuk satu pencarian bertambah paling banyak ± 5 KB (10 path × 512 karakter). Periksa apakah T02 membatasi ukuran `result`; bila ya, pastikan `result_paths` tidak terpotong diam-diam.
- Runner eval memakai Postgres tes yang sudah dipakai tes integrasi (bukan mock), supaya `ts_rank` dan konfigurasi bahasa sungguhan yang diukur.
- Kode tidak meng-import package module lain (aturan arsitektur T00); data lewat bus atau kontrak.

## Titik sentuh

- `backend/internal/domain/discovery/internal/knowledgetools/base/searching.go` (`SearchHits.Summary`)
- `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/` (tes)
- `backend/internal/domain/discovery/internal/mcpgateway/base/audit.go` (hanya dibaca; `Summary` sudah disalin ke `result`)
- `backend/cmd/cli` (subperintah baru)
- `backend/testdata/searcheval/`, `makefile`, `docs/mcp-gateway.md` (bagian Audit), dokumen baru `docs/search-quality.md`

## Kriteria penerimaan

1. Satu panggilan `search_knowledge` dengan 3 hasil menghasilkan entri `mcp.search_knowledge` dengan `result.result_paths` berisi 3 path dengan urutan sama seperti teks hasil, ditambah `count` dan `total` seperti sebelumnya.
2. Hasil berisi lebih dari 10 item: `result_paths` hanya 10 teratas; `count` dan `total` tetap menggambarkan hasil sebenarnya.
3. Tes memastikan tidak ada cuplikan, skor, atau isi file di `result` audit.
4. Pencarian tanpa hasil: `result_paths` kosong (array kosong), `total = 0`.
5. `Result.Text` dan `Result.Structured` untuk `search_knowledge` identik dengan sebelum perubahan (tes snapshot).
6. Bila penulisan audit gagal, respons pencarian ke agent tetap sama (perilaku fail-open T02 tidak berubah).
7. Pada data uji: pencarian A diikuti `read_file` ke hasil ke-2 dalam 30 detik oleh key yang sama → laporan menghitung follow-up dengan rank 2. `read_file` oleh key lain, project lain, atau 3 menit kemudian → tidak dihitung.
8. Pada data uji: dua pencarian berurutan dengan query berbeda oleh key yang sama dalam 60 detik → dihitung satu reformulasi; query yang sama persis diulang → tidak dihitung.
9. Entri audit lama tanpa `result_paths` tidak membuat laporan error dan tidak ikut dalam metrik follow-up/rank.
10. `grimoire search-quality report --project <id> --since 7d` mencetak keempat metrik; `--json` menghasilkan JSON valid.
11. `make test-search-eval` menjalankan golden set terhadap korpus fixture dan mencetak recall@1, recall@5, MRR, dan rincian per `kind`.
12. `baseline.json` berisi angka implementasi saat ini; menurunkan kualitas (mis. mengganti konfigurasi pencarian ke yang lebih buruk) membuat tes gagal, perbaikan tidak.
13. Golden set memuat minimal 30 pasang dan minimal 3 dari tiap `kind`; pasangan ditulis dari kebutuhan nyata (istilah yang dipakai tim), bukan disalin dari keluaran pencarian.
14. `docs/search-quality.md` menjelaskan definisi metrik, cara membaca laporan, cara menambah pasangan golden set, dan cara memperbarui baseline.

## Risiko

- Metrik follow-up hanya perkiraan: agent bisa membuka hasil karena alasan lain, atau tidak membuka karena cuplikan sudah cukup. Dibaca sebagai tren, bukan kebenaran mutlak. Ditulis di `docs/search-quality.md`.
- Dengan retensi 90 hari, tren jangka panjang hilang. Laporan `--json` bisa disimpan manual bila diperlukan.
- Golden set bisa usang bila korpus berubah. Fixture korpus dibuat tetap (di-commit), terpisah dari knowledge sungguhan.
- Korpus fixture kecil tidak mewakili skala produksi; skor absolut tidak boleh dipakai sebagai klaim performa produksi, hanya untuk membandingkan dua versi pencarian.

## Pertanyaan terbuka

1. Apakah kita juga ingin mencatat `result_paths` untuk `list_files`? Saat ini tidak diperlukan; ditunda.
2. Siapa yang menulis pasangan golden set awal? Idealnya anggota tim yang memakai knowledge sehari-hari, karena kualitasnya menentukan nilai seluruh tiket ini.
3. Apakah hasil laporan perlu ditampilkan di UI Super Admin pada tiket lanjutan, atau cukup CLI?
