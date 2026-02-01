'use client'

import { useTransition } from 'react'
import { useRouter } from 'next/navigation'

/**
 * Hook for filter navigation with smooth transitions.
 * Uses React's useTransition to mark navigation as non-urgent,
 * allowing the old content to remain visible while new data loads.
 */
export function useFilterNavigation(basePath: string) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const navigate = (params: Record<string, string | null>) => {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value) searchParams.set(key, value)
    })
    const queryString = searchParams.toString()

    startTransition(() => {
      router.push(queryString ? `${basePath}?${queryString}` : basePath)
    })
  }

  return { navigate, isPending }
}
