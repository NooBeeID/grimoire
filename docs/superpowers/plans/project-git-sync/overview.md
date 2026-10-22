# T14 core/git-credential + project/git-sync — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T14-project-git-sync.md`.
Sumber aturan: `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog audit), `rules.txt` (bagian 4-8), `.claude/rules/migrations.md`, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (desain produk; file `*knowledge-hub-design.md` tidak ada, ini padanannya), `DESIGN.md` (frontend).
Konteks yang dicek: `CLAUDE.md` tidak ada. `.claude/rules/` hanya berisi `migrations.md`. Kode T05/T06/T10/T12/T13 sudah ada di `main`.

Dokumen ini adalah plan FOUNDATION: keputusan bersama, infra baru, entity `base/`, kontrak bus, migrasi, OpenAPI, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-git-sync/usecase-<slug>.md`. Frontend rinci ada di `docs/superpowers/plans/project-git-sync/frontend-plan.md`. Keduanya WAJIB mengikuti dokumen ini; bila bertentangan, dokumen ini yang menang.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | fullstack (backend: plan ini + plan usecase; frontend: `frontend-plan.md`) |
| Domain utama / Module utama | project / git-sync (`gitsync`) |
| Module kedua (baru) | core / git-credential (`gitcredential`) |
| Module yang ditambah pintu masuk bus | project / knowledge (`busupsertfiles`), project / api-spec (`busupsertspecs`) |
| Resource root | `backend/internal/domain/project/internal/gitsync` |
| Resource root kedua | `backend/internal/domain/core/internal/gitcredential` |
| Bergantung pada | T01, T02, T03, T05, T06 (+ T10 regresi Swagger 2.0), semua sudah merged |

| Endpoint | Method | Version | Module / feature |
|---|---|---|---|
| `/api/me/git-credentials` | GET | `/api` tanpa prefix versi; `api/modules/core/git-credential.yml` `info.version` 0.1.0 | gitcredential/listcredentials |
| `/api/me/git-credentials/{provider}` | PUT | sama | gitcredential/savecredential |
| `/api/me/git-credentials/{provider}?host=` | DELETE | sama | gitcredential/deletecredential |
| `/api/projects/{projectID}/git-sync?kind=` | GET | `/api`; `api/modules/project/git-sync.yml` `info.version` 0.1.0 | gitsync/getconfig |
| `/api/projects/{projectID}/git-sync/run` | POST | sama | gitsync/runsync |
| bus `core.gitcredential.get-token` | Call | v1 (kontrak baru) | gitcredential/bustoken |
| bus `project.knowledge.upsert-files` | Call | v1 (kontrak baru) | knowledge/busupsertfiles |
| bus `project.apispec.upsert-specs` | Call | v1 (kontrak baru) | apispec/busupsertspecs |

`previewsync` TIDAK dibuat (Pertanyaan terbuka 1, default tiket: peringatan di dialog + laporan sesudahnya).

---

## 2. Summary Plan

User menyimpan Personal Access Token (PAT) per `(provider, host)` di halaman Profil. Token dienkripsi AES-256-GCM dengan kunci `GIT_CREDENTIAL_KEY` dan tidak pernah dikembalikan ke klien. Di halaman Knowledge dan API Spec, tombol **Sync dari Git** menerima URL repo/folder dan branch. Grimoire lalu menarik file lewat REST API GitHub/GitLab (tree + blob, tanpa binary `git`) dan menulisnya lewat jalur upload yang sudah ada:

- `core/gitcredential` menyimpan PAT dan hanya membukanya untuk module lain lewat bus `core.gitcredential.get-token`.
- `project/gitsync` menyimpan config per `(project, kind)`, memegang advisory lock, mengambil file lewat `infra/gitprovider` (HTTP aman `infra/safehttp`), lalu memanggil `project.knowledge.upsert-files` atau `project.apispec.upsert-specs`.
- Module knowledge dan apispec mendapat pintu masuk bus batch. Orkestrasi tulis upload dipindah ke paket bersama `base/ingest` agar upload multipart dan sync memakai satu jalur (K7). Perilaku upload tidak berubah.
- Infra generik baru: `lib/common/secret` (tipe token tersamar), `infra/crypto/secretbox`, `infra/safehttp` (anti-SSRF di dial), `infra/gitprovider` (klien REST GitHub/GitLab), `middleware.ExtendTimeout`.
- Keamanan PAT dijaga tipe `secret.String` dan tes "PAT canary" (token unik tidak boleh muncul di log, respons, audit, maupun tabel config).

---

## 3. Konteks terverifikasi di kode (bukan asumsi)

| Fakta | Lokasi | Dampak ke plan |
|---|---|---|
| Upload knowledge: blok tx `LockProject -> CountFiles -> ExistingHashes -> base.PlanUpload -> InsertFiles/UpdateFiles` ada di usecase | `project/internal/knowledge/uploadfiles/usecase.go` | dipindah ke `knowledge/base/ingest` (D1) |
| Rule knowledge: `ClassifyPart`, `PlanUpload`, `Summarize`, `ChangedPaths`, `SortByIndex`, `NormalizePath`, `NormalizePrefix` | `knowledge/base/upload.go`, `base/path.go` | dipakai ulang, tidak ditulis ulang |
| Reason knowledge berupa kode huruf kecil (`unsupported_type`, `too_large`, `project_file_limit`, `hidden`, ...) | `knowledge/base/reasons.go` | dipakai apa adanya di laporan sync |
| Upload apispec: `processFile` + `persist` (retry `ErrNameTaken`) di usecase | `project/internal/apispec/uploadspecs/usecase.go` | dipindah ke `apispec/base/ingest` (D1) |
| Reason apispec huruf besar (`NOT_SPEC_FILE`, `DUPLICATE_NAME_IN_UPLOAD`, `SPEC_LIMIT_REACHED`, `INVALID_SPEC`, ...) | `apispec/base/upload.go` | dipakai apa adanya |
| `apispec/base/audit.BusRecorder.Record` mengambil aktor dari `reqctx` (ctx), `Event` tidak punya field `Actor` | `apispec/base/audit/audit.go` | ditambah `Event.Actor *base.Actor` (override opsional, aditif) |
| `knowledge/base/audit.Event` sudah punya `Actor` | `knowledge/uploadfiles/usecase.go` | tidak perlu diubah |
| `RegisterCodeStatus` panic bila kode sudah terdaftar (termasuk bawaan); module memakai `sync.Once` | `infra/http/respond/status.go`, `core/internal/auth/base.go` | kode yang dipakai dua module tidak boleh didaftarkan dua kali (bagian 6.6) |
| `api.Use(middleware.Timeout(cfg.HTTPDefaultTimeout), CSRF, Session, PasswordChangeGate)` | `infra/http/router.go:55` | `ExtendTimeout` perlu ctx dasar sebelum deadline, tapi tetap membawa nilai Session (bagian 5.5) |
| `http.Server` tanpa `WriteTimeout` | `infra/http/graceful.go` | request sync 120 s aman di sisi server |
| Advisory lock sesi pada koneksi dipin: `pg_try_advisory_lock(hashtext($1))` | `infra/jobs/lock.go` | pola lock D4 |
| `checkaccess.Level{None,Member,Owner,Admin}`, Admin = super_admin | `lib/contracts/project/management/checkaccess` | otorisasi gitsync (D7) |
| Halaman `/profile` = `app/views/ProfileView.svelte` (`max-w-sm`) | frontend | seksi Token Git di sini (Pertanyaan terbuka 2) |
| Header Knowledge di `app/views/workspace/knowledge/{KnowledgeTab,KnowledgeDialogs}.svelte`; API spec di `app/views/workspace/ApiSpecsTab.svelte` | frontend | lokasi tombol Sync |
| JSON REST repo memakai snake_case (`created_by_label`, `source_path`) | DTO api-key/apispec | semua JSON T14 snake_case (bagian 6.8) |
| Migrasi terakhir: core `20261021233706157`, project `20261022160000000` | `backend/migrations/*` | versi baru di-generate saat implement |

---

## 4. Keputusan lintas usecase

- **D1 (K7, satu jalur tulis).** Rule upload tetap di entity (`knowledge/base`, `apispec/base`). Orkestrasi tulis per batch dipindah ke paket bersama `knowledge/base/ingest` dan `apispec/base/ingest`. Precedent: `base/store`, `base/access`, `base/audit` sudah berisi I/O. `uploadfiles`/`uploadspecs` direfaktor memakai paket ini tanpa perubahan perilaku (dijaga tes yang ada). Feature bus baru juga memakainya (rule 4.3 melarang feature meng-import feature).
- **D2 (transaksi).** Mengikuti upload: knowledge memakai satu transaksi untuk semua file (`LockProject`); API spec memakai satu transaksi per file. Laporan per file selalu lengkap.
- **D3 (batas, K6).** Sync ditolak utuh (`SYNC_LIMIT_EXCEEDED`, `last_status=failed`, nol tulis) bila jumlah kandidat > `MaxFiles` atau total byte > `MaxTotalBytes`. Pengecekan pertama dari tree sebelum fetch, lalu diulang kumulatif saat fetch. Semua konten diambil ke memori dulu, baru ditulis. File tunggal > batas per file -> `TooLarge=true` -> `rejected` per file oleh provider. Batas isi project (`MAX_PROJECT_FILES`, `APISPEC_MAX_SPECS`) diterapkan per file oleh provider (`project_file_limit` / `SPEC_LIMIT_REACHED`), sama dengan upload.
- **D4 (lock, K10).** `pg_try_advisory_lock(hashtext('grimoire.gitsync.' || $1 || '.' || $2))` pada koneksi `*sqlx.Conn` yang dipin selama sync, `pg_advisory_unlock` di `defer`. Bila unlock gagal, koneksi dibuang (`conn.Raw` + `driver.ErrBadConn`, pola `infra/jobs/lock.go`). Lock gagal -> 409 `SYNC_IN_PROGRESS` tanpa tulis config. Implementasi di `gitsync/runsync/repository.go`.
- **D5 (urutan runsync).** (1) access check -> 404 non-anggota; (2) parse kind, URL, host policy, prefix -> 400/422 tanpa tulis; (3) get-token -> `GIT_FEATURE_DISABLED`/`CREDENTIAL_MISSING`/`CREDENTIAL_UNREADABLE` tanpa tulis config (config milik A tidak tertimpa oleh B yang gagal); (4) try lock; (5) upsert config; (6) list tree, select, cek batas, fetch; gagal -> `SaveResult(failed, ErrorCode)` + audit lalu error HTTP; (7) bus upsert; (8) `SaveResult(success|partial)` + audit `git_sync.run` + 200. Audit `git_sync.run` juga ditulis untuk kegagalan setelah langkah 5.
- **D6 (keamanan PAT).** Token hanya hidup sebagai `secret.String`. Token tidak pernah ada di respons, log, audit, pesan error, URL, `last_result`, atau Publish. Body error provider tidak pernah diteruskan atau di-log. Tes PAT canary wajib di savecredential, bustoken, dan runsync.
- **D7 (otorisasi).** `/api/me/git-credentials*` untuk user login. Identity diambil dari `reqctx` dan query selalu dibatasi `user_id` sendiri. `/api/projects/{projectID}/git-sync*` memakai `base/access.Require` (check-access): `member`, `owner`, `admin` (termasuk super_admin non-anggota) boleh; `none` -> 404; tanpa login -> 401. Token yang dipakai sync selalu milik identity request (`UserID = actor.ID`).
- **D8 (pemetaan nama, K8).** Knowledge: path tujuan = `TargetPrefix + "/" + RelPath`; join dan normalisasi dilakukan provider knowledge. API spec: nama = `DeriveName(basename)`, `source_path` = path repo. Nama ganda dalam satu sync -> `rejected DUPLICATE_NAME_IN_UPLOAD` (kode yang sudah ada).
- **D9 (transport).** Route run memakai `middleware.ExtendTimeout(cfg.GitSyncTimeout)` dan `BodyLimit(16<<10)`. Route PUT credential memakai `BodyLimit(8<<10)`.
- **D10 (kode error lintas module, menyesuaikan panic `RegisterCodeStatus`).** Kode yang dipakai KEDUA module (`HOST_NOT_ALLOWED`, `GIT_UPSTREAM_ERROR`, `GIT_FEATURE_DISABLED`) menjadi kode bersama di `lib/common/errs` dan didaftarkan di peta bawaan `respond/status.go` (precedent T12 `ALREADY_REGISTERED`). Kode lain didaftarkan module masing-masing dalam `sync.Once`. Lihat tabel 6.6.
- **D11 (FK `user_id`).** `git_credentials.user_id REFERENCES users(id) ON DELETE CASCADE`. README "Konvensi arsitektur" mengizinkan FK `user_id` lintas module dengan CASCADE. Alasannya keamanan: user yang di-hard-delete (mis. oleh `purgeinvitations`) tidak boleh meninggalkan PAT terenkripsi yatim. `git_sync_configs.last_synced_by` tanpa FK (rule 8.5, label snapshot rule 8.7).
- **D12 (PK).** `git_sync_configs` memakai `id uuid PRIMARY KEY` + `UNIQUE (project_id, kind)` (rule 8.4 PK uuid; tiket meminta unique `(project_id, kind)`). Upsert memakai `ON CONFLICT (project_id, kind)`.
- **D13 (konstruksi infra).** `app.Deps` TIDAK diubah. Tiap module membangun dependensinya di `base.go` dari `d.Config`: `secretbox.New(key)` (gitcredential), `gitprovider.NewFromConfig(cfg)` (gitcredential untuk verify, gitsync untuk tree/blob). Konstruksi gagal = panic saat wiring (rule 5.9), tetapi config sudah memvalidasi lebih dulu sehingga tidak terjadi di runtime.
- **D14 (reason skipped dari seleksi).** Entri yang ekstensinya tidak didukung dilewati di gitsync (tanpa fetch) dengan kode dari kosakata module tujuan agar FE memakai i18n yang sama dengan upload: knowledge `unsupported_type`, api_spec `NOT_SPEC_FILE`. Kode ini menggantikan `NOT_SUPPORTED_TYPE` di catatan dekomposisi.

