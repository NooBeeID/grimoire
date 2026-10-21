# T11 usecase `searcheval`: Runner golden set offline

Tiket: `docs/tickets/T11-discovery-search-quality.md`. Foundation: `docs/superpowers/plans/discovery-search-quality/overview.md` (D10, D11, K4, K5; bila bertentangan, overview menang).

Konteks yang dicek: tidak ada `CLAUDE.md`; tidak ada rules yang menyentuh usecase ini (`.claude/rules/migrations.md` tidak relevan, tidak ada migrasi). Usecase ini bukan feature di dalam module: ia tidak menyentuh `backend/internal/domain/**`. Isinya satu paket util netral, fixture data, satu tes integrasi di `cmd/internal/server`, target makefile, dan dokumen. Tidak ada UI, jadi tidak ada bagian Visual.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket / usecase | T11 / `searcheval` |
| Scope | backend |
| Domain / Module | discovery / knowledge-tools (resource root `backend/internal/domain/discovery/internal/knowledgetools`; usecase ini tidak mengubah file di sana) |
| Kode baru | `backend/lib/util/rankeval/` (paket netral), `backend/cmd/internal/server/searcheval_integration_test.go` (hanya tes) |
| Migrasi | Tidak ada |
| `endpointsChanged` | false |

Endpoint yang dipakai (semuanya existing, tidak diubah):

| Endpoint | Method | Version | Peran di usecase ini |
|---|---|---|---|
| `/api/projects/{id}/knowledge/files` | POST | REST (tanpa versi, T05) | seed korpus fixture; 201 = sukses |
| `/api/projects`, `/api/projects/{id}/api-keys`, `/api/auth/login` | POST | REST (T04, T06, T01) | disiapkan oleh pola `setupMCP` |
| `/mcp` `tools/call` `search_knowledge` | POST | MCP JSON-RPC 2.0 (protocolVersion `2025-06-18`) | dipanggil per pasangan golden, `limit=10` |
| `make test-search-eval` | make target (baru) | - | menjalankan `TestSearchEval` |
| `make search-eval-baseline` | make target (baru) | - | menulis ulang `baseline.json` |

Tidak ada endpoint bus. Paket `rankeval` tidak berbicara ke bus.

allowedPaths (hanya ini yang boleh disentuh): `backend/lib/util/rankeval/`, `backend/testdata/searcheval/`, `backend/cmd/internal/server/searcheval_integration_test.go`, `makefile`, `docs/search-quality.md`. `go.mod` tidak berubah (`go.yaml.in/yaml/v3` v3.0.5 sudah ada). `mcpgateway_integration_test.go` dan `knowledgetools_integration_test.go` TIDAK diubah; yang dipakai ulang hanya yang sudah ada di package `server` (`mcpEnv`, `testDBURL`, `testCfg`, `startRun`, `newJar`, `(*mcpEnv).rest/newKey/seedKnowledge/mcpCall`).

---

## 2. Summary Plan

Menyediakan tolok ukur offline kualitas `search_knowledge`: korpus markdown tetap + golden set (query, path yang diharapkan, kind) dijalankan terhadap Postgres tes sungguhan lewat tool MCP, menghasilkan recall@1, recall@5, MRR (keseluruhan dan per kind), lalu dibandingkan dengan `baseline.json` yang di-commit. Tes gagal hanya bila salah satu dari tiga metrik keseluruhan turun lebih dari toleransi (default 0.02); perbaikan tidak pernah gagal.

Pembagian tanggung jawab:

- `lib/util/rankeval` (murni, tanpa I/O selain `Load*`/`Save*` file): model golden, validasi, metrik, baseline, perbandingan. Satu paket, satu tanggung jawab (metrik peringkat); tidak dipecah artifisial menjadi sub-paket.
- `searcheval_integration_test.go`: orkestrasi (setup server, seed, panggil, kumpulkan, lapor, bandingkan). Tidak ada rule metrik di sini.
- Fixture `backend/testdata/searcheval/`: data.
- Tes fixture (tanpa DB) di dalam `rankeval` memastikan golden valid dan tiap `expect` ada di korpus, sehingga kesalahan fixture tertangkap oleh `go test ./...` biasa, bukan hanya oleh `make test-search-eval`.

Prinsip anti-tujuan tiket: golden TIDAK boleh disalin dari keluaran pencarian. Urutan kerja developer mengikat (lihat Plan langkah 5-7): tulis korpus dan golden dulu, validasi, baru jalankan pencarian untuk pertama kali.

---

## 3. Plan (langkah berurutan)

### Langkah 1. Paket `backend/lib/util/rankeval/` (TDD: tes dulu per file)

Tidak boleh meng-import `grimoire/internal/**` atau `grimoire/cmd/**` (dicek tes arsitektur lokal, langkah 1g). Dependensi eksternal hanya `go.yaml.in/yaml/v3`.

**1a. `golden.go`**

```go
package rankeval

const (
	KindExactName = "exact-name"
	KindKeyword   = "keyword"
	KindSynonym   = "synonym"
	KindTypo      = "typo"
	KindPhrase    = "phrase"

	MinPairs        = 30
	MinPairsPerKind = 3
	TopK            = 10 // kedalaman peringkat yang dinilai MRR; sama dengan limit tool
)

// Kinds mengembalikan kind valid dalam urutan tetap (untuk laporan dan JSON deterministik).
func Kinds() []string

type Pair struct {
	Query  string   `yaml:"query"`
	Expect []string `yaml:"expect"`
	Kind   string   `yaml:"kind"`
}

type Golden struct {
	Pairs []Pair `yaml:"pairs"`
}

// Load membaca YAML golden dari path. Decoder memakai KnownFields(true) agar salah ketik kunci
// ("expects", "kinds") gagal keras. Load TIDAK memanggil Validate; pemanggil memanggilnya.
func Load(path string) (Golden, error)

// Parse sama dengan Load tetapi dari bytes (dipakai tes).
func Parse(b []byte) (Golden, error)

// Validate mengumpulkan SEMUA pelanggaran (errors.Join), bukan berhenti di yang pertama:
//  - len(Pairs) >= MinPairs
//  - tiap kind di Kinds() muncul >= MinPairsPerKind kali
//  - Kind tiap pasang termasuk Kinds()
//  - Query tidak kosong setelah strings.TrimSpace; query unik (perbandingan exact setelah TrimSpace,
//    karena Evaluate memakai query sebagai kunci map)
//  - Expect tidak kosong; tiap item tidak kosong, tanpa awalan "/", tanpa "\\", tanpa duplikat dalam satu pasang
func (g Golden) Validate() error
```

