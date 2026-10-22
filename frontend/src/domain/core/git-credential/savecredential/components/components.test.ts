import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import Form from './SaveTokenForm.svelte';

beforeEach(() => setLocale('id'));

describe('SaveTokenForm', () => {
  it('GitHub has no host field and shows the repo scope', () => {
    render(Form, { props: { provider: 'github', gitlabHosts: ['gitlab.com'], onsubmit: vi.fn() } });
    expect(screen.queryByLabelText('Host')).toBeNull();
    expect(screen.getByText('repo')).toBeInTheDocument();
  });
  it('scope follows the provider', async () => {
    render(Form, { props: { provider: 'github', gitlabHosts: ['gitlab.com'], onsubmit: vi.fn() } });
    await userEvent.selectOptions(screen.getByLabelText('Provider'), 'gitlab');
    expect(screen.getByText('read_api')).toBeInTheDocument();
    expect(screen.getByText('read_repository')).toBeInTheDocument();
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
  });
  it('token input is a password field that is not autofilled', () => {
    render(Form, { props: { gitlabHosts: [], onsubmit: vi.fn() } });
    const el = screen.getByLabelText('Token');
    expect(el).toHaveAttribute('type', 'password');
    expect(el).toHaveAttribute('autocomplete', 'off');
  });
  it('Enter submits', async () => {
    const onsubmit = vi.fn();
    render(Form, { props: { gitlabHosts: [], onsubmit } });
    await userEvent.type(screen.getByLabelText('Token'), 'abc{Enter}');
    expect(onsubmit).toHaveBeenCalled();
  });
});
