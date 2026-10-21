import { z } from 'zod';
import type { Identity } from '../../../../shared/stores/session';

export const MeSchema = z.object({
  id: z.string().uuid(),
  email: z.string(),
  name: z.string(),
  role: z.enum(['super_admin', 'developer']),
  must_change_password: z.boolean(),
  email_enabled: z.boolean(),
});
export type Me = z.infer<typeof MeSchema>;

// Respons login tidak membawa email_enabled, jadi hanya field user yang dibutuhkan.
export function toIdentity(m: Omit<Me, 'email_enabled'>): Identity {
  return { userId: m.id, email: m.email, name: m.name, role: m.role, mustChangePassword: m.must_change_password };
}
