'use client'

import { forwardRef, useCallback } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { outboundRel } from '@/lib/outboundRel'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'

const NEW_TAB_SUFFIX = '(opens in a new tab)'

/**
 * A trailing new-tab parenthetical that a caller already wrote by hand.
 *
 * Deliberately NOT a bare substring test. Two independent constraints:
 *
 *  - It must anchor at the END. The name it inspects can carry operator-entered
 *    text (a venue or station name), so a mid-string match would let stored
 *    content suppress the announcement for the whole control. The anchor
 *    NARROWS that window rather than closing it: a call site that interpolates
 *    operator text last could still, in principle, produce a name ending in
 *    this shape. Accepted because the alternative (no tolerance at all) trades
 *    a far-fetched suppression for a routine stutter.
 *  - It must tolerate phrasing variants. The one hand-written announcement this
 *    codebase actually had read "(opens Google Maps in a new tab)", which a
 *    literal check for the canonical string would miss, and "window" is the
 *    other spelling people reach for.
 */
const HAND_WRITTEN_NEW_TAB = /\(opens[^)]*\bnew\s+(?:tab|window)\)$/i

/**
 * Appends the outbound announcement to an accessible name, idempotently.
 *
 * The idempotence is a safety net, not the contract: callers are told not to
 * write this phrase (see `external`), because a hand-written copy is what
 * drifted from the `target` it described in the first place. But the habit is
 * the obvious thing to reach for, and a doubled announcement reads as a
 * stutter to a screen reader while being invisible in a visual diff. Absorbing
 * the duplicate keeps a bad habit from having a bad OUTCOME.
 *
 * Callers resolve the base name first (see `resolveAccessibleName`), so this
 * never has to guess what an empty string meant.
 */
function withNewTabSuffix(name: string): string {
  if (HAND_WRITTEN_NEW_TAB.test(name)) return name
  return `${name} ${NEW_TAB_SUFFIX}`
}

/**
 * The accessible name before any outbound suffix: an explicit `ariaLabel`,
 * else the visible label. A blank/whitespace `ariaLabel` is treated as absent
 * rather than honored — an empty accessible name is never what a caller
 * building one by interpolation (`${room.name} website`) intended, and an
 * unnamed control is worse than a redundant one.
 */