Pesan error mengutip indeks pasangan dan query (data fixture, bukan input user, aman), mis. `pasangan #7 (query "rate limt"): expect kosong`.

**1b. `metrics.go`**

```go
// RecallAtK = |expect ∩ ranked[:k]| / |expect|. k <= 0, expect kosong -> 0. Tidak ada NaN.
// Duplikat di ranked tidak dihitung dua kali (set).
func RecallAtK(ranked, expect []string, k int) float64

// ReciprocalRank = 1/rank (1-based) dari path expect pertama dalam ranked[:TopK]; 0 bila tidak ada.
func ReciprocalRank(ranked, expect []string) float64

type KindMetrics struct {
	Pairs   int     `json:"pairs"`
	Recall1 float64 `json:"recall_at_1"`
	Recall5 float64 `json:"recall_at_5"`
	MRR     float64 `json:"mrr"`
}

type Metrics struct {
	Pairs   int                    `json:"pairs"`
	Recall1 float64                `json:"recall_at_1"`
	Recall5 float64                `json:"recall_at_5"`
	MRR     float64                `json:"mrr"`
	PerKind map[string]KindMetrics `json:"per_kind"`
}

// Evaluate: results = map[query][]path (urut peringkat). Query golden yang tak ada di results
// diperlakukan sebagai peringkat kosong (skor 0, tidak panic). Kunci results yang bukan query
// golden diabaikan. Rata-rata aritmetika atas pasangan. Kind tanpa pasangan tidak muncul di PerKind.
func (g Golden) Evaluate(results map[string][]string) Metrics

type PairOutcome struct {
	Pair Pair
	Rank int      // rank 1-based expect pertama di top TopK; 0 = tidak ada
	Got  []string // top 5 path hasil (untuk log)
}

// Failures mengembalikan pasangan yang recall@5-nya < 1 (belum semua expect masuk top 5),
// urut sesuai urutan di golden. Dipakai hanya untuk pelaporan, tidak untuk gerbang.
func (g Golden) Failures(results map[string][]string) []PairOutcome
```

**1c. `baseline.go`**

```go
type Baseline = Metrics // satu skema JSON untuk laporan dan baseline

// SaveBaseline menulis JSON ber-indent 2 spasi, kunci per_kind urut Kinds(), semua float dibulatkan
// 4 desimal (math.Round(v*1e4)/1e4), diakhiri newline, 0644. Deterministik: diff git bersih.
func SaveBaseline(path string, m Metrics) error

// LoadBaseline membaca JSON; DisallowUnknownFields; error bila file tak ada (errors.Is(err, fs.ErrNotExist)
// tetap bisa diperiksa pemanggil lewat %w) atau Pairs <= 0.
func LoadBaseline(path string) (Baseline, error)
```

**1d. `compare.go`**

```go
const epsilon = 1e-9

type Drop struct {
	Metric string  // "recall_at_1" | "recall_at_5" | "mrr"
	Base   float64
	Cur    float64
	Delta  float64 // Base - Cur, > 0
}

// Compare membandingkan HANYA metrik keseluruhan (D11/K5; per kind hanya dicetak).
// Kedua sisi dibulatkan 4 desimal dulu (baseline di disk sudah 4 desimal) agar noise float tidak
// memicu gagal. Sebuah metrik dilaporkan bila (Base - Cur) > tol + epsilon: turun TEPAT sebesar tol
// masih lulus, perbaikan (Cur > Base) tidak pernah dilaporkan. tol < 0 diperlakukan sebagai 0.
// Urutan hasil tetap: recall_at_1, recall_at_5, mrr. Kosong = lulus.
func Compare(cur, base Metrics, tol float64) []Drop
```

**1e. `report.go`** (format laporan, supaya tes integrasi tipis dan bisa diuji unit)

```go
// FormatSummary mengembalikan baris-baris teks: "pairs=40 recall@1=0.xxxx recall@5=0.xxxx MRR=0.xxxx".
func FormatSummary(m Metrics) string
// FormatPerKind mengembalikan satu baris per kind (urut Kinds()): "synonym  pairs=8 r@1=... r@5=... mrr=...".
func FormatPerKind(m Metrics) []string
// FormatDrops: "recall_at_5 turun 0.0300 (baseline 0.7000 -> sekarang 0.6700)".
func FormatDrops(d []Drop) []string
```

**1f. Tes unit** (lihat bagian Test Scenario).

**1g. `arch_test.go`**: pakai `go/parser` (`ImportsOnly`) pada semua `*.go` non-test di folder paket; gagal bila ada import berawalan `grimoire/internal` atau `grimoire/cmd`.

### Langkah 2. Struktur fixture `backend/testdata/searcheval/`

```
backend/testdata/searcheval/
  corpus/<folder>/<nama>.md   (25 file, daftar di langkah 3)
  golden.yaml                 (40 pasang, langkah 4)
  baseline.json               (dihasilkan langkah 7, JANGAN ditulis tangan)
```

`testdata/` dilewati tooling Go; tidak ada `.go` di dalamnya.

### Langkah 3. Korpus (25 file, ditulis DULU, sebelum golden dan sebelum pencarian apa pun)

Setiap file 15-40 baris markdown realistis untuk tim produk fiktif "Payments Platform": judul `# ...`, beberapa seksi `##`, daftar, blok kode kecil, campuran bahasa Indonesia dan Inggris seperti tim sungguhan (istilah teknis Inggris, kalimat penjelas Indonesia). Aturan penulisan yang mengikat:

