'use client'

import Link from 'next/link'
import { Library, Users, Star } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import type { Crate } from '../types'

interface CrateCardProps {
  crate: Crate
}

export function CrateCard({ crate }: CrateCardProps) {
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Icon / cover image placeholder */}
        <div className="h-16 w-16 shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden">
          {crate.cover_image_url ? (
            <img
              src={crate.cover_image_url}
              alt={`${crate.title} cover`}
              className="h-full w-full object-cover"
            />
          ) : (
            <Library className="h-8 w-8 text-muted-foreground/40" />
          )}
        </div>

        {/* Text content */}
        <div className="flex-1 min-w-0">
          <Link href={`/crates/${crate.slug}`} className="block group">
            <h3 className="font-bold text-foreground group-hover:text-primary transition-colors truncate">
              {crate.title}
            </h3>
          </Link>

          <div className="flex items-center gap-2 flex-wrap mt-0.5">
            {crate.is_featured && (
              <Badge variant="default" className="text-[10px] px-1.5 py-0">
                <Star className="h-2.5 w-2.5 mr-0.5" />
                Featured
              </Badge>
            )}
            {crate.collaborative && (
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                Collaborative
              </Badge>
            )}
          </div>

          {crate.description && (
            <p
              className={cn(
                'text-sm text-muted-foreground mt-1 line-clamp-2'
              )}
            >
              {crate.description}
            </p>
          )}

          <div className="mt-1.5 flex items-center gap-3 text-xs text-muted-foreground">
            <span>by {crate.creator_name}</span>
            <span className="flex items-center gap-1">
              <Library className="h-3 w-3" />
              {crate.item_count === 1
                ? '1 item'
                : `${crate.item_count} items`}
            </span>
            {crate.subscriber_count > 0 && (
              <span className="flex items-center gap-1">
                <Users className="h-3 w-3" />
                {crate.subscriber_count === 1
                  ? '1 subscriber'
                  : `${crate.subscriber_count} subscribers`}
              </span>
            )}
          </div>
        </div>
      </div>
    </article>
  )
}
