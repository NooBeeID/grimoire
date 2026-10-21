<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { copyText } from '../../../../../shared/utils/clipboard';
  import { asObj, asStr, type Obj } from '../model/guards';
  import type { OperationInfo } from '../model/operations';
  import { examplesOf } from '../model/examples';
  import { deref } from '../model/refs';
  import { typeLabel } from '../model/schema';
  import { isNoAuth, usedSchemes } from '../model/security';
  import ExampleBlock from './ExampleBlock.svelte';
  import MediaTabs from './MediaTabs.svelte';
  import MethodBadge from './MethodBadge.svelte';
  import Pill from './Pill.svelte';
  import Section from './Section.svelte';
  import SecuritySchemesList from './SecuritySchemesList.svelte';
  import ServersList from './ServersList.svelte';

  let {
    doc,
    method,
    path,
    info,
    autofocus = true,
  }: { doc: unknown; method: string; path: string; info: OperationInfo; autofocus?: boolean } = $props();
  const schemes = $derived(usedSchemes(doc, info.security));
  const noAuth = $derived(isNoAuth(info.security));

  const op = $derived(info.operation);
  const IN = ['path', 'query', 'header', 'cookie'] as const;
  const params = $derived(IN.map((where) => ({ where, items: info.params.filter((p) => p.in === where) })).filter((g) => g.items.length > 0));
  const body = $derived(asObj(deref(doc, op.requestBody).node));
  const responses = $derived(
    Object.entries(asObj(op.responses))
      .map(([code, r]) => ({ code, r: asObj(deref(doc, r).node) }))
      .sort((a, b) => (a.code === 'default' ? 1 : b.code === 'default' ? -1 : 0)),
  );
  const media = (o: Obj) => Object.entries(asObj(o.content));
  const tags = $derived(Array.isArray(op.tags) ? op.tags.filter((x): x is string => typeof x === 'string') : []);
  // POST/PUT/PATCH tanpa body tetap diberi bagian sendiri, supaya "tidak ada body" terbaca sebagai fakta, bukan bagian yang hilang.
  const expectsBody = $derived(['POST', 'PUT', 'PATCH'].includes(method.toUpperCase()));
  const tone = (code: string) => (code.startsWith('2') ? 'positive' : code.startsWith('4') ? 'warning' : code.startsWith('5') ? 'negative' : 'neutral');

  let titleEl: HTMLHeadingElement | undefined = $state();
  let copied = $state(false);
  async function copyPath() {
    if (await copyText(path)) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    }
  }
  $effect(() => {
    void method;
    void path;
    if (autofocus) titleEl?.focus();
  });
</script>

