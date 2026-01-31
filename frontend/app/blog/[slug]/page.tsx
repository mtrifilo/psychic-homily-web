import { notFound } from 'next/navigation'
import { getBlogPost, getBlogSlugs } from '@/lib/blog'
import { MDXContent } from '@/components/blog/mdx-content'
import Link from 'next/link'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateBlogPostingSchema } from '@/lib/seo/jsonld'

interface BlogPostPageProps {
  params: Promise<{ slug: string }>
}

// Generate static paths for all blog posts
export async function generateStaticParams() {
  const slugs = getBlogSlugs()
  return slugs.map(slug => ({ slug }))
}

// Generate metadata for the page
export async function generateMetadata({ params }: BlogPostPageProps) {
  const { slug } = await params
  const post = getBlogPost(slug)

  if (!post) {
    return { title: 'Post Not Found' }
  }

  return {
    title: post.frontmatter.title,
    description: post.frontmatter.description || post.excerpt,
    openGraph: {
      title: post.frontmatter.title,
      description: post.frontmatter.description || post.excerpt,
      type: 'article',
      publishedTime: post.frontmatter.date,
      url: `/blog/${slug}`,
    },
  }
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default async function BlogPostPage({ params }: BlogPostPageProps) {
  const { slug } = await params
  const post = getBlogPost(slug)

  if (!post) {
    notFound()
  }

  return (
    <>
      <JsonLd data={generateBlogPostingSchema({
        title: post.frontmatter.title,
        date: post.frontmatter.date,
        description: post.frontmatter.description || post.excerpt,
        slug,
      })} />
      <div className="flex min-h-screen items-start justify-center bg-background">
        <article className="w-full max-w-3xl px-4 py-8 md:px-8">
          <header className="mb-8">
            <h1 className="text-3xl font-bold leading-tight mb-3">
              {post.frontmatter.title}
            </h1>

            <div className="text-sm text-muted-foreground flex flex-wrap gap-2 items-center">
              <time>{formatDate(post.frontmatter.date)}</time>

              {post.frontmatter.categories &&
                post.frontmatter.categories.length > 0 && (
                  <>
                    <span>•</span>
                    <span>
                      {post.frontmatter.categories.map((category, index) => (
                        <span key={category}>
                          {index > 0 && ', '}
                          <Link
                            href={`/categories/${category.toLowerCase().replace(/\s+/g, '-')}`}
                            className="hover:text-foreground transition-colors"
                          >
                            {category}
                          </Link>
                        </span>
                      ))}
                    </span>
                  </>
                )}
            </div>
          </header>

          <div className="text-base leading-relaxed">
            <MDXContent source={post.content} />
          </div>

          <footer className="mt-12 pt-6 border-t border-border">
            <Link
              href="/blog"
              className="text-sm text-muted-foreground hover:text-foreground transition-colors"
            >
              ← Back to Blog
            </Link>
          </footer>
        </article>
      </div>
    </>
  )
}
