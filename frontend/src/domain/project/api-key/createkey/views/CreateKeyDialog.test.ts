import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { MOCK_RAW_KEY, mockCreated } from '../mocks/created';
import Dialog from './CreateKeyDialog.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const errBody = (code: string, details: unknown = null) => ({ error: { code, message: 'pesan server', details }, trace_id: 'TR-9' });
const props = () => ({
  open: true,
  projectId: 'p1',
  onclose: vi.fn(),
  oncreated: vi.fn(),
  onprojectgone: vi.fn(),
  onstale: vi.fn(),
});
async function submit(name: string) {
  await userEvent.type(screen.getByLabelText('Nama key'), name);
  await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
}

describe('CreateKeyDialog', () => {
  it('T04-AC1 success shows result with raw key and snippets; oncreated gets key only', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(201, mockCreated())));
    const p = props();
    render(Dialog, { props: p });
    await submit('claude-code-budi');
    expect(await screen.findByLabelText('API key')).toHaveValue(MOCK_RAW_KEY);
    expect(screen.getByRole('alert')).toHaveTextContent('tidak akan ditampilkan lagi');
    expect((screen.getByLabelText('Claude Code') as HTMLTextAreaElement).value).toContain('https://grimoire.tim.id/mcp');
    expect(p.oncreated).toHaveBeenCalledTimes(1);
    expect(p.oncreated.mock.calls[0]![0]).not.toHaveProperty('raw_key');
  });
  it('mcp_url null falls back to window origin', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(201, mockCreated({ mcp_url: null }))));
    render(Dialog, { props: props() });
    await submit('claude-code-budi');
    const v = ((await screen.findByLabelText('Claude Code')) as HTMLTextAreaElement).value;
    expect(v).toContain(`${window.location.origin}/mcp`);
  });
  it('client validation blocks the request', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, { props: props() });
    await submit('a');
    expect(await screen.findByText('Nama key wajib 2-60 karakter')).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
  });
  it('T04-AC4 NAME_TAKEN keeps input and form', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, errBody('CONFLICT', { reason: 'NAME_TAKEN' }))));
    render(Dialog, { props: props() });
    await submit('dupe');
    expect(await screen.findByText(/sudah dipakai key aktif lain/)).toBeInTheDocument();
    expect(screen.getByLabelText('Nama key')).toHaveValue('dupe');
  });
  it('T04-AC5 KEY_LIMIT_REACHED mentions limit', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, errBody('CONFLICT', { reason: 'KEY_LIMIT_REACHED', limit: 20 }))));
    render(Dialog, { props: props() });
    await submit('another');
    expect(await screen.findByText(/Batas 20 key aktif/)).toBeInTheDocument();
  });
  it('server 400 fields.name shown on field', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(400, errBody('INVALID_ARGUMENT', { fields: { name: 'nama buruk' } }))));
    render(Dialog, { props: props() });
    await submit('okname');
    expect(await screen.findByText('nama buruk')).toBeInTheDocument();
  });
  it('404 closes and calls onprojectgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, errBody('NOT_FOUND'))));
    const p = props();
    render(Dialog, { props: p });
    await submit('okname');
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
    expect(p.onclose).toHaveBeenCalled();
  });
  it('invalid 201 body -> createUncertain + onstale', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(201, { key: {} })));
    const p = props();
    render(Dialog, { props: p });
    await submit('okname');
    expect(await screen.findByText(/Key mungkin sudah dibuat/)).toBeInTheDocument();
    expect(p.onstale).toHaveBeenCalled();
  });
  it('500 shows message with trace id', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(500, errBody('INTERNAL'))));
    render(Dialog, { props: props() });
    await submit('okname');
    expect(await screen.findByText('pesan server')).toBeInTheDocument();
    expect(screen.getByText(/TR-9/)).toBeInTheDocument();
  });
  it('double submit sends one request', async () => {
    const f = vi.fn(async () => json(201, mockCreated()));
    vi.stubGlobal('fetch', f);
    render(Dialog, { props: props() });
    await userEvent.type(screen.getByLabelText('Nama key'), 'okname{Enter}{Enter}');
    await screen.findByLabelText('API key');
    expect(f).toHaveBeenCalledTimes(1);
  });
  it('T04-AC2 closing discards the raw key; reopen is a blank form', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(201, mockCreated())));
    const p = props();
    const view = render(Dialog, { props: p });
    await submit('claude-code-budi');
    await userEvent.click(await screen.findByRole('button', { name: 'Saya sudah menyimpan key' }));
    expect(p.onclose).toHaveBeenCalled();
    expect(view.container.ownerDocument.body.innerHTML).not.toContain(MOCK_RAW_KEY);
    expect(screen.getByLabelText('Nama key')).toHaveValue('');
  });
  it('raw key never reaches console', async () => {
    const spies = (['log', 'info', 'warn', 'error'] as const).map((m) => vi.spyOn(console, m).mockImplementation(() => {}));
    vi.stubGlobal('fetch', vi.fn(async () => json(201, mockCreated())));
    render(Dialog, { props: props() });
    await submit('claude-code-budi');
    await screen.findByLabelText('API key');
    for (const s of spies) expect(JSON.stringify(s.mock.calls)).not.toContain(MOCK_RAW_KEY);
    spies.forEach((s) => s.mockRestore());
  });
});
