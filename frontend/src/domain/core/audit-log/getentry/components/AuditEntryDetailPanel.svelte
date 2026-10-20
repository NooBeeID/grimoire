<script lang="ts">
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { DetailState } from '../types';
  import { formatDateTime, formatJSON } from './format';

  let { detail, onclose, onretry }: { detail: DetailState; onclose: () => void; onretry: () => void } = $props();

  let heading: HTMLHeadingElement | undefined = $state();
  const currentId = $derived(detail.status === 'idle' ? null : detail.id);

  $effect(() => {
    void currentId;
    heading?.focus();
  });

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose();
  }

  const statusClass = { ok: 'text-fg', error: 'text-danger', denied: 'text-danger' } as const;
</script>

<svelte:window {onkeydown} />

<aside
  aria-labelledby="audit-detail-title"
  aria-busy={detail.status === 'loading'}
  class="min-w-0 rounded-base border border-border bg-surface p-4"
>
  <div class="flex items-center justify-between gap-2">
    <h2 id="audit-detail-title" tabindex="-1" bind:this={heading} class="text-lg font-semibold">
      {$t('auditLog.detail.title')}
    </h2>
    <button
      type="button"
      aria-label={$t('auditLog.detail.close')}
      onclick={onclose}
      class="rounded-base px-2 py-1 hover:bg-black/5 focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none"
    >
      <span aria-hidden="true">×</span>
    </button>
  </div>

  {#if detail.status === 'loading'}
    <div class="p-6 text-center"><Spinner /></div>
  {:else if detail.status === 'error'}
    {#if detail.error === 'notFound'}
      <EmptyState title={$t('auditLog.detail.notFound')} description={$t('auditLog.detail.notFoundHint')} />
    {:else if detail.error === 'forbidden'}
      <ErrorState error={detail.apiError} />
    {:else}
      <ErrorState error={detail.apiError} {onretry} />
    {/if}
  {:else if detail.status === 'ready'}
    {@const e = detail.entry}
    <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
      <dt class="text-muted">{$t('auditLog.columns.time')}</dt>
      <dd title={e.occurred_at}>{formatDateTime(e.occurred_at, $locale)}</dd>
      <dt class="text-muted">{$t('auditLog.columns.actor')}</dt>
      <dd class="break-words">
        {$t(`auditLog.actorType.${e.actor_type}`)} · {e.actor_label}
        {#if e.actor_id}<div class="font-mono text-xs text-muted">{e.actor_id}</div>{/if}
      </dd>
      <dt class="text-muted">{$t('auditLog.columns.project')}</dt>
      <dd class="break-words">
        {e.project_label ?? '-'}
        {#if e.project_id}<div class="font-mono text-xs text-muted">{e.project_id}</div>{/if}
      </dd>
      <dt class="text-muted">{$t('auditLog.columns.action')}</dt>
      <dd class="font-mono break-all">{e.action}</dd>
      <dt class="text-muted">{$t('auditLog.columns.target')}</dt>
      <dd class="break-all">{e.target_type ?? '-'}{e.target ? ` ${e.target}` : ''}</dd>
      <dt class="text-muted">{$t('auditLog.columns.status')}</dt>
      <dd class="font-medium {statusClass[e.status]}">{$t(`auditLog.status.${e.status}`)}</dd>
    </dl>

    <section class="mt-4">
      <h3 class="text-sm font-semibold">{$t('auditLog.detail.args')}</h3>
      <pre class="mt-1 overflow-auto rounded-base bg-black/5 p-2 text-xs break-words whitespace-pre-wrap">{formatJSON(e.args)}</pre>
      <p class="mt-1 text-xs text-muted">{$t('auditLog.detail.sanitizedHint')}</p>
    </section>
    <section class="mt-4">
      <h3 class="text-sm font-semibold">{$t('auditLog.detail.result')}</h3>
      <pre class="mt-1 overflow-auto rounded-base bg-black/5 p-2 text-xs break-words whitespace-pre-wrap">{formatJSON(e.result)}</pre>
    </section>
    <dl class="mt-4 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
      <dt class="text-muted">{$t('auditLog.detail.ip')}</dt>
      <dd class="font-mono break-all">{e.ip ?? '-'}</dd>
      <dt class="text-muted">{$t('auditLog.detail.userAgent')}</dt>
      <dd class="break-all">{e.user_agent ?? '-'}</dd>
      <dt class="text-muted">{$t('auditLog.detail.traceId')}</dt>
      <dd class="font-mono break-all">{e.trace_id || '-'}</dd>
    </dl>
  {/if}
</aside>
