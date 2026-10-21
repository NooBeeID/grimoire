<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { toPrettyJson, type ExampleItem } from '../model/examples';

  let { examples }: { examples: ExampleItem[] } = $props();

  let idx = $state(0);
  let copied = $state(false);
  let failed = $state(false);
  const cur = $derived(examples[Math.min(idx, examples.length - 1)]);
  const text = $derived(cur ? (cur.externalValue !== null && cur.value === undefined ? cur.externalValue : toPrettyJson(cur.value)) : '');
  let pre: HTMLPreElement | undefined = $state();

  async function copy() {
    failed = false;
    if (await copyText(text)) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } else {
      failed = true;
      if (pre) {
        const r = document.createRange();
        r.selectNodeContents(pre);
        const sel = window.getSelection();
        sel?.removeAllRanges();
        sel?.addRange(r);
      }
    }
  }
</script>

{#if cur}
  <div class="mt-1">
    <div class="flex flex-wrap items-center gap-2 text-xs text-muted">
      <span>{$t('apispec.doc.examples')}</span>
      {#if examples.length > 1}
        <select aria-label={$t('apispec.doc.examples')} bind:value={idx} class="h-7 rounded-base border border-control bg-surface px-2 text-xs">
          {#each examples as e, i (i)}<option value={i}>{e.summary ?? e.name}</option>{/each}
        </select>
      {:else if cur.summary}
        <span>{cur.summary}</span>
      {/if}
      <button type="button" onclick={copy} class="ml-auto rounded-base px-2 py-0.5 hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">
        {copied ? $t('common.copied') : $t('common.copy')}
      </button>
    </div>
    <pre bind:this={pre} class="mt-1 max-h-72 overflow-auto rounded-base bg-raised p-2 font-mono text-xs whitespace-pre-wrap break-all"><code>{text}</code></pre>
    {#if failed}<p role="status" class="text-xs text-muted">{$t('common.copyManual')}</p>{/if}
  </div>
{/if}