- Tulis sebagai dokumen kerja nyata, bukan sebagai jebakan atau umpan pencarian. Jangan menyisipkan sinonim query atau salah ketik ke dokumen agar pasangan tertentu lulus atau gagal.
- Konsisten dengan istilah tim: dokumen refund hanya memakai "refund" (bukan "pengembalian dana"), dokumen rollback memakai "rollback" dan "revert" (bukan "kembalikan versi"), dokumen invoice memakai "invoice" (bukan "tagihan"/"faktur"), dokumen webhook memakai "webhook" (bukan "callback") kecuali satu kalimat wajar, dst. Inilah yang membuat pasangan `synonym` secara jujur sulit untuk `'simple'` FTS.
- Boleh ada tumpang tindih alami antar dokumen (mis. `api/rate-limit.md` menyebut "API key" dan `auth/api-keys.md` menyebut "rate limit" satu kali), karena itu yang membuat peringkat bermakna. Jangan dirapikan.
- Path, isi, dan seluruh teks dikunci (di-commit). Mengubah korpus berarti mengubah baseline (aturan di dokumen).

| # | Path | Topik inti (istilah yang wajar muncul) |
|---|---|---|
| 1 | `auth/login-flow.md` | login, OAuth 2.0, authorization code, session cookie, refresh token, logout |
| 2 | `auth/api-keys.md` | API key, rotasi, revoke, prefix `gh_`, disimpan sebagai hash, satu key per integrasi |
| 3 | `auth/password-reset.md` | forgot password, reset password, reset token kedaluwarsa 30 menit, email |
| 4 | `auth/sso-saml.md` | SSO, SAML, Okta, IdP, metadata XML, attribute mapping |
| 5 | `auth/roles-permissions.md` | role, permission, super_admin, member, least privilege |
| 6 | `api/rate-limit.md` | rate limit, 429, `Retry-After`, per API key, burst, 120 request per menit |
| 7 | `api/pagination.md` | cursor pagination, `next_cursor`, `limit` maks 100, urutan stabil |
| 8 | `api/error-codes.md` | error codes, `INVALID_ARGUMENT`, `NOT_FOUND`, format error JSON |
| 9 | `api/versioning.md` | versi API `/v1`, deprecation, sunset header, kompatibilitas mundur |
| 10 | `deploy/release-process.md` | release, tag semver, changelog, canary, approval |
| 11 | `deploy/rollback.md` | rollback, revert, versi sebelumnya, kapan boleh rollback, verifikasi pasca-rollback |
| 12 | `deploy/docker-compose-staging.md` | staging, docker compose, healthcheck, seed data |
| 13 | `deploy/env-variables.md` | environment variables, `DATABASE_URL`, `SESSION_SECRET`, `.env.example`, secret manager |
| 14 | `deploy/database-migration.md` | database migration, urutan timestamp, expand-contract, lock, backup sebelum production |
| 15 | `payments/refund-policy.md` | refund, batas 14 hari, partial refund, approval finance |
| 16 | `payments/invoice-generation.md` | invoice, PDF, nomor urut, pajak PPN 11%, kirim email |
| 17 | `payments/payment-gateway-midtrans.md` | Midtrans, virtual account, VA kadaluarsa 24 jam, notifikasi pembayaran |
| 18 | `payments/idempotency-keys.md` | idempotency key, header `Idempotency-Key`, cegah duplicate charge, TTL 24 jam |
| 19 | `webhook/webhook-signature.md` | webhook signature, HMAC SHA-256, header `X-Signature`, timestamp tolerance, replay |
| 20 | `webhook/retry-policy.md` | retry, exponential backoff, maksimum 8 percobaan, dead letter, endpoint pelanggan |
| 21 | `webhook/event-catalog.md` | daftar event: `payment.succeeded`, `refund.created`, payload contoh |
| 22 | `onboarding/dev-setup.md` | setup lokal, `make dev`, Postgres via docker compose, seed |
| 23 | `onboarding/first-week-checklist.md` | checklist minggu pertama, akses repo, 1:1, tugas pertama (istilah "new hire") |
| 24 | `onboarding/code-review-guidelines.md` | code review, PR kecil, checklist reviewer, SLA review 1 hari kerja |
| 25 | `ops/incident-response.md` | incident, severity, on-call, rotation mingguan, postmortem blameless |

Jumlah 25 (rentang tiket "~25"; tes fixture menerima 20-30).

### Langkah 4. `golden.yaml` (40 pasang, ditulis dari kebutuhan nyata tim, SEBELUM pencarian dijalankan)

Skema:

```yaml
pairs:
  - query: "rate-limit"
    expect: ["api/rate-limit.md"]
    kind: exact-name
```

Isi final (query ditulis seperti yang akan diketik orang atau agent dalam pekerjaan sehari-hari; `expect` ditentukan dari niat query, bukan dari hasil pencarian). Komposisi: exact-name 8, keyword 10, synonym 8, typo 7, phrase 7 = 40.

| # | query | expect | kind |
|---|---|---|---|
| 1 | `rate-limit` | `api/rate-limit.md` | exact-name |
| 2 | `idempotency keys` | `payments/idempotency-keys.md` | exact-name |
| 3 | `webhook signature` | `webhook/webhook-signature.md` | exact-name |
| 4 | `refund policy` | `payments/refund-policy.md` | exact-name |
| 5 | `error codes` | `api/error-codes.md` | exact-name |
| 6 | `first week checklist` | `onboarding/first-week-checklist.md` | exact-name |
| 7 | `incident response` | `ops/incident-response.md` | exact-name |
| 8 | `rollback` | `deploy/rollback.md` | exact-name |
| 9 | `SAML Okta` | `auth/sso-saml.md` | keyword |
| 10 | `Retry-After 429` | `api/rate-limit.md` | keyword |
| 11 | `HMAC` | `webhook/webhook-signature.md` | keyword |
| 12 | `Midtrans virtual account` | `payments/payment-gateway-midtrans.md` | keyword |
| 13 | `DATABASE_URL` | `deploy/env-variables.md` | keyword |
| 14 | `cursor pagination` | `api/pagination.md` | keyword |
| 15 | `postmortem on-call` | `ops/incident-response.md` | keyword |
| 16 | `refresh token session cookie` | `auth/login-flow.md` | keyword |
| 17 | `changelog tag` | `deploy/release-process.md` | keyword |
| 18 | `reset token` | `auth/password-reset.md` | keyword |
| 19 | `kembalikan versi lama` | `deploy/rollback.md` | synonym |
| 20 | `batas permintaan` | `api/rate-limit.md` | synonym |
| 21 | `pengembalian dana` | `payments/refund-policy.md` | synonym |
| 22 | `tagihan pelanggan` | `payments/invoice-generation.md` | synonym |
| 23 | `kredensial akses programatik` | `auth/api-keys.md` | synonym |
| 24 | `lupa kata sandi` | `auth/password-reset.md` | synonym |
| 25 | `callback pelanggan` | `webhook/event-catalog.md`, `webhook/retry-policy.md` | synonym |
| 26 | `karyawan baru` | `onboarding/first-week-checklist.md` | synonym |
| 27 | `rate limt` | `api/rate-limit.md` | typo |
| 28 | `pagnation` | `api/pagination.md` | typo |
| 29 | `idempotency kyes` | `payments/idempotency-keys.md` | typo |
| 30 | `loggin flow` | `auth/login-flow.md` | typo |
| 31 | `rollbak` | `deploy/rollback.md` | typo |
| 32 | `retrry backoff` | `webhook/retry-policy.md` | typo |
| 33 | `enviroment variables` | `deploy/env-variables.md` | typo |
| 34 | `"rate limit" per API key` | `api/rate-limit.md` | phrase |
| 35 | `"reset password"` | `auth/password-reset.md` | phrase |
| 36 | `"exponential backoff"` | `webhook/retry-policy.md` | phrase |
| 37 | `"code review" checklist` | `onboarding/code-review-guidelines.md` | phrase |
| 38 | `"database migration" production` | `deploy/database-migration.md` | phrase |
| 39 | `"virtual account" kadaluarsa` | `payments/payment-gateway-midtrans.md` | phrase |
| 40 | `"on-call" rotation` | `ops/incident-response.md` | phrase |

