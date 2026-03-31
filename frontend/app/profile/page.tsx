'use client'

import { Suspense, useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useUpdateProfile } from '@/features/auth'
import { redirect } from 'next/navigation'
import { Loader2, User, Settings, Shield, LayoutList, Check } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { SettingsPanel } from '@/components/settings'
import { ContributorProfilePreview, PrivacySettingsPanel, ProfileSectionsEditor } from '@/components/contributor'

function ProfileTab() {
  const { user } = useAuthContext()
  const updateProfile = useUpdateProfile()

  const [username, setUsername] = useState('')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [bio, setBio] = useState('')
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Initialize form values from user data
  useEffect(() => {
    if (user) {
      setUsername(user.username || '')
      setFirstName(user.first_name || '')
      setLastName(user.last_name || '')
      setBio(user.bio || '')
    }
  }, [user])

  const handleSave = async () => {
    setError(null)
    setSaved(false)

    try {
      await updateProfile.mutateAsync({
        username: username.trim() || undefined,
        first_name: firstName.trim(),
        last_name: lastName.trim(),
        bio: bio.trim(),
      })
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
    (firstName.trim() !== (user?.first_name || '')) ||
    (lastName.trim() !== (user?.last_name || '')) ||
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

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="first_name">First Name</Label>
                <Input
                  id="first_name"
                  placeholder="First name"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="last_name">Last Name</Label>
                <Input
                  id="last_name"
                  placeholder="Last name"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                />
              </div>
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

          <div className="flex items-center gap-3">
            <Button
              onClick={handleSave}
              disabled={updateProfile.isPending || !hasChanges}
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
            {saved && (
              <span className="flex items-center gap-1 text-sm text-green-600 dark:text-green-400">
                <Check className="h-4 w-4" />
                Profile updated
              </span>
            )}
          </div>
        </CardContent>
      </Card>

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
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()

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

  return (
    <div className="container max-w-4xl mx-auto px-4 py-12">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <User className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold tracking-tight">My Profile</h1>
        </div>
        <p className="text-muted-foreground">
          Manage your profile, privacy, and settings
        </p>
      </div>

      {/* Tabs */}
      <Tabs
        value={currentTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList className="mb-6">
          <TabsTrigger value="profile" className="gap-1.5">
            <User className="h-4 w-4" />
            Profile
          </TabsTrigger>
          <TabsTrigger value="privacy" className="gap-1.5">
            <Shield className="h-4 w-4" />
            Privacy
          </TabsTrigger>
          <TabsTrigger value="sections" className="gap-1.5">
            <LayoutList className="h-4 w-4" />
            Sections
          </TabsTrigger>
          <TabsTrigger value="settings" className="gap-1.5">
            <Settings className="h-4 w-4" />
            Settings
          </TabsTrigger>
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
