# search-eval

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 14. LATE: butuh seluruh stack (wiring selesai, REST kamus, search baru). Dependensi: semua usecase lain + wiring.

## Changes
- `backend/testdata/searcheval/` (synonyms.yaml baru, baseline.json)
- `backend/lib/util/rankeval/{synonyms.go,synonyms_test.go}`
- `backend/cmd/internal/server/searchsynonym_integration_test.go` (baru), `backend/cmd/internal/server/searcheval_integration_test.go`
- `docs/search-quality.md`
- `backend/internal/domain/project/internal/knowledge/base/rank.go` (HANYA file ini di base/: nilai konstanta hasil penyelarasan Step 4; jangan ubah struktur/signature)

## Steps
Ikuti Task 14 Step 1-6. Target K1 tidak boleh diturunkan; bila tidak tercapai, catat angka jujur dan pasangan GAGAL di `docs/superpowers/plans/search-recall/eval-notes.md` (bahan tiket, dibaca usecase ticket-docs). Tabel percobaan penyelarasan juga ke file itu. Pesan commit memuat angka sebelum/sesudah.
## Tests
`make test-search-eval`, `TestT20_SynonymViaAPI`, `TestT20_SynonymAuthz`, `TestSynonymsFixture`.
