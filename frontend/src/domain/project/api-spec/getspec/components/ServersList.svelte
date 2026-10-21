<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asObj, asStr } from '../model/guards';
  import Section from './Section.svelte';

  let { servers }: { servers: unknown[] } = $props();
</script>

{#if servers.length > 0}
  <Section title={$t('apispec.doc.servers')} flush>
    <ul class="divide-y divide-rule">
      {#each servers as s, i (i)}
        {@const url = asStr(asObj(s).url) ?? ''}
        {@const desc = asStr(asObj(s).description)}
        <li class="px-4 py-2.5">
          <span class="font-mono text-[13px] break-all">
            {#if safeHref(url)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(url)} rel="noopener noreferrer nofollow" target="_blank">{url}</a>{:else}{url}{/if}
          </span>
          {#if desc}<div class="text-xs text-muted"><MarkdownView source={desc} /></div>{/if}
        </li>
      {/each}
    </ul>
  </Section>
{/if}
