import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mockFile } from '../../../../domain/project/knowledge/base';
import { setLocale } from '../../../../shared/stores/i18n';
import { navigate, startRouter } from '../../../routes/router';

const { fakeTarget } = vi.hoisted(() => ({
  fakeTarget: {
    key: 'spec',
    labelKey: 'knowledge.search.label',
    acceptExts: ['.yaml'],
    accepts: (p: string) => p.endsWith('.yaml'),
    upload: async () => [],
  },
}));
vi.mock('../../../workspace/uploadTargets', () => ({ extraUploadTargets: [fakeTarget] }));

import Tab from './KnowledgeTab.svelte';

const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';
const T = '2026-10-21T09:30:00Z';
const root = {
  prefix: '',
  folders: [{ name: 'docs', path: 'docs', file_count: 2 }],
  files: [{ name: 'README.md', path: 'README.md', size_bytes: 10, updated_at: T }],
  total_files: 3,
};
const empty = { prefix: '', folders: [], files: [], total_files: 0 };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });

type Handler = (url: URL, init?: RequestInit) => Response | undefined;
let reload: ReturnType<typeof vi.fn>;
let stop: () => void;
let calls: string[];

function stubFetch(extra: Handler = () => undefined, tree: unknown = root) {
  calls = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init?: RequestInit) => {
      const url = new URL(input, 'http://localhost');
      calls.push(`${init?.method ?? 'GET'} ${url.pathname}${url.search}`);
      const custom = extra(url, init);
      if (custom) return custom;
      if (url.pathname.endsWith('/knowledge/tree')) {
        const prefix = url.searchParams.get('prefix') ?? '';
        return json(200, prefix ? { prefix, folders: [], files: [], total_files: 0 } : tree);
      }
      if (url.pathname.endsWith('/knowledge/files')) {
        return json(200, mockFile(url.searchParams.get('path') ?? 'x.md', { title: 'Alur login' }));
      }
      if (url.pathname.endsWith('/knowledge/search')) {
        return json(200, {
          items: [{ path: 'docs/login.md', title: 'Hasil login', score: 1, snippet: [{ text: 'login', highlight: true }] }],
          total: 1,
        });
      }
      return json(404, { error: { code: 'NOT_FOUND', message: 'tidak ada' } });
    }),
  );
}

function mount() {
  reload = vi.fn(async () => {});
  const ctx = { project: { id: PID }, reload, replace: vi.fn() } as never;
  return render(Tab, { ctx, currentUserId: 'u1', onprojectgone: vi.fn(), onleft: vi.fn() });
}
const at = (qs: string) => navigate(`/projects/${PID}/knowledge${qs}`, { replace: true });

