# T14 core/git-credential — Usecase `savecredential`

Plan usecase. Mengikuti `overview.md` (foundation) di folder yang sama; bila bertentangan, overview yang menang. Semua yang disebut "foundation" (secret, secretbox, safehttp, gitprovider, config, kontrak, migrasi, entity `base/`, kode error bersama, aksi audit, OpenAPI skeleton) dikerjakan di langkah 0 overview dan dianggap sudah ada di branch.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | usecase ini: backend + bagian frontend `savecredential` (fullstack untuk tiket) |
| Domain / Module / Feature | core / git-credential (`gitcredential`) / `savecredential` |
| Resource root | `backend/internal/domain/core/internal/gitcredential` (folder feature: `savecredential/`) |
| Bergantung pada | langkah 0 overview: `lib/common/secret`, `infra/crypto/secretbox`, `infra/gitprovider`, `infra/config` (`GitCredentialKey`, `GitFeatureEnabled()`), migrasi `git_credentials`, `gitcredential/base/*` (provider, host, token, credential, aad, errors, audit), aksi `git_credential.saved`, kode bersama `HOST_NOT_ALLOWED`/`GIT_UPSTREAM_ERROR`/`GIT_FEATURE_DISABLED` |
| Kriteria penerimaan T14 | 1 (simpan, `last4` saja, token invalid tidak tersimpan), 7 (bagian host: self-hosted di allowlist ok, di luar allowlist `HOST_NOT_ALLOWED`), 13 (audit `git_credential.saved` tanpa token; canary PAT tidak di log/audit) |

| Endpoint | Method | Version | Catatan |
|---|---|---|---|
| `/api/me/git-credentials/{provider}` | PUT | `/api` tanpa prefix versi; `api/modules/core/git-credential.yml` `info.version` 0.1.0 | RequireAuth + CSRF (global di `api.Use`), `BodyLimit(8<<10)`, rate limit per user |

Kontrak bus yang dikonsumsi: `core.audit.record` (lewat `gitcredential/base/audit`, `Call`, fail-open). Kontrak bus yang disediakan: tidak ada (bus `core.gitcredential.get-token` milik `bustoken`).

---

## 2. Summary Plan

Satu usecase, satu alur lurus. Tidak dipecah. Urutan wajib (tiap langkah gagal = berhenti, token tidak disimpan):

1. Identity dari `reqctx` (kosong -> 401).
2. Fitur aktif? (`GIT_CREDENTIAL_KEY` kosong -> 503 `GIT_FEATURE_DISABLED`). Dicek setelah identity (401 menang) dan sebelum validasi lain, sehingga klien tahu fitur mati tanpa membocorkan apa pun.
3. `base.ParseProvider(raw)` -> 400.
4. `base.NormalizeHost(provider, in.Host)` -> github dipaksa `github.com`; gitlab default `gitlab.com`; format salah -> 400; github dengan host asing -> 422 `HOST_NOT_ALLOWED`.
5. `HostPolicy.Allowed(provider, host)` -> 422 `HOST_NOT_ALLOWED` (gitlab self-hosted di luar `GIT_SYNC_ALLOWED_HOSTS`).
6. `base.ValidateToken(in.Token)` -> 400 (format 8-512 byte ASCII cetak). Menghasilkan `secret.String` ter-trim.
7. `verifier.VerifyToken` lewat `infra/gitprovider` (safehttp; K11: GitHub `GET api.github.com/user`, GitLab `GET https://{host}/api/v4/user`). Tanpa transaksi DB terbuka selama panggilan jaringan.
8. `sealer.Seal(token, base.AAD(userID, provider, host))`.
9. `repo.Upsert` satu statement `INSERT ... ON CONFLICT (user_id, provider, host) DO UPDATE ... RETURNING created_at, updated_at, (xmax<>0) AS replaced`.
10. Audit `git_credential.saved` (Call, fail-open, setelah upsert), Args `{provider, host, replaced}`.
11. Respons 200 `{provider, host, last4, updated_at, replaced}` dengan `Cache-Control: no-store`.

Pembagian tanggung jawab:

- **Entity (`base/`, sudah foundation):** parse/normalisasi/validasi, `AAD`, `Credential`/DTO, sentinel error. Tidak ada rule baru di usecase.
- **Usecase:** orkestrasi saja. Hasil verifikasi gagal TIDAK dipetakan ke `*errs.Error` di usecase; usecase membungkusnya dalam `*VerifyFailure{Provider, Host, Cause}` (tanpa token, tanpa URL) dan handler yang memetakan (sesuai arahan "pemetaan error di handler"). Error validasi dari `base` sudah `*errs.Error` dan diteruskan apa adanya.
- **Handler:** decode body (params), set header, pemetaan `VerifyFailure` -> `TOKEN_INVALID` 422 / `RATE_LIMITED` 429 (+ `Retry-After`) / `GIT_UPSTREAM_ERROR` 502.
- **Repository:** satu-satunya SQL.

Keputusan lokal usecase ini (tidak bertentangan dengan overview):

- **Rate limit per user** `10/menit`, burst 5 (konstanta di `init.go`, tanpa env baru). Alasan: endpoint ini memicu panggilan keluar ke provider memakai input user; tanpa pembatas, ia menjadi oracle token dan amplifier. Pola sama dengan `changepassword/init.go` (`middleware.NewLimiter` + `KeyByUser`). Respons limiter memakai `RATE_LIMITED` 429 bawaan.
- **Route didaftarkan dengan pola penuh di `d.API`** (`d.API.With(...).Put("/me/git-credentials/{provider}", ...)`), bukan `Route("/me/git-credentials", ...)`. Alasan: empat feature (`save`, `list`, `delete`) berbagi prefix; `chi.Route` dua kali pada prefix yang sama panic karena mount ganda. Pola penuh aman untuk semua feature dan selaras dengan konvensi README. Bila foundation sudah memasang group, `Init` cukup menerima `chi.Router` dan memanggil `.Put("/{provider}", ...)`; sisanya sama.
- **Ctx dibatalkan klien** (`context.Canceled`) tidak dipetakan ke 502; diteruskan ke `respond.WriteError` (perilaku bawaan). `context.DeadlineExceeded` dan semua error jaringan/safehttp dipetakan 502 `GIT_UPSTREAM_ERROR`.
- **Host di pesan error** hanya diulang bila sudah lolos `NormalizeHost` (tidak pernah memuat userinfo), dipotong 253 karakter. Host mentah yang gagal format tidak diulang (bisa memuat `user:secret@`).
- **`replaced`** memakai idiom `xmax <> 0` pada klausa `RETURNING` upsert (true = baris lama ditimpa). Dikunci tes integrasi, termasuk 8 upsert serentak.

---

## 3. Plan (langkah berurutan)

Semua path relatif ke `backend/internal/domain/core/internal/gitcredential/`, kecuali disebut lain.

### 3.1 Prasyarat (verifikasi, jangan dibuat ulang)

Pastikan ada dari foundation: `base/provider.go`, `base/host.go`, `base/token.go`, `base/credential.go`, `base/aad.go`, `base/errors.go`, `base/actor.go`, `base/audit/`, `base/store/`. Dua hal yang mungkin belum ada, tambahkan di `base/` oleh usecase ini bila absen (aditif, satu file masing-masing):

