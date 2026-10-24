import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { MIN_SIGNUP_AGE } from '@/lib/legal'
import { StepCard, StepHeading } from './step-card'

/**
 * The under-age refusal every sign-up path ends on: no form, so nothing
 * invites a second date. `focusHeading` moves focus to its heading as it
 * mounts, for a card that replaces a form the person just submitted.
 */
export function AgeRefusedCard({ focusHeading }: { focusHeading: boolean }) {
  return (
    <StepCard>
      <StepHeading
        eyebrow="Account not created"
        title="We can't create an account for you."
        focusOnMount={focusHeading}
      />
      <p className="text-sm leading-[22px] text-foreground">
        Psychic Homily accounts are for people {MIN_SIGNUP_AGE} and over, so we
        haven&apos;t created one. You can still browse shows, artists and venues
        without an account.
      </p>
      <Button asChild className="w-full">
        <Link href="/shows">Browse upcoming shows</Link>
      </Button>
    </StepCard>
  )
}
