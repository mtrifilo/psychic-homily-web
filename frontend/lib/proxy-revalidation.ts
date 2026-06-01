import * as Sentry from '@sentry/nextjs'
import { safeRevalidatePath } from './revalidate-entity'

/**
 * ISR revalidation rules for mutations routed through the catch-all API
 * proxy (app/api/[...path]/route.ts) — PSY-939.
 *
 * Entity pages are ISR-cached (revalidate: 3600; /explore: 300) and re-seed
 * the TanStack Query cache via prefetchEntity on load. Any mutation that
 * changes data embedded in an ISR payload must revalidate the affected
 * paths, or a reload within the window re-serves pre-mutation data and the
 * save appears lost (PSY-936/PSY-937).
 *
 * Each rule maps (HTTP method + backend path pattern) → the ISR paths made
 * stale by that mutation. Slugs come from, in order of preference:
 *   1. the mutation response body (most backend mutations return the entity)
 *   2. the request path (collection routes are slug-addressed)
 *   3. a backend ID→slug lookup GET (entity GETs accept ID-or-slug)
 *
 * Rules exist ONLY for data that is actually in an ISR payload. Verified
 * non-cases (no rule on purpose): comments/field notes, follows, likes,
 * saved shows, attendance, votes, requests, notification/profile/auth
 * surfaces, and entity tagging → *entity* pages (entity pages client-fetch
 * their tags; only the /tags/{slug} page ISR-caches usage counts).
 *
 * Known gaps (follow-up tickets):
 *   - PSY-940: collections tagged via the generic /entities/collection/...
 *     path (collections GET is slug-only; no ID→slug lookup possible)
 *   - PSY-941: cascade renames (artist rename → show pages embedding the
 *     name) need revalidateTag; out of reach for path-based rules
 *   - Deletes by numeric ID can't revalidate the deleted entity's own detail
 *     page (empty response, entity gone); the soft-404 proxy handles dead
 *     slugs (PSY-897/913/906)
 *   - Admin tag hygiene (merge/snooze/parent/bulk) and radio admin CRUD
 */

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

const MUTATION_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

/** True for HTTP methods that can mutate backend state. */
export function isMutationMethod(method: string): boolean {
  return MUTATION_METHODS.has(method)
}

// ---------------------------------------------------------------------------
// Rule types
// ---------------------------------------------------------------------------

interface RuleContext {
  /** Match of the rule's pattern against the backend path. */
  match: RegExpMatchArray
  /** Parsed JSON response body; undefined when empty or unparseable. */
  body: unknown
  /** Parsed JSON request body; undefined when empty or unparseable. */
  requestBody: unknown
  /** Resolve an entity slug from a numeric ID via a backend GET. */
  lookupSlug: (urlSegment: string, id: string) => Promise<string | undefined>
}

interface RevalidationRule {
  /** Identifies the rule in Sentry reports. */
  name: string
  methods: readonly string[]
  /** Matched against the backend path (no /api prefix, no query string). */
  pattern: RegExp
  /**
   * ISR paths made stale by this mutation. undefined entries (missing slugs)
   * are dropped; duplicates are revalidated once.
   */
  paths: (
    ctx: RuleContext
  ) => Promise<Array<string | undefined>> | Array<string | undefined>
}

// ---------------------------------------------------------------------------
// Safe field access on untyped JSON bodies
// ---------------------------------------------------------------------------

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null
    ? (value as Record<string, unknown>)
    : undefined
}

/** Non-empty string field of a JSON object, else undefined. */
function stringField(value: unknown, key: string): string | undefined {
  const field = asRecord(value)?.[key]
  return typeof field === 'string' && field !== '' ? field : undefined
}

/** body.slug — the shape every backend entity mutation response uses. */
function slugOf(body: unknown): string | undefined {
  return stringField(body, 'slug')
}

/** Slugs of the objects under body[key][] (e.g. a show's artists/venues). */
function nestedSlugs(body: unknown, key: string): string[] {
  const list = asRecord(body)?.[key]
  if (!Array.isArray(list)) return []
  return list
    .map((item) => slugOf(item))
    .filter((slug): slug is string => slug !== undefined)
}

