# T00 Foundation: Usecase `architecture-guard`

Tiket: `docs/tickets/T00-foundation.md` (F9, kriteria T00-AC4). Overview: `docs/superpowers/plans/foundation/overview.md` (bagian 3.1, 3.3, 7 langkah 8, 11, 13). Aturan: `rules.txt` bagian 3.3, 4.1 sampai 4.7, 12.3 sampai 12.5, 13.5, 15.4.

Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. `rules.txt` bagian 4 tidak punya nomor untuk "lib mengimpor internal" (hanya 4.5 untuk `lib/contracts`); usecase ini menambahkan 4.9.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | `architecture-guard` |
| Scope | fullstack (Go test, konfigurasi dependency-cruiser, skrip shell, CI). Tanpa SQL, tanpa UI |
| Domain | infra |
| Module | foundation (bukan module domain; pola `init/handler/params/usecase/repository` tidak berlaku) |
| Resource root | `backend/internal/infra` (kode produksi tidak disentuh; artefak utama di `backend/internal/archtest`) |
| Endpoint | tidak ada |
| Method | tidak ada |
| Version | tidak ada (tanpa prefix versi). `api/openapi.yml` hasil bundle mengikuti `info.version` di `api/root.yml` (`0.1.0`) |
| Bergantung pada | langkah Go: `repo-skeleton-config` (go.mod, folder, stub domain). Langkah OpenAPI: `http-server` (`api/modules/infra/foundation.yml`). Langkah frontend: `static-frontend-scaffold` (`frontend/package.json`, `tsconfig.json`, struktur `src/`). Final/hijau penuh setelah `cli-and-wiring` |
| Dibutuhkan oleh | semua tiket T01 sampai T09 (gerbang CI) |

Endpoint bus yang dimiliki: **tidak ada**. Tidak ada kontrak bus, konstanta, atau aksi audit baru.

---

## 2. Summary Plan

Membuat pagar otomatis yang membuat build gagal bila batas module dilanggar, plus dua pagar pendamping dan satu pipeline CI:

1. **`backend/internal/archtest`**. Fungsi murni `Check(imports []Import) []Violation` tanpa I/O (mudah dites tanpa mock, rule 5.1/13.1). Test terpisah memindai repo nyata lewat `golang.org/x/tools/go/packages` (termasuk file `_test.go` dan build tag `integration`) lalu memanggil `Check`. Repo bersih harus nol pelanggaran.
2. **`frontend/.dependency-cruiser.cjs`** dengan skrip `npm run test:arch` yang membuktikan config benar-benar menolak fixture pelanggar dan meloloskan fixture bersih.
3. **Tooling OpenAPI**: `scripts/api-bundle.sh`, `make api-bundle`, `make api-check` (bundle ke file sementara lalu `diff`; gagal bila basi atau diedit manual; juga gagal bila ada `api/modules/**.yml` yang tidak direferensikan `api/root.yml`), `make api-selftest` (membuktikan `api-check` benar-benar gagal).
4. **CI** `.github/workflows/ci.yml`: `lint`, `test`, `test-integration` (service postgres), `api-check`, `depcruise`, `build`, `e2e`.

Keputusan desain yang mengikat:

1. **Satu usecase, satu PR.** Go archtest, depcruise, OpenAPI tooling, dan CI satu kesatuan "penegakan" (F9). Tidak dipecah. Pengerjaan tetap berurutan di dalam PR (bagian 3) karena sebagian langkah menunggu usecase lain.
2. **`Check` murni.** Masukan daftar edge `Import{From, To}` berisi import path penuh; tidak membaca disk, tidak tahu go/packages. Pemindai ada di file test (`scan_test.go`), jadi `x/tools` tidak pernah ter-link ke binary produksi.
3. **Satu edge, paling banyak satu pelanggaran.** Urutan evaluasi menentukan nomor aturan yang dilaporkan (bagian 3 langkah 1). Edge duplikat (mis. dari varian test) dibuang, hasil diurutkan deterministik (Rule, From, To).
4. **Rule 4.4 (consumer hanya boleh import constants/contracts/bus) tidak punya cek tersendiri.** "Consumer" dan "provider" adalah peran per endpoint, bukan properti path. Dalam praktik consumer yang mengimpor provider selalu melintasi module atau domain, sehingga tertangkap 4.1/4.2. Pesan 4.1 dan 4.2 sengaja menyebut "lihat rules.txt 4.4". Test `Rule44` memastikan hal ini.
5. **`lib/contracts` boleh mengimpor `lib/contracts/**` dan `lib/common/**`**, serta stdlib dan library pihak ketiga. Dilarang: `internal/**`, `cmd/**`, `lib/constants`, `lib/util`. Alasan mengizinkan sesama contracts: tipe bersama antar kontrak; menyamakan "lib non-common" di tiket dengan "selain common dan contracts".
6. **Rule baru 4.9** di `rules.txt`: "lib/** DILARANG meng-import internal/** dan cmd/**. lib netral". `lib/contracts` yang melanggar tetap dilaporkan sebagai 4.5 (lebih spesifik); lib lain dilaporkan 4.9.
7. **Tes ikut memindai `_test.go`** (varian `pkg [pkg.test]` dan `pkg_test`) dan build tag `integration`, agar test integrasi tidak menjadi jalur bocor.
8. **Tidak ada pengecualian/allowlist.** Pelanggaran diperbaiki, bukan di-whitelist. Menambah pengecualian = ubah `rules.txt` (rule 15.3).
9. **Tidak dicakup (sengaja):** 4.7 (cmd tanpa logika bisnis), 4.8 (arah handler -> usecase -> repository), 5.x (layering di dalam feature), dan impor base/ ke feature. Tidak bisa dideteksi dari graf import saja; tetap lewat review. Dicatat di komentar dokumentasi `rules.go`.
10. **Versi tool dipin** dan alasannya dicatat di PR (rule 15.4): `golang.org/x/tools` (pemindai paket resmi Go), `dependency-cruiser` (aturan impor FE berbasis regex path), `@redocly/cli` (bundle OpenAPI, dijalankan lewat `npx` dengan versi eksak di `scripts/api-bundle.sh`).
11. **Redocly lewat `npx` dengan versi pin di skrip**, bukan devDependency frontend, agar tooling API tidak bergantung pada `npm ci` frontend. Konsekuensi: job CI `api-check` hanya butuh setup-node. (Penyimpangan kecil dari overview bagian 11, yang membolehkan keduanya; pin ada di satu tempat, `scripts/api-bundle.sh`, bukan makefile.)

