<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { isMarkdown, type KnowledgeFile } from '../../base';

  let { file, resolveLink }: { file: KnowledgeFile; resolveLink?: (href: string) => string | null } = $props();

  let root = $state<HTMLDivElement | null>(null);
  let items = $state<{ id: string; text: string; depth: number }[]>([]);
  let active = $state('');

  // Daftar isi dibaca dari heading hasil render (id "md-…" dari renderMarkdown).
  $effect(() => {
    void file.content;
    const el = root;
    if (!el) return;
    items = [...el.querySelectorAll<HTMLElement>('.md-body h1, .md-body h2, .md-body h3')].map((h) => ({
      id: h.id,
      text: h.textContent ?? '',
      depth: Number(h.tagName[1]),
    }));
  });

  const minDepth = $derived(items.length ? Math.min(...items.map((i) => i.depth)) : 1);

  function jump(id: string) {
    active = id;
    root?.querySelector<HTMLElement>(`[id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }
</script>

{#if file.content === ''}
  <p class="text-sm text-muted">{$t('knowledge.viewer.emptyFile')}</p>
{:else if isMarkdown(file.path)}
  <div class="flex items-start gap-8" bind:this={root}>
    <div class="min-w-0 flex-1">
      <MarkdownView source={file.content} {resolveLink} />
    </div>
    {#if items.length >= 2}
      <nav aria-label={$t('knowledge.viewer.outline')} class="sticky top-0 hidden max-h-[70vh] w-56 shrink-0 overflow-y-auto text-sm xl:block">
        <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">{$t('knowledge.viewer.outline')}</p>
        <ul class="flex flex-col border-l border-rule">
          {#each items as it (it.id + it.text)}
            <li>
              <button
                type="button"
                onclick={() => jump(it.id)}
                style="padding-left: {0.75 + (it.depth - minDepth) * 0.75}rem"
                class="block w-full py-1 pr-2 text-left text-muted hover:text-fg {active === it.id ? 'font-medium text-fg' : ''}"
              >{it.text}</button>
            </li>
          {/each}
        </ul>
      </nav>
    {/if}
  </div>
{:else}
  <pre class="font-mono text-[13px] whitespace-pre-wrap">{file.content}</pre>
{/if}