// ---------------------------------------------------------------------------
// Page-path helpers
// ---------------------------------------------------------------------------

/**
 * ISR list pages by entity URL segment. Only /artists, /venues, and /shows
 * have ISR list routes (revalidate: 3600); the other entity types' browse
 * pages are client-fetched.
 */
const ISR_LIST_PAGES: Readonly<Record<string, string>> = {
  artists: '/artists',
  venues: '/venues',
  shows: '/shows',
}

/**
 * Singular entity_type values (backend contracts) → URL path segments.
 *
 * Deliberately parallel to ENTITY_PLURAL in
 * features/contributions/hooks/useSuggestEdit.ts (a client module this
 * server-only lib must not import). Keep both in sync when adding entity
 * types.
 */
const SINGULAR_TO_SEGMENT: Readonly<Record<string, string>> = {
  artist: 'artists',
  venue: 'venues',
  show: 'shows',
  release: 'releases',
  label: 'labels',
  festival: 'festivals',
  collection: 'collections',
}

/** Detail page (when the slug is known) + ISR list page (when one exists). */
function entityPages(
  segment: string,
  slug: string | undefined
): Array<string | undefined> {
  return [slug ? `/${segment}/${slug}` : undefined, ISR_LIST_PAGES[segment]]
}

/**
 * Rule resolver for the most common shape: the mutation response is the
 * entity itself, so revalidate its detail page (+ ISR list page if one
 * exists) using the slug from the response body.
 */
function bodySlugPages(segment: string): RevalidationRule['paths'] {
  return ({ body }) => entityPages(segment, slugOf(body))
}

/**
 * Pages affected by a show mutation: the show itself, the upcoming-show
 * surfaces (/shows, /explore), the /artists and /venues lists (both embed
 * upcoming-show data), and each billed artist's detail page (artist pages
 * ISR-cache stats.shows_tracked).
 */
function showPages(body: unknown): Array<string | undefined> {
  const slug = slugOf(body)
  return [
    slug ? `/shows/${slug}` : undefined,
    '/shows',
    '/explore',
    '/artists',
    '/venues',
    ...nestedSlugs(body, 'artists').map((artistSlug) => `/artists/${artistSlug}`),
  ]
}

/**
 * Pages affected by a release mutation: the release itself plus each
 * credited artist's detail page (artist pages ISR-cache stats.releases).
 */
function releasePages(body: unknown): Array<string | undefined> {
  return [
    ...entityPages('releases', slugOf(body)),
    ...nestedSlugs(body, 'artists').map((artistSlug) => `/artists/${artistSlug}`),
  ]
}

// ---------------------------------------------------------------------------
// Rules
// ---------------------------------------------------------------------------
//
// First match wins (rules are disjoint by method+path; ordering is only a
// tiebreaker for readability — most-specific patterns first within a group).

