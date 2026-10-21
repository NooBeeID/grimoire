import { describe, expect, it } from 'vitest';
import { claudeCodeSnippet, genericJsonSnippet, resolveMcpUrl, shellQuote } from './snippets';

const K = 'gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0';

describe('snippets', () => {
  it('resolveMcpUrl', () => {
    expect(resolveMcpUrl('https://grimoire.tim.id/mcp', 'http://localhost:8080')).toBe('https://grimoire.tim.id/mcp');
    expect(resolveMcpUrl(null, 'http://localhost:8080')).toBe('http://localhost:8080/mcp');
    expect(resolveMcpUrl(null, 'http://localhost:8080/')).toBe('http://localhost:8080/mcp');
  });
  it('claudeCodeSnippet', () => {
    expect(claudeCodeSnippet('claude-code-budi', 'https://g.tim.id/mcp', K)).toBe(
      `claude mcp add --transport http claude-code-budi https://g.tim.id/mcp --header "Authorization: Bearer ${K}"`,
    );
  });
  it('quotes unsafe names', () => {
    expect(shellQuote("budi's key")).toBe("'budi'\\''s key'");
    expect(shellQuote('a;rm -rf ~')).toBe("'a;rm -rf ~'");
  });
  it('genericJsonSnippet is valid JSON even with quotes in name', () => {
    const name = 'a"b';
    expect(JSON.parse(genericJsonSnippet(name, 'https://g/mcp', K))).toEqual({
      mcpServers: { [name]: { type: 'http', url: 'https://g/mcp', headers: { Authorization: `Bearer ${K}` } } },
    });
  });
});
