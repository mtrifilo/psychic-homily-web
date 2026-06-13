'use client'

import { useState } from 'react'
import { usePathname } from 'next/navigation'
import dynamic from 'next/dynamic'
import { Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'

// Mobile admin drawer (config + the 7 queue-count hooks) is a separate chunk
// loaded only when an admin opens the drawer on /admin — off the public bundle.
const AdminDrawerNav = dynamic(() => import('../AdminDrawerNav'), { ssr: false })

// The mobile admin-sections drawer. PSY-1020 retired the public hamburger sheet
// — the bottom tab bar (BottomTabBar) is now the primary mobile nav, with the
// long tail in its Browse sheet — so this component is reduced to the one job
// the tab bar doesn't cover: the context-aware admin section nav on the /admin
// tab-shell (PSY-933). It renders nothing anywhere else.
//
// Gated on isAdmin (mid-redirect safety) + scoped to the exact /admin shell
// (usePathname() strips ?tab=); standalone /admin/<section> sub-routes get no
// drawer, matching their pre-PSY-933 behavior.
export function MobileNav() {
  const [open, setOpen] = useState(false)
  const { user } = useAuthContext()
  const pathname = usePathname()

  const showAdminNav = !!user?.is_admin && pathname === '/admin'
  if (!showAdminNav) return null

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild className="lg:hidden">
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