---

## 3. Plan (langkah berurutan)

Urutan: A (Go, bisa mulai setelah `repo-skeleton-config`), B (OpenAPI, setelah `http-server` menghasilkan `foundation.yml`), C (frontend, setelah `static-frontend-scaffold`), D (CI), E (dokumen), F (verifikasi akhir setelah `cli-and-wiring`). A, B, C independen satu sama lain bila dependensinya terpenuhi.

### A. Backend: `internal/archtest`

#### Langkah A1: aturan murni (TDD: tulis `rules_test.go` dulu)

Buat `backend/internal/archtest/rules.go` (package `archtest`, tanpa import selain stdlib `fmt`, `sort`, `strings`).

```go
package archtest

// ModulePath harus sama dengan baris `module` di backend/go.mod (dijaga TestModulePathMatchesGoMod).
const ModulePath = "grimoire"

// ID aturan = nomor di rules.txt. Dijaga TestRuleCatalog_ExistsInRulesTxt.
const (
    Rule41 = "4.1" // domain -> domain lain
    Rule42 = "4.2" // module -> module lain (satu domain atau lintas domain tidak berlaku, lihat 4.1)
    Rule43 = "4.3" // feature -> feature lain dalam module
    Rule45 = "4.5" // lib/contracts -> internal/**, cmd/**, lib/constants, lib/util
    Rule46 = "4.6" // infra -> domain
    Rule49 = "4.9" // lib/** (selain contracts) -> internal/**, cmd/**
)

type Import struct{ From, To string } // import path penuh, mis. "grimoire/internal/infra/bus"

type Violation struct {
    Rule, From, To, Reason string
}

// String: "rules.txt 4.2: <From> imports <To> (<Reason>)"
func (v Violation) String() string

// Check murni. Edge ke luar ModulePath (stdlib, pihak ketiga) dan edge From == To diabaikan.
// Hasil bebas duplikat dan terurut (Rule, From, To).
func Check(imports []Import) []Violation
```

Klasifikasi (fungsi privat `classify(path string) loc`), `loc{kind, domain, module, feature, lib string}`:

| Path (setelah `grimoire/`) | kind | Field |
|---|---|---|
| bukan `grimoire` atau `grimoire/...` (cek dengan pemisah `/`, jadi `grimoire-x` bukan internal) | external | |
| `internal/domain/<D>` (+ subpath non-`internal`) | domain | domain=D |
| `internal/domain/<D>/internal/<M>` | domain | domain=D, module=M |
| `internal/domain/<D>/internal/<M>/base[/...]` | domain | module=M, feature="" (base bukan feature) |
| `internal/domain/<D>/internal/<M>/<F>[/...]`, F != `base` | domain | module=M, feature=F (subpackage seperti `<F>/repository` ikut feature F) |
| `internal/infra[/...]` | infra | |
| `internal/<lain>` (mis. `archtest`) | internalOther | |
| `lib/contracts[/...]` | contracts | |
| `lib/<x>[/...]` | lib | lib=x (`common`, `constants`, `util`) |
| `cmd[/...]` | cmd | |
| lainnya (mis. root `grimoire`) | external | |

Urutan evaluasi per edge (berhenti di pelanggaran pertama):

1. From domain dan To domain:
   - domain berbeda -> **4.1**, reason `domain -> domain lain; berinteraksi lewat infra/bus + lib/contracts, lihat rules.txt 4.4`.
   - domain sama, kedua sisi punya module, module berbeda -> **4.2**, reason `module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4`. (Edge dari root domain `domain/<D>` ke module diizinkan: itu wiring domain, rule 5.5. Edge module ke root domain diabaikan, Go sudah menolak siklusnya.)
   - module sama, kedua sisi punya feature, feature berbeda -> **4.3**, reason `feature -> feature lain dalam module; bagi lewat base/`. (Feature -> `base/`, module root -> feature, dan feature -> feature sama diizinkan.)
