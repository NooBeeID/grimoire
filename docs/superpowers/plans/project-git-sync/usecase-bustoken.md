# T14 / core/git-credential / bustoken — Plan Usecase

Tiket: `docs/tickets/T14-project-git-sync.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync/overview.md` (bagian 5.1, 5.2, 6.1, 6.4.1, 10.4).
Konteks yang dicek: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan, tanpa migrasi); `rules.txt` bagian 4, 6 (6.3, 6.7, 6.9, 6.10), 7. Pola acuan kode: `backend/internal/domain/core/internal/auth/getuserbus/{init,handler,usecase,repository}.go`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | backend saja (tanpa route REST, tanpa FE) |
| Domain / Module / Feature | core / git-credential (`gitcredential`) / `bustoken` |
| Folder | `backend/internal/domain/core/internal/gitcredential/bustoken` |
| Endpoint | bus `core.gitcredential.get-token` (konstanta `BUS_CORE_GITCREDENTIAL_GET_TOKEN`) |
| Method | bus `Call` (sinkron; Publish dilarang karena hasilnya dibutuhkan pemanggil dan membawa rahasia) |
| Version | kontrak v1 (baru) |
| Kontrak | `backend/lib/contracts/core/gitcredential/gettoken` (`Request{UserID, Provider, Host}`, `Response{Found, Unreadable, Disabled, Token secret.String}`), dibuat di FOUNDATION langkah 0 butir 8 |
| Consumer | project/gitsync `runsync` (wajib), `getconfig` (fail-soft, hanya memakai `Found/Unreadable/Disabled`, abaikan `Token`) |
| Ketergantungan | FOUNDATION: `lib/common/secret`, `infra/crypto/secretbox`, konstanta bus, kontrak `gettoken`, migrasi `git_credentials`, `gitcredential/base` (`ParseProvider`, `AAD`, `store.ValidUUID`) |

---

## 2. Summary Plan

Handler bus read-only yang membuka PAT milik satu user untuk module lain. Alur:

1. `bus.Expect[gettoken.Request]` — tipe salah (string, pointer `*gettoken.Request`) -> error jelas `bus.ErrBadRequestType` (rule 6.9).
2. Validasi bentuk: `UserID` uuid valid, `Provider` dikenal (`github`/`gitlab`), `Host` tidak kosong; selain itu `*errs.Error` `CodeInvalid` (error Go, bukan `Found=false`; rule 6.7).
3. Fitur nonaktif (`GIT_CREDENTIAL_KEY` kosong, `box == nil`) -> `Response{Disabled: true}` tanpa menyentuh DB.
4. `repo.Get(userID, provider, host)`: satu SELECT `token_enc` dengan `WHERE user_id AND provider AND host`. Tidak ada baris -> `Response{Found: false}`.
5. `box.Open(blob, AAD(userID, provider, host))` gagal (kunci beda/blob rusak/AAD beda) -> `Response{Found: true, Unreadable: true}` + log `warn` tanpa isi blob/token. Ini skenario "kunci hilang", BUKAN error 500.
6. Sukses -> `Response{Found: true, Token: secret.New(plain)}`.

Jaminan keamanan (anti-tujuan):
- Hanya mengembalikan token milik `Request.UserID`. Query selalu membawa `user_id`. Pemanggil (gitsync) WAJIB mengisi `UserID` dari identity request (`actor.ID`), bukan dari `last_synced_by` atau data tersimpan; handler tidak membaca identity dari ctx dan tidak punya jalur "cari token user lain".
- Token tidak pernah di-log, tidak diaudit (handler ini tidak menulis audit sama sekali), tidak di-`Publish`, tidak masuk pesan error. Satu-satunya tempat plaintext adalah `secret.String` pada `Response`; plaintext `[]byte` hasil `Open` langsung dibungkus dan tidak disimpan di variabel yang hidup lama.
- `repository` hanya meng-SELECT `token_enc` (tidak `last4`, tidak `SELECT *`).

---

## 3. Plan (langkah berurutan)

Prasyarat: FOUNDATION selesai (commit langkah 0). Tidak ada file yang dibagi dengan usecase lain kecuali satu baris `Init` di `gitcredential/base.go` dan satu baris di tes registrasi module.

### Langkah 1 — Antarmuka dependensi (`usecase.go`)

Interface kecil milik feature (consumer-defined):

```go
// opener dipenuhi *secretbox.Box (dan fake di tes).
type opener interface {
	Open(blob, aad []byte) ([]byte, error)
}

// tokenReader dipenuhi Repository (dan fake di tes).
type tokenReader interface {
	Get(ctx context.Context, userID string, p base.Provider, host string) (blob []byte, found bool, err error)
}
```

