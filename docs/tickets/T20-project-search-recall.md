# T20: project / knowledge (pencarian lebih toleran) + searchsynonym (kamus sinonim)

| | |
|---|---|
| Domain / Module | project / knowledge (pencarian) dan discovery / knowledge-tools (tool `search_knowledge`); module baru project / searchsynonym untuk kamus sinonim |
| Ukuran | L |
| Bergantung pada | T05, T08, T11 (alat ukur), T18 (parser heading `mdoutline`) |
| Sumber | permintaan pemilik; spec `docs/superpowers/specs/2026-10-26-search-recall-design.md`; plan `docs/superpowers/plans/2026-10-26-search-recall.md` |

## Ringkasan

`search_knowledge` memakai full-text Postgres config `simple`: tanpa sinonim, stemming, atau toleransi typo. Baseline T11 (40 pasangan) recall@5 0,5875 dan MRR 0,6; kind `synonym` 0,0625 dan `typo` 0. Tiket ini membuat pencarian knowledge (HTTP dan tool MCP) menemukan dokumen lewat sinonim (kamus per project, dikelola manusia), salah ketik, variasi imbuhan Indonesia/Inggris, dengan judul/path/heading berbobot lebih tinggi dan cuplikan diambil dari bagian (heading) yang paling cocok.

## Tujuan

1. **Sinonim:** kamus sinonim per project; query diperluas ke kelompok sinonim (dua arah, termasuk term multi-kata).
2. **Typo dan variasi kata:** toleransi salah ketik (trigram) dan stemming Indonesia + Inggris (mengirim, dikirim, pengiriman; deploy, deployment).
3. **Urutan:** path/judul (bobot A) dan heading (B) di atas isi (D); frasa utuh pada judul/heading dinaikkan.
4. **Cuplikan:** snippet dari bagian paling cocok, hasil memuat field `section` supaya agent bisa lanjut `read_section` (T18).

## Non-goals

- Embedding, pgvector, layanan atau model eksternal; pencarian lintas project; sinonim otomatis; terjemahan lintas bahasa.
- Mengubah kontrak argumen `search_knowledge` (hasil hanya bertambah field opsional `section`).

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Operator tsquery atau pola trigram dari input pengguna dieksekusi mentah (hanya teks term sebagai parameter ke `phraseto_tsquery` / `similarity`).
- Recall naik tetapi presisi runtuh (sampah di peringkat atas).
- Pengindeksan ulang memblokir tulisan, atau migrasi gagal pada database berisi data.
- Kamus sinonim bisa diubah oleh agent (tool tetap read-only; hanya REST manusia).

## Keputusan pemilik

- Hanya Postgres, tanpa layanan eksternal; keempat masalah (sinonim, typo/variasi, urutan, cuplikan) harus diatasi.
- Kamus sinonim per project, dikelola manusia.

## Keputusan pembangun (mohon dicek)

Dari spec:

