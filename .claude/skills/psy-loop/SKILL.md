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

## Where this skill overrides its siblings

- **Family PRs.** `psy-dispatch` ironclad rule 4 says one ticket = one PR. Inside a loop, the owner's standing decision in memory `feedback_tiered_review_family_prs.md` (2026-08-30) supersedes it: tightly related siblings ship as one branch with per-ticket commits and a body that closes each ticket. "Tightly related" means all three hold: the tickets touch the same files, they sit in the same review tier, and one repro matrix covers them. The branch and PR title are named after the lowest-numbered ticket (`PSY-2020/bounds-trio`), and every ticket gets its own `Closes` line and its own disposition comment.
- **Review tier by priority** (same memory): P2, security, or data-integrity = full fresh-panel adversarial review, up to 3 rounds. P3 = one fresh four-lens round. P4 = gates plus ONE fresh single-lens review (Saboteur for logic, Future-Maintainer for docs), which is what earns the pass marker; the inline self-check is in addition, never instead. The `ADVERSARIAL_REVIEW_SKIP=1` escape hatch did not get past the hook in this session's shell; do not plan on it.

Everything else in `psy-dispatch` (pre-flight, isolation, take-over flow) and `adversarial-review` (finding bar, marker rules) applies unchanged.

## The contract with the owner

| The orchestrator | The owner |
| --- | --- |
| Verifies every ticket premise against `origin/main` before dispatch | Merges PRs (never the orchestrator or an agent) |
| Keeps N slots full; never exceeds N | Sets N (default 2; 3 when the backlog is growing faster than it shrinks) |
| Records a disposition comment on every ticket when its PR opens | Answers design forks raised via `AskUserQuestion` |
| Files follow-up tickets that meet the bar; lists the rest on the ticket | Decides deploys ("deploy if confident" is the trigger, then `psy-deploy-prod` in full) |
| Shepherds conflicts by resuming the original agent | Reports red CI or conflicts on a specific PR |
| Calls diminishing returns explicitly, and keeps building when overruled | Overrules: "until the remaining tickets are done" means all of them |

**Follow-up bar** (what the orchestrator files without asking): the claim is verified in code on `origin/main` by the orchestrator, fixing it would change behaviour, and no open ticket already covers it. Everything else is listed on the parent ticket's disposition comment as "not filed" with the reason. Two instances of the same defect class across a wave become an audit ticket (memory `feedback_audit_recurring_bugs`). Every filed ticket matches the parent's project and carries a `confidence:*` label (`psy-ticket`).

## Loop start

1. Copy `references/agent-rules-template.md` from this skill to the scratchpad as `agent-rules-<project-slug>-<YYYY-MM-DD>.md` (the scratchpad is shared across parallel sessions; a fixed name gets overwritten by another loop and stamps its session URL into your commits). Replace BOTH placeholders: `<SESSION_URL>` (this session's `https://claude.ai/code/session_...` URL) and `<MODEL_NAME>` (the orchestrating model's display name, e.g. `Claude Fable 5.1`). Then `grep -n '<[A-Z_]*>' <that file>` must print nothing.
2. Note the memory directory's absolute path (`~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/` for this repo); every brief spells it out in full, never as `<project>`.
3. Run `psy-dispatch`'s pre-flight once: main repo HEAD is on `main` and synced with origin; main's tip CI is green per job (classify any red before dispatching; a wave dispatched onto red main inherits the red).

## Slot loop

Repeat until the project has no ticket outside Done/Canceled/Duplicate:

