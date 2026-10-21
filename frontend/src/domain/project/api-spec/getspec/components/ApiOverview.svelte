<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asArr, asObj, asStr } from '../model/guards';

  let { doc }: { doc: unknown } = $props();
  const info = $derived(asObj(asObj(doc).info));
  const servers = $derived(asArr(asObj(doc).servers));
  const ext = $derived(asObj(asObj(doc).externalDocs));
</script>

<section class="flex flex-col gap-4">
  <header>
    <h3 class="text-base font-semibold">{$t('apispec.detail.overview')}</h3>
    <p class="mt-1 text-sm">{asStr(info.title)} · {asStr(info.version)}</p>
    {#if asStr(info.description)}<p class="mt-1 text-sm text-muted">{asStr(info.description)}</p>{/if}
  </header>
  {#if servers.length > 0}
    <div>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.servers')}</h4>
      <ul class="mt-1">
        {#each servers as s, i (i)}
          {@const url = asStr(asObj(s).url) ?? ''}
          <li class="font-mono text-[13px] break-all">
            {#if safeHref(url)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(url)} rel="noopener noreferrer" target="_blank">{url}</a>{:else}{url}{/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
  {#if asStr(ext.url)}
    {@const url = asStr(ext.url)!}
    <p class="text-sm">
      {#if safeHref(url)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(url)} rel="noopener noreferrer" target="_blank">{asStr(ext.description) ?? url}</a>{:else}{url}{/if}
    </p>
  {/if}
</section>
