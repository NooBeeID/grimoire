import { describe, expect, it } from 'vitest';
import { mockCredential, mockCredentialList } from './mocks/credentials';
import { buildRows, GitCredentialListSchema, maskLast4 } from './types';

describe('GitCredentialListSchema', () => {
  it('accepts a valid response', () => {
    expect(GitCredentialListSchema.safeParse(mockCredentialList()).success).toBe(true);
  });
  it('rejects an extra token field', () => {
    const bad = mockCredentialList();
    (bad.items[0] as Record<string, unknown>).token = 'ghp_secret';
    expect(GitCredentialListSchema.safeParse(bad).success).toBe(false);
  });
  it('rejects last4 longer than 4 chars', () => {
    const bad = mockCredentialList({ items: [mockCredential({ last4: 'abcde' })] });
    expect(GitCredentialListSchema.safeParse(bad).success).toBe(false);
  });
});

describe('buildRows', () => {
  it('empty -> github + gitlab.com placeholder', () => {
    expect(buildRows([]).map((r) => [r.provider, r.host, r.saved])).toEqual([
      ['github', 'github.com', null],
      ['gitlab', 'gitlab.com', null],
    ]);
  });
  it('gitlab hosts sorted by host', () => {
    const rows = buildRows([
      mockCredential({ provider: 'gitlab', host: 'git.noobee.id' }),
      mockCredential({ provider: 'gitlab', host: 'gitlab.com' }),
    ]);
    expect(rows.map((r) => r.host)).toEqual(['github.com', 'git.noobee.id', 'gitlab.com']);
  });
  it('maskLast4', () => expect(maskLast4('7Qx2')).toBe('…7Qx2'));
});
