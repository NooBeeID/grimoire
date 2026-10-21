<script lang="ts">
  import { t } from '../stores/i18n';
  import { renderMermaid } from '../utils/markdown/mermaid';
  import { renderMarkdown } from '../utils/markdown/render';

  let {
    source,
    resolveLink,
    mermaidLabel,
  }: { source: string; resolveLink?: (href: string) => string | null; mermaidLabel?: string } = $props();
  const rendered = $derived(renderMarkdown(source, { resolveLink }));
  const uid = $props.id();

  let root = $state<HTMLDivElement | null>(null);

  // Ganti tiap blok .md-mermaid dengan SVG hasil renderMermaid (sudah lewat sanitizeSvg).
  // Token `stale` membatalkan render usang bila source berubah sebelum selesai.
  $effect(() => {
    const el = root;
    const codes = rendered.mermaid;
    const label = mermaidLabel ?? $t('md.mermaidLabel');
    const errorText = $t('md.mermaidError');
    if (!el || codes.length === 0) return;
    let stale = false;
    el.querySelectorAll<HTMLElement>('.md-mermaid[data-mermaid-index]').forEach((box) => {
      const code = codes[Number(box.dataset.mermaidIndex)];
      if (code === undefined) return;
      void renderMermaid(code, `mmd-${uid}-${box.dataset.mermaidIndex}`).then((r) => {
        if (stale || !box.isConnected) return;
        if (r.ok) {
          box.innerHTML = r.svg; // keluaran sanitizeSvg
          box.setAttribute('role', 'img');
          box.setAttribute('aria-label', label);
        } else if (!box.querySelector('.md-mermaid-error')) {
          const note = document.createElement('p');
          note.className = 'md-mermaid-error text-sm text-danger';
          note.textContent = errorText;
          box.appendChild(note);
        }
      });
    });
    return () => {
      stale = true;
    };
  });
</script>

<!-- Satu-satunya {@html}; masukan selalu keluaran renderMarkdown() yang sudah lewat sanitizeHtml. -->
<div class="md-body" bind:this={root}>{@html rendered.html}</div>
