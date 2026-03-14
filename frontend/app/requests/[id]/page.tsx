import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { RequestDetail } from '@/features/requests/components'

interface RequestPageProps {
  params: Promise<{ id: string }>
}

export const metadata = {
  title: 'Request',
  description: 'View request details',
}

function RequestLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function RequestPage({ params }: RequestPageProps) {
  const { id } = await params
  const requestId = parseInt(id, 10)

  if (isNaN(requestId) || requestId <= 0) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Request</h1>
          <p className="text-muted-foreground">
            The request could not be found.
          </p>
        </div>
      </div>
    )
  }

  return (
    <Suspense fallback={<RequestLoadingFallback />}>
      <RequestDetail requestId={requestId} />
    </Suspense>
  )
}
