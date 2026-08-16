'use client'

import { useState } from 'react'
import dynamic from 'next/dynamic'
import { Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'

// The admin drawer body (config + the 7 queue-count hooks) is a separate chunk
// loaded only when an admin actually opens the drawer.
const AdminDrawerNav = dynamic(() => import('./AdminDrawerNav'), { ssr: false })

// The mobile admin-sections drawer (formerly MobileNav). PSY-1020 retired the
// public hamburger sheet — the bottom tab bar (BottomTabBar) is now the primary
// mobile nav, with the long tail in its Browse sheet — so this component keeps
// the one job the tab bar doesn't cover: admin section nav below `md`, where
// AdminSidebar's rail is hidden.
//
// PSY-1817: mounted by app/admin/layout.tsx inside AdminGuard, not by the
// global TopBar. Route scope and admin-only access are now structural — the
// /admin route segment and the guard enforce them — which is what retired the
// `pathname === '/admin'` string and the `user.is_admin` client check this
// carried while it hung off shared public chrome. One deliberate consequence:
// standalone /admin/<section> routes now get the drawer too, which is what
// AdminSidebar has always done on desktop; below `md` those routes previously
// had no admin nav at all.
//
// `md:hidden` mirrors AdminSidebar's `hidden md:flex` — the drawer stands in
// for the rail exactly where the rail is absent, with no overlap band. The
// wrapper is the sole owner of that breakpoint and of the trigger's gutter,
// which matches the `px-4` the admin pages set on their own content.
export function AdminMobileDrawer() {
  const [open, setOpen] = useState(false)

  return (
    <div className="px-4 pt-4 md:hidden">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>
          <Button {...replayOnHydrate} variant="ghost" size="icon" aria-label="Open admin menu">
            <Menu className="size-5" />
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-[280px] border-r-border/50 p-0">
          <SheetHeader className="px-4 pt-4">
            <SheetTitle className="text-left">Admin</SheetTitle>
          </SheetHeader>
          <nav className="flex flex-col gap-1 overflow-y-auto px-2 py-4">
            <AdminDrawerNav onNavigate={() => setOpen(false)} />
          </nav>
        </SheetContent>
      </Sheet>
    </div>
  )
}
