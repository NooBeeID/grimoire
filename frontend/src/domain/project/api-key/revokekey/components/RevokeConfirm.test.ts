import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import RevokeConfirm from './RevokeConfirm.svelte';

beforeEach(() => setLocale('id'));

describe('RevokeConfirm', () => {
  it('mentions name and prefix', () => {
    const { container } = render(RevokeConfirm, { props: { target: { id: 'x', name: 'ci-pipeline', prefix: 'a1b2c3d4' } } });
    expect(container.textContent).toContain('ci-pipeline');
    expect(container.querySelector('code')).toHaveTextContent('a1b2c3d4');
  });
  it('renders a name containing HTML literally', () => {
    const name = '<img src=x onerror=alert(1)>';
    const { container } = render(RevokeConfirm, { props: { target: { id: 'x', name, prefix: 'a1b2c3d4' } } });
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain(name);
  });
  it('shows error in the alert region', () => {
    render(RevokeConfirm, { props: { target: { id: 'x', name: 'n', prefix: 'a1b2c3d4' }, error: 'Gagal mencabut' } });
    expect(screen.getByRole('alert')).toHaveTextContent('Gagal mencabut');
  });
});
