<script lang="ts">
  import { asObj } from '../model/guards';
  import { examplesOf, type ExampleItem } from '../model/examples';
  import { sampleFromSchema, type SampleMode } from '../model/sampleFromSchema';
  import { t } from '../../../../../shared/stores/i18n';
  import ExampleBlock from './ExampleBlock.svelte';
  import SchemaTree from './SchemaTree.svelte';

  // Satu media type = label biasa; lebih dari satu = tab. Hanya tab terpilih yang dirender (skema bisa besar).
  let { doc, media, mode = 'response' }: { doc: unknown; media: Array<[string, unknown]>; mode?: SampleMode } = $props();

  let idx = $state(0);
  const cur = $derived(media[Math.min(idx, media.length - 1)]);
  // Spec tanpa contoh: buat dari skema dan tandai sebagai hasil generate. Hanya untuk media type JSON.
  const examples = $derived.by((): ExampleItem[] => {
    if (!cur) return [];
    const own = examplesOf(doc, cur[1]);
    const schema = asObj(cur[1]).schema;
    if (own.length > 0 || schema === undefined || !/json/i.test(cur[0])) return own;
    return [{ name: 'generated', summary: $t('apispec.doc.generatedExample'), value: sampleFromSchema(doc, schema, mode), externalValue: null }];
  });
</script>

{#if cur}
  {#if media.length > 1}
    <div role="tablist" class="flex gap-4 border-b border-rule">
      {#each media as [type], i (type)}
        <button
          type="button"
          role="tab"
          aria-selected={i === idx}
          onclick={() => (idx = i)}
          class="-mb-px h-9 border-b-2 font-mono text-xs focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {i === idx
            ? 'border-accent font-medium text-fg'
            : 'border-transparent text-muted hover:text-fg'}">{type}</button
        >
      {/each}
    </div>
  {:else}
    <p class="font-mono text-xs text-muted">{cur[0]}</p>
  {/if}
  <div class="mt-3 flex flex-col gap-3" role={media.length > 1 ? 'tabpanel' : undefined}>
    <div class="rounded-base bg-raised/60 px-3 py-1.5">
      <SchemaTree {doc} schema={asObj(cur[1]).schema} />
    </div>
    <ExampleBlock {examples} />
  </div>
{/if}
