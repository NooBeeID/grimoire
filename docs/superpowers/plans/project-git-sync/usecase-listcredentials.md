# T14 core/git-credential: usecase `listcredentials`

Plan usecase (backend + frontend feature `listcredentials`). Wajib mengikuti `overview.md` di folder yang sama; bila bertentangan, overview menang. Bergantung pada foundation (Langkah 0 overview): migrasi `git_credentials`, `gitcredential/base` (`Credential`, `CredentialDTO`, `ActorFrom`, `store.ValidUUID`), `gitprovider.HostPolicy.GitLabHosts()`, config `GitCredentialKey`/`GitSyncAllowedHosts`/`GitFeatureEnabled()`.

Konteks yang dibaca: `docs/tickets/T14-project-git-sync.md`, `overview.md`, `docs/tickets/README.md` (konvensi), kode pembanding `project/internal/apikey/listkeys/*` (pola handler/usecase/repository/tes), `core/internal/auth/getme/handler_test.go`, `frontend/src/domain/project/api-key/listkeys/*`, `frontend/src/app/views/ProfileView.svelte`, `frontend/.dependency-cruiser.cjs`, `DESIGN.md`, `design-taste-frontend/references/app-screens.md`. `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak ada migrasi di usecase ini).

Catatan keputusan: teks tugas menulis kunci JSON camelCase (`allowedGitLabHosts`, `createdAt`). Overview 6.8 dan 12.1 mewajibkan snake_case untuk seluruh JSON T14, jadi kontrak final: `allowed_gitlab_hosts`, `created_at`, `updated_at`. Zod di FE mengikuti snake_case.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | fullstack (backend + frontend feature ini; PUT/DELETE dan dialog di plan usecase lain) |
| Domain / Module | core / git-credential (`gitcredential`) |
| Feature | `listcredentials` |
| Resource root BE | `backend/internal/domain/core/internal/gitcredential/listcredentials` |
| Resource root FE | `frontend/src/domain/core/git-credential/listcredentials` |
| Kriteria penerimaan T14 | 1 (daftar hanya `last4`, token tak pernah di respons), 13 (tanpa token di respons/log/audit; usecase baca-saja tidak menulis audit) |

| Endpoint | Method | Version | Auth |
|---|---|---|---|
| `/api/me/git-credentials` | GET | tanpa prefix versi (`/api`); `api/modules/core/git-credential.yml` `info.version` 0.1.0 | `RequireAuth` (sesi). Tanpa CSRF (GET) |

Bus: tidak menyediakan dan tidak memanggil kontrak apa pun. Audit: tidak ada (baca-saja, sesuai katalog audit T14).

---

## 2. Summary Plan

User membuka halaman Profil dan perlu tahu token Git mana yang sudah tersimpan dan untuk host apa, tanpa pernah melihat token. Endpoint mengembalikan tiga hal:

1. `enabled`: apakah `GIT_CREDENTIAL_KEY` terpasang (`cfg.GitFeatureEnabled()`), agar UI menampilkan pesan "belum dikonfigurasi admin" dan bukan error.
2. `allowed_gitlab_hosts`: `["gitlab.com", ...GIT_SYNC_ALLOWED_HOSTS]` dari `gitprovider.HostPolicy.GitLabHosts()`, sumber opsi Select host di dialog simpan token.
3. `items`: baris milik user yang login saja: `provider, host, last4, created_at, updated_at`.

Prinsip:

- Identity hanya dari `reqctx`; tidak ada parameter user di URL/query/body. Query selalu `WHERE user_id = $1` dengan id identity. Baris user lain tidak mungkin terbaca (D7).
- Kolom `token_enc` TIDAK pernah di-SELECT, tidak ada di struct hasil, tidak ada di DTO. Dijaga struktur (SQL tanpa kolom itu) dan tes (SQL const, `TokenEnc == nil`, JSON tanpa kata `token`).
- Fitur nonaktif tetap 200 (`enabled=false`) dengan `items` dari DB (bisa kosong). Dekripsi tidak dibutuhkan, jadi usecase tidak butuh `secretbox` dan tidak memanggil provider.
- Fail-closed: `userID` bukan uuid -> daftar kosong non-nil tanpa query.
- Urutan stabil `ORDER BY provider, host` (unique `(user_id, provider, host)` menjamin tidak ada tie).
- Respons `Cache-Control: no-store` (data akun per user).
- Usecase hanya orkestrasi: `ActorFrom` -> `repo.ListByUser` -> map ke DTO + gabung `Settings`. Tidak ada rule bisnis baru; bentuk DTO ada di entity `base.Credential.ToDTO()` (foundation).

---

## 3. Plan (langkah berurutan)

Prasyarat: Langkah 0 overview sudah ada di branch (migrasi, `gitcredential/base`, config, `gitprovider`). Bila `base.Credential`/`ToDTO`/`ActorFrom`/`store.ValidUUID` belum ada, kerjakan foundation dulu; jangan menduplikasi di feature.

### 3.1 Backend

1. **Tes dulu (merah)**: tulis `usecase_test.go`, `handler_test.go` (bagian 6.1, 6.2) dengan fake `Lister`. Tulis `repository_integration_test.go` (6.3) dan helper-nya.
2. **`listcredentials/usecase.go`**:
   ```go
   package listcredentials

   // Lister membaca kredensial milik satu user. Tidak pernah mengembalikan token terenkripsi.
   type Lister interface {
       ListByUser(ctx context.Context, userID string) ([]base.Credential, error)
   }

   // Settings adalah nilai config yang dihitung sekali saat Init (bukan dibaca per request).
   type Settings struct {
       Enabled     bool     // cfg.GitFeatureEnabled()
       GitLabHosts []string // policy.GitLabHosts(), sudah termasuk gitlab.com
   }

   // Result adalah body 200. Satu-satunya bentuk keluar; tidak ada field token.
   type Result struct {
       Enabled            bool                `json:"enabled"`
       AllowedGitLabHosts []string            `json:"allowed_gitlab_hosts"`
       Items              []base.CredentialDTO `json:"items"`
   }

   type Usecase struct { repo Lister; set Settings }

   func NewUsecase(r Lister, s Settings) *Usecase // menyalin s.GitLabHosts (hindari aliasing), nil -> []string{}
   func (u *Usecase) Execute(ctx context.Context) (Result, error)
   ```
   `Execute`: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`; `creds, err := u.repo.ListByUser(ctx, actor.ID)`; error dibungkus `fmt.Errorf("listcredentials: list: %w", err)` (bukan `*errs.Error`, jadi 500 generik tanpa bocor). `items := make([]base.CredentialDTO, 0, len(creds))` lalu `c.ToDTO()` per baris (non-nil walau kosong). Kembalikan `Result{Enabled: u.set.Enabled, AllowedGitLabHosts: copy, Items: items}`. Tidak ada filter/sort di memori (urutan milik repository).
