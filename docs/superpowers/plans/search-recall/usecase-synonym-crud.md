# synonym-crud

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 5 (sisi usecase), 6, 7. Dependensi: foundation `backend/internal/domain/project/internal/searchsynonym/base/` (NormalizeTerms, Store, access, audit, errors).

## Changes
- `backend/internal/domain/project/internal/searchsynonym/listgroups/{init.go,handler.go,usecase.go,repository.go,handler_test.go,usecase_test.go}`
- `backend/internal/domain/project/internal/searchsynonym/creategroup/{init.go,handler.go,params.go,usecase.go,repository.go,handler_test.go,usecase_test.go,repository_integration_test.go}`
- `backend/internal/domain/project/internal/searchsynonym/updategroup/*`, `backend/internal/domain/project/internal/searchsynonym/deletegroup/*` (pola sama; tanpa params.go untuk delete)
- `backend/internal/domain/project/internal/searchsynonym/routes_test.go` (route GET/POST/PUT/DELETE terdaftar; tanpa auth 401). Tes ini membangun route lewat Init feature, bukan base.go; bila butuh base.go, tulis tes dengan build tag dan jalankan di fase wiring.

## Steps
1. Ikuti Task 6 dan 7 apa adanya (kontrak Usecase.Execute, urutan access -> normalize -> InTx{LockProject; Count; TermsInUse; Insert/Update/Delete} -> audit setelah commit).
2. Route pola penuh di `d.API`: `/projects/{projectID}/search/synonyms[/{groupID}]`, `middleware.RequireAuth()`, `BodyLimit(8<<10)` untuk POST/PUT. `init.go` tiap feature mengekspos fungsi `Init(d)` yang dipanggil base.go (fase wiring).
3. Respons list `{items:[]}` tidak pernah null; error pakai fungsi dari base/errors.go.

## Tests
Task 6 Step 1-2 (termasuk `TestCreateGroup_ConcurrentSameTerm`, Review Focus 4) dan Task 7 Step 1. Jalankan `cd backend && go test ./internal/domain/project/internal/searchsynonym/...` dan tag integration dengan `TEST_DATABASE_URL=postgres://grimoire:grimoire@localhost:5433/grimoire_test?sslmode=disable`.
