import { afterEach, describe, expect, it, vi } from 'vitest';
import { isApiError } from '../../../../../shared/services/http';
import { mockProject, mockProjectList } from '../mocks/projects';
import { listProjects, parseListQuery, toSearchParams } from './listprojects.service';

function stub(status: number, body: unknown) {
  const f = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
afterEach(() => vi.unstubAllGlobals());

describe('listProjects', () => {
  it('builds query with q, limit and offset', async () => {
    const f = stub(200, mockProjectList(1, 1));
    await listProjects({ q: 'pay', page: 2 });
    expect(f.mock.calls[0]![0]).toBe('/api/projects?q=pay&limit=20&offset=20');
  });
  it('omits empty q', async () => {
    const f = stub(200, mockProjectList(1));
    await listProjects({ q: '', page: 1 });
    expect(f.mock.calls[0]![0]).toBe('/api/projects?limit=20&offset=0');
  });
  it('accepts valid fixture including null counts', async () => {
    stub(200, mockProjectList(2));
    const r = await listProjects({ q: '', page: 1 });
    expect(r.items).toHaveLength(2);
    expect(r.items[0]!.counts.files).toBeNull();
  });
  it('rejects unknown my_role as INVALID_RESPONSE', async () => {
    stub(200, { ...mockProjectList(0), items: [mockProject(1, { my_role: 'admin' })], total: 1 });
    await expect(listProjects({ q: '', page: 1 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('rejects missing total', async () => {
    stub(200, { items: [], limit: 20, offset: 0 });
    await expect(listProjects({ q: '', page: 1 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('keeps 400 details intact', async () => {
    stub(400, { error: { code: 'INVALID_ARGUMENT', message: 'x', details: { fields: { limit: 'salah' } } }, trace_id: 't' });
    const e = await listProjects({ q: '', page: 1 }).catch((x) => x);
    expect(isApiError(e)).toBe(true);
    expect(e.details).toEqual({ fields: { limit: 'salah' } });
  });
});

describe('parseListQuery / toSearchParams', () => {
  it('normalizes page', () => {
    for (const p of ['abc', '0', '-1', '']) expect(parseListQuery(new URLSearchParams('page=' + p)).page).toBe(1);
    expect(parseListQuery(new URLSearchParams('page=3')).page).toBe(3);
  });
  it('truncates q to 100 runes and trims', () => {
    expect([...parseListQuery(new URLSearchParams({ q: 'a'.repeat(150) })).q]).toHaveLength(100);
    expect(parseListQuery(new URLSearchParams({ q: '  pay ' })).q).toBe('pay');
  });
  it('does not write page=1 or empty q', () => {
    expect(toSearchParams({ q: '', page: 1 }).toString()).toBe('');
    expect(toSearchParams({ q: 'a', page: 2 }).toString()).toBe('q=a&page=2');
  });
});
