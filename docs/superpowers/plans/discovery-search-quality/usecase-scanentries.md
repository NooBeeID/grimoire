# T11 usecase `scanentries`: provider bus `core.audit.scan-entries`

Mengikuti `overview.md` (FOUNDATION). Bila bertentangan, overview berlaku. Foundation (F1 konstanta, F2 `record/limits.go`, F3 kontrak `lib/contracts/core/audit/scanentries`) sudah selesai sebelum usecase ini; usecase ini TIDAK mengubahnya.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T11 |
| Scope | backend |
| Domain | core (provider tambahan; consumer: discovery/knowledgetools `searchquality`) |
| Module | core/auditlog |
| Feature | `scanentries` (folder `backend/internal/domain/core/internal/auditlog/scanentries/`) |
| Migrasi | Tidak ada |
| `endpointsChanged` | false (tidak ada REST/OpenAPI) |

| Endpoint | Method | Version | Keterangan |
|---|---|---|---|
| `core.audit.scan-entries` (`constants.BUS_CORE_AUDIT_SCAN_ENTRIES`) | bus Call, baca-saja | v1 (kontrak `scanentries.Request`/`Response`) | Tanpa route HTTP. Pagination keyset ASC |

## 2. Summary Plan

Menambah satu feature baca-saja di core/auditlog yang melayani consumer lintas module tanpa membuka tabel `audit_log`. Pola mengikuti `record` (handler bus) dan `listentries` (`buildQuery` parameterized), tetapi urutan ASC dan keyset `>`.

Pembagian tanggung jawab:

- `handler.go`: `bus.Expect[scanentries.Request]`, delegasi ke usecase. Tipe salah -> error membungkus `bus.ErrBadRequestType`.
- `params.go`: validasi + default (rule input; murni, tanpa I/O). Rule format aksi memakai `base.ValidAction` (entity).
- `usecase.go`: orkestrasi saja: validasi -> `repo.Scan(limit+1)` -> potong ke `limit`, bentuk `Next` bila ada baris lebih. Tidak ada SQL, tidak ada aturan bisnis baru.
- `repository.go`: satu-satunya yang menyentuh SQL; satu `SELECT` parameterized; filter `project_id` WAJIB; unmarshal jsonb -> `map[string]any`; NULL -> string kosong.
- `init.go`: rakit repo -> usecase -> handler, `d.Bus.Register(constants.BUS_CORE_AUDIT_SCAN_ENTRIES, ...)`.

Satu usecase, tidak dipecah. Tidak ada `UPDATE`/`DELETE`/`TRUNCATE` sehingga `auditlog/arch_test.go` (AC7: `TestAppendOnlySources`, `TestOnlyGetRoutes` tetap tepat 2 rute GET) tetap hijau.

Keputusan desain:

- Limit lebih dari `MaxLimit` ditolak (`ErrInvalid`), tidak di-clamp (kontrak F3).
- Pesan error tidak pernah memuat nilai input (hanya nama field), memakai pola `invalidField` dari `listentries/params.go` (`errs.WithDetails(errs.CodeInvalid, name+" tidak valid", {"field": name})` dengan `e.Err = errs.ErrInvalid`).
- `Actions` diduplikasi (urutan pertama dipertahankan) sebelum dihitung terhadap batas `MaxActions`; kosong setelah trim -> invalid.
- UUID dibandingkan kanonik: `uuid.Parse` lalu `.String() == lower(input)`; bentuk non-kanonik (kurung, urn:, tanpa strip) ditolak. Nilai yang diteruskan ke SQL adalah bentuk kanonik.
- Waktu dinormalisasi ke UTC. `To` nol -> `now()` dari `time.Now` yang di-inject ke usecase (agar bisa dites). `To` harus `After(From)`.
- Index `audit_log_project_idx (project_id, occurred_at DESC, id DESC)` dapat dibaca mundur, jadi ORDER ASC tetap memakai index; tidak perlu migrasi. Jangan tambah index.
- `Args`/`Result` tidak pernah `nil`: jsonb `null` atau kosong -> `map[string]any{}`. jsonb yang bukan object (mis. array) -> dibungkus error `scanentries: decode result` (tidak mungkin dari `record`, tetapi jangan panic).
- Kolom `actor_label`, `project_label`, `ip`, `user_agent`, `trace_id` TIDAK di-SELECT (minimisasi data).

