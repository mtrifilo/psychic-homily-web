---
name: psy-self-review
description: Sub-agent review of an in-flight PR draft against session evidence. Catches unverified `[x]` test-plan claims, missing coverage-gap disclosure, asymmetric peer-component checks, and convention drift. Runs AFTER `/simplify` and BEFORE push as the final gate. Pattern matches `/simplify` (3 parallel reviewer sub-agents); failure forces the agent back to the verification phase rather than letting an unverified claim ship.
argument-hint: "[optional: path to draft PR body markdown]"
---

# psy-self-review: pre-push evidence-of-verification audit

Encodes the convention that PR test-plan `[x]` items are statements of "I verified this," not aspirations. Spawns 3 parallel reviewer sub-agents to check the in-flight PR draft against the session's evidence trail (bash log, screenshots, test runs). Same shape as `/simplify`; runs immediately after `/simplify` and before `git push`.

Born out of the May 16–17 Entity Pages Density Rollout retro: PSY-658 PR claimed `[Add to collection]` would render for unauth viewers without verifying — that claim was wrong, and the empty-linkbox bug shipped to main, caught only by a post-shipped audit (PSY-663). PSY-664 (Graph Dialog hash bug) had the same shape: assumed-correct CLOSE path, never actually clicked. This skill is the gate that catches that pattern before push.

## When this skill fires

The user invokes it explicitly via `/psy-self-review`, OR an upstream skill (`/psy-solo` phase 8.5, `/psy-dispatch` per-agent step 8.5) calls it as part of the standard pre-push sequence.

Do NOT invoke for:
- Backend-only / docs-only / config-only PRs (test plan is integration-test or "docs-only, no manual repro applicable"; nothing for the sub-agents to evidence-check beyond what `/simplify` already covers).
- The `/simplify` pass itself — different skill, different concerns.

## What this skill does NOT do

Honest about the scope so callers don't over-rely:
- **Cannot verify a screenshot actually shows what the PR claims.** This skill checks `/tmp/psy-XXX.png` EXISTS and was taken in the session; it can't semantically check that the screenshot proves "unauth viewer sees `[Add to collection]`." Use Read on the PNG (separate caller responsibility) to visually verify content.
- **Cannot catch logic bugs in tested code.** This is an evidence-trail audit, not a test-suite audit. Code correctness is `/simplify` + the unit/integration tests.
- **Cannot infer behavior the diff doesn't explicitly call out.** If a diff changes 5 things but the test plan only mentions 3, the sub-agent will flag the omission BUT can't tell you what the missing 2 should test. Up to the agent + reviewer to decide.

This is a process gate, not a correctness oracle.

## Prerequisites

- `/simplify` has run and any code changes from it have been committed.
- The draft PR body exists either (a) inline in the agent's recent conversation context, (b) at a passed-in markdown path, or (c) as the already-opened PR's body via `gh pr view <#> --json body`.
- The session bash log is accessible via `/private/tmp/claude-501/<project>/<session>/tasks/*.output` files.

## The three sub-agents

Same parallel-agent dispatch pattern as `/simplify`. Send all three Agent calls in a SINGLE assistant message so the harness runs them concurrently.

### Agent 1 — Test-plan evidence audit

For each `[x]` item in the draft PR body's `## Test plan` section, check the session bash log + on-disk artifacts for evidence:

| Claim shape | Evidence required |
|---|---|
| `[x] bun run typecheck — clean` | session bash log shows the command + zero error output |
| `[x] bun run test:run features/X — N/N passing` | session bash log shows the command + matching "N passed" output |
| `[x] go test ./pkg/... — ok` | session bash log shows the command + zero "FAIL" output |
| `[x] /simplify — clean` (or with diff outcome) | recent agent run with `Skill(skill: "simplify")` invocation; simplify's reported outcome matches the claim |
| `[x] Manual repro: <description>` | at least one `agent-browser open <relevant-url>` + screenshot taken in the session; OR a `curl` invocation for backend; OR a referenced integration test |
| `[x] Manual repro: unauth + auth states` | at least TWO navigations + screenshots; flag if only one |
| `[x] Manual repro: rich + sparse data` | at least TWO distinct entity slugs navigated; flag if only one |

For each `[x]` without sufficient evidence: flag with the specific gap (e.g. `[x] verified unauth state — but no agent-browser screenshot found in session`). Recommend downgrading to `[ ]` and re-doing the verification.

Special case: `[x] Manual repro: <some-description>` items that depend on bracket variants or behavior visible only with auth — flag explicitly if no auth-state screenshot was taken. The PSY-658 → PSY-663 miss was exactly this shape (auth-only `[Add to collection]` was assumed to render for unauth; never visually checked).

