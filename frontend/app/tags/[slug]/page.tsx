'use client'

import { Suspense } from 'react'
import { useParams } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { TagDetail } from '@/features/tags/components'

function TagLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default function TagDetailPage() {
  const params = useParams()

  return (
    <Suspense fallback={<TagLoadingFallback />}>
      <TagDetail slug={params.slug as string} />
    </Suspense>
  )
}
