import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mockFileResult } from '../mocks/results';
import { createUploadDialogStore } from './uploadDialog.store';

const pf = (path: string) => ({ file: new File(['x'], path), path });

describe('uploadDialog store', () => {
  it('walks pick -> validating -> validated -> uploading -> done and merges local results', () => {
    const s = createUploadDialogStore();
    s.pick([pf('a.yaml'), pf('README.md')]);
    expect(get(s)).toMatchObject({ phase: 'pick' });
    expect(get(s).spec).toHaveLength(1);
    s.startValidate();
    expect(get(s).phase).toBe('validating');
    s.validated([mockFileResult({ status: 'valid' })]);
    expect(get(s).results.map((r) => r.status)).toEqual(['valid', 'skipped']);
    s.startUpload();
    s.uploaded([mockFileResult()]);
    expect(get(s).phase).toBe('done');
  });
  it('picking after done discards old results; fail keeps picked files', () => {
    const s = createUploadDialogStore();
    s.pick([pf('a.yaml')]);
    s.startUpload();
    s.fail({ key: 'apispec.upload.error.tooLarge' });
    expect(get(s).picked).toHaveLength(1);
    expect(get(s).phase).toBe('pick');
    s.uploaded([mockFileResult()]);
    s.pick([pf('b.yaml')]);
    expect(get(s).results).toEqual([]);
  });
});
