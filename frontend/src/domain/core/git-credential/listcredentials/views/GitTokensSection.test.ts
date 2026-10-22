import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { resetSession, setIdentity } from '../../../../../shared/stores/session';
import { mockCredential, mockCredentialList } from '../mocks/credentials';
import { gitCredentials } from '../stores/gitCredentials.store';
import Section from './GitTokensSection.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
beforeEach(() => {
  setLocale('id');
  setIdentity({ userId: 'u1', email: 'a@b.id', name: 'Rina', role: 'developer', mustChangePassword: false });
});
afterEach(() => {
  vi.unstubAllGlobals();
  gitCredentials.reset();
  resetSession();
});

describe('GitTokensSection', () => {
  it('loads once on mount and calls onready', async () => {
    const f = vi.fn(async () => json(200, mockCredentialList()));
    vi.stubGlobal('fetch', f);
    const onready = vi.fn();
    render(Section, { props: { onready, onsave: vi.fn() } });
    await screen.findByText('…7Qx2');
    expect(f).toHaveBeenCalledTimes(1);
    expect(onready).toHaveBeenCalledTimes(1);
    expect(document.getElementById('token-git-title')).not.toBeNull();
  });
  it('invalidate reloads', async () => {
    const f = vi.fn(async () => json(200, mockCredentialList()));
    vi.stubGlobal('fetch', f);
    render(Section, { props: {} });
    await screen.findByText('…7Qx2');
    gitCredentials.invalidate();
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(2));
  });
  it('disabled feature shows admin note and no actions', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { enabled: false, allowed_gitlab_hosts: ['gitlab.com'], items: [] })));
    render(Section, { props: { onsave: vi.fn(), ondelete: vi.fn() } });
    expect((await screen.findByText('GIT_CREDENTIAL_KEY')).tagName).toBe('CODE');
    expect(screen.queryByRole('button', { name: /token/i })).toBeNull();
  });
  it('add-host link opens the save dialog for GitLab', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockCredentialList({ items: [mockCredential()] }))));
    const onadd = vi.fn();
    render(Section, { props: { onadd } });
    await screen.findByText('…a9F3');
    // gitlab.com sudah punya baris placeholder; git.noobee.id belum -> tautan tampil
    await userEvent.click(screen.getByRole('button', { name: 'Tambah host GitLab' }));
    expect(onadd).toHaveBeenCalledWith(expect.objectContaining({ provider: 'gitlab', gitlabHosts: ['git.noobee.id'] }));
  });
  it('add-host link stays available even when every allowed host is saved (host is typed freely)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockCredentialList({ allowed_gitlab_hosts: ['git.noobee.id'] }))));
    render(Section, { props: { onadd: vi.fn() } });
    await screen.findByText('…7Qx2');
    expect(screen.getByRole('button', { name: 'Tambah host GitLab' })).toBeInTheDocument();
  });

});
