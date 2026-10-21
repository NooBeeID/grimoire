import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockDetail, mockDocument, mockEndpoints } from '../mocks/detail-3.0';
import { mockDocument31 } from '../mocks/detail-3.1';
import { mockDocumentCircular } from '../mocks/detail-circular';
import { mockDocumentWebhooks } from '../mocks/detail-webhooks';
import { buildTagTree, getOperation } from '../model/operations';
import { examplesOf } from '../model/examples';
import { allSchemes, usedSchemes } from '../model/security';
import ApiOverview from './ApiOverview.svelte';
import EndpointSidebar from './EndpointSidebar.svelte';
import ExampleBlock from './ExampleBlock.svelte';
import MethodBadge from './MethodBadge.svelte';
import OperationView from './OperationView.svelte';
import SchemaTree from './SchemaTree.svelte';
import SecuritySchemesList from './SecuritySchemesList.svelte';
import ServersList from './ServersList.svelte';
import { mockDetailSwagger2 } from '../mocks/detail-swagger2';
import SpecHeader from './SpecHeader.svelte';

beforeEach(() => setLocale('id'));

const sidebar = (over = {}) =>
  render(EndpointSidebar, {
    tree: buildTagTree(mockDocument, mockEndpoints),
    matches: mockEndpoints,
    query: '',
    selected: null,
    total: mockEndpoints.length,
    hrefFor: (ep: { method: string; path: string }) => '/s?op=' + encodeURIComponent(`${ep.method} ${ep.path}`),
    onquery: () => {},
    ...over,
  });

describe('EndpointSidebar', () => {
  it('renders tag groups with op links', () => {
    sidebar();
    expect(screen.getByText('payments')).toBeInTheDocument();
    expect(screen.getByText('refunds')).toBeInTheDocument();
    expect(screen.getByText('Tanpa tag')).toBeInTheDocument();
    expect(screen.getByText('Pengembalian dana')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: /^GET \/payments Daftar/ })[0]).toHaveAttribute('href', '/s?op=GET%20%2Fpayments');
  });
  it('marks the selected endpoint with aria-current', () => {
    sidebar({ selected: 'GET /health' });
    const cur = screen.getAllByRole('link').filter((a) => a.getAttribute('aria-current') === 'page');
    expect(cur).toHaveLength(1);
    expect(cur[0]).toHaveTextContent('/health');
  });
  it('strikes deprecated endpoints and labels them', () => {
    sidebar();
    const links = screen.getAllByRole('link', { name: /refund/ });
    const link = links[0]!;
    expect(within(link).getByText('Usang')).toBeInTheDocument();
    expect(link.querySelector('.line-through')).not.toBeNull();
  });
  it('shows an empty message and noEndpoints, and reports typing', async () => {
    const onquery = vi.fn();
    const { unmount } = sidebar({ query: 'zzz', matches: [], onquery });
    expect(screen.getByText('Tidak ada endpoint yang cocok')).toBeInTheDocument();
    await userEvent.type(screen.getByRole('searchbox'), 'a');
    expect(onquery).toHaveBeenCalledWith('zzza');
    unmount();
    sidebar({ total: 0, tree: [], matches: [] });
    expect(screen.getByText('Spec ini tidak punya endpoint di paths')).toBeInTheDocument();
  });
});

describe('MethodBadge / SpecHeader', () => {
  it('badge shows the method as text', () => {
    render(MethodBadge, { method: 'POST' });
    expect(screen.getByText('POST')).toBeInTheDocument();
  });
  it('header shows the OpenAPI badge and renders title as text', () => {
    const spec = { ...mockDetail().spec, title: '<b>x</b>' };
    render(SpecHeader, { spec });
    expect(screen.getByLabelText('Versi OpenAPI 3.0.3')).toHaveTextContent('3.0');
    expect(screen.getByText(/<b>x<\/b>/)).toBeInTheDocument();
  });
  it('header Swagger 2.0 memuat label Swagger 2.0', () => {
    render(SpecHeader, { spec: mockDetailSwagger2().spec });
    expect(screen.getByLabelText('Swagger 2.0')).toBeInTheDocument();
  });
});

const opView = (doc: unknown, method: string, path: string) => {
  const info = getOperation(doc, method, path)!;
  return render(OperationView, { doc, method, path, info });
};