---

## 5. Infra dan lib baru (FOUNDATION, langkah 0)

Semua tanpa import `domain/**` (rule 4.6). Semua tes unit pakai `httptest`/resolver palsu, tanpa jaringan nyata.

### 5.1 `backend/lib/common/secret/secret.go`

```go
package secret

type String struct{ v string } // field tidak diekspor

func New(s string) String
func (s String) Reveal() string            // satu-satunya jalan ke plaintext
func (s String) IsZero() bool
func (s String) Last4() string             // 4 karakter terakhir (rune), "" bila kosong
func (s String) String() string            // "[REDACTED]"
func (s String) GoString() string          // "[REDACTED]"
func (s String) LogValue() slog.Value      // slog.StringValue("[REDACTED]")
func (s String) MarshalJSON() ([]byte, error) // "\"[REDACTED]\""
func (s String) Format(f fmt.State, verb rune) // selalu "[REDACTED]" untuk semua verb termasuk %q %x
```

- `Format` ditambahkan agar `%x`/`%q` tidak membocorkan field lewat refleksi fmt.
- `UnmarshalJSON` TIDAK disediakan di lib. Decode body dilakukan di `params.go` feature, yaitu decode ke `string` lalu langsung `secret.New`.
- Tes `secret_test.go`: `%v %+v %#v %s %q %x` pada nilai dan pada struct berisi field `secret.String`; `slog` JSON handler; `json.Marshal` struct; `errors.New(fmt.Sprint(s))`. Semua tidak memuat plaintext canary. `Reveal`/`Last4` benar.

### 5.2 `backend/internal/infra/crypto/secretbox/secretbox.go`

```go
var ErrDecrypt = errors.New("secretbox: decrypt failed")
var ErrKeySize = errors.New("secretbox: key must be 32 bytes")

type Box struct{ aead cipher.AEAD }
func New(key []byte) (*Box, error)                      // AES-256-GCM, wajib 32 byte
func (b *Box) Seal(plain, aad []byte) ([]byte, error)  // nonce(12) || ciphertext+tag; nonce crypto/rand
func (b *Box) Open(blob, aad []byte) ([]byte, error)   // len < 12+16 atau gagal auth -> ErrDecrypt
```

- AAD dibangun di entity gitcredential (`base.AAD`). Secretbox generik dan tidak tahu formatnya.
- Tes: round-trip; kunci salah -> `ErrDecrypt`; AAD beda -> `ErrDecrypt`; blob terpotong/kosong -> `ErrDecrypt`; dua Seal teks sama -> blob berbeda (nonce unik); kunci 31/33 byte -> `ErrKeySize`.

### 5.3 `backend/internal/infra/safehttp/`

```go
type Options struct {
	Timeout            time.Duration // total per request (http.Client.Timeout)
	MaxRedirects       int           // default 3, hanya host sama
	InsecureAllowHosts []string      // "host:port"; HANYA test/e2e: izinkan http + loopback untuk host:port ini
	Resolver           Resolver      // nil -> net.DefaultResolver; seam tes DNS rebinding
}
func New(o Options) *http.Client
func ReadLimited(resp *http.Response, max int64) (body []byte, tooLarge bool, err error)
var ErrBlockedAddress = errors.New("safehttp: destination address not allowed")
var ErrSchemeNotAllowed = errors.New("safehttp: only https allowed")
var ErrCrossHostRedirect = errors.New("safehttp: redirect to another host not allowed")
```

- Transport: `Proxy: nil` (proxy env dimatikan), `DialContext` memakai `net.Dialer{Control: checkIP}`. `checkIP` menolak IP hasil resolve SAAT dial: loopback, private (10/8, 172.16/12, 192.168/16, fc00::/7), link-local (169.254/16 termasuk 169.254.169.254, fe80::/10), unspecified, multicast, 100.64/10 (CGNAT), 0.0.0.0/8, IPv4-mapped IPv6 dari rentang tersebut. Pemeriksaan di dial menutup DNS rebinding.
- `RoundTripper` pembungkus menolak skema selain `https` (kecuali host:port di `InsecureAllowHosts`).
- `CheckRedirect`: host berbeda -> `ErrCrossHostRedirect`; lebih dari `MaxRedirects` -> error. Header `Authorization`/`PRIVATE-TOKEN` hanya dipertahankan pada host yang sama (perilaku default Go, ditegaskan dengan tes).
- `InsecureAllowHosts` hanya membolehkan loopback dan http untuk host:port yang persis terdaftar.
- Tes: 127.0.0.1, 10.0.0.5, 169.254.169.254, `[fc00::1]`, `[::ffff:127.0.0.1]` ditolak; resolver palsu yang menjawab IP publik lalu privat (rebinding) tetap ditolak di dial; redirect `httptest` ke host lain tidak diikuti; `http://` ditolak; `ReadLimited` tepat di batas dan batas+1; `InsecureAllowHosts` membuka `httptest` hanya untuk port tersebut.

### 5.4 `backend/internal/infra/gitprovider/`

Adapter integrasi seperti `infra/mailer`, tanpa logika domain.

```go
type Provider string
const (GitHub Provider = "github"; GitLab Provider = "gitlab")

type HostPolicy struct{ AllowedGitLabHosts []string } // lowercase, termasuk port bila ada
func (p HostPolicy) Allowed(provider Provider, host string) bool
// github: hanya "github.com". gitlab: "gitlab.com" atau anggota AllowedGitLabHosts.
func (p HostPolicy) GitLabHosts() []string // ["gitlab.com", ...allowed] untuk listcredentials

type Repo struct {
	Provider Provider
	Host     string // github.com | gitlab.com | host self-hosted
	Path     string // github "owner/repo"; gitlab "group/sub/repo"
	Branch   string
}
type TreeEntry struct{ Path string; Size int64 /* -1 bila tidak diketahui */; BlobSHA string }

type Client struct{ /* http *http.Client (safehttp), policy HostPolicy, apiBase override untuk tes */ }
func New(httpClient *http.Client, policy HostPolicy, o Options) *Client
func NewFromConfig(cfg config.Config) *Client // safehttp.New dari GitSyncTimeout, InsecureTestHosts
func (c *Client) Policy() HostPolicy
func (c *Client) VerifyToken(ctx context.Context, provider Provider, host string, token secret.String) error
func (c *Client) ListTree(ctx context.Context, repo Repo, folder string, token secret.String) ([]TreeEntry, error)
func (c *Client) FetchBlob(ctx context.Context, repo Repo, sha string, maxBytes int64, token secret.String) (data []byte, tooLarge bool, err error)

var (
	ErrUnauthorized  = errors.New("gitprovider: unauthorized")
	ErrForbidden     = errors.New("gitprovider: forbidden")
	ErrNotFound      = errors.New("gitprovider: not found")
	ErrTreeTooLarge  = errors.New("gitprovider: tree too large")
	ErrUpstream      = errors.New("gitprovider: upstream error")
	ErrHostNotAllowed = errors.New("gitprovider: host not allowed")
)
type RateLimitedError struct{ RetryAfter time.Duration } // errors.As; Is(ErrRateLimited)
var ErrRateLimited = errors.New("gitprovider: rate limited")
```

- `type Options struct{ GitHubAPIBase string }`: kosong = `https://api.github.com`. Override hanya diterima bila host:port-nya ada di `GIT_SYNC_INSECURE_TEST_HOSTS` (dipakai e2e/integration lewat env `GIT_SYNC_GITHUB_API_BASE`, yang hanya dibaca bila insecure test hosts terisi dan `APP_ENV != production`). Dengan begitu tidak ada jalan produksi untuk mengarahkan GitHub ke host lain.
- Setiap metode memanggil `policy.Allowed` lebih dulu (pertahanan berlapis) -> `ErrHostNotAllowed`.
- **GitHub** (K5): `VerifyToken` = `GET https://api.github.com/user`. `ListTree` = satu request `GET https://api.github.com/repos/{owner}/{repo}/git/trees/{url.PathEscape(branch)}?recursive=1`; `truncated=true` -> `ErrTreeTooLarge`; hanya `type=="blob"`; filter prefix `folder + "/"` (folder kosong = semua). `FetchBlob` = `GET /repos/{o}/{r}/git/blobs/{sha}` dengan `Accept: application/vnd.github.raw`, `ReadLimited(maxBytes)`. Tarball tidak dipakai karena redirect ke `codeload.github.com` (host lain).
- **GitLab**: `VerifyToken` = `GET https://{host}/api/v4/user`. `ListTree` = `GET https://{host}/api/v4/projects/{url.PathEscape(path)}/repository/tree?path={folder}&ref={branch}&recursive=true&per_page=100&pagination=keyset`; ikuti header `Link rel="next"` HANYA bila host sama, maksimal 50 halaman (lebih -> `ErrTreeTooLarge`); hanya `type=="blob"`; `Size=-1`. `FetchBlob` = `GET .../repository/blobs/{sha}/raw`.
- Header: GitHub `Authorization: Bearer <token>` + `X-GitHub-Api-Version: 2022-11-28`; GitLab `PRIVATE-TOKEN: <token>`. Token tidak pernah masuk URL. `User-Agent: grimoire-git-sync`.
- Pemetaan status: 401 -> `ErrUnauthorized`; 403 dengan `x-ratelimit-remaining: 0`, atau 429, atau ada `Retry-After` -> `RateLimitedError`; 403 lain -> `ErrForbidden`; 404 -> `ErrNotFound`; 5xx, error jaringan, error safehttp, JSON rusak -> `ErrUpstream` (dibungkus `%w` tanpa URL berisi token). Body respons error TIDAK dibaca ke log, dibuang dengan `io.Copy(io.Discard, io.LimitReader(body, 64<<10))`.
- Respons JSON tree dibatasi `ReadLimited(16 MiB)`.
- Tes: tabel status -> sentinel (GitHub dan GitLab); paginasi keyset 3 halaman; `Link` ke host lain diabaikan; 51 halaman -> `ErrTreeTooLarge`; `truncated` -> `ErrTreeTooLarge`; branch `feature/x` di-escape; header auth benar dan tidak ada token di URL yang tercatat server; blob melebihi `maxBytes` -> `tooLarge=true`; host tidak diizinkan -> `ErrHostNotAllowed` tanpa request.

