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
  <div class="overflow-hidden rounded-base border border-border">
    <div class="flex min-h-9 flex-wrap items-center gap-2 bg-raised px-3 py-1 text-xs text-muted">
      <span class="font-medium text-fg">{$t('apispec.doc.examples')}</span>
      {#if examples.length > 1}
        <select aria-label={$t('apispec.doc.examples')} bind:value={idx} class="h-7 rounded-base border border-control bg-surface px-2 text-xs text-fg">
          {#each examples as e, i (i)}<option value={i}>{e.summary ?? e.name}</option>{/each}
        </select>
      {:else if cur.summary}
        <span>{cur.summary}</span>
      {/if}
      <button type="button" onclick={copy} class="ml-auto h-7 rounded-base px-2 hover:bg-active focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">
        {copied ? $t('common.copied') : $t('common.copy')}
      </button>
    </div>
    <pre bind:this={pre} class="max-h-72 overflow-auto bg-surface p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-all"><code>{text}</code></pre>
  </div>
  {#if failed}<p role="status" class="mt-1 text-xs text-muted">{$t('common.copyManual')}</p>{/if}
{/if}
