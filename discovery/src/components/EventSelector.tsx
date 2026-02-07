import { useState } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { EventList } from './discovery/EventList'
import { LoadingSpinner } from './shared/LoadingSpinner'
import { ErrorAlert } from './shared/ErrorAlert'
import { scrapeVenueEvents } from '../lib/api'
import type { VenueConfig, PreviewEvent, ScrapedEvent, ImportStatusMap } from '../lib/types'

interface Props {
  venues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  selectedEventIds: Record<string, Set<string>>
  importStatuses: ImportStatusMap
  scrapedEvents: ScrapedEvent[]
  onToggle: (venueSlug: string, eventId: string) => void
  onSelectAll: (venueSlug: string) => void
  onSelectNone: (venueSlug: string) => void
  onScrapeComplete: (events: ScrapedEvent[]) => void
  onBack: () => void
  onNext: () => void
}

export function EventSelector({
  venues,
  previewEvents,
  selectedEventIds,
  importStatuses,
  scrapedEvents,
  onToggle,
  onSelectAll,
  onSelectNone,
  onScrapeComplete,
  onBack,
  onNext,
}: Props) {
  const [loading, setLoading] = useState(false)
  const [scrapeProgress, setScrapeProgress] = useState<string>('')
  const [error, setError] = useState<string>('')

  const totalSelected = Object.values(selectedEventIds).reduce(
    (sum, set) => sum + set.size,
    0
  )

  const scrapeSelected = async () => {
    setLoading(true)
    setError('')

    try {
      for (const venue of venues) {
        const ids = Array.from(selectedEventIds[venue.slug] || new Set())
        if (ids.length === 0) continue

        setScrapeProgress(`Scraping ${venue.name}...`)
        const events = await scrapeVenueEvents(venue.slug, ids)
        onScrapeComplete(events)
      }

      setScrapeProgress('')
      onNext()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to scrape events')
    } finally {
      setLoading(false)
      setScrapeProgress('')
    }
  }

  // Filter to only show past today
  const today = new Date().toISOString().split('T')[0]

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-foreground">Select Events</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Choose which events to scrape and import
          </p>
        </div>
        <Badge variant="secondary">
          {totalSelected} event{totalSelected !== 1 ? 's' : ''} selected
        </Badge>
      </div>

      {error && (
        <ErrorAlert
          message={error}
          onRetry={() => {
            setError('')
            scrapeSelected()
          }}
        />
      )}

      <div className="space-y-4">
        {venues.map(venue => {
          const events = previewEvents[venue.slug] || []
          const selected = selectedEventIds[venue.slug] || new Set()
          const futureEvents = events.filter(e => e.date >= today)

          return (
            <div
              key={venue.slug}
              className="bg-card rounded-lg border overflow-hidden"
            >
              <div className="flex items-center justify-between px-4 py-3 bg-muted/50 border-b">
                <div className="flex items-center gap-3">
                  <h3 className="font-medium text-foreground">{venue.name}</h3>
                  <span className="text-sm text-muted-foreground">
                    {selected.size}/{futureEvents.length} selected
                  </span>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="link"
                    size="sm"
                    onClick={() => onSelectAll(venue.slug)}
                    className="px-0"
                  >
                    All
                  </Button>
                  <span className="text-muted-foreground">|</span>
                  <Button
                    variant="link"
                    size="sm"
                    onClick={() => onSelectNone(venue.slug)}
                    className="px-0 text-muted-foreground"
                  >
                    None
                  </Button>
                </div>
              </div>

              <div className="max-h-80 overflow-y-auto">
                <EventList
                  events={events}
                  selectedIds={selected}
                  importStatuses={importStatuses}
                  onToggle={(eventId) => onToggle(venue.slug, eventId)}
                />
              </div>
            </div>
          )
        })}
      </div>

      <div className="flex justify-between">
        <Button variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button
          onClick={scrapeSelected}
          disabled={loading || totalSelected === 0}
        >
          {loading && <LoadingSpinner size="sm" className="mr-2" />}
          {loading ? scrapeProgress || 'Scraping...' : `Scrape ${totalSelected} Events`}
        </Button>
      </div>
    </div>
  )
}
