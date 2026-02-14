import { Skeleton } from '@/components/ui/skeleton'

function CityFiltersSkeleton() {
  return (
    <div className="flex flex-wrap gap-2 mb-6">
      <Skeleton className="h-8 w-20 rounded-full" />
      <Skeleton className="h-8 w-28 rounded-full" />
      <Skeleton className="h-8 w-24 rounded-full" />
    </div>
  )
}

function ShowCardSkeleton() {
  return (
    <div className="border-b border-border/50 py-5 -mx-3 px-3">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <Skeleton className="h-4 w-24 mb-1.5" />
          <Skeleton className="h-3 w-16" />
        </div>
        {/* Right column: Artists + Venue */}
        <div className="w-full md:w-4/5 md:pl-4">
          <Skeleton className="h-5 w-3/4 mb-2" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      </div>
    </div>
  )
}

export function ShowListSkeleton() {
  return (
    <section className="w-full max-w-4xl">
      <CityFiltersSkeleton />
      {Array.from({ length: 6 }, (_, i) => (
        <ShowCardSkeleton key={i} />
      ))}
    </section>
  )
}