const RULES: readonly RevalidationRule[] = [
  // --- shows -------------------------------------------------------------
  {
    name: 'show-create',
    methods: ['POST'],
    pattern: /^\/shows$/,
    paths: ({ body }) => showPages(body),
  },
  {
    name: 'show-update',
    methods: ['PUT'],
    pattern: /^\/shows\/\d+$/,
    paths: ({ body }) => showPages(body),
  },
  {
    name: 'show-delete',
    methods: ['DELETE'],
    pattern: /^\/shows\/\d+$/,
    // Empty response body — the show's own page can't be resolved, but every
    // list surface that embedded it can.
    paths: () => ['/shows', '/explore', '/artists', '/venues'],
  },
  {
    name: 'show-status',
    methods: ['POST'],
    pattern: /^\/shows\/\d+\/(publish|unpublish|make-private|sold-out|cancelled)$/,
    paths: ({ body }) => showPages(body),
  },
  {
    name: 'show-moderation',
    methods: ['POST'],
    pattern: /^\/admin\/shows\/\d+\/(approve|reject)$/,
    paths: ({ body }) => showPages(body),
  },

  // --- suggest-edit pipeline + moderation --------------------------------
  {
    name: 'suggest-edit',
    methods: ['PUT'],
    pattern: /^\/(artists|venues|festivals|releases|labels)\/\d+\/suggest-edit$/,
    paths: ({ match, body }) => {
      // Pending-only edits don't change the entity; only auto-applied edits
      // (admin / trusted tier — `applied: true`) make ISR pages stale.
      const record = asRecord(body)
      if (record?.applied !== true) return []
      const slug = stringField(record.pending_edit, 'entity_slug')
      return entityPages(match[1], slug)
    },
  },
  {
    name: 'pending-edit-approve',
    methods: ['POST'],
    pattern: /^\/admin\/pending-edits\/\d+\/approve$/,
    // Rejections don't change the entity, so there is no /reject rule.
    paths: ({ body }) => {
      const entityType = stringField(body, 'entity_type')
      const segment = entityType ? SINGULAR_TO_SEGMENT[entityType] : undefined
      if (!segment) return []
      return entityPages(segment, stringField(body, 'entity_slug'))
    },
  },

  // --- artists ------------------------------------------------------------
  {
    name: 'artist-create',
    methods: ['POST'],
    pattern: /^\/admin\/artists$/,
    paths: bodySlugPages('artists'),
  },
  {
    name: 'artist-update',
    methods: ['PATCH'],
    pattern: /^\/admin\/artists\/\d+$/,
    paths: bodySlugPages('artists'),
  },
  {
    name: 'artist-delete',
    methods: ['DELETE'],
    pattern: /^\/artists\/\d+$/,
    paths: () => ['/artists'],
  },
  {
    name: 'artist-merge',
    methods: ['POST'],
    pattern: /^\/admin\/artists\/merge$/,
    // MergeArtistResult carries IDs only; resolve the canonical artist's slug.
    paths: async ({ body, lookupSlug }) => {
      const canonicalId = asRecord(body)?.canonical_artist_id
      const slug =
        typeof canonicalId === 'number'
          ? await lookupSlug('artists', String(canonicalId))
          : undefined
      return entityPages('artists', slug)
    },
  },

  // --- venues -------------------------------------------------------------
  {
    name: 'venue-create',
    methods: ['POST'],
    pattern: /^\/admin\/venues$/,
    paths: bodySlugPages('venues'),
  },
  {
    name: 'venue-update',
    methods: ['PUT'],
    pattern: /^\/venues\/\d+$/,
    paths: bodySlugPages('venues'),
  },
  {
    name: 'venue-delete',
    methods: ['DELETE'],
    pattern: /^\/venues\/\d+$/,
    paths: () => ['/venues'],
  },
  {
    name: 'venue-verify',
    methods: ['POST'],
    pattern: /^\/admin\/venues\/\d+\/verify$/,
    paths: bodySlugPages('venues'),
  },

  // --- releases -----------------------------------------------------------
  {
    name: 'release-create',
    methods: ['POST'],
    pattern: /^\/releases$/,
    paths: ({ body }) => releasePages(body),
  },
  {
    name: 'release-update',
    methods: ['PUT'],
    pattern: /^\/releases\/\d+$/,
    paths: ({ body }) => releasePages(body),
  },
  {
    name: 'release-links',
    methods: ['POST', 'DELETE'],
    pattern: /^\/releases\/(\d+)\/links(\/\d+)?$/,
    // Release pages ISR-cache external_links[]; the response is the link
    // (or empty), so resolve the release slug from the path ID.
    paths: async ({ match, lookupSlug }) => {
      const slug = await lookupSlug('releases', match[1])
      return [slug ? `/releases/${slug}` : undefined]
    },
  },

  // --- labels -------------------------------------------------------------
  {
    name: 'label-create',
    methods: ['POST'],
    pattern: /^\/labels$/,
    paths: bodySlugPages('labels'),
  },
  {
    name: 'label-update',
    methods: ['PUT'],
    pattern: /^\/labels\/\d+$/,
    paths: bodySlugPages('labels'),
  },
  {
    name: 'label-roster',
    methods: ['POST'],
    pattern: /^\/admin\/labels\/(\d+)\/(artists|releases)$/,
    // Label pages ISR-cache artist_count / release_count; the response is
    // just {success}, so resolve the label slug from the path ID.
    paths: async ({ match, lookupSlug }) => {
      const slug = await lookupSlug('labels', match[1])
      return [slug ? `/labels/${slug}` : undefined]
    },
  },

  // --- festivals ----------------------------------------------------------
  {
    name: 'festival-create',
    methods: ['POST'],
    pattern: /^\/festivals$/,
    paths: bodySlugPages('festivals'),
  },
  {
    name: 'festival-update',
    methods: ['PUT'],
    pattern: /^\/festivals\/\d+$/,
    paths: bodySlugPages('festivals'),
  },
  {
    name: 'festival-lineup',
    methods: ['POST', 'PUT', 'DELETE'],
    pattern: /^\/festivals\/(\d+)\/(artists|venues)(\/\d+)?$/,
    // Festival pages ISR-cache artist_count / venue_count; the response is
    // the lineup entry (or empty), so resolve the festival slug from the
    // path ID.
    paths: async ({ match, lookupSlug }) => {
      const slug = await lookupSlug('festivals', match[1])
      return [slug ? `/festivals/${slug}` : undefined]
    },
  },

  // --- collections ---------------------------------------------------------
  // Collection routes are slug-addressed, and the collection ISR payload
  // embeds everything the routes below touch (items[], item_count,
  // like_count, subscriber_count, tags[], is_featured).
  {
    name: 'collection-create',
    methods: ['POST'],
    pattern: /^\/collections$/,
    paths: bodySlugPages('collections'),
  },
  {
    name: 'collection-clone',
    methods: ['POST'],
    pattern: /^\/collections\/[^/]+\/clone$/,
    // The response is the newly created clone.
    paths: bodySlugPages('collections'),
  },
  {
    name: 'collection-feature',
    methods: ['PUT'],
    pattern: /^\/collections\/([^/]+)\/feature$/,
    // Featured collections also surface on /explore.
    paths: ({ match }) => [`/collections/${match[1]}`, '/explore'],
  },
  {
    name: 'collection-engagement',
    methods: ['POST', 'PUT', 'PATCH', 'DELETE'],
    pattern: /^\/collections\/([^/]+)\/(items|like|subscribe|tags)(\/.*)?$/,
    paths: ({ match }) => [`/collections/${match[1]}`],
  },
  {
    name: 'collection-update',
    methods: ['PUT'],
    pattern: /^\/collections\/([^/]+)$/,
    // A title edit can change the slug — revalidate both the old (path) and
    // new (response body) URLs so neither serves stale ISR HTML.
    paths: ({ match, body }) => {
      const bodySlug = slugOf(body)
      return [
        `/collections/${match[1]}`,
        bodySlug ? `/collections/${bodySlug}` : undefined,
      ]
    },
  },
  {
    name: 'collection-delete',
    methods: ['DELETE'],
    pattern: /^\/collections\/([^/]+)$/,
    paths: ({ match }) => [`/collections/${match[1]}`],
  },

  // --- tags ----------------------------------------------------------------
  {
    name: 'tag-create',
    methods: ['POST'],
    pattern: /^\/tags$/,
    paths: bodySlugPages('tags'),
  },
  {
    name: 'tag-update',
    methods: ['PUT'],
    pattern: /^\/tags\/\d+$/,
    paths: bodySlugPages('tags'),
  },
  {
    name: 'entity-tag-add',
    methods: ['POST'],
    pattern: /^\/entities\/[a-z]+\/\d+\/tags$/,
    // Entity detail pages client-fetch their tags, so they are NOT affected.
    // The /tags/{slug} page IS affected (usage_breakdown is ISR-cached). The
    // response body is empty; the tag reference lives in the REQUEST body —
    // tag_id for existing tags. (tag_name-only requests create a brand-new
    // tag, which has no cached ISR page yet, so nothing to revalidate.)
    paths: async ({ requestBody, lookupSlug }) => {
      const tagId = asRecord(requestBody)?.tag_id
      if (typeof tagId !== 'number' || tagId === 0) return []
      const slug = await lookupSlug('tags', String(tagId))
      return [slug ? `/tags/${slug}` : undefined]
    },
  },
  {
    name: 'entity-tag-remove',
    methods: ['DELETE'],
    pattern: /^\/entities\/[a-z]+\/\d+\/tags\/(\d+)$/,
    paths: async ({ match, lookupSlug }) => {
      const slug = await lookupSlug('tags', match[1])
      return [slug ? `/tags/${slug}` : undefined]
    },
  },

  // --- explore curation ----------------------------------------------------
  {
    name: 'featured-slots',
    methods: ['POST', 'DELETE'],
    pattern: /^\/admin\/featured-slots(\/.+)?$/,
    paths: () => ['/explore'],
  },
]

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

