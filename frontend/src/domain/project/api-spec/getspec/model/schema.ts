import { asArr, asObj, asStr, isObj } from './guards';
import { deref, refName } from './refs';

// Label tipe ringkas; menormalkan 3.0 (nullable) dan 3.1 (type: [.., "null"]).
export function typeLabel(schema: unknown): string {
  if (!isObj(schema)) return 'any';
  if (typeof schema.$ref === 'string') return refName(schema.$ref);
  let base: string;
  const ty = schema.type;
  const types = Array.isArray(ty) ? ty.filter((x): x is string => typeof x === 'string') : typeof ty === 'string' ? [ty] : [];
  if (schema.nullable === true && !types.includes('null')) types.push('null');
  if (types.length === 0) {
    const comp = (['oneOf', 'anyOf', 'allOf'] as const).find((k) => Array.isArray(schema[k]));
    if (comp) base = `${comp}<${asArr(schema[comp]).map(typeLabel).join(', ')}>`;
    else if ('const' in schema) base = JSON.stringify(schema.const);
    else base = 'any';
  } else {
    base = types
      .map((x) => {
        if (x === 'array') return `array<${schema.items === undefined ? 'any' : typeLabel(schema.items)}>`;
        const f = asStr(schema.format);
        return f && x !== 'null' ? `${x} (${f})` : x;
      })
      .join(' | ');
  }
  return base;
}

export interface SchemaRow {
  name: string;
  schema: unknown;
  required: boolean;
  readOnly: boolean;
  writeOnly: boolean;
  deprecated: boolean;
  ref: string | null;
  circular: boolean;
  unresolved: boolean;
}

// Anak langsung dari sebuah skema (properties, items, additionalProperties). Tidak pernah rekursif sendiri.
export function schemaChildren(doc: unknown, schema: unknown, stack: string[] = []): SchemaRow[] {
  const here = deref(doc, schema, stack);
  const s = asObj(here.node);
  const nextStack = here.ref ? [...stack, here.ref] : stack;
  const required = new Set(asArr(s.required).filter((x): x is string => typeof x === 'string'));
  const row = (name: string, raw: unknown, req: boolean): SchemaRow => {
    const d = deref(doc, raw, nextStack);
    const n = asObj(d.node);
    return {
      name,
      schema: raw,
      required: req,
      readOnly: n.readOnly === true,
      writeOnly: n.writeOnly === true,
      deprecated: n.deprecated === true,
      ref: d.ref,
      circular: d.circular,
      unresolved: d.unresolved,
    };
  };
  const rows: SchemaRow[] = [];
  const props = s.properties;
  if (isObj(props)) for (const [k, v] of Object.entries(props)) rows.push(row(k, v, required.has(k)));
  if (s.items !== undefined) rows.push(row('[]', s.items, false));
  if (isObj(s.additionalProperties)) rows.push(row('*', s.additionalProperties, false));
  for (const k of ['oneOf', 'anyOf', 'allOf'] as const) {
    asArr(s[k]).forEach((v, i) => rows.push(row(`${k}[${i}]`, v, false)));
  }
  return rows;
}
