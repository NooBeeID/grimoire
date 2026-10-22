<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { announceBucket, percentOf, remainingSeconds, truncateMiddle } from '../progress';
  import type { Progress } from '../stores/runsync.store';

  let { progress, now }: { progress: Progress; now: number } = $props();

  const total = $derived(progress.phase === 'listing' ? null : progress.total);
  const done = $derived(progress.phase === 'fetching' || progress.phase === 'waiting' ? progress.done : progress.phase === 'saving' ? progress.total : 0);
  const percent = $derived(progress.phase === 'listing' ? null : progress.phase === 'saving' ? 100 : total === null ? null : percentOf(done, total));
  const path = $derived(progress.phase === 'fetching' || progress.phase === 'waiting' ? progress.path : '');
  const seconds = $derived(progress.phase === 'waiting' ? remainingSeconds(progress.until, now) : 0);
  const fill = $derived(progress.phase === 'waiting' ? 'bg-border' : 'bg-accent');
  const leftLabel = $derived(
    progress.phase === 'listing' || (progress.phase === 'waiting' && total === null)
      ? $t('gitSync.progress.listing')
      : progress.phase === 'saving'
        ? $t('gitSync.progress.saving')
        : $t('gitSync.progress.count', { done, total: total ?? 0 }),
  );
  const valueText = $derived(total === null ? undefined : $t('gitSync.progress.count', { done, total }));
  // sr-only: satu pengumuman saat mulai menunggu, lalu hitung mundur per 5 detik.
  const announce = $derived(
    progress.phase === 'waiting' ? `${$t('gitSync.progress.waitingAnnounce')} ${$t('gitSync.progress.waiting', { seconds: announceBucket(seconds) })}` : '',
  );
</script>

<div class="flex flex-col gap-2" data-testid="sync-progress" aria-busy={progress.phase === 'listing' || undefined}>
  <div class="flex items-baseline justify-between gap-3 text-sm">
    <span class="text-fg tabular-nums" data-testid="sync-progress-count">{leftLabel}</span>
    <span class="text-muted tabular-nums" data-testid="sync-progress-percent">{percent === null ? '–' : $t('gitSync.progress.percent', { percent })}</span>
  </div>
  <div
    class="h-2 w-full overflow-hidden rounded-full bg-raised"
    role="progressbar"
    aria-label={$t('gitSync.progress.label')}
    aria-valuemin={0}
    aria-valuemax={total ?? undefined}
    aria-valuenow={total === null ? undefined : done}
    aria-valuetext={valueText}
    data-testid="sync-progress-bar"
  >
    {#if percent !== null}
      <div class="h-full {fill} transition-[width] duration-120 ease-out motion-reduce:transition-none" style="width: {percent}%" data-testid="sync-progress-fill"></div>
    {/if}
  </div>
  {#if path}
    <code class="block truncate font-mono text-xs text-muted" title={path} data-testid="sync-progress-path">{truncateMiddle(path)}</code>
  {/if}
  {#if progress.phase === 'waiting'}
    <p class="flex items-center gap-2 text-[13px] text-warning" data-testid="sync-waiting">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="shrink-0" data-testid="sync-waiting-icon">
        <circle cx="8" cy="8" r="6" />
        <path d="M8 4.75V8l2 1.5" />
      </svg>
      <span>{$t('gitSync.progress.waiting', { seconds })}</span>
    </p>
  {/if}
  <p class="sr-only" aria-live="polite" data-testid="sync-progress-live">{announce}</p>
</div>