3. **`listcredentials/repository.go`**:
   ```go
   // Kolom token_enc SENGAJA tidak ada di sini.
   const listByUserSQL = `SELECT provider, host, last4, created_at, updated_at
   FROM git_credentials
   WHERE user_id = $1::uuid
   ORDER BY provider, host`

   type Repo struct{ db *db.DB }
   func newRepo(d *db.DB) *Repo

   type row struct {
       Provider  string    `db:"provider"`
       Host      string    `db:"host"`
       Last4     string    `db:"last4"`
       CreatedAt time.Time `db:"created_at"`
       UpdatedAt time.Time `db:"updated_at"`
   }

   func (r *Repo) ListByUser(ctx context.Context, userID string) ([]base.Credential, error)
   ```
   - `!store.ValidUUID(userID)` -> `[]base.Credential{}, nil` tanpa query.
   - `SelectContext` ke `[]row`; map ke `base.Credential{UserID: userID, Provider: base.Provider(r.Provider), Host: r.Host, Last4: r.Last4, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}`; `TokenEnc` dibiarkan `nil`. Hasil kosong -> slice non-nil.
   - Error dibungkus `fmt.Errorf("listcredentials: select: %w", err)`.
   - Komentar kode: "DILARANG menambah token_enc ke SELECT ini; jalur dekripsi hanya di bustoken."
4. **`listcredentials/handler.go`**: pola `listkeys`:
   ```go
   func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       res, err := h.uc.Execute(r.Context())
       if err != nil { respond.WriteError(w, r, err); return }
       w.Header().Set("Cache-Control", "no-store")
       respond.WriteJSON(w, http.StatusOK, res)
   }
   ```
   Query string dan body diabaikan sepenuhnya.
5. **`listcredentials/init.go`**:
   ```go
   // Init mendaftarkan GET /api/me/git-credentials.
   func Init(d app.Deps) {
       policy := gitprovider.HostPolicy{AllowedGitLabHosts: d.Config.GitSyncAllowedHosts}
       uc := NewUsecase(newRepo(d.DB), Settings{
           Enabled:     d.Config.GitFeatureEnabled(),
           GitLabHosts: policy.GitLabHosts(),
       })
       d.API.With(middleware.RequireAuth()).Get("/me/git-credentials", newHandler(uc).ServeHTTP)
   }
   ```
   Route didaftarkan dengan path penuh (`Get("/me/git-credentials")`), BUKAN `Route("/me/git-credentials", ...)`, agar tidak bentrok dengan feature `savecredential`/`deletecredential` yang mendaftarkan `/me/git-credentials/{provider}` (chi panic bila mount ganda pada prefix yang sama). Bila foundation `gitcredential/base.go` ternyata sudah membuat `Route` group, sesuaikan Init mengikuti group itu (koordinasi dengan langkah 12 foundation); perilaku tidak berubah.
