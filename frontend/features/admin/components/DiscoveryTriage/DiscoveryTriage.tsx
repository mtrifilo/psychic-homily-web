'use client'

/**
 * /admin/discovery — admin triage queue for the bulk-backfill music-link
 * suggestions (PSY-1207). Lists PENDING `artist_link_suggestions` rows
 * (pre-computed by the PSY-1206 sweep across the ~1859 link-less artists),
 * high-confidence first, paginated. Each row is human-reviewed: Accept
 * writes the link to the artist; Reject discards the candidate. Auto-apply
 * is ruled out (spikes PSY-1196/1197) — there is no bulk-accept.
 *
 * Page shape mirrors `/admin/streaming-worklist` (PSY-840) — same TanStack
 * Query layout, same inline success/error banners
 * (`pattern_mutation_feedback.md`), same admin-only gating via the parent
 * admin layout. The row's candidate-card rendering (confidence badge +
 * `review`-tier caveat, live/region indicators) mirrors the per-artist
 * Discover Music card on the artist detail page (PSY-1198,
 * `ArtistDetail.tsx` → `DiscoveryCandidateCard`).
 *
 * ASYNC-BANDCAMP AFFORDANCE (honest copy, locked)
 * ─────────────────────────────────────────────────────────────────────
 * Accepting a Spotify suggestion writes the link and the embed renders on
 * the artist page immediately. Accepting a Bandcamp suggestion writes the
 * profile URL but the embed is resolved SERVER-SIDE ASYNC (the PSY-1190
 * profile→embed resolver runs after the accept returns). The success banner
 * must NOT claim the Bandcamp embed is instantly live — it fills shortly,
 * after a refresh of the artist page.
 */

