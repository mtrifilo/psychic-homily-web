'use client'

import { useState } from 'react'
import { Flame, Mic2, MapPin, Disc3, ArrowRight, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  useChartsOverview,
  useTrendingShows,
  usePopularArtists,
  useActiveVenues,
  useHotReleases,
} from '../hooks'
import { TrendingShowsList } from './TrendingShowsList'
import { PopularArtistsList } from './PopularArtistsList'
import { ActiveVenuesList } from './ActiveVenuesList'
import { HotReleasesList } from './HotReleasesList'
import type { ChartView } from '../types'

const views: { id: ChartView; label: string; icon: typeof Flame }[] = [
  { id: 'overview', label: 'Overview', icon: Flame },
  { id: 'trending-shows', label: 'Upcoming Shows', icon: Flame },
  { id: 'popular-artists', label: 'Popular Artists', icon: Mic2 },
  { id: 'active-venues', label: 'Active Venues', icon: MapPin },
  { id: 'hot-releases', label: 'Recent Releases', icon: Disc3 },
]

export function ChartsPage() {
  const [activeView, setActiveView] = useState<ChartView>('overview')

  return (
    <div className="space-y-6">
      {/* View selector tabs */}
      <div className="flex flex-wrap gap-1.5 rounded-lg bg-muted/50 p-1">
        {views.map(view => {
          const Icon = view.icon
          const isActive = activeView === view.id
          return (
            <Button
              key={view.id}
              variant={isActive ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setActiveView(view.id)}
              className="gap-1.5 text-xs"
            >
              <Icon className="h-3.5 w-3.5" />
              {view.label}
            </Button>
          )
        })}
      </div>

      {/* Content */}
      {activeView === 'overview' ? (
        <OverviewGrid onViewAll={setActiveView} />
      ) : activeView === 'trending-shows' ? (
        <DetailView title="Upcoming Shows" description="Shows coming up soon, ordered by date.">
          <TrendingShowsDetail />
        </DetailView>
      ) : activeView === 'popular-artists' ? (
        <DetailView title="Popular Artists" description="Artists with the most followers and upcoming shows.">
          <PopularArtistsDetail />
        </DetailView>
      ) : activeView === 'active-venues' ? (
        <DetailView title="Active Venues" description="Venues with the most upcoming shows and followers.">
          <ActiveVenuesDetail />
        </DetailView>
      ) : (
        <DetailView title="Recent Releases" description="Recently added releases.">
          <HotReleasesDetail />
        </DetailView>
      )}
    </div>
  )
}

// --- Overview Grid ---

function OverviewGrid({ onViewAll }: { onViewAll: (view: ChartView) => void }) {
  const { data, isLoading, error } = useChartsOverview()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <p className="text-sm text-destructive py-4 text-center">
        Failed to load charts. Please try again later.
      </p>
    )
  }

  if (!data) return null

  return (
    <div className="grid gap-6 md:grid-cols-2">
      <ChartCard
        title="Upcoming Shows"
        icon={Flame}
        onViewAll={() => onViewAll('trending-shows')}
      >
        <TrendingShowsList shows={data.trending_shows} compact />
      </ChartCard>
      <ChartCard
        title="Popular Artists"
        icon={Mic2}
        onViewAll={() => onViewAll('popular-artists')}
      >
        <PopularArtistsList artists={data.popular_artists} compact />
      </ChartCard>
      <ChartCard
        title="Active Venues"
        icon={MapPin}
        onViewAll={() => onViewAll('active-venues')}
      >
        <ActiveVenuesList venues={data.active_venues} compact />
      </ChartCard>
      <ChartCard
        title="Recent Releases"
        icon={Disc3}
        onViewAll={() => onViewAll('hot-releases')}
      >
        <HotReleasesList releases={data.hot_releases} compact />
      </ChartCard>
    </div>
  )
}

// --- Chart Card ---

function ChartCard({
  title,
  icon: Icon,
  onViewAll,
  children,
}: {
  title: string
  icon: typeof Flame
  onViewAll: () => void
  children: React.ReactNode
}) {
  return (
    <div className="rounded-xl border border-border/50 bg-card">
      <div className="flex items-center justify-between border-b border-border/30 px-4 py-3">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">{title}</h2>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={onViewAll}
          className="gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          View all
          <ArrowRight className="h-3 w-3" />
        </Button>
      </div>
      <div className="p-2">
        {children}
      </div>
    </div>
  )
}

// --- Detail View Wrapper ---

function DetailView({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <div className="rounded-xl border border-border/50 bg-card">
      <div className="border-b border-border/30 px-4 py-3">
        <h2 className="text-lg font-semibold">{title}</h2>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="p-2">
        {children}
      </div>
    </div>
  )
}

// --- Detail Loaders ---

function TrendingShowsDetail() {
  const { data, isLoading, error } = useTrendingShows()
  if (isLoading) return <DetailLoading />
  if (error) return <DetailError />
  if (!data) return null
  return <TrendingShowsList shows={data.shows} />
}

function PopularArtistsDetail() {
  const { data, isLoading, error } = usePopularArtists()
  if (isLoading) return <DetailLoading />
  if (error) return <DetailError />
  if (!data) return null
  return <PopularArtistsList artists={data.artists} />
}

function ActiveVenuesDetail() {
  const { data, isLoading, error } = useActiveVenues()
  if (isLoading) return <DetailLoading />
  if (error) return <DetailError />
  if (!data) return null
  return <ActiveVenuesList venues={data.venues} />
}

function HotReleasesDetail() {
  const { data, isLoading, error } = useHotReleases()
  if (isLoading) return <DetailLoading />
  if (error) return <DetailError />
  if (!data) return null
  return <HotReleasesList releases={data.releases} />
}

function DetailLoading() {
  return (
    <div className="flex items-center justify-center py-8">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  )
}

function DetailError() {
  return (
    <p className="text-sm text-destructive py-4 text-center">
      Failed to load chart data. Please try again later.
    </p>
  )
}
