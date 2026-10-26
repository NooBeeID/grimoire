# T20 Search Recall: dekomposisi backend

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md)
Resource root: `backend/internal/domain/project/internal/knowledge` (module knowledge) dan module baru `backend/internal/domain/project/internal/searchsynonym`.

## Foundation (selesai dulu, satu agen, berurutan; semua path di `base/`, `base.go`, lib, migrasi, infra)
1. Task 3: `backend/lib/util/stemid/`.
2. Task 4: migrasi `backend/migrations/project/<VERSION>_knowledge_search_recall.sql`, `backend/internal/infra/db/migrate_searchrecall_integration_test.go`, ubah `backend/internal/infra/db/testing.go` dan `backend/cmd/internal/server/run_integration_test.go` (pg_trgm). Ukur durasi K7 dan catat di file `docs/superpowers/plans/search-recall/foundation-notes.md` (bahan tiket).
3. Task 5: migrasi `create_knowledge_synonym_groups`, `backend/internal/domain/project/internal/searchsynonym/base/` (group.go, reasons.go, errors.go, store, access, audit + tes), aksi audit di `backend/lib/contracts/core/audit/record/{actions.go,actions_test.go}`.
4. Task 8 (bagian kontrak): `backend/lib/contracts/project/searchsynonym/listgroups/listgroups.go` dan konstanta `BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS` di `backend/lib/constants/bus.go`.
5. Task 9: `backend/internal/domain/project/internal/knowledge/base/{query.go,plan.go,rank.go}` + tes (ParseQuery tetap ada sampai Task 12).
6. Task 10 (bagian entity): `backend/internal/domain/project/internal/knowledge/base/sections.go`, `SectionsJSON` di `base/file.go`, `base/store/write.go` menulis array `sections`.
7. Task 11: `SearchHit.Section` di `base/snippet.go`, `Store.SuggestTerms` dan `Store.Search` di `base/store/store.go` + tes integrasi + EXPLAIN (catat ke foundation-notes.md).
8. Task 12 (bagian base): `backend/internal/domain/project/internal/knowledge/base/searchrun/` (+test), `SearchItemDTO.Section`, `SearchResultDTO.Expanded`, `NewSearchResult(hits,total,expanded)` di snippet.go; kontrak `backend/lib/contracts/project/knowledge/search/search.go` (Item.Section, Response.Expanded). Hapus `ParseQuery` hanya setelah usecase search/bussearch pindah (dilakukan fase wiring).
Foundation harus lulus `go build ./...` dan tes unit/integrasi paketnya; bussearch/search lama tetap kompil (tambah, jangan hapus API lama yang dipakai mereka).

## Usecase (paralel setelah foundation)
- synonym-crud, synonym-bus, knowledge-write-sections, knowledge-search-orchestration, mcp-search-tool, openapi-contract.
- Late: search-eval (butuh semua jalan lewat wiring + REST), ticket-docs (butuh angka dari foundation-notes dan search-eval).

## Wiring (setelah usecase, oleh orkestrator)
`backend/internal/domain/project/internal/searchsynonym/base.go` (Register 5 feature + route), `backend/internal/domain/project/base.go` (`searchsynonym.Register(d)`), `backend/internal/domain/project/internal/knowledge/base.go` (`backfillsections.Init(d)`, searchrun dibuat di bussearch/search `Init`), hapus `ParseQuery`, jalankan `go build ./... && go test ./internal/archtest/`.

## Di luar lingkup dekomposisi backend
Frontend Task 15-17 (diplan terpisah, bergantung pada `api/modules/project/search-synonym.yml` dari usecase openapi-contract).
