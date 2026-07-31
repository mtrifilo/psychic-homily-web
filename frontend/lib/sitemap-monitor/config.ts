/**
 * Monitor configuration, resolved from the environment.
 *
 * Every threshold is configurable and nothing about the comparison is
 * hardcoded, per pattern_infra_assumptions_must_be_measured.md: the numbers
 * below are a conservative STARTING POINT chosen to avoid false alarms, not
 * measured limits. Narrow them once there is a baseline of real variance —
 * show counts move constantly as events pass and get ingested, and a bulk
 * release import moves `releases` by thousands in one run.
 *
 * Parsing is strict and fails closed. A typo'd `SITEMAP_MONITOR_DRIFT_RATIO=0.2x`
 * silently coercing to NaN would make every comparison pass, turning the
 * monitor into a green light that checks nothing — the exact shape of the
 * incident this exists to catch.
 */

/** Defaults are documented next to each field in `resolveConfig`. */
export interface MonitorConfig {
  /** Origin whose sitemap is being checked, e.g. `https://psychichomily.com`. */
  target: string
  /** API origin serving `GET /sitemap/entries`. */
  apiBase: string
  /** Entry point path, relative to `target`. */
  entryPath: string
  /**
   * Allowed relative drift between sitemap and API counts, as a fraction of
   * the API count. Applied in both directions: under-coverage means URLs are
   * missing, over-coverage means the sitemap advertises entries the API no
   * longer has.
   */
  driftRatio: number
  /**
   * Absolute drift always tolerated, regardless of `driftRatio`.
   *
   * Small families need this: `festivals` has 10 entries, so a single row
   * added between the sitemap's hourly revalidate and this check is a 10%
   * drift on its own. Without a floor the small families would alarm
   * constantly and the whole monitor would get muted.
   */
  driftFloor: number
  /**
   * Minimum number of shows dated today or later that must appear.
   *
   * The incident served 114 correctly-formed show URLs, all historical — a
   * pure count check passed it. This is the assertion that would not have.
   */
  minFutureShows: number
  /** How many `<loc>` values to fetch and assert reachable. */
  sampleSize: number
  /** Per-request timeout for sitemap/API fetches. */
  fetchTimeoutMs: number
  /** Per-request timeout for the sampled URL probes. */
  sampleTimeoutMs: number
  /** Discord webhook to alert on. Empty disables alerting (the check still runs). */
  discordWebhookUrl: string
  /** Sent as `x-vercel-protection-bypass`, for checking an SSO-gated preview. */
  vercelBypassToken: string
  /** Post to Discord even when the check passes. Used to prove the webhook works. */
  notifyOnSuccess: boolean
}

export type Env = Record<string, string | undefined>

function readString(env: Env, key: string, fallback: string): string {
  const raw = env[key]
  return raw === undefined || raw.trim() === '' ? fallback : raw.trim()
}

function readNumber(
  env: Env,
  key: string,
  fallback: number,
  { min, max }: { min: number; max: number }
): number {
  const raw = env[key]
  if (raw === undefined || raw.trim() === '') return fallback

  // Number() rather than parseFloat(): parseFloat('0.2x') returns 0.2, which
  // would accept a malformed threshold instead of reporting it.
  const value = Number(raw.trim())
  if (!Number.isFinite(value)) {
    throw new Error(`${key} must be a number, got ${JSON.stringify(raw)}`)
  }
  if (value < min || value > max) {
    throw new Error(`${key} must be between ${min} and ${max}, got ${value}`)
  }
  return value
}

function readBoolean(env: Env, key: string, fallback: boolean): boolean {
  const raw = env[key]
  if (raw === undefined || raw.trim() === '') return fallback
  const value = raw.trim().toLowerCase()
  if (value === 'true' || value === '1') return true
  if (value === 'false' || value === '0') return false
  throw new Error(`${key} must be true or false, got ${JSON.stringify(raw)}`)
}

/** Strip a trailing slash so `${origin}${path}` never doubles up. */
function normalizeOrigin(value: string, key: string): string {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    throw new Error(`${key} must be an absolute URL, got ${JSON.stringify(value)}`)
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error(`${key} must be an http(s) URL, got ${JSON.stringify(value)}`)
  }
  return value.replace(/\/+$/, '')
}

export function resolveConfig(env: Env): MonitorConfig {
  return {
    target: normalizeOrigin(
      readString(env, 'SITEMAP_MONITOR_TARGET', 'https://psychichomily.com'),
      'SITEMAP_MONITOR_TARGET'
    ),
    apiBase: normalizeOrigin(
      readString(env, 'SITEMAP_MONITOR_API_BASE', 'https://api.psychichomily.com'),
      'SITEMAP_MONITOR_API_BASE'
    ),
    // robots.txt advertises /sitemap-index; /sitemap.xml 308s here for old
    // crawlers. Configurable so the monitor can be pointed at either.
    entryPath: readString(env, 'SITEMAP_MONITOR_ENTRY_PATH', '/sitemap-index'),
    // 20%: wide enough to absorb the sitemap's 1h revalidate window plus an
    // ingest run landing between the two reads. The incident was a 97% gap.
    driftRatio: readNumber(env, 'SITEMAP_MONITOR_DRIFT_RATIO', 0.2, {
      min: 0,
      max: 1,
    }),
    driftFloor: readNumber(env, 'SITEMAP_MONITOR_DRIFT_FLOOR', 10, {
      min: 0,
      max: 1_000_000,
    }),
    // Low on purpose. This is a liveness assertion, not a coverage one: any
    // healthy catalogue has hundreds of upcoming shows, so a handful is enough
    // to separate "working" from "serving only history" without alarming
    // during a genuinely quiet stretch.
    minFutureShows: readNumber(env, 'SITEMAP_MONITOR_MIN_FUTURE_SHOWS', 10, {
      min: 0,
      max: 1_000_000,
    }),
    sampleSize: readNumber(env, 'SITEMAP_MONITOR_SAMPLE_SIZE', 10, {
      min: 0,
      max: 200,
    }),
    fetchTimeoutMs: readNumber(env, 'SITEMAP_MONITOR_FETCH_TIMEOUT_MS', 30_000, {
      min: 1_000,
      max: 300_000,
    }),
    sampleTimeoutMs: readNumber(env, 'SITEMAP_MONITOR_SAMPLE_TIMEOUT_MS', 15_000, {
      min: 1_000,
      max: 300_000,
    }),
    discordWebhookUrl: readString(env, 'DISCORD_WEBHOOK_URL', ''),
    vercelBypassToken: readString(env, 'VERCEL_PROTECTION_BYPASS', ''),
    notifyOnSuccess: readBoolean(env, 'SITEMAP_MONITOR_NOTIFY_ON_SUCCESS', false),
  }
}
