<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { asObj, asStr } from '../model/guards';
  import { deref, refName } from '../model/refs';
  import { schemaChildren, typeLabel } from '../model/schema';

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
  <div class="text-sm" style="padding-left: {depth > 0 ? 16 : 0}px">
    <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 py-0.5">
      {#if kids.length > 0 && depth > 0}
        <button
          type="button"
          aria-expanded={isOpen}
          onclick={() => toggle(key)}
          class="font-mono text-[13px] underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">{label ?? ''}</button
        >
      {:else if label}
        <span class="font-mono text-[13px]">{label}</span>
      {/if}
      <span class="font-mono text-xs text-muted">{typeLabel(sch)}</span>
      {#if req}<span class="text-xs text-warning">{$t('apispec.doc.required')}</span>{/if}
      {#if n.readOnly === true}<span class="text-xs text-muted">readOnly</span>{/if}
      {#if n.writeOnly === true}<span class="text-xs text-muted">writeOnly</span>{/if}
      {#if n.deprecated === true}<span class="text-xs text-warning">{$t('apispec.doc.deprecated')}</span>{/if}
      {#if d.circular}<span class="text-xs text-muted">circular{d.ref ? ` · ${refName(d.ref)}` : ''}</span>{/if}
      {#if d.unresolved}<span class="text-xs text-danger">unresolved</span>{/if}
    </div>
    {#if asStr(n.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(n.description)!} /></div>{/if}
    {#if isOpen}
      {#each kids as c (c.name)}
        {@render node(c.schema, next, depth + 1, key + '/' + c.name, c.name, c.required)}
      {/each}
    {/if}
  </div>
{/snippet}

{@render node(schema, [], 0, '', name, required)}