6. **Wiring**: di `core/internal/gitcredential/base.go` tambah SATU baris `listcredentials.Init(d)` (overview sudah mencantumkannya di `Register`). Tambah SATU kasus di tes registrasi route module (`routes_test.go`): `GET /api/me/git-credentials` terdaftar dengan `RequireAuth` (tanpa login -> 401).
7. **OpenAPI**: path `GET /me/git-credentials` dan schema `GitCredential`, `GitCredentialList` sudah disiapkan foundation (langkah 15). Di usecase ini hanya verifikasi bentuknya sama dengan bagian 8 (properti, `required`, contoh `enabled=false`, respons 401/500 via `Error`). Bila ada selisih, rapikan `api/modules/core/git-credential.yml`, jalankan `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau. `GitCredential` memakai `additionalProperties: false` dan tidak memiliki properti `token*`.
8. **Verifikasi BE**: `go test ./backend/internal/domain/core/internal/gitcredential/listcredentials/...`, lalu `go test -tags integration` pada package yang sama, `go vet`, `go test ./backend/internal/archtest/...`, `make api-check`.

### 3.2 Frontend (mengikuti kontrak bagian 8; selaras dengan `frontend-plan.md` bila sudah ada, bila bertentangan overview menang)

Struktur feature `frontend/src/domain/core/git-credential/listcredentials/`:

```
index.ts
types.ts, types.test.ts
mocks/credentials.ts
services/listcredentials.service.ts (+ .test.ts)
stores/gitCredentials.store.ts (+ .test.ts)
components/GitTokensTable.svelte, ProviderCell.svelte, components.test.ts
views/GitTokensSection.svelte (+ .test.ts)
```

1. **`types.ts`** (Zod, snake_case, mengikuti `GitCredentialList` di OpenAPI):
   ```ts
   export const GIT_PROVIDERS = ['github', 'gitlab'] as const;
   export const GitCredentialSchema = z.object({
     provider: z.enum(GIT_PROVIDERS),
     host: z.string().min(1),
     last4: z.string().min(1).max(4),
     created_at: z.string().datetime({ offset: true }),
     updated_at: z.string().datetime({ offset: true }),
   }).strict();   // field tak dikenal (mis. token) -> INVALID_RESPONSE
   export const GitCredentialListSchema = z.object({
     enabled: z.boolean(),
     allowed_gitlab_hosts: z.array(z.string()),
     items: z.array(GitCredentialSchema),
   });
   ```
   `.strict()` sengaja: bila server pernah mengirim field tak terduga (termasuk token), respons ditolak dan tidak masuk store. Tambahan murni:
   ```ts
   export type TokenRow = { provider: GitProvider; host: string; saved: GitCredential | null };
   // GitHub selalu ada (github.com); GitLab satu baris per host tersimpan; bila belum ada GitLab
   // sama sekali, satu baris placeholder gitlab.com "Belum ada". Urutan: github, lalu gitlab by host.
   export function buildRows(items: GitCredential[]): TokenRow[]
   export function maskLast4(last4: string): string // `…${last4}`
   ```
2. **`services/listcredentials.service.ts`**: `listCredentials(signal?)` = `get('/api/me/git-credentials', { schema: GitCredentialListSchema, signal })` (helper `get` dari `shared/services/http`, tanpa header CSRF).
3. **`stores/gitCredentials.store.ts`**: pola `keyList.store`, tetapi ber-kunci user agar tidak bocor lintas login pada SPA yang sama:
   ```ts
   type State =
     | { status: 'idle' }
     | { status: 'loading'; userId: string; previous: GitCredentialList | null }
     | { status: 'ready'; userId: string; data: GitCredentialList }
     | { status: 'error'; userId: string; error: ApiError; previous: GitCredentialList | null };
   export interface GitCredentialsStore extends Readable<State> {
     version: Readable<number>;
     begin(userId: string): void;   // previous hanya dipertahankan bila userId sama
     resolve(userId: string, data: GitCredentialList): void; // userId basi -> diabaikan
     fail(userId: string, e: ApiError): void;
     invalidate(): void;            // dipanggil savecredential/deletecredential setelah sukses
     reset(): void;                 // ke idle (dipakai saat identity berubah/logout)
   }
   export function createGitCredentialsStore(): GitCredentialsStore;
   export const gitCredentials: GitCredentialsStore; // instance bersama
   export const hasCredential = (s: State, provider: GitProvider, host: string): boolean | null; // null bila belum ready
   ```
   `gitCredentials` dan `hasCredential` diekspor lewat `index.ts` publik. Konsumen lintas module (GitSyncDialog) TIDAK memakainya: sesuai overview bagian 9, dialog memakai `has_credential`/`credential_unreadable`/`feature_enabled` dari `getconfig` dan error `CREDENTIAL_MISSING`. `depcruise` sebenarnya mengizinkan import `index.ts` feature lain, tetapi aturan 12.4 menempatkan komposisi lintas module di `app/`, sehingga bila nanti dibutuhkan, `app/` yang meneruskan nilainya sebagai prop. Tidak ada import `domain/project/git-sync` dari feature ini dan sebaliknya.
4. **`components/ProviderCell.svelte`**: ikon outline 16px (SVG inline, `currentColor`, stroke 1.5, bukan logo merek) + nama provider ("GitHub"/"GitLab"). **`components/GitTokensTable.svelte`**: membungkus `shared/components/Table.svelte` (sudah panel radius 8 + `shadow-panel` + header 13px pada pita `bg-raised`, baris 48px). Props: `rows: TokenRow[]`, `loading`, `error`, `onretry`, `canEdit: boolean` (= `enabled`), `onsave?: (row: TokenRow) => void`, `ondelete?: (row: TokenRow) => void`. Kolom: Provider, Host (mono 13px), Status, Diperbarui (rata kanan, `tabular-nums`, `formatDateTime`), Aksi. Status: tersimpan -> pil `bg-pill-positive-bg text-pill-positive-ink` "Tersimpan" + `…7Qx2` mono `text-muted`; belum -> pil neutral "Belum ada" dan Diperbarui `–` redup. Aksi (hanya bila `canEdit`): tersimpan -> ghost "Ganti" + ghost "Hapus"; belum -> ghost "Simpan". Tombol Aksi dirender hanya bila callback-nya diberikan. `aria-label` per tombol memuat provider+host.
5. **`views/GitTokensSection.svelte`**: props `{ onsave?, ondelete?, onadd? }`. Mengambil `userId` dari `session` store (`shared/stores/session`), memuat dengan `AbortController` (pola `ListKeysView.load`, subscribe `store.version` untuk muat ulang), menyimpan hasil ke `gitCredentials`. Layout: judul section ("Token Git", `h2`), satu kalimat penjelas (`text-muted`), lalu salah satu:
   - loading pertama: skeleton lewat `Table loading`; error: `ErrorState` + coba lagi (data lama dipertahankan bila ada `previous`);
   - `enabled=false`: panel berbingkai dengan teks "Sync Git belum dikonfigurasi oleh admin (GIT_CREDENTIAL_KEY)." (`GIT_CREDENTIAL_KEY` mono), baris tersimpan (bila ada) tetap tampil tanpa aksi;
   - `enabled=true`: tabel + di bawahnya tautan teks "Tambah host GitLab" (hanya bila `onadd` ada) dan baris bantu `text-muted`: scope minimal (`repo` read untuk GitHub; `read_api` + `read_repository` untuk GitLab) dan "Token tidak bisa dilihat lagi setelah disimpan."
   View mengekspos `allowed_gitlab_hosts` ke callback (`onsave`/`onadd` menerima `{ row, allowedGitLabHosts }`) supaya dialog simpan (plan `savecredential`) mengisi Select host tanpa fetch ulang.
6. **`index.ts`**: `export { default as GitTokensSection } from './views/GitTokensSection.svelte'; export { gitCredentials, createGitCredentialsStore, hasCredential } from './stores/gitCredentials.store'; export type { GitCredential, GitCredentialList, TokenRow } from './types';` (hanya ini yang boleh diimpor dari luar feature).
7. **`mocks/credentials.ts`**: `mockCredential(provider, host, over)`, `mockList(...)`, set data contoh dari bagian 8 (`github.com` `a9F3`, `git.noobee.id` `7Qx2`), plus `mockListDisabled`.
8. **Komposisi `frontend/src/app/views/ProfileView.svelte`**:
   - Bungkus lama (identitas + `ChangePasswordView`) tetap `max-w-sm`; container luar jadi `max-w-2xl` (overview bagian 9) agar tabel muat. Judul `h1` tetap.
   - Tambah `<section id="token-git" aria-labelledby="token-git-title" class="scroll-mt-6">` berisi `<GitTokensSection />` (import dari `../../domain/core/git-credential/listcredentials`).
   - Anchor: bila `location.hash === '#token-git'`, setelah data siap panggil `document.getElementById('token-git')?.scrollIntoView()` dan beri fokus ke `h2` (`tabindex="-1"`). Tautan `/settings#token-git` dari dialog Sync dan error `CREDENTIAL_MISSING` harus bekerja pada load penuh maupun navigasi in-app.
   - Callback `onsave`/`ondelete`/`onadd` dibiarkan tidak diberikan di plan ini. Plan `savecredential` dan `deletecredential` masing-masing menambah dialog dan satu pasang prop/handler di ProfileView (`gitCredentials.invalidate()` setelah sukses). Sampai itu masuk, tabel hanya-baca (tanpa tombol aksi); ini valid dan teruji.
