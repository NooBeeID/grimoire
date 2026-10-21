<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asArr, asObj, asStr } from '../model/guards';
  import { getWebhookOperations } from '../model/operations';
  import { allSchemes } from '../model/security';
  import OperationView from './OperationView.svelte';
  import SecuritySchemesList from './SecuritySchemesList.svelte';
  import ServersList from './ServersList.svelte';

  let { doc }: { doc: unknown } = $props();
  const info = $derived(asObj(asObj(doc).info));
  const servers = $derived(asArr(asObj(doc).servers));
  const ext = $derived(asObj(asObj(doc).externalDocs));
  const schemes = $derived(allSchemes(doc));
  const tags = $derived(
    asArr(asObj(doc).tags)
      .map((x) => asObj(x))
      .filter((x) => asStr(x.name) !== null),
  );
  const webhooks = $derived(getWebhookOperations(doc));
  const contact = $derived(asObj(info.contact));
  const license = $derived(asObj(info.license));
</script>

{#snippet link(url: string, label: string)}
  {#if safeHref(url)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(url)} rel="noopener noreferrer nofollow" target="_blank">{label}</a>{:else}<span class="break-all">{label}</span>{/if}
{/snippet}

<section class="flex flex-col gap-4">
  <header>
    <h3 class="text-base font-semibold">{$t('apispec.detail.overview')}</h3>
    <p class="mt-1 text-sm">{asStr(info.title)} · {asStr(info.version)}</p>
    {#if asStr(info.description)}<div class="mt-1 text-sm text-muted"><MarkdownView source={asStr(info.description)!} /></div>{/if}
    <ul class="mt-1 flex flex-wrap gap-x-4 text-xs text-muted">
      {#if asStr(info.termsOfService)}<li>{@render link(asStr(info.termsOfService)!, asStr(info.termsOfService)!)}</li>{/if}
      {#if asStr(contact.url) || asStr(contact.name) || asStr(contact.email)}
        <li>{#if asStr(contact.url)}{@render link(asStr(contact.url)!, asStr(contact.name) ?? asStr(contact.url)!)}{:else}{asStr(contact.name) ?? asStr(contact.email)}{/if}</li>
      {/if}
      {#if asStr(license.name)}<li>{#if asStr(license.url)}{@render link(asStr(license.url)!, asStr(license.name)!)}{:else}{asStr(license.name)}{/if}</li>{/if}
    </ul>
  </header>

  <ServersList {servers} />

  {#if schemes.length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.securitySchemes')}</h4>
      <SecuritySchemesList {schemes} />
    </section>
  {/if}

  {#if asStr(ext.url)}
    <p class="text-sm">{@render link(asStr(ext.url)!, asStr(ext.description) ?? asStr(ext.url)!)}</p>
  {/if}

  {#if tags.length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.tags')}</h4>
      <ul class="mt-1 flex flex-col gap-1">
        {#each tags as tg, i (i)}
          <li class="text-sm">
            <span class="font-mono text-[13px]">{asStr(tg.name)}</span>
            {#if asStr(tg.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(tg.description)!} /></div>{/if}
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if webhooks.length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.webhooks')}</h4>
      <div class="mt-1 flex flex-col gap-2">
        {#each webhooks as w (w.name + ' ' + w.method)}
          <details class="rounded-base border border-control p-2">
            <summary class="cursor-pointer text-sm"><span class="font-mono text-[13px]">{w.method} {w.name}</span></summary>
            <div class="mt-3"><OperationView {doc} method={w.method} path={w.name} info={w.info} autofocus={false} /></div>
          </details>
        {/each}
      </div>
    </section>
  {/if}
</section>
