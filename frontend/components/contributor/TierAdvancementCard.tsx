'use client'

import Link from 'next/link'
import { ArrowRight, Award } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { UserTierBadge } from './UserTierBadge'
import { getNextTierInfo, TIERS_HELP_PATH } from '@/lib/tiers'
import type { UserTier } from '@/features/auth'

interface TierAdvancementCardProps {
  tier: UserTier
}

export function TierAdvancementCard({ tier }: TierAdvancementCardProps) {
  const next = getNextTierInfo(tier)

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <Award className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-lg">Contributor Tier</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm text-muted-foreground">Your tier:</span>
          <UserTierBadge tier={tier} />
        </div>

        {next ? (
          <div className="rounded-md border border-border/60 bg-muted/30 p-4">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-sm text-muted-foreground">Next:</span>
              <UserTierBadge tier={next.tier} />
              <ArrowRight className="h-3.5 w-3.5 text-muted-foreground" />
            </div>
            <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-2">
              Requirements
            </p>
            <ul className="list-disc pl-5 space-y-1 text-sm">
              {next.advancementRequirements?.map(req => (
                <li key={req.text}>{req.text}</li>
              ))}
            </ul>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            You&rsquo;re at the highest contributor tier. Thanks for being
            a pillar of the community.
          </p>
        )}

        <p className="text-xs text-muted-foreground">
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
