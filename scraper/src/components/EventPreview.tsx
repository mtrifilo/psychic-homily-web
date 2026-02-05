import { useState } from 'react'
import type { VenueConfig, PreviewEvent } from '../lib/types'
import { previewVenueEvents, previewVenueEventsBatch } from '../lib/api'

interface Props {
  venues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  onPreviewComplete: (venueSlug: string, events: PreviewEvent[]) => void
  onBack: () => void
  onNext: () => void
}

export function EventPreview({
  venues,
  previewEvents,
  onPreviewComplete,
  onBack,
  onNext,
}: Props) {
  const [loading, setLoading] = useState<Record<string, boolean>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [batchLoading, setBatchLoading] = useState(false)
  const [batchProgress, setBatchProgress] = useState({ completed: 0, total: 0 })

  const previewVenue = async (venue: VenueConfig) => {
    setLoading(prev => ({ ...prev, [venue.slug]: true }))
    setErrors(prev => ({ ...prev, [venue.slug]: '' }))

    try {
      const events = await previewVenueEvents(venue.slug)
      onPreviewComplete(venue.slug, events)
    } catch (error) {
      setErrors(prev => ({
        ...prev,
        [venue.slug]: error instanceof Error ? error.message : 'Failed to preview',
      }))
    } finally {
      setLoading(prev => ({ ...prev, [venue.slug]: false }))
    }
  }

  const previewAllParallel = async () => {
    // Get venues that haven't been previewed yet
    const venuesToPreview = venues.filter(v => !previewEvents[v.slug])
    if (venuesToPreview.length === 0) return

    setBatchLoading(true)
    setBatchProgress({ completed: 0, total: venuesToPreview.length })

    try {
      // Use batch preview endpoint for parallel processing
      const results = await previewVenueEventsBatch(venuesToPreview.map(v => v.slug))

      // Process results
      for (const result of results) {
        if (result.error) {
          setErrors(prev => ({ ...prev, [result.venueSlug]: result.error! }))
        } else if (result.events) {
          onPreviewComplete(result.venueSlug, result.events)
        }
        setBatchProgress(prev => ({ ...prev, completed: prev.completed + 1 }))
      }
    } catch (error) {
      // If batch fails, show a general error
      const message = error instanceof Error ? error.message : 'Batch preview failed'
      for (const venue of venuesToPreview) {
        if (!previewEvents[venue.slug]) {
          setErrors(prev => ({ ...prev, [venue.slug]: message }))
        }
      }
    } finally {
      setBatchLoading(false)
      setBatchProgress({ completed: 0, total: 0 })
    }
  }

  const allPreviewed = venues.every(v => previewEvents[v.slug])
  const totalEvents = Object.values(previewEvents).reduce(
    (sum, events) => sum + events.length,
    0
  )
  const previewedCount = venues.filter(v => previewEvents[v.slug]).length
  const pendingCount = venues.length - previewedCount

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Preview Events</h2>
          <p className="text-sm text-gray-500 mt-1">
            Quick scan of upcoming events at each venue
            {previewedCount > 0 && ` (${previewedCount}/${venues.length} loaded)`}
          </p>
        </div>
        <button
          onClick={previewAllParallel}
          disabled={allPreviewed || batchLoading}
          className={`px-4 py-2 rounded-lg text-sm font-medium flex items-center gap-2 ${
            allPreviewed || batchLoading
              ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
              : 'bg-blue-600 text-white hover:bg-blue-700'
          }`}
        >
          {batchLoading && (
            <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
          )}
          {batchLoading
            ? `Previewing (${batchProgress.completed}/${batchProgress.total})...`
            : allPreviewed
            ? 'All Previewed'
            : `Preview All (${pendingCount})`}
        </button>
      </div>

      {/* Batch progress bar */}
      {batchLoading && batchProgress.total > 0 && (
        <div className="bg-blue-50 rounded-lg p-4">
          <div className="flex items-center justify-between text-sm text-blue-700 mb-2">
            <span>Scraping {batchProgress.total} venues in parallel...</span>
            <span>{Math.round((batchProgress.completed / batchProgress.total) * 100)}%</span>
          </div>
          <div className="h-2 bg-blue-200 rounded-full overflow-hidden">
            <div
              className="h-full bg-blue-600 transition-all duration-300"
              style={{ width: `${(batchProgress.completed / batchProgress.total) * 100}%` }}
            />
          </div>
        </div>
      )}

      <div className="space-y-4">
        {venues.map(venue => (
          <div
            key={venue.slug}
            className="bg-white rounded-lg border border-gray-200 overflow-hidden"
          >
            <div className="flex items-center justify-between px-4 py-3 bg-gray-50 border-b">
              <div className="flex items-center gap-3">
                <h3 className="font-medium text-gray-900">{venue.name}</h3>
                <span className="text-xs text-gray-400">{venue.city}, {venue.state}</span>
                {previewEvents[venue.slug] && (
                  <span className="text-sm text-gray-500">
                    {previewEvents[venue.slug].length} events
                  </span>
                )}
              </div>
              <button
                onClick={() => previewVenue(venue)}
                disabled={loading[venue.slug] || batchLoading}
                className={`px-3 py-1.5 rounded text-sm font-medium ${
                  loading[venue.slug] || batchLoading
                    ? 'bg-gray-100 text-gray-400'
                    : previewEvents[venue.slug]
                    ? 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                    : 'bg-blue-100 text-blue-700 hover:bg-blue-200'
                }`}
              >
                {loading[venue.slug]
                  ? 'Loading...'
                  : previewEvents[venue.slug]
                  ? 'Refresh'
                  : 'Preview'}
              </button>
            </div>

            {errors[venue.slug] && (
              <div className="px-4 py-3 bg-red-50 text-red-700 text-sm">
                {errors[venue.slug]}
              </div>
            )}

            {previewEvents[venue.slug] && (
              <div className="max-h-64 overflow-y-auto">
                <table className="w-full text-sm">
                  <thead className="bg-gray-50 sticky top-0">
                    <tr>
                      <th className="px-4 py-2 text-left text-gray-600 font-medium">
                        Date
                      </th>
                      <th className="px-4 py-2 text-left text-gray-600 font-medium">
                        Event
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100">
                    {previewEvents[venue.slug].map(event => (
                      <tr key={event.id} className="hover:bg-gray-50">
                        <td className="px-4 py-2 text-gray-500 whitespace-nowrap">
                          {formatDate(event.date)}
                        </td>
                        <td className="px-4 py-2 text-gray-900">{event.title}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {!previewEvents[venue.slug] && !loading[venue.slug] && !errors[venue.slug] && !batchLoading && (
              <div className="px-4 py-8 text-center text-gray-400">
                Click "Preview" to load events
              </div>
            )}

            {(loading[venue.slug] || (batchLoading && !previewEvents[venue.slug])) && (
              <div className="px-4 py-8 text-center">
                <div className="inline-block animate-spin rounded-full h-6 w-6 border-2 border-gray-300 border-t-blue-600" />
                <p className="mt-2 text-sm text-gray-500">Loading events...</p>
              </div>
            )}
          </div>
        ))}
      </div>

      <div className="flex justify-between">
        <button
          onClick={onBack}
          className="px-4 py-2 rounded-lg text-gray-600 hover:bg-gray-100"
        >
          Back
        </button>
        <button
          onClick={onNext}
          disabled={totalEvents === 0}
          className={`px-4 py-2 rounded-lg font-medium ${
            totalEvents > 0
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'bg-gray-200 text-gray-400 cursor-not-allowed'
          }`}
        >
          Select Events ({totalEvents} available)
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