2. From infra dan To domain -> **4.6**, reason `infra -> domain; infra tidak tahu bisnis`.
3. From contracts dan To in {domain, infra, internalOther, cmd, lib dengan lib in {constants, util}} -> **4.5**, reason `lib/contracts hanya boleh mengimpor stdlib, lib/common, lib/contracts`. (To = contracts atau lib/common diizinkan.)
4. From lib (common, constants, util) dan To in {domain, infra, internalOther, cmd} -> **4.9**, reason `lib -> internal/cmd; lib harus netral`.
5. Selain itu diizinkan. Khusus yang harus tetap lolos: domain -> infra, domain -> lib/**, cmd -> apa saja, domain root -> module, module root -> base sendiri, module root -> feature sendiri, feature -> base sendiri, infra -> infra, infra -> lib, internalOther (`archtest`) -> apa saja.

Bagian komentar `rules.go` berisi tabel aturan di atas dan daftar yang sengaja tidak dicakup (keputusan 9).

#### Langkah A2: pemindai repo (file test, bukan produksi)

Tambahkan dependensi: dari `backend/`, `go get golang.org/x/tools@<versi>` lalu `go mod tidy`. Pilih versi stabil yang kompatibel dengan directive `go 1.23` di `go.mod`; bila `go get` menaikkan directive `go`, mundurkan ke versi `x/tools` yang lebih lama. Catat alasan di PR (rule 15.4).

`backend/internal/archtest/scan_test.go` (package `archtest`):

```go
func scanImports(t testing.TB, dir string) []Import
```

- `packages.Load(&packages.Config{Mode: NeedName|NeedImports|NeedFiles, Dir: dir, Tests: true, BuildFlags: []string{"-tags=integration"}, Env: append(os.Environ(), "GOWORK=off")}, "./...")`.
- Normalisasi `pkg.PkgPath`: potong akhiran ` [...]` (varian test), potong akhiran `_test` (package eksternal), lewati yang berakhiran `.test` (main hasil generate).
- Untuk tiap key di `pkg.Imports` buat `Import{From: norm(pkg.PkgPath), To: key}`; dedupe.
- `pkg.Errors` bertipe `ListError` atau `ParseError` -> `t.Fatalf` dengan pesan error (jangan diam-diam lulus). Kegagalan `packages.Load` -> `t.Fatalf`.

`backend/internal/archtest/repo_test.go`:

- `repoRoot` dicari dari `runtime.Caller(0)` (`backend/internal/archtest` -> naik dua level ke `backend/`; `rules.txt` satu level di atasnya).
- `TestT00_AC4_RepoIsClean`: `scanImports(backend)` lalu `Check`; ada pelanggaran -> `t.Errorf` satu baris per `Violation.String()`. Guard anti-lulus-kosong: edge yang dipindai harus memuat paket `grimoire/internal/archtest` dan `grimoire/internal/domain/core` (atau lebih tepat: set `From` mengandung keduanya).
- `TestModulePathMatchesGoMod`: baca baris `module` di `backend/go.mod` == `ModulePath`.
- `TestRuleCatalog_ExistsInRulesTxt`: untuk tiap `Rule41..Rule49` regex `(?m)^<id>\s` ada di `rules.txt` (mencegah nomor melenceng saat `rules.txt` dirapikan).
- `TestT00_AC4_ScanFindsViolationsInTempModule`: bukti jalur nyata end-to-end. Buat modul sementara di `t.TempDir()`: `go.mod` (`module grimoire`, `go 1.23`) dan paket stdlib-only berikut, masing-masing satu file; yang melanggar mengimpor target sesuai kolom:

  | File (paket) | Mengimpor | Aturan yang harus muncul |
  |---|---|---|
  | `internal/domain/project/a.go` | `grimoire/internal/domain/core` | 4.1 |
  | `internal/domain/project/internal/apikey/x.go` | `.../internal/knowledge/base` | 4.2 |
  | `internal/domain/project/internal/knowledge/upload/u.go` | `.../knowledge/search` | 4.3 |
  | `internal/domain/core/internal/auth/login/l_test.go` (file test) | `.../auth/logout` | 4.3 (membuktikan file `_test.go` dipindai) |
  | `internal/infra/bus/b.go` | `grimoire/internal/domain/core` | 4.6 |
  | `lib/contracts/core/c.go` | `grimoire/internal/infra/bus` | 4.5 |
  | `lib/util/u.go` | `grimoire/internal/infra/bus` | 4.9 |
  | `internal/domain/core/internal/auth/withtag/w.go` dengan `//go:build integration` | `grimoire/internal/domain/project` | 4.1 (membuktikan tag `integration` dipindai) |

  Hasil `Check(scanImports(tmp))` harus memuat tepat set tuple (Rule, From, To) di atas, tidak lebih.

#### Langkah A3: `backend/.golangci.yml`

Belum ada pemilik di usecase lain, jadi dibuat di sini (CI `lint` membutuhkannya):

```yaml
version: "2"
run:
  timeout: 5m
linters:
  default: standard        # errcheck, govet, ineffassign, staticcheck, unused
  enable: [errorlint, bodyclose, noctx, sqlclosecheck, rowserrcheck]
formatters:
  enable: [gofmt, goimports]
```

Bila implementer memilih golangci-lint v1, tulis ulang ke format v1 dan sesuaikan `version` di CI. Jangan menambah linter arsitektur (depguard) di sini: satu sumber penegakan, yaitu `archtest`.

### B. Tooling OpenAPI

Prasyarat: `api/modules/infra/foundation.yml` ada (dari `http-server`). Format file modul: fragmen dengan kunci level atas `paths` dan `components` saja (bukan dokumen OpenAPI penuh).

#### Langkah B1: `api/root.yml`

```yaml
openapi: 3.1.0
info:
  title: NooBee Grimoire API
  version: 0.1.0
servers:
  - url: /
paths:
  /healthz:
    $ref: ./modules/infra/foundation.yml#/paths/~1healthz
  /readyz:
    $ref: ./modules/infra/foundation.yml#/paths/~1readyz
```

Konvensi untuk tiket lain (ditulis di `docs/tickets/README.md`, langkah E2): tiap module menambah `$ref` path-nya ke `api/root.yml` di PR yang sama; komponen bersama (`Error`, responses) dirujuk dari `foundation.yml`.

Spike wajib sebelum lanjut: jalankan bundle dan periksa hasil. Syarat: tidak ada `$ref` ke file eksternal tersisa, `/healthz`, `/readyz`, dan `components.schemas.Error` ada, `components.responses.*` bernama wajar. Bila redocly menamai komponen hasil hoisting secara aneh atau menyisakan ref ke file, tambahkan `--dereferenced` ke perintah bundle dan ulangi. Jalankan bundle dua kali dan pastikan keluaran identik (determinisme, syarat `api-check`).

#### Langkah B2: skrip

Semua berkas `scripts/*.sh` executable (`chmod +x`), `#!/usr/bin/env bash`, `set -euo pipefail`, `cd "$(dirname "$0")/.."` (kerja dari root repo).

`scripts/api-bundle.sh <output>`:
```bash
REDOCLY_VERSION="${REDOCLY_VERSION:-X.Y.Z}"   # pin eksak; isi hasil `npm view @redocly/cli version` saat implementasi, catat di PR
out="${1:?usage: api-bundle.sh <output>}"
tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
npx --yes "@redocly/cli@${REDOCLY_VERSION}" bundle api/root.yml -o "$tmp"      # tambah --dereferenced bila spike B1 mengharuskan
{ printf '%s\n' '# GENERATED by scripts/api-bundle.sh from api/root.yml + api/modules/**. DO NOT EDIT. Run: make api-bundle'; cat "$tmp"; } > "$out"
```
`X.Y.Z` bukan nilai yang dibiarkan: implementer menggantinya dengan versi eksak sebelum commit.

`scripts/api-check.sh`:
1. Cakupan: untuk tiap `api/modules/**/*.yml` pastikan `grep -qF "modules/<path relatif>" api/root.yml`; kurang -> cetak `api-check: <file> tidak direferensikan di api/root.yml` ke stderr, exit 1.
2. `tmp=$(mktemp); scripts/api-bundle.sh "$tmp"`; `diff -u --label api/openapi.yml --label bundle-baru api/openapi.yml "$tmp"`; beda -> cetak `api-check: api/openapi.yml basi atau diedit manual. Jalankan: make api-bundle` ke stderr, exit 1. `trap` menghapus tmp.

`scripts/api-selftest.sh` (bukti AC4 bagian OpenAPI; memakai `trap` untuk memulihkan SEMUA perubahan, termasuk bila gagal di tengah):
1. Prasyarat: `scripts/api-check.sh` pada kondisi bersih exit 0.
2. Kasus A: tambahkan satu baris sembarang ke `api/openapi.yml` (dari cadangan `cp`), `api-check` harus exit != 0 dan stderr berisi `make api-bundle`.
3. Kasus B: buat `api/modules/zz-selftest.yml` (`paths: {}`) tanpa referensi di root, `api-check` harus exit != 0 dan stderr berisi `tidak direferensikan`.
4. Pulihkan keadaan, jalankan `api-check` lagi, harus exit 0. Cetak `api-selftest: OK`.

#### Langkah B3: makefile

Ganti stub `api-bundle` dan `api-check` dari kerangka (stub saat ini mencetak "diisi oleh usecase ..." dan exit 1), serta tambahkan target baru. Hanya sentuh target di bawah ini, jangan ubah target milik usecase lain (`cli-and-wiring` memfinalkan makefile; rebase dan gabungkan bila bentrok):

```make
api-bundle:   ; ./scripts/api-bundle.sh api/openapi.yml
api-check:    ; ./scripts/api-check.sh
api-selftest: ; ./scripts/api-selftest.sh
depcruise:    ; cd frontend && npm run depcruise
test-arch:    ; cd backend && go test ./internal/archtest/... && cd ../frontend && npm run test:arch
```
(tulis dengan recipe di baris sendiri dan tab, sesuai gaya makefile). Pastikan target `lint` memanggil `depcruise`. Tambahkan semuanya ke `.PHONY`.

#### Langkah B4: hasilkan dan commit `api/openapi.yml`

Jalankan `make api-bundle`, commit hasilnya. Jalankan `make api-check` dan `make api-selftest` hingga hijau.

### C. Frontend: dependency-cruiser

Prasyarat: `frontend/package.json`, `tsconfig.json`, `svelte.config.js`, dan folder `src/{app,domain,shared}` dari `static-frontend-scaffold`. Bila belum ada, kerjakan setelahnya; jangan membuat scaffold di sini.

#### Langkah C1: dependensi dan skrip

`npm install --save-dev --save-exact dependency-cruiser@<16.x terbaru yang dirilis > 2 minggu>` (pastikan `svelte` dan `typescript` sudah devDependency; keduanya dibutuhkan untuk memparsing `.svelte` dan `lang="ts"`). Tambahkan ke `package.json`:

```json
"scripts": {
  "depcruise": "depcruise --config .dependency-cruiser.cjs --validate src",
  "test:arch": "node scripts/test-arch.mjs"
}
```

#### Langkah C2: `frontend/.dependency-cruiser.cjs`

Semua `severity: 'error'`. Nama aturan stabil (dipakai `test-arch.mjs`):

| Nama | from | to | Rule |
|---|---|---|---|
| `shared-no-domain` | `^src/shared/` | `^src/domain/` | 12.5 |
| `shared-no-app` | `^src/shared/` | `^src/app/` | 12.5 (shared paling bawah) |
| `domain-no-app` | `^src/domain/` | `^src/app/` | 12.4 (komposisi hanya di app) |
| `no-cross-feature-internals` | `^src/domain/([^/]+)/([^/]+)/([^/]+)/` | `path: ^src/domain/`, `pathNot: [^src/domain/$1/$2/$3/, ^src/domain/[^/]+/[^/]+/[^/]+/index\.ts$]` | 12.3, 12.4 |
| `outside-domain-only-via-index` | `^src/(app/\|main\.ts$)` | `path: ^src/domain/`, `pathNot: ^src/domain/[^/]+/[^/]+/[^/]+/index\.ts$` | 12.3 |
| `no-circular` | semua | `circular: true` | kebersihan |
| `not-to-test` | `pathNot: \.(test\|spec)\.ts$` | `path: \.(test\|spec)\.ts$` | kode produksi tidak mengimpor test |

Setiap aturan diberi `comment` yang menyebut nomor `rules.txt` agar output tool menjelaskan. Layout feature: `src/domain/<domain>/<module>/<feature>/`; hanya `index.ts` pada level feature yang boleh diimpor dari luar feature itu (impor dalam feature yang sama bebas).

`options`: `doNotFollow: { path: 'node_modules' }`, `includeOnly: '^src'`, `tsPreCompilationDeps: true`, `tsConfig: { fileName: 'tsconfig.json' }`, `exclude: { path: '\\.d\\.ts$' }`, `enhancedResolveOptions: { exportsFields: ['exports'], conditionNames: ['import', 'default'] }`. Pastikan `.svelte` ikut dianalisis (cek keluaran `npx depcruise --info` / hasil pada file `.svelte` di fixture bersih).

#### Langkah C3: fixture dan `scripts/test-arch.mjs`

Fixture berada di luar `src/` dan di luar `tsconfig.json` utama (tambahkan `test/` ke `exclude` di tsconfig, eslint, svelte-check, dan `vitest.config.ts` `exclude`), supaya tidak ikut `npm run check`/`npm test`:

```
frontend/test/depcruise-fixtures/
  ok/
    tsconfig.json                          (minimal, include ["src"])
    src/shared/utils/a.ts
    src/shared/components/b.ts             (impor ../utils/a)
    src/domain/core/auth/login/index.ts
    src/domain/core/auth/login/services/api.ts
    src/domain/core/auth/login/views/v.ts  (impor ../services/api: dalam feature sama, boleh)
    src/domain/project/knowledge/upload/x.ts   (impor ../../../core/auth/login/index  -> boleh, via index.ts)
                                               (impor shared/utils/a -> boleh)
    src/app/routes/r.ts                    (impor domain/core/auth/login/index -> boleh)
  bad/
    tsconfig.json
    src/shared/utils/a.ts                  (kosong)
    src/shared/components/bad1.ts          -> impor ../../domain/core/auth/login/index        => shared-no-domain
    src/shared/components/bad2.ts          -> impor ../../app/routes/r                          => shared-no-app
    src/domain/core/auth/login/index.ts, services/api.ts
    src/domain/project/knowledge/upload/bad3.ts -> impor ../../../core/auth/login/services/api  => no-cross-feature-internals
    src/domain/core/auth/login/bad4.ts     -> impor ../../../../app/routes/r                    => domain-no-app
    src/app/routes/r.ts
    src/app/routes/bad5.ts                 -> impor ../../domain/core/auth/login/services/api  => outside-domain-only-via-index
    src/shared/utils/c1.ts <-> c2.ts       (saling impor)                                       => no-circular
    src/shared/utils/bad6.ts               -> impor ./t.test                                    => not-to-test
