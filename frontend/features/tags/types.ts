// Tag types — aligned with backend contracts/tag.go response types.

export const TAG_CATEGORIES = [
  'genre', 'locale', 'other'
] as const
export type TagCategory = typeof TAG_CATEGORIES[number]

// Sort options for the tag browse page. Values are the URL-facing slugs;
// `backend` is the value passed to the /tags `sort` query param.
export type TagSortOption = 'popularity' | 'alphabetical' | 'newest'

export const TAG_SORT_OPTIONS: { value: TagSortOption; label: string; backend: string }[] = [
  { value: 'popularity', label: 'Popularity', backend: 'usage' },
  { value: 'alphabetical', label: 'Alphabetical', backend: 'name' },
  { value: 'newest', label: 'Newest', backend: 'created' },
]

export const DEFAULT_TAG_SORT: TagSortOption = 'popularity'

export type TagView = 'grid' | 'cloud'
export const DEFAULT_TAG_VIEW: TagView = 'grid'

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

export function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    genre: 'bg-blue-500/10 text-blue-400 border-blue-500/20',
    locale: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20',
    other: 'bg-zinc-500/10 text-zinc-400 border-zinc-500/20',
  }
  return colors[category] || colors.other
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
    default:
      return entityType
  }
}
