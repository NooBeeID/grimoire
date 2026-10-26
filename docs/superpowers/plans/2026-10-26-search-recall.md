# T20 Search Recall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `search_knowledge` dan pencarian knowledge HTTP menemukan dokumen lewat sinonim (kamus per project), salah ketik, variasi imbuhan Indonesia/Inggris, dengan urutan judul/path/heading lebih kuat, dan cuplikan dari bagian heading yang paling cocok (field `section`).

**Architecture:** Semua di Postgres 16. `knowledge_files` mendapat kolom `sections` (jsonb hasil `mdoutline` di Go), dua tsvector generated (`search` config `simple` dengan bobot baru, `search_stem` config `indonesian` + `english`), dan tabel kosakata `knowledge_terms` (dipelihara trigger, indeks trigram) untuk koreksi typo. Query user diurai Go menjadi `SearchPlan` (kelompok term + alternatif: sinonim, varian peluluhan K/P/T/S, kandidat typo) dan dikirim ke SQL hanya sebagai array parameter; tsquery dibangun di SQL dengan `phraseto_tsquery`. Module baru `project/searchsynonym` memiliki tabel `knowledge_synonym_groups`, CRUD REST, dan bus `project.searchsynonym.list-groups` yang dipanggil knowledge.

**Tech Stack:** Go 1.x + sqlx + pgx, Postgres 16 (`pg_trgm` 1.6, snowball `indonesian`/`english`), Svelte 5 + Zod + Vitest, OpenAPI bundling (`make api-bundle`).

**Spec:** `docs/superpowers/specs/2026-10-26-search-recall-design.md` (T20). Bergantung pada T18 (`backend/lib/util/mdoutline`, plan `docs/superpowers/plans/2026-10-26-mcp-outline-tools.md`).

## Global Constraints

- Hanya Postgres, tanpa layanan/model eksternal, tanpa embedding/pgvector. Tanpa pencarian lintas project, tanpa terjemahan lintas bahasa, tanpa sinonim otomatis.
- Argumen `search_knowledge` tidak berubah; hasil hanya bertambah field opsional `section`. Aksi audit tetap `mcp.search_knowledge`; `result.summary` boleh bertambah `expanded` (hanya ditulis bila `true`).
- Input user tidak pernah dieksekusi sebagai operator tsquery atau pola trigram: hanya teks term (huruf/angka/`_`/spasi) sebagai parameter ke `phraseto_tsquery` / `similarity`.
- Kamus sinonim tidak bisa diubah agent (tidak ada tool MCP baru); hanya REST manusia di `/api/projects/{projectID}/search/synonyms`, anggota project (level member/owner/admin; non-anggota 404).
- Kelompok sinonim: 2 sampai 10 term, huruf kecil, unik dalam project. Target K1 di korpus fixture: recall@5 keseluruhan >= 0.80, `typo` recall@5 >= 0.50, `synonym` recall@5 >= 0.50, MRR >= 0.70, `exact-name` dan `phrase` tidak turun lebih dari toleransi 0.02. Bila tidak tercapai: laporkan angka, jangan turunkan target.
- Migrasi forward-only dan idempoten, nama `<VERSION>_<nama>.sql` dengan VERSION di-generate saat implement (`.claude/rules/migrations.md`). `CREATE EXTENSION` memakai `IF NOT EXISTS` dan gagal dengan pesan jelas bila tak berizin.
- Format snippet lama dipertahankan: bus/MCP `**term**`, HTTP segmen `{text, highlight}`.
- Teks UI lewat i18n `id` dan `en` (kunci datar di `frontend/src/shared/i18n/{id,en}.json`). Komponen baru memakai `shared/components` dan token `DESIGN.md` (fully rounded, glass hanya kerangka/dialog, satu tombol ink per layar).
- Setiap commit diakhiri baris `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Kerjakan di branch `feat/t20-search-recall`.
- Selesai = `make lint`, `make test`, `make test-integration`, `make test-search-eval`, `make api-check`, `cd frontend && npm run check && npm test -- --run && npm run test:arch` lulus.

## Fakta lingkungan (diperiksa 2026-10-26 di `postgres:16`, PostgreSQL 16.15)

Perintah: `docker compose -f docker-compose.dev.yml exec -T db psql -U grimoire -d postgres -c "..."`.

- `select cfgname from pg_ts_config` -> 29 config, **termasuk `indonesian` dan `english`** (snowball bawaan).
- `select name from pg_available_extensions where name='pg_trgm'` -> **`pg_trgm` 1.6 tersedia**. Role `grimoire` superuser di dev; CI (`.github/workflows/ci.yml`) juga `postgres:16`. Sejak PG13 `pg_trgm` adalah trusted extension (pemilik DB berhak CREATE cukup).
- `to_tsvector('indonesian', 'mengirim dikirim pengiriman kirimkan')` -> `'irim':1,3 'kirim':2,4`; `menulis` -> `'ulis'`, `tulisan` -> `'tulis'`; `memakai`/`pakai` -> `'paka'`; `menyimpan`/`penyimpanan` -> `'simpan'`; `menggunakan`/`pengguna` -> `'guna'`. Jadi snowball Indonesia **tidak memulihkan peluluhan k/t** (meng+k, men+t). Config `indonesian` di PG **tidak punya stopword** (`dan`, `yang` tetap lexeme).
- `to_tsvector('english', 'deploy deployment deploys deployed')` -> `'deploy':1,2,3,4`.
- `similarity()` (DB sementara, sudah di-drop): pagnation/pagination 0.615, rollbak/rollback 0.545, loggin/login 0.625, retrry/retry 0.625, enviroment/environment 0.643, autentkasi/autentikasi 0.643, limt/limit 0.375, kyes/keys 0.111.
- `tsquery_or(tsquery,tsquery)` ada (dasar operator `||`), `tsquery_or('a','')` = `'a'` (dengan NOTICE). `to_tsvector(regconfig, jsonb)` IMMUTABLE (boleh di kolom generated; hanya nilai string yang diindeks).
- Runner migrasi (`backend/internal/infra/db/migrate.go:192`) menjalankan tiap file dalam satu transaksi -> `CREATE INDEX CONCURRENTLY` tidak mungkin.
- Tes integrasi memakai schema per tes dengan `search_path=<schema>,public`; ekstensi dipasang di `public` lewat `ensureTestExtensions` (`backend/internal/infra/db/testing.go:72`) dan `run_integration_test.go:59` (saat ini hanya `citext`).

## Keputusan (K3/K4 dan turunannya)

- **K3 (stemming): pakai snowball `indonesian` + `english` di indeks** (kolom generated `search_stem`), karena keduanya tersedia di `postgres:16`. Celah peluluhan k/p/t/s ditutup sisi query oleh fungsi murni `backend/lib/util/stemid.Variants` (mis. `kirim` -> `mengirim`, `pengirim`; `menulis` -> `tulis`), dan varian itu dilewatkan ke `phraseto_tsquery('indonesian', ...)` sehingga lexeme `irim`/`kirim` keduanya tercakup. Tidak ada tabel term ternormalisasi untuk stem. Varian hanya masuk query stem, tidak ke query `simple`, dan tidak menyalakan `expanded`.
- **K4 (typo): `pg_trgm`.** Kosakata project = tabel `knowledge_terms(project_id, file_id, term)` berisi lexeme `simple` dari `search` yang berbobot A/B (path, judul, heading) atau muncul >= 2 kali di isi, dipelihara trigger `AFTER INSERT OR UPDATE OF path, title, content, sections` (backfill di migrasi), indeks `gin (term gin_trgm_ops)`. Kandidat typo dicari hanya untuk kata tunggal tanpa sinonim, >= `TypoMinRunes` rune, bukan angka saja, yang tidak cocok satu dokumen pun (cek `@@` pada kedua tsvector). Konstanta awal: `TypoMinSimilarity = 0.4` (>= ambang default operator `%` 0.3 sehingga indeks terpakai), `TypoMaxCandidates = 3`, `TypoMinRunes = 4`. `kyes`/`limt` sengaja tidak tertolong trigram; tertolong aturan cakupan (D3).
- **D1 Heading untuk indeks dan cuplikan:** kolom `sections jsonb` (`[{"h":heading,"s":runeStart,"e":runeEnd}]`) dihitung Go di entity `File` (seperti `Title`), ditulis oleh semua writer. Baris lama `sections IS NULL` diisi job `knowledge.backfill-sections` (tiap menit, batch 200) sehingga migrasi tidak perlu Go dan tulisan tidak terblokir lama. Selama NULL, dokumen tetap tercari; cuplikan memakai seluruh isi dan `section = ""`.
- **D2 Pembobotan (K5):** `search` = path+judul `A`, heading `B`, isi `D`. Skor = `ts_rank_cd(W, search, qs, 1) + StemFactor * ts_rank_cd(W, search_stem, qt, 1) + PhraseBoost` (bila ada alternatif multi-kata yang cocok sebagai frasa di judul/heading). `W = RankWeights` dikirim sebagai parameter dari konstanta Go dan diselaraskan dengan golden set.
- **D3 Cakupan:** dokumen cocok bila cocok dengan minimal `ceil(n * MinCoverage)` dari n kelompok term (MinCoverage awal 0.5). Urutan: jumlah kelompok cocok DESC, skor DESC, path. Dokumen yang memuat semua term selalu di atas yang memuat sebagian (menjaga presisi).
- **D4 Sinonim lewat bus, fail-soft:** knowledge memanggil `project.searchsynonym.list-groups`; bila error, log warn dan cari tanpa sinonim. Pencocokan sinonim (multi-kata, dua arah) adalah rule entity knowledge (`NewSearchPlan`), normalisasi term milik entity searchsynonym.
- **D5 Audit kamus:** aksi baru `search_synonym.created`, `search_synonym.updated`, `search_synonym.deleted` (konten manusia, pola sama dengan `knowledge.file.*`, fail-open). `mcp.search_knowledge` tidak berubah selain `expanded`.
- **D6 Trigger:** trigger `knowledge_terms_refresh` adalah skema milik module knowledge (setara kolom generated); kode Go tetap hanya menyentuh SQL lewat repository/store.
- **K7 risiko:** `ALTER TABLE ... ADD COLUMN ... GENERATED STORED` menulis ulang `knowledge_files` di bawah `ACCESS EXCLUSIVE` dalam transaksi migrasi (CONCURRENTLY tidak mungkin). Migrasi berjalan saat start sebelum server melayani. Durasi diukur di Task 4 dan dicatat di tiket.

## Review Focus

1. Query berisi hanya operator/tanda baca (`-`, `!`, `:*`, `&|`, `"`, `''`), atau stopword Inggris saja (`the`): tidak ada error SQL, hasil kosong atau wajar. Tes: Task 9 (`TestParseTerms_OperatorsInert`) dan Task 11 (`TestSearch_OperatorOnlyAndStopword`).
2. Konten dengan rune multi-byte (emoji, aksara non-Latin) sebelum heading: offset `sections` harus rune, sehingga `substr` SQL memotong tepat di heading. Tes: Task 10 (`TestSections_RuneOffsets`) dan Task 11 (`TestSearch_SectionSnippetMultibyte`).
3. Baris lama dengan `sections IS NULL` (sebelum backfill) tetap muncul dengan snippet dan `section = ""`. Tes: Task 11 (`TestSearch_NullSections`).
4. Dua POST bersamaan dengan term yang sama di satu project: tepat satu sukses, lainnya 409 `term_in_use`. Tes: Task 6 (`TestCreateGroup_ConcurrentSameTerm`, integrasi).
5. Query kata sangat panjang (200 rune) atau aksara non-Latin: tidak masuk kandidat typo bila > 64 rune, tanpa panic. Tes: Task 9 (`TestSearchPlan_TypoCandidatesBounds`).

