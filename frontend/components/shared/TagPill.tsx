'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'

export interface TagPillProps {
  /** The tag label text */
  label: string
  /** Optional vote count to display inline, e.g. +12 */
  voteCount?: number
  /** URL to navigate to when clicked (e.g., filter view) */
  href?: string
  /** Additional CSS classes */
  className?: string
  /** Click handler (used when no href is provided) */
  onClick?: () => void
}

/**
 * A small rounded pill for displaying tags on entity cards and detail pages.
 * Supports optional vote count display and click-to-filter navigation.
 *
 * Usage:
 *   <TagPill label="post-punk" voteCount={12} href="/shows?tag=post-punk" />
 *   <TagPill label="shoegaze" onClick={() => handleTagFilter('shoegaze')} />
 */
export function TagPill({
  label,
  voteCount,
  href,
  className,
  onClick,
}: TagPillProps) {
  const pillClasses = cn(
    'inline-flex items-center gap-1 rounded-full px-2.5 py-0.5',
    'text-xs font-medium',
    'bg-muted text-muted-foreground',
    'border border-border/50',
    'transition-colors duration-100',
    'hover:bg-muted/80 hover:text-foreground hover:border-border',
    'cursor-pointer select-none',
    className
  )

  const content = (
    <>
      <span>{label}</span>
      {voteCount !== undefined && (
        <span className="text-primary/70 font-semibold">
          {voteCount >= 0 ? `+${voteCount}` : voteCount}
        </span>
      )}
    </>
  )

  if (href) {
    return (
      <Link href={href} className={pillClasses}>
        {content}
      </Link>
    )
  }

  return (
    <button type="button" onClick={onClick} className={pillClasses}>
      {content}
    </button>
  )
}
