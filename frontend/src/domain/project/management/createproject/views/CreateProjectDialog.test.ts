import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { get } from 'svelte/store';
import { mockCreated } from '../mocks/project';
import Dialog from './CreateProjectDialog.svelte';

const props = (over: Record<string, unknown> = {}) => ({ open: true, isSuperAdmin: false, onclose: vi.fn(), oncreated: vi.fn(), ...over });
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('CreateProjectDialog', () => {
  it('T03-AC1 invalid name shows field error without request', async () => {
    const f = stub(201, mockCreated());
    render(Dialog, props());
    await userEvent.type(screen.getByLabelText(/Nama project/), 'A');
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    expect(await screen.findByText('Nama project wajib 2-100 karakter')).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
  });
  it('success calls oncreated and toasts', async () => {
    stub(201, mockCreated());
    const p = props();
    render(Dialog, p);
    await userEvent.type(screen.getByLabelText(/Nama project/), 'Payments');
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    await vi.waitFor(() => expect(p.oncreated).toHaveBeenCalledTimes(1));
    expect(p.oncreated.mock.calls[0]![0].name).toBe('Payments');
    expect(get(toasts).some((x) => x.message.includes('Payments'))).toBe(true);
  });
  it('server field error shows on name and input is kept', async () => {
    stub(400, { error: { code: 'INVALID_ARGUMENT', message: 'x', details: { fields: { name: 'sudah dipakai' } } } });
    render(Dialog, props());
    const input = screen.getByLabelText(/Nama project/) as HTMLInputElement;
    await userEvent.type(input, 'Payments');
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    expect(await screen.findByText('sudah dipakai')).toBeInTheDocument();
    expect(input.value).toBe('Payments');
  });
  it('server 500 shows formError with trace id and keeps dialog open', async () => {
    stub(500, { error: { code: 'INTERNAL', message: 'boom', details: null }, trace_id: 'T9' });
    const p = props();
    render(Dialog, p);
    await userEvent.type(screen.getByLabelText(/Nama project/), 'Payments');
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/boom.*T9/);
    expect(p.oncreated).not.toHaveBeenCalled();
  });
  it('double click sends a single request', async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const f = vi.fn(async () => {
      await gate;
      return new Response(JSON.stringify(mockCreated()), { status: 201 });
    });
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await userEvent.type(screen.getByLabelText(/Nama project/), 'Payments');
    const b = screen.getByRole('button', { name: 'Buat' });
    await userEvent.dblClick(b);
    await vi.waitFor(() => expect(f).toHaveBeenCalled());
    expect(f).toHaveBeenCalledTimes(1);
    release();
  });
  it('super admin sees the hint', () => {
    render(Dialog, props({ isSuperAdmin: true }));
    expect(screen.getByText(/Super Admin Anda tidak otomatis/)).toBeInTheDocument();
  });
});
