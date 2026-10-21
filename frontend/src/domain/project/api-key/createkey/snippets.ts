export function resolveMcpUrl(mcpUrl: string | null, origin: string): string {
  return mcpUrl ?? `${origin.replace(/\/+$/, '')}/mcp`;
}

export function shellQuote(s: string): string {
  if (/^[A-Za-z0-9._~:/@%+=,-]+$/.test(s)) return s;
  return `'${s.replaceAll("'", "'\\''")}'`;
}

// Raw key aman di dalam kutip ganda: alfabetnya hanya [A-Za-z0-9_-] (dijamin RAW_KEY_RE).
export function claudeCodeSnippet(name: string, url: string, rawKey: string): string {
  return `claude mcp add --transport http ${shellQuote(name)} ${shellQuote(url)} --header "Authorization: Bearer ${rawKey}"`;
}

export function genericJsonSnippet(name: string, url: string, rawKey: string): string {
  return JSON.stringify({ mcpServers: { [name]: { type: 'http', url, headers: { Authorization: `Bearer ${rawKey}` } } } }, null, 2);
}
