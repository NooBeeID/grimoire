import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKey, mockRevokedKey, mockTeamKeys } from '../mocks/keys';
import type { ApiKey } from '../types';
import KeysTable from './KeysTable.svelte';

beforeEach(() => setLocale('id'));
const props = (items: ApiKey[], over: Record<string, unknown> = {}) => ({ items, onrevoke: vi.fn(), ...over });

describe('KeysTable', () => {
  it('T04-AC2 never renders a raw key even if present in data', () => {
    const k = { ...mockKey(1), raw_key: 'gr_a1b2c3d4_SECRETSECRETSECRETSECRETSECRETSECRET12345' };
    const { container } = render(KeysTable, props([k]));
    expect(container.innerHTML).not.toContain('SECRET');
    expect(container.innerHTML).toContain('gr_a1b2c3d4_…');
  });
  it('revoked row: dimmed, data-status, revoked-by line, no revoke button', () => {
    const { container } = render(KeysTable, props([mockRevokedKey(2)]));
    const tr = container.querySelector('tbody tr')!;
    expect(tr.className).toContain('opacity-60');
    expect(tr.getAttribute('data-status')).toBe('revoked');
    expect(screen.getByText(/Dicabut oleh ana@tim.id pada/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Cabut/ })).toBeNull();
  });
  it('active row: revoke button with aria-label calls onrevoke', async () => {
    const k = mockKey(1);
    const p = props([k]);
    render(KeysTable, p);
    await userEvent.click(screen.getByRole('button', { name: 'Cabut key ci-pipeline' }));
    expect(p.onrevoke).toHaveBeenCalledWith(k);
  });
  it('never used shows dash with aria-label; used shows time', () => {
    const { container } = render(KeysTable, props([mockKey(1, { last_used_at: null }), mockKey(2)]));
    expect(screen.getByLabelText('Belum pernah dipakai')).toHaveTextContent('–');
    expect(container.querySelector('time[datetime="2026-10-23T11:42:00Z"]')).not.toBeNull();
  });
  it('default has no owner column, avatar, or creator email', () => {
    const { container } = render(KeysTable, props(mockTeamKeys().slice(0, 2)));
    expect(screen.queryByRole('columnheader', { name: 'Dibuat oleh' })).toBeNull();
    expect(container.querySelector('.avatar')).toBeNull();
    expect(container.textContent).not.toContain('ana@tim.id');
  });
  it('showOwner adds the owner column after Nama with avatar and email', () => {
    const { container } = render(KeysTable, props(mockTeamKeys(), { showOwner: true }));
    const headers = screen.getAllByRole('columnheader');
    expect(headers[0]).toHaveTextContent('Nama');
    expect(headers[1]).toHaveTextContent('Dibuat oleh');
    const row = container.querySelector('tbody tr[data-key-id$="000000000004"]')!;
    expect(row.textContent).toContain('budi@tim.id');
    const av = row.querySelectorAll('.avatar');
    expect(av).toHaveLength(1);
    expect(av[0]).toHaveTextContent('B');
  });
  it('legacy key shows unknown owner without avatar', () => {
    const { container } = render(KeysTable, props(mockTeamKeys().slice(4), { showOwner: true }));
    const unknown = screen.getByText('Tidak diketahui');
    expect(unknown.className).toContain('text-muted');
    expect(container.querySelector('.avatar')).toBeNull();
  });
  it('created column is date only', () => {
    for (const showOwner of [false, true]) {
      const { container, unmount } = render(KeysTable, props([mockKey(1)], { showOwner }));
      const time = container.querySelector('time[datetime="2026-10-23T10:00:00Z"]')!;
      expect(time.textContent).not.toMatch(/@|·/);
      unmount();
    }
  });
  it('long owner email is truncated with a full title', () => {
    const email = 'a'.repeat(50) + '@tim.id';
    const { container } = render(KeysTable, props([mockKey(1, { created_by_label: email })], { showOwner: true }));
    const span = container.querySelector(`span[title="${email}"]`)!;
    expect(span.className).toContain('truncate');
  });
  it('audit link only when auditHref given', () => {
    const { unmount } = render(KeysTable, props([mockKey(1)]));
    expect(screen.queryByRole('link')).toBeNull();
    unmount();
    render(KeysTable, props([mockKey(1)], { auditHref: () => '/audit-log?x=1' }));
    expect(screen.getByRole('link', { name: /ci-pipeline/ })).toHaveAttribute('href', '/audit-log?x=1');
  });
  it('renders names literally and has no delete control', () => {
    const { container } = render(KeysTable, props([mockKey(1, { name: '<img src=x onerror=alert(1)>' })]));
    expect(container.querySelector('img')).toBeNull();
    expect(screen.queryByText(/Hapus/)).toBeNull();
  });
});
