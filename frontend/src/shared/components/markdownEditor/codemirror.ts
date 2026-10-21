// Satu-satunya tempat yang mengimpor codemirror / @codemirror/*; dimuat lazy oleh MarkdownEditor.
import { markdown } from '@codemirror/lang-markdown';
import { Compartment, EditorState } from '@codemirror/state';
import { EditorView, keymap } from '@codemirror/view';
import { basicSetup } from 'codemirror';

export interface CmHandle {
  setValue(v: string): void;
  setReadonly(ro: boolean): void;
  focus(): void;
  destroy(): void;
}

export interface CmOptions {
  parent: HTMLElement;
  value: string;
  readonly: boolean;
  label: string;
  onchange(v: string): void;
  onsave(): void;
}

export function createEditor(o: CmOptions): CmHandle {
  const lockCompartment = new Compartment();
  const lock = (ro: boolean) => [EditorState.readOnly.of(ro), EditorView.editable.of(!ro)];
  const view = new EditorView({
    parent: o.parent,
    state: EditorState.create({
      doc: o.value,
      extensions: [
        basicSetup,
        markdown(),
        EditorView.lineWrapping,
        EditorView.contentAttributes.of({ 'aria-label': o.label }),
        keymap.of([{ key: 'Mod-s', preventDefault: true, run: () => (o.onsave(), true) }]),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) o.onchange(u.state.doc.toString());
        }),
        lockCompartment.of(lock(o.readonly)),
      ],
    }),
  });
  return {
    setValue(v) {
      if (v === view.state.doc.toString()) return;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
    },
    setReadonly(ro) {
      view.dispatch({ effects: lockCompartment.reconfigure(lock(ro)) });
    },
    focus: () => view.focus(),
    destroy: () => view.destroy(),
  };
}
