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
  it('in_progress defaults to false, passes true through, rejects non-boolean', () => {
    const none = GetConfigResponseSchema.safeParse({ ...base, config: null });
    expect(none.success && none.data.in_progress).toBe(false);
    const yes = GetConfigResponseSchema.safeParse({ ...base, config: null, in_progress: true });
    expect(yes.success && yes.data.in_progress).toBe(true);
    expect(GetConfigResponseSchema.safeParse({ ...base, config: null, in_progress: 'yes' }).success).toBe(false);
  });
});
