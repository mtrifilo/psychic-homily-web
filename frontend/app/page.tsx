import Link from 'next/link'
import { HomeShowList } from '@/components/shows'
import { getBlogSlugs, getBlogPost } from '@/lib/blog'
import { getAllMixes } from '@/lib/mixes'
import { MDXContent } from '@/components/blog/mdx-content'
import { SoundCloud } from '@/components/blog/soundcloud-embed'

export const metadata = {
  title: 'Psychic Homily | Arizona Music Community',
  description:
    'Discover upcoming live music shows, blog posts, and DJ sets from the Arizona music scene.',
  openGraph: {
    title: 'Psychic Homily | Arizona Music Community',
    description: 'Discover upcoming live music shows, blog posts, and DJ sets from the Arizona music scene.',
    url: '/',
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

/**
 * Extract embed from MDX content
 */
function extractEmbed(content: string): string | null {
  const embedMatch = content.match(/<(Bandcamp|SoundCloud)[^>]+\/>/)
  return embedMatch ? embedMatch[0] : null
}

/**
 * Get text excerpt
 */
function getTextExcerpt(content: string, maxLength = 200): string {
  let text = content.replace(/<[^>]+\/>/g, '')
  text = text.replace(/<[^>]+>[^<]*<\/[^>]+>/g, '')
  text = text.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
  text = text.replace(/[#*_`]/g, '')
  text = text.replace(/\s+/g, ' ').trim()
  if (text.length > maxLength) {
    text = text.substring(0, maxLength).trim() + '...'
  }
  return text
}

export default function Home() {
  // Get latest blog post
  const blogSlugs = getBlogSlugs()
  const allPosts = blogSlugs
    .map(slug => getBlogPost(slug))
    .filter((post): post is NonNullable<typeof post> => post !== null)
    .sort(
      (a, b) =>
        new Date(b.frontmatter.date).getTime() -
        new Date(a.frontmatter.date).getTime()
    )
  const latestPost = allPosts[0]

  // Get latest DJ set
  const allMixes = getAllMixes()
  const latestMix = allMixes[0]

  return (
    <div className="flex min-h-screen items-start justify-center bg-background">
      <main className="w-full max-w-4xl px-4 py-8 md:px-8">
        {/* Upcoming Shows Section */}
        <section className="mb-14">
          <div className="flex justify-between items-center mb-5">
            <h2 className="text-2xl font-bold tracking-tight">
              Upcoming Shows
            </h2>
            <Link
              href="/shows"
              className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
            >
              View all →
            </Link>
          </div>
          <HomeShowList />
        </section>

        {/* Latest Blog Post Section */}
        {latestPost && (
          <section className="mb-14">
            <div className="flex justify-between items-center mb-5">
              <h2 className="text-2xl font-bold tracking-tight">
                Latest from the Blog
              </h2>
              <Link
                href="/blog"
                className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
              >
                View all →
              </Link>
            </div>
            <article className="bg-card/50 border border-border/50 rounded-xl p-6 hover:border-border transition-colors">
              <h3 className="text-xl font-semibold leading-tight tracking-tight">
                <Link
                  href={`/blog/${latestPost.slug}`}
                  className="hover:text-primary transition-colors"
                >
                  {latestPost.frontmatter.title}
                </Link>
              </h3>
              <div className="text-sm text-muted-foreground mt-1.5">
                {formatDate(latestPost.frontmatter.date)}
              </div>

              {extractEmbed(latestPost.content) && (
                <div className="mt-4">
                  <MDXContent source={extractEmbed(latestPost.content)!} />
                </div>
              )}

              <div className="mt-3 leading-relaxed text-foreground/85">
                {getTextExcerpt(latestPost.content)}
              </div>

              <Link
                href={`/blog/${latestPost.slug}`}
                className="inline-block mt-4 px-4 py-2 text-sm bg-muted/50 border border-border/50 rounded-lg hover:bg-muted hover:border-border transition-colors"
              >
                Read more
              </Link>
            </article>
          </section>
        )}

        {/* Latest DJ Set Section */}
        {latestMix && (
          <section className="mb-14">
            <div className="flex justify-between items-center mb-5">
              <h2 className="text-2xl font-bold tracking-tight">
                Latest DJ Set
              </h2>
              <Link
                href="/dj-sets"
                className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
              >
                View all →
              </Link>
            </div>
            <article className="bg-card/50 border border-border/50 rounded-xl p-6 hover:border-border transition-colors">
              <h3 className="text-xl font-semibold leading-tight tracking-tight">
                <Link
                  href={`/dj-sets/${latestMix.slug}`}
                  className="hover:text-primary transition-colors"
                >
                  {latestMix.title}
                </Link>
              </h3>
              <div className="text-sm text-muted-foreground mt-1.5">
                {formatDate(latestMix.date)} by {latestMix.artist}
              </div>

              {latestMix.description && (
                <div className="mt-3 leading-relaxed text-foreground/85">
                  {latestMix.description}
                </div>
              )}

              <div className="mt-4">
                <SoundCloud
                  url={latestMix.soundcloud_url}
                  title={latestMix.title}
                  artist={latestMix.artist}
                  artist_url={latestMix.artist_url}
                  track_url={latestMix.track_url}
                />
              </div>
            </article>
          </section>
        )}
      </main>
    </div>
  )
}
