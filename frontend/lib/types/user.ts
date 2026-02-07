export interface UserSubmissionStats {
  approved: number
  pending: number
  rejected: number
  total: number
}

export interface AdminUser {
  id: number
  email: string | null
  username: string | null
  first_name: string | null
  last_name: string | null
  avatar_url: string | null
  is_active: boolean
  is_admin: boolean
  email_verified: boolean
  auth_methods: string[]
  submission_stats: UserSubmissionStats
  created_at: string
  deleted_at?: string | null
}

export interface AdminUsersResponse {
  users: AdminUser[]
  total: number
}