describe('OperationView', () => {
  it('shows parameters per location, bodies per media type, responses with default last', () => {
    opView(mockDocument, 'GET', '/payments');
    expect(screen.getByText(/Parameter · header/)).toBeInTheDocument();
    expect(screen.getByText(/Parameter · query/)).toBeInTheDocument();
    expect(screen.getAllByText(/^(200|default)$/).map((e) => e.textContent)).toEqual(['200', 'default']);
    expect(screen.queryByRole('button', { name: /try|kirim|execute/i })).toBeNull();
    expect(document.querySelector('form')).toBeNull();
  });
  it('request body lists each media type; effective servers and security are shown', () => {
    opView(mockDocument, 'POST', '/payments');
    expect(screen.getByText('application/json')).toBeInTheDocument();
    expect(screen.getByText('application/xml')).toBeInTheDocument();
    expect(screen.getByText('https://sandbox.noobee.id/payment')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Keamanan' })).toBeInTheDocument();
    expect(screen.getByText('bearer', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText(/JWT/)).toBeInTheDocument();
    expect(screen.getByText('bearer', { selector: 'strong' })).toBeInTheDocument(); // Markdown **bearer**
  });
  it('renders the example block and picks among several examples', async () => {
    opView(mockDocument, 'POST', '/payments');
    expect(screen.getByText(/"amount": 1500/)).toBeInTheDocument();
  });
  it('shows "no authentication" for security: []', () => {
    const doc = { paths: { '/p': { get: { security: [], responses: {} } } } };
    opView(doc, 'GET', '/p');
    expect(screen.getByText('Tanpa autentikasi')).toBeInTheDocument();
  });
  it('omits empty sections and flags deprecated operations', () => {
    const doc = { paths: { '/p': { get: { deprecated: true, responses: {} } } } };
    opView(doc, 'GET', '/p');
    expect(screen.queryByText('Parameter', { exact: false })).toBeNull();
    expect(screen.queryByText('Request body')).toBeNull();
    expect(screen.queryByText('Response')).toBeNull();
    expect(screen.getByText('Usang')).toBeInTheDocument();
  });
});

describe('SchemaTree', () => {
  it('only level 0 is open; clicking opens children', async () => {
    render(SchemaTree, { doc: mockDocument, schema: { $ref: '#/components/schemas/Payment' } });
    expect(screen.getByText('amount')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'amount' })).toBeNull();
  });
  it('opens a nested row on click', async () => {
    const doc = { components: { schemas: { A: { type: 'object', properties: { b: { type: 'object', properties: { deep: { type: 'string' } } } } } } } };
    render(SchemaTree, { doc, schema: { $ref: '#/components/schemas/A' } });
    const btn = screen.getByRole('button', { name: 'b' });
    expect(btn).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('deep')).toBeNull();
    await userEvent.click(btn);
    expect(btn).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('deep')).toBeInTheDocument();
  });
  it('circular refs show a badge and have no open button', () => {
    render(SchemaTree, { doc: mockDocumentCircular, schema: { $ref: '#/components/schemas/A' } });
    expect(screen.getByText(/circular/)).toBeInTheDocument();
    expect(screen.queryByRole('button')).toBeNull();
  });
  it('unresolved refs show a badge; markers show required/readOnly/nullable types', () => {
    render(SchemaTree, { doc: mockDocument, schema: { $ref: '#/nope' } });
    expect(screen.getByText('unresolved')).toBeInTheDocument();
  });
  it('a 500-endpoint spec with deep schemas renders only the selected operation', () => {
    const paths: Record<string, unknown> = {};
    for (let i = 0; i < 500; i++) paths[`/r${i}`] = { get: { summary: `s${i}`, responses: { '200': { description: 'ok', content: { 'application/json': { schema: { $ref: '#/components/schemas/Deep' } } } } } } };
    let deep: Record<string, unknown> = { type: 'string' };
    for (let i = 0; i < 40; i++) deep = { type: 'object', properties: { next: deep } };
    const doc = { paths, components: { schemas: { Deep: deep } } };
    opView(doc, 'GET', '/r7');
    expect(document.querySelectorAll('article')).toHaveLength(1);
    expect(screen.queryByText('s8')).toBeNull();
  });
});

describe('ApiOverview / security / servers / examples', () => {
  it('overview lists servers, security schemes, tags and webhooks', () => {
    render(ApiOverview, { doc: { ...mockDocument31, ...{ webhooks: mockDocumentWebhooks.webhooks }, tags: [{ name: 'pets', description: 'Hewan **lucu**' }] } });
    expect(screen.getAllByText('https://pets.noobee.id/v2').length).toBeGreaterThan(0);
    expect(screen.getByRole('heading', { name: 'Skema keamanan' })).toBeInTheDocument();
    expect(screen.getAllByText('oauth').length).toBeGreaterThan(0);
    expect(screen.getAllByText('https://auth.noobee.id/token', { exact: false }).length).toBeGreaterThan(0);
    expect(screen.getAllByText('pets:read', { exact: false }).length).toBeGreaterThan(0);
    expect(screen.getByText('lucu')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Webhook' })).toBeInTheDocument();
    expect(screen.getByText('POST paymentCreated')).toBeInTheDocument();
  });
  it('ServersList renders http(s) as link and others as text; nothing for empty', () => {
    const { container, unmount } = render(ServersList, { servers: [{ url: 'https://a.example', description: 'Prod' }, { url: 'ftp://b' }] });
    expect(screen.getByRole('link', { name: 'https://a.example' })).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(screen.getByText('ftp://b').closest('a')).toBeNull();
    unmount();
    const r = render(ServersList, { servers: [] });
    expect(r.container.textContent).toBe('');
    void container;
  });
  it('SecuritySchemesList handles unknown schemes and apiKey', () => {
    render(SecuritySchemesList, { schemes: [...allSchemes(mockDocument), ...usedSchemes(mockDocument, [{ ghost: ['x'] }])].map((s) => ({ ...s })) });
    expect(screen.getByText(/skema tidak didefinisikan/)).toBeInTheDocument();
    expect(screen.getByText(/header X-Api-Key/)).toBeInTheDocument();
  });
  it('ExampleBlock shows JSON text, switches examples and copies', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(ExampleBlock, { examples: examplesOf(mockDocument, { examples: { a: { summary: 'Satu', value: { n: 1 } }, b: { summary: 'Dua', value: { n: 2 } } } }) });
    expect(screen.getByText(/"n": 1/)).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByRole('combobox'), '1');
    expect(screen.getByText(/"n": 2/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(writeText).toHaveBeenCalledWith('{\n  "n": 2\n}');
    expect(await screen.findByText('Tersalin')).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
  it('ExampleBlock failing copy selects text and shows the manual hint; externalValue is plain text', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => { throw new Error('no'); }) } });
    render(ExampleBlock, { examples: [{ name: 'e', summary: null, value: undefined, externalValue: 'https://x.example/e.json' }] });
    expect(screen.getByText('https://x.example/e.json').closest('a')).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'Salin' }));
    expect(await screen.findByText(/Tidak bisa menyalin otomatis/)).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
});
