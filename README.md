# Grimoire

Knowledge base per project untuk tim, dengan gateway MCP read-only agar agent AI (Claude Code dan sejenisnya) bisa membaca dokumen dan spesifikasi API project lewat API key.

- Backend: Go 1.25, Postgres 16, satu binary (`bin/grimoire`) dengan frontend ter-embed.
- Frontend: Svelte + Vite.
- Gateway MCP: `POST /mcp` dengan tool `list_files`, `read_file`, `search_knowledge`, `list_api_specs`, `list_api_endpoints`, `get_api_endpoint`.

Image siap pakai: [`achimonchi/grimoire` di Docker Hub](https://hub.docker.com/r/achimonchi/grimoire) (tag `scratch` dan `alpine`). Lihat [Menjalankan dari Docker Hub](#menjalankan-dari-docker-hub).

## Prasyarat

| Tool | Keterangan |
|---|---|
| Go 1.25+ | backend |
| Node.js + npm | frontend |
| Docker + Docker Compose | Postgres untuk dev |
| make | semua perintah di bawah lewat `make` |

## Quick start (dev)

```bash
# 1. Siapkan env
cp .env.example .env
#    ganti SESSION_SECRET dengan string acak >= 32 byte, mis.:
#    openssl rand -base64 48

# 2. Jalankan Postgres (docker compose, port 5432)
make db-up

# 3. Migrasi database
make migrate

# 4. Seed super admin dev
make seed

# 5. Jalankan backend + frontend
make dev
```

Setelah `make dev`:

| Layanan | URL |
|---|---|
| Aplikasi (backend, proxy ke Vite) | http://localhost:8080 |
| Vite dev server | http://localhost:5173 |
| Health check | http://localhost:8080/healthz |

Login dengan akun hasil seed (lihat bagian [Seeding](#seeding)). Akun ini wajib ganti password pada login pertama.

> `make dev` sudah menjalankan `RUN_MIGRATION=true`, jadi langkah 3 boleh dilewati untuk dev. Langkah 3 tetap berguna bila Anda ingin migrasi tanpa menjalankan server, dan `make seed` butuh tabel `users` sudah ada, jadi migrasi harus sudah pernah jalan sebelum seed.

## Konfigurasi

Konfigurasi hanya lewat environment (`.env` dibaca otomatis oleh `make` dan CLI). Dua yang wajib:

| Env | Keterangan |
|---|---|
| `DATABASE_URL` | URL Postgres. Default dev: `postgres://grimoire:grimoire@localhost:5432/grimoire?sslmode=disable` |
| `SESSION_SECRET` | Minimal 32 byte |

Daftar lengkap beserta default dan validasi ada di [docs/configuration.md](docs/configuration.md).

## Database

Postgres dev dijalankan lewat `docker-compose.dev.yml` (user/password/db: `grimoire`, volume `grimoire_pgdata`).

```bash
make db-up      # start Postgres dan tunggu sampai healthy
make db-down    # stop container (data tetap ada di volume)
```

Reset total database dev (hapus data):

```bash
docker compose -f docker-compose.dev.yml down -v
make db-up && make migrate && make seed
```

## Migration

File SQL ter-embed di binary, ada di `backend/migrations/`:

```
backend/migrations/
├── core/      # auth, audit log
└── project/   # projects, api keys, api specs, knowledge files
```

### Menjalankan

```bash
make migrate                         # setara: cd backend && go run ./cmd/cli migrate up
./bin/grimoire migrate up            # dari binary hasil make build
RUN_MIGRATION=true         # jalankan otomatis saat server start
```

Perilaku runner:

- Urutan eksekusi mengikuti versi (timestamp) naik, lintas folder `core` dan `project`.
- Idempoten: versi yang sudah tercatat di tabel `schema_migrations` dilewati.
- Satu file = satu transaksi. Gagal berarti rollback penuh dan file berikutnya tidak dijalankan.
- Aman dijalankan banyak instance sekaligus (advisory lock).
- Forward-only: tidak ada `down`. Perbaikan skema dibuat sebagai migrasi baru.

### Membuat migrasi baru

1. Generate timestamp (jangan ditebak, `date +%N` tidak ada di macOS):

   ```bash
   python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"
   ```

2. Buat file `backend/migrations/<core|project>/<timestamp>_<snake_case>.sql`, mis. `20261023101500123_add_tags.sql`.
3. Tulis SQL biasa. Jangan menulis `BEGIN`/`COMMIT` dan jangan pakai `CREATE INDEX CONCURRENTLY` atau `VACUUM` (runner sudah membungkus transaksi).
4. Pastikan tidak ada versi kembar:

   ```bash
   ls backend/migrations/*/ | cut -c1-17 | sort | uniq -d   # harus kosong
   ```

5. Jangan mengedit atau me-rename migrasi yang sudah ter-merge ke `main`.

Detail: [docs/infra/db-migrations.md](docs/infra/db-migrations.md).

## Seeding

```bash
make seed
```

Perintah ini menjalankan `scripts/seed/dev_admin.sql` ke DB dev lewat `psql` di dalam container. Hasilnya satu akun super admin:

| Field | Nilai |
|---|---|
| Email | `admin@noobee.id` |
| Password | `SeCr3t` |
| Role | `super_admin` |
| `must_change_password` | `true` (dipaksa ganti password saat login pertama) |

Sifat seeder:

- Idempoten (`ON CONFLICT (email) DO NOTHING`): aman diulang dan tidak menimpa akun yang sudah ada.
- Hanya untuk dev. **Jangan dipakai di produksi.**

### Membuat admin untuk produksi

Gunakan CLI, bukan seeder:

```bash
# password dibaca dari prompt (TTY) atau dari env CREATE_ADMIN_PASSWORD
./bin/grimoire create-admin --email admin@perusahaan.id --name "Nama Admin"
```

## Cara pakai aplikasi

1. Login sebagai super admin dan ganti password.
2. Buat **project**, lalu tambahkan anggota.
3. Isi knowledge: upload file dokumen (markdown dan sejenisnya) dan/atau spesifikasi API (OpenAPI) ke project. Dukungan Swagger 2 masih dalam perencanaan (tiket T10).
4. Buat **API key** untuk project (format `gr_<prefix>_<secret>`). Secret hanya tampil sekali.
5. Hubungkan agent AI ke gateway MCP memakai API key tersebut.

### Menghubungkan Claude Code ke MCP

```bash
claude mcp add --transport http grimoire http://localhost:8080/mcp \
  --header "Authorization: Bearer gr_<prefix>_<secret>"
```

Setiap API key terikat ke satu project, jadi agent hanya melihat knowledge project itu. Semua tool read-only. Detail protokol, tool, error, dan rate limit: [docs/mcp-gateway.md](docs/mcp-gateway.md).

### Memasang skill Grimoire

Repo ini menyertakan skill Claude Code di [`skills/grimoire`](skills/grimoire/SKILL.md) yang mengajari agent cara memakai tool Grimoire (memilih server per project, menelusuri dokumen, menelusuri API spec). Pasang di device masing-masing:

```bash
npx skills add NooBeeID/grimoire --skill grimoire
```

Perintah ini memakai CLI [`skills`](https://github.com/vercel-labs/skills) dan akan menanyakan agent serta lokasi pemasangan (global atau per project). Flag `--skill grimoire` perlu karena repo ini juga menyimpan skill desain untuk pengembangan frontend (`.agents/skills`, `.claude/skills`) yang tidak dibutuhkan pengguna Grimoire. Tanpa flag itu, CLI menampilkan semuanya.

Alternatif tanpa npx, salin manual dari clone repo ini:

```bash
# untuk semua project di device ini
mkdir -p ~/.claude/skills
cp -r skills/grimoire ~/.claude/skills/

# atau hanya untuk satu repo
mkdir -p <repo-tujuan>/.claude/skills
cp -r skills/grimoire <repo-tujuan>/.claude/skills/
```

Mulai ulang sesi Claude Code, lalu skill aktif otomatis saat Anda menyebut Grimoire, knowledge base, atau spesifikasi API. Skill ini hanya panduan; agent tetap butuh server MCP yang terhubung (langkah di atas).

## Perintah make

Jalankan `make help` untuk daftar lengkap. Yang utama:

| Perintah | Fungsi |
|---|---|
| `make dev` | Postgres + backend (:8080) + Vite (:5173) |
| `make db-up` / `make db-down` | start / stop Postgres dev |
| `make migrate` | jalankan migrasi |
| `make seed` | seed super admin dev |
| `make build` | build `bin/grimoire` (frontend ter-embed) |
| `make test` | unit test backend dan frontend |
| `make test-integration` | test integrasi (pakai DB `grimoire_test`, terpisah dari DB dev) |
| `make test-e2e` | Playwright terhadap `bin/grimoire` |
| `make vet` / `make lint` | analisis statis |
| `make api-bundle` | bundle `api/root.yml` menjadi `api/openapi.yml` |

## Build dan jalankan production

```bash
make build

export DATABASE_URL='postgres://user:pass@host:5432/grimoire?sslmode=require'
export SESSION_SECRET="$(openssl rand -base64 48)"

./bin/grimoire migrate up
./bin/grimoire create-admin --email admin@perusahaan.id --name "Nama Admin"
./bin/grimoire serve        # default bila tanpa perintah
```

Perintah CLI: `serve`, `migrate up`, `create-admin --email E --name N`, `version`, `help`.

Di produksi biarkan `COOKIE_SECURE=true` (default) dan `APP_ENV=production` (default), serta set `PUBLIC_URL`.

## Menjalankan dari Docker Hub

Tanpa perlu Go atau Node. Image: [`achimonchi/grimoire`](https://hub.docker.com/r/achimonchi/grimoire).

| Tag | Isi | Ukuran terkompres |
|---|---|---|
| `scratch` | binary saja, tanpa shell (disarankan untuk production) | ~6.8MB |
| `alpine` | ada shell + `HEALTHCHECK` (untuk debug) | ~11MB |

Image multi-arch: `linux/amd64` dan `linux/arm64`.

### Dengan Docker Compose (Postgres + Grimoire)

```bash
# 1. Ambil compose file
curl -O https://raw.githubusercontent.com/NooBeeID/grimoire/main/docker-compose.yml

# 2. Buat .env di folder yang sama
printf 'POSTGRES_PASSWORD=%s\nSESSION_SECRET=%s\n' \
  "$(openssl rand -hex 16)" "$(openssl rand -base64 48)" > .env

# 3. Jalankan (migrasi otomatis saat start)
docker compose up -d

# 4. Buat admin pertama (prompt password)
docker compose run --rm -it grimoire create-admin --email admin@perusahaan.id --name "Nama Admin"
```

Buka http://localhost:8080 dan login dengan akun tersebut.

Opsi di `.env`:

| Env | Default | Keterangan |
|---|---|---|
| `POSTGRES_PASSWORD` | - (wajib) | Password Postgres |
| `SESSION_SECRET` | - (wajib) | Minimal 32 byte |
| `GRIMOIRE_TAG` | `scratch` | Tag image (`scratch` / `alpine`) |
| `COOKIE_SECURE` | `false` | Set `true` bila diakses lewat HTTPS |
| `PUBLIC_URL` | `http://localhost:8080` | URL publik aplikasi |

Env lain ada di [docs/configuration.md](docs/configuration.md); tambahkan di blok `environment` service `grimoire`.

Perintah berguna:

```bash
docker compose logs -f grimoire               # log aplikasi
docker compose pull && docker compose up -d   # update ke image terbaru
docker compose down                           # stop (data tetap di volume)
docker compose down -v                        # stop + hapus data
```

### Tanpa Compose (docker run)

```bash
docker network create grimoire
docker run -d --name grimoire-db --network grimoire \
  -e POSTGRES_USER=grimoire -e POSTGRES_PASSWORD=ganti-ini -e POSTGRES_DB=grimoire \
  -v grimoire_pgdata:/var/lib/postgresql/data postgres:16

docker run -d --name grimoire --network grimoire -p 8080:8080 \
  -e DATABASE_URL='postgres://grimoire:ganti-ini@grimoire-db:5432/grimoire?sslmode=disable' \
  -e SESSION_SECRET="$(openssl rand -base64 48)" \
  -e RUN_MIGRATION=true -e COOKIE_SECURE=false \
  achimonchi/grimoire:scratch

docker exec -it grimoire /grimoire create-admin --email admin@perusahaan.id --name "Nama Admin"
```

Untuk tag `scratch`, `docker exec` hanya bisa memanggil binary `/grimoire` (tidak ada shell).

## Docker

```bash
# varian scratch (paling kecil, tanpa shell) atau alpine (ada shell + HEALTHCHECK)
docker build -f Dockerfile.scratch -t grimoire:scratch \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) .
docker build -f Dockerfile.alpine -t grimoire:alpine .

# multi-arch + push (butuh docker login)
docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.scratch -t <user>/grimoire:scratch --push .

# migrasi dan buat admin (one-off), lalu jalankan server
docker run --rm -e DATABASE_URL -e SESSION_SECRET grimoire:scratch migrate up
docker run --rm -it -e DATABASE_URL -e SESSION_SECRET grimoire:scratch create-admin --email admin@perusahaan.id --name "Nama Admin"
docker run -d -p 8080:8080 -e DATABASE_URL -e SESSION_SECRET -e PUBLIC_URL grimoire:scratch
```

Image berisi satu binary dengan frontend ter-embed, berjalan sebagai user non-root di atas `scratch` (tanpa shell). Healthcheck pakai probe orchestrator ke `GET /healthz`. Dari container, `localhost` bukan host Anda; arahkan `DATABASE_URL` ke host Postgres yang bisa dijangkau (mis. `host.docker.internal` di Docker Desktop).

## Struktur repo

```
api/          spesifikasi OpenAPI per module (+ openapi.yml hasil bundle)
backend/      Go: cmd/ (http, cli), internal/, lib/, migrations/
frontend/     Svelte + Vite
docs/         konfigurasi, migrasi, MCP gateway, tiket
skills/       skill Claude Code untuk dipasang pengguna (grimoire)
scripts/      make include, seeder, helper
```
