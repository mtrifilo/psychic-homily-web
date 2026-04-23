'use client'

import { useProfile } from '@/features/auth'
import {
  ReplyPermissionSelect,
  useSetDefaultReplyPermission,
  type ReplyPermission,
} from '@/features/comments'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Loader2, MessageSquare } from 'lucide-react'

/**
 * Settings panel section for the user's default reply permission. PSY-296.
 *
 * Applied to all new top-level comments the user creates. Per-comment
 * override is still available from the Comment form.
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
        <div className="flex items-center gap-2">
          <MessageSquare className="h-5 w-5" />
          <CardTitle>Default reply permission</CardTitle>
        </div>
        <CardDescription>
          Applied to new comments you create. You can still change this
          per-comment.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div className="space-y-0.5">
            <Label htmlFor="default-reply-permission">Who can reply</Label>
            <p className="text-sm text-muted-foreground">
              &ldquo;Everyone&rdquo; is the default. Choose &ldquo;Followers
              only&rdquo; or &ldquo;Replies disabled&rdquo; if you prefer a
              quieter conversation.
            </p>
          </div>
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
