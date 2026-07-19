'use client'

import { Suspense, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useUpdateProfile } from '@/features/auth'
import { redirect } from 'next/navigation'
import { Loader2, Check } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { SettingsPanel } from '@/features/auth'
import {
  ContributorProfilePreview,
  PrivacySettingsPanel,
  ProfileSectionsEditor,
  TierAdvancementCard,
} from '@/features/profile'
import { useUrlHash } from '@/lib/hooks/common/useUrlHash'

// Sentinel for the adjust-during-render form seeding below: a value guaranteed
// distinct from any real `user`, so the guard also fires on the FIRST render
// (the prior effect always ran on mount and seeded the form).
const UNSET = Symbol('unset')

const PROFILE_FIELD_HASHES = new Set(['username', 'bio'])

function ProfileTab() {
  const router = useRouter()
  const { user } = useAuthContext()
  const updateProfile = useUpdateProfile()
  const urlHash = useUrlHash()

  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [bio, setBio] = useState('')
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Initialize / re-seed form values whenever the user object changes (async
  // load, or a fresh reference after a successful save). React 19.2: adjust
  // state during render via the canonical previous-value-guard idiom instead
  // of a cascading effect. The tracker starts at a sentinel so the guard also
  // fires on the FIRST render when `user` is already present (matching the
  // prior effect, which always ran on mount). Guarding on the user reference
  // otherwise preserves the prior `[user]`-dependency semantics exactly.
  const [prevUser, setPrevUser] = useState<typeof user | typeof UNSET>(UNSET)
  if (user && user !== prevUser) {
    setPrevUser(user)
    setUsername(user.username || '')
    setDisplayName(user.display_name || '')
    setBio(user.bio || '')
  }

  // Deep-link from /users/me claim CTAs: /profile#username | /profile#bio.
  // Profile is the default tab, so hashing the field id is enough.
  useEffect(() => {
    const fieldId = urlHash.replace(/^#/, '')
    if (!PROFILE_FIELD_HASHES.has(fieldId)) return

    const el = document.getElementById(fieldId)
    if (!el) return

    el.focus()
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }, [urlHash])

  const handleSave = async () => {
    setError(null)
    setSaved(false)

    const wasUsernameEmpty = !user?.username
    const claimedUsername = username.trim()

    try {
      await updateProfile.mutateAsync({
        username: claimedUsername || undefined,
        display_name: displayName.trim(),
        bio: bio.trim(),
      })

      // First-time username claim: land on the new public profile immediately.
      // Editing an already-set username stays on /profile with the toast.
      if (wasUsernameEmpty && claimedUsername) {
        router.push(`/users/${claimedUsername}`)
        return
      }

      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message)
      } else {
        setError('Failed to update profile')
      }
    }
  }

  // Check if form has changes compared to current user data
  const hasChanges =
    (username.trim() !== (user?.username || '')) ||
    (displayName.trim() !== (user?.display_name || '')) ||
    (bio.trim() !== (user?.bio || ''))

  return (
    <div className="space-y-6">
      {/* Contributor profile preview */}
      <ContributorProfilePreview />

      {/* Identity edit form */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Edit Profile</CardTitle>
          <CardDescription>
            Set your username, display name, and bio. Your username appears in
            attributions and on your public profile.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4">
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                placeholder="your_username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                maxLength={30}
              />
              <p className="text-xs text-muted-foreground">
                3-30 characters. Letters, numbers, underscores, and hyphens only.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="display_name">Display name</Label>
              <Input
                id="display_name"
                placeholder="Display name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                maxLength={100}
              />
              <p className="text-xs text-muted-foreground">
                Shown on your public profile and attributions. Leave blank to
                fall back to your username.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="bio">Bio</Label>
              <Textarea
                id="bio"
                placeholder="Tell others a bit about yourself..."
                value={bio}
                onChange={(e) => setBio(e.target.value)}
                maxLength={500}
                rows={3}
              />
              <p className="text-xs text-muted-foreground">
                {bio.length}/500 characters
              </p>
            </div>
          </div>

          {error && (
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          )}

          <div className="flex items-center justify-between gap-3 border-t border-border/60 pt-4">
            <p className="text-xs text-muted-foreground">
              {saved ? (
                <span className="flex items-center gap-1 text-sm text-green-600 dark:text-green-400">
                  <Check className="h-4 w-4" />
                  Profile updated
                </span>
              ) : (
                'Saved changes appear on your public profile.'
              )}
            </p>
            <Button
              onClick={handleSave}
              disabled={updateProfile.isPending || !hasChanges}
              className="shrink-0"
            >
              {updateProfile.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                'Save Changes'
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Tier + advancement requirements (board-H order: after the form) */}
      <TierAdvancementCard tier={(user?.user_tier ?? 'new_user')} />

      {/* Account details (read-only) */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Account Details</CardTitle>
          <CardDescription>Your account information</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4">
            <div className="space-y-1">
              <p className="text-sm font-medium text-muted-foreground">Email</p>
              <p className="text-sm">{user?.email}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function ProfilePageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user, isAuthenticated, isLoading: authLoading } = useAuthContext()

  // Get current tab from URL or default to "profile"
  const currentTab = searchParams.get('tab') || 'profile'

  // Handle tab change
  const handleTabChange = (tab: string) => {
    const newParams = new URLSearchParams(searchParams.toString())
    if (tab === 'profile') {
      newParams.delete('tab')
    } else {
      newParams.set('tab', tab)
    }
    const newPath = newParams.toString()
      ? `/profile?${newParams.toString()}`
      : '/profile'
    router.replace(newPath, { scroll: false })
  }

  // Redirect if not authenticated
  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  // The Edit target of the public profile (PSY-1054, Figma board H): the
  // page names itself as the EDITOR — the profile lives at /users/[username].
  const publicProfileHref = user?.username
    ? `/users/${user.username}`
    : '/users/me'

  return (
    <div className="container max-w-3xl mx-auto px-4 py-10">
      {/* Header */}
      <header className="mb-8">
        <h1 className="text-3xl font-bold tracking-tight">
          Edit profile &amp; settings
        </h1>
        <p className="mt-1.5 text-sm text-muted-foreground">
          {user?.username ? (
            <>
              Your public profile is at{' '}
              <span className="font-mono text-xs">/users/{user.username}</span>
            </>
          ) : (
            <>Claim a username to make your public profile shareable</>
          )}
          <span aria-hidden> · </span>
          <Link
            href={publicProfileHref}
            className="text-primary hover:underline"
          >
            View public profile →
          </Link>
        </p>
      </header>

      {/* Tabs — underline style per the editorial design direction (board H) */}
      <Tabs
        value={currentTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList className="mb-6 h-auto w-full justify-start gap-1 rounded-none border-b border-border bg-transparent p-0">
          {(
            [
              ['profile', 'Profile'],
              ['privacy', 'Privacy'],
              ['sections', 'Sections'],
              ['settings', 'Settings'],
            ] as const
          ).map(([value, label]) => (
            <TabsTrigger
              key={value}
              value={value}
              className="flex-none rounded-none border-0 border-b-2 border-b-transparent bg-transparent px-3 py-2 text-muted-foreground shadow-none data-[state=active]:border-b-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
            >
              {label}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="profile">
          <ProfileTab />
        </TabsContent>

        <TabsContent value="privacy">
          <PrivacySettingsPanel />
        </TabsContent>

        <TabsContent value="sections">
          <ProfileSectionsEditor />
        </TabsContent>

        <TabsContent value="settings">
          <SettingsPanel />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function ProfilePageLoading() {
  return (
    <div className="flex justify-center items-center min-h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}

export default function ProfilePage() {
  return (
    <Suspense fallback={<ProfilePageLoading />}>
      <ProfilePageContent />
    </Suspense>
  )
}
