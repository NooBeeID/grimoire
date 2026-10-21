import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../stores/i18n';
import CopyField from './CopyField.svelte';
import Dialog from './Dialog.svelte';
import Table from './Table.svelte';

const text = (s: string) => createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('Dialog dismissible/size', () => {
  it('dismissible=false: cancel is prevented, backdrop ignored, close button disabled', async () => {
    const onclose = vi.fn();
    render(Dialog, { props: { open: true, title: 'T', dismissible: false, onclose, children: text('x') } });
    const dlg = document.querySelector('dialog')!;
    const ev = new Event('cancel', { cancelable: true });
    dlg.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    await fireEvent.click(dlg);
    expect(onclose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Tutup', hidden: true })).toBeDisabled();
  });
  it('default stays closable (backdrop click)', async () => {
    const onclose = vi.fn();
    render(Dialog, { props: { open: true, title: 'T', onclose, children: text('x') } });
    await fireEvent.click(document.querySelector('dialog')!);
    expect(onclose).toHaveBeenCalled();
  });
  it('size lg uses max-w-2xl', () => {
    render(Dialog, { props: { open: true, title: 'T', size: 'lg', children: text('x') } });
    expect(document.querySelector('dialog')!.className).toContain('max-w-2xl');
  });
});

describe('Table rowClass/rowAttrs', () => {
  it('applies class and only data-* attributes', () => {
    const cell = createRawSnippet(() => ({ render: () => `<span>c</span>` }));
    render(Table, {
      columns: [{ key: 'a', header: 'A' }],
      rows: [{ a: '1' }],
      rowKey: (r: unknown) => (r as { a: string }).a,
      cell,
      rowClass: () => 'opacity-60',
      rowAttrs: () => ({ 'data-status': 'revoked', onclick: 'x', id: 'bad' }),
    });
    const tr = document.querySelector('tbody tr')!;
    expect(tr.className).toContain('opacity-60');
    expect(tr.getAttribute('data-status')).toBe('revoked');
    expect(tr.hasAttribute('onclick')).toBe(false);
    expect(tr.hasAttribute('id')).toBe(false);
  });
});

describe('CopyField', () => {
  it('copies, shows Tersalin, reverts after 2s', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(CopyField, { value: 'secret-1', label: 'API key' });
    expect(screen.getByLabelText('API key')).toHaveValue('secret-1');
    await fireEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(writeText).toHaveBeenCalledWith('secret-1');
    expect(await screen.findByRole('button', { name: 'Tersalin' })).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(2100);
    expect(screen.getByRole('button', { name: 'Salin' })).toBeInTheDocument();
  });
  it('failure selects content and shows manual hint', async () => {
    vi.stubGlobal('navigator', {});
    render(CopyField, { value: 'abcdef', label: 'L' });
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    const input = screen.getByLabelText('L') as HTMLInputElement;
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(6);
    expect(await screen.findByText(/Tidak bisa menyalin otomatis/)).toBeInTheDocument();
  });
  it('multiline renders textarea', () => {
    render(CopyField, { value: 'a\nb', label: 'M', multiline: true });
    expect(screen.getByLabelText('M').tagName).toBe('TEXTAREA');
  });
});
