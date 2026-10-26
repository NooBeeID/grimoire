# Catatan search-eval (bahan tiket T20)

Tanggal 2026-10-26. Postgres localhost:5433, 40 pasangan golden, 25 dokumen korpus, kamus `testdata/searcheval/synonyms.yaml` (9 kelompok) di-seed lewat REST.

## Angka K1

| Kind | recall@1 | recall@5 | MRR |
|---|---|---|---|
| keseluruhan | 0.9875 | 1.0000 | 1.0000 |
| exact-name | 1.0000 | 1.0000 | 1.0000 |
| keyword | 1.0000 | 1.0000 | 1.0000 |
| synonym | 0.9375 | 1.0000 | 1.0000 |
| typo | 1.0000 | 1.0000 | 1.0000 |
| phrase | 1.0000 | 1.0000 | 1.0000 |

Sebelum (baseline 2026-10-21): recall@1 0.5875, recall@5 0.5875, MRR 0.6000; synonym MRR 0.125, typo 0.0.
Target K1 tercapai di semua kind (recall@5 >= 0.80, typo >= 0.50, synonym >= 0.50, MRR >= 0.70, exact-name dan phrase tidak turun). Tidak ada pasangan GAGAL.

## Tabel percobaan penyelarasan (Step 4)

Nilai awal (MinCoverage 0.5, TypoMinSimilarity 0.4, bobot B 0.5) sudah memenuhi K1, sehingga nilai di `rank.go` TIDAK diubah.
Grid 18 kombinasi dijalankan pada salinan sementara worktree (rank.go asli tidak disentuh).

| MinCoverage | TypoMinSimilarity | Bobot B | recall@1 | recall@5 | MRR | GAGAL |
|---|---|---|---|---|---|---|
| 0.5 | 0.35 / 0.4 / 0.45 | 0.4 / 0.5 / 0.7 | 0.9875 | 1.0000 | 1.0000 | 0 (9 kombinasi identik) |
| 0.67 | 0.35 | 0.4 / 0.5 / 0.7 | 0.8375 | 0.8500 | 0.8500 | 6 |
| 0.67 | 0.4 / 0.45 | 0.4 / 0.5 / 0.7 | 0.8125 | 0.8250 | 0.8250 | 7 |

Kesimpulan: MinCoverage 0.5 mutlak diperlukan (0.67 menjatuhkan synonym ke 0.625 dan phrase ke 0.714 recall@5). Pada MinCoverage 0.5, hasil tidak sensitif terhadap TypoMinSimilarity 0.35 sampai 0.45 dan bobot B 0.4 sampai 0.7 di korpus ini; golden set tidak cukup besar untuk membedakannya, jadi nilai awal dipertahankan.

## Catatan

- Skor 1.0 pada fixture 25 dokumen bukan klaim performa produksi; kamus ditulis oleh tim yang sama dengan golden set.
- Golden set tidak membedakan bobot B dan TypoMinSimilarity; bila korpus diperbesar, ulangi grid.
- Pengujian end-to-end (`TestSearchEval`, `TestT20_SynonymViaAPI`, `TestT20_SynonymAuthz`) kini berjalan pada wiring asli (`searchsynonym.Register`); dijalankan ulang 2026-10-27 dan seluruhnya lulus dengan angka K1 di atas.
