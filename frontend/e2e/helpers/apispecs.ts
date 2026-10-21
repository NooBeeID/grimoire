import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type APIRequestContext } from '@playwright/test';
import { z } from 'zod';
import { CSRF } from './auth';

// Helper e2e T09 discovery/api-spec-tools: fixture spec OpenAPI, upload, dan parser teks tool MCP.
// Zod di sini hanya untuk e2e, bukan kode aplikasi.

export interface SpecFile {
  file: string;
  content: string;
}
export interface SpecUploadResult {
  results: { file: string; name: string; status: string; reason_code: string | null; endpoint_count: number | null }[];
  summary: Record<'created' | 'updated' | 'unchanged' | 'skipped' | 'rejected', number>;
}

/** Upload multipart `files` ke endpoint T06; nama spec = nama file tanpa ekstensi. */
export async function uploadSpecsApi(request: APIRequestContext, projectId: string, files: SpecFile[]): Promise<SpecUploadResult> {
  const form = new FormData();
  for (const f of files) form.append('files', new Blob([new Uint8Array(Buffer.from(f.content))], { type: 'application/yaml' }), f.file);
  const res = await request.post(`/api/projects/${projectId}/api-specs/upload`, { headers: CSRF, multipart: form });
  expect(res.status(), `upload specs: ${await res.text()}`).toBe(200);
  const body = (await res.json()) as SpecUploadResult;
  expect(body.summary.rejected, JSON.stringify(body.results)).toBe(0);
  return body;
}

export const petstoreYaml = `openapi: 3.0.3
info:
  title: Swagger Petstore
  version: 1.0.0
servers:
  - url: https://petstore.noobee.id/v1
paths:
  /pets:
    get:
      operationId: listPets
      tags: [pets]
      summary: List all pets
      responses:
        '200':
          description: Daftar pet
    post:
      operationId: createPet
      tags: [pets]
      summary: Create a pet
      deprecated: true
      responses:
        '201':
          description: Pet dibuat
  /pets/{petId}:
    get:
      operationId: showPetById
      tags: [pets]
      summary: Info for a specific pet
      security:
        - apiKey: []
      parameters:
        - name: petId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Expected response to a valid request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
components:
  securitySchemes:
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
  schemas:
    Pet:
      type: object
      required: [id, name]
      properties:
        id:
          type: integer
          format: int64
        name:
          type: string
        owner:
          $ref: '#/components/schemas/Owner'
        children:
          type: array
          items:
            $ref: '#/components/schemas/Pet'
    Owner:
      type: object
      properties:
        name:
          type: string
        address:
          $ref: '#/components/schemas/Address'
    Address:
      type: object
      properties:
        city:
          type: string
        country:
          type: string
    Unused:
      type: object
      properties:
        legacy:
          type: string
`;

export const billingYaml = `openapi: 3.0.3
info:
  title: Billing API
  version: 2.1.0
paths:
  /invoices:
    get:
      operationId: listInvoices
      tags: [invoices]
      summary: List invoices
      responses:
        '200':
          description: Daftar invoice
  /invoices/{id}/pay:
    post:
      operationId: payInvoice
      tags: [invoices]
      summary: Pay an invoice
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Invoice dibayar
`;

export const soloYaml = `openapi: 3.0.3
info:
  title: Ledger Health
  version: 0.9.0
paths:
  /health:
    get:
      operationId: healthCheck
      tags: [ops]
      summary: Health check
      responses:
        '200':
          description: Layanan sehat
`;

/** n operasi GET /items/{i} (i = 000..n-1), operationId getItem<i>, tag "items". */
export function manyEndpointsYaml(n: number): string {
  const lines = ['openapi: 3.0.3', 'info:', '  title: Catalog API', '  version: 1.0.0', 'paths:'];
  for (let i = 0; i < n; i++) {
    const id = String(i).padStart(3, '0');
    lines.push(
      `  /items/${id}:`,
      '    get:',
      `      operationId: getItem${id}`,
      '      tags: [items]',
      `      summary: Get item ${id}`,
      '      responses:',
      "        '200':",
      '          description: OK',
    );
  }
  return `${lines.join('\n')}\n`;
}

const LEVELS = 8;
const PROPS_PER_LEVEL = 40;

/**
 * GET /report dengan skema bersarang 8 tingkat. Tiap tingkat punya 40 properti string berdeskripsi 60 karakter
 * dan dua rujukan ke tingkat berikutnya, sehingga hasil depth 8 jauh di atas 100352 byte
 * sementara file tetap puluhan KiB.
 */
