'use client'

import Image from 'next/image'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useTheme } from 'next-themes'
import {
  Menu, LogOut, Loader2, Shield, Settings, Moon, Sun, Search,
  Library, ExternalLink,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuGroup,
  DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import { getUserInitials, getUserDisplayName } from '@/app/nav-utils'
import { sidebarGroups } from './Sidebar'

interface TopBarProps {
  mobileOpen: boolean
  onMobileOpenChange: (open: boolean) => void
  onSearchClick?: () => void
}

export function TopBar({ mobileOpen, onMobileOpenChange, onSearchClick }: TopBarProps) {
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()
  const { theme, setTheme } = useTheme()
  const pathname = usePathname()

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/'
    return pathname === href || pathname.startsWith(href + '/')
  }

  return (
    <>
      {/* SVG filter for logo glitch effect */}
      <svg width="0" height="0" className="absolute">
        <defs>
          <filter id="glitch">
            <feTurbulence type="fractalNoise" baseFrequency="0.003 0.002" numOctaves="1" seed="1" result="noise1">
              <animate attributeName="seed" dur="2s" values="1;2;3;4;5;6;7;8;1" repeatCount="indefinite" />
            </feTurbulence>
            <feDisplacementMap in="SourceGraphic" in2="noise1" scale="3" result="base" />
            <feTurbulence type="fractalNoise" baseFrequency="0.09" numOctaves="1" seed="1" result="noise2">
              <animate attributeName="seed" dur="0.3s" values="1;5;1" repeatCount="indefinite" calcMode="discrete" />
            </feTurbulence>
            <feDisplacementMap in="base" in2="noise2" scale="1" />
          </filter>
        </defs>
      </svg>

      <header className="sticky top-0 z-50 flex h-[var(--topbar-height)] w-full items-center justify-between border-b border-border/30 bg-background/95 px-4 backdrop-blur-sm supports-[backdrop-filter]:bg-background/60">
        {/* Left: mobile hamburger + logo */}
        <div className="flex items-center gap-3">
          <Sheet open={mobileOpen} onOpenChange={onMobileOpenChange}>
            <SheetTrigger asChild className="md:hidden">
              <Button variant="ghost" size="icon" aria-label="Open menu">
                <Menu className="h-5 w-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-[280px] border-r-border/50 p-0">
              <SheetHeader className="px-4 pt-4">
                <SheetTitle className="text-left">Menu</SheetTitle>
              </SheetHeader>
              <nav className="flex flex-col gap-1 px-2 py-4">
                {sidebarGroups.map(group => (
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
                          onClick={() => onMobileOpenChange(false)}
                          className={cn(
                            'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                            active
                              ? 'bg-accent text-accent-foreground'
                              : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                          )}
                        >
                          <Icon className="h-4 w-4" />
                          <span>{item.label}</span>
                          {item.external && <ExternalLink className="ml-auto h-3 w-3 opacity-50" />}
                        </Link>
                      )
                    })}
                  </div>
                ))}

                {/* Mobile auth section */}
                {isLoading ? (
                  <div className="flex items-center justify-center py-3">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                  </div>
                ) : isAuthenticated && user ? (
                  <>
                    <div className="mx-3 mb-2 border-t border-border/30" />
                    <Link
                      href="/library"
                      onClick={() => onMobileOpenChange(false)}
                      className={cn(
                        'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                        isActive('/library')
                          ? 'bg-accent text-accent-foreground'
                          : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                      )}
                    >
                      <Library className="h-4 w-4" />
                      Library
                    </Link>
                    <Link
                      href="/profile"
                      onClick={() => onMobileOpenChange(false)}
                      className={cn(
                        'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                        isActive('/profile')
                          ? 'bg-accent text-accent-foreground'
                          : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                      )}
                    >
                      <Settings className="h-4 w-4" />
                      Settings
                    </Link>
                    {user.is_admin && (
                      <Link
                        href="/admin"
                        onClick={() => onMobileOpenChange(false)}
                        className={cn(
                          'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                          isActive('/admin')
                            ? 'bg-accent text-accent-foreground'
                            : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                        )}
                      >
                        <Shield className="h-4 w-4" />
                        Admin
                      </Link>
                    )}
                    <div className="mx-3 my-2 border-t border-border/30" />
                    <div className="px-3 py-2 space-y-3">
                      <p className="text-sm text-muted-foreground truncate">{user.email}</p>
                      <Button
                        variant="outline"
                        className="w-full justify-start"
                        onClick={() => {
                          logout()
                          onMobileOpenChange(false)
                        }}
                      >
                        <LogOut className="h-4 w-4 mr-2" />
                        Sign out
                      </Button>
                    </div>
                  </>
                ) : (
                  <>
                    <div className="mx-3 mb-2 border-t border-border/30" />
                    <Link
                      href="/auth"
                      onClick={() => onMobileOpenChange(false)}
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
                  <Sun className="h-4 w-4 dark:hidden" />
                  <Moon className="h-4 w-4 hidden dark:block" />
                  {theme === 'dark' ? 'Light mode' : 'Dark mode'}
                </button>
              </nav>
            </SheetContent>
          </Sheet>

          <Link href="/" className="flex items-center gap-2 transition-opacity hover:opacity-80">
            <div className="relative h-[36px] w-[36px] overflow-hidden rounded-full">
              <Image
                src="/PsychicHomilyLogov2.svg"
                alt="Psychic Homily Logo"
                width={36}
                height={36}
                priority
                className="rounded-full"
                style={{ filter: 'url(#glitch)' }}
              />
            </div>
            <span className="hidden text-sm font-semibold sm:inline">Psychic Homily</span>
          </Link>
        </div>

        {/* Right: search + theme + user */}
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="hidden h-8 w-48 cursor-pointer justify-start text-xs text-muted-foreground sm:flex"
            onClick={onSearchClick}
          >
            <Search className="mr-2 h-3.5 w-3.5" />
            Search...
            <kbd className="pointer-events-none ml-auto inline-flex h-5 items-center rounded border bg-muted px-1.5 font-mono text-[10px] text-muted-foreground">
              ⌘K
            </kbd>
          </Button>

          <Button
            variant="ghost"
            size="icon"
            className="hidden cursor-pointer sm:flex"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          >
            <Sun className="h-[1.2rem] w-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
            <Moon className="absolute h-[1.2rem] w-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
            <span className="sr-only">Toggle theme</span>
          </Button>

          {isLoading ? (
            <Loader2 className="hidden h-4 w-4 animate-spin text-muted-foreground sm:block" />
          ) : isAuthenticated && user ? (
            <div className="hidden sm:block">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="relative h-9 w-9 cursor-pointer rounded-full ring-2 ring-muted-foreground/25 transition-all duration-150 hover:scale-105 hover:ring-primary/50"
                    aria-label="User menu"
                  >
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
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
                      <Link href="/library">
                        <Library className="mr-2 h-4 w-4" />
                        My Library
                      </Link>
                    </DropdownMenuItem>
                    <DropdownMenuItem asChild>
                      <Link href="/profile">
                        <Settings className="mr-2 h-4 w-4" />
                        Profile
                      </Link>
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                  {user.is_admin && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem asChild>
                        <Link href="/admin" prefetch={false}>
                          <Shield className="mr-2 h-4 w-4" />
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
                    <LogOut className="mr-2 h-4 w-4" />
                    Sign out
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ) : (
            <Link
              href="/auth"
              className="hidden text-sm text-muted-foreground transition-colors hover:text-primary sm:block"
            >
              login / sign-up
            </Link>
          )}
        </div>
      </header>
    </>
  )
}
