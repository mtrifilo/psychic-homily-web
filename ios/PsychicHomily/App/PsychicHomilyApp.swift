import SwiftUI

@main
struct PsychicHomilyApp: App {
    @State private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(appState)
                .onOpenURL { url in
                    handleURL(url)
                }
        }
    }

    private func handleURL(_ url: URL) {
        guard url.scheme == "psychichomily" else { return }

        if url.host == "submit", url.queryParameters["source"] == "share" {
            appState.selectedTab = .submit
            appState.pendingShareImage = true
        }
    }
}

struct ContentView: View {
    @Environment(AppState.self) private var appState

    var body: some View {
        @Bindable var appState = appState

        TabView(selection: $appState.selectedTab) {
            Tab("Shows", systemImage: "music.note.list", value: AppTab.shows) {
                NavigationStack {
                    ShowListView()
                }
            }

            Tab("My List", systemImage: "bookmark.fill", value: AppTab.myList) {
                NavigationStack {
                    if appState.isAuthenticated {
                        SavedShowsView()
                    } else {
                        AuthPromptView(message: "Sign in to save shows and build your personal list.")
                    }
                }
            }

            Tab("Submit", systemImage: "camera.fill", value: AppTab.submit) {
                NavigationStack {
                    if appState.isAuthenticated {
                        SubmitShowView()
                    } else {
                        AuthPromptView(message: "Sign in to submit shows from screenshots.")
                    }
                }
            }

            Tab("Profile", systemImage: "person.circle", value: AppTab.profile) {
                NavigationStack {
                    ProfileView()
                }
            }
        }
        .task {
            await appState.restoreSession()
        }
    }
}

// MARK: - Auth Prompt (inline, not modal)

struct AuthPromptView: View {
    let message: String
    @Environment(AppState.self) private var appState

    var body: some View {
        VStack(spacing: 24) {
            Image(systemName: "person.crop.circle.badge.plus")
                .font(.system(size: 64))
                .foregroundStyle(.secondary)

            Text(message)
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)

            NavigationLink("Sign In") {
                LoginView()
            }
            .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

// MARK: - URL Query Parameter Extension

private extension URL {
    var queryParameters: [String: String] {
        guard let components = URLComponents(url: self, resolvingAgainstBaseURL: false),
              let items = components.queryItems else {
            return [:]
        }
        return Dictionary(uniqueKeysWithValues: items.compactMap { item in
            item.value.map { (item.name, $0) }
        })
    }
}
