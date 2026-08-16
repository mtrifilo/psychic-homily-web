'use client';

import * as React from 'react';
import { Moon, Sun } from 'lucide-react';
import { useTheme } from 'next-themes';

import { Button } from '@/components/ui/button';
import { replayOnHydrate } from '@/lib/hydration/clickReplay'

// The one theme-flip implementation (PSY-1818). Every theme control consumes
// this instead of re-deriving the flip inline — the top bar, the mobile Browse
// sheet, the hero lab, and ModeToggle below. Four copies had drifted apart
// before this hook existed, and the same regression had to be pinned by a test
// in each of them.
//
// Keys off resolvedTheme, NOT theme, so the first click always flips the
// VISIBLE theme: under theme === 'system' on a dark device a `theme === 'dark'`
// check would set an explicit 'dark' and appear to do nothing. next-themes also
// reports resolvedTheme as undefined until it has read storage, which falls to
// the 'dark' branch here rather than throwing.
//
// `label` names the ACTION ("Light mode" while dark), for callers that render
// text rather than a sun/moon glyph.
export function useThemeToggle(): {
  isDark: boolean;
  toggle: () => void;
  label: string;
} {
  const { resolvedTheme, setTheme } = useTheme();
  const isDark = resolvedTheme === 'dark';

  const toggle = React.useCallback(() => {
    setTheme(isDark ? 'light' : 'dark');
  }, [isDark, setTheme]);

  return { isDark, toggle, label: isDark ? 'Light mode' : 'Dark mode' };
}

export function ModeToggle() {
  const { toggle } = useThemeToggle();

  return (
    <Button {...replayOnHydrate} variant='outline' size='icon' className='cursor-pointer' onClick={toggle}>
      <Sun className='h-[1.2rem] w-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90' />
      <Moon className='absolute h-[1.2rem] w-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0' />
      <span className='sr-only'>Toggle theme</span>
    </Button>
  );
}