Aturan tambahan:

- Query dalam YAML ditulis dengan petik tunggal bila memuat petik ganda (mis. `query: '"reset password"'`).
- Pasangan `synonym` dan `typo` DIHARAPKAN gagal pada baseline (K4); itu tujuan golden set. Jangan menambah sinonim ke korpus atau mengubah query agar lulus.
- Pasangan 25 sengaja multi-expect agar recall@k pecahan teruji pada data nyata.
- Setelah langkah 5 lulus, `golden.yaml` DIBEKUKAN sebelum pencarian pertama dijalankan. Bila setelah melihat hasil developer ingin mengubah pasangan, alasannya harus kebutuhan (query tak realistis, expect salah secara isi), bukan "supaya lulus"; perubahan itu dicatat di pesan commit.

### Langkah 5. Tes fixture tanpa DB: `backend/lib/util/rankeval/fixture_test.go`

Berjalan pada `go test ./...` biasa (tanpa build tag). Lokasi fixture relatif dari folder paket: `../../../testdata/searcheval`. Memastikan: `golden.yaml` lolos `Load` + `Validate`; tiap `expect` adalah file regular di `corpus/` (jalur dengan `/`); jumlah file korpus 20-30; tiap file korpus berekstensi `.md`, tidak kosong, UTF-8 valid, <= 100 KB; bila `baseline.json` ada, `LoadBaseline` lolos dan `Pairs == len(golden.Pairs)` (menangkap golden diubah tanpa memperbarui baseline). Sebelum langkah 7, `baseline.json` belum ada, tes tidak menuntutnya.

### Langkah 6. Tes integrasi `backend/cmd/internal/server/searcheval_integration_test.go`

Build tag `//go:build integration`, package `server`. Semua helper lokal berprefiks `se`.

```go
// seSetupMCP seperti setupMCP (mcpgateway_integration_test.go) tetapi dengan MCP_RATE_PER_MINUTE tinggi.
// Tidak mengubah setupMCP. Duplikasi ~25 baris disengaja (aturan overview 8: helper hanya di file milik usecase).
func seSetupMCP(t *testing.T) *mcpEnv
```

Isi `seSetupMCP`: `testDBURL(t)`; `testCfg(t, dbURL, map[string]string{"RUN_MIGRATION":"true","COOKIE_SECURE":"false","MCP_RATE_PER_MINUTE":"10000"})` (10000 = batas atas `intr` di config); `startRun` + `WithAppHook` menangkap `app.Deps`; `createadmin` via bus; login REST; buat project; `e.newKey(...)`. Mengembalikan `*mcpEnv` siap pakai (`rawKey`, `projID`).

Konstanta dan helper lain:

```go
const (
	seFixtureDir   = "../../../testdata/searcheval" // relatif ke cmd/internal/server
	seEnvTolerance = "SEARCH_EVAL_TOLERANCE"        // default 0.02
	seEnvUpdate    = "SEARCH_EVAL_UPDATE_BASELINE"  // "1" = tulis ulang baseline
)

func seTolerance(t *testing.T) float64      // kosong -> 0.02; strconv.ParseFloat; NaN/Inf/<0/>1 -> t.Fatalf
func seSeedCorpus(t *testing.T, e *mcpEnv)  // filepath.WalkDir corpus/, hanya *.md, path relatif ToSlash -> e.seedKnowledge(e.projID, rel, string(content))
func seSearch(t *testing.T, e *mcpEnv, q string) []string
```

`seSearch`: `e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": q, "limit": 10})`; `IsError` -> `t.Fatalf("search_knowledge error untuk %q: %s", q, r.Raw)` (pasangan golden yang valid tidak boleh membuat tool error; query berisi tanda kutip sudah ditangani `ParseQuery`); ambil `r.StructuredContent["items"].([]any)`, tiap `item.(map[string]any)["path"].(string)`; bentuk tak terduga -> `t.Fatalf`. Items kosong -> `[]string{}` (bukan error, itu hasil nol yang sah).

`TestSearchEval(t *testing.T)` alur:

