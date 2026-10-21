# T11 discovery/knowledge-tools: Usecase `searchknowledge` (Ubah, tes saja)

Mengikuti `docs/superpowers/plans/discovery-search-quality/overview.md` (FOUNDATION). Bila bertentangan, overview yang berlaku.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T11 (AC1-AC6, AC3, AC5) |
| Scope | backend |
| Domain | discovery |
| Module | knowledge-tools (package `knowledgetools`) |
| Usecase | `searchknowledge` (jenis: Ubah, TANPA mengubah kode feature) |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| allowedPaths | `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/`, `backend/cmd/internal/server/searchquality_audit_integration_test.go`, `docs/mcp-gateway.md` |
| Migrasi | Tidak ada |
| `endpointsChanged` | false |

| Endpoint | Method | Version | Keterangan |
|---|---|---|---|
| `/mcp` `tools/call` `search_knowledge` | POST (MCP JSON-RPC 2.0) | tidak berubah | Teks dan `structuredContent` identik dengan sebelum T11. Hanya `audit_log.result` mendapat `result_paths` (dari `base.SearchHits.Summary`, foundation F5) |
| bus `core.audit.record` | Call (sudah ada, dikonsumsi gateway) | v1 | Tidak diubah; dipakai tes AC6 untuk membuktikan fail-open |
| bus `project.knowledge.search` | Call (sudah ada) | v1 | Dipalsukan di unit test via `fakeBus` |

Prasyarat: foundation F5 selesai (`ResultPaths()`, `Summary()` baru, `TruncateUTF8`, konstanta `MaxResultPaths`, `ResultPathsBudgetBytes`). Usecase ini dilarang menyentuh `base/`, `handler.go`, `usecase.go`, `params.go`, `init.go`, `repository.go`.

---

## 2. Summary Plan

Perilaku baru (`result_paths` di audit) datang dari entity `SearchHits.Summary()` di foundation. Handler `searchknowledge/handler.go` sudah memanggil `hits.Result()` sehingga `Summary` mengalir ke `toolkit.Result.Summary` lalu disalin gateway ke `audit_log.result`. Karena itu tidak ada perubahan kode feature. Pekerjaan usecase ini hanya:

1. Unit test di paket `searchknowledge` yang membuktikan Summary yang keluar dari handler membawa `result_paths` sesuai AC1-AC4 plus batas panjang/anggaran.
2. Snapshot golden `Text()` dan `Structured()` untuk AC5 (bukti teks agent tidak berubah).
3. Tes integrasi baru yang memanggil `search_knowledge` lewat MCP sungguhan lalu membaca `audit_log` (AC1, AC2, AC4 end-to-end) dan memastikan kegagalan `core.audit.record` tidak mengubah respons agent (AC6).
4. Memperbarui `docs/mcp-gateway.md` bagian Audit.

Rule bisnis tidak ditulis di sini (ada di entity). Tes handler memakai `fakeBus` yang sudah ada di `handler_test.go` (tidak diubah; file baru hanya memakai helper-nya karena satu paket).

---

## 3. Plan (langkah berurutan)

### Langkah 1. Unit test Summary lewat handler

File baru: `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/summary_test.go` (package `searchknowledge`). Memakai `newFakeBus(true)` dan `f.call(projectID, args)` dari `handler_test.go`; `f.resp` diisi per kasus. Tambah helper lokal di file ini saja:

```go
// items membangun n search.Item dengan path "docs/f%02d.md", skor menurun, snippet "sn %d".
func items(n int) []search.Item

// resultPaths mengambil Summary["result_paths"] sebagai []string; fatal bila bukan []string.
func resultPaths(t *testing.T, s map[string]any) []string
```

Catatan tipe: `Summary()` mengembalikan `[]string` (bukan `[]any`) karena entity memegang nilai Go; tes harus assert `[]string`. Verifikasi bentuk JSON-nya lewat `json.Marshal(r.Summary)`.

Fungsi tes (semua tabel/singular, tanpa `t.Parallel` karena `fakeBus` tidak thread-safe):

