'use client'

import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import dynamic from 'next/dynamic'
import {
  Shield,
  MapPin,
  Loader2,
  Upload,
  BadgeCheck,
  Flag,
  ScrollText,
  Users,
  LayoutDashboard,
  Clock,
  Disc3,
  Tag,
  Tags,
  Tent,
  Workflow,
  Library,
  Music,
  ClipboardCheck,
  BarChart3,
  Inbox,
  FileText,
  Settings,
} from 'lucide-react'
import { usePendingVenueEdits } from '@/lib/hooks/admin/useAdminVenueEdits'
import { useUnverifiedVenues } from '@/lib/hooks/admin/useAdminVenues'
import { usePendingReports } from '@/lib/hooks/admin/useAdminReports'
import { usePendingArtistReports } from '@/lib/hooks/admin/useAdminArtistReports'
import { usePendingShows } from '@/lib/hooks/admin/useAdminShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

// Dynamic imports for heavy components - only loaded when their tab is active
const ShowImportPanel = dynamic(
  () =>
    import('@/app/admin/_components/ShowImportPanel').then(
      (m) => m.ShowImportPanel
    ),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const VenueEditsPage = dynamic(() => import('./venue-edits/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const UnverifiedVenuesPage = dynamic(
  () => import('./unverified-venues/page'),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const ReportsPage = dynamic(() => import('./reports/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const AuditLogPage = dynamic(() => import('./audit-log/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const PendingShowsPage = dynamic(() => import('./pending-shows/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const DashboardPage = dynamic(() => import('./dashboard/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const ReleasesPage = dynamic(() => import('./releases/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const LabelsPage = dynamic(() => import('./labels/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const FestivalsPage = dynamic(() => import('./festivals/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const PipelineVenuesComponent = dynamic(
  () =>
    import('@/components/admin/PipelineVenues').then((m) => m.PipelineVenues),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const UsersPage = dynamic(() => import('./users/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const TagsPage = dynamic(() => import('./tags/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const DataQualityPage = dynamic(() => import('./data-quality/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const AnalyticsPage = dynamic(() => import('./analytics/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const ArtistsPage = dynamic(() => import('./artists/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const CollectionManagementComponent = dynamic(
  () =>
    import('@/components/admin/CollectionManagement').then((m) => ({
      default: m.CollectionManagement,
    })),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

// Category type and default sub-tabs
type CategoryId = 'dashboard' | 'review' | 'content' | 'pipeline' | 'platform'

const DEFAULT_SUB_TABS: Record<CategoryId, string> = {
  dashboard: '',
  review: 'pending-shows',
  content: 'releases',
  pipeline: 'pipeline-venues',
  platform: 'data-quality',
}

export default function AdminPage() {
  const [activeCategory, setActiveCategory] =
    useState<CategoryId>('dashboard')
  const [activeSubTabs, setActiveSubTabs] =
    useState<Record<CategoryId, string>>(DEFAULT_SUB_TABS)
  const { user, isLoading, isAuthenticated } = useAuthContext()
  const isAdmin = !!user?.is_admin
  const router = useRouter()

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) {
      router.replace('/auth')
    } else if (!isAdmin) {
      router.replace('/')
    }
  }, [isLoading, isAuthenticated, isAdmin, router])

  const { data: pendingShowsData } = usePendingShows({ enabled: isAdmin })
  const { data: venueEditsData } = usePendingVenueEdits()
  const { data: unverifiedVenuesData } = useUnverifiedVenues({
    enabled: isAdmin,
  })
  const { data: reportsData } = usePendingReports()
  const { data: artistReportsData } = usePendingArtistReports()

  const handleSubTabChange = useCallback(
    (category: CategoryId, subTab: string) => {
      setActiveSubTabs((prev) => ({ ...prev, [category]: subTab }))
    },
    []
  )

  // Calculate total review queue count for the category badge
  const reviewQueueCount =
    (pendingShowsData?.total || 0) +
    (venueEditsData?.total || 0) +
    (unverifiedVenuesData?.total || 0) +
    (reportsData?.total || 0) +
    (artistReportsData?.total || 0)

  if (isLoading || !isAuthenticated || !isAdmin) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-4xl">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
              <Shield className="h-5 w-5 text-primary" />
            </div>
            <h1 className="text-2xl font-bold tracking-tight">
              Admin Console
            </h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Manage pending submissions, venues, and users.
          </p>
        </div>

        {/* Category Navigation (top-level) */}
        <nav
          className="mb-6 inline-flex h-9 w-fit items-center justify-center rounded-lg bg-muted p-[3px]"
          aria-label="Admin categories"
        >
          <CategoryButton
            active={activeCategory === 'dashboard'}
            onClick={() => setActiveCategory('dashboard')}
            icon={<LayoutDashboard className="h-4 w-4" />}
            label="Dashboard"
          />
          <CategoryButton
            active={activeCategory === 'review'}
            onClick={() => setActiveCategory('review')}
            icon={<Inbox className="h-4 w-4" />}
            label="Review Queue"
            badge={reviewQueueCount > 0 ? reviewQueueCount : undefined}
            badgeColor="bg-amber-500"
          />
          <CategoryButton
            active={activeCategory === 'content'}
            onClick={() => setActiveCategory('content')}
            icon={<FileText className="h-4 w-4" />}
            label="Content"
          />
          <CategoryButton
            active={activeCategory === 'pipeline'}
            onClick={() => setActiveCategory('pipeline')}
            icon={<Workflow className="h-4 w-4" />}
            label="Pipeline"
          />
          <CategoryButton
            active={activeCategory === 'platform'}
            onClick={() => setActiveCategory('platform')}
            icon={<Settings className="h-4 w-4" />}
            label="Platform"
          />
        </nav>

        {/* Category Content */}

        {/* Dashboard - no sub-tabs */}
        {activeCategory === 'dashboard' && (
          <div className="space-y-4">
            <DashboardPage />
          </div>
        )}

        {/* Review Queue */}
        {activeCategory === 'review' && (
          <Tabs
            value={activeSubTabs.review}
            onValueChange={(v) => handleSubTabChange('review', v)}
            className="w-full"
          >
            <TabsList className="mb-6">
              <TabsTrigger value="pending-shows" className="gap-2">
                <Clock className="h-4 w-4" />
                Pending Shows
                {pendingShowsData?.total !== undefined &&
                  pendingShowsData.total > 0 && (
                    <span className="ml-1 rounded-full bg-amber-500 px-2 py-0.5 text-xs font-medium text-white">
                      {pendingShowsData.total}
                    </span>
                  )}
              </TabsTrigger>
              <TabsTrigger value="pending-venue-edits" className="gap-2">
                <MapPin className="h-4 w-4" />
                Venue Edits
                {venueEditsData?.total !== undefined &&
                  venueEditsData.total > 0 && (
                    <span className="ml-1 rounded-full bg-amber-500 px-2 py-0.5 text-xs font-medium text-white">
                      {venueEditsData.total}
                    </span>
                  )}
              </TabsTrigger>
              <TabsTrigger value="unverified-venues" className="gap-2">
                <BadgeCheck className="h-4 w-4" />
                Unverified Venues
                {unverifiedVenuesData?.total !== undefined &&
                  unverifiedVenuesData.total > 0 && (
                    <span className="ml-1 rounded-full bg-orange-500 px-2 py-0.5 text-xs font-medium text-white">
                      {unverifiedVenuesData.total}
                    </span>
                  )}
              </TabsTrigger>
              <TabsTrigger value="reports" className="gap-2">
                <Flag className="h-4 w-4" />
                Reports
                {(reportsData?.total || 0) +
                  (artistReportsData?.total || 0) >
                  0 && (
                  <span className="ml-1 rounded-full bg-red-500 px-2 py-0.5 text-xs font-medium text-white">
                    {(reportsData?.total || 0) +
                      (artistReportsData?.total || 0)}
                  </span>
                )}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="pending-shows" className="space-y-4">
              <PendingShowsPage />
            </TabsContent>
            <TabsContent value="pending-venue-edits" className="space-y-4">
              <VenueEditsPage />
            </TabsContent>
            <TabsContent value="unverified-venues" className="space-y-4">
              <UnverifiedVenuesPage />
            </TabsContent>
            <TabsContent value="reports" className="space-y-4">
              <ReportsPage />
            </TabsContent>
          </Tabs>
        )}

        {/* Content */}
        {activeCategory === 'content' && (
          <Tabs
            value={activeSubTabs.content}
            onValueChange={(v) => handleSubTabChange('content', v)}
            className="w-full"
          >
            <TabsList className="mb-6">
              <TabsTrigger value="releases" className="gap-2">
                <Disc3 className="h-4 w-4" />
                Releases
              </TabsTrigger>
              <TabsTrigger value="labels" className="gap-2">
                <Tag className="h-4 w-4" />
                Labels
              </TabsTrigger>
              <TabsTrigger value="festivals" className="gap-2">
                <Tent className="h-4 w-4" />
                Festivals
              </TabsTrigger>
              <TabsTrigger value="collections" className="gap-2">
                <Library className="h-4 w-4" />
                Collections
              </TabsTrigger>
              <TabsTrigger value="tags" className="gap-2">
                <Tags className="h-4 w-4" />
                Tags
              </TabsTrigger>
              <TabsTrigger value="artists-admin" className="gap-2">
                <Music className="h-4 w-4" />
                Artists
              </TabsTrigger>
            </TabsList>

            <TabsContent value="releases" className="space-y-4">
              <ReleasesPage />
            </TabsContent>
            <TabsContent value="labels" className="space-y-4">
              <LabelsPage />
            </TabsContent>
            <TabsContent value="festivals" className="space-y-4">
              <FestivalsPage />
            </TabsContent>
            <TabsContent value="collections" className="space-y-4">
              <CollectionManagementComponent />
            </TabsContent>
            <TabsContent value="tags" className="space-y-4">
              <TagsPage />
            </TabsContent>
            <TabsContent value="artists-admin" className="space-y-4">
              <ArtistsPage />
            </TabsContent>
          </Tabs>
        )}

        {/* Pipeline */}
        {activeCategory === 'pipeline' && (
          <Tabs
            value={activeSubTabs.pipeline}
            onValueChange={(v) => handleSubTabChange('pipeline', v)}
            className="w-full"
          >
            <TabsList className="mb-6">
              <TabsTrigger value="pipeline-venues" className="gap-2">
                <Workflow className="h-4 w-4" />
                Pipeline
              </TabsTrigger>
              <TabsTrigger value="import-show" className="gap-2">
                <Upload className="h-4 w-4" />
                Import Show
              </TabsTrigger>
            </TabsList>

            <TabsContent value="pipeline-venues" className="space-y-4">
              <PipelineVenuesComponent />
            </TabsContent>
            <TabsContent value="import-show" className="space-y-4">
              <ShowImportPanel />
            </TabsContent>
          </Tabs>
        )}

        {/* Platform */}
        {activeCategory === 'platform' && (
          <Tabs
            value={activeSubTabs.platform}
            onValueChange={(v) => handleSubTabChange('platform', v)}
            className="w-full"
          >
            <TabsList className="mb-6">
              <TabsTrigger value="data-quality" className="gap-2">
                <ClipboardCheck className="h-4 w-4" />
                Data Quality
              </TabsTrigger>
              <TabsTrigger value="analytics" className="gap-2">
                <BarChart3 className="h-4 w-4" />
                Analytics
              </TabsTrigger>
              <TabsTrigger value="users" className="gap-2">
                <Users className="h-4 w-4" />
                Users
              </TabsTrigger>
              <TabsTrigger value="audit-log" className="gap-2">
                <ScrollText className="h-4 w-4" />
                Audit Log
              </TabsTrigger>
            </TabsList>

            <TabsContent value="data-quality" className="space-y-4">
              <DataQualityPage />
            </TabsContent>
            <TabsContent value="analytics" className="space-y-4">
              <AnalyticsPage />
            </TabsContent>
            <TabsContent value="users" className="space-y-4">
              <UsersPage />
            </TabsContent>
            <TabsContent value="audit-log" className="space-y-4">
              <AuditLogPage />
            </TabsContent>
          </Tabs>
        )}
      </div>
    </div>
  )
}

/** Reusable button for category-level navigation, styled to match TabsTrigger */
function CategoryButton({
  active,
  onClick,
  icon,
  label,
  badge,
  badgeColor = 'bg-amber-500',
}: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
  badge?: number
  badgeColor?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex h-[calc(100%-1px)] items-center justify-center gap-1.5 rounded-md border border-transparent px-3 py-1 text-sm font-medium whitespace-nowrap transition-[color,box-shadow]',
        active
          ? 'bg-background text-foreground shadow-sm dark:border-input dark:bg-input/30 dark:text-foreground'
          : 'text-foreground dark:text-muted-foreground hover:text-foreground/80'
      )}
    >
      {icon}
      {label}
      {badge !== undefined && (
        <span
          className={cn(
            'ml-1 rounded-full px-2 py-0.5 text-xs font-medium text-white',
            badgeColor
          )}
        >
          {badge}
        </span>
      )}
    </button>
  )
}
