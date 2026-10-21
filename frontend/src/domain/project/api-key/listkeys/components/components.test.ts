import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKey, mockRevokedKey } from '../mocks/keys';
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
    expect(screen.getByLabelText('Belum pernah dipakai')).toHaveTextContent('-');
    expect(container.querySelector('time[datetime="2026-10-23T11:42:00Z"]')).not.toBeNull();
  });
  it('unknown creator label', () => {
    render(KeysTable, props([mockKey(1, { created_by_label: null })]));
    expect(screen.getByText(/Tidak diketahui/)).toBeInTheDocument();
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
