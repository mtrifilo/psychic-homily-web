// Public API for the comments feature module

// API (endpoints + query keys)
export { commentEndpoints, commentQueryKeys, fieldNoteEndpoints, fieldNoteQueryKeys } from './api'

// Types
export type {
  Comment,
  CommentListResponse,
  CommentThreadResponse,
  FieldNoteStructuredData,
  CreateFieldNoteInput,
} from './types'

// Hooks
export {
  useComments,
  useCommentThread,
  useCreateComment,
  useReplyToComment,
  useUpdateComment,
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
} from './components'
