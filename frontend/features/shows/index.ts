// Public API for the shows feature module

// API (endpoints + query keys)
export { showEndpoints, showQueryKeys } from './api'

// Types
export type {
  ShowArtistSocials,
  SetType,
  ArtistResponse,
  VenueResponse,
  ShowStatus,
  ShowResponse,
  OrphanedArtist,
  CursorPaginationMeta,
  UpcomingShowsResponse,
  PendingShowsResponse,
  RejectedShowsResponse,
  ApproveShowRequest,
  RejectShowRequest,
  RejectionCategory,
  BatchShowError,
  BatchApproveResponse,
  BatchRejectResponse,
  SavedShowResponse,
  SavedShowsListResponse,
  SaveShowResponse,
  MySubmissionsResponse,
  ShowCity,
  ShowCitiesResponse,
  ShowReportType,
  ShowReportStatus,
  ShowReportShowInfo,
  ShowReportResponse,
  CreateShowReportRequest,
  MyShowReportResponse,
  ShowReportsListResponse,
  AdminReportActionRequest,
  ResolveReportRequest,
  CalendarTokenStatusResponse,
  CalendarTokenCreateResponse,
  CalendarTokenDeleteResponse,
  ShowSaveCount,
  SaveCountEntry,
  BatchSaveCountsResponse,
} from './types'

// Hooks
export { useUpcomingShows, useShow, useShowCities } from './hooks'

export { useShowDelete } from './hooks'
export { useShowExtraction } from './hooks'

export {
  type ExportShowData,
  type VenueMatchResult,
  type ArtistMatchResult,
  type ImportPreviewResponse,
  useShowImportPreview,
  useShowImportConfirm,
} from './hooks'

export { useShowMakePrivate } from './hooks'
export { useShowPublish } from './hooks'
export { useSetShowReminders } from './hooks'

export { useMyShowReport, useReportShow } from './hooks'

export { type ShowSubmission, useShowSubmit } from './hooks'

export { useShowUnpublish } from './hooks'

export {
  type ShowUpdateVenue,
  type ShowUpdateArtist,
  type ShowUpdate,
  type ShowUpdateResponse,
  useShowUpdate,
} from './hooks'

export {
  useInfiniteSavedShows,
  useSavedShows,
  useSaveShow,
  useUnsaveShow,
  useSaveShowToggle,
  useShowSaveCount,
  useShowSaveCountBatch,
} from './hooks'

export { useMySubmissions } from './hooks'

// Utilities
export { dedupArtistShows, dedupVenueShows } from './utils'

// Components
export {
  ShowDetail,
  ShowCard,
  ShowForm,
  ShowList,
  ShowListSkeleton,
  HomeShowList,
  DeleteShowDialog,
  PublishShowDialog,
  UnpublishShowDialog,
  MakePrivateDialog,
  ExportShowButton,
  ReportShowButton,
  ReportShowDialog,
  ShowStatusBadge,
  CompactShowRow,
  AIFormFiller,
  SHOW_LIST_FEATURE_POLICY,
} from './components'

export type {
  ShowCardDensity,
  ShowCardProps,
  ShowListContext,
  ShowListFeaturePolicy,
} from './components'
