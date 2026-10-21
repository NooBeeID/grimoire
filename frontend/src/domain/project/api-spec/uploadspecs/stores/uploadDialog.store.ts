import { writable, type Readable } from 'svelte/store';
import { mergeResults, partitionPicked, type LocalResult } from '../model';
import type { ApiSpecFileResult, PickedFile } from '../types';

export type Phase = 'pick' | 'validating' | 'validated' | 'uploading' | 'done';
export interface FormError {
  key: string;
  message?: string;
  traceId?: string;
}
export interface UploadDialogState {
  phase: Phase;
  picked: PickedFile[];
  spec: PickedFile[];
  local: LocalResult[];
  name: string;
  nameError: string | null;
  formError: FormError | null;
  results: ApiSpecFileResult[];
}

const initial = (): UploadDialogState => ({
  phase: 'pick',
  picked: [],
  spec: [],
  local: [],
  name: '',
  nameError: null,
  formError: null,
  results: [],
});

export interface UploadDialogStore extends Readable<UploadDialogState> {
  pick(files: PickedFile[]): void;
  setName(v: string): void;
  setNameError(e: string | null): void;
  startValidate(): void;
  validated(server: ApiSpecFileResult[]): void;
  startUpload(): void;
  uploaded(server: ApiSpecFileResult[]): void;
  fail(f: FormError): void;
  reset(): void;
}

// State dialog per instance; tanpa fetch.
export function createUploadDialogStore(): UploadDialogStore {
  const s = writable<UploadDialogState>(initial());
  const patch = (p: Partial<UploadDialogState>) => s.update((x) => ({ ...x, ...p }));
  return {
    subscribe: s.subscribe,
    pick(files) {
      const { spec, local } = partitionPicked(files);
      s.update((x) => ({ ...x, phase: 'pick', picked: files, spec, local, results: [], formError: null }));
    },
    setName: (name) => patch({ name, nameError: null }),
    setNameError: (nameError) => patch({ nameError }),
    startValidate: () => patch({ phase: 'validating', formError: null }),
    validated: (server) => s.update((x) => ({ ...x, phase: 'validated', results: mergeResults(server, x.local) })),
    startUpload: () => patch({ phase: 'uploading', formError: null }),
    uploaded: (server) => s.update((x) => ({ ...x, phase: 'done', results: mergeResults(server, x.local) })),
    // File tetap terpilih agar bisa dicoba lagi.
    fail: (formError) => s.update((x) => ({ ...x, phase: x.results.length ? 'validated' : 'pick', formError })),
    reset: () => s.set(initial()),
  };
}
