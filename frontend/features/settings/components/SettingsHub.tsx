'use client'

import { useCallback, useId, useRef } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
// Concrete module paths, not the `@/features/auth` barrel: that barrel is
// root-layout reachable and a `'use client'` barrel is not tree-shaken per
// export. See features/sharedChunkBarrelGuard.test.ts.
import { HomeLayoutSettingsList } from '@/features/auth/components/settings/home-layout'
import { SETTINGS_ANCHOR_SCROLL_MT } from '@/features/auth/components/settings/useAnchorScroll'
import {
  SETTINGS_SECTIONS,
  type SettingsLinkRow,
  type SettingsRow,
  type SettingsSection,
} from '../sections'
import { jumpToAnchor } from '../anchorTargets'
import { useActiveSection } from '../hooks/useActiveSection'
import { useFragmentLanding } from '../hooks/useFragmentLanding'
import { SettingsJumpIndex, SettingsRail } from './SettingsNav'

const SECTION_ANCHORS: readonly string[] = SETTINGS_SECTIONS.map(
  section => section.anchor
)

const ROW_CLASS =
  'flex flex-col items-start gap-2 border-t border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4'

/** Classes every fragment target carries: clear of the TopBar, and focusable
 *  on landing without a focus ring on a whole section. */
const ANCHOR_TARGET_CLASS = cn(SETTINGS_ANCHOR_SCROLL_MT, 'focus:outline-none')

function SettingsLinkRowView({ row }: { row: SettingsLinkRow }) {
  // Several rows share an action label, so each link is described by the
  // settings its row names.
  const coversId = useId()

  return (
    <div
      id={row.anchor}
      tabIndex={row.anchor ? -1 : undefined}
      className={cn(ROW_CLASS, row.anchor && ANCHOR_TARGET_CLASS)}
    >
      <p id={coversId} className="text-sm text-foreground">
        {row.covers.join(' · ')}
      </p>
      <Button asChild variant="outline" size="sm" className="shrink-0">
        <Link href={row.href} aria-describedby={coversId}>
          {row.action}
        </Link>
      </Button>
    </div>
  )
}

function SettingsRowView({ row }: { row: SettingsRow }) {
  if (row.kind === 'home-layout') return <HomeLayoutSettingsList />
  return <SettingsLinkRowView row={row} />
}

function SettingsSectionView({ section }: { section: SettingsSection }) {
  const titleId = `settings-${section.anchor}-title`

  return (
    <section
      id={section.anchor}
      tabIndex={-1}
      aria-labelledby={titleId}
      className={cn(
        ANCHOR_TARGET_CLASS,
        'rounded-lg border border-border bg-card text-card-foreground'
      )}
    >
      <header className="px-4 py-4">
        <h2 id={titleId} className="text-lg font-medium">
          {section.title}
        </h2>
        {section.blurb && (
          <p className="mt-1 text-sm text-muted-foreground">{section.blurb}</p>
        )}
      </header>
      {section.rows.map((row, index) => (
        <SettingsRowView key={index} row={row} />
      ))}
    </section>
  )
}

/**
 * The `/settings` hub: every section on one page, reached by fragment.
 * Desktop pairs a sticky rail with the sections; below `lg` a jump index
 * heads the stacked sections instead.
 */
export function SettingsHub() {
  const hubRef = useRef<HTMLDivElement>(null)
  const { activeAnchor, select } = useActiveSection(SECTION_ANCHORS, hubRef)
  useFragmentLanding(hubRef)
  const jump = useCallback(
    (anchor: string) => {
      jumpToAnchor(hubRef.current, anchor)
      select(anchor)
    },
    [select]
  )

  return (
    <div
      ref={hubRef}
      className="container mx-auto max-w-7xl px-4 py-6 lg:px-8 lg:py-10"
    >
      <div className="lg:grid lg:grid-cols-[15rem_minmax(0,1fr)] lg:items-start lg:gap-10">
        <div className="lg:sticky lg:top-[calc(var(--topbar-height)+1rem)]">
          <header className="mb-4">
            <p className="font-mono text-[11px] text-primary">/settings</p>
            <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
          </header>
          <SettingsRail
            activeAnchor={activeAnchor}
            onJump={jump}
            className="hidden lg:block"
          />
        </div>

        <div className="flex flex-col gap-6">
          <SettingsJumpIndex onJump={jump} className="lg:hidden" />
          {SETTINGS_SECTIONS.map(section => (
            <SettingsSectionView key={section.anchor} section={section} />
          ))}
        </div>
      </div>
    </div>
  )
}
