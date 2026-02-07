import { useState } from 'react'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ErrorAlert } from '../shared/ErrorAlert'
import { ExportList } from './ExportList'
import { useExportVenues } from '../../lib/hooks/useExport'
import { Lock } from 'lucide-react'
import type { ExportedVenue } from '../../lib/types'

interface VenuesTabProps {
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onDataLoaded: (venues: ExportedVenue[]) => void
  hasLocalToken: boolean
}

export function VenuesTab({
  selectedIds,
  onSelectionChange,
  onDataLoaded,
  hasLocalToken,
}: VenuesTabProps) {
  const [search, setSearch] = useState('')
  const [enabled, setEnabled] = useState(false)

  const { data, isLoading, error, refetch } = useExportVenues(
    { limit: 100, search },
    enabled && hasLocalToken
  )

  const venues = data?.venues || []
  const total = data?.total || 0

  const handleLoad = () => {
    setEnabled(true)
    if (enabled) {
      refetch()
    }
  }

  // Update parent when data loads
  if (data?.venues) {
    onDataLoaded(data.venues)
  }

  const handleToggle = (id: string) => {
    const next = new Set(selectedIds)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    onSelectionChange(next)
  }

  const handleSelectAll = () => {
    onSelectionChange(new Set(venues.map(getVenueId)))
  }

  const handleClear = () => {
    onSelectionChange(new Set())
  }

  return (
    <div className="space-y-3">
      {/* Controls row */}
      <div className="flex items-center gap-3">
        <Input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search venues..."
          disabled={!hasLocalToken}
          className="w-48"
        />
        <Button
          onClick={handleLoad}
          disabled={isLoading || !hasLocalToken}
          title={!hasLocalToken ? 'Configure Local token in Settings first' : undefined}
        >
          {isLoading && <LoadingSpinner size="sm" />}
          {!hasLocalToken && <Lock className="h-4 w-4" />}
          {isLoading ? 'Loading...' : 'Load Venues'}
        </Button>
        {venues.length > 0 && (
          <span className="text-sm text-muted-foreground">
            {venues.length} of {total}
          </span>
        )}
      </div>

      {error && (
        <ErrorAlert
          message={error instanceof Error ? error.message : 'Failed to load venues'}
          onRetry={() => refetch()}
        />
      )}

      {/* Content area */}
      {venues.length > 0 ? (
        <ExportList
          items={venues}
          selectedIds={selectedIds}
          getId={getVenueId}
          loading={isLoading}
          onToggle={handleToggle}
          onSelectAll={handleSelectAll}
          onClear={handleClear}
          emptyMessage="No venues loaded"
          renderItem={(venue) => <VenueListItem venue={venue} />}
        />
      ) : (
        <div className="border rounded-lg bg-muted/30 py-8 px-4 text-center">
          {!hasLocalToken ? (
            <p className="text-sm text-muted-foreground">
              Configure your Local API token in Settings to load venues
            </p>
          ) : isLoading ? (
            <div className="flex items-center justify-center gap-2">
              <LoadingSpinner size="sm" />
              <span className="text-sm text-muted-foreground">Loading venues...</span>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Click <strong>Load Venues</strong> to fetch data from your local database
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// Generate stable ID for venues
function getVenueId(venue: ExportedVenue): string {
  return `${venue.name}-${venue.city}-${venue.state}`
}

function VenueListItem({ venue }: { venue: ExportedVenue }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <div className="min-w-0">
        <div className="font-medium text-foreground">{venue.name}</div>
        <div className="text-sm text-muted-foreground">
          {venue.city}, {venue.state}
          {venue.address && ` • ${venue.address}`}
        </div>
      </div>
      {venue.verified && (
        <Badge variant="default" className="shrink-0">
          Verified
        </Badge>
      )}
    </div>
  )
}