### Langkah 2 — `repository.go`

```go
type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

const getSQL = `SELECT token_enc FROM git_credentials
                WHERE user_id = $1 AND provider = $2 AND host = $3`

func (r *Repository) Get(ctx context.Context, userID string, p base.Provider, host string) ([]byte, bool, error)
```

- Satu-satunya yang menyentuh SQL (rule 4.x). `sql.ErrNoRows` -> `(nil, false, nil)`. Error lain dibungkus `fmt.Errorf("bustoken: get: %w", err)` (tanpa parameter SQL di pesan).
- Memakai helper `base/store` bila foundation menyediakan `Get` bersama; bila tidak, query di file ini (tidak ada duplikasi lintas feature karena hanya bustoken yang membaca `token_enc`).
- Tidak ada tx; satu query read-only.

### Langkah 3 — `usecase.go`

```go
type Usecase struct {
	repo tokenReader
	box  opener // nil interface = fitur nonaktif
	log  *slog.Logger
}

func NewUsecase(repo tokenReader, box opener, log *slog.Logger) *Usecase
func (u *Usecase) Execute(ctx context.Context, in gettoken.Request) (gettoken.Response, error)
```

Catatan nil: `Init` harus meneruskan `nil` interface murni (bukan `(*secretbox.Box)(nil)` yang membungkus ke interface non-nil). Di `Init`: `var o opener; if sh.Box != nil { o = sh.Box }`. Tes menutup jebakan ini (kasus 7 di bagian 6.2).

Urutan `Execute` (orkestrasi saja; rule bisnis ada di entity `base`):
1. `base.ValidUUID(in.UserID)` -> bukan uuid: `&errs.Error{Code: errs.CodeInvalid, Message: "user_id tidak valid"}` (tanpa echo nilai).
2. `p, err := base.ParseProvider(in.Provider)` -> `ErrInvalidProvider` (dibungkus bila perlu, tetap `*errs.Error`). Catatan: `ParseProvider` menerima case-insensitive; di sini Provider hasil parse (lowercase) dipakai untuk query dan AAD.
3. `host := strings.ToLower(strings.TrimSpace(in.Host))`; kosong -> `CodeInvalid` "host wajib diisi". (Bentuk host yang lain tidak diperiksa di sini; host yang tidak pernah disimpan hanya menghasilkan `Found=false`. Normalisasi otoritatif adalah `base.NormalizeHost` di jalur simpan dan di gitsync.)
4. `if u.box == nil { return gettoken.Response{Disabled: true}, nil }`.
5. `blob, found, err := u.repo.Get(ctx, in.UserID, p, host)`; err -> return `gettoken.Response{}, err` (infra error, tanpa Token). `!found` -> `Response{Found: false}`.
6. `plain, err := u.box.Open(blob, base.AAD(in.UserID, p, host))`; err -> `u.log.WarnContext(ctx, "git credential unreadable", "user_id", in.UserID, "provider", string(p), "host", host)` (TANPA `err`, blob, atau token; `ErrDecrypt` tidak informatif lagi) -> `Response{Found: true, Unreadable: true}, nil`.
7. `return gettoken.Response{Found: true, Token: secret.New(string(plain))}, nil`.

Larangan eksplisit di file ini: tidak mengimpor `core/audit/record`, tidak memanggil `bus.Publish`, tidak ada `slog` dengan field `token`/`blob`/`plain`.

### Langkah 4 — `handler.go`

```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[gettoken.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in) // mengembalikan NILAI gettoken.Response
}
```

Mengembalikan nilai (bukan pointer) agar `bus.CallTyped[gettoken.Response]` bekerja.

### Langkah 5 — `init.go`

```go
func Init(d app.Deps, sh base.Shared) {
	var o opener
	if sh.Box != nil {
		o = sh.Box
	}
	h := NewHandler(NewUsecase(NewRepository(d.DB), o, d.Log))
	d.Bus.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, h.Handle)
}
```

`base.Shared` adalah struct bersama module gitcredential yang dirakit di `gitcredential/base.go` dari `d.Config` (overview D13: `Box *secretbox.Box` nil bila `!cfg.GitFeatureEnabled()`; `Git *gitprovider.Client`). Bila foundation belum mendefinisikan `Shared`, tambahkan di `gitcredential/base/shared.go` hanya field yang dibutuhkan feature (`Box`, `Git`) — perubahan aditif kecil, dikoordinasikan dengan usecase `savecredential` yang membutuhkan struct yang sama (jangan buat dua definisi).

