'use client'

import { useState } from 'react'
import { Loader2, TrendingUp, Heart, Users, ShieldCheck } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  useGrowthMetrics,
  useEngagementMetrics,
  useCommunityHealth,
  useDataQualityTrends,
} from '@/lib/hooks/admin/useAnalytics'
import type {
  MonthlyCount,
  TopContributor,
  WeeklyContribution,
} from '@/lib/hooks/admin/useAnalytics'
import {
  ResponsiveContainer,
  LineChart,
  Line,
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts'

// --- Constants ---

type MonthRange = 3 | 6 | 12 | 24
const MONTH_OPTIONS: MonthRange[] = [3, 6, 12, 24]

// Chart color palette — distinct, accessible colors
const COLORS = {
  shows: '#3b82f6', // blue-500
  artists: '#8b5cf6', // violet-500
  venues: '#f97316', // orange-500
  releases: '#10b981', // emerald-500
  labels: '#ec4899', // pink-500
  users: '#6366f1', // indigo-500
  // Engagement
  bookmarks: '#3b82f6',
  tags_added: '#8b5cf6',
  tag_votes: '#a78bfa',
  crate_items: '#f97316',
  requests: '#10b981',
  request_votes: '#34d399',
  revisions: '#ec4899',
  follows: '#6366f1',
  attendance: '#f59e0b',
  // Data quality
  approved: '#10b981',
  rejected: '#ef4444',
}

// --- Sub-section labels ---
type AnalyticsView = 'growth' | 'engagement' | 'community' | 'data-quality'

const VIEW_CONFIG: { key: AnalyticsView; label: string; icon: typeof TrendingUp }[] = [
  { key: 'growth', label: 'Growth', icon: TrendingUp },
  { key: 'engagement', label: 'Engagement', icon: Heart },
  { key: 'community', label: 'Community Health', icon: Users },
  { key: 'data-quality', label: 'Data Quality', icon: ShieldCheck },
]

// --- Helpers ---

/** Format "2026-03" → "Mar 2026" */
function formatMonth(month: string): string {
  const [year, mon] = month.split('-')
  const date = new Date(Number(year), Number(mon) - 1)
  return date.toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
}

/** Format "2026-W11" → "W11" */
function formatWeek(week: string): string {
  const match = week.match(/W(\d+)/)
  return match ? `W${match[1]}` : week
}

/** Format float 0-1 → "72%" */
function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`
}

/** Build unified chart data from multiple series keyed by month */
function mergeMonthlyData(
  series: Record<string, MonthlyCount[]>
): Record<string, string | number>[] {
  const monthMap = new Map<string, Record<string, string | number>>()
  for (const [key, data] of Object.entries(series)) {
    for (const item of data) {
      if (!monthMap.has(item.month)) {
        monthMap.set(item.month, { month: formatMonth(item.month) })
      }
      monthMap.get(item.month)![key] = item.count
    }
  }
  // Sort chronologically
  const entries = Array.from(monthMap.entries())
  entries.sort(([a], [b]) => a.localeCompare(b))
  return entries.map(([, v]) => v)
}

// --- Stat Card Component ---

function StatCard({
  label,
  value,
  description,
}: {
  label: string
  value: string | number
  description?: string
}) {
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-sm font-medium text-muted-foreground">{label}</p>
        <p className="mt-1 text-2xl font-bold tabular-nums">{value}</p>
        {description && (
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        )}
      </CardContent>
    </Card>
  )
}

// --- Month Range Selector ---

function MonthRangeSelector({
  value,
  onChange,
}: {
  value: MonthRange
  onChange: (m: MonthRange) => void
}) {
  return (
    <div className="flex items-center gap-1">
      <span className="mr-2 text-sm text-muted-foreground">Range:</span>
      {MONTH_OPTIONS.map((m) => (
        <Button
          key={m}
          variant={value === m ? 'default' : 'outline'}
          size="sm"
          onClick={() => onChange(m)}
          className="h-7 px-2.5 text-xs"
        >
          {m}mo
        </Button>
      ))}
    </div>
  )
}

// --- Chart Wrapper ---

function ChartCard({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-72">{children}</div>
      </CardContent>
    </Card>
  )
}

// --- Growth Section ---

function GrowthSection({ months }: { months: MonthRange }) {
  const { data, isLoading, error } = useGrowthMetrics(months)

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load growth metrics." />
  if (!data) return null

  const chartData = mergeMonthlyData({
    shows: data.shows,
    artists: data.artists,
    venues: data.venues,
    releases: data.releases,
    labels: data.labels,
    users: data.users,
  })

  return (
    <div className="space-y-4">
      <ChartCard title="Entity Creation Trends">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
            <XAxis
              dataKey="month"
              tick={{ fontSize: 12 }}
              className="fill-muted-foreground"
            />
            <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
            <Tooltip
              contentStyle={{
                backgroundColor: 'hsl(var(--popover))',
                border: '1px solid hsl(var(--border))',
                borderRadius: '0.5rem',
                color: 'hsl(var(--popover-foreground))',
              }}
            />
            <Legend />
            <Line
              type="monotone"
              dataKey="shows"
              stroke={COLORS.shows}
              strokeWidth={2}
              dot={false}
              name="Shows"
            />
            <Line
              type="monotone"
              dataKey="artists"
              stroke={COLORS.artists}
              strokeWidth={2}
              dot={false}
              name="Artists"
            />
            <Line
              type="monotone"
              dataKey="venues"
              stroke={COLORS.venues}
              strokeWidth={2}
              dot={false}
              name="Venues"
            />
            <Line
              type="monotone"
              dataKey="releases"
              stroke={COLORS.releases}
              strokeWidth={2}
              dot={false}
              name="Releases"
            />
            <Line
              type="monotone"
              dataKey="labels"
              stroke={COLORS.labels}
              strokeWidth={2}
              dot={false}
              name="Labels"
            />
            <Line
              type="monotone"
              dataKey="users"
              stroke={COLORS.users}
              strokeWidth={2}
              dot={false}
              name="Users"
            />
          </LineChart>
        </ResponsiveContainer>
      </ChartCard>
    </div>
  )
}

// --- Engagement Section ---

function EngagementSection({ months }: { months: MonthRange }) {
  const { data, isLoading, error } = useEngagementMetrics(months)

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load engagement metrics." />
  if (!data) return null

  // Group 1: Content curation (tags + crates)
  const curationData = mergeMonthlyData({
    tags_added: data.tags_added,
    tag_votes: data.tag_votes,
    crate_items: data.collection_items, // API field is collection_items
  })

  // Group 2: Requests & voting
  const requestsData = mergeMonthlyData({
    requests: data.requests,
    request_votes: data.request_votes,
  })

  // Group 3: Social engagement
  const socialData = mergeMonthlyData({
    bookmarks: data.bookmarks,
    follows: data.follows,
    attendance: data.attendance,
    revisions: data.revisions,
  })

  return (
    <div className="space-y-4">
      <ChartCard title="Content Curation">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={curationData}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
            <XAxis
              dataKey="month"
              tick={{ fontSize: 12 }}
              className="fill-muted-foreground"
            />
            <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
            <Tooltip
              contentStyle={{
                backgroundColor: 'hsl(var(--popover))',
                border: '1px solid hsl(var(--border))',
                borderRadius: '0.5rem',
                color: 'hsl(var(--popover-foreground))',
              }}
            />
            <Legend />
            <Area
              type="monotone"
              dataKey="tags_added"
              stroke={COLORS.tags_added}
              fill={COLORS.tags_added}
              fillOpacity={0.15}
              strokeWidth={2}
              name="Tags Added"
            />
            <Area
              type="monotone"
              dataKey="tag_votes"
              stroke={COLORS.tag_votes}
              fill={COLORS.tag_votes}
              fillOpacity={0.15}
              strokeWidth={2}
              name="Tag Votes"
            />
            <Area
              type="monotone"
              dataKey="crate_items"
              stroke={COLORS.crate_items}
              fill={COLORS.crate_items}
              fillOpacity={0.15}
              strokeWidth={2}
              name="Crate Items"
            />
          </AreaChart>
        </ResponsiveContainer>
      </ChartCard>

      <div className="grid gap-4 md:grid-cols-2">
        <ChartCard title="Requests & Voting">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={requestsData}>
              <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
              <XAxis
                dataKey="month"
                tick={{ fontSize: 12 }}
                className="fill-muted-foreground"
              />
              <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'hsl(var(--popover))',
                  border: '1px solid hsl(var(--border))',
                  borderRadius: '0.5rem',
                  color: 'hsl(var(--popover-foreground))',
                }}
              />
              <Legend />
              <Area
                type="monotone"
                dataKey="requests"
                stroke={COLORS.requests}
                fill={COLORS.requests}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Requests"
              />
              <Area
                type="monotone"
                dataKey="request_votes"
                stroke={COLORS.request_votes}
                fill={COLORS.request_votes}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Request Votes"
              />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title="Social Engagement">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={socialData}>
              <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
              <XAxis
                dataKey="month"
                tick={{ fontSize: 12 }}
                className="fill-muted-foreground"
              />
              <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'hsl(var(--popover))',
                  border: '1px solid hsl(var(--border))',
                  borderRadius: '0.5rem',
                  color: 'hsl(var(--popover-foreground))',
                }}
              />
              <Legend />
              <Area
                type="monotone"
                dataKey="bookmarks"
                stroke={COLORS.bookmarks}
                fill={COLORS.bookmarks}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Bookmarks"
              />
              <Area
                type="monotone"
                dataKey="follows"
                stroke={COLORS.follows}
                fill={COLORS.follows}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Follows"
              />
              <Area
                type="monotone"
                dataKey="attendance"
                stroke={COLORS.attendance}
                fill={COLORS.attendance}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Attendance"
              />
              <Area
                type="monotone"
                dataKey="revisions"
                stroke={COLORS.revisions}
                fill={COLORS.revisions}
                fillOpacity={0.15}
                strokeWidth={2}
                name="Revisions"
              />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>
      </div>
    </div>
  )
}

// --- Community Health Section ---

function CommunityHealthSection() {
  const { data, isLoading, error } = useCommunityHealth()

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load community health." />
  if (!data) return null

  const weeklyData = data.contributions_per_week.map(
    (w: WeeklyContribution) => ({
      week: formatWeek(w.week),
      count: w.count,
    })
  )

  return (
    <div className="space-y-4">
      {/* Stat cards */}
      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard
          label="Active Contributors (30d)"
          value={data.active_contributors_30d}
          description="Users with at least 1 contribution in the last 30 days"
        />
        <StatCard
          label="Request Fulfillment Rate"
          value={formatPercent(data.request_fulfillment_rate)}
          description="Percentage of requests that have been fulfilled"
        />
        <StatCard
          label="New Crates (30d)"
          value={data.new_collections_30d}
          description="Crates created in the last 30 days"
        />
      </div>

      {/* Weekly contributions chart */}
      <ChartCard title="Weekly Contributions (Last 12 Weeks)">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={weeklyData}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
            <XAxis
              dataKey="week"
              tick={{ fontSize: 12 }}
              className="fill-muted-foreground"
            />
            <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
            <Tooltip
              contentStyle={{
                backgroundColor: 'hsl(var(--popover))',
                border: '1px solid hsl(var(--border))',
                borderRadius: '0.5rem',
                color: 'hsl(var(--popover-foreground))',
              }}
            />
            <Bar
              dataKey="count"
              fill={COLORS.shows}
              radius={[4, 4, 0, 0]}
              name="Contributions"
            />
          </BarChart>
        </ResponsiveContainer>
      </ChartCard>

      {/* Top contributors table */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">
            Top Contributors (30d)
          </CardTitle>
        </CardHeader>
        <CardContent>
          {data.top_contributors.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No contributions in the last 30 days.
            </p>
          ) : (
            <div className="divide-y divide-border">
              {data.top_contributors.map(
                (contributor: TopContributor, index: number) => (
                  <div
                    key={contributor.user_id}
                    className="flex items-center justify-between py-2.5"
                  >
                    <div className="flex items-center gap-3">
                      <span className="flex h-6 w-6 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground">
                        {index + 1}
                      </span>
                      <div>
                        <p className="text-sm font-medium">
                          {contributor.display_name || contributor.username}
                        </p>
                        {contributor.display_name && (
                          <p className="text-xs text-muted-foreground">
                            @{contributor.username}
                          </p>
                        )}
                      </div>
                    </div>
                    <Badge variant="secondary" className="tabular-nums">
                      {contributor.count} contribution
                      {contributor.count !== 1 ? 's' : ''}
                    </Badge>
                  </div>
                )
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

// --- Data Quality Trends Section ---

function DataQualityTrendsSection({ months }: { months: MonthRange }) {
  const { data, isLoading, error } = useDataQualityTrends(months)

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load data quality trends." />
  if (!data) return null

  const chartData = mergeMonthlyData({
    approved: data.shows_approved,
    rejected: data.shows_rejected,
  })

  return (
    <div className="space-y-4">
      {/* Snapshot stat cards */}
      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard
          label="Pending Review"
          value={data.pending_review_count}
          description="Shows awaiting admin review"
        />
        <StatCard
          label="Artists Without Releases"
          value={data.artists_without_releases}
          description="Artists with no linked releases"
        />
        <StatCard
          label="Inactive Venues (90d)"
          value={data.inactive_venues_90d}
          description="Venues with no shows in the last 90 days"
        />
      </div>

      {/* Approved vs Rejected trends */}
      <ChartCard title="Show Approval Trends">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
            <XAxis
              dataKey="month"
              tick={{ fontSize: 12 }}
              className="fill-muted-foreground"
            />
            <YAxis tick={{ fontSize: 12 }} className="fill-muted-foreground" />
            <Tooltip
              contentStyle={{
                backgroundColor: 'hsl(var(--popover))',
                border: '1px solid hsl(var(--border))',
                borderRadius: '0.5rem',
                color: 'hsl(var(--popover-foreground))',
              }}
            />
            <Legend />
            <Area
              type="monotone"
              dataKey="approved"
              stroke={COLORS.approved}
              fill={COLORS.approved}
              fillOpacity={0.2}
              strokeWidth={2}
              name="Approved"
            />
            <Area
              type="monotone"
              dataKey="rejected"
              stroke={COLORS.rejected}
              fill={COLORS.rejected}
              fillOpacity={0.2}
              strokeWidth={2}
              name="Rejected"
            />
          </AreaChart>
        </ResponsiveContainer>
      </ChartCard>
    </div>
  )
}

// --- Shared states ---

function LoadingState() {
  return (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
      <p className="text-sm text-destructive">{message}</p>
    </div>
  )
}

// --- Main Dashboard ---

export function AnalyticsDashboard() {
  const [activeView, setActiveView] = useState<AnalyticsView>('growth')
  const [months, setMonths] = useState<MonthRange>(6)

  const showMonthSelector = activeView !== 'community'

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-lg font-semibold">Analytics</h2>
        <p className="text-sm text-muted-foreground">
          Platform growth, engagement, and data health metrics.
        </p>
      </div>

      {/* View selector + month range */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-1">
          {VIEW_CONFIG.map(({ key, label, icon: Icon }) => (
            <Button
              key={key}
              variant={activeView === key ? 'default' : 'outline'}
              size="sm"
              onClick={() => setActiveView(key)}
              className="gap-1.5"
            >
              <Icon className="h-3.5 w-3.5" />
              {label}
            </Button>
          ))}
        </div>

        {showMonthSelector && (
          <MonthRangeSelector value={months} onChange={setMonths} />
        )}
      </div>

      {/* Active section */}
      {activeView === 'growth' && <GrowthSection months={months} />}
      {activeView === 'engagement' && <EngagementSection months={months} />}
      {activeView === 'community' && <CommunityHealthSection />}
      {activeView === 'data-quality' && (
        <DataQualityTrendsSection months={months} />
      )}
    </div>
  )
}
