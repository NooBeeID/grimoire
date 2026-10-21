<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatBytes } from '../../../../../shared/utils/bytes';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { KnowledgeFile } from '../../base';

  let { file }: { file: Pick<KnowledgeFile, 'size_bytes' | 'updated_at' | 'updated_by'> } = $props();
</script>

<p class="mt-1 text-sm text-muted">
  {$t('knowledge.viewer.size', { size: formatBytes(file.size_bytes, $locale) })}
  <span aria-hidden="true">·</span>
  {$t('knowledge.viewer.updatedBy', { name: file.updated_by.label, time: '' })}<time datetime={file.updated_at}>{formatDateTime(file.updated_at, $locale)}</time>
</p>
