'use client'

import { useState } from 'react'
import {
  usePipelineVenues,
  useVenueRejectionStats,
  useUpdateVenueConfig,
  useVenueExtractionRuns,
  useResetRenderMethod,
  useExtractVenue,
  useImportHistory,
  type PipelineVenueInfo,
  type VenueExtractionRun,
  type ImportHistoryEntry,
} from '@/lib/hooks/usePipeline'
import { useVenueSearch } from '@/features/venues'
import { Switch } from '@/components/ui/switch'

function ApprovalBadge({ rate }: { rate?: number }) {
  if (rate === undefined || rate === null) {
    return <span className="text-xs text-muted-foreground">No data</span>
  }

  const pct = Math.round(rate * 100)
  let color = 'bg-green-500/20 text-green-400'
  if (pct < 70) color = 'bg-red-500/20 text-red-400'
  else if (pct < 90) color = 'bg-yellow-500/20 text-yellow-400'

  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>
      {pct}%
    </span>
  )
}

function RunHistorySection({ venueId }: { venueId: number }) {
  const [isOpen, setIsOpen] = useState(false)
  const { data, isLoading } = useVenueExtractionRuns(venueId, { enabled: isOpen })

  return (
    <div>
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="text-sm font-medium flex items-center gap-1 hover:text-foreground text-muted-foreground"
      >
        <span className={`transition-transform ${isOpen ? 'rotate-90' : ''}`}>&#9654;</span>
        Extraction Run History
      </button>
      {isOpen && (
        <div className="mt-2">
          {isLoading ? (
            <p className="text-sm text-muted-foreground">Loading runs...</p>
          ) : !data?.runs?.length ? (
            <p className="text-sm text-muted-foreground">No extraction runs yet</p>
          ) : (
            <div className="border border-border rounded overflow-hidden">
              <table className="w-full text-xs">
                <thead className="bg-muted/50">
                  <tr>
                    <th className="text-left p-2 font-medium">Date</th>
                    <th className="text-left p-2 font-medium">Method</th>
                    <th className="text-center p-2 font-medium">Extracted</th>
                    <th className="text-center p-2 font-medium">Imported</th>
                    <th className="text-right p-2 font-medium">Duration</th>
                    <th className="text-center p-2 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {data.runs.map((run: VenueExtractionRun) => (
                    <tr key={run.id}>
                      <td className="p-2 text-muted-foreground">
                        {new Date(run.run_at || run.created_at).toLocaleString()}
                      </td>
                      <td className="p-2 text-muted-foreground">{run.render_method ?? '—'}</td>
                      <td className="p-2 text-center">{run.events_extracted}</td>
                      <td className="p-2 text-center">{run.events_imported}</td>
                      <td className="p-2 text-right text-muted-foreground">{run.duration_ms}ms</td>
                      <td className="p-2 text-center">
                        {run.error ? (
                          <span className="text-red-400" title={run.error}>
                            Error
                          </span>
                        ) : (
                          <span className="text-green-400">OK</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function ConfigEditForm({
  venue,
  onClose,
}: {
  venue: PipelineVenueInfo
  onClose: () => void
}) {
  const updateConfig = useUpdateVenueConfig()
  const [calendarUrl, setCalendarUrl] = useState(venue.calendar_url ?? '')
  const [preferredSource, setPreferredSource] = useState(venue.preferred_source || 'ai')
  const [renderMethod, setRenderMethod] = useState(venue.render_method ?? '')
  const [feedUrl, setFeedUrl] = useState(venue.feed_url ?? '')
  const [autoApprove, setAutoApprove] = useState(venue.auto_approve)
  const [strategyLocked, setStrategyLocked] = useState(venue.strategy_locked)
  const [extractionNotes, setExtractionNotes] = useState(venue.extraction_notes ?? '')

  const handleSave = () => {
    updateConfig.mutate(
      {
        venueId: venue.venue_id,
        config: {
          calendar_url: calendarUrl || null,
          preferred_source: preferredSource,
          render_method: renderMethod || null,
          feed_url: feedUrl || null,
          auto_approve: autoApprove,
          strategy_locked: strategyLocked,
          extraction_notes: extractionNotes || null,
        },
      },
      { onSuccess: () => onClose() }
    )
  }

  return (
    <div className="space-y-3 border border-border rounded p-3 bg-muted/20">
      <h4 className="text-sm font-medium">Edit Configuration</h4>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-xs text-muted-foreground block mb-1">Calendar URL</label>
          <input
            type="text"
            value={calendarUrl}
            onChange={(e) => setCalendarUrl(e.target.value)}
            className="w-full bg-background border border-border rounded px-2 py-1 text-sm"
            placeholder="https://venue.com/events"
          />
        </div>

        <div>
          <label className="text-xs text-muted-foreground block mb-1">Feed URL (iCal/RSS)</label>
          <input
            type="text"
            value={feedUrl}
            onChange={(e) => setFeedUrl(e.target.value)}
            className="w-full bg-background border border-border rounded px-2 py-1 text-sm"
            placeholder="https://venue.com/events.ics"
          />
        </div>

        <div>
          <label className="text-xs text-muted-foreground block mb-1">Preferred Source</label>
          <select
            value={preferredSource}
            onChange={(e) => setPreferredSource(e.target.value)}
            className="w-full bg-background border border-border rounded px-2 py-1 text-sm"
          >
            <option value="ai">AI</option>
            <option value="ical">iCal</option>
            <option value="rss">RSS</option>
          </select>
        </div>

        <div>
          <label className="text-xs text-muted-foreground block mb-1">Render Method</label>
          <select
            value={renderMethod}
            onChange={(e) => setRenderMethod(e.target.value)}
            className="w-full bg-background border border-border rounded px-2 py-1 text-sm"
          >
            <option value="">Auto-detect</option>
            <option value="static">Static</option>
            <option value="dynamic">Dynamic</option>
            <option value="screenshot">Screenshot</option>
          </select>
        </div>
      </div>

      <div className="flex items-center gap-6">
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={autoApprove} onCheckedChange={setAutoApprove} size="sm" />
          Auto-approve
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={strategyLocked} onCheckedChange={setStrategyLocked} size="sm" />
          Strategy locked
        </label>
      </div>

      <div>
        <label className="text-xs text-muted-foreground block mb-1">Extraction Notes</label>
        <textarea
          value={extractionNotes}
          onChange={(e) => setExtractionNotes(e.target.value)}
          className="w-full h-20 bg-background border border-border rounded p-2 text-sm resize-none"
          placeholder='e.g., "Skip karaoke Tuesdays and trivia Wednesdays"'
        />
      </div>

      <div className="flex gap-2">
        <button
          onClick={handleSave}
          disabled={updateConfig.isPending}
          className="px-3 py-1 bg-primary text-primary-foreground rounded text-sm hover:bg-primary/90 disabled:opacity-50"
        >
          {updateConfig.isPending ? 'Saving...' : 'Save'}
        </button>
        <button
          onClick={onClose}
          className="px-3 py-1 border border-border rounded text-sm hover:bg-muted"
        >
          Cancel
        </button>
      </div>
      {updateConfig.error && (
        <p className="text-sm text-red-400">
          {updateConfig.error instanceof Error ? updateConfig.error.message : 'Save failed'}
        </p>
      )}
    </div>
  )
}

function AddVenueDialog({
  existingVenueIds,
  onClose,
}: {
  existingVenueIds: Set<number>
  onClose: () => void
}) {
  const [searchQuery, setSearchQuery] = useState('')
  const { data: searchResults } = useVenueSearch({ query: searchQuery })
  const updateConfig = useUpdateVenueConfig()

  const filteredVenues = (searchResults?.venues ?? []).filter(
    (v) => !existingVenueIds.has(v.id)
  )

  const handleSelectVenue = (venueId: number) => {
    updateConfig.mutate(
      {
        venueId,
        config: {
          preferred_source: 'ai',
          auto_approve: false,
          strategy_locked: false,
        },
      },
      { onSuccess: () => onClose() }
    )
  }

  return (
    <div className="border border-border rounded-lg p-4 space-y-3 bg-card">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Configure New Venue</h3>
        <button onClick={onClose} className="text-muted-foreground hover:text-foreground text-sm">
          Close
        </button>
      </div>

      <input
        type="text"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        className="w-full bg-background border border-border rounded px-3 py-2 text-sm"
        placeholder="Search venues..."
        autoFocus
      />

      {searchQuery && filteredVenues.length > 0 && (
        <div className="max-h-48 overflow-y-auto border border-border rounded divide-y divide-border">
          {filteredVenues.map((venue) => (
            <button
              key={venue.id}
              onClick={() => handleSelectVenue(venue.id)}
              disabled={updateConfig.isPending}
              className="w-full text-left px-3 py-2 text-sm hover:bg-muted disabled:opacity-50"
            >
              <span className="font-medium">{venue.name}</span>
              <span className="text-muted-foreground ml-2 text-xs">
                {venue.city}, {venue.state}
              </span>
            </button>
          ))}
        </div>
      )}

      {searchQuery && filteredVenues.length === 0 && (
        <p className="text-sm text-muted-foreground">No matching venues found</p>
      )}

      {updateConfig.isPending && (
        <p className="text-sm text-muted-foreground">Creating config...</p>
      )}
      {updateConfig.error && (
        <p className="text-sm text-red-400">
          {updateConfig.error instanceof Error ? updateConfig.error.message : 'Failed'}
        </p>
      )}
    </div>
  )
}

function VenueDetailPanel({
  venue,
  onClose,
}: {
  venue: PipelineVenueInfo
  onClose: () => void
}) {
  const { data: stats, isLoading: statsLoading } = useVenueRejectionStats(venue.venue_id)
  const resetRenderMethod = useResetRenderMethod()
  const extractVenue = useExtractVenue()
  const [isEditingConfig, setIsEditingConfig] = useState(false)

  const handleExtract = (dryRun: boolean) => {
    extractVenue.mutate({ venueId: venue.venue_id, dryRun })
  }

  const handleResetRenderMethod = () => {
    if (window.confirm('Reset render method to auto-detect? It will be re-detected on the next pipeline run.')) {
      resetRenderMethod.mutate({ venueId: venue.venue_id })
    }
  }

  return (
    <div className="border border-border rounded-lg p-4 space-y-4 bg-card">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">{venue.venue_name}</h3>
        <button onClick={onClose} className="text-muted-foreground hover:text-foreground text-sm">
          Close
        </button>
      </div>

      {/* Config overview */}
      {isEditingConfig ? (
        <ConfigEditForm venue={venue} onClose={() => setIsEditingConfig(false)} />
      ) : (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-medium">Configuration</h4>
            <button
              onClick={() => setIsEditingConfig(true)}
              className="text-xs text-blue-400 hover:text-blue-300"
            >
              Edit
            </button>
          </div>
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div>
              <span className="text-muted-foreground">Calendar URL:</span>{' '}
              {venue.calendar_url ? (
                <a
                  href={venue.calendar_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-400 hover:underline break-all"
                >
                  {venue.calendar_url.length > 40
                    ? venue.calendar_url.slice(0, 40) + '...'
                    : venue.calendar_url}
                </a>
              ) : (
                <span className="text-muted-foreground italic">Not set</span>
              )}
            </div>
            <div>
              <span className="text-muted-foreground">Preferred source:</span>{' '}
              {venue.preferred_source || 'ai'}
            </div>
            <div>
              <span className="text-muted-foreground">Render method:</span>{' '}
              {venue.render_method ?? 'auto-detect'}
              {venue.render_method && (
                <button
                  onClick={handleResetRenderMethod}
                  disabled={resetRenderMethod.isPending}
                  className="ml-2 text-xs text-yellow-400 hover:text-yellow-300 disabled:opacity-50"
                >
                  {resetRenderMethod.isPending ? 'Resetting...' : 'Reset'}
                </button>
              )}
            </div>
            <div>
              <span className="text-muted-foreground">Auto-approve:</span>{' '}
              {venue.auto_approve ? 'Yes' : 'No'}
            </div>
            <div>
              <span className="text-muted-foreground">Strategy locked:</span>{' '}
              {venue.strategy_locked ? 'Yes' : 'No'}
            </div>
            <div>
              <span className="text-muted-foreground">Last extracted:</span>{' '}
              {venue.last_extracted_at
                ? new Date(venue.last_extracted_at).toLocaleDateString()
                : 'Never'}
            </div>
            <div>
              <span className="text-muted-foreground">Failures:</span>{' '}
              {venue.consecutive_failures}
            </div>
            <div>
              <span className="text-muted-foreground">Events expected:</span>{' '}
              {venue.events_expected}
              {venue.last_run && venue.events_expected > 0 && (
                <>
                  {' '}
                  <span className="text-muted-foreground">(last:</span>{' '}
                  <span
                    className={
                      venue.last_run.events_extracted < venue.events_expected * 0.5
                        ? 'text-yellow-400'
                        : ''
                    }
                  >
                    {venue.last_run.events_extracted}
                  </span>
                  <span className="text-muted-foreground">)</span>
                  {venue.last_run.events_extracted < venue.events_expected * 0.5 && (
                    <span className="text-yellow-400 text-xs ml-1" title="Less than 50% of expected events">
                      Low
                    </span>
                  )}
                </>
              )}
            </div>
          </div>
          {venue.extraction_notes && (
            <div className="text-sm">
              <span className="text-muted-foreground">Notes:</span>{' '}
              <span className="italic">{venue.extraction_notes}</span>
            </div>
          )}
        </div>
      )}

      {/* Rejection stats */}
      <div>
        <h4 className="text-sm font-medium mb-2">Rejection Breakdown</h4>
        {statsLoading ? (
          <p className="text-sm text-muted-foreground">Loading stats...</p>
        ) : stats ? (
          <div className="space-y-2">
            <div className="grid grid-cols-4 gap-2 text-sm">
              <div className="text-center p-2 bg-muted rounded">
                <div className="text-lg font-semibold">{stats.total_extracted}</div>
                <div className="text-muted-foreground text-xs">Total</div>
              </div>
              <div className="text-center p-2 bg-green-500/10 rounded">
                <div className="text-lg font-semibold text-green-400">{stats.approved}</div>
                <div className="text-muted-foreground text-xs">Approved</div>
              </div>
              <div className="text-center p-2 bg-red-500/10 rounded">
                <div className="text-lg font-semibold text-red-400">{stats.rejected}</div>
                <div className="text-muted-foreground text-xs">Rejected</div>
              </div>
              <div className="text-center p-2 bg-yellow-500/10 rounded">
                <div className="text-lg font-semibold text-yellow-400">{stats.pending}</div>
                <div className="text-muted-foreground text-xs">Pending</div>
              </div>
            </div>

            {Object.keys(stats.rejection_breakdown).length > 0 && (
              <div className="text-sm space-y-1">
                <p className="text-muted-foreground text-xs">Rejection categories:</p>
                {Object.entries(stats.rejection_breakdown)
                  .sort(([, a], [, b]) => b - a)
                  .map(([category, count]) => (
                    <div key={category} className="flex justify-between">
                      <span className="capitalize">{category.replace('_', ' ')}</span>
                      <span className="text-muted-foreground">{count}</span>
                    </div>
                  ))}
              </div>
            )}

            {stats.suggested_auto_approve && !venue.auto_approve && (
              <div className="text-sm bg-green-500/10 border border-green-500/20 rounded p-2 text-green-400">
                High approval rate ({Math.round(stats.approval_rate * 100)}%) — consider enabling
                auto-approve for this venue.
              </div>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No extraction data yet</p>
        )}
      </div>

      {/* Extraction run history */}
      <RunHistorySection venueId={venue.venue_id} />

      {/* Extract actions */}
      <div className="flex gap-2 pt-2 border-t border-border">
        <button
          onClick={() => handleExtract(true)}
          disabled={extractVenue.isPending}
          className="px-3 py-1 border border-border rounded text-sm hover:bg-muted disabled:opacity-50"
        >
          {extractVenue.isPending ? 'Extracting...' : 'Dry Run'}
        </button>
        <button
          onClick={() => handleExtract(false)}
          disabled={extractVenue.isPending}
          className="px-3 py-1 bg-primary text-primary-foreground rounded text-sm hover:bg-primary/90 disabled:opacity-50"
        >
          Extract & Import
        </button>
      </div>

      {extractVenue.data && (
        <div className="text-sm bg-muted rounded p-2">
          <p>
            Extracted: {extractVenue.data.events_extracted} | Imported:{' '}
            {extractVenue.data.events_imported} | Filtered:{' '}
            {extractVenue.data.events_skipped_non_music} | {extractVenue.data.duration_ms}ms
          </p>
          {extractVenue.data.warnings?.map((w, i) => (
            <p key={i} className="text-yellow-400 text-xs">
              {w}
            </p>
          ))}
        </div>
      )}

      {extractVenue.error && (
        <p className="text-sm text-red-400">
          {extractVenue.error instanceof Error
            ? extractVenue.error.message
            : 'Extraction failed'}
        </p>
      )}
    </div>
  )
}

function SourceTypeBadge({ sourceType }: { sourceType: string }) {
  const colors: Record<string, string> = {
    ai: 'bg-purple-500/20 text-purple-400',
    ical: 'bg-blue-500/20 text-blue-400',
    rss: 'bg-orange-500/20 text-orange-400',
  }
  const color = colors[sourceType] ?? 'bg-muted text-muted-foreground'
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>
      {sourceType.toUpperCase()}
    </span>
  )
}

function ImportHistorySection() {
  const PAGE_SIZE = 20
  const [offset, setOffset] = useState(0)
  const { data, isLoading, error } = useImportHistory(PAGE_SIZE, offset)

  if (isLoading) return <p className="text-muted-foreground text-sm">Loading import history...</p>
  if (error) return <p className="text-red-400 text-sm">Failed to load import history</p>

  const imports = data?.imports ?? []
  const total = data?.total ?? 0
  const hasMore = offset + PAGE_SIZE < total
  const hasPrev = offset > 0

  if (imports.length === 0 && offset === 0) {
    return <p className="text-muted-foreground text-sm">No extraction runs recorded yet.</p>
  }

  return (
    <div className="space-y-3">
      <div className="border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left p-3 font-medium">Date</th>
              <th className="text-left p-3 font-medium">Venue</th>
              <th className="text-center p-3 font-medium">Source</th>
              <th className="text-center p-3 font-medium">Extracted</th>
              <th className="text-center p-3 font-medium">Imported</th>
              <th className="text-right p-3 font-medium">Duration</th>
              <th className="text-center p-3 font-medium">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {imports.map((entry: ImportHistoryEntry) => (
              <tr key={entry.id}>
                <td className="p-3 text-muted-foreground text-xs">
                  {new Date(entry.run_at).toLocaleString()}
                </td>
                <td className="p-3">
                  <span className="font-medium">{entry.venue_name}</span>
                </td>
                <td className="p-3 text-center">
                  <SourceTypeBadge sourceType={entry.source_type} />
                </td>
                <td className="p-3 text-center">{entry.events_extracted}</td>
                <td className="p-3 text-center">{entry.events_imported}</td>
                <td className="p-3 text-right text-muted-foreground text-xs">
                  {entry.duration_ms >= 1000
                    ? `${(entry.duration_ms / 1000).toFixed(1)}s`
                    : `${entry.duration_ms}ms`}
                </td>
                <td className="p-3 text-center">
                  {entry.error ? (
                    <span className="text-red-400 text-xs" title={entry.error}>
                      Error
                    </span>
                  ) : (
                    <span className="text-green-400 text-xs">OK</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {(hasPrev || hasMore) && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">
            Showing {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
              disabled={!hasPrev}
              className="px-3 py-1 border border-border rounded text-sm hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Previous
            </button>
            <button
              onClick={() => setOffset(offset + PAGE_SIZE)}
              disabled={!hasMore}
              className="px-3 py-1 border border-border rounded text-sm hover:bg-muted disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export function PipelineVenues() {
  const { data, isLoading, error } = usePipelineVenues()
  const [selectedVenueId, setSelectedVenueId] = useState<number | null>(null)
  const [showAddVenue, setShowAddVenue] = useState(false)
  const [activeView, setActiveView] = useState<'venues' | 'history'>('venues')

  if (isLoading) return <p className="text-muted-foreground">Loading pipeline venues...</p>
  if (error) return <p className="text-red-400">Failed to load pipeline venues</p>

  const venues = data?.venues ?? []
  const selectedVenue = venues.find((v) => v.venue_id === selectedVenueId)
  const existingVenueIds = new Set(venues.map((v) => v.venue_id))

  return (
    <div className="space-y-4">
      {/* View toggle */}
      <div className="flex items-center gap-2 border-b border-border pb-3">
        <button
          onClick={() => setActiveView('venues')}
          className={`px-3 py-1.5 rounded text-sm font-medium transition-colors ${
            activeView === 'venues'
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted'
          }`}
        >
          Venue Status
        </button>
        <button
          onClick={() => setActiveView('history')}
          className={`px-3 py-1.5 rounded text-sm font-medium transition-colors ${
            activeView === 'history'
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted'
          }`}
        >
          Import History
        </button>
      </div>

      {activeView === 'history' ? (
        <ImportHistorySection />
      ) : (
      <>
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Pipeline Venues</h2>
        <button
          onClick={() => {
            setShowAddVenue(!showAddVenue)
            setSelectedVenueId(null)
          }}
          className="px-3 py-1 bg-primary text-primary-foreground rounded text-sm hover:bg-primary/90"
        >
          {showAddVenue ? 'Cancel' : 'Configure Venue'}
        </button>
      </div>

      {showAddVenue && (
        <AddVenueDialog
          existingVenueIds={existingVenueIds}
          onClose={() => setShowAddVenue(false)}
        />
      )}

      {venues.length === 0 ? (
        <p className="text-muted-foreground">No venues configured</p>
      ) : (
        <>
          {/* Venue table */}
          <div className="border border-border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-muted/50">
                <tr>
                  <th className="text-left p-3 font-medium">Venue</th>
                  <th className="text-left p-3 font-medium">Method</th>
                  <th className="text-center p-3 font-medium">Approval</th>
                  <th className="text-center p-3 font-medium">Auto</th>
                  <th className="text-left p-3 font-medium">Last Run</th>
                  <th className="text-center p-3 font-medium">Notes</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {venues.map((venue) => (
                  <tr
                    key={venue.venue_id}
                    onClick={() =>
                      setSelectedVenueId(
                        selectedVenueId === venue.venue_id ? null : venue.venue_id
                      )
                    }
                    className={`cursor-pointer hover:bg-muted/30 ${
                      selectedVenueId === venue.venue_id ? 'bg-muted/50' : ''
                    }`}
                  >
                    <td className="p-3">
                      <div className="font-medium">{venue.venue_name}</div>
                      {venue.consecutive_failures > 0 && (
                        <span className="text-xs text-red-400">
                          {venue.consecutive_failures} failure(s)
                        </span>
                      )}
                    </td>
                    <td className="p-3 text-muted-foreground">
                      {venue.render_method ?? '—'}
                    </td>
                    <td className="p-3 text-center">
                      <ApprovalBadge rate={venue.approval_rate} />
                    </td>
                    <td className="p-3 text-center">
                      {venue.auto_approve ? (
                        <span className="text-green-400 text-xs">On</span>
                      ) : (
                        <span className="text-muted-foreground text-xs">Off</span>
                      )}
                    </td>
                    <td className="p-3 text-muted-foreground text-xs">
                      {venue.last_run
                        ? `${venue.last_run.events_extracted} events, ${new Date(venue.last_run.created_at).toLocaleDateString()}`
                        : 'Never'}
                    </td>
                    <td className="p-3 text-center">
                      {venue.extraction_notes ? (
                        <span className="text-xs text-blue-400" title={venue.extraction_notes}>
                          Has notes
                        </span>
                      ) : (
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Detail panel */}
          {selectedVenue && (
            <VenueDetailPanel
              venue={selectedVenue}
              onClose={() => setSelectedVenueId(null)}
            />
          )}
        </>
      )}
      </>
      )}
    </div>
  )
}
