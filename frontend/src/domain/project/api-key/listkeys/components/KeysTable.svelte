<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Table from '../../../../../shared/components/Table.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import { displayPrefix, type ApiKey } from '../types';
  import { createdLine, revokedLine } from './format';
  import KeyStatusBadge from './KeyStatusBadge.svelte';

  let {
    items,
    loading = false,
    error = null,
    onretry,
    onrevoke,
    auditHref,
  }: {
    items: ApiKey[];
    loading?: boolean;
    error?: ApiError | null;
    onretry?: () => void;
    onrevoke: (key: ApiKey) => void;
    auditHref?: (key: ApiKey) => string;
  } = $props();

  const columns = $derived([
    { key: 'name', header: $t('project.apikey.col.name') },
    { key: 'prefix', header: $t('project.apikey.col.prefix') },
    { key: 'created', header: $t('project.apikey.col.created') },
    { key: 'lastUsed', header: $t('project.apikey.col.lastUsed') },
    { key: 'status', header: $t('project.apikey.col.status') },
    { key: 'actions', header: $t('project.apikey.col.actions'), class: 'text-right' },
  ]);
</script>

{#snippet cell(k: ApiKey, col: { key: string })}
  {#if col.key === 'name'}
    <span class="break-words">{k.name}</span>
  {:else if col.key === 'prefix'}
    <code class="font-mono text-[13px]">{displayPrefix(k.prefix)}</code>
  {:else if col.key === 'created'}
    <time datetime={k.created_at}>{createdLine(k, $locale)}</time>
  {:else if col.key === 'lastUsed'}
    {#if k.last_used_at}
      <time datetime={k.last_used_at}>{formatDateTime(k.last_used_at, $locale)}</time>
    {:else}
      <span class="text-muted" aria-label={$t('project.apikey.neverUsed')}>-</span>
    {/if}
  {:else if col.key === 'status'}
    <KeyStatusBadge status={k.status} />
    {#if k.status === 'revoked'}<span class="mt-1 block text-xs text-muted">{revokedLine(k, $locale)}</span>{/if}
  {:else}
    <div class="flex flex-wrap items-center justify-end gap-1">
      {#if auditHref}
        <a
          href={auditHref(k)}
          aria-label={$t('project.apikey.auditLinkAria', { name: k.name })}
          class="rounded-base px-2 py-1 text-[13px] text-accent-ink underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
          >{$t('project.apikey.auditLink')}</a
        >
      {/if}
      {#if k.status === 'active'}
        <Button size="sm" variant="ghost" ariaLabel={$t('project.apikey.revoke.aria', { name: k.name })} onclick={() => onrevoke(k)}>
          {$t('project.apikey.revoke.open')}
        </Button>
      {/if}
    </div>
  {/if}
{/snippet}

<div class="overflow-x-auto">
  <Table
    {columns}
    rows={items}
    rowKey={(k) => k.id}
    {cell}
    {loading}
    {error}
    {onretry}
    caption={$t('project.apikey.title')}
    rowClass={(k) => (k.status === 'revoked' ? 'opacity-60' : '')}
    rowAttrs={(k) => ({ 'data-key-id': k.id, 'data-status': k.status })}
  />
</div>
