// Data mock sesuai skema GitCredentialList (api/modules/core/git-credential.yml).
import type { GitCredential, GitCredentialList } from '../types';

export const mockCredential = (over: Partial<GitCredential> = {}): GitCredential => ({
  provider: 'github',
  host: 'github.com',
  last4: 'a9F3',
  created_at: '2026-10-22T07:00:00Z',
  updated_at: '2026-10-22T07:00:00Z',
  ...over,
});

export const mockGitlabCredential = (over: Partial<GitCredential> = {}): GitCredential =>
  mockCredential({ provider: 'gitlab', host: 'git.noobee.id', last4: '7Qx2', created_at: '2026-10-22T07:05:00Z', updated_at: '2026-10-22T07:10:00Z', ...over });

export const mockCredentialList = (over: Partial<GitCredentialList> = {}): GitCredentialList => ({
  enabled: true,
  allowed_gitlab_hosts: ['gitlab.com', 'git.noobee.id'],
  items: [mockCredential(), mockGitlabCredential()],
  ...over,
});
