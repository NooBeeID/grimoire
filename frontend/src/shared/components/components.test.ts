import { render, screen, fireEvent } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../services/http';
import { setLocale } from '../stores/i18n';
import { toast } from '../stores/toast';
import Button from './Button.svelte';
import CopyField from './CopyField.svelte';
import Dialog from './Dialog.svelte';
import EmptyState from './EmptyState.svelte';
import ErrorState from './ErrorState.svelte';
import Input from './Input.svelte';
import LocaleSwitch from './LocaleSwitch.svelte';
import Pagination from './Pagination.svelte';
import Select from './Select.svelte';
import Spinner from './Spinner.svelte';
import Table from './Table.svelte';
import Textarea from './Textarea.svelte';
import ToastHost from './Toast.svelte';

afterEach(() => {
  setLocale('id');
  toast.clear();
});

const text = (s: string) => createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));

describe('Button', () => {
  it('loading disables and sets aria-busy', () => {
    render(Button, { loading: true, children: text('Go') });
    const b = screen.getByRole('button');
    expect(b).toBeDisabled();
    expect(b).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByRole('status')).toBeInTheDocument();
  });
  it('does not call onclick when disabled; type default button', async () => {
    const onclick = vi.fn();
    render(Button, { disabled: true, onclick, children: text('Go') });
    const b = screen.getByRole('button');
    await userEvent.click(b);
    expect(onclick).not.toHaveBeenCalled();
    expect(b).toHaveAttribute('type', 'button');
  });
});

describe('Input', () => {
  it('links label, error, and binds value', async () => {
    render(Input, { label: 'Nama', error: 'wajib', hint: 'petunjuk' });
    const input = screen.getByLabelText('Nama') as HTMLInputElement;
    expect(input).toHaveAttribute('aria-invalid', 'true');
    const ids = input.getAttribute('aria-describedby')!.split(' ');
    expect(ids.map((i) => document.getElementById(i)?.textContent)).toContain('wajib');
    await userEvent.type(input, 'abc');
    expect(input.value).toBe('abc');
  });
});

describe('Textarea', () => {
  it('links label, error, and binds value', async () => {
    render(Textarea, { label: 'Deskripsi', error: 'terlalu panjang', rows: 3 });
    const ta = screen.getByLabelText('Deskripsi') as HTMLTextAreaElement;
    expect(ta).toHaveAttribute('aria-invalid', 'true');
    expect(ta).toHaveAttribute('rows', '3');
    const ids = ta.getAttribute('aria-describedby')!.split(' ');
    expect(ids.map((i) => document.getElementById(i)?.textContent)).toContain('terlalu panjang');
    await userEvent.type(ta, 'abc');
    expect(ta.value).toBe('abc');
  });
});

describe('Dialog', () => {
  it('opens, has i18n close label, close calls onclose', async () => {
    const onclose = vi.fn();
    render(Dialog, { open: true, title: 'Judul', onclose, children: text('isi') });
    const dlg = document.querySelector('dialog')!;
    expect(dlg).toHaveAttribute('open');
    const close = screen.getByLabelText('Tutup', { selector: 'button' });
    await fireEvent.click(close);
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(dlg).not.toHaveAttribute('open');
  });
  it('dismissible=false ignores backdrop click and disables close button', async () => {
    const onclose = vi.fn();
    render(Dialog, { open: true, title: 'J', dismissible: false, onclose, children: text('isi') });
    await fireEvent.click(document.querySelector('dialog')!);
    expect(onclose).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Tutup', { selector: 'button' })).toBeDisabled();
  });
  it('size lg widens the dialog', () => {
    render(Dialog, { open: true, title: 'J', size: 'lg', children: text('isi') });
    expect(document.querySelector('dialog')!.className).toContain('max-w-2xl');
  });
});

