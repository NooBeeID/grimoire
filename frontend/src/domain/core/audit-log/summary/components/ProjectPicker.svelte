<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { ProjectOption } from '../types';

  let {
    projectId,
    options,
    total,
    status,
    fallbackLabel,
    onchange,
    onretry,
  }: {
    projectId: string | null;
    options: ProjectOption[];
    total: number;
    status: 'idle' | 'loading' | 'ready' | 'error';
    fallbackLabel?: string | null;
    onchange: (projectId: string | null) => void;
    onretry: () => void;
  } = $props();

  const items = $derived.by(() => {
    const list = [{ value: '', label: status === 'loading' ? $t('dashboard.project.loading') : $t('dashboard.project.all') }];
    for (const o of options) list.push({ value: o.id, label: o.name });
    if (projectId && !options.some((o) => o.id === projectId)) {
      list.push({ value: projectId, label: fallbackLabel ?? projectId.slice(0, 8) });
    }
    return list;
  });
  let value = $state('');
  $effect(() => {
    value = projectId ?? '';
  });
</script>

<div class="flex min-w-0 flex-col gap-1">
  <Select
    bind:value
    label={$t('dashboard.project.label')}
    options={items}
    disabled={status === 'loading' || status === 'error'}
    onchange={(v) => onchange(v || null)}
  />
  {#if status === 'error'}
    <p class="flex items-center gap-2 text-xs text-muted">
      {$t('dashboard.project.loadFailed')}
      <Button variant="ghost" size="sm" onclick={onretry}>{$t('common.retry')}</Button>
    </p>
  {:else if total > options.length}
    <p class="text-xs text-muted">{$t('dashboard.project.more', { n: options.length })}</p>
  {/if}
</div>
