import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { gitCredentials, mockCredentialList } from '../../domain/core/git-credential/listcredentials';
import { setLocale } from '../../shared/stores/i18n';
import { resetSession, setIdentity } from '../../shared/stores/session';
import Settings from './SettingsView.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });

beforeEach(() => {
  setLocale('id');
  setIdentity({ userId: 'u1', email: 'rina@tim.id', name: 'Rina', role: 'developer', mustChangePassword: false });
  history.replaceState({}, '', '/settings');
  Element.prototype.scrollIntoView = vi.fn();
});
afterEach(() => {
  vi.unstubAllGlobals();
  gitCredentials.reset();
  resetSession();
});

describe('SettingsView', () => {
  it('shows the Token Git section', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockCredentialList())));
    const { container } = render(Settings);
    expect(container.querySelector('section#token-git')).not.toBeNull();
    const wrap = container.querySelector('section#token-git')?.parentElement;
    expect(wrap?.className).toContain('max-w-2xl');
    expect(wrap?.className).not.toContain('mx-auto');
    expect(container.querySelector('h2#token-git-title')).not.toBeNull();
    await screen.findByText('…7Qx2');
  });
  it('Ganti opens the save dialog; saving invalidates the store and reloads', async () => {
    const f = vi.fn(async (_u: string, init?: RequestInit) =>
      init?.method === 'PUT'
        ? json(200, { provider: 'gitlab', host: 'git.noobee.id', last4: 'ZZ99', updated_at: '2026-10-22T08:00:00Z', replaced: true })
        : json(200, mockCredentialList()),
    );
    vi.stubGlobal('fetch', f);
    render(Settings);
    await screen.findByText('…7Qx2');
    await userEvent.click(screen.getByRole('button', { name: 'Ganti token GitLab untuk git.noobee.id' }));
    await userEvent.type(await screen.findByLabelText('Token'), 'glpat-abcdefgh');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(f.mock.calls.filter((c) => c[1]?.method !== 'PUT')).toHaveLength(2));
  });
  it('Hapus opens the delete dialog naming the host', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockCredentialList())));
    render(Settings);
    await screen.findByText('…7Qx2');
    await userEvent.click(screen.getByRole('button', { name: 'Hapus token GitLab untuk git.noobee.id' }));
    expect(await screen.findByText(/Hapus token GitLab\?/)).toBeInTheDocument();
  });
  it('#token-git scrolls to and focuses the heading once data is ready', async () => {
    history.replaceState({}, '', '/settings#token-git');
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockCredentialList())));
    render(Settings);
    await screen.findByText('…7Qx2');
    await vi.waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
    expect(document.activeElement?.id).toBe('token-git-title');
  });
});
