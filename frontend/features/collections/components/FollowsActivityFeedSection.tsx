'use client'

import { useState } from 'react'
import {
  Rss,
  Copy,
  Check,
  RefreshCw,
  Trash2,
  Loader2,
  AlertCircle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useCalendarTokenStatus,
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from '@/features/auth'

/**
 * Settings manage surface for the followed-artist Atom activity feed (PSY-1505).
 * Reuses the personal calendar_tokens row from PSY-1430 — create / regenerate /
 * revoke rotates both the iCal and Atom URLs.
 */
export function FollowsActivityFeedSection() {
  const { data: tokenStatus, isLoading } = useCalendarTokenStatus()
  const createToken = useCreateCalendarToken()
  const deleteToken = useDeleteCalendarToken()
  const [createdToken, setCreatedToken] = useState<{
    token: string
    follows_feed_url: string
  } | null>(null)
  const [copied, setCopied] = useState(false)

  const handleCreate = async () => {
    try {
      const result = await createToken.mutateAsync()
      setCreatedToken({
        token: result.token,
        follows_feed_url: result.follows_feed_url,
      })
    } catch {
      // Error handled by mutation state
    }
  }

  const handleRegenerate = async () => {
    try {
      const result = await createToken.mutateAsync()
      setCreatedToken({
        token: result.token,
        follows_feed_url: result.follows_feed_url,
      })
    } catch {
      // Error handled by mutation state
    }
  }

  const handleDelete = async () => {
    try {
      await deleteToken.mutateAsync()
      setCreatedToken(null)
    } catch {
      // Error handled by mutation state
    }
  }

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Fallback: select the input text
    }
  }

  if (isLoading) {
    return null
  }

  if (createdToken) {
    return (
      <div className="rounded-md border border-border bg-card p-4">
        <div className="flex items-center gap-2 mb-3">
          <Rss className="h-5 w-5 text-primary" />
          <h3 className="text-sm font-semibold">Followed artists activity feed</h3>
        </div>
        <p className="text-xs text-muted-foreground mb-3">
          Subscribe in any Atom/RSS reader. New shows and releases for artists
          you follow appear here (last 90 days).
        </p>

        <div className="flex gap-2 mb-3">
          <Input
            readOnly
            value={createdToken.follows_feed_url}
            className="font-mono text-xs"
            onFocus={e => e.target.select()}
            aria-label="Follows activity feed URL"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleCopy(createdToken.follows_feed_url)}
            className="shrink-0"
            aria-label={copied ? 'Copied' : 'Copy feed URL'}
          >
            {copied ? (
              <Check className="h-4 w-4" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </Button>
        </div>

        <div className="flex items-start gap-2 mb-3 text-xs text-muted-foreground">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span>
            Anyone with this URL can see followed-artist activity. Regenerating
            rotates this URL and your saved-shows calendar feed immediately —
            they share one personal feed token. Copy it now; it won&apos;t be
            shown again until you regenerate.
          </span>
        </div>

        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRegenerate}
            disabled={createToken.isPending}
            className="text-xs"
          >
            {createToken.isPending ? (
              <Loader2 className="h-3 w-3 mr-1 animate-spin" />
            ) : (
              <RefreshCw className="h-3 w-3 mr-1" />
            )}
            Regenerate
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleDelete}
            disabled={deleteToken.isPending}
            className="text-xs text-destructive hover:text-destructive"
          >
            {deleteToken.isPending ? (
              <Loader2 className="h-3 w-3 mr-1 animate-spin" />
            ) : (
              <Trash2 className="h-3 w-3 mr-1" />
            )}
            Disable
          </Button>
        </div>
      </div>
    )
  }

  if (tokenStatus?.has_token) {
    return (
      <div className="rounded-md border border-border bg-card p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 items-start gap-2">
            <Rss className="h-5 w-5 text-primary mt-0.5" />
            <div>
              <h3 className="text-sm font-semibold">
                Followed artists activity feed
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Your personal feed token is enabled. The subscribe URL was shown
                when you created it — regenerate to get a new URL (invalidates
                the old one and your calendar feed URL immediately).
              </p>
              <div className="flex items-start gap-2 mt-2 text-xs text-muted-foreground">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span>
                  Anyone with the URL can see followed-artist activity.
                  Regenerate if it may have leaked.
                </span>
              </div>
            </div>
          </div>
          <div className="flex flex-wrap gap-2 shrink-0">
            <Button
              variant="outline"
              size="sm"
              onClick={handleRegenerate}
              disabled={createToken.isPending}
              className="text-xs"
            >
              {createToken.isPending ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <RefreshCw className="h-3 w-3 mr-1" />
              )}
              Regenerate
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleDelete}
              disabled={deleteToken.isPending}
              className="text-xs text-destructive hover:text-destructive"
            >
              {deleteToken.isPending ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <Trash2 className="h-3 w-3 mr-1" />
              )}
              Disable
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-md border border-border bg-card p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 className="text-sm font-medium">Followed artists activity feed</h3>
          <p className="text-xs text-muted-foreground">
            Atom feed of new shows and releases for artists you follow.
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            Uses the same personal feed token as your calendar feed (not your
            login). Anyone with the URL can see followed-artist activity.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleCreate}
          disabled={createToken.isPending}
          className="text-xs shrink-0"
        >
          {createToken.isPending ? 'Enabling…' : 'Enable'}
        </Button>
      </div>
    </div>
  )
}
