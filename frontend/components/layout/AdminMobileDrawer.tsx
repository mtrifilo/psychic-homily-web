'use client'

import { useState } from 'react'
import dynamic from 'next/dynamic'
import { Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'

// The drawer body (config + the 7 queue-count hooks) is a separate chunk,
// loaded only once an admin actually opens the drawer.
const AdminDrawerNav = dynamic(() => import('./AdminDrawerNav'), { ssr: false })

// The mobile admin-sections drawer (formerly MobileNav). PSY-1020 retired the
// public hamburger sheet — the bottom tab bar (BottomTabBar) is the primary
// mobile nav, with the long tail in its Browse sheet — leaving this component
// the one job the tab bar does not cover: reaching the admin sections on a
// phone, where AdminSidebar's rail is hidden.
//
// It carries no route or auth gate. app/admin/layout.tsx mounts it inside
// AdminGuard, so the /admin segment scopes it and the guard authorizes it; that
// layout also owns where it sits and the breakpoint that pairs it with the
// rail. Anything resembling a `pathname === ...` or `is_admin` check in here is
// a sign that mount point has been lost (PSY-1817).
//
// Deliberately no `replayOnHydrate`: that mechanism replays clicks landing on
// server HTML before React attaches handlers, and AdminGuard resolves auth from
// a client-side query — this button cannot exist until well after hydration.
export function AdminMobileDrawer() {
  const [open, setOpen] = useState(false)

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Open admin menu">
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
  )
}
