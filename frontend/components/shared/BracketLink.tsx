'use client'

import { forwardRef, useCallback } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'

export interface BracketLinkProps
  extends Omit<
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    'onClick' | 'type'
  > {
  /** Visible label, rendered inside literal [brackets]. e.g. label="Follow" -> "[Follow]" */
  label: string
  /** Navigation target. When provided, renders as <Link>; otherwise as <button type="button">. */
  href?: string
  /**
   * Click handler. Without `href`, the button's onClick; with `href`, runs
   * alongside navigation (e.g. closing a popover before the route change).
   */
  onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void
  /** Active / toggled-on state. Emphasizes the link (e.g. [Following] after a successful follow). */
  active?: boolean
  /**
   * `href` points OUTSIDE the app: renders a plain anchor with
   * `target="_blank" rel="noopener noreferrer"` instead of a Next `<Link>`.
   * Callers put the outbound marker in the label themselves ("Directions ↗")
   * so the glyph stays part of the announced name. Ignored without `href`.
   */
  external?: boolean
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
 * Radix `asChild` trigger (e.g. inside `<PopoverTrigger asChild>`). BOTH anchor
 * branches — the internal <Link> and the `external` plain anchor — receive `onClick`
 * (fired alongside navigation) but NOT the ref or spread props; an external bracket
 * cannot be a Radix trigger and silently drops extra DOM props, same as <Link>.
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
      external = false,
      disabled = false,
      title,
      ariaLabel,
      className,
      ...rest
    },
    ref
  ) {
    // Every bracket control ships in server HTML and is clickable before React
    // wires it up, so replay is owned here rather than re-declared at ~71 call
    // sites — one of which would inevitably forget, silently. The <Link> branch
    // below deliberately does NOT get it: a real anchor already works through
    // the whole window. See lib/hydration/clickReplay.ts.
    //
    // Composed locally rather than via `radix-ui/internal`: that is a private,
    // unversioned entrypoint, and when a Radix upgrade breaks it the tempting
    // repair is `ref={ref}` — which type-checks, renders identically, and
    // silently deletes replay for every bracket control in the app.
    const composedRef = useCallback(
      (node: HTMLButtonElement | null) => {
        replayOnHydrate.ref(node)
        if (typeof ref === 'function') ref(node)
        else if (ref) ref.current = node
      },
      [ref]
    )

    // The external branch only renders http(s): a shared primitive named
    // `external` is exactly where a user-controlled URL will eventually
    // arrive, and the scheme floor belongs in the primitive rather than in
    // every caller's repair logic. An unsafe value renders the DISABLED
    // button fallback (same as href+disabled) — never an enabled dead
    // control, and never the raw href.
    const unsafeExternalHref =
      external && !!href && !/^https?:\/\//i.test(href)
    const effectiveDisabled = disabled || unsafeExternalHref

    const classes = cn(
      'inline-flex items-baseline whitespace-nowrap text-sm tabular-nums',
      // Tailwind's preflight leaves <button> at `cursor: default`, so the
      // button branch reads as plain text on hover without this. Declared
      // before the disabled clause so tailwind-merge lets
      // `cursor-not-allowed` win when disabled.
      'cursor-pointer transition-colors',
      variant === 'default' &&
        !active &&
        'text-muted-foreground hover:text-foreground',
      variant === 'default' && active && 'text-foreground font-medium',
      variant === 'danger' && 'text-destructive hover:text-destructive/80',
      // LOAD-BEARING: `pointer-events-none` must stay. FollowButton's
      // pre-hydration safety argument is that its disabled loading bracket
      // cannot receive a click at all — a styling change here would silently
      // turn that into a live gap. See FollowButton.tsx and
      // lib/hydration/clickReplay.ts.
      effectiveDisabled && 'opacity-50 cursor-not-allowed pointer-events-none',
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

    if (href && !effectiveDisabled) {
      // ONE bag of anchor props for both anchor-shaped branches, so a shared
      // concern added to internal links (an aria fix, a data attribute)
      // cannot silently skip the external ones — both render as an <a> and
      // the omission would be invisible in review. Spread BEFORE the literal
      // target/rel so the bag can never override the hygiene attributes.
      const anchorProps = {
        onClick: onClick as React.MouseEventHandler | undefined,
        className: classes,
        title,
        'aria-label': ariaLabel ?? label,
      }
      if (external) {
        return (
          <a {...anchorProps} href={href} target="_blank" rel="noopener noreferrer">
            {content}
          </a>
        )
      }
      return (
        <Link {...anchorProps} href={href}>
          {content}
        </Link>
      )
    }

    return (
      <button
        // The spread is here for the MARKER ATTRIBUTE, which is the half that
        // makes capture work; do not delete it as redundant. Its `ref` is
        // deliberately overridden below because `composedRef` already calls
        // `replayOnHydrate.ref` alongside the caller's forwarded ref, which
        // Radix `asChild` triggers depend on. Order matters: spread first.
        {...replayOnHydrate}
        ref={composedRef}
        type="button"
        onClick={onClick}
        disabled={effectiveDisabled}
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
