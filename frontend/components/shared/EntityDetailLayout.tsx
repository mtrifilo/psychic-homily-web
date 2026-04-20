'use client'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { Breadcrumb } from './Breadcrumb'
import type { BreadcrumbEntry } from './Breadcrumb'

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
  /**
   * Tab definitions. Omit or pass an empty array to render a flat,
   * single-surface layout (no `<Tabs>` wrapper, no `<TabsList>`).
   */
  tabs?: EntityDetailTab[]
  /** Currently active tab value. Required when `tabs` is non-empty. */
  activeTab?: string
  /** Callback when tab changes. Required when `tabs` is non-empty. */
  onTabChange?: (value: string) => void
  /** Optional sidebar content */
  sidebar?: React.ReactNode
  /**
   * Main content area. When `tabs` is non-empty, should include TabsContent
   * components from @/components/ui/tabs for each tab value — the layout
   * wraps everything in a Tabs provider. When `tabs` is empty/undefined,
   * children render directly in the main column.
   */
  children: React.ReactNode
  className?: string
}

/**
 * Reusable entity detail page layout with breadcrumb navigation, header,
 * optional tabs, and optional sidebar.
 *
 * Two shapes:
 * - Tabbed (default): pass `tabs`, `activeTab`, `onTabChange`. Children are
 *   TabsContent panels wrapped in a Tabs provider.
 * - Flat (single-surface): omit `tabs` (or pass `[]`). No tabs bar is
 *   rendered and children are laid out directly in the main column. Useful
 *   for pages like ShowDetail/VenueDetail that have a single content surface.
 *
 * Usage (tabbed):
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
 *
 * Usage (flat):
 * ```tsx
 * <EntityDetailLayout
 *   fallback={{ href: '/shows', label: 'Shows' }}
 *   entityName="Show Title"
 *   header={<ShowHeader show={show} />}
 * >
 *   <ShowSection />
 *   <AnotherSection />
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
  const hasTabs = !!tabs && tabs.length > 0

  const contentBody = (
    <div
      className={cn(
        'flex flex-col gap-8',
        sidebar && 'lg:flex-row'
      )}
    >
      {/* Main Content (tab panels when tabbed, otherwise flat children) */}
      <div className="flex-1 min-w-0">{children}</div>

      {/* Sidebar */}
      {sidebar && (
        <aside className="w-full lg:w-80 shrink-0">{sidebar}</aside>
      )}
    </div>
  )

  return (
    <div className={cn('container max-w-6xl mx-auto px-4 py-6', className)}>
      {/* Breadcrumb Navigation */}
      <Breadcrumb fallback={resolvedFallback} currentPage={resolvedEntityName} />

      {/* Header */}
      <header className="mb-6">{header}</header>

      {hasTabs ? (
        <Tabs value={activeTab} onValueChange={onTabChange}>
          <TabsList className="mb-6">
            {tabs!.map(tab => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>

          {contentBody}
        </Tabs>
      ) : (
        contentBody
      )}
    </div>
  )
}
