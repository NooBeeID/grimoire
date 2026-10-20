<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { ChangePasswordErrorView, ChangePasswordInput } from '../types';

  let {
    submitting = false,
    error = null,
    cooldownSeconds = 0,
    resetKey = 0,
    onsubmit,
  }: {
    submitting?: boolean;
    error?: ChangePasswordErrorView | null;
    cooldownSeconds?: number;
    resetKey?: number;
    onsubmit: (input: ChangePasswordInput) => void;
  } = $props();

  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let local = $state<Record<string, string>>({});

  let lastKey: number | undefined;
  $effect(() => {
    if (lastKey !== undefined && resetKey !== lastKey) {
      current = next = confirm = '';
      local = {};
    }
    lastKey = resetKey;
  });

  const fieldErr = (n: string) => local[n] ?? (error?.kind === 'fields' ? error.fields[n] : undefined);
  const message = $derived.by(() => {
    if (error?.kind === 'conflict') return $t('auth.changePassword.error.conflict');
    if (error?.kind === 'rateLimited') return $t('auth.login.error.rateLimited');
    return null;
  });

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (current === '') errs.current_password = $t('form.required');
    if (next === '') errs.new_password = $t('form.required');
    if (confirm === '') errs.confirm_password = $t('form.required');
    if (!errs.confirm_password && !errs.new_password && confirm !== next) errs.confirm_password = $t('auth.changePassword.error.mismatch');
    if (!errs.new_password && !errs.current_password && next === current) errs.new_password = $t('auth.changePassword.error.sameAsCurrent');
    local = errs;
    if (Object.keys(errs).length > 0) return;
    onsubmit({ current_password: current, new_password: next });
  }
</script>

<form onsubmit={submit} novalidate class="flex flex-col gap-4">
  <Input bind:value={current} type="password" name="current_password" label={$t('auth.changePassword.current')} autocomplete="current-password" required disabled={submitting} error={fieldErr('current_password')} />
  <Input bind:value={next} type="password" name="new_password" label={$t('auth.changePassword.new')} autocomplete="new-password" hint={$t('auth.changePassword.hint')} required disabled={submitting} error={fieldErr('new_password')} />
  <Input bind:value={confirm} type="password" name="confirm_password" label={$t('auth.changePassword.confirm')} autocomplete="new-password" required disabled={submitting} error={fieldErr('confirm_password')} />
  {#if message}
    <p role="alert" class="text-sm text-danger">{message}</p>
  {:else if error?.kind === 'generic'}
    <div role="alert" class="text-sm text-danger">
      <p>{error.message}</p>
      {#if error.traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: error.traceId })}</p>{/if}
    </div>
  {/if}
  <Button type="submit" loading={submitting} disabled={cooldownSeconds > 0}>
    {cooldownSeconds > 0 ? $t('auth.login.wait', { seconds: cooldownSeconds }) : $t('auth.changePassword.submit')}
  </Button>
</form>
