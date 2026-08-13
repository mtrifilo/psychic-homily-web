'use client'

import { useState } from 'react'
import { ExternalLink, Download } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  googleCalendarSubscribeUrl,
} from '@/lib/utils/calendarFeedUrls'
import { API_BASE_URL } from '@/lib/api-base'

/**
 * Builds the absolute URL of the scene's public .ics feed.
 *
 * Mirrors `showCalendarIcsUrl`: the link is a download / subscribe target that
 * may be copied out of the browser, so it has to be absolute. `API_BASE_URL`
 * is the relative `/api` in a development browser (proxied same-origin), which
 * still resolves — the Next catch-all proxy forwards `/api/*` and passes the
 * calendar content-type through — it just has to be made absolute first.
 * SSR-safe: on the server the relative branch yields a root-relative URL, and
 * nothing renders it before the popover opens anyway.
 */
export function sceneCalendarIcsUrl(slug: string): string {
  const base = API_BASE_URL.replace(/\/$/, '')
  const path = `/scenes/${encodeURIComponent(slug)}/calendar.ics`

  if (/^https?:\/\//.test(API_BASE_URL)) {
    return `${base}${path}`
  }

  const origin = typeof window === 'undefined' ? '' : window.location.origin
  return `${origin}${base}${path}`
}

interface SceneAddToCalendarProps {
  slug: string
}

/**
 * "[Subscribe: .ics]" popover for a scene feed: Google Calendar subscribe and
 * a one-shot .ics download. Locked mock copy (P6); the two actions mirror
 * `ShowAddToCalendar`, except this is a live feed rather than a single event,
 * so Google gets `googleCalendarSubscribeUrl` instead of a template URL.
 *
 * Deliberately never auth-gated: the backend feed is public by slug.
 */
export function SceneAddToCalendar({ slug }: SceneAddToCalendarProps) {
  const [open, setOpen] = useState(false)
  const icsUrl = sceneCalendarIcsUrl(slug)
  const googleUrl = googleCalendarSubscribeUrl(icsUrl)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <BracketLink label="Subscribe: .ics" active={open} />
      </PopoverTrigger>
      <PopoverContent className="w-72 p-4" align="start">
        <div className="flex flex-col gap-2.5">
          <a
            href={googleUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:text-primary/80"
          >
            Google Calendar
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
          <a
            href={icsUrl}
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:text-primary/80"
          >
            Apple / Outlook (.ics)
            <Download className="h-3.5 w-3.5" />
          </a>
        </div>
      </PopoverContent>
    </Popover>
  )
}
