<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { asObj, asStr, type Obj } from '../model/guards';
  import type { OperationInfo } from '../model/operations';
  import { examplesOf } from '../model/examples';
  import { deref } from '../model/refs';
  import { typeLabel } from '../model/schema';
  import { isNoAuth, usedSchemes } from '../model/security';
  import ExampleBlock from './ExampleBlock.svelte';
  import MethodBadge from './MethodBadge.svelte';
  import SchemaTree from './SchemaTree.svelte';
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
  let titleEl: HTMLHeadingElement | undefined = $state();
  $effect(() => {
    void method;
    void path;
    if (autofocus) titleEl?.focus();
  });
</script>

<article class="flex flex-col gap-5" aria-labelledby="op-title">
  <header class="flex flex-col gap-1">
    <h3 id="op-title" bind:this={titleEl} tabindex="-1" class="flex flex-wrap items-center gap-2 text-base font-semibold focus:outline-none">
      <MethodBadge {method} /><span class="font-mono text-[15px] break-all {op.deprecated === true ? 'line-through' : ''}">{path}</span>
      {#if op.deprecated === true}<span class="text-xs font-normal text-warning">{$t('apispec.doc.deprecated')}</span>{/if}
    </h3>
    {#if asStr(op.summary)}<p class="text-sm">{asStr(op.summary)}</p>{/if}
    {#if asStr(op.operationId)}<code class="font-mono text-xs text-muted">{asStr(op.operationId)}</code>{/if}
    {#if Array.isArray(op.tags) && op.tags.length > 0}
      <p class="text-xs text-muted">{op.tags.filter((x) => typeof x === 'string').join(', ')}</p>
    {/if}
    {#if asStr(op.description)}<div class="text-sm text-muted"><MarkdownView source={asStr(op.description)!} /></div>{/if}
  </header>

  {#if schemes.length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.security')}</h4>
      <SecuritySchemesList {schemes} />
    </section>
  {:else if noAuth}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.security')}</h4>
      <p class="mt-1 text-sm text-muted">{$t('apispec.doc.noAuth')}</p>
    </section>
  {/if}

  <ServersList servers={info.servers} />

  {#each params as g (g.where)}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.parameters')} · {g.where}</h4>
      <ul class="mt-1 flex flex-col gap-1">
        {#each g.items as p, i (i)}
          <li class="text-sm">
            <span class="font-mono text-[13px]">{asStr(p.name)}</span>
            <span class="font-mono text-xs text-muted">{typeLabel(deref(doc, p.schema).node)}</span>
            {#if p.required === true}<span class="text-xs text-warning">{$t('apispec.doc.required')}</span>{/if}
            {#if p.deprecated === true}<span class="text-xs text-warning">{$t('apispec.doc.deprecated')}</span>{/if}
            {#if asStr(p.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(p.description)!} /></div>{/if}
            <ExampleBlock examples={examplesOf(doc, p)} />
          </li>
        {/each}
      </ul>
    </section>
  {/each}

  {#if media(body).length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.requestBody')}{#if body.required === true} <span class="font-normal text-warning">{$t('apispec.doc.required')}</span>{/if}</h4>
      {#if asStr(body.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(body.description)!} /></div>{/if}
      {#each media(body) as [type, m] (type)}
        <p class="mt-1 font-mono text-xs text-muted">{type}</p>
        <SchemaTree {doc} schema={asObj(m).schema} />
        <ExampleBlock examples={examplesOf(doc, m)} />
      {/each}
    </section>
  {/if}

  {#if responses.length > 0}
    <section>
      <h4 class="text-[13px] font-semibold">{$t('apispec.doc.responses')}</h4>
      <ul class="mt-1 flex flex-col gap-3">
        {#each responses as { code, r } (code)}
          <li>
            <p class="text-sm"><span class="font-mono text-[13px] font-medium">{code}</span></p>
            {#if asStr(r.description)}<div class="text-sm"><MarkdownView source={asStr(r.description)!} /></div>{/if}
            {#each media(r) as [type, m] (type)}
              <p class="font-mono text-xs text-muted">{type}</p>
              <SchemaTree {doc} schema={asObj(m).schema} />
              <ExampleBlock examples={examplesOf(doc, m)} />
            {/each}
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</article>
