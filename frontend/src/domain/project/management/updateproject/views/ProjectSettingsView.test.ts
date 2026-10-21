import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { mockUpdated } from '../mocks/project';
import View from './ProjectSettingsView.svelte';

const project = { id: '0192f5a0-1111-7aaa-8bbb-000000000001', name: 'Payments', description: 'Layanan pembayaran internal' };
const props = () => ({ project, onupdated: vi.fn(), onprojectgone: vi.fn(), onforbidden: vi.fn() });
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const err = (status: number, code: string, details: unknown = null) => ({ error: { code, message: 'pesan server', details } });
async function rename(to = ' v2') {
  await userEvent.type(screen.getByLabelText(/Nama project/), to);
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
}
beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('ProjectSettingsView', () => {
  it('T03-AC2: member edits name -> PATCH {name} only, onupdated + toast', async () => {
    const f = stub(200, mockUpdated({ name: 'Payments v2' }));
    const p = props();
    render(View, p);
    await rename();
    await vi.waitFor(() => expect(p.onupdated).toHaveBeenCalled());
    expect(JSON.parse(f.mock.calls[0]![1]!.body as string)).toEqual({ name: 'Payments v2' });
    expect(get(toasts).some((x) => x.message === 'Perubahan disimpan')).toBe(true);
  });
  it('save disabled without changes and sends no fetch', async () => {
    const f = stub(200, mockUpdated());
    render(View, props());
    const b = screen.getByRole('button', { name: 'Simpan' });
    expect(b).toBeDisabled();
    await userEvent.click(b);
    expect(f).not.toHaveBeenCalled();
  });
  it('400 fields.name shows on input; fields.body -> nothingToSave', async () => {
    stub(400, err(400, 'INVALID_ARGUMENT', { fields: { name: 'nama dipakai' } }));
    render(View, props());
    await rename();
    expect(await screen.findByText('nama dipakai')).toBeInTheDocument();
  });
  it('400 fields.body', async () => {
    stub(400, err(400, 'INVALID_ARGUMENT', { fields: { body: 'x' } }));
    render(View, props());
    await rename();
    expect(await screen.findByRole('alert')).toHaveTextContent('Tidak ada perubahan untuk disimpan');
  });
  it('404 -> onprojectgone; 403 -> toast + onforbidden', async () => {
    stub(404, err(404, 'NOT_FOUND'));
    const a = props();
    const r = render(View, a);
    await rename();
    await vi.waitFor(() => expect(a.onprojectgone).toHaveBeenCalled());
    r.unmount();
    stub(403, err(403, 'FORBIDDEN'));
    const b = props();
    render(View, b);
    await rename();
    await vi.waitFor(() => expect(b.onforbidden).toHaveBeenCalled());
    expect(get(toasts).some((x) => x.message === 'Anda tidak memiliki akses')).toBe(true);
  });
  it('500 keeps input and shows formError', async () => {
    stub(500, err(500, 'INTERNAL'));
    render(View, props());
    await rename();
    expect(await screen.findByRole('alert')).toHaveTextContent('pesan server');
    expect((screen.getByLabelText(/Nama project/) as HTMLInputElement).value).toBe('Payments v2');
  });
});
