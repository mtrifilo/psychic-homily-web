// Tag types — aligned with backend contracts/tag.go response types.

export const TAG_CATEGORIES = [
  'genre', 'locale', 'other', 'crew'
] as const
export type TagCategory = typeof TAG_CATEGORIES[number]

/**
 * A tag naming a MUSIC BOOKER: a promoter, a DIY crew or collective, or a
 * named series or residency that books live music.
 *
 * Crew is the one category that is not a description of sound, and it is the
 * one the server will not let a non-admin mint. Every rule below that treats
 * it differently follows from the first fact.
 */
export const TAG_CATEGORY_CREW = 'crew' as const satisfies TagCategory

/** A category that describes the sound. */
export type FacetTagCategory = Exclude<TagCategory, typeof TAG_CATEGORY_CREW>

/**
 * Whether a category describes the SOUND of the music. `crew` names the party
 * that booked it instead, which is the one fact every rule below turns on: a
 * surface that cannot say which kind of tag it is showing reads a booker's
 * name as a genre.
 *
 * An unrecognized category counts as descriptive, so a category this build
 * has not heard of renders rather than silently vanishing. The comparison is
 * normalized because `tags.category` is an unconstrained column: `Crew` must
 * not slip past a guard that only knows `crew`.
 */
export function isDescriptiveTagCategory(category: string): boolean {
  return category.trim().toLowerCase() !== TAG_CATEGORY_CREW
}

/**
 * The known descriptive categories: the vocabulary of a chip row that prints
 * no category of its own, which is the browse-page facet panels and the
 * add-tag dialog's filter row. A non-descriptive chip standing among genre
 * chips dilutes the filter it sits in.
 *
 * The add-tag dialog's create-category control reads this same list, because
 * that dialog copies the chosen filter chip into the category it would mint:
 * a chip with no matching option there is a value the control cannot show yet
 * still submits.
 *
 * This enumerates the categories this build knows. To judge an arbitrary
 * category string, use `isDescriptiveTagCategory`, which admits an
 * unrecognized one rather than dropping it.
 */
export const FACET_TAG_CATEGORIES: readonly FacetTagCategory[] =
  TAG_CATEGORIES.filter((c): c is FacetTagCategory => isDescriptiveTagCategory(c))

// Sort options for the tag browse page. Values are the URL-facing slugs;
// `backend` is the value passed to the /tags `sort` query param.
export type TagSortOption = 'popularity' | 'alphabetical' | 'newest'

export const TAG_SORT_OPTIONS: { value: TagSortOption; label: string; backend: string }[] = [
  { value: 'popularity', label: 'Popularity', backend: 'usage' },
  { value: 'alphabetical', label: 'Alphabetical', backend: 'name' },
  { value: 'newest', label: 'Newest', backend: 'created' },
]

export const DEFAULT_TAG_SORT: TagSortOption = 'popularity'

export const TAG_ENTITY_TYPES = [
  // PSY-354: collection joined the polymorphic tag corpus. Order matches
  // backend `models.TagEntityTypes`.
  'artist', 'release', 'label', 'show', 'venue', 'festival', 'collection',
] as const
export type TagEntityType = typeof TAG_ENTITY_TYPES[number]

export interface TagListItem {
  id: number
  name: string
  slug: string
  category: string
  is_official: boolean
  usage_count: number
  created_at: string
  /**
   * Populated only by the tag search / autocomplete endpoint when the query
   * matched an entry in `tag_aliases` rather than `tags.name`. The value is
   * the specific alias that matched, so the add-tag dialog can show
   * "matched `{alias}`" under the canonical row for transparency (PSY-442).
   */
  matched_via_alias?: string
}

export interface TagDetailResponse extends TagListItem {
  description?: string
  parent_id?: number
  parent_name?: string
  child_count: number
  aliases: string[]
  created_by_user_id?: number
  created_by_username?: string
  updated_at: string
}

// ──────────────────────────────────────────────
// Enriched tag detail (GET /tags/{slug}/detail)
// ──────────────────────────────────────────────

/** Minimal tag summary used in parent/children/related arrays. */
export interface TagSummary {
  id: number
  name: string
  slug: string
  category: string
  is_official: boolean
  usage_count: number
}

