<script lang="ts">
  import { setIdentity } from '../../../../../shared/stores/session';
  import { t } from '../../../../../shared/stores/i18n';
  import LoginForm from '../components/LoginForm.svelte';
  import { login } from '../services/login.service';
  import { toLoginError, type LoginErrorView, type LoginInput } from '../types';

  const DEFAULT_COOLDOWN = 60;

  let submitting = $state(false);
  let error = $state<LoginErrorView | null>(null);
  let cooldown = $state(0);
  let timer: ReturnType<typeof setInterval> | null = null;

  function stopTimer() {
    if (timer !== null) clearInterval(timer);
    timer = null;
  }

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

  $effect(() => stopTimer);

  async function onsubmit(input: LoginInput) {
    submitting = true;
    error = null;
    try {
      // Navigasi diturunkan dari state sesi oleh guard di app/.
      setIdentity(await login(input));
    } catch (e) {
      error = toLoginError(e);
      if (error.kind === 'rateLimited') startCooldown(error.retryAfterSeconds ?? DEFAULT_COOLDOWN);
    } finally {
      submitting = false;
    }
  }
</script>

<section class="flex flex-col gap-4">
  <h1 class="text-[22px] leading-8 font-semibold">{$t('auth.login.title')}</h1>
  <LoginForm {submitting} {error} cooldownSeconds={cooldown} {onsubmit} />
</section>
