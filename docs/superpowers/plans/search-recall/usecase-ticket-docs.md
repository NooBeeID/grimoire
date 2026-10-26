# ticket-docs

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 1 dan Task 18. LATE: isi akhir bergantung pada foundation-notes.md (durasi K7, EXPLAIN) dan eval-notes.md (angka).

## Changes
- `docs/tickets/` (T20-project-search-recall.md baru, README.md), `rules.txt`
- `docs/superpowers/plans/search-recall/` (hanya membaca notes)

## Steps
1. Task 1 Step 1-2 (tiket format T17, README: tabel, tree, struktur module, katalog bus dan audit, catatan field aditif).
2. Task 18 Step 2-3: tabel pemetaan T20-AC1..AC8 ke nama tes (cek nama tes sungguhan dengan grep), bagian Risiko K7 (durasi), EXPLAIN, tabel penyelarasan bobot, status tiket. Status sesuai hasil verifikasi, jangan klaim lulus tanpa menjalankan.
3. Task 18 Step 1: jalankan verifikasi akhir backend (make lint/test/test-integration/test-search-eval/api-check/api-selftest) dan lampirkan hasil jujur di tiket.