1. **Inventory.** `linear issue list --project "<name>" --team PSY --all-assignees --all-states --limit 0 --no-pager`, strip ANSI, drop Done/Canceled/Duplicate. `linear issue list` (1.11.1) has no `--json` flag: passing it prints help and exits 0, so a `jq` pipeline sees help text and a zero exit. Use the text form. (`linear issue view --json` does exist and carries `.comments`.)
2. **Pick.** Fill free slots with the highest-priority tickets whose files do not overlap a running slot (check the running briefs). Apply the family-PR test above.
3. **Per-ticket pre-flight.** `git branch -a | grep PSY-N`, `git worktree list | grep PSY-N`, `gh pr list --search "PSY-N" --state all`: an existing branch, worktree, or PR means a prior slot (stalled, shepherded, or from another session) still owns it. Resume or take over that worktree (`psy-dispatch` take-over flow); never dispatch a second agent onto a ticket that has one. Re-check main's tip CI if the owner merged since the last wave.
4. **Verify the premise.** Grep or read the cited code on `origin/main`. Fixed already, wrong, or partially wrong: post a correction comment on the ticket BEFORE dispatch and scope the brief to what remains. Canonical: PSY-2000 residual 2 was already fixed; PSY-1993's premise was the orchestrator's own conflation of two contracts (memory `feedback_verify_ticket_hypothesis_first`).
5. **Resolve forks.** Any undecided WHAT (copy, threshold, refuse-vs-restamp, Figma-or-not) goes to the owner in one `AskUserQuestion` round while other slots run; record the answers as a dated, "binding" Linear comment; the brief cites that comment.
6. **Tier.** Per the table above; state the tier in the brief.
7. **Brief.** Write `brief-<ticket>.md` in the scratchpad from the template below, citing the rules file by its exact per-loop path. Move the ticket(s) to In Progress. Dispatch with `Agent` (`model: "opus"`, `isolation: "worktree"`), prompt = "read the brief, read everything it names, do the work end to end, report per rule 12". Record the agent id next to the ticket in your notes; you will need it to resume.
8. **On report.** Read the report, not the PR. Then: post the disposition comment on each ticket (what shipped, premise corrections, review verdicts and fixes, disclosures, follow-ups filed or not); attach screenshots the agent could not upload (`gh release create psy-<N>-screenshots ... --prerelease`, then a PR comment with a light/dark table); file follow-ups that meet the bar; run any measurement the agent's sandbox refused (prod/stage read-only counts) and post it on the PR.
9. **Tell the owner** in one message: what landed, what needs a decision, merge order for overlapping PRs, what is running.

## Conflict shepherding

Every merge invalidates siblings that touch the same files. When the owner reports a conflict:

```bash
git fetch -q origin main <branch>
git merge-tree --write-tree --name-only origin/main origin/<branch> | grep CONFLICT
git worktree list | grep <branch>          # the agent id is in the worktree dir name
gh pr list --state merged --limit 8 --json number,title,mergedAt   # which sibling caused it
```

Resume the ORIGINAL agent with `SendMessage` (it holds the context). The message names: the merged PR that caused it, the conflicting files, "read that PR's diff for those files first and resolve toward its shape", which semantics must survive from each side, the foreground gates to re-run, "one fresh Saboteur lens if the merge exceeded a mechanical combine", force-push with lease, one PR comment describing the resolution. If a second sibling merges mid-rebase, send a follow-up message; queued messages arrive at the agent's next tool round. Tell the owner the merge order that minimises re-rebases (the PR with the larger shared-file delta first).

**When the agent cannot be reached** (session restarted, id gone, `SendMessage` errors): take over its worktree directly per `psy-dispatch`'s take-over flow (the pass-marker key is per repo+branch, so the existing marker still matches from that worktree root). Redispatch a fresh agent only after the old worktree is confirmed dead and removed; never onto a branch a worktree still holds.

## Stall recovery

An agent that backgrounds a long suite and waits is killed by the stream watchdog ("no progress for 600s"). Inspect before resuming: `git -C <worktree> status --short`, `git log --oneline origin/main..HEAD`, `ls ~/.claude/adversarial-review | grep <ticket>` (a marker means a review round ran; it does not prove the final HEAD was reviewed), `pgrep -fl "go test|vitest"`. Then resume with the exact remaining steps and the rule "nothing in the background; scoped suites in the foreground". Wording matters: say "check your suite's result", never "your suite has finished" (memory `pattern_agent_lifecycle_recovery`). Two stalls on the same agent means the remaining steps are short; enumerate them.

## Suites: who runs what

