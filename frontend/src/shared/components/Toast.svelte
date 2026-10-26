<script lang="ts">
  import { toast, toasts } from '../stores/toast';
  import { t } from '../stores/i18n';

  const tone = {
    success: 'bg-pill-positive-bg text-pill-positive-ink',
    error: 'bg-pill-negative-bg text-pill-negative-ink',
    info: 'bg-surface text-fg',
  };
</script>

<div role="status" aria-live="polite" class="fixed right-4 bottom-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2">
  {#each $toasts as item (item.id)}
    <div
      role={item.type === 'error' ? 'alert' : undefined}
      class="outlined flex items-start gap-2 rounded-card p-3.5 font-semibold shadow-pop {tone[item.type]}"
    >
      <div class="flex-1 text-sm">
        <p>{item.message}</p>
        {#if item.traceId}<p class="mt-1 text-xs text-muted">{$t('error.trace_id', { id: item.traceId })}</p>{/if}
      </div>
      <button
        type="button"
        aria-label={$t('toast.close')}
        onclick={() => toast.dismiss(item.id)}
        class="rounded-base px-1.5 hover:bg-black/10 focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
      >
        <span aria-hidden="true">&times;</span>
      </button>
    </div>
  {/each}
</div>
