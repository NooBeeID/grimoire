import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockFile, V2 } from '../../base';
import { parseConflict, updateFile } from './updatefile.service';

afterEach(() => vi.unstubAllGlobals());

describe('updateFile', () => {
  it('PUTs {content, version} with an encoded path query', async () => {
    const f = vi.fn(async () => new Response(JSON.stringify(mockFile('docs/a.md')), { status: 200 }));
    vi.stubGlobal('fetch', f);
    await updateFile('p1', 'docs/a.md', { content: 'x', version: V2 });
    const [url, init] = f.mock.calls[0]! as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/knowledge/files?path=docs%2Fa.md');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body as string)).toEqual({ content: 'x', version: V2 });
  });
  it('parseConflict only accepts 409 with valid details', () => {
    const current = { content: 'c', version: V2, updated_at: '2026-10-21T09:30:00Z', updated_by_label: 'budi@tim.id' };
    const mk = (status: number, details: unknown) => new ApiError({ status, code: 'CONFLICT', message: 'm', details });
    expect(parseConflict(mk(409, { reason: 'version_conflict', current }))?.version).toBe(V2);
    expect(parseConflict(mk(409, { reason: 'path_exists' }))).toBeNull();
    expect(parseConflict(mk(400, { reason: 'version_conflict', current }))).toBeNull();
    expect(parseConflict(new Error('x'))).toBeNull();
  });
});