---

## File Structure

Backend baru:
- `backend/lib/util/stemid/stemid.go` (+`_test.go`): `Variants` peluluhan K/P/T/S.
- `backend/migrations/project/<VERSION>_knowledge_search_recall.sql`: pg_trgm, agregat, `sections`, `search`, `search_stem`, `knowledge_terms` + trigger.
- `backend/migrations/project/<VERSION>_create_knowledge_synonym_groups.sql`.
- `backend/internal/domain/project/internal/searchsynonym/`
  - `base.go` (wiring), `routes_test.go`
  - `base/{group.go,errors.go,reasons.go}` (+tests), `base/access/access.go`, `base/audit/audit.go`, `base/store/store.go` (+integration test)
  - `listgroups/`, `creategroup/`, `updategroup/`, `deletegroup/`, `buslistgroups/` (masing-masing `init.go handler.go [params.go] usecase.go repository.go *_test.go`)
- `backend/lib/contracts/project/searchsynonym/listgroups/listgroups.go`
- `backend/internal/domain/project/internal/knowledge/base/{plan.go,rank.go,sections.go}` (+tests), `base/searchrun/searchrun.go` (+test)
- `backend/internal/domain/project/internal/knowledge/backfillsections/{init.go,usecase.go,repository.go,*_test.go}`
- `backend/testdata/searcheval/synonyms.yaml`, `backend/lib/util/rankeval/synonyms.go` (+test)
- `backend/cmd/internal/server/searchsynonym_integration_test.go`

Backend diubah:
- `knowledge/base/query.go` (`ParseQuery` -> `ParseTerms`), `base/snippet.go` (Section, Expanded), `base/file.go` (`SectionsJSON`), `base/store/{store.go,write.go}`, `createfile/repository.go`, `updatefile/repository.go`, `bussearch/*`, `search/*`, `knowledge/base.go`
- `lib/contracts/project/knowledge/search/search.go` (Item.Section, Response.Expanded)
- `discovery/internal/knowledgetools/base/searching.go`, `searchknowledge/init.go` (deskripsi), `searchknowledge/testdata/*.golden`
- `lib/constants/bus.go`, `lib/contracts/core/audit/record/{actions.go,actions_test.go}`, `internal/domain/project/base.go`
- `internal/infra/db/testing.go`, `cmd/internal/server/run_integration_test.go`, `cmd/internal/server/searcheval_integration_test.go`, `testdata/searcheval/baseline.json`
- `api/modules/project/{knowledge.yml,search-synonym.yml(baru)}`, `api/root.yml`, `api/openapi.yml` (generated)
- `docs/tickets/T20-project-search-recall.md` (baru), `docs/tickets/README.md`, `rules.txt` (2.1), `docs/search-quality.md`

Frontend baru (`frontend/src/domain/project/search-synonym/`):
- `listgroups/{types.ts,index.ts,services/listgroups.service.ts,stores/groupList.store.ts,components/SynonymGroupsTable.svelte,views/ListGroupsView.svelte,mocks/groups.ts,*.test.ts}`
- `savegroup/{types.ts,index.ts,model/terms.ts,services/savegroup.service.ts,components/TermsInput.svelte,views/SaveGroupDialog.svelte,*.test.ts}`
- `deletegroup/{index.ts,services/deletegroup.service.ts,views/DeleteGroupDialog.svelte,*.test.ts}`
- `frontend/src/app/views/workspace/SearchTab.svelte` (+`SearchTab.test.ts`)

Frontend diubah: `app/workspace/tabs.ts`, `shared/i18n/{id,en}.json`, `domain/project/knowledge/search/{types.ts,components/SearchResults.svelte,components/components.test.ts}`.

---

## Task 1: Tiket T20 dan katalog

**Files:**
- Create: `docs/tickets/T20-project-search-recall.md`
- Modify: `docs/tickets/README.md` (tabel tiket, tree dependensi, struktur module `project/internal/{...,searchsynonym}`, katalog bus, katalog aksi audit), `rules.txt` bagian 2.1 (`project : management, apikey, knowledge, apispec, gitsync, searchsynonym`)

**Interfaces:**
- Produces: nama yang dipakai semua task: bus `project.searchsynonym.list-groups` (`BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS`), aksi `search_synonym.created|updated|deleted`, route `/api/projects/{projectID}/search/synonyms[/{groupID}]`.

- [ ] **Step 1:** Tulis tiket dengan format T17 (Ringkasan, Tujuan, Non-goals, Anti-tujuan, Keputusan pemilik, Keputusan pembangun K1-K8 dari spec + D1-D6 + "Fakta lingkungan" dari plan ini, Features per folder, Kontrak bus, Aturan bisnis, UI, Titik sentuh, Kriteria penerimaan bernomor `T20-AC1..AC8` dari spec, Risiko K7, bagian "EXPLAIN" kosong yang diisi Task 11).
- [ ] **Step 2:** README: baris T20 (`project`, `knowledge (pencarian) + searchsynonym (kamus sinonim)`, L, `T05, T08, T11, T18`, `Dalam pengerjaan`); katalog bus `project.searchsynonym.list-groups | Call | T20 | T20 (knowledge bussearch, search)` + paragraf kontrak; katalog audit baris "Kamus pencarian (T20)"; catat field aditif `section`/`expanded` di paragraf kontrak T05.
- [ ] **Step 3:** Commit `docs(tickets): T20 search recall` .

## Task 2: OpenAPI

**Files:**
- Create: `api/modules/project/search-synonym.yml`
- Modify: `api/modules/project/knowledge.yml` (skema hasil search), `api/root.yml`, `api/openapi.yml` (generated)

