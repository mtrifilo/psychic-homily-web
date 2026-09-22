'use client'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
// Concrete module path, not the `@/features/home` barrel: that barrel is
// root-layout reachable and a `'use client'` barrel is not tree-shaken per
// export. See features/sharedChunkBarrelGuard.test.ts.
import { HomeSectionList } from '@/features/home/components/HomeSectionList'
import { HOME_LAYOUT_SETTINGS_ANCHOR } from '@/features/home/sections'

/**
 * The Settings mirror of the home page's customize popover.
 *
 * The SAME list component, reading and writing the same profile cache entry,
 * so the two surfaces cannot report different layouts. There is no server
 * fallback document here: this page is not the home page, so there is no order
 * to get right before hydration.
 */
export function HomeLayoutSettings() {
  return (
    <Card id={HOME_LAYOUT_SETTINGS_ANCHOR}>
      <CardHeader>
        <CardTitle className="text-base">Home page</CardTitle>
        <CardDescription>
          Show, hide, and reorder the sections on your home page. Changes apply
          immediately.
        </CardDescription>
      </CardHeader>
      <CardContent className="px-0">
        <HomeSectionList className="border-t border-border" />
      </CardContent>
    </Card>
  )
}
