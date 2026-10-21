import { z } from 'zod';

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
});
export const MemberListSchema = z.object({ items: z.array(MemberSchema) });
export const CandidateSchema = z.object({ id: z.string().uuid(), name: z.string(), email: z.string() });
export const CandidateListSchema = z.object({ items: z.array(CandidateSchema) });

export type Member = z.infer<typeof MemberSchema>;
export type Candidate = z.infer<typeof CandidateSchema>;
export type AccessLevel = 'owner' | 'member' | 'admin'; // salinan lokal

export const CANDIDATE_LIMIT = 10;
export const CANDIDATE_DEBOUNCE_MS = 250;
export const MAX_CANDIDATE_QUERY = 100;
export const KNOWN_REASONS = ['LAST_OWNER', 'ALREADY_MEMBER', 'USER_NOT_FOUND', 'USER_NOT_DEVELOPER', 'USER_INACTIVE'] as const;
