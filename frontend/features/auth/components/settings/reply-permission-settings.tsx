'use client'

import { useProfile } from '@/features/auth'
import {
  ReplyPermissionSelect,
  useSetDefaultReplyPermission,
  type ReplyPermission,
} from '@/features/comments'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Loader2 } from 'lucide-react'

/**
 * Settings panel section for the user's default reply permission. PSY-296.
 *
 * Applied to all new top-level comments the user creates. Per-comment
 * override is still available from the CommentForm.
 */
export function ReplyPermissionSettings() {
  const { data: profileData } = useProfile()
  const setDefault = useSetDefaultReplyPermission()

  // The backend default is 'anyone'. If the user has never touched this
  // preference, fall back to 'anyone' for the UI.
  const current = (profileData?.user?.preferences?.default_reply_permission ??
    'anyone') as ReplyPermission

  const handleChange = (next: ReplyPermission) => {
    if (next === current) return
    setDefault.mutate(next)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Default reply permission</CardTitle>
        <CardDescription>
          Who can reply to your comments and field notes by default.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2">
          {setDefault.isPending && (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          )}
          <ReplyPermissionSelect
            id="default-reply-permission"
            value={current}
            onChange={handleChange}
            disabled={setDefault.isPending}
            ariaLabel="Default reply permission"
          />
        </div>
        {setDefault.isError && (
          <p className="mt-2 text-sm text-destructive">
            Failed to update setting. Please try again.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
