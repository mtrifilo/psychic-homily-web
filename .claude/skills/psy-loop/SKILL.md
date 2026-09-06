---
name: psy-loop
description: Run a Linear project's backlog to zero as a standing pipeline of dispatched worktree agents. Use when the user says "work the backlog in groups", "keep the slots full", "loop until the project is done", "continue with the looping slots", or hands over a project view and asks to drive it to completion. Owns slot accounting, tiered review, family PRs, conflict shepherding, disposition records, follow-up filing, and the deploy cadence. Builds on psy-dispatch (single batch) and psy-ticket (filing).
argument-hint: "[Linear project name] [slots=2|3]"
---

# psy-loop: drive a project backlog to zero, N slots at a time

`psy-dispatch` runs one batch of tickets and stops. `psy-loop` keeps N implementation slots full until a project's non-done ticket count reaches zero, while the owner merges PRs, answers design questions, and decides when to deploy. It was distilled from the Show Page Redesign close-out (2026-08-30 to 2026-09-06: ~45 tickets, ~30 PRs, 4 production deploys, one orchestrator session).

## When this skill fires

- The user names a Linear project and asks to keep working it "in groups", "two at a time", "until done", or "keep the slots full".
- A prior loop session's handoff says which tickets are running and which are next.

Do not use it for a single ticket (`psy-solo`) or a one-off batch with no follow-on (`psy-dispatch`).

## The contract with the owner

| The orchestrator | The owner |
| --- | --- |
| Verifies every ticket premise against `origin/main` before dispatch | Merges PRs (never the orchestrator or an agent) |
| Keeps N slots full; never exceeds N | Sets N (default 2; 3 when the backlog is growing faster than it shrinks) |
| Records a disposition comment on every ticket when its PR opens | Answers design forks raised via `AskUserQuestion` |
| Files follow-up tickets that meet the bar; lists the rest on the ticket | Decides deploys ("deploy if confident" is the trigger, then `psy-deploy-prod` in full) |
| Shepherds conflicts by resuming the original agent | Reports red CI or conflicts on a specific PR |
| Calls diminishing returns explicitly, and keeps building when overruled | Overrules: "until the remaining tickets are done" means all of them |

## Slot loop

Repeat until the project has no ticket outside Done/Canceled/Duplicate:

