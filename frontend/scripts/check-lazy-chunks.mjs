// Penjaga AC13: chart.js tidak boleh masuk chunk entri atau import statisnya.
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const manifestPath = process.argv[2] ? resolve(process.argv[2]) : join(dist, '.vite', 'manifest.json');
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const entry = manifest['index.html'];
if (!entry) {
  console.error('check-lazy-chunks: entri index.html tidak ada di manifest');
  process.exit(1);
}
const seen = new Set();
const bad = [];
(function walk(key) {
  if (seen.has(key)) return;
  seen.add(key);
  const c = manifest[key];
  if (!c) return;
  const src = String(c.src ?? key);
  if (src.includes('node_modules/chart.js')) bad.push(src);
  for (const i of c.imports ?? []) walk(i);
})('index.html');
if (bad.length) {
  console.error('check-lazy-chunks: chart.js ada di chunk entri:', bad.join(', '));
  process.exit(1);
}
console.log(`check-lazy-chunks: ok (${seen.size} chunk entri diperiksa)`);
