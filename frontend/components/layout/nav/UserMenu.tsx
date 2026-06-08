'use client'

import Link from 'next/link'
import { Loader2, LogOut, Shield, Settings, Library, Bell } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthContext } from '@/lib/context/AuthContext'
import { getUserInitials, getUserDisplayName } from '@/app/nav-utils'
import { NotificationBell } from '@/features/notifications'

// The right-hand account cluster: notification bell + avatar dropdown when
// signed in, otherwise the "login / sign-up" text link (matching the deployed
// app). Behaviour is preserved verbatim from the previous TopBar; PSY-1018
// redesigns the authenticated bar (+ Submit · 🔔 · avatar) once it is designed.
// Visibility is controlled by the parent (hidden below the search/auth
// breakpoint); the mobile sheet carries the same actions on small screens.
export function UserMenu() {
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()

  if (isLoading) {
    return <Loader2 className="size-4 animate-spin text-muted-foreground" />
  }

  if (isAuthenticated && user) {
    return (
      <div className="flex items-center gap-1">
        <NotificationBell />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="relative size-9 cursor-pointer rounded-full ring-2 ring-muted-foreground/25 transition-all duration-150 hover:scale-105 hover:ring-primary/50"
              aria-label="User menu"
            >
              <div className="flex size-8 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
                {getUserInitials(user)}
              </div>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel className="font-normal">
              <div className="flex flex-col space-y-1">
                {getUserDisplayName(user) && (
                  <p className="text-sm font-medium leading-none">{getUserDisplayName(user)}</p>
                )}
                <p className="text-xs leading-none text-muted-foreground">{user.email}</p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild>
                <Link href="/notifications">
                  <Bell className="mr-2 size-4" />
                  Notifications
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href="/library">
                  <Library className="mr-2 size-4" />
                  My Library
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href="/profile">
                  <Settings className="mr-2 size-4" />
                  Profile
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            {user.is_admin && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href="/admin" prefetch={false}>
                    <Shield className="mr-2 size-4" />
                    Admin
                  </Link>
                </DropdownMenuItem>
              </>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={logout}
              className="text-destructive focus:text-destructive"
            >
              <LogOut className="mr-2 size-4" />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    )
  }

  return (
    <Link
      href="/auth"
      className="text-sm text-muted-foreground transition-colors hover:text-primary"
    >
      login / sign-up
    </Link>
  )
}