9. **i18n** di `frontend/src/shared/i18n/{id,en}.json` (kunci sama di kedua berkas, tes paritas i18n yang ada harus hijau):

   | Kunci | id | en |
   |---|---|---|
   | `gitCredential.title` | Token Git | Git tokens |
   | `gitCredential.intro` | Token pribadi untuk menarik file dari GitHub/GitLab. Hanya Anda yang memakainya. | Personal tokens used to pull files from GitHub/GitLab. Only you can use them. |
   | `gitCredential.col.provider` | Provider | Provider |
   | `gitCredential.col.host` | Host | Host |
   | `gitCredential.col.status` | Status | Status |
   | `gitCredential.col.updated` | Diperbarui | Updated |
   | `gitCredential.col.actions` | Aksi | Actions |
   | `gitCredential.status.saved` | Tersimpan | Saved |
   | `gitCredential.status.none` | Belum ada | Not set |
   | `gitCredential.provider.github` | GitHub | GitHub |
   | `gitCredential.provider.gitlab` | GitLab | GitLab |
   | `gitCredential.action.save` / `.replace` / `.delete` | Simpan / Ganti / Hapus | Save / Replace / Delete |
   | `gitCredential.action.save.aria` | Simpan token {provider} untuk {host} | Save {provider} token for {host} |
   | `gitCredential.action.replace.aria` | Ganti token {provider} untuk {host} | Replace {provider} token for {host} |
   | `gitCredential.action.delete.aria` | Hapus token {provider} untuk {host} | Delete {provider} token for {host} |
   | `gitCredential.addGitlabHost` | Tambah host GitLab | Add GitLab host |
   | `gitCredential.disabled` | Sync Git belum dikonfigurasi oleh admin ({key}). | Git sync has not been configured by an admin ({key}). |
   | `gitCredential.scopes.github` | GitHub: scope `repo` (baca). | GitHub: `repo` scope (read). |
   | `gitCredential.scopes.gitlab` | GitLab: scope `read_api` dan `read_repository`. | GitLab: `read_api` and `read_repository` scopes. |
   | `gitCredential.hiddenNote` | Token tidak bisa dilihat lagi setelah disimpan. | A token cannot be viewed again after it is saved. |
   | `gitCredential.error.load` | Gagal memuat token Git. | Failed to load Git tokens. |

