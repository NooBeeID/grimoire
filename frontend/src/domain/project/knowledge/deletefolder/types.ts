import { z } from 'zod';

export const DeleteFolderResultSchema = z.object({ deleted: z.number().int().nonnegative() });
export type DeleteFolderResult = z.infer<typeof DeleteFolderResultSchema>;