export interface ProxyMutationArgs {
  method: string
  /** Backend path: pathname with the /api prefix stripped, no query string. */
  path: string
  /** Raw response body text (null for 204 No Content). */
  responseText: string | null
  /** Raw request body text that was forwarded to the backend. */
  requestText: string | null | undefined
  /** Cookie header to forward on lookup GETs (for auth-gated entities). */
  cookieHeader: string | undefined
}

/**
 * Revalidate the ISR pages affected by a successful proxied mutation.
 *
 * Call AFTER the backend returned 2xx. Never throws — revalidation is a
 * cache-freshness concern and must not break an already-persisted mutation.
 */
export async function revalidateAfterProxyMutation(
  args: ProxyMutationArgs
): Promise<void> {
  const { method, path } = args
  if (!isMutationMethod(method)) return

  // Single pass: find the first rule whose method + pattern match, keeping
  // the match for its capture groups.
  let rule: RevalidationRule | undefined
  let match: RegExpMatchArray | null = null
  for (const candidate of RULES) {
    if (!candidate.methods.includes(method)) continue
    match = path.match(candidate.pattern)
    if (match) {
      rule = candidate
      break
    }
  }
  if (!rule || !match) return

  try {
    const ctx: RuleContext = {
      match,
      body: parseJson(args.responseText),
      requestBody: parseJson(args.requestText),
      lookupSlug: (urlSegment, id) =>
        lookupSlug(urlSegment, id, args.cookieHeader),
    }

    const paths = await rule.paths(ctx)
    const uniquePaths = [
      ...new Set(paths.filter((p): p is string => typeof p === 'string')),
    ]
    for (const isrPath of uniquePaths) {
      safeRevalidatePath(isrPath, rule.name)
    }
  } catch (error) {
    // A rules-engine bug must never break the mutation response.
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'isr-revalidation', rule: rule.name },
      extra: { method, path },
    })
  }
}

