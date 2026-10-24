'use client'

import Link from 'next/link'
import { Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { HomeSectionList, type HomeVisibilityChange } from './HomeSectionList'
import { PRIMARY_LINK_CLASS, ResolvedHomeCityShowsLink } from './HomeCityShowsLink'
import { DISCOVER_ROW_CLASS, HomeDiscoverLinks } from './HomeDiscoverLinks'
import {
  HOME_LAYOUT_SETTINGS_HREF,
  HOME_SECTIONS,
  type HomeLayoutDocument,
} from '../sections'

/**
 * The always-present control row above the signed-in home's sections.
 *
 * It is a row of its own rather than a gear on a section header: the popover
 * anchors here, and this row never moves, so nothing under the cursor jumps
 * when the page reorders behind it. Anonymous viewers never reach this
 * component: the server picks the signed-in variant before paint.
 *
 * The popover is used at every width: capped to the viewport with a gutter so
 * it fits at 390px without the modal weight of a sheet, and capped to the
 * available HEIGHT so a short viewport (a landscape phone, a small window, or
 * simply more sections than five) scrolls the rows rather than putting the
 * footer out of reach.
 */
export function CustomizeHomeToolbar({
  open,
  onOpenChange,
  initialLayout,
  onBeforeChange,
  withCityLink,
  withDiscoverLinks,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialLayout?: HomeLayoutDocument | null
  onBeforeChange?: (change: HomeVisibilityChange) => void
  /** Last stop for the "All upcoming shows in {city} →" link, taken when both
   *  the nearby section and the saved-shows module are hidden. */
  withCityLink: boolean
  /** The Discover row's stop on the same layout, while at least one other
   *  section still renders. */
  withDiscoverLinks: boolean
}) {
  return (
    <div
      className={cn(
        'flex flex-wrap justify-between gap-x-4 gap-y-2 pt-2',
        // A two-line cluster tops out level with the button rather than
        // centring on it.
        withDiscoverLinks ? 'items-start' : 'items-center'
      )}
    >
      <div
        className={cn(
          'flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1',
          // A content-sized cluster carrying the Discover row would claim the
          // row's full one-line width and push the button below it at every
          // width. A 20rem basis keeps the button beside the cluster wherever
          // both fit (the row wraps inside the cluster instead) and lets it
          // drop to its own line on a phone.
          withDiscoverLinks && 'grow basis-80'
        )}
      >
        <p className="font-mono text-[11px] uppercase tracking-[1.4px] text-muted-foreground">
          Home · {HOME_SECTIONS.length} sections · Your layout
        </p>
        {withCityLink && <ResolvedHomeCityShowsLink />}
        {withDiscoverLinks && (
          <HomeDiscoverLinks className={cn(DISCOVER_ROW_CLASS, 'basis-full pt-1')} />
        )}
      </div>

      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm">
            <Settings className="h-4 w-4" aria-hidden="true" />
            Customize home
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="flex max-h-[var(--radix-popover-content-available-height)] w-[min(24rem,calc(100vw-2rem))] flex-col overflow-y-auto"
          aria-label="Customize home"
        >
          <div className="px-4 pb-3 pt-4">
            <p className="font-mono text-[11px] uppercase tracking-[1.4px] text-muted-foreground">
              Customize home
            </p>
            <p className="mt-1.5 text-sm text-foreground">
              Show, hide, and reorder the sections on your home page. Saved on
              your account.
            </p>
          </div>
          <HomeSectionList
            initialLayout={initialLayout}
            onBeforeChange={onBeforeChange}
            className="border-t border-border"
            footerAction={
              <Link href={HOME_LAYOUT_SETTINGS_HREF} className={PRIMARY_LINK_CLASS}>
                All settings →
              </Link>
            }
          />
        </PopoverContent>
      </Popover>
    </div>
  )
}
