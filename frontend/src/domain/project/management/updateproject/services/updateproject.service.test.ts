import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockUpdated } from '../mocks/project';
import { diffPatch, updateProject, validatePatch } from './updateproject.service';

afterEach(() => vi.unstubAllGlobals());
const cur = { id: 'x', name: 'Payments', description: 'desc' };

describe('diffPatch', () => {
  it('unchanged -> {}', () => {
    expect(diffPatch(cur, { name: 'Payments', description: 'desc' })).toEqual({});
    expect(diffPatch(cur, { name: ' Payments ', description: ' desc ' })).toEqual({});
  });
  it('only changed fields', () => {
    expect(diffPatch(cur, { name: ' Payments v2 ', description: 'desc' })).toEqual({ name: 'Payments v2' });
    expect(diffPatch(cur, { name: 'Payments', description: '' })).toEqual({ description: '' });
  });
});

describe('validatePatch', () => {
  it('same rules as create', () => {
    expect(validatePatch({ name: 'A' }).name).toBe('project.validation.nameLength');
    expect(validatePatch({ name: 'AB' })).toEqual({});
    expect(validatePatch({ description: 'x'.repeat(1001) }).description).toBe('project.validation.descriptionLength');
    expect(validatePatch({ description: 'a\nb' })).toEqual({});
    expect(validatePatch({ name: 'a\u0001b' }).name).toBe('project.validation.nameControl');
  });
});

describe('updateProject', () => {
  it('PATCHes only the given fields with CSRF header', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockUpdated({ name: 'V2' })), { status: 200 }));
    vi.stubGlobal('fetch', f);
    const r = await updateProject('abc', { name: 'V2' });
    expect(r.name).toBe('V2');
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('/api/projects/abc');
    expect(init!.method).toBe('PATCH');
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'V2' });
    expect((init!.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
  });
});
