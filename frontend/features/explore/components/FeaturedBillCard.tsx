'use client'

/**
 * FeaturedBillCard (PSY-837)
 *
 * Hero card for the admin-curated featured bill slot. Renders a 160px
 * thumb (when an image_url is present), a venue/date eyebrow, the
 * headline (artists / show title), the curator's note (rendered HTML
 * from the markdown source — sanitized server-side by goldmark +
 * bluemonday), and a "View show →" link.
 *
 * The parent collapses the section entirely when `bill` is null —
 * this component assumes a non-null prop.
 */

import Link from 'next/link'
import Image from 'next/image'
import type { ExploreFeaturedBill } from '../types'
import { formatShowDate } from '@/lib/utils/formatters'

interface FeaturedBillCardProps {
  bill: ExploreFeaturedBill
}

export function FeaturedBillCard({ bill }: FeaturedBillCardProps) {
  const detailsHref = `/shows/${bill.slug || bill.id}`
  const dateLine = formatShowDate(bill.event_date, bill.venue_state)
  const venueLine = [bill.venue_name, bill.venue_city, bill.venue_state]
    .filter(Boolean)
    .join(' · ')

  return (
    <article className="bg-card/50 border border-border/50 rounded-xl p-6 hover:border-border transition-colors">
      <div className="flex flex-col sm:flex-row gap-5">
        {bill.image_url && (
          <Link
            href={detailsHref}
            className="shrink-0 block overflow-hidden rounded-lg"
            aria-label={bill.title}
          >
            <Image
              src={bill.image_url}
              alt={bill.title}
              width={160}
              height={160}
              className="h-40 w-40 object-cover"
            />
          </Link>
        )}

        <div className="flex-1 min-w-0">
          <div className="text-xs uppercase tracking-wider text-muted-foreground">
            {dateLine}
            {venueLine && (
              <>
                <span className="px-1.5">·</span>
                <span className="normal-case tracking-normal">{venueLine}</span>
              </>
            )}
          </div>

          <h3 className="mt-1.5 text-xl font-semibold leading-tight tracking-tight">
            <Link
              href={detailsHref}
              className="hover:text-primary transition-colors"
            >
              {bill.headliner_name || bill.title}
            </Link>
          </h3>

          {bill.curator_note_html && (
            <div
              className="mt-3 text-sm leading-relaxed text-foreground/85 prose prose-sm max-w-none dark:prose-invert"
              dangerouslySetInnerHTML={{ __html: bill.curator_note_html }}
            />
          )}

          <Link
            href={detailsHref}
            className="inline-block mt-4 px-4 py-2 text-sm bg-muted/50 border border-border/50 rounded-lg hover:bg-muted hover:border-border transition-colors"
          >
            View show →
          </Link>
        </div>
      </div>
    </article>
  )
}
