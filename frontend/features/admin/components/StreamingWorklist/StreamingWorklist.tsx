'use client'

/**
 * /admin/streaming-worklist — admin triage UI for the streaming-discovery
 * worklist. Surfaces artists whose `streaming_discovery_status` is
 * non-terminal AND who have at least one upcoming show, ordered by
 * soonest show date.
 *
 * Page shape mirrors `/admin/featured` (PSY-838) — same inline-banner
 * mutation feedback (`pattern_mutation_feedback.md`), same TanStack
 * Query layout, same admin-only gating via the parent admin layout.
 *
 * ENGINE SEAM (locked decision: worklist writes, engine stays stateless)
 * ─────────────────────────────────────────────────────────────────────
 * PSY-816 shipped the candidate-review surface INLINE on the artist
 * detail page (`frontend/features/artists/components/ArtistDetail.tsx`,
 * `useDiscoverMusic` / `useUpdateArtistBandcamp` / `useUpdateArtistSpotify`).
 * There is no separate drawer/modal to embed inside the worklist.
 *
 * The "Review →" button on each row opens the artist detail page in a
 * new tab. The admin reviews candidates and saves bandcamp/spotify URLs
 * there. The status flip to `linked` is concentrated HERE — the admin
 * clicks "Mark linked" on the worklist row once they've confirmed the
 * URLs got saved. This avoids cross-feature coupling and keeps the
 * discovery engine itself stateless (it returns candidates, doesn't
 * track triage state).
 *
 * The worklist refetches on window focus so the soonest-show row data
 * stays accurate when the admin hops back from a long candidate-review
 * session.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import {
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Filter,
  Inbox,
  Link2,
  Link2Off,
  Loader2,
  Radio,
  SkipForward,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { AdminEmptyState } from '@/components/admin'
import { InlineErrorBanner, StatusBanner } from '@/components/shared'
import { formatAdminDate } from '@/lib/utils/formatters'
import {
  useStreamingWorklist,
  useUpdateStreamingDiscoveryStatus,
} from './useStreamingWorklist'
import {
  STREAMING_WORKLIST_DEFAULT_LIMIT,
  STREAMING_WORKLIST_STATUS_FILTER_OPTIONS,
  type StreamingWorklistAction,
  type StreamingWorklistEntry,
  type StreamingWorklistStatusFilter,
} from './types'

// ──────────────────────────────────────────────
// Status badge — small visual marker per row
// ──────────────────────────────────────────────

function StatusBadge({ status }: { status: StreamingWorklistEntry['streaming_discovery_status'] }) {
  // Only non-terminal values appear in the list response, but the type
  // allows the full enum so the badge stays correct if a backend tweak
  // ever widens what's surfaced.
  //
  // Bound to the DS categorical palette (`--chart-*`, PSY-943) instead of the
  // prior dark-only amber/blue/green/zinc hues. Hue follows the triage flow:
  // unreviewed = chart-3 (gold, needs attention), candidates_pending =
  // chart-6 (denim, active), linked = chart-2 (green, done), the two terminal
  // dead-ends (no_links_found / skipped) = muted.
  const palette: Record<typeof status, string> = {
    unreviewed: 'border-chart-3/30 bg-chart-3/15 text-chart-3',
    candidates_pending: 'border-chart-6/30 bg-chart-6/15 text-chart-6',
    linked: 'border-chart-2/30 bg-chart-2/15 text-chart-2',
    no_links_found: 'border-border bg-muted text-muted-foreground',
    skipped: 'border-border bg-muted text-muted-foreground',
  }
  const labels: Record<typeof status, string> = {
    unreviewed: 'Unreviewed',
    candidates_pending: 'Candidates pending',
    linked: 'Linked',
    no_links_found: 'No links',
    skipped: 'Skipped',
  }
  return (
    <span
      className={`inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${palette[status]}`}
      data-testid={`streaming-worklist-status-${status}`}
    >
      {labels[status]}
    </span>
  )
}

// ──────────────────────────────────────────────
// Inline reason form — collapses below the row when one of the
// "no_links_found" / "skipped" / "linked" buttons is clicked.
// Mark-linked also routes through this form so the admin can drop a
// short note about which candidate they picked.
// ──────────────────────────────────────────────

const ACTION_COPY: Record<
  StreamingWorklistAction,
  { title: string; placeholder: string; submit: string; successPrefix: string }
> = {
  linked: {
    title: 'Mark linked',
    placeholder:
      'Optional: which candidate did you pick? (e.g. "Asheville NC band, Bleeds 2025")',
    submit: 'Mark linked',
    successPrefix: 'Marked linked',
  },
  no_links_found: {
    title: 'Mark no links found',
    placeholder:
      'Optional: brief note for the next reviewer (e.g. "Only ghost profiles, no real candidates")',
    submit: 'Mark no links',
    successPrefix: 'Marked no-links',
  },
  skipped: {
    title: 'Mark skipped',
    placeholder: 'Optional: why skipped (e.g. "Same-name collision, ambiguous")',
    submit: 'Mark skipped',
    successPrefix: 'Marked skipped',
  },
}

interface ActionFormProps {
  entry: StreamingWorklistEntry
  action: StreamingWorklistAction
  onClose: () => void
  onSuccess: (entry: StreamingWorklistEntry, action: StreamingWorklistAction) => void
}

function ActionForm({ entry, action, onClose, onSuccess }: ActionFormProps) {
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  const mutation = useUpdateStreamingDiscoveryStatus()
  const copy = ACTION_COPY[action]

  const handleSubmit = useCallback(() => {
    setError(null)
    mutation.mutate(
      {
        artist_id: entry.artist_id,
        status: action,
        reason: reason.trim() ? reason.trim() : null,
      },
      {
        onSuccess: () => {
          onSuccess(entry, action)
        },
        onError: (err) => {
          setError(
            err instanceof Error
              ? err.message
              : `Failed to mark ${action.replace('_', ' ')}.`
          )
        },
      }
    )
  }, [mutation, entry, action, reason, onSuccess])

  return (
    <div
      className="rounded-md border border-border/60 bg-muted/20 p-3 space-y-2"
      data-testid={`streaming-worklist-action-form-${entry.artist_id}-${action}`}
    >
      <Label
        htmlFor={`reason-${entry.artist_id}-${action}`}
        className="text-xs font-semibold uppercase tracking-wide text-muted-foreground"
      >
        {copy.title} — {entry.artist_name}
      </Label>
      <textarea
        id={`reason-${entry.artist_id}-${action}`}
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        rows={2}
        maxLength={2000}
        placeholder={copy.placeholder}
        className="w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        disabled={mutation.isPending}
      />
      {error && (
        <InlineErrorBanner
          testId={`streaming-worklist-action-error-${entry.artist_id}-${action}`}
        >
          {error}
        </InlineErrorBanner>
      )}
      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onClose}
          disabled={mutation.isPending}
        >
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={handleSubmit}
          disabled={mutation.isPending}
          data-testid={`streaming-worklist-submit-${entry.artist_id}-${action}`}
        >
          {mutation.isPending ? (
            <>
              <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
              Saving...
            </>
          ) : (
            copy.submit
          )}
        </Button>
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────
// Row — artist | upcoming show | status | actions
// ──────────────────────────────────────────────

interface WorklistRowProps {
  entry: StreamingWorklistEntry
  onActionSuccess: (entry: StreamingWorklistEntry, action: StreamingWorklistAction) => void
}

function WorklistRow({ entry, onActionSuccess }: WorklistRowProps) {
  const [openAction, setOpenAction] = useState<StreamingWorklistAction | null>(null)

  // Artist detail page is the home of the PSY-816 candidate-review
  // panel. New tab so the worklist stays open for fast triage.
  const artistHref = entry.artist_slug
    ? `/artists/${entry.artist_slug}`
    : `/artists/${entry.artist_id}`

  const eventDate = formatAdminDate(entry.soonest_event_date)
  const venueLabel = [entry.venue_name, entry.venue_city]
    .filter(Boolean)
    .join(' · ')

  return (
    <div
      className="rounded-lg border border-border/60 bg-card/40 p-3 space-y-3"
      data-testid={`streaming-worklist-row-${entry.artist_id}`}
    >
      <div className="grid grid-cols-1 md:grid-cols-12 gap-3 items-center">
        {/* Artist (cols 1–4) */}
        <div className="md:col-span-4 min-w-0">
          <Link
            href={artistHref}
            target="_blank"
            rel="noreferrer"
            className="text-sm font-semibold hover:underline truncate inline-flex items-center gap-1"
            data-testid={`streaming-worklist-artist-link-${entry.artist_id}`}
          >
            <Radio className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            <span className="truncate">{entry.artist_name}</span>
          </Link>
          {entry.upcoming_show_count > 1 && (
            <p className="text-[11px] text-muted-foreground mt-0.5">
              {entry.upcoming_show_count} upcoming shows
            </p>
          )}
        </div>

        {/* Soonest show (cols 5–8) */}
        <div className="md:col-span-4 min-w-0">
          <p className="text-sm truncate">{eventDate}</p>
          {venueLabel && (
            <p className="text-xs text-muted-foreground truncate">
              {venueLabel}
            </p>
          )}
        </div>

        {/* Status badge (cols 9–10) */}
        <div className="md:col-span-2">
          <StatusBadge status={entry.streaming_discovery_status} />
        </div>

        {/* Actions (cols 11–12) */}
        <div className="md:col-span-2 flex flex-wrap items-center justify-end gap-1.5">
          <Button
            type="button"
            variant="outline"
            size="sm"
            asChild
            className="gap-1.5"
            data-testid={`streaming-worklist-review-${entry.artist_id}`}
          >
            <Link href={artistHref} target="_blank" rel="noreferrer">
              <ExternalLink className="h-3.5 w-3.5" />
              Review
            </Link>
          </Button>
        </div>
      </div>

      {/* Inline action buttons — appear below the row so the form has
       room to expand without breaking the table grid. */}
      {!openAction ? (
        <div className="flex flex-wrap items-center gap-1.5 border-t border-border/40 pt-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setOpenAction('linked')}
            className="gap-1.5 text-green-300 hover:text-green-200"
            data-testid={`streaming-worklist-open-linked-${entry.artist_id}`}
          >
            <Link2 className="h-3.5 w-3.5" />
            Mark linked
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setOpenAction('no_links_found')}
            className="gap-1.5 text-muted-foreground hover:text-foreground"
            data-testid={`streaming-worklist-open-no_links_found-${entry.artist_id}`}
          >
            <Link2Off className="h-3.5 w-3.5" />
            Mark no links found
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setOpenAction('skipped')}
            className="gap-1.5 text-muted-foreground hover:text-foreground"
            data-testid={`streaming-worklist-open-skipped-${entry.artist_id}`}
          >
            <SkipForward className="h-3.5 w-3.5" />
            Mark skipped
          </Button>
        </div>
      ) : (
        <ActionForm
          entry={entry}
          action={openAction}
          onClose={() => setOpenAction(null)}
          onSuccess={(e, a) => {
            setOpenAction(null)
            onActionSuccess(e, a)
          }}
        />
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Top-level page component
// ──────────────────────────────────────────────

export function StreamingWorklist() {
  const [status, setStatus] = useState<StreamingWorklistStatusFilter>('')
  const [offset, setOffset] = useState(0)
  const limit = STREAMING_WORKLIST_DEFAULT_LIMIT
  const [recentMutation, setRecentMutation] = useState<{
    artistName: string
    action: StreamingWorklistAction
  } | null>(null)

  const { data, isLoading, isError, error } = useStreamingWorklist({
    status,
    limit,
    offset,
  })

  const entries = data?.entries ?? []
  const total = data?.total ?? 0

  // Page index is derived — keeps the parent state minimal and avoids a
  // useEffect for prop-derived state.
  const pageIndex = Math.floor(offset / limit)
  const totalPages = Math.max(1, Math.ceil(total / limit))

  // Rewind to the last non-empty page when a mutation drops the only
  // visible row on a non-first page. Without this, total goes (e.g.) 26
  // → 25 but offset stays at 25 — the user sees the empty-state copy
  // even though the queue isn't actually clear. We use useEffect rather
  // than calculate-during-render because (a) offset is parent state we
  // need to mutate via setOffset, and (b) the condition fires only when
  // the query result lands, not on every render.
  useEffect(() => {
    if (data && entries.length === 0 && offset > 0 && total > 0) {
      const lastValidOffset = Math.max(0, (Math.ceil(total / limit) - 1) * limit)
      setOffset(lastValidOffset)
    }
  }, [data, entries.length, offset, total, limit])

  const handleStatusChange = useCallback((value: StreamingWorklistStatusFilter) => {
    setStatus(value)
    setOffset(0)
  }, [])

  const handlePrev = useCallback(() => {
    setOffset((prev) => Math.max(0, prev - limit))
  }, [limit])

  const handleNext = useCallback(() => {
    setOffset((prev) => prev + limit)
  }, [limit])

  const handleActionSuccess = useCallback(
    (entry: StreamingWorklistEntry, action: StreamingWorklistAction) => {
      setRecentMutation({ artistName: entry.artist_name, action })
    },
    []
  )

  const errorMessage = useMemo(() => {
    if (!isError) return null
    return error instanceof Error
      ? error.message
      : 'Could not load the streaming-discovery worklist.'
  }, [isError, error])

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-6xl space-y-6">
        <header>
          <div className="flex items-center gap-2 mb-1">
            <Radio className="h-5 w-5 text-primary" />
            <h1 className="text-2xl font-bold tracking-tight">
              Streaming-discovery worklist
            </h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Artists with an upcoming show whose Bandcamp/Spotify links
            still need a human pick. Review a row → opens the artist
            page where the candidate panel lives. Once you save the
            picks (or decide there aren&apos;t any), come back here and
            mark the row to drop it from the queue.
          </p>
        </header>

        {/* Recent-mutation success banner — auto-dismiss after 4s.
            Inline + ephemeral so it doesn't clutter the table. */}
        {recentMutation && (
          <StatusBanner
            variant="success"
            dismissAfterMs={4000}
            onDismiss={() => setRecentMutation(null)}
            testId="streaming-worklist-success-banner"
          >
            <p className="text-sm">
              {ACTION_COPY[recentMutation.action].successPrefix}:{' '}
              <span className="font-semibold">{recentMutation.artistName}</span>
            </p>
          </StatusBanner>
        )}

        {/* Filter bar */}
        <div className="flex flex-wrap items-center gap-3 rounded-lg border border-border/60 bg-card/40 p-3">
          <Filter className="h-4 w-4 text-muted-foreground" />
          <Label
            htmlFor="streaming-worklist-status-filter"
            className="text-xs uppercase tracking-wide text-muted-foreground"
          >
            Status
          </Label>
          <select
            id="streaming-worklist-status-filter"
            value={status}
            onChange={(e) =>
              handleStatusChange(e.target.value as StreamingWorklistStatusFilter)
            }
            className="h-8 rounded-md border border-input bg-transparent px-2 py-1 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            data-testid="streaming-worklist-status-filter"
          >
            {STREAMING_WORKLIST_STATUS_FILTER_OPTIONS.map((opt) => (
              <option key={opt.value || 'all'} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <div className="ml-auto text-xs text-muted-foreground">
            {total > 0 ? (
              <>
                {total} total · showing {offset + 1}–
                {Math.min(offset + entries.length, total)}
              </>
            ) : null}
          </div>
        </div>

        {/* List state */}
        {isLoading && (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        )}

        {errorMessage && (
          <InlineErrorBanner
            variant="queryFallback"
            testId="streaming-worklist-load-error"
          >
            {errorMessage}
          </InlineErrorBanner>
        )}

        {!isLoading && !errorMessage && entries.length === 0 && (
          <AdminEmptyState
            icon={Inbox}
            title="Worklist clear"
            message={
              status
                ? `No artists in "${status}" with an upcoming show.`
                : 'No artists with non-terminal status have upcoming shows right now.'
            }
            testId="streaming-worklist-empty"
          />
        )}

        {!isLoading && !errorMessage && entries.length > 0 && (
          <div className="space-y-2" data-testid="streaming-worklist-rows">
            {entries.map((entry) => (
              <WorklistRow
                key={entry.artist_id}
                entry={entry}
                onActionSuccess={handleActionSuccess}
              />
            ))}
          </div>
        )}

        {/* Pagination */}
        {!errorMessage && total > limit && (
          <div className="flex items-center justify-between gap-2 pt-2">
            <div className="text-xs text-muted-foreground">
              Page {pageIndex + 1} of {totalPages}
            </div>
            <div className="flex items-center gap-1.5">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handlePrev}
                disabled={offset === 0 || isLoading}
                data-testid="streaming-worklist-prev"
                className="gap-1.5"
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                Prev
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleNext}
                disabled={offset + limit >= total || isLoading}
                data-testid="streaming-worklist-next"
                className="gap-1.5"
              >
                Next
                <ChevronRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default StreamingWorklist
