export { EntityTagList, AddTagDialog } from './EntityTagList'
export type { AddTagDialogProps } from './EntityTagList'
export { TagBrowse } from './TagBrowse'
// TagDetail is intentionally NOT re-exported here (PSY-1772). The route page
// `app/tags/[slug]/page.tsx` imports it directly via `dynamic()` from the
// component file so Turbopack can evict it from the global shared client chunk.
// Re-adding a barrel export makes it multi-route-reachable again and re-hoists
// TagDetail.tsx into the chunk that loads on every route.
export { TagOfficialIndicator } from './TagOfficialIndicator'
export {
  TagFacetPanel,
  parseTagsParam,
  buildTagsParam,
  type TagFacetPanelProps,
  type TagFacetLayout,
} from './TagFacetPanel'
export { TagFacetSheet, type TagFacetSheetProps } from './TagFacetSheet'
