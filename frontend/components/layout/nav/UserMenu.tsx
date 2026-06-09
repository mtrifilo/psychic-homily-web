'use client'

import Link from 'next/link'
import { Loader2, LogOut, Shield, UserCircle, Library, Bell } from 'lucide-react'
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

// The right-hand actions cluster. Signed in (PSY-1018, Figma 537:91): the
// "+ Submit" primary CTA → notification bell → avatar dropdown. Signed out: the
// "login / sign-up" text link (matching the deployed app). The authenticated
// bar deliberately promotes Submit to a standalone CTA — unlike the anonymous
// bar, where Submit stays inside the Contribute menu (OQ-2), since logged-in
// users can be asked to contribute. Visibility is controlled by the parent
// (hidden below the search/auth breakpoint); the mobile sheet carries Submit
// via its Contribute group (a dedicated mobile CTA is PSY-1020's scope).
export function UserMenu() {
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()

  if (isLoading) {
    return <Loader2 className="size-4 animate-spin text-muted-foreground" />
  }

  if (isAuthenticated && user) {
    // "Profile" lands the user on their OWN public identity view
    // (`/users/[username]`) — the same dense page visitors see — not the
    // settings form. The route is keyed on username, which is nullable
    // (OAuth-only accounts; see users.username migration). When the user has
    // no username yet, fall back to /profile (the settings form, where they
    // can set one) so the link is never broken. Mirrors the UserAttribution
    // linkability rule (only link when username is non-empty). PSY-1025.
    const profileHref = user.username ? `/users/${user.username}` : '/profile'

    return (
      <div className="flex items-center gap-2">
        <Button asChild>
          <Link href="/shows/submit">+ Submit</Link>
        </Button>
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
                <Link href={profileHref}>
                  <UserCircle className="mr-2 size-4" />
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
