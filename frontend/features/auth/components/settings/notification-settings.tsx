'use client'

import {
  useProfile,
  useSetTierEditNotificationPreference,
} from '@/features/auth'
import { useSetShowReminders } from '@/features/shows'
import { useSetCollectionDigestPreference } from '@/features/collections'
import { useSetSceneDigestPreference } from '@/features/scenes'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Loader2 } from 'lucide-react'

/**
 * Notification preference toggles (board J / PSY-1414). Each row: label +
 * supporting text on the left, Switch (optional pending spinner) on the right.
 *
 * Toggles wired up:
 *   - Show reminders — day-before email for saved shows.
 *   - Weekly collection digest (PSY-350 / PSY-515): opt-IN; server default OFF.
 *   - Weekly scene digest (PSY-1342): opt-IN; server default OFF.
 *   - Tier-change + edit-review emails (PSY-756 / PSY-807): opt-OUT; default ON.
 *
 * Board J shows only reminders + tier + edit; digests stay here until they
 * move to per-collection / per-scene surfaces (board copy notes digest
 * frequency lives with each followed collection).
 */
export function NotificationSettings() {
  const { data: profileData } = useProfile()
  const setShowReminders = useSetShowReminders()
  const setCollectionDigest = useSetCollectionDigestPreference()
  const setSceneDigest = useSetSceneDigestPreference()
  const setTierEditNotifications = useSetTierEditNotificationPreference()

  const showRemindersEnabled =
    profileData?.user?.preferences?.show_reminders ?? false
  const collectionDigestEnabled =
    profileData?.user?.preferences?.notify_on_collection_digest ?? false
  const sceneDigestEnabled =
    profileData?.user?.preferences?.notify_on_scene_digest ?? false
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

  const handleSceneDigestToggle = (checked: boolean) => {
    setSceneDigest.mutate(checked)
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
        <CardTitle className="text-base">Notifications</CardTitle>
        <CardDescription>
          Email and reminder preferences. Digest frequency lives with each
          followed collection.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Show reminders */}
        <div>
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="show-reminders">Show reminders</Label>
              <p className="text-sm text-muted-foreground">
                Day-before reminders for shows you&apos;ve saved
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

        {/* Scene digest (PSY-1342) */}
        <div>
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="scene-digest">
                Weekly digest for scenes I follow
              </Label>
              <p className="text-sm text-muted-foreground">
                One email a week with this week&apos;s shows and new bands for
                the scenes you follow.
              </p>
            </div>
            <div className="flex items-center gap-2">
              {setSceneDigest.isPending && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
              <Switch
                id="scene-digest"
                checked={sceneDigestEnabled}
                onCheckedChange={handleSceneDigestToggle}
                disabled={setSceneDigest.isPending}
              />
            </div>
          </div>
          {setSceneDigest.isError && (
            <p className="mt-2 text-sm text-destructive">
              Failed to update setting. Please try again.
            </p>
          )}
        </div>

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
