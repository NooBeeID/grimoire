import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '../App.svelte';
import DashboardView from './DashboardView.svelte';
import { notFoundRoute, routes } from '../routes/index';
import { startRouter } from '../routes/router';
import { setLocale } from '../../shared/stores/i18n';
import { resetSession, setIdentity, type Role } from '../../shared/stores/session';

vi.mock('chart.js', async () => (await import('../../test-stubs/chartjs')).chartJsMock());

const mockProjectOptions = { items: [], total: 0 };
const mockSummary7d = {
  range: '7d',
  from: '2026-10-15T10:30:15Z',
  to: '2026-10-22T10:30:15Z',
  bucket: 'day',
  project_id: null,
  total_calls: 1234,
  total_errors: 31,
  error_rate: 31 / 1234,
  top_projects: [],
  tool_usage: [],
  tool_metrics: [],
  traffic: [{ at: '2026-10-15T00:00:00Z', calls: 1234, errors: 31 }],
  search_quality: null,
};

let stop: () => void;
let fetchMock: ReturnType<typeof vi.fn>;
function boot(path: string, role: Role) {
  fetchMock = vi.fn(async (url: string) =>
    url.startsWith('/api/audit-log/summary')
      ? new Response(JSON.stringify(mockSummary7d), { status: 200 })
      : new Response(JSON.stringify(mockProjectOptions), { status: 200 }),
  );
  vi.stubGlobal('fetch', fetchMock);
  history.replaceState({}, '', path);
  stop = startRouter(routes, notFoundRoute);
  const r = render(App);
  setIdentity({ userId: 'u', email: 'e@x', name: 'N', role, mustChangePassword: false });
  return r;
}
beforeEach(() => {
  setLocale('id');
  resetSession();
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe('DashboardView / route /dashboard', () => {
  it('super_admin sees the title, the lazy summary and a nav item', async () => {
    boot('/dashboard', 'super_admin');
    expect(await screen.findByTestId('dashboard-title')).toHaveTextContent('Dashboard');
    expect(await screen.findByText('1.234')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('aria-current', 'page');
  });
  it('developer gets not found, no nav item and no summary request', async () => {
    boot('/dashboard', 'developer');
    expect(await screen.findByTestId('notfound-title')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Dashboard' })).toBeNull();
    expect(fetchMock.mock.calls.some((c) => String(c[0]).startsWith('/api/audit-log/summary'))).toBe(false);
  });

  it('renders a single h1, then the Trafik card title', async () => {
    boot('/dashboard', 'super_admin');
    expect(await screen.findByTestId('dashboard-title')).toHaveTextContent('Dashboard');
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
    expect(await screen.findByText('Trafik')).toBeInTheDocument();
  });
  it('shows a spinner under the title while the chunk is pending', () => {
    history.replaceState({}, '', '/dashboard');
    render(DashboardView, { load: () => new Promise<never>(() => {}) });
    expect(screen.getByTestId('dashboard-title')).toHaveTextContent('Dashboard');
    expect(screen.getByRole('status')).toBeInTheDocument();
  });
  it('changing the range replaces the URL and keeps history length', async () => {
    boot('/dashboard', 'super_admin');
    await screen.findByText('1.234');
    const before = history.length;
    await userEvent.click(screen.getByLabelText('24 jam'));
    await vi.waitFor(() => expect(window.location.pathname + window.location.search).toBe('/dashboard?range=24h'));
    expect(history.length).toBe(before);
  });
});

describe('DashboardView chunk failure', () => {
  it('keeps the h1, shows ErrorState, and "Coba lagi" imports again', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        new Response(JSON.stringify(url.startsWith('/api/audit-log/summary') ? mockSummary7d : mockProjectOptions), { status: 200 }),
      ),
    );
    history.replaceState({}, '', '/dashboard');
    let imports = 0;
    const load = vi.fn(async () => {
      imports++;
      if (imports === 1) throw new Error('chunk gagal');
      return await import('../../domain/core/audit-log/summary');
    });
    render(DashboardView, { load });
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-title')).toHaveTextContent('Dashboard');
    expect(load).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByText('1.234')).toBeInTheDocument();
    expect(load).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
