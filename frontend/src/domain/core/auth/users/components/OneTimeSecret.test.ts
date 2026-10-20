import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import OneTimeSecret from './OneTimeSecret.svelte';

describe('OneTimeSecret', () => {
  it('copies and shows Tersalin', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    render(OneTimeSecret, { value: 'Kq7mZp3RtX9aHw2N' });
    expect(screen.getByRole('note')).toBeInTheDocument();
    const input = screen.getByLabelText('Password sementara') as HTMLInputElement;
    expect(input).toHaveAttribute('readonly');
    expect(input.value).toBe('Kq7mZp3RtX9aHw2N');
    expect(input.getAttribute('title')).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(writeText).toHaveBeenCalledWith('Kq7mZp3RtX9aHw2N');
    expect(await screen.findByRole('button', { name: 'Tersalin' })).toBeInTheDocument();
  });
  it('falls back to manual copy when clipboard rejects', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('no')) }, configurable: true });
    render(OneTimeSecret, { value: 'abc' });
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(await screen.findByText('Salin manual: teks sudah dipilih')).toBeInTheDocument();
  });
});

