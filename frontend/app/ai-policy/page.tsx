import { Metadata } from 'next'
import Link from 'next/link'
import {
  AI_POLICY_COPY,
  AI_POLICY_PATH,
  AI_POLICY_TITLE,
  COPY_PENDING_PLACEHOLDER,
  isAiPolicyCopyPending,
  isCopySlotBlank,
  type CopySlot,
} from './content'

/**
 * The public AI policy page.
 *
 * Deliberately contains NO prose. Every word a reader sees comes from
 * `./content.ts`, which is the one file the owner edits — read the header there
 * before changing anything here.
 *
 * Layout is the existing static-page shell (`/terms`, `/privacy` and
 * `/help/tiers` all use it verbatim): a centred `max-w-3xl` main with `prose`
 * sections. No new visual treatment, so the Figma-first gate does not apply.
 */

const copyPending = isAiPolicyCopyPending(AI_POLICY_COPY)

export const metadata: Metadata = {
  // Bare title on purpose: the root layout's `template: '%s | Psychic Homily'`
  // appends the suffix. /terms and /privacy hardcode it too and render
  // "… | Psychic Homily | Psychic Homily" — a pre-existing bug not copied here.
  title: AI_POLICY_TITLE,
  // A page held back for missing copy must not advertise itself: no
  // description to surface in a preview, and an explicit noindex. Both derive
  // from the same copy object that drives the on-page banner and the sitemap
  // entry, so the three cannot drift apart.
  ...(isCopySlotBlank(AI_POLICY_COPY.description)
    ? {}
    : { description: AI_POLICY_COPY.description }),
  alternates: {
    canonical: `https://psychichomily.com${AI_POLICY_PATH}`,
  },
  ...(copyPending ? { robots: { index: false, follow: false } } : {}),
}

function Paragraphs({ body }: { body: CopySlot }) {
  if (isCopySlotBlank(body)) {
    return (
      <p className="text-destructive font-mono text-sm leading-relaxed">
        {COPY_PENDING_PLACEHOLDER}
      </p>
    )
  }

  return (
    <>
      {/*
        Index keys: the list is static content read straight off a module
        constant, never reordered or filtered at runtime, and two paragraphs of
        the copy could legitimately be identical.
      */}
      {(body ?? []).map((paragraph, index) => (
        <p
          key={index}
          className="text-foreground/90 leading-relaxed mb-3 last:mb-0"
        >
          {paragraph}
        </p>
      ))}
    </>
  )
}

export default function AiPolicyPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">
          {AI_POLICY_TITLE}
        </h1>
        {!isCopySlotBlank(AI_POLICY_COPY.lastUpdated) && (
          <p className="text-center text-muted-foreground mb-8">
            Last Updated: {AI_POLICY_COPY.lastUpdated}
          </p>
        )}

        {copyPending && (
          <div
            role="alert"
            className="mb-8 rounded-md border-2 border-destructive bg-destructive/10 p-4 text-destructive"
          >
            <p className="font-mono text-sm font-bold uppercase tracking-wide">
              Unpublished draft — not real policy copy
            </p>
            <p className="mt-2 text-sm leading-relaxed">
              This page is scaffolding. The policy text has not been written
              yet, so the page is marked noindex and is kept out of the sitemap.
              Nothing below is a statement of policy.
            </p>
          </div>
        )}

        <div className="prose prose-neutral dark:prose-invert max-w-none space-y-8">
          <section>
            <Paragraphs body={AI_POLICY_COPY.intro} />
          </section>

          {AI_POLICY_COPY.sections.map(section => (
            <section key={section.id} id={section.id}>
              <h2 className="text-xl font-semibold mb-3">{section.heading}</h2>
              <Paragraphs body={section.body} />
            </section>
          ))}
        </div>

        <div className="mt-12 pt-6 border-t border-border text-center text-sm text-muted-foreground">
          <Link href="/terms" className="underline hover:text-foreground">
            Terms of Service
          </Link>
          {' · '}
          <Link href="/privacy" className="underline hover:text-foreground">
            Privacy Policy
          </Link>
          {' · '}
          <Link href="/" className="underline hover:text-foreground">
            Return to Home
          </Link>
        </div>
      </main>
    </div>
  )
}
