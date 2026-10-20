<script lang="ts">
  import { get } from 'svelte/store';
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { session, setIdentity } from '../../../../../shared/stores/session';
  import { toast } from '../../../../../shared/stores/toast';
  import ChangePasswordForm from '../components/ChangePasswordForm.svelte';
  import { changePassword } from '../services/changepassword.service';
  import { toChangePasswordError, type ChangePasswordErrorView, type ChangePasswordInput } from '../types';

  let { mode, onlogout }: { mode: 'forced' | 'profile'; onlogout?: () => void } = $props();

  let submitting = $state(false);
  let error = $state<ChangePasswordErrorView | null>(null);
  let cooldown = $state(0);
  let resetKey = $state(0);
  let timer: ReturnType<typeof setInterval> | null = null;

  function stopTimer() {
    if (timer !== null) clearInterval(timer);
    timer = null;
  }
  $effect(() => stopTimer);

  function startCooldown(seconds: number) {
    stopTimer();
    cooldown = seconds;
    timer = setInterval(() => {
      cooldown -= 1;
      if (cooldown <= 0) {
        cooldown = 0;
        stopTimer();
      }
    }, 1000);
  }

  async function onsubmit(input: ChangePasswordInput) {
    submitting = true;
    error = null;
    try {
      await changePassword(input);
      const id = get(session).identity;
      if (id) setIdentity({ ...id, mustChangePassword: false });
      toast.push({ type: 'success', message: $t('auth.changePassword.success') });
      resetKey += 1;
    } catch (e) {
      error = toChangePasswordError(e);
      if (error.kind === 'rateLimited') startCooldown(error.retryAfterSeconds ?? 60);
    } finally {
      submitting = false;
    }
  }
</script>

<section class="mx-auto flex max-w-sm flex-col gap-4">
  {#if mode === 'forced'}
    <h1 class="text-2xl font-bold">{$t('auth.changePassword.forcedTitle')}</h1>
    <p class="text-muted">{$t('auth.changePassword.forcedDescription')}</p>
  {:else}
    <h2 class="text-lg font-semibold">{$t('auth.changePassword.forcedTitle')}</h2>
  {/if}
  <ChangePasswordForm {submitting} {error} cooldownSeconds={cooldown} {resetKey} {onsubmit} />
  {#if mode === 'forced' && onlogout}
    <Button variant="ghost" onclick={onlogout}>{$t('nav.logout')}</Button>
  {/if}
</section>
