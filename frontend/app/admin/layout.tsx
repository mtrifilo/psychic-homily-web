import { Metadata } from 'next'
import AdminGuard from './admin-guard'
import { AdminSidebar } from '@/components/layout/AdminSidebar'

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

// The admin area renders its own always-on left rail (PSY-1114) alongside the
// global TopBar, restoring the admin nav that was orphaned when PSY-1013
// retired the global Sidebar. The rail lives inside AdminGuard so it only
// mounts for authenticated admins; `min-w-0` lets the content column shrink
// instead of forcing horizontal overflow on wide admin tables.
export default function AdminLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AdminGuard>
      <div className="flex flex-1">
        <AdminSidebar />
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </AdminGuard>
  )
}
