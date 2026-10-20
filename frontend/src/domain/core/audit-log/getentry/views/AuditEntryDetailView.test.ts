import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockDetail } from '../mocks/entries';

vi.mock('../services/getentry.service', async (orig) => ({
  ...(await orig<typeof import('../services/getentry.service')>()),
  getAuditEntry: vi.fn(),
}));
import { getAuditEntry } from '../services/getentry.service';
import View from './AuditEntryDetailView.svelte';

const mocked = vi.mocked(getAuditEntry);
beforeEach(() => {
  setLocale('id');
  mocked.mockReset();
});

describe('AuditEntryDetailView', () => {
  it('id null makes no request', () => {
    render(View, { id: null, onclose: vi.fn() });
    expect(mocked).not.toHaveBeenCalled();
  });
  it('loads entry', async () => {
    mocked.mockResolvedValue(mockDetail);
    render(View, { id: 'A', onclose: vi.fn() });
    expect(await screen.findByText('mcp.read_file')).toBeInTheDocument();
  });
  it('A -> B aborts A and slow A does not overwrite B', async () => {
    let resolveA!: (v: typeof mockDetail) => void;
    let signalA: AbortSignal | undefined;
    mocked.mockImplementationOnce((_id, s) => {
      signalA = s;
      return new Promise((r) => (resolveA = r));
    });
    mocked.mockResolvedValueOnce({ ...mockDetail, action: 'mcp.search_knowledge' });
    const { rerender } = render(View, { id: 'A', onclose: vi.fn() });
    await rerender({ id: 'B', onclose: vi.fn() });
    expect(signalA?.aborted).toBe(true);
    expect(await screen.findByText('mcp.search_knowledge')).toBeInTheDocument();
    resolveA(mockDetail);
    await Promise.resolve();
    expect(screen.queryByText('mcp.read_file')).toBeNull();
  });
  it('404 shows notFound; 500 retry calls again with same id', async () => {
    mocked.mockRejectedValueOnce(new ApiError({ status: 404, code: 'NOT_FOUND', message: 'x' }));
    const a = render(View, { id: 'A', onclose: vi.fn() });
    expect(await screen.findByText('Entri tidak ditemukan')).toBeInTheDocument();
    a.unmount();
    mocked.mockReset();
    mocked.mockRejectedValueOnce(new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    mocked.mockResolvedValueOnce(mockDetail);
    render(View, { id: 'Z', onclose: vi.fn() });
    await userEvent.click(await screen.findByRole('button', { name: 'Coba lagi' }));
    expect(mocked).toHaveBeenCalledTimes(2);
    expect(mocked.mock.calls[1]![0]).toBe('Z');
  });
});
