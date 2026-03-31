// Public API for the tags feature module.
// Other features should import from '@/features/tags', not from internal paths.

export type {
  TagCategory,
  TagEntityType,
  TagListItem,
  TagDetailResponse,
  EntityTag,
  TagAlias,
  TagListResponse,
  TagSearchResponse,
  EntityTagsResponse,
  TagAliasesResponse,
  TaggedEntityItem,
  TagEntitiesResponse,
} from './types'

export {
  TAG_CATEGORIES,
  TAG_ENTITY_TYPES,
  getCategoryColor,
  getCategoryLabel,
} from './types'

export {
  useTags,
  useSearchTags,
  useTag,
  useEntityTags,
  useTagEntities,
  useAddTagToEntity,
  useRemoveTagFromEntity,
  useVoteOnTag,
  useRemoveTagVote,
} from './hooks'

export { EntityTagList, TagBrowse, TagDetail } from './components'