### 5.5 `backend/internal/infra/middleware/timeout.go` + `ExtendTimeout`

- `Timeout(d)` menyimpan `r.Context()` sebelum deadline di context key privat `baseCtxKey{}` lalu memasang `WithTimeout`.
- `ExtendTimeout(d)`: ambil `base` dari key (tidak ada -> pakai `r.Context()`), `dl, cancel := context.WithTimeout(base, d)`, lalu bungkus dengan `type extendedCtx struct{ values context.Context; dl context.Context }`. `Deadline/Done/Err` didelegasikan ke `dl`, `Value` ke `values` (ctx request saat ini). Pembungkus ini wajib karena Session/TraceID dipasang SETELAH Timeout. Memulai ulang dari `base` saja akan menghilangkan identity.
- Tes: deadline route = d (> default); identity dan trace_id dari middleware setelah Timeout tetap terbaca; pembatalan klien (cancel ctx asal) merambat ke ctx extended; tanpa Timeout di depan tetap berfungsi.

### 5.6 `backend/internal/infra/config`

| Env | Field | Default | Validasi |
|---|---|---|---|
| `GIT_CREDENTIAL_KEY` | `GitCredentialKey []byte` (tidak pernah di-log; `LogValue` config menulis `set`/`unset`) | kosong | Kosong -> fitur Git nonaktif, warn sekali saat start. Terisi tetapi bukan base64 std 32 byte -> gagal start: `GIT_CREDENTIAL_KEY harus base64 dari tepat 32 byte` (tanpa nilai). |
| `GIT_SYNC_ALLOWED_HOSTS` | `GitSyncAllowedHosts []string` | kosong | Dipisah koma, trim, lowercase; tiap entri `host[:port]` tanpa skema/path/userinfo; `github.com`/`gitlab.com` diabaikan (sudah implisit); entri invalid -> gagal start. |
| `GIT_SYNC_TIMEOUT` | `GitSyncTimeout time.Duration` | `120s` | Harus > `HTTP_DEFAULT_TIMEOUT`. |
| `GIT_SYNC_INSECURE_TEST_HOSTS` | `GitSyncInsecureTestHosts []string` | kosong | `host:port`; terisi saat `APP_ENV=production` -> gagal start. |
| `GIT_SYNC_GITHUB_API_BASE` | `GitSyncGitHubAPIBase string` | kosong | Hanya test/e2e. Terisi -> wajib `http(s)://host:port` dengan host:port di `GIT_SYNC_INSECURE_TEST_HOSTS`; terisi saat `APP_ENV=production` -> gagal start. |

- Method `func (c Config) GitFeatureEnabled() bool { return len(c.GitCredentialKey) == 32 }`.
- Dokumentasi: `docs/configuration.md` (tabel env + paragraf "Kunci hilang/berganti: semua PAT tak terbaca, user diminta simpan ulang") dan `.env.example` (baris komentar + nilai kosong). `envexample_test` dan `docs_test` harus hijau.
- Batas sync memakai env yang sudah ada: `MAX_FILE_BYTES`, `MAX_UPLOAD_FILES`, `MAX_UPLOAD_BYTES`, `MAX_PROJECT_FILES`, `APISPEC_MAX_FILE_BYTES`, `APISPEC_MAX_SPECS`.
- Tes config: base64 valid 32 byte; 31 byte -> gagal; bukan base64 -> gagal; pesan error tidak memuat nilai; allowed hosts dinormalisasi; timeout <= default -> gagal; insecure hosts + production -> gagal.

### 5.7 `backend/lib/constants/bus.go`

```go
// Provider: T14 core/gitcredential (feature bustoken). Consumer: project/gitsync runsync, getconfig.
// Kontrak: lib/contracts/core/gitcredential/gettoken. Token tidak boleh di-log/diaudit/di-Publish/dikirim ke klien.
const BUS_CORE_GITCREDENTIAL_GET_TOKEN = "core.gitcredential.get-token"

// Provider: T14 project/knowledge (feature busupsertfiles). Consumer: project/gitsync runsync.
// Kontrak: lib/contracts/project/knowledge/upsertfiles. Menulis; tanpa cek keanggotaan (consumer wajib check-access).
const BUS_PROJECT_KNOWLEDGE_UPSERT_FILES = "project.knowledge.upsert-files"

// Provider: T14 project/apispec (feature busupsertspecs). Consumer: project/gitsync runsync.
// Kontrak: lib/contracts/project/apispec/upsertspecs. Menulis; tanpa cek keanggotaan (consumer wajib check-access).
const BUS_PROJECT_APISPEC_UPSERT_SPECS = "project.apispec.upsert-specs"
```

### 5.8 Aksi audit `backend/lib/contracts/core/audit/record/actions.go`

`ActionGitCredentialSaved = "git_credential.saved"`, `ActionGitCredentialDeleted = "git_credential.deleted"`, `ActionGitSyncRun = "git_sync.run"`. Tambahkan ke `Actions()` dan perbarui `actions_test.go`.

### 5.9 Kode error bersama `backend/lib/common/errs/errs.go` (D10)

`CodeHostNotAllowed = "HOST_NOT_ALLOWED"` (422), `CodeGitUpstream = "GIT_UPSTREAM_ERROR"` (502), `CodeGitFeatureDisabled = "GIT_FEATURE_DISABLED"` (503). Tambahkan ke peta bawaan `infra/http/respond/status.go` dan ke tes peta status.

---

## 6. Entity bersama di `base/`

### 6.1 `core/internal/gitcredential/base/` (tanpa I/O kecuali subpaket)

| File | Isi |
|---|---|
| `provider.go` | `type Provider string`; `ParseProvider(raw) (Provider, error)` hanya `github`/`gitlab` -> selain itu `ErrInvalidProvider` (400). |
| `host.go` | `NormalizeHost(p Provider, raw string) (string, error)`: github -> selalu `"github.com"` (input lain diabaikan bila kosong/`github.com`/`api.github.com`; selain itu `ErrHostNotAllowed`); gitlab -> trim, lowercase, buang `https://`, tolak userinfo/path/query, port boleh (1-65535), kosong -> `"gitlab.com"`, panjang 1-253. Policy dipanggil usecase. |
| `token.go` | `ValidateToken(raw string) (secret.String, error)`: trim; 8-512 byte; hanya ASCII cetak `0x21..0x7E` -> selain itu `ErrTokenFormat` (400, pesan tanpa isi token). |
| `credential.go` | `type Credential struct{ UserID string; Provider Provider; Host string; TokenEnc []byte; Last4 string; CreatedAt, UpdatedAt time.Time }`; `ToDTO() CredentialDTO`; `CredentialDTO{Provider, Host, Last4, CreatedAt, UpdatedAt}` dengan tag JSON `provider, host, last4, created_at, updated_at` (tanpa field token). |
| `aad.go` | `AAD(userID string, p Provider, host string) []byte` = `"grimoire.git_credential.v1|"+userID+"|"+p+"|"+host`. |
| `errors.go` | Konstanta kode `CodeTokenInvalid errs.Code = "TOKEN_INVALID"` (422). Sentinel `*errs.Error`: `ErrInvalidProvider`, `ErrTokenFormat` (`CodeInvalid` + details.fields), `ErrHostNotAllowed` (`errs.CodeHostNotAllowed`), `ErrTokenInvalid` ("token tidak valid atau tidak punya akses"), `ErrFeatureDisabled` (`errs.CodeGitFeatureDisabled`, "sync Git belum dikonfigurasi oleh admin"), `ErrUpstream` (`errs.CodeGitUpstream`, "provider Git tidak dapat dihubungi, coba lagi"), `ErrRateLimited` (`errs.CodeRateLimited`), `ErrCredentialNotFound` (`errs.CodeNotFound`, "token tidak ditemukan"). Pesan Indonesia, tidak memuat token. |
| `actor.go` | `ActorFrom(reqctx.Identity, bool) (Actor{ID, Label}, bool)` (pola apispec). |
| `audit/` | `Recorder` interface + `BusRecorder` lewat `bus.CallTyped` (Call, rule 6.3), fail-open; `TargetType = "git_credential"`; Args hanya `{provider, host, replaced}`. |
| `store/` | Helper sqlx bersama (`ValidUUID`, pemetaan constraint). |

Module wiring `core/internal/gitcredential/base.go`:
- `Register(d app.Deps)`: `registerCodes()` (`sync.Once`: `TOKEN_INVALID` 422); `box := secretbox.New(cfg.GitCredentialKey)` bila `GitFeatureEnabled()`, selain itu `nil`; `git := gitprovider.NewFromConfig(cfg)`; panggil `savecredential.Init`, `listcredentials.Init`, `deletecredential.Init`, `bustoken.Init`.
- Route group: `d.API.With(middleware.RequireAuth()).Route("/me/git-credentials", ...)`. Rute `/me` bukan di bawah `/projects`, jadi `Route` diizinkan.

### 6.2 `project/internal/gitsync/base/`

