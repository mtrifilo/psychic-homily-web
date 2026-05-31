'use client'

import { useEffect, useCallback, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Shield, Loader2 } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Tabs, TabsContent } from '@/components/ui/tabs'

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
  'dashboard', 'moderation', 'pending-shows', 'unverified-venues',
  'reports', 'import-show', 'releases', 'labels', 'festivals', 'pipeline',
  'collections', 'tags', 'data-quality', 'analytics', 'artists-admin', 'radio',
  'users', 'audit-log',
] as const

type AdminTab = (typeof VALID_TABS)[number]

function isValidTab(value: string | null): value is AdminTab {
  return value !== null && (VALID_TABS as readonly string[]).includes(value)
}

function AdminPageContent() {
  const searchParams = useSearchParams()
  const tabParam = searchParams.get('tab')
  // The URL ?tab= param is the single source of truth for the active section —
  // the context-aware Sidebar / mobile drawer (PSY-933) navigate by setting it,
  // and it's deep-linkable. Deriving activeTab from it (rather than mirroring it
  // into useState + a sync effect) keeps the two from desyncing and avoids the
  // react-hooks/set-state-in-effect cascade.
  const activeTab: string = isValidTab(tabParam) ? tabParam : 'dashboard'
  const { user, isLoading, isAuthenticated } = useAuthContext()
  const isAdmin = !!user?.is_admin
  const router = useRouter()

  // Programmatic tab switches (e.g. DashboardPage quick-links) update the URL;
  // activeTab re-derives from it on the next render.
  const handleTabChange = useCallback((value: string) => {
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
        <Tabs value={activeTab} className="w-full">
          {/* Section navigation lives in the context-aware Sidebar / mobile
              drawer (PSY-933) — the old horizontal ScrollableTabBar (18 tabs,
              past NN/G's 3–6 limit) was retired. Tabs stay value-controlled:
              sidebar links set ?tab=, the searchParams effect syncs activeTab. */}

          <TabsContent value="dashboard" className="space-y-4" data-testid="admin-tab-dashboard">
            <DashboardPage onNavigate={handleTabChange} />
          </TabsContent>

          <TabsContent value="moderation" className="space-y-4" data-testid="admin-tab-moderation">
            <ModerationPage />
          </TabsContent>

          <TabsContent value="pending-shows" className="space-y-4" data-testid="admin-tab-pending-shows">
            <PendingShowsPage />
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
