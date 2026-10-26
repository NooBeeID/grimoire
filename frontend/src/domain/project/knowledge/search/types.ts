import { z } from 'zod';

export const SegmentSchema = z.object({ text: z.string(), highlight: z.boolean() });
export const SearchItemSchema = z.object({
  path: z.string(),
  title: z.string(),
  score: z.number(),
  snippet: z.array(SegmentSchema),
  // Nama heading bagian yang cocok (T20); kosong bila di luar heading atau respons lama.
  section: z.string().optional(),
});
export const SearchResultSchema = z.object({ items: z.array(SearchItemSchema), total: z.number().int().nonnegative(),
  // true bila sinonim atau koreksi typo dipakai (T20).
  expanded: z.boolean().optional(),
});
export type SearchResult = z.infer<typeof SearchResultSchema>;
export type SearchItem = z.infer<typeof SearchItemSchema>;

export const DEFAULT_LIMIT = 20;
export const MAX_LIMIT = 50;
export const MAX_QUERY_CHARS = 200;
