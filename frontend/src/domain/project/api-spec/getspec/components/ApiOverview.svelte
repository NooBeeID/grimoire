<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asArr, asObj, asStr } from '../model/guards';
  import { getWebhookOperations } from '../model/operations';
  import { allSchemes } from '../model/security';
  import MethodBadge from './MethodBadge.svelte';
  import OperationView from './OperationView.svelte';
  import Section from './Section.svelte';
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

<div class="flex flex-col gap-6">
  <Section title={$t('apispec.detail.overview')} level={3}>
    <p class="text-lg leading-snug font-semibold">{asStr(info.title)} · {asStr(info.version)}</p>
    {#if asStr(info.description)}<div class="mt-2 text-sm text-muted"><MarkdownView source={asStr(info.description)!} /></div>{/if}
    <ul class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
      {#if asStr(info.termsOfService)}<li>{@render link(asStr(info.termsOfService)!, asStr(info.termsOfService)!)}</li>{/if}
      {#if asStr(contact.url) || asStr(contact.name) || asStr(contact.email)}
        <li>{#if asStr(contact.url)}{@render link(asStr(contact.url)!, asStr(contact.name) ?? asStr(contact.url)!)}{:else}{asStr(contact.name) ?? asStr(contact.email)}{/if}</li>
      {/if}
      {#if asStr(license.name)}<li>{#if asStr(license.url)}{@render link(asStr(license.url)!, asStr(license.name)!)}{:else}{asStr(license.name)}{/if}</li>{/if}
      {#if asStr(ext.url)}<li>{@render link(asStr(ext.url)!, asStr(ext.description) ?? asStr(ext.url)!)}</li>{/if}
    </ul>
  </Section>

  {#if servers.length > 0 || schemes.length > 0}
    <div class="grid items-start gap-6 {servers.length > 0 && schemes.length > 0 ? 'xl:grid-cols-2' : ''}">
      <ServersList {servers} />
      {#if schemes.length > 0}
        <Section title={$t('apispec.doc.securitySchemes')} flush><SecuritySchemesList {schemes} /></Section>
      {/if}
    </div>
  {/if}

  {#if tags.length > 0}
    <Section title={$t('apispec.doc.tags')} flush>
      {#snippet meta()}{tags.length}{/snippet}
      <ul class="divide-y divide-rule">
        {#each tags as tg, i (i)}
          <li class="px-4 py-3 text-sm">
            <span class="font-mono text-[13px] font-medium">{asStr(tg.name)}</span>
            {#if asStr(tg.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(tg.description)!} /></div>{/if}
          </li>
        {/each}
      </ul>
    </Section>
  {/if}

  {#if webhooks.length > 0}
    <Section title={$t('apispec.doc.webhooks')} flush>
      {#snippet meta()}{webhooks.length}{/snippet}
      <div class="divide-y divide-rule">
        {#each webhooks as w (w.name + ' ' + w.method)}
          <details class="group">
            <summary class="flex min-h-12 cursor-pointer list-none items-center gap-3 px-4 py-2 hover:bg-raised/60 focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none focus-visible:ring-inset [&::-webkit-details-marker]:hidden">
              <MethodBadge method={w.method} />
              <span class="min-w-0 flex-1 font-mono text-[13px]">{w.method} {w.name}</span>
              <svg aria-hidden="true" viewBox="0 0 16 16" class="size-4 shrink-0 text-muted transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M6 3.5 10.5 8 6 12.5" /></svg>
            </summary>
            <div class="px-4 pt-2 pb-4"><OperationView {doc} method={w.method} path={w.name} info={w.info} autofocus={false} /></div>
          </details>
        {/each}
      </div>
    </Section>
  {/if}
</div>
