export function ArtistListSkeleton() {
  return (
    <div className="w-full max-w-4xl animate-pulse">
      {/* Search + filter chips skeleton */}
      <div className="mb-6 space-y-4">
        <div className="h-9 w-full max-w-sm rounded-md bg-muted" />
        <div className="flex flex-wrap gap-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-8 rounded-full bg-muted"
              style={{ width: `${60 + i * 20}px` }}
            />
          ))}
        </div>
      </div>

      {/* Grid skeleton */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-2">
        {Array.from({ length: 12 }).map((_, i) => (
          <div key={i} className="py-2 px-3">
            <div className="h-5 bg-muted rounded w-36 mb-1" />
            <div className="h-3 bg-muted rounded w-24" />
          </div>
        ))}
      </div>
    </div>
  )
}
