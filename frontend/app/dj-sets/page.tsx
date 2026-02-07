import Link from 'next/link'
import { getAllMixes } from '@/lib/mixes'
import { SoundCloud } from '@/components/blog/soundcloud-embed'

export const metadata = {
  title: 'DJ Sets',
  description: 'Featured DJ mixes from Arizona DJs and beyond.',
  openGraph: {
    title: 'DJ Sets | Psychic Homily',
    description: 'Featured DJ mixes from Arizona DJs and beyond.',
    url: '/dj-sets',
    type: 'website',
  },
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default function DJSetsPage() {
  const mixes = getAllMixes()

  return (
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
                {formatDate(mix.date)} by {mix.artist}
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
  )
}