### Langkah 6 — Wiring module

- `backend/internal/domain/core/internal/gitcredential/base.go` `Register`: tambah satu baris `bustoken.Init(d, sh)`.
- Tes registrasi bus module (`gitcredential/base_test.go`): tambah satu asersi bahwa `core.gitcredential.get-token` terdaftar tepat sekali (register ganda = panic, jadi tes memanggil `Register` pada registry baru dan memeriksa `Call` tidak mengembalikan error "endpoint tidak terdaftar").

### Langkah 7 — Katalog bus `docs/tickets/README.md`

FOUNDATION langkah 16 menambah tiga baris katalog. Pastikan baris ini ada (tambahkan bila belum, tanpa mengubah baris lain):

`| core.gitcredential.get-token | Call | T14 | T14 (gitsync runsync, getconfig) |`

Instruksi tiket: consumer yang dicantumkan `T14 (gitsync runsync)`; `getconfig` juga memanggil (fail-soft), ikuti teks overview. Tambahkan juga satu kalimat di paragraf lokasi kontrak: "Kontrak `core.gitcredential.get-token` di `lib/contracts/core/gitcredential/gettoken`; `Response.Token` (`secret.String`) tidak boleh di-log, diaudit, di-Publish, atau dikirim ke klien; pemanggil wajib mengisi `UserID` dari identity request."

### Langkah 8 — Verifikasi

`go build ./...`, `go vet`, `go test ./backend/internal/domain/core/internal/gitcredential/bustoken/...`, `go test -tags integration` pada paket yang sama, `backend/internal/archtest` (bustoken tidak mengimpor module lain; `gitsync` tidak mengimpor `gitcredential`).

---

## 4. Migration

Tidak ada. Tabel `git_credentials` dibuat oleh FOUNDATION (`backend/migrations/core/<VERSION>_create_git_credentials.sql`, overview bagian 7.1). Usecase ini hanya membaca.

---

## 5. Changes

| File | Jenis | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/gitcredential/bustoken/init.go` | baru | `Init(d, sh)`: registrasi handler, penanganan nil box |
| `.../bustoken/handler.go` | baru | `Handler.Handle` dengan `bus.Expect` |
| `.../bustoken/usecase.go` | baru | `Usecase.Execute`, interface `opener`/`tokenReader` |
| `.../bustoken/repository.go` | baru | `Repository.Get` (SELECT `token_enc`) |
| `.../bustoken/handler_test.go` | baru | tes unit handler + usecase dengan fake (bagian 6.1) |
| `.../bustoken/repository_integration_test.go` | baru (`//go:build integration`) | isolasi per user/host (bagian 6.2) |
| `backend/internal/domain/core/internal/gitcredential/base.go` | ubah | +1 baris `bustoken.Init(d, sh)` |
| `backend/internal/domain/core/internal/gitcredential/base_test.go` | ubah | +asersi bus terdaftar |
| `docs/tickets/README.md` | ubah | baris katalog bus + kalimat kontrak (bila belum dari foundation) |

Tidak berubah: `app.Deps`, kontrak `gettoken` (sudah dari foundation), migrasi, OpenAPI (tidak ada route REST), FE.

---

## 6. Test Scenario

Fake: `fakeRepo` (mengembalikan blob/found/err, merekam argumen `userID/provider/host`), `secretbox.New` nyata dengan kunci uji 32 byte (bukan fake `opener`, agar enkripsi/AAD benar-benar diuji). Logger uji: `slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))`.

### 6.1 `handler_test.go` (unit)

Helper `newBus(repo, box)` membuat `bus.Registry`, mendaftarkan `NewHandler(...).Handle` pada `constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN`. Blob uji dibuat `box.Seal([]byte(canary), base.AAD(uid, base.GitHub, "github.com"))`. `canary = "ghp_CANARY0123456789abcdefCANARY"`.

