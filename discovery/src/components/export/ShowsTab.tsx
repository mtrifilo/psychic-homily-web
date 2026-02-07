import { useState } from 'react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ErrorAlert } from '../shared/ErrorAlert'
import { ExportList } from './ExportList'
import { useExportShows } from '../../lib/hooks/useExport'
import { Lock } from 'lucide-react'
import type { ExportedShow } from '../../lib/types'

interface ShowsTabProps {
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onDataLoaded: (shows: ExportedShow[]) => void
  hasLocalToken: boolean
}

export function ShowsTab({
  selectedIds,
  onSelectionChange,
  onDataLoaded,
  hasLocalToken,
}: ShowsTabProps) {
  const [showStatus, setShowStatus] = useState('approved')
  const [enabled, setEnabled] = useState(false)

  const { data, isLoading, error, refetch } = useExportShows(
    { limit: 100, status: showStatus },
    enabled && hasLocalToken
  )

  const shows = data?.shows || []
  const total = data?.total || 0

  const handleLoad = () => {
    setEnabled(true)
    if (enabled) {
      refetch()
    }
  }

  // Update parent when data loads
  if (data?.shows) {
    onDataLoaded(data.shows)
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
    onSelectionChange(new Set(shows.map(getShowId)))
  }

  const handleClear = () => {
    onSelectionChange(new Set())
  }

  return (
    <div className="space-y-3">
      {/* Controls row */}
      <div className="flex items-center gap-3">
        <select
          value={showStatus}
          onChange={(e) => {
            setShowStatus(e.target.value)
            setEnabled(false)
            onSelectionChange(new Set())
          }}
          disabled={!hasLocalToken}
          className="h-9 px-3 border rounded-md text-sm bg-background disabled:opacity-50"
        >
          <option value="approved">Approved</option>
          <option value="pending">Pending</option>
          <option value="all">All</option>
        </select>
        <Button
          onClick={handleLoad}
          disabled={isLoading || !hasLocalToken}
          title={!hasLocalToken ? 'Configure Local token in Settings first' : undefined}
        >
          {isLoading && <LoadingSpinner size="sm" />}
          {!hasLocalToken && <Lock className="h-4 w-4" />}
          {isLoading ? 'Loading...' : 'Load Shows'}
        </Button>
        {shows.length > 0 && (
          <span className="text-sm text-muted-foreground">
            {shows.length} of {total}
          </span>
        )}
      </div>

      {error && (
        <ErrorAlert
          message={error instanceof Error ? error.message : 'Failed to load shows'}
          onRetry={() => refetch()}
        />
      )}

      {/* Content area */}
      {shows.length > 0 ? (
        <ExportList
          items={shows}
          selectedIds={selectedIds}
          getId={getShowId}
          loading={isLoading}
          onToggle={handleToggle}
          onSelectAll={handleSelectAll}
          onClear={handleClear}
          emptyMessage="No shows loaded"
          renderItem={(show) => <ShowListItem show={show} />}
        />
      ) : (
        <div className="border rounded-lg bg-muted/30 py-8 px-4 text-center">
          {!hasLocalToken ? (
            <p className="text-sm text-muted-foreground">
              Configure your Local API token in Settings to load shows
            </p>
          ) : isLoading ? (
            <div className="flex items-center justify-center gap-2">
              <LoadingSpinner size="sm" />
              <span className="text-sm text-muted-foreground">Loading shows...</span>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Click <strong>Load Shows</strong> to fetch data from your local database
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// Generate stable ID for shows
function getShowId(show: ExportedShow): string {
  return `${show.title}-${show.eventDate}`
}

function ShowListItem({ show }: { show: ExportedShow }) {
  return (
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0">
        <div className="font-medium text-foreground truncate">{show.title}</div>
        <div className="text-sm text-muted-foreground">
          {formatDate(show.eventDate)} •{' '}
          {show.venues.map((v) => v.name).join(', ') || 'No venue'} •{' '}
          {show.artists.length} artist{show.artists.length !== 1 ? 's' : ''}
        </div>
      </div>
      <Badge
        variant={
          show.status === 'approved'
            ? 'default'
            : show.status === 'pending'
            ? 'secondary'
            : 'outline'
        }
        className="shrink-0"
      >
        {show.status}
      </Badge>
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}
