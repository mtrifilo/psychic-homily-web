import { Checkbox } from '../ui/checkbox'
import { Label } from '../ui/label'
import { Badge } from '../ui/badge'
import { EmptyState } from '../shared/EmptyState'
import { Calendar } from 'lucide-react'
import type { PreviewEvent, ImportStatusMap } from '../../lib/types'
import { getLocalDateString } from '../../lib/dates'

interface EventListProps {
  events: PreviewEvent[]
  selectedIds: Set<string>
  importStatuses: ImportStatusMap
  updatableIds?: Set<string>
  onToggle: (eventId: string) => void
}

export function EventList({ events, selectedIds, importStatuses, updatableIds, onToggle }: EventListProps) {
  // Filter to only show future events
  const today = getLocalDateString()
  const futureEvents = events.filter(e => e.date >= today)

  if (futureEvents.length === 0) {
    return (
      <EmptyState
        icon={Calendar}
        title="No upcoming events"
        description="All events are in the past"
        className="py-6"
      />
    )
  }

  return (
    <div className="divide-y divide-border">
      {futureEvents.map(event => {
        const status = importStatuses[event.id]
        return (
          <label
            key={event.id}
            className="flex items-center gap-3 px-4 py-3 hover:bg-muted/30 cursor-pointer"
          >
            <Checkbox
              checked={selectedIds.has(event.id)}
              onCheckedChange={() => onToggle(event.id)}
              id={`event-${event.id}`}
            />
            <span className="text-sm text-muted-foreground w-16 shrink-0">
              {formatDate(event.date)}
            </span>
            <Label
              htmlFor={`event-${event.id}`}
              className="text-sm text-foreground cursor-pointer flex-1"
            >
              {event.title}
            </Label>
            {status?.exists && <ImportStatusBadge status={status.status} hasUpdates={updatableIds?.has(event.id)} />}
          </label>
        )
      })}
    </div>
  )
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
