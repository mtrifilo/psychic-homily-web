import { useState } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { VenuePreviewCard } from './discovery/VenuePreviewCard'
import { LoadingSpinner } from './shared/LoadingSpinner'
import { previewVenueEvents, previewVenueEventsBatch, checkImportStatus } from '../lib/api'
import type { VenueConfig, PreviewEvent, ImportStatusMap } from '../lib/types'

interface Props {
  venues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  onPreviewComplete: (venueSlug: string, events: PreviewEvent[]) => void
  onSetImportStatuses: (statuses: ImportStatusMap) => void
  onBack: () => void
  onNext: () => void
}

export function EventPreview({
  venues,
  previewEvents,
  onPreviewComplete,
  onSetImportStatuses,
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
    const venuesToPreview = venues.filter(v => !previewEvents[v.slug])
    if (venuesToPreview.length === 0) return

    setBatchLoading(true)
    setBatchProgress({ completed: 0, total: venuesToPreview.length })

    try {
      const results = await previewVenueEventsBatch(venuesToPreview.map(v => v.slug))

      for (const result of results) {
        if (result.error) {
          setErrors(prev => ({ ...prev, [result.venueSlug]: result.error! }))
        } else if (result.events) {
          onPreviewComplete(result.venueSlug, result.events)
        }
        setBatchProgress(prev => ({ ...prev, completed: prev.completed + 1 }))
      }
    } catch (error) {
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

  const retryVenue = async (venue: VenueConfig) => {
    setErrors(prev => ({ ...prev, [venue.slug]: '' }))
    await previewVenue(venue)
  }

  const retryFailed = async () => {
    const failedVenues = venues.filter(v => errors[v.slug])
    for (const venue of failedVenues) {
      await previewVenue(venue)
    }
  }

  const [checkingStatus, setCheckingStatus] = useState(false)

  const handleNext = async () => {
    // Check import status for all previewed events before navigating
    const allEvents = Object.entries(previewEvents).flatMap(([venueSlug, events]) =>
      events.map(e => ({ id: e.id, venueSlug }))
    )

    if (allEvents.length > 0) {
      setCheckingStatus(true)
      try {
        const statuses = await checkImportStatus(allEvents)
        onSetImportStatuses(statuses)
      } catch {
        // Non-critical — continue without badges
      } finally {
        setCheckingStatus(false)
      }
    }

    onNext()
  }

  const allPreviewed = venues.every(v => previewEvents[v.slug])
  const totalEvents = Object.values(previewEvents).reduce(
    (sum, events) => sum + events.length,
    0
  )
  const previewedCount = venues.filter(v => previewEvents[v.slug]).length
  const pendingCount = venues.length - previewedCount
  const failedCount = venues.filter(v => errors[v.slug]).length

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-foreground">Preview Events</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Quick scan of upcoming events at each venue
            {previewedCount > 0 && (
              <Badge variant="secondary" className="ml-2">
                {previewedCount}/{venues.length} loaded
              </Badge>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {failedCount > 0 && (
            <Button variant="outline" onClick={retryFailed}>
              Retry Failed ({failedCount})
            </Button>
          )}
          <Button
            onClick={previewAllParallel}
            disabled={allPreviewed || batchLoading}
          >
            {batchLoading && <LoadingSpinner size="sm" className="mr-2" />}
            {batchLoading
              ? `Previewing (${batchProgress.completed}/${batchProgress.total})...`
              : allPreviewed
              ? 'All Previewed'
              : `Preview All (${pendingCount})`}
          </Button>
        </div>
      </div>

      {/* Batch progress bar */}
      {batchLoading && batchProgress.total > 0 && (
        <div className="bg-primary/10 rounded-lg p-4">
          <div className="flex items-center justify-between text-sm text-primary mb-2">
            <span>Scraping {batchProgress.total} venues in parallel...</span>
            <span>{Math.round((batchProgress.completed / batchProgress.total) * 100)}%</span>
          </div>
          <div className="h-2 bg-primary/20 rounded-full overflow-hidden">
            <div
              className="h-full bg-primary transition-all duration-300"
              style={{ width: `${(batchProgress.completed / batchProgress.total) * 100}%` }}
            />
          </div>
        </div>
      )}

      <div className="space-y-4">
        {venues.map(venue => (
          <VenuePreviewCard
            key={venue.slug}
            venue={venue}
            events={previewEvents[venue.slug]}
            loading={loading[venue.slug] || (batchLoading && !previewEvents[venue.slug])}
            error={errors[venue.slug]}
            onPreview={() => previewVenue(venue)}
            onRetry={() => retryVenue(venue)}
          />
        ))}
      </div>

      <div className="flex justify-between">
        <Button variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button onClick={handleNext} disabled={totalEvents === 0 || checkingStatus}>
          {checkingStatus && <LoadingSpinner size="sm" className="mr-2" />}
          {checkingStatus ? 'Checking status...' : `Select Events (${totalEvents} available)`}
        </Button>
      </div>
    </div>
  )
}
