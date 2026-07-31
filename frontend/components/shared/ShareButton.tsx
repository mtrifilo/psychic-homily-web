'use client'

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { Check, Share2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { cn } from '@/lib/utils'

/**
 * What this browser can actually do.
 *
 * `share` and `copy` render an IDENTICAL control — same label, same icon, same
 * width. Only the click behaviour differs. That equivalence is what lets the
 * server render the button without knowing which one it will turn out to be,
 * and it is load-bearing: see {@link getServerCapability}.
 */
type ShareCapability = 'share' | 'copy' | 'none'

/** Inline confirmation state; mirrors the copy-link convention already in use. */
type FeedbackState = 'idle' | 'copied' | 'failed'

/** How long the inline confirmation stays up before reverting to `Share`. */
const FEEDBACK_RESET_MS = 2000

/**
 * Build the canonical, shareable URL for a page path.
 *
 * Two properties are load-bearing and are enforced structurally rather than by
 * convention, because both failure modes are silent:
 *
 * 1. **The origin is always the canonical apex.** A caller cannot redirect the
 *    share, and — more routinely — a share taken from a preview/stage
 *    deployment still hands out a URL that works for the recipient. Resolving
 *    the caller's path against the base and then re-assigning only `pathname`
 *    means an absolute URL or a protocol-relative `//host` cannot escape.
 *
 * 2. **Query and fragment are dropped.** This is the mechanical half of the
 *    "user shares are never campaign-tagged" rule (PSY-1587): a visitor who
 *    arrived on `?utm_source=bluesky&utm_campaign=...` would otherwise re-share
 *    those tags, attributing their friends' organic word-of-mouth to whichever
 *    post *they* came from. Tags survive copy-paste, so one leaked share seeds
 *    the whole onward chain. `lib/analytics/campaignUrl.ts` is for links WE
 *    post; this path deliberately has no way to reach it.
 *
 * Sharing `window.location.href` would violate both at once, which is why it is
 * never read here.
 *
 * Returns `null` for anything that cannot become a real entity URL, and the
 * component then renders no control at all. Two cases reach that, and both are
 * silent without it:
 *
 * - `new URL()` genuinely throws on some inputs (`'http://'`, `'//'`). This is
 *   called from an async `onClick`, so a throw becomes an unhandled promise
 *   rejection — noise in Sentry, and no feedback for the user.
 * - A path that resolves to bare `/` means the caller interpolated something
 *   empty. Handing out the homepage in place of the show someone meant to send
 *   is worse than offering no share at all.
 *
 * Note what this deliberately does NOT catch: `/shows/null`, from interpolating
 * a slug that was null at runtime. It is indistinguishable from a real slug, so
 * callers must not interpolate a value that may be missing — pass `null` for
 * `path` instead. `catalog.Show.Slug` and `catalog.Artist.Slug` are `*string`
 * server-side even though the frontend types declare them required.
 */
export function buildShareUrl(path: string): string | null {
  const canonical = new URL(SITE_URL)
  let resolved: URL
  try {
    resolved = new URL(path, canonical)
  } catch {
    return null
  }
  if (resolved.pathname === '/') return null
  canonical.pathname = resolved.pathname
  return canonical.toString()
}

/**
 * Did the user simply dismiss the share sheet?
 *
 * Deliberately duck-typed on `name` rather than `instanceof Error`: browsers
 * reject with a `DOMException`, and `DOMException extends Error` is a
 * relatively recent WebIDL guarantee that older engines do not honour. An
 * `instanceof` check that returns false in one browser would quietly turn every
 * cancelled share into a red "Copy failed", which is precisely the misreport
 * this function exists to prevent.
 */
function isAbortError(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as { name?: unknown }).name === 'AbortError'
  )
}

/**
 * Feature-detect the best available share mechanism.
 *
 * Presence of the API IS the secure-context check: browsers expose neither
 * `navigator.share` nor `navigator.clipboard` outside a secure context, so an
 * explicit `window.isSecureContext` test would add a second source of truth
 * that can only ever disagree with the first.
 *
 * `none` is a real outcome, not a theoretical one — an insecure-origin page
 * (a phone hitting a dev server over plain `http://192.168.x.x`) reaches it —
 * and the component renders nothing in that case rather than a dead button.
 */