```

`frontend/scripts/test-arch.mjs` (Node ESM, tanpa dependensi tambahan selain `child_process`):
1. Jalankan `depcruise --config <abs path .dependency-cruiser.cjs> --validate --output-type json src` dengan `cwd = test/depcruise-fixtures/ok`. Harus exit 0 dan nol pelanggaran. (cwd per fixture membuat path tetap `src/...` sehingga regex yang sama berlaku.)
2. Sama untuk `bad`. Harus exit != 0 dan himpunan `summary.violations[].rule.name` HARUS sama persis dengan `{shared-no-domain, shared-no-app, domain-no-app, no-cross-feature-internals, outside-domain-only-via-index, no-circular, not-to-test}`; selisih apa pun -> cetak yang kurang/lebih dan exit 1. Ini menjaga setiap aturan benar-benar aktif (aturan yang mati karena regex salah ketahuan).
3. Jalankan `npm run depcruise` terhadap `src/` nyata (cwd `frontend`); harus exit 0. Cetak ringkas `test-arch: OK`.

### D. CI

#### Langkah D1: `.github/workflows/ci.yml`

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
permissions:
  contents: read
env:
  CI: "true"          # test integrasi GAGAL (bukan skip) bila TEST_DATABASE_URL kosong

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: backend/go.mod, cache-dependency-path: backend/go.sum }
      - uses: actions/setup-node@v4
        with: { node-version: 22, cache: npm, cache-dependency-path: frontend/package-lock.json }
      - run: go vet ./...
        working-directory: backend
      - uses: golangci/golangci-lint-action@v8     # v8 untuk golangci-lint v2
        with: { version: v2.x, working-directory: backend }   # pin minor eksak saat implementasi
      - run: npm ci
        working-directory: frontend
      - run: npm run check                          # svelte-check + tsc
        working-directory: frontend

  test:
    runs-on: ubuntu-latest
    steps:
      - checkout, setup-go, setup-node (sama)
      - name: architecture-guard (rules.txt 4 dan 13.5)
        run: go test -count=1 -race ./internal/archtest/...
        working-directory: backend
      - run: go test -count=1 -race ./...
        working-directory: backend
      - run: npm ci && npm test -- --run
        working-directory: frontend

  test-integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env: { POSTGRES_USER: grimoire, POSTGRES_PASSWORD: grimoire, POSTGRES_DB: grimoire }
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U grimoire" --health-interval 5s
          --health-timeout 5s --health-retries 10
    env:
      TEST_DATABASE_URL: postgres://grimoire:grimoire@localhost:5432/grimoire?sslmode=disable
    steps:
      - checkout, setup-go
      - run: go test -count=1 -race -tags integration ./...
        working-directory: backend

  api-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 22 }
      - run: make api-check
      - run: make api-selftest

  depcruise:
    runs-on: ubuntu-latest
    steps:
      - checkout, setup-node (cache npm)
      - run: npm ci
        working-directory: frontend
      - run: npm run depcruise
        working-directory: frontend
      - run: npm run test:arch
        working-directory: frontend

  build:
    needs: [lint, test, test-integration, api-check, depcruise]
    runs-on: ubuntu-latest
    steps:
      - checkout, setup-go, setup-node
      - run: make build
      - run: ./bin/grimoire version
      - uses: actions/upload-artifact@v4
        with: { name: grimoire-linux-amd64, path: bin/grimoire, if-no-files-found: error }

  e2e:
    needs: [build]
    runs-on: ubuntu-latest
    services: { postgres: ... sama }
    steps:
      - checkout, setup-node
      - uses: actions/download-artifact@v4
        with: { name: grimoire-linux-amd64, path: bin }
      - run: chmod +x bin/grimoire
      - run: npm ci && npx playwright install --with-deps chromium
        working-directory: frontend
      - run: npx playwright test           # smoke T00-AC1/AC2 terhadap binary; proses server dijalankan oleh webServer di playwright.config.ts (milik static-frontend-scaffold)
        working-directory: frontend
```

