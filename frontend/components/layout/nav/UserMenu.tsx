'use client'

import Link from 'next/link'
import { LogOut } from 'lucide-react'
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
import { accountNavItems, visibleNavItems } from './navData'
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
  const { user, authStatus, logout } = useAuthContext()

  // The `login / sign-up` link below is an identity claim about the viewer, so
  // the gate is `authStatus` per the rule stated on the AuthStatus type.
  //
  // The unsettled slot renders neither that link nor a spinner. Pending covers
  // two windows: the profile in flight, which resolves in a moment, and a
  // profile that failed on a non-definitive error, which resolves only on the
  // throttled focus/reconnect refetch `useProfile` arms. So the slot can stay
  // empty for a long time, and while it does this bar offers no route to
  // /auth; typing the URL still works, and /auth redirects a viewer who turns
  // out to be signed in.
  //
  // The box is the avatar trigger's size. The row still reflows when auth
  // settles, because the two settled states are a text link and a
  // three-control cluster.
  if (authStatus === 'pending') {
    return <div aria-hidden="true" className="size-9 shrink-0" />
  }

  if (authStatus === 'authenticated' && user) {
    // The canonical account destination list (navData, PSY-1821) — Profile's
    // username-aware href, the Profile/Settings split, and each entry's icon
    // are all decided at that table, shared with the mobile Account sheet.
    // Admin-gated entries render after a separator, in their own group.
    const items = visibleNavItems(accountNavItems(user), user)
    const menuItems = items.filter(item => !item.adminOnly)
    const adminItems = items.filter(item => item.adminOnly)

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
            {/* No unread badge on the Notifications row here, unlike the
                mobile Account sheet which renders this same table (PSY-1819):
                this dropdown only exists at widths where NotificationBell sits
                a few pixels away in the same cluster, already carrying the
                count. Badging both would state it twice in one control group. */}
            <DropdownMenuGroup>
              {menuItems.map(item => {
                const Icon = item.icon
                return (
                  <DropdownMenuItem key={item.href} asChild>
                    <Link href={item.href}>
                      <Icon className="mr-2 size-4" />
                      {item.label}
                    </Link>
                  </DropdownMenuItem>
                )
              })}
            </DropdownMenuGroup>
            {adminItems.length > 0 && (
              <>
                <DropdownMenuSeparator />
                {adminItems.map(item => {
                  const Icon = item.icon
                  return (
                    <DropdownMenuItem key={item.href} asChild>
                      <Link href={item.href} prefetch={false}>
                        <Icon className="mr-2 size-4" />
                        {item.label}
                      </Link>
                    </DropdownMenuItem>
                  )
                })}
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
