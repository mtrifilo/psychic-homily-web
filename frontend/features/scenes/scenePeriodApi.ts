/**
 * Reading one PERIOD of a scene's calendar from the API — a week, a night —
 * and nothing else.
 *
 * A leaf module by design, importing nothing but the API base: the weekly share
 * card renders on the edge runtime, and reaching this through a page module
 * would drag the page view and its JSON-LD helpers into that bundle for no
 * reason.
 *
 * The two-phase freshness probe below is the subtlest thing in this feature,
 * and it rests on an observation about a specific Next version. It lives here,
 * once, so that observation has ONE place to be re-verified on an upgrade and
 * one place to be corrected — a second copy would go silently wrong the first
 * time only one of them was fixed.
 */
import * as Sentry from '@sentry/nextjs'

/**
 * How long a fetched period stays fresh.
 *
 * A product decision, not a tuning detail. A period that can still gain shows —
 * the current one, and any future one — has to stay fresh, because the count is
 * the whole point of the page. Only a period that has actually ended is
 * immutable.
 *
 * Note what does NOT decide this: the URL shape. A keyed permalink serves the
 * LIVE period as often as an ended one, because the same key names the current
 * week or night while it is still running. `fetchScenePeriod` picks between
 * these two from the payload itself, never from the URL. (Do not re-derive this
 * from canonical tags: /tonight canonicalizes to the week permalink and a dated
 * day permalink to itself, and neither fact says anything about freshness.)
 */
export const CURRENT_PERIOD_REVALIDATE = 900
export const ARCHIVED_PERIOD_REVALIDATE = 86400

/** Which surface a failure came from, so Sentry triage can tell them apart. */
export type ScenePeriodService = 'scene-week' | 'scene-day' | 'og-image'

interface ScenePeriodSpec<T> {
  /** Human label for the Sentry message, e.g. `Scene week`. */
  label: string
  /**
   * WHICH scene this is about — the first thing triage needs, and the one thing
   * the period key alone cannot say. Carried explicitly because the slug is
   * otherwise closed over inside `buildUrl` and would vanish from every report.
   */
  slug: string
  /** The API URL for this period key, or for the CURRENT period when omitted. */
  buildUrl: (key: string | undefined) => string
  /**
   * Fields that NAME the period, each paired with the shape its value must
   * have.
   *
   * Every value must first name something: present, not empty, and carrying no
   * leading or trailing space. A blank one is worse than an absent one, because
   * it survives every truthiness check downstream and collapses `/scenes/x/y`
   * shapes into `/scenes//`, which names a different page; an untrimmed one
   * names a different page outright.
   *
   * The paired predicate then says what KIND of name it has to be, because
   * naming something is not the same as naming something of the right kind:
   * `date: "tonight"` and `slug: "../.."` both name something and both reach a
   * URL interpolation that has no second chance to check them.
   *
   * Shape is still not existence. `2026-02-30` and `2025-W53` pass every
   * predicate here and are not real periods; only the backend, which owns the
   * calendar maths and the scene's timezone, can say so.
   *
   * Pairs rather than a record so a misspelled or misplaced field is a compile
   * error at the `fetchScenePeriod<T>` call rather than a silently disabled
   * check.
   */
  identityFields: readonly (readonly [keyof T & string, (value: string) => boolean])[]
  /**
   * Fields whose EMPTY value is itself an answer. `''` passes; anything else
   * must name something, on the same terms as an identity field. A body that
   * omits one is not this payload.
   */
  presenceFields: readonly (keyof T & string)[]
  /** Reads the payload's "this period has ended" flag. */
  isFrozen: (payload: T) => boolean
  service: ScenePeriodService
}

/**
 * Does this wire value name something?
 *
 * Trimmed rather than merely non-empty, because these values are interpolated
 * into URLs: `" phoenix-az"` and `"phoenix-az"` are different addresses, and
 * only one of them is a page.
 */
function namesSomething(value: unknown): value is string {
  return typeof value === 'string' && value !== '' && value === value.trim()
}

/**
 * The shape rule for an identity field that is PRINTED, never addressed.
 *
 * Naming something is the whole requirement for such a field, and stating that
 * as its own predicate is what keeps every entry in `identityFields` an
 * explicit answer to "what kind of name is this".
 */
export function anyName(): boolean {
  return true
}

/**
 * Accept a 200 body only if it is actually the payload we asked for.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else. Checking the fields
 * a consumer dereferences blindly turns a crash into the ordinary "no data"
 * path; the rest of the payload is already optional-safe.
 */
