import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../stores/i18n';
import EmailDeliveryNote from './EmailDeliveryNote.svelte';
import TemporaryCredentialsDialog from './TemporaryCredentialsDialog.svelte';

afterEach(() => setLocale('id'));

const creds = { email: 'rani@tim.id', temporary_password: 'Hm8tYq3RkW9vNcPz', login_url: '/login' };
const base = { open: true, credentials: creds, emailStatus: 'not_configured' as const, expiresAt: '2026-10-28T10:05:00Z', onclose: vi.fn() };

describe('EmailDeliveryNote', () => {
  it.each([
    [true, /email berisi alamat login/],
    [false, /Email belum diatur di server ini/],
    [null, /Bila email gagal terkirim/],
  ])('varian %s', (v, re) => {
    render(EmailDeliveryNote, { emailEnabled: v });
    expect(screen.getByTestId('email-delivery-note')).toHaveTextContent(re);
  });
});

describe('TemporaryCredentialsDialog', () => {
  it('menampilkan tiga field, URL relatif diresolusi ke origin, dan pesan siap tempel', () => {
    render(TemporaryCredentialsDialog, { ...base, onclose: vi.fn() });
    expect(screen.getByLabelText('Email')).toHaveValue('rani@tim.id');
    expect(screen.getByTestId('temporary-password')).toHaveValue('Hm8tYq3RkW9vNcPz');
    const url = `${window.location.origin}/login`;
    expect(screen.getByLabelText('Alamat login')).toHaveValue(url);
    const msg = (screen.getByLabelText('Pesan untuk penerima') as HTMLTextAreaElement).value;
    expect(msg).toContain(url);
    expect(msg).toContain('rani@tim.id');
    expect(msg).toContain('Hm8tYq3RkW9vNcPz');
    expect(screen.getByRole('alert')).toHaveTextContent('hanya ditampilkan sekali');
  });
  it('kalimat sebab berbeda untuk failed dan not_configured', () => {
    const { unmount } = render(TemporaryCredentialsDialog, { ...base, emailStatus: 'failed' });
    expect(screen.getByText(/gagal terkirim/)).toBeInTheDocument();
    unmount();
    render(TemporaryCredentialsDialog, base);
    expect(screen.getByText(/belum diatur di server, jadi undangan/)).toBeInTheDocument();
  });
  it('tombol X nonaktif dan tombol selesai memanggil onclose sekali', async () => {
    const onclose = vi.fn();
    render(TemporaryCredentialsDialog, { ...base, onclose });
    expect(screen.getByRole('button', { name: 'Tutup' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Saya sudah menyalin' }));
    expect(onclose).toHaveBeenCalledTimes(1);
  });
  it('tanpa credentials tidak ada password di DOM', () => {
    const { container } = render(TemporaryCredentialsDialog, { ...base, credentials: null });
    expect(container.innerHTML).not.toContain('Hm8tYq3RkW9vNcPz');
  });
});
