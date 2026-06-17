'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'

/**
 * Owner-facing empty-section prompt: the dashed CTA box the profile boards
 * use for "this slot is yours to fill" states (bio, collections). PSY-1062.
 */
export function ProfileEmptyPrompt({
  message,
  ctaLabel,
  ctaHref,
}: {
  message: string
  ctaLabel: string
  ctaHref: string
}) {
  return (
    <div className="mt-2 flex items-center justify-between gap-3 rounded-md border border-dashed border-border bg-muted/20 px-4 py-3">
      <p className="text-sm text-muted-foreground">{message}</p>
      <Button asChild variant="outline" size="sm" className="shrink-0">
        <Link href={ctaHref}>{ctaLabel}</Link>
      </Button>
    </div>
  )
}
