import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { MOCK_RAW_KEY } from '../mocks/created';
import CreateKeyForm from './CreateKeyForm.svelte';
import KeyResult from './KeyResult.svelte';

beforeEach(() => setLocale('id'));

describe('CreateKeyForm', () => {
  it('submit button and input are disabled while submitting', () => {
    render(CreateKeyForm, { submitting: true, onsubmit: vi.fn() });
    expect(screen.getByRole('button', { name: /Buat/ })).toBeDisabled();
    expect(screen.getByRole('textbox')).toBeDisabled();
  });
  it('Enter submits the typed name', async () => {
    const onsubmit = vi.fn();
    render(CreateKeyForm, { onsubmit });
    await userEvent.type(screen.getByRole('textbox'), 'ci-pipeline{Enter}');
    expect(onsubmit).toHaveBeenCalledWith('ci-pipeline');
  });
  it('fieldError sets aria-invalid on the input', () => {
    render(CreateKeyForm, { fieldError: 'Nama wajib diisi', onsubmit: vi.fn() });
    expect(screen.getByRole('textbox')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByText('Nama wajib diisi')).toBeInTheDocument();
  });
  it('formError shows in an alert with trace id', () => {
    render(CreateKeyForm, { formError: 'Gagal', traceId: 'tr-1', onsubmit: vi.fn() });
    expect(screen.getByRole('alert')).toHaveTextContent('Gagal');
    expect(screen.getByRole('alert')).toHaveTextContent('tr-1');
  });
});

describe('KeyResult', () => {
  const props = { rawKey: MOCK_RAW_KEY, name: 'claude-code-budi', mcpUrl: 'https://grimoire.tim.id/mcp' };
  it('shows the once-only warning as an alert', () => {
    render(KeyResult, props);
    expect(screen.getByRole('alert')).toHaveTextContent('tidak akan ditampilkan lagi');
  });
  it('raw key is in the "API key" CopyField', () => {
    render(KeyResult, props);
    expect(screen.getByLabelText('API key')).toHaveValue(MOCK_RAW_KEY);
  });
  it('snippets contain raw key and URL', () => {
    render(KeyResult, props);
    const cc = screen.getByLabelText('Claude Code') as HTMLTextAreaElement;
    expect(cc.value).toContain('claude mcp add');
    expect(cc.value).toContain(MOCK_RAW_KEY);
    expect(cc.value).toContain(props.mcpUrl);
    const json = screen.getByLabelText(/Konfigurasi JSON/) as HTMLTextAreaElement;
    expect(json.value).toContain(MOCK_RAW_KEY);
    expect(json.value).toContain(props.mcpUrl);
  });
});
