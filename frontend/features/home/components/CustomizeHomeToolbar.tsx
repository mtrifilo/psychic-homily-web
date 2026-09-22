'use client'

import Link from 'next/link'
import { Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { HomeSectionList, type HomeVisibilityChange } from './HomeSectionList'
import { PRIMARY_LINK_CLASS, ResolvedHomeCityShowsLink } from './HomeCityShowsLink'
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
 * The popover is used at every width. At 390px it is capped to the viewport
 * with a gutter, which fits the five rows without the modal weight of a sheet.
 */
export function CustomizeHomeToolbar({
  open,
  onOpenChange,
  initialLayout,
  onBeforeChange,
  withCityLink,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialLayout?: HomeLayoutDocument | null
  onBeforeChange?: (change: HomeVisibilityChange) => void
  /** Last stop for the "All upcoming shows in {city} →" link, taken when both
   *  the nearby section and the saved-shows module are hidden. */
  withCityLink: boolean
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 pt-2">
      <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">
        <p className="font-mono text-[11px] uppercase tracking-[1.4px] text-muted-foreground">
          Home · {HOME_SECTIONS.length} sections · Your layout
        </p>
        {withCityLink && <ResolvedHomeCityShowsLink />}
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
          className="w-[min(24rem,calc(100vw-2rem))]"
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
            <p className="mt-1.5 font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
              Changes apply immediately
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