- **K1** Ukuran keberhasilan: golden set T11. Target di korpus fixture: recall@5 keseluruhan >= 0,80; `typo` recall@5 >= 0,50; `synonym` recall@5 >= 0,50 (dengan fixture kamus `synonyms.yaml`); MRR >= 0,70; `exact-name` dan `phrase` tidak turun lebih dari toleransi 0,02. Target tidak diturunkan; bila tidak tercapai, angka jujur dilaporkan di bagian "Hasil verifikasi".
- **K2** Tabel `knowledge_synonym_groups(id, project_id, terms text[], created_at, updated_at, *_by_label)`; satu baris = satu kelompok setara (2 sampai 10 term, huruf kecil, unik dalam project). REST `/api/projects/{projectID}/search/synonyms[/{groupID}]`: list, create, update, delete; anggota project (non-anggota 404). UI tab "Pencarian" di workspace project.
- **K3** Stemming: snowball `indonesian` + `english` di indeks (kolom generated `search_stem`), karena keduanya tersedia di `postgres:16`. Celah peluluhan k/p/t/s ditutup sisi query oleh `backend/lib/util/stemid.Variants`; varian hanya masuk query stem dan tidak menyalakan `expanded`.
- **K4** Typo: `pg_trgm` atas kosakata project `knowledge_terms` (lexeme berbobot A/B atau muncul >= 2 kali di isi, dipelihara trigger). Konstanta: `TypoMinSimilarity = 0.4`, `TypoMaxCandidates = 3`, `TypoMinRunes = 4`. Kandidat hanya untuk kata tunggal tanpa sinonim yang tidak cocok dokumen mana pun.
- **K5** Bobot: path/judul A, heading B, isi D; `PhraseBoost` untuk frasa utuh di judul/heading. Nilai akhir diselaraskan lewat golden set (lihat tabel penyelarasan).
- **K6** Cuplikan: `sections` (jsonb) dihitung Go dari `mdoutline`; `ts_headline` hanya dari bagian paling cocok; field `section`. Format snippet lama dipertahankan (`**term**`, segmen `{text, highlight}`).
- **K7** Pengindeksan ulang: migrasi forward-only dan idempoten; `CREATE INDEX CONCURRENTLY` tidak mungkin (runner migrasi satu transaksi per file), dicatat sebagai risiko di bawah.
- **K8** Aksi audit `mcp.search_knowledge` tidak berubah; `result.summary` boleh memuat `expanded` (hanya ditulis bila `true`).

Dari plan:

- **D1** Kolom `sections` dihitung di entity `File`, ditulis semua writer; baris lama diisi job `knowledge.backfill-sections` (tiap menit, batch 200). Selama `NULL`, dokumen tetap tercari, snippet dari seluruh isi, `section = ""`.
- **D2** Skor = `ts_rank_cd(W, search, qs, 1) + StemFactor * ts_rank_cd(W, search_stem, qt, 1) + PhraseBoost`; `W = RankWeights` dari konstanta Go.
- **D3** Cakupan: dokumen cocok bila memenuhi minimal `ceil(n * MinCoverage)` dari n kelompok term (MinCoverage awal 0,5). Urutan: jumlah kelompok cocok DESC, skor DESC, path.
- **D4** Sinonim lewat bus, fail-soft: bila `project.searchsynonym.list-groups` gagal, log warn dan cari tanpa sinonim.
- **D5** Audit kamus: `search_synonym.created|updated|deleted` (konten manusia, fail-open).
- **D6** Trigger `knowledge_terms_refresh` adalah skema milik module knowledge; kode Go menyentuh SQL hanya lewat repository/store.

## Fakta lingkungan (diperiksa 2026-10-26, `postgres:16`, PostgreSQL 16.15)

- 29 config teks, termasuk `indonesian` dan `english`; `pg_trgm` 1.6 tersedia (trusted extension sejak PG13).
- Snowball Indonesia tidak memulihkan peluluhan k/t (`menulis` -> `ulis`, `tulisan` -> `tulis`) dan tidak punya stopword; ditutup `stemid.Variants`.
- `similarity()`: pagnation/pagination 0,615; rollbak/rollback 0,545; loggin/login 0,625; enviroment/environment 0,643; limt/limit 0,375; kyes/keys 0,111 (dua terakhir sengaja tidak tertolong trigram).
- Runner migrasi (`backend/internal/infra/db/migrate.go`) menjalankan tiap file dalam satu transaksi.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `project/knowledge/base` (plan, rank, sections, searchrun, store) | `SearchPlan` (sinonim, varian, kandidat typo), peringkat, orkestrasi pencarian | internal | internal |
| `project/knowledge/search`, `bussearch` | Pencarian HTTP dan bus, field `section` dan `expanded` | `GET /api/projects/{id}/knowledge/search`, bus `project.knowledge.search` | anggota; bus |
| `project/knowledge/backfillsections` | Job pengisi `sections` baris lama | job `knowledge.backfill-sections` | sistem |
| `project/searchsynonym/{listgroups,creategroup,updategroup,deletegroup}` | CRUD kamus sinonim | `/api/projects/{projectID}/search/synonyms[/{groupID}]` | anggota project |
| `project/searchsynonym/buslistgroups` | Daftar kelompok untuk knowledge | bus `project.searchsynonym.list-groups` | knowledge |
| `discovery/knowledge-tools/searchknowledge` | Menampilkan `section` dan `expanded` | tool MCP `search_knowledge` | agent (read-only) |
| `lib/util/stemid` | Varian peluluhan K/P/T/S | internal | internal |
| `frontend/domain/project/search-synonym/*`, `app/views/workspace/SearchTab.svelte` | Tab "Pencarian": daftar, tambah/ubah, hapus kelompok | tab workspace | anggota project |

