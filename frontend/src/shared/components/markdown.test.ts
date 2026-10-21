import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const renderMermaid = vi.fn();
vi.mock('../utils/markdown/mermaid', () => ({ renderMermaid: (...a: unknown[]) => renderMermaid(...a) }));

import MarkdownEditor from './MarkdownEditor.svelte';
import MarkdownView from './MarkdownView.svelte';

const src = '# Judul\n\n```mermaid\nflowchart LR\n  A-->B\n```\n';

beforeEach(() => renderMermaid.mockReset());

describe('MarkdownView mermaid', () => {
  it('replaces the mermaid block with the svg inside role=img', async () => {
    renderMermaid.mockResolvedValue({ ok: true, svg: '<svg data-t="1"><text>A</text></svg>' });
    const { container } = render(MarkdownView, { source: src });
    const img = await screen.findByRole('img', { name: 'Diagram Mermaid' });
    expect(img.querySelector('svg')).not.toBeNull();
    expect(container.querySelector('.md-mermaid pre')).toBeNull();
    expect(renderMermaid).toHaveBeenCalledWith('flowchart LR\n  A-->B', expect.any(String));
  });

  it('keeps the code and shows a note when rendering fails', async () => {
    renderMermaid.mockResolvedValue({ ok: false });
    const { container } = render(MarkdownView, { source: src });
    expect(await screen.findByText('Diagram Mermaid tidak bisa dirender')).toBeInTheDocument();
    expect(container.querySelector('.md-mermaid pre')).not.toBeNull();
  });

  it('does not render mermaid for plain markdown', () => {
    render(MarkdownView, { source: '# Halo' });
    expect(renderMermaid).not.toHaveBeenCalled();
  });

  it('ignores a stale render when the source changes before it resolves', async () => {
    let resolveFirst!: (v: unknown) => void;
    renderMermaid.mockImplementationOnce(() => new Promise((r) => (resolveFirst = r)));
    const { rerender, container } = render(MarkdownView, { source: src });
    await rerender({ source: '# Lain saja' });
    resolveFirst({ ok: true, svg: '<svg data-stale="1"></svg>' });
    await new Promise((r) => setTimeout(r, 10));
    expect(container.querySelector('[data-stale]')).toBeNull();
  });
});

describe('MarkdownEditor (textarea fallback)', () => {
  it('edits the value two-way', async () => {
    const { component } = render(MarkdownEditor, { value: 'awal', label: 'Isi' });
    const area = await screen.findByLabelText('Isi');
    expect(area).toHaveValue('awal');
    await userEvent.type(area, '!');
    expect(area).toHaveValue('awal!');
    void component;
  });

  it('calls onsave on Ctrl+S', async () => {
    const onsave = vi.fn();
    render(MarkdownEditor, { value: '', label: 'Isi', onsave });
    const area = await screen.findByLabelText('Isi');
    area.focus();
    await userEvent.keyboard('{Control>}s{/Control}');
    await waitFor(() => expect(onsave).toHaveBeenCalledTimes(1));
  });

  it('respects readonly', async () => {
    render(MarkdownEditor, { value: 'x', label: 'Isi', readonly: true });
    expect(await screen.findByLabelText('Isi')).toHaveAttribute('readonly');
  });
});
