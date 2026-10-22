<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import { errorText } from '../reason';
  import type { GitSyncConfig, Kind } from '../types';
  import RepoOriginChip from './RepoOriginChip.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let { config, kind }: { config: GitSyncConfig | null; kind: Kind } = $props();

  const failure = $derived(
    config?.last_status === 'failed' && config.last_result ? errorText($t, config.last_result.error_code, config.last_result.error_message) : null,
  );
  const s = $derived(config?.last_result?.summary ?? null);
</script>

<div class="flex flex-col gap-2" data-testid="config-summary">
  {#if config === null}
    <p class="text-sm text-muted">{$t('gitSync.config.neverSynced', { kind: $t(`gitSync.kind.${kind}`) })}</p>
  {:else}
    <RepoOriginChip host={config.host} repoPath={config.repo_path} branch={config.branch} folder={config.folder} />
    {#if config.last_synced_at === null}
      <p class="text-sm text-muted">{$t('gitSync.config.neverSynced', { kind: $t(`gitSync.kind.${kind}`) })}</p>
    {:else}
      <p class="flex flex-wrap items-center gap-2 text-sm">
        <span class="text-muted">
          {$t('gitSync.config.lastSync', { time: formatDateTime(config.last_synced_at, $locale), label: config.last_synced_by?.label ?? '–' })}
        </span>
        {#if config.last_status}<StatusBadge status={config.last_status} />{/if}
      </p>
      {#if failure}<p class="text-sm text-danger">{failure}</p>{/if}
      {#if s}
        <p class="text-[13px] tabular-nums text-muted">
          {s.created} {$t('gitSync.summary.created')} · {s.updated} {$t('gitSync.summary.updated')} · {s.unchanged} {$t('gitSync.summary.unchanged')} · {s.skipped} {$t('gitSync.summary.skipped')} · {s.rejected} {$t('gitSync.summary.rejected')}
        </p>
      {/if}
    {/if}
  {/if}
</div>
