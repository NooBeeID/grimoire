import { z } from 'zod';
import { GitSyncConfigSchema, KINDS } from '../base';

export const GetConfigResponseSchema = z.object({
  kind: z.enum(KINDS),
  feature_enabled: z.boolean(),
  has_credential: z.boolean(),
  credential_unreadable: z.boolean(),
  config: GitSyncConfigSchema.nullable(),
  in_progress: z.boolean().default(false),
});
export type GetConfigResponse = z.infer<typeof GetConfigResponseSchema>;
