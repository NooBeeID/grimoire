# Kualitas pencarian

Dokumen ini menjelaskan cara mengukur kualitas tool `search_knowledge`. Ada dua alat ukur yang saling melengkapi.

## 1. Tujuan dan ruang lingkup

- **Sinyal produksi** dari audit log: seberapa sering pencarian nol hasil, apakah hasil dibuka agent, dan apakah query diulang.
- **Tolok ukur offline** (golden set): korpus tetap dan pasangan query-jawaban yang dijalankan terhadap Postgres tes.

Skor absolut dari korpus kecil bukan klaim performa produksi. Gunanya hanya membandingkan dua versi implementasi: apakah perubahan memperbaiki atau memperburuk.

## 2. Definisi metrik produksi

- **Zero-result rate**: porsi pencarian dengan `total = 0`. Penyebut: pencarian yang `total`-nya terbaca.
- **Follow-up rate**: porsi pencarian yang diikuti `read_file` oleh aktor dan project yang sama dengan selisih `0 < delta <= 120 detik`, dengan target file ada di `result_paths`. Penyebut: pencarian yang punya `result_paths`.
- **Rank dibuka**: posisi (bucket 1..10 atau `not_in_results`) file yang dibuka pada read pertama yang cocok.
- **Reformulation rate**: porsi pencarian `ok` yang langsung disusul pencarian `ok` berikutnya oleh aktor sama dalam jendela waktu, dengan query berbeda setelah normalisasi huruf kecil dan spasi.

Entri audit lama tanpa `result_paths` dilewati, dan hanya pencarian berstatus `ok` yang dihitung.

## 3. Definisi metrik offline

- **recall@k** = `|expect ∩ top-k| / |expect|`, dirata-rata atas semua pasangan.
- **MRR** = rata-rata `1/rank` dari path `expect` pertama di top-10; 0 bila tidak ada.

Contoh: `expect` berisi dua path, satu di rank 2 dan satu di rank 7. recall@1 = 0, recall@5 = 0.5, recall@10 = 1, RR = 0.5.

## 4. Membaca laporan CLI

```
grimoire search-quality report --project <id> --since 7d [--until] [--window] [--json]
```

Keluaran teks menampilkan keempat metrik produksi di atas dan baris `Entri lama dilewati` untuk entri tanpa `result_paths`. Opsi `--json` cocok untuk disimpan berkala.

## 5. Metrik sebagai tren

Follow-up rate hanyalah perkiraan: agent bisa membuka file karena alasan lain, atau cuplikan sudah cukup. Reformulasi bisa berarti eksplorasi. Bacalah perubahan antar minggu, bukan nilai mutlak.

Retensi audit adalah 90 hari. Untuk tren jangka panjang, simpan keluaran `--json` tiap minggu dengan nama berstempel tanggal, mis. `search-quality-2026-10-21.json`.

Metrik produksi yang sama juga tampil di halaman Dashboard (`/dashboard`, khusus Super Admin) untuk project terpilih, lewat `GET /api/audit-log/summary?project_id=...` (T15). Dashboard memakai rentang preset 24 jam, 7 hari, atau 30 hari dengan jendela follow-up default 120 detik, sama dengan CLI. CLI `search-quality report` tetap ada. Catatan snapshot: laporan ini dihitung T11 lewat koneksi sendiri, sehingga bisa berbeda sedikit dari agregat tool di halaman yang sama bila ada entri audit yang masuk bersamaan. Bila laporan gagal dimuat, Dashboard menampilkan blok kualitas pencarian tidak tersedia dengan tombol coba lagi, sementara chart lain tetap tampil.

## 6. Menjalankan golden set

```
make test-search-eval
```

Perintah ini membutuhkan Postgres tes. Log menampilkan ringkasan (recall@1, recall@5, MRR), satu baris per kind, dan satu baris `GAGAL` per pasangan yang recall@5-nya belum 1.

Tes lulus kecuali salah satu dari recall@1, recall@5, atau MRR keseluruhan turun lebih dari toleransi (default 0.02, ubah dengan `SEARCH_EVAL_TOLERANCE=0.05 make test-search-eval`). Perbaikan tidak pernah gagal. Angka per kind hanya informasi.

## 7. Menambah pasangan golden

Fixture ada di `backend/testdata/searcheval/` (`corpus/`, `golden.yaml`, `baseline.json`).

- Tulis pasangan dari kebutuhan nyata dan istilah tim, SEBELUM menjalankan pencarian. Jangan menyalin keluaran pencarian.
- Setiap `expect` harus path yang ada di `corpus/`; query harus unik; `kind` salah satu dari `exact-name`, `keyword`, `synonym`, `typo`, `phrase`.
- Minimal 30 pasangan dan 3 per kind.
- Kamus sinonim fixture ada di `backend/testdata/searcheval/synonyms.yaml` (`groups: [[term, ...], ...]`). `TestSearchEval` mengirimnya lewat `POST /api/projects/{id}/search/synonyms` setelah korpus di-seed dan sebelum mencari, persis seperti admin mengisinya. Tulis kelompok dari istilah tim (2 sampai 10 term, huruf kecil, unik lintas kelompok), periksa kosakata korpus dengan `grep`, dan jangan menyalin keluaran pencarian atau memasukkan query golden mentah-mentah sebagai kelompok. `go test ./lib/util/rankeval/` (`TestSynonymsFixture`) memvalidasi file tanpa DB.
- Dokumen baru di `corpus/` ditulis natural, tanpa menyisipkan sinonim atau salah ketik sebagai umpan.
- Karena jumlah pasangan berubah, perbarui baseline (bagian 8).
- `cd backend && go test ./lib/util/rankeval/` memeriksa fixture tanpa DB.

