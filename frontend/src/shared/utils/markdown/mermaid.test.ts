import { beforeEach, describe, expect, it, vi } from 'vitest';

const initialize = vi.fn();
const render = vi.fn();
vi.mock('mermaid', () => ({ default: { initialize, render } }));

beforeEach(() => {
  initialize.mockClear();
  render.mockReset();
  vi.resetModules();
});

describe('renderMermaid', () => {
  it('initializes once with strict security and sanitizes svg', async () => {
    render.mockResolvedValue({ svg: '<svg xmlns="http://www.w3.org/2000/svg"><g onclick="alert(1)"><text>A</text></g><script>alert(1)</script></svg>' });
    const { renderMermaid } = await import('./mermaid');
    const r1 = await renderMermaid('graph TD; A-->B', 'm1');
    const r2 = await renderMermaid('graph TD; B-->C', 'm2');
    expect(initialize).toHaveBeenCalledTimes(1);
    expect(initialize).toHaveBeenCalledWith(expect.objectContaining({ startOnLoad: false, securityLevel: 'strict', theme: 'neutral' }));
    expect(r1.ok).toBe(true);
    expect(r2.ok).toBe(true);
    if (r1.ok) {
      expect(r1.svg).toContain('<text>A</text>');
      expect(r1.svg).not.toContain('onclick');
      expect(r1.svg).not.toContain('script');
    }
  });

  it('refuses code over 50000 chars without rendering', async () => {
    const { renderMermaid } = await import('./mermaid');
    const r = await renderMermaid('a'.repeat(50_001), 'big');
    expect(r).toEqual({ ok: false });
    expect(render).not.toHaveBeenCalled();
  });

  it('returns ok:false when mermaid throws', async () => {
    render.mockRejectedValue(new Error('parse'));
    const { renderMermaid } = await import('./mermaid');
    expect(await renderMermaid('bad', 'x')).toEqual({ ok: false });
  });
});
