'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { SectionHeader } from '@/components/shared/SectionHeader'

/**
 * Owner-only onboarding checklist (PSY-1045, design board B): shown instead
 * of empty profile sections when the owner has no content yet, so a new user
 * sees concrete next steps rather than a wall of nothing. Also used by the
 * /users/me claim-username self view.
 */
export function GetStartedChecklist() {
  const steps = [
    {
      n: '1',
      title: "Save a show you're into",
      sub: 'Build your list — and add to the show’s buzz',
      href: '/shows',
      cta: 'Find shows',
    },
    {
      n: '2',
      title: 'Follow artists you love',
      sub: 'Shape your taste graph and get show alerts',
      href: '/artists',
      cta: 'Browse',
    },
    {
      n: '3',
      title: 'Start your first collection',
      sub: 'Curate a list worth sharing',
      href: '/collections',
      cta: 'New list',
    },
  ]
  return (
    <section aria-label="Get started">
      <SectionHeader title="Get started" as="h2" size="md" />
      <div className="mt-1 divide-y divide-border/60">
        {steps.map(step => (
          <div key={step.n} className="flex items-center gap-4 py-3">
            <span className="w-5 shrink-0 font-mono text-sm font-bold text-primary">
              {step.n}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{step.title}</p>
              <p className="text-xs text-muted-foreground">{step.sub}</p>
            </div>
            <Button asChild variant="outline" size="sm" className="shrink-0">
              <Link href={step.href}>{step.cta}</Link>
            </Button>
          </div>
        ))}
      </div>
    </section>
  )
}
