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
import { cn } from '@/lib/utils'

/**
 * What this browser can actually do, resolved on the client only.
 *
 * `unknown` is the server/hydration snapshot and renders nothing — capability
 * is not knowable during SSR, and a control that might turn out to be dead must
 * not ship in server HTML.
 */
type ShareCapability = 'unknown' | 'share' | 'copy' | 'none'

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
 */
export function buildShareUrl(path: string): string {
  const canonical = new URL(SITE_URL)
  const resolved = new URL(path, canonical)
  canonical.pathname = resolved.pathname
  return canonical.toString()
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
 * and the caller renders nothing in that case rather than a dead button.
 */
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
const getServerCapability = (): ShareCapability => 'unknown'

export interface ShareButtonProps {
  /**
   * Canonical page path with a leading slash, e.g. `/shows/some-show`.
   * Any query string or fragment is stripped — see {@link buildShareUrl}.
   */
  path: string
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
 * Note on hydration: because the control mounts only after capability is
 * resolved, it never exists in server HTML and therefore has no pre-hydration
 * click window to replay (`lib/hydration/clickReplay.ts`). That is a
 * requirement here, not just a happy accident — `navigator.share` needs
 * transient user activation, which a replayed untrusted click does not carry,
 * so a replayed share click would reject where a real one succeeds.
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

  const handleShare = useCallback(async () => {
    const url = buildShareUrl(path)

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

    if (capability === 'none') return
    await copyToClipboard(url)
  }, [capability, path, copyToClipboard])

  // Nothing renders until capability is known, and nothing renders at all when
  // neither mechanism exists. This is the "never a dead button" guarantee.
  if (capability === 'unknown' || capability === 'none') return null

  const label =
    feedback === 'copied'
      ? 'Copied'
      : feedback === 'failed'
        ? 'Copy failed'
        : 'Share'

  if (variant === 'bracket') {
    return (
      <BracketLink
        label={label}
        onClick={handleShare}
        active={feedback === 'copied'}
        variant={feedback === 'failed' ? 'danger' : 'default'}
        ariaLabel={ariaLabel ?? 'Share'}
        className={className}
      />
    )
  }

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={handleShare}
      aria-label={ariaLabel ?? 'Share'}
      className={cn('gap-1.5', className)}
    >
      {feedback === 'copied' ? (
        <Check className="h-4 w-4" />
      ) : (
        <Share2 className="h-4 w-4" />
      )}
      <span
        // Announced on change so the copy confirmation is not visual-only.
        aria-live="polite"
        className={cn(feedback === 'failed' && 'text-destructive')}
      >
        {label}
      </span>
    </Button>
  )
}
