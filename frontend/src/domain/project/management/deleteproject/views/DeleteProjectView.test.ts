import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import View from './DeleteProjectView.svelte';

const project = { id: '0192f5a0-1111-7aaa-8bbb-000000000001', name: 'Payments', member_count: 2, counts: { files: null, specs: null, keys: null } };
const props = () => ({ project, ondeleted: vi.fn(), onprojectgone: vi.fn(), onforbidden: vi.fn() });
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(body === null ? null : JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const err = (status: number, code: string, details: unknown = null) => ({ error: { code, message: 'pesan server', details } });
async function openAndConfirm(name = 'Payments') {
  await userEvent.click(screen.getByRole('button', { name: 'Hapus project' }));
  await userEvent.type(screen.getByLabelText('Nama project'), name);
  await userEvent.click(screen.getByRole('button', { name: 'Hapus permanen' }));
}
beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('DeleteProjectView', () => {
  it('opens dialog with focus on the confirmation input and shows impact with dashes', async () => {
    render(View, props());
    await userEvent.click(screen.getByRole('button', { name: 'Hapus project' }));
    expect(screen.getByLabelText('Nama project')).toHaveFocus();
    expect(screen.getByText(/Akan dihapus: 2 anggota, - file, - spec, - key\./)).toBeInTheDocument();
  });
  it('success calls ondeleted and toasts', async () => {
    const f = stub(204, null);
    const p = props();
    render(View, p);
    await openAndConfirm();
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('Payments'));
    expect(JSON.parse(f.mock.calls[0]![1]!.body as string)).toEqual({ confirm_name: 'Payments' });
    expect(get(toasts).some((x) => x.message === 'Project "Payments" dihapus')).toBe(true);
  });
  it('400 CONFIRMATION_MISMATCH shows i18n message and keeps dialog', async () => {
    stub(400, err(400, 'INVALID_ARGUMENT', { reason: 'CONFIRMATION_MISMATCH' }));
    const p = props();
    render(View, p);
    await openAndConfirm();
    expect(await screen.findByRole('alert')).toHaveTextContent('Nama konfirmasi tidak cocok');
    expect(p.ondeleted).not.toHaveBeenCalled();
  });
  it('404 -> toast alreadyGone + onprojectgone; 403 -> toast + onforbidden', async () => {
    stub(404, err(404, 'NOT_FOUND'));
    const a = props();
    const r = render(View, a);
    await openAndConfirm();
    await vi.waitFor(() => expect(a.onprojectgone).toHaveBeenCalled());
    expect(get(toasts).some((x) => x.message === 'Project sudah tidak ada')).toBe(true);
    r.unmount();
    stub(403, err(403, 'FORBIDDEN'));
    const b = props();
    render(View, b);
    await openAndConfirm();
    await vi.waitFor(() => expect(b.onforbidden).toHaveBeenCalled());
  });
  it('500 keeps dialog open with server message', async () => {
    stub(500, err(500, 'INTERNAL'));
    render(View, props());
    await openAndConfirm();
    expect(await screen.findByRole('alert')).toHaveTextContent('pesan server');
  });
  it('cancel clears the typed name', async () => {
    render(View, props());
    await userEvent.click(screen.getByRole('button', { name: 'Hapus project' }));
    await userEvent.type(screen.getByLabelText('Nama project'), 'Pay');
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    await userEvent.click(screen.getByRole('button', { name: 'Hapus project' }));
    expect((screen.getByLabelText('Nama project') as HTMLInputElement).value).toBe('');
  });
});
