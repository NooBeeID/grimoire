import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockSummary } from '../mocks/entries';

vi.mock('../services/listentries.service', async (orig) => ({
  ...(await orig<typeof import('../services/listentries.service')>()),
  listEntries: vi.fn(),
}));
import { listEntries } from '../services/listentries.service';
import View from './ListEntriesView.svelte';

const mocked = vi.mocked(listEntries);
const page = (start: number, n: number, next: string | null) => ({
  items: Array.from({ length: n }, (_, i) => mockSummary(start + i)),
  nextCursor: next,
});
const props = (q = '', over: Record<string, unknown> = {}) => ({
  query: new URLSearchParams(q),
  selectedId: null,
  onquerychange: vi.fn(),
  onselect: vi.fn(),
  ...over,
});
beforeEach(() => {
  setLocale('id');
  mocked.mockReset();
});

describe('ListEntriesView', () => {
  it('initial load calls service once with filter from query', async () => {
    mocked.mockResolvedValue(page(1, 2, null));
    render(View, props('action=auth.logout&status=ok'));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(1));
    const f = mocked.mock.calls[0]![0];
    expect(f.actions).toEqual(['auth.logout']);
    expect(f.statuses).toEqual(['ok']);
    expect(await screen.findAllByRole('button', { name: /20\d\d|\d{4}/ })).toHaveLength(2);
  });
  it('filter change refetches and aborts previous; entry-only change does not', async () => {
    mocked.mockResolvedValue(page(1, 1, null));
    const { rerender } = render(View, props('action=auth.logout'));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(1));
    const sig = mocked.mock.calls[0]![3];
    await rerender(props('action=auth.logout&entry=abc'));
    expect(mocked).toHaveBeenCalledTimes(1);
    await rerender(props('action=user.created'));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(2));
    expect(sig?.aborted).toBe(true);
  });
  it('load more uses nextCursor and same range; failure keeps page 1 and retry reuses cursor', async () => {
    mocked.mockResolvedValueOnce(page(1, 2, 'C1'));
    render(View, props('range=7d'));
    await userEvent.click(await screen.findByRole('button', { name: 'Muat berikutnya' }));
    expect(mocked.mock.calls[1]![2]).toBe('C1');
    expect(mocked.mock.calls[1]![1]).toEqual(mocked.mock.calls[0]![1]);
  });
  it('failed page 2 keeps page 1', async () => {
    mocked.mockResolvedValueOnce(page(1, 2, 'C1'));
    mocked.mockRejectedValueOnce(new ApiError({ status: 500, code: 'INTERNAL', message: 'gagal2' }));
    mocked.mockResolvedValueOnce(page(3, 1, null));
    const { container } = render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Muat berikutnya' }));
    expect(await screen.findByText('gagal2')).toBeInTheDocument();
    expect(container.querySelectorAll('button[data-entry-id]')).toHaveLength(2);
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    await vi.waitFor(() => expect(container.querySelectorAll('button[data-entry-id]')).toHaveLength(3));
    expect(mocked.mock.calls[2]![2]).toBe('C1');
  });
  it('apply calls onquerychange without entry; refresh reloads', async () => {
    mocked.mockResolvedValue(page(1, 1, null));
    const p = props('entry=abc&foo=1');
    render(View, p);
    await screen.findByRole('button', { name: 'Terapkan' });
    await userEvent.click(screen.getByRole('button', { name: 'Terapkan' }));
    expect(p.onquerychange.mock.calls[0]![0].toString()).toBe('foo=1');
    await userEvent.click(screen.getByRole('button', { name: 'Muat ulang' }));
    await vi.waitFor(() => expect(mocked).toHaveBeenCalledTimes(2));
  });
});
