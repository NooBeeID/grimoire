import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockCreated } from '../mocks/project';
import { createProject, validateCreate } from './createproject.service';

afterEach(() => vi.unstubAllGlobals());

describe('validateCreate', () => {
  it('name length', () => {
    expect(validateCreate({ name: 'A', description: '' }).name).toBe('project.validation.nameLength');
    expect(validateCreate({ name: 'AB', description: '' })).toEqual({});
    expect(validateCreate({ name: '\u{1F44D}'.repeat(100), description: '' })).toEqual({});
    expect(validateCreate({ name: '\u{1F44D}'.repeat(101), description: '' }).name).toBe('project.validation.nameLength');
  });
  it('control chars', () => {
    expect(validateCreate({ name: 'a\nb', description: '' }).name).toBe('project.validation.nameControl');
    expect(validateCreate({ name: 'ab', description: 'a\u0000' }).description).toBe('project.validation.descriptionControl');
  });
  it('description limits, newline allowed', () => {
    expect(validateCreate({ name: 'ab', description: 'x'.repeat(1001) }).description).toBe('project.validation.descriptionLength');
    expect(validateCreate({ name: 'ab', description: 'a\nb' })).toEqual({});
  });
});

describe('createProject', () => {
  it('POSTs trimmed body with CSRF header and validates response', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockCreated()), { status: 201 }));
    vi.stubGlobal('fetch', f);
    const p = await createProject({ name: '  Payments ', description: ' d ' });
    expect(p.name).toBe('Payments');
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('/api/projects');
    expect(init!.method).toBe('POST');
    expect((init!.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'Payments', description: 'd' });
  });
  it('rejects malformed response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ id: 'x' }), { status: 201 })));
    await expect(createProject({ name: 'ab', description: '' })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
