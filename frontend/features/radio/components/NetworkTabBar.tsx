'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
import { getStationDetailUrl } from '../types'
import type { RadioStationDetail } from '../types'

interface NetworkTabBarProps {
  // The station whose detail page we're currently on.
  currentStation: RadioStationDetail
}

interface TabEntry {
  slug: string
  label: string
  href: string
  isCurrent: boolean
  isFlagship: boolean
}

// PSY-674: tab bar for stations that belong to a radio network. Renders one
// tab per station in the network (current station + siblings), flagship-first.
// Each tab is a Link to the canonical /radio detail URL — no client-side
// panel swap; the URL change drives the content swap via Next.js routing.
//
// Hidden entirely when the station has no network — KEXP/NTS today render
// without a tab bar.
export function NetworkTabBar({ currentStation }: NetworkTabBarProps) {
  if (!currentStation.network) {
    return null
  }

  const tabs = buildTabs(currentStation)
  // Single-station networks (theoretical: a future network with only the
  // flagship and no siblings) shouldn't render a one-tab bar.
  if (tabs.length < 2) {
    return null
  }

  return (
    <nav
      aria-label={`${currentStation.network.name} network channels`}
      className="-mx-4 md:mx-0 mb-6 border-b border-border/50"
    >
      <ul className="flex items-end gap-1 overflow-x-auto px-4 md:px-0 scrollbar-thin">
        {tabs.map(tab => (
          <li key={tab.slug} className="shrink-0">
            <NetworkTab tab={tab} />
          </li>
        ))}
      </ul>
    </nav>
  )
}

function NetworkTab({ tab }: { tab: TabEntry }) {
  return (
    <Link
      href={tab.href}
      aria-current={tab.isCurrent ? 'page' : undefined}
      className={cn(
        'inline-flex items-center px-3 py-2 text-sm whitespace-nowrap transition-colors',
        'border-b-2 -mb-px',
        tab.isCurrent
          ? 'border-primary text-foreground font-medium'
          : 'border-transparent text-muted-foreground hover:text-foreground hover:border-border'
      )}
    >
      {tab.label}
    </Link>
  )
}

// buildTabs assembles the per-station tab list for the current station's
// network. Tab order is flagship-first then alphabetical-by-name. The
// flagship's label appends the frequency when one exists ("WFMU 91.1")
// so the active tab visually differs from the parent network H1.
function buildTabs(station: RadioStationDetail): TabEntry[] {
  if (!station.network) return []

  const entries: TabEntry[] = []

  entries.push({
    slug: station.slug,
    label: tabLabel({
      name: station.name,
      isFlagship: station.network.is_flagship,
      frequencyMHz: station.frequency_mhz,
    }),
    href: getStationDetailUrl(station.slug, station.network),
    isCurrent: true,
    isFlagship: station.network.is_flagship,
  })

  for (const sib of station.sibling_stations) {
    entries.push({
      slug: sib.slug,
      label: tabLabel({
        name: sib.name,
        isFlagship: sib.is_flagship,
        // Sibling responses don't carry frequency; flagship-sibling tabs render name-only.
        frequencyMHz: null,
      }),
      href: getStationDetailUrl(sib.slug, {
        slug: station.network.slug,
        is_flagship: sib.is_flagship,
      }),
      isCurrent: false,
      isFlagship: sib.is_flagship,
    })
  }

  entries.sort((a, b) => {
    if (a.isFlagship !== b.isFlagship) return a.isFlagship ? -1 : 1
    return a.label.localeCompare(b.label)
  })

  return entries
}

function tabLabel(args: {
  name: string
  isFlagship: boolean
  frequencyMHz: number | null
}): string {
  if (args.isFlagship && args.frequencyMHz != null) {
    return `${args.name} ${args.frequencyMHz}`
  }
  return args.name
}
