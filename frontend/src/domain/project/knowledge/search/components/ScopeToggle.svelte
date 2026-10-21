<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let { contextDir, scope, onchange }: { contextDir: string; scope: string; onchange: (scope: string) => void } = $props();
</script>

{#if contextDir}
  <label class="flex items-center gap-2 px-1 text-[13px]">
    <input
      type="checkbox"
      checked={scope !== ''}
      onchange={(e) => onchange(e.currentTarget.checked ? contextDir : '')}
      class="accent-primary"
    />
    <span class="break-all">{$t('knowledge.search.onlyIn', { dir: contextDir })}</span>
  </label>
{/if}
