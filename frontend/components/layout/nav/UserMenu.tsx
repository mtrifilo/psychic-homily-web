'use client'

import Link from 'next/link'
import { Loader2, LogOut, Shield, UserCircle, Library, Bell, Palette, Settings } from 'lucide-react'
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
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { getUserInitials, getUserDisplayName } from './userDisplay'
import { NotificationBell } from '@/features/notifications'

// The right-hand actions cluster. Signed in (PSY-1018, Figma 537:91): the
// "+ Submit" primary CTA → notification bell → avatar dropdown. Signed out: the
// "login / sign-up" text link (matching the deployed app). The authenticated
// bar deliberately promotes Submit to a standalone CTA — unlike the anonymous
// bar, where Submit stays inside the Contribute menu (OQ-2), since logged-in
// users can be asked to contribute. Visibility is controlled by the parent
// (hidden below the search/auth breakpoint); on small screens Submit stays
// reachable through the bottom tab bar's Browse sheet (Contribute group,
// PSY-1020), which also mirrors these account entries in its Account sheet.
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
    // no username yet, route to /users/me — the claim-username self view that
    // renders the profile experience with a "set username" banner (PSY-1045;
    // previously fell back to the /profile settings form). PSY-1025.
    const profileHref = user.username ? `/users/${user.username}` : '/users/me'

    return (
      <div className="flex items-center gap-2">
        <Button asChild>
          <Link href="/shows/submit">+ Submit</Link>
        </Button>
        <NotificationBell />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              // Radix DropdownMenu opens on pointerdown, which is why the
              // primitive replays the whole pointer sequence, not just a click.
              {...replayOnHydrate}
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
              {/* PSY-1486: Settings → /profile editor (parity with the retired hamburger sheet..
                  Profile above is the public identity view; this is Edit
                  profile & settings. */}
              <DropdownMenuItem asChild>
                <Link href="/profile">
                  <Settings className="mr-2 size-4" />
                  Settings
                </Link>
              </DropdownMenuItem>
              {/* Appearance is reachable here in the DEFAULT top-bar mode — the
                  Sidebar entry only renders once already in side-nav mode, so
                  this is the entry point for the primary top → side switch. */}
              <DropdownMenuItem asChild>
                <Link href="/settings/appearance">
                  <Palette className="mr-2 size-4" />
                  Appearance
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

  // `shrink-0 whitespace-nowrap` for the same reason the authenticated controls
  // carry it (the Button base variant): the search field is the top bar's only
  // slack absorber, so nothing else here may give. Without it this link was
  // shrinkable, and the flex algorithm split any shortfall between it and the
  // search — stacking "login /" over "sign-up" at 640px and 1280px, which is
  // where the anonymous bar ran out of room (PSY-1638).
  return (
    <Link
      href="/auth"
      className="shrink-0 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-primary"
    >
      login / sign-up
    </Link>
  )
}
