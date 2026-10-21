import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../shared/stores/i18n';
import { resetSession, setIdentity } from '../../shared/stores/session';
import { startRouter } from '../routes/router';
import View from './ProjectWorkspaceView.svelte';

const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';
const ME = '0192f5a0-cccc-7aaa-8bbb-000000000001';
const detail = (level: 'owner' | 'member' | 'admin') => ({
  id: PID,
  name: 'Payments',
  description: 'desc',
  created_by: null,
  created_by_label: null,
  created_at: '2026-10-22T09:00:00Z',
  updated_at: '2026-10-22T09:00:00Z',
  my_role: level === 'admin' ? null : level,
  access_level: level,
  is_member: level !== 'admin',
  member_count: 1,
  counts: { files: null, specs: null, keys: null },
  owner_count: 1,
});
const members = {
  items: [
    {
      user_id: ME,
      name: 'Ana',
      email: 'a@tim.id',
      is_active: true,
      found: true,
      role: 'owner',
      added_at: '2026-10-22T09:00:00Z',
      added_by: null,
      added_by_label: null,
    },
  ],
};
function stub(level: 'owner' | 'member' | 'admin') {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/members')) return new Response(JSON.stringify(members), { status: 200 });
      return new Response(JSON.stringify(detail(level)), { status: 200 });
    }),
  );
}
let stop: () => void;
beforeEach(() => {
  setLocale('id');
  resetSession();
  setIdentity({ userId: ME, email: 'a@tim.id', name: 'Ana', role: 'developer', mustChangePassword: false });
  history.replaceState({}, '', '/');
  stop = startRouter([]);
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe('ProjectWorkspaceView', () => {
  it('without tab replaces to the default tab', async () => {
    stub('owner');
    render(View, { params: { projectID: PID } });
    await vi.waitFor(() => expect(window.location.pathname).toBe(`/projects/${PID}/api-keys`));
  });
  it('unknown tab keeps the header and shows tab notFound', async () => {
    stub('owner');
    render(View, { params: { projectID: PID, tab: 'xyz' } });
    expect(await screen.findByText('Bagian ini tidak tersedia')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Payments');
  });
  it('nav marks the active tab with aria-current and sets document title', async () => {
    stub('owner');
    render(View, { params: { projectID: PID, tab: 'settings' } });
    const link = await screen.findByRole('link', { name: 'Settings' });
    expect(link).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Members' })).not.toHaveAttribute('aria-current');
    await vi.waitFor(() => expect(document.title).toBe('Payments · Grimoire'));
  });
  it('members tab renders the members list', async () => {
    stub('owner');
    render(View, { params: { projectID: PID, tab: 'members' } });
    expect(await screen.findByText('a@tim.id')).toBeInTheDocument();
  });
  it('member does not see the danger zone; owner and admin do (UX)', async () => {
    stub('member');
    const a = render(View, { params: { projectID: PID, tab: 'settings' } });
    await screen.findByLabelText(/Nama project/);
    expect(screen.queryByText('Zona bahaya')).toBeNull();
    a.unmount();
    for (const lvl of ['owner', 'admin'] as const) {
      stub(lvl);
      const r = render(View, { params: { projectID: PID, tab: 'settings' } });
      expect(await screen.findByText('Zona bahaya')).toBeInTheDocument();
      r.unmount();
    }
  });
  it('api-keys tab renders and is marked current', async () => {
    stub('owner');
    render(View, { params: { projectID: PID, tab: 'api-keys' } });
    const link = await screen.findByRole('link', { name: 'API Keys' });
    expect(link).toHaveAttribute('aria-current', 'page');
  });
});
