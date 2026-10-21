import { asArr, asObj, asStr, isObj, type Obj } from './guards';
import { resolvePointer } from './refs';

// Pemetaan Swagger 2.0 -> bentuk tampilan 3.x, hanya di memori. Murni, tidak memutasi, tidak melempar untuk bentuk tak dikenal.

const METHODS = ['get', 'put', 'post', 'delete', 'options', 'head', 'patch'] as const;
const PARAM_SCHEMA_KEYS = [
  'type', 'format', 'items', 'enum', 'default', 'maximum', 'exclusiveMaximum', 'minimum', 'exclusiveMinimum',
  'maxLength', 'minLength', 'pattern', 'maxItems', 'minItems', 'uniqueItems', 'multipleOf', 'collectionFormat',
] as const;
const MAX_HOPS = 16;

export function isSwagger2(doc: unknown): boolean {
  return isObj(doc) && !('openapi' in doc) && (doc.swagger === '2.0' || doc.swagger === 2);
}

export function rewriteRef(ref: string): string {
  if (ref.startsWith('#/definitions/')) return '#/components/schemas/' + ref.slice('#/definitions/'.length);
  if (ref.startsWith('#/parameters/')) return '#/components/parameters/' + ref.slice('#/parameters/'.length);
  if (ref.startsWith('#/responses/')) return '#/components/responses/' + ref.slice('#/responses/'.length);
  return ref;
}

const SCHEMA_MAPS = ['properties', 'patternProperties'] as const;
const SCHEMA_LISTS = ['allOf', 'anyOf', 'oneOf'] as const;

export function adaptSchema(node: unknown): unknown {
  if (Array.isArray(node)) return node.map(adaptSchema);
  if (!isObj(node)) return node;
  const out: Obj = { ...node };
  if (typeof node.$ref === 'string') out.$ref = rewriteRef(node.$ref);
  if (node['x-nullable'] === true) out.nullable = true;
  if (node.type === 'file') {
    out.type = 'string';
    out.format = 'binary';
  }
  for (const k of SCHEMA_MAPS) {
    if (isObj(node[k])) out[k] = Object.fromEntries(Object.entries(node[k] as Obj).map(([n, v]) => [n, adaptSchema(v)]));
  }
  for (const k of SCHEMA_LISTS) if (Array.isArray(node[k])) out[k] = (node[k] as unknown[]).map(adaptSchema);
  for (const k of ['additionalProperties', 'items', 'not'] as const) if (k in node) out[k] = adaptSchema(node[k]);
  return out;
}

export function paramSchema(p: Obj): Obj {
  const schema: Obj = {};
  const rest: Obj = {};
  for (const [k, v] of Object.entries(p)) {
    if ((PARAM_SCHEMA_KEYS as readonly string[]).includes(k)) schema[k] = v;
    else if (k === 'x-example') rest.example = v;
    else rest[k] = v;
  }
  if (typeof p.$ref === 'string') return { ...rest, $ref: rewriteRef(p.$ref) };
  return { ...rest, schema: adaptSchema(schema) };
}

const effective = (op: Obj, root: Obj, key: 'consumes' | 'produces'): string[] => {
  const pick = Array.isArray(op[key]) ? op[key] : Array.isArray(root[key]) ? root[key] : [];
  return (pick as unknown[]).filter((m): m is string => typeof m === 'string');
};

// $ref '#/parameters/X' atau '#/responses/X' di-resolve satu langkah dari dokumen asli.
function resolveLocal(doc: Obj, node: unknown): unknown {
  let cur = node;
  for (let hop = 0; hop < MAX_HOPS; hop++) {
    if (!isObj(cur) || typeof cur.$ref !== 'string') return cur;
    if (!cur.$ref.startsWith('#/parameters/') && !cur.$ref.startsWith('#/responses/')) return cur;
    const target = resolvePointer(doc, cur.$ref);
    if (target === undefined) return { ...cur, $ref: rewriteRef(cur.$ref) };
    cur = target;
  }
  return cur;
}

