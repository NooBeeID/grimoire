<script lang="ts">
  import Button from '../../shared/components/Button.svelte';
  import ErrorState from '../../shared/components/ErrorState.svelte';
  import { t } from '../../shared/stores/i18n';
  let { error, onretry }: { error: unknown; onretry: () => void } = $props();
</script>

<div class="flex flex-col items-center gap-3 text-center">
  <h1 class="text-xl font-bold">{$t('auth.session.loadFailed')}</h1>
  <ErrorState {error} />
  <Button onclick={onretry}>{$t('common.retry')}</Button>
</div>
