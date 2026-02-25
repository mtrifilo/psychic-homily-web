'use client'

import { useState } from 'react'
import {
  CalendarDays,
  Copy,
  Check,
  RefreshCw,
  Trash2,
  Loader2,
  ExternalLink,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useCalendarTokenStatus,
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from '@/lib/hooks/useCalendarFeed'

export function CalendarFeedSection() {
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

  // Just created a token — show the feed URL
  if (createdToken) {
    const webcalUrl = createdToken.feed_url.replace(/^https?:\/\//, 'webcal://')
    const googleCalUrl = `https://calendar.google.com/calendar/r?cid=${encodeURIComponent(webcalUrl)}`

    return (
      <div className="mb-6 rounded-lg border border-border bg-card p-4">
        <div className="flex items-center gap-2 mb-3">
          <CalendarDays className="h-5 w-5 text-primary" />
          <h3 className="text-sm font-semibold">Calendar Feed Active</h3>
        </div>
        <p className="text-xs text-muted-foreground mb-3">
          Copy this URL and add it to your calendar app. Your saved shows will
          automatically stay in sync.
        </p>

        {/* Feed URL with copy button */}
        <div className="flex gap-2 mb-3">
          <Input
            readOnly
            value={createdToken.feed_url}
            className="font-mono text-xs"
            onFocus={(e) => e.target.select()}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleCopy(createdToken.feed_url)}
            className="shrink-0"
          >
            {copied ? (
              <Check className="h-4 w-4" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </Button>
        </div>

        {/* Quick-add links */}
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
            href={webcalUrl}
            className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary/80 underline underline-offset-2"
          >
            Apple Calendar
            <ExternalLink className="h-3 w-3" />
          </a>
        </div>

        {/* Management actions */}
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

  // Token exists (from a previous session) — show status
  if (tokenStatus?.has_token) {
    return (
      <div className="mb-6 rounded-lg border border-border bg-card p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CalendarDays className="h-5 w-5 text-primary" />
            <div>
              <h3 className="text-sm font-semibold">Calendar Feed Active</h3>
              <p className="text-xs text-muted-foreground">
                Your saved shows are syncing to your calendar app.
              </p>
            </div>
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
      </div>
    )
  }

  // No token — show setup prompt
  return (
    <div className="mb-6 rounded-lg border border-dashed border-border bg-card/50 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <CalendarDays className="h-5 w-5 text-muted-foreground" />
          <div>
            <h3 className="text-sm font-semibold">Subscribe to Calendar</h3>
            <p className="text-xs text-muted-foreground">
              Sync your saved shows to Google Calendar, Apple Calendar, or
              Outlook.
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleCreate}
          disabled={createToken.isPending}
          className="text-xs"
        >
          {createToken.isPending ? (
            <Loader2 className="h-3 w-3 mr-1 animate-spin" />
          ) : (
            <CalendarDays className="h-3 w-3 mr-1" />
          )}
          Enable
        </Button>
      </div>
    </div>
  )
}
