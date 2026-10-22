import { put } from '../../../../../shared/services/http';
import { SavedCredentialSchema, type GitProvider, type SavedCredential } from '../types';

// Token di-trim sebelum dikirim. Tidak menerima signal: membatalkan PUT yang sedang memverifikasi tidak bermakna.
export function saveCredential(provider: GitProvider, input: { token: string; host?: string }): Promise<SavedCredential> {
  const token = input.token.trim();
  const body = provider === 'gitlab' ? { host: input.host ?? '', token } : { token };
  return put('/api/me/git-credentials/' + encodeURIComponent(provider), body, { schema: SavedCredentialSchema });
}
