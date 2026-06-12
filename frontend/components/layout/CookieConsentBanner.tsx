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

interface ConsentBarProps {
  gpcSignalDetected: boolean
  acceptAll: () => void
  rejectAll: () => void
  openPreferences: () => void
}

function ConsentBar({
  gpcSignalDetected,
  acceptAll,
  rejectAll,
  openPreferences,
}: ConsentBarProps) {
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
    <div
      ref={barRef}
      className="fixed bottom-0 left-0 right-0 z-50 border-t bg-background px-4 py-2.5 motion-safe:animate-in motion-safe:slide-in-from-bottom motion-safe:duration-300"
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
    acceptAll,
    rejectAll,
    openPreferences,
    preferencesOpen,
    closePreferences,
    savePreferences,
    consent,
  } = useCookieConsent()

  return (
    <>
      {showBanner && (
        <ConsentBar
          gpcSignalDetected={gpcSignalDetected}
          acceptAll={acceptAll}
          rejectAll={rejectAll}
          openPreferences={openPreferences}
        />
      )}

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
