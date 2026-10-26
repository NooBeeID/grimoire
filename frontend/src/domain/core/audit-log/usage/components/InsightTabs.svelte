<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  type Tab = 'documents' | 'search';
  let { value, onchange }: { value: Tab; onchange: (v: Tab) => void } = $props();
  const uid = $props.id();
  const tabs: Tab[] = ['documents', 'search'];

  function onkeydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    e.preventDefault();
    const next = tabs[(tabs.indexOf(value) + (e.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length]!;
    onchange(next);
    queueMicrotask(() => document.getElementById(`tab-${uid}-${next}`)?.focus());
  }
</script>

<div role="tablist" aria-label={$t('dashboard.usage.tab.label')} class="glass inline-flex gap-1 rounded-full p-1">
  {#each tabs as tab (tab)}
    <button
      type="button"
      role="tab"
      id="tab-{uid}-{tab}"
      aria-selected={value === tab}
      aria-controls="panel-{uid}-{tab}"
      tabindex={value === tab ? 0 : -1}
      onclick={() => onchange(tab)}
      {onkeydown}
      class="h-8 cursor-pointer rounded-full px-4 text-sm font-medium transition-colors {value === tab
        ? 'bg-primary text-primary-fg'
        : 'text-muted hover:bg-surface/70'}"
    >
      {$t(`dashboard.usage.tab.${tab}`)}
    </button>
  {/each}
</div>
