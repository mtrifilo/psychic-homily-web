'use client'

import { useEffect } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ArrowLeft, Hash, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb } from '@/components/shared'
import { useNavigationBreadcrumbs } from '@/lib/context/NavigationBreadcrumbContext'
import { useTag } from '../hooks'
import { getCategoryColor, getCategoryLabel } from '../types'

interface TagDetailProps {
  slug: string
}

export function TagDetail({ slug }: TagDetailProps) {
  const { data: tag, isLoading, error } = useTag(slug)
  const pathname = usePathname()
  const { pushBreadcrumb } = useNavigationBreadcrumbs()

  // Push breadcrumb when tag data is loaded
  useEffect(() => {
    if (tag) {
      pushBreadcrumb(tag.name, pathname)
    }
  }, [tag, pathname, pushBreadcrumb])

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load tag'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Tag Not Found' : 'Error Loading Tag'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The tag you're looking for doesn't exist."
              : errorMessage}
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

  if (!tag) {
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

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        currentPage={tag.name}
      />

      {/* Header */}
      <header className="mb-8">
        <div className="flex items-start gap-4">
          <div className="mt-1">
            <Hash className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <div className="flex items-center gap-3 mb-2">
              <h1 className="text-3xl font-bold tracking-tight">{tag.name}</h1>
              {tag.is_official && (
                <Badge variant="secondary">Official</Badge>
              )}
            </div>

            <div className="flex items-center gap-3 mb-4">
              <span
                className={cn(
                  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
                  getCategoryColor(tag.category)
                )}
              >
                {getCategoryLabel(tag.category)}
              </span>
              <span className="text-sm text-muted-foreground">
                {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
              </span>
            </div>

            {tag.description && (
              <p className="text-muted-foreground whitespace-pre-line mb-4">
                {tag.description}
              </p>
            )}
          </div>
        </div>
      </header>

      {/* Parent tag */}
      {tag.parent_id && tag.parent_name && (
        <section className="mb-6">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Parent Tag
          </h2>
          <Link
            href={`/tags/${tag.parent_id}`}
            className="inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-sm border border-border/50 hover:bg-muted/50 transition-colors"
          >
            <Hash className="h-3.5 w-3.5 text-muted-foreground" />
            {tag.parent_name}
          </Link>
        </section>
      )}

      {/* Child tags count */}
      {tag.child_count > 0 && (
        <section className="mb-6">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Sub-tags
          </h2>
          <p className="text-sm text-muted-foreground">
            {tag.child_count} {tag.child_count === 1 ? 'sub-tag' : 'sub-tags'}
          </p>
        </section>
      )}

      {/* Aliases */}
      {tag.aliases && tag.aliases.length > 0 && (
        <section className="mb-6">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Also known as
          </h2>
          <div className="flex flex-wrap gap-2">
            {tag.aliases.map((alias: string) => (
              <span
                key={alias}
                className="inline-flex items-center rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground border border-border/50"
              >
                {alias}
              </span>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
