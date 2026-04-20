// Public API for the tags feature module.
// Other features should import from '@/features/tags', not from internal paths.

export type {
  TagCategory,
  TagEntityType,
  TagListItem,
  TagDetailResponse,
  TagEnrichedDetailResponse,
  TagSummary,
  TagUserRef,
  TagContributor,
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
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  DEFAULT_TAG_VIEW,
  getCategoryColor,
  getCategoryLabel,
} from './types'

export type { TagSortOption, TagView } from './types'

export {
  useTags,
  useSearchTags,
  useTag,
  useTagDetail,
  useEntityTags,
  useTagEntities,
  useAddTagToEntity,
  useRemoveTagFromEntity,
  useVoteOnTag,
  useRemoveTagVote,
} from './hooks'

export {
  EntityTagList,
  TagBrowse,
  TagDetail,
  TagOfficialIndicator,
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from './components'

export type {
  TagFacetPanelProps,
  TagFacetSheetProps,
} from './components'
