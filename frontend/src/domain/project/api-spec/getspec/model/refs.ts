import { isObj, type Obj } from './guards';

// Hanya pointer internal "#" / "#/...". Tidak pernah fetch.
export function resolvePointer(doc: unknown, ref: string): unknown {
  if (ref === '#') return doc;
  if (!ref.startsWith('#/')) return undefined;
  let cur: unknown = doc;
  for (const raw of ref.slice(2).split('/')) {
    const seg = raw.replace(/~1/g, '/').replace(/~0/g, '~');
    if (Array.isArray(cur)) {
      const i = /^\d+$/.test(seg) ? Number(seg) : -1;
      cur = cur[i];
    } else if (isObj(cur) && Object.prototype.hasOwnProperty.call(cur, seg)) cur = cur[seg];
    else return undefined;
  }
  return cur;
}

export interface Deref {
  node: unknown;
  ref: string | null;
  circular: boolean;
  unresolved: boolean;
}

const MAX_HOPS = 16;

// Mengikuti rantai $ref. `stack` memuat ref yang sedang dibuka (untuk deteksi sirkular).
export function deref(doc: unknown, node: unknown, stack: string[] = []): Deref {
  let cur = node;
  let ref: string | null = null;
  const seen = [...stack];
  for (let hop = 0; hop < MAX_HOPS; hop++) {
    if (!isObj(cur) || typeof cur.$ref !== 'string') return { node: cur, ref, circular: false, unresolved: false };
    const r = cur.$ref;
    const sibling: Obj = {};
    if (typeof cur.description === 'string') sibling.description = cur.description;
    if (typeof cur.summary === 'string') sibling.summary = cur.summary;
    if (seen.includes(r)) return { node: cur, ref: r, circular: true, unresolved: false };
    seen.push(r);
    const target = resolvePointer(doc, r);
    if (target === undefined) return { node: cur, ref: r, circular: false, unresolved: true };
    ref = ref ?? r;
    cur = isObj(target) && Object.keys(sibling).length > 0 ? { ...target, ...sibling } : target;
  }
  return { node: cur, ref, circular: true, unresolved: false };
}

// "#/components/schemas/Customer" -> "Customer"
export const refName = (ref: string): string => ref.split('/').pop()?.replace(/~1/g, '/').replace(/~0/g, '~') ?? ref;
