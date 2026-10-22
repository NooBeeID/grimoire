import { z } from 'zod';

export type GitProvider = 'github' | 'gitlab';

// Salinan lokal bentuk respons (feature tidak saling import). .strict(): field "token" ditolak.
export const SavedCredentialSchema = z
  .object({
    provider: z.enum(['github', 'gitlab']),
    host: z.string().min(1),
    last4: z.string().min(1).max(4),
    updated_at: z.string().datetime({ offset: true }),
    replaced: z.boolean(),
  })
  .strict();
export type SavedCredential = z.infer<typeof SavedCredentialSchema>;

export interface SaveTarget {
  provider: GitProvider | null;
  host: string | null;
  gitlabHosts: string[];
}

export type TokenProblem = 'length' | 'charset' | null;

// trim; 8-512 karakter; ASCII 0x21-0x7E.
export function validateToken(raw: string): TokenProblem {
  const v = raw.trim();
  if (v.length < 8 || v.length > 512) return 'length';
  if (!/^[\x21-\x7e]+$/.test(v)) return 'charset';
  return null;
}

export const scopeFor = (p: GitProvider): string[] => (p === 'github' ? ['repo'] : ['read_api', 'read_repository']);
