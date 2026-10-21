import { afterEach, describe, expect, it, vi } from 'vitest';
import { reasonOf } from '../../../../../shared/validation/error';
import { mockCreated } from '../mocks/created';
import { createKey } from './createkey.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('createKey', () => {
  it('POST body + CSRF, validates 201', async () => {
    const f = stub(201, mockCreated());
    const r = await createKey('p1', 'claude-code-budi');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/api-keys');
    expect(f.mock.calls[0]![1]!.body).toBe('{"name":"claude-code-budi"}');
    expect(f.mock.calls[0]![1]!.headers).toMatchObject({ 'X-Requested-With': 'grimoire' });
    expect(r.raw_key).toMatch(/^gr_/);
  });
  it('409 NAME_TAKEN exposes reason', async () => {
    stub(409, { error: { code: 'CONFLICT', message: 'x', details: { reason: 'NAME_TAKEN' } } });
    const e = await createKey('p1', 'n').catch((x) => x);
    expect(e.status).toBe(409);
    expect(reasonOf(e.details)).toBe('NAME_TAKEN');
  });
});
