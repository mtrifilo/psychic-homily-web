import { RefreshCw, AlertCircle } from 'lucide-react'
import { Button } from '../ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ListSkeleton } from '../shared/LoadingSkeleton'
import { EmptyState } from '../shared/EmptyState'
import { EventList } from './EventList'
import type { VenueConfig, PreviewEvent, ImportStatusMap, EventMetadataMap } from '../../lib/types'
import { getLocalDateString } from '../../lib/dates'

interface VenuePreviewCardProps {
  venue: VenueConfig
  events?: PreviewEvent[]
  loading: boolean
  error?: string
  onPreview: () => void
  onRetry?: () => void
  selectedIds?: Set<string>
  importStatuses?: ImportStatusMap
  eventMetadata?: EventMetadataMap
  updatableIds?: Set<string>
  onToggle?: (eventId: string) => void
  onSelectAll?: () => void
  onSelectNone?: () => void
  onIgnore?: (eventId: string, ignored: boolean) => void
}

export function VenuePreviewCard({
  venue,
  events,
  loading,
  error,
  onPreview,
  onRetry,
  selectedIds,
  importStatuses = {},
  eventMetadata,
  updatableIds,
  onToggle,
  onSelectAll,
  onSelectNone,
  onIgnore,
}: VenuePreviewCardProps) {
  const hasEvents = events && events.length > 0
  const selectable = !!selectedIds && !!onToggle

  // Count future events by category
  const today = getLocalDateString()
  const futureEvents = events?.filter(e => e.date >= today) ?? []
  const ignoredCount = futureEvents.filter(e => eventMetadata?.[e.id]?.isIgnored).length
  const importedCount = futureEvents.filter(e => !eventMetadata?.[e.id]?.isIgnored && importStatuses[e.id]?.exists).length
  const activeCount = futureEvents.length - ignoredCount - importedCount

  return (
    <div className="bg-card rounded-lg border overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 bg-muted/50 border-b">
        <div className="flex items-center gap-3">
          <h3 className="font-medium text-foreground">{venue.name}</h3>
          <span className="text-xs text-muted-foreground">
            {venue.city}, {venue.state}
          </span>
          {hasEvents && (
            <span className="text-sm text-muted-foreground">
              {selectable
                ? `${selectedIds!.size}/${activeCount} selected`
                : `${events.length} events`
              }
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {selectable && hasEvents && (
            <div className="flex gap-2">
              <Button
                variant="link"
                size="sm"
                onClick={onSelectAll}
                className="px-0"
              >
                All
              </Button>
              <span className="text-muted-foreground">|</span>
              <Button
                variant="link"
                size="sm"
                onClick={onSelectNone}
                className="px-0 text-muted-foreground"
              >
                None
              </Button>
            </div>
          )}
          <Button
            variant={events ? 'ghost' : 'secondary'}
            size="sm"
            onClick={onPreview}
            disabled={loading}
          >
            {loading ? (
              <>
                <LoadingSpinner size="sm" />
                Loading...
              </>
            ) : events ? (
              <>
                <RefreshCw className="h-4 w-4" />
                Refresh
              </>
            ) : (
              'Preview'
            )}
          </Button>
        </div>
      </div>

      {/* Error state */}
      {error && (
        <div className="px-4 py-3 bg-destructive/10 text-destructive text-sm flex items-center justify-between">
          <div className="flex items-center gap-2">
            <AlertCircle className="h-4 w-4" />
            <span>{error}</span>
          </div>
          {onRetry && (
            <Button variant="ghost" size="sm" onClick={onRetry}>
              Retry
            </Button>
          )}
        </div>
      )}

      {/* Loading skeleton */}
      {loading && !events && (
        <div className="p-4">
          <ListSkeleton count={3} />
        </div>
      )}

      {/* Events list with tabs (selectable mode) */}
      {selectable && hasEvents && (
        <Tabs defaultValue="active">
          <div className="px-4 pt-2">
            <TabsList>
              <TabsTrigger value="active">
                New{activeCount > 0 ? ` (${activeCount})` : ''}
              </TabsTrigger>
              <TabsTrigger value="imported">
                Imported{importedCount > 0 ? ` (${importedCount})` : ''}
              </TabsTrigger>
              <TabsTrigger value="ignored">
                Ignored{ignoredCount > 0 ? ` (${ignoredCount})` : ''}
              </TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="active">
            <div className="max-h-80 overflow-y-auto">
              <EventList
                events={events}
                selectedIds={selectedIds!}
                importStatuses={importStatuses}
                eventMetadata={eventMetadata}
                updatableIds={updatableIds}
                onToggle={onToggle!}
                onIgnore={onIgnore}
                filter="active"
              />
            </div>
          </TabsContent>
          <TabsContent value="imported">
            <div className="max-h-80 overflow-y-auto">
              {importedCount > 0 ? (
                <EventList
                  events={events}
                  selectedIds={selectedIds!}
                  importStatuses={importStatuses}
                  eventMetadata={eventMetadata}
                  updatableIds={updatableIds}
                  onToggle={onToggle!}
                  onIgnore={onIgnore}
                  filter="imported"
                />
              ) : (
                <EmptyState
                  title="No imported events"
                  description="Events you've imported will appear here"
                  className="py-6"
                />
              )}
            </div>
          </TabsContent>
          <TabsContent value="ignored">
            <div className="max-h-80 overflow-y-auto">
              {ignoredCount > 0 ? (
                <EventList
                  events={events}
                  selectedIds={selectedIds!}
                  importStatuses={importStatuses}
                  eventMetadata={eventMetadata}
                  updatableIds={updatableIds}
                  onToggle={onToggle!}
                  onIgnore={onIgnore}
                  filter="ignored"
                />
              ) : (
                <EmptyState
                  title="No ignored events"
                  description="Use the eye icon to ignore events you don't want to see"
                  className="py-6"
                />
              )}
            </div>
          </TabsContent>
        </Tabs>
      )}

      {/* Events table (read-only mode) */}
      {!selectable && hasEvents && (
        <div className="max-h-64 overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 sticky top-0">
              <tr>
                <th className="px-4 py-2 text-left text-muted-foreground font-medium">
                  Date
                </th>
                <th className="px-4 py-2 text-left text-muted-foreground font-medium">
                  Event
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {events.map(event => (
                <tr key={event.id} className="hover:bg-muted/30">
                  <td className="px-4 py-2 text-muted-foreground whitespace-nowrap">
                    {formatDate(event.date)}
                  </td>
                  <td className="px-4 py-2 text-foreground">{event.title}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Empty state - no events loaded yet */}
      {!loading && !events && !error && (
        <EmptyState
          title="No events loaded"
          description="Click Preview to load events"
          className="py-8"
        />
      )}

      {/* Empty state - no events found */}
      {events && events.length === 0 && (
        <EmptyState
          title="No events found"
          description="This venue has no upcoming events"
          className="py-8"
        />
      )}
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}
