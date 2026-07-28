import type { Metadata, Viewport } from 'next'
import localFont from 'next/font/local'
import { Space_Mono } from 'next/font/google'
import { Suspense } from 'react'
import './globals.css'

/* PSY-647 editorial type system — Fontshare (ITF License, self-hosted) + Google Fonts.
 * Clash Display: display / headings (4 weights). Satoshi: body / UI (3 weights —
 * Fontshare's free Satoshi has no 600 weight; use Medium 500 where Semibold would
 * apply). Space Mono: data / metadata / numerics (Google Fonts). */
const clashDisplay = localFont({
  src: [
    { path: './fonts/ClashDisplay-Regular.woff2', weight: '400', style: 'normal' },
    { path: './fonts/ClashDisplay-Medium.woff2', weight: '500', style: 'normal' },
    { path: './fonts/ClashDisplay-Semibold.woff2', weight: '600', style: 'normal' },
    { path: './fonts/ClashDisplay-Bold.woff2', weight: '700', style: 'normal' },
  ],
  variable: '--font-clash-display',
  display: 'swap',
})

const satoshi = localFont({
  src: [
    { path: './fonts/Satoshi-Regular.woff2', weight: '400', style: 'normal' },
    { path: './fonts/Satoshi-Medium.woff2', weight: '500', style: 'normal' },
    { path: './fonts/Satoshi-Bold.woff2', weight: '700', style: 'normal' },
  ],
  variable: '--font-satoshi',
  display: 'swap',
})

const spaceMono = Space_Mono({
  weight: ['400', '700'],
  subsets: ['latin'],
  variable: '--font-space-mono',
  display: 'swap',
})
import {
  ThemeProvider,
  Providers,
  Footer,
  CookieConsentBanner,
  PostHogProvider,
  PostHogIdentify,
  AppShell,
  AuthHydrator,
} from '@/components/layout'
import { CookieConsentProvider } from '@/lib/context/CookieConsentContext'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateOrganizationSchema } from '@/lib/seo/jsonld'
import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import InternalTrafficAnalytics from '@/components/analytics/InternalTrafficAnalytics'
import { SpeedInsights } from '@vercel/speed-insights/next'

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: 'white' },
    { media: '(prefers-color-scheme: dark)', color: 'black' },
  ],
}

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
        <link rel="preconnect" href="https://api.psychichomily.com" crossOrigin="anonymous" />
        <link rel="dns-prefetch" href="//api.psychichomily.com" />
        <link rel="dns-prefetch" href="//open.spotify.com" />
        <link rel="dns-prefetch" href="//bandcamp.com" />
        <JsonLd data={generateOrganizationSchema()} />
      </head>
      <body
        className={`${clashDisplay.variable} ${satoshi.variable} ${spaceMono.variable} antialiased`}
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
