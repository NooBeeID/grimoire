import { render, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

const handle = { setValue: vi.fn(), setReadonly: vi.fn(), focus: vi.fn(), destroy: vi.fn() };
const createEditor = vi.fn((_o: { onchange(v: string): void; onsave(): void }) => handle);
vi.mock('./markdownEditor/codemirror', () => ({ createEditor: (o: never) => createEditor(o) }));

import MarkdownEditor from './MarkdownEditor.svelte';

describe('MarkdownEditor with CodeMirror', () => {
  it('replaces the textarea, forwards save and syncs the value in', async () => {
    const onsave = vi.fn();
    const { container, rerender, unmount } = render(MarkdownEditor, { value: 'a', label: 'Isi', onsave });
    await waitFor(() => expect(createEditor).toHaveBeenCalled());
    await waitFor(() => expect(container.querySelector('textarea')).toBeNull());
    createEditor.mock.calls[0]![0].onsave();
    expect(onsave).toHaveBeenCalledTimes(1);
    await rerender({ value: 'b', label: 'Isi', onsave });
    await waitFor(() => expect(handle.setValue).toHaveBeenLastCalledWith('b'));
    unmount();
    expect(handle.destroy).toHaveBeenCalled();
  });
});
