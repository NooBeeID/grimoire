import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import Button from './DownloadSpecButton.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('DownloadSpecButton', () => {
  it('downloads and saves the file', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('x', { status: 200 })));
    const create = vi.fn(() => 'blob:x');
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: create, revokeObjectURL: vi.fn() }));
    let name = '';
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      name = this.download;
    });
    render(Button, { projectId: 'p1', spec: { name: 'payment', format: 'yaml' }, onspecgone: vi.fn() });
    await userEvent.click(screen.getByRole('button', { name: 'Unduh' }));
    await vi.waitFor(() => expect(name).toBe('payment.yaml'));
  });
  it('404 calls onspecgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    const onspecgone = vi.fn();
    render(Button, { projectId: 'p1', spec: { name: 'payment', format: 'yaml' }, onspecgone });
    await userEvent.click(screen.getByRole('button', { name: 'Unduh' }));
    await vi.waitFor(() => expect(onspecgone).toHaveBeenCalled());
  });
});
