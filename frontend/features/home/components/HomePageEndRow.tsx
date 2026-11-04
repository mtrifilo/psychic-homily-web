'use client'

import { useState } from 'react'
import Link from 'next/link'
// Concrete module path, not the `@/components/shared` barrel: this component
// is reachable from the home route. See features/sharedChunkBarrelGuard.test.ts.
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { bottomTabBarHeight } from '@/components/layout/nav/bottomTabBarHeight'
import { useHomeLayoutWriteFailed } from '../hooks/useHomeLayout'
import {
  HomeSectionList,
  SAVE_FAILED_MESSAGE,
  type HomeVisibilityChange,
} from './HomeSectionList'
import { HOME_ACCENT_LINK_CLASS, ResolvedHomeCityShowsLink } from './HomeCityShowsLink'
import { SignedInHomeDiscoverLinks } from './HomeDiscoverLinks'
import { useHomeCityLinkSlot, useHomeDiscoverLinksSlot } from './HomeLinkSlots'
import {
  HOME_LAYOUT_SETTINGS_HREF,
  type HomeLayoutDocument,
} from '../sections'

/** Space kept between the popover and the viewport edge, or the phone tab bar. */
const POPOVER_GUTTER_PX = 8

/**
 * The always-present row after the signed-in home's last section: the one
 * "Customize home →" control, its popover, and the layout write's failure line.
 *
 * It is also the last stop for the city link (nearby and saved shows both
 * hidden) and, while any section still renders, for the Discover row; both
 * render above the Customize link, city link first.
 *
 * The popover opens upward by default: the link sits at the end of the page,
 * where the space is above it. Radix flips it when the space above runs out,
 * and its collision boundary stops above the fixed phone tab bar, which Radix
 * cannot see: without that, a popover opening downward would overlap the bar
 * and hide the page's own navigation behind it. Its
 * position is recomputed every frame while open, because hiding or showing a
 * section above the link moves the link for the length of the slot's height
 * animation.
 *
 * The popover is capped to the viewport width with a gutter so it fits at
 * 390px without the modal weight of a sheet, and capped to the available
 * HEIGHT so a short viewport scrolls the rows rather than putting the footer
 * out of reach. Anonymous viewers never reach this component: the server picks
 * the signed-in variant before paint.
 */
export function HomePageEndRow({
  initialLayout,
  onBeforeChange,
  className,
}: {
  initialLayout?: HomeLayoutDocument | null
  onBeforeChange?: (change: HomeVisibilityChange) => void
  className?: string
}) {
  const withCityLink = useHomeCityLinkSlot() === 'pageEnd'
  const withDiscoverLinks = useHomeDiscoverLinksSlot() === 'pageEnd'
  // Read from the shared mutation cache, not from the popover: the write
  // outlives the popover, so a viewer who closed it would otherwise watch their
  // change silently undo itself with no explanation anywhere.
  const hasWriteFailed = useHomeLayoutWriteFailed()
  // Measured each time the popover opens, not at mount: the bar is absent from
  // `xl`, and the viewport may have crossed it since. A resize across `xl`
  // while the popover stays open keeps the inset it opened with.
  const [tabBarInset, setTabBarInset] = useState(0)
  return (
    <div
      data-testid="home-page-end"
      className={cn('flex flex-col items-start gap-3', className)}
    >
      {withCityLink && <ResolvedHomeCityShowsLink />}
      {withDiscoverLinks && <SignedInHomeDiscoverLinks />}

      <Popover
        onOpenChange={open => {
          if (open) setTabBarInset(bottomTabBarHeight())
        }}
      >
        <PopoverTrigger asChild>
          <button type="button" className={HOME_ACCENT_LINK_CLASS}>
            Customize home →
          </button>
        </PopoverTrigger>
        <PopoverContent
          side="top"
          align="start"
          sideOffset={8}
          updatePositionStrategy="always"
          collisionPadding={{
            top: POPOVER_GUTTER_PX,
            bottom: tabBarInset + POPOVER_GUTTER_PX,
          }}
          className="flex max-h-[var(--radix-popover-content-available-height)] w-[min(24rem,calc(100vw-2rem))] flex-col overflow-y-auto"
          aria-label="Customize home"
        >
          <div className="px-4 pb-3 pt-4">
            <p className="text-xs text-muted-foreground">
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
              <Link href={HOME_LAYOUT_SETTINGS_HREF} className={HOME_ACCENT_LINK_CLASS}>
                All settings →
              </Link>
            }
          />
        </PopoverContent>
      </Popover>

      {hasWriteFailed && (
        <InlineErrorBanner className="self-stretch">
          {SAVE_FAILED_MESSAGE}
        </InlineErrorBanner>
      )}
    </div>
  )
}
