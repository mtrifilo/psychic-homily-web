import type { Metadata, Viewport } from 'next'
import { Golos_Text, Hanken_Grotesk, Schibsted_Grotesk } from 'next/font/google'
import { Suspense } from 'react'
import './globals.css'

/* The theme is `@theme inline`, so `--font-sans` is not a runtime custom property
 * and Tailwind's preflight default (`--default-font-family`) never resolves to it.
 * The body face therefore has to be applied as the `font-sans` utility on <body>;
 * without it the page's default text renders in the system sans.
 *
 * Three-tier editorial type system: Hanken Grotesk for display and headings,
 * Schibsted Grotesk for body and UI, Golos Text for the data tier (counts, times,
 * prices, dates). Variable fonts, so no weight list; next/font self-hosts them at
 * build time. Golos Text ships no italic. Schibsted Grotesk and Golos Text have no
 * Vietnamese glyphs, so only Hanken requests that subset. */
const hankenGrotesk = Hanken_Grotesk({
  subsets: ['latin', 'latin-ext', 'vietnamese'],
  variable: '--font-hanken',
  display: 'swap',
})

const schibstedGrotesk = Schibsted_Grotesk({
  subsets: ['latin', 'latin-ext'],
  variable: '--font-schibsted',
  display: 'swap',
})

const golosText = Golos_Text({
  subsets: ['latin', 'latin-ext'],
  variable: '--font-golos',
  display: 'swap',
})
import { ThemeProvider } from '@/components/layout/theme-provider'
import { Providers } from '@/components/layout/providers'
import Footer from '@/components/layout/Footer'
import { CookieConsentBanner } from '@/components/layout/CookieConsentBanner'
import { PostHogProvider } from '@/components/layout/PostHogProvider'
import { PostHogIdentify } from '@/components/layout/PostHogIdentify'
import { AppShell } from '@/components/layout/AppShell'
import { AuthHydrator } from '@/components/layout/AuthHydrator'
import { ClickReplayScript } from '@/components/layout/ClickReplayScript'
import { CookieConsentProvider } from '@/lib/context/CookieConsentContext'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateOrganizationSchema } from '@/lib/seo/jsonld'
import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import { appViewport } from '@/lib/seo/viewport'
import InternalTrafficAnalytics from '@/components/analytics/InternalTrafficAnalytics'
import { SpeedInsights } from '@vercel/speed-insights/next'

// Defined in lib/seo/viewport.ts so it is unit-testable — see the comment there
// for why viewport-fit=cover is load-bearing.
export const viewport: Viewport = appViewport

export const metadata: Metadata = {
  metadataBase: new URL('https://psychichomily.com'),
  title: {
    default: 'Psychic Homily',
    template: '%s | Psychic Homily',
  },
  description: SITE_DESCRIPTION,
  openGraph: {
    type: 'website',
    locale: 'en_US',
    siteName: 'Psychic Homily',
    images: [{ url: '/og-image.jpg', width: 1200, height: 630, alt: 'Psychic Homily' }],
  },
  twitter: {
    card: 'summary_large_image',
    images: ['/og-image.jpg'],
  },
  robots: {
    index: true,
    follow: true,
    'max-snippet': -1,
    'max-image-preview': 'large' as const,
    'max-video-preview': -1,
  },
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* First script in the document: it must already be listening during
            the window where controls are painted but not yet interactive. */}
        <ClickReplayScript />
        <link rel="preconnect" href="https://api.psychichomily.com" crossOrigin="anonymous" />
        <link rel="dns-prefetch" href="//api.psychichomily.com" />
        <link rel="dns-prefetch" href="//open.spotify.com" />
        <link rel="dns-prefetch" href="//bandcamp.com" />
        <JsonLd data={generateOrganizationSchema()} />
      </head>
      <body
        className={`${hankenGrotesk.variable} ${schibstedGrotesk.variable} ${golosText.variable} font-sans antialiased`}
      >
        <Providers>
          <ThemeProvider
            attribute="class"
            defaultTheme="system"
            enableSystem
            disableTransitionOnChange
          >
            <CookieConsentProvider>
              <PostHogProvider>
                {/* AuthHydrator seeds the profile cache AND mounts the
                    session-scoped providers inside its HydrationBoundary, so
                    the server render sees the real signed-in / signed-out tree
                    instead of a loading shell. PostHogProvider stays outside it
                    on purpose: consent (and the banner below) must not wait on
                    the profile prefetch. */}
                <Suspense fallback={null}>
                  <AuthHydrator>
                    <PostHogIdentify />
                    <AppShell>
                      <main id="main-content" className="flex-1">{children}</main>
                      <Footer />
                    </AppShell>
                  </AuthHydrator>
                </Suspense>
                <CookieConsentBanner />
                <InternalTrafficAnalytics />
                <SpeedInsights />
              </PostHogProvider>
            </CookieConsentProvider>
          </ThemeProvider>
        </Providers>
      </body>
    </html>
  )
}
