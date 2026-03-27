'use client'

import { useState, useEffect, useCallback, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Shield, MapPin, Loader2, Upload, BadgeCheck, Flag, ScrollText, Users, LayoutDashboard, Clock, Disc3, Tag, Tags, Tent, Workflow, Library, Music, ClipboardCheck, BarChart3 } from 'lucide-react'
import { usePendingVenueEdits } from '@/lib/hooks/admin/useAdminVenueEdits'
import { useUnverifiedVenues } from '@/lib/hooks/admin/useAdminVenues'
import { usePendingReports } from '@/lib/hooks/admin/useAdminReports'
import { usePendingArtistReports } from '@/lib/hooks/admin/useAdminArtistReports'
import { usePendingShows } from '@/lib/hooks/admin/useAdminShows'
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

const CrateManagementComponent = dynamic(
  () => import('@/components/admin/CrateManagement').then(m => ({ default: m.CrateManagement })),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const VALID_TABS = [
  'dashboard', 'pending-shows', 'pending-venue-edits', 'unverified-venues',
  'reports', 'import-show', 'releases', 'labels', 'festivals', 'pipeline',
  'crates', 'tags', 'data-quality', 'analytics', 'artists-admin',
  'users', 'audit-log',
] as const

type AdminTab = (typeof VALID_TABS)[number]

function isValidTab(value: string | null): value is AdminTab {
  return value !== null && (VALID_TABS as readonly string[]).includes(value)
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
          <TabsList className="mb-6">
            <TabsTrigger value="dashboard" className="gap-2">
              <LayoutDashboard className="h-4 w-4" />
              Dashboard
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
            <TabsTrigger value="crates" className="gap-2">
              <Library className="h-4 w-4" />
              Crates
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
            <TabsTrigger value="users" className="gap-2">
              <Users className="h-4 w-4" />
              Users
            </TabsTrigger>
            <TabsTrigger value="audit-log" className="gap-2">
              <ScrollText className="h-4 w-4" />
              Audit Log
            </TabsTrigger>
          </TabsList>

          <TabsContent value="dashboard" className="space-y-4" data-testid="admin-tab-dashboard">
            <DashboardPage onNavigate={setActiveTab} />
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

          <TabsContent value="crates" className="space-y-4" data-testid="admin-tab-crates">
            <CrateManagementComponent />
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
