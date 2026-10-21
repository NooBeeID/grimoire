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

Tanggal 2026-10-21, 40 pasangan, 25 dokumen korpus (implementasi `simple` FTS sebelum perbaikan kualitas).

| Kind | Pasangan | recall@1 | recall@5 | MRR |
|---|---|---|---|---|
| keseluruhan | 40 | 0.5875 | 0.5875 | 0.6000 |
| exact-name | 8 | 1.0000 | 1.0000 | 1.0000 |
| keyword | 10 | 1.0000 | 1.0000 | 1.0000 |
| synonym | 8 | 0.0625 | 0.0625 | 0.1250 |
| typo | 7 | 0.0000 | 0.0000 | 0.0000 |
| phrase | 7 | 0.7143 | 0.7143 | 0.7143 |

`synonym` dan `typo` diharapkan rendah dan menjadi target perbaikan berikutnya (trigram, stemming).

## 10. Keterbatasan

- Korpus hanya 25 file dengan bahasa campuran Indonesia dan Inggris.
- Pencarian tanpa filter path dan dengan `limit=10`.
- Skor tidak mewakili performa produksi.
