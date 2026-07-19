'use client'

import Link from 'next/link'
import { Check } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { UserTierBadge } from './UserTierBadge'
import { getNextTierInfo, TIERS_HELP_PATH } from '@/lib/tiers'
import { useAdvancementProgress } from '@/features/auth'
import type { AdvancementRequirement } from '@/features/auth'
import type { UserTier } from '@/features/auth'

interface TierAdvancementCardProps {
  tier: UserTier
}

const APPROVED_EDITS_ID = 'approved_edits'

function findApprovedEdits(
  requirements: AdvancementRequirement[] | undefined
): AdvancementRequirement | undefined {
  return requirements?.find(r => r.requirement === APPROVED_EDITS_ID)
}

function progressPercent(current: number, threshold: number): number {
  if (threshold <= 0) return 100
  return Math.min(100, Math.max(0, (current / threshold) * 100))
}

/**
 * Contributor-tier card on /profile (design board H, PSY-1061 + PSY-1087):
 * tier badge, next-tier row with Space Mono "current / threshold" counter,
 * primary approved-edits progress bar, and a dense met/unmet requirements list.
 */
export function TierAdvancementCard({ tier }: TierAdvancementCardProps) {
  const next = getNextTierInfo(tier)
  // Highest tier has nothing to fetch; skip the request.
  const { data: advancement } = useAdvancementProgress(Boolean(next))

  const edits = findApprovedEdits(advancement?.requirements)
  const current = edits?.current ?? 0
  const threshold = edits?.threshold ?? 0
  const showBar =
    Boolean(next) &&
    edits != null &&
    typeof edits.current === 'number' &&
    typeof edits.threshold === 'number' &&
    edits.threshold > 0

  const metById = new Map(
    (advancement?.requirements ?? []).map(r => [r.requirement, r.met])
  )

  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-base font-semibold">Contributor tier</h2>
          <UserTierBadge tier={tier} />
        </div>

        {next ? (
          <div className="mt-4 border-t border-border/50 pt-3 space-y-3">
            <div className="flex items-center justify-between gap-3 flex-wrap">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm text-muted-foreground">Next:</span>
                <UserTierBadge tier={next.tier} />
              </div>
              {showBar && (
                <p className="font-mono text-xs text-muted-foreground tabular-nums">
                  {Math.floor(current)} / {Math.floor(threshold)} qualifying
                  edits
                </p>
              )}
            </div>

            {showBar && (
              <div
                className="h-2 w-full overflow-hidden rounded bg-muted"
                role="progressbar"
                aria-valuenow={Math.floor(current)}
                aria-valuemin={0}
                aria-valuemax={Math.floor(threshold)}
                aria-label="Approved edits toward next tier"
              >
                <div
                  className="h-full rounded bg-primary transition-[width] duration-300"
                  style={{ width: `${progressPercent(current, threshold)}%` }}
                />
              </div>
            )}

            <div>
              <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
                Requirements
              </p>
              <ul className="mt-1.5 space-y-1 text-sm">
                {next.advancementRequirements?.map(req => {
                  const met = metById.get(req.id)
                  return (
                    <li key={req.id} className="flex items-baseline gap-2">
                      {met === true ? (
                        <Check
                          aria-label="Met"
                          className="h-3 w-3 shrink-0 text-primary translate-y-0.5"
                        />
                      ) : (
                        <span
                          aria-hidden
                          className="text-[8px] leading-none text-muted-foreground"
                        >
                          ●
                        </span>
                      )}
                      <span
                        className={
                          met === true ? 'text-muted-foreground' : undefined
                        }
                      >
                        {req.text}
                      </span>
                    </li>
                  )
                })}
              </ul>
            </div>
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