| File | Isi |
|---|---|
| `kind.go` | `type Kind string` (`KindKnowledge="knowledge"`, `KindAPISpec="api_spec"`); `ParseKind(raw) (Kind, error)` -> `ErrInvalidKind` (400); `(k Kind) AllowedExt(path) bool` (knowledge `.md`/`.txt`; api_spec `.yaml`/`.yml`/`.json`; case-insensitive); `(k Kind) SkipReason() string` (knowledge `"unsupported_type"`, api_spec `"NOT_SPEC_FILE"`, D14). |
| `limits.go` | `type Limits struct{ MaxFileBytes int64; MaxFiles int; MaxTotalBytes int64 }`; `LimitsFor(k Kind, cfg config.Config) Limits`: knowledge `{MaxFileBytes, MaxUploadFiles, MaxUploadBytes}`; api_spec `{APISpecMaxFileBytes, min(MaxUploadFiles, APISpecMaxSpecs), MaxUploadBytes}`. |
| `repourl.go` | `type RepoRef struct{ Provider, Host, Path, Branch, Folder, CanonicalURL string }`; `ParseRepoURL(raw, branchOverride, folderOverride string, allowed func(provider, host string) bool) (RepoRef, error)` (K3, rinci di 6.3). |
| `prefix.go` | `NormalizeTargetPrefix(raw string) (string, error)`: trim, buang `/` depan/belakang, tolak segmen `.`/`..`, NUL/kontrol, UTF-8 tidak valid, > 512 rune -> `ErrInvalidTargetPrefix` (400). Pemeriksaan otoritatif tetap di provider knowledge (`NormalizePrefix`). |
| `select.go` | `type Candidate struct{ RepoPath, RelPath, SHA string; Size int64 }`; `SelectFiles(entries []TreeEntry, folder string, k Kind) (cands []Candidate, skipped []ReportItem, err error)`: hanya entri di bawah `folder` (rekursif); `RelPath` = path tanpa prefix folder; ekstensi tidak didukung -> `skipped` (status `skipped`, `ReasonCode=k.SkipReason()`); folder tidak kosong dan tidak ada entri sama sekali di bawahnya -> `ErrFolderNotFound`; urut `RepoPath` naik. `TreeEntry` adalah tipe lokal base (bukan import infra) yang diisi usecase dari hasil gitClient. |
| `limits_check.go` | `CheckBatchLimits(cands []Candidate, l Limits) error`: `len > MaxFiles` atau jumlah `Size` yang diketahui (`>= 0`) > `MaxTotalBytes` -> `ErrSyncLimitExceeded(details{limit, max_files, max_total_bytes, files, known_bytes})`. `CheckFetchedTotal(total int64, l Limits) error` untuk pengecekan kumulatif. |
| `report.go` | `type Report struct{ Status Status; Summary Summary; Items []ReportItem; Truncated bool; ErrorCode, ErrorMessage string }`; `ReportItem{RepoPath, Target, Status, ReasonCode, Reason string; Issues []Issue}`; `Issue{Code, Message, Pointer string; Line int}`; `Status` `success|partial|failed`; item status `created|updated|unchanged|skipped|rejected`. `NewReport(items) Report` (urut RepoPath + hitung Summary dari items); `(r *Report) DeriveStatus()`: failed bila `ErrorCode != ""`; partial bila `Rejected > 0`; selain itu success. `FailedReport(code, msg) Report`; `(r Report) CapForStorage(n int) Report` memotong `Items` ke n dan set `Truncated`. Summary SELALU dihitung dari semua item sebelum dipotong. |
| `config.go` | `type Config struct{ ID, ProjectID string; Kind Kind; Provider, Host, RepoURL, RepoPath, Branch, Folder, TargetPrefix string; LastStatus *string; LastSyncedAt *time.Time; LastSyncedBy, LastSyncedByLabel *string; LastResult *Report; CreatedAt, UpdatedAt time.Time }`; `NewConfig(projectID, kind, ref RepoRef, prefix string) Config` (prefix dikosongkan untuk api_spec); `ToDTO() ConfigDTO`. |
| `dto.go` | `ConfigDTO`, `ReportDTO`, `SyncedByDTO{ID, Label}` dengan tag snake_case (bagian 12). |
| `actor.go` | `type Actor struct{ ID, Role, Label string }`, `ActorFrom`. |
| `errors.go` | Konstanta kode module (6.6) dan sentinel `*errs.Error` (pesan Indonesia, tanpa token). |
| `access/` | `Checker.Require(ctx, projectID) (Actor, error)` via bus `project.management.check-access` (salinan pola `knowledge/base/access`, level member/owner/admin lolos, none -> `ErrProjectNotFound` 404, tanpa identity -> 401). |
| `audit/` | `Recorder` `git_sync.run` via Call, fail-open; `TargetType="git_sync"`, `Target=kind`; Args `{kind, provider, host, repo_path, branch, folder, target_prefix}`; Result `{status, created, updated, unchanged, skipped, rejected, error_code}`. Tidak pernah ada token, URL lengkap dengan query, atau isi file. |
| `store/` | Helper sqlx bersama: `scanConfig`, kolom `configColumns`, marshal/unmarshal `last_result` jsonb. |

Module wiring `project/internal/gitsync/base.go`: `Register(d)`: `registerCodes()` (`sync.Once`); `git := gitprovider.NewFromConfig(cfg)`; `getconfig.Init(d, deps)`, `runsync.Init(d, deps)`. Route pola penuh di `d.API` (konvensi `/projects`):
- `d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/git-sync", getconfig)`
- `d.API.With(middleware.RequireAuth(), middleware.BodyLimit(16<<10), middleware.ExtendTimeout(cfg.GitSyncTimeout)).Post("/projects/{projectID}/git-sync/run", runsync)` (CSRF sudah global di `api.Use`).

### 6.3 `ParseRepoURL` (K3) — spesifikasi

1. `url.Parse(strings.TrimSpace(raw))`; panjang <= 2048; skema harus `https` -> selain itu `INVALID_REPO_URL` ("hanya URL https yang didukung"); `User != nil`, `RawQuery != ""`, `Fragment != ""` -> `INVALID_REPO_URL`.
2. Host lowercase (dengan port). `github.com` / `www.github.com` -> provider github, host `github.com`. Selain itu provider gitlab.
3. `allowed(provider, host) == false` -> `HOST_NOT_ALLOWED`.
4. Path di-split `/`, segmen kosong dibuang, segmen `.`/`..` -> `INVALID_REPO_URL`.
   - GitHub: minimal 2 segmen `{owner}/{repo}`; repo dibuang suffix `.git`; segmen ke-3 harus `tree` bila ada (`blob` -> `INVALID_REPO_URL` "gunakan URL folder (tree)"); setelah `tree`: segmen pertama = branch kandidat, sisanya folder.
   - GitLab: cari segmen `-`; sebelum `-` = project path (minimal 2 segmen, `.git` dibuang); setelah `-` wajib `tree`, lalu branch kandidat + folder. Tanpa `-`, seluruh path = project path.
5. Branch: `branchOverride` (trim) menang. Bila override diisi dan URL punya segmen setelah `tree`, folder dari URL dihitung ulang: bila segmen-segmen setelah `tree` diawali `strings.Split(override, "/")`, sisanya = folder; bila tidak, folder URL diabaikan. Ini membuat `.../tree/feature/x/docs` + branch `feature/x` -> folder `docs`. Tanpa override, branch = segmen pertama setelah `tree`. Tanpa branch sama sekali -> `INVALID_REPO_URL` dengan details `{field: "branch"}` ("isi Branch").
6. Folder: `folderOverride` (bila tidak kosong) menang atas folder URL. Dinormalisasi: tanpa `/` depan/belakang, tanpa `..`/`.`, tanpa NUL, maksimal 1024.
7. Branch divalidasi: 1-255 byte, tanpa spasi/kontrol, tanpa `..`, tidak diawali `-`.
8. `CanonicalURL` disusun ulang dari bagian yang lolos (tidak menyimpan string mentah user).

---

## 6.4 Kontrak bus (`lib/contracts`, tipe netral, request dikirim sebagai NILAI)

### 6.4.1 `lib/contracts/core/gitcredential/gettoken/gettoken.go`

```go
// Package gettoken: kontrak core.gitcredential.get-token (T14).
// Token TIDAK BOLEH di-log, diaudit, di-Publish, disimpan, atau dikirim ke klien.
// Pemanggil WAJIB mengisi UserID dari identity request (bukan dari data tersimpan),
// sehingga PAT user lain tidak pernah dipakai.
package gettoken

type Request struct {
	UserID   string // uuid
	Provider string // "github" | "gitlab"
	Host     string // "github.com" | host gitlab ternormalisasi
}

type Response struct {
	Found      bool          // false = user belum menyimpan token untuk (provider, host)
	Unreadable bool          // Found && dekripsi gagal (kunci berganti) -> minta simpan ulang
	Disabled   bool          // GIT_CREDENTIAL_KEY kosong; Found/Token tidak bermakna
	Token      secret.String // hanya terisi bila Found && !Unreadable && !Disabled
}
```
Error Go hanya untuk request invalid (UserID bukan uuid, provider tidak dikenal, host kosong) atau kegagalan DB. Menggantikan `ErrCredentialNotFound` di tiket (konvensi `Found bool`, rule 6.7). Package mengimpor `lib/common/secret` (lib -> lib, diizinkan 4.5/4.9).

### 6.4.2 `lib/contracts/project/knowledge/upsertfiles/upsertfiles.go`

```go
// Package upsertfiles: kontrak project.knowledge.upsert-files (T14).
// Provider TIDAK memeriksa keanggotaan: consumer WAJIB check-access lebih dulu.
// Provider selalu membatasi query pada Request.ProjectID (rule 6.10). Tidak menghapus file.
package upsertfiles

const SourceGitSync = "git_sync"

type Actor struct{ UserID, Role, Label string }
type Meta struct{ IP, UserAgent string }

type Item struct {
	Index    int    // urutan dari consumer; dipantulkan di ItemResult
	Path     string // relatif terhadap folder git (tanpa TargetPrefix)
	Content  []byte // nil bila TooLarge
	TooLarge bool
}

type Request struct {
	ProjectID    string
	Actor        Actor
	TargetPrefix string // "" = root; divalidasi provider (invalid -> *errs.Error CodeInvalid, sebelum tulis)
	Source       string // SourceGitSync
	Meta         Meta
	Items        []Item
}

type ItemResult struct {
	Index  int
	Path   string // path akhir di knowledge (setelah prefix + normalisasi), atau path mentah bila ditolak
	Status string // created|updated|unchanged|skipped|rejected
	Reason string // kode reason knowledge (mis. "too_large", "hidden"); "" bila sukses
}

type Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int }

type Response struct {
	Items   []ItemResult // urut Index
	Summary Summary
}
```

### 6.4.3 `lib/contracts/project/apispec/upsertspecs/upsertspecs.go`

```go
// Package upsertspecs: kontrak project.apispec.upsert-specs (T14).
// Provider TIDAK memeriksa keanggotaan; selalu membatasi query pada Request.ProjectID. Tidak menghapus spec.
package upsertspecs

const SourceGitSync = "git_sync"

type Actor struct{ UserID, Role, Label string }
type Meta struct{ IP, UserAgent string }

type Item struct {
	Index      int
	SourcePath string // path di repo, mis. "api/openapi/billing.yaml"
	Content    []byte
	TooLarge   bool
}

type Request struct {
	ProjectID string
	Actor     Actor
	Source    string
	Meta      Meta
	Items     []Item
}

type Issue struct {
	Code, Message, Pointer string
	Line                   int
}

type ItemResult struct {
	Index          int
	SourcePath     string
	Name           string // nama spec hasil DeriveName; "" bila tidak terbentuk
	Status         string // created|updated|unchanged|skipped|rejected
	ReasonCode     string // kode apispec (NOT_SPEC_FILE, INVALID_SPEC, DUPLICATE_NAME_IN_UPLOAD, ...)
	Reason         string // pesan Indonesia
	Issues         []Issue
	OpenAPIVersion string
	Title          string
	EndpointCount  int
}

type Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int }

type Response struct {
	Items   []ItemResult
	Summary Summary
}
```

### 6.4.4 Aturan bersama provider bus

