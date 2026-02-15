'use client'

import { useState } from 'react'
import Image from 'next/image'
import Link from 'next/link'
import { Menu, LogOut, Loader2, Shield, Library, Settings } from 'lucide-react'
import { ModeToggle } from '@/components/layout'
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
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import { navLinks, isExternal, getUserInitials, getUserDisplayName } from './nav-utils'

export default function Nav() {
  const [open, setOpen] = useState(false)
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()

  return (
    <>
      <svg width="0" height="0" className="absolute">
        <defs>
          <filter id="glitch">
            {/* Base turbulence for subtle movement */}
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.003 0.002"
              numOctaves="1"
              seed="1"
              result="noise1"
            >
              <animate
                attributeName="seed"
                dur="2s"
                values="1;2;3;4;5;6;7;8;1"
                repeatCount="indefinite"
              />
            </feTurbulence>
            <feDisplacementMap
              in="SourceGraphic"
              in2="noise1"
              scale="3"
              result="base"
            />

            {/* Glitch layer */}
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.09"
              numOctaves="1"
              seed="1"
              result="noise2"
            >
              <animate
                attributeName="seed"
                dur="0.3s"
                values="1;5;1"
                repeatCount="indefinite"
                calcMode="discrete"
              />
            </feTurbulence>
            <feDisplacementMap in="base" in2="noise2" scale="1" />
          </filter>
        </defs>
      </svg>

      <nav className="flex w-full items-center justify-between px-4 py-3 border-b border-border/30">
        <div className="flex items-center gap-5">
          <Link
            href="/"
            className="flex-shrink-0 hover:opacity-80 transition-opacity"
          >
            <div className="relative w-[40px] h-[40px] rounded-full overflow-hidden">
              <Image
                src="/PsychicHomilyLogov2.svg"
                alt="Psychic Homily Logo"
                width={40}
                height={40}
                priority
                className="rounded-full"
                style={{ filter: 'url(#glitch)' }}
              />
            </div>
          </Link>

          {/* Desktop Navigation */}
          <div className="hidden md:flex items-center gap-1">
            {navLinks.map(link => (
              <Link
                key={link.href}
                href={link.href}
                target={isExternal(link) ? '_blank' : undefined}
                rel={isExternal(link) ? 'noopener noreferrer' : undefined}
                prefetch={'prefetch' in link ? link.prefetch : undefined}
                className="px-3 py-1.5 text-sm font-medium rounded-md hover:bg-muted/50 hover:text-primary transition-colors"
              >
                {link.label}
              </Link>
            ))}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground hidden sm:block" />
          ) : isAuthenticated && user ? (
            <div className="hidden sm:block">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="relative h-9 w-9 rounded-full ring-2 ring-muted-foreground/25 hover:ring-primary/50 hover:scale-105 transition-all duration-150 cursor-pointer"
                    aria-label="User menu"
                  >
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-medium">
                      {getUserInitials(user)}
                    </div>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuLabel className="font-normal">
                    <div className="flex flex-col space-y-1">
                      {getUserDisplayName(user) && (
                        <p className="text-sm font-medium leading-none">
                          {getUserDisplayName(user)}
                        </p>
                      )}
                      <p className="text-xs leading-none text-muted-foreground">
                        {user.email}
                      </p>
                    </div>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuItem asChild>
                      <Link href="/collection">
                        <Library className="mr-2 h-4 w-4" />
                        My Collection
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
              className="hidden sm:inline text-sm text-muted-foreground hover:text-primary transition-colors"
            >
              login / sign-up
            </Link>
          )}
          <ModeToggle />

          {/* Mobile Menu */}
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger asChild className="md:hidden">
              <Button variant="ghost" size="icon" aria-label="Open menu">
                <Menu className="h-5 w-5" />
              </Button>
            </SheetTrigger>
            <SheetContent
              side="right"
              className="w-[300px] sm:w-[400px] border-l-border/50"
            >
              <SheetHeader>
                <SheetTitle className="text-left">Menu</SheetTitle>
              </SheetHeader>
              <nav className="flex flex-col gap-1 mt-8">
                {navLinks.map(link => (
                  <Link
                    key={link.href}
                    href={link.href}
                    target={isExternal(link) ? '_blank' : undefined}
                    rel={isExternal(link) ? 'noopener noreferrer' : undefined}
                    onClick={() => setOpen(false)}
                    className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors"
                  >
                    {link.label}
                  </Link>
                ))}

                {/* Mobile auth section */}
                {isLoading ? (
                  <div className="flex items-center justify-center py-3 sm:hidden">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                  </div>
                ) : isAuthenticated && user ? (
                  <div className="sm:hidden">
                    <div className="my-2 mx-4 border-t border-border/30" />
                    <p className="px-4 py-1 text-xs font-medium text-muted-foreground uppercase tracking-wider">
                      Your Account
                    </p>
                    <Link
                      href="/collection"
                      onClick={() => setOpen(false)}
                      className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-2"
                    >
                      <Library className="h-4 w-4" />
                      My Collection
                    </Link>
                    <Link
                      href="/profile"
                      onClick={() => setOpen(false)}
                      className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-2"
                    >
                      <Settings className="h-4 w-4" />
                      Profile
                    </Link>
                    {user.is_admin && (
                      <Link
                        href="/admin"
                        onClick={() => setOpen(false)}
                        className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-2"
                      >
                        <Shield className="h-4 w-4" />
                        Admin
                      </Link>
                    )}
                    <div className="px-4 py-3 space-y-3 mt-2">
                      <p className="text-sm text-muted-foreground truncate">
                        {user.email}
                      </p>
                      <Button
                        variant="outline"
                        className="w-full justify-start"
                        onClick={() => {
                          logout()
                          setOpen(false)
                        }}
                      >
                        <LogOut className="h-4 w-4 mr-2" />
                        Sign out
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Link
                    href="/auth"
                    onClick={() => setOpen(false)}
                    className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors sm:hidden"
                  >
                    login / sign-up
                  </Link>
                )}
              </nav>
            </SheetContent>
          </Sheet>
        </div>
      </nav>
    </>
  )
}