| Fungsi | Skenario | Asersi |
|---|---|---|
| `TestSummary_AC1_ThreePathsInOrder` | `f.resp = {Items: items(3), Total: 3}`, args `{"query":"q"}` | `result_paths` == `[docs/f01.md, docs/f02.md, docs/f03.md]`; urutan sama dengan baris `N. path` di `r.Text` (parse regex `^\d+\. (\S+)  \(skor`) dan dengan `r.Structured["items"]`; `count==3`, `total==3`; tidak ada kunci `result_paths_truncated` |
| `TestSummary_AC2_CappedAtTen` | `Items: items(15)`, `Total: 15`, args `{"query":"q","limit":20}` | `len(result_paths)==10` dan sama dengan 10 path pertama; `count==15`; `total==15`. Varian `Total: 42` -> `total==42`, `count==15` (count = jumlah item, total sebenarnya dari resp) |
| `TestSummary_AC4_NoResultsEmptyArray` | `f.resp = search.Response{}` | `result_paths` bertipe `[]string` non-nil panjang 0; `json.Marshal(r.Summary)` memuat `"result_paths":[]` dan TIDAK memuat `"result_paths":null`; `total==0`, `count==0` |
| `TestSummary_AC3_NoLeakedKeys` | 3 item dengan snippet `**rate**` dan skor 0.6079, query `"rahasia-query"` | kunci Summary persis `{count,total,result_paths}` (len 3); JSON Summary tidak memuat substring `snippet`, `score`, `content`, `query`, `rahasia-query`, `**rate**`, `0.6079` |
| `TestSummary_LongPathTruncatedAtUTF8` | 1 item path 1500 byte multibyte (`"docs/" + strings.Repeat("é", 800)` = 1605 byte; dan varian 3-byte `日` agar batas 1024 jatuh di tengah rune) | `len(path) <= record.MaxTargetBytes` (1024), `utf8.ValidString(path)`, path adalah prefix dari path asli, dan `len(path)` adalah byte terbesar <= 1024 pada batas rune (mis. `1024` untuk `é`, `1023` bila rune 3-byte jatuh di tengah). Import `grimoire/lib/contracts/core/audit/record` (boleh: `lib/contracts`) |
| `TestSummary_BudgetTruncationFlag` | 10 item, tiap path tepat 1024 byte ASCII (`"docs/" + strings.Repeat("a", 1019)`), Total 10 | `result_paths_truncated == true` (bool); `len(result_paths) < 10`; anggaran JSON `sum(len(json.Marshal(p))) + commas + 2 <= base.ResultPathsBudgetBytes` (6144) - memakai konstanta exported dari `base` yang sudah boleh di-import oleh feature; dengan rumus foundation hasil yang diharapkan adalah 5 path (6 path = 6163 byte > 6144); simpan asersi `len==5` bersama komentar rumus; JANGAN meng-import `auditlog/base`. Lalu simulasi gateway: `m := map[string]any{"bytes":99999,"original_bytes":99999,"truncated":false,"duration_ms":99999,"error_code":"invalid_request"}` digabung dengan Summary, `json.Marshal` panjangnya `< record.MaxResultJSONBytes` (8192); dan `_truncated` TIDAK menjadi kunci (`_, has := merged["_truncated"]; !has`) - artinya `Sanitize` tidak akan mengganti seluruh result |
| `TestSummary_BudgetNoFlagWhenOnlyCapTen` | 15 path pendek | `result_paths_truncated` tidak ada (pembuangan karena batas 10 bukan karena anggaran) |
| `TestSummary_EscapedCharsCountedByEncodedLength` | 10 path berisi `<>&` (mis. `"docs/" + strings.Repeat("<", 150)` = 155 byte, ter-escape ~6x) | `json.Marshal(result_paths)` panjang <= 6144 (anggaran memakai panjang ter-escape; `json.Marshal` memang meng-escape `<>&`), flag `result_paths_truncated` true bila ada yang dibuang |
| `TestSummary_PathOrderPreservedIncludingDuplicatePrefix` | item path acak non-alfabet (`z.md`, `a.md`, `m.md`) | urutan `result_paths` == urutan input, tidak diurutkan ulang |

Aturan: assert pesan kegagalan memuat nilai aktual (`t.Fatalf("%v", ...)`); jangan membuat ulang logika `ResultPaths` di tes (hitung ekspektasi manual/literal, kecuali rumus anggaran yang dihitung ulang sebagai oracle independen sederhana).

### Langkah 2. Snapshot golden Text() dan Structured() (AC5)

