import { expect, type APIRequestContext } from '@playwright/test';
import { z } from 'zod';
import { RpcSuccess, mcpPost, toolsCallBody } from './mcp';

export const ToolCallResult = z.object({
  content: z.array(z.object({ type: z.literal('text'), text: z.string() })).min(1),
  isError: z.boolean(),
  structuredContent: z.record(z.string(), z.unknown()).optional(),
  _meta: z.record(z.string(), z.unknown()).optional(),
});
export type ToolCallResultT = z.infer<typeof ToolCallResult>;

export const ListStructured = z.object({
  items: z.array(z.object({ path: z.string(), size_bytes: z.number().int().nonnegative(), updated_at: z.string() })),
  folders: z.array(z.string()),
  total: z.number().int().nonnegative(),
  next_offset: z.number().int().nullable(),
  recursive: z.boolean(),
});
export const SearchStructured = z.object({
  items: z.array(z.object({ path: z.string(), score: z.number(), snippet: z.string() })),
  total: z.number().int().nonnegative(),
  query: z.string(),
});

let seq = 1000;

/** tools/call; gagal bila respons JSON-RPC berisi `error`. */
export async function callTool(request: APIRequestContext, key: string, name: string, args: unknown): Promise<ToolCallResultT> {
  const res = await mcpPost(request, key, toolsCallBody(seq++, name, args));
  const text = await res.text();
  expect(res.status(), text).toBe(200);
  return ToolCallResult.parse(RpcSuccess.parse(JSON.parse(text)).result);
}

export const textOf = (r: ToolCallResultT): string => r.content[0].text;

const MARKER_LINE = /^\[(terpotong|ada \d+ file lagi)[^\]]*offset=(\d+)[^\]]*\]$/;

/** Ambil N dari penanda di baris terakhir yang diawali '['; null bila tidak ada penanda. */
export function nextOffset(text: string): number | null {
  const last = text.split('\n').at(-1) ?? '';
  const m = MARKER_LINE.exec(last);
  return m ? Number(m[2]) : null;
}

/** Buang penanda dan dua `\n` di depannya; isi tanpa penanda tidak berubah. */
export function stripMarker(text: string): string {
  const idx = text.lastIndexOf('\n\n[');
  if (idx < 0) return text;
  return MARKER_LINE.test(text.slice(idx + 2)) ? text.slice(0, idx) : text;
}
