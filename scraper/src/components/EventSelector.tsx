import { useState } from 'react'
import type { VenueConfig, PreviewEvent, ScrapedEvent } from '../lib/types'
import { scrapeVenueEvents } from '../lib/api'

interface Props {
  venues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  selectedEventIds: Record<string, Set<string>>
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
          <h2 className="text-lg font-semibold text-gray-900">Select Events</h2>
          <p className="text-sm text-gray-500 mt-1">
            Choose which events to scrape and import
          </p>
        </div>
        <div className="text-sm text-gray-600">
          {totalSelected} event{totalSelected !== 1 ? 's' : ''} selected
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg px-4 py-3 text-red-700">
          {error}
        </div>
      )}

      <div className="space-y-4">
        {venues.map(venue => {
          const events = previewEvents[venue.slug] || []
          const selected = selectedEventIds[venue.slug] || new Set()
          const futureEvents = events.filter(e => e.date >= today)

          return (
            <div
              key={venue.slug}
              className="bg-white rounded-lg border border-gray-200 overflow-hidden"
            >
              <div className="flex items-center justify-between px-4 py-3 bg-gray-50 border-b">
                <div className="flex items-center gap-3">
                  <h3 className="font-medium text-gray-900">{venue.name}</h3>
                  <span className="text-sm text-gray-500">
                    {selected.size}/{futureEvents.length} selected
                  </span>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => onSelectAll(venue.slug)}
                    className="text-sm text-blue-600 hover:text-blue-700"
                  >
                    All
                  </button>
                  <span className="text-gray-300">|</span>
                  <button
                    onClick={() => onSelectNone(venue.slug)}
                    className="text-sm text-gray-500 hover:text-gray-700"
                  >
                    None
                  </button>
                </div>
              </div>

              <div className="max-h-80 overflow-y-auto">
                {futureEvents.length === 0 ? (
                  <div className="px-4 py-6 text-center text-gray-400">
                    No upcoming events
                  </div>
                ) : (
                  <div className="divide-y divide-gray-100">
                    {futureEvents.map(event => (
                      <label
                        key={event.id}
                        className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 cursor-pointer"
                      >
                        <input
                          type="checkbox"
                          checked={selected.has(event.id)}
                          onChange={() => onToggle(venue.slug, event.id)}
                          className="w-4 h-4 text-blue-600 rounded border-gray-300"
                        />
                        <span className="text-sm text-gray-500 w-16 shrink-0">
                          {formatDate(event.date)}
                        </span>
                        <span className="text-sm text-gray-900">{event.title}</span>
                      </label>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>

      <div className="flex justify-between">
        <button
          onClick={onBack}
          className="px-4 py-2 rounded-lg text-gray-600 hover:bg-gray-100"
        >
          Back
        </button>
        <button
          onClick={scrapeSelected}
          disabled={loading || totalSelected === 0}
          className={`px-4 py-2 rounded-lg font-medium flex items-center gap-2 ${
            loading || totalSelected === 0
              ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
              : 'bg-blue-600 text-white hover:bg-blue-700'
          }`}
        >
          {loading && (
            <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
          )}
          {loading ? scrapeProgress || 'Scraping...' : `Scrape ${totalSelected} Events`}
        </button>
      </div>
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
