import { z } from 'zod';

// Salinan lokal bentuk ApiKey (feature tidak saling import).
export const RevokedKeySchema = z.object({
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
export const RevokeKeyResponseSchema = z.object({ key: RevokedKeySchema });
export type RevokedKey = z.infer<typeof RevokedKeySchema>;
export interface RevokeTarget {
  id: string;
  name: string;
  prefix: string;
}
