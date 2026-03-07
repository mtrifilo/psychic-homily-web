export function ArtistListSkeleton() {
  return (
    <div className="w-full max-w-6xl animate-pulse">
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

      {/* Grid skeleton matching new card layout */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {Array.from({ length: 12 }).map((_, i) => (
          <div key={i} className="rounded-lg border border-border/50 p-4">
            <div className="h-5 bg-muted rounded w-36 mb-3" />
            <div className="space-y-2">
              <div className="h-4 bg-muted rounded w-24" />
              <div className="h-4 bg-muted rounded w-20" />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
