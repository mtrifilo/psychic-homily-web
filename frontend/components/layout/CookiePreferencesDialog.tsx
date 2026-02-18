'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'

interface CookiePreferencesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  gpcSignalDetected: boolean
  currentAnalytics: boolean
  onSave: (analytics: boolean) => void
}

interface CookiePreferencesFormProps {
  gpcSignalDetected: boolean
  currentAnalytics: boolean
  onCancel: () => void
  onSave: (analytics: boolean) => void
}

function CookiePreferencesForm({
  gpcSignalDetected,
  currentAnalytics,
  onCancel,
  onSave,
}: CookiePreferencesFormProps) {
  const [analytics, setAnalytics] = useState(currentAnalytics)

  const handleSave = () => {
    onSave(analytics)
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Cookie Preferences</DialogTitle>
        <DialogDescription>
          Manage your cookie preferences. You can change these settings at any
          time.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-4 py-4">
        {gpcSignalDetected && (
          <div className="rounded-md bg-muted p-3">
            <p className="text-sm text-muted-foreground">
              <strong>Global Privacy Control detected.</strong> Your browser
              has indicated a preference to opt out of data sharing. We
              respect this signal.
            </p>
          </div>
        )}

        <div className="flex items-center justify-between rounded-lg border p-4">
          <div className="space-y-0.5">
            <Label htmlFor="essential-cookies" className="text-base">
              Essential Cookies
            </Label>
            <p className="text-sm text-muted-foreground">
              Required for authentication and security. Cannot be disabled.
            </p>
          </div>
          <Switch
            id="essential-cookies"
            checked={true}
            disabled
            aria-label="Essential cookies (always enabled)"
          />
        </div>

        <div className="flex items-center justify-between rounded-lg border p-4">
          <div className="space-y-0.5">
            <Label htmlFor="analytics-cookies" className="text-base">
              Analytics Cookies
            </Label>
            <p className="text-sm text-muted-foreground">
              Help us understand how you use the site to improve your
              experience.
            </p>
          </div>
          <Switch
            id="analytics-cookies"
            checked={analytics}
            onCheckedChange={setAnalytics}
            aria-label="Analytics cookies"
          />
        </div>
      </div>

      <DialogFooter className="gap-2 sm:gap-0">
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button onClick={handleSave}>Save Preferences</Button>
      </DialogFooter>
    </>
  )
}

export function CookiePreferencesDialog({
  open,
  onOpenChange,
  gpcSignalDetected,
  currentAnalytics,
  onSave,
}: CookiePreferencesDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <CookiePreferencesForm
          key={`${open}-${currentAnalytics}`}
          gpcSignalDetected={gpcSignalDetected}
          currentAnalytics={currentAnalytics}
          onCancel={() => onOpenChange(false)}
          onSave={onSave}
        />
      </DialogContent>
    </Dialog>
  )
}
