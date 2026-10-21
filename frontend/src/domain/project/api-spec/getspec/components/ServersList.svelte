<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asObj, asStr } from '../model/guards';

  let { servers }: { servers: unknown[] } = $props();
</script>

{#if servers.length > 0}
  <section>
    <h4 class="text-[13px] font-semibold">{$t('apispec.doc.servers')}</h4>
    <ul class="mt-1 flex flex-col gap-1">
      {#each servers as s, i (i)}
        {@const url = asStr(asObj(s).url) ?? ''}
        {@const desc = asStr(asObj(s).description)}
        <li>
          <span class="font-mono text-[13px] break-all">
            {#if safeHref(url)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(url)} rel="noopener noreferrer" target="_blank">{url}</a>{:else}{url}{/if}
          </span>
          {#if desc}<div class="text-xs text-muted"><MarkdownView source={desc} /></div>{/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