export function hugeYaml(): string {
  const lines = [
    'openapi: 3.0.3',
    'info:',
    '  title: Huge Report',
    '  version: 1.0.0',
    'paths:',
    '  /report:',
    '    get:',
    '      operationId: getReport',
    '      tags: [reports]',
    '      summary: Huge report',
    '      responses:',
    "        '200':",
    '          description: OK',
    '          content:',
    '            application/json:',
    '              schema:',
    "                $ref: '#/components/schemas/Level1'",
    'components:',
    '  schemas:',
  ];
  for (let level = 1; level <= LEVELS; level++) {
    lines.push(`    Level${level}:`, '      type: object', '      properties:');
    for (let p = 0; p < PROPS_PER_LEVEL; p++) {
      const name = `field${String(p).padStart(2, '0')}`;
      const desc = `Level ${level} ${name} `.padEnd(60, 'x');
      lines.push(`        ${name}:`, '          type: string', `          description: ${desc}`);
    }
    if (level < LEVELS) {
      for (const k of ['left', 'right']) lines.push(`        ${k}:`, `          $ref: '#/components/schemas/Level${level + 1}'`);
    }
  }
  return `${lines.join('\n')}\n`;
}

export interface EndpointLine {
  group: string;
  method: string;
  path: string;
  operationId: string;
  tags: string[];
  deprecated: boolean;
}

const GROUP_LINE = /^== (.+) ==$/;
const ENDPOINT_LINE = /^(GET|PUT|POST|DELETE|OPTIONS|HEAD|PATCH|TRACE) (\/\S*)(?: — (.*?))?$/;

/** Baris endpoint dari teks list_api_endpoints. Baris "== x ==" mengganti group; header dan penanda diabaikan. */
export function parseEndpointLines(text: string): EndpointLine[] {
  const out: EndpointLine[] = [];
  let group = '';
  for (const line of text.split('\n')) {
    const g = GROUP_LINE.exec(line);
    if (g) {
      group = g[1];
      continue;
    }
    const m = ENDPOINT_LINE.exec(line);
    if (!m) continue;
    let rest = m[3] ?? '';
    const deprecated = rest.endsWith(' [deprecated]');
    if (deprecated) rest = rest.slice(0, -' [deprecated]'.length);
    const tagsMatch = /\s\(([^()]*)\)$/.exec(rest);
    if (tagsMatch) rest = rest.slice(0, tagsMatch.index);
    const opMatch = /\s?\[([^\]]*)\]$/.exec(rest);
    const operationId = opMatch ? opMatch[1] : '';
    out.push({
      group,
      method: m[1],
      path: m[2],
      operationId,
      tags: tagsMatch ? tagsMatch[1].split(',').map((t) => t.trim()).filter(Boolean) : [],
      deprecated,
    });
  }
  return out;
}

const NEXT_MARKER = /^\[ada \d+ endpoint lagi, panggil list_api_endpoints dengan offset=(\d+)[^\]]*\]$/;

/** N dari baris penanda "[ada K endpoint lagi, ... offset=N ...]"; null bila tidak ada. */
export function nextOffset(text: string): number | null {
  for (const line of text.split('\n')) {
    const m = NEXT_MARKER.exec(line);
    if (m) return Number(m[1]);
  }
  return null;
}

export const EndpointDetail = z.object({
  spec: z.string(),
  method: z.enum(['GET', 'PUT', 'POST', 'DELETE', 'OPTIONS', 'HEAD', 'PATCH', 'TRACE']),
  path: z.string().startsWith('/'),
  schema_depth: z.number().int().min(0).max(8),
  truncated: z.boolean(),
  hint: z.string().optional(),
  markers: z.object({ circular: z.array(z.string()), collapsed: z.array(z.string()), legend: z.string() }).optional(),
  operation: z.record(z.string(), z.unknown()),
  servers: z.array(z.record(z.string(), z.unknown())),
  securitySchemes: z.record(z.string(), z.unknown()),
});
export type EndpointDetailT = z.infer<typeof EndpointDetail>;

/** JSON.parse teks get_api_endpoint lalu validasi Zod. */
export function parseDetail(text: string): EndpointDetailT {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    throw new Error(`teks get_api_endpoint bukan JSON valid: ${text.slice(0, 80)}`);
  }
  return EndpointDetail.parse(json);
}

/** Baca fixture dari e2e/fixtures/apispec sebagai SpecFile (nama file = nama fixture kecuali `as`). */
export function readSpecFixture(name: string, as: string = name): SpecFile {
  const content = readFileSync(join(__dirname, '..', 'fixtures', 'apispec', name), 'utf8');
  return { file: as, content };
}

/** Seperti uploadSpecsApi tetapi tanpa assert rejected == 0 (untuk kasus negatif). */
export async function uploadSpecsRaw(request: APIRequestContext, projectId: string, files: SpecFile[]): Promise<SpecUploadResult> {
  const form = new FormData();
  for (const f of files) form.append('files', new Blob([new Uint8Array(Buffer.from(f.content))], { type: 'application/yaml' }), f.file);
  const res = await request.post(`/api/projects/${projectId}/api-specs/upload`, { headers: CSRF, multipart: form });
  expect(res.status(), `upload specs: ${await res.text()}`).toBe(200);
  return (await res.json()) as SpecUploadResult;
}