**Interfaces:**
- Produces (JSON wire, dipakai Task 6-8, 12, 15-17):
  - `SynonymGroup {id: uuid, terms: string[2..10], created_at, updated_at: date-time, created_by_label, updated_by_label: string}`
  - `GET /api/projects/{projectID}/search/synonyms` -> 200 `{items: SynonymGroup[]}` (urut `created_at, id`)
  - `POST .../search/synonyms` body `{terms: string[]}` -> 201 `SynonymGroup`
  - `PUT .../search/synonyms/{groupID}` body `{terms: string[]}` -> 200 `SynonymGroup`
  - `DELETE .../search/synonyms/{groupID}` -> 204
  - Error: 400 `INVALID_ARGUMENT` `details.reason` in `too_few_terms|too_many_terms|term_empty|term_too_long|term_invalid_char|duplicate_term|invalid_group_id`; 409 `CONFLICT` `details.reason` `term_in_use` (+`details.terms: string[]`) atau `group_limit`; 404 `NOT_FOUND` (project bukan milik / grup tak ada); 401. Mutasi butuh header CSRF.
  - Knowledge `SearchItem` + `section: string` (kosong bila di luar heading), `SearchResult` + `expanded: boolean`. `info.version` knowledge naik minor.

- [ ] **Step 1:** Tulis kedua file, tambah 2 `$ref` path di `api/root.yml` (pola `~1api~1projects~1{projectID}~1search~1synonyms`).
- [ ] **Step 2:** Run `make api-bundle && make api-check`. Expected: lulus, `api/openapi.yml` berubah.
- [ ] **Step 3:** Commit `docs(api): kontrak kamus sinonim dan field section/expanded (T20)`.

## Task 3: `lib/util/stemid`

**Files:**
- Create: `backend/lib/util/stemid/stemid.go`, `backend/lib/util/stemid/stemid_test.go`

**Interfaces:**
- Produces: `func Variants(word string) []string` — input huruf kecil; mengembalikan bentuk alternatif (tanpa kata asli), unik, urutan deterministik; `nil` bila < 4 rune atau tak ada aturan.

- [ ] **Step 1: Tes gagal** `TestVariants` table-driven:
  - `mengirim` -> berisi `kirim`; `pengiriman` -> berisi `kiriman`; `menulis` -> berisi `tulis`; `memakai` -> berisi `pakai`; `menyimpan` -> berisi `simpan` dan TIDAK berisi `tyimpan`
  - `kirim` -> berisi `mengirim`, `pengirim`; `tulis` -> berisi `menulis`, `penulis`; `pakai` -> `memakai`, `pemakai`; `simpan` -> `menyimpan`, `penyimpan`
  - `api`, `deploy`, `""` -> `nil`; hasil tidak pernah memuat kata asli.
- [ ] **Step 2:** `cd backend && go test ./lib/util/stemid/` -> FAIL (undefined).
- [ ] **Step 3:** Implement. Aturan (cek `meny`/`peny` sebelum `men`/`pen`, vokal = `aiueo`): `meng|peng`+vokal -> `k`+sisa; `mem|pem`+vokal -> `p`+sisa; `men|pen`+vokal -> `t`+sisa; `meny|peny`+vokal -> `s`+sisa; arah balik: `k|p|t|s`+vokal -> dua bentuk berawalan `meng/peng`, `mem/pem`, `men/pen`, `meny/peny` dengan huruf awal dilebur. Tanpa import selain stdlib (rules 4.9).
- [ ] **Step 4:** Test PASS. **Step 5:** Commit `feat(stemid): varian peluluhan K/P/T/S untuk query (T20 K3)`.

## Task 4: Migrasi knowledge search recall

**Files:**
- Create: `backend/migrations/project/<VERSION>_knowledge_search_recall.sql`, `backend/internal/infra/db/migrate_searchrecall_integration_test.go`
- Modify: `backend/internal/infra/db/testing.go:74` (daftar ekstensi `{"citext", "pg_trgm"}`), `backend/cmd/internal/server/run_integration_test.go:59` (juga `pg_trgm`)

**Interfaces:**
- Produces (skema): kolom `knowledge_files.sections jsonb NULL`; `search tsvector` (dibuat ulang), `search_stem tsvector`; agregat `knowledge_tsquery_or_agg(tsquery)`; tabel `knowledge_terms(project_id uuid NOT NULL, file_id uuid NOT NULL REFERENCES knowledge_files(id) ON DELETE CASCADE, term text NOT NULL, PRIMARY KEY (file_id, term))`; indeks `knowledge_files_search_gin`, `knowledge_files_search_stem_gin`, `knowledge_files_sections_null` (partial `WHERE sections IS NULL`), `knowledge_terms_term_trgm` (`gin (term gin_trgm_ops)`), `knowledge_terms_project_term` (`(project_id, term)`).

- [ ] **Step 1: Tes gagal** (tag `integration`, pakai `migratedDB`):
  - `TestSearchRecallMigration_Columns`: insert file (konten `# Pengiriman\nmengirim deployment`, path `ops/kirim.md`) -> `search @@ to_tsquery('simple','pengiriman')`, `search_stem @@ to_tsquery('indonesian','kirim') || ...'irim'`, `search_stem @@ to_tsquery('english','deploy')` true.
  - `TestSearchRecallMigration_Weights`: `ts_rank_cd` dengan bobot `{0,0,0,1}` > 0 untuk term di path/judul, = 0 untuk term hanya di isi; set `sections='[{"h":"Rollback cepat","s":0,"e":10}]'` -> term `rollback` berbobot B.
  - `TestSearchRecallMigration_TermsTrigger`: setelah insert, `knowledge_terms` berisi lexeme path/judul dan lexeme isi yang muncul >= 2 kali, tidak berisi lexeme isi sekali muncul; UPDATE content mengganti isi tabel; DELETE file mengosongkan baris file itu.
  - `TestSearchRecallMigration_OrAgg`: `SELECT knowledge_tsquery_or_agg(q) FROM (VALUES ('a'::tsquery),('b'::tsquery)) v(q)` = `'a' | 'b'`.
  - `TestSearchRecallMigration_Idempotent`: menjalankan isi file migrasi dua kali di schema yang sama tidak error.
- [ ] **Step 2:** `make dev-test-db && cd backend && TEST_DATABASE_URL=postgres://grimoire:grimoire@localhost:5433/grimoire_test?sslmode=disable go test -tags integration ./internal/infra/db/ -run SearchRecall` -> FAIL.
- [ ] **Step 3:** Generate VERSION (`python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"`), tulis migrasi berurutan:
  1. `DO $$ BEGIN CREATE EXTENSION IF NOT EXISTS pg_trgm; EXCEPTION WHEN insufficient_privilege THEN RAISE EXCEPTION 'T20: ekstensi pg_trgm wajib. Minta pemilik database/superuser menjalankan: CREATE EXTENSION pg_trgm;'; END $$;`
  2. `CREATE OR REPLACE AGGREGATE knowledge_tsquery_or_agg(tsquery) (SFUNC = tsquery_or, STYPE = tsquery);` (tanpa INITCOND; sfunc strict).
  3. `ADD COLUMN IF NOT EXISTS sections jsonb NULL`; `DROP INDEX IF EXISTS knowledge_files_search_gin`; `DROP COLUMN IF EXISTS search`; tambah ulang:
     ```sql
     search tsvector GENERATED ALWAYS AS (
       setweight(to_tsvector('simple', translate(path, '/._-', '    ')) || to_tsvector('simple', title), 'A') ||
       setweight(to_tsvector('simple', coalesce(sections, '[]'::jsonb)), 'B') ||
       setweight(to_tsvector('simple', left(content, 200000)), 'D')) STORED
     ```
     dan `search_stem` dengan pola sama memakai `'indonesian'` dan `'english'` (enam `to_tsvector`, digabung `||`, bobot A/B/D sama).
  4. Indeks GIN kedua kolom + partial index `sections IS NULL`.
  5. Tabel `knowledge_terms`, indeks, fungsi plpgsql `knowledge_terms_refresh()` (DELETE baris `file_id = NEW.id`; INSERT `SELECT NEW.project_id, NEW.id, u.lexeme FROM unnest(NEW.search) u WHERE char_length(u.lexeme) BETWEEN 3 AND 64 AND (u.weights && ARRAY['A','B']::text[] OR cardinality(u.positions) >= 2) ON CONFLICT DO NOTHING`; `RETURN NULL`), `DROP TRIGGER IF EXISTS` + `CREATE TRIGGER knowledge_terms_refresh AFTER INSERT OR UPDATE OF path, title, content, sections ON knowledge_files FOR EACH ROW EXECUTE FUNCTION knowledge_terms_refresh()`; backfill `INSERT ... SELECT` dari semua baris dengan filter sama, `ON CONFLICT DO NOTHING`.
  Komentar kepala file: T20, forward-only, risiko K7 (rewrite tabel, ACCESS EXCLUSIVE).
