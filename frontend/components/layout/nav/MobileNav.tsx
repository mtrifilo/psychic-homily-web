'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import dynamic from 'next/dynamic'
import { useTheme } from 'next-themes'
import {
  Menu, LogOut, Loader2, Shield, Settings, Moon, Sun, Library, ExternalLink, Bell,
  UserCircle,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import { sidebarGroups } from '../Sidebar'
import { isNavActive } from './navData'

// Mobile admin drawer (config + the 7 queue-count hooks) is a separate chunk
// loaded only when an admin opens the drawer on /admin — off the public bundle.
const AdminDrawerNav = dynamic(() => import('../AdminDrawerNav'), { ssr: false })

// The hamburger sheet for small/tablet screens (below `lg`). This is the
// previous TopBar drawer, preserved verbatim — it still drives the long-tail nav
// while the desktop top bar is condensed. PSY-1020 replaces it with the Option-A
// bottom tab bar; until then it keeps the full `sidebarGroups` reachable on
// mobile (the desktop sidebar that used to render these is retired in PSY-1013).
export function MobileNav() {
  const [open, setOpen] = useState(false)
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()
  const { theme, setTheme } = useTheme()
  const pathname = usePathname()

  // Context-aware drawer (PSY-933): under the /admin tab-shell the nav groups
  // swap to the grouped admin sections (lazily loaded). Gated on isAdmin
  // (mid-redirect safety) + scoped to the exact /admin shell (usePathname()
  // strips ?tab=); standalone /admin/<section> sub-routes keep the public nav.
  const isAdmin = !!user?.is_admin
  const showAdminNav = isAdmin && pathname === '/admin'

  const isActive = (href: string) => isNavActive(pathname, href)

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild className="lg:hidden">
        <Button variant="ghost" size="icon" aria-label="Open menu">
          <Menu className="size-5" />
        </Button>
      </SheetTrigger>
      <SheetContent side="left" className="w-[280px] border-r-border/50 p-0">
        <SheetHeader className="px-4 pt-4">
          <SheetTitle className="text-left">Menu</SheetTitle>
        </SheetHeader>
        <nav className="flex flex-col gap-1 px-2 py-4">
          {showAdminNav ? (
            <AdminDrawerNav onNavigate={() => setOpen(false)} />
          ) : (
            sidebarGroups.map(group => (
              <div key={group.label} className="mb-4">
                <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground/50">
                  {group.label}
                </p>
                {group.items.map(item => {
                  const Icon = item.icon
                  const active = !item.external && isActive(item.href)
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      target={item.external ? '_blank' : undefined}
                      rel={item.external ? 'noopener noreferrer' : undefined}
                      onClick={() => setOpen(false)}
                      className={cn(
                        'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                        active
                          ? 'bg-accent text-accent-foreground'
                          : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                      )}
                    >
                      <Icon className="size-4" />
                      <span>{item.label}</span>
                      {item.external && <ExternalLink className="ml-auto size-3 opacity-50" />}
                    </Link>
                  )
                })}
              </div>
            ))
          )}

          {/* Mobile auth section */}
          {isLoading ? (
            <div className="flex items-center justify-center py-3">
              <Loader2 className="size-5 animate-spin text-muted-foreground" />
            </div>
          ) : isAuthenticated && user ? (
            <>
              <div className="mx-3 mb-2 border-t border-border/30" />
              <Link
                href="/notifications"
                onClick={() => setOpen(false)}
                className={cn(
                  'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                  isActive('/notifications')
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                )}
              >
                <Bell className="size-4" />
                Notifications
              </Link>
              <Link
                href="/library"
                onClick={() => setOpen(false)}
                className={cn(
                  'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                  isActive('/library')
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                )}
              >
                <Library className="size-4" />
                Library
              </Link>
              {/* /users/me redirects to the user's public profile when a
                  username is set, and renders the claim-username self view
                  otherwise (PSY-1045). */}
              <Link
                href="/users/me"
                onClick={() => setOpen(false)}
                className={cn(
                  'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                  isActive('/users/me')
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                )}
              >
                <UserCircle className="size-4" />
                Profile
              </Link>
              <Link
                href="/profile"
                onClick={() => setOpen(false)}
                className={cn(
                  'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                  isActive('/profile')
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                )}
              >
                <Settings className="size-4" />
                Settings
              </Link>
              {user.is_admin && (
                <Link
                  href="/admin"
                  onClick={() => setOpen(false)}
                  className={cn(
                    'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                    isActive('/admin')
                      ? 'bg-accent text-accent-foreground'
                      : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                  )}
                >
                  <Shield className="size-4" />
                  Admin
                </Link>
              )}
              <div className="mx-3 my-2 border-t border-border/30" />
              <div className="space-y-3 px-3 py-2">
                <p className="truncate text-sm text-muted-foreground">{user.email}</p>
                <Button
                  variant="outline"
                  className="w-full justify-start"
                  onClick={() => {
                    logout()
                    setOpen(false)
                  }}
                >
                  <LogOut className="mr-2 size-4" />
                  Sign out
                </Button>
              </div>
            </>
          ) : (
            <>
              <div className="mx-3 mb-2 border-t border-border/30" />
              <Link
                href="/auth"
                onClick={() => setOpen(false)}
                className="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
              >
                login / sign-up
              </Link>
            </>
          )}

          {/* Theme toggle */}
          <div className="mx-3 my-2 border-t border-border/30" />
          <button
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
          >
            <Sun className="size-4 dark:hidden" />
            <Moon className="hidden size-4 dark:block" />
            {theme === 'dark' ? 'Light mode' : 'Dark mode'}
          </button>
        </nav>
      </SheetContent>
    </Sheet>
  )
}