/** Minimal user reference. `username` doubles as the profile URL slug. */
export interface TagUserRef {
  id: number
  username?: string
}

/** Top-contributor row with tag-application count. */
export interface TagContributor {
  user: TagUserRef
  count: number
}

/**
 * Enriched tag detail response — extends TagDetailResponse with description_html,
 * parent/children, usage breakdown, top contributors, created_by, and related tags.
 * Returned by GET /tags/{slug}/detail.
 */
export interface TagEnrichedDetailResponse extends TagDetailResponse {
  /** Sanitized HTML rendered from the markdown description. Empty when no description. */
  description_html?: string
  /** Full parent tag summary, or undefined/null when no parent. */
  parent?: TagSummary | null
  /** Direct child tag summaries (always present, may be empty array). */
  children: TagSummary[]
  /** Map of entity_type → count. Every valid entity type is always present (zero included). */
  usage_breakdown: Record<string, number>
  /** Top 5 contributors by count (may be empty array). */
  top_contributors: TagContributor[]
  /** Creator attribution, or undefined/null when unknown. */
  created_by?: TagUserRef | null
  /** Co-occurring tags, capped at 5. May be empty array. */
  related_tags: TagSummary[]
}

export interface EntityTag {
  tag_id: number
  name: string
  slug: string
  category: string
  is_official: boolean
  upvotes: number
  downvotes: number
  wilson_score: number
  user_vote?: number | null
  /**
   * Attribution fields (PSY-479). Backend always populates these on
   * /entities/{type}/{id}/tags responses:
   * - `added_by_user_id` is the FK on `entity_tags`. Null only for
   *   pre-attribution legacy rows that don't exist in the current schema.
   * - `added_by_username` is the resolved username (joined from `users`).
   *   Null when the user account has no username (older accounts that never
   *   set one). The hover card renders "Source: system seed" in that case.
   * - `added_at` is the UTC timestamp the tag was applied.
   */
  added_by_user_id?: number | null
  added_by_username?: string | null
  added_at?: string | null
}

export interface TagAlias {
  id: number
  alias: string
  created_at: string
}

export interface TagListResponse {
  tags: TagListItem[]
  total: number
}

export interface TagSearchResponse {
  tags: TagListItem[]
}

export interface EntityTagsResponse {
  tags: EntityTag[]
}

export interface TaggedEntityItem {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  // PSY-485: optional per-entity-type fields populated by the backend so the
  // tag detail page can render proper entity cards instead of bare links.
  // All fields are omitted on entity types where they don't apply.
  city?: string
  state?: string
  // Venue-specific.
  verified?: boolean
  // Artist/venue-specific.
  upcoming_show_count?: number
  // Festival-specific.
  edition_year?: number
  start_date?: string
  end_date?: string
  // Status applies to festivals (announced/confirmed/cancelled/completed)
  // and labels (active/inactive/defunct).
  status?: string
  // Counts populated for festivals (artists in lineup) and labels
  // (artists on roster, releases in catalog).
  artist_count?: number
  release_count?: number
  venue_count?: number
  // Release-specific.
  release_type?: string
  release_year?: number
  cover_art_url?: string
  // Show-specific.
  event_date?: string
  venue_name?: string
  venue_slug?: string
  headliner_name?: string
  headliner_slug?: string
}

export interface TagEntitiesResponse {
  entities: TaggedEntityItem[]
  total: number
}

// ──────────────────────────────────────────────
// Cross-entity tag intersection (GET /tags/intersection — PSY-995)
// ──────────────────────────────────────────────

/**
 * One entity-type slice of a tag intersection: the full match `count` plus a
 * `preview` (page 1 of that type's "show all" browse). The rebuilt tag detail
 * page (PSY-993) renders its grouped sections from these groups — for both the
 * single-tag case (`tags=[slug]`) and the multi-tag "+ add a tag" pivot.
 */
export interface TagIntersectionGroup {
  entity_type: string
  count: number
  preview: TaggedEntityItem[]
}

