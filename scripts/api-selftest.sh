#!/usr/bin/env bash
# Membuktikan api-check benar-benar gagal pada openapi.yml yang diedit manual
# dan pada modul yang tidak direferensikan. Semua perubahan dipulihkan lewat trap.
set -euo pipefail
cd "$(dirname "$0")/.."

backup="$(mktemp)"
errf="$(mktemp)"
cp api/openapi.yml "$backup"
cleanup() {
  cp "$backup" api/openapi.yml
  rm -f api/modules/zz-selftest.yml "$backup" "$errf"
}
trap cleanup EXIT

scripts/api-check.sh >/dev/null 2>&1 || { echo "api-selftest: prasyarat gagal: api-check tidak bersih" >&2; exit 1; }

# Kasus A: edit manual
echo "# edit manual" >> api/openapi.yml
if scripts/api-check.sh >/dev/null 2>"$errf"; then
  echo "api-selftest: kasus A: api-check seharusnya gagal" >&2; exit 1
fi
grep -qF "make api-bundle" "$errf" || { echo "api-selftest: kasus A: pesan tidak memuat 'make api-bundle'" >&2; exit 1; }
cp "$backup" api/openapi.yml

# Kasus B: modul tidak direferensikan
echo "paths: {}" > api/modules/zz-selftest.yml
if scripts/api-check.sh >/dev/null 2>"$errf"; then
  echo "api-selftest: kasus B: api-check seharusnya gagal" >&2; exit 1
fi
grep -qF "tidak direferensikan" "$errf" || { echo "api-selftest: kasus B: pesan tidak memuat 'tidak direferensikan'" >&2; exit 1; }
rm -f api/modules/zz-selftest.yml

# Pulih
scripts/api-check.sh >/dev/null 2>&1 || { echo "api-selftest: setelah pemulihan api-check gagal" >&2; exit 1; }
echo "api-selftest: OK"