1. `g, err := rankeval.Load(filepath.Join(seFixtureDir, "golden.yaml"))`; `g.Validate()`; gagal -> `t.Fatal`.
2. `tol := seTolerance(t)`; `update := os.Getenv(seEnvUpdate) == "1"`.
3. Bila bukan update: `rankeval.LoadBaseline(...baseline.json)`; error (termasuk tidak ada) -> `t.Fatalf("baseline.json tidak terbaca: %v (buat dengan: make search-eval-baseline)", err)`. Dibaca SEBELUM server dijalankan agar gagal cepat.
4. `e := seSetupMCP(t)`; `seSeedCorpus(t, e)`.
5. Untuk tiap pair berurutan: `results[p.Query] = seSearch(t, e, p.Query)`. Sekuensial (tanpa goroutine) agar deterministik dan jauh di bawah limiter.
6. `cur := g.Evaluate(results)`.
7. Laporan lewat `t.Log`/`t.Logf` (terlihat dengan `-v`): `FormatSummary(cur)`, lalu `FormatPerKind(cur)` per baris, lalu untuk tiap `g.Failures(results)` satu baris: `GAGAL [synonym] "batas permintaan" expect=[api/rate-limit.md] rank=0 top5=[...]`.
8. Bila `update`: `rankeval.SaveBaseline(path, cur)` (error -> `t.Fatal`), `t.Logf("baseline.json diperbarui: %s", FormatSummary(cur))`, return (tidak membandingkan; commit eksplisit oleh developer).
9. Bila `base.Pairs != cur.Pairs`: `t.Fatalf("jumlah pasangan golden (%d) != baseline (%d); golden berubah, perbarui baseline: make search-eval-baseline", ...)`. Membandingkan angka antar himpunan pasangan berbeda tidak bermakna.
10. `drops := rankeval.Compare(cur, base, tol)`; tiap drop -> `t.Errorf` memakai `FormatDrops`; ringkas `t.Logf("toleransi=%.4f baseline: %s", ...)`. Tanpa drop = lulus.

Tes ini TIDAK memanggil `t.Parallel()` dan tidak menyentuh state global selain env (dibaca, tidak diset). `Setenv` tidak dipakai di kode produksi tes.

### Langkah 7. Menghasilkan `baseline.json` (urutan wajib, tiap langkah di-gate)

1. Pastikan langkah 3-5 selesai: `cd backend && go test ./lib/util/rankeval/` hijau (fixture valid).
2. Commit/stage korpus dan `golden.yaml` (tanda bahwa golden dibekukan sebelum ada angka).
3. `make search-eval-baseline` (target langkah 8). Ini pertama kalinya pencarian dijalankan terhadap korpus.
4. Periksa `backend/testdata/searcheval/baseline.json`: `pairs` = 40, nilai di [0,1]. Sanity ringan (bukan penyetelan): `synonym` dan `typo` diharapkan jelas lebih rendah dari `exact-name`. Bila `exact-name` ikut sangat rendah, curigai bug fixture (path salah, seed gagal), bukan golden yang "dibetulkan".
5. `make test-search-eval` harus hijau dua kali berturut-turut tanpa perubahan (determinisme; ts_rank pada korpus tetap harus stabil). Bila flaky, catat dan telusuri (urutan seri skor sama?) sebelum lanjut; jangan melonggarkan toleransi.
6. Salin angka ke `docs/search-quality.md` bagian "Baseline saat ini".

### Langkah 8. `makefile`

Tambah ke `.PHONY` dan target (tab sebagai indentasi, gaya target `test-integration`):

```make
test-search-eval: dev-test-db ## golden set kualitas pencarian vs baseline (butuh Postgres)
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" SEARCH_EVAL_TOLERANCE="$(SEARCH_EVAL_TOLERANCE)" \
	  go test -count=1 -tags integration -run TestSearchEval -v ./cmd/internal/server/

search-eval-baseline: dev-test-db ## tulis ulang testdata/searcheval/baseline.json dari implementasi saat ini
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" SEARCH_EVAL_UPDATE_BASELINE=1 \
	  go test -count=1 -tags integration -run TestSearchEval -v ./cmd/internal/server/
```

Catatan: `-count=1` mencegah cache hasil tes (fixture dibaca lewat file; hasil lama tidak boleh dipakai). `SEARCH_EVAL_TOLERANCE` dikirim kosong bila tidak di-set; `seTolerance` memperlakukan string kosong sebagai default 0.02 (jangan memakai `?=` dengan nilai default berbeda dari kode, satu sumber default: kode). Tanpa `-race` (tes ini bukan tes konkurensi; mempercepat). Dua target ini bergantung pada `dev-test-db` persis seperti `test-integration`.

### Langkah 9. `docs/search-quality.md` (baru, Bahasa Indonesia)

Bagian wajib:

1. **Tujuan dan ruang lingkup**: dua alat ukur (sinyal produksi dari audit; tolok ukur offline), skor absolut korpus kecil bukan klaim performa produksi, hanya pembanding dua versi.
2. **Definisi metrik produksi** (keempatnya, sesuai tiket dan overview 5.2): zero-result rate (`total = 0`; penyebut = pencarian dengan `total` terbaca), follow-up rate (`read_file` oleh aktor+project sama, `0 < Δ <= 120s`, target di `result_paths`; penyebut = pencarian ber-`result_paths`), rank dibuka (bucket 1..10 dan `not_in_results`; read pertama yang cocok), reformulation rate (search `ok` berikutnya yang langsung menyusul, aktor sama, dalam jendela, query beda setelah normalisasi huruf kecil+spasi). Sebut aturan entri lama tanpa `result_paths` dan hanya status `ok`.
3. **Definisi metrik offline**: recall@k = `|expect ∩ top-k| / |expect|` dirata-rata; MRR = rata-rata `1/rank` expect pertama di top-10 (0 bila tidak ada); contoh hitung kecil (mis. expect 2 path, satu di rank 2 dan satu di rank 7: recall@1 = 0, recall@5 = 0.5, RR = 0.5).
4. **Cara membaca laporan CLI** `grimoire search-quality report --project <id> --since 7d [--until] [--window] [--json]` dengan contoh keluaran teks (dari overview 12.3), arti tiap baris, `Entri lama dilewati`.
5. **Metrik sebagai tren**: follow-up adalah perkiraan (agent bisa membuka karena alasan lain atau cuplikan sudah cukup), reformulasi bisa berarti eksplorasi; baca perubahan antar minggu, bukan nilai mutlak. Retensi audit 90 hari: simpan keluaran `--json` berkala (mis. mingguan) bila butuh tren jangka panjang; sarankan penamaan file berstempel tanggal.
6. **Menjalankan golden set**: `make test-search-eval`, apa yang dicetak (ringkasan, per kind, pasangan gagal), syarat lulus (turun > toleransi pada recall@1/recall@5/MRR keseluruhan), `SEARCH_EVAL_TOLERANCE`, bahwa perbaikan tidak gagal dan per kind hanya informasi.
7. **Menambah pasangan golden**: aturan (tulis dari kebutuhan nyata/istilah tim, SEBELUM menjalankan pencarian; jangan menyalin keluaran pencarian; `expect` harus path di `corpus/`; query unik; kind salah satu dari lima; minimal 30 pasang dan 3 per kind); bila perlu dokumen baru tambahkan ke `corpus/` (tulis natural, tanpa menyisipkan sinonim umpan); lalu perbarui baseline karena jumlah pasangan berubah; `go test ./lib/util/rankeval/` memeriksa fixture tanpa DB.
8. **Memperbarui baseline**: kapan (perubahan algoritma yang disengaja, perubahan golden/korpus), perintah `make search-eval-baseline`, tinjau diff `baseline.json`, commit eksplisit dengan alasan di pesan commit; jangan memperbarui baseline untuk menutupi penurunan yang tidak dimengerti.
9. **Baseline saat ini**: tanggal, commit, tabel keseluruhan (recall@1, recall@5, MRR, jumlah pasangan) dan per kind, disalin dari `baseline.json` hasil langkah 7. Angka diisi dari hasil nyata; plan ini sengaja tidak menebaknya. Catat bahwa `synonym` dan `typo` diharapkan rendah dan menjadi target perbaikan tiket berikutnya (trigram, stemming).
10. **Keterbatasan**: korpus 25 file, pencarian tanpa filter path, `limit=10`, satu bahasa campuran; skor tidak mewakili produksi.

