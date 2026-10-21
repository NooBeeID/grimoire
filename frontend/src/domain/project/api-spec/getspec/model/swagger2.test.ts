import { describe, expect, it } from 'vitest';
import { mockDocumentSwagger2 } from '../mocks/detail-swagger2';
import { OpenApiDocumentSchema } from '../types';
import type { Obj } from './guards';
import { adaptSwagger2, isSwagger2, rewriteRef, swagger2Servers } from './swagger2';
import { getOperation } from './operations';
import { schemaChildren } from './schema';

function deepFreeze<T>(o: T): T {
  if (typeof o === 'object' && o !== null) {
    Object.values(o).forEach(deepFreeze);
    Object.freeze(o);
  }
  return o;
}
const adapt = (doc: unknown): any => adaptSwagger2(doc as Obj);
const base = (extra: Obj = {}) => ({ swagger: '2.0', info: { title: 't', version: '1' }, paths: {}, ...extra });

describe('isSwagger2', () => {
  it('mengenali 2.0 string dan angka, menolak lainnya', () => {
    expect(isSwagger2({ swagger: '2.0' })).toBe(true);
    expect(isSwagger2({ swagger: 2 })).toBe(true);
    expect(isSwagger2({ openapi: '3.0.3' })).toBe(false);
    expect(isSwagger2({ openapi: '3.0.3', swagger: '2.0' })).toBe(false);
    expect(isSwagger2({ swagger: '1.2' })).toBe(false);
    expect(isSwagger2(null)).toBe(false);
    expect(isSwagger2([])).toBe(false);
  });
});

