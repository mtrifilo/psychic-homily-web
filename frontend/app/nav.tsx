'use client'

import { useState } from 'react'
import Image from 'next/image'
import Link from 'next/link'
import { Menu } from 'lucide-react'
import { ModeToggle } from '@/components/mode-toggle'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

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

      <nav className="flex w-full items-center justify-between px-4 py-2">
        <div className="flex items-center gap-4">
          <Link href="/" className="flex-shrink-0">
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
          <div className="hidden md:flex items-center gap-4">
            {navLinks.map(link => (
              <Link
                key={link.href}
                href={link.href}
                target={isExternal(link) ? '_blank' : undefined}
                rel={isExternal(link) ? 'noopener noreferrer' : undefined}
                className="hover:text-muted-foreground transition-colors"
              >
                {link.label}
              </Link>
            ))}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Link
            href="/auth"
            className="hidden sm:inline hover:text-muted-foreground transition-colors"
          >
            login / sign-up
          </Link>
          <ModeToggle />

          {/* Mobile Menu */}
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger asChild className="md:hidden">
              <Button variant="ghost" size="icon" aria-label="Open menu">
                <Menu className="h-5 w-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="right" className="w-[300px] sm:w-[400px]">
              <SheetHeader>
                <SheetTitle>Menu</SheetTitle>
              </SheetHeader>
              <nav className="flex flex-col gap-2 mt-8">
                {navLinks.map(link => (
                  <Link
                    key={link.href}
                    href={link.href}
                    target={isExternal(link) ? '_blank' : undefined}
                    rel={isExternal(link) ? 'noopener noreferrer' : undefined}
                    onClick={() => setOpen(false)}
                    className="text-lg px-4 py-3 rounded-md hover:bg-accent hover:text-accent-foreground transition-colors"
                  >
                    {link.label}
                  </Link>
                ))}
                <Link
                  href="/auth"
                  onClick={() => setOpen(false)}
                  className="text-lg px-4 py-3 rounded-md hover:bg-accent hover:text-accent-foreground transition-colors sm:hidden"
                >
                  login / sign-up
                </Link>
              </nav>
            </SheetContent>
          </Sheet>
        </div>
      </nav>
    </>
  )
}
