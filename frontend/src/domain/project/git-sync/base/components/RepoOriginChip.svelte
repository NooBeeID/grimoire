<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { copyText } from '../../../../../shared/utils/clipboard';

  let { host, repoPath, branch = '', folder = '' }: { host: string; repoPath: string; branch?: string; folder?: string } = $props();

  const text = $derived(`${host}/${repoPath}${branch ? `@${branch}` : ''}${folder ? `:${folder}/` : ''}`);

  async function copy() {
    const ok = await copyText(text);
    toast.push({ type: ok ? 'success' : 'error', message: $t(ok ? 'knowledge.copy.done' : 'knowledge.copy.failed') });
  }
</script>

<span class="inline-flex max-w-full items-center gap-2 rounded-base bg-raised px-2 py-1 text-[13px] text-fg" data-testid="repo-origin">
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="shrink-0 text-muted">
    <circle cx="4" cy="3.5" r="1.5" />
    <circle cx="4" cy="12.5" r="1.5" />
    <circle cx="12" cy="6.5" r="1.5" />
    <path d="M4 5v6M4 9c0-2 2-2.5 6.5-2.5" />
  </svg>
  <code class="truncate font-mono" title={text}>{text}</code>
  <button
    type="button"
    aria-label={$t('gitSync.copyOrigin')}
    onclick={copy}
    class="shrink-0 rounded-base p-0.5 text-muted transition-colors hover:text-fg focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
  >
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true">
      <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
      <path d="M10.5 5.5v-2a1.5 1.5 0 0 0-1.5-1.5H4A1.5 1.5 0 0 0 2.5 3.5V9A1.5 1.5 0 0 0 4 10.5h1.5" />
    </svg>
  </button>
</span>
