'use client'

import { useState } from 'react'
import {
  usePipelineVenues,
  useVenueRejectionStats,
  useUpdateExtractionNotes,
  useExtractVenue,
  type PipelineVenueInfo,
} from '@/lib/hooks/usePipeline'

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

function VenueDetailPanel({
  venue,
  onClose,
}: {
  venue: PipelineVenueInfo
  onClose: () => void
}) {
  const { data: stats, isLoading: statsLoading } = useVenueRejectionStats(venue.venue_id)
  const updateNotes = useUpdateExtractionNotes()
  const extractVenue = useExtractVenue()
  const [notes, setNotes] = useState(venue.extraction_notes ?? '')
  const [isEditing, setIsEditing] = useState(false)

  const handleSaveNotes = () => {
    updateNotes.mutate(
      { venueId: venue.venue_id, extractionNotes: notes || null },
      { onSuccess: () => setIsEditing(false) }
    )
  }

  const handleExtract = (dryRun: boolean) => {
    extractVenue.mutate({ venueId: venue.venue_id, dryRun })
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
      <div className="grid grid-cols-2 gap-2 text-sm">
        <div>
          <span className="text-muted-foreground">Render method:</span>{' '}
          {venue.render_method ?? 'auto-detect'}
        </div>
        <div>
          <span className="text-muted-foreground">Auto-approve:</span>{' '}
          {venue.auto_approve ? 'Yes' : 'No'}
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
      </div>

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

      {/* Extraction notes */}
      <div>
        <h4 className="text-sm font-medium mb-2">Extraction Notes</h4>
        <p className="text-xs text-muted-foreground mb-1">
          These notes are included in the AI prompt when extracting events for this venue.
        </p>
        {isEditing ? (
          <div className="space-y-2">
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="w-full h-24 bg-background border border-border rounded p-2 text-sm resize-none"
              placeholder='e.g., "This venue hosts karaoke every Tuesday and trivia every Wednesday — skip those"'
            />
            <div className="flex gap-2">
              <button
                onClick={handleSaveNotes}
                disabled={updateNotes.isPending}
                className="px-3 py-1 bg-primary text-primary-foreground rounded text-sm hover:bg-primary/90 disabled:opacity-50"
              >
                {updateNotes.isPending ? 'Saving...' : 'Save'}
              </button>
              <button
                onClick={() => {
                  setNotes(venue.extraction_notes ?? '')
                  setIsEditing(false)
                }}
                className="px-3 py-1 border border-border rounded text-sm hover:bg-muted"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <div
            onClick={() => setIsEditing(true)}
            className="cursor-pointer p-2 border border-border rounded text-sm hover:bg-muted min-h-[2.5rem]"
          >
            {venue.extraction_notes || (
              <span className="text-muted-foreground italic">Click to add notes...</span>
            )}
          </div>
        )}
      </div>

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

export function PipelineVenues() {
  const { data, isLoading, error } = usePipelineVenues()
  const [selectedVenueId, setSelectedVenueId] = useState<number | null>(null)

  if (isLoading) return <p className="text-muted-foreground">Loading pipeline venues...</p>
  if (error) return <p className="text-red-400">Failed to load pipeline venues</p>
  if (!data?.venues.length) return <p className="text-muted-foreground">No venues configured</p>

  const selectedVenue = data.venues.find((v) => v.venue_id === selectedVenueId)

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold">Pipeline Venues</h2>

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
            {data.venues.map((venue) => (
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
    </div>
  )
}
