// Public API for the notifications feature module.
// Other features should import from '@/features/notifications', not from internal paths.

export type {
  NotificationFilter,
  CreateFilterInput,
  UpdateFilterInput,
  NotifyEntityType,
  FilterCity,
  NotificationLogEntry,
  NotificationListResponse,
  MarkReadResponse,
} from './types'

export {
  NOTIFY_ENTITY_TYPES,
  formatTimeAgo,
  getFilterSummary,
  isCommentNotification,
} from './types'

export {
  useNotificationFilters,
  useNotificationFilterCheck,
  useCreateFilter,
  useUpdateFilter,
  useDeleteFilter,
  useQuickCreateFilter,
  useUserNotifications,
  useMarkNotificationsRead,
} from './hooks'

export {
  FilterList,
  FilterCard,
  FilterForm,
  NotifyMeButton,
  NotificationBell,
  NotificationList,
} from './components'