import { useCallback, useMemo, useState } from 'react'
import Link from 'next/link'
import {
  AlertCircle,
  Check,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Inbox,
  Loader2,
  MapPin,
  Music,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { AdminEmptyState } from '@/components/admin'
import { InlineErrorBanner, StatusBanner } from '@/components/shared'
import type { ApiError } from '@/lib/api'
import {
  useLinkSuggestions,
  useReviewLinkSuggestion,
  type LinkSuggestionVerdict,
} from './useDiscoveryTriage'
import {
  LINK_SUGGESTIONS_DEFAULT_LIMIT,
  type LinkSuggestionEntry,
} from './types'

// ──────────────────────────────────────────────
// Platform label — small visual marker per row
// ──────────────────────────────────────────────

const PLATFORM_LABEL: Record<LinkSuggestionEntry['platform'], string> = {
  bandcamp: 'Bandcamp',
  spotify: 'Spotify',
}

// ──────────────────────────────────────────────
// Per-verdict success copy. Accept copy is platform-aware so the Bandcamp
// async-resolver affordance stays honest (the embed fills after the
// server-side resolver runs — don't claim it's instantly live).
// ──────────────────────────────────────────────

function successMessage(
  entry: LinkSuggestionEntry,
  verdict: LinkSuggestionVerdict
): string {
  if (verdict === 'reject') {
    return `Rejected ${PLATFORM_LABEL[entry.platform]} suggestion for ${entry.artist_name}.`
  }
  if (entry.platform === 'spotify') {
    return `Linked Spotify for ${entry.artist_name} — the embed renders on the artist page now.`
  }
  // Bandcamp: the profile URL is saved, but the embed is resolved
  // server-side async. Be explicit that it fills shortly, not instantly.
  return `Linked Bandcamp for ${entry.artist_name} — the embed fills in shortly (the resolver runs in the background); refresh the artist page to see it.`
}

/**
 * Map an accept/reject failure to admin-facing copy. apiRequest throws an
 * ApiError carrying `.status`, so the three meaningful codes are surfaced
 * distinctly rather than collapsed into a generic "failed" — a conflicting
 * verdict (409) and an invalid URL (422) are NOT silently dropped.
 */
function reviewErrorMessage(err: unknown): string {
  const status = (err as ApiError | undefined)?.status
  if (status === 409) {
    return 'Already reviewed with a different verdict — refresh the queue to see the current state.'
  }
  if (status === 422) {
    return 'The candidate URL failed validation (not a valid Spotify artist / Bandcamp profile URL). The link was not written.'
  }
  if (status === 404) {
    return 'This suggestion no longer exists — it may have been reviewed already. Refresh the queue.'
  }
  if (err instanceof Error && err.message) {
    return err.message
  }
  return 'Review failed — try again.'
}

// ──────────────────────────────────────────────
// Row — one candidate suggestion. Mirrors the per-artist Discover Music
// card (PSY-1198): artist + platform header, confidence badge, candidate
// URL, live/region indicators, the `review`-tier caveat, notes, and the
// Accept / Reject buttons.
// ──────────────────────────────────────────────

interface SuggestionRowProps {
  entry: LinkSuggestionEntry
  onReviewSuccess: (entry: LinkSuggestionEntry, verdict: LinkSuggestionVerdict) => void
}

function SuggestionRow({ entry, onReviewSuccess }: SuggestionRowProps) {
  const review = useReviewLinkSuggestion()
  const [error, setError] = useState<string | null>(null)
  // Track which verdict is in flight so only the clicked button spins and
  // the pair disables together (no double-submit, no ambiguous spinner).
  const [pendingVerdict, setPendingVerdict] = useState<LinkSuggestionVerdict | null>(
    null
  )

  // `review` tier = a possible touring act or namesake. NEVER auto-accepted
  // or hidden — the row is fully rendered and the admin still picks; the
  // badge + caveat just flag the lower certainty (PSY-1191 semantics).
  const isHigh = entry.confidence === 'high'

  const artistHref = entry.artist_slug
    ? `/artists/${entry.artist_slug}`
    : `/artists/${entry.artist_id}`

  const handleReview = useCallback(
    (verdict: LinkSuggestionVerdict) => {
      setError(null)
      setPendingVerdict(verdict)
      review.mutate(
        { suggestionId: entry.id, verdict },
        {
          onSuccess: () => {
            onReviewSuccess(entry, verdict)
          },
          onError: (err) => {
            setPendingVerdict(null)
            setError(reviewErrorMessage(err))
          },
        }
      )
    },
    [review, entry, onReviewSuccess]
  )

  const disabled = review.isPending

  return (
    <div
      className="rounded-lg border border-border/60 bg-card/40 p-3 space-y-3"
      data-testid={`link-suggestion-row-${entry.id}`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          {/* Artist (linked) + platform + confidence tier */}
          <div className="flex items-center gap-2 flex-wrap">
            <Link
              href={artistHref}
              target="_blank"
              rel="noreferrer"
              className="text-sm font-semibold hover:underline inline-flex items-center gap-1"
              data-testid={`link-suggestion-artist-link-${entry.id}`}
            >
              <Music className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <span className="truncate">{entry.artist_name}</span>
            </Link>
            <span className="text-xs text-muted-foreground">
              {PLATFORM_LABEL[entry.platform]}
            </span>
            {isHigh ? (
              <Badge variant="accent">High confidence</Badge>
            ) : (
              <Badge
                variant="outline"
                className="border-pending-foreground/40 text-pending-foreground"
                data-testid={`link-suggestion-verify-badge-${entry.id}`}
              >
                Verify
              </Badge>
            )}
          </div>

          {/* MusicBrainz attribution (the candidate's provenance) */}
          {entry.mb_artist_name && (
            <p className="text-xs text-muted-foreground">
              MusicBrainz: {entry.mb_artist_name}
            </p>
          )}

          {/* Candidate URL — React-escaped; href is the same string. Opens
              in a new tab so the admin can eyeball the profile before
              accepting. */}
          <a
            href={entry.url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-muted-foreground hover:underline break-all block"
            data-testid={`link-suggestion-url-${entry.id}`}
          >
            {entry.url}
          </a>

          {/* Liveness + region-match indicators */}
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span
              className={`inline-flex items-center gap-1 ${entry.live ? 'text-success-foreground' : 'text-muted-foreground'}`}
            >
              {entry.live ? (
                <Check className="h-3 w-3" />
              ) : (
                <AlertCircle className="h-3 w-3" />
              )}
              {entry.live ? 'Reachable' : 'No response'}
            </span>
            <span className="inline-flex items-center gap-1">
              <MapPin className="h-3 w-3" />
              {entry.region_match ? 'Region match' : 'Region mismatch'}
            </span>
          </div>

          {/* `review`-tier caveat — only on the lower-certainty tier. */}
          {!isHigh && (
            <p
              className="text-xs italic text-pending-foreground"
              data-testid={`link-suggestion-caveat-${entry.id}`}
            >
              Verify — possible touring act or namesake.
            </p>
          )}

          {/* Optional reviewer note from the sweep — React-escaped. */}
          {entry.notes && (
            <p className="text-xs italic text-muted-foreground">{entry.notes}</p>
          )}
        </div>

        {/* Accept / Reject + review-on-artist-page link */}
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <div className="flex items-center gap-1.5">
            <Button
              type="button"
              size="sm"
              onClick={() => handleReview('accept')}
              disabled={disabled}
              className="gap-1.5"
              data-testid={`link-suggestion-accept-${entry.id}`}
            >
              {pendingVerdict === 'accept' ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Check className="h-3.5 w-3.5" />
              )}
              Accept
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => handleReview('reject')}
              disabled={disabled}
              className="gap-1.5"
              data-testid={`link-suggestion-reject-${entry.id}`}
            >
              {pendingVerdict === 'reject' ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <X className="h-3.5 w-3.5" />
              )}
              Reject
            </Button>
          </div>
          <Link
            href={artistHref}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:underline"
            data-testid={`link-suggestion-review-${entry.id}`}
          >
            <ExternalLink className="h-3 w-3" />
            Artist page
          </Link>
        </div>
      </div>

      {/* Inline error — a 409 (conflicting verdict) / 422 (invalid URL) /
          404 is surfaced HERE, not silently swallowed. The row stays put so
          the admin can refresh the queue or move on. */}
      {error && (
        <InlineErrorBanner testId={`link-suggestion-error-${entry.id}`}>
          {error}
        </InlineErrorBanner>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Top-level page component
// ──────────────────────────────────────────────

export function DiscoveryTriage() {
  const [offset, setOffset] = useState(0)
  const limit = LINK_SUGGESTIONS_DEFAULT_LIMIT
  const [recentReview, setRecentReview] = useState<{
    message: string
    // Monotonic id so each success remounts the StatusBanner (via `key`),
    // giving every confirmation its own fresh auto-dismiss window — even two
    // rapid successes. Without this a second success within the window would
    // inherit the first banner's countdown (the timer re-arms on banner
    // identity, not on content change).
    nonce: number
  } | null>(null)

  const { data, isLoading, isError, error } = useLinkSuggestions({ limit, offset })

  const suggestions = data?.suggestions ?? []
  const total = data?.total ?? 0

  const pageIndex = Math.floor(offset / limit)
  const totalPages = Math.max(1, Math.ceil(total / limit))

  // Rewind to the last non-empty page when reviewing the only visible row on
  // a non-first page would otherwise strand the admin on the empty-state copy
  // while rows remain. React 19.2: adjust state during render. The
  // `offset !== lastValidOffset` guard makes the correction idempotent.
  if (data && suggestions.length === 0 && offset > 0 && total > 0) {
    const lastValidOffset = Math.max(0, (Math.ceil(total / limit) - 1) * limit)
    if (offset !== lastValidOffset) {
      setOffset(lastValidOffset)
    }
  }

  const handlePrev = useCallback(() => {
    setOffset((prev) => Math.max(0, prev - limit))
  }, [limit])

  const handleNext = useCallback(() => {
    setOffset((prev) => prev + limit)
  }, [limit])

  const handleReviewSuccess = useCallback(
    (entry: LinkSuggestionEntry, verdict: LinkSuggestionVerdict) => {
      setRecentReview((prev) => ({
        message: successMessage(entry, verdict),
        nonce: (prev?.nonce ?? 0) + 1,
      }))
    },
    []
  )

  const errorMessage = useMemo(() => {
    if (!isError) return null
    return error instanceof Error
      ? error.message
      : 'Could not load the link-suggestion review queue.'
  }, [isError, error])

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-4xl space-y-6">
        <header>
          <div className="flex items-center gap-2 mb-1">
            <Music className="h-5 w-5 text-primary" />
            <h1 className="text-2xl font-bold tracking-tight">
              Discovery triage
            </h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Bandcamp / Spotify link candidates the sweep found for artists with
            no streaming link, high-confidence first. Every row is reviewed by
            hand — Accept writes the link to the artist; Reject discards it.
            <span className="font-medium text-pending-foreground">
              {' '}
              Verify
            </span>{' '}
            rows are a possible touring act or namesake — eyeball the profile
            before accepting.
          </p>
        </header>

        {/* Recent-review success banner — auto-dismiss after 6s. Longer than
            the worklist's 4s because the Bandcamp copy is a full sentence the
            admin needs to read (the async-resolver affordance). */}
        {recentReview && (
          <StatusBanner
            key={recentReview.nonce}
            variant="success"
            dismissAfterMs={6000}
            onDismiss={() => setRecentReview(null)}
            testId="link-suggestion-success-banner"
          >
            <p className="text-sm">{recentReview.message}</p>
          </StatusBanner>
        )}

        {/* Count summary */}
        {total > 0 && (
          <div className="text-xs text-muted-foreground">
            {total} pending · showing {offset + 1}–
            {Math.min(offset + suggestions.length, total)}
          </div>
        )}

        {/* List state */}
        {isLoading && (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        )}

        {errorMessage && (
          <InlineErrorBanner
            variant="queryFallback"
            testId="link-suggestion-load-error"
          >
            {errorMessage}
          </InlineErrorBanner>
        )}

        {!isLoading && !errorMessage && suggestions.length === 0 && (
          <AdminEmptyState
            icon={Inbox}
            title="Queue clear"
            message="No pending link suggestions to review right now."
            testId="link-suggestion-empty"
          />
        )}

        {!isLoading && !errorMessage && suggestions.length > 0 && (
          <div className="space-y-2" data-testid="link-suggestion-rows">
            {suggestions.map((entry) => (
              <SuggestionRow
                key={entry.id}
                entry={entry}
                onReviewSuccess={handleReviewSuccess}
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
                data-testid="link-suggestion-prev"
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
                data-testid="link-suggestion-next"
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

export default DiscoveryTriage
