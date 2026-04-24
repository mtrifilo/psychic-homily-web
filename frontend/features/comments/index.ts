// Public API for the comments feature module

// API (endpoints + query keys)
export {
  commentEndpoints,
  commentQueryKeys,
  commentPreferencesEndpoints,
  fieldNoteEndpoints,
  fieldNoteQueryKeys,
} from './api'

// Types
export type {
  Comment,
  CommentListResponse,
  CommentThreadResponse,
  FieldNoteStructuredData,
  CreateFieldNoteInput,
  ReplyPermission,
} from './types'
export {
  REPLY_PERMISSION_VALUES,
  REPLY_PERMISSION_LABELS,
  REPLY_PERMISSION_BADGE_LABELS,
} from './types'

// Hooks
export {
  useComments,
  useCommentThread,
  useCreateComment,
  useReplyToComment,
  useUpdateComment,
  useUpdateReplyPermission,
  useSetDefaultReplyPermission,
  useDeleteComment,
  useVoteComment,
  useUnvoteComment,
  useFieldNotes,
  useCreateFieldNote,
} from './hooks'

// Components
export {
  CommentThread,
  CommentCard,
  CommentForm,
  FieldNoteForm,
  FieldNoteCard,
  FieldNotesSection,
  CommentEditHistory,
  EditHistoryBody,
  ReplyPermissionSelect,
} from './components'
