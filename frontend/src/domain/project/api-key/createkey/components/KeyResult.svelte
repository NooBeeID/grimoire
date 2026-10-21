<script lang="ts">
  import CopyField from '../../../../../shared/components/CopyField.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { claudeCodeSnippet, genericJsonSnippet } from '../snippets';

  let { rawKey, name, mcpUrl }: { rawKey: string; name: string; mcpUrl: string } = $props();
</script>

<div class="flex flex-col gap-4">
  <p role="alert" class="rounded-base border border-warning px-3 py-2 text-sm font-medium text-warning">
    {$t('project.apikey.result.onceWarning')}
  </p>
  <CopyField value={rawKey} label={$t('project.apikey.result.keyLabel')} testid="raw-key" />
  <CopyField value={claudeCodeSnippet(name, mcpUrl, rawKey)} label={$t('project.apikey.result.claudeCode')} multiline rows={3} />
  <CopyField value={genericJsonSnippet(name, mcpUrl, rawKey)} label={$t('project.apikey.result.genericJson')} multiline rows={8} />
  <p class="text-xs text-muted">{$t('project.apikey.result.urlNote', { url: mcpUrl })}</p>
</div>
