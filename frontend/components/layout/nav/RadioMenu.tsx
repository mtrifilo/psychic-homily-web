'use client'

import { useState } from 'react'
import { usePathname } from 'next/navigation'
import { ChevronDown } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RadioPanel } from '@/features/radio'
import { isNavActive, navItemClassName } from './navData'

// Radio ▾ — PSY-1016 swaps the minimal PSY-1013 dropdown for the rich Option-D2
// "station overview" panel: a click-to-open, two-pane Popover (station list +
// the selected station's identity, Now Playing, and recent shows, with every
// artist / release / label a one-click graph hop). Click-to-open (not hover)
// because the panel is rich; the same RadioPanel layout powers the /radio page.
// Real providers only: KEXP / WFMU / NTS.
export function RadioMenu() {
  const pathname = usePathname()
  const active = isNavActive(pathname, '/radio')
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger className={navItemClassName(active)} aria-label="Radio">
        Radio
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={8}
        className="w-[640px] max-w-[calc(100vw-2rem)] overflow-hidden p-0 shadow-[0px_2px_8px_0px_rgba(0,0,0,0.08)]"
      >
        <RadioPanel onNavigate={() => setOpen(false)} />
      </PopoverContent>
    </Popover>
  )
}