- [ ] **Step 4:** Test PASS; `ls backend/migrations/*/ | cut -c1-17 | sort | uniq -d` kosong.
- [ ] **Step 5:** Ukur: seed 10.000 file sintetis 8 KB di DB tes, jalankan ulang bagian ALTER dalam transaksi, catat durasi di tiket bagian Risiko.
- [ ] **Step 6:** Commit `feat(knowledge): migrasi search recall (pg_trgm, search_stem, sections, knowledge_terms)`.

## Task 5: Module searchsynonym: migrasi, entity, store

**Files:**
- Create: `backend/migrations/project/<VERSION>_create_knowledge_synonym_groups.sql`, `searchsynonym/base/{group.go,reasons.go,errors.go,group_test.go,errors_test.go}`, `searchsynonym/base/store/{store.go,store_integration_test.go}`, `searchsynonym/base/access/{access.go,access_test.go}`, `searchsynonym/base/audit/{audit.go,audit_test.go}`
- Modify: `backend/lib/contracts/core/audit/record/{actions.go,actions_test.go}`

**Interfaces:**
- Consumes: bus `project.management.check-access`, `project.management.get-project`, `core.audit.record` (salin pola `knowledge/base/access` dan `knowledge/base/audit`, tanpa import knowledge).
- Produces (package `searchsynonym/base`):
  ```go
  const (MinTerms = 2; MaxTerms = 10; MaxTermRunes = 64; MaxGroupsPerProject = 500)
  type Group struct { ID, ProjectID string; Terms []string; CreatedAt, UpdatedAt time.Time
      CreatedBy, CreatedByLabel, UpdatedBy, UpdatedByLabel string }
  type GroupDTO struct { ID string `json:"id"`; Terms []string `json:"terms"`; CreatedAt time.Time `json:"created_at"`
      UpdatedAt time.Time `json:"updated_at"`; CreatedByLabel string `json:"created_by_label"`; UpdatedByLabel string `json:"updated_by_label"` }
  type Actor struct{ ID, Label, Role string }
  func NormalizeTerms(raw []string) ([]string, Reason)
  func NewGroup(projectID string, terms []string, a Actor) Group
  func (g Group) WithTerms(terms []string, a Actor) Group
  func (g Group) ToDTO() GroupDTO
  type Reason string // ReasonTooFewTerms "too_few_terms", ReasonTooManyTerms, ReasonTermEmpty, ReasonTermTooLong, ReasonTermInvalidChar, ReasonDuplicateTerm, ReasonInvalidGroupID, ReasonTermInUse "term_in_use", ReasonGroupLimit "group_limit"
  func Invalid(r Reason) *errs.Error            // 400 INVALID_ARGUMENT, wraps errs.ErrInvalid
  func TermInUse(terms []string) *errs.Error    // 409 CONFLICT details {reason, terms}
  func GroupLimit() *errs.Error                 // 409 CONFLICT details {reason: group_limit}
  var ErrProjectNotFound, ErrGroupNotFound, ErrUnauthenticated
  ```
  `base/store`: `New(*db.DB) *Store`; `InTx(ctx, func(*Store) error) error`; `LockProject(ctx, projectID) error` (`pg_advisory_xact_lock(hashtextextended('search-synonym:' || $1, 0))`); `List(ctx, projectID) ([]base.Group, error)`; `Get(ctx, projectID, id) (base.Group, bool, error)`; `Count(ctx, projectID) (int, error)`; `TermsInUse(ctx, projectID string, terms []string, exceptID string) ([]string, error)` (urut alfabet); `Insert(ctx, base.Group) (base.Group, error)`; `Update(ctx, base.Group) (base.Group, bool, error)`; `Delete(ctx, projectID, id string) (base.Group, bool, error)`. ID non-UUID -> not found tanpa query.
  Aksi `record.ActionSearchSynonymCreated = "search_synonym.created"`, `...Updated`, `...Deleted` (masuk `Actions()`).

- [ ] **Step 1: Tes gagal** `TestNormalizeTerms` (unit): `[" Login ", "Autentikasi"]` -> `["login","autentikasi"]`, reason `""`; `["Rate  Limit","batas permintaan"]` -> `"rate limit"` (spasi diringkas); `["a"]` -> `too_few_terms`; 11 term -> `too_many_terms`; `["login","LOGIN"]` -> `duplicate_term`; `["", "x"]` -> `term_empty`; term 65 rune -> `term_too_long`; `["a&b","c"]`, `["x:*","y"]` -> `term_invalid_char` (izin: huruf, angka, spasi, `-`, `_`). `TestErrors` memeriksa status/kode/details.
- [ ] **Step 2: Tes gagal** `TestStore_*` (integration): CRUD, `TermsInUse` mengabaikan `exceptID` dan project lain, `List` hanya project sendiri (rule 6.10), cascade hapus project menghapus grup.
- [ ] **Step 3:** Run unit + integration -> FAIL.
- [ ] **Step 4:** Implement migrasi (`knowledge_synonym_groups`: kolom spec K2 + `created_by uuid NOT NULL`, `updated_by uuid NOT NULL`, CHECK `cardinality(terms) BETWEEN 2 AND 10`, indeks `(project_id, created_at, id)` dan `gin (terms)`), entity, store, access (member/owner/admin -> actor; lainnya 404), audit (Args `{terms}`, Result `{terms_count}`, target type `synonym_group`, target = group id).
- [ ] **Step 5:** Test PASS. **Step 6:** Commit `feat(searchsynonym): tabel, entity, store, access, audit`.

## Task 6: Feature listgroups dan creategroup

**Files:**
- Create: `searchsynonym/listgroups/{init.go,handler.go,usecase.go,repository.go,handler_test.go,usecase_test.go}`, `searchsynonym/creategroup/{init.go,handler.go,params.go,usecase.go,repository.go,handler_test.go,usecase_test.go,repository_integration_test.go}`, `searchsynonym/base.go`, `searchsynonym/routes_test.go`
- Modify: `backend/internal/domain/project/base.go` (`searchsynonym.Register(d)`)

**Interfaces:**
- Consumes: Task 5.
- Produces: route `GET /projects/{projectID}/search/synonyms`, `POST /projects/{projectID}/search/synonyms` (`middleware.RequireAuth()`, `BodyLimit(8<<10)` untuk POST), dipasang di `d.API` dengan pola penuh.
  - `creategroup.Usecase.Execute(ctx, projectID string, in Input{Terms []string}) (base.GroupDTO, error)`: access -> NormalizeTerms -> InTx{LockProject; Count >= MaxGroupsPerProject -> GroupLimit; TermsInUse -> TermInUse; Insert} -> audit created (setelah commit).
  - `listgroups.Usecase.Execute(ctx, projectID) ([]base.GroupDTO, error)` -> respons `{items}` (tidak pernah null).

- [ ] **Step 1: Tes gagal** usecase dengan repo/access/audit palsu: create sukses memanggil audit sekali; reason invalid -> 400 tanpa sentuh repo; term dipakai grup lain -> 409 `term_in_use` dengan `terms`; 500 grup -> 409 `group_limit`; non-anggota -> 404. Handler: JSON rusak -> 400, `terms` bukan array -> 400, 201 + body DTO. List kosong -> `{"items":[]}`.
- [ ] **Step 2: Tes gagal** `TestCreateGroup_ConcurrentSameTerm` (integration): dua goroutine POST `["login","masuk"]` dan `["login","signin"]` -> tepat satu sukses, satu `TermInUse`.
- [ ] **Step 3:** Run -> FAIL. **Step 4:** Implement + `routes_test.go` (route terdaftar, tanpa auth -> 401).
- [ ] **Step 5:** Test PASS. **Step 6:** Commit `feat(searchsynonym): list dan create kelompok sinonim`.

## Task 7: Feature updategroup dan deletegroup

**Files:**
- Create: `searchsynonym/updategroup/*`, `searchsynonym/deletegroup/*` (pola Task 6)
- Modify: `searchsynonym/base.go`, `routes_test.go`

**Interfaces:**
- Produces: `PUT /projects/{projectID}/search/synonyms/{groupID}` -> 200 DTO; `DELETE ...` -> 204. Update: access -> validasi groupID UUID (`invalid_group_id`) -> NormalizeTerms -> InTx{LockProject; Get (404 bila tidak ada/project lain); TermsInUse(exceptID=groupID); Update} -> audit updated (Args `{terms}`, Result `{terms_count}`). Delete: access -> InTx{Delete} -> 404 bila tidak ada -> audit deleted.

