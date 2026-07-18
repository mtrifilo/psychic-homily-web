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
 * Cascades (PSY-941): mutations that change or remove an entity's NAME also
 * invalidate every page that embeds that name (show/release/collection
 * pages), via dynamic route patterns — see RENAME_CASCADES below.
 *
 * Known gaps (follow-up tickets):
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
  ) =>
    | Promise<ReadonlyArray<string | undefined>>
    | ReadonlyArray<string | undefined>
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

// ---------------------------------------------------------------------------
// Cascade invalidation (PSY-941)
// ---------------------------------------------------------------------------

// Dynamic route patterns. safeRevalidatePath revalidates these with type
// 'page', invalidating every cached page under the route on its next visit.
const ALL_SHOW_PAGES = '/shows/[slug]'
const ALL_RELEASE_PAGES = '/releases/[slug]'
const ALL_COLLECTION_PAGES = '/collections/[slug]'
const ALL_SCENE_PAGES = '/scenes/[slug]'

/**
 * Route patterns made stale when an entity of the given segment is renamed,
 * merged, or deleted — the pages that embed the entity's NAME in their own
 * ISR payload (verified against backend contracts):
 *
 *   - Show pages embed artist + venue names (ShowResponse.artists/venues)
 *   - Release pages embed artist + label names (ReleaseDetailResponse)
 *   - Collection pages embed item entity names of every entity type
 *     (CollectionItemResponse.entity_name)
 *   - Scene + tag pages embed only counts → no rename cascade
 *
 * Path-based rules can't enumerate the specific affected pages (that would
 * need revalidateTag with tagged fetches), so the whole route pattern is
 * invalidated. Rename-class mutations are rare admin/trusted events and
 * pages regenerate lazily on their next visit, so the over-invalidation is
 * cheap.
 */
const RENAME_CASCADES: Readonly<Record<string, readonly string[]>> = {
  artists: [ALL_SHOW_PAGES, ALL_RELEASE_PAGES, ALL_COLLECTION_PAGES],
  venues: [ALL_SHOW_PAGES, ALL_COLLECTION_PAGES],
  shows: [ALL_COLLECTION_PAGES],
  releases: [ALL_COLLECTION_PAGES],
  labels: [ALL_RELEASE_PAGES, ALL_COLLECTION_PAGES],
  festivals: [ALL_COLLECTION_PAGES],
}

/** Cascade route patterns for a segment; empty for types nothing embeds. */
function cascadePages(segment: string): readonly string[] {
  return RENAME_CASCADES[segment] ?? []
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
 * bodySlugPages + the segment's rename cascade — for update-class rules
 * where the mutation may have changed the entity's name (which other pages
 * embed in their ISR payloads).
 */
function bodySlugPagesWithCascade(
  segment: string
): RevalidationRule['paths'] {
  return ({ body }) => [
    ...entityPages(segment, slugOf(body)),
    ...cascadePages(segment),
  ]
}

/**
 * Pages affected by a show mutation: the show itself, the upcoming-show
 * surfaces (/shows, /explore), the /artists and /venues lists (both embed
 * upcoming-show data), every scene page (per-city show counts in
 * SceneStats), and each billed artist's detail page (artist pages
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
    ALL_SCENE_PAGES,
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

/**
 * The tagged entity's own detail page, when its ISR payload embeds tags.
 *
 * Only collections do (CollectionDetailResponse.tags) — every other entity
 * type client-fetches its tags, so tagging them never stales their pages.
 * The generic tagging path carries a numeric entity ID; resolving it to a
 * slug requires the collection GET to accept IDs (backend change shipped
 * with PSY-940).
 */
