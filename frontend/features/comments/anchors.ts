/**
 * DOM anchor ids for comment deep links (PSY-1512).
 *
 * Backend notification/email URLs append `#comment-{id}` (see
 * filter_service.go CommentURL and comment_notification.go
 * buildCommentURL) — these helpers are the frontend half of that
 * contract. Kept in a plain module (not the hook file) so components can
 * render anchors without importing react-query machinery.
 */

/** DOM id for a comment card. */
export function commentAnchorId(commentId: number): string {
  return `comment-${commentId}`
}

/** DOM id of the whole comments section — the deep-link fallback target. */
export const COMMENTS_SECTION_ANCHOR = 'comments'

/**
 * DOM id of the field notes section. Unlike the comments anchor this is not a
 * notification target; it exists so the logged-out sign-in prompt can send a
 * reader back to the notes rather than the top of a long show page.
 */
export const FIELD_NOTES_SECTION_ANCHOR = 'field-notes'