| # | Kasus | Ekspektasi |
|---|---|---|
| 1 | Tipe request salah: `"string"`, `&gettoken.Request{}` (pointer), `struct{}` | `errors.Is(err, bus.ErrBadRequestType)`, pesan memuat `want=gettoken.Request`; repo tidak dipanggil |
| 2 | Sukses | `CallTyped` -> `Found=true, Unreadable=false, Disabled=false`, `Token.Reveal()==canary`; repo menerima persis `(uid, github, github.com)` |
| 3 | Not found (repo `found=false`) | `Found=false`, `Token.IsZero()`, err nil |
| 4 | Unreadable karena kunci beda: blob disegel kunci A, handler memakai kunci B | `Found=true, Unreadable=true`, `Token.IsZero()`, err nil (bukan error/500); log berisi `warn` + `user_id/provider/host` |
| 5 | Unreadable karena AAD beda: blob disegel untuk user lain / host lain lalu ditaruh di baris ini | sama seperti 4 (mencegah pemindahan blob antar user) |
| 6 | Unreadable karena blob rusak/terpotong (`[]byte{1,2,3}`) | sama seperti 4 |
| 7 | Disabled: `Init`/`NewUsecase` dengan box nil | `Disabled=true`, `Found=false`, repo TIDAK dipanggil (cek `fakeRepo.calls==0`); termasuk lewat `Init` dengan `Shared{Box: nil}` (jebakan nil-interface) |
| 8 | Validasi bentuk: `UserID` `""`/`"x"`/`"not-a-uuid"` | `*errs.Error` `CodeInvalid`; repo tidak dipanggil |
| 9 | Provider `"bitbucket"`, `""` | `*errs.Error` (`ErrInvalidProvider`, 400); repo tidak dipanggil |
| 10 | Host `""` dan `"   "` | `CodeInvalid`; repo tidak dipanggil |
| 11 | Provider/host kapital dan spasi (`"GitHub"`, `" GitHub.com "`) | repo menerima `github` + `github.com` (dinormalkan ringan), sukses |
| 12 | Disabled lebih diprioritaskan setelah validasi: request invalid + box nil | tetap error validasi (urutan spek: validasi -> disabled) |
| 13 | Error infra repo (`errors.New("db down")`) | err dikembalikan (dibungkus `%w`), `Response` kosong, `Token.IsZero()` |
| 14 | PAT canary | Pada kasus 2, 4, 5, 6, 13: buffer log TIDAK memuat `canary`, tidak memuat byte blob (hex/base64), tidak memuat `[REDACTED]` selain bila ada field Token; `fmt.Sprintf("%v %+v %#v", res)` tidak memuat canary; tidak ada `core.audit.record` yang dipanggil (registry uji tidak punya handler audit; panggilan ke audit akan gagal "tidak terdaftar" -> tes gagal) |
| 15 | Registrasi ganda | `Init` dua kali pada registry sama -> panic (perilaku registry, 6.1) — cukup di `base_test.go`, bukan di sini |

### 6.2 `repository_integration_test.go` (tag `integration`, Postgres nyata)

Setup: `db.TestDB(t)`, `db.Migrate(..., migrations.FS, nil)`; helper `seedUser(t, db)` (INSERT langsung ke `users`, kolom minimal sesuai migrasi auth) dan `seedCred(t, db, userID, provider, host, blob)` (INSERT langsung ke `git_credentials`, mengisi `last4='abcd'`; blob = `Seal` sungguhan agar `octet_length(token_enc) > 28`). Tidak memakai feature `savecredential` (menghindari ketergantungan antar-usecase).

| # | Kasus | Ekspektasi |
|---|---|---|
| 1 | User A punya (github, github.com); `Get(A, github, github.com)` | blob sama dengan yang disimpan, `found=true` |
| 2 | Isolasi per user: A dan B masing-masing punya token berbeda untuk (github, github.com) | `Get(A)` mengembalikan blob A, `Get(B)` blob B; tidak pernah silang |
| 3 | User C tanpa token, A punya | `Get(C,...)` -> `found=false` (tidak jatuh ke token A) |
| 4 | Isolasi per host: A punya gitlab `gitlab.com` dan gitlab `git.noobee.id` | `Get(A, gitlab, git.noobee.id)` blob host itu; `Get(A, gitlab, other.host)` `found=false` |
| 5 | Isolasi per provider: A punya gitlab `gitlab.com` saja | `Get(A, github, github.com)` `found=false` |
| 6 | uuid valid tetapi user tidak ada | `found=false`, tanpa error |
| 7 | Ujung ke ujung lewat bus dengan `Init(app.Deps{DB, Bus, Log}, base.Shared{Box})`: seed dengan kunci K1; `CallTyped` dengan K1 -> `Token` benar; registry baru dengan K2 -> `Unreadable=true` (skenario kunci berganti); registry tanpa box -> `Disabled=true`; request A tidak pernah mengembalikan token B | sesuai |
| 8 | PAT canary di DB: setelah seed, `SELECT token_enc::text` tidak memuat plaintext canary (bukti terenkripsi) dan handler tidak menulis baris `audit_log` (hitung baris sebelum = sesudah, bila tabel ada di skema uji) | sesuai |

