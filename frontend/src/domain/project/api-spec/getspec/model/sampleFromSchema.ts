import { asArr, asObj, isObj } from './guards';
import { deref } from './refs';

const MAX_DEPTH = 6;

export type SampleMode = 'request' | 'response';

const byFormat = (format: string | undefined): string => {
  switch (format) {
    case 'date-time': return '2026-10-24T10:00:00Z';
    case 'date': return '2026-10-24';
    case 'time': return '10:00:00';
    case 'email': return 'user@example.com';
    case 'uuid': return '3fa85f64-5717-4562-b3fc-2c963f66afa6';
    case 'uri':
    case 'url': return 'https://example.com';
    case 'hostname': return 'example.com';
    case 'ipv4': return '192.0.2.1';
    case 'ipv6': return '2001:db8::1';
    case 'byte': return 'U3dhZ2dlcg==';
    case 'binary': return '(binary)';
    case 'password': return '********';
    default: return 'string';
  }
};

// Contoh nilai dari skema, murni dan tanpa efek samping. Urutan: example > default > enum/const > turunan dari tipe.
// request melewati properti readOnly, response melewati writeOnly. Sirkular dan melewati batas kedalaman jadi null.
export function sampleFromSchema(doc: unknown, schema: unknown, mode: SampleMode = 'request', stack: string[] = [], depth = 0): unknown {
  if (depth > MAX_DEPTH) return null;
  const d = deref(doc, schema, stack);
  if (d.circular || d.unresolved) return null;
  const s = asObj(d.node);
  const next = d.ref ? [...stack, d.ref] : stack;
  if ('example' in s) return s.example;
  if (Array.isArray(s.examples) && s.examples.length > 0) return s.examples[0];
  if ('default' in s) return s.default;
  if ('const' in s) return s.const;
  if (Array.isArray(s.enum) && s.enum.length > 0) return s.enum[0];

  if (Array.isArray(s.allOf) && s.allOf.length > 0) {
    const parts = s.allOf.map((p) => sampleFromSchema(doc, p, mode, next, depth + 1));
    return parts.every(isObj) ? Object.assign({}, ...parts) : parts.find((p) => p !== null) ?? null;
  }
  for (const k of ['oneOf', 'anyOf'] as const) {
    const list = asArr(s[k]);
    if (list.length > 0) return sampleFromSchema(doc, list[0], mode, next, depth + 1);
  }

  const ty = Array.isArray(s.type) ? s.type.find((x) => x !== 'null') : s.type;
  const props = s.properties;
  if (ty === 'object' || (ty === undefined && isObj(props))) {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(asObj(props))) {
      const child = asObj(deref(doc, v, next).node);
      if (mode === 'request' ? child.readOnly === true : child.writeOnly === true) continue;
      out[k] = sampleFromSchema(doc, v, mode, next, depth + 1);
    }
    if (Object.keys(out).length === 0 && isObj(s.additionalProperties)) {
      out.key = sampleFromSchema(doc, s.additionalProperties, mode, next, depth + 1);
    }
    return out;
  }
  if (ty === 'array') return s.items === undefined ? [] : [sampleFromSchema(doc, s.items, mode, next, depth + 1)];
  if (ty === 'integer') return typeof s.minimum === 'number' ? s.minimum : 0;
  if (ty === 'number') return typeof s.minimum === 'number' ? s.minimum : 0;
  if (ty === 'boolean') return true;
  if (ty === 'string') return byFormat(typeof s.format === 'string' ? s.format : undefined);
  return null;
}
