#!/usr/bin/env bash
# Gagal bila api/openapi.yml basi/diedit manual, atau ada api/modules/**.yml
# yang tidak direferensikan api/root.yml.
set -euo pipefail
cd "$(dirname "$0")/.."

status=0
while IFS= read -r f; do
  rel="${f#api/}"
  if ! grep -qF "$rel" api/root.yml; then
    echo "api-check: $f tidak direferensikan di api/root.yml" >&2
    status=1
  fi
done < <(find api/modules -type f \( -name '*.yml' -o -name '*.yaml' \) | LC_ALL=C sort)
[ "$status" -eq 0 ] || exit 1

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
tmp="$tmpdir/bundle.yml"
scripts/api-bundle.sh "$tmp" >/dev/null

if ! diff -u --label api/openapi.yml --label bundle-baru api/openapi.yml "$tmp" >&2; then
  echo "api-check: api/openapi.yml basi atau diedit manual. Jalankan: make api-bundle" >&2
  exit 1
fi
echo "api-check: OK"
