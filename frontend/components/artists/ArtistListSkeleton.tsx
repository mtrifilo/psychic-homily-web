export function ArtistListSkeleton() {
  return (
    <div className="w-full max-w-4xl animate-pulse">
      {/* Filter chips skeleton */}
      <div className="flex flex-wrap gap-2 mb-6">
        {Array.from({ length: 4 }).map((_, i) => (
          <div
            key={i}
            className="h-8 rounded-full bg-muted"
            style={{ width: `${60 + i * 20}px` }}
          />
        ))}
      </div>

      {/* Card skeletons */}
      {Array.from({ length: 8 }).map((_, i) => (
        <div
          key={i}
          className="border border-border/50 rounded-lg mb-4 overflow-hidden bg-card px-4 py-4"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="flex-1">
              <div className="h-5 bg-muted rounded w-48 mb-2" />
              <div className="h-4 bg-muted rounded w-32" />
            </div>
            <div className="flex gap-2">
              <div className="h-9 w-9 bg-muted rounded" />
              <div className="h-9 w-9 bg-muted rounded" />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