- [ ] **Step 1: Tes gagal**: update mengganti terms dan `updated_by_label`; update dengan term miliknya sendiri tidak 409; grup project lain -> 404; delete dua kali -> kedua 404; audit dipanggil hanya bila sukses.
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement. **Step 4:** PASS. **Step 5:** Commit `feat(searchsynonym): update dan delete kelompok sinonim`.

## Task 8: Bus project.searchsynonym.list-groups

**Files:**
- Create: `backend/lib/contracts/project/searchsynonym/listgroups/listgroups.go`, `searchsynonym/buslistgroups/{init.go,handler.go,usecase.go,repository.go,handler_test.go,usecase_test.go,repository_integration_test.go}`
- Modify: `backend/lib/constants/bus.go`, `searchsynonym/base.go`

**Interfaces:**
- Produces:
  ```go
  // package listgroups (lib/contracts/project/searchsynonym/listgroups)
  // Provider T20 searchsynonym, consumer T20 knowledge. Tanpa cek akses: consumer sudah memverifikasi
  // project (HTTP check-access / API key). Query selalu dibatasi ProjectID.
  type Request struct{ ProjectID string }
  type Response struct{ Groups [][]string } // tiap kelompok term huruf kecil; urut created_at, id; nil/kosong bila tidak ada
  // ProjectID bukan UUID -> error wrap errs.ErrInvalid; tipe salah -> bus.ErrBadRequestType.
  const BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS = "project.searchsynonym.list-groups"
  ```
- [ ] **Step 1: Tes gagal**: handler tipe salah -> `bus.ErrBadRequestType`; ProjectID invalid -> `errors.Is(err, errs.ErrInvalid)`; integration: dua project, hanya grup project yang diminta kembali.
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement. **Step 4:** PASS + `go test ./internal/archtest/`. **Step 5:** Commit `feat(searchsynonym): bus list-groups`.

## Task 9: Entity SearchPlan (ParseTerms, sinonim, typo, varian)

**Files:**
- Modify: `knowledge/base/query.go` (ganti `ParseQuery` dengan `ParseTerms`), `knowledge/base/query_test.go`
- Create: `knowledge/base/plan.go`, `knowledge/base/plan_test.go`, `knowledge/base/rank.go`

**Interfaces:**
- Consumes: `stemid.Variants` (Task 3).
- Produces:
  ```go
  type Term struct { Words []string; Quoted bool } // Words huruf kecil, hanya rune kata (isWordRune)
  func ParseTerms(q string) ([]Term, bool)          // batas MaxQueryRunes=200, MaxQueryTerms=16 tetap
  type AltSource uint8 // AltOriginal, AltSynonym, AltTypo, AltVariant
  type Alt struct { Text string; Source AltSource } // Text = kata dipisah satu spasi
  type QueryGroup struct { Alts []Alt }             // Alts[0] = asli
  type SearchPlan struct { Groups []QueryGroup }
  func NewSearchPlan(terms []Term, synonyms [][]string) SearchPlan
  func (p SearchPlan) TypoCandidates() []string
  func (p SearchPlan) WithTypos(s map[string][]string) SearchPlan
  func (p SearchPlan) Expanded() bool               // true bila ada Alt AltSynonym atau AltTypo
  type PlanParams struct { Texts []string; Groups []int32; StemOnly []bool }
  func (p SearchPlan) Params() PlanParams           // satu baris per Alt; AltVariant -> StemOnly=true
  // rank.go (nilai awal; diselaraskan di Task 14)
  var RankWeights = [4]float32{0.1, 0.2, 0.5, 1.0} // urutan Postgres {D, C, B, A}
  const ( StemFactor = 0.5; PhraseBoost = 0.5; MinCoverage = 0.5
          TypoMinSimilarity = 0.4; TypoMaxCandidates = 3; TypoMinRunes = 4; TypoMaxRunes = 64
          MaxAltsPerGroup = 12; MaxPlanAlts = 64 )
  ```
  Aturan `NewSearchPlan`: term sinonim dinormalisasi sama dengan query (huruf kecil, dipecah `isWordRune`, sehingga `rate-limit` = `rate limit`). Term berkutip cocok bila seluruh katanya sama dengan satu term kamus. Deret kata tak berkutip dicocokkan serakah kiri-ke-kanan, kecocokan terpanjang dulu; deret yang cocok menjadi satu `QueryGroup` (asli = kata-kata digabung) dengan alternatif semua term lain di kelompoknya (`AltSynonym`). Kata sisa = grup sendiri. Setiap kata tunggal mendapat `AltVariant` dari `stemid.Variants`. Alternatif unik per grup, dipotong `MaxAltsPerGroup`, total `MaxPlanAlts` (alternatif asli tidak pernah dibuang). `TypoCandidates`: kata dari grup satu-kata yang hanya punya alternatif asli/varian, `TypoMinRunes <= runes <= TypoMaxRunes`, bukan angka semua, unik. `WithTypos` menambah `AltTypo` ke grup kata tersebut.

- [ ] **Step 1: Tes gagal** (unit, tanpa mock), nama sesuai AC:
  - `TestParseTerms_OperatorsInert` (T20-AC2): `"a & b | !c:* <-> 'd'"` -> Terms `[a],[b],[c],[d]` tanpa operator; `"-"`, `"!!"`, `""`, `"   "` -> `ok=false`; kutip tak tertutup jadi frasa.
  - `TestParseTerms_Long`: 300 rune -> dipotong 200, maks 16 term.
  - `TestSearchPlan_SynonymBothDirections` (T20-AC2): groups `[["login","autentikasi"]]`; query `login` -> grup alt `{login, autentikasi}`; query `autentikasi` -> `{autentikasi, login}`; `Expanded()==true`.
  - `TestSearchPlan_MultiwordSynonym`: groups `[["rollback","kembalikan versi"]]`, query `kembalikan versi lama` -> 2 grup: `{kembalikan versi, rollback}`, `{lama}`; query `"kembalikan versi"` -> 1 grup dengan `rollback`.
  - `TestSearchPlan_HyphenTerm`: groups `[["rate-limit","batas permintaan"]]`, query `rate limit` -> alt `batas permintaan`.
  - `TestSearchPlan_NoSynonyms`: `Expanded()==false`, varian `mengirim` untuk `kirim` ada dengan `StemOnly=true` di `Params()`.
  - `TestSearchPlan_TypoCandidatesBounds`: `rollbak` masuk; `api` (3 rune), `2026`, kata 65 rune, kata yang punya sinonim -> tidak; `WithTypos({"rollbak": {"rollback"}})` -> `Expanded()==true`.
  - `TestSearchPlan_Caps`: 16 term x kelompok 10 term -> `len(Params().Texts) <= MaxPlanAlts` dan setiap grup tetap punya alt asli.
- [ ] **Step 2:** `cd backend && go test ./internal/domain/project/internal/knowledge/base/ -run 'ParseTerms|SearchPlan'` -> FAIL.
- [ ] **Step 3:** Implement. `ParseQuery` tetap ada (pemanggil lama) dan baru dihapus di Task 12, supaya setiap commit hijau.
- [ ] **Step 4:** PASS. **Step 5:** Commit `feat(knowledge): SearchPlan dengan sinonim, varian, typo`.

## Task 10: Sections di entity dan jalur tulis + backfill

**Files:**
- Create: `knowledge/base/sections.go`, `knowledge/base/sections_test.go`, `knowledge/backfillsections/{init.go,usecase.go,repository.go,usecase_test.go,repository_integration_test.go}`
- Modify: `knowledge/base/file.go` (`SectionsJSON []byte` diisi `NewFile`/`WithContent`), `knowledge/base/store/write.go` (insert/update array `sections`), `knowledge/createfile/repository.go`, `knowledge/updatefile/repository.go`, `knowledge/base/store/store.go` (`fileRow`/`FileColumns` tidak perlu membaca `sections`), `knowledge/base.go` (`backfillsections.Init(d)`)

**Interfaces:**
- Consumes (T18, `backend/lib/util/mdoutline`): `type Heading struct { Level int; Text string; Line int; Start, End int }` (Start = offset byte awal baris heading, End = offset byte akhir bagian) dan `func Parse(content string) []Heading` (urut dokumen, ATX + Setext, abaikan fenced code/HTML). Bila nama di T18 berbeda, sesuaikan HANYA di `sections.go`.
- Produces:
  ```go
  const MaxSections = 300
  type Section struct { Heading string `json:"h"`; Start int `json:"s"`; End int `json:"e"` } // offset RUNE, [Start,End)
  func Sections(content string) []Section    // bagian = heading sampai heading berikutnya (level apa pun); pra-heading non-kosong -> Heading ""; > MaxSections: bagian terakhir diperpanjang ke akhir; tanpa heading -> satu bagian "" seluruh isi; isi kosong -> []
  func SectionsJSON(content string) []byte   // json.Marshal(Sections), "[]" untuk kosong
  ```
  `backfillsections`: job `jobs.Job{Name: "knowledge.backfill-sections", Every: time.Minute}`; usecase `Run(ctx) error` mengulang maks 10 batch: `repo.NullSections(ctx, 200) ([]Pending{ID, Content string}, error)` -> `repo.SetSections(ctx, id string, sections []byte) error` (`UPDATE ... SET sections=$2 WHERE id=$1 AND sections IS NULL`); berhenti bila batch kosong atau ctx selesai.

