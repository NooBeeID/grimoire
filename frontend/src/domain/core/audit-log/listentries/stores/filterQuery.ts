import {
  ACTION_RE,
  ACTOR_TYPES,
  AUDIT_STATUSES,
  EMPTY_FILTER,
  MAX_ACTIONS,
  MAX_TEXT,
  RANGE_PRESETS,
  UUID_RE,
  type ActorType,
  type AuditFilter,
  type AuditStatus,
  type RangeKey,
} from '../types';

const FILTER_KEYS = [
  'actor_type',
  'actor_id',
  'actor_label',
  'project_id',
  'action',
  'status',
  'range',
  'from',
  'to',
  'target',
];
const PRESET_KEYS: string[] = RANGE_PRESETS.map((p) => p.key);

function validIso(v: string | null): v is string {
  return !!v && !Number.isNaN(Date.parse(v));
}

export function filterFromQuery(q: URLSearchParams): AuditFilter {
  const f: AuditFilter = { ...EMPTY_FILTER, actions: [], statuses: [] };
  const at = q.get('actor_type');
  if (at && (ACTOR_TYPES as readonly string[]).includes(at)) f.actorType = at as ActorType;
  const aid = q.get('actor_id');
  if (aid && UUID_RE.test(aid)) f.actorId = aid;
  f.actorLabel = (q.get('actor_label') ?? '').slice(0, MAX_TEXT);
  const pid = q.get('project_id');
  if (pid && UUID_RE.test(pid)) f.projectId = pid;
  f.actions = [...new Set(q.getAll('action').filter((a) => ACTION_RE.test(a)))].slice(0, MAX_ACTIONS);
  f.statuses = [...new Set(q.getAll('status'))].filter((s): s is AuditStatus =>
    (AUDIT_STATUSES as readonly string[]).includes(s),
  );
  f.target = (q.get('target') ?? '').slice(0, MAX_TEXT);
  const from = q.get('from');
  const to = q.get('to');
  const range = q.get('range');
  if (validIso(from) || validIso(to)) {
    f.range = 'custom';
    f.from = validIso(from) ? from : '';
    f.to = validIso(to) ? to : '';
  } else if (range && PRESET_KEYS.includes(range)) {
    f.range = range as RangeKey;
  }
  return f;
}

export function filterToQuery(f: AuditFilter, base?: URLSearchParams): URLSearchParams {
  const out = new URLSearchParams();
  if (base) {
    for (const [k, v] of base) if (!FILTER_KEYS.includes(k) && k !== 'entry') out.append(k, v);
  }
  const filterParams = new URLSearchParams();
  if (f.actorType) filterParams.append('actor_type', f.actorType);
  if (f.actorId) filterParams.append('actor_id', f.actorId);
  if (f.actorLabel) filterParams.append('actor_label', f.actorLabel);
  if (f.projectId) filterParams.append('project_id', f.projectId);
  for (const a of f.actions) filterParams.append('action', a);
  for (const s of f.statuses) filterParams.append('status', s);
  if (f.range === 'custom') {
    if (f.from) filterParams.append('from', f.from);
    if (f.to) filterParams.append('to', f.to);
  } else if (f.range !== 'all') {
    filterParams.append('range', f.range);
  }
  if (f.target) filterParams.append('target', f.target);
  for (const [k, v] of filterParams) out.append(k, v);
  return out;
}

export function filterKey(f: AuditFilter): string {
  return JSON.stringify([
    f.actorType,
    f.actorId,
    f.actorLabel,
    f.projectId,
    [...f.actions].sort(),
    [...f.statuses].sort(),
    f.range,
    f.range === 'custom' ? f.from : '',
    f.range === 'custom' ? f.to : '',
    f.target,
  ]);
}

export function validateFilter(f: AuditFilter): Partial<Record<keyof AuditFilter, string>> {
  const e: Partial<Record<keyof AuditFilter, string>> = {};
  if (f.actorId && !UUID_RE.test(f.actorId)) e.actorId = 'auditLog.filter.actorInvalid';
  if (f.projectId && !UUID_RE.test(f.projectId)) e.projectId = 'auditLog.filter.projectInvalid';
  if (f.actorLabel.length > MAX_TEXT) e.actorLabel = 'auditLog.filter.textTooLong';
  if (f.target.length > MAX_TEXT) e.target = 'auditLog.filter.textTooLong';
  if (f.actions.length > MAX_ACTIONS) e.actions = 'auditLog.filter.tooManyActions';
  if (f.range === 'custom' && f.from && f.to && !(Date.parse(f.from) < Date.parse(f.to))) {
    e.range = 'auditLog.filter.rangeInvalid';
  }
  return e;
}
