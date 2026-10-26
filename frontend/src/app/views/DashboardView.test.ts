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

const usageItems: Record<string, unknown> = {
  top_files: { available: true, items: [{ path: 'docs/payments/refund-flow.md', reads: 48, keys: 3, last_read_at: '2026-10-26T09:12:00Z' }] },
  unread_files: { available: true, truncated: false, items: [] },
  zero_queries: { available: true, items: [] },
  per_key: {
    available: true,
    items: [
      { key_id: 'k1', label: 'ci-review-agent', calls: 412, errors: 9, searches: 120, reads: 260, last_used_at: '2026-10-26T10:02:00Z', top_tool: 'read_file' },
      { key_id: 'k2', label: 'support-triage', calls: 96, errors: 0, searches: 70, reads: 20, last_used_at: '2026-10-25T16:30:00Z', top_tool: 'search_knowledge' },
    ],
  },
};
const usageResponse = (url: string) => {
  const section = new URL(url, 'http://x').searchParams.get('section') ?? 'keys';
  const key = section === 'keys' ? 'per_key' : section;
  const body = {
    range: '7d', from: '2026-10-19T10:30:15Z', to: '2026-10-26T10:30:15Z', project_id: null,
    top_files: null, unread_files: null, zero_queries: null, per_key: null,
    [key]: usageItems[key],
  };
  return new Response(JSON.stringify(body), { status: 200 });
};
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
    url.startsWith('/api/audit-log/usage')
      ? usageResponse(url)
      : url.startsWith('/api/audit-log/summary')
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
    expect(screen.getAllByRole('status')[0]).toBeInTheDocument();
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

describe('DashboardView agent insights', () => {
  it('renders Wawasan agent below the header for super_admin', async () => {
    boot('/dashboard', 'super_admin');
    expect(await screen.findByRole('heading', { name: 'Wawasan agent' })).toBeInTheDocument();
    expect(await screen.findByText('ci-review-agent')).toBeInTheDocument();
  });
  it('still renders when the summary chunk fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => usageResponse(url)));
    history.replaceState({}, '', '/dashboard');
    render(DashboardView, { load: async () => { throw new Error('chunk gagal'); } });
    expect(await screen.findByRole('heading', { name: 'Wawasan agent' })).toBeInTheDocument();
    expect(await screen.findByText('support-triage')).toBeInTheDocument();
  });
  it('developer triggers no usage request', async () => {
    boot('/dashboard', 'developer');
    await screen.findByTestId('notfound-title');
    expect(fetchMock.mock.calls.some((c) => String(c[0]).startsWith('/api/audit-log/usage'))).toBe(false);
  });
  it('usage feature does not import chart.js', () => {
    const files = import.meta.glob('../../domain/core/audit-log/usage/**/*.{ts,svelte}', {
      query: '?raw',
      import: 'default',
      eager: true,
    }) as Record<string, string>;
    for (const [path, src] of Object.entries(files)) {
      if (path.endsWith('.test.ts')) continue;
      expect(src, path).not.toMatch(/from ['"]chart\.js/);
    }
  });
});

describe('DashboardView chunk failure', () => {
  it('keeps the h1, shows ErrorState, and "Coba lagi" imports again', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        url.startsWith('/api/audit-log/usage')
          ? usageResponse(url)
          : new Response(JSON.stringify(url.startsWith('/api/audit-log/summary') ? mockSummary7d : mockProjectOptions), { status: 200 }),
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