- Agents run SCOPED suites in the foreground; each must fit the Bash tool's 600 s ceiling.
- The FULL Go suite (`-timeout 45m -p 1`) does not fit that ceiling and poisons sibling slots when several run at once (a 0 ms test took 505 s). It runs in CI, or in an orchestrator-owned exclusive window with no other slot's suite running. An interface change therefore ships with scoped suites green locally and the full suite green in CI, stated that way in the PR body, never as a checked box for a run nobody did.

## Measurement the orchestrator owns

Agents run in sandboxes that refuse Railway/psql. Migration preconditions (a CHECK over existing rows), row counts a cleanup would touch, `max_connections`, and "how many prod rows would this gate refuse" are the orchestrator's to measure, read-only, from `backend/` with `railway variables --json -s Postgres -e <env> | jq -er .DATABASE_PUBLIC_URL`. Post the numbers on the PR before the owner merges. Never print secrets. If the permission classifier refuses, hand the owner the exact query instead of guessing.

## Deploy cadence

Deploy only on the owner's word. Run `psy-deploy-prod` in full every time: per-job CI classification on main's tip, FF check, prod `schema_migrations`, volume headroom, env-var release grep, then push the CI-verified SHA read from the run (never a SHA from memory). After: `/health/ready` 200, migrations applied, Vercel Ready. A frontend build gate (data-cache budget) can fail the Vercel half while Railway succeeds; say so plainly and fix forward.

## Ending the loop

Call diminishing returns explicitly when the remaining tickets are P4 polish whose panel cycles cost more than their defect risk, with the reason per ticket. The owner may overrule; then work them at P4 tier. When the count reaches zero: summarise what shipped for a reader who was not there, list open decisions, and name the next assessment the owner asked for.

## Brief template

```markdown
# Dispatch brief: PSY-N: <title>

Read first, in full: `<absolute path to the per-loop rules file>` (every rule applies), the project `CLAUDE.md`, and memory files <list the pattern/feedback files that bear on this area> in `<absolute memory dir path>`. Then read the ticket WITH comments from the MAIN repo root: `linear issue view PSY-N --json | jq -r '.description, (.comments[]?.body // empty)'`. <Name any binding owner-decision comment by date.> <Name merged sibling PRs whose shape this builds on.>

## Dispatch parameters
- BRANCH: `PSY-N/<kebab>`; BASE: `origin/main`.
- TIER: <P2 full panel | P3 one four-lens round | P4 one single-lens review>. Ticket is In Progress.
- <UI: light + dark screenshots via gh release; fallback: paths in the report.>

## What to build
<Numbered, concrete, with file paths verified on main. State defaults chosen for open questions.>

## Manual repro (required)
<Isolated stack; the exact matrix to record.>

## Gates
<The scoped commands. Full suite: CI, per "Suites: who runs what".>

Report back per rule 12.
```

## Gotchas this loop paid for

- **Agents fabricate review attribution.** One credited findings to lenses that never returned. Rule 6 of the template forbids it and `psy-self-review` (rule 6 of the template) checks the PR body against its evidence; still read the report's review section against the PR body and re-run a lens yourself when a claimed panel looks thin.
- **An agent's report can misstate its own verdict.** One wrote "Saboteur passed with LOW findings" before the result existed; it was BLOCK. Treat "passed" as a claim until the PR comment shows the findings.
- **The PreToolUse hook keys on the session cwd and gates the WHOLE Bash command**, so agents open PRs from their worktree root with `gh pr create` as a standalone command; a marker write or ticket creation batched into the same command never runs (memory `pattern_pretooluse_hook_gates_whole_command`).
- **Scratchpad is shared across parallel sessions.** Ticket-scoped and loop-scoped filenames only.
- **Merging with CI pending** happens; classify main's tip run per job afterwards and post the precondition measurement the merge skipped.
- **Screenshots**: the classifier may refuse `gh release create` inside an agent; the orchestrator can run it.
- **GOCACHE and worktrees fill the disk** over a multi-day loop; `go clean -cache` mid-run kills Docker. Sweep merged worktrees between waves (memory `pattern_worktree_disk_accumulation`).