- `base/meta.go`: `type RequestMeta struct{ IP, UserAgent string }`, `func NewRequestMeta(ip, ua string) RequestMeta` (UA dipotong 512 karakter; salin pola `project/internal/apikey/base/meta.go`).
- `base/shared.go`: struct wiring yang dipakai semua `Init` feature module ini:
  ```go
  // Shared dibangun sekali di gitcredential/base.go dan diteruskan ke Init tiap feature.
  type Shared struct {
  	Enabled bool                 // cfg.GitFeatureEnabled()
  	Box     *secretbox.Box       // nil bila !Enabled
  	Git     *gitprovider.Client  // policy + VerifyToken
  	Clock   Clock                // func() time.Time (bila foundation belum punya, tambahkan: type Clock func() time.Time; var SystemClock Clock = time.Now)
  }
  ```
  `base/` mengimpor `infra/*` untuk tipe ini diperbolehkan (precedent `core/internal/auth/base.Shared`). Bila foundation sudah mendefinisikan `Shared` dengan nama lain, pakai itu dan sesuaikan tanda tangan `Init` di bawah.

### 3.2 `savecredential/params.go`

```go
// SaveRequest adalah body PUT /me/git-credentials/{provider}.
// Token bertipe secret.String: String()/Format/slog/json.Marshal selalu "[REDACTED]".
type SaveRequest struct {
	Host  string
	Token secret.String
}

// UnmarshalJSON ada di sini (bukan di lib/common/secret) agar tipe lib tetap tidak bisa dibuat dari JSON.
func (r *SaveRequest) UnmarshalJSON(b []byte) error
```

Aturan `UnmarshalJSON`:
- Decode ke struct bantu `struct{ Host *string \`json:"host"\`; Token *string \`json:"token"\` }` memakai `json.NewDecoder(bytes.NewReader(b))` dengan `DisallowUnknownFields()` (decoder luar `respond.DecodeJSON` tidak menurunkan opsi ini ke `UnmarshalJSON` kustom).
- `Token == nil` (field hilang/null) -> `*errs.Error` `CodeInvalid` details `fields.token = "token wajib diisi"`.
- Semua kegagalan decode (tipe salah, field tidak dikenal, JSON rusak) dikembalikan sebagai `errs.New(errs.CodeInvalid, "permintaan tidak valid")` TANPA membungkus `err` asli (pesan `json` bisa memantulkan potongan input). Detail field tidak dikenal tidak disebut nilainya.
- `r.Token = secret.New(*aux.Token)` segera; string mentah tidak disimpan di variabel yang hidup lebih lama dari fungsi.
- `Host` null/hilang -> `""` (usecase/`NormalizeHost` yang menentukan default).
- Handler juga mengabaikan `r.Body` setelah decode; tidak ada logging body di middleware (diverifikasi tes canary di 5.2).

### 3.3 `savecredential/usecase.go`

```go
// Package savecredential menyimpan atau mengganti PAT milik user login untuk (provider, host).
package savecredential

// VerifyFailure dikembalikan usecase bila provider menolak/gagal diverifikasi. Tidak memuat token atau URL.
type VerifyFailure struct {
	Provider base.Provider
	Host     string
	Cause    error // sentinel gitprovider.* atau *gitprovider.RateLimitedError atau error jaringan (sudah dibersihkan dari URL oleh gitprovider)
}
func (e *VerifyFailure) Error() string { return "savecredential: verify failed" } // konstan, tanpa Cause.Error()
func (e *VerifyFailure) Unwrap() error { return e.Cause }

type Verifier interface {
	VerifyToken(ctx context.Context, p gitprovider.Provider, host string, token secret.String) error
}
type HostPolicy interface {
	Allowed(p gitprovider.Provider, host string) bool
}
type Sealer interface {
	Seal(plain, aad []byte) ([]byte, error)
}
type Repository interface {
	Upsert(ctx context.Context, c base.Credential) (base.Saved, error)
}
type Auditor = audit.Recorder // base/audit: Record(ctx, audit.Event)

type Usecase struct {
	enabled  bool
	policy   HostPolicy
	verifier Verifier
	sealer   Sealer
	repo     Repository
	audit    audit.Recorder
	clock    base.Clock
	log      *slog.Logger
}
func NewUsecase(enabled bool, p HostPolicy, v Verifier, s Sealer, r Repository, a audit.Recorder, c base.Clock, log *slog.Logger) *Usecase

type Result struct {
	Provider  string
	Host      string
	Last4     string
	UpdatedAt time.Time
	Replaced  bool
}

func (u *Usecase) Execute(ctx context.Context, rawProvider string, in SaveRequest, meta base.RequestMeta) (Result, error)
```

`base.Saved` = `struct{ CreatedAt, UpdatedAt time.Time; Replaced bool }` (tambah di `base/credential.go` bila belum ada; tipe nilai murni).

Alur `Execute` (tepat urutan 2.1-2.11). Detail:
- Langkah 1: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
- Langkah 2: `!u.enabled` -> `base.ErrFeatureDisabled`.
- Langkah 3-6: panggil `base` apa adanya, return error mereka tanpa membungkus (sudah `*errs.Error`). Policy: `u.policy.Allowed(gitprovider.Provider(p), host)` false -> `base.HostNotAllowed(host)` (konstruktor di `base/errors.go` yang menyisipkan `details.host`; bila foundation hanya menyediakan sentinel `ErrHostNotAllowed`, tambahkan `HostNotAllowed(host string) *errs.Error` yang meng-clone sentinel dengan details `{host}`).
- Langkah 7: `if err := u.verifier.VerifyToken(ctx, gitprovider.Provider(p), host, token); err != nil { return Result{}, &VerifyFailure{Provider: p, Host: host, Cause: err} }`. Tidak ada `log` di jalur ini selain level Debug tanpa field error mentah: `u.log.Debug("git verify failed", "provider", p, "host", host)` (tanpa `err`, karena cause bisa membawa konteks URL). Hanya menulis log bila `!errors.Is(err, ErrUnauthorized|ErrForbidden)` pada Warn dengan `slog.String("cause_kind", kind)` (kind = `rate_limited|upstream|other`), tidak pernah `err.Error()`.
- Langkah 8: `enc, err := u.sealer.Seal([]byte(token.Reveal()), base.AAD(actor.ID, p, host))`; error -> `fmt.Errorf("savecredential: seal: %w", err)` (secretbox error tidak memuat plaintext). `Reveal()` hanya dipanggil di baris ini dan di `VerifyToken` (di dalam gitprovider). Slice plaintext tidak disimpan di field.
- Langkah 9: `repo.Upsert(ctx, base.Credential{UserID: actor.ID, Provider: p, Host: host, TokenEnc: enc, Last4: token.Last4(), ...})`. Error repo diteruskan (repo sudah memetakan constraint).
- Langkah 10: `u.audit.Record(ctx, audit.Event{Actor: actor, Action: record.ActionGitCredentialSaved, Provider: p, Host: host, Args: map[string]any{"provider": p, "host": host, "replaced": saved.Replaced}, Meta: meta})`. Record fail-open (error ditelan di recorder, dilog tanpa token). Args TIDAK memuat token atau last4.
- Langkah 11: kembalikan `Result{Provider, Host, Last4: token.Last4(), UpdatedAt: saved.UpdatedAt, Replaced: saved.Replaced}`.

### 3.4 `savecredential/repository.go`

