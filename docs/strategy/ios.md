# iOS Track

> **STATUS: CODE COMPLETE, NOT SHIPPED.** All Swift code written (41 files), building in simulator. Blocked on Apple Developer Program enrollment.
>
> Native iOS companion app (Swift 6, SwiftUI, iOS 18+). Retained as reference.

## Current Status

All Swift code written (39 files), all backend changes complete, building and running in simulator (Xcode 26.2). Phases 1-7 complete. Phases 8-9 (polish, testing, App Store submission) blocked on Apple Developer Program enrollment.

## Next Priorities

1. **Apple Developer enrollment** ($99/year) — unblocks everything below
2. **Re-enable capabilities** — Sign in with Apple, App Groups, Keychain Sharing
3. **Polish & error states** — error views, loading skeletons, haptic feedback, retry logic
4. **Device testing** — physical device testing across screen sizes
5. **TestFlight → App Store submission**

## Roadmap

### Now: Ship v1 (Phase 1)

- [ ] Apple Developer enrollment
- [ ] Re-enable capabilities (commented out in `project.yml`)
- [ ] Error states and retry for all screens
- [ ] Loading skeletons / spinners
- [ ] Haptic feedback on save/unsave
- [ ] Physical device testing (iPhone SE, 15, 16 Pro Max)
- [ ] VoiceOver accessibility pass
- [ ] Unit + UI tests
- [ ] App Store submission

### Post-Launch

- [ ] Push notifications (show reminders, new shows at saved venues)
- [ ] Offline mode (cached show list)
- [ ] Widget (upcoming saved shows)

### Knowledge Graph Features (aligns with web Phase 2+)

- [ ] Label browsing
- [ ] Genre/tag filtering
- [ ] Scene pages
- [ ] Feature parity with web as new entities are added

## Architecture

- **Language:** Swift 6 (strict concurrency)
- **UI:** SwiftUI (iOS 18+, `Tab` API)
- **Pattern:** MVVM with `@Observable`, `@MainActor` on all ViewModels
- **Networking:** URLSession async/await, actor-based `APIClient`
- **Auth:** Bearer token (Keychain), not cookies
- **Dependencies:** SPM only, zero third-party packages

## Key Files

| Area | Files |
|------|-------|
| Xcode project | `ios/project.yml` (XcodeGen) |
| App entry | `ios/PsychicHomily/App/PsychicHomilyApp.swift` |
| Auth state | `ios/PsychicHomily/App/AppState.swift` |
| API client | `ios/PsychicHomily/Networking/APIClient.swift` |
| ViewModels | `ios/PsychicHomily/ViewModels/*.swift` |
| Backend auth | `backend/internal/services/apple_auth.go` |

## Open Decisions

- **Distribution:** TestFlight beta vs. direct App Store submission
- **Long-term mobile strategy:** Continue native iOS or evaluate PWA as the Music Scene Index grows
- **V2 scope:** Push notifications vs. widget vs. offline — which first?