- `bus.Expect[T](req)` dulu (tipe salah -> error jelas, rule 6.9).
- `ProjectID` harus uuid valid dan `Actor.UserID` tidak kosong -> selain itu `*errs.Error CodeInvalid`.
- Aktor audit diambil dari `Request.Actor`, bukan dari ctx. ctx tetap meneruskan `trace_id`.
- Audit memakai aksi yang sudah ada (`knowledge.upload.completed` / `apispec.uploaded`) dengan Args tambahan `"source": "git_sync"`.
- Katalog bus di `docs/tickets/README.md` ditambah tiga baris berikut, beserta paragraf lokasi kontrak:
  - `core.gitcredential.get-token | Call | T14 | T14 (gitsync runsync, getconfig)`
  - `project.knowledge.upsert-files | Call | T14 | T14 (gitsync runsync)`
  - `project.apispec.upsert-specs | Call | T14 | T14 (gitsync runsync)`

### 6.5 Perubahan `base/` di module knowledge dan apispec (D1)

- **`knowledge/base/ingest/ingest.go` (baru)**:
  ```go
  type Tx interface {
  	LockProject(ctx context.Context, projectID string) error
  	CountFiles(ctx context.Context, projectID string) (int, error)
  	ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
  	InsertFiles(ctx context.Context, files []base.File) error
  	UpdateFiles(ctx context.Context, files []base.File) error
  }
  // Apply dipanggil DI DALAM tx pemanggil. Mengembalikan keputusan terencana + byte yang ditulis.
  func Apply(ctx context.Context, tx Tx, projectID string, actor base.Actor, incoming []base.Incoming, maxProjectFiles int) ([]base.Decision, int, error)
  ```
  `uploadfiles/usecase.go` mengganti blok di dalam `InTx` dengan `ingest.Apply` (interface `repository` uploadfiles memenuhi `ingest.Tx`). Tidak ada perubahan SQL.
- **`apispec/base/ingest/ingest.go` (baru)**:
  ```go
  type Tx interface { LockProjectSpecs; LockSpec; CountSpecs; InsertSpec; UpdateSpecContent; ReplaceEndpoints } // signature persis dari uploadspecs.Tx
  type Repo interface { InTx(ctx context.Context, fn func(Tx) error) error }
  type Deps struct { Repo Repo; Audit audit.Recorder; Clock base.Clock; MaxSpecs int; MaxFileBytes int64 }
  type Request struct { ProjectID string; Actor base.Actor; Meta base.RequestMeta; Source string /* "" = upload */ }
  func ProcessFile(ctx context.Context, d Deps, req Request, f upload.File, explicitName string, reserved map[string]struct{}) (base.FileResult, error)
  ```
  Isinya = `processFile` + `persist` + `reject` saat ini. Bila `req.Source != ""`, Args audit ditambah `"source"`. Event audit membawa `Actor: &req.Actor`.
- **`apispec/base/audit/audit.go`**: `Event.Actor *base.Actor` (opsional). Bila non-nil, dipakai menggantikan `reqctx`. Perubahan aditif dengan tes baru, tanpa mengubah tes lama.
- Tes regresi: semua tes `uploadfiles` dan `uploadspecs` (termasuk Swagger 2.0 T10) hijau tanpa perubahan asersi.

### 6.6 Kode error dan status HTTP

| Kode | Status | Didefinisikan / didaftarkan di | Dipakai oleh |
|---|---|---|---|
| `HOST_NOT_ALLOWED` | 422 | `lib/common/errs` + peta bawaan (D10) | savecredential, runsync |
| `GIT_UPSTREAM_ERROR` | 502 | `lib/common/errs` + peta bawaan | savecredential, runsync |
| `GIT_FEATURE_DISABLED` | 503 | `lib/common/errs` + peta bawaan | savecredential, runsync |
| `TOKEN_INVALID` | 422 | `gitcredential/base/errors.go`, `registerCodes` | savecredential |
| `CREDENTIAL_MISSING` | 422 | `gitsync/base/errors.go`, `registerCodes` | runsync |
| `CREDENTIAL_UNREADABLE` | 422 | gitsync | runsync |
| `INVALID_REPO_URL` | 400 | gitsync | runsync |
| `GIT_TOKEN_REJECTED` | 422 | gitsync | runsync |
| `GIT_ACCESS_DENIED` | 422 | gitsync | runsync |
| `GIT_NOT_FOUND` | 404 | gitsync | runsync |
| `SYNC_LIMIT_EXCEEDED` | 422 | gitsync | runsync |
| `SYNC_IN_PROGRESS` | 409 | gitsync | runsync |
| `GIT_TREE_TOO_LARGE` | 422 | gitsync | runsync |
| `RATE_LIMITED` | 429 | bawaan | savecredential, runsync (+ header `Retry-After` bila diketahui) |
| `TIMEOUT` | 504 | bawaan | runsync (ctx deadline) |
| `INVALID_ARGUMENT` | 400 | bawaan | provider/kind/token/prefix/body tidak valid |
| `NOT_FOUND` | 404 | bawaan | project tidak ada/non-anggota; token tidak ditemukan saat hapus |
| `UNAUTHORIZED` | 401 | bawaan | tanpa login |

Handler test tiap module memanggil `registerCodes()` (yang memakai `sync.Once`), bukan `RegisterCodeStatus` langsung, agar tidak panic ganda.

### 6.7 Format error (tidak berubah, `infra/http/respond`)

`{"error": {"code": "...", "message": "...", "details": {...} | null}, "trace_id": "..."}`.

### 6.8 Konvensi JSON

Semua field JSON T14 memakai snake_case (konvensi repo), misalnya `repo_url`, `target_prefix`, `last_synced_at`, `has_credential`, `allowed_gitlab_hosts`, `reason_code`, `repo_path`. Catatan dekomposisi yang memakai camelCase disesuaikan ke snake_case. Zod di FE mengikuti.

---

## 7. Migration

Placeholder versi di-generate saat implementasi (`python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"`), sesuai `.claude/rules/migrations.md`. Versi otomatis > `20261022160000000`. Cek duplikat sebelum commit: `ls backend/migrations/*/ | cut -c1-17 | sort | uniq -d` harus kosong.

### 7.1 `backend/migrations/core/<VERSION>_create_git_credentials.sql`

```sql
-- T14 core/gitcredential. Forward-only. PAT terenkripsi AES-256-GCM (nonce||ciphertext||tag).
-- user_id FK CASCADE (README konvensi: FK lintas module hanya project_id/user_id + CASCADE):
-- user yang dihapus tidak meninggalkan PAT yatim (D11).
CREATE TABLE IF NOT EXISTS git_credentials (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    text        NOT NULL,
    host        text        NOT NULL,
    token_enc   bytea       NOT NULL,
    last4       text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT git_credentials_provider_check CHECK (provider IN ('github','gitlab')),
    CONSTRAINT git_credentials_host_check CHECK (host = lower(host) AND char_length(host) BETWEEN 1 AND 253),
    CONSTRAINT git_credentials_github_host CHECK (provider <> 'github' OR host = 'github.com'),
    CONSTRAINT git_credentials_token_enc_len CHECK (octet_length(token_enc) > 28),
    CONSTRAINT git_credentials_last4_len CHECK (char_length(last4) BETWEEN 1 AND 4),
    CONSTRAINT git_credentials_user_provider_host_key UNIQUE (user_id, provider, host)
);
```
Index tambahan tidak perlu: unique `(user_id, provider, host)` melayani `ListByUser` (prefix `user_id`), `Get`, dan `Delete`.

### 7.2 `backend/migrations/project/<VERSION>_create_git_sync_configs.sql`

```sql
-- T14 project/gitsync. Forward-only. Tidak menyimpan PAT.
-- last_synced_by tanpa FK ke users (rule 8.5); label snapshot (rule 8.7).
CREATE TABLE IF NOT EXISTS git_sync_configs (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id           uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind                 text        NOT NULL,
    provider             text        NOT NULL,
    host                 text        NOT NULL,
    repo_url             text        NOT NULL,
    repo_path            text        NOT NULL,
    branch               text        NOT NULL,
    folder               text        NOT NULL DEFAULT '',
    target_prefix        text        NOT NULL DEFAULT '',
    last_status          text        NULL,
    last_synced_at       timestamptz NULL,
    last_synced_by       uuid        NULL,
    last_synced_by_label text        NULL,
    last_result          jsonb       NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT git_sync_configs_kind_check CHECK (kind IN ('knowledge','api_spec')),
    CONSTRAINT git_sync_configs_provider_check CHECK (provider IN ('github','gitlab')),
    CONSTRAINT git_sync_configs_repo_url_len CHECK (char_length(repo_url) <= 2048),
    CONSTRAINT git_sync_configs_branch_len CHECK (char_length(branch) BETWEEN 1 AND 255),
    CONSTRAINT git_sync_configs_status_check CHECK (last_status IS NULL OR last_status IN ('success','partial','failed')),
    CONSTRAINT git_sync_configs_prefix_kind CHECK (kind = 'knowledge' OR target_prefix = ''),
    CONSTRAINT git_sync_configs_project_kind_key UNIQUE (project_id, kind)
);
```
Runner menjalankan tiap file dalam satu tx dengan simple protocol, tanpa `CONCURRENTLY`. Tidak ada down-migration (konvensi repo).

---

## 8. Plan (langkah berurutan)

### Langkah 0 — FOUNDATION (satu commit, sebelum usecase)

