import { Skeleton } from '@/components/ui/skeleton'

function CityFiltersSkeleton() {
  return (
    <div className="flex flex-col gap-2 mb-6">
      <div className="flex items-center gap-2">
        <Skeleton className="h-8 w-36 rounded-md" />
      </div>
      <Skeleton className="h-4 w-64" />
    </div>
  )
}

function ShowCardSkeleton() {
  return (
    <div className="border border-border/50 rounded-lg px-3 py-3 sm:px-4 sm:py-4 mb-3">
      <div className="flex gap-3 sm:gap-4">
        {/* Date badge skeleton */}
        <div className="shrink-0 w-14 sm:w-16 rounded-md bg-muted/50 flex flex-col items-center justify-center py-2">
          <Skeleton className="h-3 w-8 mb-1" />
          <Skeleton className="h-3.5 w-10" />
        </div>
        {/* Content skeleton */}
        <div className="flex-1 min-w-0 flex items-start justify-between gap-2">
          <div className="flex-1">
            <Skeleton className="h-5 w-3/4 mb-1.5" />
            <Skeleton className="h-3.5 w-1/3 mb-1.5" />
            <Skeleton className="h-3.5 w-1/2" />
          </div>
          <div className="shrink-0 text-right">
            <Skeleton className="h-4 w-16 mb-1" />
            <Skeleton className="h-3 w-12 ml-auto" />
          </div>
        </div>
      </div>
    </div>
  )
}

export function ShowListSkeleton() {
  return (
    <section className="w-full max-w-6xl">
      <CityFiltersSkeleton />
      {Array.from({ length: 6 }, (_, i) => (
        <ShowCardSkeleton key={i} />
      ))}
    </section>
  )
}