async function taggedEntityPages(
  entityType: string,
  entityId: string,
  lookupSlug: RuleContext['lookupSlug']
): Promise<Array<string | undefined>> {
  if (entityType !== 'collection') return []
  const slug = await lookupSlug('collections', entityId)
  return [slug ? `/collections/${slug}` : undefined]
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
    // A title edit also stales collection pages embedding the show's name.
    paths: ({ body }) => [...showPages(body), ...cascadePages('shows')],
  },
  {
    name: 'show-delete',
    methods: ['DELETE'],
    pattern: /^\/shows\/\d+$/,
    // Empty response body — the show's own page can't be resolved, but every
    // list surface that embedded it can, plus collection pages that listed it.
    paths: () => [
      '/shows',
      '/explore',
      '/artists',
      '/venues',
      ALL_SCENE_PAGES,
      ...cascadePages('shows'),
    ],
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
  {
    name: 'show-batch-moderation',
    methods: ['POST'],
    pattern: /^\/admin\/shows\/(batch-approve|batch-reject)$/,
    // The response carries only counts ({approved/rejected, errors}) — no
    // slugs — so the affected show pages can't be enumerated. Blast the show
    // route along with every list surface a (de)published show appears on.
    paths: () => [
      ALL_SHOW_PAGES,
      '/shows',
      '/explore',
      '/artists',
      '/venues',
      ALL_SCENE_PAGES,
    ],
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
      // Applied edits may rename the entity → cascade to embedding pages.
      return [...entityPages(match[1], slug), ...cascadePages(match[1])]
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
      // Approved edits may rename the entity → cascade to embedding pages.
      return [
        ...entityPages(segment, stringField(body, 'entity_slug')),
        ...cascadePages(segment),
      ]
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
    paths: bodySlugPagesWithCascade('artists'),
  },
  {
    name: 'artist-delete',
    methods: ['DELETE'],
    pattern: /^\/artists\/\d+$/,
    // The deleted artist disappears from show bills, release credits, and
    // collection items → cascade to every embedding page.
    paths: () => ['/artists', ...cascadePages('artists')],
  },
  {
    name: 'artist-merge',
    methods: ['POST'],
    pattern: /^\/admin\/artists\/merge$/,
    // MergeArtistResult carries IDs only; resolve the canonical artist's slug.
    // Shows/releases/collections that credited the merged artist now show the
    // canonical one → cascade.
    paths: async ({ body, lookupSlug }) => {
      const canonicalId = asRecord(body)?.canonical_artist_id
      const slug =
        typeof canonicalId === 'number'
          ? await lookupSlug('artists', String(canonicalId))
          : undefined
      return [...entityPages('artists', slug), ...cascadePages('artists')]
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
    paths: bodySlugPagesWithCascade('venues'),
  },
  {
    name: 'venue-delete',
    methods: ['DELETE'],
    pattern: /^\/venues\/\d+$/,
    paths: () => ['/venues', ...cascadePages('venues')],
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
    // A title edit also stales collection pages embedding the release's name.
    paths: ({ body }) => [...releasePages(body), ...cascadePages('releases')],
  },
  {
    name: 'release-delete',
    methods: ['DELETE'],
    pattern: /^\/releases\/\d+$/,
    // No ISR list page for releases; collection pages listing it go stale.
    paths: () => cascadePages('releases'),
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
    paths: bodySlugPagesWithCascade('labels'),
  },
  {
    name: 'label-delete',
    methods: ['DELETE'],
    pattern: /^\/labels\/\d+$/,
    // Release pages list label credits; collection pages may list the label.
    paths: () => cascadePages('labels'),
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
    paths: bodySlugPagesWithCascade('festivals'),
  },
  {
    name: 'festival-delete',
    methods: ['DELETE'],
    pattern: /^\/festivals\/\d+$/,
    // Collection pages may list the festival as an item.
    paths: () => cascadePages('festivals'),
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
    // Featured flag lives on the collection detail/list surfaces.
    paths: ({ match }) => [`/collections/${match[1]}`],
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
    pattern: /^\/entities\/([a-z]+)\/(\d+)\/tags$/,
    // Two pages can be affected:
    //   1. The /tags/{slug} page (usage_breakdown is ISR-cached). The
    //      response body is empty; the tag reference lives in the REQUEST
    //      body — tag_id for existing tags. (tag_name-only requests create a
    //      brand-new tag, which has no cached ISR page yet.)
    //   2. The tagged entity's own page, but ONLY for collections — every
    //      other entity type client-fetches its tags (PSY-940).
    paths: async ({ match, requestBody, lookupSlug }) => {
      const tagId = asRecord(requestBody)?.tag_id
      const [tagSlug, entityOwnPages] = await Promise.all([
        typeof tagId === 'number' && tagId !== 0
          ? lookupSlug('tags', String(tagId))
          : Promise.resolve(undefined),
        taggedEntityPages(match[1], match[2], lookupSlug),
      ])
      return [tagSlug ? `/tags/${tagSlug}` : undefined, ...entityOwnPages]
    },
  },
  {
    name: 'entity-tag-remove',
    methods: ['DELETE'],
    pattern: /^\/entities\/([a-z]+)\/(\d+)\/tags\/(\d+)$/,
    paths: async ({ match, lookupSlug }) => {
      const [tagSlug, entityOwnPages] = await Promise.all([
        lookupSlug('tags', match[3]),
        taggedEntityPages(match[1], match[2], lookupSlug),
      ])
      return [tagSlug ? `/tags/${tagSlug}` : undefined, ...entityOwnPages]
    },
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
 * festivals, tags, collections — the last as of PSY-940) has a GET that
 * accepts ID-or-slug. Returns undefined on any failure — the caller then
 * skips that page, which means it stays stale until its ISR window expires;
 * failures are reported so the gap is visible.
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
