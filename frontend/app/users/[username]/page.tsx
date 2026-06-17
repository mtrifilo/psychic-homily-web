'use client'

import { use } from 'react'
import { Suspense } from 'react'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { PublicProfile } from '@/features/profile'

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
    notFound()
  }

  return (
    <Suspense fallback={<ProfileLoadingFallback />}>
      <PublicProfile username={username} />
    </Suspense>
  )
}