describe('ToastHost', () => {
  it('shows toasts, role alert for errors, trace id, dismiss', async () => {
    render(ToastHost);
    toast.push({ type: 'error', message: 'gagal', traceId: 'abc', timeoutMs: 0 });
    expect(await screen.findByRole('alert')).toHaveTextContent('gagal');
    expect(screen.getByText(/abc/)).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('Tutup notifikasi'));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});

describe('Table', () => {
  const columns = [{ key: 'name', header: 'Nama' }];
  const rowKey = (r: unknown) => (r as { name: string }).name;
  it('loading', () => {
    render(Table, { columns, rows: [], rowKey, loading: true });
    expect(screen.getByRole('table')).toHaveAttribute('aria-busy', 'true');
  });
  it('empty', () => {
    render(Table, { columns, rows: [], rowKey });
    expect(screen.getByText('Tidak ada data untuk ditampilkan')).toBeInTheDocument();
  });
  it('error with retry', async () => {
    const onretry = vi.fn();
    render(Table, { columns, rows: [], rowKey, error: new ApiError({ status: 500, code: 'INTERNAL', message: 'rusak' }), onretry });
    expect(screen.getByRole('alert')).toHaveTextContent('rusak');
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
  it('rows', () => {
    render(Table, { columns, rows: [{ name: 'Budi' }], rowKey });
    expect(screen.getByText('Budi')).toBeInTheDocument();
  });
  it('rowClass and rowAttrs apply class and only data-* attributes', () => {
    render(Table, {
      columns,
      rows: [{ name: 'Budi' }],
      rowKey,
      rowClass: () => 'opacity-60',
      rowAttrs: () => ({ 'data-status': 'revoked', onclick: 'x' }),
    });
    const tr = document.querySelector('tbody tr')!;
    expect(tr.className).toContain('opacity-60');
    expect(tr.getAttribute('data-status')).toBe('revoked');
    expect(tr.hasAttribute('onclick')).toBe(false);
  });
});

describe('CopyField', () => {
  it('copies value to clipboard and shows Tersalin', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(CopyField, { value: 'abc', label: 'Kunci' });
    expect(screen.getByLabelText('Kunci')).toHaveValue('abc');
    await fireEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(writeText).toHaveBeenCalledWith('abc');
    expect(await screen.findByRole('button', { name: 'Tersalin' })).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
  it('multiline renders a textarea', () => {
    render(CopyField, { value: 'a\nb', label: 'M', multiline: true });
    expect(screen.getByLabelText('M').tagName).toBe('TEXTAREA');
  });
});

describe('Pagination', () => {
  it('disables edges and calls onchange', async () => {
    const onchange = vi.fn();
    const { rerender } = render(Pagination, { page: 1, pageSize: 10, total: 25, onchange });
    expect(screen.getByRole('button', { name: 'Sebelumnya' })).toBeDisabled();
    expect(screen.getByText('Halaman 1 dari 3')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Berikutnya' }));
    expect(onchange).toHaveBeenCalledWith(2);
    await rerender({ page: 3, pageSize: 10, total: 25, onchange });
    expect(screen.getByRole('button', { name: 'Berikutnya' })).toBeDisabled();
  });
  it('total 0 -> 1 page', () => {
    render(Pagination, { page: 1, pageSize: 10, total: 0, onchange: () => {} });
    expect(screen.getByRole('button', { name: 'Sebelumnya' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Berikutnya' })).toBeDisabled();
    expect(screen.getByText('Halaman 1 dari 1')).toBeInTheDocument();
  });
});

describe('state components', () => {
  it('defaults', () => {
    render(Spinner);
    expect(screen.getByText('Memuat...')).toBeInTheDocument();
    render(EmptyState);
    expect(screen.getByText('Belum ada data')).toBeInTheDocument();
  });
  it('ErrorState shows trace id', () => {
    render(ErrorState, { error: new ApiError({ status: 500, code: 'X', message: 'm', traceId: 'tid' }) });
    expect(screen.getByText(/tid/)).toBeInTheDocument();
  });
});

describe('LocaleSwitch', () => {
  it('switches locale and other components follow', async () => {
    render(LocaleSwitch);
    render(EmptyState);
    await userEvent.selectOptions(screen.getByLabelText('Bahasa'), 'en');
    expect(screen.getByText('No data yet')).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('en');
  });
});

describe('Select', () => {
  const options = [
    { value: 'a', label: 'Opsi A' },
    { value: 'b', label: 'Opsi B' },
  ];
  it('labels the control and reflects value', () => {
    render(Select, { label: 'Pilih', options, value: 'b' });
    expect(screen.getByLabelText('Pilih')).toHaveValue('b');
  });
  it('exposes error and hint accessibly', () => {
    render(Select, { label: 'Pilih', options, error: 'Salah', hint: 'Petunjuk' });
    const sel = screen.getByLabelText('Pilih');
    expect(sel).toHaveAttribute('aria-invalid', 'true');
    expect(sel.getAttribute('aria-describedby')).toContain('-err');
    expect(screen.getByText('Salah')).toBeInTheDocument();
  });
  it('can be disabled', () => {
    render(Select, { label: 'Pilih', options, disabled: true });
    expect(screen.getByLabelText('Pilih')).toBeDisabled();
  });
});
