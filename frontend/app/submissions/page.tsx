'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { Loader2, Music } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ShowForm } from '@/components/forms'

export default function SubmissionsPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading, user } = useAuthContext()

  // Redirect unauthenticated users to login
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth')
    }
  }, [isAuthenticated, isLoading, router])

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render if not authenticated (will redirect)
  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Music className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Submit a Show</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Add an upcoming show to the Arizona music calendar
          </p>
          {user && (
            <p className="mt-1 text-xs text-muted-foreground">
              Submitting as {user.email}
            </p>
          )}
        </div>

        {/* Form Card */}
        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="pb-4">
            <CardTitle className="text-lg">Show Details</CardTitle>
            <CardDescription>
              Fill out the information below to add a show. Artists and venues
              will be matched or created automatically.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ShowForm mode="create" />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
