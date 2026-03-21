// Public API for the labels feature module

// API (endpoints + query keys)
export { labelEndpoints, labelQueryKeys } from './api'

// Types
export type {
  LabelStatus,
  LabelSocial,
  LabelDetail as LabelDetailType,
  LabelListItem,
  LabelsListResponse,
  ArtistLabel,
  ArtistLabelsResponse,
  LabelArtist,
  LabelArtistsResponse,
  LabelRelease,
  LabelReleasesResponse,
} from './types'

export {
  LABEL_STATUS_LABELS,
  LABEL_STATUSES,
  getLabelStatusVariant,
  getLabelStatusLabel,
  formatLabelLocation,
} from './types'

// Hooks
export {
  useLabels,
  useLabel,
  useArtistLabels,
  useLabelRoster,
  useLabelCatalog,
} from './hooks'

export {
  type CreateLabelInput,
  type UpdateLabelInput,
  useCreateLabel,
  useUpdateLabel,
  useDeleteLabel,
} from './hooks'

// Components
export { LabelCard, LabelDetail, LabelList } from './components'
