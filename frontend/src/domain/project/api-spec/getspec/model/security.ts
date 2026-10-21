import { asArr, asObj, asStr, isObj } from './guards';
import { deref } from './refs';

export interface UsedScheme {
  name: string;
  scheme: Record<string, unknown> | null; // null = nama tidak ada di components.securitySchemes
  scopes: string[];
}

function schemeOf(doc: unknown, name: string): Record<string, unknown> | null {
  const raw = asObj(asObj(asObj(doc).components).securitySchemes)[name];
  if (raw === undefined) return null;
  const d = deref(doc, raw);
  return d.unresolved || d.circular || !isObj(d.node) ? null : d.node;
}

// Skema yang dipakai oleh security requirement (OR antar objek, AND di dalam objek); digabung per nama.
export function usedSchemes(doc: unknown, security: unknown[] | null): UsedScheme[] {
  const out = new Map<string, UsedScheme>();
  for (const req of asArr(security)) {
    for (const [name, scopes] of Object.entries(asObj(req))) {
      const cur = out.get(name) ?? { name, scheme: schemeOf(doc, name), scopes: [] };
      for (const s of asArr(scopes)) if (typeof s === 'string' && !cur.scopes.includes(s)) cur.scopes.push(s);
      out.set(name, cur);
    }
  }
  return [...out.values()];
}

// `security: []` (atau requirement kosong `{}`) berarti tanpa autentikasi; null = tidak didefinisikan.
export function isNoAuth(security: unknown[] | null): boolean {
  return security !== null && (security.length === 0 || security.some((r) => isObj(r) && Object.keys(r).length === 0));
}

export interface SchemeInfo {
  name: string;
  scheme: Record<string, unknown>;
  type: string | null;
}

// Semua components.securitySchemes (untuk ringkasan API).
export function allSchemes(doc: unknown): SchemeInfo[] {
  const out: SchemeInfo[] = [];
  for (const name of Object.keys(asObj(asObj(asObj(doc).components).securitySchemes))) {
    const scheme = schemeOf(doc, name);
    if (scheme) out.push({ name, scheme, type: asStr(scheme.type) });
  }
  return out;
}
