import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockUsageAll, PROJECT_ID } from '../mocks/usage';
import { getAgentUsage } from './usage.service';

function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('getAgentUsage', () => {
  it('sends range, project_id, section', async () => {
    const f = stub(200, mockUsageAll);
    await getAgentUsage({ range: '30d', projectId: PROJECT_ID }, 'keys');
    expect(f.mock.calls[0]![0]).toBe(`/api/audit-log/usage?range=30d&project_id=${PROJECT_ID}&section=keys`);
  });
  it('omits project_id when null', async () => {
    const f = stub(200, mockUsageAll);
    await getAgentUsage({ range: '7d', projectId: null }, 'zero_queries');
    expect(f.mock.calls[0]![0]).toBe('/api/audit-log/usage?range=7d&section=zero_queries');
  });
  it('throws ApiError on schema mismatch', async () => {
    stub(200, { ...mockUsageAll, extra: 1 });
    await expect(getAgentUsage({ range: '7d', projectId: null }, 'keys')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