---

## 4. Migration

Tidak ada migrasi di usecase ini. Bergantung pada migrasi foundation `backend/migrations/core/<VERSION>_create_git_credentials.sql` (overview 7.1): tabel `git_credentials` dengan unique `(user_id, provider, host)` yang melayani `WHERE user_id = $1 ORDER BY provider, host` lewat prefix index unique; tidak perlu index tambahan. Usecase tidak merujuk nama file migrasi, jadi aman terhadap versi timestamp yang di-generate saat implementasi (`.claude/rules/migrations.md`).

---

## 5. Changes (per file/folder)

| File / folder | Jenis | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/gitcredential/listcredentials/init.go` | baru | `Init(d)`: policy, `Settings`, route GET |
| `.../listcredentials/handler.go` | baru | `ServeHTTP`, `Cache-Control: no-store` |
| `.../listcredentials/usecase.go` | baru | `Lister`, `Settings`, `Result`, `Usecase.Execute` |
| `.../listcredentials/repository.go` | baru | `listByUserSQL` tanpa `token_enc`, `ListByUser` fail-closed |
| `.../listcredentials/handler_test.go` | baru | unit HTTP (fake Lister) |
| `.../listcredentials/usecase_test.go` | baru | unit (fake Lister) |
| `.../listcredentials/repository_integration_test.go` | baru | tag `integration`, Postgres nyata, termasuk helper seed user/credential |
| `backend/internal/domain/core/internal/gitcredential/base.go` | ubah (1 baris) | `listcredentials.Init(d)` |
| `backend/internal/domain/core/internal/gitcredential/routes_test.go` | ubah (1 kasus) | route GET terdaftar + 401 tanpa login |
| `api/modules/core/git-credential.yml`, `api/openapi.yml` | verifikasi/rapikan | bentuk sama dengan bagian 8 |
| `frontend/src/domain/core/git-credential/listcredentials/**` | baru | lihat 3.2 |
| `frontend/src/app/views/ProfileView.svelte` (+ `app.test.ts` bila menguji profil) | ubah | lebar `max-w-2xl`, seksi `#token-git`, scroll ke anchor |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | kunci `gitCredential.*` (3.2 no. 9) |
| `frontend/e2e/git-sync.spec.ts` | ubah (bagian profil) | skenario bagian 7 (file dibagi dengan usecase lain; hanya tambah blok `describe('Token Git: daftar')`) |
| `savecredential/**`, `deletecredential/**`, `bustoken/**`, `docs/tickets/README.md`, migrasi, `app.Deps` | TIDAK disentuh | milik foundation / usecase lain |

---

## 6. Test Scenario

### 6.1 Unit `usecase_test.go` (fake `Lister` merekam `calls`, `userID`)

`authCtxAs(id, role)` = `reqctx.WithIdentity(ctx, Identity{UserID: id, Role: role, Email: ...})`.

| Kasus | Setup | Harapan |
|---|---|---|
| Tanpa identitas | `context.Background()` | `errs.CodeUnauthorized`; repo tidak dipanggil |
| Identitas dipakai | user `u1` | `repo.userID == "u1"`, dipanggil 1x |
| Role tidak relevan | `super_admin` | tetap hanya `repo.userID == "u1"`-nya sendiri (tidak ada bypass admin) |
| Map DTO | 2 kredensial | `Items` urut sama seperti repo; `provider/host/last4/created_at/updated_at` benar |
| Kosong | repo `nil` dan `[]` | `Items` non-nil; JSON `"items":[]` (bukan `null`) |
| Settings enabled | `Settings{Enabled: true, GitLabHosts: ["gitlab.com","git.noobee.id"]}` | `Enabled=true`, hosts sama persis |
| Fitur nonaktif | `Enabled=false`, repo berisi 1 baris | `Enabled=false`, items tetap terisi (tidak dikosongkan) |
| Hosts nil | `GitLabHosts=nil` | JSON `"allowed_gitlab_hosts":[]` |
| Tanpa aliasing | ubah slice sumber setelah `NewUsecase` | hasil tidak ikut berubah |
| Error repo | `boom` | dibungkus (`errors.Is(err, boom)`), bukan `*errs.Error`, `Result` zero |
| Tanpa token di output | kredensial dengan `TokenEnc = []byte("CANARY...")` dari repo palsu | `json.Marshal(Result)` tidak memuat `CANARY`, `token`, `token_enc`, `TokenEnc` |

### 6.2 Unit `handler_test.go`

| Kasus | Harapan |
|---|---|
| 200 bentuk | key tingkat atas persis `{enabled, allowed_gitlab_hosts, items}`; key tiap item persis `{provider, host, last4, created_at, updated_at}` |
| Header | `Content-Type: application/json`, `Cache-Control: no-store` |
| Tanpa token | body (string) tidak mengandung substring `token` (case-insensitive) |
| Nonaktif | `enabled=false`, `items` bisa `[]`, status 200 (bukan 4xx/5xx) |
| Abaikan query/body | `?user_id=lain`, body acak -> 200, repo menerima id dari identity (bukan query) |
| 401 | tanpa identitas -> 401 envelope `UNAUTHORIZED` dengan `trace_id`; tanpa `no-store` |
| 500 | repo error `connection refused: dsn=postgres://secret` -> 500 envelope generik, body tidak memuat teks internal |
| Metode lain | (di `routes_test.go`) `POST /api/me/git-credentials` -> 405 |

### 6.3 Integration `repository_integration_test.go` (tag `integration`, `db.TestDB` + `db.Up` + migrasi nyata)

Helper: `mkUser(t, d, email) string` (`INSERT INTO users (email,name,password_hash,role) VALUES ($1,'U','x','developer') RETURNING id::text`), `mkCred(t, d, userID, provider, host, last4 string, created, updated time.Time, enc []byte)` (enc default 40 byte berawalan `CANARY` agar lolos `octet_length > 28`).

| Kasus | Seed | Harapan |
|---|---|---|
| Hanya milik sendiri | A: github + gitlab `git.noobee.id`; B: github | `ListByUser(A)` = 2 baris A; `ListByUser(B)` = 1 baris B |
| User lain tidak terlihat | B punya `gitlab.com` | tidak muncul di hasil A; hasil A tidak memuat `last4` B |
| Urutan stabil | A: gitlab `z.example.com`, github, gitlab `a.example.com`, gitlab `gitlab.com` (sisip acak) | urutan: `github/github.com`, `gitlab/a.example.com`, `gitlab/gitlab.com`, `gitlab/z.example.com`; dua kali panggil = hasil sama |
| Kosong | user tanpa baris | slice non-nil panjang 0 |
| userID bukan uuid | `"bukan-uuid"` | slice kosong non-nil, nil error (tanpa error cast Postgres) |
| uuid acak tak ada | uuid valid tak terdaftar | kosong, nil error |
| `token_enc` tidak dibaca | baris berisi `CANARY` | tiap `TokenEnc == nil`; `fmt.Sprintf("%+v", got)` tidak memuat `CANARY`; konstanta `listByUserSQL` tidak mengandung substring `token_enc` |
| Field & UTC | created/updated berbeda | `CreatedAt`/`UpdatedAt` benar, `Location() == time.UTC`, `Provider` bertipe `base.Provider` |
| Setelah replace | update `last4`, `updated_at` | hasil mencerminkan nilai baru (satu baris, bukan dua) |
| Hapus user | `DELETE FROM users` | baris credential ikut hilang (CASCADE), `ListByUser` kosong |

Tes HTTP integrasi tipis (di file yang sama atau `listcredentials_integration_test.go`): router chi + `Init` + `reqctx` identity dari header uji; user A dan B masing-masing GET -> hanya barisnya; `GIT_CREDENTIAL_KEY` kosong (`Config` tanpa key) -> `enabled=false` dengan items DB; `GitSyncAllowedHosts=["git.noobee.id"]` -> `allowed_gitlab_hosts == ["gitlab.com","git.noobee.id"]`; tanpa identitas -> 401.

### 6.4 Tes PAT canary (bagian dari 10.4 overview)

Lengkapi di sini: seed baris dengan `token_enc` berisi canary `ghp_CANARY0123456789abcdefCANARY` (apa adanya, bukan ciphertext asli, karena yang diuji adalah jalur baca), lalu GET dengan logger buffer slog level debug terpasang. Assert canary tidak ada di body respons, di buffer log, dan tidak ada panggilan `record.Request` ke audit (fake audit bus tidak menerima panggilan: usecase baca-saja).

### 6.5 Arch

`archtest` hijau: `listcredentials` tidak meng-import feature lain, tidak ada import `infra/crypto/secretbox` (tidak boleh membuka token), hanya `base`, `base/store`, `infra/{app,db,middleware,http/respond,gitprovider}`, `lib/common/{errs,reqctx}`. Grep tes sederhana di `usecase_test.go`: file `repository.go` tidak mengandung `token_enc` pada SQL.

### 6.6 Frontend unit/komponen (vitest + testing-library)

| File | Kasus |
|---|---|
| `types.test.ts` | skema menerima contoh 200; menolak item dengan field `token` (`.strict()`); `last4` kosong/5 karakter ditolak; `buildRows`: kosong -> GitHub "belum" + GitLab `gitlab.com` placeholder; hanya GitLab tersimpan -> GitHub placeholder di atas, GitLab sesuai host urut; dua host GitLab -> dua baris tanpa placeholder; `maskLast4('7Qx2') === '…7Qx2'` |
| `services/*.test.ts` | GET `/api/me/git-credentials` tanpa header CSRF; respons invalid (`items:[{provider:'x'}]`) -> `INVALID_RESPONSE`; 401 -> `ApiError` 401; 500 envelope diteruskan |
| `stores/*.test.ts` | begin/resolve -> ready; `begin` dengan userId berbeda membuang `previous`; `resolve` userId basi diabaikan; fail 500 mempertahankan `previous`; `invalidate` menaikkan `version`; `reset` -> idle; `hasCredential` true/false/null |
| `components.test.ts` | `GitTokensTable`: baris tersimpan menampilkan pil "Tersimpan" dan `…7Qx2`, tanpa teks token penuh; baris belum menampilkan "Belum ada" dan `–`; `canEdit=false` -> tidak ada tombol aksi; callback tidak diberikan -> tombol tidak dirender; klik "Ganti"/"Hapus"/"Simpan" memanggil callback dengan baris yang benar; `aria-label` memuat provider+host; host mono |
| `views/GitTokensSection.test.ts` | loading -> skeleton; sukses -> tabel; `enabled=false` -> teks admin `GIT_CREDENTIAL_KEY`, tanpa tombol aksi, baris tersimpan tetap tampil; error 500 -> `ErrorState` + retry memuat ulang; `invalidate()` memicu fetch ulang; ganti identity (userId lain) -> tidak menampilkan data user sebelumnya; DOM tidak mengandung `token` selain label UI |
| `app/views/app.test.ts` (atau `ProfileView.test.ts` baru) | profil memuat seksi dengan `id="token-git"`; `location.hash='#token-git'` memanggil `scrollIntoView`; blok password lama tetap ada |
| i18n | tes paritas kunci id/en hijau |

`npm run depcruise` / `test:arch`: feature ini hanya diimpor lewat `index.ts` dari `app/views/ProfileView.svelte`; tidak ada import ke `domain/project/**`.

---

## 7. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`, blok "Token Git: daftar")

Setup mengikuti overview bagian 11 (backend e2e dengan `GIT_CREDENTIAL_KEY`, fake git server untuk verifikasi token saat simpan). Skenario di bawah hanya yang menguji daftar.

Positif:
1. Login sebagai A, buka `/profile`: seksi "Token Git" tampil dengan baris GitHub "Belum ada" dan baris GitLab `gitlab.com` "Belum ada"; tombol aksi tidak ada (sebelum plan save masuk) atau "Simpan" (setelah).
2. Setelah A menyimpan token sah (lewat UI/API fixture `ghp_e2eValidToken7Qx2`): muat ulang `/profile`, baris GitHub menampilkan pil "Tersimpan" dan `…7Qx2`; `page.on('response')` pada `/api/me/git-credentials` memastikan body tidak memuat `ghp_e2e` maupun key bernama `token*`; DOM tidak memuat token penuh.
3. Dua host: A menyimpan GitLab `git.noobee.id` (host ada di `GIT_SYNC_ALLOWED_HOSTS` e2e): daftar menampilkan GitHub, lalu GitLab `git.noobee.id`; urutan stabil setelah reload.
4. Buka `/settings#token-git` lewat tautan langsung: halaman menggulir ke seksi dan fokus pada judul seksi.
5. Daftar per user: login sebagai B (tanpa token): `GET /api/me/git-credentials` lewat `request` hanya `items: []` (tidak memuat baris A); UI B menampilkan semua baris "Belum ada".

Negatif:
1. Tanpa login: `GET /api/me/git-credentials` -> 401 envelope `UNAUTHORIZED`; membuka `/profile` mengarahkan ke login.
2. Fitur nonaktif: `page.route('**/api/me/git-credentials', ...)` mengembalikan `{"enabled":false,"allowed_gitlab_hosts":["gitlab.com"],"items":[]}` (200): seksi tetap berbingkai, menampilkan "Sync Git belum dikonfigurasi oleh admin (GIT_CREDENTIAL_KEY).", tanpa tombol aksi, tanpa toast error.
3. Server error: `page.route` mengembalikan 500: seksi menampilkan `ErrorState` "Gagal memuat token Git." dengan tombol coba lagi; klik coba lagi dengan route dilepas memulihkan tabel. Halaman Profil lainnya (identitas, ganti password) tetap berfungsi.
4. Respons tak valid: route mengembalikan item berisi field `token` -> UI menampilkan `ErrorState` (INVALID_RESPONSE), nilai token tidak dirender di DOM.
5. Ganti user pada tab yang sama (logout A, login B): tabel B tidak sempat menampilkan baris A (store ber-kunci user).

