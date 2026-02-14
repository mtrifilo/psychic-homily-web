import { useState, useEffect } from 'react'
import { Badge } from '../ui/badge'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ErrorAlert } from '../shared/ErrorAlert'
import { ExportList } from './ExportList'
import { useExportShows } from '../../lib/hooks/useExport'
import type { ExportedShow } from '../../lib/types'

interface ShowsTabProps {
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onDataLoaded: (shows: ExportedShow[]) => void
  hasLocalToken: boolean
  stageShowIds?: Set<string>
  productionShowIds?: Set<string>
}

export function ShowsTab({
  selectedIds,
  onSelectionChange,
  onDataLoaded,
  hasLocalToken,
  stageShowIds,
  productionShowIds,
}: ShowsTabProps) {
  const [showStatus, setShowStatus] = useState('approved')

  const { data, isLoading, error, refetch } = useExportShows(
    { limit: 100, status: showStatus },
    hasLocalToken
  )

  const shows = data?.shows || []
  const total = data?.total || 0

  // Update parent when data loads
  useEffect(() => {
    if (data?.shows) {
      onDataLoaded(data.shows)
    }
  }, [data?.shows, onDataLoaded])

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
            onSelectionChange(new Set())
          }}
          disabled={!hasLocalToken}
          className="h-9 px-3 border rounded-md text-sm bg-background disabled:opacity-50"
        >
          <option value="approved">Approved</option>
          <option value="pending">Pending</option>
          <option value="all">All</option>
        </select>
        {isLoading && <LoadingSpinner size="sm" />}
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
          renderItem={(show) => (
            <ShowListItem
              show={show}
              stageShowIds={stageShowIds}
              productionShowIds={productionShowIds}
            />
          )}
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
              No shows found
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

function ShowListItem({
  show,
  stageShowIds,
  productionShowIds,
}: {
  show: ExportedShow
  stageShowIds?: Set<string>
  productionShowIds?: Set<string>
}) {
  const showKey = getShowId(show)
  const onStage = stageShowIds?.has(showKey) ?? false
  const onProd = productionShowIds?.has(showKey) ?? false

  const artistNames = show.artists
    .sort((a, b) => a.position - b.position)
    .map((a) => a.name)

  const detailParts: string[] = []
  const venueName = show.venues.map((v) => v.name).join(', ')
  if (venueName) detailParts.push(venueName)
  if (show.price != null) detailParts.push(formatPrice(show.price))
  if (show.ageRequirement) detailParts.push(show.ageRequirement)
  const time = formatTime(show.eventDate)
  if (time) detailParts.push(time)
  detailParts.push(formatDate(show.eventDate))

  return (
    <div className={show.isCancelled ? 'opacity-60' : undefined}>
      {/* Line 1: Artist names + badges */}
      <div className="flex items-center gap-2">
        <div className="min-w-0 truncate font-medium text-foreground">
          {artistNames.length > 0 ? artistNames.join(' \u2022 ') : show.title}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {show.isCancelled && (
            <Badge variant="outline" className="text-xs text-red-600 border-red-300">Cancelled</Badge>
          )}
          {show.isSoldOut && (
            <Badge variant="outline" className="text-xs text-orange-600 border-orange-300">Sold Out</Badge>
          )}
          <Badge
            variant={
              show.status === 'approved'
                ? 'default'
                : show.status === 'pending'
                ? 'secondary'
                : 'outline'
            }
          >
            {show.status}
          </Badge>
          {onStage && onProd && (
            <Badge variant="outline" className="text-xs">Both</Badge>
          )}
          {onStage && !onProd && (
            <Badge variant="outline" className="text-xs text-blue-600 border-blue-300">Stage</Badge>
          )}
          {!onStage && onProd && (
            <Badge variant="outline" className="text-xs text-green-600 border-green-300">Prod</Badge>
          )}
        </div>
      </div>
      {/* Line 2: Venue, price, age, time, date */}
      <div className="text-sm text-muted-foreground truncate">
        {detailParts.map((part, i) => (
          <span key={i}>
            {i === 0 && venueName ? (
              <span className="text-primary">{part}</span>
            ) : (
              part
            )}
            {i < detailParts.length - 1 && ' \u2022 '}
          </span>
        ))}
      </div>
      {/* Line 3: Description (optional) */}
      {show.description && (
        <div className="text-xs text-muted-foreground/70 line-clamp-1 mt-0.5">
          {show.description}
        </div>
      )}
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr)
  const hours = date.getHours()
  const minutes = date.getMinutes()
  // Skip if midnight (no time info)
  if (hours === 0 && minutes === 0) return ''
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  })
}

function formatPrice(price: number): string {
  if (price === 0) return 'Free'
  return `$${price.toFixed(2)}`
}
