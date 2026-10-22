<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { scopeFor, type GitProvider } from '../types';

  let { provider }: { provider: GitProvider } = $props();
</script>

<p class="rounded-base bg-raised px-3 py-2 text-[13px]">
  <span class="text-muted">{$t('gitCredential.scope.label')}</span>
  {#each scopeFor(provider) as s, i (s)}{i > 0 ? ' ' : ' '}<code class="font-mono">{s}</code>{/each}
</p>
