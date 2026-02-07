import Link from 'next/link'
import { getBlogSlugs, getBlogPost } from '@/lib/blog'
import { MDXContent } from '@/components/blog/mdx-content'

export const metadata = {
  title: 'Blog',
  description: 'Music news, reviews, and updates from the Arizona music scene.',
  openGraph: {
    title: 'Blog | Psychic Homily',
    description: 'Music news, reviews, and updates from the Arizona music scene.',
    url: '/blog',
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
 * Extract a text-only summary from MDX content (for the excerpt after embed)
 */
function getTextExcerpt(content: string, maxLength = 200): string {
  // Remove MDX/JSX components
  let text = content.replace(/<[^>]+\/>/g, '')
  text = text.replace(/<[^>]+>[^<]*<\/[^>]+>/g, '')
  // Remove markdown links but keep text
  text = text.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
  // Remove markdown formatting
  text = text.replace(/[#*_`]/g, '')
  // Remove extra whitespace
  text = text.replace(/\s+/g, ' ').trim()
  // Truncate
  if (text.length > maxLength) {
    text = text.substring(0, maxLength).trim() + '...'
  }
  return text
}

/**
 * Extract embed components from MDX content
 */
function extractEmbed(content: string): string | null {
  // Match Bandcamp or SoundCloud components
  const embedMatch = content.match(/<(Bandcamp|SoundCloud)[^>]+\/>/)
  return embedMatch ? embedMatch[0] : null
}

export default function BlogPage() {
  const slugs = getBlogSlugs()

  // Get full posts and sort by date
  const posts = slugs
    .map(slug => getBlogPost(slug))
    .filter((post): post is NonNullable<typeof post> => post !== null)
    .sort(
      (a, b) =>
        new Date(b.frontmatter.date).getTime() -
        new Date(a.frontmatter.date).getTime()
    )

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Blog</h1>

        <section className="w-full">
          {posts.map(post => {
            const embed = extractEmbed(post.content)
            const textExcerpt = getTextExcerpt(post.content)

            return (
              <article
                key={post.slug}
                className="border-b border-border pb-6 mt-6 first:mt-0"
              >
                <h2 className="text-xl font-semibold leading-tight">
                  <Link
                    href={`/blog/${post.slug}`}
                    className="hover:text-muted-foreground transition-colors"
                  >
                    {post.frontmatter.title}
                  </Link>
                </h2>

                <div className="text-sm text-muted-foreground mt-1">
                  {formatDate(post.frontmatter.date)}
                </div>

                {/* Render embed if present */}
                {embed && (
                  <div className="mt-4">
                    <MDXContent source={embed} />
                  </div>
                )}

                <div className="mt-3 leading-relaxed text-foreground/90">
                  {textExcerpt}
                </div>

                <Link
                  href={`/blog/${post.slug}`}
                  className="inline-block mt-3 px-3 py-1 text-xs border border-border rounded hover:bg-muted transition-colors"
                >
                  read more
                </Link>
              </article>
            )
          })}

          {posts.length === 0 && (
            <p className="text-center text-muted-foreground py-12">
              No blog posts yet.
            </p>
          )}
        </section>
      </main>
    </div>
  )
}
