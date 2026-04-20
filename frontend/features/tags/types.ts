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
  'artist', 'release', 'label', 'show', 'venue', 'festival'
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
  added_by_username?: string
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
