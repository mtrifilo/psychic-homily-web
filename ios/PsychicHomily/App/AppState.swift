import SwiftUI

enum AppTab: Hashable {
    case shows
    case myList
    case submit
    case profile
}

@Observable
final class AppState {
    var selectedTab: AppTab = .shows
    var currentUser: User?
    var pendingShareImage: Bool = false

    var isAuthenticated: Bool {
        currentUser != nil
    }

    func signOut() {
        currentUser = nil
        KeychainManager.deleteToken()
    }

    /// Attempt to restore session from stored token
    func restoreSession() async {
        guard KeychainManager.getToken() != nil else { return }

        do {
            let response: ProfileResponse = try await APIClient.shared.request(.getProfile)
            if response.success {
                currentUser = response.user
            } else {
                // Token is invalid, clear it
                signOut()
            }
        } catch {
            // Network error — keep token but don't set user
            // User can retry when connection is available
        }
    }
}
