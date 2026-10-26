# knowledge-search-orchestration

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 12 (sisi usecase). Dependensi: `base/searchrun`, kontrak search, DTO di foundation.

## Changes
- `backend/internal/domain/project/internal/knowledge/bussearch/{init.go,usecase.go,usecase_test.go}`
- `backend/internal/domain/project/internal/knowledge/search/{init.go,usecase.go,usecase_test.go,handler_test.go}`

## Steps
1. Usecase: validasi -> `base.ParseTerms` (kosong -> hasil kosong tanpa memanggil Runner) -> prefix/limit -> `searchrun.Runner.Run` -> map ke Response/DTO (Section, Expanded). `init.go` membuat `searchrun.New(store, searchrun.NewBusSynonyms(d.Bus), d.Log)`.
2. Ganti pemakaian `ParseQuery`/Store.Search lama; tetap lolos `go build ./...`. Penghapusan `ParseQuery` di query.go dilakukan fase wiring.
## Tests
Task 12 Step 1 (usecase: Section diteruskan, query kosong tidak memanggil Runner; handler HTTP JSON memuat `section` dan `expanded`). Periksa tes lama bussearch/search masih relevan dan perbarui.
