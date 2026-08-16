'use client'

import { useEffect, useRef } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import { CookiePreferencesDialog } from './CookiePreferencesDialog'

/* PSY-1029: the consent banner is a slim fixed bottom bar (one compact row of
 * copy + small buttons) instead of the old two-deck layout, so the first
 * content row of the logged-out homepage stays fully visible on first paint.
 * While the bar is visible it mirrors its height into `body` bottom padding,
 * so content at the document end (the footer) stays reachable by scrolling
 * rather than being permanently covered until the visitor consents. */

function ConsentBar() {
  const { gpcSignalDetected, acceptAll, rejectAll, openPreferences } =
    useCookieConsent()
  const barRef = useRef<HTMLDivElement>(null)

  // Reserve scroll space matching the bar's rendered height (it varies with
  // viewport width / text wrapping), and release it when the bar unmounts.
  useEffect(() => {
    const bar = barRef.current
    if (!bar) return

    const reserveSpace = () => {
      document.body.style.paddingBottom = `${bar.offsetHeight}px`
    }
    reserveSpace()
    const observer = new ResizeObserver(reserveSpace)
    observer.observe(bar)

    return () => {
      observer.disconnect()
      document.body.style.paddingBottom = ''
    }
  }, [])

  return (
    // Below `xl` the banner sits ABOVE the BottomTabBar's slot (PSY-1020): both
    // are fixed to the viewport bottom and the banner is z-50 over the bar's
    // z-40, so bottom-0 would fully occlude the primary mobile nav for every
    // pre-consent visitor. The bar's side of this contract is the comment in
    // BottomTabBar.tsx. At `xl` the bar is gone and the banner returns to the
    // true bottom.
    //
    // Safe-area handling (PSY-1820 turned the insets on via viewport-fit=cover).
    // Every inset is absorbed as PADDING, never as an offset, so the banner's
    // background keeps bleeding to the screen edges instead of leaving a strip
    // of page content showing through beside or beneath it:
    //   • left/right — the px-4 gutters grow by the landscape notch inset, so
    //     the copy and buttons stay clear of the notch and rounded corners.
    //   • bottom — only at `xl`, where the banner owns the true bottom edge and
    //     nothing else clears the home indicator for it. Below `xl` the bar
    //     underneath already absorbs that inset, so adding it here would
    //     double-count it.
    // offsetHeight (mirrored into body padding above) grows with this padding,
    // so the scroll reservation stays correct on every device.
    <div
      ref={barRef}
      className="fixed bottom-[calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))] left-0 right-0 z-50 border-t bg-background py-2.5 pl-[calc(1rem+env(safe-area-inset-left))] pr-[calc(1rem+env(safe-area-inset-right))] motion-safe:animate-in motion-safe:slide-in-from-bottom motion-safe:duration-300 xl:bottom-0 xl:pb-[calc(0.625rem+env(safe-area-inset-bottom))]"
      role="dialog"
      aria-label="Cookie consent"
      aria-describedby="cookie-consent-description"
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-6 gap-y-2">
        <div className="min-w-0">
          <p
            id="cookie-consent-description"
            className="text-sm text-muted-foreground"
          >
            We use cookies to improve your experience.{' '}
            <Link
              href="/privacy"
              className="underline underline-offset-4 hover:text-foreground"
            >
              Learn more
            </Link>
          </p>
          {gpcSignalDetected && (
            <p className="mt-0.5 text-xs text-muted-foreground">
              We detected a Global Privacy Control signal from your browser.
            </p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={rejectAll}>
            Reject All
          </Button>
          <Button size="sm" variant="outline" onClick={openPreferences}>
            Customize
          </Button>
          <Button size="sm" onClick={acceptAll}>
            Accept All
          </Button>
        </div>
      </div>
    </div>
  )
}

export function CookieConsentBanner() {
  const {
    showBanner,
    gpcSignalDetected,
    preferencesOpen,
    closePreferences,
    savePreferences,
    consent,
  } = useCookieConsent()

  return (
    <>
      {showBanner && <ConsentBar />}

      <CookiePreferencesDialog
        open={preferencesOpen}
        onOpenChange={(open) => !open && closePreferences()}
        gpcSignalDetected={gpcSignalDetected}
        currentAnalytics={consent?.categories.analytics ?? false}
        onSave={savePreferences}
      />
    </>
  )
}
