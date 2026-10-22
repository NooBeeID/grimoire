<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
</script>

<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 rounded-base bg-raised p-3 text-[13px]">
  <dt class="text-muted">{$t('gitCredential.provider.github')}</dt>
  <dd><code class="font-mono">repo</code></dd>
  <dt class="text-muted">{$t('gitCredential.provider.gitlab')}</dt>
  <dd><code class="font-mono">read_api</code>, <code class="font-mono">read_repository</code></dd>
</dl>
