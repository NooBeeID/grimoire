// Membuktikan .dependency-cruiser.cjs benar-benar menolak pelanggaran dan meloloskan kode bersih.
// 1) fixture ok  : nol pelanggaran
// 2) fixture bad : himpunan aturan terlanggar == EXPECTED (aturan mati/salah regex ketahuan)
// 3) src/ nyata  : lolos
import { spawnSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const config = join(frontend, '.dependency-cruiser.cjs');
const bin = join(frontend, 'node_modules', '.bin', 'depcruise');

const EXPECTED = [
  'shared-no-domain',
  'shared-no-app',
  'domain-no-app',
  'no-cross-feature-internals',
  'outside-domain-only-via-index',
  'no-circular',
  'not-to-test',
];

function cruise(cwd, extra = []) {
  const r = spawnSync(bin, ['--config', config, ...extra, 'src'], { cwd, encoding: 'utf8' });
  if (r.error) throw r.error;
  return r;
}

function violationsOf(cwd) {
  const r = cruise(cwd, ['--output-type', 'json']);
  let json;
  try {
    json = JSON.parse(r.stdout);
  } catch {
    console.error(`test-arch: keluaran depcruise bukan JSON di ${cwd}\n${r.stdout}\n${r.stderr}`);
    process.exit(1);
  }
  return { violations: json.summary.violations };
}

const fail = (msg) => {
  console.error(`test-arch: GAGAL: ${msg}`);
  process.exit(1);
};

const okDir = join(frontend, 'test', 'depcruise-fixtures', 'ok');
const badDir = join(frontend, 'test', 'depcruise-fixtures', 'bad');

const ok = violationsOf(okDir);
if (cruise(okDir).status !== 0 || ok.violations.length > 0) {
  fail(`fixture ok harus bersih, ditemukan: ${ok.violations.map((v) => `${v.rule.name} ${v.from} -> ${v.to}`).join('; ')}`);
}

const bad = violationsOf(badDir);
if (cruise(badDir).status === 0) fail('fixture bad harus gagal (exit != 0)');
const got = new Set(bad.violations.map((v) => v.rule.name));
const missing = EXPECTED.filter((n) => !got.has(n));
const extra = [...got].filter((n) => !EXPECTED.includes(n));
if (missing.length || extra.length) {
  fail(`aturan fixture bad tidak sesuai. kurang: [${missing}] lebih: [${extra}]`);
}

const real = cruise(frontend);
if (real.status !== 0) {
  process.stdout.write(real.stdout);
  process.stderr.write(real.stderr);
  fail('src/ nyata melanggar aturan');
}

console.log('test-arch: OK');
