import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ProjectSettingsForm from './ProjectSettingsForm.svelte';

const initial = { id: 'x', name: 'Payments', description: 'desc' };

describe('ProjectSettingsForm', () => {
  it('save disabled until a change, discard restores initial', async () => {
    render(ProjectSettingsForm, { initial, onsubmit: vi.fn() });
    const save = screen.getByRole('button', { name: 'Simpan' });
    expect(save).toBeDisabled();
    const name = screen.getByLabelText(/Nama project/) as HTMLInputElement;
    await userEvent.type(name, ' v2');
    expect(save).toBeEnabled();
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan perubahan' }));
    expect(name.value).toBe('Payments');
    expect(save).toBeDisabled();
  });
  it('local validation error disables save', async () => {
    render(ProjectSettingsForm, { initial, onsubmit: vi.fn() });
    const name = screen.getByLabelText(/Nama project/);
    await userEvent.clear(name);
    await userEvent.type(name, 'A');
    expect(screen.getByText('Nama project wajib 2-100 karakter')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Simpan' })).toBeDisabled();
  });
  it('follows initial when clean, keeps user value when dirty', async () => {
    const { rerender } = render(ProjectSettingsForm, { initial, onsubmit: vi.fn() });
    const name = screen.getByLabelText(/Nama project/) as HTMLInputElement;
    await rerender({ initial: { ...initial, name: 'Server' }, onsubmit: vi.fn() });
    expect(name.value).toBe('Server');
    await userEvent.type(name, 'X');
    await rerender({ initial: { ...initial, name: 'Server2' }, onsubmit: vi.fn() });
    expect(name.value).toBe('ServerX');
  });
});
