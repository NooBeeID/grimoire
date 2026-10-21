import { z } from 'zod';
import { hasControlChars, runeLength } from '../../../../shared/utils/text';

export const NAME_MIN = 2;
export const NAME_MAX = 60;
export const RAW_KEY_RE = /^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$/;

// Salinan lokal bentuk ApiKey (feature tidak saling import).
export const CreatedKeySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  prefix: z.string().regex(/^[a-z0-9]{8}$/),
  status: z.enum(['active', 'revoked']),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  last_used_at: z.string().datetime({ offset: true }).nullable(),
  revoked_at: z.string().datetime({ offset: true }).nullable(),
  revoked_by: z.string().nullable(),
  revoked_by_label: z.string().nullable(),
});
export const CreateKeyResponseSchema = z.object({
  key: CreatedKeySchema,
  raw_key: z.string().regex(RAW_KEY_RE),
  mcp_url: z.string().url().nullable(),
});
export type CreateKeyResponse = z.infer<typeof CreateKeyResponseSchema>;
export type CreatedKey = z.infer<typeof CreatedKeySchema>;
export type NameError = 'length' | 'control' | null;

// Cermin ValidateName backend; hanya UX, server tetap sumber kebenaran.
export function validateName(raw: string): { name: string; error: NameError } {
  const name = raw.trim();
  const n = runeLength(name);
  if (n < NAME_MIN || n > NAME_MAX) return { name, error: 'length' };
  if (hasControlChars(name)) return { name, error: 'control' };
  return { name, error: null };
}
