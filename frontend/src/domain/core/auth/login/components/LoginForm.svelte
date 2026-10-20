<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { LoginErrorView, LoginInput } from '../types';

  let {
    submitting = false,
    error = null,
    cooldownSeconds = 0,
    onsubmit,
  }: {
    submitting?: boolean;
    error?: LoginErrorView | null;
    cooldownSeconds?: number;
    onsubmit: (input: LoginInput) => void;
  } = $props();

  let email = $state('');
  let password = $state('');
  let missing = $state<{ email?: boolean; password?: boolean }>({});
  let root: HTMLFormElement | undefined = $state();

  $effect(() => {
    if (error?.kind === 'invalid') {
      password = '';
      root?.querySelector<HTMLInputElement>('input[type="password"]')?.focus();
    }
  });

  const message = $derived.by(() => {
    if (!error) return null;
    switch (error.kind) {
      case 'invalid':
        return $t('auth.login.error.invalid');
      case 'locked':
        return error.retryAfterSeconds === null
          ? $t('auth.login.error.lockedNoTime')
          : $t('auth.login.error.locked', { minutes: Math.max(1, Math.ceil(error.retryAfterSeconds / 60)) });
      case 'rateLimited':
        return $t('auth.login.error.rateLimited');
      default:
        return null;
    }
  });
  const fieldErr = (name: string) => (error?.kind === 'fields' ? error.fields[name] : undefined);

  function submit(e: SubmitEvent) {
    e.preventDefault();
    missing = { email: email.trim() === '', password: password === '' };
    if (missing.email || missing.password) return;
    onsubmit({ email, password });
  }
</script>

<form bind:this={root} onsubmit={submit} novalidate class="flex flex-col gap-4">
  <Input
    bind:value={email}
    type="email"
    name="email"
    label={$t('auth.login.email')}
    autocomplete="username"
    required
    disabled={submitting}
    error={missing.email ? $t('form.required') : fieldErr('email')}
  />
  <Input
    bind:value={password}
    type="password"
    name="password"
    label={$t('auth.login.password')}
    autocomplete="current-password"
    required
    disabled={submitting}
    error={missing.password ? $t('form.required') : fieldErr('password')}
  />
  {#if message}
    <p role="alert" class="text-sm text-danger">{message}</p>
  {:else if error?.kind === 'generic'}
    <div role="alert" class="text-sm text-danger">
      <p>{error.message}</p>
      {#if error.traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: error.traceId })}</p>{/if}
    </div>
  {/if}
  <Button type="submit" loading={submitting} disabled={cooldownSeconds > 0}>
    {cooldownSeconds > 0 ? $t('auth.login.wait', { seconds: cooldownSeconds }) : $t('auth.login.submit')}
  </Button>
</form>