### Langkah 10. Verifikasi akhir usecase

`cd backend && go build ./... && go vet ./... && go test ./lib/util/rankeval/ && go vet -tags integration ./cmd/internal/server/`, lalu `make test-search-eval` (hijau), lalu uji gerbang manual (lihat E2E negatif 1). Tidak ada file di luar allowedPaths yang berubah (`git status`).

---

## 4. Migration

Tidak ada. Tidak ada tabel, kolom, atau index baru. Fixture hanya memakai REST knowledge yang sudah ada pada Postgres tes (schema per tes dari `testDBURL`).

---

## 5. Changes

| File / folder | Perubahan |
|---|---|
| `backend/lib/util/rankeval/golden.go` | baru: `Kind*`, `Pair`, `Golden`, `Load`, `Parse`, `Validate`, `Kinds` |
| `backend/lib/util/rankeval/metrics.go` | baru: `RecallAtK`, `ReciprocalRank`, `Metrics`, `KindMetrics`, `(Golden).Evaluate`, `(Golden).Failures` |
| `backend/lib/util/rankeval/baseline.go` | baru: `Baseline`, `SaveBaseline`, `LoadBaseline` |
| `backend/lib/util/rankeval/compare.go` | baru: `Drop`, `Compare` |
| `backend/lib/util/rankeval/report.go` | baru: `FormatSummary`, `FormatPerKind`, `FormatDrops` |
| `backend/lib/util/rankeval/{golden,metrics,baseline,compare,report}_test.go` | baru: tes unit |
| `backend/lib/util/rankeval/fixture_test.go` | baru: tes fixture tanpa DB |
| `backend/lib/util/rankeval/arch_test.go` | baru: larangan import `internal/**` dan `cmd/**` |
| `backend/testdata/searcheval/corpus/**.md` | baru: 25 file |
| `backend/testdata/searcheval/golden.yaml` | baru: 40 pasang |
| `backend/testdata/searcheval/baseline.json` | baru: dihasilkan `make search-eval-baseline` |
| `backend/cmd/internal/server/searcheval_integration_test.go` | baru: `TestSearchEval`, helper `se*` |
| `makefile` | + `test-search-eval`, `search-eval-baseline`, entri `.PHONY` |
| `docs/search-quality.md` | baru |

Tidak berubah: `go.mod`/`go.sum`, kode domain mana pun, `mcpgateway_integration_test.go`, `knowledgetools_integration_test.go`, `docs/tickets/**` (katalog dan catatan ticket diurus foundation F7).

---

## 6. Test Scenario

### Unit `rankeval` (tabel, tanpa DB)

`metrics_test.go`:
- `RecallAtK`: expect 1 di rank 1 -> k=1 -> 1.0; expect di rank 2 -> k=1 -> 0, k=5 -> 1.0; expect 2 path, satu di rank 2 dan satu di rank 7 -> k=5 -> 0.5, k=10 -> 1.0; `k=0`/negatif -> 0; expect kosong -> 0 (bukan NaN, `math.IsNaN` false); ranked kosong/nil -> 0; duplikat di ranked tidak menggandakan hit.
- `ReciprocalRank`: rank 1 -> 1; rank 3 -> 1/3; tidak ada -> 0; expect di posisi 11 (di luar top-10) -> 0; beberapa expect -> memakai yang terdekat; ranked nil -> 0.
- `Evaluate`: golden 3 pasang dengan hasil tangan-hitung (nilai exact via toleransi 1e-9); query tak ada di results -> skor 0 tanpa panic; kunci results ekstra diabaikan; `PerKind` hanya memuat kind yang punya pasangan; `Pairs` = jumlah pasangan.
- `Failures`: hanya pasangan dengan recall@5 < 1, urutan sesuai golden, `Rank` benar (0 bila tak ada, 1-based), `Got` maksimal 5.

`golden_test.go`:
- `Parse` YAML valid; kunci tak dikenal (`expects:`) -> error; YAML rusak -> error; `Load` file tak ada -> error `errors.Is(err, fs.ErrNotExist)`.
- `Validate`: golden valid (30 pasang, 6 per kind) lolos; 29 pasang -> error; satu kind hanya 2 pasang -> error menyebut kind itu; kind tak dikenal -> error; expect kosong -> error; expect berisi string kosong, awalan `/`, backslash, duplikat -> error; query kosong/whitespace saja -> error; query duplikat (termasuk beda spasi tepi) -> error; beberapa pelanggaran sekaligus -> semua muncul di satu error gabungan.

`baseline_test.go`:
- Save lalu Load roundtrip sama (setelah pembulatan 4 desimal); output deterministik (dua kali Save -> byte identik), berakhir newline, urutan `per_kind` mengikuti `Kinds()`; nilai 0.123456 tersimpan 0.1235; `LoadBaseline` file tak ada -> error `fs.ErrNotExist`; JSON dengan kunci asing -> error; `pairs: 0` -> error; JSON rusak -> error.

