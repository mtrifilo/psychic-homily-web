'use client'

import { useProfile } from '@/features/auth'
import { useSetShowReminders } from '@/lib/hooks/shows/useShowReminders'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Bell, Loader2 } from 'lucide-react'

export function NotificationSettings() {
  const { data: profileData } = useProfile()
  const setShowReminders = useSetShowReminders()

  const isEnabled = profileData?.user?.preferences?.show_reminders ?? false

  const handleToggle = (checked: boolean) => {
    setShowReminders.mutate(checked)
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Bell className="h-5 w-5" />
          <CardTitle>Notifications</CardTitle>
        </div>
        <CardDescription>
          Control how you&apos;re notified about upcoming shows
        </CardDescription>
      </CardHeader>
      <CardContent>
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
              checked={isEnabled}
              onCheckedChange={handleToggle}
              disabled={setShowReminders.isPending}
            />
          </div>
        </div>
        {setShowReminders.isError && (
          <p className="mt-2 text-sm text-destructive">
            Failed to update setting. Please try again.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