/**
 * Response body of GET /tags/intersection. `tags` echoes the resolved input
 * tags (request order) for the chip UI. `groups` carries one entry per valid
 * entity type — zero-count groups included (complete keyset) so the frontend
 * decides which sections to show. Groups arrive in the backend's canonical
 * TagEntityTypes order; the detail page reorders them to the design's fixed
 * display order (TAG_DETAIL_SECTION_ORDER).
 */
export interface TagIntersectionResponse {
  tags: TagSummary[]
  tag_match: string
  groups: TagIntersectionGroup[]
}

/**
 * Fixed display order for the rebuilt /tags/{slug} detail page sections
 * (PSY-993, design frame 424:7). Differs from the backend's canonical
 * TagEntityTypes order — the design leads with the discovery-relevant types
 * (artists, releases, upcoming shows) before the supporting ones. Empty-count
 * sections are suppressed at render time.
 */
export const TAG_DETAIL_SECTION_ORDER = [
  'artist',
  'release',
  'show',
  'venue',
  'label',
  'festival',
  'collection',
] as const

/**
 * Section heading label per entity type, matching the design copy. Mostly the
 * plural label, except shows which read "Upcoming shows" (the section is gated
 * to upcoming/approved shows by the backend).
 */
export function getTagSectionLabel(entityType: string): string {
  if (entityType === 'show') return 'Upcoming shows'
  return getEntityTypePluralLabel(entityType)
}

/**
 * Build the "show all" deep-link for a section: the existing per-type browse
 * filtered by the active tag(s). The six entity browse pages
 * (artists/releases/shows/venues/labels/festivals) already support `?tags=` +
 * `tag_match=` (PSY-993 decision #1); multiple slugs join with `tag_match=all`.
 * Collections have no tag-filtered browse, so this returns null for them and
 * the collection section omits its "show all" link.
 */
export function getTagSectionBrowseUrl(
  entityType: string,
  slugs: string[]
): string | null {
  const base: Record<string, string> = {
    artist: '/artists',
    release: '/releases',
    show: '/shows',
    venue: '/venues',
    label: '/labels',
    festival: '/festivals',
  }
  const path = base[entityType]
  if (!path) return null
  const params = new URLSearchParams()
  params.set('tags', slugs.join(','))
  if (slugs.length > 1) params.set('tag_match', 'all')
  return `${path}?${params.toString()}`
}

export interface TagAliasesResponse {
  aliases: TagAlias[]
}

/** Global alias listing row — alias paired with its canonical tag (admin view). */
export interface TagAliasListing {
  id: number
  alias: string
  tag_id: number
  tag_name: string
  tag_slug: string
  tag_category: string
  tag_is_official: boolean
  created_at: string
}

export interface TagAliasListingResponse {
  aliases: TagAliasListing[]
  total: number
}

export interface BulkAliasImportItem {
  alias: string
  canonical: string
}

export interface BulkAliasImportSkipped {
  row: number
  alias: string
  canonical: string
  reason: string
}

export interface BulkAliasImportResult {
  imported: number
  skipped: BulkAliasImportSkipped[]
}

/** Preview of a merge — returned by GET /admin/tags/{source_id}/merge-preview */
export interface MergeTagsPreview {
  moved_entity_tags: number
  moved_votes: number
  /** Up/down split of moved_votes (PSY-487). Always equal to moved_votes when summed. */
  moved_upvotes: number
  moved_downvotes: number
  skipped_entity_tags: number
  skipped_votes: number
  source_aliases_count: number
  source_name: string
  target_name: string
}

/** Result of a completed merge — returned by POST /admin/tags/{source_id}/merge */
export interface MergeTagsResult {
  moved_entity_tags: number
  moved_votes: number
  skipped_entity_tags: number
  skipped_votes: number
  alias_created: boolean
  moved_aliases: number
}

/**
 * Reason identifier for why a tag appeared in the low-quality review queue.
 * Keep in sync with the backend constants in `tag_low_quality.go`.
 */
export type LowQualityReason =
  | 'orphaned'
  | 'aging_unused'
  | 'downvoted'
  | 'short_name'
  | 'long_name'

/** One row in the admin low-quality tag review queue (PSY-310). */
export interface LowQualityTagQueueItem extends TagListItem {
  upvotes: number
  downvotes: number
  reasons: LowQualityReason[]
}

