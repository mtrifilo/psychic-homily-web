// Public API for the shows feature module

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
  BatchApproveRequest,
  BatchRejectRequest,
  BatchShowError,
  BatchApproveResponse,
  BatchRejectResponse,
  SavedShowResponse,
  SavedShowsListResponse,
  SaveShowResponse,
  CheckSavedResponse,
  CheckBatchSavedResponse,
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
} from './types'

// Hooks
export {
  useUpcomingShows,
  useShow,
  useShowCities,
} from './hooks'

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

export {
  useMyShowReport,
  useReportShow,
} from './hooks'

export {
  type ShowSubmission,
  useShowSubmit,
} from './hooks'

export { useShowUnpublish } from './hooks'

export {
  type ShowUpdateVenue,
  type ShowUpdateArtist,
  type ShowUpdate,
  type ShowUpdateResponse,
  useShowUpdate,
} from './hooks'

export {
  useSavedShows,
  useSavedShowBatch,
  useIsShowSaved,
  useSaveShow,
  useUnsaveShow,
  useSaveShowToggle,
} from './hooks'

export { useMySubmissions } from './hooks'

// Components
export {
  ShowDetail,
  ShowCard,
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
  SHOW_LIST_FEATURE_POLICY,
} from './components'

export type {
  ShowCardDensity,
  ShowCardProps,
  ShowListContext,
  ShowListFeaturePolicy,
} from './components'
