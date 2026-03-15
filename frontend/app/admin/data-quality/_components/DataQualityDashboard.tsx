'use client'

import { useState } from 'react'
import {
  AlertTriangle,
  ChevronLeft,
  ExternalLink,
  Loader2,
  type LucideIcon,
  MapPin,
  Music,
  Ticket,
  Users,
  BadgeCheck,
  DollarSign,
  ListOrdered,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  useDataQualitySummary,
  useDataQualityCategory,
  type DataQualityCategory,
  type DataQualityItem,
} from '@/lib/hooks/admin/useDataQuality'

// Map category keys to icons
const categoryIcons: Record<string, LucideIcon> = {
  artists_missing_links: Music,
  artists_missing_location: MapPin,
  artists_no_aliases: Users,
  venues_missing_social: Ticket,
  venues_unverified_with_shows: BadgeCheck,
  shows_no_billing_order: ListOrdered,
  shows_missing_price: DollarSign,
}

// Map entity types to base paths for links
function getEntityUrl(item: DataQualityItem): string {
  const slug = item.slug || String(item.entity_id)
  switch (item.entity_type) {
    case 'artist':
      return `/artists/${slug}`
    case 'venue':
      return `/venues/${slug}`
    case 'show':
      return `/shows/${slug}`
    default:
      return '#'
  }
}

// Summary card for a single category
function CategoryCard({
  category,
  onClick,
}: {
  category: DataQualityCategory
  onClick: () => void
}) {
  const Icon = categoryIcons[category.key] || AlertTriangle

  return (
    <Card
      className="cursor-pointer transition-colors hover:bg-muted/50"
      onClick={onClick}
    >
      <CardContent className="flex items-center gap-4 py-4">
        <div
          className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
            category.count > 0
              ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
              : 'bg-muted text-muted-foreground'
          }`}
        >
          <Icon className="h-5 w-5" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="font-medium">{category.label}</p>
            {category.count > 0 && (
              <Badge variant="secondary" className="tabular-nums">
                {category.count}
              </Badge>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            {category.description}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

// Item row for category detail view
function ItemRow({ item }: { item: DataQualityItem }) {
  const entityUrl = getEntityUrl(item)

  return (
    <div className="flex items-center justify-between border-b border-border py-3 last:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <a
            href={entityUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="font-medium text-foreground hover:text-primary hover:underline"
          >
            {item.name}
          </a>
          <ExternalLink className="h-3 w-3 text-muted-foreground" />
        </div>
        <p className="text-sm text-muted-foreground">{item.reason}</p>
      </div>
      <div className="flex items-center gap-3">
        {item.show_count > 0 && (
          <Badge variant="outline" className="tabular-nums">
            {item.show_count} show{item.show_count !== 1 ? 's' : ''}
          </Badge>
        )}
        <Badge variant="secondary" className="capitalize">
          {item.entity_type}
        </Badge>
      </div>
    </div>
  )
}

// Category detail view with paginated items
function CategoryDetail({
  category,
  onBack,
}: {
  category: DataQualityCategory
  onBack: () => void
}) {
  const [page, setPage] = useState(0)
  const limit = 50
  const offset = page * limit

  const { data, isLoading } = useDataQualityCategory(
    category.key,
    limit,
    offset
  )

  const totalPages = data ? Math.ceil(data.total / limit) : 0

  return (
    <div>
      <Button
        variant="ghost"
        size="sm"
        onClick={onBack}
        className="mb-4 gap-1"
      >
        <ChevronLeft className="h-4 w-4" />
        Back to summary
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {category.label}
            <Badge variant="secondary" className="tabular-nums">
              {data?.total ?? category.count} total
            </Badge>
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {category.description}
          </p>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : data?.items && data.items.length > 0 ? (
            <>
              <div className="divide-y divide-border">
                {data.items.map((item) => (
                  <ItemRow
                    key={`${item.entity_type}-${item.entity_id}`}
                    item={item}
                  />
                ))}
              </div>

              {totalPages > 1 && (
                <div className="mt-4 flex items-center justify-between">
                  <p className="text-sm text-muted-foreground">
                    Showing {offset + 1}--
                    {Math.min(offset + limit, data.total)} of {data.total}
                  </p>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page === 0}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page >= totalPages - 1}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No items in this category.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

// Main dashboard component
export function DataQualityDashboard() {
  const { data: summary, isLoading, error } = useDataQualitySummary()
  const [selectedCategory, setSelectedCategory] =
    useState<DataQualityCategory | null>(null)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">
          Failed to load data quality summary.
        </p>
      </div>
    )
  }

  if (selectedCategory) {
    return (
      <CategoryDetail
        category={selectedCategory}
        onBack={() => setSelectedCategory(null)}
      />
    )
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-lg font-semibold">Data Quality</h2>
        <p className="text-sm text-muted-foreground">
          Entities that need attention, ranked by impact.
          {summary && summary.total_items > 0 && (
            <span className="ml-1 font-medium text-amber-600 dark:text-amber-400">
              {summary.total_items} total items need work.
            </span>
          )}
        </p>
      </div>

      <div className="grid gap-3">
        {summary?.categories.map((category) => (
          <CategoryCard
            key={category.key}
            category={category}
            onClick={() => setSelectedCategory(category)}
          />
        ))}
      </div>
    </div>
  )
}