## 8. Memperbarui baseline

Perbarui hanya saat ada perubahan algoritma yang disengaja, atau perubahan golden/korpus:

```
make search-eval-baseline
```

Tinjau diff `baseline.json`, lalu commit dengan alasan di pesan commit. Jangan memperbarui baseline untuk menutupi penurunan yang tidak dimengerti.

## 9. Baseline saat ini

Tanggal 2026-10-26, 40 pasangan, 25 dokumen korpus, implementasi T20 (sinonim, stemming, typo, bobot, frasa) dengan kamus fixture.

| Kind | Pasangan | recall@1 | recall@5 | MRR | MRR lama |
|---|---|---|---|---|---|
| keseluruhan | 40 | 0.9875 | 1.0000 | 1.0000 | 0.6000 |
| exact-name | 8 | 1.0000 | 1.0000 | 1.0000 | 1.0000 |
| keyword | 10 | 1.0000 | 1.0000 | 1.0000 | 1.0000 |
| synonym | 8 | 0.9375 | 1.0000 | 1.0000 | 0.1250 |
| typo | 7 | 1.0000 | 1.0000 | 1.0000 | 0.0000 |
| phrase | 7 | 1.0000 | 1.0000 | 1.0000 | 0.7143 |

Pembanding (2026-10-21, `simple` FTS sebelum T20): keseluruhan recall@1 0.5875, recall@5 0.5875, MRR 0.6000. Target K1 T20 (recall@5 >= 0.80, typo dan synonym recall@5 >= 0.50, MRR >= 0.70, exact-name dan phrase tidak turun) tercapai. Hasil `synonym` bergantung pada kamus fixture; tanpa kamus kinerjanya kembali rendah.

## 10. Keterbatasan

- Korpus hanya 25 file dengan bahasa campuran Indonesia dan Inggris, dan kamus fixture ditulis oleh tim yang sama dengan golden set. Skor 1.0 menandakan tidak ada regresi pada fixture ini, bukan kualitas produksi.
- Pencarian tanpa filter path dan dengan `limit=10`.
- Trigram lemah untuk kata pendek atau salah ketik berupa transposisi, mis. `kyes` untuk `keys`; kemiripannya rendah sehingga bisa lolos dari ambang `TypoMinSimilarity`.
- Stemmer Indonesia bawaan Postgres tidak memulihkan peluluhan (mis. `mengirim` ke `kirim`); itu ditutup oleh `stemid` di sisi query.
- Skor tidak mewakili performa produksi.

## 11. Cara kerja pencarian

- **Bobot**: path dan judul berbobot A, heading B, isi D (`RankWeights` di `knowledge/base/rank.go`, urutan {D, C, B, A} = {0.1, 0.2, 0.5, 1.0}). Skor dasar adalah `ts_rank_cd` indeks `simple` ditambah `StemFactor` (0.5) kali `ts_rank_cd` indeks stem. Kecocokan frasa utuh pada judul atau heading menambah `PhraseBoost` (0.5).
- **Cakupan**: query dipecah menjadi grup term (kata, frasa berkutip, atau deret yang cocok dengan satu term kamus). Dokumen lolos bila memuat minimal `ceil(n * MinCoverage)` grup (`MinCoverage` = 0.5). Urutan akhir: jumlah grup cocok, lalu skor, lalu path.
- **Sinonim**: kelompok kamus project berlaku dua arah. Term sinonim dinormalisasi seperti query (`rate-limit` sama dengan `rate limit`), deret kata terpanjang dicocokkan dulu, dan setiap anggota kelompok menjadi alternatif (OR) di grup yang sama. Kamus dikelola lewat tab Pencarian atau `/api/projects/{id}/search/synonyms`.
- **Typo**: kata yang tidak cocok satu dokumen pun di project dicarikan kandidat dari kosakata project (`knowledge_terms`, pg_trgm) dengan kemiripan minimal `TypoMinSimilarity` (0.4), maksimal `TypoMaxCandidates` (3) per kata, hanya untuk kata 4 sampai 64 karakter yang bukan angka.
- **Section**: cuplikan diambil dari bagian (heading sampai heading berikutnya) yang paling cocok; field `section` berisi teks heading, kosong bila di luar heading.
- **expanded**: `true` bila rencana pencarian diperluas oleh sinonim atau koreksi typo. Nilainya ikut di hasil dan di audit `mcp.search_knowledge` (`result.expanded`), sehingga reformulasi akibat kamus kurang bisa dilacak.

Penyelarasan konstanta dan tabel percobaan ada di `docs/superpowers/plans/search-recall/eval-notes.md`.