- [ ] **Step 1: Tes gagal** `TestSections`: Markdown dengan preamble, `#`, `##`, heading di dalam ``` fence (tidak dihitung), Setext; `TestSections_RuneOffsets` (Review Focus 2): `"😀😀\n# Judul\nisi"` -> bagian `Judul` `Start==3` (rune), `string([]rune(c)[s:e])` diawali `# Judul`; `TestSections_Cap`: 400 heading -> 300 bagian, bagian terakhir berakhir di `len([]rune(c))`.
- [ ] **Step 2: Tes gagal** integration: create/update/upload/upsert-files menulis `sections` tidak NULL (perluas `knowledge_integration_test.go` dengan satu asersi per writer); `TestBackfillSections`: baris di-set NULL manual -> `Run` mengisi, baris yang sementara di-update writer tidak ditimpa.
- [ ] **Step 3:** Run -> FAIL. **Step 4:** Implement. **Step 5:** PASS. **Step 6:** Commit `feat(knowledge): sections per heading di jalur tulis dan job backfill`.

## Task 11: Store.SuggestTerms dan Store.Search baru

**Files:**
- Modify: `knowledge/base/store/store.go`, `knowledge/base/snippet.go` (`SearchHit.Section`), `knowledge/base/store/store_integration_test.go`
- Modify: `docs/tickets/T20-project-search-recall.md` (bagian EXPLAIN)

**Interfaces:**
- Consumes: `base.SearchPlan.Params()`, konstanta `rank.go`, skema Task 4.
- Produces:
  ```go
  type SearchHit struct { Path, Title string; Score float64; Headline, Section string }
  func (s *Store) SuggestTerms(ctx context.Context, projectID string, words []string) (map[string][]string, error)
  func (s *Store) Search(ctx context.Context, projectID string, plan base.SearchPlan, likePrefix string, limit int) ([]base.SearchHit, int, error)
  ```
  `SuggestTerms`: projectID invalid atau `words` kosong -> map kosong tanpa query. Satu SQL: `unnest($2::text[]) w(word)`, lewati kata yang cocok satu dokumen project (`search @@ phraseto_tsquery('simple', word) OR search_stem @@ (phraseto_tsquery('indonesian', word) || phraseto_tsquery('english', word))`), lalu `CROSS JOIN LATERAL (SELECT DISTINCT term, similarity(term, w.word) sim FROM knowledge_terms WHERE project_id = $1 AND term % w.word AND similarity(term, w.word) >= $3 AND term <> w.word ORDER BY sim DESC, term LIMIT $4)`.

  `Search` (plan tanpa grup -> kosong tanpa query). SQL (algoritma yang harus dipertahankan; nama CTE bebas):
  ```sql
  WITH alt AS (
    SELECT u.g, CASE WHEN u.stem_only THEN NULL ELSE phraseto_tsquery('simple', u.a) END AS qs,
           phraseto_tsquery('indonesian', u.a) || phraseto_tsquery('english', u.a) AS qt
    FROM unnest($2::text[], $3::int[], $4::bool[]) AS u(a, g, stem_only)),
  grp AS (SELECT g, knowledge_tsquery_or_agg(qs) qs, knowledge_tsquery_or_agg(qt) qt FROM alt GROUP BY g),
  q AS (SELECT knowledge_tsquery_or_agg(qs) qs, knowledge_tsquery_or_agg(qt) qt, count(*) n FROM grp),
  phr AS (SELECT knowledge_tsquery_or_agg(qs) qs FROM alt WHERE numnode(qs) > 1),
  cand AS (
    SELECT f.id, f.path, f.title,
      (SELECT count(*) FROM grp WHERE f.search @@ grp.qs OR f.search_stem @@ grp.qt) AS matched,
      ts_rank_cd($5::float4[], f.search, q.qs, 1) + $6 * ts_rank_cd($5::float4[], f.search_stem, q.qt, 1)
      + CASE WHEN phr.qs IS NOT NULL AND (to_tsvector('simple', f.title) || to_tsvector('simple', coalesce(f.sections,'[]'))) @@ phr.qs
             THEN $7 ELSE 0 END AS score
    FROM knowledge_files f, q, phr
    WHERE f.project_id = $1 AND (f.search @@ q.qs OR f.search_stem @@ q.qt) AND f.path LIKE $8 ESCAPE '\'),
  hits AS (
    SELECT c.*, count(*) OVER () AS total FROM cand c, q
    WHERE c.matched >= ceil(q.n * $9)
    ORDER BY c.matched DESC, c.score DESC, c.path COLLATE "C" LIMIT $10)
  SELECT h.path, h.title, h.score, h.total, s.heading AS section, <headline> AS headline
  FROM hits h JOIN knowledge_files f ON f.id = h.id, q
  CROSS JOIN LATERAL (
    -- sections NULL -> satu bagian {"h":"","s":0,"e":char_length(content)}
    SELECT e->>'h' AS heading, substr(left(f.content, 200000), (e->>'s')::int + 1, (e->>'e')::int - (e->>'s')::int) AS body
    FROM jsonb_array_elements(coalesce(f.sections, jsonb_build_array(jsonb_build_object('h','','s',0,'e',char_length(f.content))))) WITH ORDINALITY x(e, i)
    ORDER BY ts_rank_cd(to_tsvector('simple', (e->>'h') || ' ' || <body>), q.qs) + ts_rank_cd(to_tsvector('indonesian', <body>) || to_tsvector('english', <body>), q.qt) DESC, i
    LIMIT 1) s
  ORDER BY h.matched DESC, h.score DESC, h.path COLLATE "C"
  ```
  `<headline>`: marker `\x01`/`\x02` dibuang dari body seperti sekarang; `ts_headline('simple', body, q.qs, $11)` bila `to_tsvector('simple', body) @@ q.qs`, selain itu `ts_headline('indonesian', body, q.qt, $11)`. `$11 = HeadlineOptions` (tetap). Bagian tanpa cocok sama sekali (judul saja yang cocok) -> bagian pertama.

- [ ] **Step 1: Tes gagal** (integration, korpus kecil di tes):
  - `TestSearch_Stem` (T20-AC: variasi kata): dokumen "Pengiriman paket dikirim" -> query `mengirim` dan `kirim` menemukan; "deployment" -> query `deploy` dan `deploys` menemukan.
  - `TestSearch_TitleOutranksBody`: term di judul/path peringkat di atas term yang hanya di isi.
  - `TestSearch_HeadingWeightAndSection`: term di heading `## Rollback cepat` -> `Section == "Rollback cepat"`, snippet memuat marker.
  - `TestSearch_Coverage`: query 2 kata, dokumen A memuat keduanya, B satu -> A di atas B, keduanya ada; query 3 kata, dokumen dengan 1 kata saja tidak ada.
  - `TestSearch_PhraseBoost`: `"exponential backoff"` di heading mengalahkan dua kata terpisah di isi.
  - `TestSuggestTerms_Typo` (T20-AC: typo ringan): `rollbak` -> `["rollback"]`; kata yang sudah cocok -> tidak ada entri; project lain tidak bocor.
  - `TestSearch_NullSections` (Review Focus 3), `TestSearch_SectionSnippetMultibyte` (Review Focus 2), `TestSearch_OperatorOnlyAndStopword` (Review Focus 1: plan dari `"the"` dan dari alternatif kosong tidak error), `TestSearch_ProjectIsolation`.
- [ ] **Step 2:** Run integration -> FAIL. **Step 3:** Implement.
- [ ] **Step 4:** PASS. **Step 5:** Seed 2.000 file sintetis, jalankan `EXPLAIN (ANALYZE, BUFFERS)` untuk kueri `cand` dan `SuggestTerms`; pastikan `Bitmap Index Scan` pada `knowledge_files_search_gin`/`_search_stem_gin` dan `knowledge_terms_term_trgm`; tempel ringkasan di tiket (rule 8.8).
- [ ] **Step 6:** Commit `feat(knowledge): kueri pencarian berbobot, cakupan, trigram, dan cuplikan per bagian`.

## Task 12: Orkestrasi pencarian, kontrak bus, HTTP

