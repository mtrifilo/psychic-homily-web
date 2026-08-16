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
// auth gate of its own — being mounted here inside AdminGuard is the gate
// (PSY-1817; the drawer previously hung off the global TopBar behind a
// `pathname === '/admin'` string and a client `is_admin` check, putting
// admin-only chrome on every public route's shell).
//
// The drawer's wrapper is the rail's exact inverse, `md:hidden` against
// AdminSidebar's `hidden md:flex`, so no viewport band has both admin navs or
// neither — layout.test.tsx asserts the two together rather than trusting each
// file to stay in step. It is `sticky` under the top bar because that is where
// the trigger used to live: static, an admin on the moderation queue would have
// to scroll back to the top to change sections.
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
          <div className="sticky top-[var(--topbar-height)] z-30 bg-background px-4 py-2 md:hidden">
            <AdminMobileDrawer />
          </div>
          {children}
        </div>
      </div>
    </AdminGuard>
  )
}