## 3. Plan (langkah berurutan)

Prasyarat: pastikan `lib/constants/bus.go` punya `BUS_CORE_AUDIT_SCAN_ENTRIES` dan paket `lib/contracts/core/audit/scanentries` ada (foundation F1/F3). Bila belum, berhenti dan laporkan; jangan menambahnya di sini.

Urutan disarankan (TDD per file):

1. `params.go` + `params_test.go`.
2. `repository.go` (`buildQuery`, `sqlRepository.Scan`) + `repository_unit_test.go` (tanpa DB).
3. `usecase.go` + `usecase_test.go` (repo palsu).
4. `handler.go` + `handler_test.go`.
5. `init.go`.
6. `repository_integration_test.go` (build tag `integration`) dan `init_integration_test.go`.
7. Wiring: `auditlog/base.go` panggil `scanentries.Init(d)`.
8. Verifikasi: `cd backend && go build ./... && go vet ./internal/domain/core/... && go test ./internal/domain/core/internal/auditlog/... && go test -tags integration ./internal/domain/core/internal/auditlog/...`.

### 3.1 `params.go`

Package `scanentries` (doc: "feature bus core.audit.scan-entries").

```go
type params struct {
	ProjectID string
	Actions   []string
	From, To  time.Time
	After     *scanentries.Cursor // OccurredAt UTC, ID kanonik
	Limit     int
}

// parse memvalidasi Request dan menerapkan default. now dipakai bila To nol.
func parse(in scanentries.Request, now time.Time) (params, error)
```

Aturan (urutan pemeriksaan = urutan di bawah; error pertama dikembalikan):

| Field | Aturan | Pesan (`field`) |
|---|---|---|
| ProjectID | wajib; uuid kanonik huruf kecil | `project_id tidak valid` |
| Actions | `1 <= len <= scanentries.MaxActions` setelah dedup; tiap item `base.ValidAction` | `actions tidak valid` |
| From | wajib (`IsZero` -> invalid); simpan `.UTC()` | `from tidak valid` |
| To | nol -> `now.UTC()`; wajib `To.After(From)` | `to tidak valid` |
| Limit | `0` -> `scanentries.DefaultLimit`; `<0` atau `> MaxLimit` -> invalid | `limit tidak valid` |
| After | nil ok; bila ada: `OccurredAt` tidak nol, `ID` uuid kanonik | `after tidak valid` |

Helper `invalidField(name)` lokal (salin pola `listentries`, jangan import antar feature). Tanpa nilai input di pesan maupun details.

### 3.2 `repository.go`

```go
// Repository adalah satu-satunya yang menyentuh SQL di feature ini.
type Repository interface {
	// Scan mengembalikan paling banyak limit baris project p.ProjectID, urut occurred_at ASC, id ASC,
	// setelah p.After bila ada.
	Scan(ctx context.Context, p params, limit int) ([]scanentries.Entry, error)
}
type sqlRepository struct{ q db.Querier }
func NewRepository(q db.Querier) Repository
```

`buildQuery(p params, limit int) (string, []any)` merakit SQL dari fragmen literal, nilai lewat placeholder berurutan (pola `listentries.buildQuery`):

```sql
SELECT id::text, occurred_at, actor_type, actor_id::text, action, target, status, args, result
FROM audit_log
WHERE project_id = $1::uuid
  AND action = ANY($2::text[])
  AND occurred_at >= $3 AND occurred_at < $4
  [AND (occurred_at, id) > ($5::timestamptz, $6::uuid)]
ORDER BY occurred_at ASC, id ASC
LIMIT $n
```

- `project_id = $1` SELALU ada (rules 6.10); tidak ada jalur kode yang menghilangkannya.
- `actions` dikirim sebagai `[]string` lewat `pq.Array` bila driver di repo memakainya; ikuti persis cara `listentries` mengikat `f.Actions` (di sana `[]string` langsung ke `ANY($n::text[])`). Cek `db.Querier` dan salin.
- Limit yang diteruskan repository sudah `limit+1` dari usecase.

Row struct:

```go
type row struct {
	ID         string         `db:"id"`
	OccurredAt time.Time      `db:"occurred_at"`
	ActorType  string         `db:"actor_type"`
	ActorID    sql.NullString `db:"actor_id"`
	Action     string         `db:"action"`
	Target     sql.NullString `db:"target"`
	Status     string         `db:"status"`
	Args       []byte         `db:"args"`
	Result     []byte         `db:"result"`
}
```

Mapping: `OccurredAt.UTC()`, `ActorID.String`, `Target.String` (NULL -> ""), `decodeObject(raw []byte) (map[string]any, error)` (kosong/`null` -> `map[string]any{}`; `json.Unmarshal` ke `map[string]any`). Error: `fmt.Errorf("scanentries: query: %w", err)` dan `fmt.Errorf("scanentries: decode args: %w", err)`. Hasil slice tidak pernah nil (`make([]..., 0, len(rows))`).

### 3.3 `usecase.go`

```go
type Usecase struct {
	repo Repository
	now  func() time.Time
}
func NewUsecase(repo Repository, now func() time.Time) *Usecase

func (u *Usecase) Execute(ctx context.Context, in scanentries.Request) (scanentries.Response, error) {
	p, err := parse(in, u.now())
	if err != nil { return scanentries.Response{}, err }
	rows, err := u.repo.Scan(ctx, p, p.Limit+1)
	if err != nil { return scanentries.Response{}, err }
	resp := scanentries.Response{Entries: rows}
	if len(rows) > p.Limit {
		resp.Entries = rows[:p.Limit]
		last := resp.Entries[p.Limit-1]
		resp.Next = &scanentries.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
	}
	if resp.Entries == nil { resp.Entries = []scanentries.Entry{} }
	return resp, nil
}
```

Cursor `Next` = baris TERAKHIR yang dikembalikan (bukan baris ekstra), karena filter berikutnya memakai `>`. `OccurredAt` dari DB sudah presisi mikrodetik Postgres sehingga round-trip ke `After` tidak kehilangan presisi.

### 3.4 `handler.go`

```go
type scanner interface {
	Execute(ctx context.Context, in scanentries.Request) (scanentries.Response, error)
}
type handler struct{ uc scanner }

func (h handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[scanentries.Request](req)
	if err != nil { return nil, err }
	resp, err := h.uc.Execute(ctx, in)
	if err != nil { return scanentries.Response{}, err }
	return resp, nil
}
```

Respons dikembalikan sebagai NILAI `scanentries.Response` (consumer memakai `bus.CallTyped[scanentries.Response]`).

### 3.5 `init.go`

```go
// Init merakit repo -> usecase -> handler dan mendaftarkan endpoint bus baca-saja.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepository(d.DB), time.Now)
	d.Bus.Register(constants.BUS_CORE_AUDIT_SCAN_ENTRIES, handler{uc: uc}.Handle)
}
```

### 3.6 Wiring `auditlog/base.go`

Tambah import `grimoire/internal/domain/core/internal/auditlog/scanentries` dan panggil `scanentries.Init(d)` tepat setelah `record.Init(d)`, DI LUAR blok `d.API.Route("/audit-log", ...)` (bus saja, tanpa route, tanpa `RequireRole`). Perbarui komentar `Register`: "...dan mendaftarkan endpoint bus baca-saja core.audit.scan-entries untuk consumer lintas module (T11)." Tidak ada perubahan lain di file itu.

## 4. Migration

Tidak ada. Tidak ada perubahan skema/index/retensi; `.claude/rules/migrations.md` tidak berlaku. Query memakai `audit_log_project_idx` yang ada.

## 5. Changes

| File | Perubahan |
|---|---|
| `backend/internal/domain/core/internal/auditlog/scanentries/init.go` | baru: `Init(d app.Deps)` |
| `.../scanentries/handler.go` | baru: adapter bus |
| `.../scanentries/params.go` | baru: `parse`, `invalidField` |
| `.../scanentries/usecase.go` | baru: `Usecase`, `Execute` |
| `.../scanentries/repository.go` | baru: `Repository`, `buildQuery`, `sqlRepository` |
| `.../scanentries/params_test.go`, `usecase_test.go`, `handler_test.go`, `repository_unit_test.go` | baru: unit |
| `.../scanentries/repository_integration_test.go`, `init_integration_test.go` | baru: integrasi (`//go:build integration`) |
| `backend/internal/domain/core/internal/auditlog/base.go` | + import dan `scanentries.Init(d)`, komentar |