function detectShareCapability(): ShareCapability {
  if (typeof navigator === 'undefined') return 'none'
  if (typeof navigator.share === 'function') return 'share'
  if (typeof navigator.clipboard?.writeText === 'function') return 'copy'
  return 'none'
}

/**
 * Capability is read through `useSyncExternalStore` rather than an effect.
 *
 * It is the sanctioned way to hold a value that legitimately differs between
 * server and client: React renders the server snapshot during hydration and
 * swaps to the client snapshot afterwards, so there is no mismatch to warn
 * about and no `setState` inside an effect (which the react-compiler lint
 * correctly rejects as a cascading render).
 *
 * The store never changes after load — browser capabilities do not — so
 * `subscribe` is a no-op. Both snapshot functions must be module-level and
 * return primitives, or `useSyncExternalStore` re-renders forever.
 */
const subscribeToNothing = () => () => {}

/**
 * The server assumes `copy`, and that assumption is why this control ships in
 * server HTML instead of appearing after hydration.
 *
 * An earlier revision returned an `unknown` snapshot and rendered nothing until
 * the client had measured itself. That was wrong, and measurably so. Both host
 * rows — `ShowHeader`'s `sm:items-end` cluster and `EntityHeader`'s
 * `sm:justify-between` row — are RIGHT-ALIGNED and shrink-to-fit, so a control
 * appearing at hydration widens the row and drags every existing sibling
 * leftward. Measured on the show page at 1280px: **the save button moved 91px**.
 * Playwright resolves a click point and then stability-checks the box, so a
 * jump landing in that window turns a save into a miss or a retry — which is
 * exactly the intermittent, CI-only, show-and-artist-page-only E2E failure this
 * replaced. Users got the same jump as layout shift.
 *
 * `copy` is the honest default rather than a guess: every secure context exposes
 * `navigator.clipboard`, so a browser that renders this page at all almost
 * always has it. And because `share` and `copy` render identically, the
 * post-hydration swap between them changes nothing on screen — there is no
 * second layout to shift to.
 *
 * `none` (an insecure origin, e.g. a phone on `http://192.168.x.x`) is the one
 * case that still unmounts. It shifts, but it is the case where the control
 * genuinely must not exist, and no HTTPS visitor reaches it.
 */
const getServerCapability = (): ShareCapability => 'copy'

export interface ShareButtonProps {
  /**
   * Canonical page path with a leading slash, e.g. `/shows/some-show`.
   * Any query string or fragment is stripped — see {@link buildShareUrl}.
   *
   * Pass `null` when the entity has no slug yet rather than interpolating a
   * possibly-missing value: `/shows/${slug}` with a null slug silently becomes
   * the literal path `/shows/null`, which no guard here can tell apart from a
   * real one. No path means no control.
   */
  path: string | null | undefined
  /**
   * Visual style. `bracket` is the dense entity-header linkbox (`[Share]`);
   * `button` is the standard outline Button used in ordinary action rows.
   */
  variant?: 'button' | 'bracket'
  /**
   * Accessible name. Defaults to `Share`. Pass something specific when the
   * bare word is ambiguous out of context, e.g. `Share this show`.
   */
  ariaLabel?: string
  className?: string
}

/**
 * Share affordance for public entity pages.
 *
 * Opens the OS share sheet via the Web Share API where it exists — which is
 * what puts a link directly into Messages / WhatsApp / Signal, the surfaces
 * that render OG cards faithfully. Everywhere else (most desktop browsers, so
 * this is the common path rather than an edge case) it copies the canonical URL
 * and confirms inline.
 *
 * Deliberately shares a bare URL: no title, no composed text. The OG card
 * carries the context, and any prose here would be invented voice attached to
 * someone else's message. It also means nothing entity-derived is ever written
 * into the share payload or the clipboard — only an origin-locked URL built
 * from a path.
 *
 * Note on hydration: this ships in server HTML rather than appearing once the
 * client has measured its own capabilities. Both host rows are right-aligned
 * and shrink-to-fit, so a control that arrives at hydration drags its siblings
 * sideways — 91px on the show page, onto the save button. See
 * {@link getServerCapability}. Being in server HTML means it needs
 * `replayOnHydrate` like any other pre-hydration-clickable control.
 */
