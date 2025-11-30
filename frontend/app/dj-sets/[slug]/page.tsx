import { notFound } from 'next/navigation'
import { getMix, getMixSlugs } from '@/lib/mixes'
import { MDXContent } from '@/components/blog/mdx-content'
import Link from 'next/link'

interface MixPageProps {
  params: Promise<{ slug: string }>
}

// Generate static paths for all mixes
export async function generateStaticParams() {
  const slugs = getMixSlugs()
  return slugs.map(slug => ({ slug }))
}

// Generate metadata for the page
export async function generateMetadata({ params }: MixPageProps) {
  const { slug } = await params
  const mix = getMix(slug)

  if (!mix) {
    return { title: 'Mix Not Found' }
  }

  return {
    title: `${mix.frontmatter.title} | Psychic Homily`,
    description: mix.frontmatter.description,
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

export default async function MixPage({ params }: MixPageProps) {
  const { slug } = await params
  const mix = getMix(slug)

  if (!mix) {
    notFound()
  }

  return (
    <div className="flex min-h-screen items-start justify-center bg-background">
      <article className="w-full max-w-3xl px-4 py-8 md:px-8">
        <header className="mb-8">
          <h1 className="text-3xl font-bold leading-tight mb-3">
            {mix.frontmatter.title}
          </h1>

          <div className="text-sm text-muted-foreground">
            {formatDate(mix.frontmatter.date)} by {mix.frontmatter.artist}
          </div>
        </header>

        <div className="text-base leading-relaxed">
          <MDXContent source={mix.content} />
        </div>

        <footer className="mt-12 pt-6 border-t border-border">
          <Link
            href="/dj-sets"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            ← Back to DJ Sets
          </Link>
        </footer>
      </article>
    </div>
  )
}

