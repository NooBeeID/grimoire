// Parser SSE minimal (subset spesifikasi WHATWG). Tanpa state global, tanpa dependensi.
export interface SseParser {
  push(chunk: string): void;
  end(): void;
}

export function createSseParser(onEvent: (name: string, data: string) => void): SseParser {
  let buf = '';
  let started = false;
  let name = '';
  let data: string[] = [];
  let hasData = false;

  function line(l: string) {
    if (l === '') {
      if (hasData) onEvent(name || 'message', data.join('\n'));
      name = '';
      data = [];
      hasData = false;
      return;
    }
    if (l.startsWith(':')) return;
    const i = l.indexOf(':');
    const field = i === -1 ? l : l.slice(0, i);
    let value = i === -1 ? '' : l.slice(i + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'event') name = value;
    else if (field === 'data') {
      data.push(value);
      hasData = true;
    }
  }

  function drain(final: boolean) {
    let pos = 0;
    for (;;) {
      let end = -1;
      let skip = 1;
      for (let j = pos; j < buf.length; j++) {
        const c = buf[j];
        if (c === '\n') {
          end = j;
          break;
        }
        if (c === '\r') {
          if (j + 1 >= buf.length && !final) break; // tahan: mungkin \r\n
          end = j;
          if (buf[j + 1] === '\n') skip = 2;
          break;
        }
      }
      if (end === -1) break;
      line(buf.slice(pos, end));
      pos = end + skip;
    }
    buf = buf.slice(pos);
  }

  return {
    push(chunk) {
      if (!started) {
        started = true;
        if (chunk.charCodeAt(0) === 0xfeff) chunk = chunk.slice(1);
      }
      buf += chunk;
      drain(false);
    },
    end() {
      drain(true);
      buf = '';
      name = '';
      data = [];
      hasData = false;
    },
  };
}
