# Mutation Testing Evaluation (PSY-252)

**Date:** 2026-03-30
**Context:** After removing ~200+ low-value tests and strengthening assertions, we evaluated mutation testing as an objective measure of test effectiveness.

## What Is Mutation Testing?

Mutation testing modifies source code (e.g., flipping `>` to `>=`, removing statements, changing string literals) and checks whether tests catch the change. If a test suite still passes after a mutation, that's a "surviving mutant" -- evidence of a gap in test coverage quality, not just quantity.

## Tools Evaluated

### Backend (Go)

| Tool | Version | Last Release | Status | Verdict |
|------|---------|-------------|--------|---------|
| [gremlins](https://github.com/go-gremlins/gremlins) | v0.6.0 | 2025-12-06 | Actively maintained | **Recommended** |
| [go-mutesting](https://github.com/zimmski/go-mutesting) | v0.0.0 (2021) | 2021-06-10 | Abandoned | **Not viable** |

**gremlins** (go-gremlins/gremlins):
- Actively maintained, v0.6.0 released Dec 2025 with new filtering capabilities
- Supports 6 mutator types: conditionals negation, conditionals boundary, arithmetic base, increment/decrement, invert negatives, and more
- JSON output for CI integration (`-o output.json`)
- File exclusion via `--exclude-files` regex
- Install: `go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`

**go-mutesting** (zimmski/go-mutesting):
- Last updated 2021, depends on `golang.org/x/tools` from 2019
- **Crashes with nil pointer dereference** on our codebase due to incompatible Go toolchain
- Issue #100 ("Is go-mutesting dead?") opened 2022, no response
- **Do not use** -- fundamentally broken with modern Go (1.23+)

### Frontend (TypeScript/Vitest)

| Tool | Version | Status | Verdict |
|------|---------|--------|---------|
| [Stryker Mutator](https://stryker-mutator.io/) + [Vitest Runner](https://stryker-mutator.io/docs/stryker-js/vitest-runner/) | v9.6.0 | Actively maintained | **Recommended** |

**Stryker Mutator:**
- Official Vitest runner plugin (`@stryker-mutator/vitest-runner`)
- 161 mutant types including: conditional expressions, string literals, regex, optional chaining, block statements, arithmetic, etc.
- Built-in coverage analysis for smart test selection per mutant
- HTML + clear-text reporters
- Install: `bun add -d @stryker-mutator/core @stryker-mutator/vitest-runner`
- Optional TypeScript checker: `@stryker-mutator/typescript-checker` (requires clean TS compilation -- we have some pre-existing test type errors that block this)

## How to Run

### Backend (gremlins)

```bash
# Install (one-time)
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest

# Run on a specific package (pure unit test packages work best)
cd backend
gremlins unleash ./internal/utils/
gremlins unleash ./internal/models/

# With JSON output
gremlins unleash -o results.json ./internal/utils/

# Exclude files
gremlins unleash -E "artist\.go$" -E "festival\.go$" ./internal/services/catalog/

# Increase timeout for slower test suites
gremlins unleash --timeout-coefficient 3 ./internal/models/
```

### Frontend (Stryker)

```bash
# Install (one-time)
cd frontend
bun add -d @stryker-mutator/core @stryker-mutator/vitest-runner

# Create stryker.config.json
cat > stryker.config.json << 'EOF'
{
  "testRunner": "vitest",
  "vitest": {},
  "checkers": [],
  "mutate": ["lib/utils/timeUtils.ts"],
  "reporters": ["clear-text", "html"],
  "htmlReporter": { "fileName": "reports/mutation/index.html" },
  "concurrency": 4,
  "timeoutMS": 30000
}
EOF

# Run
npx stryker run

# Target different files
# Edit "mutate" array in stryker.config.json
```

## Sample Results

### Backend: `internal/utils/` (slug.go + timezone.go)

```
Killed: 4, Lived: 1, Not covered: 0
Test efficacy: 80.00%
Mutator coverage: 100.00%
Time: 1.7 seconds
```

**Surviving mutant:** `CONDITIONALS_BOUNDARY` at `slug.go:74:16` -- changed `i <= 100` to `i < 100` in `GenerateUniqueSlug`. Tests don't exercise the exact boundary of 100 slug collisions (reasonable -- this is a fallback path).

### Backend: `internal/models/` (5 files with tests)

```
Killed: 52, Lived: 5, Not covered: 25
Test efficacy: 91.23%
Mutator coverage: 69.51%
Time: 11.6 seconds
```

**Surviving mutants (5):**
- `artist_relationship.go:50:79` -- arithmetic boundary in WilsonScore formula (subtle float precision)
- `artist_relationship.go:73:7` -- boundary condition in CanonicalOrder
- `user_webauthn.go:136:16, 136:57, 140:16` -- boundary conditions in WebAuthn validation

These are genuinely informative: they highlight boundary conditions that tests don't pin down precisely.

### Backend: `internal/services/catalog/` (label.go -- integration tests)

```
Killed: 0, Lived: 0, Not covered: 24
Timed out: 57, Not viable: 0
Test efficacy: 0.00%
Time: 8.5 seconds
```

**Key finding:** Integration tests using testcontainers (PostgreSQL) cause nearly all mutants to **time out**. Gremlins' default timeout is based on initial test run duration, but testcontainer startup is variable. The `--timeout-coefficient` flag helps but doesn't fully solve this. **Mutation testing works best on packages with pure unit tests.**

### Frontend: `lib/utils/timeUtils.ts`

```
Total mutation score: 61.49%
Covered mutation score: 89.19%
Killed: 99, Survived: 12, No coverage: 50
Time: 42 seconds
```

**Surviving mutants (12 notable examples):**

| Mutation | Location | What It Tells Us |
|----------|----------|-----------------|
| `if (!timezone)` -> `if (false)` | Line 45 | Tests don't verify the no-timezone fallback path is taken (they still pass because timezone path produces same result) |
| Regex `$` anchor removed (`/\.\d{3}Z$/` -> `/\.\d{3}Z/`) | Lines 48, 84 | Removing the end-of-string anchor makes no difference because `.000Z` only appears once in ISO strings |
| Optional chaining removed (`.value` vs `?.value`) | Lines 67, 197 | Intl.DateTimeFormat always returns all requested parts, so `?.` is defensive but never actually needed |
| `'24'` midnight check disabled | Lines 72, 200 | Tests don't include a midnight case that would trigger `hour === 24` |
| String literal `'minute'` -> `""` | Lines 73, 203 | Tests don't verify minute precision (e.g., 7:30 vs 7:00) |
| `'AZ': 'America/Phoenix'` -> `""` | Line 10 | Static map entry -- no test calls `getTimezoneForState('AZ')` and checks the exact timezone string |

**50 "no coverage" mutants** are mostly in `formatDateWithYearInTimezone`, `formatDateInTimezone`, `formatTimeInTimezone`, and `formatInTimezone` -- these wrapper functions have no dedicated tests.

## Key Findings

### 1. Mutation Testing Surfaces Real Test Gaps
The surviving mutants are genuinely informative. They found:
- Missing boundary tests (slug collision limit, WebAuthn validation thresholds)
- Missing edge cases (midnight timezone conversion, minute precision)
- Untested defensive code (optional chaining that's never needed)
- Completely untested functions (format helpers in timeUtils.ts)

### 2. Integration Tests Don't Work Well with Mutation Testing
Go services using testcontainers have unpredictable startup times that cause timeouts. Mutation testing is most effective on:
- Pure utility functions (slug generation, timezone conversion)
- Model methods (WilsonScore, validation)
- Frontend utilities and hooks
- Any code with fast, deterministic unit tests

### 3. Performance Is Acceptable for Targeted Runs
| Target | Mutants | Time | Per-Mutant |
|--------|---------|------|------------|
| Go `internal/utils/` (2 files) | 5 | 1.7s | 0.34s |
| Go `internal/models/` (5 files) | 82 | 11.6s | 0.14s |
| TS `timeUtils.ts` (1 file) | 161 | 42s | 0.26s |

Running against an entire large package would be slow (estimated 5-15 minutes for all catalog services, 10-30 minutes for all frontend utils).

### 4. Stryker Has Smart Test Selection
Stryker only runs tests that cover each mutant, not the full suite. It ran an average of 10.93 tests per mutant (out of 528 total). This makes it much faster than naive mutation testing.

## Recommendation: **Adopt for Targeted Use**

### Do
- **Run after writing new tests** to validate they actually catch mutations in the code under test
- **Run on pure utility/model packages** where tests are fast and deterministic
- **Use as a periodic quality check** (monthly or after major test refactors) on key packages
- **Target high-value code**: scoring algorithms, slug generation, date/time handling, validation logic

### Don't
- **Don't add to CI per-PR** -- too slow (42s for one file, would be 10+ minutes for meaningful coverage)
- **Don't run on integration test packages** -- testcontainer timeouts make results unreliable
- **Don't chase 100% mutation score** -- some surviving mutants are acceptable (e.g., defensive optional chaining, exact boundary of a 100-iteration fallback loop)

### Estimated CI Cost (If Added Later)

| Approach | Scope | Time | When |
|----------|-------|------|------|
| Targeted nightly | 5-10 key utility files | 2-5 min | Nightly cron |
| Full nightly | All unit-testable packages | 15-30 min | Nightly cron |
| Per-PR (targeted) | Changed files only | 1-3 min | PR check |
| Per-PR (full) | All packages | 15-30 min | Not recommended |

**If we adopt for CI later:** run against changed files only on PRs (Stryker supports `--since` for git-aware mutation; gremlins would need scripting). Nightly runs on key packages for trend tracking.

### Setup Cost
- **Backend:** Zero config needed. `go install` + `gremlins unleash ./path/` works immediately.
- **Frontend:** Needs `stryker.config.json` and dev dependencies. TypeScript checker requires clean test compilation (we'd need to fix ~10 test type errors first). Works fine without the TS checker.

### Next Steps (If Adopting)
1. Fix the ~10 test files with TypeScript errors to enable Stryker's TS checker
2. Add tests for the untested format functions in `timeUtils.ts` (50 "no coverage" mutants)
3. Add a midnight test case for `combineDateTimeToUTC` (surviving mutant at line 72)
4. Consider a `scripts/mutate.sh` convenience script that targets common packages
5. Track mutation scores over time to measure test quality trends
