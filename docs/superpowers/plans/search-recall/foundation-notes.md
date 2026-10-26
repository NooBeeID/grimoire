# T20 foundation notes (bahan tiket)

## K7: durasi migrasi `knowledge_search_recall` (Task 4)

Diukur 2026-10-26 di `postgres:16` lokal (docker, port 5433): 10.000 file sintetis, isi ~8 KB, `sections IS NULL`.
Migrasi dijalankan ulang penuh di schema yang sudah termigrasi (drop + add `search` dan `search_stem`, dua indeks GIN,
backfill `knowledge_terms`): **26,3 detik**, menghasilkan 99.901 baris `knowledge_terms`. Selama itu `knowledge_files`
terkunci `ACCESS EXCLUSIVE` (runner migrasi satu transaksi per file, `CREATE INDEX CONCURRENTLY` tidak mungkin).
Migrasi berjalan saat start sebelum server melayani, jadi dampaknya penundaan start, kira-kira linear terhadap
total ukuran konten (~0,33 s per MB). Pada instalasi dengan ratusan ribu file perlu jendela pemeliharaan.

## EXPLAIN (Task 11, rule 8.8)

Korpus sintetis: 1 project, 2.000 file (~1.000 kata acak berbeda per file, ~8 KB) ditambah 1 file target; `ANALYZE` dijalankan.
Postgres 16 lokal, semua di cache (`shared hit`).

- `Store.Search` ("peluncuran khusus", 2 kelompok): **~13 ms** (Execution Time 11,7 sampai 14,1 ms antar run). Penyaring kandidat
  `search @@ (SELECT qs FROM q) OR search_stem @@ (SELECT qt FROM q)` ditulis sebagai InitPlan. Pada korpus satu-project ini planner
  memilih `Bitmap Heap Scan` lewat `knowledge_files_project_updated_idx` (atau `Seq Scan`) dengan `@@` sebagai filter, bukan
  GIN: biaya GIN diperkirakan 1.394 per indeks versus 31 untuk btree project, karena project memegang seluruh tabel. Bentuk
  query yang sama dengan tsquery konstan juga tidak memakai GIN di korpus ini, jadi ini pilihan planner, bukan akibat InitPlan.
  Bukti GIN dapat dipakai: kueri tanpa filter project `search @@ ... OR search_stem @@ ...` memakai `BitmapOr` dari
  `Bitmap Index Scan on knowledge_files_search_gin` dan `knowledge_files_search_stem_gin` (0,65 ms). Konsekuensi: biaya pencarian
  linear terhadap jumlah file project (~6 microdetik per file 8 KB), diterima karena `MaxProjectFiles` membatasi ukuran project
  dan korpus multi-project memberi selektivitas ke project. Bila project jauh lebih besar, pertimbangkan indeks gabungan
  `btree_gin (project_id)` + `search` (butuh ekstensi tambahan, di luar T20).
- `Store.SuggestTerms` (2 kata): **~41 sampai 44 ms**, kebanyakan memindai 8.831 baris `knowledge_terms` project via
  `knowledge_terms_project_term` dengan `%` sebagai filter (indeks trigram `knowledge_terms_term_trgm` tidak dipilih karena project
  memegang seluruh tabel) dan anti-join `NOT EXISTS` terhadap 2.001 file (satu pemeriksaan `@@` per kata). Hanya berjalan untuk kata
  tunggal tanpa sinonim yang >= 4 rune, maksimal 16 kata per query.
