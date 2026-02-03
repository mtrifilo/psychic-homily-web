'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import { CookiePreferencesDialog } from './CookiePreferencesDialog'

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

  if (!showBanner) {
    return (
      <CookiePreferencesDialog
        open={preferencesOpen}
        onOpenChange={(open) => !open && closePreferences()}
        gpcSignalDetected={gpcSignalDetected}
        currentAnalytics={consent?.categories.analytics ?? false}
        onSave={savePreferences}
      />
    )
  }

  return (
    <>
      <div
        className="fixed bottom-0 left-0 right-0 z-50 border-t bg-background p-4 shadow-lg motion-safe:animate-in motion-safe:slide-in-from-bottom motion-safe:duration-300"
        role="dialog"
        aria-label="Cookie consent"
        aria-describedby="cookie-consent-description"
      >
        <div className="mx-auto max-w-4xl">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex-1">
              <p className="text-sm font-medium">Cookie Preferences</p>
              <p
                id="cookie-consent-description"
                className="mt-1 text-sm text-muted-foreground"
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
                <p className="mt-1 text-xs text-muted-foreground">
                  We detected a Global Privacy Control signal from your browser.
                </p>
              )}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Button
                variant="outline"
                onClick={rejectAll}
                className="w-full sm:w-auto"
              >
                Reject All
              </Button>
              <Button
                variant="outline"
                onClick={openPreferences}
                className="w-full sm:w-auto"
              >
                Customize
              </Button>
              <Button onClick={acceptAll} className="w-full sm:w-auto">
                Accept All
              </Button>
            </div>
          </div>
        </div>
      </div>

      <CookiePreferencesDialog
        open={preferencesOpen}
        onOpenChange={(open) => !open && closePreferences()}
        gpcSignalDetected={gpcSignalDetected}
        currentAnalytics={false}
        onSave={savePreferences}
      />
    </>
  )
}