Regresi: `frontend/e2e` profil/ganti-password yang ada tetap hijau setelah layout `max-w-2xl`.

---

## 8. Request/Response body

Format error repo: `{"error": {"code", "message", "details"}, "trace_id"}`. GET tanpa body dan tanpa header CSRF.

### 8.1 GET `/api/me/git-credentials`

200, fitur aktif, dua token tersimpan:
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

200, fitur aktif, belum ada token:
```json
{ "enabled": true, "allowed_gitlab_hosts": ["gitlab.com"], "items": [] }
```

200, fitur nonaktif (`GIT_CREDENTIAL_KEY` kosong), tetap bukan error:
```json
{ "enabled": false, "allowed_gitlab_hosts": ["gitlab.com"], "items": [] }
```

200, fitur nonaktif tetapi baris lama masih ada di DB (key pernah dicabut):
```json
{
  "enabled": false,
  "allowed_gitlab_hosts": ["gitlab.com"],
  "items": [
    { "provider": "github", "host": "github.com", "last4": "a9F3", "created_at": "2026-10-22T07:00:00Z", "updated_at": "2026-10-22T07:00:00Z" }
  ]
}
```

Header respons: `Content-Type: application/json`, `Cache-Control: no-store`. Tidak pernah ada field `token`, `token_enc`, atau ciphertext. Query string diabaikan.

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

