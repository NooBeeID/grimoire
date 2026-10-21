import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import RenameForm from './RenameForm.svelte';

beforeEach(() => setLocale('id'));

describe('RenameForm', () => {
  it('shows the initial value, the rules hint and the new URL preview', () => {
    render(RenameForm, { value: 'payment', projectId: 'p1', onsubmit: vi.fn() });
    expect(screen.getByLabelText('Nama baru')).toHaveValue('payment');
    expect(screen.getByText('URL baru: /projects/p1/api-specs/payment')).toBeInTheDocument();
  });
  it('updates the preview while typing and hides it when empty', async () => {
    render(RenameForm, { value: 'payment', projectId: 'p1', onsubmit: vi.fn() });
    const input = screen.getByLabelText('Nama baru');
    await userEvent.clear(input);
    expect(screen.queryByText(/URL baru/)).toBeNull();
    await userEvent.type(input, 'payment-v2');
    expect(screen.getByText('URL baru: /projects/p1/api-specs/payment-v2')).toBeInTheDocument();
  });
  it('submits on Enter without reloading the page', async () => {
    const onsubmit = vi.fn();
    render(RenameForm, { value: 'a', projectId: 'p1', onsubmit });
    await userEvent.type(screen.getByLabelText('Nama baru'), 'b{Enter}');
    expect(onsubmit).toHaveBeenCalledTimes(1);
  });
  it('shows a field error and can be disabled; disables autocomplete', () => {
    render(RenameForm, { value: 'a', projectId: 'p1', error: 'Nama sudah dipakai', disabled: true, onsubmit: vi.fn() });
    const input = screen.getByLabelText('Nama baru');
    expect(screen.getByText('Nama sudah dipakai')).toBeInTheDocument();
    expect(input).toBeDisabled();
    expect(input).toHaveAttribute('autocomplete', 'off');
  });
});
