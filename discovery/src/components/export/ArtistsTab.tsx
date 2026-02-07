import { useState } from 'react'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ErrorAlert } from '../shared/ErrorAlert'
import { ExportList } from './ExportList'
import { useExportArtists } from '../../lib/hooks/useExport'
import { Lock } from 'lucide-react'
import type { ExportedArtist } from '../../lib/types'

interface ArtistsTabProps {
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onDataLoaded: (artists: ExportedArtist[]) => void
  hasLocalToken: boolean
}

export function ArtistsTab({
  selectedIds,
  onSelectionChange,
  onDataLoaded,
  hasLocalToken,
}: ArtistsTabProps) {
  const [search, setSearch] = useState('')
  const [enabled, setEnabled] = useState(false)

  const { data, isLoading, error, refetch } = useExportArtists(
    { limit: 100, search },
    enabled && hasLocalToken
  )

  const artists = data?.artists || []
  const total = data?.total || 0

  const handleLoad = () => {
    setEnabled(true)
    if (enabled) {
      refetch()
    }
  }

  // Update parent when data loads
  if (data?.artists) {
    onDataLoaded(data.artists)
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
    onSelectionChange(new Set(artists.map(getArtistId)))
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
          placeholder="Search artists..."
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
          {isLoading ? 'Loading...' : 'Load Artists'}
        </Button>
        {artists.length > 0 && (
          <span className="text-sm text-muted-foreground">
            {artists.length} of {total}
          </span>
        )}
      </div>

      {error && (
        <ErrorAlert
          message={error instanceof Error ? error.message : 'Failed to load artists'}
          onRetry={() => refetch()}
        />
      )}

      {/* Content area */}
      {artists.length > 0 ? (
        <ExportList
          items={artists}
          selectedIds={selectedIds}
          getId={getArtistId}
          loading={isLoading}
          onToggle={handleToggle}
          onSelectAll={handleSelectAll}
          onClear={handleClear}
          emptyMessage="No artists loaded"
          renderItem={(artist) => <ArtistListItem artist={artist} />}
        />
      ) : (
        <div className="border rounded-lg bg-muted/30 py-8 px-4 text-center">
          {!hasLocalToken ? (
            <p className="text-sm text-muted-foreground">
              Configure your Local API token in Settings to load artists
            </p>
          ) : isLoading ? (
            <div className="flex items-center justify-center gap-2">
              <LoadingSpinner size="sm" />
              <span className="text-sm text-muted-foreground">Loading artists...</span>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Click <strong>Load Artists</strong> to fetch data from your local database
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// Generate stable ID for artists
function getArtistId(artist: ExportedArtist): string {
  return artist.name
}

function ArtistListItem({ artist }: { artist: ExportedArtist }) {
  return (
    <div>
      <div className="font-medium text-foreground">{artist.name}</div>
      {(artist.city || artist.state) && (
        <div className="text-sm text-muted-foreground">
          {[artist.city, artist.state].filter(Boolean).join(', ')}
        </div>
      )}
    </div>
  )
}
