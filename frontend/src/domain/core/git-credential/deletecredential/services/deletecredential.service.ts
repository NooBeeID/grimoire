import { del } from '../../../../../shared/services/http';
import type { DeleteTarget } from '../types';

// GitHub tanpa ?host; GitLab selalu ?host=.
export async function deleteCredential(target: Pick<DeleteTarget, 'provider' | 'host'>): Promise<void> {
  await del('/api/me/git-credentials/' + encodeURIComponent(target.provider), {
    query: target.provider === 'gitlab' ? { host: target.host } : undefined,
  });
}
