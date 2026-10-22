<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { fieldErrorsOf, retryAfterOf } from '../../../../../shared/validation/error';
  import SaveTokenForm from '../components/SaveTokenForm.svelte';
  import { saveCredential } from '../services/savecredential.service';
  import { validateToken, type GitProvider, type SavedCredential, type SaveTarget } from '../types';

  let {
    open,
    target,
    onclose,
    onsaved,
  }: {
    open: boolean;
    target: SaveTarget;
    onclose: () => void;
    onsaved: (c: SavedCredential) => void;
  } = $props();

  // Token hanya hidup di $state ini: dikosongkan saat dibuka, saat sukses, dan saat ditutup.
  let provider = $state<GitProvider>('github');
  let host = $state('');
  let token = $state('');
  let submitting = $state(false);
  let errors = $state<{ token?: string; host?: string }>({});
  let formError = $state<string | null>(null);
  let traceId = $state<string | undefined>();
  let locked = $state(false);

  $effect(() => {
    if (open) {
      provider = target.provider ?? 'github';
      host = target.host ?? target.gitlabHosts[0] ?? '';
      token = '';
      errors = {};
      formError = null;
      traceId = undefined;
      locked = false;
    } else {
      token = '';
    }
  });

  const providerName = $derived($t(`gitCredential.provider.${provider}`));
  const title = $derived(
    target.provider && target.host ? $t('gitCredential.save.titleReplace', { provider: $t(`gitCredential.provider.${target.provider}`), host: target.host }) : $t('gitCredential.save.title'),
  );

  async function submit() {
    if (submitting || locked) return;
    errors = {};
    formError = null;
    traceId = undefined;
    const problem = validateToken(token);
    if (problem) {
      errors = { token: $t(`gitCredential.save.validation.${problem}`) };
      return;
    }
    if (provider === 'gitlab' && !host) {
      errors = { host: $t('gitCredential.save.validation.host') };
      return;
    }
    submitting = true;
    try {
      const saved = await saveCredential(provider, { token, host: provider === 'gitlab' ? host : undefined });
      token = '';
      onsaved(saved);
      onclose();
      toast.push({ type: 'success', message: $t('gitCredential.save.saved', { provider: providerName }) });
    } catch (e) {
      if (!isApiError(e)) {
        formError = $t('gitCredential.save.error.generic');
        return;
      }
      traceId = e.traceId;
      switch (e.code) {
        case 'TOKEN_INVALID':
          errors = { token: $t('gitCredential.save.error.TOKEN_INVALID') };
          traceId = undefined;
          break;
        case 'HOST_NOT_ALLOWED':
          errors = { host: $t('gitCredential.save.error.HOST_NOT_ALLOWED', { key: 'GIT_SYNC_ALLOWED_HOSTS' }) };
          traceId = undefined;
          break;
        case 'INVALID_ARGUMENT': {
          const f = fieldErrorsOf(e.details);
          errors = { token: f.token, host: f.host };
          if (!f.token && !f.host) formError = e.message;
          break;
        }
        case 'RATE_LIMITED': {
          const s = retryAfterOf(e.details);
          formError = s === null ? $t('gitCredential.save.error.RATE_LIMITED') : $t('gitCredential.save.error.RATE_LIMITED_RETRY', { seconds: s });
          break;
        }
        case 'GIT_FEATURE_DISABLED':
          formError = $t('gitCredential.save.error.GIT_FEATURE_DISABLED');
          locked = true;
          break;
        case 'GIT_UPSTREAM_ERROR':
          formError = $t('gitCredential.save.error.GIT_UPSTREAM_ERROR');
          break;
        default:
          formError = $t('gitCredential.save.error.generic');
      }
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog {open} {title} dismissible={!submitting} {onclose}>
  <SaveTokenForm
    bind:provider
    bind:host
    bind:token
    providerLocked={target.provider !== null}
    gitlabHosts={target.gitlabHosts}
    disabled={submitting || locked}
    {errors}
    onsubmit={submit}
  />
  {#if formError}
    <p role="alert" class="mt-3 text-sm text-danger">
      {formError}
      {#if traceId}<span class="block text-xs text-muted">{$t('error.trace_id', { id: traceId })}</span>{/if}
    </p>
  {/if}
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button variant="primary" loading={submitting} disabled={locked} onclick={submit}>
      {submitting ? $t('gitCredential.save.verifying', { provider: providerName }) : $t('gitCredential.save.submit')}
    </Button>
  {/snippet}
</Dialog>
