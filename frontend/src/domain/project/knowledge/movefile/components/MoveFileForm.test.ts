import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import MoveFileForm from './MoveFileForm.svelte';

beforeEach(() => setLocale('id'));

describe('MoveFileForm', () => {
  const props = (over: Record<string, unknown> = {}) => ({
    from: 'docs/auth/login.md',
    to: 'docs/auth/login.md',
    active: true,
    onsubmit: vi.fn((e: Event) => e.preventDefault()),
    ...over,
  });

  it('shows the source path in mono and the new path input', () => {
    render(MoveFileForm, props());
    expect(screen.getByText('docs/auth/login.md')).toHaveClass('font-mono');
    expect(screen.getByLabelText('Path baru')).toHaveValue('docs/auth/login.md');
  });

  it('focuses the input and selects the file name without its extension', async () => {
    render(MoveFileForm, props());
    const input = screen.getByLabelText('Path baru') as HTMLInputElement;
    await vi.waitFor(() => expect(input).toHaveFocus());
    expect([input.selectionStart, input.selectionEnd]).toEqual([10, 15]);
  });

  it('does not steal focus while inactive', async () => {
    render(MoveFileForm, props({ active: false }));
    await Promise.resolve();
    expect(screen.getByLabelText('Path baru')).not.toHaveFocus();
  });

  it('shows an inline error and submits through the form', () => {
    const p = props({ error: 'sudah ada' });
    render(MoveFileForm, p);
    expect(screen.getByText('sudah ada')).toBeInTheDocument();
    expect(screen.getByLabelText('Path baru')).toHaveAttribute('aria-invalid', 'true');
    document.querySelector('form')!.requestSubmit();
    expect(p.onsubmit).toHaveBeenCalledTimes(1);
  });
});
