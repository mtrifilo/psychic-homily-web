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
  getCategoryColor,
  getCategoryLabel,
} from './types'

export type { TagSortOption } from './types'

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

// NOTE: TagDetail is intentionally omitted (PSY-1772). The route page imports it
// directly via `dynamic()` from '@/features/tags/components/TagDetail' so
// Turbopack evicts it from the global shared client chunk (loaded on every
// route). Re-adding it here re-hoists TagDetail.tsx into that chunk.
export {
  EntityTagList,
  AddTagDialog,
  TagBrowse,
  TagOfficialIndicator,
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from './components'

export type {
  AddTagDialogProps,
  TagFacetPanelProps,
  TagFacetLayout,
  TagFacetSheetProps,
} from './components'
