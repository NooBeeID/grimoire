# knowledge-write-sections

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 10 (sisi non-base). Dependensi: `SectionsJSON` di file.go dan write.go dari foundation.

## Changes
- `backend/internal/domain/project/internal/knowledge/createfile/repository.go`, `backend/internal/domain/project/internal/knowledge/updatefile/repository.go` (teruskan `sections` dari entity File ke store)
- `backend/internal/domain/project/internal/knowledge/backfillsections/{init.go,usecase.go,repository.go,usecase_test.go,repository_integration_test.go}`
- `backend/internal/domain/project/internal/knowledge/knowledge_integration_test.go` (satu asersi per writer: create, update, upload, upsert-files menulis `sections` tidak NULL)

## Steps
1. Pastikan jalur uploadfiles/busupsertfiles sudah menulis sections lewat store (kemungkinan sudah dari foundation write.go); bila butuh perubahan di folder mereka, laporkan sebagai temuan, jangan sentuh (di luar allowedPaths).
2. backfillsections: job `knowledge.backfill-sections`, Every 1 menit, maks 10 batch x 200, `SetSections` dengan `WHERE sections IS NULL`. `Init(d)` mendaftarkan job (dipanggil di knowledge/base.go saat wiring).
## Tests
Task 10 Step 2 (`TestBackfillSections`, asersi writer). Review Focus 3 sudah diuji di foundation.
