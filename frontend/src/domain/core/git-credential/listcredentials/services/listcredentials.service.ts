import { get } from '../../../../../shared/services/http';
import { GitCredentialListSchema, type GitCredentialList } from '../types';

export function listCredentials(signal?: AbortSignal): Promise<GitCredentialList> {
  return get('/api/me/git-credentials', { schema: GitCredentialListSchema, signal });
}
