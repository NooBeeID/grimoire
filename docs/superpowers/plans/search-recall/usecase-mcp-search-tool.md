# mcp-search-tool

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 13. Dependensi: `search.Item.Section` dan `search.Response.Expanded` dari foundation.

## Changes
- `backend/internal/domain/discovery/internal/knowledgetools/base/searching.go` dan `searching_test.go` (hanya dua file ini di folder base module tersebut)
- `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/{init.go,summary_test.go,snapshot_test.go}` dan `searchknowledge/testdata/`
- `backend/cmd/internal/server/knowledgetools_searchknowledge_integration_test.go` (hanya jika asersi lama gagal akibat field baru)

## Steps
Ikuti Task 13: `SearchHits.Expanded`; Structured() menyertakan `section` hanya bila tak kosong; Text() menambah baris "   bagian: ..." dan baris penutup read_section; Summary() menambah `expanded: true` hanya bila true; deskripsi tool diperbarui; skema argumen tidak berubah. Regenerasi golden dengan flag update snapshot_test.go dan tinjau diff.
## Tests
Task 13 Step 1.
