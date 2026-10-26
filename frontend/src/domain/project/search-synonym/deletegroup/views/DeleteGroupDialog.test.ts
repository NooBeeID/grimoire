import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { SynonymGroup } from '../../listgroups';
import Dialog from './DeleteGroupDialog.svelte';

const mockGroup = (n: number, over: Partial<SynonymGroup> = {}): SynonymGroup => ({
  id: `0192f5a0-3333-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  terms: ['login', 'autentikasi', 'masuk'],
  created_at: '2026-10-23T10:00:00Z',
  updated_at: '2026-10-23T10:00:00Z',
  created_by_label: 'ana@tim.id',
  updated_by_label: 'ana@tim.id',
  ...over,
});


beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = (over: Record<string, unknown> = {}) => ({
  open: true,
  projectId: 'p1',
  group: mockGroup(1),
  onclose: vi.fn(),
  ondeleted: vi.fn(),
  onstale: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});

describe('DeleteGroupDialog', () => {
  it('lists the terms and deletes on confirm', async () => {
    const f = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, p);
    expect(screen.getByText('autentikasi')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Hapus kelompok' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith(mockGroup(1).id));
    expect(p.onclose).toHaveBeenCalled();
  });
  it('404 -> onstale', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x' } }), { status: 404 })));
    const p = props();
    render(Dialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus kelompok' }));
    await vi.waitFor(() => expect(p.onstale).toHaveBeenCalled());
    expect(p.ondeleted).not.toHaveBeenCalled();
  });
  it('500 keeps the dialog open with the message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'Server gagal' } }), { status: 500 })));
    const p = props();
    render(Dialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus kelompok' }));
    expect(await screen.findByText('Server gagal')).toBeInTheDocument();
    expect(p.onclose).not.toHaveBeenCalled();
  });
});
