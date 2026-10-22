import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteCredential } from './deletecredential.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body?: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(body === undefined ? null : JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('deleteCredential', () => {
  it('GitHub has no host query', async () => {
    const f = stub(204);
    await expect(deleteCredential({ provider: 'github', host: 'github.com' })).resolves.toBeUndefined();
    expect(f.mock.calls[0]![0]).toBe('/api/me/git-credentials/github');
    expect(f.mock.calls[0]![1]!.method).toBe('DELETE');
  });
  it('GitLab sends ?host=', async () => {
    const f = stub(204);
    await deleteCredential({ provider: 'gitlab', host: 'git.noobee.id' });
    expect(f.mock.calls[0]![0]).toBe('/api/me/git-credentials/gitlab?host=git.noobee.id');
  });
  it('404 -> ApiError status 404', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'token tidak ditemukan', details: null } });
    await expect(deleteCredential({ provider: 'github', host: 'github.com' })).rejects.toMatchObject({ status: 404 });
  });
});