500 (kegagalan DB, pesan internal tidak bocor):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(`code`/`message` 500 mengikuti `respond.WriteError` yang ada; tes handler mengunci bahwa teks error asli tidak muncul.)

### 8.2 Skema OpenAPI (`api/modules/core/git-credential.yml`, bagian GET)

```yaml
GitCredential:
  type: object
  additionalProperties: false
  required: [provider, host, last4, created_at, updated_at]
  properties:
    provider: { type: string, enum: [github, gitlab] }
    host: { type: string, example: git.noobee.id }
    last4: { type: string, minLength: 1, maxLength: 4, example: 7Qx2 }
    created_at: { type: string, format: date-time }
    updated_at: { type: string, format: date-time }
GitCredentialList:
  type: object
  additionalProperties: false
  required: [enabled, allowed_gitlab_hosts, items]
  properties:
    enabled: { type: boolean, description: "false bila GIT_CREDENTIAL_KEY kosong; tetap 200" }
    allowed_gitlab_hosts: { type: array, items: { type: string } }
    items: { type: array, items: { $ref: '#/components/schemas/GitCredential' } }
```

### 8.3 Bentuk Go (internal)

```go
listcredentials.Result{
    Enabled:            true,
    AllowedGitLabHosts: []string{"gitlab.com", "git.noobee.id"},
    Items: []base.CredentialDTO{
        {Provider: "github", Host: "github.com", Last4: "a9F3", CreatedAt: t0, UpdatedAt: t0},
    },
}
```