```go
type Repo struct{ db *db.DB }
func newRepo(d *db.DB) *Repo
var _ Repository = (*Repo)(nil)

const qUpsert = `
INSERT INTO git_credentials (user_id, provider, host, token_enc, last4)
VALUES ($1::uuid, $2, $3, $4, $5)
ON CONFLICT (user_id, provider, host) DO UPDATE
   SET token_enc = EXCLUDED.token_enc,
       last4     = EXCLUDED.last4,
       updated_at = now()
RETURNING created_at, updated_at, (xmax <> 0) AS replaced`
```

- `Upsert`: `QueryRowxContext(ctx, qUpsert, ...).Scan(&created, &updated, &replaced)`. Tanpa transaksi (satu statement atomik; `BeginTx` tidak perlu).
- `base.ValidUUID(c.UserID)` lebih dulu; tidak valid -> `errs.New(errs.CodeUnauthorized, "perlu login")` (fail-closed, tidak menyentuh DB).
- Pemetaan error lewat `base/store` (`store.Violation(err)` -> code, constraint):
  - check `git_credentials_github_host` / `git_credentials_host_check` / `git_credentials_provider_check` -> `base.HostNotAllowed(c.Host)` (pertahanan berlapis; seharusnya tak tercapai karena entity).
  - FK `git_credentials_user_id_fkey` (user terhapus di tengah request) -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
  - lainnya -> `fmt.Errorf("savecredential: upsert: %w", err)`. Pembungkus tidak menyertakan argumen query (sqlx tidak memuat nilai parameter di pesan pq; tes canary memastikan).
- Tambahkan konstanta nama constraint ke `base/store` bila belum ada: `ConstraintGitHubHost = "git_credentials_github_host"`, `ConstraintUserFK = "git_credentials_user_id_fkey"`.

### 3.5 `savecredential/handler.go`

