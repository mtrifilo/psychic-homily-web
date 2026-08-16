import { Metadata } from 'next'
import AdminGuard from './admin-guard'
import { AdminSidebar } from '@/components/layout/AdminSidebar'
import { AdminMobileDrawer } from '@/components/layout/AdminMobileDrawer'

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

// The admin area renders its own always-on left rail (PSY-1114) alongside the
// global TopBar, restoring the admin nav that was orphaned when PSY-1013
// retired the global Sidebar. The rail lives inside AdminGuard so it only
// mounts for authenticated admins; `min-w-0` lets the content column shrink
// instead of forcing horizontal overflow on wide admin tables.
//
// This layout owns BOTH admin navs, which is what lets the pair be read in one
// place: the rail above `md`, the drawer below it. Neither carries a route or
// auth gate of its own — being mounted here inside AdminGuard is the gate, so
// admin-only chrome no longer rides on every public route's shell (PSY-1817).
//
// The drawer's wrapper is the rail's exact inverse, `md:hidden` against
// AdminSidebar's `hidden md:flex`, so no viewport band has both admin navs or
// neither. layout.test.tsx asserts that as a property — each side carries one
// responsive display utility and they name the same breakpoint — rather than
// pinning the two literal strings, which would still pass if someone added a
// second breakpoint to either side and reopened the gap.
//
// It is deliberately NOT sticky. In the top bar the trigger was sticky for
// free, because global chrome owns a screen band and a z-layer that nothing
// else competes for; the content column owns neither. A sticky bar here would
// have to claim `top: var(--topbar-height)` on every current and future
// /admin/* page, and pages already claim it — hero-lab pins its own toolbar
// there (hero-lab/_components/HeroLab.tsx), which an opaque bar at a higher
// z-index silently covers below `md`. Reaching the sections from deep in a long
// queue costs a scroll back to the top; that is the honest price of the move,
// and buying it back needs a coordinated offset, not a z-index this layout
// wins by accident.
//
// `px-4` matches the tab shell's own gutter (app/admin/page.tsx). Standalone
// /admin/<section> pages render flush to the viewport edge below `md` — a
// pre-existing inconsistency between the tab shell and the sub-routes, not one
// this wrapper introduces.
export default function AdminLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AdminGuard>
      <div className="flex flex-1">
        <AdminSidebar />
        <div className="min-w-0 flex-1">
          <div data-testid="admin-drawer-bar" className="px-4 py-2 md:hidden">
            <AdminMobileDrawer />
          </div>
          {children}
        </div>
      </div>
    </AdminGuard>
  )
}
