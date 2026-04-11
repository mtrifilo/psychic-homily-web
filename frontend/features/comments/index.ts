// Public API for the comments feature module

// API (endpoints + query keys)
export { commentEndpoints, commentQueryKeys } from './api'

// Types
export type {
  Comment,
  CommentListResponse,
  CommentThreadResponse,
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
} from './hooks'

// Components
export { CommentThread, CommentCard, CommentForm } from './components'
