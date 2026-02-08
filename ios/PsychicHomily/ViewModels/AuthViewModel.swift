import SwiftUI
import AuthenticationServices

@Observable
final class AuthViewModel {
    var email = ""
    var password = ""
    var firstName = ""
    var lastName = ""
    var isLoading = false
    var errorMessage: String?
    var registrationSuccess = false

    func login(appState: AppState) async {
        guard !email.isEmpty, !password.isEmpty else {
            errorMessage = "Email and password are required"
            return
        }

        isLoading = true
        errorMessage = nil

        do {
            let response: AuthResponse = try await APIClient.shared.request(
                .login(email: email, password: password)
            )

            if response.success, let token = response.token {
                KeychainManager.saveToken(token)
                appState.currentUser = response.user
            } else {
                errorMessage = response.message ?? "Login failed"
            }
        } catch let error as APIError {
            errorMessage = error.localizedDescription
        } catch {
            errorMessage = "An unexpected error occurred"
        }

        isLoading = false
    }

    func register(appState: AppState) async {
        guard !email.isEmpty, !password.isEmpty else {
            errorMessage = "Email and password are required"
            return
        }

        isLoading = true
        errorMessage = nil

        do {
            let response: AuthResponse = try await APIClient.shared.request(
                .register(
                    email: email,
                    password: password,
                    firstName: firstName.isEmpty ? nil : firstName,
                    lastName: lastName.isEmpty ? nil : lastName
                )
            )

            if response.success, let token = response.token {
                KeychainManager.saveToken(token)
                appState.currentUser = response.user
                registrationSuccess = true
            } else {
                errorMessage = response.message ?? "Registration failed"
            }
        } catch let error as APIError {
            errorMessage = error.localizedDescription
        } catch {
            errorMessage = "An unexpected error occurred"
        }

        isLoading = false
    }

    func handleAppleSignIn(result: Result<ASAuthorization, Error>, appState: AppState) async {
        switch result {
        case .success(let authorization):
            guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
                  let identityTokenData = credential.identityToken,
                  let identityToken = String(data: identityTokenData, encoding: .utf8) else {
                errorMessage = "Failed to get Apple identity token"
                return
            }

            let firstName = credential.fullName?.givenName
            let lastName = credential.fullName?.familyName

            isLoading = true
            errorMessage = nil

            do {
                let response: AuthResponse = try await APIClient.shared.request(
                    .appleCallback(identityToken: identityToken, firstName: firstName, lastName: lastName)
                )

                if response.success, let token = response.token {
                    KeychainManager.saveToken(token)
                    appState.currentUser = response.user
                } else {
                    errorMessage = response.message ?? "Apple sign-in failed"
                }
            } catch let error as APIError {
                errorMessage = error.localizedDescription
            } catch {
                errorMessage = "An unexpected error occurred"
            }

            isLoading = false

        case .failure(let error):
            if (error as NSError).code != ASAuthorizationError.canceled.rawValue {
                errorMessage = "Apple sign-in failed: \(error.localizedDescription)"
            }
        }
    }
}
