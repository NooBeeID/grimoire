import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig } from '../../base';
import { getConfig } from './getconfig.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const resp = (config: unknown) => ({ kind: 'knowledge', feature_enabled: true, has_credential: true, credential_unreadable: false, config });

describe('getConfig', () => {
  it('GET with kind query and an encoded project id', async () => {
    const f = stub(200, resp(mockConfig()));
    const r = await getConfig('a/b', 'knowledge');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/a%2Fb/git-sync?kind=knowledge');
    expect(f.mock.calls[0]![1]!.headers).not.toHaveProperty('X-Requested-With');
    expect(r.config?.branch).toBe('main');
  });
  it('config null passes', async () => {
    stub(200, resp(null));
    await expect(getConfig('p', 'knowledge')).resolves.toMatchObject({ config: null });
  });
  it('invalid body -> INVALID_RESPONSE', async () => {
    stub(200, { kind: 'knowledge' });
    await expect(getConfig('p', 'knowledge')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('404 -> ApiError 404', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'project tidak ditemukan', details: null } });
    await expect(getConfig('p', 'api_spec')).rejects.toMatchObject({ status: 404 });
  });
});