File baru: `searchknowledge/snapshot_test.go` (package `searchknowledge`), flag `var updateGolden = flag.Bool("update-golden", false, "tulis ulang testdata/*.golden")`. Penamaan flag berbeda dari `-update` di `knowledgetools/base_test.go` karena itu paket lain; di paket ini belum ada flag `update`, namun gunakan `update-golden` agar tidak ambigu.

Fixture tetap (konstanta di file tes), `resp` dibangun langsung (tanpa bus) lewat `base.NewSearchHits(query, prefix, resp)` dan lewat handler untuk kasus berikut:

| Nama kasus | Input | File golden |
|---|---|---|
| `three_hits` | query `rate limit`, prefix `docs`, 3 item (skor 0.6079, 0.4, 0.12345; satu snippet kosong, satu dengan newline/`**bold**`), Total 3 | `testdata/three_hits.text.golden`, `testdata/three_hits.structured.golden` |
| `more_hits_hint` | query `a`, prefix `""`, 2 item, Total 9 (memicu baris "[ada 7 hasil lain ...]") | `more_hits_hint.text.golden`, `more_hits_hint.structured.golden` |
| `no_hits` | query `x`, prefix `docs/guides`, 0 item | `no_hits.text.golden`, `no_hits.structured.golden` |

Isi `*.structured.golden` = `json.MarshalIndent(h.Structured(), "", "  ")` + `\n` (urutan kunci peta Go deterministik oleh `encoding/json`). Isi `*.text.golden` = `h.Text()` apa adanya.

Tes `TestSnapshot_TextAndStructuredUnchanged`: loop kasus, bandingkan dengan file golden byte-per-byte; `-update-golden` menulis ulang. Tambahan: lewat handler (`f.call`) hasil `r.Text` dan `r.Structured` harus sama dengan nilai yang diturunkan dari golden (membuktikan jalur handler tidak menambah apa pun).

**Pembuatan golden (wajib dari commit sebelum T11)**, supaya bukti AC5 bermakna:

1. `git worktree add <scratchpad>/pre-t11 c6d1f13` (HEAD sebelum foundation T11; hash dari `git log` saat plan ditulis. Bila lupa hash: commit induk dari commit pertama T11).
2. Salin hanya `snapshot_test.go` ke `<scratchpad>/pre-t11/backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/`.
3. Jalankan di worktree itu: `go test ./internal/domain/discovery/internal/knowledgetools/searchknowledge/ -run TestSnapshot -update-golden`.
4. Salin direktori `testdata/` hasilnya ke repo kerja, commit sebagai bagian usecase ini. Hapus worktree.
5. Di repo kerja, jalankan `go test ... -run TestSnapshot` TANPA flag: harus hijau (membuktikan foundation F5 tidak mengubah Text/Structured).

`snapshot_test.go` tidak boleh meng-import apa pun yang baru ada setelah F5 (hanya `base.NewSearchHits`, `Text`, `Structured`) agar dapat dijalankan di commit lama.

### Langkah 3. Tes integrasi `searchquality_audit_integration_test.go`

File baru: `backend/cmd/internal/server/searchquality_audit_integration_test.go`, `//go:build integration`, package `server`. Semua helper berprefiks `sqa` (konvensi overview bagian 8). Memakai tanpa mengubah: `setupMCP`, `mcpEnv.mcpCall`, `mcpEnv.seedKnowledge`, `mcpEnv.newProject` dari file tes yang sudah ada. Default `MCP_RATE_PER_MINUTE` = 120 sudah cukup (jumlah panggilan < 30), jadi tidak perlu varian setup.

Helper:

```go
// sqaAuditRow adalah baris audit_log terbaru untuk action mcp.search_knowledge.
type sqaAuditRow struct {
	Target string
	Status string
	Args   map[string]any
	Result map[string]any
}

// sqaLastSearchAudit membaca baris terbaru: SELECT target, status, args, result FROM <schema>.audit_log
// WHERE action='mcp.search_knowledge' ORDER BY occurred_at DESC, id DESC LIMIT 1 (lewat e.admin; schema dari e.schema).
func sqaLastSearchAudit(t *testing.T, e *mcpEnv) sqaAuditRow

// sqaPathsFromText mengurai baris "N. path  (skor ...)" dari ktResult.Text.
func sqaPathsFromText(text string) []string

// sqaPathsFromResult mengubah result["result_paths"] ([]any) menjadi []string; fatal bila bukan array string.
func sqaPathsFromResult(t *testing.T, res map[string]any) []string

// sqaSeed menyiapkan n file "kb/doc%02d.md" yang semuanya memuat kata "kuota" (isi unik per file).
func sqaSeed(e *mcpEnv, n int)
```