describe('adaptSwagger2', () => {
  it('menulis ulang $ref definitions/parameters/responses; ref lain dibiarkan', () => {
    expect(rewriteRef('#/definitions/A~1B')).toBe('#/components/schemas/A~1B');
    expect(rewriteRef('#/parameters/P')).toBe('#/components/parameters/P');
    expect(rewriteRef('#/responses/R')).toBe('#/components/responses/R');
    expect(rewriteRef('#/paths/~1x')).toBe('#/paths/~1x');
  });
  it('tidak menulis ulang $ref di example/default/enum; properti bernama example ditelusuri', () => {
    const r = adapt(
      base({
        definitions: {
          A: {
            type: 'object',
            example: { $ref: '#/definitions/X' },
            default: { $ref: '#/definitions/X' },
            properties: { example: { $ref: '#/definitions/B' } },
          },
        },
      }),
    );
    expect(r.components.schemas.A.example.$ref).toBe('#/definitions/X');
    expect(r.components.schemas.A.default.$ref).toBe('#/definitions/X');
    expect(r.components.schemas.A.properties.example.$ref).toBe('#/components/schemas/B');
  });
  it('body tanpa consumes -> application/json; dua MIME; operasi menimpa root; [] -> json', () => {
    const body = { name: 'b', in: 'body', required: true, description: 'isi', schema: { type: 'string' } };
    const doc = base({
      consumes: ['text/plain'],
      paths: {
        '/a': { post: { parameters: [body], consumes: ['application/json', 'application/xml'], responses: {} } },
        '/b': { post: { parameters: [body], responses: {} } },
        '/c': { post: { parameters: [body], consumes: [], responses: {} } },
      },
    });
    const r = adapt(doc);
    expect(Object.keys(r.paths['/a'].post.requestBody.content)).toEqual(['application/json', 'application/xml']);
    expect(Object.keys(r.paths['/b'].post.requestBody.content)).toEqual(['text/plain']);
    expect(Object.keys(r.paths['/c'].post.requestBody.content)).toEqual(['application/json']);
    expect(r.paths['/a'].post.requestBody.required).toBe(true);
    expect(r.paths['/a'].post.requestBody.description).toBe('isi');
    expect(r.paths['/a'].post.parameters).toEqual([]);
    expect(r.paths['/a'].post.consumes).toBeUndefined();
  });
  it('formData tanpa file -> urlencoded; dengan file -> multipart dan binary', () => {
    const r = adapt(
      base({
        paths: {
          '/u': { post: { parameters: [{ name: 'q', in: 'formData', type: 'string', required: true }], responses: {} } },
          '/f': { post: { parameters: [{ name: 'file', in: 'formData', type: 'file', required: true }, { name: 'c', in: 'formData', type: 'string', description: 'kat' }], responses: {} } },
        },
      }),
    );
    const u = r.paths['/u'].post.requestBody.content['application/x-www-form-urlencoded'].schema;
    expect(u.properties.q).toEqual({ type: 'string' });
    expect(u.required).toEqual(['q']);
    const f = r.paths['/f'].post.requestBody.content['multipart/form-data'].schema;
    expect(f.properties.file).toEqual({ type: 'string', format: 'binary' });
    expect(f.properties.c.description).toBe('kat');
  });
  it('parameter non-body memindahkan tipe ke schema dan x-example -> example', () => {
    const r = adapt(
      base({
        paths: { '/p': { get: { parameters: [{ name: 'q', in: 'query', type: 'array', items: { type: 'string' }, collectionFormat: 'csv', 'x-example': 'a', description: 'd' }], responses: {} } } },
      }),
    );
    const p = r.paths['/p'].get.parameters[0];
    expect(p.schema).toEqual({ type: 'array', items: { type: 'string' }, collectionFormat: 'csv' });
    expect(p.example).toBe('a');
    expect(p.description).toBe('d');
    expect(p.type).toBeUndefined();
  });
  it('parameter path-level via $ref di-resolve; operasi menang untuk in+name sama', () => {
    const r = adapt(
      base({
        parameters: { T: { name: 'X-T', in: 'header', type: 'string', required: true } },
        paths: { '/p': { parameters: [{ $ref: '#/parameters/T' }], get: { parameters: [{ name: 'X-T', in: 'header', type: 'integer' }], responses: {} }, post: { responses: {} } } },
      }),
    );
    expect(r.paths['/p'].get.parameters).toHaveLength(1);
    expect(r.paths['/p'].get.parameters[0].schema.type).toBe('integer');
    expect(r.paths['/p'].post.parameters[0].name).toBe('X-T');
  });
  it('parameter $ref ke body menjadi requestBody', () => {
    const r = adapt(
      base({
        parameters: { B: { name: 'b', in: 'body', required: true, schema: { $ref: '#/definitions/X' } } },
        paths: { '/p': { post: { parameters: [{ $ref: '#/parameters/B' }], responses: {} } } },
      }),
    );
    expect(r.paths['/p'].post.requestBody.content['application/json'].schema.$ref).toBe('#/components/schemas/X');
    expect(r.paths['/p'].post.parameters).toEqual([]);
  });
  it('respons: content per produces, default json, tanpa schema tanpa content, examples, $ref, header', () => {
    const r = adapt(
      base({
        responses: { NF: { description: 'nf', schema: { type: 'string' }, examples: { 'application/json': { a: 1 } } } },
        paths: {
          '/p': {
            get: {
              produces: ['text/plain', 'application/json'],
              responses: {
                '200': { description: 'ok', schema: { type: 'string' }, headers: { 'X-R': { type: 'integer', description: 'rate' } } },
                '204': { description: 'kosong' },
                '404': { $ref: '#/responses/NF' },
                '500': { description: 'x', examples: { 'text/html': '<b>' } },
              },
            },
          },
          '/q': { get: { responses: { '200': { description: 'ok', schema: { type: 'string' } } } } },
        },
      }),
    );
    const g = r.paths['/p'].get.responses;
    expect(Object.keys(g['200'].content)).toEqual(['text/plain', 'application/json']);
    expect(g['200'].headers['X-R']).toEqual({ description: 'rate', schema: { type: 'integer' } });
    expect(g['204'].content).toBeUndefined();
    expect(g['404'].content['text/plain'].schema).toEqual({ type: 'string' });
    expect(g['404'].content['application/json'].example).toEqual({ a: 1 });
    expect(g['500'].content['text/html'].example).toBe('<b>');
    expect(Object.keys(r.paths['/q'].get.responses['200'].content)).toEqual(['application/json']);
  });
  it('x-nullable -> nullable, termasuk bersarang dan items; type file -> binary', () => {
    const r = adapt(base({ definitions: { A: { type: 'object', properties: { a: { type: 'string', 'x-nullable': true }, l: { type: 'array', items: { type: 'integer', 'x-nullable': true } }, f: { type: 'file' } } } } }));
    const p = r.components.schemas.A.properties;
    expect(p.a.nullable).toBe(true);
    expect(p.l.items.nullable).toBe(true);
    expect(p.f).toEqual({ type: 'string', format: 'binary' });
  });
  it('discriminator string dan collectionFormat lewat apa adanya', () => {
    const r = adapt(base({ definitions: { A: { allOf: [{ $ref: '#/definitions/B' }], discriminator: 'kind' } } }));
    expect(r.components.schemas.A.discriminator).toBe('kind');
    expect(r.components.schemas.A.allOf[0].$ref).toBe('#/components/schemas/B');
  });
  it('securityDefinitions dipetakan', () => {
    const r = adapt(
      base({
        securityDefinitions: {
          b: { type: 'basic', description: 'd' },
          k: { type: 'apiKey', in: 'header', name: 'X' },
          i: { type: 'oauth2', flow: 'implicit', authorizationUrl: 'https://a', scopes: { s: 'x' } },
          p: { type: 'oauth2', flow: 'password', tokenUrl: 'https://t', scopes: {} },
          c: { type: 'oauth2', flow: 'application', tokenUrl: 'https://t', scopes: {} },
          a: { type: 'oauth2', flow: 'accessCode', authorizationUrl: 'https://a', tokenUrl: 'https://t', scopes: {} },
          z: { type: 'oauth2', flow: 'aneh', tokenUrl: 'https://t', scopes: {} },
          u: { type: 'baru', foo: 1 },
        },
      }),
    );
    const s = r.components.securitySchemes;
    expect(s.b).toEqual({ type: 'http', scheme: 'basic', description: 'd' });
    expect(s.k).toEqual({ type: 'apiKey', in: 'header', name: 'X' });
    expect(s.i.flows.implicit).toEqual({ authorizationUrl: 'https://a', scopes: { s: 'x' } });
    expect(s.p.flows.password.tokenUrl).toBe('https://t');
    expect(s.c.flows.clientCredentials.tokenUrl).toBe('https://t');
    expect(Object.keys(s.a.flows.authorizationCode)).toEqual(['authorizationUrl', 'tokenUrl', 'scopes']);
    expect(s.z.flows.aneh.tokenUrl).toBe('https://t');
    expect(s.u).toEqual({ type: 'baru', foo: 1 });
  });
  it.each([
    [{ host: 'h', basePath: '/v1', schemes: ['http', 'https'] }, ['http://h/v1', 'https://h/v1']],
    [{ host: 'h', basePath: '/v1' }, ['https://h/v1']],
    [{ basePath: '/v1', schemes: ['https'] }, ['/v1']],
    [{ host: 'h' }, ['https://h/']],
    [{}, ['/']],
    [{ host: 'h', schemes: ['http', 'http', 3, 'https'] }, ['http://h/', 'https://h/']],
  ])('server %j', (root, urls) => {
    expect(swagger2Servers(root as Obj).map((s) => s.url)).toEqual(urls);
  });
  it('definisi sirkular selesai dan ditandai circular', () => {
    const r = adapt(base({ definitions: { Node: { type: 'object', properties: { children: { type: 'array', items: { $ref: '#/definitions/Node' } } } } } }));
    expect(r.components.schemas.Node.properties.children.items.$ref).toBe('#/components/schemas/Node');
    const ref = { $ref: '#/components/schemas/Node' };
    const level1 = schemaChildren(r, ref);
    const items = schemaChildren(r, level1[0]!.schema, ['#/components/schemas/Node']);
    expect(items[0]!.circular).toBe(true);
  });
  it('trace dan kunci path item lain disalin apa adanya', () => {
    const r = adapt(base({ paths: { '/p': { trace: { a: 1 }, 'x-y': 2 } } }));
    expect(r.paths['/p'].trace).toEqual({ a: 1 });
    expect(r.paths['/p']['x-y']).toBe(2);
  });
  it.each([
    ['paths array', { paths: [] }],
    ['operasi string', { paths: { '/p': { get: 'x' } } }],
    ['parameters objek', { paths: { '/p': { get: { parameters: {}, responses: {} } } } }],
    ['responses null', { paths: { '/p': { get: { responses: null } } } }],
    ['securityDefinitions array', { securityDefinitions: [] }],
    ['definitions angka', { definitions: 5 }],
  ])('bentuk tak dikenal tidak melempar: %s', (_n, extra) => {
    expect(() => adaptSwagger2(base(extra as Obj))).not.toThrow();
  });
  it('tidak memutasi masukan dan hasil memenuhi OpenApiDocumentSchema', () => {
    const doc = deepFreeze(JSON.parse(JSON.stringify(mockDocumentSwagger2)));
    const before = JSON.stringify(doc);
    const r = adaptSwagger2(doc);
    expect(JSON.stringify(doc)).toBe(before);
    expect(OpenApiDocumentSchema.safeParse(r).success).toBe(true);
    expect(r.swagger).toBeUndefined();
    expect((r.info as Obj).title).toBe('HR Platform API');
  });
  it('mock hr-platform menghasilkan operasi yang dapat dibaca model', () => {
    const r = adapt(mockDocumentSwagger2);
    expect(r.servers).toEqual([{ url: 'https://api.hr-platform.noobee.id/v1' }]);
    expect(getOperation(r, 'POST', '/employees')).not.toBeNull();
    expect(r.components.securitySchemes.basicAuth.scheme).toBe('basic');
  });
  it('kinerja dokumen besar sintetis', () => {
    const paths: Obj = {};
    const definitions: Obj = {};
    for (let i = 0; i < 1500; i++) definitions['D' + i] = { type: 'object', properties: { r: { $ref: `#/definitions/D${(i + 1) % 1500}` } } };
    for (let i = 0; i < 600; i++) {
      const op = { parameters: [{ name: 'b', in: 'body', schema: { $ref: `#/definitions/D${i}` } }], responses: { '200': { description: 'ok', schema: { $ref: `#/definitions/D${i}` } } } };
      paths['/p' + i] = { get: op, post: op };
    }
    const t0 = performance.now();
    expect(() => adaptSwagger2(base({ paths, definitions }))).not.toThrow();
    console.info(`adaptSwagger2 dokumen besar: ${Math.round(performance.now() - t0)} ms`);
  });
});
