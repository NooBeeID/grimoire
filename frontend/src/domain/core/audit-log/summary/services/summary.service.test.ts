import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockProjectOptions, mockSummary7d, PROJECT_IDS } from '../mocks/summary';
import { listProjectOptions } from './projectOptions.service';
import { getUsageSummary } from './summary.service';

function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('getUsageSummary', () => {
  it('omits project_id without project', async () => {
    const f = stub(200, mockSummary7d);
    const r = await getUsageSummary({ range: '7d', projectId: null });
    expect(f.mock.calls[0]![0]).toBe('/api/audit-log/summary?range=7d');
    expect(r.total_calls).toBe(1234);
  });
  it('sends project_id and signal', async () => {
    const f = stub(200, mockSummary7d);
    const c = new AbortController();
    await getUsageSummary({ range: '24h', projectId: PROJECT_IDS.payments }, c.signal);
    expect(f.mock.calls[0]![0]).toBe(`/api/audit-log/summary?range=24h&project_id=${PROJECT_IDS.payments}`);
    expect(f.mock.calls[0]![1]?.signal).toBeDefined();
  });
  it('400 keeps details.field', async () => {
    stub(400, { error: { code: 'INVALID_ARGUMENT', message: 'range tidak valid', details: { field: 'range' } }, trace_id: 't' });
    await expect(getUsageSummary({ range: '7d', projectId: null })).rejects.toMatchObject({
      status: 400,
      details: { field: 'range' },
    });
  });
  it('schema mismatch -> INVALID_RESPONSE', async () => {
    stub(200, { ...mockSummary7d, args: {} });
    await expect(getUsageSummary({ range: '7d', projectId: null })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});

describe('listProjectOptions', () => {
  it('uses limit=100 and sorts by name', async () => {
    const f = stub(200, { items: [...mockProjectOptions.items].reverse().map((i) => ({ ...i, my_role: 'owner' })), total: 5 });
    const r = await listProjectOptions();
    expect(f.mock.calls[0]![0]).toBe('/api/projects?limit=100');
    expect(r.options.map((o) => o.name)).toEqual([...mockProjectOptions.items.map((i) => i.name)].sort((a, b) => a.localeCompare(b)));
    expect(r.total).toBe(5);
  });
});
