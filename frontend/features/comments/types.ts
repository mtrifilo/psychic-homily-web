// PSY-296: permissioned reply gating.
export type ReplyPermission = 'anyone' | 'followers' | 'author_only'

export const REPLY_PERMISSION_VALUES: ReplyPermission[] = [
  'anyone',
  'followers',
  'author_only',
]

// User-facing labels for reply_permission values.
export const REPLY_PERMISSION_LABELS: Record<ReplyPermission, string> = {
  anyone: 'Everyone',
  followers: 'Followers only',
  author_only: 'Replies disabled',
}

export const REPLY_PERMISSION_BADGE_LABELS: Record<ReplyPermission, string> = {
  anyone: '',
  followers: 'Followers-only replies',
  author_only: 'Replies disabled',
}

export interface Comment {
  id: number
  entity_type: string
  entity_id: number
  // PSY-567: "comment" | "field_note". Reusing the unified Comment shape
  // for both kinds means field-note-only UI (e.g. the 30-min author-edit
  // window) needs to discriminate on this field at render time. Backend
  // emits it on every `CommentResponse`. Optional in the TS type because
  // pre-existing test fixtures + cache-write paths predate this field;
  // treat absence as "comment" at use sites.
  kind?: string
  user_id: number
  author_name: string
  /**
   * Author's username when set, used to link the byline to /users/:username.
   * Null when the user has not set a username — render the name as plain
   * text in that case (PSY-552, mirrors PSY-353 collection attribution).
   */
  author_username?: string | null
  body: string
  body_html: string
  parent_id: number | null
  root_id: number | null
  depth: number
  ups: number
  downs: number
  score: number
  visibility: string
  reply_permission: string
  edit_count: number
  is_edited: boolean
  // PSY-514: count of direct visible replies. Populated by the list endpoint
  // for top-level comments so we can suppress the "Show replies" affordance
  // on zero-reply threads. Other endpoints leave this at 0; treat it as a
  // hint, not an authoritative source for nested rendering.
  reply_count?: number
  created_at: string
  updated_at: string
  user_vote?: number | null
  structured_data?: FieldNoteStructuredData | null
}

export interface CommentListResponse {
  comments: Comment[]
  total: number
  has_more: boolean
}

export interface CommentThreadResponse {
  comment: Comment
  replies: Comment[]
}

// ============================================================================
// Field Notes
// ============================================================================

export interface FieldNoteStructuredData {
  show_artist_id?: number | null
  song_position?: number | null
  sound_quality?: number | null
  crowd_energy?: number | null
  notable_moments?: string | null
  setlist_spoiler: boolean
}

export interface CreateFieldNoteInput {
  body: string
  show_artist_id?: number
  song_position?: number
  sound_quality?: number
  crowd_energy?: number
  notable_moments?: string
  setlist_spoiler?: boolean
}

// PSY-567: time-bounded edit / delete window for a field note's author.
// Matches the backend `FieldNoteEditWindow` constant. Author Edit + Delete
// affordances are gated on `now - created_at < this`; after expiry, the
// only retraction path is Report (moderator action).
//
// Implemented as a derived boolean evaluated on each render. We deliberately
// do NOT install a ticking interval — the buttons disappear naturally as the
// note ages out during the user's session, which is acceptable UX (the user
// either gets a stale visible button that fails with a 403 on click, or the
// component re-renders for an unrelated reason and the buttons disappear).
// The backend window check is authoritative; the frontend gate is a UX hint.
export const FIELD_NOTE_EDIT_WINDOW_MS = 30 * 60 * 1000

export function isWithinFieldNoteEditWindow(createdAt: string): boolean {
  const created = new Date(createdAt).getTime()
  if (!Number.isFinite(created)) return false
  return Date.now() - created < FIELD_NOTE_EDIT_WINDOW_MS
}
