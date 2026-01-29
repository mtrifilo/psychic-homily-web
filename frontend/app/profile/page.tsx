'use client'

import { Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import { Loader2, User, Settings } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { SettingsPanel } from '@/components/SettingsPanel'

function ProfileTab() {
  const { user } = useAuthContext()

  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile Information</CardTitle>
        <CardDescription>Your account details</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4">
          <div className="space-y-1">
            <p className="text-sm font-medium text-muted-foreground">Email</p>
            <p className="text-sm">{user?.email}</p>
          </div>
          {user?.first_name && (
            <div className="space-y-1">
              <p className="text-sm font-medium text-muted-foreground">First Name</p>
              <p className="text-sm">{user.first_name}</p>
            </div>
          )}
          {user?.last_name && (
            <div className="space-y-1">
              <p className="text-sm font-medium text-muted-foreground">Last Name</p>
              <p className="text-sm">{user.last_name}</p>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
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
          Manage your account and settings
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
          <TabsTrigger value="settings" className="gap-1.5">
            <Settings className="h-4 w-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        <TabsContent value="profile">
          <ProfileTab />
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
