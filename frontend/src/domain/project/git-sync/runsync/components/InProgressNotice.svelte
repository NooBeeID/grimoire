<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { loading, onreload }: { loading: boolean; onreload: () => void } = $props();
</script>

<div role="status" class="flex flex-col gap-3 rounded-card bg-pill-neutral-bg p-3 sm:flex-row sm:items-center sm:justify-between" data-testid="sync-in-progress">
  <div class="flex items-start gap-2">
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="mt-0.5 shrink-0 text-pill-neutral-ink">
      <circle cx="8" cy="8" r="6" />
      <path d="M8 4.75V8l2 1.5" />
    </svg>
    <div class="flex flex-col gap-1">
      <p class="text-sm font-medium text-pill-neutral-ink">{$t('gitSync.inProgress.title')}</p>
      <p class="text-[13px] text-muted">{$t('gitSync.inProgress.body')}</p>
    </div>
  </div>
  <div class="shrink-0">
    <Button variant="secondary" size="sm" disabled={loading} onclick={onreload}>{$t('gitSync.inProgress.reload')}</Button>
  </div>
</div>
