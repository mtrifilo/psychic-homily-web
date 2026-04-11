export interface Comment {
  id: number
  entity_type: string
  entity_id: number
  user_id: number
  author_name: string
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
  created_at: string
  updated_at: string
  user_vote?: number | null
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
