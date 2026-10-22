<script lang="ts">
  import { untrack } from 'svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { session } from '../../../../../shared/stores/session';
  import GitTokensTable from '../components/GitTokensTable.svelte';
  import ScopeBlock from '../components/ScopeBlock.svelte';
  import { listCredentials } from '../services/listcredentials.service';
  import { gitCredentials } from '../stores/gitCredentials.store';
  import { buildRows, type SaveRequest, type TokenRow } from '../types';

  let {
    onsave,
    ondelete,
    onadd,
    onready,
  }: {
    onsave?: (row: TokenRow, gitlabHosts: string[]) => void;
    ondelete?: (row: TokenRow) => void;
    onadd?: (req: SaveRequest) => void;
    onready?: () => void;
  } = $props();

  let ver = $state(0);
  $effect(() => gitCredentials.version.subscribe((v) => (ver = v)));
  const userId = $derived($session.identity?.userId ?? null);
  let ctrl: AbortController | null = null;
  let readied = false;

  async function load(id: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    gitCredentials.begin(id);
    try {
      const r = await listCredentials(c.signal);
      if (!c.signal.aborted) {
        gitCredentials.resolve(id, r);
        if (!readied) {
          readied = true;
          onready?.();
        }
      }
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      gitCredentials.fail(id, isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
    }
  }

  $effect(() => {
    const id = userId;
    void ver;
    if (!id) {
      gitCredentials.reset();
      return;
    }
    void untrack(() => load(id));
    return () => ctrl?.abort();
  });

  const data = $derived(
    $gitCredentials.status === 'ready' ? $gitCredentials.data : $gitCredentials.status === 'loading' || $gitCredentials.status === 'error' ? $gitCredentials.previous : null,
  );
  const loading = $derived($gitCredentials.status === 'idle' || ($gitCredentials.status === 'loading' && data === null));
  const error = $derived($gitCredentials.status === 'error' && data === null ? $gitCredentials.error : null);
  const rows = $derived(buildRows(data?.items ?? []));
  const enabled = $derived(data?.enabled ?? true);
  const hosts = $derived(data?.allowed_gitlab_hosts ?? []);
  // Baris yang sudah tampil (tersimpan atau placeholder) punya tombol Simpan sendiri; tautan tambah hanya untuk host lain.
  const addable = $derived(hosts.filter((h) => !rows.some((r) => r.provider === 'gitlab' && r.host === h)));
  const disabledParts = $derived($t('gitCredential.disabled', { key: '\u0001' }).split('\u0001'));
</script>

<div class="flex flex-col gap-3">
  <div>
    <h2 id="token-git-title" tabindex="-1" class="text-base font-semibold focus:outline-none">{$t('gitCredential.title')}</h2>
    <p class="mt-1 text-sm text-muted">{$t('gitCredential.intro')}</p>
  </div>

  {#if !enabled}
    <p class="rounded-card border border-border bg-raised p-4 text-sm text-muted">
      {disabledParts[0]}<code class="font-mono text-[13px]">GIT_CREDENTIAL_KEY</code>{disabledParts[1] ?? ''}
    </p>
  {/if}

  <GitTokensTable
    {rows}
    {loading}
    {error}
    onretry={() => gitCredentials.invalidate()}
    canEdit={enabled}
    onsave={onsave ? (r) => onsave?.(r, hosts) : undefined}
    {ondelete}
  />

  {#if $gitCredentials.status === 'error' && data}
    <p role="alert" class="text-sm text-danger">{$gitCredentials.error.message}</p>
  {/if}

  {#if enabled && onadd && addable.length > 0}
    <button
      type="button"
      class="w-fit text-sm text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
      onclick={() => onadd?.({ provider: 'gitlab', host: null, gitlabHosts: addable })}
    >
      {$t('gitCredential.addGitlabHost')}
    </button>
  {/if}

  <ScopeBlock />
</div>