export function ShareButton({
  path,
  variant = 'button',
  ariaLabel,
  className,
}: ShareButtonProps) {
  const capability = useSyncExternalStore(
    subscribeToNothing,
    detectShareCapability,
    getServerCapability
  )
  const [feedback, setFeedback] = useState<FeedbackState>('idle')
  // Tracked so a rapid re-click extends the confirmation instead of an earlier
  // timer clipping it short, and so an unmount mid-confirmation cannot land a
  // setState on a dead component.
  const resetTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  // Cleanup only — an unmount mid-confirmation must not land a setState on a
  // dead component.
  useEffect(() => () => clearTimeout(resetTimer.current), [])

  const showFeedback = useCallback((next: Exclude<FeedbackState, 'idle'>) => {
    setFeedback(next)
    clearTimeout(resetTimer.current)
    resetTimer.current = setTimeout(() => setFeedback('idle'), FEEDBACK_RESET_MS)
  }, [])

  const copyToClipboard = useCallback(
    async (url: string) => {
      try {
        await navigator.clipboard.writeText(url)
        showFeedback('copied')
      } catch {
        // Permissions, a non-focused document, or a clipboard the browser
        // refuses to write: say so inline rather than silently doing nothing.
        showFeedback('failed')
      }
    },
    [showFeedback]
  )

  const shareUrl = path ? buildShareUrl(path) : null

  const handleShare = useCallback(async () => {
    if (!shareUrl) return
    const url = shareUrl

    if (capability === 'share') {
      try {
        await navigator.share({ url })
        // The OS sheet is its own confirmation; adding one here would double up.
        return
      } catch (error) {
        // Dismissing the sheet rejects with AbortError. That is the user
        // deciding not to share — a normal outcome, not a failure. It must not
        // render an error state and must not reach Sentry.
        if (isAbortError(error)) return
        // Anything else means the sheet never opened (no share target, a
        // gesture the browser did not honour). Falling through to the clipboard
        // leaves the user with a working way to share rather than a dead end.
      }
    }

    // No `capability === 'none'` guard: that state renders nothing, so this is
    // unreachable from it. If a future change did render it, the try/catch in
    // `copyToClipboard` already covers a missing `navigator.clipboard`.
    await copyToClipboard(url)
  }, [capability, shareUrl, copyToClipboard])

  // The "never a dead button, never the wrong link" guarantee: nothing renders
  // when neither mechanism exists, and nothing renders when there is no real
  // URL to hand out.
  if (capability === 'none') return null
  if (!shareUrl) return null

  const label =
    feedback === 'copied'
      ? 'Copied'
      : feedback === 'failed'
        ? 'Copy failed'
        : 'Share'

  // While confirming, the accessible name must track the visible label or the
  // two disagree (WCAG 2.5.3, "Label in Name") — a screen reader would keep
  // saying "Share this show" on a control now reading "Copy failed".
  const accessibleName = feedback === 'idle' ? (ariaLabel ?? 'Share') : label

  // The confirmation is otherwise visual only. The button variant's label is
  // itself the live region; the bracket variant renders its label through
  // BracketLink, so it needs this separate one to reach parity.
  const liveRegion = (
    <span role="status" aria-live="polite" className="sr-only">
      {feedback === 'idle' ? '' : label}
    </span>
  )

  if (variant === 'bracket') {
    return (
      <>
        <BracketLink
          label={label}
          onClick={handleShare}
          active={feedback === 'copied'}
          variant={feedback === 'failed' ? 'danger' : 'default'}
          ariaLabel={accessibleName}
          className={className}
        />
        {liveRegion}
      </>
    )
  }

  return (
    <Button
      // This ships in server HTML, so it is clickable before React wires it up
      // and a click in that window is otherwise dropped silently. The bracket
      // branch above inherits replay from BracketLink.
      //
      // A replayed click is untrusted and so carries no transient activation:
      // `navigator.share` will reject it with NotAllowedError. That is handled,
      // not ignored — the non-Abort branch in `handleShare` falls through to the
      // clipboard, so an early clicker still gets a copied link rather than
      // nothing.
      {...replayOnHydrate}
      type="button"
      variant="outline"
      size="sm"
      onClick={handleShare}
      aria-label={accessibleName}
      className={cn('gap-1.5', className)}
    >
      {feedback === 'copied' ? (
        <Check className="h-4 w-4" />
      ) : (
        <Share2 className="h-4 w-4" />
      )}
      <span className={cn(feedback === 'failed' && 'text-destructive')}>
        {label}
      </span>
      {liveRegion}
    </Button>
  )
}
