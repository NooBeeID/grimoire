# Versioning API

API publik diberi versi lewat prefix path, saat ini `/v1`.

## Kompatibilitas mundur

Perubahan berikut dianggap aman dalam satu versi:

- menambah field baru di respons,
- menambah endpoint baru,
- menambah nilai opsional pada request.

Perubahan berikut membutuhkan versi baru:

- menghapus atau mengganti nama field,
- mengubah tipe field,
- mengubah arti status code.

## Deprecation

1. Umumkan deprecation di changelog dan email ke pemilik integrasi.
2. Tambahkan header `Deprecation: true` dan `Sunset: <tanggal>` pada respons.
3. Beri waktu minimal 6 bulan sebelum sunset.
4. Setelah sunset, endpoint mengembalikan 410.

## Menjalankan dua versi

`/v1` dan `/v2` berjalan di binary yang sama. Handler bersama dipisah lewat lapisan mapping respons, bukan salin-tempel handler.

## Checklist rilis versi baru

- Dokumentasi migrasi dari versi lama.
- Contoh request dan respons.
- Tes kontrak untuk kedua versi.
