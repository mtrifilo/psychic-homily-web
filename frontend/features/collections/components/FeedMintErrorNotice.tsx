'use client'

import { AlertCircle } from 'lucide-react'
import { mintErrorMessage } from '@/lib/errors'
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'

interface FeedMintErrorNoticeProps {
  /** The create-token mutation's error, or null while it has not failed. */
  error: unknown
}

/**
 * The refusal line for the personal feed token, shown beside every control that
 * can mint one: Enable and Regenerate on both the calendar card and the follows
 * activity card. Without it a refused click is silent.
 *
 * One component rather than a copy per card, because the two cards rotate the
 * same token and a reader who saw one message in one place and different words
 * in the other would read them as different failures.
 */
export function FeedMintErrorNotice({ error }: FeedMintErrorNoticeProps) {
  if (!error) return null

  return (
    <InlineErrorBanner className="flex items-start gap-2 mt-3 p-2 text-xs">
      <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      <span>
        {mintErrorMessage(
          error,
          'Could not update the feed. Please try again.'
        )}
      </span>
    </InlineErrorBanner>
  )
}