Catatan: tuliskan YAML lengkap (bukan singkatan "sama") di file nyata. Action dipin ke tag mayor (kompromi sederhana; ganti ke SHA bila kebijakan keamanan menuntut). Job `e2e` bergantung pada `playwright.config.ts` dan smoke spec dari `static-frontend-scaffold`; bila belum ada saat PR ini dibuat, tambahkan job `e2e` belakangan, jangan di-skip diam-diam. Skrip smoke binary di folder kosong (`bin/grimoire` + dua env) milik `cli-and-wiring`; langkahnya ditambahkan ke job `build` oleh usecase itu.

Pengaturan repositori (di luar berkas, dicatat di PR): jadikan `lint`, `test`, `test-integration`, `api-check`, `depcruise`, `build` sebagai required status checks di branch protection `main`.

### E. Dokumen

1. `rules.txt` (perubahan kecil, rule 15.3 dan 15.2):
   - 3.1: tambahkan `infra/app`, `internal/archtest`, `cmd/internal/{server,buildinfo}` pada daftar (overview bagian 3.3). Bila `repo-skeleton-config` belum melakukannya, lakukan di sini.
   - 3.3: sebut `api/root.yml` sebagai sumber bundle dan `make api-selftest`.
   - 4: tambahkan `4.9  DILARANG: lib/** (selain yang dikecualikan 4.5 yang lebih spesifik) meng-import internal/** atau cmd/**. lib netral.`
   - 13.5: tambahkan kalimat "Tes arsitektur Go di internal/archtest; tes arsitektur FE di frontend/.dependency-cruiser.cjs (`npm run test:arch`)."
