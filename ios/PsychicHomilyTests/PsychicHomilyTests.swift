import Testing
@testable import PsychicHomily

@Suite("PsychicHomily Tests")
struct PsychicHomilyTests {
    @Test func appStateDefaultsToShowsTab() {
        let appState = AppState()
        #expect(appState.selectedTab == .shows)
        #expect(appState.isAuthenticated == false)
    }
}
