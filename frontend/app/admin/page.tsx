'use client'

import { useState } from 'react'
import dynamic from 'next/dynamic'
import { Shield, MapPin, Loader2, Upload, BadgeCheck, Flag, ScrollText, Users } from 'lucide-react'
import { usePendingVenueEdits } from '@/lib/hooks/useAdminVenueEdits'
import { useUnverifiedVenues } from '@/lib/hooks/useAdminVenues'
import { usePendingReports } from '@/lib/hooks/useAdminReports'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

// Dynamic imports for heavy components - only loaded when their tab is active
const ShowImportPanel = dynamic(
  () => import('@/components/admin/ShowImportPanel').then(m => m.ShowImportPanel),
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

const UsersPage = dynamic(() => import('./users/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('pending-venue-edits')

  const {
    data: venueEditsData,
  } = usePendingVenueEdits()
  const {
    data: unverifiedVenuesData,
  } = useUnverifiedVenues()
  const {
    data: reportsData,
  } = usePendingReports()

  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
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
        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList className="mb-6">
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
              {reportsData?.total !== undefined &&
                reportsData.total > 0 && (
                  <span className="ml-1 rounded-full bg-red-500 px-2 py-0.5 text-xs font-medium text-white">
                    {reportsData.total}
                  </span>
                )}
            </TabsTrigger>
            <TabsTrigger value="import-show" className="gap-2">
              <Upload className="h-4 w-4" />
              Import Show
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

          <TabsContent value="pending-venue-edits" className="space-y-4">
            <VenueEditsPage />
          </TabsContent>

          <TabsContent value="unverified-venues" className="space-y-4">
            <UnverifiedVenuesPage />
          </TabsContent>

          <TabsContent value="reports" className="space-y-4">
            <ReportsPage />
          </TabsContent>

          <TabsContent value="import-show" className="space-y-4">
            <ShowImportPanel />
          </TabsContent>

          <TabsContent value="users" className="space-y-4">
            <UsersPage />
          </TabsContent>

          <TabsContent value="audit-log" className="space-y-4">
            <AuditLogPage />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
