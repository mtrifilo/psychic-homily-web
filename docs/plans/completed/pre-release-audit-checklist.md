# Pre-Release Audit Checklist

## Phase 1: Security & Authorization
- [x] 1.1 Fix UpdateShowHandler IDOR vulnerability (ownership check)
- [x] 1.2 Validate production secrets at startup

## Phase 2: Input Validation
- [x] 2.1 Backend email format validation on registration
- [x] 2.2 Max-length constraints on text fields (title, description, age_requirement, artist name, venue name, rejection reason)
- [x] 2.3 Price range validation (0-10000)
- [x] 2.4 Offset validation in admin paginated endpoints

## Phase 3: Backend Performance & Robustness
- [x] 3.1 Fix N+1 query in buildShowResponse
- [x] 3.2 Add error handling in buildShowResponse for showArtists query
- [x] 3.3 Add index on duplicate_of_show_id
- [x] 3.4 Fix race condition in duplicate detection with advisory lock

## Phase 4: Frontend UX
- [x] 4.1 Replace string-based 404 detection with status code
- [x] 4.2 Add retry buttons on error states
- [x] 4.3 Add pagination controls (Load More)
- [x] 4.4 Make duplicate show link clickable in admin
