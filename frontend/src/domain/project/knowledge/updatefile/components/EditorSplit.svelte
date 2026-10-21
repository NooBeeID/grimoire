<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import MarkdownEditor from '../../../../../shared/components/MarkdownEditor.svelte';
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { utf8Length } from '../../../../../shared/utils/bytes';
  import { isMarkdown } from '../../base';
  import type { Pane } from './EditorToolbar.svelte';

  const AUTO_PREVIEW_MAX_BYTES = 200 * 1024;
  const PREVIEW_DEBOUNCE_MS = 250;

  let {
    value = $bindable(''),
    path,
    pane = 'write',
    label,
    onsave,
  }: { value?: string; path: string; pane?: Pane; label: string; onsave?: () => void } = $props();

  let previewSource = $state(untrack(() => value));
  let timer: ReturnType<typeof setTimeout> | undefined;
  const manual = $derived(utf8Length(value) > AUTO_PREVIEW_MAX_BYTES);

  $effect(() => {
    const v = value;
    if (manual) return;
    clearTimeout(timer);
    timer = setTimeout(() => (previewSource = v), PREVIEW_DEBOUNCE_MS);
  });
  onDestroy(() => clearTimeout(timer));
</script>

<div class="grid gap-4 lg:grid-cols-2">
  <div class={pane === 'write' ? '' : 'max-lg:hidden'}>
    <MarkdownEditor bind:value {label} rows={22} {onsave} />
  </div>
  <div class="flex flex-col gap-1 {pane === 'preview' ? '' : 'max-lg:hidden'}">
    <div class="flex items-center justify-between gap-2">
      <span class="text-[13px] font-medium">{$t('knowledge.editor.preview')}</span>
      {#if manual}
        <Button size="sm" variant="secondary" onclick={() => (previewSource = value)}>{$t('knowledge.editor.refreshPreview')}</Button>
      {/if}
    </div>
    <div class="min-h-40 rounded-base border border-border p-3">
      {#if isMarkdown(path)}<MarkdownView source={previewSource} />{:else}<pre class="font-mono text-[13px] whitespace-pre-wrap">{previewSource}</pre>{/if}
    </div>
  </div>
</div>
