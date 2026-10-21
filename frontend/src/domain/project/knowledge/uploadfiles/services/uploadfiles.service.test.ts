import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeXhr, xhrFrom } from '../../../../../shared/services/fakeXhr';
import { setLocale } from '../../../../../shared/stores/i18n';
import { knowledgeTarget, uploadKnowledgeFiles } from './uploadfiles.service';

beforeEach(() => {
  setLocale('id');
  vi.stubGlobal('XMLHttpRequest', FakeXhr);
});
afterEach(() => vi.unstubAllGlobals());

const result = {
  summary: { created: 1, updated: 0, unchanged: 0, skipped: 0, rejected: 1 },
  items: [
    { path: 'docs/auth/login.md', status: 'created' },
    { path: '../x.md', status: 'rejected', reason: 'path_traversal' },
  ],
};
const lf = (path: string) => ({ path, file: new File(['x'], path.split('/').pop()!) });

describe('uploadKnowledgeFiles', () => {
  it('sends the relative path as the part filename, not the basename', async () => {
    const { Xhr, calls } = xhrFrom(() => new Response(JSON.stringify(result), { status: 200 }));
    vi.stubGlobal('XMLHttpRequest', Xhr);
    await uploadKnowledgeFiles('p1', [lf('docs/auth/login.md')]);
    expect(calls[0]!.url).toBe('/api/projects/p1/knowledge/upload');
    const form = calls[0]!.body as FormData;
    expect((form.get('files') as File).name).toBe('docs/auth/login.md');
  });
  it('forwards upload progress', async () => {
    const seen: unknown[] = [];
    const p = uploadKnowledgeFiles('p1', [lf('a.md')], { onProgress: (e) => seen.push(e) });
    const x = FakeXhr.last!;
    x.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 4 });
    x.respond(200, JSON.stringify(result));
    await p;
    expect(seen).toEqual([{ loaded: 1, total: 4, phase: 'sending' }]);
  });
  it('maps server items to report items with readable reasons', async () => {
    vi.stubGlobal('XMLHttpRequest', xhrFrom(() => new Response(JSON.stringify(result), { status: 200 })).Xhr);
    const items = await knowledgeTarget.upload('p1', [lf('a.md')], { signal: new AbortController().signal, onProgress: () => {} });
    expect(items[0]).toMatchObject({ status: 'created', target: 'knowledge' });
    expect(items[1]!.reasonText).not.toBe('path_traversal');
  });
});
