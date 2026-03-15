'use client'

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import {
  UserCheck,
  Mic2,
  MapPin,
  Tag,
  Tent,
  Loader2,
  UserMinus,
  Users,
} from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useMyFollowing, useUnfollow } from '@/lib/hooks/common/useFollow'
import type { FollowingEntity } from '@/lib/types/follow'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'

const TYPE_TABS = ['all', 'artist', 'venue', 'label', 'festival'] as const
type TypeTab = (typeof TYPE_TABS)[number]

function isTypeTab(value: string | null): value is TypeTab {
  return value !== null && TYPE_TABS.includes(value as TypeTab)
}

/** Maps singular entity type to its plural URL path and display label */
const entityTypeInfo: Record<
  string,
  { plural: string; label: string; href: (slug: string) => string }
> = {
  artist: { plural: 'artists', label: 'Artist', href: (slug) => `/artists/${slug}` },
  venue: { plural: 'venues', label: 'Venue', href: (slug) => `/venues/${slug}` },
  label: { plural: 'labels', label: 'Label', href: (slug) => `/labels/${slug}` },
  festival: {
    plural: 'festivals',
    label: 'Festival',
    href: (slug) => `/festivals/${slug}`,
  },
}

function getEntityIcon(entityType: string) {
  switch (entityType) {
    case 'artist':
      return Mic2
    case 'venue':
      return MapPin
    case 'label':
      return Tag
    case 'festival':
      return Tent
    default:
      return Users
  }
}

function FollowingEntityCard({ entity }: { entity: FollowingEntity }) {
  const unfollow = useUnfollow()
  const Icon = getEntityIcon(entity.entity_type)
  const info = entityTypeInfo[entity.entity_type]

  const handleUnfollow = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    if (!info || unfollow.isPending) return

    unfollow.mutate({
      entityType: info.plural,
      entityId: entity.entity_id,
    })
  }

  const href = info?.href(entity.slug) ?? '#'
  const followedDate = new Date(entity.followed_at)
  const formattedDate = followedDate.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })

  return (
    <article className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="shrink-0 h-9 w-9 rounded-md bg-muted flex items-center justify-center">
            <Icon className="h-4 w-4 text-muted-foreground" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <Link
                href={href}
                className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate"
              >
                {entity.name}
              </Link>
              <Badge variant="secondary" className="shrink-0 text-xs">
                {info?.label ?? entity.entity_type}
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              Followed {formattedDate}
            </p>
          </div>
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={handleUnfollow}
          disabled={unfollow.isPending}
          className="text-muted-foreground hover:text-destructive shrink-0"
          title="Unfollow"
          aria-label={`Unfollow ${entity.name}`}
        >
          {unfollow.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <UserMinus className="h-4 w-4" />
          )}
        </Button>
      </div>
    </article>
  )
}

function FollowingList({ type }: { type: string }) {
  const [offset, setOffset] = useState(0)
  const limit = 20

  const { data, isLoading, error, isFetching } = useMyFollowing({
    type,
    limit,
    offset,
  })

  const following = data?.following ?? []
  const total = data?.total ?? 0
  const hasMore = offset + limit < total

  if (isLoading && !data) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load your following list. Please try again later.</p>
      </div>
    )
  }

  if (following.length === 0) {
    const emptyMessages: Record<string, { title: string; description: string }> = {
      all: {
        title: 'Not following anything yet',
        description:
          'Follow artists, venues, labels, and festivals to see them here.',
      },
      artist: {
        title: 'No artists followed',
        description: 'Follow artists to keep up with their shows and releases.',
      },
      venue: {
        title: 'No venues followed',
        description: 'Follow venues to stay updated on their upcoming shows.',
      },
      label: {
        title: 'No labels followed',
        description:
          'Follow labels to discover new releases and roster updates.',
      },
      festival: {
        title: 'No festivals followed',
        description: 'Follow festivals to get lineup and schedule updates.',
      },
    }

    const msg = emptyMessages[type] || emptyMessages.all

    return (
      <div className="text-center py-12 text-muted-foreground">
        <Users className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
        <p className="text-lg mb-2">{msg.title}</p>
        <p className="text-sm">{msg.description}</p>
        <Link
          href="/artists"
          className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          Browse Artists
        </Link>
      </div>
    )
  }

  return (
    <div
      className={
        isFetching
          ? 'opacity-60 transition-opacity duration-75'
          : 'transition-opacity duration-75'
      }
    >
      <section className="w-full">
        {following.map((entity) => (
          <FollowingEntityCard
            key={`${entity.entity_type}-${entity.entity_id}`}
            entity={entity}
          />
        ))}
      </section>

      {hasMore && (
        <div className="text-center py-6">
          <Button
            variant="outline"
            onClick={() => setOffset((prev) => prev + limit)}
            disabled={isFetching}
          >
            {isFetching ? 'Loading...' : 'Load More'}
          </Button>
        </div>
      )}

      {total > 0 && (
        <p className="text-center text-xs text-muted-foreground mt-2">
          Showing {Math.min(offset + limit, total)} of {total}
        </p>
      )}
    </div>
  )
}

function FollowingContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()

  const rawTab = searchParams.get('tab')
  const currentTab: TypeTab = isTypeTab(rawTab) ? rawTab : 'all'

  const handleTabChange = (tab: string) => {
    if (!isTypeTab(tab)) return
    const params = new URLSearchParams()
    if (tab !== 'all') {
      params.set('tab', tab)
    }
    const queryString = params.toString()
    router.replace(
      queryString ? `/following?${queryString}` : '/following',
      { scroll: false }
    )
  }

  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  return (
    <div className="container max-w-6xl mx-auto px-4 py-12">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <UserCheck className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold tracking-tight">Following</h1>
        </div>
        <p className="text-muted-foreground">
          Artists, venues, labels, and festivals you follow
        </p>
      </div>

      {/* Tabs */}
      <Tabs
        value={currentTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList className="mb-6">
          <TabsTrigger value="all" className="gap-1.5">
            All
          </TabsTrigger>
          <TabsTrigger value="artist" className="gap-1.5">
            <Mic2 className="h-4 w-4" />
            Artists
          </TabsTrigger>
          <TabsTrigger value="venue" className="gap-1.5">
            <MapPin className="h-4 w-4" />
            Venues
          </TabsTrigger>
          <TabsTrigger value="label" className="gap-1.5">
            <Tag className="h-4 w-4" />
            Labels
          </TabsTrigger>
          <TabsTrigger value="festival" className="gap-1.5">
            <Tent className="h-4 w-4" />
            Festivals
          </TabsTrigger>
        </TabsList>

        <TabsContent value="all">
          <FollowingList type="all" />
        </TabsContent>

        <TabsContent value="artist">
          <FollowingList type="artist" />
        </TabsContent>

        <TabsContent value="venue">
          <FollowingList type="venue" />
        </TabsContent>

        <TabsContent value="label">
          <FollowingList type="label" />
        </TabsContent>

        <TabsContent value="festival">
          <FollowingList type="festival" />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function FollowingLoading() {
  return (
    <div className="flex justify-center items-center min-h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}

export default function FollowingPage() {
  return (
    <Suspense fallback={<FollowingLoading />}>
      <FollowingContent />
    </Suspense>
  )
}