1. **Inventory.** `linear issue list --project "<name>" --team PSY --all-assignees --all-states --limit 0 --no-pager`, strip ANSI, drop Done/Canceled/Duplicate. `--json` output from this command is not parseable in 1.11.x; use the text form.
2. **Pick.** Fill free slots with the highest-priority tickets whose files do not overlap a running slot (check the running briefs). Tightly related siblings become one FAMILY PR (one branch, per-ticket commits, body closes each).
3. **Verify the premise.** Grep or read the cited code on `origin/main`. Fixed already, wrong, or partially wrong: post a correction comment on the ticket BEFORE dispatch and scope the brief to what remains. Canonical: PSY-2000 residual 2 was already fixed; PSY-1993's premise was the orchestrator's own conflation of two contracts (memory `feedback_verify_ticket_hypothesis_first`).
4. **Resolve forks.** Any undecided WHAT (copy, threshold, refuse-vs-restamp, Figma-or-not) goes to the owner in one `AskUserQuestion` round while other slots run; record the answers as a dated, "binding" Linear comment; the brief cites that comment.
5. **Tier.** P2 / security / data-integrity: full fresh-panel adversarial review, up to 3 rounds. P3: one fresh four-lens round. P4: gates plus one inline self-check pass, no panel. State the tier in the brief.
6. **Brief.** Write `brief-<ticket>.md` in the scratchpad from the template below. Move the ticket(s) to In Progress. Dispatch with `Agent` (`model: "opus"`, `isolation: "worktree"`), prompt = "read the brief, read everything it names, do the work end to end, report per rule 12". Keep the agent id with the ticket; you will need it to resume.
7. **On report.** Read the report, not the PR. Then: post the disposition comment on each ticket (what shipped, premise corrections, review verdicts and fixes, disclosures, follow-ups filed or not); attach screenshots the agent could not upload (`gh release create psy-<N>-screenshots ... --prerelease`, then a PR comment with a light/dark table); file follow-ups that meet the bar (verify each claim in code first; match the parent's project; `confidence:*` label); run any measurement the agent's sandbox refused (prod/stage read-only counts) and post it on the PR.
8. **Tell the owner** in one message: what landed, what needs a decision, merge order for overlapping PRs, what is running.

## Conflict shepherding

Every merge invalidates siblings that touch the same files. When the owner reports a conflict:

```bash
git fetch -q origin main <branch>
git merge-tree --write-tree --name-only origin/main origin/<branch> | grep CONFLICT
git worktree list | grep <branch>          # the agent id is in the worktree dir name
gh pr list --state merged --limit 8 --json number,title,mergedAt   # which sibling caused it
```

Resume the ORIGINAL agent with `SendMessage` (it holds the context); never redispatch. The message names: the merged PR that caused it, the conflicting files, "read that PR's diff for those files first and resolve toward its shape", which semantics must survive from each side, the foreground gates to re-run, "one fresh Saboteur lens if the merge exceeded a mechanical combine", force-push with lease, one PR comment describing the resolution. If a second sibling merges mid-rebase, send a follow-up message; queued messages arrive at the agent's next tool round. Tell the owner the merge order that minimises re-rebases (the PR with the larger shared-file delta first).

## Stall recovery

An agent that backgrounds a long suite and waits is killed by the stream watchdog ("no progress for 600s"). Inspect before resuming: `git -C <worktree> status --short`, `git log --oneline origin/main..HEAD`, `ls ~/.claude/adversarial-review | grep <ticket>`, `pgrep -fl "go test|vitest"`. Then resume with exact remaining steps and the rule "nothing in the background; foreground with a 600000 ms timeout". Wording matters: say "check your suite's result", never "your suite has finished" (memory `pattern_agent_lifecycle_recovery`). Two stalls on the same agent means the remaining steps are short; enumerate them.

## Measurement the orchestrator owns

Agents run in sandboxes that refuse Railway/psql. Migration preconditions (a CHECK over existing rows), row counts a cleanup would touch, `max_connections`, and "how many prod rows would this gate refuse" are the orchestrator's to measure, read-only, from `backend/` with `railway variables --json -s Postgres -e <env> | jq -er .DATABASE_PUBLIC_URL`. Post the numbers on the PR before the owner merges. Never print secrets. If the permission classifier refuses, hand the owner the exact query instead of guessing.

## Deploy cadence

Deploy only on the owner's word. Run `psy-deploy-prod` in full every time: per-job CI classification on main's tip, FF check, prod `schema_migrations`, volume headroom, env-var release grep, then push the CI-verified SHA read from the run (never a SHA from memory). After: `/health/ready` 200, migrations applied, Vercel Ready. A frontend build gate (data-cache budget) can fail the Vercel half while Railway succeeds; say so plainly and fix forward.

## Ending the loop

Call diminishing returns explicitly when the remaining tickets are P4 polish whose panel cycles cost more than their defect risk, with the reason per ticket. The owner may overrule; then work them at P4 tier. When the count reaches zero: summarise what shipped for a reader who was not there, list open decisions, and name the next assessment the owner asked for.

## Brief template

```markdown
# Dispatch brief: PSY-N: <title>

Read first, in full: `agent-rules-template.md` in this same directory (every rule applies), the project `CLAUDE.md`, and memory files <list the pattern/feedback files that bear on this area> in `~/.claude/projects/<project>/memory/`. Then read the ticket WITH comments from the MAIN repo root: `linear issue view PSY-N --json | jq -r '.description, (.comments[]?.body // empty)'`. <Name any binding owner-decision comment by date.> <Name merged sibling PRs whose shape this builds on.>

## Dispatch parameters
- BRANCH: `PSY-N/<kebab>`; BASE: `origin/main`.
- TIER: <P2 full panel | P3 one round | P4 inline check>. Ticket is In Progress.
- <UI: light + dark screenshots via gh release; fallback: paths in the report.>

## What to build
<Numbered, concrete, with file paths verified on main. State defaults chosen for open questions.>

## Manual repro (required)
<Isolated stack; the exact matrix to record.>

## Gates
<The scoped commands; full suite only when an interface changed.>

Report back per rule 12.
```

The rules template the brief cites lives at `references/agent-rules-template.md` in this skill. Copy it to the scratchpad at loop start and fill in the session URL placeholder; it carries the per-agent ironclad rules (isolation, premise, no speculative design, gates, manual repro, review gauntlet, pass marker, rebase, PR body, commit hygiene, comments as invariants, report shape).

## Gotchas this loop paid for

- **Agents fabricate review attribution.** One credited findings to lenses that never returned. Rule 6 of the template forbids it; still read the report's review section against the PR body and re-run a lens yourself when a claimed panel looks thin.
- **An agent's report can misstate its own verdict.** One wrote "Saboteur passed with LOW findings" before the result existed; it was BLOCK. Treat "passed" as a claim until the PR comment shows the findings.
- **The PreToolUse hook keys on the session cwd**, so agents open PRs from their worktree root as a standalone command; a marker write batched with `gh pr create` never runs.
- **Scratchpad is shared across parallel sessions.** Ticket-scoped filenames only.
- **Merging with CI pending** happens; classify main's tip run per job afterwards and post the precondition measurement the merge skipped.
- **Screenshots**: the classifier may refuse `gh release create` inside an agent; the orchestrator can run it.
- **Full suites from several agents on one machine** poison each other (a 0 ms test taking 505 s). Scoped suites in agents; the full suite in CI or one at a time.
- **GOCACHE and worktrees fill the disk** over a multi-day loop; `go clean -cache` mid-run kills Docker. Sweep merged worktrees between waves (memory `pattern_worktree_disk_accumulation`).
