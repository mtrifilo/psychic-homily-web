'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  CalendarDays,
  Copy,
  Check,
  RefreshCw,
  Trash2,
  Loader2,
  ExternalLink,
  AlertCircle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useCalendarTokenStatus,
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from '@/features/auth'

export type CalendarFeedVariant = 'settings' | 'library'

interface CalendarFeedSectionProps {
  /** settings owns regenerate/disable; library is subscribe/copy + link to settings */
  variant?: CalendarFeedVariant
}

function googleCalendarSubscribeURL(feedURL: string): string {
  const webcalUrl = feedURL.replace(/^https?:\/\//, 'webcal://')
  return `https://calendar.google.com/calendar/r?cid=${encodeURIComponent(webcalUrl)}`
}

function webcalURL(feedURL: string): string {
  return feedURL.replace(/^https?:\/\//, 'webcal://')
}

export function CalendarFeedSection({
  variant = 'library',
}: CalendarFeedSectionProps) {
  const { data: tokenStatus, isLoading } = useCalendarTokenStatus()
  const createToken = useCreateCalendarToken()
  const deleteToken = useDeleteCalendarToken()
  const [createdToken, setCreatedToken] = useState<{
    token: string
    feed_url: string
  } | null>(null)
  const [copied, setCopied] = useState(false)

  const handleCreate = async () => {
    try {
      const result = await createToken.mutateAsync()
      setCreatedToken({ token: result.token, feed_url: result.feed_url })
    } catch {
      // Error handled by mutation state
    }
  }

  const handleRegenerate = async () => {
    try {
      const result = await createToken.mutateAsync()
      setCreatedToken({ token: result.token, feed_url: result.feed_url })
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

  const isSettings = variant === 'settings'

  // Just created / regenerated — show the feed URL (only moment plaintext is available)
  if (createdToken) {
    const googleCalUrl = googleCalendarSubscribeURL(createdToken.feed_url)
    const appleCalUrl = webcalURL(createdToken.feed_url)

    return (
      <div className="rounded-md border border-border bg-card p-4">
        <div className="flex items-center gap-2 mb-3">
          <CalendarDays className="h-5 w-5 text-primary" />
          <h3 className="text-sm font-semibold">
            {isSettings ? 'Saved shows calendar feed' : 'Calendar Feed Active'}
          </h3>
        </div>
        <p className="text-xs text-muted-foreground mb-3">
          Copy this URL and subscribe in Google Calendar or Apple Calendar. Your
          saved upcoming shows stay in sync.
        </p>

        <div className="flex gap-2 mb-3">
          <Input
            readOnly
            value={createdToken.feed_url}
            className="font-mono text-xs"
            onFocus={e => e.target.select()}
            aria-label="Calendar feed URL"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleCopy(createdToken.feed_url)}
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

        <div className="flex flex-wrap gap-2 mb-3">
          <a
            href={googleCalUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary/80 underline underline-offset-2"
          >
            Google Calendar
            <ExternalLink className="h-3 w-3" />
          </a>
          <a
            href={appleCalUrl}
            className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary/80 underline underline-offset-2"
          >
            Apple Calendar
            <ExternalLink className="h-3 w-3" />
          </a>
        </div>

        {isSettings && (
          <div className="flex items-start gap-2 mb-3 text-xs text-muted-foreground">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span>
              Anyone with this URL can see your saved shows. Regenerate
              immediately if it leaks — the old URL stops working right away.
              Copy it now; it won&apos;t be shown again until you regenerate.
            </span>
          </div>
        )}

        {isSettings ? (
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
        ) : (
          <p className="text-xs text-muted-foreground">
            Regenerate or disable from{' '}
            <Link
              href="/profile?tab=settings"
              className="text-primary underline underline-offset-2"
            >
              Settings
            </Link>
            .
          </p>
        )}
      </div>
    )
  }

  // Token exists (from a previous session) — plaintext URL is not recoverable
  if (tokenStatus?.has_token) {
    if (isSettings) {
      return (
        <div className="rounded-md border border-border bg-card p-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 items-start gap-2">
              <CalendarDays className="h-5 w-5 text-primary mt-0.5" />
              <div>
                <h3 className="text-sm font-semibold">
                  Saved shows calendar feed
                </h3>
                <p className="text-xs text-muted-foreground mt-1">
                  Your feed is enabled. The subscribe URL was shown when you
                  created it — regenerate to get a new URL (invalidates the old
                  one immediately).
                </p>
                <div className="flex items-start gap-2 mt-2 text-xs text-muted-foreground">
                  <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                  <span>
                    Anyone with the URL can see your saved shows. Regenerate if
                    it may have leaked.
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

    // Library: subscribe affordance + point to Settings for manage
    return (
      <div className="rounded-md border border-border bg-card px-5 py-3.5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-2">
            <CalendarDays className="h-5 w-5 text-primary shrink-0" />
            <div>
              <h3 className="text-sm font-semibold">Calendar feed enabled</h3>
              <p className="text-xs text-muted-foreground">
                Copy or regenerate your subscribe URL in{' '}
                <Link
                  href="/profile?tab=settings"
                  className="text-primary underline underline-offset-2"
                >
                  Settings
                </Link>
                .
              </p>
            </div>
          </div>
          <Button variant="outline" size="sm" className="text-xs shrink-0" asChild>
            <Link href="/profile?tab=settings">Manage feed</Link>
          </Button>
        </div>
      </div>
    )
  }

  // No token — setup prompt
  return (
    <div
      className={
        isSettings
          ? 'rounded-md border border-border bg-card p-4'
          : 'rounded-md border border-border bg-card px-5 py-3.5'
      }
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 className="text-sm font-medium">
            {isSettings
              ? 'Saved shows calendar feed'
              : 'Subscribe to calendar'}
          </h3>
          <p className="text-xs text-muted-foreground">
            Sync your saved shows to Google Calendar, Apple Calendar, or
            Outlook.
          </p>
          {isSettings && (
            <p className="text-xs text-muted-foreground mt-1">
              Uses a dedicated feed token (not your login). Anyone with the URL
              can see your saved shows.
            </p>
          )}
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
