/**
 * ActivityFeedPlaceholder (PSY-389) — the reserved "Across the scene" slot on
 * the logged-out homepage (Figma `491:29`, section "Across the scene").
 *
 * Per the PSY-389 decision this is a tasteful RESERVED placeholder for v1, NOT
 * a live feed. The real activity feed is owned by the separate "Following Feed
 * — Personalized Activity" project (PSY-988–991): logged-out will show a public
 * "across the scene" pulse, logged-in shows activity from who/what you follow,
 * powered by the `activity_events` spine. That project's feed-surface ticket
 * delivers this placement; here we ship only the reserved dashed slot so the
 * page layout is final and the cross-project dependency is visible.
 *
 * The sample rows are illustrative of the eventual feed shape — they are static
 * examples inside the placeholder, deliberately not wired to any data source.
 */

const SAMPLE_ITEMS: ReadonlyArray<{ text: string; age: string }> = [
  { text: '@scenekid added 3 shows in Phoenix', age: '2h' },
  { text: 'New collection — “Pacific NW Post-Hardcore” by @oly92', age: '5h' },
  { text: 'Field note on Bikini Kill @ The Rebel Lounge by @riotfan', age: '6h' },
  { text: 'Unwound · “Repetition” linked to Kill Rock Stars', age: '1d' },
]

export function ActivityFeedPlaceholder() {
  return (
    <section
      aria-labelledby="home-activity-heading"
      className="flex w-full flex-col gap-4"
    >
      <div className="flex items-center justify-between">
        <h2
          id="home-activity-heading"
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          Across the scene
        </h2>
        <span className="font-mono text-xs text-muted-foreground">
          Following Feed · in design →
        </span>
      </div>

      <div className="flex flex-col gap-[13px] rounded-xl border-[1.5px] border-dashed border-border px-[22px] py-5">
        <p className="text-[15px] font-semibold text-foreground">
          Reserved for the Activity feed
        </p>
        <p className="text-[13px] text-muted-foreground">
          Logged-out: a public &ldquo;across the scene&rdquo; pulse. Logged-in:
          activity from the artists, labels, venues, radio shows &amp; people you
          follow. Powered by the <code>activity_events</code> spine.
        </p>

        <ul className="flex flex-col gap-[9px] py-1" aria-hidden>
          {SAMPLE_ITEMS.map(item => (
            <li key={item.text} className="flex items-center gap-[9px]">
              <span className="text-[8px] text-primary">●</span>
              <span className="flex-1 text-[13px] font-medium text-muted-foreground">
                {item.text}
              </span>
              <span className="font-mono text-[11px] text-muted-foreground">
                {item.age}
              </span>
            </li>
          ))}
        </ul>

        <p className="font-mono text-[11px] text-muted-foreground">
          ↳ Designed in the &ldquo;Following Feed — Personalized Activity&rdquo;
          Linear project (PSY-988&ndash;991).
        </p>
      </div>
    </section>
  )
}