const paramKey = (p: Obj) => `${asStr(p.in) ?? ''}:${asStr(p.name) ?? ''}`;

function adaptResponse(doc: Obj, res: unknown, produces: string[]): unknown {
  const r = resolveLocal(doc, res);
  if (!isObj(r)) return r;
  if (typeof r.$ref === 'string') return { ...r, $ref: rewriteRef(r.$ref) };
  const out: Obj = {};
  for (const [k, v] of Object.entries(r)) if (!['schema', 'examples', 'headers'].includes(k)) out[k] = v;
  if (isObj(r.headers)) {
    out.headers = Object.fromEntries(
      Object.entries(r.headers).map(([n, h]) => {
        const ho = asObj(h);
        const ps = paramSchema(ho);
        return [n, { ...(typeof ho.description === 'string' ? { description: ho.description } : {}), schema: ps.schema }];
      }),
    );
  }
  const mimes = produces.length > 0 ? produces : ['application/json'];
  const content: Obj = {};
  if (r.schema !== undefined) for (const m of mimes) content[m] = { schema: adaptSchema(r.schema) };
  if (isObj(r.examples)) {
    for (const [m, ex] of Object.entries(r.examples)) {
      content[m] = { ...(r.schema !== undefined ? { schema: adaptSchema(r.schema) } : {}), ...asObj(content[m]), example: ex };
    }
  }
  if (Object.keys(content).length > 0) out.content = content;
  return out;
}

function adaptResponses(doc: Obj, responses: unknown, produces: string[]): unknown {
  if (!isObj(responses)) return responses;
  return Object.fromEntries(Object.entries(responses).map(([code, r]) => [code, adaptResponse(doc, r, produces)]));
}

function adaptOperation(doc: Obj, item: Obj, op: Obj): Obj {
  const consumes = effective(op, doc, 'consumes');
  const produces = effective(op, doc, 'produces');
  const merged = new Map<string, Obj>();
  for (const src of [asArr(item.parameters), asArr(op.parameters)]) {
    for (const raw of src) {
      const p = resolveLocal(doc, raw);
      if (isObj(p)) merged.set(paramKey(p), p);
    }
  }
  const params: Obj[] = [];
  let body: Obj | null = null;
  const form: Obj[] = [];
  for (const p of merged.values()) {
    if (p.in === 'body') body = p;
    else if (p.in === 'formData') form.push(p);
    else params.push(paramSchema(p) as Obj);
  }
  const out: Obj = {};
  for (const [k, v] of Object.entries(op)) if (!['parameters', 'consumes', 'produces', 'responses'].includes(k)) out[k] = v;
  out.parameters = params;
  if (body) {
    const b: Obj = body;
    const mimes = consumes.length > 0 ? consumes : ['application/json'];
    out.requestBody = {
      ...(typeof b.description === 'string' ? { description: b.description } : {}),
      ...(b.required === true ? { required: true } : {}),
      content: Object.fromEntries(mimes.map((m) => [m, { schema: adaptSchema(b.schema) }])),
    };
  } else if (form.length > 0) {
    const hasFile = form.some((p) => p.type === 'file');
    const properties: Obj = {};
    const required: string[] = [];
    for (const p of form) {
      const name = asStr(p.name);
      if (name === null) continue;
      const ps = paramSchema(p);
      properties[name] = { ...asObj(ps.schema), ...(typeof p.description === 'string' ? { description: p.description } : {}) };
      if (p.required === true) required.push(name);
    }
    out.requestBody = {
      ...(required.length > 0 ? { required: true } : {}),
      content: {
        [hasFile ? 'multipart/form-data' : 'application/x-www-form-urlencoded']: {
          schema: { type: 'object', properties, ...(required.length > 0 ? { required } : {}) },
        },
      },
    };
  }
  if ('responses' in op) out.responses = adaptResponses(doc, op.responses, produces);
  return out;
}

