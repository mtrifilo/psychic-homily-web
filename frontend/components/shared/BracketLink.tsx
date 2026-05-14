'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'

export interface BracketLinkProps {
  /** Visible label, rendered inside literal [brackets]. e.g. label="Follow" -> "[Follow]" */
  label: string
  /** Navigation target. When provided, renders as <Link>; otherwise as <button type="button">. */
  href?: string
  /** Click handler (used when href is omitted). */
  onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void
  /** Active / toggled-on state. Emphasizes the link (e.g. [Following] after a successful follow). */
  active?: boolean
  /** Visual variant. `danger` is red for destructive actions like [Remove] / [Delete] / [X]. */
  variant?: 'default' | 'danger'
  /**
   * Disabled state — un-clickable, dimmed.
   *
   * Note: when `href` is also set, a disabled BracketLink renders as a
   * `<button disabled>` rather than an `<a>`. Anchors have no native disabled
   * state, so the alternatives (`aria-disabled` + `tabIndex={-1}` + preventing
   * default click) leak click-to-navigate to consumers that bypass keyboard /
   * AT (right-click → open in new tab still navigates). The button swap is
   * the simplest correct behavior.
   */
  disabled?: boolean
  /** Tooltip (also exposed as title attribute). */
  title?: string
  /** ARIA label override (defaults to the visible label). */
  ariaLabel?: string
  /** Additional CSS classes. */
  className?: string
}

/**
 * Gazelle-style bracketed text link. Replaces icon-buttons in dense entity-page chrome:
 * the brackets ARE the affordance, not a hover state. Renders as <Link> when `href` is
 * provided, otherwise as <button type="button">.
 *
 * Usage:
 *   <BracketLink label="Follow" onClick={handleFollow} />
 *   <BracketLink label="Following" active onClick={handleUnfollow} />
 *   <BracketLink label="View history" href={`/artists/${slug}/history`} />
 *   <BracketLink label="X" variant="danger" onClick={handleRemove} title="Remove" />
 */
export function BracketLink({
  label,
  href,
  onClick,
  active = false,
  variant = 'default',
  disabled = false,
  title,
  ariaLabel,
  className,
}: BracketLinkProps) {
  const classes = cn(
    'inline-flex items-baseline whitespace-nowrap text-sm tabular-nums',
    'transition-colors',
    variant === 'default' && !active && 'text-muted-foreground hover:text-foreground',
    variant === 'default' && active && 'text-foreground font-medium',
    variant === 'danger' && 'text-destructive hover:text-destructive/80',
    disabled && 'opacity-50 cursor-not-allowed pointer-events-none',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 rounded-sm',
    className
  )

  const content = (
    <>
      <span aria-hidden="true">[</span>
      <span>{label}</span>
      <span aria-hidden="true">]</span>
    </>
  )

  if (href && !disabled) {
    return (
      <Link
        href={href}
        className={classes}
        title={title}
        aria-label={ariaLabel ?? label}
      >
        {content}
      </Link>
    )
  }

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={classes}
      title={title}
      aria-label={ariaLabel ?? label}
      aria-pressed={active || undefined}
    >
      {content}
    </button>
  )
}
