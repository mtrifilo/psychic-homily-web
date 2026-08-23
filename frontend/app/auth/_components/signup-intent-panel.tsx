/**
 * Intent panel for the /auth "Create account" tab (PSY-1900).
 *
 * Replaces the old "Sign up to submit shows and join the community" line. The
 * ledger states what an account is FOR before the form asks for an email,
 * ordered the way a new listener actually meets those capabilities: save and
 * follow first, alerts as the payoff, submission last.
 *
 * Deliberately local to `app/auth`. PSY-1901 renders a similar four-line block
 * on the verify-email landing; hoisting this into a shared component before
 * both copies have settled would couple two surfaces that are still moving.
 */

interface LedgerRow {
  /** Short all-caps capability name shown in the left column. */
  label: string
  detail: string
}

const LEDGER_ROWS: readonly LedgerRow[] = [
  { label: 'SAVE', detail: 'shows you plan to catch' },
  { label: 'FOLLOW', detail: 'artists and venues you care about' },
  { label: 'ALERTS', detail: 'hear when they announce something near you' },
  { label: 'SUBMIT', detail: 'for completists: add what we are missing' },
]

/**
 * Monospace columns the label plus its dot leader occupies. A fixed total is
 * what makes the detail column line up in the mono face; every label above is
 * short enough to leave at least one dot.
 */
const LEADER_COLUMNS = 12

function dotLeaderFor(label: string): string {
  return ' ' + '.'.repeat(Math.max(1, LEADER_COLUMNS - label.length - 1))
}

export function SignupIntentPanel() {
  return (
    <div className="space-y-4">
      <p className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
        Psychic Homily account
      </p>

      <h2 className="font-display text-[26px] font-bold leading-tight text-foreground">
        Never miss a show.
      </h2>

      {/*
        The dot leaders are decoration, not content: a screen reader announcing
        "SAVE dot dot dot dot dot dot dot" would be worse than useless, so they
        are hidden and the term/detail pairing carries the meaning instead.
      */}
      <dl className="font-mono text-xs leading-[22px] text-foreground">
        {LEDGER_ROWS.map(row => (
          <div key={row.label} className="flex gap-[1ch]">
            <dt className="shrink-0 whitespace-pre">
              {row.label}
              <span aria-hidden="true" className="text-muted-foreground">
                {dotLeaderFor(row.label)}
              </span>
            </dt>
            <dd className="min-w-0">{row.detail}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

interface SignupIntentFooterProps {
  /** Switches the auth card to the sign-in tab. */
  onSignInClick: () => void
}

export function SignupIntentFooter({ onSignInClick }: SignupIntentFooterProps) {
  return (
    <p className="text-xs text-muted-foreground">
      Already have an account?{' '}
      <button
        type="button"
        onClick={onSignInClick}
        className="underline underline-offset-4 hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
      >
        Sign in
      </button>
      {' · '}
      You choose what shows on your public profile.
    </p>
  )
}