Fungsi tes:

| Fungsi | Skenario | Asersi |
|---|---|---|
| `TestT11_Int_AC1_ResultPathsInAudit` | `sqaSeed(e, 3)`; `mcpCall(rawKey, "search_knowledge", {"query":"kuota"})` | respons `!IsError`; baris audit `status=ok`; `result.result_paths` == `sqaPathsFromText(Text)` == path dari `StructuredContent.items` (urut sama, 3 item); `result.count==3`, `result.total==3`; tidak ada kunci `_truncated` di result; `result` memuat `bytes`, `duration_ms` (kunci gateway tetap ada); `args.query=="kuota"`; `result` tidak memuat string `snippet`/`score` (marshal lalu `strings.Contains`) |
| `TestT11_Int_AC2_CappedTenWithRealTotal` | `sqaSeed(e, 15)`; panggil dengan `limit: 15` | `Text`/`StructuredContent.items` berisi 15 item; audit `result.count==15`, `result.total==15`, `len(result_paths)==10`, 10 path = 10 item pertama dari teks |
| `TestT11_Int_AC4_NoResultsEmptyArray` | query `zzzzzzzz-tidak-ada` | audit `result.result_paths` ada, bertipe `[]any` panjang 0 (bukan `nil`: cek `res["result_paths"] != nil` sebelum type assertion), `result.total==0`, `count==0`; baca JSON mentah kolom `result` (`result::text`) memuat `"result_paths": []` (jsonb menormalisasi spasi: gunakan `strings.Contains(raw, "\"result_paths\": []")`) |
| `TestT11_Int_AC3_NoContentInAudit` | seed satu file dengan kalimat unik `RAHASIA-ISI-UNIK`; cari `RAHASIA-ISI-UNIK` | `result::text` dan `args` hanya memuat query di `args.query`; `result::text` TIDAK memuat `RAHASIA-ISI-UNIK` (query ada hanya di args) dan tidak ada kunci `snippet`/`score`/`content` |
| `TestT11_Int_AC6_AuditFailureDoesNotChangeAgentResponse` | lihat di bawah | respons identik, tidak ada error |
| `TestT11_Int_ProjectIsolation` | seed file `kuota` di project B (`e.newProject`) | `result_paths` di audit project A tidak memuat path project B; baris audit `project_id` = project A (SELECT `project_id::text`) |

Detail `TestT11_Int_AC6`:

1. `sqaSeed(e, 3)`; panggilan kontrol `ctrl := e.mcpCall(key, "search_knowledge", {"query":"kuota"})` (audit normal tercatat; `before := e.countAudit("mcp.search_knowledge")` == 1).
2. Rusak jalur `core.audit.record` tanpa menyentuh kode produksi: lewat koneksi admin ganti nama tabel di skema tes: `ALTER TABLE <schema>.audit_log RENAME TO audit_log_sqa_broken`. Daftarkan `t.Cleanup` yang mengembalikan nama (`... RENAME TO audit_log`) bila belum dikembalikan (idempoten, abaikan error "does not exist"). Alasan memilih opsi ini: registry bus tidak punya override/hook (`Register` panic pada duplikat), sehingga mengganti handler `core.audit.record` tidak mungkin tanpa mengubah kode non-tes; rename tabel membuat INSERT gagal dengan error SQL nyata sehingga `auditwriter` mengembalikan error (jalur fail-open yang sesungguhnya).
3. `broken := e.mcpCall(key, "search_knowledge", {"query":"kuota"})`. Assert: `!broken.IsError`; `broken.Text == ctrl.Text`; `reflect.DeepEqual(broken.StructuredContent, ctrl.StructuredContent)`; `broken.Meta` sama (tidak ada `truncated` baru). Tidak ada JSON-RPC error (`mcpCall` sudah fatal bila ada).
4. Kembalikan nama tabel; `e.countAudit("mcp.search_knowledge")` masih `before` (panggilan saat rusak tidak tercatat, tidak ada baris parsial). Panggil sekali lagi (`query` lain) dan assert baris baru muncul dengan `result_paths` (pulih normal, server tidak mati).
5. Skenario tambahan di fungsi sama: panggilan saat rusak untuk query tanpa hasil juga sukses (error audit tidak memengaruhi `isError`).

