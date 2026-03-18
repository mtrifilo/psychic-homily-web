'use client'

import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { FilterList } from '@/features/notifications'

export default function NotificationSettingsPage() {
  const { isAuthenticated, isLoading } = useAuthContext()

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!isAuthenticated) {
    redirect('/auth')
  }

  return (
    <div className="container max-w-3xl mx-auto px-4 py-6">
      <FilterList />
    </div>
  )
}