**Files:**
- Create: `knowledge/base/searchrun/{searchrun.go,searchrun_test.go}`
- Modify: `lib/contracts/project/knowledge/search/search.go`, `knowledge/bussearch/{init.go,usecase.go,usecase_test.go}`, `knowledge/search/{init.go,usecase.go,usecase_test.go,handler_test.go}`, `knowledge/base/snippet.go` (`SearchItemDTO.Section`, `SearchResultDTO.Expanded`, `NewSearchResult(hits, total, expanded)`), `knowledge/base/query.go` (hapus `ParseQuery` bila masih ada)

**Interfaces:**
- Consumes: Task 8 (bus), Task 9, Task 11.
- Produces:
  ```go
  // lib/contracts/project/knowledge/search (aditif)
  type Item struct { Path string; Score float64; Snippet string; Section string } // Section "" = di luar heading
  type Response struct { Items []Item; Total int; Expanded bool }                  // Expanded: sinonim/typo dipakai
  // knowledge/base/searchrun
  type Store interface {
      SuggestTerms(ctx context.Context, projectID string, words []string) (map[string][]string, error)
      Search(ctx context.Context, projectID string, plan base.SearchPlan, likePrefix string, limit int) ([]base.SearchHit, int, error) }
  type Synonyms interface { Groups(ctx context.Context, projectID string) ([][]string, error) }
  func NewBusSynonyms(b *bus.Registry) Synonyms
  type Outcome struct { Hits []base.SearchHit; Total int; Expanded bool }
  type Runner struct{ /* store, syn, log */ }
  func New(st Store, syn Synonyms, log *slog.Logger) *Runner
  func (r *Runner) Run(ctx context.Context, projectID string, terms []base.Term, likePrefix string, limit int) (Outcome, error)
  ```
  `Run`: Groups (error -> `log.WarnContext` tanpa isi query, lanjut tanpa sinonim) -> `NewSearchPlan` -> `TypoCandidates` tak kosong -> `SuggestTerms` (error dikembalikan terbungkus) -> `WithTypos` -> `Search`. Usecase bussearch/search tetap: validasi -> `ParseTerms` (kosong -> hasil kosong tanpa memanggil Runner) -> prefix/limit -> `Runner.Run` -> map ke Response/DTO (`Section`, `Expanded`).

- [ ] **Step 1: Tes gagal** `searchrun_test.go` (palsu): bus sinonim error -> tetap Search, `Expanded=false`; sinonim dipakai -> `Expanded=true`; tanpa kandidat typo -> `SuggestTerms` tidak dipanggil. Usecase tests: Section diteruskan, query kosong tidak memanggil Runner. Handler HTTP: JSON berisi `section` dan `expanded`.
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement. **Step 4:** `cd backend && go build ./... && go test ./internal/domain/project/... ./internal/archtest/` PASS.
- [ ] **Step 5:** Commit `feat(knowledge): orkestrasi pencarian dengan sinonim lewat bus, field section dan expanded`.

## Task 13: Tool search_knowledge menampilkan section dan expanded

**Files:**
- Modify: `discovery/internal/knowledgetools/base/{searching.go,searching_test.go}`, `searchknowledge/{init.go,summary_test.go,snapshot_test.go}`, `searchknowledge/testdata/*.golden`

**Interfaces:**
- Consumes: `search.Item.Section`, `search.Response.Expanded`.
- Produces: `SearchHits.Expanded bool`; `Structured()` item memuat `"section"` hanya bila tak kosong; `Text()` menambah baris `   bagian: <section>` di bawah snippet bila tak kosong, dan baris penutup menjadi `Baca isi lengkap dengan read_file(path), atau satu bagian dengan read_section(path, heading).` bila ada item bersection; `Summary()` menambah `"expanded": true` hanya bila true. Deskripsi tool: sebut toleran sinonim/salah ketik/imbuhan dan field `section` untuk `read_section`. Skema argumen tidak berubah.

- [ ] **Step 1: Tes gagal**: summary tanpa expanded tidak punya kunci `expanded` (entri lama tetap valid); dengan expanded -> `true`; structured tanpa section tidak punya kunci `section`.
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement, regenerasi golden snapshot dengan flag update yang dipakai `snapshot_test.go`; tinjau diff.
- [ ] **Step 4:** PASS. **Step 5:** Commit `feat(knowledgetools): section dan expanded di search_knowledge`.

## Task 14: Golden set, kamus fixture, integrasi end-to-end, penyelarasan bobot

**Files:**
- Create: `backend/testdata/searcheval/synonyms.yaml`, `backend/lib/util/rankeval/{synonyms.go,synonyms_test.go}`, `backend/cmd/internal/server/searchsynonym_integration_test.go`
- Modify: `backend/cmd/internal/server/searcheval_integration_test.go` (seed kamus via REST sebelum mencari), `backend/testdata/searcheval/baseline.json`, `knowledge/base/rank.go` (nilai hasil penyelarasan), `docs/search-quality.md`

**Interfaces:**
- Produces: `func LoadSynonyms(path string) ([][]string, error)` dan `func ValidateSynonyms(groups [][]string) error` (2-10 term, huruf kecil, unik lintas kelompok). Format YAML: `groups: [[term, ...], ...]`.

- [ ] **Step 1:** Tulis `synonyms.yaml` dari istilah tim (bukan dari keluaran pencarian; periksa kosakata korpus dengan `grep`), mis. `[rollback, kembalikan versi, revert]`, `[rate limit, batas permintaan, throttling]`, `[refund, pengembalian dana]`, `[invoice, tagihan, faktur]`, `[customer, pelanggan]`, `[api key, kredensial akses, kunci api]`, `[password, kata sandi, sandi]`, `[webhook, callback]`, `[onboarding, karyawan baru, new hire]`. Tes `TestSynonymsFixture` di rankeval memvalidasi file tanpa DB.
- [ ] **Step 2: Tes gagal** `searchsynonym_integration_test.go` (T20-AC integrasi, pola `seSetupMCP`):
  - `TestT20_SynonymViaAPI`: seed `auth/autentikasi.md` ("Alur autentikasi pengguna"); `search_knowledge {"query":"login"}` -> 0 item; `POST /api/projects/{id}/search/synonyms {"terms":["login","autentikasi"]}` -> 201; ulangi -> item pertama `auth/autentikasi.md`, entri audit `mcp.search_knowledge` terbaru `result.expanded == true`; `DELETE` grup -> 204; ulangi -> 0 item.
  - `TestT20_SynonymAuthz`: user non-anggota -> 404 untuk GET/POST; tanpa CSRF -> 403; `tools/list` tidak memuat tool yang menulis sinonim.
- [ ] **Step 3:** Ubah `TestSearchEval`: setelah `seSeedCorpus`, `LoadSynonyms` lalu POST tiap kelompok (gagal = `t.Fatalf`). Run `make test-search-eval` dan catat angka per kind.
- [ ] **Step 4:** Selaraskan `RankWeights`, `StemFactor`, `PhraseBoost`, `MinCoverage`, `TypoMinSimilarity` dalam rentang kecil (mis. MinCoverage {0.5, 0.67}, TypoMinSimilarity {0.35, 0.4, 0.45}, bobot B {0.4, 0.5, 0.7}); pilih kombinasi dengan MRR tertinggi yang memenuhi K1 tanpa menurunkan `exact-name`/`phrase` > 0.02 dari baseline lama. Catat tabel percobaan di tiket. Bila K1 tidak tercapai: hentikan penyelarasan, tulis angka dan pasangan `GAGAL` beserta penyebab di tiket.
- [ ] **Step 5:** `make search-eval-baseline`, tinjau diff `baseline.json`. Perbarui `docs/search-quality.md`: bagian 7 (kamus `synonyms.yaml` di-seed tes, aturan menulisnya), bagian 9 (tabel baru, tanggal, implementasi T20, angka lama sebagai pembanding), bagian 10 (keterbatasan: trigram lemah untuk kata pendek/transposisi seperti `kyes`; stemmer Indonesia tidak memulihkan peluluhan, ditutup `stemid`), bagian baru "Cara kerja pencarian" (bobot, cakupan, sinonim, typo, `section`, `expanded`).
- [ ] **Step 6:** Commit dengan alasan di pesan: `test(searcheval): kamus sinonim fixture dan baseline T20` + badan berisi angka sebelum/sesudah.

## Task 15: FE service dan tipe search-synonym

**Files:**
- Create: `frontend/src/domain/project/search-synonym/listgroups/{types.ts,types.test.ts,index.ts,services/listgroups.service.ts,services/listgroups.service.test.ts,mocks/groups.ts}`, `.../savegroup/{types.ts,model/terms.ts,model/terms.test.ts,services/savegroup.service.ts,services/savegroup.service.test.ts,index.ts}`, `.../deletegroup/{services/deletegroup.service.ts,services/deletegroup.service.test.ts,index.ts}`

