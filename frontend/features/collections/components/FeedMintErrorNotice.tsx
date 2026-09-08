'use client'

import { AlertCircle } from 'lucide-react'
import { isReauthRequired, REAUTH_REQUIRED_MESSAGE } from '@/lib/errors'

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
    <div
      role="alert"
      className="flex items-start gap-2 mt-3 text-xs text-destructive"
    >
      <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      <span>
        {isReauthRequired(error)
          ? REAUTH_REQUIRED_MESSAGE
          : 'Could not update the feed. Please try again.'}
      </span>
    </div>
  )
}
