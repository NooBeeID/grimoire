<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import MarkdownEditor from '../../../../../shared/components/MarkdownEditor.svelte';
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { isMarkdown } from '../../base';

  let {
    path,
    content,
    pathError,
    formError,
    exists,
    submitting,
    valid,
    onpathchange,
    oncontentchange,
    onsubmit,
    oncancel,
    onopenexisting,
    ontouch,
  }: {
    path: string;
    content: string;
    pathError?: string;
    formError: string | null;
    exists: boolean;
    submitting: boolean;
    valid: boolean;
    onpathchange: (v: string) => void;
    oncontentchange: (v: string) => void;
    onsubmit: () => void;
    oncancel: () => void;
    onopenexisting?: () => void;
    ontouch: () => void;
  } = $props();

  // Pratinjau di-debounce 250 ms agar mengetik tidak merender ulang Markdown tiap tombol.
  let previewSource = $state(untrack(() => content));
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const c = content;
    clearTimeout(timer);
    timer = setTimeout(() => (previewSource = c), 250);
  });
  onDestroy(() => clearTimeout(timer));
</script>

<form
  onsubmit={(e) => {
    e.preventDefault();
    onsubmit();
  }}
  class="flex flex-col gap-4"
  novalidate
>
  <h2 class="text-lg font-semibold">{$t('knowledge.new.title')}</h2>
  <div onfocusout={ontouch} role="none">
    <Input
      bind:value={() => path, (v) => onpathchange(v)}
      label={$t('knowledge.new.path')}
      hint={$t('knowledge.new.pathHint')}
      error={pathError}
    />
  </div>
  {#if exists && onopenexisting}
    <button type="button" class="self-start text-sm underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none" onclick={onopenexisting}>
      {$t('knowledge.new.openExisting')}
    </button>
  {/if}
  {#if formError}<p role="alert" class="text-sm text-danger">{formError}</p>{/if}
  <div class="grid gap-4 lg:grid-cols-2">
    <MarkdownEditor bind:value={() => content, (v) => oncontentchange(v)} label={$t('knowledge.editor.label')} rows={18} onsave={onsubmit} />
    <div class="flex flex-col gap-1">
      <span class="text-[13px] font-medium">{$t('knowledge.editor.preview')}</span>
      <div class="min-h-40 rounded-base border border-border p-3">
        {#if isMarkdown(path) || path === '' || path.endsWith('/')}
          <MarkdownView source={previewSource} />
        {:else}
          <pre class="font-mono text-[13px] whitespace-pre-wrap">{previewSource}</pre>
        {/if}
      </div>
    </div>
  </div>
  <div class="flex gap-2">
    <Button type="submit" loading={submitting} disabled={!valid}>{$t('knowledge.editor.save')}</Button>
    <Button variant="secondary" onclick={oncancel}>{$t('knowledge.editor.cancel')}</Button>
  </div>
</form>
