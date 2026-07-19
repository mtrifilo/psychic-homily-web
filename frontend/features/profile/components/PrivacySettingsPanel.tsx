'use client'

import { useState, useEffect, useRef, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Loader2, AlertCircle, CheckCircle2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  useOwnContributorProfile,
  useUpdateVisibility,
  useUpdatePrivacy,
  type PrivacyLevel,
  type PrivacySettings,
  type UpdatePrivacyInput,
} from '@/features/auth'

// Sentinel for the adjust-during-render sync below: a value guaranteed
// distinct from any real `privacy_settings`, so the guard also fires on the
// FIRST render (the prior effect always ran on mount and seeded).
const UNSET = Symbol('unset')

const privacyFields: {
  key: keyof Omit<PrivacySettings, 'last_active' | 'profile_sections'>
  label: string
  description: string
}[] = [
  {
    key: 'contributions',
    label: 'Contributions',
    description: 'Your contribution history and stats',
  },
  {
    key: 'saved_shows',
    label: 'Saved shows',
    description: 'Shows you have saved to your list',
  },
  {
    key: 'following',
    label: 'Following',
    description: 'Artists, venues & labels you follow',
  },
  {
    key: 'collections',
    label: 'Collections',
    description: 'Your public collections',
  },
]

const binaryPrivacyFields: {
  key: 'last_active' | 'profile_sections'
  label: string
  description: string
}[] = [
  {
    key: 'last_active',
    label: 'Last active',
    description: 'When you were last active on the site',
  },
  {
    key: 'profile_sections',
    label: 'Custom sections',
    description: 'Your custom profile sections',
  },
]

const privacyLevelOptions: { value: PrivacyLevel; label: string }[] = [
  { value: 'visible', label: 'Visible' },
  { value: 'count_only', label: 'Count only' },
  { value: 'hidden', label: 'Hidden' },
]

/** Pill Switch: board I uses a rounded track; rounded-full ban is badges-only. */
const pillSwitchClassName =
  'h-5 w-9 rounded-full [&_[data-slot=switch-thumb]]:rounded-full'

