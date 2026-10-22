import { describe, expect, it } from 'vitest';
import { createSseParser } from './sse';

function run(chunks: string[], end = false) {
  const out: [string, string][] = [];
  const p = createSseParser((n, d) => out.push([n, d]));
  for (const c of chunks) p.push(c);
  if (end) p.end();
  return out;
}
const STREAM = 'event: start\ndata: {"total":3}\n\nevent: progress\ndata: {"done":1,"total":3,"path":"a.md"}\n\n';
const EXPECTED = [
  ['start', '{"total":3}'],
  ['progress', '{"done":1,"total":3,"path":"a.md"}'],
];

describe('createSseParser', () => {
  it('parses a whole event in one chunk', () => {
    expect(run(['event: start\ndata: {"total":3}\n\n'])).toEqual([['start', '{"total":3}']]);
  });
  it('is chunk-boundary independent', () => {
    for (let i = 1; i < STREAM.length; i++) expect(run([STREAM.slice(0, i), STREAM.slice(i)])).toEqual(EXPECTED);
    expect(run(STREAM.split(''))).toEqual(EXPECTED);
  });
  it('handles CRLF and CR split across chunks without empty duplicates', () => {
    expect(run(['event: a\r', '\ndata: 1\r', '\n\r', '\n'])).toEqual([['a', '1']]);
    expect(run(['event: a\rdata: 1\r\rdata: 2'])).toEqual([['a', '1']]);
  });
  it('ignores comments and data-less events', () => {
    expect(run([': ping\n\nevent: x\n\n: ping\n\n'])).toEqual([]);
  });
  it('joins multiple data lines, strips BOM, keeps order, drops tail on end()', () => {
    expect(run(['﻿data: a\ndata: b\n\n'])).toEqual([['message', 'a\nb']]);
    expect(run(['event: a\ndata: 1\n\nevent: b\ndata: 2\n\n'])).toEqual([['a', '1'], ['b', '2']]);
    expect(run(['event: a\ndata: 1\n'], true)).toEqual([]);
  });
});
