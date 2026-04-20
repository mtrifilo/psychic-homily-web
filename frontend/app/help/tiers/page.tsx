import { Metadata } from 'next'
import Link from 'next/link'
import { TIERS } from '@/lib/tiers'

export const metadata: Metadata = {
  title: 'Contributor Tiers | Psychic Homily',
  description:
    'How contributor trust tiers work on Psychic Homily: what each tier can do and the criteria to advance.',
  alternates: {
    canonical: 'https://psychichomily.com/help/tiers',
  },
}

export default function TiersHelpPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold mb-2">Contributor Tiers</h1>
        <p className="text-muted-foreground mb-8">
          Trust tiers control what you can do without review. You start at New
          User and advance automatically as you contribute — we do not handle
          promotions manually.
        </p>

        <div className="space-y-8">
          {TIERS.map(tier => (
            <section
              key={tier.tier}
              id={tier.tier}
              className="rounded-lg border border-border/60 p-6"
            >
              <h2 className="text-xl font-semibold mb-2">{tier.label}</h2>
              <p className="text-foreground/90 leading-relaxed mb-4">
                {tier.summary}
              </p>

              <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-2">
                What you can do
              </h3>
              <ul className="list-disc pl-5 space-y-1 mb-4 text-sm">
                {tier.permissions.map(perm => (
                  <li key={perm}>{perm}</li>
                ))}
              </ul>

              {tier.advancementRequirements && (
                <>
                  <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-2">
                    How to reach this tier
                  </h3>
                  <ul className="list-disc pl-5 space-y-1 text-sm">
                    {tier.advancementRequirements.map(req => (
                      <li key={req.text}>{req.text}</li>
                    ))}
                  </ul>
                </>
              )}
            </section>
          ))}
        </div>

        <div className="mt-10 rounded-md border border-border/60 bg-muted/30 p-4 text-sm text-muted-foreground">
          <p className="mb-2">
            Advancement is evaluated automatically on a daily schedule.
            Approval rate is computed across your pending edits and
            direct revisions.
          </p>
          <p>
            Questions?{' '}
            <Link
              href="/profile"
              className="underline hover:text-foreground"
            >
              View your profile
            </Link>{' '}
            to see your current tier.
          </p>
        </div>
      </main>
    </div>
  )
}
