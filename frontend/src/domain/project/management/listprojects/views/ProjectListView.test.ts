import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockProjectList } from '../mocks/projects';

vi.mock('../services/listprojects.service', async (orig) => ({
  ...(await orig<typeof import('../services/listprojects.service')>()),
  listProjects: vi.fn(),
}));
import { listProjects } from '../services/listprojects.service';
import View from './ProjectListView.svelte';

const mocked = vi.mocked(listProjects);
const props = (q = '', over: Record<string, unknown> = {}) => ({
  query: new URLSearchParams(q),
  onquerychange: vi.fn(),
  canCreate: true,
  oncreate: vi.fn(),
  ...over,
});
beforeEach(() => {
  setLocale('id');
  mocked.mockReset();
});

describe('ProjectListView', () => {
  it('first load shows spinner with aria-busy, then rows', async () => {
    mocked.mockResolvedValue(mockProjectList(2));
    const { container } = render(View, props());
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByRole('link', { name: 'Project 1' })).toBeInTheDocument();
  });
  it('empty without q vs with q', async () => {
    mocked.mockResolvedValue(mockProjectList(0));
    const a = render(View, props());
    expect(await screen.findByText('Belum ada project')).toBeInTheDocument();
    a.unmount();
    const p = props('q=zzz');
    render(View, p);
    expect(await screen.findByText('Tidak ada project yang cocok dengan "zzz"')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reset pencarian' }));
    expect(p.onquerychange).toHaveBeenCalledTimes(1);
    expect(p.onquerychange.mock.calls[0]![0].has('q')).toBe(false);
  });
  it('error shows trace id and retry refetches', async () => {
    mocked.mockRejectedValueOnce(new ApiError({ status: 500, code: 'INTERNAL', message: 'boom', traceId: 'T-1' }));
    mocked.mockResolvedValueOnce(mockProjectList(1));
    render(View, props());
    expect(await screen.findByText('boom')).toBeInTheDocument();
    expect(screen.getByText(/T-1/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByRole('link', { name: 'Project 1' })).toBeInTheDocument();
    expect(mocked).toHaveBeenCalledTimes(2);
  });
  it('out-of-range page jumps to last page once', async () => {
    mocked.mockResolvedValue({ items: [], total: 45, limit: 20, offset: 80 });
    const p = props('page=5');
    render(View, p);
    await vi.waitFor(() => expect(p.onquerychange).toHaveBeenCalledTimes(1));
    expect(p.onquerychange.mock.calls[0]![0].get('page')).toBe('3');
  });
  it('query change aborts the previous request and shows the latest', async () => {
    mocked.mockResolvedValue(mockProjectList(1));
    const { rerender } = render(View, props('q=a'));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(1));
    const sig = mocked.mock.calls[0]![1];
    await rerender(props('q=ab'));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(2));
    expect(sig?.aborted).toBe(true);
  });
  it('canCreate shows the new-project button', async () => {
    mocked.mockResolvedValue(mockProjectList(1));
    const p = props();
    render(View, p);
    await userEvent.click(await screen.findByRole('button', { name: 'Project baru' }));
    expect(p.oncreate).toHaveBeenCalled();
  });
  it('paginates when total exceeds page size', async () => {
    mocked.mockResolvedValue(mockProjectList(20, 45));
    const p = props();
    render(View, p);
    await userEvent.click(await screen.findByRole('button', { name: 'Berikutnya' }));
    expect(p.onquerychange.mock.calls[0]![0].get('page')).toBe('2');
  });
});
