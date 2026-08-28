// Public API for the comments feature module

// API (endpoints + query keys)
export {
  commentEndpoints,
  commentQueryKeys,
  commentPreferencesEndpoints,
  fieldNoteEndpoints,
  fieldNoteQueryKeys,
  VENUE_FIELD_NOTE_TEASER_LIMIT,
} from './api'

// Types
export type {
  Comment,
  CommentListResponse,
  CommentThreadResponse,
  FieldNoteStructuredData,
  CreateFieldNoteInput,
  ReplyPermission,
  VenueFieldNote,
  VenueFieldNoteListResponse,
} from './types'
export {
  REPLY_PERMISSION_VALUES,
  REPLY_PERMISSION_LABELS,
  REPLY_PERMISSION_BADGE_LABELS,
} from './types'

// Quoting a field note somewhere other than its own card (PSY-1590).
export {
  isSetlistSpoiler,
  fieldNoteTeaserText,
  pickFieldNoteForTeaser,
} from './teaser'
export type { FieldNoteTeaserPick } from './teaser'

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
  useVenueFieldNotes,
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
