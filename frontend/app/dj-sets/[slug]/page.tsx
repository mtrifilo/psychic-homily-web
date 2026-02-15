import { notFound } from 'next/navigation'
import { getMix, getMixSlugs } from '@/lib/mixes'
import { MDXContent } from '@/components/blog/mdx-content'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicRecordingSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { formatContentDate } from '@/lib/utils/formatters'
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
    title: mix.frontmatter.title,
    description: mix.frontmatter.description || `DJ set by ${mix.frontmatter.artist}`,
    alternates: {
      canonical: `https://psychichomily.com/dj-sets/${slug}`,
    },
    openGraph: {
      title: mix.frontmatter.title,
      description: mix.frontmatter.description || `DJ set by ${mix.frontmatter.artist}`,
      type: 'music.song',
      url: `/dj-sets/${slug}`,
    },
  }
}

export default async function MixPage({ params }: MixPageProps) {
  const { slug } = await params
  const mix = getMix(slug)

  if (!mix) {
    notFound()
  }

  return (
    <>
      <JsonLd data={generateMusicRecordingSchema({
        title: mix.frontmatter.title,
        artist: mix.frontmatter.artist,
        date: mix.frontmatter.date,
        slug,
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'DJ Sets', url: 'https://psychichomily.com/dj-sets' },
        { name: mix.frontmatter.title, url: `https://psychichomily.com/dj-sets/${slug}` },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <article className="w-full max-w-3xl px-4 py-8 md:px-8">
          <header className="mb-8">
            <h1 className="text-3xl font-bold leading-tight mb-3">
              {mix.frontmatter.title}
            </h1>

            <div className="text-sm text-muted-foreground">
              {formatContentDate(mix.frontmatter.date)} by {mix.frontmatter.artist}
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
    </>
  )
}

