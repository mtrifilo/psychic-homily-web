import { notFound } from 'next/navigation'
import Link from 'next/link'
import {
  getAllCategories,
  getCategorySlug,
  getCategoryFromSlug,
  getPostsByCategory,
} from '@/lib/blog'
import { MDXContent } from '@/components/blog/mdx-content'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { formatContentDate } from '@/lib/utils/formatters'

interface CategoryPageProps {
  params: Promise<{ category: string }>
}

// Generate static paths for all categories
export async function generateStaticParams() {
  const categories = getAllCategories()
  return categories.map(category => ({
    category: getCategorySlug(category),
  }))
}

// Generate metadata for the page
export async function generateMetadata({ params }: CategoryPageProps) {
  const { category: categorySlug } = await params
  const categoryName = getCategoryFromSlug(categorySlug)

  if (!categoryName) {
    return { title: 'Category Not Found' }
  }

  return {
    title: `${categoryName} | Psychic Homily`,
    description: `Blog posts in the ${categoryName} category`,
    alternates: {
      canonical: `https://psychichomily.com/categories/${categorySlug}`,
    },
    openGraph: {
      title: `${categoryName} | Psychic Homily`,
      description: `Blog posts in the ${categoryName} category`,
      type: 'website',
    },
  }
}

/**
 * Extract a text-only summary from MDX content
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

/**
 * Extract embed components from MDX content
 */
function extractEmbed(content: string): string | null {
  const embedMatch = content.match(/<(Bandcamp|SoundCloud)[^>]+\/>/)
  return embedMatch ? embedMatch[0] : null
}

export default async function CategoryPage({ params }: CategoryPageProps) {
  const { category: categorySlug } = await params
  const categoryName = getCategoryFromSlug(categorySlug)

  if (!categoryName) {
    notFound()
  }

  const posts = getPostsByCategory(categorySlug)

  return (
    <>
    <JsonLd data={generateBreadcrumbSchema([
      { name: 'Home', url: 'https://psychichomily.com' },
      { name: 'Blog', url: 'https://psychichomily.com/blog' },
      { name: categoryName, url: `https://psychichomily.com/categories/${categorySlug}` },
    ])} />
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">{categoryName}</h1>
        <p className="text-center text-muted-foreground mb-8">
          {posts.length} {posts.length === 1 ? 'post' : 'posts'}
        </p>

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
                  {formatContentDate(post.frontmatter.date)}
                </div>

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
              No posts in this category.
            </p>
          )}
        </section>

        <footer className="mt-12 pt-6 border-t border-border">
          <Link
            href="/blog"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            ← Back to Blog
          </Link>
        </footer>
      </main>
    </div>
    </>
  )
}
