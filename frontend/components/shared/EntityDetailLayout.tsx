'use client'

import Link from 'next/link'
import { ArrowLeft } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

export interface EntityDetailTab {
  value: string
  label: string
}

export interface EntityDetailBackLink {
  href: string
  label: string
}

interface EntityDetailLayoutProps {
  /** Back navigation link */
  backLink: EntityDetailBackLink
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
 * Reusable entity detail page layout with back link, header, tabs, and optional sidebar.
 *
 * Usage:
 * ```tsx
 * <EntityDetailLayout
 *   backLink={{ href: '/releases', label: 'Back to Releases' }}
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
  backLink,
  header,
  tabs,
  activeTab,
  onTabChange,
  sidebar,
  children,
  className,
}: EntityDetailLayoutProps) {
  return (
    <div className={cn('container max-w-6xl mx-auto px-4 py-6', className)}>
      {/* Back Navigation */}
      <div className="mb-6">
        <Link
          href={backLink.href}
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          {backLink.label}
        </Link>
      </div>

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
