'use client'

import { useId, useRef } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
// Concrete module paths, not the `@/features/auth` barrel: that barrel is
// root-layout reachable and a `'use client'` barrel is not tree-shaken per
// export. See features/sharedChunkBarrelGuard.test.ts.
import { HomeLayoutSettingsList } from '@/features/auth/components/settings/home-layout'
import {
  SETTINGS_ANCHOR_SCROLL_MT,
  useAnchorScroll,
} from '@/features/auth/components/settings/useAnchorScroll'
import {
  SETTINGS_SECTIONS,
  type SettingsLinkRow as SettingsLinkRowData,
  type SettingsRow,
  type SettingsSection,
} from '../sections'
import { SettingsJumpIndex, SettingsRail } from './SettingsNav'

const ROW_CLASS =
  'flex flex-col items-start gap-2 border-t border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4'

function SettingsLinkRowContent({ row }: { row: SettingsLinkRowData }) {
  // Several rows share an action label, so each link is described by the
  // settings its row names.
  const coversId = useId()

  return (
    <>
      <p id={coversId} className="text-sm text-foreground">
        {row.covers.join(' · ')}
      </p>
      <Button asChild variant="outline" size="sm" className="shrink-0">
        <Link href={row.href} aria-describedby={coversId}>
          {row.action}
        </Link>
      </Button>
    </>
  )
}

/** A link row that is itself a fragment target inside its section. */
function AnchoredSettingsLinkRow({
  row,
  anchor,
}: {
  row: SettingsLinkRowData
  anchor: string
}) {
  const anchorRef = useAnchorScroll(anchor)

  return (
    <div
      id={anchor}
      ref={anchorRef}
      tabIndex={-1}
      className={cn(ROW_CLASS, SETTINGS_ANCHOR_SCROLL_MT, 'focus:outline-none')}
    >
      <SettingsLinkRowContent row={row} />
    </div>
  )
}

function SettingsRowView({ row }: { row: SettingsRow }) {
  if (row.kind === 'home-layout') return <HomeLayoutSettingsList />
  if (row.anchor) return <AnchoredSettingsLinkRow row={row} anchor={row.anchor} />
  return (
    <div className={ROW_CLASS}>
      <SettingsLinkRowContent row={row} />
    </div>
  )
}

function SettingsSectionView({ section }: { section: SettingsSection }) {
  const anchorRef = useAnchorScroll(section.anchor)
  const titleId = `settings-${section.anchor}-title`

  return (
    <section
      id={section.anchor}
      ref={anchorRef}
      tabIndex={-1}
      aria-labelledby={titleId}
      className={cn(
        SETTINGS_ANCHOR_SCROLL_MT,
        'rounded-lg border border-border bg-card text-card-foreground focus:outline-none'
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
          <SettingsRail rootRef={hubRef} className="hidden lg:block" />
        </div>

        <div className="flex flex-col gap-6">
          <SettingsJumpIndex rootRef={hubRef} className="lg:hidden" />
          {SETTINGS_SECTIONS.map(section => (
            <SettingsSectionView key={section.anchor} section={section} />
          ))}
        </div>
      </div>
    </div>
  )
}
