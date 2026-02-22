import { Checkbox } from '../ui/checkbox'
import { Badge } from '../ui/badge'
import { EmptyState } from '../shared/EmptyState'
import { Calendar, EyeOff, Eye } from 'lucide-react'
import type { PreviewEvent, ImportStatusMap, EventMetadataMap, FieldChange } from '../../lib/types'
import { getLocalDateString } from '../../lib/dates'

export type EventListFilter = 'active' | 'imported' | 'ignored'

interface EventListProps {
  events: PreviewEvent[]
  selectedIds: Set<string>
  importStatuses: ImportStatusMap
  eventMetadata?: EventMetadataMap
  updatableIds?: Set<string>
  onToggle: (eventId: string) => void
  onIgnore?: (eventId: string, ignored: boolean) => void
  filter?: EventListFilter
}

export function EventList({
  events,
  selectedIds,
  importStatuses,
  eventMetadata,
  updatableIds,
  onToggle,
  onIgnore,
  filter = 'active',
}: EventListProps) {
  // Filter to only show future events
  const today = getLocalDateString()
  const futureEvents = events.filter(e => e.date >= today)

  // Filter by tab
  const filteredEvents = futureEvents.filter(e => {
    const meta = eventMetadata?.[e.id]
    const isIgnored = meta?.isIgnored ?? false
    const isImported = importStatuses[e.id]?.exists ?? false

    switch (filter) {
      case 'ignored':
        return isIgnored
      case 'imported':
        return !isIgnored && isImported
      case 'active':
      default:
        return !isIgnored && !isImported
    }
  })

  if (filteredEvents.length === 0) {
    if (filter !== 'active') return null
    return (
      <EmptyState
        icon={Calendar}
        title="No upcoming events"
        description="All events are in the past or already evaluated"
        className="py-6"
      />
    )
  }

  return (
    <div className="divide-y divide-border">
      {filteredEvents.map(event => {
        const status = importStatuses[event.id]
        const meta = eventMetadata?.[event.id]
        return (
          <div
            key={event.id}
            className="flex items-center gap-3 px-4 py-3 hover:bg-muted/30 cursor-pointer"
            onClick={() => { if (filter !== 'ignored') onToggle(event.id) }}
          >
            {filter !== 'ignored' && (
              <Checkbox
                checked={selectedIds.has(event.id)}
                onCheckedChange={() => onToggle(event.id)}
                id={`event-${event.id}`}
                onClick={(e) => e.stopPropagation()}
              />
            )}
            <span className="text-sm text-muted-foreground w-16 shrink-0">
              {formatDate(event.date)}
            </span>
            <span className="text-sm text-foreground cursor-pointer flex-1 flex items-center gap-2 flex-wrap">
              {event.title}
              {meta?.isNew && (
                <Badge className="bg-purple-100 text-purple-800 hover:bg-purple-100 shrink-0">New</Badge>
              )}
              {meta?.changes && meta.changes.map(change => (
                <ChangeBadge key={change.field} change={change} />
              ))}
            </span>
            {status?.exists && <ImportStatusBadge status={status.status} hasUpdates={updatableIds?.has(event.id)} />}
            {onIgnore && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  onIgnore(event.id, filter !== 'ignored')
                }}
                className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
                title={filter === 'ignored' ? 'Unignore this event' : 'Ignore this event'}
              >
                {filter === 'ignored' ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
              </button>
            )}
          </div>
        )
      })}
    </div>
  )
}

function ChangeBadge({ change }: { change: FieldChange }) {
  switch (change.field) {
    case 'isSoldOut':
      if (change.newValue) {
        return <Badge className="bg-red-100 text-red-800 hover:bg-red-100 shrink-0">Sold Out</Badge>
      }
      return <Badge className="bg-green-100 text-green-800 hover:bg-green-100 shrink-0">Back on Sale</Badge>
    case 'isCancelled':
      if (change.newValue) {
        return <Badge className="bg-gray-200 text-gray-700 hover:bg-gray-200 shrink-0">Cancelled</Badge>
      }
      return <Badge className="bg-green-100 text-green-800 hover:bg-green-100 shrink-0">Uncancelled</Badge>
    case 'price':
      return <Badge className="bg-blue-100 text-blue-800 hover:bg-blue-100 shrink-0">Price Changed</Badge>
    case 'date':
      return <Badge className="bg-amber-100 text-amber-800 hover:bg-amber-100 shrink-0">Date Changed</Badge>
    case 'title':
      return <Badge className="bg-amber-100 text-amber-800 hover:bg-amber-100 shrink-0">Updated</Badge>
    default:
      return null
  }
}

function ImportStatusBadge({ status, hasUpdates }: { status?: string; hasUpdates?: boolean }) {
  if (hasUpdates) {
    return <Badge className="bg-blue-100 text-blue-800 hover:bg-blue-100 shrink-0">Has Updates</Badge>
  }
  switch (status) {
    case 'approved':
      return <Badge className="bg-green-100 text-green-800 hover:bg-green-100 shrink-0">Imported</Badge>
    case 'pending':
      return <Badge className="bg-yellow-100 text-yellow-800 hover:bg-yellow-100 shrink-0">Pending</Badge>
    case 'rejected':
      return <Badge className="bg-red-100 text-red-800 hover:bg-red-100 shrink-0">Rejected</Badge>
    default:
      return <Badge className="bg-gray-100 text-gray-800 hover:bg-gray-100 shrink-0">Exists</Badge>
  }
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}
