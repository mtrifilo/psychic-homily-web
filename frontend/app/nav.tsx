'use client'

import { useState } from 'react'
import Image from 'next/image'
import Link from 'next/link'
import { Menu, LogOut, Loader2, Shield, Heart } from 'lucide-react'
import { ModeToggle } from '@/components/mode-toggle'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'

const navLinks = [
  { href: '/shows', label: 'Shows' },
  { href: '/blog', label: 'Blog' },
  { href: '/dj-sets', label: 'DJ Sets' },
  {
    href: 'https://psychichomily.substack.com/',
    label: 'Substack',
    external: true,
  },
  { href: '/submissions', label: 'Submissions' },
]

function isExternal(link: (typeof navLinks)[number]): boolean {
  return 'external' in link && link.external === true
}

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
                className="px-3 py-1.5 text-sm font-medium rounded-md hover:bg-muted/50 hover:text-primary transition-colors"
              >
                {link.label}
              </Link>
            ))}
            {/* My List link - only show to authenticated users */}
            {isAuthenticated && (
              <Link
                href="/shows/saved"
                className="px-3 py-1.5 text-sm font-medium rounded-md hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-1.5"
              >
                <Heart className="h-3.5 w-3.5" />
                My List
              </Link>
            )}
            {/* Admin link - only show to admins */}
            {isAuthenticated && user?.is_admin && (
              <Link
                href="/admin"
                className="px-3 py-1.5 text-sm font-medium rounded-md hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-1.5"
              >
                <Shield className="h-3.5 w-3.5" />
                Admin
              </Link>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground hidden sm:block" />
          ) : isAuthenticated && user ? (
            <div className="hidden sm:flex items-center gap-3">
              <span className="text-sm text-muted-foreground truncate max-w-[150px]">
                {user.email}
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={logout}
                className="text-muted-foreground hover:text-primary"
              >
                <LogOut className="h-4 w-4" />
                <span className="sr-only">Sign out</span>
              </Button>
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
                {/* My List link - only show to authenticated users */}
                {isAuthenticated && (
                  <Link
                    href="/shows/saved"
                    onClick={() => setOpen(false)}
                    className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-2"
                  >
                    <Heart className="h-4 w-4" />
                    My List
                  </Link>
                )}
                {/* Admin link - only show to admins */}
                {isAuthenticated && user?.is_admin && (
                  <Link
                    href="/admin"
                    onClick={() => setOpen(false)}
                    className="text-lg font-medium px-4 py-3 rounded-lg hover:bg-muted/50 hover:text-primary transition-colors flex items-center gap-2"
                  >
                    <Shield className="h-4 w-4" />
                    Admin
                  </Link>
                )}

                {/* Mobile auth section */}
                {isLoading ? (
                  <div className="flex items-center justify-center py-3 sm:hidden">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                  </div>
                ) : isAuthenticated && user ? (
                  <div className="px-4 py-3 space-y-3 sm:hidden">
                    <div className="text-sm text-muted-foreground truncate">
                      Signed in as {user.email}
                    </div>
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
