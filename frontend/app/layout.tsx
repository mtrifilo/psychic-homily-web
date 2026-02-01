import type { Metadata } from 'next'
import { GeistSans } from 'geist/font/sans'
import { GeistMono } from 'geist/font/mono'
import './globals.css'
import { ThemeProvider, Providers, Footer } from '@/components/layout'
import Nav from '@/app/nav'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateOrganizationSchema } from '@/lib/seo/jsonld'

export const metadata: Metadata = {
  metadataBase: new URL('https://psychichomily.com'),
  title: {
    default: 'Psychic Homily | Arizona Music Community',
    template: '%s | Psychic Homily',
  },
  description: 'Discover upcoming live music shows, blog posts, and DJ sets from the Arizona music scene.',
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
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <JsonLd data={generateOrganizationSchema()} />
      </head>
      <body
        className={`${GeistSans.variable} ${GeistMono.variable} antialiased`}
      >
        <Providers>
          <ThemeProvider
            attribute="class"
            defaultTheme="system"
            enableSystem
            disableTransitionOnChange
          >
            <div className="flex flex-col min-h-screen">
              <Nav />
              <main className="flex-1">{children}</main>
              <Footer />
            </div>
          </ThemeProvider>
        </Providers>
      </body>
    </html>
  )
}
