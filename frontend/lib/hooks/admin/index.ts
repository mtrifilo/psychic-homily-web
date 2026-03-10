export {
  usePendingArtistReports,
  useDismissArtistReport,
  useResolveArtistReport,
} from './useAdminArtistReports'

export {
  type MusicPlatform,
  type DiscoverMusicResponse,
  type DiscoverBandcampResponse,
  type UpdateBandcampResponse,
  type UpdateSpotifyResponse,
  useDiscoverMusic,
  useDiscoverBandcamp,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
  useArtistUpdate,
} from './useAdminArtists'

export { useAuditLogs } from './useAdminAuditLogs'

export {
  type CreateFestivalInput,
  type UpdateFestivalInput,
  type AddFestivalArtistInput,
  type UpdateFestivalArtistInput,
  type AddFestivalVenueInput,
  useCreateFestival,
  useUpdateFestival,
  useDeleteFestival,
  useAddFestivalArtist,
  useUpdateFestivalArtist,
  useRemoveFestivalArtist,
  useAddFestivalVenue,
  useRemoveFestivalVenue,
} from './useAdminFestivals'

export {
  type CreateLabelInput,
  type UpdateLabelInput,
  useCreateLabel,
  useUpdateLabel,
  useDeleteLabel,
} from './useAdminLabels'

export {
  usePendingReports,
  useDismissReport,
  useResolveReport,
} from './useAdminReports'

export {
  adminQueryKeys,
  usePendingShows,
  useRejectedShows,
  useApproveShow,
  useRejectShow,
  useSetShowSoldOut,
  useSetShowCancelled,
  useBatchApproveShows,
  useBatchRejectShows,
} from './useAdminShows'

export { useAdminStats } from './useAdminStats'

export { useAdminUsers } from './useAdminUsers'

export {
  usePendingVenueEdits,
} from './useAdminVenueEdits'

export {
  useUnverifiedVenues,
  useVerifyVenue,
} from './useAdminVenues'