### Agent 2 — Diff vs claim coverage gap

Read `git diff main` and the PR body's `## Summary` + `## Test plan` sections. For each significant behavior change in the diff, check whether the test plan mentions it:

- New `BracketLink` added to a header → test plan should mention `[x] verified the new bracket renders for [auth state]`.
- New conditional render (`{condition && (...)}`) → test plan should mention `[x] verified BOTH true and false branches`.
- New API call in a hook → test plan should mention `[x] verified the response shape; verified loading + error states`.
- New `onClick` / `onOpenChange` / `onSuccess` handler → test plan should mention `[x] exercised the handler end-to-end`.
- Removed prop / removed conditional → test plan should mention `[x] verified the removed-state matches the new default`.

For each behavior the diff introduces but the test plan doesn't claim to have exercised: flag as a coverage gap. Recommend adding to `## Coverage gaps` (honest disclosure) OR going back to phase 6 to actually exercise it.

This catches the PSY-664 shape: the diff introduced a close-path branch (`onOpenChange` handler) but the test plan only claimed OPEN-path verification. CLOSE was assumed-correct and shipped broken.

### Agent 3 — Convention + asymmetry check

Sanity-check the PR body for project-convention drift + scan the diff for known asymmetry traps:

- PR title format: `PSY-{N}: <under 70 chars>` — **NOTE**: PR title is passed to `gh pr create --title` SEPARATELY from the body. If the title isn't supplied alongside the body in this skill's input, mark as **N/A — title not in scope**; recommend the calling agent verify length manually before `gh pr create`. Future improvement: caller passes `title` as a separate input.
- PR body contains `Closes PSY-{N}` — flag if missing.
- PR body does NOT contain unresolved `PSY-XXX` / `PSY-???` / `PSY-{N}` placeholder strings — these slip through when follow-up filing (phase 7) happens AFTER the PR body is drafted. Flag any placeholder.
- `## Coverage gaps` section present (or explicitly stated as "no gaps") — flag if missing entirely; reviewers need to know what wasn't exercised.
- `## Heads up from /simplify` (or `## Findings from /simplify`) present if simplify surfaced concerns; absent if `/simplify` was clean.
- `## Deferred (per pre-implementation Q&A)` section present if any spike questions resulted in "skip + file follow-up" decisions during phase 2; absent if none.
- For frontend diffs touching shared components: if `if (!isAuthenticated) return null` is added/modified, check peer components (FollowButton.tsx, NotifyMeButton.tsx, AddToCollectionButton.tsx, EntityTagList.tsx, etc.) for the convention. Asymmetric unauth fallbacks is the PSY-663 footgun.
- For frontend diffs gating on optional API data: check the gate uses shape-aware predicates (`Object.values(x).some(...)`, `.length > 0`) not truthy-only (`!!x` where `x` could be `{}` or `[]`). PSY-657 root cause.
- For frontend diffs adding dialog open/close paths: check the URL-hash-cleanup symmetry (PSY-664 root cause) — if open SETS a hash, close should CLEAR it.
- **For frontend diffs passing year-shape numeric values to `<StatsList />`** (`founded_year`, `edition_year`, `release_year`, `established_year`, etc.): check the value is wrapped in `String()` to bypass `Intl.NumberFormat` thousands separator. **Caught: PSY-656 manual repro** — 4AD's `founded_year: 1980` rendered as `1,980` in the sidebar. Fix: `value: String(label.founded_year)` with an inline comment explaining the why.

For each finding: flag with a specific file:line reference + the convention/asymmetry it violates.

## Workflow

```
1. Receive (or locate) the draft PR body. Three sources, in order of preference:
   a. Caller passed `argument-hint` with a markdown file path → read it.
   b. PR already opened via `gh pr create` → fetch via `gh pr view <#> --json body --jq .body`.
   c. Draft inline in the agent's conversation context → caller must extract and pass.

2. Locate the session bash log directory:
   `/private/tmp/claude-501/<project-hash>/<session-id>/tasks/*.output`
   Use Bash with `ls -t` to find the most recent session if unsure.

3. Get the git diff:
   `git -C <main-repo> diff main` (or `gh pr view <#> --json files` for an opened PR)

4. Spawn 3 Agent calls in parallel (single assistant message, all three subagent_type: "general-purpose"):
   - Agent 1: Test-plan evidence audit (per above)
   - Agent 2: Diff vs claim coverage gap (per above)
   - Agent 3: Convention + asymmetry check (per above)

   Each agent gets:
   - The draft PR body text (inline in the prompt)
   - The git diff (truncated to relevant hunks if very large; ~10k char cap)
   - The list of session bash log file paths + screenshot paths
   - The skill-specific responsibility (one of the three above)