## Kontrak bus

| Endpoint | Jenis | Pemilik | Konsumen |
|---|---|---|---|
| `project.searchsynonym.list-groups` (`BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS`) | Call (baca-saja) | T20 | T20 (knowledge `bussearch`, `search`) |

Kontrak: `lib/contracts/project/searchsynonym/listgroups`. Perubahan aditif pada `project.knowledge.search`: `Item.Section` dan `Response.Expanded`.

## Aturan bisnis

- Kamus hanya dapat diubah manusia lewat REST; tidak ada tool MCP yang menulis sinonim.
- Kelompok: 2 sampai 10 term, huruf kecil, unik dalam project (409 `term_in_use` bila term sudah dipakai kelompok lain; dua POST bersamaan dengan term sama: tepat satu sukses).
- Kegagalan membaca kamus tidak menggagalkan pencarian (fail-soft).
- Query hanya berisi operator/tanda baca atau stopword tidak menimbulkan error SQL.

## UI (Svelte)

- Tab baru "Pencarian" di `workspaceTabs`: tabel kelompok (term sebagai pill mono), keadaan kosong, tambah/ubah lewat dialog (input term dengan validasi: minimal 2, maksimal 10, duplikat, karakter operator), hapus dengan konfirmasi; galat 404/409/500 ditampilkan di dialog.
- Hasil pencarian knowledge menampilkan nama bagian (`section`) bila ada.
- Teks lewat i18n `id` dan `en`; komponen memakai `shared/components` dan token `DESIGN.md`.

## Titik sentuh

Backend: `project/internal/knowledge/{base,search,bussearch,createfile,updatefile,backfillsections}`, `project/internal/searchsynonym/*`, `discovery/internal/knowledgetools/{base,searchknowledge}`, `lib/contracts/project/{knowledge/search,searchsynonym/listgroups}`, `lib/contracts/core/audit/record`, `lib/constants/bus.go`, `lib/util/stemid`, `lib/util/rankeval`, `migrations/project/*_knowledge_search_recall.sql` dan `*_create_knowledge_synonym_groups.sql`, `infra/db/testing.go`.
Frontend: `domain/project/search-synonym/*`, `domain/project/knowledge/search/*`, `app/workspace/tabs.ts`, `app/views/workspace/SearchTab.svelte`, i18n.
Lainnya: `api/modules/project/{knowledge.yml,search-synonym.yml}`, `api/root.yml`, `api/openapi.yml`, `docs/search-quality.md`, `docs/tickets/README.md`, `rules.txt` (2.1).

## Kriteria penerimaan

Nomor mengacu ke tabel pemetaan di bawah.

- **T20-AC1** Golden set memenuhi target K1, atau laporan angka jujur beserta penyebab.
- **T20-AC2** Unit: perluasan sinonim dua arah dan satu kelompok.
- **T20-AC3** Unit: query berisi operator tsquery tetap aman.
- **T20-AC4** Unit: typo ringan ditemukan.
- **T20-AC5** Unit: query kosong dan sangat panjang ditangani.
- **T20-AC6** Integrasi: tambah kelompok lewat API lalu `search_knowledge` menemukan dokumen lewat sinonim; hapus kelompok dan hasil kembali.
- **T20-AC7** UI: daftar kosong, tambah, ubah, hapus, validasi (term terlalu sedikit, duplikat), error API.
- **T20-AC8** OpenAPI dan katalog bus/tiket diperbarui; `make lint`, `make test`, `make test-integration`, `make test-search-eval` lulus.