### 6.3 Arch/test lain

- `archtest` hijau: `bustoken` hanya mengimpor `gitcredential/base`, `lib/contracts/core/gitcredential/gettoken`, `lib/common/secret`, `lib/common/errs`, `lib/constants`, `infra/{app,bus,db}`.
- Tidak ada test OpenAPI karena tidak ada route REST (`make api-check` tidak terpengaruh).

---

## 7. E2E Scenarios

Handler ini tidak punya endpoint HTTP dan tidak ada FE; tidak ada spec Playwright sendiri. Perilakunya diverifikasi tidak langsung lewat E2E gitsync (`frontend/e2e/git-sync.spec.ts`, overview bagian 11):

Positif:
1. Setelah user menyimpan token sah (Profil), `Sync dari Git` berhasil: `runsync` memanggil bus dengan `UserID` = user yang login dan `Found=true`; sync memakai token milik user itu.
2. `GET /api/projects/{id}/git-sync` menampilkan `has_credential=true` setelah simpan (`getconfig` membaca `Found`).

Negatif:
1. User tanpa token -> run: 422 `CREDENTIAL_MISSING` (dari `Found=false`); `getconfig` `has_credential=false`.
2. User B (anggota project, tanpa token) tidak pernah memakai token user A yang membuat config: run oleh B -> `CREDENTIAL_MISSING`, config A tidak berubah (bukti `UserID` dari identity request).
3. Backend dijalankan dengan `GIT_CREDENTIAL_KEY` berbeda dari saat token disimpan (varian e2e/integrasi runsync) -> `CREDENTIAL_UNREADABLE` dan `getconfig.credential_unreadable=true`, bukan 500.
4. Backend tanpa `GIT_CREDENTIAL_KEY` -> `GIT_FEATURE_DISABLED` (503) di run, `feature_enabled=false` di getconfig.
5. Tes PAT canary E2E: token `ghp_e2eValidToken7Qx2` tidak muncul di respons/DOM/log backend pada seluruh skenario di atas.

---

## 8. Request/Response body

Tidak ada body HTTP. Kontrak bus (bentuk Go; request dikirim sebagai NILAI):

```go
// Request
gettoken.Request{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Provider: "github", Host: "github.com"}

// Response
gettoken.Response{Found: true, Token: secret.New("ghp_...")}   // sukses
gettoken.Response{Found: false}                                  // belum disimpan
gettoken.Response{Found: true, Unreadable: true}                 // kunci berganti / blob rusak
gettoken.Response{Disabled: true}                                // GIT_CREDENTIAL_KEY kosong
```

Error Go (hanya request invalid / kegagalan infra):

```go
bus.ErrBadRequestType // tipe request salah: "bus: bad request type: want=gettoken.Request got=string"
&errs.Error{Code: errs.CodeInvalid, Message: "user_id tidak valid"}
&errs.Error{Code: errs.CodeInvalid, Message: "host wajib diisi"}
base.ErrInvalidProvider // *errs.Error 400
fmt.Errorf("bustoken: get: %w", dbErr)
```

Log (satu-satunya keluaran log, level warn, tanpa isi rahasia):

```json
{"level":"WARN","msg":"git credential unreadable","user_id":"0192f5a0-aaaa-7aaa-8bbb-00000000000a","provider":"github","host":"github.com","trace_id":"4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"}
```

Pemetaan oleh consumer (gitsync `runsync`, rujukan; bukan tanggung jawab bustoken):

| Response | Hasil di REST |
|---|---|
| `Disabled` | 503 `GIT_FEATURE_DISABLED` |
| `!Found` | 422 `CREDENTIAL_MISSING` |
| `Found && Unreadable` | 422 `CREDENTIAL_UNREADABLE` |
| `Found && !Unreadable` | lanjut sync memakai `Token.Reveal()` hanya di dalam `gitprovider` |

---

## 9. Risiko dan catatan

- Kunci hilang/berganti: seluruh PAT menjadi `Unreadable` (bukan 500); user diminta simpan ulang. Didokumentasikan di `docs/configuration.md` oleh foundation.
- Jebakan nil-interface pada `opener` ditangani di `Init` dan diuji (kasus 6.1 #7).
- Tanpa cache token: setiap sync melakukan satu SELECT + satu AES-GCM open, murah dan menghindari plaintext hidup lama di memori.
- Tanpa rate limit/audit pada handler ini (audit `git_sync.run` dicatat oleh `runsync`, tanpa token).
