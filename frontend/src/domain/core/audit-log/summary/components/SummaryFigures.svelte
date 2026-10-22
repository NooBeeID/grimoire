<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { UsageSummary } from '../types';
  import AgentChip from './AgentChip.svelte';
  import { formatCount, formatPercent } from './format';

  let {
    status,
    data,
    error,
    refreshing,
    onretry,
    onreset,
  }: {
    status: 'idle' | 'loading' | 'ready' | 'error';
    data: UsageSummary | null;
    error: ApiError | null;
    refreshing: boolean;
    onretry: () => void;
    onreset: () => void;
  } = $props();

  const loading = $derived(status === 'idle' || status === 'loading');
  const retryable = $derived(!!error && error.status !== 400 && error.status !== 403);
</script>

<div class="rounded-card border border-border bg-surface" aria-busy={loading || refreshing}>
  {#if status === 'error' && error}
    <div role="alert" class="flex flex-wrap items-center gap-x-4 gap-y-2 p-4 sm:p-6">
      <p class="text-sm">{$t('dashboard.loadFailed')}</p>
      {#if error.traceId}<p class="font-mono text-xs text-muted">{error.traceId}</p>{/if}
      {#if retryable}
        <Button variant="secondary" size="sm" onclick={onretry}>{$t('common.retry')}</Button>
      {:else if error.status === 400}
        <Button variant="secondary" size="sm" onclick={onreset}>{$t('dashboard.resetFilter')}</Button>
      {/if}
    </div>
  {:else}
    <dl class="m-0 grid grid-cols-2 divide-x divide-rule">
      <div class="flex flex-col gap-1 p-4 sm:p-6">
        <dt class="flex items-center gap-2 text-sm text-muted"><AgentChip />{$t('dashboard.figures.calls')}</dt>
        <dd class="m-0 text-[28px] leading-9 tabular-nums">
          {#if loading || !data}<span class="block h-9 w-24 bg-raised" aria-hidden="true"></span>{:else}{formatCount(data.total_calls, $locale)}{/if}
        </dd>
      </div>
      <div class="flex flex-col gap-1 p-4 sm:p-6">
        <dt class="flex h-6 items-center text-sm text-muted">{$t('dashboard.figures.errorRate')}</dt>
        <dd class="m-0 flex items-baseline gap-2 text-[28px] leading-9 tabular-nums">
          {#if loading || !data}
            <span class="block h-9 w-24 bg-raised" aria-hidden="true"></span>
          {:else}
            {formatPercent(data.error_rate, $locale)}
            <span class="text-sm text-muted">{$t('dashboard.figures.errors', { n: formatCount(data.total_errors, $locale) })}</span>
          {/if}
        </dd>
      </div>
    </dl>
  {/if}
</div>