5. Aggregate findings as a single report. Group by severity:
   - **BLOCKING** — `[x]` items with zero evidence. Cannot push until re-verified or downgraded to `[ ]`.
   - **WARNING** — coverage gaps the diff introduces. Recommend adding `## Coverage gaps` section disclosure OR re-doing phase 6.
   - **NIT** — convention drift (PR title length, missing sections). Easy to fix in the PR body before push.

6. Report back to the calling agent. If any BLOCKING findings: STOP. The agent fixes (re-verifies / downgrades / re-screenshots) and re-invokes `/psy-self-review`. If only WARNING + NIT: report inline; agent decides whether to fix or document.
```

## Output format

A single structured report to the calling agent:

```
## /psy-self-review report

### Blocking (must fix before push)
- [x] "Manual repro: unauth viewer sees [Add to collection] in linkbox" — NO evidence: no agent-browser navigation to /releases/<slug> found in session log; no /tmp/psy-XXX-release.png screenshot. Action: navigate + screenshot, OR downgrade to [ ] with explicit "not verified locally" note in body.

### Warning (consider addressing before push)
- Diff introduces close-path handler at ArtistDetail.tsx:1051 (`onOpenChange={(open) => ...}`). Test plan claims OPEN path only. Recommend [x] verifying close also clears the URL hash, OR adding "Coverage gaps: X-close path not exercised locally" to PR body.

### Nit (cosmetic)
- PR title "PSY-657: FestivalDetail density — remove 4-tab system + BracketLink linkbox" is 69 chars. Within budget but tight.

### Pass
- /simplify outcome matches claim ✓
- Test-plan typecheck + scoped test claims have matching command output ✓
```

## Per-agent prompt template

Each sub-agent's prompt MUST be self-contained (the sub-agent has none of this conversation's context). Fill in placeholders.

### Agent 1 prompt (evidence audit)

```markdown
You are auditing a PR draft against session evidence.

Draft PR body:
<paste full body verbatim>

Session bash log directory: <path>
Files in /tmp/ from this session: <ls /tmp/psy-* output>

Your job: scan the `## Test plan` section. For each `[x]` item, check the session bash log + on-disk artifacts for evidence (commands run, screenshots taken, tests passed). Use this mapping:

| Claim shape | Evidence required |
|---|---|
| `[x] bun run typecheck — clean` | session log shows the command + zero error output |
| `[x] bun run test:run features/X — N/N passing` | session log shows the command + matching "N passed" output |
| `[x] /simplify — clean` | recent Skill(skill: "simplify") invocation; outcome matches |
| `[x] Manual repro: <description>` | at least one agent-browser navigation + screenshot in the session |
| `[x] Manual repro: unauth + auth states` | TWO navigations + screenshots; flag if only one |
| `[x] Manual repro: rich + sparse data` | TWO distinct entity slugs navigated; flag if only one |

Report findings as:
- **Blocking**: claim with no evidence → action recommendation
- **Pass**: claim with evidence
- Skip "warning" / "nit" — those belong to other agents.

Under 250 words.
```

### Agent 2 prompt (coverage gap)

```markdown
You are auditing whether a PR's test plan covers all the behaviors the diff introduces.

Git diff (truncated to relevant hunks):
<paste diff>

Draft PR body's `## Summary` and `## Test plan` sections:
<paste both>

Your job: identify behavior changes in the diff that the test plan does NOT claim to have exercised. Common shapes:
- New `BracketLink` added → test plan should claim "[x] verified bracket renders for <auth state>"
- New conditional `{cond && (...)}` → test plan should claim "[x] verified both branches"
- New onClick/onOpenChange/onSuccess handler → test plan should claim "[x] exercised handler end-to-end"
- New API call → test plan should claim "[x] verified loading + error + populated states"
- Removed prop/conditional → test plan should claim "[x] verified the new default matches the removed-state contract"

For each gap, report:
- The diff hunk + behavior introduced
- What the test plan should have claimed
- Recommended action: add to `## Coverage gaps` section (honest disclosure) OR re-do phase 6 to actually exercise it

Under 250 words. Skip code-correctness review (that's /simplify's job).
```

### Agent 3 prompt (convention + asymmetry)

```markdown
You are auditing a PR draft for project convention drift + known asymmetry traps.

Draft PR body:
<paste full body>

Git diff (truncated to relevant hunks):
<paste diff>

Your job — check for:

