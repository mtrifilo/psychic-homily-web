'use client'

import {
  useProfile,
  useSetTierEditNotificationPreference,
} from '@/features/auth'
import { useSetShowReminders } from '@/features/shows'
import { useSetCollectionDigestPreference } from '@/features/collections'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Bell, Loader2 } from 'lucide-react'

/**
 * Notification preference toggles. Each row follows the same shape — a label
 * + supporting text on the left, a Switch (with optional pending spinner) on
 * the right — so the surface stays scannable as we add more channels.
 *
 * Toggles wired up:
 *   - Show reminders (PSY): email 24h before saved shows.
 *   - Weekly collection digest (PSY-350 / PSY-515): batched email of new
 *     items in collections you follow. Server default is OFF (opt-IN); the
 *     UI shows the unchecked Switch until the user enables it.
 *   - Tier-change + edit-review emails (PSY-756 / PSY-807): per-category
 *     opt-OUT. Server default is ON for both, so an undefined preference
 *     renders the Switch checked until the user opts out.
 */
export function NotificationSettings() {
  const { data: profileData } = useProfile()
  const setShowReminders = useSetShowReminders()
  const setCollectionDigest = useSetCollectionDigestPreference()
  const setTierEditNotifications = useSetTierEditNotificationPreference()

  const showRemindersEnabled =
    profileData?.user?.preferences?.show_reminders ?? false
  const collectionDigestEnabled =
    profileData?.user?.preferences?.notify_on_collection_digest ?? false
  // Opt-OUT: default to ON when the server hasn't sent an explicit value.
  const tierNotificationsEnabled =
    profileData?.user?.preferences?.notify_on_tier_notifications ?? true
  const editNotificationsEnabled =
    profileData?.user?.preferences?.notify_on_edit_notifications ?? true

  const handleShowRemindersToggle = (checked: boolean) => {
    setShowReminders.mutate(checked)
  }

  const handleCollectionDigestToggle = (checked: boolean) => {
    setCollectionDigest.mutate(checked)
  }

  const handleTierNotificationsToggle = (checked: boolean) => {
    setTierEditNotifications.mutate({ notify_on_tier_notifications: checked })
  }

  const handleEditNotificationsToggle = (checked: boolean) => {
    setTierEditNotifications.mutate({ notify_on_edit_notifications: checked })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Bell className="h-5 w-5" />
          <CardTitle>Notifications</CardTitle>
        </div>
        <CardDescription>
          Control how you&apos;re notified about upcoming shows, your
          collections, and your contributions
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Show reminders */}
        <div>
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="show-reminders">Show reminders</Label>
              <p className="text-sm text-muted-foreground">
                Get an email 24 hours before your saved shows
              </p>
            </div>
            <div className="flex items-center gap-2">
              {setShowReminders.isPending && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
              <Switch
                id="show-reminders"
                checked={showRemindersEnabled}
                onCheckedChange={handleShowRemindersToggle}
                disabled={setShowReminders.isPending}
              />
            </div>
          </div>
          {setShowReminders.isError && (
            <p className="mt-2 text-sm text-destructive">
              Failed to update setting. Please try again.
            </p>
          )}
        </div>

        {/* Collection digest (PSY-350 / PSY-515) */}
        <div>
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="collection-digest">
                Weekly digest of new items in collections I follow
              </Label>
              <p className="text-sm text-muted-foreground">
                One email a week summarizing items added to collections you
                subscribe to.
              </p>
            </div>
            <div className="flex items-center gap-2">
              {setCollectionDigest.isPending && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
              <Switch
                id="collection-digest"
                checked={collectionDigestEnabled}
                onCheckedChange={handleCollectionDigestToggle}
                disabled={setCollectionDigest.isPending}
              />
            </div>
          </div>
          {setCollectionDigest.isError && (
            <p className="mt-2 text-sm text-destructive">
              Failed to update setting. Please try again.
            </p>
          )}
        </div>

        {/* Tier-change emails (PSY-756 / PSY-807) */}
        <div>
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="tier-notifications">Tier-change emails</Label>
              <p className="text-sm text-muted-foreground">
                Get an email when your contributor tier changes (promotion,
                demotion, or an at-risk warning).
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
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="edit-notifications">Edit-review emails</Label>
              <p className="text-sm text-muted-foreground">
                Get an email when your submitted edits are approved or rejected.
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