## Pemetaan kriteria penerimaan ke tes

Nama tes diperiksa dengan grep pada worktree (2026-10-26). Path relatif terhadap `backend/` kecuali FE. "Belum ada" berarti tes dijadwalkan plan tetapi belum ditulis saat dokumen ini dibuat.

| AC | Tes | Lokasi | Status |
|---|---|---|---|
| T20-AC1 | `TestSearchEval` | `cmd/internal/server/searcheval_integration_test.go` | lulus (2026-10-27, wiring terpasang): 40 pasangan, recall@1 0,9875, recall@5 1,0000, MRR 1,0000; target K1 tercapai |
| T20-AC1 | `TestSynonymsFixture`, `TestValidateSynonyms`, `TestLoadSynonyms_Errors` | `lib/util/rankeval/synonyms_test.go` | ada (validasi fixture) |
| T20-AC2 | `TestSearchPlan_SynonymBothDirections`, `TestSearchPlan_MultiwordSynonym`, `TestSearchPlan_NoSynonyms`, `TestSearchPlan_SynonymsInsertedFromDictionaryAreNormalized` | `internal/domain/project/internal/knowledge/base/plan_test.go` | lulus |
| T20-AC2 | `TestRun_SynonymExpands`, `TestRun_SynonymFailureIsSoft` | `.../knowledge/base/searchrun/searchrun_test.go` | lulus |
| T20-AC2 | `TestSearch_SynonymAndExpandedPlan` | `.../knowledge/base/store/search_integration_test.go` | lulus (integrasi DB) |
| T20-AC3 | `TestParseTerms_OperatorsInert` | `.../knowledge/base/query_test.go` | lulus |
| T20-AC3 | `TestSearch_OperatorOnlyAndStopword` | `.../knowledge/base/store/search_integration_test.go` | lulus |
| T20-AC4 | `TestSearchPlan_WithTypos_MaxCandidates`, `TestSearchPlan_TypoCandidatesBounds`, `TestSearchPlan_TypoCandidates_PhraseAndDigitsExcluded` | `.../knowledge/base/plan_test.go` | lulus |
| T20-AC4 | `TestSuggestTerms_Typo` | `.../knowledge/base/store/search_integration_test.go` | lulus |
| T20-AC4 | `TestRun_TypoApplied` | `.../knowledge/base/searchrun/searchrun_test.go` | lulus |
| T20-AC4 | `TestVariants`, `TestVariants_None`, `TestVariants_UniqueDeterministic` | `lib/util/stemid/stemid_test.go` | lulus |
| T20-AC5 | `TestParseTerms_Long`, `TestEmptyQuery` | `.../knowledge/base/query_test.go`, `.../knowledge/bussearch/usecase_test.go` | lulus |
| T20-AC6 | `TestT20_SynonymViaAPI`, `TestT20_SynonymAuthz` | `cmd/internal/server/searchsynonym_integration_test.go` | **Gagal / belum lulus**: `TestT20_SynonymViaAPI` gagal karena route kamus belum terpasang (wiring); `TestT20_SynonymAuthz` belum diverifikasi |
| T20-AC6 | `TestBusSynonyms` | `.../knowledge/base/searchrun/searchrun_test.go` | lulus (sinonim lewat bus, tanpa HTTP) |
| T20-AC6 | `TestGroupLifecycle`, `TestCreateGroup_ConcurrentSameTerm`, `TestUpdateGroup_Integration`, `TestDeleteGroup_Integration` | `.../searchsynonym/base/group_test.go`, `.../searchsynonym/{creategroup,updategroup,deletegroup}` | lulus |
| T20-AC7 | `SearchTab.test.ts` (tab terdaftar, muat, tambah, hapus) | `frontend/src/app/views/workspace/` | ada; hasil di bagian verifikasi |
| T20-AC7 | `ListGroupsView` (kosong, error + retry, 404), `SaveGroupDialog` (tambah, ubah, tooFew, duplikat, operator, 409 `term_in_use`, 409 `group_limit`, 500), `DeleteGroupDialog` (hapus, 404, 500), `normalizeTerms` | `frontend/src/domain/project/search-synonym/**` | ada; hasil di bagian verifikasi |
| T20-AC8 | `TestSearchRecallMigration_*` (idempoten, kolom, bobot, trigger, `or_agg`) | `internal/infra/db/migrate_searchrecall_integration_test.go` | lulus |
| T20-AC8 | `make lint`, `make test`, `make test-integration`, `make test-search-eval`, `make api-check`, `make api-selftest` | lihat "Hasil verifikasi" | lihat di bawah |

