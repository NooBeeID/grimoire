import { z } from 'zod';
import { EmailStatusSchema, TemporaryCredentialsSchema } from '../../../../shared/validation/credentials';

export const MEMBER_ROLES = ['owner', 'member'] as const;
export type MemberRole = (typeof MEMBER_ROLES)[number];

// Mengikuti Member/MemberList/MemberCandidateList di api/modules/project/management.yml.
export const MemberSchema = z.object({
  user_id: z.string().uuid(),
  name: z.string().nullable(),
  email: z.string().nullable(),
  is_active: z.boolean(),
  found: z.boolean(),
  role: z.enum(MEMBER_ROLES),
  added_at: z.string().datetime({ offset: true }),
  added_by: z.string().nullable(),
  added_by_label: z.string().nullable(),
  invitation_pending: z.boolean(),
  invitation_expires_at: z.string().datetime({ offset: true }).nullable(),
});
export const MemberListSchema = z.object({ items: z.array(MemberSchema) });
export const CandidateSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  email: z.string(),
  invitation_pending: z.boolean(),
});
export const CandidateListSchema = z.object({ items: z.array(CandidateSchema) });

// Mengikuti Invitation/InviteResponse di api/modules/project/management.yml (kontrak T12).
export const InvitationSchema = z.object({
  user_id: z.string().uuid(),
  email: z.string(),
  name: z.string(),
  role: z.enum(MEMBER_ROLES),
  invited_by_label: z.string().nullable(),
  added_at: z.string().datetime({ offset: true }),
  sent_at: z.string().datetime({ offset: true }).nullable(),
  expires_at: z.string().datetime({ offset: true }),
  expired: z.boolean(),
  delivery_status: EmailStatusSchema,
  email_failed: z.boolean(),
  resend_count: z.number().int().nonnegative(),
});
export const InvitationListSchema = z.object({ items: z.array(InvitationSchema) });
export const InviteResponseSchema = z.object({
  member: MemberSchema,
  created: z.boolean(),
  email_sent: z.boolean(),
  email_status: EmailStatusSchema,
  expires_at: z.string().datetime({ offset: true }),
  credentials: TemporaryCredentialsSchema.optional(),
});
export const ResendResponseSchema = InviteResponseSchema.pick({
  email_sent: true,
  email_status: true,
  expires_at: true,
  credentials: true,
});

export type Invitation = z.infer<typeof InvitationSchema>;
export type InviteResponse = z.infer<typeof InviteResponseSchema>;
export type ResendResponse = z.infer<typeof ResendResponseSchema>;
export interface InviteInput {
  email: string;
  role: MemberRole;
  name: string;
}
export const MAX_EMAIL = 254;
export const MAX_NAME = 100;

export type Member = z.infer<typeof MemberSchema>;
export type Candidate = z.infer<typeof CandidateSchema>;
export type AccessLevel = 'owner' | 'member' | 'admin'; // salinan lokal

export const CANDIDATE_LIMIT = 10;
export const CANDIDATE_DEBOUNCE_MS = 250;
export const MAX_CANDIDATE_QUERY = 100;
export const KNOWN_REASONS = ['LAST_OWNER', 'ALREADY_MEMBER', 'USER_NOT_FOUND', 'USER_NOT_DEVELOPER', 'USER_INACTIVE', 'INVITATION_NOT_PENDING'] as const;
