import Link from 'next/link'
import { getAllMixes } from '@/lib/mixes'
import { SoundCloud } from '@/components/blog/soundcloud-embed'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { formatContentDate } from '@/lib/utils/formatters'

export const metadata = {
  title: 'DJ Sets',
  description: 'Featured DJ mixes from Arizona DJs and beyond.',
  alternates: {
    canonical: 'https://psychichomily.com/dj-sets',
  },
  openGraph: {
    title: 'DJ Sets | Psychic Homily',
    description: 'Featured DJ mixes from Arizona DJs and beyond.',
    url: '/dj-sets',
    type: 'website',
  },
}

export default function DJSetsPage() {
  const mixes = getAllMixes()

  return (
    <>
      {mixes.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'DJ Sets',
          description: 'Featured DJ mixes from Arizona DJs and beyond.',
          listItems: mixes.map(mix => ({
            url: `https://psychichomily.com/dj-sets/${mix.slug}`,
            name: mix.title,
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'DJ Sets', url: 'https://psychichomily.com/dj-sets' },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-3xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">DJ Sets</h1>

          <section className="w-full">
          {mixes.map(mix => (
            <article
              key={mix.slug}
              className="border-b border-border pb-6 mt-6 first:mt-0"
            >
              <h2 className="text-xl font-semibold leading-tight">
                <Link
                  href={`/dj-sets/${mix.slug}`}
                  className="hover:text-muted-foreground transition-colors"
                >
                  {mix.title}
                </Link>
              </h2>

              <div className="text-sm text-muted-foreground mt-1">
                {formatContentDate(mix.date)} by {mix.artist}
              </div>

              {mix.description && (
                <div className="mt-3 leading-relaxed text-foreground/90">
                  {mix.description}
                </div>
              )}

              <div className="mt-4">
                <SoundCloud
                  url={mix.soundcloud_url}
                  title={mix.title}
                  artist={mix.artist}
                  artist_url={mix.artist_url}
                  track_url={mix.track_url}
                />
              </div>
            </article>
          ))}

          {mixes.length === 0 && (
            <p className="text-center text-muted-foreground py-12">
              No DJ sets yet.
            </p>
          )}
        </section>
        </main>
      </div>
    </>
  )
}