2. `docs/tickets/README.md`: di Konvensi arsitektur tambahkan (a) folder `infra/app`, `internal/archtest`, `cmd/internal/*`; (b) konvensi OpenAPI: tiap module menambah `$ref` ke `api/root.yml`, jalankan `make api-bundle`, `make api-check` di CI; (c) satu paragraf "Penegakan otomatis" yang menunjuk `internal/archtest`, `.dependency-cruiser.cjs`, dan CI. Jangan mengubah katalog bus (tidak ada endpoint baru).

### F. Verifikasi akhir (setelah `cli-and-wiring`)

1. `cd backend && go test -race ./internal/archtest/...` hijau; `go test -race -tags integration ./internal/archtest/...` hijau.
2. Uji mutasi manual (tidak di-commit), tiap kali harus gagal dengan pesan jelas lalu dikembalikan: (a) tambahkan impor `grimoire/internal/domain/project` ke `internal/domain/core/base.go`; (b) tambahkan impor `grimoire/internal/domain/core` ke `internal/infra/bus/bus.go`; (c) di frontend, impor `../../domain/core/...` dari `src/shared/utils/`.
3. `make api-check`, `make api-selftest`, `make test-arch`, `make lint` hijau pada pohon bersih.
4. Buka PR percobaan atau dorong ke cabang: semua job CI hijau; ubah satu baris `api/openapi.yml` dan job `api-check` merah.

---

## 4. Migration

Tidak ada. Usecase ini tidak menyentuh database; tidak ada file di `backend/migrations/`.

---

## 5. Changes (per file/folder)

Baru:
- `backend/internal/archtest/rules.go`
- `backend/internal/archtest/rules_test.go`
- `backend/internal/archtest/scan_test.go`
- `backend/internal/archtest/repo_test.go`
- `backend/.golangci.yml`
- `frontend/.dependency-cruiser.cjs`
- `frontend/scripts/test-arch.mjs`
- `frontend/test/depcruise-fixtures/{ok,bad}/**` (bagian C3)
- `api/root.yml`
- `api/openapi.yml` (generated, dikomit)
- `scripts/api-bundle.sh`, `scripts/api-check.sh`, `scripts/api-selftest.sh`
- `.github/workflows/ci.yml`

Diubah:
- `backend/go.mod`, `backend/go.sum`: tambah `golang.org/x/tools` (hanya dipakai test).
- `frontend/package.json`, `frontend/package-lock.json`: devDependency `dependency-cruiser`, skrip `depcruise` dan `test:arch`.
- `frontend/tsconfig.json`, `frontend/vitest.config.ts`, konfigurasi eslint/svelte-check bila ada: kecualikan `test/depcruise-fixtures/**`.
- `makefile`: target `api-bundle`, `api-check`, `api-selftest`, `depcruise`, `test-arch` (ganti stub), `lint` memanggil `depcruise`, `.PHONY`.
- `rules.txt`: 3.1, 3.3, 4.9 baru, 13.5 (bagian E1).
- `docs/tickets/README.md`: bagian E2.

