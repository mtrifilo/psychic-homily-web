import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { SceneDetailView } from '@/features/scenes'

interface ScenePageProps {
  params: Promise<{ slug: string }>
}

export async function generateMetadata({ params }: ScenePageProps) {
  const { slug } = await params

  // Parse slug to create a readable title (e.g., "phoenix-az" -> "Phoenix, AZ")
  const parts = slug.split('-')
  const state = parts.pop()?.toUpperCase() || ''
  const city = parts.map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ')

  return {
    title: `${city}, ${state} Music Scene`,
    description: `Explore the ${city}, ${state} music scene — venues, active artists, upcoming shows, and scene pulse.`,
    alternates: {
      canonical: `https://psychichomily.com/scenes/${slug}`,
    },
    openGraph: {
      title: `${city}, ${state} Music Scene | Psychic Homily`,
      description: `Explore the ${city}, ${state} music scene — venues, active artists, upcoming shows, and scene pulse.`,
      url: `/scenes/${slug}`,
      type: 'website',
    },
  }
}

function SceneLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ScenePage({ params }: ScenePageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Scene</h1>
          <p className="text-muted-foreground">
            The scene could not be found.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <Suspense fallback={<SceneLoadingFallback />}>
          <SceneDetailView slug={slug} />
        </Suspense>
      </main>
    </div>
  )
}
