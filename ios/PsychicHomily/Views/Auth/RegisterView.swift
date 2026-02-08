import SwiftUI

struct RegisterView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.dismiss) private var dismiss
    @State private var viewModel = AuthViewModel()

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                VStack(spacing: 16) {
                    HStack(spacing: 12) {
                        TextField("First Name", text: $viewModel.firstName)
                            .textContentType(.givenName)
                            .padding()
                            .background(.phSurface)
                            .clipShape(RoundedRectangle(cornerRadius: 12))

                        TextField("Last Name", text: $viewModel.lastName)
                            .textContentType(.familyName)
                            .padding()
                            .background(.phSurface)
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                    }

                    TextField("Email", text: $viewModel.email)
                        .textContentType(.emailAddress)
                        .keyboardType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .padding()
                        .background(.phSurface)
                        .clipShape(RoundedRectangle(cornerRadius: 12))

                    SecureField("Password", text: $viewModel.password)
                        .textContentType(.newPassword)
                        .padding()
                        .background(.phSurface)
                        .clipShape(RoundedRectangle(cornerRadius: 12))

                    if let error = viewModel.errorMessage {
                        Text(error)
                            .font(.caption)
                            .foregroundStyle(.red)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }

                    Button {
                        Task { await viewModel.register(appState: appState) }
                    } label: {
                        if viewModel.isLoading {
                            ProgressView()
                                .tint(.white)
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 4)
                        } else {
                            Text("Create Account")
                                .fontWeight(.semibold)
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 4)
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.phPrimary)
                    .disabled(viewModel.isLoading)
                }
                .padding(.horizontal)

                if viewModel.registrationSuccess {
                    VStack(spacing: 8) {
                        Image(systemName: "envelope.badge")
                            .font(.title)
                            .foregroundStyle(.phAccent)

                        Text("Check your email to verify your account")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)

                        Text("You'll need to verify before submitting shows.")
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                    }
                    .padding()
                    .background(.phSurface)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .padding(.horizontal)
                }
            }
            .padding(.top, 16)
            .padding(.bottom, 32)
        }
        .navigationTitle("Create Account")
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: appState.isAuthenticated) {
            if appState.isAuthenticated {
                dismiss()
            }
        }
    }
}

#Preview {
    NavigationStack {
        RegisterView()
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
