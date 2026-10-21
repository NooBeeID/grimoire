import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockRevoked } from '../mocks/revoked';
import { revokeKey } from './revokekey.service';

afterEach(() => vi.unstubAllGlobals());

describe('revokeKey', () => {
  it('POST without body, with CSRF, encoded ids; returns key', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockRevoked()), { status: 200 }));
    vi.stubGlobal('fetch', f);
    const k = await revokeKey('p/1', 'k/1');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p%2F1/api-keys/k%2F1/revoke');
    expect(f.mock.calls[0]![1]!.method).toBe('POST');
    expect(f.mock.calls[0]![1]!.body).toBeUndefined();
    expect(f.mock.calls[0]![1]!.headers).toMatchObject({ 'X-Requested-With': 'grimoire' });
    expect(k.status).toBe('revoked');
  });
  it('invalid body -> INVALID_RESPONSE', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"key":{}}', { status: 200 })));
    await expect(revokeKey('p', 'k')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
