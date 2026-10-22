import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockProjectOptions } from '../mocks/summary';
import { listProjectOptions } from './projectOptions.service';

function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('listProjectOptions', () => {
  it('requests limit=100', async () => {
    const f = stub(200, mockProjectOptions);
    await listProjectOptions();
    expect(f.mock.calls[0]![0]).toBe('/api/projects?limit=100');
  });
  it('sorts by name and returns total', async () => {
    stub(200, { items: [...mockProjectOptions.items].reverse(), total: 5 });
    const r = await listProjectOptions();
    expect(r.options.map((o) => o.name)).toEqual(['Grimoire Docs', 'Ledger Core', 'Onboarding Mobile', 'Partner Portal', 'Payments']);
    expect(r.total).toBe(5);
  });
  it('extra item fields (my_role, counts) do not fail parsing', async () => {
    stub(200, { items: mockProjectOptions.items.map((i) => ({ ...i, my_role: 'owner', counts: { files: 1, specs: 0, keys: 0 } })), total: 5 });
    const r = await listProjectOptions();
    expect(r.options).toHaveLength(5);
  });
  it('item without a name is rejected', async () => {
    stub(200, { items: [{ id: mockProjectOptions.items[0]!.id }], total: 1 });
    await expect(listProjectOptions()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('forwards the abort signal', async () => {
    const f = stub(200, mockProjectOptions);
    await listProjectOptions(new AbortController().signal);
    expect(f.mock.calls[0]![1]?.signal).toBeDefined();
  });
});