1. `lib/common/secret` + tes (5.1).
2. `infra/crypto/secretbox` + tes (5.2).
3. `infra/safehttp` + tes (5.3).
4. `infra/gitprovider` + tes (5.4). Bergantung pada 1 dan 3.
5. `infra/middleware/timeout.go`: `Timeout` menyimpan ctx dasar, tambah `ExtendTimeout` + tes (5.5).
6. `infra/config`: 4 env baru + `GitFeatureEnabled()` + tes; `docs/configuration.md`; `.env.example` (5.6).
7. `lib/constants/bus.go`: 3 konstanta (5.7).
8. `lib/contracts/core/gitcredential/gettoken`, `lib/contracts/project/knowledge/upsertfiles`, `lib/contracts/project/apispec/upsertspecs` (6.4).
9. `lib/contracts/core/audit/record/actions.go` + `actions_test.go` (5.8).
10. `lib/common/errs` 3 kode bersama + `infra/http/respond/status.go` + tes (5.9).
11. Migrasi core dan project (bagian 7), versi di-generate.
12. Skeleton module `core/internal/gitcredential/{base.go, base/...}` dan `project/internal/gitsync/{base.go, base/...}` lengkap dengan entity + tes unit (bagian 6.1-6.3). `base.go` memanggil `Init` feature yang belum ada -> pada commit foundation `Register` hanya mendaftarkan kode error. Feature ditambahkan oleh usecase.
13. Wiring domain: `core/base.go` memanggil `gitcredential.Register(d)`; `project/base.go` memanggil `gitsync.Register(d)`.
14. Refaktor D1 (6.5): `knowledge/base/ingest`, `apispec/base/ingest`, `apispec/base/audit.Event.Actor`; `uploadfiles` dan `uploadspecs` memakainya. Jalankan tes knowledge/apispec (unit + integration) dan pastikan hijau tanpa perubahan asersi.
15. OpenAPI skeleton: `api/modules/core/git-credential.yml` dan `api/modules/project/git-sync.yml` dengan schema bersama (`GitCredential`, `GitCredentialList`, `GitSyncConfig`, `GitSyncReport`, `GitSyncReportItem`, `GitSyncIssue`) dan seluruh path (bagian 12), `$ref` di `api/root.yml`, `Error` dari `api/modules/infra/foundation.yml`. `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau. Usecase hanya merapikan deskripsi/contoh bila perlu.
16. `docs/tickets/README.md`: katalog bus (3 baris + paragraf kontrak), katalog audit (`git_credential.saved`, `git_credential.deleted`, `git_sync.run`), struktur (`core/internal/{auth,auditlog,gitcredential}`, `project/internal/{management,apikey,knowledge,apispec,gitsync}`, `infra/{...,mailer,crypto,safehttp,gitprovider}`). Status T14 diisi saat merge.
17. Verifikasi foundation: `go build ./...`, `go test ./backend/...` (unit), `go test -tags integration` untuk knowledge/apispec/migrasi, `backend/internal/archtest`, lint.

### Langkah 1 — usecase backend (paralel setelah langkah 0, file tidak tumpang tindih)

| Gelombang | Slug | Plan | Ketergantungan |
|---|---|---|---|
| 1a | `savecredential` | `usecase-savecredential.md` | langkah 0 |
| 1a | `listcredentials` | `usecase-listcredentials.md` | langkah 0 |
| 1a | `deletecredential` | `usecase-deletecredential.md` | langkah 0 |
| 1a | `bustoken` | `usecase-bustoken.md` | langkah 0 |
| 1a | `busupsertfiles` | `usecase-busupsertfiles.md` | langkah 0 (termasuk refaktor D1) |
| 1a | `busupsertspecs` | `usecase-busupsertspecs.md` | langkah 0 (termasuk refaktor D1) |
| 1a | `getconfig` | `usecase-getconfig.md` | langkah 0 (tes memakai fake bus get-token) |
| 1b | `runsync` | `usecase-runsync.md` | unit dengan fake bus bisa paralel; `runsync_integration_test.go` butuh bustoken, busupsertfiles, busupsertspecs |

Tiap feature menambah `Init` di `base.go` module masing-masing (gitcredential, gitsync, knowledge, apispec). Untuk menghindari konflik: tiap usecase hanya menambah satu baris `Init` + satu baris di `routes_test.go`/`bus registration test` module itu.

### Langkah 2 — frontend (paralel dengan langkah 1, kontrak di bagian 12)

Lihat `frontend-plan.md` dan ringkasan bagian 9.

### Langkah 3 — verifikasi akhir

`go test ./...`, `go test -tags integration ./...`, archtest, lint, `make api-check`, `make api-selftest`, `npm run test`, `npm run depcruise`, `npm run test:arch`, `npm run build`, e2e `frontend/e2e/git-sync.spec.ts` + regresi `knowledge*.spec.ts`/`api-spec*.spec.ts`. Tes PAT canary (bagian 10.4) hijau.

---

## 9. Frontend (ringkasan; rinci di `frontend-plan.md`)

- `frontend/src/domain/core/git-credential/{listcredentials,savecredential,deletecredential}/`: services (Zod), store `gitCredentials`, view `GitTokensSection` (list), `SaveTokenDialog` (save), konfirmasi hapus. Ekspor lewat `index.ts`.
- Komposisi: `frontend/src/app/views/ProfileView.svelte` mendapat seksi "Token Git" dengan anchor `id="token-git"`; lebar dinaikkan dari `max-w-sm` ke `max-w-2xl` (Pertanyaan terbuka 2: bagian dari halaman Profil yang ada, bukan halaman baru).
- `frontend/src/domain/project/git-sync/{getconfig,runsync}/`: service `getConfig`/`runSync`, store dialog (`idle|running|done|error`), view `GitSyncDialog` (prop `kind`, `projectID`, `onSynced`), komponen `RepoOriginChip`, `ConfigSummary`, `SyncReportTable`, `StatusBadge`.
- Tombol sekunder "Sync dari Git" di `PageHeader` Knowledge (`app/views/workspace/knowledge/KnowledgeTab.svelte` + `KnowledgeDialogs.svelte`) dan `app/views/workspace/ApiSpecsTab.svelte`; aksi utama tetap Upload; `onSynced` memuat ulang pohon/daftar.
- Status PAT di dialog: memakai `has_credential`/`credential_unreadable`/`feature_enabled` dari `getconfig` dan error `CREDENTIAL_MISSING` (tanpa import silang git-credential dari git-sync; depcruise hijau). Tautan `/profile#token-git`.
- i18n `gitCredential.*` dan `gitSync.*` di `frontend/src/shared/i18n/{id,en}.json`, termasuk setiap kode error (6.6). Reason per item memakai kunci reason knowledge/apispec yang sudah ada (D14).
- **Visual** (DESIGN.md, framed-sheet):
  - Profil/Token Git. Pertanyaan: "Token Git mana yang sudah saya simpan, dan untuk host apa?" Tabel panel (radius 8, `shadow-panel`), kolom Provider (ikon outline 16px + nama), Host (mono), Status (pil positive "Tersimpan" + `…7Qx2` mono redup, atau pil neutral "Belum ada"), Diperbarui (relatif, rata kanan), Aksi (ghost Simpan/Ganti, Hapus). Baris GitHub selalu ada; baris GitLab per host + aksi "Tambah host GitLab". Dialog simpan (`shadow-pop`): Provider, Host (Select dari `allowed_gitlab_hosts`), Token (`type=password`, `autocomplete=off`), teks scope mono (`repo` read; `read_api` + `read_repository`), peringatan redup "Token tidak bisa dilihat lagi setelah disimpan." Fitur nonaktif: seksi tetap berbingkai dengan teks "Sync Git belum dikonfigurasi oleh admin (GIT_CREDENTIAL_KEY)".
  - Dialog Sync dari Git. Pertanyaan: "Apa yang akan/telah ditarik dari repo, dan file mana yang berubah?" Dialog max 720 dengan dua state. State Form berisi URL (mono), Branch, Folder (auto dari URL), "Simpan di folder" (knowledge saja, placeholder "/ (root)"), blok `text-warning` "File bernama sama akan ditimpa. File yang tidak ada di repo tidak dihapus.", ringkasan config terakhir, tombol sekunder "Sync ulang" dan tombol utama "Sync". State Hasil berisi ringkasan angka (tabular-nums) dan tabel panel: Path repo (mono), Tujuan (mono), Status (pil: dibuat=positive, diperbarui=neutral, tidak berubah=neutral redup, dilewati=neutral, ditolak=negative), Alasan (+ salin). Baris ditolak dapat diperluas untuk menampilkan issues (pointer mono). Progres ditunjukkan tombol yang berganti teks "Menarik dari GitHub…" dengan spinner kecil. Tanpa PAT: tombol Sync nonaktif + kotak info "Atur Token Git".
  - Elemen khas: chip asal-repo mono `github.com/noobee/grimoire-docs@main:docs/` (gaya chip agent).
  - Token: `bg-surface`, `border-border`, `shadow-pop`, `shadow-panel`, `text-muted`, `pill-{positive,negative,warning,neutral}`, `text-warning`, `accent-ink` hanya untuk cincin fokus. Radius 6/8/16. Tinggi baris 48, kontrol 32.
  - Data contoh: bagian 12 (repo `noobee/grimoire-docs`, GitLab `git.noobee.id`, last4 `7Qx2`).

---

## 10. Test Scenario (foundation)

### 10.1 Unit lib/infra

| Paket | Kasus |
|---|---|
| `lib/common/secret` | semua verb fmt, slog text/JSON, json.Marshal struct, `Reveal`, `Last4` ("abc" -> "abc", "" -> "") |
| `infra/crypto/secretbox` | round-trip, kunci salah, AAD beda, blob pendek, nonce unik, ukuran kunci |
| `infra/safehttp` | IPv4 privat/loopback/metadata/CGNAT/0.0.0.0, IPv6 ULA/link-local/mapped, DNS rebinding (resolver palsu), redirect host lain, redirect > 3, http ditolak, `InsecureAllowHosts` hanya port terdaftar, proxy env diabaikan (`HTTPS_PROXY` di-set dalam tes), `ReadLimited` |
| `infra/gitprovider` | pemetaan status (401/403 rate/403 biasa/404/429+Retry-After/500/JSON rusak), GitHub tree + truncated, GitLab keyset 3 halaman, Link host lain, 51 halaman, escape branch `feature/x`, header auth, token tidak ada di URL/log server, blob tooLarge, host tidak diizinkan tanpa request |
| `infra/middleware` | `ExtendTimeout`: deadline, nilai ctx tetap, cancel merambat |
| `infra/config` | 4 env baru (5.6) |
| `lib/contracts/core/audit/record` | `Actions()` memuat 3 aksi baru |
| `infra/http/respond` | 3 kode bersama -> 422/502/503 |

### 10.2 Unit entity

- `gitcredential/base`: `ParseProvider` (github, gitlab; `GitHub` diterima case-insensitive lalu di-lowercase; `bitbucket` dan "" -> invalid); `NormalizeHost` (github kosong/`github.com`/`api.github.com` -> `github.com`; github `evil.com` -> HOST_NOT_ALLOWED; gitlab kosong -> `gitlab.com`; `HTTPS://Git.Noobee.ID/` -> `git.noobee.id`; `git.noobee.id:8443` ok; `user@host` -> invalid; `host/path` -> invalid); `ValidateToken` (7 char, 513 char, spasi, non-ASCII, trim); `AAD` deterministik; `ToDTO` JSON tanpa kata `token`.
- `gitsync/base`:
  - `ParseRepoURL` (tabel): `https://github.com/noobee/grimoire-docs/tree/main/docs` -> (github, github.com, noobee/grimoire-docs, main, docs); tanpa tree + branch override `main` -> folder ""; tanpa tree tanpa branch -> INVALID_REPO_URL field branch; `.git` dibuang; `/blob/` -> INVALID; `/tree/feature/x/docs` + override `feature/x` -> folder `docs`; `/tree/feature/x/docs` tanpa override -> branch `feature`, folder `x/docs`; folder override menang; GitLab `https://gitlab.com/grp/sub/repo/-/tree/main/api` -> path `grp/sub/repo`; GitLab self-hosted di allowlist -> ok, tidak -> HOST_NOT_ALLOWED; `http://` -> INVALID; userinfo `https://tok@github.com/...` -> INVALID (dan pesan tidak memuat `tok`); query/fragment -> INVALID; `..` -> INVALID; > 2048 -> INVALID; branch `-x` / `a..b` -> INVALID.
  - `NormalizeTargetPrefix`: "" -> "", "/guides/" -> "guides", "a/../b" -> invalid, NUL -> invalid, 513 rune -> invalid.
  - `SelectFiles`: folder `docs` dengan `docs/a.md`, `docs/sub/b.txt`, `docs/img/x.png` (skipped `unsupported_type`), `docsx/c.md` (di luar), `README.md` (di luar); folder kosong = semua; folder tidak ada -> `ErrFolderNotFound`; api_spec `.YAML` diterima; urutan stabil.
  - `CheckBatchLimits`: tepat batas lolos; +1 file -> SYNC_LIMIT_EXCEEDED; total size > max -> SYNC_LIMIT_EXCEEDED; size -1 diabaikan; `CheckFetchedTotal`.
  - `Report`: `DeriveStatus` (ErrorCode -> failed; rejected>0 -> partial; else success; nol item -> success); `CapForStorage(2)` memotong item tetapi Summary utuh dan `Truncated=true`; JSON report tidak punya field token.
  - `LimitsFor`: api_spec `MaxFiles = min(MaxUploadFiles, APISpecMaxSpecs)`.
- `knowledge/base/ingest`, `apispec/base/ingest`: tes unit dengan fake Tx (dipindah dari tes usecase upload bila ada yang menguji blok yang sama; tes upload lama tetap ada).

### 10.3 Integration (tag `integration`, Postgres nyata)