export function adaptSecurity(defs: unknown): Obj {
  if (!isObj(defs)) return {};
  const out: Obj = {};
  for (const [name, raw] of Object.entries(defs)) {
    const d = asObj(raw);
    if (d.type === 'basic') {
      out[name] = { type: 'http', scheme: 'basic', ...(typeof d.description === 'string' ? { description: d.description } : {}) };
    } else if (d.type === 'oauth2') {
      const flow = asStr(d.flow) ?? '';
      const scopes = asObj(d.scopes);
      const au = d.authorizationUrl;
      const tu = d.tokenUrl;
      const map: Record<string, string> = { implicit: 'implicit', password: 'password', application: 'clientCredentials', accessCode: 'authorizationCode' };
      const key = map[flow] ?? flow;
      const f: Obj = {};
      if (au !== undefined && (flow === 'implicit' || flow === 'accessCode' || !(flow in map))) f.authorizationUrl = au;
      if (tu !== undefined && flow !== 'implicit') f.tokenUrl = tu;
      f.scopes = scopes;
      const rest: Obj = {};
      for (const [k, v] of Object.entries(d)) if (!['flow', 'authorizationUrl', 'tokenUrl', 'scopes'].includes(k)) rest[k] = v;
      out[name] = { ...rest, flows: { [key]: f } };
    } else out[name] = d;
  }
  return out;
}

export function swagger2Servers(root: Obj): { url: string }[] {
  const host = typeof root.host === 'string' && root.host !== '' ? root.host : '';
  const basePath = typeof root.basePath === 'string' && root.basePath !== '' ? root.basePath : '/';
  const schemes: string[] = [];
  for (const s of asArr(root.schemes)) if (typeof s === 'string' && s !== '' && !schemes.includes(s)) schemes.push(s);
  if (host === '') return [{ url: basePath }];
  const list = schemes.length > 0 ? schemes : ['https'];
  return list.map((s) => ({ url: `${s}://${host}${basePath}` }));
}

export function adaptSwagger2(doc: Obj): Obj {
  const out: Obj = { openapi: '3.0.3', servers: swagger2Servers(doc) };
  for (const [k, v] of Object.entries(doc)) {
    if (['info', 'tags', 'externalDocs', 'security'].includes(k) || k.startsWith('x-')) out[k] = v;
  }
  const paths: Obj = {};
  for (const [path, rawItem] of Object.entries(asObj(doc.paths))) {
    if (!isObj(rawItem)) {
      paths[path] = rawItem;
      continue;
    }
    const item: Obj = {};
    for (const [k, v] of Object.entries(rawItem)) if (k !== 'parameters' && !(METHODS as readonly string[]).includes(k)) item[k] = v;
    const shared = asArr(rawItem.parameters)
      .map((p) => resolveLocal(doc, p))
      .filter((p) => isObj(p) && p.in !== 'body' && p.in !== 'formData')
      .map((p) => paramSchema(p as Obj));
    if (rawItem.parameters !== undefined) item.parameters = shared;
    for (const m of METHODS) {
      if (isObj(rawItem[m])) item[m] = adaptOperation(doc, rawItem, rawItem[m] as Obj);
      else if (m in rawItem) item[m] = rawItem[m];
    }
    paths[path] = item;
  }
  out.paths = paths;
  const produces = effective({}, doc, 'produces');
  out.components = {
    schemas: Object.fromEntries(Object.entries(asObj(doc.definitions)).map(([n, s]) => [n, adaptSchema(s)])),
    parameters: Object.fromEntries(
      Object.entries(asObj(doc.parameters)).map(([n, p]) => {
        const po = asObj(p);
        return [n, po.in === 'body' || po.in === 'formData' ? po : paramSchema(po)];
      }),
    ),
    responses: Object.fromEntries(Object.entries(asObj(doc.responses)).map(([n, r]) => [n, adaptResponse(doc, r, produces)])),
    securitySchemes: adaptSecurity(doc.securityDefinitions),
  };
  return out;
}
