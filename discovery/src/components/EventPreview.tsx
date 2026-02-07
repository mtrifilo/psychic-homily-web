import { useState, useEffect, useRef } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { VenuePreviewCard } from './discovery/VenuePreviewCard'
import { LoadingSpinner } from './shared/LoadingSpinner'
import { ErrorAlert } from './shared/ErrorAlert'
import { previewVenueEvents, previewVenueEventsBatch, checkImportStatus, scrapeVenueEvents } from '../lib/api'
import type { VenueConfig, PreviewEvent, ScrapedEvent, ImportStatusMap } from '../lib/types'

interface Props {
  venues: VenueConfig[]
  previewEvents: Record<string, PreviewEvent[]>
  selectedEventIds: Record<string, Set<string>>
  importStatuses: ImportStatusMap
  onPreviewComplete: (venueSlug: string, events: PreviewEvent[]) => void
  onSetImportStatuses: (statuses: ImportStatusMap) => void
  onToggle: (venueSlug: string, eventId: string) => void
  onSelectAll: (venueSlug: string) => void
  onSelectNone: (venueSlug: string) => void
  onScrapeComplete: (events: ScrapedEvent[]) => void
  onBack: () => void
  onNext: () => void
}

export function EventPreview({
  venues,
  previewEvents,
  selectedEventIds,
  importStatuses,
  onPreviewComplete,
  onSetImportStatuses,
  onToggle,
  onSelectAll,
  onSelectNone,
  onScrapeComplete,
  onBack,
  onNext,
}: Props) {
  const [loading, setLoading] = useState<Record<string, boolean>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [batchLoading, setBatchLoading] = useState(false)
  const [batchProgress, setBatchProgress] = useState({ completed: 0, total: 0 })
  const [scraping, setScraping] = useState(false)
  const [scrapeProgress, setScrapeProgress] = useState('')
  const [scrapeError, setScrapeError] = useState('')

  // Check import statuses in the background whenever previews change
  const checkedSlugsRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    const newSlugs = Object.keys(previewEvents).filter(s => !checkedSlugsRef.current.has(s))
    if (newSlugs.length === 0) return

    newSlugs.forEach(s => checkedSlugsRef.current.add(s))

    const events = newSlugs.flatMap(venueSlug =>
      (previewEvents[venueSlug] || []).map(e => ({ id: e.id, venueSlug }))
    )
    if (events.length === 0) return

    checkImportStatus(events)
      .then(statuses => onSetImportStatuses(statuses))
      .catch(() => {}) // Non-critical
  }, [previewEvents, onSetImportStatuses])

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

  // Auto-preview venues when entering step or when venues change
  const autoPreviewedSlugsRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    // Prune stale slugs that are no longer in the venue list
    const currentSlugs = new Set(venues.map(v => v.slug))
    for (const slug of autoPreviewedSlugsRef.current) {
      if (!currentSlugs.has(slug)) {
        autoPreviewedSlugsRef.current.delete(slug)
      }
    }

    const unpreviewed = venues.filter(
      v => !previewEvents[v.slug] && !autoPreviewedSlugsRef.current.has(v.slug)
    )
    if (unpreviewed.length === 0) return

    for (const v of unpreviewed) {
      autoPreviewedSlugsRef.current.add(v.slug)
    }
    previewAllParallel()
  }, [venues]) // eslint-disable-line react-hooks/exhaustive-deps

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

  const scrapeSelected = async () => {
    setScraping(true)
    setScrapeError('')

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
      setScrapeError(err instanceof Error ? err.message : 'Failed to scrape events')
    } finally {
      setScraping(false)
      setScrapeProgress('')
    }
  }

  const totalSelected = Object.values(selectedEventIds).reduce(
    (sum, set) => sum + set.size,
    0
  )
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
          <h2 className="text-lg font-semibold text-foreground">Preview & Select Events</h2>
          <div className="text-sm text-muted-foreground mt-1">
            Preview venues, then select events to scrape
            {previewedCount > 0 && (
              <Badge variant="secondary" className="ml-2">
                {previewedCount}/{venues.length} loaded
              </Badge>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {failedCount > 0 && (
            <Button variant="outline" onClick={retryFailed}>
              Retry Failed ({failedCount})
            </Button>
          )}
          <Button
            onClick={previewAllParallel}
            disabled={venues.length === previewedCount || batchLoading}
          >
            {batchLoading && <LoadingSpinner size="sm" className="mr-2" />}
            {batchLoading
              ? `Previewing (${batchProgress.completed}/${batchProgress.total})...`
              : venues.length === previewedCount
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

      {scrapeError && (
        <ErrorAlert
          message={scrapeError}
          onRetry={() => {
            setScrapeError('')
            scrapeSelected()
          }}
        />
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
            selectedIds={selectedEventIds[venue.slug]}
            importStatuses={importStatuses}
            onToggle={(eventId) => onToggle(venue.slug, eventId)}
            onSelectAll={() => onSelectAll(venue.slug)}
            onSelectNone={() => onSelectNone(venue.slug)}
          />
        ))}
      </div>

      <div className="flex justify-between">
        <Button variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button
          onClick={scrapeSelected}
          disabled={scraping || totalSelected === 0}
        >
          {scraping && <LoadingSpinner size="sm" className="mr-2" />}
          {scraping ? scrapeProgress || 'Scraping...' : `Scrape ${totalSelected} Events`}
        </Button>
      </div>
    </div>
  )
}
