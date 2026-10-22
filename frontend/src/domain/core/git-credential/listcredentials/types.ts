import { z } from 'zod';

export const GIT_PROVIDERS = ['github', 'gitlab'] as const;
export type GitProvider = (typeof GIT_PROVIDERS)[number];

// .strict(): field tak dikenal (termasuk "token") menjadi INVALID_RESPONSE dan tidak pernah masuk store.
export const GitCredentialSchema = z
  .object({
    provider: z.enum(GIT_PROVIDERS),
    host: z.string().min(1),
    last4: z.string().min(1).max(4),
    created_at: z.string().datetime({ offset: true }),
    updated_at: z.string().datetime({ offset: true }),
  })
  .strict();
export const GitCredentialListSchema = z
  .object({
    enabled: z.boolean(),
    allowed_gitlab_hosts: z.array(z.string().min(1)),
    items: z.array(GitCredentialSchema),
  })
  .strict();
export type GitCredential = z.infer<typeof GitCredentialSchema>;
export type GitCredentialList = z.infer<typeof GitCredentialListSchema>;

export interface TokenRow {
  provider: GitProvider;
  host: string;
  saved: GitCredential | null;
}

// GitHub (github.com) selalu ada; GitLab satu baris per host tersimpan; tanpa GitLab sama sekali -> placeholder gitlab.com.
export function buildRows(items: GitCredential[]): TokenRow[] {
  const gh = items.find((i) => i.provider === 'github');
  const rows: TokenRow[] = [{ provider: 'github', host: gh?.host ?? 'github.com', saved: gh ?? null }];
  const gl = items.filter((i) => i.provider === 'gitlab').sort((a, b) => a.host.localeCompare(b.host));
  if (gl.length === 0) rows.push({ provider: 'gitlab', host: 'gitlab.com', saved: null });
  for (const c of gl) rows.push({ provider: 'gitlab', host: c.host, saved: c });
  return rows;
}

export const maskLast4 = (last4: string): string => `…${last4}`;
export const providerLabel = (p: GitProvider): string => (p === 'github' ? 'GitHub' : 'GitLab');

// Permintaan membuka dialog Simpan dari tautan "Tambah host GitLab".
export interface SaveRequest {
  provider: GitProvider | null;
  host: string | null;
  gitlabHosts: string[];
}
