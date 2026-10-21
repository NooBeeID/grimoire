<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { asObj, asStr } from '../model/guards';
  import { deref, refName } from '../model/refs';
  import { schemaChildren, typeLabel } from '../model/schema';
  import Pill from './Pill.svelte';

  let { doc, schema, name, required = false }: { doc: unknown; schema: unknown; name?: string; required?: boolean } = $props();

  // Anak dimuat saat baris dibuka; kunci = jalur dari akar (komponen satu, rekursi lewat snippet).
  let opened = $state<Record<string, boolean>>({});
  const toggle = (k: string) => (opened = { ...opened, [k]: !opened[k] });
</script>

{#snippet node(sch: unknown, stack: string[], depth: number, key: string, label: string | undefined, req: boolean)}
  {@const d = deref(doc, sch, stack)}
  {@const n = asObj(d.node)}
  {@const next = d.ref ? [...stack, d.ref] : stack}
  {@const kids = !d.circular && !d.unresolved ? schemaChildren(doc, sch, stack) : []}
  {@const isOpen = depth === 0 || opened[key] === true}
  <div class="text-sm">
    <div class="flex flex-wrap items-center gap-x-2 gap-y-1 py-1.5">
      {#if kids.length > 0 && depth > 0}
        <button
          type="button"
          aria-expanded={isOpen}
          onclick={() => toggle(key)}
          class="inline-flex items-center gap-1 rounded-base font-mono text-[13px] font-medium hover:text-accent-ink focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
        >
          <svg aria-hidden="true" viewBox="0 0 16 16" class="size-3 text-muted transition-transform {isOpen ? 'rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M6 3.5 10.5 8 6 12.5" /></svg>{label ?? ''}</button
        >
      {:else if label}
        <span class="font-mono text-[13px] font-medium {depth > 0 ? 'pl-4' : ''}">{label}</span>
      {/if}
      <span class="font-mono text-xs text-muted">{typeLabel(sch)}</span>
      {#if req}<Pill tone="warning">{$t('apispec.doc.required')}</Pill>{/if}
      {#if n.readOnly === true}<Pill>readOnly</Pill>{/if}
      {#if n.writeOnly === true}<Pill>writeOnly</Pill>{/if}
      {#if n.deprecated === true}<Pill tone="warning">{$t('apispec.doc.deprecated')}</Pill>{/if}
      {#if d.circular}<Pill>circular{d.ref ? ` · ${refName(d.ref)}` : ''}</Pill>{/if}
      {#if d.unresolved}<Pill tone="negative">unresolved</Pill>{/if}
    </div>
    {#if asStr(n.description)}<div class="-mt-1 pb-1.5 text-xs text-muted {depth > 0 ? 'pl-4' : ''}"><MarkdownView source={asStr(n.description)!} /></div>{/if}
    {#if isOpen && kids.length > 0}
      <div class="ml-1.5 border-l border-rule pl-3">
        {#each kids as c (c.name)}
          {@render node(c.schema, next, depth + 1, key + '/' + c.name, c.name, c.required)}
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

{@render node(schema, [], 0, '', name, required)}
