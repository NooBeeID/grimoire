// Helper test: membangun Response SSE dari event mentah.
export const sseFrame = (name: string, data: unknown) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`;

export function sseResponse(frames: string[], opts: { keepOpen?: boolean } = {}): { res: Response; cancelled: () => boolean } {
  const enc = new TextEncoder();
  let cancelled = false;
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      for (const f of frames) c.enqueue(enc.encode(f));
      if (!opts.keepOpen) c.close();
    },
    cancel() {
      cancelled = true;
    },
  });
  return { res: new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }), cancelled: () => cancelled };
}

// Stream yang bisa didorong manual dari tes.
export function controlledStream() {
  const enc = new TextEncoder();
  let ctl!: ReadableStreamDefaultController<Uint8Array>;
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      ctl = c;
    },
  });
  return {
    res: new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }),
    push: (s: string) => ctl.enqueue(enc.encode(s)),
    close: () => ctl.close(),
  };
}
