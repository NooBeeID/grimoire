<script lang="ts">
  import { ChangePasswordView } from '../../domain/core/auth/changepassword';
  import { logoutAndLeave } from '../boot';
</script>

<ChangePasswordView mode="forced" onlogout={() => void logoutAndLeave()} />
