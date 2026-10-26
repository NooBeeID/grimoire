# synonym-bus

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 8 (sisi handler). Dependensi: kontrak dan konstanta bus dari foundation.

## Changes
- `backend/internal/domain/project/internal/searchsynonym/buslistgroups/{init.go,handler.go,usecase.go,repository.go,handler_test.go,usecase_test.go,repository_integration_test.go}`

## Steps
1. Handler didaftarkan ke bus dengan `BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS` (`Init(d)` dipanggil base.go di fase wiring). Tipe request salah -> `bus.ErrBadRequestType`; ProjectID bukan UUID -> wrap `errs.ErrInvalid`.
2. Repository memakai `base/store.List` dan memetakan ke `[][]string` (urut created_at, id), selalu dibatasi project.
## Tests
Task 8 Step 1; jalankan juga `go test ./internal/archtest/`.