```go
type Handler struct{ uc *Usecase }
func newHandler(uc *Usecase) *Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Alur:
1. `w.Header().Set("Cache-Control", "no-store"); w.Header().Set("Pragma", "no-cache")` (sebelum apa pun, sehingga error pun tidak ter-cache).
2. `var in SaveRequest; respond.DecodeJSON(r, &in)` -> error `respond.WriteError` (413 dari `BodyLimit` diurus `respond`, pesan generik).
3. `meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())`.
4. `res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "provider"), in, meta)`.
5. `err != nil` -> `respond.WriteError(w, r, mapError(w, err))`.
6. Sukses: `respond.WriteJSON(w, http.StatusOK, newResponse(res))`.

`mapError(w, err) error` (fungsi murni dalam handler.go, mudah dites):

| Kondisi | Hasil |
|---|---|
| `errors.As(err, &vf)` dan `errors.Is(vf.Cause, gitprovider.ErrUnauthorized)` atau `ErrForbidden` | `base.TokenInvalid(vf.Provider, vf.Host)` -> 422 `TOKEN_INVALID`, message "token tidak valid atau tidak punya akses", details `{provider, host}` |
| `errors.As(vf.Cause, &rl *gitprovider.RateLimitedError)` atau `errors.Is(vf.Cause, gitprovider.ErrRateLimited)` | 429 `RATE_LIMITED`, message "provider Git membatasi permintaan; coba lagi nanti", details `{retry_after_seconds: n}` bila `RetryAfter > 0` (dibulatkan ke atas, maksimum 3600); set header `Retry-After: n` |
| `context.Canceled` | teruskan apa adanya |
| Cause lain: `ErrUpstream`, `ErrNotFound`, `ErrHostNotAllowed` dari gitprovider, error safehttp (`ErrBlockedAddress`, `ErrSchemeNotAllowed`, `ErrCrossHostRedirect`), `context.DeadlineExceeded`, error jaringan | `base.ErrUpstream` -> 502 `GIT_UPSTREAM_ERROR`, message "provider Git tidak dapat dihubungi, coba lagi", details null |
| bukan `VerifyFailure` | `err` apa adanya (`*errs.Error` dari base: 400/401/422/503; selain itu 500 generik oleh `respond`) |

Catatan: `ErrNotFound` dari `GET /user` tidak diharapkan; dipetakan ke 502 agar tidak menimbulkan false "token invalid". `base.TokenInvalid(provider, host)` ditambahkan di `base/errors.go` bila foundation hanya punya sentinel `ErrTokenInvalid` (clone dengan details). Pesan dan details tidak pernah memuat token.

`newResponse(res)` -> `struct{ Provider string \`json:"provider"\`; Host string \`json:"host"\`; Last4 string \`json:"last4"\`; UpdatedAt time.Time \`json:"updated_at"\`; Replaced bool \`json:"replaced"\` }` (didefinisikan di `params.go`, bersebelahan dengan `SaveRequest`; tidak ada field token).

### 3.6 `savecredential/init.go`

```go
// Init memasang PUT /me/git-credentials/{provider} pada d.API.
func Init(d app.Deps, s base.Shared) {
	uc := NewUsecase(
		s.Enabled,
		s.Git.Policy(),
		s.Git,
		sealerOf(s), // s.Box; nil-safe: bila !s.Enabled kembalikan sealer yang selalu error (tak pernah dipanggil)
		newRepo(d.DB),
		audit.New(d.Bus, d.Log),
		s.Clock,
		d.Log,
	)
	lim := middleware.NewLimiter(d.Ctx, middleware.Rule{
		Name: "gitcredential.save", Rate: rate.Limit(float64(saveRatePerMinute) / 60),
		Burst: saveBurst, KeyFunc: middleware.KeyByUser,
	})
	d.API.With(
		middleware.RequireAuth(),
		middleware.BodyLimit(8<<10),
		middleware.RateLimit(lim),
	).Put("/me/git-credentials/{provider}", newHandler(uc).ServeHTTP)
}

const (saveRatePerMinute = 10; saveBurst = 5)
```

Hindari jebakan nil-interface: `s.Box` bertipe `*secretbox.Box`; jangan menyimpan pointer nil ke interface `Sealer` lalu memanggilnya. `sealerOf` mengembalikan `nil` interface bila `s.Box == nil`; usecase sudah menolak di langkah 2 sebelum menyentuh sealer, dan konstruktor menegakkan `enabled => sealer != nil` (panic saat wiring, rule 5.9).

### 3.7 Wiring module (satu baris)

`gitcredential/base.go` `Register`: tambah `savecredential.Init(d, shared)` (dan hanya itu; `listcredentials`/`deletecredential`/`bustoken` menambah barisnya sendiri). Tambah satu baris di `gitcredential/routes_test.go` (lihat 5.4).

### 3.8 OpenAPI

Foundation sudah membuat skeleton `api/modules/core/git-credential.yml`. Usecase ini merapikan bagian `PUT /me/git-credentials/{provider}`:
- `operationId: saveGitCredential`, parameter path `provider` enum `[github, gitlab]`.
- Request schema `SaveGitCredentialRequest` (`token` required, `host` optional; contoh gitlab self-hosted dan github), `writeOnly: true` pada `token`, `format: password`.
- Respons: `200 SaveGitCredentialResponse`, `400`, `401`, `403` (CSRF), `413`, `422` (dua contoh: `TOKEN_INVALID`, `HOST_NOT_ALLOWED`), `429` (header `Retry-After`), `502`, `503` — semua `Error` dari `foundation.yml`.
- Jalankan `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau.

### 3.9 Frontend (bagian `savecredential`)

Struktur baru `frontend/src/domain/core/git-credential/savecredential/`:

```
savecredential/
├── index.ts                      # ekspor: SaveTokenDialog, type SavedCredential
├── types.ts                      # Zod + validator
├── types.test.ts
├── services/savecredential.service.ts (+ .test.ts)
├── components/SaveTokenForm.svelte (+ .test.ts)
└── views/SaveTokenDialog.svelte (+ .test.ts)
```

- `types.ts`:
  - `GitProviderSchema = z.enum(['github','gitlab'])`.
  - `SavedCredentialSchema = z.object({ provider: GitProviderSchema, host: z.string(), last4: z.string(), updated_at: z.string(), replaced: z.boolean() })`; ekspor `type SavedCredential` (camelCase di sisi UI: `updatedAt`) lewat transform.
  - `validateToken(raw): { error?: 'length' | 'charset' }` cermin entity backend (trim; 8-512; ASCII 0x21..0x7E). Hanya UX; backend otoritatif.
  - `validateHost(provider, raw)` cermin ringan (gitlab: tidak kosong bila dipilih "lainnya", tanpa spasi/`/`/`@`).
- `services/savecredential.service.ts`:
  ```ts
  export function saveCredential(provider: GitProvider, body: { host?: string; token: string }): Promise<SavedCredential> {
    return put(`/api/me/git-credentials/${encodeURIComponent(provider)}`, body, { schema: SavedCredentialSchema });
  }
  ```
  Tanpa AbortSignal (pola `createKey`). Service tidak me-log body. Header CSRF diurus `shared/services/http`.
- `components/SaveTokenForm.svelte`: field Provider (`Select`; GitHub/GitLab; terkunci bila prop `provider` diberikan), Host (untuk GitLab: `Select` dari prop `gitlabHosts: string[]` (`allowed_gitlab_hosts` dari listcredentials; fallback `['gitlab.com']`), tersembunyi untuk GitHub), Token (`Input type="password" autocomplete="off" name="git-token"`, tanpa `value` yang di-persist), teks scope mono, catatan "Token tidak bisa dilihat lagi setelah disimpan."
- `views/SaveTokenDialog.svelte` props: `open`, `provider?`, `host?`, `gitlabHosts`, `onclose`, `onsaved(cred: SavedCredential)`. Perilaku:
  - Token hanya di `$state` lokal; `reset()` dipanggil di `close()` dan setelah sukses (kosongkan, `step` kembali). Tidak ke store, tidak ke `localStorage`, tidak ke URL.
  - Submit: validasi klien -> `saveCredential` -> `onsaved` -> tutup + toast "Token disimpan".
  - Pemetaan error (`isApiError`, `err.code`): `TOKEN_INVALID` -> galat di field Token ("Token tidak valid atau tidak punya akses"); `HOST_NOT_ALLOWED` -> galat di field Host; `GIT_FEATURE_DISABLED` -> galat form + dialog nonaktif; `RATE_LIMITED` -> galat form dengan `retry_after_seconds` bila ada; `GIT_UPSTREAM_ERROR` -> galat form "Provider Git tidak dapat dihubungi, coba lagi" + trace id; `INVALID_ARGUMENT` -> `fieldErrorsOf` ke field; lainnya -> galat umum + trace id. Setiap galat: nilai token di input TIDAK dihapus (user bisa memperbaiki), kecuali setelah sukses.
  - Tombol submit menampilkan spinner dan teks "Memverifikasi ke GitHub…" (nama provider) selama request; double-submit dicegah (`submitting`).
- Komposisi di `frontend/src/app/views/ProfileView.svelte` (lintas module diizinkan di `app/`):
  - Tambah seksi `<section id="token-git" data-testid="git-token-section">` di bawah `ChangePasswordView`, lebar halaman dinaikkan `max-w-sm` -> `max-w-2xl` (keputusan overview; bila `listcredentials` sudah merge, seksi itu sudah ada: cukup tambahkan `SaveTokenDialog` dan hubungkan tombol "Simpan/Ganti" ke dialog, `onsaved` memuat ulang daftar).
  - Bila seksi belum ada (usecase ini selesai lebih dulu), pasang kerangka minimal: heading i18n `gitCredential.title`, dua baris (GitHub, GitLab) masing-masing tombol ghost "Simpan token" yang membuka `SaveTokenDialog`. `listcredentials` kemudian mengganti isi baris dengan tabel status. Keduanya hanya menyentuh `ProfileView.svelte` pada blok seksi ini (aturan satu-blok agar tidak konflik).
  - Tidak mengimpor internals `savecredential`; hanya lewat `index.ts`.
- i18n `frontend/src/shared/i18n/{id,en}.json` (kunci `gitCredential.save.*`):
  `title`, `titleReplace`, `provider`, `host`, `hostOther`, `token`, `tokenPlaceholder`, `scopeGithub` ("repo (read-only)"), `scopeGitlab` ("read_api, read_repository"), `scopeLabel`, `noReveal` ("Token tidak bisa dilihat lagi setelah disimpan."), `submit`, `verifying` ("Memverifikasi ke {provider}…"), `saved`, `validation.length`, `validation.charset`, `validation.host`, `error.TOKEN_INVALID`, `error.HOST_NOT_ALLOWED`, `error.GIT_FEATURE_DISABLED`, `error.RATE_LIMITED`, `error.RATE_LIMITED_RETRY` ("coba lagi dalam {seconds} detik"), `error.GIT_UPSTREAM_ERROR`, `error.generic`. Isi id dan en lengkap; tes i18n paritas kunci yang ada harus hijau.

### 3.10 Visual (DESIGN.md, framed-sheet)

- **Pertanyaan layar:** "Token apa yang saya masukkan, untuk host mana, dan akankah ia tersimpan aman?" Satu aksi utama: Simpan.
- **Pola DESIGN.md:** dialog `shadow-pop`, radius 16, lebar `md`; label di atas input, tinggi kontrol 32, tombol utama `accent` hanya satu; cincin fokus `accent-ink`.
- **Elemen khas:** blok scope bergaya mono (`font-mono text-[13px] text-muted`, latar `bg-surface-2`, radius 6) berisi `repo` (GitHub) atau `read_api` + `read_repository` (GitLab); nilai berubah mengikuti provider terpilih. Catatan "Token tidak bisa dilihat lagi" berwarna `text-muted`, 13px, dengan ikon kunci outline 16px.
- **Token warna/ruang:** `bg-surface`, `border-border`, `text-muted`, `text-negative` untuk galat field, `pill-*` tidak dipakai di dialog ini. Input token `type=password` dengan font mono saat berisi.
- **Data contoh:** GitHub `ghp_…a9F3`, GitLab host `git.noobee.id`, `last4` `7Qx2`; galat "Token tidak valid atau tidak punya akses".
- **State:** idle, mengetik, memverifikasi (tombol disabled + spinner + teks "Memverifikasi ke GitHub…"), galat per field, fitur nonaktif (formulir disabled + keterangan "Sync Git belum dikonfigurasi oleh admin"), sukses (dialog tertutup + toast).
- Aksesibilitas: label terhubung, `aria-invalid` + `aria-describedby` pada galat, fokus awal ke Token, `Esc` menutup (dismissible kecuali saat `submitting`), pesan galat `role="alert"`.

---

## 4. Migration

Tidak ada di usecase ini. Tabel `git_credentials` dibuat di foundation: `backend/migrations/core/<VERSION>_create_git_credentials.sql` (overview bagian 7.1; versi 17 digit di-generate saat implement, sesuai `.claude/rules/migrations.md`). Usecase mengandalkan: unique `git_credentials_user_provider_host_key (user_id, provider, host)`, CHECK `git_credentials_github_host`, `git_credentials_token_enc_len (octet_length > 28)`, `git_credentials_last4_len (1..4)`, FK `user_id -> users(id) ON DELETE CASCADE`. Tes integrasi tidak boleh mengacu nama file migrasi; cukup `db.Up(...)`.

Catatan: `token_enc` hasil `Seal` = 12 (nonce) + len(token >= 8) + 16 (tag) >= 36 byte, selalu lolos CHECK > 28.

---

## 5. Test Scenario

Semua tes unit tanpa jaringan. Konstanta canary: `const canary = "ghp_CANARY0123456789abcdefCANARY"`. Gunakan `registerCodes()` module (sync.Once) di handler test, bukan `RegisterCodeStatus` langsung.

### 5.1 `usecase_test.go` (fake repo, fake verifier, fake sealer/real `secretbox`, fake audit, fake policy)

Helper: identitas lewat `reqctx.WithIdentity`, `fakeVerifier{err error; calls []call}`, `fakeRepo{saved base.Saved; err error; got []base.Credential}`, `fakeAudit{events []audit.Event}`, `fakePolicy{allow map[string]bool}`. `box` asli `secretbox.New(key32)` untuk tes round-trip.

| Tes | Setup | Harapan |
|---|---|---|
| `TestExecute_GitHubValid` | provider `github`, host kosong, token canary | verifier dipanggil sekali dengan `(github, "github.com", canary)`; repo menerima `Credential{UserID=actor.ID, Provider=github, Host=github.com, Last4="NARY"}`; `TokenEnc` != plaintext dan `box.Open(TokenEnc, AAD(userID,"github","github.com"))` == canary; audit sekali; hasil `Replaced=false` |
| `TestExecute_ProviderCaseInsensitive` | `GitHub` | diterima, provider tersimpan `github` |
| `TestExecute_GitHubHostForced` | provider github, host `api.github.com` dan `""` | host dipakai `github.com`; host `evil.com` -> `HOST_NOT_ALLOWED`, verifier tidak dipanggil, repo tidak dipanggil |
| `TestExecute_GitLabDefaultHost` | gitlab, host `""` | host `gitlab.com`; policy menerima |
| `TestExecute_GitLabSelfHostedAllowed` | gitlab, host `HTTPS://Git.Noobee.ID/`, policy allow `git.noobee.id` | verifier dipanggil dengan `git.noobee.id`; tersimpan host itu |
| `TestExecute_GitLabSelfHostedNotAllowed` | gitlab, `git.contoh.com`, policy tidak allow | 422 `HOST_NOT_ALLOWED`, `details.host == "git.contoh.com"`; verifier/repo/audit tidak dipanggil |
| `TestExecute_GitLabHostWithPortAllowed` | `git.noobee.id:8443` di allow | sukses; AAD memuat host dengan port |
| `TestExecute_HostWithUserinfoInvalid` | gitlab, host `tok:secret@git.noobee.id` | 400 `INVALID_ARGUMENT`; pesan/details TIDAK memuat `secret`; verifier tidak dipanggil |
| `TestExecute_InvalidProvider` | `bitbucket`, `""` | 400, `fields.provider`; tidak ada panggilan lain |
| `TestExecute_InvalidToken` | token 7 char, 513 char, berspasi di tengah, non-ASCII, kosong | 400 `fields.token`; verifier tidak dipanggil; pesan error tidak memuat token yang diberikan |
| `TestExecute_TokenTrimmed` | `"  <canary>\n"` | verifier menerima canary tanpa whitespace |
| `TestExecute_NoIdentity` | tanpa identity | 401; tidak ada panggilan lain |
| `TestExecute_FeatureDisabled` | `enabled=false` | 503 `GIT_FEATURE_DISABLED`; tidak ada panggilan lain; dengan identity kosong tetap 401 (identity dicek lebih dulu) |
| `TestExecute_VerifyUnauthorized` | verifier -> `gitprovider.ErrUnauthorized` | error `*VerifyFailure`, `errors.Is(err, gitprovider.ErrUnauthorized)`; repo TIDAK dipanggil; audit TIDAK dipanggil; sealer tidak dipanggil |
| `TestExecute_VerifyForbidden` | -> `ErrForbidden` | idem |
| `TestExecute_VerifyRateLimited` | -> `&gitprovider.RateLimitedError{RetryAfter: 60*time.Second}` | `*VerifyFailure`, `errors.As` ke `RateLimitedError` memberi 60s; tidak ada simpan |
| `TestExecute_VerifyUpstream5xx` | -> `ErrUpstream`, `errors.New("dial tcp: i/o timeout")`, `context.DeadlineExceeded`, `safehttp.ErrBlockedAddress` | semuanya `*VerifyFailure`; tidak ada simpan |
| `TestExecute_VerifyFailureErrorString` | cause berisi teks `canary` | `err.Error()` konstan, tidak memuat canary |
| `TestExecute_SealFails` | sealer error | error ter-wrap, repo tidak dipanggil, audit tidak dipanggil, pesan tidak memuat canary |
| `TestExecute_RepoError` | repo error | diteruskan; audit tidak dipanggil |
| `TestExecute_Replaced` | repo -> `Replaced=true` | `Result.Replaced=true`; Args audit `replaced=true` |
| `TestExecute_AuditEvent` | sukses | event: `Action=git_credential.saved`, `Actor.ID` = user, `Args` persis `{provider, host, replaced}` (tidak ada kunci lain, tidak ada `token`/`last4`), `Meta` diteruskan |
| `TestExecute_AuditFailOpen` | audit recorder ber-error/panic-free (implementasi `BusRecorder` dengan bus tanpa handler) | `Execute` tetap sukses |
| `TestExecute_OrderOfChecks` | semua input salah (provider, host, token) | provider menang atas host, host atas token (urutan berjalan) |
| `TestExecute_PATCanary` | logger JSON level Debug ke buffer, token canary: jalankan sukses, 401, 429, 5xx | buffer log tidak berisi `canary` (dan bagian 12+ karakternya); setiap `audit.Event`/`record.Request` hasil `fmt.Sprintf("%+v")` tidak berisi canary; `Result` `%+v` tidak berisi canary; `err.Error()` semua kasus gagal tidak berisi canary |

### 5.2 `handler_test.go` (httptest + chi + usecase dengan fake)

Router: `chi` dengan `middleware.RequireAuth()` stub yang mengisi identity (atau handler dipanggil langsung dengan `reqctx`), `registerCodes()`.

| Tes | Skenario | Harapan |
|---|---|---|
| `TestHandler_OK_GitHub` | PUT `/me/git-credentials/github` body `{"token":"<canary>"}` | 200, JSON `{provider, host:"github.com", last4:"NARY", updated_at, replaced:false}`; header `Cache-Control: no-store`; body tidak memuat canary; key respons persis lima field (decode ke `map[string]any`, `len==5`) |
| `TestHandler_OK_GitLabReplaced` | body `{"host":"git.noobee.id","token":"glpat-..."}` repo `Replaced=true` | 200, `replaced:true`, host sesuai |
| `TestHandler_TokenMissing` | body `{}` dan `{"token":null}` | 400 `INVALID_ARGUMENT`, `details.fields.token` |
| `TestHandler_TokenWrongType` | `{"token":123456789012}` | 400; body respons tidak memuat `123456789012` |
| `TestHandler_UnknownField` | `{"token":"<canary>","extra":1}` | 400; respons tidak memuat canary |
| `TestHandler_BadJSON` | `{"token":"<canary>"` (terpotong) dan bukan JSON | 400 `JSON tidak valid`/`permintaan tidak valid`; tidak memuat canary |
| `TestHandler_TrailingData` | dua objek JSON | 400 |
| `TestHandler_BodyTooLarge` | body 9 KiB lewat `BodyLimit(8<<10)` | 413 `PAYLOAD_TOO_LARGE`; tanpa canary |
| `TestHandler_ErrorMapping` | tabel: usecase mengembalikan `VerifyFailure{ErrUnauthorized}` -> 422 `TOKEN_INVALID` (message "token tidak valid atau tidak punya akses", `details {provider, host}`); `ErrForbidden` -> 422 `TOKEN_INVALID`; `RateLimitedError{60s}` -> 429 `RATE_LIMITED`, header `Retry-After: 60`, `details.retry_after_seconds=60`; `RateLimitedError{0}` -> 429 tanpa header `Retry-After`, `details` null; `ErrUpstream` / `errors.New("tcp reset")` / `DeadlineExceeded` / `safehttp.ErrBlockedAddress` / `ErrNotFound` -> 502 `GIT_UPSTREAM_ERROR`; `base.ErrFeatureDisabled` -> 503 `GIT_FEATURE_DISABLED`; `HostNotAllowed("x.com")` -> 422 `HOST_NOT_ALLOWED` details host; no identity -> 401 | status/kode/pesan sesuai; semua body error mengikuti `{"error":{code,message,details},"trace_id"}`; tidak ada yang memuat canary |
| `TestHandler_RetryAfterClamped` | `RetryAfter=90 menit` | header `3600` |
| `TestHandler_ProviderParam` | path `/me/git-credentials/Bitbucket` | 400 `fields.provider` |
| `TestHandler_NoCacheOnError` | semua kasus error | header `Cache-Control: no-store` ada |
| `TestHandler_CanceledContext` | ctx dibatalkan, fake verifier mengembalikan `context.Canceled` | tidak menghasilkan body `GIT_UPSTREAM_ERROR` |
| `TestSaveRequest_UnmarshalRedacted` | `fmt.Sprintf("%v %+v %#v %q", req)`, `slog` JSON, `json.Marshal(req)` setelah decode | tidak memuat canary |

### 5.3 `repository_integration_test.go` (tag `integration`, Postgres nyata via `db.TestDB(t)` + `db.Up`)

Helper: buat user nyata (FK) memakai helper pembuat user yang dipakai tes integrasi core lain (cari: `grep -rn "func .*[Ss]eedUser\|func .*insertUser\|func .*newUser" backend/internal/domain/core/`; bila tidak ada helper yang dapat dipakai lintas paket, tulis INSERT minimal ke `users` di file tes ini). Konstanta `userA`, `userB`.

| Tes | Skenario | Harapan |
|---|---|---|
| `TestRepo_Upsert_Insert` | upsert pertama | `replaced=false`, `created_at == updated_at` atau selisih < 1 detik, satu baris |
| `TestRepo_Upsert_TwiceOneRow` | upsert dua kali `(A, github, github.com)` dengan `token_enc` berbeda | `SELECT count(*)` = 1; `replaced=true` pada kali kedua; `token_enc` baris = blob kedua; `last4` = nilai kedua; `created_at` tidak berubah; `updated_at` kedua `>=` pertama |
| `TestRepo_Upsert_DifferentHostSeparateRow` | A gitlab `gitlab.com` lalu A gitlab `git.noobee.id` | dua baris, keduanya `replaced=false` |
| `TestRepo_Upsert_DifferentUserSeparateRow` | A dan B github | dua baris; upsert A tidak mengubah baris B |
| `TestRepo_Upsert_GitHubHostCheck` | provider github host `evil.com` | error dipetakan ke `HOST_NOT_ALLOWED` (422); tidak ada baris |
| `TestRepo_Upsert_ProviderCheck` | provider `bitbucket` | error (CHECK), tidak ada baris |
| `TestRepo_Upsert_HostLowercaseCheck` | host `Git.Noobee.ID` | error CHECK, tidak ada baris |
| `TestRepo_Upsert_UnknownUserFK` | `user_id` UUID tak terdaftar | error dipetakan ke 401 `UNAUTHORIZED` |
| `TestRepo_Upsert_InvalidUUID` | `user_id = "abc"` | 401, tanpa menyentuh DB |
| `TestRepo_Upsert_TokenEncTooShort` | `token_enc` 10 byte | error CHECK `token_enc_len` (bukan panic) |
| `TestRepo_Upsert_Concurrent` | 8 goroutine upsert key yang sama | tepat satu baris; tepat satu `replaced=false`, tujuh `replaced=true`; tanpa error |
| `TestUsecase_Integration_RoundTrip` | usecase asli + `secretbox` asli + repo asli + fake verifier sukses + fake audit | baris DB: `token_enc` tidak mengandung bytes canary (`bytes.Contains` false), `last4` = 4 karakter terakhir; `box.Open(token_enc, AAD(user, provider, host))` == canary; `Open` dengan AAD user lain / host lain -> `ErrDecrypt`; seluruh baris `git_credentials` di-`row_to_json` tidak memuat canary |
| `TestUsecase_Integration_VerifyFailureNoRow` | verifier 401 / 429 / 502 | tabel tetap kosong (token tidak disimpan) |
| `TestUsecase_Integration_ReplaceKeepsOneRow` | simpan token1 lalu token2 untuk key sama | satu baris; `Open` mengembalikan token2; `replaced=true` |

### 5.4 `routes_test.go` / HTTP-level (tingkat module `gitcredential`)

Satu baris tambahan pada tes registrasi module (`gitcredential/routes_test.go` dari foundation; bila belum ada buat file kecil):
- `PUT /api/me/git-credentials/github` terdaftar; tanpa login -> 401; tanpa header CSRF -> 403 `CSRF_REQUIRED`; `GET`/`POST`/`DELETE` ke path yang sama dengan metode yang salah tidak jatuh ke handler ini (405/404 sesuai router).
- Dengan `Config.GitCredentialKey` kosong: `PUT` terautentikasi -> 503 `GIT_FEATURE_DISABLED`.
- Rate limit: 6 request cepat dari user sama (burst 5) -> request ke-6 429 `RATE_LIMITED`; user lain tidak terpengaruh.
- Body 9 KiB -> 413.

### 5.5 Tes PAT canary end-to-end (HTTP in-process, bagian dari integrasi usecase ini)

`savecredential/canary_integration_test.go` (tag `integration`): router asli + DB nyata + `gitprovider.Client` asli diarahkan ke `httptest.Server` palsu memakai `InsecureAllowHosts`/`GITHUB_API_BASE` test (seam foundation) + logger slog JSON level Debug ke buffer + `core.audit.record` handler asli (tulis ke tabel `audit_log` bila module auditlog dipasang di harness; bila tidak, fake handler yang menyimpan `record.Request`).

Skenario: PUT sukses (server palsu menjawab 200 `/user` hanya bila header `Authorization: Bearer <canary>`), PUT dengan server 401, 429, 500, dan PUT kedua (replace). Assert canary TIDAK ada di: buffer log, setiap body respons (dan header respons), setiap `record.Request` / baris `audit_log` (semua kolom, `args::text`), baris `git_credentials` (semua kolom termasuk `token_enc` sebagai bytea), dan URL/query yang tercatat server palsu (token hanya di header). Assert server palsu menerima header auth benar tepat sekali per PUT dan `User-Agent: grimoire-git-sync`.

### 5.6 Frontend unit (vitest + testing-library)

| Berkas | Kasus |
|---|---|
| `types.test.ts` | `validateToken`: 7 char -> `length`; 8 char ok; 512 ok; 513 `length`; spasi tengah `charset`; non-ASCII `charset`; trim; `validateHost` gitlab kosong/`a b`/`a/b`/`u@h` galat; schema respons: field hilang/tipe salah ditolak; field `token` pada respons diabaikan/ditolak (tidak diekspos ke tipe `SavedCredential`) |
| `services/savecredential.service.test.ts` | memanggil `PUT /api/me/git-credentials/github` dengan body persis `{token}` (GitHub tanpa host) dan `{host, token}` (GitLab); header CSRF terkirim; respons di-parse ke camelCase; `provider` di-encode; respons tidak valid -> error; error 422 `TOKEN_INVALID` diteruskan sebagai `ApiError` dengan `code` |
| `components/SaveTokenForm.test.ts` | input token `type="password"`, `autocomplete="off"`; GitHub menyembunyikan field Host; GitLab menampilkan Select host dari prop; teks scope berganti per provider; galat field ditampilkan dengan `aria-invalid` |
| `views/SaveTokenDialog.test.ts` | sukses: memanggil service, `onsaved` dengan data, dialog tertutup, input token dikosongkan; token tidak muncul di DOM selain nilai input (tidak ada teks token di `document.body.textContent`) dan tidak di `localStorage`/`sessionStorage`; `TOKEN_INVALID` -> pesan di field Token, nilai input dipertahankan; `HOST_NOT_ALLOWED` -> galat di Host; `RATE_LIMITED` dengan `retry_after_seconds` menampilkan hitung mundur teks; `GIT_UPSTREAM_ERROR` menampilkan trace id; `GIT_FEATURE_DISABLED` men-disable form; submit ganda hanya satu request; validasi klien mencegah request; menutup lalu membuka lagi memberi form kosong |
| `app/views/ProfileView.test.ts` (ubah) | seksi `#token-git` ada; tombol membuka dialog; `max-w-2xl` terpasang; tes lama (profil, ganti password) tetap hijau |
| i18n paritas | semua kunci `gitCredential.save.*` ada di `id.json` dan `en.json` |

Arsitektur FE: `npm run depcruise` dan `npm run test:arch` hijau (tidak ada import silang internals; `app/` boleh mengomposisi).

---

## 6. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`, bagian yang diwakili usecase ini; fixture `fake-git-server.ts` dari overview bagian 11)

Fixture: server palsu menjawab `GET /user` (GitHub) 200 hanya untuk token sah `ghp_e2eValidToken7Qx2`, 401 untuk lainnya; mode kontrol untuk 429 dan 500; backend e2e berjalan dengan `GIT_CREDENTIAL_KEY`, `GIT_SYNC_INSECURE_TEST_HOSTS=127.0.0.1:<port>`, `GIT_SYNC_GITHUB_API_BASE=http://127.0.0.1:<port>`, `GIT_SYNC_ALLOWED_HOSTS` memuat host GitLab palsu pada port fixture kedua.

Positif:
1. Buka `/profile`, seksi Token Git terlihat. Klik "Simpan token" GitHub: dialog berisi Provider GitHub (terkunci), teks scope `repo`. Isi token sah, submit: tombol "Memverifikasi ke GitHub…" lalu dialog tertutup, toast "Token disimpan". Pil status "Tersimpan" dan `…7Qx2` (bila `listcredentials` sudah merge; jika belum, cukup toast dan respons 200 diverifikasi lewat `page.waitForResponse`). `page.on('response')`: tidak ada respons yang mengandung `ghp_e2eValidToken7Qx2`; `localStorage`/`sessionStorage`/`document.body.innerText` tidak memuatnya.
2. Simpan ulang token (ganti) dengan token sah lain (fixture menerima daftar token): respons `replaced:true`, `last4` baru.
3. GitLab self-hosted: pilih GitLab, Host dari daftar `allowed_gitlab_hosts` (host palsu), token sah (fixture GitLab `GET /api/v4/user` dengan `PRIVATE-TOKEN`) -> tersimpan (kriteria 7 bagian allowlist).
4. Keyboard: Tab melewati field, Enter di field Token men-submit, Esc menutup dialog dan mengosongkan input.

Negatif:
1. Token tidak sah (fixture 401): pesan "Token tidak valid atau tidak punya akses" di field Token, status tetap "Belum ada" / tidak ada baris tersimpan (`GET /api/me/git-credentials` tidak memuat item) — kriteria 1.
2. Token terlalu pendek (7 karakter): galat klien, tidak ada request PUT (`page.route` menghitung 0).
3. Host GitLab di luar allowlist: panggilan API langsung `PUT /api/me/git-credentials/gitlab` `{host:"git.tidak-terdaftar.id", token}` -> 422 `HOST_NOT_ALLOWED`; fixture tidak menerima koneksi (tidak ada request ke host itu).
4. GitHub dengan host asing lewat API langsung -> 422 `HOST_NOT_ALLOWED`.
5. Fixture mode rate limit (403 + `x-ratelimit-remaining: 0`) -> pesan "Provider Git membatasi permintaan; coba lagi nanti"; tidak tersimpan.
6. Fixture mode 500 / koneksi diputus -> pesan "Provider Git tidak dapat dihubungi, coba lagi" + trace id; tidak tersimpan.
7. Tanpa login: `PUT` -> 401. Tanpa header CSRF -> 403 `CSRF_REQUIRED`.
8. Backend dijalankan tanpa `GIT_CREDENTIAL_KEY` (proyek Playwright terpisah atau env override): dialog menampilkan keterangan fitur nonaktif, form disabled; API -> 503 `GIT_FEATURE_DISABLED`.
9. Isolasi antar user: user B menyimpan token, user A tidak melihat/ter-overwrite (`GET` A tetap berisi baris A saja).

---

## 7. Request/Response body

Semua mutasi mengirim header CSRF `X-Requested-With: grimoire` (konstanta `CSRF_VALUE` di `shared/services/http.ts`) dan cookie sesi. JSON snake_case. `trace_id` contoh: `"4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"`. Semua respons menyertakan `Cache-Control: no-store`.

### 7.1 Request

`PUT /api/me/git-credentials/gitlab`
```json
{ "host": "git.noobee.id", "token": "glpat-xxxxxxxxxxxxxxxx7Qx2" }
```

`PUT /api/me/git-credentials/github` (host opsional dan diabaikan bila kosong/`github.com`/`api.github.com`)
```json
{ "token": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxa9F3" }
```

`PUT /api/me/git-credentials/gitlab` (host default `gitlab.com`)
```json
{ "token": "glpat-yyyyyyyyyyyyyyyyyyyy" }
```

### 7.2 Respons sukses

200 (`replaced:false` bila baru; `true` bila token lama ditimpa):
```json
{
  "provider": "gitlab",
  "host": "git.noobee.id",
  "last4": "7Qx2",
  "updated_at": "2026-10-22T07:10:00Z",
  "replaced": true
}
```
Tidak ada field `token`, `token_enc`, `created_at`, atau `id`.

### 7.3 Respons error

Format: `{"error": {"code", "message", "details"}, "trace_id"}`.

401 tanpa login:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

400 provider tidak dikenal:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "provider": "provider harus github atau gitlab" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

400 token tidak valid secara format (atau hilang):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "token": "token wajib 8-512 karakter ASCII tanpa spasi" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

400 host berformat salah (nilai host tidak diulang):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "host": "host tidak valid" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

400 body rusak / field tidak dikenal / tipe salah:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

403 tanpa header CSRF:
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header CSRF wajib ada", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(teks pesan mengikuti implementasi middleware CSRF yang ada)

413 body > 8 KiB:
```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "body terlalu besar", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(teks pesan mengikuti `respond`)

422 `TOKEN_INVALID` (provider 401/403 pada `GET /user`):
```json
{ "error": { "code": "TOKEN_INVALID", "message": "token tidak valid atau tidak punya akses", "details": { "provider": "gitlab", "host": "git.noobee.id" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

422 `HOST_NOT_ALLOWED` (GitLab di luar `GIT_SYNC_ALLOWED_HOSTS`, atau GitHub dengan host selain `github.com`):
```json
{ "error": { "code": "HOST_NOT_ALLOWED", "message": "host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS", "details": { "host": "git.contoh.com" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

429 `RATE_LIMITED` dari provider (header `Retry-After: 60` bila provider memberi tahu; tanpa header dan `details: null` bila tidak):
```json
{ "error": { "code": "RATE_LIMITED", "message": "provider Git membatasi permintaan; coba lagi nanti", "details": { "retry_after_seconds": 60 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 dari limiter per user (bawaan middleware) memakai bentuk dan kode sama dengan pesan bawaan limiter.

502 `GIT_UPSTREAM_ERROR` (5xx provider, jaringan, DNS/dial diblokir safehttp, timeout):
```json
{ "error": { "code": "GIT_UPSTREAM_ERROR", "message": "provider Git tidak dapat dihubungi, coba lagi", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

503 `GIT_FEATURE_DISABLED` (`GIT_CREDENTIAL_KEY` kosong):
```json
{ "error": { "code": "GIT_FEATURE_DISABLED", "message": "sync Git belum dikonfigurasi oleh admin", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 7.4 Audit (`record.Request` ringkas, dikirim via bus `core.audit.record`)

```json
{
  "action": "git_credential.saved",
  "actor_type": "user",
  "actor_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "target_type": "git_credential",
  "target": "gitlab:git.noobee.id",
  "args": { "provider": "gitlab", "host": "git.noobee.id", "replaced": true },
  "status": "ok"
}
```
Tidak ada `token`, `last4`, atau URL. Tidak ada audit untuk percobaan yang gagal (token invalid, host ditolak, upstream error).

### 7.5 Panggilan keluar ke provider (untuk fixture/tes)

GitHub: `GET https://api.github.com/user` dengan `Authorization: Bearer <token>`, `X-GitHub-Api-Version: 2022-11-28`, `User-Agent: grimoire-git-sync`.
GitLab: `GET https://{host}/api/v4/user` dengan `PRIVATE-TOKEN: <token>`, `User-Agent: grimoire-git-sync`.
Token tidak pernah di URL/query. Body error provider tidak dibaca ke log.

---

## 8. Changes (per file/folder)

| File / folder | Jenis |
|---|---|
| `backend/internal/domain/core/internal/gitcredential/savecredential/{init.go,handler.go,params.go,usecase.go,repository.go}` | baru |
| `.../savecredential/{handler_test.go,usecase_test.go,repository_integration_test.go,canary_integration_test.go}` | baru |
| `.../gitcredential/base.go` | ubah: satu baris `savecredential.Init(d, shared)` |
| `.../gitcredential/routes_test.go` | ubah/baru: kasus PUT (5.4) |
| `.../gitcredential/base/meta.go`, `base/shared.go` | baru bila absen (3.1) |
| `.../gitcredential/base/errors.go` | ubah aditif bila perlu: `TokenInvalid(provider, host)`, `HostNotAllowed(host)` |
| `.../gitcredential/base/credential.go` | ubah aditif bila perlu: `type Saved struct{ CreatedAt, UpdatedAt time.Time; Replaced bool }` |
| `.../gitcredential/base/store/*.go` | ubah aditif bila perlu: konstanta nama constraint |
| `api/modules/core/git-credential.yml`, `api/openapi.yml` | ubah: path PUT lengkap (3.8) + `make api-bundle` |
| `frontend/src/domain/core/git-credential/savecredential/**` | baru |
| `frontend/src/app/views/ProfileView.svelte` (+ `ProfileView.test.ts`) | ubah: blok seksi Token Git + dialog; `max-w-2xl` |
| `frontend/src/shared/i18n/{id,en}.json` | ubah: kunci `gitCredential.save.*` |
| `frontend/e2e/git-sync.spec.ts` | ubah: skenario bagian 6 (file dimiliki bersama; usecase ini hanya menambah blok `describe('save credential')`) |

Tidak diubah: `app.Deps`, `cmd/**`, module lain, migrasi, `infra/**`, `lib/**`.

---

## 9. Verifikasi (urutan)

1. `go build ./... && go vet ./...` dari `backend/`.
2. `go test ./internal/domain/core/internal/gitcredential/...`.
3. `go test -tags integration ./internal/domain/core/internal/gitcredential/...` (Postgres tersedia).
4. `go test ./internal/archtest/...` (feature tidak meng-import feature lain; `gitcredential` tidak meng-import `project`; `lib/contracts` bersih).
5. Lint repo (golangci-lint sesuai CI).
6. `make api-bundle && make api-check && make api-selftest`.
7. `cd frontend && npm run test && npm run depcruise && npm run test:arch && npm run build`.
8. `npx playwright test e2e/git-sync.spec.ts -g "save credential"`.
9. Pencarian manual akhir: `grep -rn "Reveal()" backend/internal/domain/core/internal/gitcredential/savecredential` hanya menunjukkan baris `Seal` di `usecase.go`; tidak ada `%v`/`%+v` dari `SaveRequest` di log; tidak ada `log.*(... "err", err)` pada jalur verifikasi.

## 10. Risiko dan mitigasi usecase ini

| Risiko | Mitigasi |
|---|---|
| Token bocor lewat pesan error JSON decode | `UnmarshalJSON` kustom mengembalikan error generik tanpa membungkus cause; tes `TokenWrongType`/`BadJSON`/`UnknownField` |
| Token bocor lewat log/audit | tipe `secret.String`; usecase tidak pernah log `err` provider; audit Args dibatasi tiga kunci; tes canary 5.1/5.5 |
| Token invalid tersimpan karena urutan | verifikasi sebelum seal/upsert; tes `VerifyUnauthorized` memastikan repo tidak dipanggil, integrasi memastikan tabel kosong |
| Panggilan keluar dipakai sebagai SSRF/oracle | host lolos policy + safehttp cek IP saat dial; rate limit per user |
| `replaced` salah pada balapan | satu statement atomik, tes serentak |
| Konflik mount chi antar feature | pola route penuh di `d.API` |
| Kunci enkripsi kosong menyebabkan nil-pointer | `enabled` dicek sebelum sealer; konstruktor menegakkan invarian saat wiring |