Keterbatasan yang dicatat sebagai komentar di tes: DDL admin berjalan pada skema tes terisolasi (`testDBURL` membuat skema per-tes), jadi tidak mengganggu tes lain.

### Langkah 4. Perbarui `docs/mcp-gateway.md`

Di bagian `## Audit` (baris sekitar 69-71), tambah paragraf dan tabel kecil SETELAH paragraf yang sudah ada, tanpa mengubah isinya:

```markdown
### Ringkasan audit `search_knowledge`

Entri `mcp.search_knowledge` membawa `result` berikut (selain `bytes`, `original_bytes`, `truncated`, `duration_ms`, dan `error_code` dari gateway):

| Kunci | Tipe | Keterangan |
|---|---|---|
| `count` | integer | Jumlah hasil yang dikembalikan ke agent |
| `total` | integer | Jumlah dokumen cocok sebelum `limit` |
| `result_paths` | array string | Path hasil teratas, urut sama dengan keluaran, maksimal 10. Tiap path dipotong pada batas UTF-8 1024 byte (sama dengan batas `target` audit `read_file`, sehingga bisa dicocokkan persis). Tanpa hasil: `[]` (bukan `null`) |
| `result_paths_truncated` | boolean | Hanya ada (`true`) bila sebagian path dibuang karena anggaran ukuran (6144 byte JSON) agar `result` tidak melewati batas 8192 byte `Sanitize` |

Contoh:

    {"count": 15, "total": 42, "result_paths": ["docs/a.md", "docs/b.md", "...hingga 10 path"], "bytes": 3120, "original_bytes": 3120, "truncated": false, "duration_ms": 21}

Snippet, skor, isi file, dan query tidak masuk ke `result` (query tercatat di `args.query`). Path adalah metadata yang juga tercatat sebagai `target` saat `read_file`, bukan isi file.

Alasan membalik keputusan T08 ("tanpa path hasil"): metrik kualitas pencarian T11 (follow-up `read_file`, rank yang dibuka, reformulasi) hanya dapat dihitung bila urutan hasil tercatat. Batas 10 path dan anggaran 6144 byte menjaga baris audit kecil; `result` yang melewati 8192 byte akan diganti seluruhnya oleh `{_truncated, ...}` dan `count`/`total` hilang, sehingga entity memotong sebelum itu terjadi. Entri sebelum T11 tidak memiliki `result_paths` dan dilewati oleh laporan kualitas pencarian.
```

(Penulisan sebenarnya memakai blok kode berpagar tiga backtick untuk contoh JSON; di atas diindentasi hanya agar plan ini tidak bersarang.) Perbarui juga satu kalimat di bagian Module `discovery/knowledge-tools` (baris sekitar 94): "dan metadata audit tanpa isi file" menjadi "dan metadata audit tanpa isi file (path hasil pencarian dicatat sebagai `result_paths`, lihat Audit)". Jangan mengubah bagian lain. Jangan mengubah `tools_list.golden.json`.

### Langkah 5. Verifikasi

```
cd backend
go test ./internal/domain/discovery/internal/knowledgetools/... 
go vet ./internal/domain/discovery/... ./cmd/...
go test -tags integration ./cmd/internal/server/ -run 'TestT11_Int|TestT08_Int' 
go test ./internal/archtest/...
```

Integrasi memerlukan `TEST_DATABASE_URL` (tes skip bila kosong; di CI wajib). Semua tes T08 yang sudah ada harus tetap hijau tanpa diubah. Cek bahwa `git diff` tidak menyentuh file di luar allowedPaths.

---

## 4. Migration

Tidak ada. Tidak ada perubahan skema `audit_log`. (Tes AC6 mengganti nama tabel hanya di skema tes sementara via koneksi admin dan mengembalikannya; itu bukan migrasi.)

---

## 5. Changes