Tidak disentuh: kode produksi di `backend/internal/infra/**` dan `backend/internal/domain/**`, `api/modules/**`, `docs/configuration.md` (usecase ini tidak menambah env var), katalog bus, plan usecase lain.

---

## 6. Test Scenario

Penamaan mengikuti rule 13.4. Semua unit test archtest tanpa DB, tanpa jaringan.

### `rules_test.go` (tabel, memakai `Check` langsung)

Setiap kasus negatif memeriksa: tepat satu `Violation`, `Rule` benar, `From`/`To` sama dengan masukan, dan `String()` memuat paket asal, paket tujuan, dan `rules.txt <nomor>`.

- `TestT00_AC4_Rule41_DomainToDomain`: `grimoire/internal/domain/project/internal/knowledge/upload` -> `grimoire/internal/domain/core/internal/auth/base`; juga root domain `grimoire/internal/domain/project` -> `grimoire/internal/domain/core`. Pesan memuat `4.1`.
- `TestT00_AC4_Rule42_ModuleToModuleSameDomain`: `.../project/internal/knowledge/upload` -> `.../project/internal/apikey/base`. Pesan persis: `rules.txt 4.2: grimoire/internal/domain/project/internal/knowledge/upload imports grimoire/internal/domain/project/internal/apikey/base (module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4)`.
- `TestT00_AC4_Rule43_FeatureToFeature`: `.../knowledge/upload` -> `.../knowledge/search`; juga `.../knowledge/upload/repository` -> `.../knowledge/search`. Kasus lolos: `upload` -> `knowledge/base`, `upload` -> `upload/repository`, module root `.../knowledge` -> `.../knowledge/upload`.
- `TestT00_AC4_Rule44_ConsumerImportsProvider`: consumer `discovery/internal/knowledgetools/readfile` mengimpor provider `project/internal/knowledge/readfile` (4.1) dan `core/internal/auditlog/record` mengimpor `core/internal/auth/login` (4.2); pesan keduanya memuat `rules.txt 4.4`. Kasus lolos: consumer -> `grimoire/lib/constants`, `grimoire/lib/contracts/project/knowledge/readfile`, `grimoire/internal/infra/bus`.
- `TestT00_AC4_Rule45_ContractsImportInternalOrNonCommonLib`: contracts -> `internal/infra/bus`, -> `internal/domain/core`, -> `grimoire/cmd/cli`, -> `grimoire/lib/constants`, -> `grimoire/lib/util`: semuanya 4.5. Lolos: contracts -> `lib/common/errs`, -> `lib/contracts/other`, -> stdlib `time`, -> pihak ketiga `github.com/google/uuid`.
- `TestT00_AC4_Rule46_InfraImportsDomain`: `internal/infra/bus` -> `internal/domain/core`; `internal/infra/middleware` -> `internal/domain/core/internal/auth/base`. Lolos: infra -> `lib/contracts/...`, infra -> infra.
- `TestT00_AC4_Rule49_LibImportsInternal`: `lib/common/errs` -> `internal/infra/config`; `lib/constants` -> `internal/domain/core`; `lib/util` -> `grimoire/cmd/internal/buildinfo`.
- `TestT00_AC4_AllowedImportsPass`: domain -> `infra/bus`, domain -> `infra/db`, domain -> `lib/**`, domain root -> module, module root -> `base`, module root -> feature, feature -> `base`, cmd -> domain/infra/lib, `internal/archtest` -> apa saja, infra -> lib, edge ke stdlib dan pihak ketiga. Hasil kosong.
- `TestCheck_PrefixLookalikeIsExternal`: `grimoire-extra/internal/domain/x` dan `grimoire2/lib` tidak diperlakukan sebagai internal.
- `TestCheck_DedupAndDeterministicOrder`: edge dobel dan urutan masukan acak menghasilkan keluaran identik dan terurut; `From == To` diabaikan.
- `TestCheck_OneViolationPerEdge`: edge yang bisa cocok dua aturan (contracts -> domain, yang juga lib -> internal) hanya dilaporkan sekali sebagai 4.5.
- `TestViolation_StringFormat`: format `rules.txt <n>: <from> imports <to> (<reason>)`.

### `repo_test.go`

- `TestT00_AC4_RepoIsClean`: repo nyata nol pelanggaran.
- `TestModulePathMatchesGoMod`.
- `TestRuleCatalog_ExistsInRulesTxt`.
- `TestT00_AC4_ScanFindsViolationsInTempModule`: bagian A2 (termasuk file `_test.go` dan tag `integration`).

### Frontend (`npm run test:arch`)

- `ok` lulus, `bad` gagal dengan himpunan aturan persis tujuh nama, `src/` nyata lulus (C3).

### OpenAPI (`make api-selftest`, dijalankan CI)

- Kondisi bersih: `api-check` exit 0.
- Edit manual `api/openapi.yml`: exit != 0, pesan `make api-bundle`.
- Modul tidak direferensikan di `api/root.yml`: exit != 0, pesan `tidak direferensikan`.
- Setelah pemulihan: exit 0.
- Bundle dua kali menghasilkan berkas identik (determinisme, dicek sekali di spike B1 dan sebagai langkah di selftest bila murah).

---

## 7. E2E Scenarios

