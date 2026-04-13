'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Mic2,
  MapPin,
  Calendar,
  ChevronRight,
  ExternalLink,
  Music,
  ListTodo,
  Trophy,
  ArrowRight,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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

/** Get browse page URL for an entity type */
function getBrowseUrl(entityType: string): string {
  switch (entityType) {
    case 'artist':
      return '/artists'
    case 'venue':
      return '/venues'
    case 'show':
      return '/shows'
    default:
      return '/'
  }
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
  const browseUrl = getBrowseUrl(category.entity_type)
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
      <CardContent className="pt-0 pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1 text-xs text-primary">
            <span>{isSelected ? 'Viewing items' : 'View items'}</span>
            <ChevronRight className="h-3 w-3" />
          </div>
          <Link
            href={browseUrl}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            onClick={(e) => e.stopPropagation()}
          >
            Browse {formatEntityType(category.entity_type).toLowerCase()}
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}

function CategoryItems({ category, entityType }: { category: string; entityType: string }) {
  const { data, isLoading, error } = useContributeCategory(category)
  const browseUrl = getBrowseUrl(entityType)

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
        <p>No items in this category right now.</p>
        <Button variant="outline" size="sm" asChild className="mt-3">
          <Link href={browseUrl}>
            Browse all {formatEntityType(entityType).toLowerCase()}
            <ArrowRight className="h-3.5 w-3.5" />
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-muted-foreground">
          Showing {items.length} of {total} items
        </span>
        <Link
          href={browseUrl}
          className="text-sm text-primary hover:text-primary/80 flex items-center gap-1 transition-colors"
        >
          Browse all {formatEntityType(entityType).toLowerCase()}
          <ArrowRight className="h-3.5 w-3.5" />
        </Link>
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

const quickActions = [
  {
    label: 'Submit a Show',
    description: 'Add an upcoming show to the calendar',
    href: '/submissions',
    icon: Music,
  },
  {
    label: 'Browse Requests',
    description: 'Fill requests from the community',
    href: '/requests',
    icon: ListTodo,
  },
  {
    label: 'Leaderboard',
    description: 'See top contributors',
    href: '/community/leaderboard',
    icon: Trophy,
  },
]

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
  const selectedCategoryData = categories.find((c) => c.key === selectedCategory)

  return (
    <div className="space-y-8">
      {/* Quick actions */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        {quickActions.map((action) => (
          <Link
            key={action.href}
            href={action.href}
            className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors group"
          >
            <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary shrink-0">
              <action.icon className="h-4.5 w-4.5" />
            </div>
            <div className="min-w-0">
              <div className="text-sm font-medium group-hover:text-primary transition-colors">
                {action.label}
              </div>
              <div className="text-xs text-muted-foreground truncate">
                {action.description}
              </div>
            </div>
          </Link>
        ))}
      </div>

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
      {selectedCategory && selectedCategoryData && (
        <div className="mt-6">
          <h2 className="text-lg font-semibold mb-4">
            {selectedCategoryData.label}
          </h2>
          <CategoryItems
            category={selectedCategory}
            entityType={selectedCategoryData.entity_type}
          />
        </div>
      )}

      {/* Empty state */}
      {totalItems === 0 && (
        <div className="text-center py-12 space-y-4">
          <p className="text-lg font-medium text-foreground mb-2">
            Everything looks great!
          </p>
          <p className="text-muted-foreground">
            No data quality issues found. Here are other ways to contribute:
          </p>
          <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
            <Button variant="outline" size="sm" asChild>
              <Link href="/submissions">
                <Music className="h-4 w-4" />
                Submit a Show
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link href="/requests">
                <ListTodo className="h-4 w-4" />
                Browse Requests
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link href="/artists">
                <Mic2 className="h-4 w-4" />
                Browse Artists
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link href="/venues">
                <MapPin className="h-4 w-4" />
                Browse Venues
              </Link>
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
