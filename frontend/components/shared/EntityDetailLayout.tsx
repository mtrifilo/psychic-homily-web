'use client'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { Breadcrumb } from './Breadcrumb'
import type { BreadcrumbEntry } from '@/lib/context/NavigationBreadcrumbContext'

export interface EntityDetailTab {
  value: string
  label: string
}

export interface EntityDetailBackLink {
  href: string
  label: string
}

interface EntityDetailLayoutProps {
  /** Fallback breadcrumb entry for direct landings (e.g., { href: '/artists', label: 'Artists' }) */
  fallback: BreadcrumbEntry
  /** Name of the current entity (shown as the last breadcrumb) */
  entityName: string
  /** @deprecated Use `fallback` and `entityName` instead. Kept for backward compat. */
  backLink?: EntityDetailBackLink
  /** Header content (typically an EntityHeader) */
  header: React.ReactNode
  /** Tab definitions */
  tabs: EntityDetailTab[]
  /** Currently active tab value */
  activeTab: string
  /** Callback when tab changes */
  onTabChange: (value: string) => void
  /** Optional sidebar content */
  sidebar?: React.ReactNode
  /**
   * Tab content area. Should include TabsContent components from @/components/ui/tabs
   * for each tab value. The layout wraps everything in a Tabs provider.
   */
  children: React.ReactNode
  className?: string
}

/**
 * Reusable entity detail page layout with breadcrumb navigation, header, tabs, and optional sidebar.
 *
 * Usage:
 * ```tsx
 * <EntityDetailLayout
 *   fallback={{ href: '/releases', label: 'Releases' }}
 *   entityName="Album Name"
 *   header={<EntityHeader title="Album Name" subtitle="2024" />}
 *   tabs={[{ value: 'overview', label: 'Overview' }, { value: 'links', label: 'Listen/Buy' }]}
 *   activeTab={activeTab}
 *   onTabChange={setActiveTab}
 *   sidebar={<SidebarContent />}
 * >
 *   <TabsContent value="overview">...</TabsContent>
 *   <TabsContent value="links">...</TabsContent>
 * </EntityDetailLayout>
 * ```
 */
export function EntityDetailLayout({
  fallback,
  entityName,
  backLink,
  header,
  tabs,
  activeTab,
  onTabChange,
  sidebar,
  children,
  className,
}: EntityDetailLayoutProps) {
  // Support deprecated backLink prop as fallback
  const resolvedFallback = fallback ?? (backLink ? { href: backLink.href, label: backLink.label.replace(/^Back to /, '') } : { href: '/', label: 'Home' })
  const resolvedEntityName = entityName ?? ''

  return (
    <div className={cn('container max-w-6xl mx-auto px-4 py-6', className)}>
      {/* Breadcrumb Navigation */}
      <Breadcrumb fallback={resolvedFallback} currentPage={resolvedEntityName} />

      {/* Header */}
      <header className="mb-6">{header}</header>

      {/* Tabs + Content + Sidebar */}
      <Tabs value={activeTab} onValueChange={onTabChange}>
        <TabsList className="mb-6">
          {tabs.map(tab => (
            <TabsTrigger key={tab.value} value={tab.value}>
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>

        <div
          className={cn(
            'flex flex-col gap-8',
            sidebar && 'lg:flex-row'
          )}
        >
          {/* Main Content (tab panels) */}
          <div className="flex-1 min-w-0">{children}</div>

          {/* Sidebar */}
          {sidebar && (
            <aside className="w-full lg:w-80 shrink-0">{sidebar}</aside>
          )}
        </div>
      </Tabs>
    </div>
  )
}