- Migrasi core/project berjalan di DB kosong dan DB berisi data T13; constraint: github host selain `github.com` ditolak; `token_enc` 28 byte ditolak; unique `(user_id, provider, host)`; hapus user -> baris credential ikut terhapus (CASCADE); unique `(project_id, kind)`; `target_prefix` non-kosong untuk `api_spec` ditolak; hapus project -> config terhapus.
- Regresi `knowledge_integration_test.go`, `apispec_integration_test.go` hijau setelah refaktor D1.

### 10.4 Tes PAT canary (wajib, lintas usecase)

Canary `ghp_CANARY0123456789abcdefCANARY`. Logger buffer (slog JSON level debug) dipasang di Deps. Jalankan save (sukses dan 401), list, bus get-token, sync gagal (401 provider, 404, rate limit), dan sync sukses. Assert canary tidak muncul di: buffer log, setiap body respons, setiap `record.Request` yang diterima fake audit (dan baris `audit_log` pada integration), `git_sync_configs` (semua kolom, termasuk `last_result::text`), dan error message. Lokasi: `gitcredential/savecredential/usecase_test.go`, `gitcredential/bustoken/handler_test.go`, `gitsync/runsync/usecase_test.go` + `runsync_integration_test.go`.

### 10.5 Arch

`backend/internal/archtest` hijau: `gitsync` tidak meng-import `gitcredential`/`knowledge`/`apispec` (hanya `lib/contracts`, `lib/constants`, `infra/bus`); `infra/**` tidak meng-import domain; `lib/contracts/**` hanya meng-import `lib/common/secret`. `routes_test.go` per module memastikan route terdaftar dengan middleware yang benar dan handler bus terdaftar sekali.

Skenario per usecase (handler, usecase, repository integration) ada di plan usecase masing-masing.

---

## 11. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`)

Setup: fixture Node `frontend/e2e/fixtures/fake-git-server.ts` yang meniru `GET /user`, `GET /repos/{o}/{r}/git/trees/{branch}?recursive=1`, `GET /repos/{o}/{r}/git/blobs/{sha}` (Accept raw), plus endpoint kontrol untuk mengganti isi repo antar langkah. Server berjalan di `127.0.0.1:<port>` dan backend e2e dijalankan dengan `GIT_SYNC_INSECURE_TEST_HOSTS=127.0.0.1:<port>`, `GIT_CREDENTIAL_KEY=<base64 32 byte uji>`, serta `GIT_SYNC_GITHUB_API_BASE=http://127.0.0.1:<port>` (5.4/5.6; hanya terbaca bila host ada di insecure test hosts dan bukan produksi). Token sah fixture: `ghp_e2eValidToken7Qx2`.

Positif:
1. User membuka `/profile`, seksi Token Git menampilkan GitHub "Belum ada". Simpan token sah -> pil "Tersimpan" + `…7Qx2`; token tidak muncul di DOM/network response (cek `page.on('response')`).
2. Workspace project P > Knowledge > "Sync dari Git": URL `https://github.com/noobee/grimoire-docs/tree/main/docs`, prefix kosong -> hasil: `auth/login-flow.md` dibuat, `api/rate-limit.md` dibuat, `img/diagram.png` dilewati (tipe tidak didukung). Pohon knowledge memuat `auth/login-flow.md` setelah dialog ditutup.
3. Buka dialog lagi: form terisi otomatis (URL, branch `main`, folder `docs`), ringkasan "Terakhir: … · Berhasil · 2 dibuat …". Klik "Sync ulang" tanpa perubahan repo -> semua `tidak berubah`.
4. Fixture mengubah isi `docs/auth/login-flow.md` -> Sync ulang -> `diperbarui`; file knowledge buatan manual `notes/manual.md` tetap ada.
5. API Spec > Sync dari Git dengan folder `api/openapi`: `billing.yaml` -> `billing` dibuat (14 endpoint); `payments.json` ditolak ("$ref eksternal tidak didukung"), baris dapat diperluas dan memperlihatkan pointer `#/paths/~1charges/get`; status ringkasan "Berhasil sebagian".
6. Prefix "Simpan di folder" `guides` -> file masuk `guides/auth/login-flow.md`.

Negatif:
1. Simpan token tidak sah (fixture 401) -> pesan "Token tidak valid atau tidak punya akses", status tetap "Belum ada".
2. Hapus token GitHub (konfirmasi) -> buka dialog Sync: kotak info "Atur Token Git", tombol Sync nonaktif; pemanggilan API langsung `runSync` -> 422 `CREDENTIAL_MISSING`.
3. User B (anggota P tanpa token) membuka dialog: config A terlihat, Sync nonaktif; API -> `CREDENTIAL_MISSING`; config A tidak berubah.
4. URL `https://bitbucket.org/x/y` -> `HOST_NOT_ALLOWED` tampil di form.
5. URL `https://github.com/noobee/grimoire-docs` tanpa branch -> pesan "Isi Branch".
6. Folder `tidak-ada` -> `GIT_NOT_FOUND` "folder tidak ditemukan"; ringkasan config menampilkan "Gagal".
7. Fixture mode rate limit (403 + `x-ratelimit-remaining: 0`) -> pesan rate limit.
8. Non-anggota memanggil `GET /api/projects/{P}/git-sync?kind=knowledge` -> 404; tanpa login -> 401.

---

## 12. Request/Response body

Format error: `{"error": {"code", "message", "details"}, "trace_id"}`. Semua contoh `trace_id` ditulis `"4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"`. Semua mutasi mengirim header CSRF (`X-Requested-With: XMLHttpRequest`).

### 12.1 GET `/api/me/git-credentials`

200:
```json
{
  "enabled": true,
  "allowed_gitlab_hosts": ["gitlab.com", "git.noobee.id"],
  "items": [
    { "provider": "github", "host": "github.com", "last4": "a9F3", "created_at": "2026-10-22T07:00:00Z", "updated_at": "2026-10-22T07:00:00Z" },
    { "provider": "gitlab", "host": "git.noobee.id", "last4": "7Qx2", "created_at": "2026-10-22T07:05:00Z", "updated_at": "2026-10-22T07:10:00Z" }
  ]
}
```
Fitur nonaktif: `{"enabled": false, "allowed_gitlab_hosts": ["gitlab.com"], "items": []}` (200).

### 12.2 PUT `/api/me/git-credentials/{provider}`

