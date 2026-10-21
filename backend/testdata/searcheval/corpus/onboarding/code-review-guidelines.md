# Pedoman code review

Code review membantu kualitas dan berbagi pengetahuan. Ini panduan untuk penulis dan reviewer.

## Untuk penulis

- Buat PR kecil, idealnya di bawah 400 baris perubahan.
- Tulis deskripsi: apa, kenapa, dan cara menguji.
- Pastikan CI hijau sebelum meminta review.
- Tanggapi komentar dengan perubahan atau penjelasan, jangan diabaikan.

## Checklist reviewer

1. Apakah perubahan menyelesaikan masalah yang dideskripsikan?
2. Apakah ada tes untuk kasus normal dan kasus gagal?
3. Apakah ada risiko keamanan, misalnya input tidak divalidasi?
4. Apakah nama dan struktur mudah dibaca?
5. Apakah ada dampak ke migrasi atau kompatibilitas API?

## SLA

Reviewer memberi tanggapan pertama dalam 1 hari kerja. Bila tidak bisa, beri tahu penulis dan tunjuk orang lain.

## Etika komentar

- Kritik kode, bukan orang.
- Tandai komentar kecil dengan prefix `nit:` agar tidak memblokir merge.
- Approve bila sisa komentar hanya nit.
