'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { Loader2, Shield } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'

export default function AdminGuard({
  children,
}: {
  children: React.ReactNode
}) {
  const router = useRouter()
  const { user } = useAuthContext()
  const gate = useAuthRouteGuard()

  // The admin check layers on top of a settled identity: it asks what this
  // viewer may do, which is only answerable once the guard says who they are.
  useEffect(() => {
    if (gate !== 'ready') return
    if (!user?.is_admin) {
      router.push('/')
    }
  }, [gate, user, router])

  if (gate === 'loading') {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render while the sign-in navigation is in flight
  if (gate === 'blank') {
    return null
  }

  // Don't render if not admin (will redirect)
  if (!user?.is_admin) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] flex-col items-center justify-center px-4">
        <div className="rounded-full bg-destructive/10 p-3 mb-4">
          <Shield className="h-6 w-6 text-destructive" />
        </div>
        <h1 className="text-xl font-semibold mb-2">Access Denied</h1>
        <p className="text-sm text-muted-foreground text-center max-w-sm">
          You don&apos;t have permission to access the admin console.
        </p>
      </div>
    )
  }

  return <>{children}</>
}
