export interface DiffRow {
  op: 'eq' | 'add' | 'del';
  text: string;
}

const MAX_LINES = 5000;
const MAX_CELLS = 16_000_000;

// Diff baris (LCS). Awalan dan akhiran yang sama dipangkas dulu supaya perubahan kecil di file
// besar tetap murah. null bila salah satu sisi > 5000 baris atau bagian yang berbeda terlalu besar.
export function diffLines(a: string, b: string): DiffRow[] | null {
  const x = a.split('\n');
  const y = b.split('\n');
  if (x.length > MAX_LINES || y.length > MAX_LINES) return null;

  let head = 0;
  while (head < x.length && head < y.length && x[head] === y[head]) head++;
  let tail = 0;
  while (tail < x.length - head && tail < y.length - head && x[x.length - 1 - tail] === y[y.length - 1 - tail]) tail++;

  const xs = x.slice(head, x.length - tail);
  const ys = y.slice(head, y.length - tail);
  const m = xs.length;
  const n = ys.length;
  if ((m + 1) * (n + 1) > MAX_CELLS) return null;

  const w = n + 1;
  const dp = new Uint32Array((m + 1) * w);
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i * w + j] = xs[i] === ys[j] ? dp[(i + 1) * w + j + 1]! + 1 : Math.max(dp[(i + 1) * w + j]!, dp[i * w + j + 1]!);
    }
  }

  const out: DiffRow[] = x.slice(0, head).map((text) => ({ op: 'eq', text }));
  let i = 0;
  let j = 0;
  while (i < m && j < n) {
    if (xs[i] === ys[j]) {
      out.push({ op: 'eq', text: xs[i]! });
      i++;
      j++;
    } else if (dp[(i + 1) * w + j]! >= dp[i * w + j + 1]!) out.push({ op: 'del', text: xs[i++]! });
    else out.push({ op: 'add', text: ys[j++]! });
  }
  while (i < m) out.push({ op: 'del', text: xs[i++]! });
  while (j < n) out.push({ op: 'add', text: ys[j++]! });
  for (const text of y.slice(y.length - tail)) out.push({ op: 'eq', text });
  return out;
}
