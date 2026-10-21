import { derived, writable, type Readable } from 'svelte/store';
import { joinPath, validateNewFilePath, type Reason } from '../../base';

interface State {
  path: string;
  content: string;
  touched: boolean;
  submitting: boolean;
  serverPathError: string | null;
  exists: boolean;
  formError: string | null;
}

export interface NewFileStore extends Readable<State> {
  // Alasan validasi klien; null sampai field disentuh.
  pathError: Readable<Reason | null>;
  valid: Readable<boolean>;
  dirty: Readable<boolean>;
  setPath(v: string): void;
  setContent(v: string): void;
  touch(): void;
  submitting(v: boolean): void;
  pathFailed(message: string, exists?: boolean): void;
  formFailed(message: string): void;
  clearErrors(): void;
}

export function createNewFileStore(initialDir: string, initialContent = ''): NewFileStore {
  const startPath = initialDir ? joinPath(initialDir, '') : '';
  const s = writable<State>({
    path: startPath,
    content: initialContent,
    touched: false,
    submitting: false,
    serverPathError: null,
    exists: false,
    formError: null,
  });
  const up = (fn: (x: State) => Partial<State>) => s.update((x) => ({ ...x, ...fn(x) }));
  return {
    subscribe: s.subscribe,
    pathError: derived(s, (x) => (x.touched ? validateNewFilePath(x.path) : null)),
    valid: derived(s, (x) => validateNewFilePath(x.path) === null),
    dirty: derived(s, (x) => x.content !== initialContent || (x.path !== '' && x.path !== startPath)),
    setPath: (path) => up(() => ({ path, serverPathError: null, exists: false, touched: true })),
    setContent: (content) => up(() => ({ content })),
    touch: () => up(() => ({ touched: true })),
    submitting: (submitting) => up(() => ({ submitting })),
    pathFailed: (message, exists = false) => up(() => ({ serverPathError: message, exists, formError: null })),
    formFailed: (message) => up(() => ({ formError: message, serverPathError: null, exists: false })),
    clearErrors: () => up(() => ({ serverPathError: null, exists: false, formError: null })),
  };
}
