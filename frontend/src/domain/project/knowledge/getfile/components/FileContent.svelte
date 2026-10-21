<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { isMarkdown, type KnowledgeFile } from '../../base';

  let { file, resolveLink }: { file: KnowledgeFile; resolveLink?: (href: string) => string | null } = $props();
</script>

{#if file.content === ''}
  <p class="text-sm text-muted">{$t('knowledge.viewer.emptyFile')}</p>
{:else if isMarkdown(file.path)}
  <MarkdownView source={file.content} {resolveLink} />
{:else}
  <pre class="font-mono text-[13px] whitespace-pre-wrap">{file.content}</pre>
{/if}