| File | Perubahan |
|---|---|
| `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/summary_test.go` | baru: tes AC1-AC4 + batas panjang/anggaran lewat handler dengan `fakeBus` |
| `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/snapshot_test.go` | baru: tes snapshot Text/Structured + flag `-update-golden` |
| `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/testdata/{three_hits,more_hits_hint,no_hits}.{text,structured}.golden` | baru: 6 file golden, dibuat dari commit sebelum T11 |
| `backend/cmd/internal/server/searchquality_audit_integration_test.go` | baru: `TestT11_Int_AC1/AC2/AC3/AC4/AC6/ProjectIsolation` + helper `sqa*` |
| `docs/mcp-gateway.md` | bagian Audit: kunci `result_paths`, batas 10, anggaran 6144, `result_paths_truncated`, alasan membalik T08; satu kalimat di bagian knowledge-tools |

Tidak berubah: `handler.go`, `usecase.go`, `params.go`, `init.go`, `repository.go`, `handler_test.go` (existing, boleh dibaca/dipakai), `base/**`, `mcpgateway_integration_test.go`, `knowledgetools_integration_test.go`.

Catatan `handler_test.go` existing: `TestHandler_Success` meng-assert `len(r.Summary) != 2` dan pada summary hanya `count`/`total`. Setelah F5 Summary punya 3 kunci, sehingga tes itu akan GAGAL. Perbaikan assert tersebut milik tahap foundation/wiring bila sudah dilakukan di F5; bila belum, usecase ini WAJIB mengubah satu baris itu di `handler_test.go` (file masih di dalam allowedPath `searchknowledge/`): `len(r.Summary) != 3` dan tambahkan cek `result_paths == []string{"docs/guides/b.md"}`. Bagian lain tes itu (assertion bahwa Summary tidak membocorkan `rate`) tetap berlaku dan lulus karena path `docs/guides/b.md` tidak memuat "rate" sebagai substring? Perhatikan: `docs/guides/b.md` tidak mengandung `rate`; aman. Lakukan perubahan minimal ini dan sebutkan di pesan commit.

---

## 6. Test Scenario

### Unit (`searchknowledge`)

Lihat tabel Langkah 1 dan Langkah 2. Ringkas pemetaan ke AC tiket:

| AC | Tes |
|---|---|
| AC1 | `TestSummary_AC1_ThreePathsInOrder` |
| AC2 | `TestSummary_AC2_CappedAtTen` |
| AC3 | `TestSummary_AC3_NoLeakedKeys` |
| AC4 | `TestSummary_AC4_NoResultsEmptyArray` |
| AC5 | `TestSnapshot_TextAndStructuredUnchanged` (golden dari commit pra-T11) |
| Batas | `TestSummary_LongPathTruncatedAtUTF8`, `TestSummary_BudgetTruncationFlag`, `TestSummary_BudgetNoFlagWhenOnlyCapTen`, `TestSummary_EscapedCharsCountedByEncodedLength`, `TestSummary_PathOrderPreservedIncludingDuplicatePrefix` |

Edge yang wajib tercakup: query berisi tanda kutip/tanda `<>&` tidak memengaruhi Summary; `Prefix` terisi tidak muncul di Summary; error bus (`ErrInvalid`, error Go) tidak menghasilkan `result_paths` (Summary hanya berisi `error_code` dari `BusResult`; assert `result_paths` absen pada `TestHandler_ErrInvalidBecomesToolError` varian baru `TestSummary_ErrorResultHasNoResultPaths`).

### Integrasi (`cmd/internal/server`, tag `integration`)

Lihat tabel Langkah 3: AC1, AC2, AC3, AC4, AC6, isolasi project. Seluruhnya terhadap server sungguhan dan Postgres tes.

### Arsitektur

`internal/archtest` tetap hijau: file tes baru hanya meng-import `base`, `record` (kontrak), dan stdlib; tes integrasi hanya meng-import paket server yang sudah ada.

---

## 7. E2E Scenarios

Positif:
1. Agent mencari "kuota" pada project dengan 3 file cocok -> teks ke agent seperti biasa (identik golden), `audit_log` baris `mcp.search_knowledge` punya `result_paths` 3 path berurutan sama dengan teks, `count=3`, `total=3`.
2. Pencarian dengan `limit: 15` pada 15 file cocok -> agent menerima 15 hasil; audit mencatat `count=15`, `total=15`, hanya 10 path pertama.
3. Pencarian tanpa hasil -> teks "Tidak ada hasil ..." tidak berubah; audit `result_paths: []`, `total: 0`.
4. Setelah audit pulih (tabel dikembalikan), pencarian berikutnya kembali tercatat lengkap dengan `result_paths`.