Tes pendukung (presisi/robustness, Review Focus plan): `TestSections_RuneOffsets` (`.../knowledge/base/sections_test.go`), `TestSearch_SectionSnippetMultibyte`, `TestSearch_NullSections`, `TestSearch_HeadingWeightAndSection`, `TestSearch_Coverage`, `TestSearch_PhraseBoost`, `TestSearch_Stem` (`.../knowledge/base/store/search_integration_test.go`), `TestBackfillSections` (`.../knowledge/backfillsections`), `TestWritersFillSections` (`.../knowledge/knowledge_integration_test.go`), `TestExecute_SectionAndExpanded` (`.../knowledge/search/usecase_test.go`), `TestSummary_ExpandedOnlyWhenTrue` (`.../knowledgetools/searchknowledge/summary_test.go`).

## Risiko

- **K7: migrasi menulis ulang `knowledge_files`.** `ADD COLUMN ... GENERATED ... STORED` (dua kolom tsvector) dan dua indeks GIN berjalan dalam satu transaksi migrasi dengan `ACCESS EXCLUSIVE` pada `knowledge_files`; `CREATE INDEX CONCURRENTLY` tidak mungkin. **Terukur:** 10.000 file sintetis (~8 KB per file), migrasi penuh dijalankan ulang di schema yang sudah termigrasi: **26,3 detik**, menghasilkan 99.901 baris `knowledge_terms`. Kira-kira linear terhadap total ukuran konten (~0,33 detik per MB). Migrasi berjalan saat start sebelum server melayani, sehingga dampaknya penundaan start; instalasi dengan ratusan ribu file perlu jendela pemeliharaan.
- **Biaya pencarian** dibatasi `base.SearchScoreWindow` (400): hanya 400 dokumen teratas menurut (cakupan, cocok judul/path, path) yang diberi skor `ts_rank_cd`; di atas 400 dokumen cocok urutan tidak eksak penuh. Kandidat diambil lewat UNION dua pencarian GIN (planner memilih seq scan pada bentuk `OR`, 5.000 file ~630 ms p95; kini ~46 ms p95, AC9 lulus).
- **Presisi:** trigram dan stemming menambah kandidat; dijaga aturan cakupan (D3) dan peringkat "semua term di atas sebagian".
- **Peran database tanpa hak `CREATE EXTENSION pg_trgm`:** migrasi gagal dengan pesan jelas (`IF NOT EXISTS`).
- **Baris `sections IS NULL` sebelum backfill:** snippet dari seluruh isi dan `section = ""` sampai job mengisinya.

## EXPLAIN

Korpus sintetis: 1 project, 2.000 file (~1.000 kata acak berbeda per file, ~8 KB) ditambah 1 file target; `ANALYZE` dijalankan; Postgres 16 lokal, semua di cache.

