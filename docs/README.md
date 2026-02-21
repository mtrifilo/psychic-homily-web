# Docs

Documentation hub for humans and AI agents. Organized for progressive disclosure — load only what you need.

## Start Here

- **AI agents**: Read [`llm-context.md`](llm-context.md) first — it has task routing and quick conventions
- **Humans**: Read [`status.md`](status.md) for current focus and active work
- **Architecture**: `CLAUDE.md` at the project root is the primary reference (loaded automatically for AI agents)

## Suggested Reading Order

1. `llm-context.md` — orient, find the right doc for your task
2. `status.md` — understand current focus and recent completions
3. Task-specific doc (see routing table in llm-context.md)

## Directory Structure

```
docs/
  README.md                ← You are here
  llm-context.md           ← Fast context loader + task-to-doc routing
  status.md                ← Current checkpoint, active work, next tasks

  architecture/            ← Stable reference (rarely changes)
    admin-cli.md           - CLI export/import tool
    discord-notifications-setup.md
    privacy-policy-review.md

  plans/
    active/                ← In-progress work
      2026-launch-readiness-checklist.md  - Pre-launch security/compliance
      backend-refactoring-checklist.md    - 4-phase DI + testing refactor
      backend-test-coverage-plan.md       - Coverage targets by tier
      e2e-testing-playwright.md           - 69 E2E tests, CI integration
      observability-posthog-sentry.md     - Analytics + error tracking
      ios-app-v1.md                       - iOS companion app
    completed/             ← Archived — skip on read

  learnings/               ← Patterns, gotchas, debugging insights
    testing.md             - Backend + E2E test patterns, coverage stats
    production.md          - Security fixes, performance, Chrome rendering

  ideas/                   ← Future features, roadmap, monetization
    future-product-roadmap.md       - 7 features in 4 phases
    admin-feature-ideas.md          - Internal tooling improvements
    monetization-strategies.md      - Revenue models
    shared-component-refactoring.md - UI/UX improvements
```

## Doc Maintenance Rules

1. **Update `status.md`** after completing significant work or discovering blockers
2. **Update `llm-context.md`** when migration numbers change, new plans are added, or conventions shift
3. **Move completed plans** from `plans/active/` to `plans/completed/`
4. **New plans** should follow the template in `llm-context.md`
5. **Learnings** go in `learnings/` — add new files by topic if testing.md or production.md grow too large
6. **Don't duplicate CLAUDE.md** — reference it, don't copy architecture details into docs