function resolveAccessibleName(
  ariaLabel: string | undefined,
  label: string
): string {
  return ariaLabel?.trim() || label
}

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
   * `target="_blank"` and the outbound `rel` instead of a Next `<Link>`, and
   * appends "(opens in a new tab)" to the accessible name. The `rel` is the
   * hygiene tokens, plus `sponsored` when that prop is set — composed by
   * `lib/outboundRel`, never by a call site.
   *
   * The visible outbound marker is the CALLER's choice, not this component's:
   * prose-like brackets carry a "↗" ("Directions ↗", "site ↗"), while dense
   * tabular ones deliberately do not ("mp3" in an archive column, "listen"
   * down the dial), because a glyph per row is noise. Put it in `label` when
   * you want it, so it stays visual and out of the announced name.
   *
   * The new-tab announcement is NOT optional and NOT the caller's: do not
   * hand-write it into `ariaLabel`, or the claim and the `target` it describes
   * can drift apart again. A caller string that already ends in such a note is
   * used as-is rather than doubled.
   *
   * Consequence for name-based test locators: this suffix, plus the
   * entity-naming `ariaLabel` below, makes an external bracket's accessible
   * name a SUPERSTRING of the entity it names. Playwright matches
   * `getByRole(..., { name })` by substring unless told otherwise, so an
   * outbound bracket rendered beside a link named after the same entity makes
   * that entity-name locator ambiguous and fails the spec on strict mode.
   * Neither half is negotiable here, so the repair belongs in the spec: when
   * you add an `external` bracket next to a same-named link, check
   * `frontend/e2e/` for a name-matched locator needing `exact: true` or a
   * container scope. The "appends rather than replaces" test in
   * `BracketLink.test.tsx` pins the property.
   *
   * Only `http(s)` hrefs are honored — anything else renders the disabled
   * fallback instead of a live anchor (see the scheme floor below). Marking a
   * RELATIVE href `external` therefore yields a dead control, not a link.
   *
   * Ignored without `href`.
   */
  external?: boolean
  /**
   * This outbound link is monetized, so `rel` gains `sponsored` alongside the
   * hygiene tokens. Google's link-spam policy requires paid links to be
   * qualified, and an unqualified affiliate link is the site's own ranking at
   * risk, not the vendor's.
   *
   * Derive it, never assert it: the ticket call site passes what `ticketLink`
   * in `lib/tickets/ticketVendors` reports, which is the only thing that knows
   * whether a partner ID was actually attached. A hand-set `sponsored` on a
   * link carrying no tag is a claim about money that is simply false.
   * Ignored without `external`, since the internal `<Link>` branch points
   * inside this site.
   */
  sponsored?: boolean
  /** Visual variant. `danger` is red for destructive actions like [Remove] / [Delete] / [X]. */
  variant?: 'default' | 'danger'
  /**
   * ARIA label override (defaults to the visible label). Use it for CONTEXT the
   * visible label leaves out — the room name behind a bare `[site ↗]` repeated
   * down a list, the date behind a column of `[mp3]`. Without it, every row in
   * such a list announces the same name.
   *
   * Never write the outbound announcement here; `external` appends it.
   */
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
 *   <BracketLink label="site ↗" href={room.website} external ariaLabel={`${room.name} website`} />
 *
 * That last line is the whole outbound split: the optional glyph rides in
 * `label` (visual only), the disambiguating context rides in `ariaLabel`, and
 * "(opens in a new tab)" is appended by this component — callers never write
 * it. Dense tabular call sites keep the context and skip the glyph.
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
      sponsored = false,
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
    //
    // Trimmed FIRST, and the trimmed value is what gets rendered. Stored URLs
    // in this app are operator- and contributor-entered paste, and the write
    // path validates a trimmed copy while persisting the raw string, so
    // "  https://x" reaches here intact. A browser strips that whitespace when
    // parsing an href, so an untrimmed test would reject a URL the platform
    // treats as perfectly valid and hand the reader a dead grey bracket where
    // a working link used to be.
    //
    // The presence test stays on the RAW href on purpose. Testing the trimmed
    // copy would let a whitespace-only href ("   ") slip through as "no unsafe
    // href here" while the render branch below still saw a truthy string,
    // producing an enabled <a href=""> that reopens the current page in a new
    // tab while announcing that it opens something else. Raw for "was anything
    // supplied", trimmed for "is what was supplied usable".
    // Resolved ONCE, above every render branch. The suffix is a rule about the
    // name, so a branch added later inherits both halves instead of silently
    // picking up the naming rule and dropping the announcement.
    const accessibleName = resolveAccessibleName(ariaLabel, label)

    const trimmedHref = href?.trim()
    const unsafeExternalHref =
      external && !!href && !/^https?:\/\//i.test(trimmedHref ?? '')
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
      // The outbound announcement is COMPONENT-owned: one owner, no drift.
      // Appending it HERE, next to the literal target="_blank" below, is what
      // keeps the claim and the behavior it describes from being edited apart.
      //
      // Deliberately NOT applied to the disabled-button fallback further down:
      // that branch opens nothing, so announcing a new tab would be the same
      // class of lie this exists to prevent.
      const anchorProps = {
        onClick: onClick as React.MouseEventHandler | undefined,
        className: classes,
        title,
        'aria-label': external
          ? withNewTabSuffix(accessibleName)
          : accessibleName,
      }
      if (external) {
        return (
          <a
            {...anchorProps}
            href={trimmedHref}
            target="_blank"
            rel={outboundRel(sponsored)}
          >
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
        aria-label={accessibleName}
        aria-pressed={active || undefined}
        {...rest}
      >
        {content}
      </button>
    )
  }
)
