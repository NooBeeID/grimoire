<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  export type Pane = 'write' | 'preview';

  let {
    path,
    dirty,
    saving,
    pane,
    onpane,
    onsave,
    oncancel,
  }: {
    path: string;
    dirty: boolean;
    saving: boolean;
    pane: Pane;
    onpane: (p: Pane) => void;
    onsave: () => void;
    oncancel: () => void;
  } = $props();

  const tabs: Pane[] = ['write', 'preview'];
</script>

<header class="flex flex-wrap items-center justify-between gap-3 border-b border-rule pb-3">
  <div class="min-w-0">
    <h2 class="font-mono text-[13px] break-all">{path}</h2>
    <p class="text-sm {dirty ? 'text-warning' : 'text-muted'}" aria-live="polite">
      {dirty ? $t('knowledge.editor.unsaved') : $t('knowledge.editor.saved')}
    </p>
  </div>
  <div class="flex flex-wrap items-center gap-2">
    <div role="group" aria-label={$t('knowledge.editor.preview')} class="flex gap-1 lg:hidden">
      {#each tabs as p (p)}
        <button
          type="button"
          aria-pressed={pane === p}
          onclick={() => onpane(p)}
          class="h-8 rounded-base border px-3 text-sm focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {pane === p ? 'border-primary bg-active font-semibold' : 'border-control'}"
        >{$t(`knowledge.editor.${p}`)}</button>
      {/each}
    </div>
    <Button loading={saving} onclick={onsave}>{$t('knowledge.editor.save')}</Button>
    <Button variant="secondary" onclick={oncancel}>{$t('knowledge.editor.cancel')}</Button>
  </div>
</header>
