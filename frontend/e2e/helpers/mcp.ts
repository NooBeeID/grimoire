import { expect, type APIRequestContext, type APIResponse } from '@playwright/test';
import { z } from 'zod';

export const MCP_VERSIONS = ['2025-11-25', '2025-06-18', '2025-03-26'] as const;
export const UNAUTHORIZED_BODY = '{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}';

export type McpId = string | number;

/** Kirim body mentah (string). rawKey null -> tanpa header auth. headers menimpa default. */
export async function mcpPostRaw(
  request: APIRequestContext,
  rawKey: string | null,
  raw: string,
  headers: Record<string, string> = {},
): Promise<APIResponse> {
  const base: Record<string, string> = { 'Content-Type': 'application/json' };
  if (rawKey !== null) base.Authorization = `Bearer ${rawKey}`;
  return request.post('/mcp', { headers: { ...base, ...headers }, data: raw });
}

/** POST /mcp dengan body JSON. Memakai fixture `request` tanpa cookie sehingga terbukti tidak butuh sesi/CSRF. */
export function mcpPost(
  request: APIRequestContext,
  rawKey: string | null,
  body: unknown,
  headers: Record<string, string> = {},
): Promise<APIResponse> {
  return mcpPostRaw(request, rawKey, JSON.stringify(body), headers);
}

export const initializeBody = (id: McpId, version = '2025-06-18') => ({
  jsonrpc: '2.0',
  id,
  method: 'initialize',
  params: { protocolVersion: version, capabilities: {}, clientInfo: { name: 'grimoire-e2e', version: '1.0.0' } },
});
export const initializedNotification = () => ({ jsonrpc: '2.0', method: 'notifications/initialized' });
export const toolsListBody = (id: McpId) => ({ jsonrpc: '2.0', id, method: 'tools/list' });
export const pingBody = (id: McpId) => ({ jsonrpc: '2.0', id, method: 'ping' });
export const toolsCallBody = (id: McpId, name: unknown, args?: unknown) => ({
  jsonrpc: '2.0',
  id,
  method: 'tools/call',
  params: args === undefined ? { name } : { name, arguments: args },
});

const LOWER_ALNUM = 'abcdefghijklmnopqrstuvwxyz0123456789';
const URL_SAFE = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-';
const pick = (alphabet: string, n: number) =>
  Array.from({ length: n }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join('');

/** Key berformat benar tapi tidak terdaftar; prefix acak agar baris audit bisa difilter lewat target. */
export function randomWellFormedKey(): { rawKey: string; prefix: string } {
  const prefix = pick(LOWER_ALNUM, 8);
  return { rawKey: `gr_${prefix}_${pick(URL_SAFE, 43)}`, prefix };
}

export const RpcError = z.object({ code: z.number().int(), message: z.string(), data: z.unknown().optional() });
export const RpcSuccess = z.object({
  jsonrpc: z.literal('2.0'),
  id: z.union([z.string(), z.number()]),
  result: z.record(z.string(), z.unknown()),
});
export const RpcFailure = z.object({
  jsonrpc: z.literal('2.0'),
  id: z.union([z.string(), z.number(), z.null()]),
  error: RpcError,
});
export const InitializeResult = z.object({
  protocolVersion: z.enum(MCP_VERSIONS),
  capabilities: z.object({ tools: z.object({ listChanged: z.literal(false) }) }),
  serverInfo: z.object({ name: z.literal('grimoire'), version: z.string().min(1) }),
  instructions: z.string().min(1),
});
export const ToolsListResult = z.object({
  tools: z.array(
    z.object({
      name: z.string(),
      description: z.string(),
      inputSchema: z.record(z.string(), z.unknown()),
      annotations: z.object({ readOnlyHint: z.literal(true) }),
    }),
  ),
});

/** Assert status + parse body sebagai RpcFailure; mengembalikan error. */
export async function expectRpcFailure(res: APIResponse, status: number, code: number): Promise<z.infer<typeof RpcError>> {
  const text = await res.text();
  expect(res.status(), text).toBe(status);
  const { error } = RpcFailure.parse(JSON.parse(text));
  expect(error.code, text).toBe(code);
  return error;
}

/** Assert 200 + RpcSuccess; mengembalikan result. */
export async function expectRpcResult(res: APIResponse): Promise<Record<string, unknown>> {
  const text = await res.text();
  expect(res.status(), text).toBe(200);
  return RpcSuccess.parse(JSON.parse(text)).result;
}