`compare_test.go` (AC12):
- Baseline recall@5 = 0.70, sekarang 0.67 (turun 0.03), tol 0.02 -> satu `Drop{recall_at_5}` (gagal).
- Sekarang 0.73 (naik) -> kosong (lulus), untuk tiap dari tiga metrik.
- Sekarang 0.69 (turun 0.01) -> kosong (lulus).
- Turun tepat 0.02 -> lulus (batas inklusif; dijaga epsilon dan pembulatan agar 0.70-0.68 tidak gagal karena noise float).
- Turun hanya pada per kind (keseluruhan sama) -> kosong (per kind tidak menjadi gerbang).
- Beberapa metrik turun -> urutan `recall_at_1`, `recall_at_5`, `mrr`; `Delta` positif benar.
- `tol` negatif diperlakukan 0 (turun 0.0001 -> gagal); `tol` 0 dan nilai sama -> lulus.
- Campuran: R@1 naik, MRR turun 0.05 -> hanya MRR dilaporkan.

`report_test.go`: format ringkasan 4 desimal; per kind urut tetap; drops memuat nama metrik, delta, baseline dan nilai sekarang.

`arch_test.go`: tidak ada import terlarang (dan dites negatif pada potongan sumber sintetis lewat fungsi helper yang menerima source string).

`fixture_test.go` (menjaga data, tanpa DB): golden valid; setiap `expect` ada di `corpus/`; jumlah file korpus 20-30; semua `.md` tidak kosong, UTF-8 valid, <= 100 KB; `baseline.json` (bila ada) valid dan `Pairs` cocok dengan golden. Tambahan guard anti-"golden mengikuti keluaran": tes memastikan `synonym` dan `typo` masing-masing >= 3 pasang dan bahwa tidak ada query golden yang sama persis dengan nama file (tanpa ekstensi) milik `expect` pada kind `synonym`/`typo` (kind itu harus benar-benar memakai kata lain).

### Integrasi `TestSearchEval` (`//go:build integration`, Postgres tes)

- AC11: dengan `-v` log berisi ringkasan (`recall@1`, `recall@5`, `MRR`), satu baris per kind (5 baris), dan satu baris `GAGAL ...` per pasangan dengan recall@5 < 1 (termasuk pasangan `synonym`/`typo`). Output dicek manual di langkah 7; tes tidak menegaskan teksnya.
- AC12 end-to-end: lulus terhadap `baseline.json` hasil implementasi saat ini; gagal bila `golden`/pencarian memburuk (diuji manual, E2E negatif 1).
- AC13: dijamin `fixture_test.go` (>= 30, >= 3 per kind, expect ada di korpus).
- Seed 25 file lewat REST semuanya 201 (gagal -> `t.Fatalf` menyebut path); 40 panggilan `search_knowledge` tidak ada yang `isError` dan tidak ada yang kena rate limit (limiter 10000/menit; 80 request).
- Mode update (`SEARCH_EVAL_UPDATE_BASELINE=1`) menulis `baseline.json` valid dan tidak membandingkan; menjalankannya dua kali berturut-turut menghasilkan file byte identik (`git diff` kosong).
- Toleransi: `SEARCH_EVAL_TOLERANCE=abc`, `-0.1`, `2` -> `t.Fatalf` jelas; kosong -> 0.02.

### Arsitektur

`internal/archtest` hijau (tidak ada kode baru di `internal/`). `lib/util/rankeval` bebas import `internal/**`/`cmd/**` (tes lokal). `go.mod` tidak berubah (`git diff backend/go.mod` kosong).

---

## 7. E2E Scenarios

Tidak ada UI; E2E = server sungguhan + Postgres tes lewat `make`.

Positif:
1. Pada checkout bersih setelah langkah 7: `make test-search-eval` -> exit 0; log menampilkan recall@1, recall@5, MRR, 5 baris per kind, dan daftar pasangan gagal; `baseline.json` tidak berubah.
2. Perbaikan tidak gagal: sunting sementara `baseline.json` (lokal, jangan commit) menurunkan semua angka 0.1 -> `make test-search-eval` tetap hijau.
3. `make search-eval-baseline` pada kode yang tidak berubah -> `git diff --exit-code backend/testdata/searcheval/baseline.json` bersih (deterministik).
4. `SEARCH_EVAL_TOLERANCE=0.5 make test-search-eval` dengan baseline lama diturunkan simulasi kecil -> hijau (toleransi diterapkan).
5. `cd backend && go test ./lib/util/rankeval/` hijau tanpa Postgres.
6. Menambah satu pasangan golden valid + `make search-eval-baseline` -> `pairs` di baseline 41, lalu `make test-search-eval` hijau; `fixture_test` hijau.

Negatif:
1. Simulasi pencarian lebih buruk (sementara, lokal; jangan commit): mis. di `seSearch` kembalikan hasil dibalik urutannya, atau naikkan baseline.json secara manual 0.05 di `recall_at_5` -> `make test-search-eval` gagal dengan pesan `recall_at_5 turun 0.0500 (baseline ... -> sekarang ...)`, exit non-nol. Kembalikan perubahan.
2. `baseline.json` dihapus -> gagal cepat (sebelum server start) dengan petunjuk `make search-eval-baseline`.
3. `golden.yaml` ditambah pasangan tanpa memperbarui baseline -> `fixture_test` gagal (`Pairs` tidak cocok) dan `TestSearchEval` gagal dengan pesan "jumlah pasangan golden ... != baseline".
4. `expect` menunjuk file yang tidak ada di korpus -> `fixture_test` gagal menyebut path dan query.
5. Golden dengan 29 pasang, kind hanya 2, query duplikat, atau `expect: []` -> `Validate` melaporkan semua pelanggaran; `TestSearchEval` berhenti sebelum server start.
6. Kunci YAML salah ketik (`expects:`) -> `Load` gagal keras.
7. Postgres tidak jalan -> `make` berhenti di `dev-test-db`; tanpa `TEST_DATABASE_URL`, tes integrasi mengikuti perilaku `testDBURL` existing (skip/gagal sesuai konvensi tes lain), tidak ada perilaku baru.
8. `SEARCH_EVAL_TOLERANCE` tidak valid -> `t.Fatalf` dengan nilai yang diberikan dan rentang yang diterima.
9. Seed gagal (mis. REST mengembalikan non-201) -> `t.Fatalf` menyebut path dan status; tidak ada skor yang dicetak dari korpus setengah jadi.
10. Dua kali run berturut-turut menghasilkan angka berbeda (nondeterminisme) -> dianggap cacat fixture/pencarian yang harus ditelusuri sebelum baseline di-commit (langkah 7.5).

