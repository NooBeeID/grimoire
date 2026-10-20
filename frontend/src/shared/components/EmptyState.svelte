<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../stores/i18n';
  let { title, description, action }: { title?: string; description?: string; action?: Snippet } = $props();
</script>

<div class="flex flex-col items-center gap-2 p-8 text-center">
  <h3 class="text-base font-semibold">{title ?? $t('common.empty')}</h3>
  {#if description}<p class="text-sm text-muted">{description}</p>{/if}
  {@render action?.()}
</div>
