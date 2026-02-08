import Foundation

struct User: Codable, Identifiable, Sendable {
    let id: Int
    let email: String?
    let username: String?
    let firstName: String?
    let lastName: String?
    let avatarURL: String?
    let bio: String?
    let isActive: Bool
    let isAdmin: Bool
    let emailVerified: Bool
    let createdAt: String
    let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id, email, username, bio
        case firstName = "first_name"
        case lastName = "last_name"
        case avatarURL = "avatar_url"
        case isActive = "is_active"
        case isAdmin = "is_admin"
        case emailVerified = "email_verified"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    var displayName: String {
        if let first = firstName, !first.isEmpty {
            if let last = lastName, !last.isEmpty {
                return "\(first) \(last)"
            }
            return first
        }
        return email ?? "User"
    }
}

struct ProfileResponse: Codable {
    let success: Bool
    let user: User?
    let message: String?
    let errorCode: String?
    let requestId: String?

    enum CodingKeys: String, CodingKey {
        case success, user, message
        case errorCode = "error_code"
        case requestId = "request_id"
    }
}