Negatif:
1. `core.audit.record` gagal (tabel `audit_log` tidak tersedia): respons agent identik byte-per-byte (teks dan `structuredContent`) dengan panggilan kontrol, `isError=false`, tidak ada JSON-RPC error, server tetap melayani request berikutnya.
2. Argumen invalid (`query` kosong, `limit: 0`, `project_id` di argumen) -> `isError` seperti sebelumnya; audit `status=error` tanpa `result_paths`.
3. Path sangat panjang atau multibyte -> audit tetap tercatat (`status=ok`), `result` bukan `{_truncated...}`; `count`/`total` utuh.
4. Hasil pencarian project lain tidak pernah muncul di `result_paths` project pemanggil.
5. Isi file/snippet/skor/query tidak pernah muncul di `audit_log.result`.

---

## 8. Request/Response body

### 8.1 Request MCP (tidak berubah)

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"kuota","path":"kb","limit":15}}}
```

### 8.2 Respons MCP (tidak berubah; golden AC5)

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "result": {
    "content": [{"type": "text", "text": "3 dari 3 hasil untuk \"kuota\" di kb/\n1. kb/doc01.md  (skor 0.608)\n   **kuota** harian per key\n2. kb/doc02.md  (skor 0.412)\n   batas **kuota** bulanan\n3. kb/doc03.md  (skor 0.120)\n   sisa **kuota**\nBaca isi lengkap dengan read_file(path)."}],
    "structuredContent": {
      "items": [
        {"path": "kb/doc01.md", "score": 0.6079, "snippet": "**kuota** harian per key"},
        {"path": "kb/doc02.md", "score": 0.412, "snippet": "batas **kuota** bulanan"},
        {"path": "kb/doc03.md", "score": 0.12345, "snippet": "sisa **kuota**"}
      ],
      "total": 3,
      "query": "kuota"
    },
    "isError": false
  }
}
```

### 8.3 `audit_log.result` setelah T11

3 hasil:

```json
{
  "count": 3,
  "total": 3,
  "result_paths": ["kb/doc01.md", "kb/doc02.md", "kb/doc03.md"],
  "bytes": 410, "original_bytes": 410, "truncated": false, "duration_ms": 12
}
```

15 hasil (limit 15), total sebenarnya 15, hanya 10 path:

```json
{
  "count": 15,
  "total": 15,
  "result_paths": ["kb/doc01.md", "kb/doc02.md", "kb/doc03.md", "kb/doc04.md", "kb/doc05.md",
                  "kb/doc06.md", "kb/doc07.md", "kb/doc08.md", "kb/doc09.md", "kb/doc10.md"],
  "bytes": 1980, "original_bytes": 1980, "truncated": false, "duration_ms": 18
}
```

Tanpa hasil:

```json
{"count": 0, "total": 0, "result_paths": [], "bytes": 160, "original_bytes": 160, "truncated": false, "duration_ms": 5}
```

Anggaran terlampaui (10 path @1024 byte; 5 path tersisa):

```json
{"count": 10, "total": 10, "result_paths": ["<5 path, tiap 1024 byte>"], "result_paths_truncated": true,
 "bytes": 6000, "original_bytes": 6000, "truncated": false, "duration_ms": 9}
```

`audit_log.args` untuk semua kasus: `{"query": "kuota", "path": "kb", "limit": 15}` (tanpa perubahan).

### 8.4 Format error

Tidak ada endpoint/format error baru. Perilaku error sebelumnya tetap, termasuk Summary `error_code`:

Argumen invalid (tool result, bukan JSON-RPC error):

```json
{
  "jsonrpc": "2.0", "id": 7,
  "result": {
    "content": [{"type": "text", "text": "Argumen tidak valid: query wajib diisi 1-200 karakter. Contoh: query=\"rate limit\"."}],
    "isError": true
  }
}
```

Audit untuk kasus itu: `status=error`, `result = {"error_code": "invalid_argument", "bytes": ..., "duration_ms": ...}` dan TIDAK memuat `result_paths`.

Kegagalan `core.audit.record` tidak muncul ke agent sama sekali (fail-open): respons agent sama seperti 8.2; hanya log server berisi kegagalan audit (diverifikasi tidak ada perubahan perilaku, bukan isi log).
