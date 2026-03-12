'use client'

import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  Loader2,
  AlertCircle,
  CheckCircle2,
  Globe,
  Lock,
  Eye,
  EyeOff,
  Hash,
} from 'lucide-react'
import {
  useOwnContributorProfile,
  useUpdateVisibility,
  useUpdatePrivacy,
  type PrivacyLevel,
  type PrivacySettings,
  type UpdatePrivacyInput,
} from '@/features/auth'

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
    label: 'Saved Shows',
    description: 'Shows you have saved to your list',
  },
  {
    key: 'attendance',
    label: 'Attendance',
    description: 'Shows you have attended',
  },
  {
    key: 'following',
    label: 'Following',
    description: 'Artists and venues you follow',
  },
  {
    key: 'collections',
    label: 'Collections',
    description: 'Your curated collections',
  },
]

const binaryPrivacyFields: {
  key: 'last_active' | 'profile_sections'
  label: string
  description: string
}[] = [
  {
    key: 'last_active',
    label: 'Last Active',
    description: 'When you were last active on the site',
  },
  {
    key: 'profile_sections',
    label: 'Custom Sections',
    description: 'Your custom profile sections',
  },
]

function PrivacyLevelSelector({
  value,
  onChange,
}: {
  value: PrivacyLevel
  onChange: (value: PrivacyLevel) => void
}) {
  return (
    <div className="flex items-center gap-1">
      <Button
        variant={value === 'visible' ? 'default' : 'outline'}
        size="sm"
        className="h-7 px-2 text-xs gap-1"
        onClick={() => onChange('visible')}
      >
        <Eye className="h-3 w-3" />
        Visible
      </Button>
      <Button
        variant={value === 'count_only' ? 'default' : 'outline'}
        size="sm"
        className="h-7 px-2 text-xs gap-1"
        onClick={() => onChange('count_only')}
      >
        <Hash className="h-3 w-3" />
        Count Only
      </Button>
      <Button
        variant={value === 'hidden' ? 'default' : 'outline'}
        size="sm"
        className="h-7 px-2 text-xs gap-1"
        onClick={() => onChange('hidden')}
      >
        <EyeOff className="h-3 w-3" />
        Hidden
      </Button>
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

  // Initialize local privacy settings from profile
  useEffect(() => {
    if (profile?.privacy_settings && !localPrivacy) {
      setLocalPrivacy(profile.privacy_settings)
    }
  }, [profile?.privacy_settings, localPrivacy])

  const isPublic = profile?.profile_visibility === 'public'

  const handleVisibilityToggle = () => {
    updateVisibility.mutate(
      { visibility: isPublic ? 'private' : 'public' },
      {
        onSuccess: () => {
          setSaveSuccess(true)
          setTimeout(() => setSaveSuccess(false), 3000)
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
      attendance: localPrivacy.attendance,
      following: localPrivacy.following,
      collections: localPrivacy.collections,
      last_active: localPrivacy.last_active,
      profile_sections: localPrivacy.profile_sections,
    }

    updatePrivacy.mutate(input, {
      onSuccess: () => {
        setHasChanges(false)
        setSaveSuccess(true)
        setTimeout(() => setSaveSuccess(false), 3000)
      },
    })
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="p-6 flex justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      {/* Profile Visibility */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            {isPublic ? (
              <Globe className="h-5 w-5 text-muted-foreground" />
            ) : (
              <Lock className="h-5 w-5 text-muted-foreground" />
            )}
            <CardTitle className="text-lg">Profile Visibility</CardTitle>
          </div>
          <CardDescription>
            Control whether your profile is visible to others
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between rounded-lg border border-border/50 bg-muted/30 p-4">
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {isPublic ? 'Public Profile' : 'Private Profile'}
              </p>
              <p className="text-xs text-muted-foreground">
                {isPublic
                  ? 'Your profile is visible to everyone at /users/' + (profile?.username || '')
                  : 'Only you can see your profile'}
              </p>
            </div>
            <Switch
              checked={isPublic}
              onCheckedChange={handleVisibilityToggle}
              disabled={updateVisibility.isPending}
            />
          </div>
          {updateVisibility.isError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive mt-3">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{updateVisibility.error?.message || 'Failed to update visibility'}</span>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Granular Privacy Settings */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Privacy Controls</CardTitle>
          <CardDescription>
            Choose what information is visible on your public profile
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Three-level privacy fields */}
          {localPrivacy && privacyFields.map(field => (
            <div
              key={field.key}
              className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 py-3 border-b border-border/30 last:border-0"
            >
              <div className="space-y-0.5">
                <Label className="text-sm font-medium">{field.label}</Label>
                <p className="text-xs text-muted-foreground">{field.description}</p>
              </div>
              <PrivacyLevelSelector
                value={localPrivacy[field.key]}
                onChange={value => handlePrivacyFieldChange(field.key, value)}
              />
            </div>
          ))}

          {/* Binary privacy fields */}
          {localPrivacy && binaryPrivacyFields.map(field => (
            <div
              key={field.key}
              className="flex items-center justify-between gap-2 py-3 border-b border-border/30 last:border-0"
            >
              <div className="space-y-0.5">
                <Label className="text-sm font-medium">{field.label}</Label>
                <p className="text-xs text-muted-foreground">{field.description}</p>
              </div>
              <Switch
                checked={localPrivacy[field.key] === 'visible'}
                onCheckedChange={checked =>
                  handlePrivacyFieldChange(field.key, checked ? 'visible' : 'hidden')
                }
              />
            </div>
          ))}

          {/* Save Button */}
          <div className="flex items-center justify-between pt-2">
            <div>
              {saveSuccess && (
                <div className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-400">
                  <CheckCircle2 className="h-4 w-4" />
                  Settings saved
                </div>
              )}
              {updatePrivacy.isError && (
                <div className="flex items-center gap-1.5 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4" />
                  {updatePrivacy.error?.message || 'Failed to save'}
                </div>
              )}
            </div>
            <Button
              onClick={handleSavePrivacy}
              disabled={!hasChanges || updatePrivacy.isPending}
              size="sm"
            >
              {updatePrivacy.isPending && (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              )}
              Save Privacy Settings
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