**Interfaces:**
- Consumes: OpenAPI Task 2; `get/post/put/del` dari `shared/services/http.ts`.
- Produces:
  ```ts
  // listgroups/types.ts
  export const SynonymGroupSchema = z.object({ id: z.string().uuid(), terms: z.array(z.string()).min(2).max(10),
    created_at: z.string().datetime({ offset: true }), updated_at: z.string().datetime({ offset: true }),
    created_by_label: z.string(), updated_by_label: z.string() });
  export const SynonymGroupListSchema = z.object({ items: z.array(SynonymGroupSchema) });
  export type SynonymGroup = z.infer<typeof SynonymGroupSchema>;
  export function listGroups(projectId: string, signal?: AbortSignal): Promise<{ items: SynonymGroup[] }>;
  // savegroup
  export const MIN_TERMS = 2, MAX_TERMS = 10, MAX_TERM_CHARS = 64;
  export type TermsIssue = 'tooFew' | 'tooMany' | 'empty' | 'tooLong' | 'invalidChar' | 'duplicate';
  export function normalizeTerms(raw: string[]): { terms: string[]; issue: TermsIssue | null }; // cermin NormalizeTerms backend
  export function createGroup(projectId: string, terms: string[]): Promise<SynonymGroup>;
  export function updateGroup(projectId: string, groupId: string, terms: string[]): Promise<SynonymGroup>;
  // deletegroup
  export function deleteGroup(projectId: string, groupId: string): Promise<void>;
  ```
  `savegroup` mengimpor `SynonymGroupSchema` dari `../listgroups` (index publik).
- [ ] **Step 1: Tes gagal**: URL dan method benar (fetch stub), respons tak valid -> `invalid_response`; `normalizeTerms` kasus sama dengan Task 5 Step 1 (huruf kecil, spasi diringkas, duplikat setelah normalisasi).
- [ ] **Step 2:** `cd frontend && npx vitest run src/domain/project/search-synonym` -> FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(fe-search-synonym): service dan validasi kamus sinonim`.

## Task 16: FE komponen dan view kamus

**Files:**
- Create: `listgroups/{stores/groupList.store.ts,stores/groupList.store.test.ts,components/SynonymGroupsTable.svelte,views/ListGroupsView.svelte,views/ListGroupsView.test.ts}`, `savegroup/{components/TermsInput.svelte,views/SaveGroupDialog.svelte,views/SaveGroupDialog.test.ts}`, `deletegroup/{views/DeleteGroupDialog.svelte,views/DeleteGroupDialog.test.ts}`
- Modify: ketiga `index.ts`, `shared/i18n/{id,en}.json`

**Interfaces:**
- Produces:
  - `createGroupListStore()` dengan state `idle|loading|ready|notFound|error` dan method `begin/resolve/fail/upsert/remove/invalidate` (pola `createKeyListStore`; urut `created_at, id`).
  - `ListGroupsView` props `{ projectId, store, oncreate(): void, onedit(g: SynonymGroup): void, ondelete(g: SynonymGroup): void, onprojectgone(): void }`: `PageHeader` + satu tombol ink "Tambah kelompok", `Table` (kolom: term sebagai pil `rounded-full` mono, diubah oleh, diperbarui, aksi), `EmptyState` saat kosong, `ErrorState` + retry saat error, `Spinner` saat loading.
  - `SaveGroupDialog` props `{ open, projectId, group: SynonymGroup | null, onclose, onsaved(g), onstale(), onprojectgone }` (`Dialog` glass; `TermsInput` = input pil: Enter/koma menambah term, Backspace menghapus terakhir, tombol hapus per pil dengan aria-label).
  - `DeleteGroupDialog` props `{ open, projectId, group, onclose, ondeleted(id), onstale, onprojectgone }`.
  - Kunci i18n (id dan en): `project.tab.search`, `searchSynonym.title`, `searchSynonym.intro`, `searchSynonym.empty`, `searchSynonym.emptyHint`, `searchSynonym.col.terms`, `searchSynonym.col.updatedBy`, `searchSynonym.col.updated`, `searchSynonym.col.actions`, `searchSynonym.create.open`, `searchSynonym.create.title`, `searchSynonym.edit.title`, `searchSynonym.edit.aria`, `searchSynonym.form.terms`, `searchSynonym.form.hint`, `searchSynonym.form.removeTerm`, `searchSynonym.form.submit`, `searchSynonym.saved`, `searchSynonym.delete.aria`, `searchSynonym.delete.title`, `searchSynonym.delete.message`, `searchSynonym.delete.submit`, `searchSynonym.deleted`, `searchSynonym.gone`, `searchSynonym.validation.tooFew|tooMany|empty|tooLong|invalidChar|duplicate`, `searchSynonym.error.term_in_use` (dengan `{terms}`), `searchSynonym.error.group_limit`, `searchSynonym.error.load`.
- [ ] **Step 1: Tes gagal** (T20-AC UI, `@testing-library/svelte`, fetch stub, `setLocale('id')`):
  - daftar kosong menampilkan `searchSynonym.empty`; error 500 menampilkan ErrorState dan retry memanggil ulang.
  - tambah: isi `login`, `autentikasi` -> POST body `{"terms":["login","autentikasi"]}` -> baris baru muncul, toast `saved`.
  - ubah: buka dari baris, ganti term -> PUT ke `/search/synonyms/{id}`, baris diperbarui.
  - hapus: konfirmasi -> DELETE, baris hilang; 404 -> `onstale` + toast `gone`.
  - validasi lokal: satu term -> pesan `tooFew` dan tombol submit nonaktif; `Login` + `login` -> `duplicate`; tidak ada request terkirim.
  - error API 409 `term_in_use` dengan `details.terms:["login"]` -> pesan memuat `login`; 404 project -> `onprojectgone`.
  - Setiap kunci baru ada di `id.json` dan `en.json` (tes membandingkan himpunan kunci `searchSynonym.*`).
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement. **Step 4:** `npx vitest run src/domain/project/search-synonym && npm run check` PASS.
- [ ] **Step 5:** Commit `feat(fe-search-synonym): tabel, dialog simpan, dialog hapus`.

## Task 17: FE tab "Pencarian" dan section di hasil knowledge

**Files:**
- Create: `frontend/src/app/views/workspace/SearchTab.svelte`, `frontend/src/app/views/workspace/SearchTab.test.ts`
- Modify: `frontend/src/app/workspace/tabs.ts`, `frontend/src/domain/project/knowledge/search/{types.ts,components/SearchResults.svelte,components/components.test.ts}`, `shared/i18n/{id,en}.json` (`project.tab.search` = "Pencarian" / "Search", `knowledge.search.section`)

**Interfaces:**
- Consumes: index `listgroups`, `savegroup`, `deletegroup`.
- Produces: entri `{ key: 'search', labelKey: 'project.tab.search', view: SearchTab }` setelah `api-specs` (urutan: Knowledge, API Specs, Pencarian, API Keys, Members, Settings; perbarui komentar urutan). `SearchItemSchema` + `section: z.string().optional().default('')`, `SearchResultSchema` + `expanded: z.boolean().optional().default(false)`; `SearchResults` menampilkan chip mono `§ <section>` bila tak kosong.
- [ ] **Step 1: Tes gagal**: `SearchTab.test.ts` (pola `ApiKeysTab.test.ts`): render memanggil GET list, alur tambah lewat tab menutup dialog dan memperbarui tabel; `findTab('search')` tidak null. `components.test.ts`: item dengan `section` menampilkan chip; respons lama tanpa `section` tetap lolos Zod.
- [ ] **Step 2:** Run -> FAIL. **Step 3:** Implement (komposisi lintas feature hanya di `SearchTab.svelte`, rule 12.4).
- [ ] **Step 4:** `cd frontend && npm test -- --run && npm run check && npm run test:arch` PASS.
- [ ] **Step 5:** Commit `feat(fe-workspace): tab Pencarian untuk kamus sinonim dan section di hasil`.

## Task 18: Verifikasi akhir dan status tiket

**Files:**
- Modify: `docs/tickets/T20-project-search-recall.md` (angka akhir, keputusan bobot), `docs/tickets/README.md` (status)

- [ ] **Step 1:** Run `make lint && make test && make test-integration && make test-search-eval && make api-check && make api-selftest` dan `cd frontend && npm run check && npm test -- --run && npm run test:arch`. Expected: semua lulus; log `TestSearchEval` memuat ringkasan yang dicatat di tiket.
- [ ] **Step 2:** Cocokkan tiap `T20-AC*` dengan nama tes yang membuktikannya (tabel di tiket).
- [ ] **Step 3:** Commit `docs(tickets): T20 hasil akhir dan pemetaan kriteria penerimaan`.
