# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Tim kecil developer dan PM yang menulis knowledge (Markdown) dan spesifikasi API per project, dalam sesi kerja panjang. Pengguna kedua adalah agent AI (Claude Code dan sejenisnya) yang membaca isinya read-only lewat gateway MCP memakai API key. Peran: super admin, anggota workspace.

## Product Purpose

Grimoire adalah knowledge base per project untuk tim, dengan gateway MCP read-only agar agent bisa membaca dokumen dan spesifikasi API project. Berhasil bila manusia cepat menemukan dan mengedit dokumen, dan agent mendapat konteks yang benar tanpa menyalin-tempel.

## Positioning

Knowledge base yang dibuat untuk dibaca agent sama seriusnya dengan dibaca manusia: API key, jejak audit, dan pembeda manusia vs agent adalah bagian inti, bukan tambahan.

## Operating Context

Tab per project: Knowledge (pohon file + baca/edit Markdown), API Spec (OpenAPI), API Keys, workspace/anggota, audit log, manajemen users. Backend Go + Postgres, satu binary dengan frontend Svelte ter-embed. Login lalu daftar project.

## Capabilities and Constraints

- Gateway MCP `POST /mcp`: `list_files`, `read_file`, `search_knowledge`, `list_api_specs`, `list_api_endpoints`, `get_api_endpoint`.
- Audit log membedakan `actor_type=api_key` (agent) dari manusia.
- Tema terang dan gelap lewat toggle; UI dwibahasa (Indonesia, Inggris).
- Font dimuat dari `@fontsource/*` (Vite bundle); CSP `font-src 'self'`, tanpa CDN font.
- Frontend Svelte + Tailwind (`@theme` di `frontend/src/app.css`).

## Brand Commitments

- Nama produk Grimoire, di bawah NooBee.
- Kuning (madu) tetap warna utama.
- Arah visual dipilih pengguna: gaya cartoon penuh (bentuk membulat, outline, maskot), menggantikan framed-sheet yang tenang. Belum ada aset maskot; lebah baru boleh dirancang.
- Bahasa Indonesia dan Inggris tetap didukung.

## Evidence on Hand

Tidak ada aset maskot, ilustrasi, atau foto. Data contoh memakai kata produk ini (path seperti `docs/auth/login-flow.md`, prefiks key `nbk_4f9a…`); tidak ada pelanggan atau angka metrik yang boleh dikarang.

## Product Principles

- Keterbacaan konten dan data tidak dikorbankan untuk ekspresi; karakter hidup di kerangka, status, dan momen emosional.
- Manusia vs agent harus terbaca sekilas.
- Aksi berisiko (hapus, key sekali tampil) tidak boleh terasa remeh walau nadanya ramah.
- Dua bahasa, dua tema, tanpa layar yang rusak di salah satunya.

## Accessibility & Inclusion

Kontras teks 4.5:1, fokus terlihat, status tidak hanya warna, target sentuh minimal 32px, hormati `prefers-reduced-motion`.
