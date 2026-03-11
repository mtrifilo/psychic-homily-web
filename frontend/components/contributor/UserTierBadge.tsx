'use client'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { UserTier } from '@/lib/types/contributor'

const tierConfig: Record<UserTier, { label: string; className: string }> = {
  new_user: {
    label: 'New User',
    className: 'bg-muted text-muted-foreground border-0',
  },
  contributor: {
    label: 'Contributor',
    className: 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-0',
  },
  trusted_contributor: {
    label: 'Trusted Contributor',
    className: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-0',
  },
  local_ambassador: {
    label: 'Local Ambassador',
    className: 'bg-purple-500/10 text-purple-600 dark:text-purple-400 border-0',
  },
}

interface UserTierBadgeProps {
  tier: UserTier
  className?: string
}

export function UserTierBadge({ tier, className }: UserTierBadgeProps) {
  const config = tierConfig[tier] || tierConfig.new_user

  return (
    <Badge
      variant="secondary"
      className={cn(config.className, className)}
    >
      {config.label}
    </Badge>
  )
}
