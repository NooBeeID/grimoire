import { z } from 'zod';

export const SUMMARY_RANGES = ['24h', '7d', '30d'] as const;
export type SummaryRange = (typeof SUMMARY_RANGES)[number];
export const DEFAULT_SUMMARY_RANGE: SummaryRange = '7d';

export interface SummaryFilter {
  range: SummaryRange;
  projectId: string | null;
}

export const TopProjectSchema = z
  .object({ project_id: z.string().uuid(), project_label: z.string(), calls: z.number().int().min(0) })
  .strict();
export const ToolUsageSchema = z.object({ tool: z.string(), calls: z.number().int().min(0) }).strict();
export const ToolMetricSchema = z
  .object({
    tool: z.string(),
    calls: z.number().int().min(1),
    ok: z.number().int().min(0),
    error: z.number().int().min(0),
    error_rate: z.number(),
    p50_ms: z.number().nullable(),
    p95_ms: z.number().nullable(),
  })
  .strict();
export const TrafficPointSchema = z
  .object({ at: z.string().datetime({ offset: true }), calls: z.number().int().min(0), errors: z.number().int().min(0) })
  .strict();
export const QualityMetricSchema = z
  .object({ count: z.number().int().min(0), denominator: z.number().int().min(0), rate: z.number() })
  .strict();
export const RankBucketSchema = z.object({ label: z.string(), count: z.number().int().min(0) }).strict();

export const SearchQualityAvailableSchema = z
  .object({
    available: z.literal(true),
    searches: z.number().int().min(0),
    window_seconds: z.number().int().min(0),
    zero_result: QualityMetricSchema,
    follow_up: QualityMetricSchema,
    reformulation: QualityMetricSchema,
    rank_opened: z.array(RankBucketSchema),
    legacy_searches: z.number().int().min(0),
  })
  .strict();
export const SearchQualityUnavailableSchema = z
  .object({ available: z.literal(false), error_code: z.literal('UNAVAILABLE') })
  .strict();

export const UsageSummarySchema = z
  .object({
    range: z.enum(SUMMARY_RANGES),
    from: z.string().datetime({ offset: true }),
    to: z.string().datetime({ offset: true }),
    bucket: z.enum(['hour', 'day']),
    project_id: z.string().uuid().nullable(),
    total_calls: z.number().int().min(0),
    total_errors: z.number().int().min(0),
    error_rate: z.number().nullable(),
    top_projects: z.array(TopProjectSchema).max(10),
    tool_usage: z.array(ToolUsageSchema),
    tool_metrics: z.array(ToolMetricSchema),
    traffic: z.array(TrafficPointSchema),
    search_quality: z.union([z.null(), SearchQualityAvailableSchema, SearchQualityUnavailableSchema]),
  })
  .strict();

export const ProjectOptionSchema = z.object({ id: z.string().uuid(), name: z.string() }).passthrough();
export const ProjectOptionListSchema = z
  .object({ items: z.array(ProjectOptionSchema), total: z.number().int() })
  .passthrough();

export type UsageSummary = z.infer<typeof UsageSummarySchema>;
export type ToolMetric = z.infer<typeof ToolMetricSchema>;
export type TrafficPoint = z.infer<typeof TrafficPointSchema>;
export type TopProject = z.infer<typeof TopProjectSchema>;
export type SearchQuality = NonNullable<UsageSummary['search_quality']>;
export type SearchQualityAvailable = z.infer<typeof SearchQualityAvailableSchema>;
export type ProjectOption = z.infer<typeof ProjectOptionSchema>;
