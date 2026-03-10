'use client'

import { use } from 'react'
import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { PublicProfile } from '@/components/contributor'

interface UserProfilePageProps {
  params: Promise<{ username: string }>
}

function ProfileLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default function UserProfilePage({ params }: UserProfilePageProps) {
  const { username } = use(params)

  if (!username) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid User</h1>
          <p className="text-muted-foreground">
            The user could not be found.
          </p>
        </div>
      </div>
    )
  }

  return (
    <Suspense fallback={<ProfileLoadingFallback />}>
      <PublicProfile username={username} />
    </Suspense>
  )
}
