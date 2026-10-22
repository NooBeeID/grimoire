import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockSpecList } from '../mocks/specs';
import { createSpecListStore } from '../stores/specList.store';
import View from './ListSpecsView.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = () => ({ projectId: 'p1', store: createSpecListStore(), onupload: vi.fn(), onprojectgone: vi.fn() });

describe('ListSpecsView', () => {
  it('loads and renders rows with quota', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList())));
    render(View, props());
    expect(await screen.findByRole('link', { name: 'auth' })).toBeInTheDocument();
    expect(screen.getByText('2 / 100 spec')).toBeInTheDocument();
  });
  it('empty state offers upload', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList([]))));
    const p = props();
    render(View, p);
    await screen.findByText('Belum ada API spec');
    await userEvent.click(screen.getAllByRole('button', { name: 'Unggah spec' })[1]!);
    expect(p.onupload).toHaveBeenCalled();
  });
  it('500 shows an error and retry reloads', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(500, { error: { code: 'INTERNAL', message: 'gagal server', details: null } }))
      .mockResolvedValue(json(200, mockSpecList()));
    vi.stubGlobal('fetch', f);
    render(View, props());
    await screen.findByText('gagal server');
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByRole('link', { name: 'auth' })).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('404 forwards onprojectgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('invalidate reloads', async () => {
    const f = vi.fn(async () => json(200, mockSpecList()));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(View, p);
    await screen.findByRole('link', { name: 'auth' });
    p.store.invalidate();
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(2));
  });

  const extra = createRawSnippet(() => ({ render: () => '<button type="button">Aksi tambahan</button>' }));
  it('headeractions renders in the header and the empty state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList())));
    const { unmount } = render(View, { ...props(), headeractions: extra });
    await screen.findByRole('link', { name: 'auth' });
    expect(screen.getByRole('button', { name: 'Aksi tambahan' })).toBeInTheDocument();
    unmount();
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList([]))));
    render(View, { ...props(), headeractions: extra });
    await screen.findByText('Belum ada API spec');
    expect(screen.getAllByRole('button', { name: 'Aksi tambahan' })).toHaveLength(2);
  });
  it('without headeractions nothing extra is rendered', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList())));
    render(View, props());
    await screen.findByRole('link', { name: 'auth' });
    expect(screen.queryByRole('button', { name: 'Aksi tambahan' })).toBeNull();
  });
});
