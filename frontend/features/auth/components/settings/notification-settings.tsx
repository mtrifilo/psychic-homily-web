'use client'

import {
  useProfile,
  useSetTierEditNotificationPreference,
} from '@/features/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Loader2 } from 'lucide-react'

/**
 * Account email toggles (board J / PSY-1414). Each row: label + supporting
 * text on the left, Switch (optional pending spinner) on the right.
 *
 * Toggles wired up:
 *   - Tier-change + edit-review emails (PSY-756 / PSY-807): opt-OUT; default ON.
 *
 * These are the mail the site sends ABOUT YOUR ACCOUNT, which is why they
 * outlived the move. Everything that tells you about the index (follow-driven
 * show and release alerts, the day-before reminder, both weekly digests) moved
 * into the Alerts matrix above (PSY-1905). They were MOVED, not copied: two
 * controls over one boolean is a defect, not a convenience.
 */
export function NotificationSettings() {
  const { data: profileData } = useProfile()
  const setTierEditNotifications = useSetTierEditNotificationPreference()

  // Opt-OUT: default to ON when the server hasn't sent an explicit value.
  const tierNotificationsEnabled =
    profileData?.user?.preferences?.notify_on_tier_notifications ?? true
  const editNotificationsEnabled =
    profileData?.user?.preferences?.notify_on_edit_notifications ?? true

  const handleTierNotificationsToggle = (checked: boolean) => {
    setTierEditNotifications.mutate({ notify_on_tier_notifications: checked })
  }

  const handleEditNotificationsToggle = (checked: boolean) => {
    setTierEditNotifications.mutate({ notify_on_edit_notifications: checked })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Account emails</CardTitle>
        <CardDescription>
          Mail about your account rather than about the index. Everything the
          index tells you about lives in Alerts, above.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Tier-change emails (PSY-756 / PSY-807) */}
        <div>
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="tier-notifications">Tier-change emails</Label>
              <p className="text-sm text-muted-foreground">
                When your contributor tier advances
              </p>
            </div>
            <div className="flex items-center gap-2">
              {setTierEditNotifications.isPending && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
              <Switch
                id="tier-notifications"
                checked={tierNotificationsEnabled}
                onCheckedChange={handleTierNotificationsToggle}
                disabled={setTierEditNotifications.isPending}
              />
            </div>
          </div>
          {setTierEditNotifications.isError && (
            <p className="mt-2 text-sm text-destructive">
              Failed to update setting. Please try again.
            </p>
          )}
        </div>

        {/* Edit-review emails (PSY-756 / PSY-807) */}
        <div>
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="edit-notifications">Edit-review emails</Label>
              <p className="text-sm text-muted-foreground">
                When a pending edit you submitted is reviewed
              </p>
            </div>
            <div className="flex items-center gap-2">
              {setTierEditNotifications.isPending && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
              <Switch
                id="edit-notifications"
                checked={editNotificationsEnabled}
                onCheckedChange={handleEditNotificationsToggle}
                disabled={setTierEditNotifications.isPending}
              />
            </div>
          </div>
          {setTierEditNotifications.isError && (
            <p className="mt-2 text-sm text-destructive">
              Failed to update setting. Please try again.
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