/** Paginated response for GET /admin/tags/low-quality. */
export interface LowQualityTagQueueResponse {
  tags: LowQualityTagQueueItem[]
  total: number
}

/**
 * Verb for the bulk-action endpoint on the low-quality queue (PSY-487).
 * Mirrors the backend constants in `tag_low_quality.go`.
 */
export type BulkLowQualityAction = 'snooze' | 'delete' | 'mark_official'

/** Result returned from POST /admin/tags/low-quality/bulk-action. */
export interface BulkLowQualityActionResult {
  action: BulkLowQualityAction
  requested: number
  affected: number
  not_found: number
}

/** Human-readable labels for the reason pills in the queue UI. */
export const LOW_QUALITY_REASON_LABELS: Record<LowQualityReason, string> = {
  orphaned: 'Orphaned',
  aging_unused: 'Aging unused',
  downvoted: 'Downvoted',
  short_name: 'Short name',
  long_name: 'Long name',
}

/**
 * Filter chip set surfaced above the Needs Review queue (PSY-487).
 * Order matches the human-friendly read in the spec ("Orphaned, Aging unused,
 * Downvoted, Unusual length"). Short and long name are merged into a single
 * "Unusual length" chip — admins don't typically need to distinguish.
 */
export interface LowQualitySignalChip {
  id: string
  label: string
  reasons: LowQualityReason[]
}

export const LOW_QUALITY_SIGNAL_CHIPS: LowQualitySignalChip[] = [
  { id: 'orphaned', label: 'Orphaned', reasons: ['orphaned'] },
  { id: 'aging_unused', label: 'Aging unused', reasons: ['aging_unused'] },
  { id: 'downvoted', label: 'Downvoted', reasons: ['downvoted'] },
  { id: 'unusual_length', label: 'Unusual length', reasons: ['short_name', 'long_name'] },
]

/**
 * Genre-hierarchy row — returned by GET /admin/tags/hierarchy (PSY-311).
 * Flat list; the frontend builds the tree client-side from parent_id.
 */
export interface GenreHierarchyTag {
  id: number
  name: string
  slug: string
  parent_id?: number | null
  usage_count: number
  is_official: boolean
}

export interface GenreHierarchyResponse {
  tags: GenreHierarchyTag[]
}

/**
 * Node in the assembled client-side tree. Convenience shape — not a wire type.
 * `depth` is 0 for roots and increments per level; used for indentation.
 * `parent_name` is denormalized at tree-assembly time so flat-search results
 * (which strip child links) can still render the `parent › child` breadcrumb
 * chip beside the tag name without re-querying the source list.
 */
export interface GenreHierarchyNode extends GenreHierarchyTag {
  depth: number
  children: GenreHierarchyNode[]
  parent_name?: string | null
}

/**
 * One category's chip vocabulary, bound to the DS categorical palette
 * (PSY-943): genre = chart-6 (denim), locale = chart-8 (teal), other = muted
 * (the neutral catch-all). The tokens track light/dark via the CSS cascade.
 *
 * `shape` is set only for a category whose identity is not a colour, which
 * is why it is a separate field rather than more classes on the tint: a
 * text-only surface can take the tint alone, and the official accent (below)
 * can tell the two kinds of category apart without parsing a class string.
 */
interface CategoryChipTokens {
  bg: string
  text: string
  border: string
  shape?: string
}

const CATEGORY_CHIP_TOKENS: Record<TagCategory, CategoryChipTokens> = {
  genre: { bg: 'bg-chart-6/10', text: 'text-chart-6', border: 'border-chart-6/20' },
  locale: { bg: 'bg-chart-8/10', text: 'text-chart-8', border: 'border-chart-8/20' },
  other: { bg: 'bg-muted', text: 'text-muted-foreground', border: 'border-border' },
  // Figma `1402:789`: an unfilled hairline square on the border token, mono
  // uppercase on muted-foreground. Font SIZE is absent on purpose, so each
  // surface keeps its own density.
  crew: {
    bg: 'bg-transparent',
    text: 'text-muted-foreground',
    border: 'border-border',
    shape: 'rounded-[2px] font-mono uppercase tracking-[0.04em]',
  },
}