beforeEach(() => {
  setLocale('id');
  sessionStorage.clear();
  history.replaceState({}, '', '/');
  stop = startRouter([]);
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('KnowledgeTab composition', () => {
  it('?path= opens the viewer', async () => {
    stubFetch();
    at('?path=docs%2Fa.md');
    mount();
    expect(await screen.findByRole('heading', { level: 2, name: 'Alur login' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    expect(calls.some((c) => c.startsWith('GET /api/projects/' + PID + '/knowledge/files?path=docs%2Fa.md'))).toBe(true);
  });

  it('?mode=edit opens the editor', async () => {
    stubFetch();
    at('?path=docs%2Fa.md&mode=edit');
    mount();
    expect(await screen.findByLabelText('Isi Markdown')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  });

  it('?new=1&dir= opens the new-file form with the directory prefilled', async () => {
    stubFetch();
    at('?new=1&dir=docs');
    mount();
    expect(await screen.findByLabelText('Path file')).toHaveValue('docs/');
  });

  it('?q= replaces the tree with search results and the tree returns when cleared', async () => {
    stubFetch();
    at('');
    mount();
    expect(await screen.findByRole('tree')).toBeInTheDocument();
    at('?q=login');
    expect(await screen.findByText('Hasil login')).toBeInTheDocument();
    expect(screen.queryByRole('tree')).not.toBeInTheDocument();
    at('');
    expect(await screen.findByRole('tree')).toBeInTheDocument();
    expect(screen.queryByText('Hasil login')).not.toBeInTheDocument();
  });

  it('asks for confirmation when leaving a dirty form for another file', async () => {
    stubFetch();
    at('?new=1');
    mount();
    await userEvent.type(await screen.findByLabelText('Path file'), 'a.md');
    const readme = await screen.findByTitle('README.md');
    await userEvent.click(readme);
    expect(await screen.findByText('Buang perubahan yang belum disimpan?')).toBeInTheDocument();
    expect(window.location.search).toBe('?new=1');
    await userEvent.click(screen.getByRole('button', { name: 'Buang' }));
    await vi.waitFor(() => expect(window.location.search).toBe('?path=README.md'));
  });

  it('keeps editing when the discard confirmation is dismissed', async () => {
    stubFetch();
    at('?new=1');
    mount();
    await userEvent.type(await screen.findByLabelText('Path file'), 'a.md');
    await userEvent.click(await screen.findByTitle('README.md'));
    await userEvent.click(await screen.findByRole('button', { name: 'Lanjut mengedit' }));
    expect(window.location.search).toBe('?new=1');
    expect(screen.getByLabelText('Path file')).toHaveValue('a.md');
  });

  it('a 404 from the viewer calls ctx.reload and refreshes the tree', async () => {
    stubFetch((url) =>
      url.pathname.endsWith('/knowledge/files') ? json(404, { error: { code: 'NOT_FOUND', message: 'hilang' } }) : undefined,
    );
    at('?path=docs%2Fgone.md');
    mount();
    await vi.waitFor(() => expect(reload).toHaveBeenCalled());
    await vi.waitFor(() => expect(calls.filter((c) => c.includes('/knowledge/tree')).length).toBeGreaterThanOrEqual(2));
  });

  it('shows the empty state with upload and new-file actions for an empty project', async () => {
    stubFetch(() => undefined, empty);
    at('');
    mount();
    expect(await screen.findByText('Belum ada knowledge di project ini')).toBeInTheDocument();
  });

  it('passes extraUploadTargets to the upload dialog', async () => {
    stubFetch();
    at('');
    mount();
    await userEvent.click(await screen.findByRole('button', { name: 'Upload' }));
    expect(await screen.findByTestId('pick-files')).toHaveAttribute('accept', expect.stringContaining('.yaml'));
    const input = await screen.findByTestId('pick-folder');
    await userEvent.upload(input, new File(['openapi: 3.0.0'], 'api.yaml', { type: 'text/yaml' }));
    expect(await screen.findByText(/1 file ke Cari knowledge/)).toBeInTheDocument();
  });

  const gitCfg = { kind: 'knowledge', feature_enabled: true, has_credential: true, credential_unreadable: false, config: null };

  it('Sync dari Git button in the sidebar opens the knowledge dialog', async () => {
    stubFetch((url) => (url.pathname.endsWith('/git-sync') ? json(200, gitCfg) : undefined));
    at('');
    mount();
    await userEvent.click(await screen.findByRole('button', { name: 'Sync dari Git' }));
    expect(await screen.findByRole('heading', { name: 'Sync knowledge dari Git' })).toBeInTheDocument();
    expect(calls.some((c) => c.includes('/git-sync?kind=knowledge'))).toBe(true);
  });

  it('empty state also offers Sync dari Git', async () => {
    stubFetch(() => undefined, empty);
    at('');
    mount();
    await screen.findByText('Belum ada knowledge di project ini');
    expect(screen.getAllByRole('button', { name: 'Sync dari Git' })).toHaveLength(2);
  });

  it('a finished sync reloads the tree', async () => {
    const report = {
      status: 'success',
      summary: { created: 1, updated: 0, unchanged: 0, skipped: 0, rejected: 0 },
      items: [{ repo_path: 'docs/a.md', target: 'a.md', status: 'created', reason_code: '', reason: '', issues: [] }],
      truncated: false,
      error_code: '',
      error_message: '',
    };
    const config = {
      provider: 'github', host: 'github.com', repo_url: 'https://github.com/o/r', repo_path: 'o/r', branch: 'main', folder: 'docs', target_prefix: '',
      last_status: 'success', last_synced_at: T, last_synced_by: { id: 'u1', label: 'Rina' }, last_result: null,
    };
    stubFetch((url) => {
      if (url.pathname.endsWith('/git-sync/run')) return json(200, { report, config });
      if (url.pathname.endsWith('/git-sync')) return json(200, gitCfg);
      return undefined;
    });
    at('');
    mount();
    await userEvent.click(await screen.findByRole('button', { name: 'Sync dari Git' }));
    await userEvent.type(await screen.findByLabelText('URL repo atau folder'), 'https://github.com/o/r');
    const before = calls.filter((c) => c.includes('/knowledge/tree')).length;
    await userEvent.click(screen.getByRole('button', { name: 'Sync' }));
    await screen.findByText('docs/a.md');
    await vi.waitFor(() => expect(calls.filter((c) => c.includes('/knowledge/tree')).length).toBeGreaterThan(before));
  });
});
