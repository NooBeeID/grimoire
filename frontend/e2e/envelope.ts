import { z } from 'zod';

export const ErrorEnvelopeForE2E = z.object({
  error: z.object({ code: z.string().min(1), message: z.string(), details: z.unknown().nullable().optional() }),
  trace_id: z.string(),
});