---

## 8. Request/Response body

### 8.1 `search_knowledge` (MCP JSON-RPC 2.0, dikirim `mcpCall`)

Request (setelah `initialize`):

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "search_knowledge",
    "arguments": { "query": "\"reset password\"", "limit": 10 }
  }
}
```

Response sukses (yang diambil hanya `structuredContent.items[].path`):

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "result": {
    "content": [{ "type": "text", "text": "2 dari 2 hasil untuk \"\"reset password\"\"\n..." }],
    "structuredContent": {
      "items": [
        { "path": "auth/password-reset.md", "score": 0.0759, "snippet": "forgot password dan **reset** **password** ..." },
        { "path": "auth/login-flow.md", "score": 0.0607, "snippet": "..." }
      ],
      "total": 2,
      "query": "\"reset password\""
    },
    "isError": false
  }
}
```

(Angka skor dan snippet hanya ilustrasi bentuk; runner hanya memakai `path` dan urutannya, skor dan snippet diabaikan.)

Tanpa hasil: `"structuredContent": {"items": [], "total": 0, "query": "batas permintaan"}` -> `results["batas permintaan"] = []`.

Error tool (tidak boleh terjadi untuk golden valid; membuat tes gagal):

```json
{ "jsonrpc": "2.0", "id": 7, "result": { "content": [{ "type": "text", "text": "query wajib diisi 1-200 karakter. Contoh: query=\"rate limit\"." }], "isError": true } }
```

Error JSON-RPC (mis. rate limit habis atau key salah; juga membuat tes gagal lewat `mcpCall` -> `t.Fatalf("JSON-RPC error <code>: <message>")`).

### 8.2 Seed knowledge (REST T05)

Request `POST /api/projects/{projID}/knowledge/files`, header `Content-Type: application/json`, `X-Requested-With: grimoire`:

```json
{ "path": "api/rate-limit.md", "content": "# Rate limit API\n\nSetiap API key dibatasi 120 request per menit ...\n" }
```

Respons: `201 Created`. Selain 201 -> `t.Fatalf("seed api/rate-limit.md: <status> <body>")`.

### 8.3 `golden.yaml` (potongan)

```yaml
pairs:
  - query: "rate-limit"
    expect: ["api/rate-limit.md"]
    kind: exact-name
  - query: "callback pelanggan"
    expect: ["webhook/event-catalog.md", "webhook/retry-policy.md"]
    kind: synonym
  - query: '"reset password"'
    expect: ["auth/password-reset.md"]
    kind: phrase
```

### 8.4 `baseline.json` (BENTUK saja; angka di bawah ilustrasi, BUKAN baseline nyata, baseline nyata dihasilkan langkah 7)

```json
{
  "pairs": 40,
  "recall_at_1": 0.0,
  "recall_at_5": 0.0,
  "mrr": 0.0,
  "per_kind": {
    "exact-name": { "pairs": 8,  "recall_at_1": 0.0, "recall_at_5": 0.0, "mrr": 0.0 },
    "keyword":    { "pairs": 10, "recall_at_1": 0.0, "recall_at_5": 0.0, "mrr": 0.0 },
    "synonym":    { "pairs": 8,  "recall_at_1": 0.0, "recall_at_5": 0.0, "mrr": 0.0 },
    "typo":       { "pairs": 7,  "recall_at_1": 0.0, "recall_at_5": 0.0, "mrr": 0.0 },
    "phrase":     { "pairs": 7,  "recall_at_1": 0.0, "recall_at_5": 0.0, "mrr": 0.0 }
  }
}
```

### 8.5 Keluaran tes (`go test -v`) dan format error

Lulus:

```
=== RUN   TestSearchEval
    searcheval_integration_test.go:..: pairs=40 recall@1=0.xxxx recall@5=0.xxxx MRR=0.xxxx
    searcheval_integration_test.go:..: exact-name pairs=8  r@1=... r@5=... mrr=...
    searcheval_integration_test.go:..: keyword    pairs=10 r@1=... r@5=... mrr=...
    searcheval_integration_test.go:..: synonym    pairs=8  r@1=... r@5=... mrr=...
    searcheval_integration_test.go:..: typo       pairs=7  r@1=... r@5=... mrr=...
    searcheval_integration_test.go:..: phrase     pairs=7  r@1=... r@5=... mrr=...
    searcheval_integration_test.go:..: GAGAL [synonym] "batas permintaan" expect=[api/rate-limit.md] rank=0 top5=[...]
    searcheval_integration_test.go:..: toleransi=0.0200 baseline: pairs=40 recall@1=... recall@5=... MRR=...
--- PASS: TestSearchEval
```

Gagal (regresi):

```
--- FAIL: TestSearchEval
    searcheval_integration_test.go:..: recall_at_5 turun 0.0500 (baseline 0.7000 -> sekarang 0.6500), toleransi 0.0200
    searcheval_integration_test.go:..: mrr turun 0.0300 (baseline 0.6000 -> sekarang 0.5700), toleransi 0.0200
```

Pesan error lain (semuanya `t.Fatal*`, bukan error API):

| Kondisi | Pesan |
|---|---|
| golden tidak valid | `golden.yaml tidak valid: pasangan #7 (query "x"): expect kosong; kind "typo" hanya 2 pasang (minimal 3)` |
| baseline hilang/rusak | `baseline.json tidak terbaca: open ...: no such file or directory (buat dengan: make search-eval-baseline)` |
| jumlah pasangan beda | `jumlah pasangan golden (41) != baseline (40); golden berubah, perbarui baseline: make search-eval-baseline` |
| toleransi invalid | `SEARCH_EVAL_TOLERANCE="abc" tidak valid (angka 0..1)` |
| seed gagal | `seed auth/login-flow.md: 400 {...}` |
| tool error | `search_knowledge error untuk "..." : <raw>` |