/** Classes an unrecognized category falls back to. */
const FALLBACK_CATEGORY = 'other' as const satisfies TagCategory

function composeChipClasses(t: CategoryChipTokens): string {
  return [t.bg, t.text, t.border, t.shape].filter(Boolean).join(' ')
}

/**
 * Both maps are keyed by a value read straight out of an unconstrained
 * database column, so lookups go through a Map rather than an object index:
 * an object would answer `toString` with a prototype member instead of
 * missing, and the fallback would never fire.
 */
const CATEGORY_CHIP_TOKEN_MAP = new Map<string, CategoryChipTokens>(
  Object.entries(CATEGORY_CHIP_TOKENS)
)
const CATEGORY_CHIP_CLASS_MAP = new Map<string, string>(
  Object.entries(CATEGORY_CHIP_TOKENS).map(([category, t]) => [
    category,
    composeChipClasses(t),
  ])
)
const FALLBACK_CHIP_TOKENS = CATEGORY_CHIP_TOKENS[FALLBACK_CATEGORY]
const FALLBACK_CHIP_CLASSES = composeChipClasses(FALLBACK_CHIP_TOKENS)

function categoryTokens(category: string): CategoryChipTokens {
  return CATEGORY_CHIP_TOKEN_MAP.get(category) ?? FALLBACK_CHIP_TOKENS
}

/**
 * Every class a category's chip wears. Callers must compose this through
 * `cn` (or a component that does), because a category carrying `shape`
 * overrides shape utilities the caller sets first.
 */
export function getCategoryChipClasses(category: string): string {
  return CATEGORY_CHIP_CLASS_MAP.get(category) ?? FALLBACK_CHIP_CLASSES
}

/**
 * The foreground tint alone, for a surface that prints the category as text
 * rather than as a chip and so must not inherit a chip's shape.
 */
export function getCategoryTint(category: string): string {
  return categoryTokens(category).text
}

/**
 * The accent an official tag wears in place of its category tint, so curated
 * tags read as curated at a glance (ISSUE-004 from tags-audit-2).
 */
const OFFICIAL_TAG_CHIP_CLASSES = 'border-primary/40 bg-primary/10 text-foreground'

/**
 * Classes for one applied tag's chip. The official accent replaces a
 * descriptive category's tint, because one colour swapped for another loses
 * nothing. A non-descriptive category keeps its own classes: the accent is
 * the same pill an official genre tag wears, so it would erase the only
 * thing that tells the two apart. The official indicator rendered beside the
 * name still says the tag is curated.
 */
export function getTagChipClasses(tag: {
  category: string
  is_official: boolean
}): string {
  if (tag.is_official && isDescriptiveTagCategory(tag.category)) {
    return OFFICIAL_TAG_CHIP_CLASSES
  }
  return getCategoryChipClasses(tag.category)
}

export function getCategoryLabel(category: string): string {
  return category.charAt(0).toUpperCase() + category.slice(1)
}

/** Build entity URL from entity type and slug */
export function getEntityUrl(entityType: string, entitySlug: string): string {
  switch (entityType) {
    case 'artist':
      return `/artists/${entitySlug}`
    case 'venue':
      return `/venues/${entitySlug}`
    case 'show':
      return `/shows/${entitySlug}`
    case 'release':
      return `/releases/${entitySlug}`
    case 'label':
      return `/labels/${entitySlug}`
    case 'festival':
      return `/festivals/${entitySlug}`
    case 'collection':
      // PSY-553: tagged collections (PSY-354) link to the same detail page
      // entity-card chips link to from the collection browse list.
      return `/collections/${entitySlug}`
    default:
      return '#'
  }
}

/** Get a plural display label for an entity type */
export function getEntityTypePluralLabel(entityType: string): string {
  switch (entityType) {
    case 'artist':
      return 'Artists'
    case 'venue':
      return 'Venues'
    case 'show':
      return 'Shows'
    case 'release':
      return 'Releases'
    case 'label':
      return 'Labels'
    case 'festival':
      return 'Festivals'
    case 'collection':
      // PSY-553: powering the "Collections N" tab on the tag detail page.
      return 'Collections'
    default:
      return entityType
  }
}