allowedPaths: `backend/internal/domain/core/internal/auditlog/scanentries/` dan `backend/internal/domain/core/internal/auditlog/base.go`. Dilarang menyentuh `auditlog/base/` (folder entity), `arch_test.go`, atau kontrak.

Aturan arsitektur: feature hanya meng-import `auditlog/base` (ValidAction), `infra/{app,bus,db}`, `lib/constants`, `lib/common/errs`, `lib/contracts/core/audit/scanentries`. Tidak meng-import feature lain di auditlog. SQL hanya di `repository.go`.

## 6. Test Scenario

Semua tes tabel; tidak ada `time.Sleep`.

### 6.1 Unit `params_test.go`

Baseline valid: ProjectID uuid kanonik, Actions `["mcp.search_knowledge","mcp.read_file"]`, From `2026-10-14T00:00:00Z`, To nol.

- Valid minimal: Limit 0 -> 500; To nol -> `now` UTC; Waktu non-UTC -> UTC.
- Setiap field invalid -> `errors.Is(err, errs.ErrInvalid)` dan `errors.As(*errs.Error).Code == errs.CodeInvalid`, dan `err.Error()` TIDAK memuat nilai input (assert `!strings.Contains(msg, sentinelValue)` dengan sentinel unik seperti `"SECRET-XYZ"` pada ProjectID/aksi/ID cursor):
  - ProjectID: kosong, bukan uuid, uuid huruf besar, bentuk non-kanonik (`{...}`, `urn:uuid:...`, tanpa strip).
  - Actions: nil, kosong, 11 item (batas 10 valid), item `""`, `"Bad.Action"`, `"noDot"`, `"a..b"`, item > 128 byte, duplikat yang setelah dedup tinggal 1 (valid).
  - From nol.
  - To <= From (sama dan lebih awal); To = From + 1ns valid.
  - Limit: -1, 1001; 1 dan 1000 valid.
  - After: OccurredAt nol; ID kosong/bukan uuid.
- Details berisi `field` yang tepat (`project_id`, `actions`, `from`, `to`, `limit`, `after`).

### 6.2 Unit `repository_unit_test.go`

- `buildQuery` tanpa After: berisi `project_id = $1::uuid`, `ORDER BY occurred_at ASC, id ASC LIMIT $5`, tanpa `(occurred_at, id) >`, `len(args)==5`.
- Dengan After: berisi `(occurred_at, id) > ($5::timestamptz, $6::uuid)`, LIMIT `$7`, jumlah `$` = jumlah args.
- Injeksi: Actions berisi `x'; DROP TABLE audit_log; --` (lolos ke buildQuery langsung) -> nilai tidak muncul di SQL string.
- SQL tidak memuat `UPDATE`/`DELETE`/`TRUNCATE`; hanya satu `SELECT`; tidak memuat `actor_label`, `ip`, `user_agent`, `trace_id`.
- `decodeObject`: `nil`, `""`, `null` -> `map{}` non-nil; `{"a":1}` -> `float64(1)`; `[1]` -> error.

### 6.3 Unit `usecase_test.go` (repo palsu dengan data terurut di memori, mencatat `limit` yang diterima)

- Input invalid -> error, repo TIDAK dipanggil.
- Repo dipanggil dengan `limit+1` (default 501; Limit 3 -> 4).
- 7 baris, Limit 3: halaman 1 = 3 entri, `Next` = (baris ke-3); halaman 2 (After = Next) 3 entri; halaman 3 = 1 entri, `Next == nil`. Gabungan 7 entri tanpa duplikat/terlewat.
- Tepat `limit` baris tersedia -> `Next == nil` (tidak ada halaman kosong palsu).
- Repo mengembalikan 0 baris -> `Entries` non-nil (`[]`), `Next == nil`.
- Error repo diteruskan apa adanya (`errors.Is`), `Response` nol.
- `now` palsu dipakai saat To nol (assert `To` yang diterima repo).

### 6.4 Unit `handler_test.go`

