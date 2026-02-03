'use client'

import Link from 'next/link'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'

export default function Footer() {
  const { openPreferences } = useCookieConsent()
  const currentYear = new Date().getFullYear()

  return (
    <footer className="w-full border-t border-border/30 mt-auto">
      <div className="max-w-7xl mx-auto px-4 py-6">
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 text-sm text-muted-foreground">
          <p>&copy; {currentYear} Psychic Homily</p>
          <nav className="flex items-center gap-4">
            <Link
              href="/privacy"
              className="hover:text-foreground transition-colors"
            >
              Privacy Policy
            </Link>
            <Link
              href="/terms"
              className="hover:text-foreground transition-colors"
            >
              Terms of Service
            </Link>
            <Link
              href="mailto:hello@psychichomily.com"
              className="hover:text-foreground transition-colors"
            >
              Contact
            </Link>
            <button
              type="button"
              onClick={openPreferences}
              className="hover:text-foreground transition-colors"
            >
              Cookie Preferences
            </button>
          </nav>
        </div>
      </div>
    </footer>
  )
}