<article class="flex flex-col gap-6" aria-labelledby="op-title">
  <header class="flex flex-col gap-3">
    <div class="flex items-center gap-3 rounded-card border border-border bg-raised py-2 pr-2 pl-3">
      <h3 id="op-title" bind:this={titleEl} tabindex="-1" class="flex min-w-0 flex-1 flex-wrap items-center gap-3 focus:outline-none">
        <MethodBadge {method} large /><span class="font-mono text-[15px] font-medium break-all {op.deprecated === true ? 'line-through' : ''}">{path}</span>
      </h3>
      <button
        type="button"
        onclick={copyPath}
        class="h-8 shrink-0 rounded-base px-2.5 text-xs text-muted hover:bg-active hover:text-fg focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
        >{copied ? $t('common.copied') : $t('apispec.doc.copyPath')}</button
      >
    </div>
    {#if asStr(op.summary)}<p class="text-lg leading-snug font-semibold">{asStr(op.summary)}</p>{/if}
    {#if asStr(op.description)}<div class="text-sm text-muted"><MarkdownView source={asStr(op.description)!} /></div>{/if}
    {#if op.deprecated === true || asStr(op.operationId) || tags.length > 0}
      <div class="flex flex-wrap items-center gap-2">
        {#if op.deprecated === true}<Pill tone="warning">{$t('apispec.doc.deprecated')}</Pill>{/if}
        {#if asStr(op.operationId)}<code class="rounded-base bg-raised px-2 py-0.5 font-mono text-xs text-muted">{asStr(op.operationId)}</code>{/if}
        {#each tags as tag (tag)}<Pill>{tag}</Pill>{/each}
      </div>
    {/if}
  </header>

  {#if schemes.length > 0 || noAuth || info.servers.length > 0}
    <div class="grid items-start gap-6 {(schemes.length > 0 || noAuth) && info.servers.length > 0 ? 'xl:grid-cols-2' : ''}">
      {#if schemes.length > 0}
        <Section title={$t('apispec.doc.security')} flush><SecuritySchemesList {schemes} /></Section>
      {:else if noAuth}
        <Section title={$t('apispec.doc.security')}><p class="text-sm text-muted">{$t('apispec.doc.noAuth')}</p></Section>
      {/if}
      <ServersList servers={info.servers} />
    </div>
  {/if}

  {#each params as g (g.where)}
    <Section title="{$t('apispec.doc.parameters')} · {g.where}" flush>
      {#snippet meta()}{g.items.length}{/snippet}
      <table class="w-full border-collapse text-left text-sm">
        <thead>
          <tr class="text-xs text-muted">
            <th scope="col" class="h-9 w-[34%] px-4 font-medium">{$t('apispec.doc.field')}</th>
            <th scope="col" class="h-9 w-[22%] px-4 font-medium">{$t('apispec.doc.type')}</th>
            <th scope="col" class="h-9 px-4 font-medium">{$t('apispec.doc.description')}</th>
          </tr>
        </thead>
        <tbody>
          {#each g.items as p, i (i)}
            <tr class="border-t border-rule align-top">
              <td class="px-4 py-3">
                <span class="font-mono text-[13px] font-medium break-all">{asStr(p.name)}</span>
                <div class="mt-1 flex flex-wrap gap-1">
                  {#if p.required === true}<Pill tone="warning">{$t('apispec.doc.required')}</Pill>{/if}
                  {#if p.deprecated === true}<Pill tone="warning">{$t('apispec.doc.deprecated')}</Pill>{/if}
                </div>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-muted">{typeLabel(deref(doc, p.schema).node)}</td>
              <td class="px-4 py-3">
                {#if asStr(p.description)}<div class="text-sm text-muted"><MarkdownView source={asStr(p.description)!} /></div>{:else}<span class="text-muted">–</span>{/if}
                <ExampleBlock examples={examplesOf(doc, p)} />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </Section>
  {/each}

  {#if media(body).length > 0}
    <Section title={$t('apispec.doc.requestBody')}>
      {#snippet meta()}
        <Pill tone={body.required === true ? 'warning' : 'neutral'}>{body.required === true ? $t('apispec.doc.required') : $t('apispec.doc.optional')}</Pill>
      {/snippet}
      {#if asStr(body.description)}<div class="mb-3 text-sm text-muted"><MarkdownView source={asStr(body.description)!} /></div>{/if}
      <MediaTabs {doc} media={media(body)} mode="request" />
    </Section>
  {:else if expectsBody}
    <Section title={$t('apispec.doc.requestBody')}>
      <p class="text-sm text-muted">{$t('apispec.doc.noBody')}</p>
    </Section>
  {/if}

  {#if responses.length > 0}
    <Section title={$t('apispec.doc.responses')} flush>
      {#snippet meta()}{$t('apispec.doc.responseCount', { n: responses.length })}{/snippet}
      <div class="divide-y divide-rule">
        {#each responses as { code, r }, i (code)}
          <details open={i === 0} class="group">
            <summary class="flex min-h-12 cursor-pointer list-none items-start gap-3 px-4 py-3 hover:bg-raised/60 focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none focus-visible:ring-inset [&::-webkit-details-marker]:hidden">
              <Pill tone={tone(code)}><span class="font-mono">{code}</span></Pill>
              <div class="min-w-0 flex-1 text-sm">
                {#if asStr(r.description)}<MarkdownView source={asStr(r.description)!} />{:else}<span class="text-muted">–</span>{/if}
              </div>
              <svg aria-hidden="true" viewBox="0 0 16 16" class="mt-0.5 size-4 shrink-0 text-muted transition-transform group-open:rotate-90" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M6 3.5 10.5 8 6 12.5" /></svg>
            </summary>
            <div class="px-4 pb-4">
              {#if media(r).length > 0}
                <MediaTabs {doc} media={media(r)} />
              {:else}
                <p class="text-sm text-muted">{$t('apispec.doc.noContent')}</p>
              {/if}
            </div>
          </details>
        {/each}
      </div>
    </Section>
  {/if}
</article>
