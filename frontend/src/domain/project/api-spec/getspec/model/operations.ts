import { asArr, asObj, asStr, isObj, type Obj } from './guards';
import { deref } from './refs';
import { HTTP_METHODS, type ApiSpecEndpoint } from '../types';

export const METHOD_ORDER: readonly string[] = HTTP_METHODS;
export const UNTAGGED = '__untagged';

export const endpointKey = (method: string, path: string): string => `${method} ${path}`;
export function parseOpParam(op: string | null): { method: string; path: string } | null {
  if (!op) return null;
  const i = op.indexOf(' ');
  if (i <= 0) return null;
  const method = op.slice(0, i);
  const path = op.slice(i + 1);
  return METHOD_ORDER.includes(method) && path.startsWith('/') ? { method, path } : null;
}

export interface TagGroup {
  name: string;
  description: string | null;
  endpoints: ApiSpecEndpoint[];
}

const methodRank = (m: string) => {
  const i = METHOD_ORDER.indexOf(m);
  return i < 0 ? METHOD_ORDER.length : i;
};
const byPathThenMethod = (a: ApiSpecEndpoint, b: ApiSpecEndpoint) =>
  a.path < b.path ? -1 : a.path > b.path ? 1 : methodRank(a.method) - methodRank(b.method);

// Urutan tag: doc.tags[] lalu urutan kemunculan; tanpa tag -> grup terakhir. Endpoint multi-tag muncul di tiap tag.
export function buildTagTree(doc: unknown, endpoints: ApiSpecEndpoint[]): TagGroup[] {
  const order: string[] = [];
  const desc = new Map<string, string | null>();
  for (const t of asArr(asObj(doc).tags)) {
    const name = asStr(asObj(t).name);
    if (name !== null && !order.includes(name)) {
      order.push(name);
      desc.set(name, asStr(asObj(t).description));
    }
  }
  const groups = new Map<string, ApiSpecEndpoint[]>();
  const add = (k: string, e: ApiSpecEndpoint) => groups.set(k, [...(groups.get(k) ?? []), e]);
  for (const e of endpoints) {
    if (e.tags.length === 0) add(UNTAGGED, e);
    for (const t of e.tags) {
      if (!order.includes(t)) order.push(t);
      add(t, e);
    }
  }
  const out: TagGroup[] = [];
  for (const name of order) {
    const eps = groups.get(name);
    if (eps) out.push({ name, description: desc.get(name) ?? null, endpoints: [...eps].sort(byPathThenMethod) });
  }
  const un = groups.get(UNTAGGED);
  if (un) out.push({ name: UNTAGGED, description: null, endpoints: [...un].sort(byPathThenMethod) });
  return out;
}

export interface OperationInfo {
  operation: Obj;
  params: Obj[];
  servers: unknown[];
  security: unknown[] | null; // null = tidak didefinisikan (pakai root)
}

function buildInfo(doc: unknown, item: Obj, method: string): OperationInfo | null {
  const root = asObj(doc);
  const rawOp = item[method.toLowerCase()];
  if (!isObj(rawOp)) return null;
  const op = rawOp;
  const resolveParam = (p: unknown) => asObj(deref(doc, p).node);
  const merged = new Map<string, Obj>();
  for (const p of [...asArr(item.parameters), ...asArr(op.parameters)]) {
    const r = resolveParam(p);
    merged.set(`${asStr(r.in) ?? ''}:${asStr(r.name) ?? ''}`, r);
  }
  const pick = (k: string): unknown[] => {
    const a = op[k] ?? item[k] ?? root[k];
    return Array.isArray(a) ? a : [];
  };
  return {
    operation: op,
    params: [...merged.values()],
    servers: pick('servers'),
    security: Array.isArray(op.security) ? op.security : Array.isArray(root.security) ? root.security : null,
  };
}

// Menggabungkan parameter level path dengan operasi (operasi menang untuk name+in yang sama).
export function getOperation(doc: unknown, method: string, path: string): OperationInfo | null {
  const item = asObj(deref(doc, asObj(asObj(doc).paths)[path]).node);
  return buildInfo(doc, item, method);
}

export interface WebhookOperation {
  name: string;
  method: string;
  info: OperationInfo;
}

// Operasi dari `webhooks` (OpenAPI 3.1): satu entri per method per webhook.
export function getWebhookOperations(doc: unknown): WebhookOperation[] {
  const out: WebhookOperation[] = [];
  for (const [name, raw] of Object.entries(asObj(asObj(doc).webhooks))) {
    const item = asObj(deref(doc, raw).node);
    for (const method of METHOD_ORDER) {
      const info = buildInfo(doc, item, method);
      if (info) out.push({ name, method, info });
    }
  }
  return out;
}
