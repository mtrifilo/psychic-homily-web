// Public API for the notifications feature module.
// Other features should import from '@/features/notifications', not from internal paths.

export type {
  NotificationFilter,
  CreateFilterInput,
  UpdateFilterInput,
  NotifyEntityType,
  FilterCity,
  FilterSource,
  NotificationLogEntry,
  NotificationListResponse,
  MarkReadResponse,
} from './types'

export {
  NOTIFY_ENTITY_TYPES,
  FILTER_SOURCE_USER,
  FILTER_SOURCE_MANAGED,
  NOTIFICATION_ENTITY_COMMENT_REPLY,
  NOTIFICATION_ENTITY_COMMENT_MENTION,
  NOTIFICATION_ENTITY_ARTIST_SHOW_ALERT,
  formatTimeAgo,
  getFilterSummary,
  isCommentNotification,
  isArtistShowAlertNotification,
} from './types'

export {
  useNotificationFilters,
  useNotificationFilterCheck,
  useCreateFilter,
  useUpdateFilter,
  useDeleteFilter,
  useQuickCreateFilter,
  useUserNotifications,
  useUnreadNotificationCount,
  useMarkNotificationsRead,
} from './hooks'

export {
  FilterList,
  FilterCard,
  FilterForm,
  NotifyMeButton,
  NotificationBell,
  NotificationList,
  EarlierDivider,
  partitionNotificationsByRead,
} from './components'
