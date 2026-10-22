import { afterEach, describe, expect, it, vi } from 'vitest';
import { saveCredential } from './savecredential.service';

afterEach(() => vi.unstubAllGlobals());
const saved = { provider: 'github', host: 'github.com', last4: 'a9F3', updated_at: '2026-10-22T07:10:00Z', replaced: false };
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('saveCredential', () => {
  it('GitHub: PUT body is exactly {token}, with CSRF header', async () => {
    const f = stub(200, saved);
    await saveCredential('github', { token: '  ghp_abcdefgh  ' });
    expect(f.mock.calls[0]![0]).toBe('/api/me/git-credentials/github');
    const init = f.mock.calls[0]![1]!;
    expect(init.method).toBe('PUT');
    expect(init.body).toBe('{"token":"ghp_abcdefgh"}');
    expect((init.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
  });
  it('GitLab: body has host and token', async () => {
    const f = stub(200, { ...saved, provider: 'gitlab', host: 'git.noobee.id' });
    await saveCredential('gitlab', { host: 'git.noobee.id', token: 'glpat-abcdefgh' });
    expect(f.mock.calls[0]![1]!.body).toBe('{"host":"git.noobee.id","token":"glpat-abcdefgh"}');
  });
  it('422 TOKEN_INVALID -> ApiError code', async () => {
    stub(422, { error: { code: 'TOKEN_INVALID', message: 'x', details: null } });
    await expect(saveCredential('github', { token: 'ghp_abcdefgh' })).rejects.toMatchObject({ status: 422, code: 'TOKEN_INVALID' });
  });
});