Positif:
1. Pohon bersih: `make test-arch`, `make api-check`, `make depcruise`, `make lint` semuanya exit 0.
2. PR yang menambah feature baru di module dan hanya mengimpor `base/`, `lib/**`, `infra/bus`, `infra/db` lulus semua job CI.
3. PR yang mengubah `api/modules/infra/foundation.yml` dan menjalankan `make api-bundle` (openapi.yml ikut dikomit) lulus job `api-check`.
4. Tambah module baru yang file `api/modules/<d>/<m>.yml`-nya direferensikan di `api/root.yml` dan sudah dibundle: lulus.
5. Frontend: view di `src/app/` mengimpor `src/domain/core/auth/login/index.ts` dan `src/shared/...`: lulus.

Negatif:
1. Domain `project` mengimpor `grimoire/internal/domain/core/...`: `go test ./internal/archtest` gagal, keluaran memuat `rules.txt 4.1: <paket asal> imports <paket tujuan> (...)`; job `test` merah, job `build` tidak jalan.
2. Module `knowledge` mengimpor `apikey/base` (satu domain): pesan 4.2.
3. Feature `upload` mengimpor feature `search`: pesan 4.3 (bahkan bila impor hanya di file `_test.go`).
4. `lib/contracts/**` mengimpor `internal/infra/bus` atau `lib/constants`: pesan 4.5.
5. `internal/infra/middleware` mengimpor domain: pesan 4.6.
6. `lib/common/errs` mengimpor `internal/infra/config`: pesan 4.9.
7. Impor terlarang hanya di file bertag `//go:build integration`: tetap tertangkap.
8. `src/shared/components/X.svelte` mengimpor `src/domain/...`: `npm run depcruise` gagal dengan aturan `shared-no-domain`; job `depcruise` merah.
9. `src/domain/project/knowledge/upload/foo.ts` mengimpor `src/domain/core/auth/login/services/api.ts`: `no-cross-feature-internals`. Ganti ke `.../login/index.ts`: lulus.
10. `src/app/routes/x.ts` mengimpor `src/domain/core/auth/login/views/LoginView.svelte`: `outside-domain-only-via-index`.
11. `api/openapi.yml` diedit tangan: `make api-check` gagal dengan diff dan instruksi `make api-bundle`; job `api-check` merah.
12. `api/modules/project/apikey.yml` ditambah tetapi tidak direferensikan `api/root.yml`: `make api-check` gagal (`tidak direferensikan`).
13. Mengubah `api/modules/infra/foundation.yml` tanpa `make api-bundle`: `make api-check` gagal (openapi.yml basi).
14. Test integrasi dijalankan di CI tanpa `TEST_DATABASE_URL`: gagal (bukan skip) karena `CI=true`. (Perilaku milik `db-migrations`; CI ini menyediakan env dan service-nya.)

---

## 8. Request/Response body

Usecase ini tidak memiliki endpoint HTTP, jadi tidak ada body request/response dan tidak ada format error JSON API. Berikut kontrak keluaran yang dipakai developer dan CI.

### 8.1 Masukan dan keluaran `Check` (Go)

```go
imports := []archtest.Import{
    {From: "grimoire/internal/domain/project/internal/knowledge/upload",
     To:   "grimoire/internal/domain/project/internal/apikey/base"},
    {From: "grimoire/internal/domain/project/internal/knowledge/upload",
     To:   "grimoire/internal/infra/bus"},            // diizinkan
    {From: "grimoire/internal/domain/project/internal/knowledge/upload",
     To:   "context"},                                // stdlib, diabaikan
}
vs := archtest.Check(imports)
// len(vs) == 1
// vs[0] == archtest.Violation{
//   Rule:   "4.2",
//   From:   "grimoire/internal/domain/project/internal/knowledge/upload",
//   To:     "grimoire/internal/domain/project/internal/apikey/base",
//   Reason: "module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4",
// }
```

### 8.2 Contoh keluaran `go test` saat gagal (format pesan pelanggaran)

```
--- FAIL: TestT00_AC4_RepoIsClean (3.41s)
    repo_test.go:58: rules.txt 4.2: grimoire/internal/domain/project/internal/knowledge/upload imports grimoire/internal/domain/project/internal/apikey/base (module -> module lain; gunakan infra/bus + lib/contracts, lihat rules.txt 4.4)
    repo_test.go:58: rules.txt 4.6: grimoire/internal/infra/middleware imports grimoire/internal/domain/core (infra -> domain; infra tidak tahu bisnis)
    repo_test.go:60: 2 pelanggaran batas arsitektur; lihat rules.txt bagian 4
FAIL
```

### 8.3 Contoh keluaran dependency-cruiser

```
error shared-no-domain: src/shared/components/Table.svelte -> src/domain/core/auth/login/index.ts
  rules.txt 12.5: shared/ DILARANG meng-import domain/

x 1 dependency violations (1 errors, 0 warnings). 42 modules, 97 dependencies cruised.
```

### 8.4 Contoh keluaran `make api-check`

```
--- api/openapi.yml
+++ bundle-baru
@@ -12,3 +12,3 @@
-      description: Proses hidup
+      description: Proses hidup.
api-check: api/openapi.yml basi atau diedit manual. Jalankan: make api-bundle
make: *** [api-check] Error 1
```

```
api-check: api/modules/project/apikey.yml tidak direferensikan di api/root.yml
make: *** [api-check] Error 1
```

### 8.5 Header berkas hasil bundle

```yaml
# GENERATED by scripts/api-bundle.sh from api/root.yml + api/modules/**. DO NOT EDIT. Run: make api-bundle
openapi: 3.1.0
info:
  title: NooBee Grimoire API
  version: 0.1.0
...
```

### 8.6 Format error API (rujukan)

Usecase ini tidak menghasilkan error API. Format seragam rule 11.1 yang dipakai endpoint lain tetap:

```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
