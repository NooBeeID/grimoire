import { describe, expect, it } from 'vitest';
import { mockConfig } from '../base';
import { GetConfigResponseSchema } from './types';

const base = { kind: 'knowledge', feature_enabled: true, has_credential: true, credential_unreadable: false };
describe('GetConfigResponseSchema', () => {
  it('accepts a stored config', () => {
    expect(GetConfigResponseSchema.safeParse({ ...base, config: mockConfig() }).success).toBe(true);
  });
  it('accepts config null', () => {
    expect(GetConfigResponseSchema.safeParse({ ...base, kind: 'api_spec', has_credential: false, config: null }).success).toBe(true);
  });
  it('rejects an unknown kind', () => {
    expect(GetConfigResponseSchema.safeParse({ ...base, kind: 'other', config: null }).success).toBe(false);
  });
});
