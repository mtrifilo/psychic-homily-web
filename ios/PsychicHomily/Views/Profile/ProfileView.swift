import SwiftUI

struct ProfileView: View {
    @Environment(AppState.self) private var appState

    var body: some View {
        Group {
            if let user = appState.currentUser {
                List {
                    Section {
                        VStack(spacing: 8) {
                            Image(systemName: "person.circle.fill")
                                .font(.system(size: 64))
                                .foregroundStyle(.phSecondary)

                            Text(user.displayName)
                                .font(.title2)
                                .fontWeight(.semibold)

                            if let email = user.email {
                                Text(email)
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .frame(maxWidth: .infinity)
                        .listRowBackground(Color.clear)
                        .padding(.vertical, 8)
                    }

                    Section("Account") {
                        if !user.emailVerified {
                            Label {
                                Text("Email not verified")
                                    .foregroundStyle(.orange)
                            } icon: {
                                Image(systemName: "exclamationmark.triangle")
                                    .foregroundStyle(.orange)
                            }
                        }

                        LabeledContent("Member since") {
                            Text(DateFormatting.shortDate(from: user.createdAt))
                                .foregroundStyle(.secondary)
                        }
                    }

                    Section {
                        Button(role: .destructive) {
                            appState.signOut()
                        } label: {
                            Label("Sign Out", systemImage: "rectangle.portrait.and.arrow.right")
                        }
                    }
                }
            } else {
                VStack(spacing: 24) {
                    Image(systemName: "person.circle")
                        .font(.system(size: 64))
                        .foregroundStyle(.secondary)

                    Text("Sign in to manage your account and save shows.")
                        .font(.body)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 32)

                    NavigationLink("Sign In") {
                        LoginView()
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.phPrimary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Profile")
    }
}

#Preview("Logged In") {
    let state = AppState()
    NavigationStack {
        ProfileView()
    }
    .environment(state)
    .preferredColorScheme(.dark)
}

#Preview("Logged Out") {
    NavigationStack {
        ProfileView()
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