- Request bertipe salah (`*scanentries.Request`, `string`, `nil`) -> `errors.Is(err, bus.ErrBadRequestType)`, usecase tidak dipanggil.
- Sukses -> respons bertipe nilai `scanentries.Response`.
- Error usecase -> `(scanentries.Response{}, err)` dan `err` sama.

### 6.5 Integrasi `repository_integration_test.go` (`//go:build integration`, `db.TestDB`, `db.Up`)

Helper `setupDB` dan `seed` lokal (salin pola `listentries/repository_test.go`; nama tidak bentrok karena beda package). Seed lewat `INSERT INTO audit_log` langsung.

1. **Hanya project diminta:** seed project A dan B dengan action sama; scan A -> tidak ada baris B; baris dengan `project_id` NULL tidak ikut.
2. **Hanya actions diminta:** seed `mcp.search_knowledge`, `mcp.read_file`, `mcp.list_files`; scan dua pertama -> `mcp.list_files` tidak muncul.
3. **Batas waktu:** baris pada `From` (inklusif, ikut), `To` (eksklusif, tidak ikut), `From-1µs` (tidak), `To-1µs` (ikut).
4. **Urutan dan paging 3 halaman pada `occurred_at` identik:** seed 7 baris dengan `occurred_at` sama + 2 baris waktu lain, id uuid terurut acak saat insert; `Limit=3` via usecase penuh (`NewUsecase(NewRepository(d), time.Now)`) -> loop `After = Next` sampai nil; hasil = 3 halaman (3,3,3), gabungan 9 id unik sama dengan himpunan seed, urut `(occurred_at, id)` ASC (assert dengan sort pembanding).
5. **Konten:** `args`/`result` kembali `map[string]any` (angka `float64`, array `[]any`, nested object); jsonb default `{}` -> map kosong non-nil; `target` NULL -> `""`; `actor_id` NULL -> `""`; `OccurredAt` UTC.
6. **Project tanpa data / project tak dikenal:** `Entries == []`, `Next == nil`, tanpa error.
7. **Keamanan nilai:** Actions berisi string injeksi (valid format? tidak, ditolak params), jadi uji lewat `repo.Scan` langsung dengan `params` berisi aksi aneh -> tidak error SQL, tidak ada baris, tabel `audit_log` tetap utuh.

### 6.6 Integrasi `init_integration_test.go`

Bangun `bus.NewRegistry`, `app.Deps{Bus, DB, ...}`, panggil `Init(d)`, lalu:

- `bus.CallTyped[scanentries.Response](ctx, reg, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, scanentries.Request{...})` -> sukses terhadap data seed.
- Body bertipe salah (pointer `&scanentries.Request{}`) -> `errors.Is(err, bus.ErrBadRequestType)`.
- Request invalid -> `errors.Is(err, errs.ErrInvalid)`.
- `Init` dua kali pada registry yang sama -> panic/error registrasi ganda sesuai perilaku `Registry.Register` (cek tes `record` untuk pola; hanya assert bila registry memang menolak).

### 6.7 Tes arsitektur existing (tetap hijau, tidak diubah)

- `auditlog/arch_test.go` `TestAppendOnlySources`: tak ada `UPDATE/DELETE/TRUNCATE audit_log` baru.
- `TestOnlyGetRoutes`: tepat 2 rute GET (scanentries tidak menambah route). Tes ini membuat `app.Deps` dengan `Bus: bus.NewRegistry(log)` dan `DB` nil; `scanentries.Init` hanya membuat `NewRepository(d.DB)` tanpa menyentuh DB sehingga aman dengan DB nil. JANGAN melakukan akses DB di `Init`.
- `internal/archtest`: feature tidak meng-import domain lain selain kontrak/infra.

## 7. E2E Scenarios

Tidak ada UI. Lintas lapis divalidasi di tes integrasi bus (6.6) dan, setelah usecase `searchquality` selesai, di `cmd/internal/server/searchquality_report_integration_test.go` (milik usecase lain).

Positif:

