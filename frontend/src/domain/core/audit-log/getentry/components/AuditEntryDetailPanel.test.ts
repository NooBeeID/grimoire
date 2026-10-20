import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockDetail } from '../mocks/entries';
import type { DetailState } from '../types';
import Panel from './AuditEntryDetailPanel.svelte';

beforeEach(() => setLocale('id'));
const ready = (over: Record<string, unknown> = {}): DetailState => ({
  status: 'ready',
  id: mockDetail.id,
  entry: { ...mockDetail, ...over },
});
const err = (status: number): DetailState => ({
  status: 'error',
  id: 'x',
  error: status === 404 ? 'notFound' : status === 403 ? 'forbidden' : 'load',
  apiError: new ApiError({ status, code: 'X', message: 'boom', traceId: 'tr-1' }),
});

describe('AuditEntryDetailPanel', () => {
  it('T02-AC4 shows redacted literally', () => {
    const { container } = render(Panel, { detail: ready(), onclose: vi.fn(), onretry: vi.fn() });
    expect(container.textContent).toContain('[REDACTED]');
    expect(container.textContent).toContain('"bytes": 2048');
  });
  it('T02-AC3 keeps snapshot label with no live project', () => {
    render(Panel, { detail: ready({ project_label: 'Deleted Proj' }), onclose: vi.fn(), onretry: vi.fn() });
    expect(screen.getByText(/Deleted Proj/)).toBeInTheDocument();
  });
  it('renders XSS payload as text and null as dash', () => {
    const x = '<img src=x onerror=alert(1)>';
    const { container } = render(Panel, {
      detail: ready({ target: x, args: { path: x }, ip: null, user_agent: null, project_label: null, project_id: null }),
      onclose: vi.fn(),
      onretry: vi.fn(),
    });
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain(x);
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(3);
  });
  it('states: loading, notFound, forbidden, load', async () => {
    const onretry = vi.fn();
    const l = render(Panel, { detail: { status: 'loading', id: 'x' }, onclose: vi.fn(), onretry });
    expect(l.container.querySelector('[aria-busy="true"]')).not.toBeNull();
    l.unmount();
    const n = render(Panel, { detail: err(404), onclose: vi.fn(), onretry });
    expect(screen.getByText('Entri tidak ditemukan')).toBeInTheDocument();
    n.unmount();
    const f = render(Panel, { detail: err(403), onclose: vi.fn(), onretry });
    expect(screen.queryByRole('button', { name: 'Coba lagi' })).toBeNull();
    f.unmount();
    render(Panel, { detail: err(500), onclose: vi.fn(), onretry });
    expect(screen.getByText('ID jejak: tr-1')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
  it('close button and Escape call onclose; heading focused', async () => {
    const onclose = vi.fn();
    render(Panel, { detail: ready(), onclose, onretry: vi.fn() });
    expect(screen.getByRole('heading', { name: 'Detail entri' })).toHaveFocus();
    await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
    await userEvent.keyboard('{Escape}');
    expect(onclose).toHaveBeenCalledTimes(2);
  });
});
