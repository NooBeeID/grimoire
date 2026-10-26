# openapi-contract

Plan induk: docs/superpowers/plans/2026-10-26-search-recall.md (di checkout utama: /Users/ascendiz/Documents/Projects/noobeeid/research/knowledge-based/docs/superpowers/plans/2026-10-26-search-recall.md). Baca bagian Global Constraints, Keputusan, Review Focus, lalu Task yang disebut di bawah. Foundation (lihat overview.md) sudah selesai dan JANGAN diubah. Wiring (`base.go` module dan `project/base.go`) dikerjakan fase wiring, bukan di sini; uji unit/integrasi yang butuh wiring boleh memakai konstruktor `Init` langsung seperti pola feature lain. TDD: tes gagal dulu, lalu implement, commit per langkah plan. Jangan git push.

Task 2.

## Changes
- `api/modules/project/search-synonym.yml` (baru), `api/modules/project/knowledge.yml`, `api/root.yml`, `api/openapi.yml` (generated)

## Steps
Ikuti Task 2 (skema SynonymGroup, 4 operasi, daftar error reason, field aditif `section`/`expanded`, naik minor versi knowledge). `make api-bundle && make api-check` lulus. Kontrak ini dibaca frontend dan harus cocok dengan JSON wire handler di usecase synonym-crud.
