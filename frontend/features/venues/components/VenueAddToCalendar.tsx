'use client'

import { useState } from 'react'
import { Copy, Check, ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { BracketLink } from '@/components/shared/BracketLink'
import { useAutoDismissBanner } from '@/lib/hooks/common/useAutoDismissBanner'
import {
  webcalUrl,
  googleCalendarSubscribeUrl,
} from '@/lib/utils/calendarFeedUrls'
import { API_BASE_URL } from '@/lib/api-base'

/**
 * Builds the absolute URL of a venue's public ICS feed.
 *
 * It has to be absolute: `webcal://` and Google Calendar's `cid=` parameter
 * both need a full origin, and a subscription URL gets copied out of the browser
 * into a calendar client that has no notion of this page's origin.
 *
 * `API_BASE_URL` is the relative `/api` in a development browser, where requests
 * are proxied same-origin for cookie reasons. That still resolves correctly —
 * the Next catch-all proxy forwards `/api/*` to the backend and passes the
 * calendar content-type through — it just has to be made absolute first.
 *
 * MUST only run in the browser: it reads window.location for the relative case.
 * The caller renders it behind a user interaction, so it never executes during
 * SSR and cannot cause a hydration mismatch.
 */
export function venueCalendarFeedUrl(venueSlug: string): string {
  const path = `/venues/${encodeURIComponent(venueSlug)}/calendar.ics`

  if (/^https?:\/\//.test(API_BASE_URL)) {
    return `${API_BASE_URL.replace(/\/$/, '')}${path}`
  }

  const origin = typeof window === 'undefined' ? '' : window.location.origin
  return `${origin}${API_BASE_URL.replace(/\/$/, '')}${path}`
}

interface VenueAddToCalendarProps {
  venueSlug: string
  venueName: string
}

/**
 * "[Add to calendar]" popover for a venue's public ICS feed: the feed URL with
 * a copy button, plus direct Google/Apple subscribe links.
 *
 * Deliberately available to anonymous visitors — a venue linking back to its
 * own calendar is the point of the feed, and a login wall would defeat it.
 *
 * A popover rather than an inline panel so it does not change the height of
 * the host row, and so the feed URL is only computed once the popover opens —
 * which keeps window access out of the server render.
 */
export function VenueAddToCalendar({
  venueSlug,
  venueName,
}: VenueAddToCalendarProps) {
  const [open, setOpen] = useState(false)
  const { value: copied, show: showCopied } = useAutoDismissBanner<boolean>(2000)

  if (!venueSlug) return null

  const feedUrl = open ? venueCalendarFeedUrl(venueSlug) : ''

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(feedUrl)
      showCopied(true)
    } catch {
      // Clipboard can be unavailable (insecure origin, denied permission). The
      // URL stays selectable in the input, which is the fallback.
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <BracketLink label="Add to calendar" active={open} />
      </PopoverTrigger>
      <PopoverContent className="w-80" align="end">
        <p className="text-sm font-medium mb-1">
          Add shows at {venueName} to your calendar
        </p>
        <p className="text-xs text-muted-foreground mb-3">
          Upcoming shows appear in your calendar automatically, at the
          venue&apos;s local time.
        </p>

        <div className="flex gap-2 mb-3">
          <Input
            readOnly
            value={feedUrl}
            className="font-mono text-xs"
            onFocus={e => e.target.select()}
            aria-label={`Calendar feed URL for ${venueName}`}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleCopy}
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

        <div className="flex flex-wrap gap-3">
          <a
            href={googleCalendarSubscribeUrl(feedUrl)}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary/80 underline underline-offset-2"
          >
            Google Calendar
            <ExternalLink className="h-3 w-3" />
          </a>
          <a
            href={webcalUrl(feedUrl)}
            className="inline-flex items-center gap-1 text-xs text-primary hover:text-primary/80 underline underline-offset-2"
          >
            Apple Calendar
            <ExternalLink className="h-3 w-3" />
          </a>
        </div>
      </PopoverContent>
    </Popover>
  )
}