function PrivacyLevelSelector({
  value,
  onChange,
}: {
  value: PrivacyLevel
  onChange: (value: PrivacyLevel) => void
}) {
  return (
    <div className="flex shrink-0 items-center gap-1.5" role="group">
      {privacyLevelOptions.map(option => {
        const isActive = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            aria-pressed={isActive}
            className={cn(
              'rounded px-2 py-1 text-[11px] font-medium transition-colors',
              'outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
              isActive
                ? 'bg-foreground text-card'
                : 'border border-border text-muted-foreground hover:text-foreground'
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

function PrivacyRow({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="min-w-0 space-y-0.5">
        <Label className="text-[13px] font-medium">{label}</Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      {children}
    </div>
  )
}

export function PrivacySettingsPanel() {
  const { data: profile, isLoading } = useOwnContributorProfile()
  const updateVisibility = useUpdateVisibility()
  const updatePrivacy = useUpdatePrivacy()

  const [localPrivacy, setLocalPrivacy] = useState<PrivacySettings | null>(null)
  const [hasChanges, setHasChanges] = useState(false)
  const [saveSuccess, setSaveSuccess] = useState(false)
  const successTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Clean up timeouts on unmount
  useEffect(() => {
    return () => {
      if (successTimeoutRef.current) {
        clearTimeout(successTimeoutRef.current)
      }
    }
  }, [])

  // Sync local privacy settings from profile, unless the user has unsaved
  // local edits. React 19.2: adjust state during render via the canonical
  // previous-value-guard idiom instead of a cascading effect. The tracker is
  // initialized to a sentinel so the guard fires on the FIRST render too
  // (matching the prior effect, which always ran on mount and seeded). We read
  // the `hasChanges` STATE (not a ref) so the guard is render-safe; `hasChanges`
  // already mirrors the prior `hasLocalEdits` ref (both flipped in lockstep in
  // the field-change and save handlers).
  const [prevPrivacySettings, setPrevPrivacySettings] = useState<
    PrivacySettings | undefined | typeof UNSET
  >(UNSET)
  if (profile?.privacy_settings !== prevPrivacySettings) {
    setPrevPrivacySettings(profile?.privacy_settings)
    if (profile?.privacy_settings && !hasChanges) {
      setLocalPrivacy(profile.privacy_settings)
    }
  }

  const isPublic = profile?.profile_visibility === 'public'
  const username = profile?.username || ''

  const handleVisibilityToggle = () => {
    updateVisibility.mutate(
      { visibility: isPublic ? 'private' : 'public' },
      {
        onSuccess: () => {
          setSaveSuccess(true)
          if (successTimeoutRef.current) {
            clearTimeout(successTimeoutRef.current)
          }
          successTimeoutRef.current = setTimeout(() => setSaveSuccess(false), 3000)
        },
      }
    )
  }

  const handlePrivacyFieldChange = (
    key: keyof PrivacySettings,
    value: PrivacyLevel | 'visible' | 'hidden'
  ) => {
    if (!localPrivacy) return
    setLocalPrivacy({ ...localPrivacy, [key]: value })
    setHasChanges(true)
    setSaveSuccess(false)
  }

  const handleSavePrivacy = () => {
    if (!localPrivacy) return

    const input: UpdatePrivacyInput = {
      contributions: localPrivacy.contributions,
      saved_shows: localPrivacy.saved_shows,
      following: localPrivacy.following,
      collections: localPrivacy.collections,
      last_active: localPrivacy.last_active,
      profile_sections: localPrivacy.profile_sections,
    }

    updatePrivacy.mutate(input, {
      onSuccess: () => {
        setHasChanges(false)
        setSaveSuccess(true)
        if (successTimeoutRef.current) {
          clearTimeout(successTimeoutRef.current)
        }
        successTimeoutRef.current = setTimeout(() => setSaveSuccess(false), 3000)
      },
    })
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="flex justify-center p-6">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      <Card className="gap-3 py-5">
        <CardHeader className="gap-1.5 px-5">
          <CardTitle className="text-sm">Profile visibility</CardTitle>
          <CardDescription className="text-xs">
            Control whether your public profile is visible at all. Per-section
            controls below apply only while the profile is public.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-5">
          <div className="flex items-center justify-between gap-4">
            <div className="min-w-0 space-y-0.5">
              <p className="text-[13px] font-medium">Public profile</p>
              <p className="text-xs text-muted-foreground">
                {isPublic
                  ? `Anyone can view psychichomily.com/users/${username}`
                  : 'Only you can see your profile'}
              </p>
            </div>
            <Switch
              checked={isPublic}
              onCheckedChange={handleVisibilityToggle}
              disabled={updateVisibility.isPending}
              className={pillSwitchClassName}
            />
          </div>
          {updateVisibility.isError && (
            <div className="mt-3 flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>
                {updateVisibility.error?.message || 'Failed to update visibility'}
              </span>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="gap-3 py-5">
        <CardHeader className="gap-1.5 px-5">
          <CardTitle className="text-sm">Privacy controls</CardTitle>
          <CardDescription className="text-xs">
            Choose what visitors see, section by section. Everything defaults to
            visible — opt down where you want; you always see your own full
            profile.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-5">
          {localPrivacy && (
            <div className="divide-y divide-border">
              {privacyFields.map(field => (
                <PrivacyRow
                  key={field.key}
                  label={field.label}
                  description={field.description}
                >
                  <PrivacyLevelSelector
                    value={localPrivacy[field.key]}
                    onChange={value =>
                      handlePrivacyFieldChange(field.key, value)
                    }
                  />
                </PrivacyRow>
              ))}

              {binaryPrivacyFields.map(field => (
                <PrivacyRow
                  key={field.key}
                  label={field.label}
                  description={field.description}
                >
                  <Switch
                    checked={localPrivacy[field.key] === 'visible'}
                    onCheckedChange={checked =>
                      handlePrivacyFieldChange(
                        field.key,
                        checked ? 'visible' : 'hidden'
                      )
                    }
                    className={pillSwitchClassName}
                  />
                </PrivacyRow>
              ))}
            </div>
          )}
        </CardContent>
        <CardFooter className="flex flex-col items-stretch gap-3 border-t border-border px-5 pt-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            {saveSuccess ? (
              <div className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-4 w-4" />
                Settings saved
              </div>
            ) : updatePrivacy.isError ? (
              <div className="flex items-center gap-1.5 text-sm text-destructive">
                <AlertCircle className="h-4 w-4" />
                {updatePrivacy.error?.message || 'Failed to save'}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                Saved changes apply to your public profile immediately.
              </p>
            )}
          </div>
          <Button
            onClick={handleSavePrivacy}
            disabled={!hasChanges || updatePrivacy.isPending}
            size="sm"
            className="shrink-0 self-end sm:self-auto"
          >
            {updatePrivacy.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Save privacy settings
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}
