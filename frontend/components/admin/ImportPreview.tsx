'use client'

import { memo } from 'react'
import { CheckCircle, PlusCircle, AlertTriangle, Calendar, MapPin, Music } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import type { ImportPreviewResponse } from '@/lib/hooks/useShowImport'

interface ImportPreviewProps {
  preview: ImportPreviewResponse
}

/**
 * Displays the parsed show data before confirming import
 * Memoized to prevent unnecessary re-renders during import flow
 */
export const ImportPreview = memo(function ImportPreview({ preview }: ImportPreviewProps) {
  const formatDate = (dateStr: string) => {
    try {
      const date = new Date(dateStr)
      return date.toLocaleDateString('en-US', {
        weekday: 'short',
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      })
    } catch {
      return dateStr
    }
  }

  return (
    <div className="space-y-6">
      {/* Warnings */}
      {preview.warnings.length > 0 && (
        <div className="rounded-lg border border-amber-500/50 bg-amber-500/10 p-4">
          <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
            <AlertTriangle className="h-5 w-5" />
            <span className="font-medium">Warnings</span>
          </div>
          <ul className="mt-2 space-y-1 text-sm text-amber-700 dark:text-amber-300">
            {preview.warnings.map((warning, i) => (
              <li key={i}>{warning}</li>
            ))}
          </ul>
        </div>
      )}

      {/* Show Details */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="font-semibold mb-3">Show Details</h3>
        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-2">
            <Music className="h-4 w-4 text-muted-foreground" />
            <span className="font-medium">
              {preview.show.title || 'Untitled Show'}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Calendar className="h-4 w-4 text-muted-foreground" />
            <span>{formatDate(preview.show.event_date)}</span>
          </div>
          {(preview.show.city || preview.show.state) && (
            <div className="flex items-center gap-2">
              <MapPin className="h-4 w-4 text-muted-foreground" />
              <span>
                {[preview.show.city, preview.show.state]
                  .filter(Boolean)
                  .join(', ')}
              </span>
            </div>
          )}
          {preview.show.price !== undefined && preview.show.price !== null && (
            <div className="text-muted-foreground">
              Price: ${preview.show.price.toFixed(2)}
            </div>
          )}
          {preview.show.age_requirement && (
            <div className="text-muted-foreground">
              Age: {preview.show.age_requirement}
            </div>
          )}
        </div>
      </div>

      {/* Venues */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="font-semibold mb-3">
          Venues ({preview.venues.length})
        </h3>
        <div className="space-y-2">
          {preview.venues.map((venue, i) => (
            <div
              key={i}
              className="flex items-center justify-between rounded-md bg-muted/50 px-3 py-2"
            >
              <div>
                <span className="font-medium">{venue.name}</span>
                <span className="text-sm text-muted-foreground ml-2">
                  {venue.city}, {venue.state}
                </span>
              </div>
              {venue.will_create ? (
                <Badge variant="secondary" className="gap-1">
                  <PlusCircle className="h-3 w-3" />
                  Will create
                </Badge>
              ) : (
                <Badge variant="outline" className="gap-1 text-green-600">
                  <CheckCircle className="h-3 w-3" />
                  Exists
                </Badge>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Artists */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="font-semibold mb-3">
          Artists ({preview.artists.length})
        </h3>
        <div className="space-y-2">
          {preview.artists.map((artist, i) => (
            <div
              key={i}
              className="flex items-center justify-between rounded-md bg-muted/50 px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <span className="font-medium">{artist.name}</span>
                {artist.set_type === 'headliner' && (
                  <Badge variant="default" className="text-xs">
                    Headliner
                  </Badge>
                )}
                {artist.set_type === 'opener' && (
                  <Badge variant="secondary" className="text-xs">
                    Opener
                  </Badge>
                )}
              </div>
              {artist.will_create ? (
                <Badge variant="secondary" className="gap-1">
                  <PlusCircle className="h-3 w-3" />
                  Will create
                </Badge>
              ) : (
                <Badge variant="outline" className="gap-1 text-green-600">
                  <CheckCircle className="h-3 w-3" />
                  Exists
                </Badge>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Import Status */}
      <div
        className={`rounded-lg border p-4 ${
          preview.can_import
            ? 'border-green-500/50 bg-green-500/10'
            : 'border-destructive/50 bg-destructive/10'
        }`}
      >
        <div className="flex items-center gap-2">
          {preview.can_import ? (
            <>
              <CheckCircle className="h-5 w-5 text-green-600" />
              <span className="font-medium text-green-700 dark:text-green-400">
                Ready to import
              </span>
            </>
          ) : (
            <>
              <AlertTriangle className="h-5 w-5 text-destructive" />
              <span className="font-medium text-destructive">
                Cannot import - fix the warnings above
              </span>
            </>
          )}
        </div>
      </div>
    </div>
  )
})
