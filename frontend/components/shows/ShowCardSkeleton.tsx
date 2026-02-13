import { Skeleton } from '@/components/ui/skeleton'

export function ShowCardSkeleton() {
  return (
    <div className="border-b border-border/50 py-5 -mx-3 px-3">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3 w-16 mt-1.5" />
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          <Skeleton className="h-5 w-3/4" />
          <div className="flex gap-2 mt-2">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-4 w-12" />
            <Skeleton className="h-4 w-16" />
          </div>
        </div>
      </div>
    </div>
  )
}
