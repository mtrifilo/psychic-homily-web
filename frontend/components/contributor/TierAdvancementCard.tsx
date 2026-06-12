'use client'

import Link from 'next/link'
import { Card, CardContent } from '@/components/ui/card'
import { UserTierBadge } from './UserTierBadge'
import { getNextTierInfo, TIERS_HELP_PATH } from '@/lib/tiers'
import type { UserTier } from '@/features/auth'

interface TierAdvancementCardProps {
  tier: UserTier
}

/**
 * Contributor-tier card on /profile (design board H, PSY-1061): tier badge in
 * the header, next-tier requirements behind a hairline. The board's numeric
 * progress bar ("32 / 50 qualifying edits") is deferred — per-user
 * advancement progress only exists behind the admin evaluate endpoint today
 * (see the follow-up ticket on PSY-1061).
 */
export function TierAdvancementCard({ tier }: TierAdvancementCardProps) {
  const next = getNextTierInfo(tier)

  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-base font-semibold">Contributor tier</h2>
          <UserTierBadge tier={tier} />
        </div>

        {next ? (
          <div className="mt-4 border-t border-border/50 pt-3">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-sm text-muted-foreground">Next:</span>
              <UserTierBadge tier={next.tier} />
            </div>
            <p className="mt-3 font-mono text-xs uppercase tracking-wider text-muted-foreground">
              Requirements
            </p>
            <ul className="mt-1.5 space-y-1 text-sm">
              {next.advancementRequirements?.map(req => (
                <li key={req.text} className="flex items-baseline gap-2">
                  <span
                    aria-hidden
                    className="text-[8px] leading-none text-muted-foreground"
                  >
                    ●
                  </span>
                  {req.text}
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <p className="mt-4 border-t border-border/50 pt-3 text-sm text-muted-foreground">
            You&rsquo;re at the highest contributor tier. Thanks for being a
            pillar of the community.
          </p>
        )}

        <p className="mt-4 text-xs text-muted-foreground">
          Advancement is automatic and evaluated daily.{' '}
          <Link
            href={TIERS_HELP_PATH}
            className="underline hover:text-foreground"
          >
            Learn more about tiers
          </Link>
          .
        </p>
      </CardContent>
    </Card>
  )
}