**PR convention**:
- Title format `PSY-{N}: <under 70 chars>` — **NOTE**: PR title is passed to `gh pr create --title` separately. If the title isn't supplied here, mark as `N/A — title not in scope` and recommend the calling agent verify length manually before `gh pr create`.
- Body contains `Closes PSY-{N}`
- Body does NOT contain unresolved `PSY-XXX` / `PSY-???` placeholder strings (slips through when follow-up filing happens after PR-body drafting)
- `## Coverage gaps` section present (or explicitly stated as "no gaps")
- `## Heads up from /simplify` (or `## Findings from /simplify`) if /simplify surfaced concerns; absent if clean
- `## Deferred` section if phase 2 produced "skip + file follow-up" decisions

**Frontend asymmetry traps**:
- If `if (!isAuthenticated) return null` is added/modified in a shared component: check that peer components (FollowButton, NotifyMeButton, AddToCollectionButton, EntityTagList) have consistent unauth-fallback patterns. Asymmetric fallbacks were PSY-663's root cause (AddToCollectionButton returned null while Follow/Notify rendered).
- If gating render on optional API data: gate must use shape-aware predicates (`Object.values(x).some(...)`, `.length > 0`), NOT truthy-only (`!!x` where x could be `{}` or `[]`). PSY-657 root cause was `!!festival.social` being true when `social: {}`.
- If adding dialog open/close paths with URL-hash management: OPEN setting a hash means CLOSE must CLEAR the hash. PSY-664 root cause was an asymmetric close.
- If passing a year-shape numeric value to `<StatsList />` (founded_year, edition_year, release_year, established_year, etc.): check the value is wrapped in `String()` to bypass `Intl.NumberFormat` thousands separator. PSY-656 root cause: `1980` rendered as `1,980` in the 4AD sidebar.

For each finding: cite file:line + the specific convention/asymmetry violated.

Under 250 words.
```

## Anti-patterns

- **Running `/psy-self-review` AFTER pushing.** The whole point is to catch unverified claims before they ship. Push → audit → fix-PR is the May 16 anti-pattern this skill exists to break. Run BEFORE push, every time.
- **Treating BLOCKING findings as advisory.** Per `feedback_simplify_before_pr.md`'s parallel: `[x]` is a statement of "I verified this." If the evidence isn't there, the claim is false. Don't push past with "I know it works" — re-verify or downgrade.
- **Stuffing `[x] all manual repro covered` as a single line to bypass per-claim checks.** The skill checks the granular `[x]` items. Compound claims defeat the per-claim audit. Break into specific verifiable items: `[x] unauth viewer at /releases/X — empty header confirmed`; `[x] auth viewer at /releases/X — 4 brackets render`; `[x] click [Add to collection] while unauth — redirects to /auth`.
- **Skipping `/psy-self-review` for "small" PRs.** Discipline > efficiency. Small PRs have small diffs which are FAST for the sub-agents (often the report is < 30 seconds end-to-end). Skipping defeats the convention.
- **Trusting Agent 1's PASS without spot-checking the screenshot.** Agent 1 verifies a screenshot file EXISTS; it can't verify the screenshot SHOWS what was claimed. Spot-Read the PNG yourself if the claim is load-bearing for review.

## Related skills and memories

- **`/psy-solo`** — single-ticket workflow that invokes this skill at phase 8.5 (between `/simplify` and the PR open).
- **`/psy-dispatch`** — parallel-worktree dispatch; per-agent template should add a `/psy-self-review` step before the `gh pr create` call. (Update pending; will land alongside /psy-self-review commit.)
- **`/simplify`** — runs immediately before `/psy-self-review`. Different concern (code quality + reuse + efficiency) but same parallel-sub-agent shape.
- **`/psy-audit` (planned, post-PSY-656)** — multi-page post-shipped audit. Complements this skill: `/psy-self-review` catches pre-push; `/psy-audit` catches what slipped through.
- `feedback_simplify_before_pr.md` — same rule shape: failure blocks push, escalate to user instead of pushing past.
- `feedback_verify_before_push.md` — verify the fix actually fixes the thing before pushing.

## Caveats / known limitations

- **Cannot read the conversation context directly** — the calling agent must extract the draft PR body and pass it (or write it to /tmp first). This is a harness limitation, not a skill design choice.
- **Session bash log paths are harness-specific** — `/private/tmp/claude-501/...` works on this machine; future harness changes may break the path. Worth re-pointing if the harness migrates.
- **Sub-agents are stateless** — each invocation re-loads the diff + PR body + log paths. For large diffs, the cost is non-trivial. If the diff > 50k chars, truncate to the most relevant hunks.
- **False positives on `[x]` evidence detection** — bash log search uses keyword matching, which can miss commands run via `Skill` invocations (the skill's internal commands don't always surface to the session log). Cross-reference with the agent's own task list before declaring "no evidence."
