<script lang="ts">
  import ErrorState from '../shared/components/ErrorState.svelte';
  import Button from '../shared/components/Button.svelte';
  import ToastHost from '../shared/components/Toast.svelte';
  import { session } from '../shared/stores/session';
  import { t } from '../shared/stores/i18n';
  import AppLayout from './layouts/AppLayout.svelte';
  import BlankLayout from './layouts/BlankLayout.svelte';
  import { resolve } from './routes/guard';
  import { notFoundRoute } from './routes/index';
  import { current, navigate } from './routes/router';
  import PendingView from './views/PendingView.svelte';
  import SessionErrorView from './views/SessionErrorView.svelte';
  import { boot, bootState } from './boot';

  const res = $derived(resolve($current.match, $session, $current.location.fullPath));

  $effect(() => {
    if (res.kind === 'redirect') navigate(res.to, { replace: true });
  });

  function log(e: unknown) {
    console.error(e);
  }
</script>

<svelte:boundary onerror={log}>
  {#if res.kind === 'render'}
    {@const Layout = res.route.meta?.layout === 'blank' ? BlankLayout : AppLayout}
    {@const View = res.route.view}
    <Layout><View params={res.params} /></Layout>
  {:else if res.kind === 'pending'}
    <BlankLayout>
      {#if $bootState.kind === 'failed'}
        <SessionErrorView error={$bootState.error} onretry={() => void boot()} />
      {:else}
        <PendingView />
      {/if}
    </BlankLayout>
  {:else if res.kind === 'notfound'}
    {@const View = notFoundRoute.view}
    <BlankLayout><View /></BlankLayout>
  {/if}

  {#snippet failed(error, reset)}
    <BlankLayout>
      <h1 class="text-xl font-bold">{$t('error.unexpected.title')}</h1>
      <p class="text-muted">{$t('error.unexpected.description')}</p>
      <ErrorState {error} />
      <div class="flex gap-2">
        <Button onclick={() => location.reload()}>{$t('common.reload')}</Button>
        <Button variant="secondary" onclick={reset}>{$t('common.retry')}</Button>
      </div>
    </BlankLayout>
  {/snippet}
</svelte:boundary>

<ToastHost />