1. Consumer memanggil dengan `Actions=[mcp.search_knowledge, mcp.read_file]`, rentang 7 hari, `Limit=1000`: seluruh entri project terbaca berurutan lewat beberapa halaman, `Next` nil di akhir.
2. `To` tidak diisi: batas atas = sekarang; entri masa depan tidak ikut.
3. Paging dengan 2.500 entri bertimestamp identik: tiga halaman (1000, 1000, 500), tanpa duplikat/terlewat.
4. Entri tanpa `target` dan tanpa `actor_id` (aktor system/anonymous) kembali dengan string kosong.

Negatif:

1. `ProjectID` kosong atau bukan uuid -> `INVALID_ARGUMENT`; tidak ada query.
2. `Actions` kosong, > 10, atau berformat salah -> `INVALID_ARGUMENT`.
3. `From` nol, atau `To <= From` -> `INVALID_ARGUMENT`.
4. `Limit` 1001 -> `INVALID_ARGUMENT` (tidak di-clamp).
5. `After` dengan ID bukan uuid -> `INVALID_ARGUMENT`.
6. Body bukan `scanentries.Request` (pointer/tipe lain) -> `bus.ErrBadRequestType`.
7. Project lain dengan data berlimpah -> tidak pernah bocor ke hasil.
8. DB tidak terjangkau -> `scanentries: query: ...` (bukan `errs.ErrInvalid`, sehingga consumer memetakannya ke exit 1 / 500).
9. Request kuota: tidak ada endpoint HTTP, sehingga `GET /api/audit-log` tidak berubah dan tetap tepat 2 rute.

## 8. Request/Response body

Bus dipanggil dengan nilai struct Go; JSON di bawah adalah representasi field (tanpa tag json pada kontrak).

Request (halaman lanjutan):

```json
{
  "ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
  "Actions": ["mcp.search_knowledge", "mcp.read_file"],
  "From": "2026-10-14T00:00:00Z",
  "To": "2026-10-21T00:02:00Z",
  "After": {"OccurredAt": "2026-10-15T08:30:12.123456Z", "ID": "0192f0b1-0000-7000-8000-00000000a001"},
  "Limit": 1000
}
```

Request minimal (default berlaku: `To` = now, `Limit` = 500, `After` nil):

```json
{
  "ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
  "Actions": ["mcp.read_file"],
  "From": "2026-10-14T00:00:00Z"
}
```

Response dengan halaman berikutnya:

```json
{
  "Entries": [
    {
      "ID": "0192f0b1-0000-7000-8000-00000000a002",
      "OccurredAt": "2026-10-15T08:31:00.000001Z",
      "ActorType": "api_key",
      "ActorID": "0192f0a9-1111-7aaa-8bbb-0000000000c1",
      "Action": "mcp.search_knowledge",
      "Target": "docs",
      "Status": "ok",
      "Args": {"query": "rate limit", "path": "docs", "limit": 10},
      "Result": {
        "count": 3, "total": 3,
        "result_paths": ["docs/api/rate-limit.md", "docs/ops/nginx.md", "docs/faq.md"],
        "bytes": 812, "original_bytes": 812, "truncated": false, "duration_ms": 14
      }
    },
    {
      "ID": "0192f0b1-0000-7000-8000-00000000a003",
      "OccurredAt": "2026-10-15T08:31:30Z",
      "ActorType": "system",
      "ActorID": "",
      "Action": "mcp.read_file",
      "Target": "",
      "Status": "ok",
      "Args": {},
      "Result": {}
    }
  ],
  "Next": {"OccurredAt": "2026-10-15T08:31:30Z", "ID": "0192f0b1-0000-7000-8000-00000000a003"}
}
```

Halaman terakhir: `"Next": null`. Tanpa data: `{"Entries": [], "Next": null}`.

Format error (`*errs.Error`, membungkus `errs.ErrInvalid`; pesan tanpa nilai input):

```json
{"code": "INVALID_ARGUMENT", "message": "project_id tidak valid", "details": {"field": "project_id"}}
```

Pesan lain: `"actions tidak valid"`, `"from tidak valid"`, `"to tidak valid"`, `"limit tidak valid"`, `"after tidak valid"`.

Tipe body salah (error Go, bukan `errs.Error`):

```
bus: unexpected request type: want=scanentries.Request got=*scanentries.Request
```

(`errors.Is(err, bus.ErrBadRequestType)`.)

Kegagalan infra: `scanentries: query: <err>` (dibungkus `%w`, tanpa nilai parameter).