---

## 9. Visual (frontend, DESIGN.md framed-sheet)

**Pertanyaan layar:** "Token Git mana yang sudah saya simpan, dan untuk host apa?" Blok terbesar yang menjawabnya: tabel panel Token Git (bukan kartu, bukan banner). Temuan di baris di bawah judul: satu kalimat data tentang cakupan ("Hanya Anda yang memakainya"), bukan instruksi UI.

**Pola DESIGN.md yang dipakai:**
- Section di dalam sheet halaman Profil (radius 16, `border-border`, `shadow-sheet` sudah dari layout); tabel = panel (radius 8, border, `shadow-panel`), header 13px abu medium pada pita `bg-raised`, baris 48px, rule 1px `border-rule`, hover `bg-raised/60`, angka/tanggal rata kanan `tabular-nums`, satu penanda status per baris. Ini komponen `shared/components/Table.svelte` apa adanya.
- Warna hanya pada data: pil status. Tersimpan = `bg-pill-positive-bg text-pill-positive-ink`; Belum ada = `bg-pill-neutral-bg text-pill-neutral-ink`. Tidak ada tombol primer terisi di seksi ini (aksi ghost). `accent-ink` hanya untuk cincin fokus.
- Bagian kosong tetap berbingkai: semua state (loading, kosong, nonaktif, error) berada di dalam bingkai panel yang sama sehingga tinggi halaman tidak melompat.

**Elemen khas:** sel Status berupa pil + `…7Qx2` monospace redup (`font-mono text-[13px] text-muted`): satu-satunya jejak token yang pernah tampil. Kolom Host monospace. Baris GitHub selalu ada sebagai jangkar; baris GitLab bertambah per host.

**Token:** `bg-surface`, `border-border`, `border-rule`, `bg-raised`, `shadow-panel`, `text-muted`, `text-fg`, `pill-positive`, `pill-neutral`, radius 8 (panel) / 6 (kontrol), tinggi baris 48, tinggi kontrol 32, ikon outline 16px `currentColor`. Tanpa gradien, tanpa ikon dalam lingkaran berwarna, tanpa zebra, tanpa header uppercase.

**Data contoh (bukan lorem):**

| Provider | Host | Status | Diperbarui |
|---|---|---|---|
| GitHub | `github.com` | Tersimpan `…a9F3` | 22 Okt 2026, 07:00 |
| GitLab | `git.noobee.id` | Tersimpan `…7Qx2` | 22 Okt 2026, 07:10 |
| GitLab | `gitlab.com` | Belum ada | – |

Keadaan lain: nonaktif = panel berbingkai berisi "Sync Git belum dikonfigurasi oleh admin (`GIT_CREDENTIAL_KEY`)."; error = `ErrorState` di dalam bingkai; loading = baris skeleton tinggi 48 di dalam bingkai. Responsif: tabel dalam `overflow-x-auto`, tanpa scroll horizontal halaman pada lebar telepon; gutter 16px dari layout. Aksesibilitas: tabel punya `caption`, tombol aksi punya `aria-label` provider+host, pil tidak mengandalkan warna saja (teks "Tersimpan"/"Belum ada").
