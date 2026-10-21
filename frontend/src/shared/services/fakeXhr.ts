// Pengganti XMLHttpRequest untuk test (jsdom tidak mengirim jaringan sungguhan).
export class FakeXhr {
  static last: FakeXhr | null = null;
  method = '';
  url = '';
  headers: Record<string, string> = {};
  withCredentials = false;
  status = 0;
  responseText = '';
  body: unknown = null;
  aborted = false;
  respHeaders: Record<string, string> = {};
  upload: { onprogress: ((e: { lengthComputable: boolean; loaded: number; total: number }) => void) | null; onload: (() => void) | null } = {
    onprogress: null,
    onload: null,
  };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  constructor() {
    FakeXhr.last = this;
  }
  open(m: string, u: string) {
    this.method = m;
    this.url = u;
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v;
  }
  getResponseHeader(k: string) {
    return this.respHeaders[k] ?? null;
  }
  send(b: unknown) {
    this.body = b;
  }
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
  respond(status: number, text: string) {
    this.status = status;
    this.responseText = text;
    this.onload?.();
  }
}

// XHR yang otomatis menjawab dari handler bergaya fetch (untuk test yang tidak peduli progres).
export function xhrFrom(handler: (url: string, body: unknown) => Promise<Response> | Response) {
  const calls: { url: string; body: unknown }[] = [];
  class AutoXhr extends FakeXhr {
    override send(b: unknown) {
      super.send(b);
      calls.push({ url: this.url, body: b });
      void Promise.resolve(handler(this.url, b)).then(async (r) => {
        if (this.aborted) return;
        this.upload.onload?.();
        this.respond(r.status, await r.text());
      });
    }
  }
  return { Xhr: AutoXhr, calls };
}
