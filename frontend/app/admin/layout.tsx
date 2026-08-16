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
// PSY-1817: this layout also owns the mobile counterpart of that rail. The
// drawer trigger used to hang off the global TopBar behind a
// `pathname === '/admin'` string and a client `is_admin` check — admin chrome
// riding on every public route's shell. Mounting it here makes both gates
// structural: the route segment scopes it, AdminGuard authorizes it, and the
// drawer only has to know its own breakpoint. It sits at the top of the content
// column so it reads as part of the admin shell rather than the site chrome.
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
          <AdminMobileDrawer />
          {children}
        </div>
      </div>
    </AdminGuard>
  )
}
