'use client'

import { Suspense, useState, useMemo } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import { CalendarCheck, Star, Loader2, Calendar } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useMyShows } from '@/features/shows'
import type { AttendingShow } from '@/features/shows'
import { formatShowDate, formatShowTime } from '@/lib/utils/formatters'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'

const STATUS_TABS = ['all', 'going', 'interested'] as const
type StatusTab = (typeof STATUS_TABS)[number]

function isStatusTab(value: string | null): value is StatusTab {
  return value !== null && STATUS_TABS.includes(value as StatusTab)
}

function AttendingShowCard({ show }: { show: AttendingShow }) {
  return (
    <article className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Link
              href={`/shows/${show.slug || show.show_id}`}
              className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate"
            >
              {show.title}
            </Link>
            <Badge
              variant={show.status === 'going' ? 'default' : 'secondary'}
              className="shrink-0 text-xs"
            >
              {show.status === 'going' ? (
                <CalendarCheck className="h-3 w-3 mr-1" />
              ) : (
                <Star className="h-3 w-3 mr-1" />
              )}
              {show.status === 'going' ? 'Going' : 'Interested'}
            </Badge>
          </div>

          <div className="text-sm text-muted-foreground">
            {show.venue_name && (
              <>
                {show.venue_slug ? (
                  <Link
                    href={`/venues/${show.venue_slug}`}
                    className="text-primary/80 hover:text-primary font-medium transition-colors"
                  >
                    {show.venue_name}
                  </Link>
                ) : (
                  <span className="text-primary/80 font-medium">{show.venue_name}</span>
                )}
                {(show.city || show.state) && (
                  <span className="text-muted-foreground/80">
                    {' '}&middot; {[show.city, show.state].filter(Boolean).join(', ')}
                  </span>
                )}
              </>
            )}
          </div>
        </div>

        <div className="text-right shrink-0">
          <div className="text-sm font-medium text-primary">
            {formatShowDate(show.event_date, show.state ?? undefined)}
          </div>
          <div className="text-xs text-muted-foreground">
            {formatShowTime(show.event_date, show.state ?? undefined)}
          </div>
        </div>
      </div>
    </article>
  )
}

function MyShowsList({ status }: { status: string }) {
  const [offset, setOffset] = useState(0)
  const limit = 20

  const { data, isLoading, error, isFetching } = useMyShows({
    status,
    limit,
    offset,
  })

  const shows = data?.shows ?? []
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
        <p>Failed to load your shows. Please try again later.</p>
      </div>
    )
  }

  if (shows.length === 0) {
    const emptyMessages: Record<string, { title: string; description: string }> = {
      all: {
        title: 'No shows yet',
        description: 'Mark shows as "Going" or "Interested" to see them here.',
      },
      going: {
        title: 'No shows marked as Going',
        description: 'When you mark a show as "Going", it will appear here.',
      },
      interested: {
        title: 'No shows marked as Interested',
        description: 'When you mark a show as "Interested", it will appear here.',
      },
    }

    const msg = emptyMessages[status] || emptyMessages.all

    return (
      <div className="text-center py-12 text-muted-foreground">
        <Calendar className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
        <p className="text-lg mb-2">{msg.title}</p>
        <p className="text-sm">{msg.description}</p>
        <Link
          href="/shows"
          className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          Browse Shows
        </Link>
      </div>
    )
  }

  return (
    <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
      <section className="w-full">
        {shows.map(show => (
          <AttendingShowCard key={`${show.show_id}-${show.status}`} show={show} />
        ))}
      </section>

      {hasMore && (
        <div className="text-center py-6">
          <Button
            variant="outline"
            onClick={() => setOffset(prev => prev + limit)}
            disabled={isFetching}
          >
            {isFetching ? 'Loading...' : 'Load More'}
          </Button>
        </div>
      )}

      {total > 0 && (
        <p className="text-center text-xs text-muted-foreground mt-2">
          Showing {Math.min(offset + limit, total)} of {total} shows
        </p>
      )}
    </div>
  )
}

function MyShowsContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()

  const rawTab = searchParams.get('tab')
  const currentTab: StatusTab = isStatusTab(rawTab) ? rawTab : 'all'

  const handleTabChange = (tab: string) => {
    if (!isStatusTab(tab)) return
    const params = new URLSearchParams()
    if (tab !== 'all') {
      params.set('tab', tab)
    }
    const queryString = params.toString()
    router.replace(queryString ? `/my-shows?${queryString}` : '/my-shows', { scroll: false })
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
          <CalendarCheck className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold tracking-tight">My Shows</h1>
        </div>
        <p className="text-muted-foreground">
          Shows you are going to or interested in
        </p>
      </div>

      {/* Tabs */}
      <Tabs value={currentTab} onValueChange={handleTabChange} className="w-full">
        <TabsList className="mb-6">
          <TabsTrigger value="all" className="gap-1.5">
            All
          </TabsTrigger>
          <TabsTrigger value="going" className="gap-1.5">
            <CalendarCheck className="h-4 w-4" />
            Going
          </TabsTrigger>
          <TabsTrigger value="interested" className="gap-1.5">
            <Star className="h-4 w-4" />
            Interested
          </TabsTrigger>
        </TabsList>

        <TabsContent value="all">
          <MyShowsList status="all" />
        </TabsContent>

        <TabsContent value="going">
          <MyShowsList status="going" />
        </TabsContent>

        <TabsContent value="interested">
          <MyShowsList status="interested" />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function MyShowsLoading() {
  return (
    <div className="flex justify-center items-center min-h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}

export default function MyShowsPage() {
  return (
    <Suspense fallback={<MyShowsLoading />}>
      <MyShowsContent />
    </Suspense>
  )
}
