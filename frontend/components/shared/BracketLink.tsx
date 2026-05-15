'use client'

import { forwardRef } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'

export interface BracketLinkProps
  extends Omit<
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    'onClick' | 'type'
  > {
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
  /** ARIA label override (defaults to the visible label). */
  ariaLabel?: string
}

/**
 * Gazelle-style bracketed text link. Replaces icon-buttons in dense entity-page chrome:
 * the brackets ARE the affordance, not a hover state. Renders as <Link> when `href` is
 * provided, otherwise as <button type="button">.
 *
 * The button branch forwards a ref and spreads remaining props, so it composes as a
 * Radix `asChild` trigger (e.g. inside `<PopoverTrigger asChild>`). The <Link> branch
 * is plain navigation only — it does not receive the ref or spread props.
 *
 * Note: when `href` AND `disabled` are both set, renders as a `<button disabled>` rather
 * than an `<a>` — anchors have no native disabled state, and the alternatives leak
 * click-to-navigate to keyboard/AT-bypassing consumers.
 *
 * Usage:
 *   <BracketLink label="Follow" onClick={handleFollow} />
 *   <BracketLink label="Following" active onClick={handleUnfollow} />
 *   <BracketLink label="View history" href={`/artists/${slug}/history`} />
 *   <BracketLink label="X" variant="danger" onClick={handleRemove} title="Remove" />
 */
export const BracketLink = forwardRef<HTMLButtonElement, BracketLinkProps>(
  function BracketLink(
    {
      label,
      href,
      onClick,
      active = false,
      variant = 'default',
      disabled = false,
      title,
      ariaLabel,
      className,
      ...rest
    },
    ref
  ) {
    const classes = cn(
      'inline-flex items-baseline whitespace-nowrap text-sm tabular-nums',
      'transition-colors',
      variant === 'default' &&
        !active &&
        'text-muted-foreground hover:text-foreground',
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
        ref={ref}
        type="button"
        onClick={onClick}
        disabled={disabled}
        className={classes}
        title={title}
        aria-label={ariaLabel ?? label}
        aria-pressed={active || undefined}
        {...rest}
      >
        {content}
      </button>
    )
  }
)