function asPayload<T>(
  body: unknown,
  spec: ScenePeriodSpec<T>,
  key: string | undefined
): T | null {
  if (!body || typeof body !== 'object') return null
  const record = body as Record<string, unknown>

  // The two lists differ over one value: `''` is an answer for a presence field
  // and no answer at all for an identity field.
  const rejected =
    spec.identityFields.find(([field, hasShape]) => {
      const value = record[field]
      return !namesSomething(value) || !hasShape(value)
    })?.[0] ??
    spec.presenceFields.find(
      field => record[field] !== '' && !namesSomething(record[field])
    )
  if (rejected === undefined) return body as T

  // Reported because nothing else can see this. The response was a 200, so no
  // status check fires; Next stores it for the caller's whole window, so one
  // bad body quietly takes out a period's page, its card and its slice until
  // that window passes. The field name is enough to act on, so the body itself
  // is not sent.
  Sentry.captureMessage(`${spec.label}: rejected a payload on \`${rejected}\``, {
    level: 'error',
    tags: { service: spec.service },
    extra: { slug: spec.slug, key, field: rejected },
  })
  return null
}

/**
 * One request for one period, at one explicitly chosen freshness window.
 *
 * Private: choosing the window is the whole subtlety here, and
 * `fetchScenePeriod` below is the only thing entitled to do it.
 */
async function fetchPayload<T>(
  spec: ScenePeriodSpec<T>,
  key: string | undefined,
  revalidate: number
): Promise<T | null> {
  const url = spec.buildUrl(key)
  try {
    const res = await fetch(url, { next: { revalidate } })
    // `await` is load-bearing, not noise: `return res.json()` inside a try block
    // adopts the promise AFTER the block exits, so a malformed body would reject
    // past this catch and 500 the route instead of reaching the caller's
    // fallback.
    if (res.ok) return asPayload<T>(await res.json(), spec, key)
    // 404 is the expected answer for an unknown slug, a below-threshold scene,
    // or a key that does not exist (2025-W53, 2026-02-30) — not an error worth
    // reporting.
    if (res.status >= 500) {
      Sentry.captureMessage(`${spec.label}: API returned ${res.status}`, {
        level: 'error',
        tags: { service: spec.service },
        extra: { slug: spec.slug, key, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: spec.service },
      extra: { slug: spec.slug, key },
    })
  }
  return null
}

/**
 * Fetch one period of a scene's calendar, cached for exactly as long as that
 * period is allowed to be frozen.
 *
 * `key` is the period's permalink key, or omitted for the scene's CURRENT
 * period. Current is resolved SERVER-side by the backend, in the scene's own
 * timezone — a reader in Berlin and a reader in Chicago must see the same
 * Chicago period, so the client must not compute it. The frozen flag is the
 * same answer from the same authority, and it is the only thing that decides
 * the window here.
 *
 * Why a key sometimes costs two requests. `next: { revalidate }` has to be
 * supplied BEFORE the response exists, so the only way to learn which window a
 * period deserves is to have already asked for it. The long window goes first
 * deliberately: for a period that has ended — the overwhelming majority of
 * keyed URLs, and the ones a crawler walks — that single request is the only
 * one ever made, so an archived period still costs one backend query a day.
 * Only a period that can still gain shows falls through to the second, short
 * ask.
 *
 * Both asks address the same URL and therefore share ONE data-cache entry. Next
 * decides staleness against the window the CALLER passes, not the one the entry
 * was stored with (verified against Next 16.1.4's incremental cache), so the
 * short ask re-reads the backend once that shared entry passes 15 minutes while
 * the long ask goes on hitting it. Net effect: a live period refreshes on the
 * 15-minute cadence whichever URL it is served from, an ended one on the daily
 * cadence. RE-VERIFY THIS on a Next upgrade — it is the one claim here that a
 * framework release can quietly invalidate.
 *
 * A stale probe can only err one way. If it still says "not ended" for a period
 * that just ended, the fall-through costs one extra refresh and the next
 * request sees the corrected flag; it can never report a live period as frozen.
 */
export async function fetchScenePeriod<T>(
  spec: ScenePeriodSpec<T>,
  key: string | undefined
): Promise<T | null> {
  // No key: the caller is asking for whatever period is live, which by
  // definition can still change. Nothing to probe for.
  if (!key) return fetchPayload(spec, undefined, CURRENT_PERIOD_REVALIDATE)

  const archived = await fetchPayload(spec, key, ARCHIVED_PERIOD_REVALIDATE)
  // The flag is read strictly, because this is an untrusted wire payload — the
  // same reason `asPayload` exists. It also gives the right answer while a
  // deploy has a newer frontend talking to a backend that does not send the
  // field yet: absent reads as "might still change", which costs a short window
  // and self-heals.
  if (!archived || spec.isFrozen(archived)) return archived

  // `?? archived` because the period demonstrably exists — the ask above
  // returned it. A blip on this second request must not turn a real page into a
  // 404; slightly older data is the better failure.
  return (await fetchPayload(spec, key, CURRENT_PERIOD_REVALIDATE)) ?? archived
}
