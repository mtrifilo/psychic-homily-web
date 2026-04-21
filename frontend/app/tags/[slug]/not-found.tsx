import Link from 'next/link'
import type { Metadata } from 'next'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'

// Owned metadata so the tab title reads "Tag not found" instead of inheriting
// a stale parent title. Matches the `generateMetadata` fallback in
// `app/tags/[slug]/page.tsx` (PSY-497).
export const metadata: Metadata = {
  title: 'Tag not found',
  description: 'The tag you are looking for does not exist.',
}

// Rendered by Next.js when `app/tags/[slug]/page.tsx` calls `notFound()` —
// returns the hard HTTP 404 status. Copy + visual match the original
// client-rendered not-found branch in `TagDetail` (pre-PSY-497) so the UI
// is unchanged; the fix is purely at the response-status layer.
export default function TagNotFound() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <div className="text-center">
        <h1 className="text-2xl font-bold mb-2">Tag Not Found</h1>
        <p className="text-muted-foreground mb-4">
          The tag you&apos;re looking for doesn&apos;t exist.
        </p>
        <Button asChild variant="outline">
          <Link href="/tags">
            <ArrowLeft className="h-4 w-4 mr-2" />
            Back to Tags
          </Link>
        </Button>
      </div>
    </div>
  )
}
