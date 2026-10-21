import { asObj, asStr, isObj } from './guards';
import { deref } from './refs';

export interface ExampleItem {
  name: string;
  summary: string | null;
  value: unknown;
  externalValue: string | null; // hanya ditampilkan sebagai teks, tidak pernah di-fetch
}

// Contoh dari media type / parameter: `example` tunggal dan map `examples` (dengan $ref).
export function examplesOf(doc: unknown, node: unknown): ExampleItem[] {
  const n = asObj(node);
  const out: ExampleItem[] = [];
  if ('example' in n) out.push({ name: 'example', summary: null, value: n.example, externalValue: null });
  const map = n.examples;
  if (isObj(map)) {
    for (const [name, raw] of Object.entries(map)) {
      const d = deref(doc, raw);
      const e = asObj(d.node);
      if (d.unresolved || d.circular) continue;
      out.push({ name, summary: asStr(e.summary), value: e.value, externalValue: asStr(e.externalValue) });
    }
  }
  return out;
}

export const TRUNCATED_MARK = '\n… (dipotong)';

export function toPrettyJson(v: unknown, maxChars = 20000): string {
  let s: string;
  try {
    s = typeof v === 'string' ? JSON.stringify(v) : (JSON.stringify(v, null, 2) ?? String(v));
  } catch {
    s = String(v);
  }
  return s.length > maxChars ? s.slice(0, maxChars) + TRUNCATED_MARK : s;
}
