'use client'

import { useState, useEffect, useCallback, useRef, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Shield, ShieldCheck, MapPin, Loader2, Upload, BadgeCheck, Flag, ScrollText, Users, LayoutDashboard, Clock, Disc3, Tag, Tags, Tent, Workflow, Library, Music, ClipboardCheck, BarChart3, Radio, ChevronLeft, ChevronRight } from 'lucide-react'
import { usePendingVenueEdits } from '@/lib/hooks/admin/useAdminVenueEdits'
import { useUnverifiedVenues } from '@/lib/hooks/admin/useAdminVenues'
import { usePendingReports } from '@/lib/hooks/admin/useAdminReports'
import { usePendingArtistReports } from '@/lib/hooks/admin/useAdminArtistReports'
import { usePendingShows } from '@/lib/hooks/admin/useAdminShows'
import { useAdminPendingEdits } from '@/lib/hooks/admin/useAdminPendingEdits'
import { useAdminEntityReports } from '@/lib/hooks/admin/useAdminEntityReports'
import { useAdminPendingComments } from '@/lib/hooks/admin/useAdminComments'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

// Dynamic imports for heavy components - only loaded when their tab is active
const ShowImportPanel = dynamic(
  () => import('@/app/admin/_components/ShowImportPanel').then(m => m.ShowImportPanel),
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

const UnverifiedVenuesPage = dynamic(() => import('./unverified-venues/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

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
  () => import('@/components/admin/PipelineVenues').then(m => m.PipelineVenues),
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

const RadioPage = dynamic(() => import('./radio/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const ModerationPage = dynamic(() => import('./moderation/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const CollectionManagementComponent = dynamic(
  () => import('@/components/admin/CollectionManagement').then(m => ({ default: m.CollectionManagement })),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const VALID_TABS = [
  'dashboard', 'moderation', 'pending-shows', 'pending-venue-edits', 'unverified-venues',
  'reports', 'import-show', 'releases', 'labels', 'festivals', 'pipeline',
  'collections', 'tags', 'data-quality', 'analytics', 'artists-admin', 'radio',
  'users', 'audit-log',
] as const

type AdminTab = (typeof VALID_TABS)[number]

function isValidTab(value: string | null): value is AdminTab {
  return value !== null && (VALID_TABS as readonly string[]).includes(value)
}

function ScrollableTabBar({ children, activeTab }: { children: React.ReactNode; activeTab: string }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  const updateScrollState = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    setCanScrollLeft(el.scrollLeft > 0)
    setCanScrollRight(el.scrollLeft < el.scrollWidth - el.clientWidth - 1)
  }, [])

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    updateScrollState()
    el.addEventListener('scroll', updateScrollState, { passive: true })
    const observer = new ResizeObserver(updateScrollState)
    observer.observe(el)
    return () => {
      el.removeEventListener('scroll', updateScrollState)
      observer.disconnect()
    }
  }, [updateScrollState])

  // Scroll active tab into view when it changes
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const activeButton = el.querySelector(`[data-state="active"]`) as HTMLElement | null
    if (activeButton) {
      const containerRect = el.getBoundingClientRect()
      const buttonRect = activeButton.getBoundingClientRect()
      const offsetLeft = buttonRect.left - containerRect.left + el.scrollLeft
      // Center the active tab in the visible area
      const scrollTarget = offsetLeft - (containerRect.width / 2) + (buttonRect.width / 2)
      el.scrollTo({ left: scrollTarget, behavior: 'smooth' })
    }
  }, [activeTab])

  const scroll = useCallback((direction: 'left' | 'right') => {
    const el = scrollRef.current
    if (!el) return
    const scrollAmount = el.clientWidth * 0.6
    el.scrollBy({
      left: direction === 'left' ? -scrollAmount : scrollAmount,
      behavior: 'smooth',
    })
  }, [])

  return (
    <div className="relative mb-6">
      {/* Left scroll arrow */}
      {canScrollLeft && (
        <button
          onClick={() => scroll('left')}
          className="absolute left-0 top-0 z-10 flex h-full w-8 items-center justify-center bg-gradient-to-r from-background to-transparent"
          aria-label="Scroll tabs left"
        >
          <ChevronLeft className="h-4 w-4 text-muted-foreground" />
        </button>
      )}

      {/* Scrollable tabs container */}
      <div
        ref={scrollRef}
        className="overflow-x-auto scrollbar-none"
        style={{ WebkitOverflowScrolling: 'touch' }}
      >
        <TabsList className="w-max flex-nowrap">
          {children}
        </TabsList>
      </div>

      {/* Right scroll arrow */}
      {canScrollRight && (
        <button
          onClick={() => scroll('right')}
          className="absolute right-0 top-0 z-10 flex h-full w-8 items-center justify-center bg-gradient-to-l from-background to-transparent"
          aria-label="Scroll tabs right"
        >
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        </button>
      )}
    </div>
  )
}