- `Store.Search` ("peluncuran khusus", 2 kelompok): **~13 ms** (11,7 sampai 14,1 ms antar run). Penyaring kandidat `search @@ (SELECT qs FROM q) OR search_stem @@ (SELECT qt FROM q)` ditulis sebagai InitPlan. Pada korpus satu-project ini planner memilih `Bitmap Heap Scan` lewat `knowledge_files_project_updated_idx` (atau `Seq Scan`) dengan `@@` sebagai filter, bukan GIN: biaya GIN diperkirakan 1.394 per indeks versus 31 untuk btree project, karena project memegang seluruh tabel. Bentuk yang sama dengan tsquery konstan juga tidak memakai GIN di korpus ini, jadi ini pilihan planner, bukan akibat InitPlan. Bukti GIN dapat dipakai: kueri tanpa filter project memakai `BitmapOr` dari `Bitmap Index Scan on knowledge_files_search_gin` dan `knowledge_files_search_stem_gin` (0,65 ms). Konsekuensi: biaya linear terhadap jumlah file project (~6 mikrodetik per file 8 KB). Bila project jauh lebih besar, pertimbangkan indeks gabungan `btree_gin (project_id)` + `search` (butuh ekstensi tambahan, di luar T20).
- `Store.SuggestTerms` (2 kata): **~41 sampai 44 ms**, kebanyakan memindai 8.831 baris `knowledge_terms` project via `knowledge_terms_project_term` dengan `%` sebagai filter (indeks trigram `knowledge_terms_term_trgm` tidak dipilih karena project memegang seluruh tabel) dan anti-join `NOT EXISTS` terhadap 2.001 file. Hanya berjalan untuk kata tunggal tanpa sinonim yang >= 4 rune, maksimal 16 kata per query.

## Penyelarasan bobot

Task 14 Step 4 selesai: nilai awal di `knowledge/base/rank.go` sudah memenuhi K1 sehingga tidak diubah (grid 18 kombinasi ada di `docs/superpowers/plans/search-recall/eval-notes.md`).

| Parameter | Nilai awal | Nilai akhir | Alasan |
|---|---|---|---|
| `MinCoverage` | 0,5 | 0,5 | 0,67 menjatuhkan synonym dan phrase (recall@5 turun ke 0,85) |
| `TypoMinSimilarity` | 0,4 | 0,4 | hasil tidak sensitif pada 0,35 sampai 0,45 di korpus ini |
| Bobot heading (B) | 0,5 | 0,5 | hasil tidak sensitif pada 0,4 sampai 0,7 di korpus ini |

## Hasil verifikasi

Dijalankan 2026-10-27 di worktree `feat-search-recall`, Postgres `localhost:5433`, dengan wiring final. Status tiket: **backend lulus**.

| Perintah | Hasil |
|---|---|
| `go build ./...`, `go vet ./...`, `go vet -tags integration ./...` (backend) | lulus |
| `go test ./...` (backend, unit) | lulus |
| `go test -race -tags integration ./...` (setara `make test-integration`) | lulus (`TestAC9_ExplainAndP95` p95 46 ms setelah jendela skor + UNION GIN), kecuali `TestSMTP_ContextDeadline` (`infra/mailer`, data race di fake SMTP; di luar T20 dan tidak disentuh) |
| `TestSearchEval`, `TestT20_SynonymViaAPI`, `TestT20_SynonymAuthz`, `TestRoutesRegistered` | lulus |
| `make api-check`, `make api-selftest` | OK |
| `golangci-lint run` (bagian `make lint`) | tidak dijalankan: `golangci-lint` tidak terpasang di mesin ini; `go vet` lulus |

Pembersihan jalur lama: `base.ParseQuery`, `Store.SearchTSQ`, `store/legacy.go`, dan tes-tesnya dihapus; `BackfillSections.Init` melewati pendaftaran job bila `Deps.Jobs` nil (dependensi tes).

## Status

Diimplementasi (menunggu review) untuk stack backend: fondasi, kamus sinonim, pencarian baru, tool MCP, wiring, dan penyelarasan bobot selesai; target K1 tercapai. Sisa: `golangci-lint` belum terverifikasi (tidak terpasang) dan tes FE di luar cakupan verifikasi ini.