function parseJson(text: string | null | undefined): unknown {
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

/**
 * Resolve an entity's slug from its numeric ID via the backend GET endpoint.
 *
 * Every URL segment used by the rules above (artists, releases, labels,
 * festivals, tags) has a GET that accepts ID-or-slug. Returns undefined on
 * any failure — the caller then skips that page, which means it stays stale
 * until its ISR window expires; failures are reported so the gap is visible.
 *
 * Lookups run synchronously before the mutation response returns, adding one
 * backend GET to the rules that need them (admin sub-resource ops, entity
 * tagging). Deliberate tradeoff: revalidation stays inside the request
 * context where revalidatePath is known to work. Moving lookups behind
 * next/server's after() would unblock the response but needs verification
 * that revalidatePath still functions there — candidate optimization for
 * PSY-941.
 */
async function lookupSlug(
  urlSegment: string,
  id: string,
  cookieHeader: string | undefined
): Promise<string | undefined> {
  try {
    const headers: Record<string, string> = {}
    if (cookieHeader) {
      headers['Cookie'] = cookieHeader
    }
    const res = await fetch(`${BACKEND_URL}/${urlSegment}/${id}`, {
      headers,
      cache: 'no-store',
    })
    if (!res.ok) {
      Sentry.captureMessage('isr-revalidation: slug lookup failed', {
        level: 'warning',
        tags: { service: 'isr-revalidation' },
        extra: { urlSegment, id, status: res.status },
      })
      return undefined
    }
    return slugOf(await res.json())
  } catch (error) {
    Sentry.captureException(error, {
      level: 'warning',
      tags: { service: 'isr-revalidation' },
      extra: { urlSegment, id },
    })
    return undefined
  }
}