function AdminPageContent() {
  const searchParams = useSearchParams()
  const tabParam = searchParams.get('tab')
  const initialTab = isValidTab(tabParam) ? tabParam : 'dashboard'
  const [activeTab, setActiveTab] = useState<string>(initialTab)
  const { user, isLoading, isAuthenticated } = useAuthContext()
  const isAdmin = !!user?.is_admin
  const router = useRouter()

  // Sync tab state when URL search params change (e.g. from Cmd+K navigation)
  useEffect(() => {
    const newTab = searchParams.get('tab')
    if (isValidTab(newTab) && newTab !== activeTab) {
      setActiveTab(newTab)
    }
  }, [searchParams]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleTabChange = useCallback((value: string) => {
    setActiveTab(value)
    // Update URL without full navigation so the tab is bookmarkable
    const url = value === 'dashboard' ? '/admin' : `/admin?tab=${value}`
    router.replace(url, { scroll: false })
  }, [router])

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) {
      router.replace('/auth')
    } else if (!isAdmin) {
      router.replace('/')
    }
  }, [isLoading, isAuthenticated, isAdmin, router])

  const {
    data: pendingShowsData,
  } = usePendingShows({ enabled: isAdmin })
  const {
    data: venueEditsData,
  } = usePendingVenueEdits()
  const {
    data: unverifiedVenuesData,
  } = useUnverifiedVenues({ enabled: isAdmin })
  const {
    data: reportsData,
  } = usePendingReports()
  const {
    data: artistReportsData,
  } = usePendingArtistReports()
  const {
    data: pendingEditsData,
  } = useAdminPendingEdits({ status: 'pending' })
  const {
    data: entityReportsData,
  } = useAdminEntityReports({ status: 'pending' })
  const {
    data: pendingCommentsData,
  } = useAdminPendingComments()

  const moderationCount = (pendingEditsData?.total || 0) + (entityReportsData?.total || 0) + (pendingCommentsData?.total || 0)

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
            <h1 className="text-2xl font-bold tracking-tight">Admin Console</h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Manage pending submissions, venues, and users.
          </p>
        </div>

        {/* Tabs */}
        <Tabs value={activeTab} onValueChange={handleTabChange} className="w-full">
          <ScrollableTabBar activeTab={activeTab}>
            <TabsTrigger value="dashboard" className="gap-2">
              <LayoutDashboard className="h-4 w-4" />
              Dashboard
            </TabsTrigger>
            <TabsTrigger value="moderation" className="gap-2">
              <ShieldCheck className="h-4 w-4" />
              Moderation
              {moderationCount > 0 && (
                  <span className="ml-1 rounded-full bg-purple-500 px-2 py-0.5 text-xs font-medium text-white">
                    {moderationCount}
                  </span>
                )}
            </TabsTrigger>
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
              {((reportsData?.total || 0) + (artistReportsData?.total || 0)) > 0 && (
                  <span className="ml-1 rounded-full bg-red-500 px-2 py-0.5 text-xs font-medium text-white">
                    {(reportsData?.total || 0) + (artistReportsData?.total || 0)}
                  </span>
                )}
            </TabsTrigger>
            <TabsTrigger value="import-show" className="gap-2">
              <Upload className="h-4 w-4" />
              Import Show
            </TabsTrigger>
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
            <TabsTrigger value="pipeline" className="gap-2">
              <Workflow className="h-4 w-4" />
              Data Pipeline
            </TabsTrigger>
            <TabsTrigger value="collections" className="gap-2">
              <Library className="h-4 w-4" />
              Collections
            </TabsTrigger>
            <TabsTrigger value="tags" className="gap-2">
              <Tags className="h-4 w-4" />
              Tags
            </TabsTrigger>
            <TabsTrigger value="data-quality" className="gap-2">
              <ClipboardCheck className="h-4 w-4" />
              Data Quality
            </TabsTrigger>
            <TabsTrigger value="analytics" className="gap-2">
              <BarChart3 className="h-4 w-4" />
              Analytics
            </TabsTrigger>
            <TabsTrigger value="artists-admin" className="gap-2">
              <Music className="h-4 w-4" />
              Artists
            </TabsTrigger>
            <TabsTrigger value="radio" className="gap-2">
              <Radio className="h-4 w-4" />
              Radio
            </TabsTrigger>
            <TabsTrigger value="users" className="gap-2">
              <Users className="h-4 w-4" />
              Users
            </TabsTrigger>
            <TabsTrigger value="audit-log" className="gap-2">
              <ScrollText className="h-4 w-4" />
              Audit Log
            </TabsTrigger>
          </ScrollableTabBar>

          <TabsContent value="dashboard" className="space-y-4" data-testid="admin-tab-dashboard">
            <DashboardPage onNavigate={handleTabChange} />
          </TabsContent>

          <TabsContent value="moderation" className="space-y-4" data-testid="admin-tab-moderation">
            <ModerationPage />
          </TabsContent>

          <TabsContent value="pending-shows" className="space-y-4" data-testid="admin-tab-pending-shows">
            <PendingShowsPage />
          </TabsContent>

          <TabsContent value="pending-venue-edits" className="space-y-4" data-testid="admin-tab-pending-venue-edits">
            <VenueEditsPage />
          </TabsContent>

          <TabsContent value="unverified-venues" className="space-y-4" data-testid="admin-tab-unverified-venues">
            <UnverifiedVenuesPage />
          </TabsContent>

          <TabsContent value="reports" className="space-y-4" data-testid="admin-tab-reports">
            <ReportsPage />
          </TabsContent>

          <TabsContent value="import-show" className="space-y-4" data-testid="admin-tab-import-show">
            <ShowImportPanel />
          </TabsContent>

          <TabsContent value="releases" className="space-y-4" data-testid="admin-tab-releases">
            <ReleasesPage />
          </TabsContent>

          <TabsContent value="labels" className="space-y-4" data-testid="admin-tab-labels">
            <LabelsPage />
          </TabsContent>

          <TabsContent value="festivals" className="space-y-4" data-testid="admin-tab-festivals">
            <FestivalsPage />
          </TabsContent>

          <TabsContent value="pipeline" className="space-y-4" data-testid="admin-tab-pipeline">
            <PipelineVenuesComponent />
          </TabsContent>

          <TabsContent value="collections" className="space-y-4" data-testid="admin-tab-collections">
            <CollectionManagementComponent />
          </TabsContent>

          <TabsContent value="tags" className="space-y-4" data-testid="admin-tab-tags">
            <TagsPage />
          </TabsContent>

          <TabsContent value="data-quality" className="space-y-4" data-testid="admin-tab-data-quality">
            <DataQualityPage />
          </TabsContent>

          <TabsContent value="analytics" className="space-y-4" data-testid="admin-tab-analytics">
            <AnalyticsPage />
          </TabsContent>

          <TabsContent value="artists-admin" className="space-y-4" data-testid="admin-tab-artists-admin">
            <ArtistsPage />
          </TabsContent>

          <TabsContent value="radio" className="space-y-4" data-testid="admin-tab-radio">
            <RadioPage />
          </TabsContent>

          <TabsContent value="users" className="space-y-4" data-testid="admin-tab-users">
            <UsersPage />
          </TabsContent>

          <TabsContent value="audit-log" className="space-y-4" data-testid="admin-tab-audit-log">
            <AuditLogPage />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}

export default function AdminPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <AdminPageContent />
    </Suspense>
  )
}
