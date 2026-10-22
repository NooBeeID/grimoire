<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { SummaryRange } from '../types';
  import type { ProjectOptionsState } from '../stores/projectOptions.store';
  import { formatClock } from './format';
  import ProjectPicker from './ProjectPicker.svelte';
  import RangeSegmented from './RangeSegmented.svelte';

  let {
    range,
    projectId,
    projects,
    fallbackLabel,
    loadedAt,
    refreshing,
    refreshError,
    onrange,
    onproject,
    onreload,
    onretryprojects,
  }: {
    range: SummaryRange;
    projectId: string | null;
    projects: ProjectOptionsState;
    fallbackLabel?: string | null;
    loadedAt: Date | null;
    refreshing: boolean;
    refreshError: boolean;
    onrange: (r: SummaryRange) => void;
    onproject: (id: string | null) => void;
    onreload: () => void;
    onretryprojects: () => void;
  } = $props();
</script>

<div class="flex flex-col gap-2">
  <div class="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
    <div class="flex flex-wrap items-end gap-3 max-[480px]:w-full max-[480px]:flex-col max-[480px]:items-stretch">
      <RangeSegmented value={range} onchange={onrange} />
      <ProjectPicker
        {projectId}
        options={projects.options}
        total={projects.total}
        status={projects.status}
        {fallbackLabel}
        onchange={onproject}
        onretry={onretryprojects}
      />
    </div>
    <div class="ml-auto flex items-center gap-3">
      {#if loadedAt}<span class="text-xs text-muted">{$t('dashboard.asOf', { time: formatClock(loadedAt, $locale) })}</span>{/if}
      <Button variant="secondary" loading={refreshing} onclick={onreload}>{$t('dashboard.reload')}</Button>
    </div>
  </div>
  {#if refreshError}
    <p role="status" class="flex items-center gap-2 text-sm text-muted">
      {$t('dashboard.reloadFailed', { time: loadedAt ? formatClock(loadedAt, $locale) : '–' })}
      <Button variant="ghost" size="sm" onclick={onreload}>{$t('common.retry')}</Button>
    </p>
  {/if}
</div>
