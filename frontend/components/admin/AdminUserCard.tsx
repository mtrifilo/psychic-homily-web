'use client'

import { Shield, UserX } from 'lucide-react'
import type { AdminUser } from '@/lib/types/user'
import { Badge } from '@/components/ui/badge'

interface AdminUserCardProps {
  user: AdminUser
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function formatAuthMethod(method: string): string {
  switch (method) {
    case 'password':
      return 'Password'
    case 'google':
      return 'Google'
    case 'passkey':
      return 'Passkey'
    default:
      return method.charAt(0).toUpperCase() + method.slice(1)
  }
}

export function AdminUserCard({ user }: AdminUserCardProps) {
  const isDeleted = !!user.deleted_at
  const isInactive = !user.is_active

  const stats = user.submission_stats
  const hasSubmissions = stats.total > 0

  return (
    <div
      className={`flex items-start gap-3 rounded-lg border bg-card p-3 ${
        isDeleted || isInactive ? 'opacity-60' : ''
      }`}
    >
      <div className="min-w-0 flex-1">
        {/* Primary line: email + badges */}
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium truncate">
            {user.email || 'No email'}
          </span>
          {user.is_admin && (
            <Badge variant="default" className="gap-1 text-xs">
              <Shield className="h-3 w-3" />
              Admin
            </Badge>
          )}
          {isDeleted && (
            <Badge variant="destructive" className="gap-1 text-xs">
              <UserX className="h-3 w-3" />
              Deleted
            </Badge>
          )}
          {isInactive && !isDeleted && (
            <Badge variant="secondary" className="text-xs">
              Inactive
            </Badge>
          )}
        </div>

        {/* Secondary line: username + name */}
        <div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
          {user.username && <span>@{user.username}</span>}
          {user.username &&
            (user.first_name || user.last_name) && <span>&middot;</span>}
          {(user.first_name || user.last_name) && (
            <span>
              {[user.first_name, user.last_name].filter(Boolean).join(' ')}
            </span>
          )}
        </div>

        {/* Auth methods + stats + date */}
        <div className="mt-1.5 flex items-center gap-2 flex-wrap">
          {/* Auth method badges */}
          {user.auth_methods?.map(method => (
            <Badge key={method} variant="outline" className="text-xs">
              {formatAuthMethod(method)}
            </Badge>
          ))}

          <span className="text-xs text-muted-foreground">&middot;</span>

          {/* Submission stats */}
          <span className="text-xs text-muted-foreground">
            {hasSubmissions ? (
              <>
                {stats.approved > 0 && (
                  <span className="text-green-600 dark:text-green-400">
                    {stats.approved} approved
                  </span>
                )}
                {stats.approved > 0 && (stats.pending > 0 || stats.rejected > 0) && ', '}
                {stats.pending > 0 && (
                  <span className="text-amber-600 dark:text-amber-400">
                    {stats.pending} pending
                  </span>
                )}
                {stats.pending > 0 && stats.rejected > 0 && ', '}
                {stats.rejected > 0 && (
                  <span className="text-red-600 dark:text-red-400">
                    {stats.rejected} rejected
                  </span>
                )}
              </>
            ) : (
              'No submissions'
            )}
          </span>

          <span className="text-xs text-muted-foreground">&middot;</span>

          {/* Join date */}
          <span className="text-xs text-muted-foreground">
            Joined {formatDate(user.created_at)}
          </span>
        </div>
      </div>
    </div>
  )
}
