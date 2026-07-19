'use client'

/**
 * Admin queue for community radio-play match suggestions.
 *
 * Separate lane from the existing unmatched-plays bulk-link chips
 * (admin-initiated suggested_matches). Accept runs LinkPlay; optional
 * also_bulk_link_name runs BulkLinkPlays for the play's artist_name.
 */

import { useCallback, useState } from 'react'
import { CheckCircle2, Loader2, Link2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { RejectWithReasonRow } from '@/components/admin'
import { InlineErrorBanner } from '@/components/shared'
import {
  useAcceptMatchSuggestion,
  useAdminMatchSuggestions,
  useRejectMatchSuggestion,
  type RadioPlayMatchSuggestionEntry,
} from '@/lib/hooks/admin/useAdminRadio'

function formatSubmittedAt(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

function CommunitySuggestionRow({
  suggestion,
}: {
  suggestion: RadioPlayMatchSuggestionEntry
}) {
  const accept = useAcceptMatchSuggestion()
  const reject = useRejectMatchSuggestion()
  const [alsoBulkLink, setAlsoBulkLink] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const isActioning = accept.isPending || reject.isPending

  const handleAccept = useCallback(() => {
    setActionError(null)
    accept.mutate(
      { suggestionId: suggestion.id, alsoBulkLinkName: alsoBulkLink },
      {
        onError: (err) => {
          setActionError(
            err instanceof Error ? err.message : 'Failed to accept suggestion'
          )
        },
      }
    )
  }, [accept, alsoBulkLink, suggestion.id])

  const handleReject = useCallback(
    (reason: string) => {
      setActionError(null)
      reject.mutate(
        { suggestionId: suggestion.id, reason },
        {
          onError: (err) => {
            setActionError(
              err instanceof Error ? err.message : 'Failed to reject suggestion'
            )
          },
        }
      )
    },
    [reject, suggestion.id]
  )

  const checkboxId = `bulk-link-${suggestion.id}`

  return (
    <li
      className="rounded-lg border p-4"
      data-testid="community-match-suggestion-row"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <p className="font-medium">
            <span className="text-muted-foreground">Play:</span>{' '}
            {suggestion.play_artist_name}
            <span className="mx-1.5 text-muted-foreground" aria-hidden="true">
              →
            </span>
            <span className="text-primary">{suggestion.suggested_artist_name}</span>
          </p>
          <div className="flex flex-wrap items-center gap-1.5 text-sm text-muted-foreground">
            <span>
              by{' '}
              {suggestion.submitter_username
                ? `@${suggestion.submitter_username}`
                : `user #${suggestion.submitted_by}`}
            </span>
            <span aria-hidden="true">·</span>
            <span>{formatSubmittedAt(suggestion.created_at)}</span>
            <span aria-hidden="true">·</span>
            <Badge variant="outline" className="text-[10px] font-mono">
              {suggestion.play_match_state}
            </Badge>
          </div>
          {suggestion.note && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{suggestion.note}&rdquo;
            </p>
          )}
        </div>
      </div>

      <div className="mt-3 flex items-center gap-2">
        <Checkbox
          id={checkboxId}
          checked={alsoBulkLink}
          onCheckedChange={(v) => setAlsoBulkLink(v === true)}
          disabled={isActioning}
        />
        <Label
          htmlFor={checkboxId}
          className="text-sm font-normal text-muted-foreground"
        >
          Also bulk-link this artist name
        </Label>
      </div>

      <RejectWithReasonRow
        onApprove={handleAccept}
        onReject={handleReject}
        isActioning={isActioning}
        isApproving={accept.isPending}
        isRejecting={reject.isPending}
        approveLabel="Accept"
        approveIcon={Link2}
        rejectPlaceholder="Rejection reason (required)"
      />

      {actionError && (
        <div className="mt-2">
          <InlineErrorBanner testId="community-match-suggestion-error">
            {actionError}
          </InlineErrorBanner>
        </div>
      )}
    </li>
  )
}

/**
 * Pending community match-suggestion queue for the admin Matching tab.
 */
export function CommunityMatchSuggestionsQueue() {
  const { data, isLoading, isFetching, isError, error } =
    useAdminMatchSuggestions(50, 0)

  const suggestions = data?.suggestions ?? []
  const total = data?.total ?? 0

  return (
    <div className="space-y-3" data-testid="community-match-suggestions-queue">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">
          Community suggestions
          {total > 0 && (
            <Badge variant="secondary" className="ml-2 align-middle">
              {total}
            </Badge>
          )}
        </h3>
        {isFetching && !isLoading && (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : isError ? (
        <InlineErrorBanner testId="community-match-suggestions-load-error">
          {error instanceof Error
            ? error.message
            : 'Failed to load community suggestions'}
        </InlineErrorBanner>
      ) : suggestions.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <CheckCircle2 className="mx-auto mb-2 h-8 w-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">
            No pending community match suggestions.
          </p>
        </div>
      ) : (
        <ul className="space-y-3">
          {suggestions.map((suggestion) => (
            <CommunitySuggestionRow
              key={suggestion.id}
              suggestion={suggestion}
            />
          ))}
        </ul>
      )}
    </div>
  )
}
