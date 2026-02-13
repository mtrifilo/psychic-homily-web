---
name: remember
description: Update dev-docs and memory files after completing implementation work. Use when finishing a task to record progress, update tracking docs, and surface next steps.
disable-model-invocation: true
argument-hint: "[new | summary of what was done]"
---

# Remember: Update Project Tracking

After implementation work is complete, update the relevant project tracking documents and surface next steps.

## Modes

### Default: `/remember` or `/remember <summary>`

Update an existing dev-doc based on context or summary.

### Create: `/remember new` or `/remember new <topic>`

Force-create a new dev-doc for tracking work that doesn't fit an existing doc. If a topic is provided (e.g., `/remember new oauth refactor`), use it as the basis. Otherwise, infer from conversation context.

## Instructions

### Step 1: Find or create the dev-doc

**If the argument starts with "new":**
- Skip to the "Creating a new dev-doc" section below.

**Otherwise:**
- Based on the argument (or recent conversation context if no argument), glob `dev-docs/*.md` and find the doc that tracks the work just completed. Common mappings:
  - Backend test work → `backend-test-coverage-plan.md`
  - E2E test work → `e2e-testing-playwright.md`
  - Discovery/scraper work → `discovery-fixes-checklist.md`
- If no existing doc is a clear match, **do not silently skip it**. Instead:
  1. List the existing dev-docs by name.
  2. Ask: "I couldn't find a dev-doc related to this work. Would you like me to: (A) update one of the docs above, or (B) create a new one?"
  3. Wait for the user's answer before proceeding.

### Step 2: Update the doc

- Mark completed items with `[x]`
- Update stats (coverage percentages, test counts, dates)
- Add new entries for work not previously tracked
- Strike through (`~~text~~`) items that are now done in summary tables

### Step 3: Update memory

Update `memory/MEMORY.md` (at `~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/MEMORY.md`):
- Update relevant stats (coverage numbers, test counts)
- Add any new patterns or gotchas discovered during the work
- Keep it concise — MEMORY.md is loaded into every conversation's system prompt

### Step 4: Summarize next steps

After updating, read back the doc and report:
- What was just recorded
- What unchecked items remain in the doc
- A recommendation for what to tackle next (prioritized by impact)

## Creating a New Dev-Doc

When creating a new doc (via `/remember new` or when the user accepts the offer):

1. **Determine the topic** from the argument after "new", or from conversation context, or ask the user.
2. **Choose a filename**: lowercase, hyphenated, descriptive. E.g., `oauth-refactor.md`, `frontend-performance.md`.
3. **Write the doc** to `dev-docs/<filename>` using this template:

```markdown
# <Title>

**Date:** <today's date>

## Context

<Brief description of the effort and goals>

## Completed

- [x] <Items completed in this session>

## Remaining

- [ ] <Known remaining items>

## Notes

<Any patterns, gotchas, or decisions worth recording>
```

4. Continue with Steps 3 and 4 (update memory, summarize next steps).

## Rules

- Do NOT fabricate stats — only record what was actually done and verified.
- If you can't determine what work was done (no argument and no recent context), ask the user.
- Keep MEMORY.md under 200 lines total.
- When in doubt about which doc to update, ask — don't guess.
