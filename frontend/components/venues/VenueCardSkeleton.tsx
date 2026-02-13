import { Skeleton } from '@/components/ui/skeleton'

export function VenueCardSkeleton() {
  return (
    <div className="border border-border/50 rounded-lg mb-4 overflow-hidden bg-card px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <Skeleton className="h-5 w-48" />
          <div className="flex items-center gap-1 mt-2">
            <Skeleton className="h-3.5 w-3.5 rounded-full" />
            <Skeleton className="h-3.5 w-24" />
          </div>
        </div>
        <Skeleton className="h-6 w-16 rounded-full" />
      </div>
    </div>
  )
}
