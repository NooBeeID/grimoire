import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import CreateProjectForm from './CreateProjectForm.svelte';

const base = () => ({ onsubmit: vi.fn(), oncancel: vi.fn() });

describe('CreateProjectForm', () => {
  it('submits entered values and shows counter', async () => {
    const p = base();
    render(CreateProjectForm, p);
    await userEvent.type(screen.getByLabelText(/Nama project/), 'Payments');
    await userEvent.type(screen.getByLabelText(/Deskripsi/), 'abc');
    expect(screen.getByText('3/1000 karakter')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    expect(p.onsubmit).toHaveBeenCalledWith({ name: 'Payments', description: 'abc' });
  });
  it('submitting disables the button; admin hint and errors show', () => {
    render(CreateProjectForm, { ...base(), submitting: true, showAdminHint: true, formError: 'gagal', fieldErrors: { name: 'salah' } });
    expect(screen.getByRole('button', { name: /Buat/ })).toBeDisabled();
    expect(screen.getByText(/Super Admin Anda tidak otomatis/)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('gagal');
    expect(screen.getByText('salah')).toBeInTheDocument();
  });
});
