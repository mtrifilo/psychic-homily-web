// Tag types — aligned with backend contracts/tag.go response types.

export const TAG_CATEGORIES = [
  'genre', 'locale', 'other'
] as const
export type TagCategory = typeof TAG_CATEGORIES[number]

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

export interface EntityTag {
  tag_id: number
  name: string
  slug: string
  category: string
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
