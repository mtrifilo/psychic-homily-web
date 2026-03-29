'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Mic2, MapPin, Calendar, ChevronRight, ExternalLink } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { LoadingSpinner } from '@/components/shared'
import { useContributeOpportunities, useContributeCategory } from '../hooks'
import type { DataQualityCategory, DataQualityItem } from '../types'

/** Map entity types to their URL prefix */
function getEntityUrl(entityType: string, slug: string): string {
  switch (entityType) {
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

/** Map entity types to their icons */
function getEntityIcon(entityType: string): LucideIcon {
  switch (entityType) {
    case 'artist':
      return Mic2
    case 'venue':
      return MapPin
    case 'show':
      return Calendar
    default:
      return Mic2
  }
}

/** Format entity type for display */
function formatEntityType(entityType: string): string {
  return entityType.charAt(0).toUpperCase() + entityType.slice(1) + 's'
}

function CategoryCard({
  category,
  isSelected,
  onClick,
}: {
  category: DataQualityCategory
  isSelected: boolean
  onClick: () => void
}) {
  const Icon = getEntityIcon(category.entity_type)
  return (
    <Card
      className={`cursor-pointer transition-colors hover:border-primary/50 ${
        isSelected ? 'border-primary ring-1 ring-primary/20' : ''
      }`}
      onClick={onClick}
    >
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Icon className="h-4 w-4 text-muted-foreground" />
            <Badge variant="secondary" className="text-xs">
              {formatEntityType(category.entity_type)}
            </Badge>
          </div>
          <Badge variant={category.count > 0 ? 'default' : 'secondary'}>
            {category.count}
          </Badge>
        </div>
        <CardTitle className="text-base">{category.label}</CardTitle>
        <CardDescription className="text-sm">{category.description}</CardDescription>
      </CardHeader>
      {isSelected && (
        <CardContent className="pt-0 pb-2">
          <div className="flex items-center gap-1 text-xs text-primary">
            <span>View items</span>
            <ChevronRight className="h-3 w-3" />
          </div>
        </CardContent>
      )}
    </Card>
  )
}

function CategoryItems({ category }: { category: string }) {
  const { data, isLoading, error } = useContributeCategory(category)

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <LoadingSpinner />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        Failed to load items. Please try again.
      </div>
    )
  }

  const items = data?.items ?? []
  const total = data?.total ?? 0

  if (items.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No items in this category right now.
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="text-sm text-muted-foreground mb-3">
        Showing {items.length} of {total} items
      </div>
      {items.map((item: DataQualityItem) => (
        <ItemRow key={`${item.entity_type}-${item.entity_id}`} item={item} />
      ))}
    </div>
  )
}

function ItemRow({ item }: { item: DataQualityItem }) {
  const Icon = getEntityIcon(item.entity_type)
  const url = item.slug ? getEntityUrl(item.entity_type, item.slug) : '#'

  return (
    <div className="flex items-center justify-between rounded-md border px-3 py-2 hover:bg-muted/50 transition-colors">
      <div className="flex items-center gap-3 min-w-0">
        <Icon className="h-4 w-4 text-muted-foreground shrink-0" />
        <div className="min-w-0">
          <Link
            href={url}
            className="text-sm font-medium hover:underline truncate block"
          >
            {item.name}
            <ExternalLink className="inline-block ml-1 h-3 w-3 text-muted-foreground" />
          </Link>
          <span className="text-xs text-muted-foreground">{item.reason}</span>
        </div>
      </div>
      {item.show_count > 0 && (
        <Badge variant="secondary" className="text-xs shrink-0 ml-2">
          {item.show_count} shows
        </Badge>
      )}
    </div>
  )
}

export function ContributeDashboard() {
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null)
  const { data, isLoading, error } = useContributeOpportunities()

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        Failed to load contribution opportunities. Please try again later.
      </div>
    )
  }

  const categories = data?.categories ?? []
  const totalItems = data?.total_items ?? 0

  return (
    <div className="space-y-8">
      {/* Summary */}
      <div className="text-center space-y-2">
        <p className="text-muted-foreground">
          There {totalItems === 1 ? 'is' : 'are'}{' '}
          <span className="font-semibold text-foreground">{totalItems}</span>{' '}
          {totalItems === 1 ? 'item' : 'items'} across {categories.length} categories
          that could use your help.
        </p>
      </div>

      {/* Category grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {categories.map((category) => (
          <CategoryCard
            key={category.key}
            category={category}
            isSelected={selectedCategory === category.key}
            onClick={() =>
              setSelectedCategory(
                selectedCategory === category.key ? null : category.key
              )
            }
          />
        ))}
      </div>

      {/* Selected category items */}
      {selectedCategory && (
        <div className="mt-6">
          <h2 className="text-lg font-semibold mb-4">
            {categories.find((c) => c.key === selectedCategory)?.label}
          </h2>
          <CategoryItems category={selectedCategory} />
        </div>
      )}

      {/* Empty state */}
      {totalItems === 0 && (
        <div className="text-center py-12">
          <p className="text-lg font-medium text-foreground mb-2">
            Everything looks great!
          </p>
          <p className="text-muted-foreground">
            No data quality issues found. Check back later for new opportunities.
          </p>
        </div>
      )}
    </div>
  )
}
