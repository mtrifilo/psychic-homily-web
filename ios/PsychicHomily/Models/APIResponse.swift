import Foundation

struct APIErrorResponse: Codable, Sendable {
    let success: Bool
    let message: String?
    let errorCode: String?
    let requestId: String?

    enum CodingKeys: String, CodingKey {
        case success, message
        case errorCode = "error_code"
        case requestId = "request_id"
    }
}

struct AuthResponse: Codable, Sendable {
    let success: Bool
    let message: String?
    let token: String?
    let user: User?
    let errorCode: String?
    let requestId: String?

    enum CodingKeys: String, CodingKey {
        case success, message, token, user
        case errorCode = "error_code"
        case requestId = "request_id"
    }
}

struct RefreshTokenResponse: Codable, Sendable {
    let success: Bool
    let token: String?
    let message: String?
    let errorCode: String?

    enum CodingKeys: String, CodingKey {
        case success, token, message
        case errorCode = "error_code"
    }
}