Request (`provider=gitlab`):
```json
{ "host": "git.noobee.id", "token": "glpat-xxxxxxxxxxxxxxxx7Qx2" }
```
Request (`provider=github`, host opsional):
```json
{ "token": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxa9F3" }
```
200:
```json
{ "provider": "gitlab", "host": "git.noobee.id", "last4": "7Qx2", "updated_at": "2026-10-22T07:10:00Z", "replaced": true }
```
422 TOKEN_INVALID:
```json
{ "error": { "code": "TOKEN_INVALID", "message": "token tidak valid atau tidak punya akses", "details": { "provider": "gitlab", "host": "git.noobee.id" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 HOST_NOT_ALLOWED:
```json
{ "error": { "code": "HOST_NOT_ALLOWED", "message": "host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS", "details": { "host": "git.contoh.com" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
400 (provider/token):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "token": "token wajib 8-512 karakter ASCII tanpa spasi" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 RATE_LIMITED (header `Retry-After: 60` bila diketahui):
```json
{ "error": { "code": "RATE_LIMITED", "message": "provider Git membatasi permintaan; coba lagi nanti", "details": { "retry_after_seconds": 60 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
502 GIT_UPSTREAM_ERROR:
```json
{ "error": { "code": "GIT_UPSTREAM_ERROR", "message": "provider Git tidak dapat dihubungi, coba lagi", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
503 GIT_FEATURE_DISABLED:
```json
{ "error": { "code": "GIT_FEATURE_DISABLED", "message": "sync Git belum dikonfigurasi oleh admin", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 12.3 DELETE `/api/me/git-credentials/{provider}?host=git.noobee.id`

204 tanpa body. 404:
```json
{ "error": { "code": "NOT_FOUND", "message": "token tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 12.4 GET `/api/projects/{projectID}/git-sync?kind=knowledge`

200 (ada config):
```json
{
  "kind": "knowledge",
  "feature_enabled": true,
  "has_credential": true,
  "credential_unreadable": false,
  "config": {
    "provider": "github",
    "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/docs",
    "repo_path": "noobee/grimoire-docs",
    "branch": "main",
    "folder": "docs",
    "target_prefix": "",
    "last_status": "partial",
    "last_synced_at": "2026-10-22T14:05:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": {
      "status": "partial",
      "summary": { "created": 2, "updated": 0, "unchanged": 0, "skipped": 1, "rejected": 1 },
      "items": [
        { "repo_path": "docs/api/rate-limit.md", "target": "api/rate-limit.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
        { "repo_path": "docs/auth/login-flow.md", "target": "auth/login-flow.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
        { "repo_path": "docs/img/diagram.png", "target": "", "status": "skipped", "reason_code": "unsupported_type", "reason": "", "issues": [] },
        { "repo_path": "docs/legacy/../x.md", "target": "", "status": "rejected", "reason_code": "path_traversal", "reason": "", "issues": [] }
      ],
      "truncated": false,
      "error_code": "",
      "error_message": ""
    }
  }
}
```
200 (belum ada config): `{"kind": "api_spec", "feature_enabled": true, "has_credential": false, "credential_unreadable": false, "config": null}`.
400 kind invalid: `{"error": {"code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": {"fields": {"kind": "kind harus knowledge atau api_spec"}}}, "trace_id": "..."}`.
404 non-anggota: `{"error": {"code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null}, "trace_id": "..."}`.

(Catatan contoh: `docs/legacy/../x.md` hanya muncul bila provider tree mengembalikannya. `SelectFiles` tidak menormalisasi path repo; knowledge menolaknya dengan `path_traversal`.)

### 12.5 POST `/api/projects/{projectID}/git-sync/run`

Request knowledge:
```json
{ "kind": "knowledge", "url": "https://github.com/noobee/grimoire-docs/tree/main/docs", "branch": "", "folder": "", "target_prefix": "" }
```
Request api_spec:
```json
{ "kind": "api_spec", "url": "https://github.com/noobee/grimoire-docs", "branch": "main", "folder": "api/openapi" }
```
200 api_spec (partial):
```json
{
  "report": {
    "status": "partial",
    "summary": { "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 1 },
    "items": [
      { "repo_path": "api/openapi/billing.yaml", "target": "billing", "status": "created", "reason_code": "", "reason": "", "issues": [], "openapi_version": "3.0.3", "title": "Billing API", "endpoint_count": 14 },
      { "repo_path": "api/openapi/payments.json", "target": "payments", "status": "rejected", "reason_code": "INVALID_SPEC", "reason": "$ref eksternal tidak didukung",
        "issues": [ { "code": "EXTERNAL_REF", "message": "$ref eksternal tidak didukung", "pointer": "#/paths/~1charges/get", "line": 0 } ],
        "openapi_version": "", "title": "", "endpoint_count": 0 }
    ],
    "truncated": false,
    "error_code": "",
    "error_message": ""
  },
  "config": {
    "provider": "github", "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/api/openapi",
    "repo_path": "noobee/grimoire-docs", "branch": "main", "folder": "api/openapi", "target_prefix": "",
    "last_status": "partial", "last_synced_at": "2026-10-22T14:05:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": null
  }
}
```
(`config.last_result` di respons run selalu `null` karena isinya sama dengan `report`. Field `openapi_version`, `title`, dan `endpoint_count` hanya ada untuk `api_spec`, `omitempty`.)

Error run:
```json
{ "error": { "code": "CREDENTIAL_MISSING", "message": "Anda belum menyimpan token Git untuk github.com", "details": { "provider": "github", "host": "github.com", "settings_path": "/profile#token-git" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "CREDENTIAL_UNREADABLE", "message": "token Git Anda tidak dapat dibaca (kunci server berganti); simpan ulang token", "details": { "provider": "github", "host": "github.com", "settings_path": "/profile#token-git" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "INVALID_REPO_URL", "message": "isi Branch", "details": { "field": "branch" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "SYNC_IN_PROGRESS", "message": "sync lain untuk project dan jenis ini sedang berjalan", "details": { "kind": "knowledge" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "SYNC_LIMIT_EXCEEDED", "message": "folder berisi 742 file, melebihi batas 500 file per sync; persempit folder", "details": { "files": 742, "max_files": 500, "known_bytes": 3145728, "max_total_bytes": 52428800 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "GIT_NOT_FOUND", "message": "repo/branch tidak ditemukan atau token tidak punya akses", "details": { "what": "repo" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "GIT_NOT_FOUND", "message": "folder tidak ditemukan di branch ini", "details": { "what": "folder", "folder": "tidak-ada" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Kode lain mengikuti tabel 6.6 dengan bentuk yang sama: `GIT_TOKEN_REJECTED` ("token Git ditolak provider; simpan ulang token"), `GIT_ACCESS_DENIED` ("token tidak punya akses ke repo ini"), `GIT_TREE_TOO_LARGE` ("repo terlalu besar untuk dibaca sekaligus; persempit folder"), `RATE_LIMITED`, `GIT_UPSTREAM_ERROR`, `TIMEOUT` ("sync melebihi batas waktu"), `HOST_NOT_ALLOWED`, `GIT_FEATURE_DISABLED`.

### 12.6 Bus (bentuk Go, request dikirim sebagai nilai)

```go
// core.gitcredential.get-token
gettoken.Request{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Provider: "github", Host: "github.com"}
gettoken.Response{Found: true, Token: secret.New("ghp_...")}          // sukses
gettoken.Response{Found: false}                                         // belum disimpan
gettoken.Response{Found: true, Unreadable: true}                        // kunci berganti
gettoken.Response{Disabled: true}                                       // GIT_CREDENTIAL_KEY kosong

// project.knowledge.upsert-files
upsertfiles.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001",
	Actor: upsertfiles.Actor{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Role: "developer", Label: "Rina"},
	TargetPrefix: "", Source: upsertfiles.SourceGitSync, Meta: upsertfiles.Meta{IP: "10.1.2.3", UserAgent: "Mozilla/5.0"},
	Items: []upsertfiles.Item{{Index: 0, Path: "api/rate-limit.md", Content: []byte("# Rate limit")}, {Index: 1, Path: "auth/login-flow.md", Content: []byte("# Login")}}}
upsertfiles.Response{Items: []upsertfiles.ItemResult{{Index: 0, Path: "api/rate-limit.md", Status: "created"}, {Index: 1, Path: "auth/login-flow.md", Status: "unchanged"}},
	Summary: upsertfiles.Summary{Created: 1, Unchanged: 1}}
// error: &errs.Error{Code: errs.CodeInvalid, Message: "prefix tujuan tidak valid", Details: {"reason": "path_traversal"}}

// project.apispec.upsert-specs
upsertspecs.Request{ProjectID: "...", Actor: upsertspecs.Actor{...}, Source: upsertspecs.SourceGitSync,
	Items: []upsertspecs.Item{{Index: 0, SourcePath: "api/openapi/billing.yaml", Content: []byte("openapi: 3.0.3 ...")}}}
upsertspecs.Response{Items: []upsertspecs.ItemResult{{Index: 0, SourcePath: "api/openapi/billing.yaml", Name: "billing", Status: "created",
	OpenAPIVersion: "3.0.3", Title: "Billing API", EndpointCount: 14}}, Summary: upsertspecs.Summary{Created: 1}}
```

### 12.7 Audit (`record.Request` ringkas)

```json
{ "action": "git_credential.saved", "actor_type": "user", "target_type": "git_credential", "target": "gitlab:git.noobee.id",
  "args": { "provider": "gitlab", "host": "git.noobee.id", "replaced": true }, "status": "ok" }
```
```json
{ "action": "git_sync.run", "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "target_type": "git_sync", "target": "api_spec",
  "args": { "kind": "api_spec", "provider": "github", "host": "github.com", "repo_path": "noobee/grimoire-docs", "branch": "main", "folder": "api/openapi", "target_prefix": "" },
  "result": { "status": "partial", "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 1, "error_code": "" }, "status": "ok" }
```
Sync gagal: `result.status = "failed"`, `result.error_code = "GIT_NOT_FOUND"`, `status = "error"`.

---

## 13. Changes (per file/folder)

| File / folder | Jenis | Oleh |
|---|---|---|
| `backend/lib/common/secret/{secret.go,secret_test.go}` | baru | foundation |
| `backend/internal/infra/crypto/secretbox/{secretbox.go,secretbox_test.go}` | baru | foundation |
| `backend/internal/infra/safehttp/{client.go,dial.go,redirect.go,read.go,*_test.go}` | baru | foundation |
| `backend/internal/infra/gitprovider/{client.go,github.go,gitlab.go,errors.go,policy.go,*_test.go}` | baru | foundation |
| `backend/internal/infra/middleware/timeout.go` (+ `timeout_test.go`) | ubah: ctx dasar + `ExtendTimeout` | foundation |
| `backend/internal/infra/config/config.go` (+ tes), `docs/configuration.md`, `.env.example` | ubah: 4 env | foundation |
| `backend/lib/constants/bus.go` | ubah: 3 konstanta | foundation |
| `backend/lib/contracts/core/gitcredential/gettoken/gettoken.go` | baru | foundation |
| `backend/lib/contracts/project/knowledge/upsertfiles/upsertfiles.go` | baru | foundation |
| `backend/lib/contracts/project/apispec/upsertspecs/upsertspecs.go` | baru | foundation |
| `backend/lib/contracts/core/audit/record/{actions.go,actions_test.go}` | ubah: 3 aksi | foundation |
| `backend/lib/common/errs/errs.go`, `backend/internal/infra/http/respond/status.go` (+ tes) | ubah: 3 kode bersama | foundation |
| `backend/migrations/core/<VERSION>_create_git_credentials.sql` | baru | foundation |
| `backend/migrations/project/<VERSION>_create_git_sync_configs.sql` | baru | foundation |
| `backend/internal/domain/core/internal/gitcredential/{base.go,base/**}` | baru | foundation |
| `backend/internal/domain/project/internal/gitsync/{base.go,base/**}` | baru | foundation |
| `backend/internal/domain/core/base.go`, `backend/internal/domain/project/base.go` | ubah: Register module baru | foundation |
| `backend/internal/domain/project/internal/knowledge/base/ingest/**` | baru | foundation (D1) |
| `backend/internal/domain/project/internal/knowledge/uploadfiles/usecase.go` | ubah: pakai ingest | foundation (D1) |
| `backend/internal/domain/project/internal/apispec/base/ingest/**` | baru | foundation (D1) |
| `backend/internal/domain/project/internal/apispec/base/audit/audit.go` (+ tes) | ubah: `Event.Actor` opsional | foundation (D1) |
| `backend/internal/domain/project/internal/apispec/uploadspecs/usecase.go` | ubah: pakai ingest | foundation (D1) |
| `api/modules/core/git-credential.yml`, `api/modules/project/git-sync.yml`, `api/root.yml`, `api/openapi.yml` | baru/ubah | foundation |
| `docs/tickets/README.md` | ubah: katalog bus, audit, struktur | foundation |
| `.../gitcredential/savecredential/**` | baru | usecase `savecredential` |
| `.../gitcredential/listcredentials/**` | baru | usecase `listcredentials` |
| `.../gitcredential/deletecredential/**` | baru | usecase `deletecredential` |
| `.../gitcredential/bustoken/**` | baru | usecase `bustoken` |
| `.../knowledge/busupsertfiles/**`, `.../knowledge/base.go`, `.../knowledge/routes_test.go` atau tes registrasi bus | baru/ubah | usecase `busupsertfiles` |
| `.../apispec/busupsertspecs/**`, `.../apispec/base.go`, `.../apispec/routes_test.go` | baru/ubah | usecase `busupsertspecs` |
| `.../gitsync/getconfig/**` | baru | usecase `getconfig` |
| `.../gitsync/runsync/**` | baru | usecase `runsync` |
| `frontend/src/domain/core/git-credential/**`, `frontend/src/domain/project/git-sync/**`, `frontend/src/app/views/ProfileView.svelte`, `frontend/src/app/views/workspace/knowledge/{KnowledgeTab,KnowledgeDialogs}.svelte`, `frontend/src/app/views/workspace/ApiSpecsTab.svelte`, `frontend/src/shared/i18n/{id,en}.json`, `frontend/e2e/git-sync.spec.ts`, `frontend/e2e/fixtures/fake-git-server.ts` | baru/ubah | frontend |
| `app.Deps`, `cmd/**`, module management/apikey/auth/auditlog | TIDAK berubah | - |

---

## 14. Daftar usecase

| Slug | Folder | Ringkas | Kriteria T14 |
|---|---|---|---|
| `savecredential` | `core/internal/gitcredential/savecredential` | PUT simpan/ganti PAT: provider -> host -> policy -> format -> `VerifyToken` -> `Seal` -> upsert (`xmax<>0 AS replaced`) -> audit `git_credential.saved`. BodyLimit 8KiB. | 1, 7 (host), 13 |
| `listcredentials` | `core/internal/gitcredential/listcredentials` | GET status PAT milik sendiri + `enabled` + `allowed_gitlab_hosts`; tidak men-SELECT `token_enc`. | 1, 13 |
| `deletecredential` | `core/internal/gitcredential/deletecredential` | DELETE per `(provider, host)` milik sendiri, tanpa cek policy; 404 bila tidak ada; audit `git_credential.deleted`. | 2, 13 |
| `bustoken` | `core/internal/gitcredential/bustoken` | Handler bus `core.gitcredential.get-token`: Disabled / Found / Unreadable / Token. Tanpa audit, tanpa log token. | 2, 8, 13 |
| `busupsertfiles` | `project/internal/knowledge/busupsertfiles` | Handler bus `project.knowledge.upsert-files`: validasi prefix -> `ClassifyPart` per item -> `ingest.Apply` satu tx -> audit `knowledge.upload.completed` (`source: git_sync`). | 3, 4 |
| `busupsertspecs` | `project/internal/apispec/busupsertspecs` | Handler bus `project.apispec.upsert-specs`: `ingest.ProcessFile` per item (tx per file) -> audit `apispec.uploaded` (`source: git_sync`). | 5 |
| `getconfig` | `project/internal/gitsync/getconfig` | GET config + hasil terakhir per kind + `has_credential`/`credential_unreadable`/`feature_enabled` (get-token fail-soft). | 9, 12 |
| `runsync` | `project/internal/gitsync/runsync` | POST run, alur D5: access -> parse -> token -> lock -> upsert config -> tree/select/limit/fetch -> bus upsert -> report + audit `git_sync.run`. | 3-12, 13, 14 |

Kriteria 14 (tes lengkap, `make api-check`, lint, arch) dipenuhi bersama di langkah 3.
